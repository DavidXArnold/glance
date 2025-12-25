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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNodeMapCreation(t *testing.T) {
	nm := make(NodeMap)

	if len(nm) != 0 {
		t.Errorf("New NodeMap should be empty, got length %d", len(nm))
	}
}

func TestNodeStatsFields(t *testing.T) {
	stats := &NodeStats{
		Status:     nodeStatusReady,
		ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
	}

	if stats.Status != nodeStatusReady {
		t.Errorf("NodeStats.Status = %q, want %q", stats.Status, "Ready")
	}

	if stats.ProviderID != "aws:///us-west-2a/i-1234567890abcdef0" {
		t.Errorf("NodeStats.ProviderID = %q, want %q", stats.ProviderID, "aws:///us-west-2a/i-1234567890abcdef0")
	}
}

func TestTotalsCreation(t *testing.T) {
	totals := &Totals{
		TotalAllocatableCPU:    resource.NewMilliQuantity(0, resource.DecimalSI),
		TotalAllocatableMemory: resource.NewQuantity(0, resource.BinarySI),
	}

	if totals.TotalAllocatableCPU == nil {
		t.Errorf("TotalAllocatableCPU is nil")
	}

	if totals.TotalAllocatableMemory == nil {
		t.Errorf("TotalAllocatableMemory is nil")
	}
}

func TestPodInfoCreation(t *testing.T) {
	podInfo := &PodInfo{
		UsageCPU:    resource.NewMilliQuantity(100, resource.DecimalSI),
		UsageMemory: resource.NewQuantity(256000000, resource.BinarySI),
	}

	if podInfo.UsageCPU == nil {
		t.Errorf("UsageCPU is nil")
	}

	if podInfo.UsageMemory == nil {
		t.Errorf("UsageMemory is nil")
	}
}

func TestGlanceStructure(t *testing.T) {
	nodes := make(NodeMap)
	nodes["node1"] = &NodeStats{Status: nodeStatusReady}
	nodes["node2"] = &NodeStats{Status: "NotReady"}

	glance := &Glance{
		Nodes: nodes,
		Totals: Totals{
			TotalAllocatableCPU:    resource.NewMilliQuantity(4000, resource.DecimalSI),
			TotalAllocatableMemory: resource.NewQuantity(8000000000, resource.BinarySI),
		},
	}

	if len(glance.Nodes) != 2 {
		t.Errorf("Glance.Nodes should have 2 entries, got %d", len(glance.Nodes))
	}

	if glance.Nodes["node1"].Status != nodeStatusReady {
		t.Errorf("Glance.Nodes[node1].Status = %q, want %q", glance.Nodes["node1"].Status, nodeStatusReady)
	}
}

func TestNodeMapAccess(t *testing.T) {
	nm := make(NodeMap)

	stats := &NodeStats{
		Status:     "Ready",
		ProviderID: "aws:///us-west-2a/i-1234567890abcdef0",
	}
	nm["test-node"] = stats

	if nm["test-node"] == nil {
		t.Errorf("Failed to retrieve node from NodeMap")
	}

	if nm["test-node"].Status != nodeStatusReady {
		t.Errorf("Retrieved node status = %q, want %q", nm["test-node"].Status, nodeStatusReady)
	}

	if nm["nonexistent"] != nil {
		t.Errorf("Expected nil for nonexistent node, got %v", nm["nonexistent"])
	}
}

func TestTotalsInitialization(t *testing.T) {
	tests := []struct {
		name  string
		field *resource.Quantity
	}{
		{"TotalAllocatableCPU", resource.NewMilliQuantity(0, resource.DecimalSI)},
		{"TotalAllocatableMemory", resource.NewQuantity(0, resource.BinarySI)},
		{"TotalCapacityCPU", resource.NewMilliQuantity(0, resource.DecimalSI)},
		{"TotalCapacityMemory", resource.NewQuantity(0, resource.BinarySI)},
		{"TotalAllocatedCPUrequests", resource.NewMilliQuantity(0, resource.DecimalSI)},
		{"TotalAllocatedCPULimits", resource.NewMilliQuantity(0, resource.DecimalSI)},
		{"TotalAllocatedMemoryRequests", resource.NewQuantity(0, resource.BinarySI)},
		{"TotalAllocatedMemoryLimits", resource.NewQuantity(0, resource.BinarySI)},
		{"TotalUsageCPU", resource.NewMilliQuantity(0, resource.DecimalSI)},
		{"TotalUsageMemory", resource.NewQuantity(0, resource.BinarySI)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field == nil {
				t.Errorf("%s is nil", tt.name)
			}

			if tt.field.IsZero() == false && tt.field.Value() != 0 {
				t.Errorf("%s should be zero, got %v", tt.name, tt.field)
			}
		})
	}
}
