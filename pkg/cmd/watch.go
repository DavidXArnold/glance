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
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// clusterLister lists the nodes, pods and namespaces that live mode reads on
// every refresh. apiLister queries the API server each time; WatchCache serves
// the same data from informers, which list once and then stream changes.
type clusterLister interface {
	// ListNodes returns nodes matching the selectors (nil matches everything).
	ListNodes(ctx context.Context, label labels.Selector, field fields.Selector) ([]v1.Node, error)
	// ListPods returns pods in namespace ("" for all namespaces) matching the
	// selectors (nil matches everything).
	// The pods may be shared with the cache: callers must not modify them.
	ListPods(ctx context.Context, namespace string, label labels.Selector, field fields.Selector) ([]*v1.Pod, error)
	// ListNamespaces returns namespace names in sorted order.
	ListNamespaces(ctx context.Context) ([]string, error)
}

// nonTerminatedPodSelector selects pods that are not Succeeded or Failed.
var nonTerminatedPodSelector = fields.ParseSelectorOrDie(nonTerminatedPods)

// apiLister implements clusterLister with direct API calls.
type apiLister struct {
	client kubernetes.Interface
}

func (a apiLister) ListNodes(ctx context.Context, label labels.Selector, field fields.Selector) ([]v1.Node, error) {
	nodes, err := a.client.CoreV1().Nodes().List(ctx, buildListOptions(label, field))
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

func (a apiLister) ListPods(
	ctx context.Context, namespace string, label labels.Selector, field fields.Selector,
) ([]*v1.Pod, error) {
	pods, err := a.client.CoreV1().Pods(namespace).List(ctx, buildListOptions(label, field))
	if err != nil {
		return nil, err
	}
	return podPointers(pods.Items), nil
}

// podPointers returns pointers to the elements of pods.
func podPointers(pods []v1.Pod) []*v1.Pod {
	out := make([]*v1.Pod, len(pods))
	for i := range pods {
		out[i] = &pods[i]
	}
	return out
}

func (a apiLister) ListNamespaces(ctx context.Context) ([]string, error) {
	namespaces, err := a.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{ResourceVersion: "0"})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(namespaces.Items))
	for i := range namespaces.Items {
		names = append(names, namespaces.Items[i].Name)
	}
	sort.Strings(names)
	return names, nil
}

// WatchCache serves nodes, pods and namespaces from shared informers. After
// the initial list, the API server only streams changes, instead of live mode
// re-listing every pod in the cluster on every refresh. Reads come straight
// from the informer indexes; nothing is copied per event.
type WatchCache struct {
	factory    informers.SharedInformerFactory
	nodes      corelisters.NodeLister
	pods       corelisters.PodLister
	namespaces corelisters.NamespaceLister
	cancel     context.CancelFunc

	// fallback serves reads once a watch is forbidden: with list but not
	// watch permission the initial sync succeeds and the cache then goes
	// stale, so it must not be used.
	fallback  clusterLister
	forbidden atomic.Bool
}

// NewWatchCache creates an informer-based cache. Call Start before reading.
func NewWatchCache(client kubernetes.Interface) *WatchCache {
	// No periodic resync: the cache only needs to mirror the API server.
	factory := informers.NewSharedInformerFactoryWithOptions(client, 0,
		informers.WithTransform(stripUnusedFields))
	wc := &WatchCache{
		factory:    factory,
		nodes:      factory.Core().V1().Nodes().Lister(),
		pods:       factory.Core().V1().Pods().Lister(),
		namespaces: factory.Core().V1().Namespaces().Lister(),
		fallback:   apiLister{client: client},
	}
	for _, informer := range []cache.SharedIndexInformer{
		factory.Core().V1().Nodes().Informer(),
		factory.Core().V1().Pods().Informer(),
		factory.Core().V1().Namespaces().Informer(),
	} {
		_ = informer.SetWatchErrorHandlerWithContext(wc.handleWatchError)
	}
	return wc
}

// handleWatchError records forbidden watches and logs other watch errors at
// debug level; the reflector retries those on its own. (The default handler
// writes to stderr, which would corrupt the TUI.)
func (wc *WatchCache) handleWatchError(_ context.Context, r *cache.Reflector, err error) {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		if !wc.forbidden.Swap(true) {
			log.Warnf("watch not permitted (%v); listing on every refresh instead", err)
		}
		return
	}
	log.Debugf("watch %s: %v", r.TypeDescription(), err)
}

// usable reports whether reads can be served from the cache.
func (wc *WatchCache) usable() bool {
	return !wc.forbidden.Load()
}

// Start runs the informers and waits up to timeout for their initial sync.
// On error the informers are stopped and the caller should fall back to
// apiLister (for example when the user may list but not watch pods).
func (wc *WatchCache) Start(ctx context.Context, timeout time.Duration) error {
	ctx, wc.cancel = context.WithCancel(ctx)
	wc.factory.Start(ctx.Done())

	syncCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for informerType, ok := range wc.factory.WaitForCacheSync(syncCtx.Done()) {
		if !ok {
			wc.Stop()
			return fmt.Errorf("timed out after %s syncing the %v cache", timeout, informerType)
		}
	}
	return nil
}

// Stop stops the informers. It is safe to call more than once.
func (wc *WatchCache) Stop() {
	if wc.cancel != nil {
		wc.cancel()
	}
	wc.factory.Shutdown()
}

// ListNodes implements clusterLister.
func (wc *WatchCache) ListNodes(ctx context.Context, label labels.Selector, field fields.Selector) ([]v1.Node, error) {
	if !wc.usable() {
		return wc.fallback.ListNodes(ctx, label, field)
	}
	cached, err := wc.nodes.List(labelsOrEverything(label))
	if err != nil {
		return nil, err
	}
	matchAll := field == nil || field.Empty()
	set := make(fields.Set, 2)
	result := make([]v1.Node, 0, len(cached))
	for _, n := range cached {
		if matchAll || field.Matches(nodeFieldSet(n, set)) {
			result = append(result, *n)
		}
	}
	return result, nil
}

// ListPods implements clusterLister.
func (wc *WatchCache) ListPods(
	ctx context.Context, namespace string, label labels.Selector, field fields.Selector,
) ([]*v1.Pod, error) {
	if !wc.usable() {
		return wc.fallback.ListPods(ctx, namespace, label, field)
	}
	sel := labelsOrEverything(label)
	var cached []*v1.Pod
	var err error
	if namespace == metav1.NamespaceAll {
		cached, err = wc.pods.List(sel)
	} else {
		cached, err = wc.pods.Pods(namespace).List(sel)
	}
	if err != nil {
		return nil, err
	}
	// The cached objects are returned as is, without copying; callers only
	// read them.
	if field == nil || field.Empty() {
		return cached, nil
	}
	set := make(fields.Set, 10) // reused: Matches only reads it
	result := cached[:0:0]
	for _, p := range cached {
		if field.Matches(podFieldSet(p, set)) {
			result = append(result, p)
		}
	}
	return result, nil
}

// ListNamespaces implements clusterLister.
func (wc *WatchCache) ListNamespaces(ctx context.Context) ([]string, error) {
	if !wc.usable() {
		return wc.fallback.ListNamespaces(ctx)
	}
	cached, err := wc.namespaces.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cached))
	for _, ns := range cached {
		names = append(names, ns.Name)
	}
	sort.Strings(names)
	return names, nil
}

func labelsOrEverything(sel labels.Selector) labels.Selector {
	if sel == nil {
		return labels.Everything()
	}
	return sel
}

// nodeFieldSet fills set with the node fields the API server supports in
// field selectors and returns it.
func nodeFieldSet(n *v1.Node, set fields.Set) fields.Set {
	set["metadata.name"] = n.Name
	set["spec.unschedulable"] = strconv.FormatBool(n.Spec.Unschedulable)
	return set
}

// podFieldSet fills set with the pod fields the API server supports in field
// selectors and returns it.
func podFieldSet(p *v1.Pod, set fields.Set) fields.Set {
	set["metadata.name"] = p.Name
	set["metadata.namespace"] = p.Namespace
	set["spec.nodeName"] = p.Spec.NodeName
	set["spec.restartPolicy"] = string(p.Spec.RestartPolicy)
	set["spec.schedulerName"] = p.Spec.SchedulerName
	set["spec.serviceAccountName"] = p.Spec.ServiceAccountName
	set["spec.hostNetwork"] = strconv.FormatBool(p.Spec.HostNetwork)
	set["status.phase"] = string(p.Status.Phase)
	set["status.podIP"] = p.Status.PodIP
	set["status.nominatedNodeName"] = p.Status.NominatedNodeName
	return set
}

// lastAppliedAnnotation is kubectl apply's copy of the whole object.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// stripUnusedFields drops large fields glance never reads before objects
// enter the cache, which cuts its memory on large clusters. Resource
// requests, limits, status, labels, owner references and other annotations
// are kept.
func stripUnusedFields(obj interface{}) (interface{}, error) {
	accessor, ok := obj.(metav1.Object)
	if !ok {
		// Tombstones (cache.DeletedFinalStateUnknown) pass through unchanged.
		return obj, nil
	}
	accessor.SetManagedFields(nil)
	if annotations := accessor.GetAnnotations(); annotations[lastAppliedAnnotation] != "" {
		trimmed := make(map[string]string, len(annotations)-1)
		for k, v := range annotations {
			if k != lastAppliedAnnotation {
				trimmed[k] = v
			}
		}
		accessor.SetAnnotations(trimmed)
	}

	switch o := obj.(type) {
	case *v1.Pod:
		o.Spec.Volumes = nil
		for _, containers := range [][]v1.Container{o.Spec.InitContainers, o.Spec.Containers} {
			for i := range containers {
				stripContainer(&containers[i])
			}
		}
		o.Spec.EphemeralContainers = nil
	case *v1.Node:
		// Image lists are long on busy nodes and unused.
		o.Status.Images = nil
	}
	return obj, nil
}

// stripContainer drops container fields that do not affect resource
// accounting. Resources, RestartPolicy (sidecars) and the name are kept.
func stripContainer(c *v1.Container) {
	c.Command = nil
	c.Args = nil
	c.Env = nil
	c.EnvFrom = nil
	c.VolumeMounts = nil
	c.VolumeDevices = nil
	c.Ports = nil
	c.LivenessProbe = nil
	c.ReadinessProbe = nil
	c.StartupProbe = nil
	c.Lifecycle = nil
	c.SecurityContext = nil
}

// Compile-time interface checks.
var (
	_ clusterLister       = apiLister{}
	_ clusterLister       = (*WatchCache)(nil)
	_ cache.TransformFunc = stripUnusedFields
)
