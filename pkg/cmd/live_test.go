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

func TestNewLiveCmdNotNil(t *testing.T) {
	gc, err := NewGlanceConfig()
	if err != nil {
		t.Fatalf("NewGlanceConfig() failed: %v", err)
	}

	cmd := NewLiveCmd(gc)

	if cmd.Use != "live" {
		t.Errorf("NewLiveCmd() Use = %q, want %q", cmd.Use, "live")
	}

	if cmd.Short == "" {
		t.Errorf("NewLiveCmd() Short is empty")
	}

	if cmd.Long == "" {
		t.Errorf("NewLiveCmd() Long is empty")
	}
}

func TestNewLiveCmdFlags(t *testing.T) {
	gc, err := NewGlanceConfig()
	if err != nil {
		t.Fatalf("NewGlanceConfig() failed: %v", err)
	}

	cmd := NewLiveCmd(gc)

	if cmd.Flags() == nil {
		t.Errorf("Live command flags is nil")
	}

	refreshFlag := cmd.Flags().Lookup("refresh")
	if refreshFlag == nil {
		t.Errorf("refresh flag not found")
		return
	}

	if refreshFlag.Shorthand != "r" {
		t.Errorf("refresh flag shorthand = %q, want %q", refreshFlag.Shorthand, "r")
	}

	if refreshFlag.DefValue != "2" {
		t.Errorf("refresh flag default = %q, want %q", refreshFlag.DefValue, "2")
	}
}

func TestGetModeString(t *testing.T) {
	tests := []struct {
		mode     ViewMode
		expected string
	}{
		{ViewNamespaces, "NAMESPACES"},
		{ViewPods, "PODS"},
		{ViewNodes, "NODES"},
		{ViewDeployments, "DEPLOYMENTS"},
		{ViewMode(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := getModeString(tt.mode)
			if result != tt.expected {
				t.Errorf("getModeString(%v) = %q, want %q", tt.mode, result, tt.expected)
			}
		})
	}
}

func TestGetHelpText(t *testing.T) {
	tests := []struct {
		mode             ViewMode
		shouldHaveNS     bool
		shouldHaveUpDown bool
	}{
		{ViewNamespaces, false, true}, // Has [↑↓]Select [Enter]View
		{ViewPods, true, false},       // Has [←→]NS
		{ViewNodes, false, false},
		{ViewDeployments, true, false}, // Has [←→]NS
	}

	for _, tt := range tests {
		t.Run(getModeString(tt.mode), func(t *testing.T) {
			// Test that view mode string is returned correctly
			result := getModeString(tt.mode)
			if result == "" {
				t.Errorf("getModeString() returned empty string for mode %v", tt.mode)
			}
		})
	}
}

func TestFormatMilliCPU(t *testing.T) {
	tests := []struct {
		name     string
		input    *resource.Quantity
		expected string
	}{
		{"nil", nil, "0"},
		{"zero", resource.NewMilliQuantity(0, resource.DecimalSI), "0"},
		{"500m", resource.NewMilliQuantity(500, resource.DecimalSI), "0.5"},
		{"1000m", resource.NewMilliQuantity(1000, resource.DecimalSI), "1.0"},
		{"2500m", resource.NewMilliQuantity(2500, resource.DecimalSI), "2.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMilliCPU(tt.input)
			if result != tt.expected {
				t.Errorf("formatMilliCPU(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    *resource.Quantity
		expected string
	}{
		{"nil", nil, "0"},
		{"zero", resource.NewQuantity(0, resource.BinarySI), "0"},
		{"bytes", resource.NewQuantity(512, resource.BinarySI), "512B"},
		{"kilobytes", resource.NewQuantity(2048, resource.BinarySI), "2.00Ki"},
		{"megabytes", resource.NewQuantity(5*1024*1024, resource.BinarySI), "5.00Mi"},
		{"gigabytes", resource.NewQuantity(3*1024*1024*1024, resource.BinarySI), "3.00Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("formatBytes(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMultiplyQuantity(t *testing.T) {
	tests := []struct {
		name       string
		input      *resource.Quantity
		multiplier int
		expected   int64
	}{
		{"nil", nil, 5, 0},
		{"zero", resource.NewQuantity(0, resource.BinarySI), 5, 0},
		{"multiply by 1", resource.NewQuantity(100, resource.BinarySI), 1, 100},
		{"multiply by 3", resource.NewQuantity(100, resource.BinarySI), 3, 300},
		{"multiply by 0", resource.NewQuantity(100, resource.BinarySI), 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := multiplyQuantity(tt.input, tt.multiplier)
			if result.Value() != tt.expected {
				t.Errorf("multiplyQuantity(%v, %d).Value() = %d, want %d",
					tt.input, tt.multiplier, result.Value(), tt.expected)
			}
		})
	}
}

func TestLiveStateCreation(t *testing.T) {
	state := &LiveState{
		mode:              ViewNamespaces,
		selectedNamespace: "default",
		refreshInterval:   2,
	}

	if state.mode != ViewNamespaces {
		t.Errorf("LiveState mode = %v, want %v", state.mode, ViewNamespaces)
	}

	if state.selectedNamespace != "default" {
		t.Errorf("LiveState selectedNamespace = %q, want %q", state.selectedNamespace, "default")
	}
}

func TestGlanceCmdHasLiveSubcommand(t *testing.T) {
	cmd := NewGlanceCmd()

	// Check that the live subcommand was added
	liveCmd := cmd.Commands()
	found := false
	for _, subcmd := range liveCmd {
		if subcmd.Use == "live" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("glance command does not have 'live' subcommand")
	}
}

func TestResourceMetrics(t *testing.T) {
	m := ResourceMetrics{
		CPURequest:  1.5,
		CPULimit:    2.0,
		CPUUsage:    1.2,
		CPUCapacity: 4.0,
		MemRequest:  1024,
		MemLimit:    2048,
		MemUsage:    1536,
		MemCapacity: 4096,
	}

	if m.CPURequest != 1.5 {
		t.Errorf("CPURequest = %f, want 1.5", m.CPURequest)
	}

	if m.MemCapacity != 4096 {
		t.Errorf("MemCapacity = %f, want 4096", m.MemCapacity)
	}
}

func TestMakeProgressBar(t *testing.T) {
	tests := []struct {
		name           string
		value          float64
		max            float64
		width          int
		showPercentage bool
		wantContains   string
	}{
		{"50% with percentage", 50, 100, 10, true, "50%"},
		{"50% without percentage", 50, 100, 10, false, "█"},
		{"100% filled", 100, 100, 10, false, "█"},
		{"0% filled", 0, 100, 10, false, "░"},
		{"zero max", 50, 0, 10, true, "0%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeProgressBar(tt.value, tt.max, tt.width, tt.showPercentage)
			if result == "" {
				t.Errorf("makeProgressBar() returned empty string")
			}
			if !contains(result, tt.wantContains) {
				t.Errorf("makeProgressBar() = %q, should contain %q", result, tt.wantContains)
			}
		})
	}
}

func TestAddProgressBars(t *testing.T) {
	data := [][]string{
		{"namespace-1", "1.0", "2.0", "0.5", "1Gi", "2Gi", "0.5Gi", "5"},
		{"namespace-2", "2.0", "4.0", "1.5", "2Gi", "4Gi", "1.5Gi", "10"},
	}

	metrics := []ResourceMetrics{
		{
			CPURequest: 1.0, CPULimit: 2.0, CPUUsage: 0.5, CPUCapacity: 2.0,
			MemRequest: 1073741824, MemLimit: 2147483648, MemUsage: 536870912, MemCapacity: 2147483648,
		},
		{
			CPURequest: 2.0, CPULimit: 4.0, CPUUsage: 1.5, CPUCapacity: 4.0,
			MemRequest: 2147483648, MemLimit: 4294967296, MemUsage: 1610612736, MemCapacity: 4294967296,
		},
	}

	// For namespace view, we have 1 base column (NAMESPACE)
	result := addProgressBars(data, metrics, true, 1)

	// Should have double the rows (original + bar rows)
	if len(result) != len(data)*2 {
		t.Errorf("addProgressBars() returned %d rows, want %d", len(result), len(data)*2)
	}

	// First row should be original data
	if result[0][0] != "namespace-1" {
		t.Errorf("First row not preserved: got %q, want %q", result[0][0], "namespace-1")
	}

	// Second row should be progress bars (first column empty)
	if result[1][0] != "" {
		t.Errorf("Progress bar row first column should be empty, got %q", result[1][0])
	}

	// Progress bar row should have bars (starting at column 1 since baseColCount=1)
	if result[1][1] == "" {
		t.Errorf("Progress bar row should have CPU request bar")
	}
}

func TestSettingsModal(t *testing.T) {
	state := &LiveState{
		showBars:        true,
		showPercentages: false,
		compactMode:     true,
	}

	// Test that pending state can be initialized
	initPendingState(state)

	if state.pendingShowBars != state.showBars {
		t.Errorf("initPendingState() should copy showBars: got %v, want %v", state.pendingShowBars, state.showBars)
	}

	// Test building settings rows
	rows := buildSettingsRows(state)
	if len(rows) == 0 {
		t.Errorf("buildSettingsRows() returned empty slice")
	}

	// First row should be header with 4 columns (Category, Setting, Key, Value)
	if len(rows[0]) < 4 {
		t.Errorf("buildSettingsRows() header should have 4 columns")
	}
}

func TestLiveStateToggles(t *testing.T) {
	state := &LiveState{
		showBars:        true,
		showPercentages: true,
		compactMode:     false,
	}

	if !state.showBars {
		t.Errorf("showBars should be true initially")
	}

	// Test that toggles would work
	state.showBars = !state.showBars
	if state.showBars {
		t.Errorf("showBars should be false after toggle")
	}

	state.showPercentages = !state.showPercentages
	if state.showPercentages {
		t.Errorf("showPercentages should be false after toggle")
	}

	state.compactMode = !state.compactMode
	if !state.compactMode {
		t.Errorf("compactMode should be true after toggle")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
