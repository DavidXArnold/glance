package core

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsGPUResource(t *testing.T) {
	tests := []struct {
		name     v1.ResourceName
		expected bool
	}{
		{ResourceNvidiaGPU, true},
		{ResourceNvidiaGPUShared, true},
		{"nvidia.com/mig-1g.5gb", true},
		{"nvidia.com/mig-3g.20gb", true},
		{ResourceAMDGPU, true},
		{ResourceIntelGPUi915, true},
		{ResourceIntelGPUXe, true},
		{ResourceAWSNeuron, true},
		{ResourceAWSNeuronCore, true},
		{ResourceAWSNeuronDevice, true},
		{ResourceHabanaGaudi, true},
		{ResourceGoogleTPU, true},
		{"intel.com/gpu", true},
		{"custom.vendor/gpu", true},
		{v1.ResourceCPU, false},
		{v1.ResourceMemory, false},
		{v1.ResourceEphemeralStorage, false},
		{"hugepages-2Mi", false},
		{"nvidia.com/gpu-memory", false},
	}

	for _, tc := range tests {
		if got := IsGPUResource(tc.name); got != tc.expected {
			t.Errorf("IsGPUResource(%q) = %v, want %v", tc.name, got, tc.expected)
		}
	}
}

func TestSetExtraGPUResources(t *testing.T) {
	t.Cleanup(func() { _ = SetExtraGPUResources(nil) })

	if IsGPUResource("example.com/npu") {
		t.Fatal("example.com/npu matched before being registered")
	}
	if err := SetExtraGPUResources([]string{"example.com/*", " ", "vendor.io/fpga"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range []v1.ResourceName{"example.com/npu", "example.com/accel", "vendor.io/fpga"} {
		if !IsGPUResource(name) {
			t.Errorf("IsGPUResource(%q) = false after registering patterns", name)
		}
	}
	if IsGPUResource("vendor.io/other") {
		t.Error("vendor.io/other matched without a pattern for it")
	}

	if err := SetExtraGPUResources([]string{"bad[pattern"}); err == nil {
		t.Error("expected error for invalid pattern")
	}
}

func TestGPUModelFromLabels(t *testing.T) {
	tests := []struct {
		name      string
		labels    map[string]string
		wantModel string
		wantMiB   int64
	}{
		{name: "no labels"},
		{
			name: "nvidia gpu feature discovery",
			labels: map[string]string{
				LabelNvidiaGPUProduct: "NVIDIA-A100-SXM4-80GB",
				LabelNvidiaGPUMemory:  "81920",
				LabelGKEAccelerator:   "nvidia-tesla-a100",
			},
			wantModel: "NVIDIA-A100-SXM4-80GB",
			wantMiB:   81920,
		},
		{
			name:      "gke",
			labels:    map[string]string{LabelGKEAccelerator: "nvidia-l4"},
			wantModel: "nvidia-l4",
		},
		{
			name: "karpenter",
			labels: map[string]string{
				LabelKarpenterGPUName:  "a10g",
				LabelKarpenterGPUMaker: "nvidia",
				LabelKarpenterGPUMem:   "24576",
			},
			wantModel: "nvidia-a10g",
			wantMiB:   24576,
		},
		{
			name:      "bad memory label",
			labels:    map[string]string{LabelNvidiaGPUProduct: "Tesla-T4", LabelNvidiaGPUMemory: "lots"},
			wantModel: "Tesla-T4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, mib := GPUModelFromLabels(tt.labels)
			if model != tt.wantModel || mib != tt.wantMiB {
				t.Errorf("GPUModelFromLabels() = (%q, %d), want (%q, %d)", model, mib, tt.wantModel, tt.wantMiB)
			}
		})
	}
}

func readyNode(name string, labels map[string]string, resources v1.ResourceList) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: v1.NodeStatus{
			Conditions:  []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}},
			Allocatable: resources,
			Capacity:    resources,
		},
	}
}

func qty(n int64) resource.Quantity { return *resource.NewQuantity(n, resource.DecimalSI) }

func milliCPU(n int64) resource.Quantity { return *resource.NewMilliQuantity(n, resource.DecimalSI) }

func requests(list v1.ResourceList) v1.ResourceRequirements {
	return v1.ResourceRequirements{Requests: list, Limits: list}
}

// TestComputeNodeSnapshot_PerTypeAccelerators covers a node exposing full
// GPUs and MIG slices side by side ("mixed" MIG strategy).
func TestComputeNodeSnapshot_PerTypeAccelerators(t *testing.T) {
	node := readyNode("mig-node", map[string]string{LabelNvidiaGPUProduct: "NVIDIA-A100-SXM4-80GB"}, v1.ResourceList{
		v1.ResourceCPU:           milliCPU(32000),
		ResourceNvidiaGPU:        qty(1),
		"nvidia.com/mig-1g.10gb": qty(7),
	})
	podRequesting := func(list v1.ResourceList) v1.Pod {
		return v1.Pod{Spec: v1.PodSpec{Containers: []v1.Container{{Resources: requests(list)}}}}
	}
	pods := []v1.Pod{
		podRequesting(v1.ResourceList{"nvidia.com/mig-1g.10gb": qty(3)}),
		podRequesting(v1.ResourceList{ResourceNvidiaGPU: qty(1)}),
	}

	podsByNode := map[string][]v1.Pod{"mig-node": pods}
	nm, totals, err := ComputeNodeSnapshot([]v1.Node{node}, podsByNode, nil, NodeSnapshotOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := nm["mig-node"]
	if stats.GPUModel != "NVIDIA-A100-SXM4-80GB" {
		t.Errorf("GPUModel = %q", stats.GPUModel)
	}
	want := AcceleratorMap{
		ResourceNvidiaGPU:        {Allocatable: 1, Capacity: 1, Requests: 1, Limits: 1},
		"nvidia.com/mig-1g.10gb": {Allocatable: 7, Capacity: 7, Requests: 3, Limits: 3},
	}
	for name, w := range want {
		got := stats.Accelerators[name]
		if got == nil || *got != *w {
			t.Errorf("node accelerators[%s] = %+v, want %+v", name, got, w)
		}
		if tg := totals.Accelerators[name]; tg == nil || *tg != *w {
			t.Errorf("total accelerators[%s] = %+v, want %+v", name, tg, w)
		}
	}
	// Summed fields stay for backwards compatibility.
	if stats.AllocatableGPU.Value() != 8 || stats.AllocatedGPURequests.Value() != 4 {
		t.Errorf("summed GPU = %d/%d, want 4/8", stats.AllocatedGPURequests.Value(), stats.AllocatableGPU.Value())
	}

	if len(totals.GPUGroups) != 2 {
		t.Fatalf("expected 2 GPU groups, got %+v", totals.GPUGroups)
	}
	g := totals.GPUGroups[1]
	if g.Model != "NVIDIA-A100-SXM4-80GB" || g.Resource != "nvidia.com/mig-1g.10gb" || g.Nodes != 1 ||
		g.Allocatable != 7 || g.Requests != 3 || g.Free() != 4 {
		t.Errorf("unexpected MIG group: %+v", g)
	}
}

// TestComputeNodeSnapshot_InitContainersAndOverhead checks that requests
// follow the scheduler's rules rather than summing app containers only.
func TestComputeNodeSnapshot_InitContainersAndOverhead(t *testing.T) {
	always := v1.ContainerRestartPolicyAlways
	node := readyNode("node-1", nil, v1.ResourceList{v1.ResourceCPU: milliCPU(8000), ResourceNvidiaGPU: qty(4)})

	pods := []v1.Pod{
		{
			// A GPU-requesting init container larger than the app container
			// determines the effective request: max(2, 1) = 2 GPUs.
			Spec: v1.PodSpec{
				InitContainers: []v1.Container{{Resources: requests(v1.ResourceList{ResourceNvidiaGPU: qty(2)})}},
				Containers:     []v1.Container{{Resources: requests(v1.ResourceList{ResourceNvidiaGPU: qty(1)})}},
			},
		},
		{
			// A restartable sidecar init container adds to the app containers
			// (500m + 250m), and pod overhead adds 100m.
			Spec: v1.PodSpec{
				InitContainers: []v1.Container{{
					RestartPolicy: &always,
					Resources:     requests(v1.ResourceList{v1.ResourceCPU: milliCPU(250)}),
				}},
				Containers: []v1.Container{{Resources: requests(v1.ResourceList{v1.ResourceCPU: milliCPU(500)})}},
				Overhead:   v1.ResourceList{v1.ResourceCPU: milliCPU(100)},
			},
		},
	}

	nm, _, err := ComputeNodeSnapshot([]v1.Node{node}, map[string][]v1.Pod{"node-1": pods}, nil, NodeSnapshotOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stats := nm["node-1"]

	if got := stats.AllocatedGPURequests.Value(); got != 2 {
		t.Errorf("GPU requests = %d, want 2", got)
	}
	if got := stats.Accelerators[ResourceNvidiaGPU].Requests; got != 2 {
		t.Errorf("per-type GPU requests = %d, want 2", got)
	}
	if got := stats.AllocatedCPUrequests.MilliValue(); got != 850 {
		t.Errorf("CPU requests = %dm, want 850m", got)
	}
}

func TestShortAcceleratorName(t *testing.T) {
	for in, want := range map[v1.ResourceName]string{
		ResourceNvidiaGPU:       "gpu",
		"nvidia.com/mig-1g.5gb": "mig-1g.5gb",
		ResourceAWSNeuronCore:   "neuroncore",
		"nodomain":              "nodomain",
	} {
		if got := ShortAcceleratorName(in); got != want {
			t.Errorf("ShortAcceleratorName(%q) = %q, want %q", in, got, want)
		}
	}
}
