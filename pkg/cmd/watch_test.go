/*
Copyright 2025 David Arnold
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// syntheticCluster returns nodes, pods (podsPerNode on each node, spread over
// namespaces, a few Succeeded) and namespaces shaped like a real cluster's.
func syntheticCluster(numNodes, podsPerNode, numNamespaces int) []runtime.Object {
	objs := make([]runtime.Object, 0, numNodes*(podsPerNode+1)+numNamespaces)
	for i := 0; i < numNamespaces; i++ {
		objs = append(objs, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("ns-%03d", i)}})
	}
	for n := 0; n < numNodes; n++ {
		nodeName := fmt.Sprintf("node-%05d", n)
		objs = append(objs, &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"pool": fmt.Sprintf("p%d", n%4)}},
			Status: v1.NodeStatus{
				Allocatable: v1.ResourceList{v1.ResourceCPU: resource.MustParse("16"), v1.ResourceMemory: resource.MustParse("64Gi")},
				Conditions:  []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}},
			},
		})
		for p := 0; p < podsPerNode; p++ {
			phase := v1.PodRunning
			if p == 0 {
				phase = v1.PodSucceeded
			}
			objs = append(objs, &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID:             types.UID(fmt.Sprintf("uid-%05d-%03d", n, p)),
					ResourceVersion: "1",
					Name:            fmt.Sprintf("pod-%05d-%03d", n, p),
					Namespace:       fmt.Sprintf("ns-%03d", (n*podsPerNode+p)%numNamespaces),
					Labels:          map[string]string{"app": fmt.Sprintf("app-%d", p%10)},
					Annotations: map[string]string{
						lastAppliedAnnotation: `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"x"}}`,
					},
					ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}},
				},
				Spec: v1.PodSpec{
					NodeName: nodeName,
					Containers: []v1.Container{{
						Name:    "app",
						Command: []string{"/bin/app", "--serve"},
						Env:     []v1.EnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}},
						Resources: v1.ResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceCPU: resource.MustParse("100m"), v1.ResourceMemory: resource.MustParse("128Mi"),
						}},
					}},
					Volumes: []v1.Volume{{Name: "data"}},
				},
				Status: v1.PodStatus{Phase: phase},
			})
		}
	}
	return objs
}

func startedCache(t testing.TB, objs ...runtime.Object) *WatchCache {
	t.Helper()
	wc := NewWatchCache(fake.NewClientset(objs...))
	if err := wc.Start(context.Background(), 30*time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(wc.Stop)
	return wc
}

func podNames(pods []*v1.Pod) []string {
	names := make([]string, len(pods))
	for i, p := range pods {
		names[i] = p.Namespace + "/" + p.Name
	}
	sort.Strings(names)
	return names
}

// TestWatchCacheMatchesAPI checks that the cache returns the same objects as
// direct API lists for the selectors live mode and the GPU view use.
func TestWatchCacheMatchesAPI(t *testing.T) {
	objs := syntheticCluster(8, 5, 3)
	api := apiLister{client: fake.NewClientset(objs...)}
	wc := startedCache(t, objs...)
	ctx := context.Background()

	cases := []struct {
		name      string
		namespace string
		label     labels.Selector
	}{
		{name: "all pods"},
		{name: "one namespace", namespace: "ns-001"},
		{name: "label", label: labels.SelectorFromSet(labels.Set{"app": "app-3"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := api.ListPods(ctx, tc.namespace, tc.label, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := wc.ListPods(ctx, tc.namespace, tc.label, nil)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(podNames(got)) != fmt.Sprint(podNames(want)) {
				t.Errorf("cache returned %d pods, API %d", len(got), len(want))
			}
		})
	}

	// The fake clientset ignores field selectors, so check field selection
	// against known counts instead: 40 pods, 8 of them Succeeded.
	running, _ := wc.ListPods(ctx, "", nil, nonTerminatedPodSelector)
	if len(running) != 32 {
		t.Errorf("non-terminated pods = %d, want 32", len(running))
	}
	onNode, _ := wc.ListPods(ctx, "", nil, fields.OneTermEqualSelector("spec.nodeName", "node-00002"))
	if len(onNode) != 5 {
		t.Errorf("pods on node-00002 = %d, want 5", len(onNode))
	}

	nodes, _ := wc.ListNodes(ctx, labels.SelectorFromSet(labels.Set{"pool": "p1"}), nil)
	if len(nodes) != 2 {
		t.Errorf("nodes in pool p1 = %d, want 2", len(nodes))
	}
	named, _ := wc.ListNodes(ctx, nil, fields.OneTermEqualSelector("metadata.name", "node-00007"))
	if len(named) != 1 {
		t.Errorf("nodes named node-00007 = %d, want 1", len(named))
	}

	gotNS, _ := wc.ListNamespaces(ctx)
	wantNS, _ := api.ListNamespaces(ctx)
	if fmt.Sprint(gotNS) != fmt.Sprint(wantNS) {
		t.Errorf("namespaces = %v, want %v", gotNS, wantNS)
	}
}

// TestWatchCacheSeesChanges checks that pods added after the initial sync
// show up without a re-list.
func TestWatchCacheSeesChanges(t *testing.T) {
	client := fake.NewClientset()
	wc := NewWatchCache(client)
	if err := wc.Start(context.Background(), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	defer wc.Stop()

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "late", Namespace: "default"}}
	if _, err := client.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pods, _ := wc.ListPods(context.Background(), "default", nil, nil); len(pods) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pod created after sync never appeared in the cache")
}

func TestWatchCacheStartTimesOut(t *testing.T) {
	wc := NewWatchCache(fake.NewClientset())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // a canceled context can never sync
	if err := wc.Start(ctx, time.Second); err == nil {
		t.Fatal("expected Start to fail when the cache cannot sync")
	}
	wc.Stop() // safe after a failed Start
}

func TestStripUnusedFields(t *testing.T) {
	objs := syntheticCluster(1, 2, 1)
	pod := objs[2].(*v1.Pod).DeepCopy()
	pod.Annotations["kubernetes.io/config.mirror"] = "abc"
	pod.Spec.InitContainers = []v1.Container{{Name: "init", Env: []v1.EnvVar{{Name: "X"}}}}

	if _, err := stripUnusedFields(pod); err != nil {
		t.Fatal(err)
	}
	if pod.ManagedFields != nil || pod.Annotations[lastAppliedAnnotation] != "" {
		t.Error("managedFields and the last-applied annotation should be dropped")
	}
	if pod.Annotations["kubernetes.io/config.mirror"] != "abc" {
		t.Error("other annotations (used to detect mirror pods) must be kept")
	}
	c := pod.Spec.Containers[0]
	if c.Env != nil || c.Command != nil || pod.Spec.Volumes != nil || pod.Spec.InitContainers[0].Env != nil {
		t.Error("container env, command and pod volumes should be dropped")
	}
	if c.Resources.Requests.Cpu().MilliValue() != 100 || pod.Spec.NodeName == "" || pod.Labels["app"] == "" {
		t.Error("resources, node name and labels must be kept")
	}

	// Non-objects (deletion tombstones) pass through.
	if _, err := stripUnusedFields("tombstone"); err != nil {
		t.Error(err)
	}
}

func TestMetricsCacheInterval(t *testing.T) {
	// The metrics fake's object tracker does not serve seeded PodMetrics, so
	// answer pod metrics lists directly.
	all := []metricsv1beta1.PodMetrics{
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns2"}},
	}
	client := metricsfake.NewSimpleClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		out := &metricsv1beta1.PodMetricsList{}
		for _, pm := range all {
			if ns := action.GetNamespace(); ns == "" || ns == pm.Namespace {
				out.Items = append(out.Items, pm)
			}
		}
		return true, out, nil
	})
	now := time.Unix(1000, 0)
	mc := newMetricsCache(client, 10*time.Second)
	mc.now = func() time.Time { return now }
	ctx := context.Background()

	lists := func() int {
		n := 0
		for _, a := range client.Actions() {
			if a.GetVerb() == "list" {
				n++
			}
		}
		return n
	}

	if _, err := mc.PodMetrics(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := mc.NodeMetrics(ctx); err != nil {
		t.Fatal(err)
	}
	ns2, _ := mc.PodMetrics(ctx, "ns2")
	_, _ = mc.PodMetrics(ctx, "")
	_, _ = mc.NodeMetrics(ctx)
	if got := lists(); got != 2 {
		t.Errorf("within the interval: %d list calls, want 2 (one node, one pod)", got)
	}
	if len(ns2.Items) != 1 || ns2.Items[0].Name != "b" {
		t.Errorf("ns2 served from the all-namespaces result = %v", ns2.Items)
	}

	now = now.Add(11 * time.Second)
	_, _ = mc.PodMetrics(ctx, "")
	_, _ = mc.NodeMetrics(ctx)
	if got := lists(); got != 4 {
		t.Errorf("after the interval: %d list calls, want 4", got)
	}
}

// BenchmarkLivePodRefresh compares what one live refresh costs the client
// for the pod list on a 1,000-node, 30,000-pod cluster: decoding a full API
// list response (what every refresh did before) versus reading the informer
// cache. The API server's own cost of serializing the list is not included.
func BenchmarkLivePodRefresh(b *testing.B) {
	objs := syntheticCluster(1000, 30, 50)
	list := &v1.PodList{}
	for _, o := range objs {
		if p, ok := o.(*v1.Pod); ok {
			list.Items = append(list.Items, *p)
		}
	}
	body, err := json.Marshal(list)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("api-list-decode", func(b *testing.B) {
		b.ReportMetric(float64(len(body))/(1<<20), "MiB/response")
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var out v1.PodList
			if err := json.Unmarshal(body, &out); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("watch-cache", func(b *testing.B) {
		wc := startedCache(b, objs...)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := wc.ListPods(context.Background(), "", nil, nonTerminatedPodSelector); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestWatchCacheForbiddenWatchFallsBack checks that with list but not watch
// permission the cache stops serving (stale) data and lists from the API.
func TestWatchCacheForbiddenWatchFallsBack(t *testing.T) {
	client := fake.NewClientset(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}})
	client.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("no watch"))
	})
	wc := NewWatchCache(client)
	if err := wc.Start(context.Background(), 10*time.Second); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer wc.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for wc.usable() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if wc.usable() {
		t.Fatal("cache still in use after the pod watch was forbidden")
	}

	// A pod created now is visible through the fallback API lister.
	late := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"}}
	if _, err := client.CoreV1().Pods("default").Create(context.Background(), late, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	pods, err := wc.ListPods(context.Background(), "default", nil, nil)
	if err != nil || len(pods) != 2 {
		t.Fatalf("ListPods after fallback = %d pods, %v; want 2", len(pods), err)
	}
}

// benchLiveState returns live state backed by a synced cache of a synthetic
// cluster, with no metrics.
func benchLiveState(b *testing.B, numNodes, podsPerNode int) *LiveState {
	b.Helper()
	wc := startedCache(b, syntheticCluster(numNodes, podsPerNode, 50)...)
	return &LiveState{
		lister:        wc,
		metrics:       newMetricsCache(metricsfake.NewSimpleClientset(), defaultMetricsMinInterval),
		maxConcurrent: defaultMaxConcurrent,
		nodeLimit:     defaultNodeLimit,
		podLimit:      defaultPodLimit,
		sortMode:      SortByStatus,
		showBars:      true,
	}
}

// BenchmarkLiveRefresh measures one live refresh per view against a synced
// cache of 1,000 nodes and 30,000 pods: the CPU glance spends per tick.
func BenchmarkLiveRefresh(b *testing.B) {
	state := benchLiveState(b, 1000, 30)
	ctx := context.Background()
	views := []struct {
		name  string
		fetch func() error
	}{
		{"nodes", func() error { _, _, _, err := fetchNodeData(ctx, state); return err }},
		{"namespaces", func() error { _, _, _, err := fetchNamespaceData(ctx, state); return err }},
		{"pods-all", func() error { _, _, _, err := fetchPodData(ctx, "", state); return err }},
		{"gpus", func() error { _, _, err := fetchGPUData(ctx, state.lister); return err }},
	}
	for _, v := range views {
		b.Run(v.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := v.fetch(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
