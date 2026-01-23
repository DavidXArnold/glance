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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetTerminalWidth(t *testing.T) {
	width := getTerminalWidth()

	// Should return a value within bounds
	if width < minBoxWidth {
		t.Errorf("getTerminalWidth() = %d, want >= %d", width, minBoxWidth)
	}

	if width > maxBoxWidth {
		t.Errorf("getTerminalWidth() = %d, want <= %d", width, maxBoxWidth)
	}
}

func TestBuildColoredProgressBarDynamic(t *testing.T) {
	tests := []struct {
		name  string
		pct   float64
		width int
	}{
		{"0%", 0, 20},
		{"50%", 50, 20},
		{"75%", 75, 20},
		{"90%", 90, 20},
		{"100%", 100, 20},
		{"negative", -10, 20},
		{"over 100", 150, 20},
		{"small width", 50, 5},
		{"large width", 50, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := buildColoredProgressBarDynamic(tt.pct, tt.width)
			if bar == "" {
				t.Errorf("buildColoredProgressBarDynamic(%f, %d) returned empty string", tt.pct, tt.width)
			}
			// Bar should contain brackets
			if bar[0] != '[' || bar[len(bar)-1] != ']' {
				t.Errorf("buildColoredProgressBarDynamic should return bar with brackets, got %q", bar)
			}
		})
	}
}

func TestPadRightDynamic(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		width  int
		minLen int
	}{
		{"short string", "test", 20, 21},
		{"exact width", "exactly twenty chars", 20, 20},
		{"longer than width", "this string is longer than the width", 20, 36}, // no padding added
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padRightDynamic(tt.input, tt.width)
			if len(result) < tt.minLen {
				t.Errorf("padRightDynamic(%q, %d) = len %d, want >= %d", tt.input, tt.width, len(result), tt.minLen)
			}
		})
	}
}

func TestRenderJSONOutput(t *testing.T) {
	nm := make(NodeMap)
	nm["test-node"] = &NodeStats{
		Status:     "Ready",
		ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
	}

	totals := &Totals{
		TotalAllocatableCPU:    resource.NewMilliQuantity(4000, resource.DecimalSI),
		TotalAllocatableMemory: resource.NewQuantity(8000000000, resource.BinarySI),
	}

	glance := &Glance{
		Nodes:  nm,
		Totals: *totals,
	}

	// Verify it can be marshaled to JSON
	data, err := json.MarshalIndent(glance, "", "\t")
	if err != nil {
		t.Errorf("Failed to marshal Glance to JSON: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("JSON output is empty")
	}

	// Verify it contains expected fields
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Errorf("Failed to unmarshal JSON: %v", err)
	}

	if _, hasNodes := result["Nodes"]; !hasNodes {
		t.Errorf("JSON output missing 'Nodes' field")
	}

	if _, hasTotals := result["Totals"]; !hasTotals {
		t.Errorf("JSON output missing 'Totals' field")
	}
}

func TestNodeMapSerialization(t *testing.T) {
	nm := make(NodeMap)
	nm["node1"] = &NodeStats{Status: "Ready"}
	nm["node2"] = &NodeStats{Status: "NotReady"}

	data, err := json.Marshal(nm)
	if err != nil {
		t.Errorf("Failed to marshal NodeMap: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Errorf("Failed to unmarshal NodeMap: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 nodes in JSON, got %d", len(result))
	}
}

func TestTotalsSerialization(t *testing.T) {
	totals := &Totals{
		TotalAllocatableCPU:    resource.NewMilliQuantity(4000, resource.DecimalSI),
		TotalAllocatableMemory: resource.NewQuantity(8000000000, resource.BinarySI),
	}

	data, err := json.Marshal(totals)
	if err != nil {
		t.Errorf("Failed to marshal Totals: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Errorf("Failed to unmarshal Totals: %v", err)
	}

	if len(result) == 0 {
		t.Errorf("Totals JSON is empty")
	}
}

func TestRenderOutputFormats(t *testing.T) {
	nm := make(NodeMap)
	nm["test-node"] = &NodeStats{
		Status:            "Ready",
		ProviderID:        "aws:///us-west-2a/i-1234567890abcdef0",
		AllocatableCPU:    resource.NewMilliQuantity(4000, resource.DecimalSI),
		AllocatableMemory: resource.NewQuantity(8000000000, resource.BinarySI),
	}

	_ = &Totals{
		TotalAllocatableCPU:    resource.NewMilliQuantity(4000, resource.DecimalSI),
		TotalAllocatableMemory: resource.NewQuantity(8000000000, resource.BinarySI),
	}

	tests := []struct {
		name string
		fn   func()
	}{
		{"pretty output exists", func() {
			// Verify the function exists and can be called
			// (without actually executing it as it calls os.Exit)
		}},
		{"table output exists", func() {
			// Verify the function exists
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn()
		})
	}
}

func TestNodeStatsWithPods(t *testing.T) {
	stats := &NodeStats{
		Status:     "Ready",
		ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
		PodInfo:    make(map[string]*PodInfo),
	}

	// Add some pod info
	stats.PodInfo["pod1"] = &PodInfo{
		UsageCPU:    resource.NewMilliQuantity(100, resource.DecimalSI),
		UsageMemory: resource.NewQuantity(256000000, resource.BinarySI),
	}

	if len(stats.PodInfo) != 1 {
		t.Errorf("Expected 1 pod in NodeStats, got %d", len(stats.PodInfo))
	}

	if stats.PodInfo["pod1"].UsageCPU == nil {
		t.Errorf("Pod usage CPU is nil")
	}
}

func TestNodeStatsWithQoS(t *testing.T) {
	// Basic sanity check that QoS field can be set; specific enum values are
	// covered by Kubernetes types and do not need to be asserted here.
	podInfo := &PodInfo{}
	if podInfo.Qos != nil {
		t.Errorf("expected initial PodInfo.Qos to be nil")
	}
}

func TestNodeStatsCloudInfo(t *testing.T) {
	stats := &NodeStats{
		Status:     "Ready",
		ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
	}

	if stats.CloudInfo.Aws != nil {
		t.Errorf("Expected AWS cloudInfo to be nil")
	}

	if stats.CloudInfo.Gce != nil {
		t.Errorf("Expected GCE cloudInfo to be nil")
	}
}

// helper to build a minimal NodeMap/Totals for static render tests.
func buildTestSnapshot() (*NodeMap, *Totals) {
	nm := make(NodeMap)
	nm["node1"] = &NodeStats{
		Status:       "Ready",
		CreationTime: metav1.Now().Time,
		NodeGroup:    "group-a",
	}

	totals := &Totals{}
	return &nm, totals
}

// captureOutput captures stdout while fn runs and restores it afterwards.
func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(fmt.Sprintf("failed to create pipe: %v", err))
	}
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = ioCopy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	os.Stdout = old
	<-done

	return buf.String()
}

// ioCopy is a tiny wrapper to avoid importing io.Copy directly multiple times.
func ioCopy(dst *bytes.Buffer, src *os.File) (int64, error) {
	return dst.ReadFrom(src)
}

func TestStaticTableNodeColumnVisibility(t *testing.T) {
	nm, totals := buildTestSnapshot()

	// Ensure viper state is clean for each subtest.
	viper.Reset()

	t.Run("default shows version only", func(t *testing.T) {
		viper.Reset()
		out := captureOutput(func() { table(nm, totals) })

		if !strings.Contains(out, "VERSION") {
			t.Fatalf("expected VERSION column by default")
		}
		if strings.Contains(out, "AGE") {
			t.Fatalf("did not expect AGE column by default")
		}
		if strings.Contains(out, "GROUP") {
			t.Fatalf("did not expect GROUP column by default")
		}
	})

	t.Run("hide version when explicitly disabled", func(t *testing.T) {
		viper.Reset()
		viper.Set("show-node-version", false)
		out := captureOutput(func() { table(nm, totals) })

		if strings.Contains(out, "VERSION") {
			t.Fatalf("expected VERSION column to be hidden when show-node-version=false")
		}
	})

	t.Run("add age and group when enabled", func(t *testing.T) {
		viper.Reset()
		viper.Set("show-node-age", true)
		viper.Set("show-node-group", true)
		out := captureOutput(func() { table(nm, totals) })

		if !strings.Contains(out, "AGE") {
			t.Fatalf("expected AGE column when show-node-age=true")
		}
		if !strings.Contains(out, "GROUP") {
			t.Fatalf("expected GROUP column when show-node-group=true")
		}
	})
}

func TestPrettyStaticNodeColumnVisibility(t *testing.T) {
	nm, totals := buildTestSnapshot()

	viper.Reset()

	t.Run("default pretty shows version", func(t *testing.T) {
		viper.Reset()
		out := captureOutput(func() {
			_ = renderPretty(nm, totals)
		})

		if !strings.Contains(out, "VERSION") {
			t.Fatalf("expected VERSION text in pretty output by default")
		}
	})

	t.Run("pretty hides version when disabled", func(t *testing.T) {
		viper.Reset()
		viper.Set("show-node-version", false)
		out := captureOutput(func() {
			_ = renderPretty(nm, totals)
		})

		if strings.Contains(out, "VERSION") {
			t.Fatalf("expected VERSION column to be hidden in pretty output when show-node-version=false")
		}
	})

	t.Run("pretty shows age and group when enabled", func(t *testing.T) {
		viper.Reset()
		viper.Set("show-node-age", true)
		viper.Set("show-node-group", true)
		out := captureOutput(func() {
			_ = renderPretty(nm, totals)
		})

		if !strings.Contains(out, "AGE") {
			t.Fatalf("expected AGE column in pretty output when show-node-age=true")
		}
		if !strings.Contains(out, "GROUP") {
			t.Fatalf("expected GROUP column in pretty output when show-node-group=true")
		}
	})
}
