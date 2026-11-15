package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/viper"
	core "gitlab.com/davidxarnold/glance/pkg/core"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func gpuTestNode(name string, gpus int64) *v1.Node {
	res := v1.ResourceList{core.ResourceNvidiaGPU: *resource.NewQuantity(gpus, resource.DecimalSI)}
	labels := map[string]string{core.LabelGKEAccelerator: "nvidia-l4", "pool": "gpu"}
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status: v1.NodeStatus{
			Conditions:  []v1.NodeCondition{{Type: v1.NodeReady, Status: v1.ConditionTrue}},
			Allocatable: res,
			Capacity:    res,
		},
	}
}

func gpuTestPod(name, node string, gpus int64) *v1.Pod {
	list := v1.ResourceList{core.ResourceNvidiaGPU: *resource.NewQuantity(gpus, resource.DecimalSI)}
	phase := v1.PodRunning
	if node == "" {
		phase = v1.PodPending
	}
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ml"},
		Spec: v1.PodSpec{
			NodeName:   node,
			Containers: []v1.Container{{Name: "c", Resources: v1.ResourceRequirements{Requests: list, Limits: list}}},
		},
		Status: v1.PodStatus{Phase: phase},
	}
}

func TestCollectAndRenderGPUReport(t *testing.T) {
	viper.Set("selector", "pool=gpu")
	t.Cleanup(func() { viper.Set("selector", "") })

	client := fake.NewSimpleClientset(
		gpuTestNode("l4-a", 2), gpuTestNode("l4-b", 2),
		&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cpu", Labels: map[string]string{"pool": "cpu"}}},
		gpuTestPod("serve", "l4-a", 1),
		gpuTestPod("train", "", 2),
	)

	report, err := collectGPUReport(context.Background(), apiLister{client: client})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Nodes) != 2 || len(report.Pending) != 1 {
		t.Fatalf("report = %d nodes / %d pending, want 2 / 1", len(report.Nodes), len(report.Pending))
	}

	var txt bytes.Buffer
	if err := renderGPUReport(&txt, &report, "txt"); err != nil {
		t.Fatalf("render txt: %v", err)
	}
	out := txt.String()
	for _, want := range []string{
		"GPU POOLS", "nvidia-l4", "1 / 4", "3 on 2 node(s)",
		"GPU NODES", "l4-b",
		"PENDING GPU PODS", "train", "2 gpu", "yes",
		"FINDINGS", "Idle GPU node: l4-b",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("txt output missing %q:\n%s", want, out)
		}
	}

	var js bytes.Buffer
	if err := renderGPUReport(&js, &report, outputFormatJSON); err != nil {
		t.Fatalf("render json: %v", err)
	}
	var decoded core.GPUReport
	if err := json.Unmarshal(js.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, js.String())
	}
	if len(decoded.Pools) != 1 || decoded.Pools[0].Free != 3 || decoded.Pools[0].LargestFreeBlock != 2 {
		t.Errorf("decoded pools = %+v", decoded.Pools)
	}

	var pretty bytes.Buffer
	if err := renderGPUReport(&pretty, &report, outputFormatPretty); err != nil {
		t.Fatalf("render pretty: %v", err)
	}
	if !strings.Contains(pretty.String(), "╭") {
		t.Error("pretty output should use the rounded table style")
	}
}

func TestRenderGPUReport_NoGPUs(t *testing.T) {
	r := core.BuildGPUReport(nil, nil, metav1.Now().Time)
	var buf bytes.Buffer
	if err := renderGPUReport(&buf, &r, "txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "No GPU or accelerator nodes found." {
		t.Errorf("output = %q", got)
	}
}

func TestFetchGPUData(t *testing.T) {
	client := fake.NewSimpleClientset(
		gpuTestNode("l4-a", 2), gpuTestNode("l4-b", 4),
		gpuTestPod("serve", "l4-a", 1),
		gpuTestPod("train", "", 2),
	)

	header, data, err := fetchGPUData(context.Background(), apiLister{client: client})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, row := range data {
		if len(row) != len(header) {
			t.Fatalf("row %d has %d columns, header has %d: %v", i, len(row), len(header), row)
		}
	}

	var names []string
	for _, row := range data {
		names = append(names, row[0])
	}
	got := strings.Join(names, "|")
	// train needs 2 GPUs and l4-b has 4 free, so it fits: no fragmentation.
	want := "POOL gpu||l4-a|l4-b||ml/train||! Idle GPU node"
	if got != want {
		t.Errorf("row order = %q\nwant        %q", got, want)
	}
}

func TestFetchGPUData_NoGPUs(t *testing.T) {
	_, data, err := fetchGPUData(context.Background(), apiLister{client: fake.NewSimpleClientset()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 1 || !strings.Contains(data[0][0], "No GPU") {
		t.Errorf("data = %v", data)
	}
}
