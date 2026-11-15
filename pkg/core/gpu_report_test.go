package core

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func gpuPod(namespace, name, node string, gpus int64) v1.Pod {
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       v1.PodSpec{NodeName: node, Containers: []v1.Container{{Name: "c"}}},
		Status:     v1.PodStatus{Phase: v1.PodRunning},
	}
	if gpus > 0 {
		pod.Spec.Containers[0].Resources = requests(v1.ResourceList{ResourceNvidiaGPU: qty(gpus)})
	}
	if node == "" {
		pod.Status.Phase = v1.PodPending
	}
	return pod
}

func findingsByKind(r *GPUReport) map[string][]GPUFinding {
	out := map[string][]GPUFinding{}
	for _, f := range r.Findings {
		out[f.Kind] = append(out[f.Kind], f)
	}
	return out
}

func TestBuildGPUReport(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	a100 := map[string]string{LabelNvidiaGPUProduct: "NVIDIA-A100-SXM4-80GB"}
	gpus := func(n int64) v1.ResourceList { return v1.ResourceList{ResourceNvidiaGPU: qty(n)} }

	cordoned := readyNode("gpu-cordoned", a100, gpus(8))
	cordoned.Spec.Unschedulable = true
	notReady := readyNode("gpu-down", a100, gpus(8))
	notReady.Status.Conditions[0].Status = v1.ConditionFalse

	nodes := []v1.Node{
		readyNode("gpu-a", a100, gpus(8)),    // 6 used, 2 free
		readyNode("gpu-b", a100, gpus(8)),    // 5 used, 3 free
		readyNode("gpu-idle", a100, gpus(8)), // nothing scheduled
		readyNode("gpu-busy", a100, gpus(8)), // only CPU pods
		cordoned,                             // 8 free but cordoned
		notReady,                             // ignored for free capacity
		readyNode("cpu-only", nil, v1.ResourceList{v1.ResourceCPU: milliCPU(4000)}),
	}

	daemon := gpuPod("kube-system", "dcgm-exporter", "gpu-idle", 0)
	daemon.OwnerReferences = []metav1.OwnerReference{{Kind: "DaemonSet", Name: "dcgm-exporter"}}
	// Static pods such as kube-proxy on GKE run on every node.
	static := gpuPod("kube-system", "kube-proxy-gpu-idle", "gpu-idle", 0)
	static.Annotations = map[string]string{mirrorPodAnnotation: "abc123"}
	static.OwnerReferences = []metav1.OwnerReference{{Kind: "Node", Name: "gpu-idle"}}

	pendingFrag := gpuPod("ml", "train-4gpu", "", 4)
	pendingFrag.CreationTimestamp = metav1.NewTime(now.Add(-2 * time.Hour))
	pendingFrag.Status.Conditions = []v1.PodCondition{{
		Type: v1.PodScheduled, Status: v1.ConditionFalse, Reason: "Unschedulable",
		Message: "0/7 nodes are available: 4 Insufficient nvidia.com/gpu.",
	}}
	pendingFits := gpuPod("ml", "infer-1gpu", "", 1)
	pendingFits.CreationTimestamp = metav1.NewTime(now.Add(-time.Minute))
	pendingHuge := gpuPod("ml", "train-64gpu", "", 64)
	pendingHuge.CreationTimestamp = metav1.NewTime(now.Add(-time.Hour))
	finished := gpuPod("ml", "done", "gpu-idle", 8)
	finished.Status.Phase = v1.PodSucceeded

	pods := []v1.Pod{
		gpuPod("ml", "a1", "gpu-a", 4), gpuPod("ml", "a2", "gpu-a", 2),
		gpuPod("ml", "b1", "gpu-b", 5),
		daemon, static, finished,
		gpuPod("web", "nginx-1", "gpu-busy", 0), gpuPod("web", "nginx-2", "gpu-busy", 0),
		pendingFrag, pendingFits, pendingHuge,
	}

	r := BuildGPUReport(nodes, podPtrs(pods), now)

	t.Run("nodes", func(t *testing.T) { checkReportNodes(t, &r) })
	t.Run("pools", func(t *testing.T) {
		if len(r.Pools) != 1 {
			t.Fatalf("expected 1 pool, got %+v", r.Pools)
		}
		// Schedulable free: gpu-a 2 + gpu-b 3 + gpu-idle 8 + gpu-busy 8 = 21.
		want := GPUPool{
			Model: "NVIDIA-A100-SXM4-80GB", Resource: ResourceNvidiaGPU, Nodes: 6,
			Allocatable: 48, Requests: 11, Free: 21, LargestFreeBlock: 8, NodesWithFree: 4,
			UnschedulableFree: 16,
		}
		if r.Pools[0] != want {
			t.Errorf("pool = %+v\nwant   %+v", r.Pools[0], want)
		}
	})
	t.Run("pending", func(t *testing.T) { checkReportPending(t, &r) })
	t.Run("findings", func(t *testing.T) { checkReportFindings(t, &r) })
}

func checkReportNodes(t *testing.T, r *GPUReport) {
	t.Helper()
	if len(r.Nodes) != 6 {
		t.Fatalf("expected 6 GPU nodes (cpu-only excluded), got %d", len(r.Nodes))
	}
	byName := map[string]GPUNodeReport{}
	for _, n := range r.Nodes {
		byName[n.Name] = n
	}
	for name, want := range map[string]string{
		"gpu-cordoned": GPUNodeCordoned, "gpu-down": GPUNodeNotReady, "gpu-a": GPUNodeReady,
	} {
		if got := byName[name].State; got != want {
			t.Errorf("%s state = %q, want %q", name, got, want)
		}
	}
	// DaemonSet, static, and finished pods are not counted.
	if n := byName["gpu-idle"]; n.GPUPods != 0 || n.OtherPods != 0 {
		t.Errorf("gpu-idle pods = %d GPU / %d other, want 0/0", n.GPUPods, n.OtherPods)
	}
	if n := byName["gpu-busy"]; n.OtherPods != 2 {
		t.Errorf("gpu-busy other pods = %d, want 2", n.OtherPods)
	}
	if n := byName["gpu-a"]; n.GPUPods != 2 || n.Accelerators[ResourceNvidiaGPU].Requests != 6 {
		t.Errorf("gpu-a = %d pods / %d requested, want 2 / 6", n.GPUPods, n.Accelerators[ResourceNvidiaGPU].Requests)
	}
}

func checkReportPending(t *testing.T, r *GPUReport) {
	t.Helper()
	if len(r.Pending) != 3 {
		t.Fatalf("expected 3 pending GPU pods, got %+v", r.Pending)
	}
	if first := r.Pending[0]; first.Name != "train-4gpu" || first.Reason != "Unschedulable" || first.Age != 2*time.Hour {
		t.Errorf("pending pods should be sorted longest-waiting first with reason: %+v", first)
	}
	for _, p := range r.Pending {
		if wantFits := p.Name != "train-64gpu"; p.Fits != wantFits {
			t.Errorf("%s fits = %v, want %v", p.Name, p.Fits, wantFits)
		}
	}
}

func checkReportFindings(t *testing.T, r *GPUReport) {
	t.Helper()
	f := findingsByKind(r)
	if len(f[FindingIdleNode]) != 1 || f[FindingIdleNode][0].Subject != "gpu-idle" {
		t.Errorf("idle findings = %+v", f[FindingIdleNode])
	}
	if len(f[FindingIdleBusyNode]) != 1 || f[FindingIdleBusyNode][0].Subject != "gpu-busy" {
		t.Errorf("idle-busy findings = %+v", f[FindingIdleBusyNode])
	}
	if len(f[FindingCordonedFree]) != 2 {
		t.Errorf("expected cordoned and NotReady nodes with free GPUs, got %+v", f[FindingCordonedFree])
	}
	if len(f[FindingPendingNoSpace]) != 1 || f[FindingPendingNoSpace][0].Subject != "ml/train-64gpu" {
		t.Errorf("no-capacity findings = %+v", f[FindingPendingNoSpace])
	}
	if len(f[FindingFragmentation]) != 0 {
		t.Errorf("train-4gpu fits on an 8-GPU node, expected no fragmentation: %+v", f[FindingFragmentation])
	}
}

// TestBuildGPUReport_Fragmentation covers enough free GPUs in total but none
// on a single node: 4 nodes x 1 free GPU cannot host a 2-GPU pod.
func TestBuildGPUReport_Fragmentation(t *testing.T) {
	var nodes []v1.Node
	var pods []v1.Pod
	for _, name := range []string{"n1", "n2", "n3", "n4"} {
		nodes = append(nodes, readyNode(name, nil, v1.ResourceList{ResourceNvidiaGPU: qty(4)}))
		pods = append(pods, gpuPod("ml", "busy-"+name, name, 3))
	}
	pods = append(pods, gpuPod("ml", "needs-2", "", 2))

	r := BuildGPUReport(nodes, podPtrs(pods), time.Now())

	if r.Pools[0].Free != 4 || r.Pools[0].LargestFreeBlock != 1 || r.Pools[0].Model != UnknownGPUModel {
		t.Errorf("pool = %+v, want 4 free, largest block 1, unknown model", r.Pools[0])
	}
	f := findingsByKind(&r)
	if len(f[FindingFragmentation]) != 1 || f[FindingFragmentation][0].Subject != "ml/needs-2" {
		t.Fatalf("fragmentation findings = %+v", r.Findings)
	}
	want := "needs 2 gpu on one node; 4 free in total but at most 1 on any node"
	if got := f[FindingFragmentation][0].Message; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestBuildGPUReport_NoGPUs(t *testing.T) {
	r := BuildGPUReport([]v1.Node{readyNode("cpu", nil, v1.ResourceList{v1.ResourceCPU: milliCPU(1000)})}, nil, time.Now())
	if len(r.Nodes) != 0 || len(r.Pools) != 0 || len(r.Pending) != 0 || len(r.Findings) != 0 {
		t.Errorf("expected empty report, got %+v", r)
	}
	if r.Nodes == nil || r.Pools == nil || r.Pending == nil || r.Findings == nil {
		t.Error("report slices should be empty, not nil, so JSON renders [] instead of null")
	}
}
