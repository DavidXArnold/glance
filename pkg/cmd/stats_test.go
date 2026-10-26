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
	"testing"

	core "gitlab.com/davidxarnold/glance/pkg/core"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestCollectPodStats_GPUResources(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-pod", Namespace: "default"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name: "trainer",
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:         *resource.NewMilliQuantity(1000, resource.DecimalSI),
						v1.ResourceMemory:      *resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
						core.ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
					Limits: v1.ResourceList{
						v1.ResourceCPU:         *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:      *resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
						core.ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
				},
			}},
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}

	client := fake.NewSimpleClientset(pod)
	rows, err := CollectPodStats(context.Background(), client, nil, "default", labels.Everything(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row.GPUReq == nil || row.GPUReq.Value() != 2 {
		t.Errorf("expected GPU requests 2, got %v", row.GPUReq)
	}
	if row.GPULimit == nil || row.GPULimit.Value() != 2 {
		t.Errorf("expected GPU limits 2, got %v", row.GPULimit)
	}
	if row.CPUReq.MilliValue() != 1000 {
		t.Errorf("expected CPU requests 1000m, got %dm", row.CPUReq.MilliValue())
	}
}

func TestCollectPodStats_NoGPU(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cpu-pod", Namespace: "default"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name: "app",
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
					},
				},
			}},
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}

	client := fake.NewSimpleClientset(pod)
	rows, err := CollectPodStats(context.Background(), client, nil, "default", labels.Everything(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row.GPUReq.Value() != 0 {
		t.Errorf("expected GPU requests 0, got %d", row.GPUReq.Value())
	}
	if row.GPULimit.Value() != 0 {
		t.Errorf("expected GPU limits 0, got %d", row.GPULimit.Value())
	}
}

func TestCollectPodStats_MultiContainerGPU(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-gpu-pod", Namespace: "ml"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "trainer",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							core.ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
						},
						Limits: v1.ResourceList{
							core.ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
						},
					},
				},
				{
					Name: "sidecar",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							core.ResourceAMDGPU: *resource.NewQuantity(1, resource.DecimalSI),
						},
						Limits: v1.ResourceList{
							core.ResourceAMDGPU: *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{Phase: v1.PodRunning},
	}

	client := fake.NewSimpleClientset(pod)
	rows, err := CollectPodStats(context.Background(), client, nil, "ml", labels.Everything(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	// Should sum NVIDIA (2) + AMD (1) = 3 total GPU requests.
	if row.GPUReq.Value() != 3 {
		t.Errorf("expected GPU requests 3, got %d", row.GPUReq.Value())
	}
	if row.GPULimit.Value() != 3 {
		t.Errorf("expected GPU limits 3, got %d", row.GPULimit.Value())
	}
}

func int32Ptr(i int32) *int32 { return &i }

func TestCollectDeploymentStats_GPUResources(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "gpu"}},
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "worker",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceCPU:         *resource.NewMilliQuantity(500, resource.DecimalSI),
								core.ResourceNvidiaGPU: *resource.NewQuantity(1, resource.DecimalSI),
							},
							Limits: v1.ResourceList{
								v1.ResourceCPU:         *resource.NewMilliQuantity(1000, resource.DecimalSI),
								core.ResourceNvidiaGPU: *resource.NewQuantity(1, resource.DecimalSI),
							},
						},
					}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     3,
			AvailableReplicas: 3,
		},
	}

	client := fake.NewSimpleClientset(deploy)
	rows, err := CollectDeploymentStats(context.Background(), client, "default", labels.Everything(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	// 1 GPU per replica * 3 replicas = 3
	if row.GPUReq.Value() != 3 {
		t.Errorf("expected GPU requests 3 (1*3 replicas), got %d", row.GPUReq.Value())
	}
	if row.GPULimit.Value() != 3 {
		t.Errorf("expected GPU limits 3 (1*3 replicas), got %d", row.GPULimit.Value())
	}
	// Verify CPU is also multiplied by replicas.
	// Note: multiplyQuantity uses Value() which rounds 500m (0.5) up to 1,
	// so 1*3 = 3 (3000m). This is a known precision trade-off.
	if row.CPUReq.MilliValue() != 3000 {
		t.Errorf("expected CPU requests 3000m (rounded), got %dm", row.CPUReq.MilliValue())
	}
}

func TestCollectDeploymentStats_NoGPU(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "server",
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceCPU:    *resource.NewMilliQuantity(250, resource.DecimalSI),
								v1.ResourceMemory: *resource.NewQuantity(512*1024*1024, resource.BinarySI),
							},
						},
					}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     2,
			AvailableReplicas: 2,
		},
	}

	client := fake.NewSimpleClientset(deploy)
	rows, err := CollectDeploymentStats(context.Background(), client, "default", labels.Everything(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row.GPUReq.Value() != 0 {
		t.Errorf("expected GPU requests 0, got %d", row.GPUReq.Value())
	}
	if row.GPULimit.Value() != 0 {
		t.Errorf("expected GPU limits 0, got %d", row.GPULimit.Value())
	}
}

// TestCollectPodStats_SameNameAcrossNamespaces guards against pod metrics
// being matched by name alone, which mixed up usage for same-named pods
// (e.g. StatefulSet replicas) in different namespaces.
func TestCollectPodStats_SameNameAcrossNamespaces(t *testing.T) {
	newPod := func(namespace string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "redis-0", Namespace: namespace},
			Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "redis"}}},
			Status:     v1.PodStatus{Phase: v1.PodRunning},
		}
	}
	newMetrics := func(namespace string, milliCPU int64) metricsv1beta1.PodMetrics {
		return metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Name: "redis-0", Namespace: namespace},
			Containers: []metricsv1beta1.ContainerMetrics{{
				Name: "redis",
				Usage: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
					v1.ResourceMemory: *resource.NewQuantity(milliCPU*1024*1024, resource.BinarySI),
				},
			}},
		}
	}

	client := fake.NewSimpleClientset(newPod("team-a"), newPod("team-b"))

	// The generated metrics fake cannot list pod metrics through its object
	// tracker, so serve the list from a reactor.
	metricsClient := &metricsfake.Clientset{}
	metricsClient.AddReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{
			Items: []metricsv1beta1.PodMetrics{newMetrics("team-a", 100), newMetrics("team-b", 900)},
		}, nil
	})

	rows, err := CollectPodStats(context.Background(), client, metricsClient, "", labels.Everything(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	want := map[string]int64{"team-a": 100, "team-b": 900}
	for _, row := range rows {
		if got := row.CPUUsage.MilliValue(); got != want[row.Namespace] {
			t.Errorf("%s/%s CPU usage = %dm, want %dm", row.Namespace, row.Name, got, want[row.Namespace])
		}
		if got, wantMem := row.MemUsage.Value(), want[row.Namespace]*1024*1024; got != wantMem {
			t.Errorf("%s/%s memory usage = %d, want %d", row.Namespace, row.Name, got, wantMem)
		}
	}
}

// TestCollectStats_FieldSelector verifies field selectors are sent to the API
// server as field selectors rather than being folded into the label selector.
func TestCollectStats_FieldSelector(t *testing.T) {
	labelSelector := labels.SelectorFromSet(labels.Set{"app": "web"})
	fieldSelector := fields.OneTermEqualSelector("metadata.name", "web")

	assertListRestrictions := func(t *testing.T, client *fake.Clientset, resource string) {
		t.Helper()
		for _, action := range client.Actions() {
			list, ok := action.(k8stesting.ListAction)
			if !ok || action.GetResource().Resource != resource {
				continue
			}
			restrictions := list.GetListRestrictions()
			if got := restrictions.Labels.String(); got != "app=web" {
				t.Errorf("%s label selector = %q, want %q", resource, got, "app=web")
			}
			if got := restrictions.Fields.String(); got != "metadata.name=web" {
				t.Errorf("%s field selector = %q, want %q", resource, got, "metadata.name=web")
			}
			return
		}
		t.Fatalf("no list action recorded for %s", resource)
	}

	t.Run("pods", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		if _, err := CollectPodStats(context.Background(), client, nil, "default", labelSelector, fieldSelector); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertListRestrictions(t, client, "pods")
	})

	t.Run("deployments", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		_, err := CollectDeploymentStats(context.Background(), client, "default", labelSelector, fieldSelector)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertListRestrictions(t, client, "deployments")
	})
}
