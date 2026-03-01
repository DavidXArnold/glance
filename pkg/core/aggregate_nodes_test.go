package core

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestComputeNodeSnapshot_SingleReadyNode(t *testing.T) {
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{{
				Type:   v1.NodeReady,
				Status: v1.ConditionTrue,
			}},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
		},
	}

	// One pod on the node with simple requests/limits.
	pod := v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "node-1",
			Containers: []v1.Container{{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(500, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(512*1024*1024, resource.BinarySI),
					},
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
						v1.ResourceMemory: *resource.NewQuantity(1024*1024*1024, resource.BinarySI),
					},
				},
			}},
		},
	}

	podsByNode := map[string][]v1.Pod{
		"node-1": {pod},
	}

	// Node metrics with some usage.
	metrics := &metricsV1beta1api.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Usage: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(250, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(256*1024*1024, resource.BinarySI),
		},
	}

	nodeMetrics := map[string]*metricsV1beta1api.NodeMetrics{
		"node-1": metrics,
	}

	opts := NodeSnapshotOptions{RequireMetrics: true}
	nm, totals, err := ComputeNodeSnapshot([]v1.Node{node}, podsByNode, nodeMetrics, opts)
	if err != nil {
		t.Fatalf("ComputeNodeSnapshot returned error: %v", err)
	}

	if len(nm) != 1 {
		t.Fatalf("expected 1 node in NodeMap, got %d", len(nm))
	}

	stats := nm["node-1"]
	if stats == nil {
		t.Fatalf("expected stats for node-1, got nil")
	}
	if stats.Status != "Ready" {
		t.Errorf("expected status Ready, got %q", stats.Status)
	}

	// Check allocated resources.
	if stats.AllocatedCPUrequests.MilliValue() != 500 {
		t.Errorf("expected CPU requests 500m, got %dm", stats.AllocatedCPUrequests.MilliValue())
	}
	if stats.AllocatedCPULimits.MilliValue() != 1000 {
		t.Errorf("expected CPU limits 1000m, got %dm", stats.AllocatedCPULimits.MilliValue())
	}

	if stats.AllocatedMemoryRequests.Value() != 512*1024*1024 {
		t.Errorf("expected memory requests 512Mi, got %d", stats.AllocatedMemoryRequests.Value())
	}
	if stats.AllocatedMemoryLimits.Value() != 1024*1024*1024 {
		t.Errorf("expected memory limits 1024Mi, got %d", stats.AllocatedMemoryLimits.Value())
	}

	// Check usage propagated from metrics.
	if stats.UsageCPU == nil || stats.UsageCPU.MilliValue() != 250 {
		t.Errorf("expected CPU usage 250m, got %v", stats.UsageCPU)
	}
	if stats.UsageMemory == nil || stats.UsageMemory.Value() != 256*1024*1024 {
		t.Errorf("expected memory usage 256Mi, got %v", stats.UsageMemory)
	}

	// Totals should match single-node values.
	if totals.TotalAllocatableCPU.MilliValue() != 4000 {
		t.Errorf("expected total allocatable CPU 4000m, got %dm", totals.TotalAllocatableCPU.MilliValue())
	}
	if totals.TotalAllocatedCPUrequests.MilliValue() != 500 {
		t.Errorf("expected total CPU requests 500m, got %dm", totals.TotalAllocatedCPUrequests.MilliValue())
	}
	if totals.TotalUsageCPU.MilliValue() != 250 {
		t.Errorf("expected total CPU usage 250m, got %dm", totals.TotalUsageCPU.MilliValue())
	}
}

func TestComputeNodeSnapshot_NotReadyExcludedFromTotals(t *testing.T) {
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-notready"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{{
				Type:   v1.NodeReady,
				Status: v1.ConditionFalse,
			}},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
		},
	}

	opts := NodeSnapshotOptions{RequireMetrics: false}
	nm, totals, err := ComputeNodeSnapshot([]v1.Node{node}, nil, nil, opts)
	if err != nil {
		t.Fatalf("ComputeNodeSnapshot returned error: %v", err)
	}

	if len(nm) != 1 {
		t.Fatalf("expected 1 node in NodeMap, got %d", len(nm))
	}

	stats := nm["node-notready"]
	if stats == nil {
		t.Fatalf("expected stats for node-notready, got nil")
	}
	if stats.Status != "Not Ready" {
		t.Errorf("expected status Not Ready, got %q", stats.Status)
	}

	// Totals should remain zeroed for NotReady-only snapshot.
	if totals.TotalAllocatableCPU.MilliValue() != 0 {
		t.Errorf("expected total allocatable CPU 0, got %dm", totals.TotalAllocatableCPU.MilliValue())
	}
}

func TestComputeNodeSnapshot_GPUResources(t *testing.T) {
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{{
				Type:   v1.NodeReady,
				Status: v1.ConditionTrue,
			}},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(8000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
				ResourceNvidiaGPU: *resource.NewQuantity(4, resource.DecimalSI),
			},
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(8000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
				ResourceNvidiaGPU: *resource.NewQuantity(4, resource.DecimalSI),
			},
		},
	}

	// Pod requesting 2 GPUs.
	pod := v1.Pod{
		Spec: v1.PodSpec{
			NodeName: "gpu-node",
			Containers: []v1.Container{{
				Resources: v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(1000, resource.DecimalSI),
						ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewMilliQuantity(2000, resource.DecimalSI),
						ResourceNvidiaGPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
				},
			}},
		},
	}

	podsByNode := map[string][]v1.Pod{"gpu-node": {pod}}

	opts := NodeSnapshotOptions{RequireMetrics: false}
	nm, totals, err := ComputeNodeSnapshot([]v1.Node{node}, podsByNode, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := nm["gpu-node"]
	if stats == nil {
		t.Fatal("expected stats for gpu-node")
	}

	// Node-level GPU allocatable/capacity.
	if stats.AllocatableGPU == nil || stats.AllocatableGPU.Value() != 4 {
		t.Errorf("expected allocatable GPU 4, got %v", stats.AllocatableGPU)
	}
	if stats.CapacityGPU == nil || stats.CapacityGPU.Value() != 4 {
		t.Errorf("expected capacity GPU 4, got %v", stats.CapacityGPU)
	}

	// Pod-level GPU requests/limits.
	if stats.AllocatedGPURequests.Value() != 2 {
		t.Errorf("expected GPU requests 2, got %d", stats.AllocatedGPURequests.Value())
	}
	if stats.AllocatedGPULimits.Value() != 2 {
		t.Errorf("expected GPU limits 2, got %d", stats.AllocatedGPULimits.Value())
	}

	// Totals.
	if totals.TotalAllocatableGPU == nil || totals.TotalAllocatableGPU.Value() != 4 {
		t.Errorf("expected total allocatable GPU 4, got %v", totals.TotalAllocatableGPU)
	}
	if totals.TotalCapacityGPU == nil || totals.TotalCapacityGPU.Value() != 4 {
		t.Errorf("expected total capacity GPU 4, got %v", totals.TotalCapacityGPU)
	}
	if totals.TotalAllocatedGPURequests.Value() != 2 {
		t.Errorf("expected total GPU requests 2, got %d", totals.TotalAllocatedGPURequests.Value())
	}
	if totals.TotalAllocatedGPULimits.Value() != 2 {
		t.Errorf("expected total GPU limits 2, got %d", totals.TotalAllocatedGPULimits.Value())
	}
}

func TestComputeNodeSnapshot_NoGPU(t *testing.T) {
	// Verify that clusters without GPUs produce zero/nil GPU fields.
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "cpu-node"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{{
				Type:   v1.NodeReady,
				Status: v1.ConditionTrue,
			}},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			},
		},
	}

	opts := NodeSnapshotOptions{RequireMetrics: false}
	nm, totals, err := ComputeNodeSnapshot([]v1.Node{node}, nil, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := nm["cpu-node"]
	if stats.AllocatableGPU != nil {
		t.Errorf("expected nil allocatable GPU for non-GPU node, got %v", stats.AllocatableGPU)
	}
	if stats.CapacityGPU != nil {
		t.Errorf("expected nil capacity GPU for non-GPU node, got %v", stats.CapacityGPU)
	}
	if stats.AllocatedGPURequests.Value() != 0 {
		t.Errorf("expected 0 GPU requests, got %d", stats.AllocatedGPURequests.Value())
	}

	// Totals GPU should be zero.
	if totals.TotalAllocatableGPU.Value() != 0 {
		t.Errorf("expected total allocatable GPU 0, got %d", totals.TotalAllocatableGPU.Value())
	}
}

func TestIsGPUResource(t *testing.T) {
	tests := []struct {
		name     v1.ResourceName
		expected bool
	}{
		{ResourceNvidiaGPU, true},
		{ResourceAMDGPU, true},
		{"intel.com/gpu", true},
		{"custom.vendor/gpu", true},
		{v1.ResourceCPU, false},
		{v1.ResourceMemory, false},
		{"nvidia.com/mig-1g.5gb", false},
	}

	for _, tc := range tests {
		got := IsGPUResource(tc.name)
		if got != tc.expected {
			t.Errorf("IsGPUResource(%q) = %v, want %v", tc.name, got, tc.expected)
		}
	}
}
