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
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	// _ "github.com/go-echarts/go-echarts/v2"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	pt "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/api/resource"

	// needed to authenticate with GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const (
	minBoxWidth     = 60
	maxBoxWidth     = 120
	defaultBoxWidth = 80
)

const ctlC = "<C-c>"

// formatQuantity returns a human-readable or exact representation of a quantity pointer.
func formatQuantity(q *resource.Quantity) string {
	if q == nil {
		return ""
	}

	if viper.GetBool("exact") {
		// Exact value mode - show the raw value
		return q.String()
	}

	// Human-readable mode (default)
	return q.String()
}

// formatQuantityValue returns a human-readable or exact representation of a quantity value.
func formatQuantityValue(q resource.Quantity) string {
	if viper.GetBool("exact") {
		// Exact value mode - show the raw value
		return q.String()
	}

	// Human-readable mode (default)
	return q.String()
}

func render(nm *NodeMap, c *Totals) {
	switch viper.GetString("output") {
	case "json":
		renderJSON(nm, c)
	case "pretty":
		renderPretty(nm, c)
	case "chart":
		chart(nm)
		os.Exit(0)
	case "dash":
		dash(nm)
		os.Exit(0)
	case "pie":
		pie(nm)
		os.Exit(0)
	default:
		table(nm, c)
	}
}

func renderJSON(nm *NodeMap, c *Totals) {
	glance := &Glance{
		Nodes:  *nm,
		Totals: *c,
	}
	g, err := json.MarshalIndent(glance, "", "\t")
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	fmt.Println(string(g))
	os.Exit(0)
}

func renderPretty(nm *NodeMap, c *Totals) {
	// Print cluster summary dashboard
	printClusterSummary(nm, c)

	// Group nodes by status
	readyNodes := make([]string, 0)
	notReadyNodes := make([]string, 0)
	for name, node := range *nm {
		if node.Status == nodeStatusReady {
			readyNodes = append(readyNodes, name)
		} else {
			notReadyNodes = append(notReadyNodes, name)
		}
	}
	sort.Strings(readyNodes)
	sort.Strings(notReadyNodes)

	// Create main table
	t := pt.NewWriter()
	t.SetStyle(pt.StyleRounded)
	t.SetOutputMirror(os.Stdout)

	// Configure column colors
	t.SetColumnConfigs([]pt.ColumnConfig{
		{Number: 1, AutoMerge: false, Colors: text.Colors{text.FgHiWhite, text.Bold}},
		{Number: 2, AutoMerge: false},
		{Number: 3, AutoMerge: false, Colors: text.Colors{text.FgHiCyan}},
		{Number: 4, AutoMerge: false},
		{Number: 5, AutoMerge: false},
	})

	t.AppendHeader(pt.Row{
		"NODE",
		"STATUS",
		"VERSION",
		"CPU UTILIZATION",
		"MEMORY UTILIZATION",
	})

	// Add NotReady nodes first (if any) with red highlighting
	for _, name := range notReadyNodes {
		v := (*nm)[name]
		t.AppendRow(pt.Row{
			name,
			text.Colors{text.FgRed, text.Bold}.Sprint("⊘ " + v.Status),
			v.NodeInfo.KubeletVersion,
			buildCPUUtilizationCell(v),
			buildMemUtilizationCell(v),
		})
	}

	// Add separator if we have both types
	if len(notReadyNodes) > 0 && len(readyNodes) > 0 {
		t.AppendSeparator()
	}

	// Add Ready nodes with green status
	for _, name := range readyNodes {
		v := (*nm)[name]
		t.AppendRow(pt.Row{
			name,
			text.Colors{text.FgGreen, text.Bold}.Sprint("✓ " + v.Status),
			v.NodeInfo.KubeletVersion,
			buildCPUUtilizationCell(v),
			buildMemUtilizationCell(v),
		})
	}

	t.AppendSeparator()
	t.AppendFooter(pt.Row{
		text.Colors{text.FgHiWhite, text.Bold}.Sprint("CLUSTER TOTALS"),
		fmt.Sprintf("%d nodes", len(*nm)),
		"",
		buildTotalCPUCell(c),
		buildTotalMemCell(c),
	})

	fmt.Println()
	t.Render()

	// Print legend
	printLegend()

	os.Exit(0)
}

// getTerminalWidth returns the terminal width, clamped between min and max.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return defaultBoxWidth
	}
	if width < minBoxWidth {
		return minBoxWidth
	}
	if width > maxBoxWidth {
		return maxBoxWidth
	}
	return width
}

// printClusterSummary prints a dashboard header with cluster overview.
func printClusterSummary(nm *NodeMap, c *Totals) {
	readyCount := 0
	notReadyCount := 0
	for _, node := range *nm {
		if node.Status == "Ready" {
			readyCount++
		} else {
			notReadyCount++
		}
	}

	// Calculate cluster-wide utilization
	cpuUsagePct := calculatePercentage(c.TotalUsageCPU, c.TotalAllocatableCPU)
	memUsagePct := calculatePercentage(c.TotalUsageMemory, c.TotalAllocatableMemory)
	cpuAllocPct := calculatePercentage(c.TotalAllocatedCPUrequests, c.TotalAllocatableCPU)
	memAllocPct := calculatePercentage(c.TotalAllocatedMemoryRequests, c.TotalAllocatableMemory)

	// Get dynamic box width
	boxWidth := getTerminalWidth() - 2 // -2 for the border chars
	innerWidth := boxWidth - 2         // -2 for left/right padding inside box

	fmt.Println()
	boxStyle := text.Colors{text.FgHiWhite, text.Bold}
	topBorder := "╔" + strings.Repeat("═", boxWidth) + "╗"
	midBorder := "╠" + strings.Repeat("═", boxWidth) + "╣"
	botBorder := "╚" + strings.Repeat("═", boxWidth) + "╝"
	titleLine := "║" + centerText("🔍 KUBERNETES CLUSTER GLANCE", boxWidth) + "║"

	fmt.Println(boxStyle.Sprint(topBorder))
	fmt.Println(boxStyle.Sprint(titleLine))
	fmt.Println(boxStyle.Sprint(midBorder))

	// Cluster info line
	clusterInfo := fmt.Sprintf("║  Host: %s  │  Version: %s",
		text.Colors{text.FgCyan}.Sprint(c.ClusterInfo.Host),
		text.Colors{text.FgCyan}.Sprint(c.ClusterInfo.MasterVersion))
	fmt.Println(padRightDynamic(clusterInfo, boxWidth) + "║")

	// Node status line
	nodeStatus := fmt.Sprintf("║  Nodes: %s %d Ready",
		text.Colors{text.FgGreen}.Sprint("●"), readyCount)
	if notReadyCount > 0 {
		nodeStatus += fmt.Sprintf("  %s %d NotReady", text.Colors{text.FgRed}.Sprint("●"), notReadyCount)
	}
	fmt.Println(padRightDynamic(nodeStatus, boxWidth) + "║")

	fmt.Println(boxStyle.Sprint(midBorder))

	// Calculate progress bar width dynamically
	barWidth := (innerWidth - 50) // Leave room for labels and values
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 40 {
		barWidth = 40
	}

	// CPU Usage
	cpuUsageBar := buildColoredProgressBarDynamic(cpuUsagePct, barWidth)
	cpuLine := fmt.Sprintf("║  CPU Usage:      %s %5.1f%%  (%s / %s)",
		cpuUsageBar, cpuUsagePct,
		formatQuantity(c.TotalUsageCPU),
		formatQuantity(c.TotalAllocatableCPU))
	fmt.Println(padRightDynamic(cpuLine, boxWidth) + "║")

	// CPU Allocated
	cpuAllocBar := buildColoredProgressBarDynamic(cpuAllocPct, barWidth)
	cpuAllocLine := fmt.Sprintf("║  CPU Allocated:  %s %5.1f%%  (%s / %s)",
		cpuAllocBar, cpuAllocPct,
		formatQuantity(c.TotalAllocatedCPUrequests),
		formatQuantity(c.TotalAllocatableCPU))
	fmt.Println(padRightDynamic(cpuAllocLine, boxWidth) + "║")

	fmt.Println("║" + strings.Repeat(" ", boxWidth) + "║")

	// Memory Usage
	memUsageBar := buildColoredProgressBarDynamic(memUsagePct, barWidth)
	memLine := fmt.Sprintf("║  Mem Usage:      %s %5.1f%%  (%s / %s)",
		memUsageBar, memUsagePct,
		formatQuantity(c.TotalUsageMemory),
		formatQuantity(c.TotalAllocatableMemory))
	fmt.Println(padRightDynamic(memLine, boxWidth) + "║")

	// Memory Allocated
	memAllocBar := buildColoredProgressBarDynamic(memAllocPct, barWidth)
	memAllocLine := fmt.Sprintf("║  Mem Allocated:  %s %5.1f%%  (%s / %s)",
		memAllocBar, memAllocPct,
		formatQuantity(c.TotalAllocatedMemoryRequests),
		formatQuantity(c.TotalAllocatableMemory))
	fmt.Println(padRightDynamic(memAllocLine, boxWidth) + "║")

	fmt.Println(boxStyle.Sprint(botBorder))
}

// buildColoredProgressBarDynamic creates a colored progress bar with dynamic width.
func buildColoredProgressBarDynamic(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	filled := int(pct / 100 * float64(width))
	empty := width - filled

	var color text.Colors
	switch {
	case pct >= 90:
		color = text.Colors{text.FgRed, text.Bold}
	case pct >= 75:
		color = text.Colors{text.FgYellow}
	case pct >= 50:
		color = text.Colors{text.FgHiYellow}
	default:
		color = text.Colors{text.FgGreen}
	}

	bar := color.Sprint(strings.Repeat("█", filled)) +
		text.Colors{text.FgHiBlack}.Sprint(strings.Repeat("░", empty))

	return "[" + bar + "]"
}

// padRightDynamic pads a string to the specified width (accounting for ANSI codes).
func padRightDynamic(s string, width int) string {
	visibleLen := text.RuneWidthWithoutEscSequences(s)
	if visibleLen >= width+1 { // +1 for the closing border
		return s
	}
	return s + strings.Repeat(" ", width+1-visibleLen)
}

// buildSparkline creates a mini sparkline showing trend.
func buildSparkline(values ...float64) string {
	chars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	if len(values) == 0 {
		return ""
	}

	// Find max value
	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal == 0 {
		return strings.Repeat(string(chars[0]), len(values))
	}

	result := ""
	for _, v := range values {
		idx := int((v / maxVal) * float64(len(chars)-1))
		if idx >= len(chars) {
			idx = len(chars) - 1
		}
		result += string(chars[idx])
	}
	return result
}

// buildCPUUtilizationCell creates a detailed CPU utilization cell.
func buildCPUUtilizationCell(v *NodeStats) string {
	if v.AllocatableCPU == nil || v.AllocatableCPU.IsZero() {
		return "N/A"
	}

	usagePct := calculatePercentageFromQuantities(v.UsageCPU, v.AllocatableCPU)
	reqPct := calculatePercentageFromQuantities(&v.AllocatedCPUrequests, v.AllocatableCPU)
	limPct := calculatePercentageFromQuantities(&v.AllocatedCPULimits, v.AllocatableCPU)

	// Mini progress bar (15 chars)
	bar := buildMiniProgressBar(usagePct, 12)

	// Sparkline showing req, usage, limit trend
	spark := buildSparkline(reqPct, usagePct, limPct)

	return fmt.Sprintf("%s %5.1f%% %s\nReq: %s  Limit: %s",
		bar, usagePct, spark,
		formatQuantityValue(v.AllocatedCPUrequests),
		formatQuantityValue(v.AllocatedCPULimits))
}

// buildMemUtilizationCell creates a detailed memory utilization cell.
func buildMemUtilizationCell(v *NodeStats) string {
	if v.AllocatableMemory == nil || v.AllocatableMemory.IsZero() {
		return "N/A"
	}

	usagePct := calculatePercentageFromQuantities(v.UsageMemory, v.AllocatableMemory)
	reqPct := calculatePercentageFromQuantities(&v.AllocatedMemoryRequests, v.AllocatableMemory)
	limPct := calculatePercentageFromQuantities(&v.AllocatedMemoryLimits, v.AllocatableMemory)

	// Mini progress bar (15 chars)
	bar := buildMiniProgressBar(usagePct, 12)

	// Sparkline showing req, usage, limit trend
	spark := buildSparkline(reqPct, usagePct, limPct)

	return fmt.Sprintf("%s %5.1f%% %s\nReq: %s  Lim: %s",
		bar, usagePct, spark,
		formatQuantityValue(v.AllocatedMemoryRequests),
		formatQuantityValue(v.AllocatedMemoryLimits))
}

// buildMiniProgressBar creates a compact colored progress bar
func buildMiniProgressBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	filled := int(pct / 100 * float64(width))
	empty := width - filled

	var color text.Colors
	switch {
	case pct >= 90:
		color = text.Colors{text.FgRed, text.Bold}
	case pct >= 75:
		color = text.Colors{text.FgYellow}
	case pct >= 50:
		color = text.Colors{text.FgHiYellow}
	default:
		color = text.Colors{text.FgGreen}
	}

	return color.Sprint(strings.Repeat("▓", filled)) +
		text.Colors{text.FgHiBlack}.Sprint(strings.Repeat("░", empty))
}

// buildTotalCPUCell creates the totals CPU cell
func buildTotalCPUCell(c *Totals) string {
	usagePct := calculatePercentage(c.TotalUsageCPU, c.TotalAllocatableCPU)
	reqPct := calculatePercentage(c.TotalAllocatedCPUrequests, c.TotalAllocatableCPU)

	return fmt.Sprintf("Usage: %5.1f%%  Req: %5.1f%%\n%s / %s",
		usagePct, reqPct,
		formatQuantity(c.TotalUsageCPU),
		formatQuantity(c.TotalAllocatableCPU))
}

// buildTotalMemCell creates the totals memory cell
func buildTotalMemCell(c *Totals) string {
	usagePct := calculatePercentage(c.TotalUsageMemory, c.TotalAllocatableMemory)
	reqPct := calculatePercentage(c.TotalAllocatedMemoryRequests, c.TotalAllocatableMemory)

	return fmt.Sprintf("Usage: %5.1f%%  Req: %5.1f%%\n%s / %s",
		usagePct, reqPct,
		formatQuantity(c.TotalUsageMemory),
		formatQuantity(c.TotalAllocatableMemory))
}

// calculatePercentage calculates percentage from two quantities
func calculatePercentage(used, total *resource.Quantity) float64 {
	if total == nil || total.IsZero() || used == nil {
		return 0
	}
	return float64(used.MilliValue()) / float64(total.MilliValue()) * 100
}

// calculatePercentageFromQuantities calculates percentage (handles nil)
func calculatePercentageFromQuantities(used, total *resource.Quantity) float64 {
	if total == nil || total.IsZero() {
		return 0
	}
	if used == nil {
		return 0
	}
	return float64(used.MilliValue()) / float64(total.MilliValue()) * 100
}

// centerText centers a string within a given width
func centerText(s string, width int) string {
	visibleLen := text.RuneWidthWithoutEscSequences(s)
	if visibleLen >= width {
		return s
	}
	leftPad := (width - visibleLen) / 2
	rightPad := width - visibleLen - leftPad
	return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
}

// printLegend prints a color legend
func printLegend() {
	fmt.Println()
	fmt.Println(text.Colors{text.FgHiBlack}.Sprint("Legend: ") +
		text.Colors{text.FgGreen}.Sprint("■ <50%") + "  " +
		text.Colors{text.FgHiYellow}.Sprint("■ 50-75%") + "  " +
		text.Colors{text.FgYellow}.Sprint("■ 75-90%") + "  " +
		text.Colors{text.FgRed}.Sprint("■ >90%") + "  " +
		text.Colors{text.FgHiBlack}.Sprint("Sparkline: Req→Usage→Limit"))
	fmt.Println()
}

func table(nm *NodeMap, c *Totals) {
	// Sort nodes by name for consistent output
	nodeNames := make([]string, 0, len(*nm))
	for name := range *nm {
		nodeNames = append(nodeNames, name)
	}
	sort.Strings(nodeNames)

	// Print header with cluster info
	fmt.Println()
	fmt.Println(strings.Repeat("=", 120))
	fmt.Println("  KUBERNETES CLUSTER RESOURCE OVERVIEW")
	if c.ClusterInfo.Host != "" {
		fmt.Printf("  Host: %s  |  Version: %s  |  Nodes: %d\n",
			c.ClusterInfo.Host, c.ClusterInfo.MasterVersion, len(*nm))
	}
	fmt.Println(strings.Repeat("=", 120))
	fmt.Println()

	// Create main node table with borders
	t := pt.NewWriter()
	t.SetStyle(pt.StyleLight)
	t.SetOutputMirror(os.Stdout)

	// Configure column alignment
	t.SetColumnConfigs([]pt.ColumnConfig{
		{Number: 1, Align: text.AlignLeft},   // Name
		{Number: 2, Align: text.AlignCenter}, // Status
		{Number: 3, Align: text.AlignLeft},   // Version
		{Number: 4, Align: text.AlignRight},  // CPU Req
		{Number: 5, Align: text.AlignRight},  // CPU Lim
		{Number: 6, Align: text.AlignRight},  // CPU Use
		{Number: 7, Align: text.AlignRight},  // CPU %
		{Number: 8, Align: text.AlignRight},  // Mem Req
		{Number: 9, Align: text.AlignRight},  // Mem Lim
		{Number: 10, Align: text.AlignRight}, // Mem Use
		{Number: 11, Align: text.AlignRight}, // Mem %
	})

	t.AppendHeader(pt.Row{
		"NODE", "STATUS", "VERSION",
		"CPU REQ", "CPU LIM", "CPU USE", "CPU %",
		"MEM REQ", "MEM LIM", "MEM USE", "MEM %",
	})

	// Add node rows
	for _, name := range nodeNames {
		v := (*nm)[name]

		// Calculate utilization percentages
		cpuPct := "--"
		if v.AllocatableCPU != nil && v.UsageCPU != nil && v.AllocatableCPU.MilliValue() > 0 {
			pct := float64(v.UsageCPU.MilliValue()) / float64(v.AllocatableCPU.MilliValue()) * 100
			cpuPct = fmt.Sprintf("%.1f%%", pct)
		}

		memPct := "--"
		if v.AllocatableMemory != nil && v.UsageMemory != nil && v.AllocatableMemory.Value() > 0 {
			pct := float64(v.UsageMemory.Value()) / float64(v.AllocatableMemory.Value()) * 100
			memPct = fmt.Sprintf("%.1f%%", pct)
		}

		// Format status with indicator
		status := v.Status
		if status == "" {
			status = "Unknown"
		}

		t.AppendRow(pt.Row{
			name,
			status,
			v.NodeInfo.KubeletVersion,
			formatQuantityValue(v.AllocatedCPUrequests),
			formatQuantityValue(v.AllocatedCPULimits),
			formatQuantity(v.UsageCPU),
			cpuPct,
			formatQuantityValue(v.AllocatedMemoryRequests),
			formatQuantityValue(v.AllocatedMemoryLimits),
			formatQuantity(v.UsageMemory),
			memPct,
		})
	}

	// Add totals footer
	totalCPUPct := "--"
	if c.TotalAllocatableCPU != nil && c.TotalUsageCPU != nil && c.TotalAllocatableCPU.MilliValue() > 0 {
		pct := float64(c.TotalUsageCPU.MilliValue()) / float64(c.TotalAllocatableCPU.MilliValue()) * 100
		totalCPUPct = fmt.Sprintf("%.1f%%", pct)
	}

	totalMemPct := "--"
	if c.TotalAllocatableMemory != nil && c.TotalUsageMemory != nil && c.TotalAllocatableMemory.Value() > 0 {
		pct := float64(c.TotalUsageMemory.Value()) / float64(c.TotalAllocatableMemory.Value()) * 100
		totalMemPct = fmt.Sprintf("%.1f%%", pct)
	}

	t.AppendSeparator()
	t.AppendFooter(pt.Row{
		"TOTALS",
		fmt.Sprintf("%d nodes", len(*nm)),
		"",
		formatQuantity(c.TotalAllocatedCPUrequests),
		formatQuantity(c.TotalAllocatedCPULimits),
		formatQuantity(c.TotalUsageCPU),
		totalCPUPct,
		formatQuantity(c.TotalAllocatedMemoryRequests),
		formatQuantity(c.TotalAllocatedMemoryLimits),
		formatQuantity(c.TotalUsageMemory),
		totalMemPct,
	})

	t.Render()

	// Print capacity summary
	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("  CLUSTER CAPACITY")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  Allocatable CPU:    %s\n", formatQuantity(c.TotalAllocatableCPU))
	fmt.Printf("  Allocatable Memory: %s\n", formatQuantity(c.TotalAllocatableMemory))
	fmt.Printf("  Total Capacity CPU: %s\n", formatQuantity(c.TotalCapacityCPU))
	fmt.Printf("  Total Capacity Mem: %s\n", formatQuantity(c.TotalCapacityMemory))
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()

	os.Exit(0)
}

func chart(_ *NodeMap) {
	log.Fatalf("Not yet implemented")
	// TODO: Fix go-echarts compatibility with current version
	// The BarData and Label types have changed, this needs updating
}

func dash(nm *NodeMap) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	cpu := widgets.NewBarChart()
	mem := widgets.NewBarChart()
	for k, v := range *nm {
		cpu.Title = k + " CPU"
		cpu.Data = []float64{
			float64(v.AllocatedCPUrequests.ScaledValue(resource.Micro)),
			float64(v.AllocatedCPULimits.ScaledValue(resource.Micro)),
			float64(v.AllocatedCPULimits.ScaledValue(resource.Micro))}
		mem.Title = k + " Memory"
		mem.Data = []float64{
			float64(v.AllocatedMemoryRequests.Value()),
			float64(v.AllocatedMemoryLimits.Value()),
			float64(v.UsageMemory.Value())}
	}
	cpu.Labels = []string{"Allocated", "Limits", "Usage"}
	cpu.SetRect(0, 0, 50, 5)
	cpu.BarWidth = 10
	cpu.BarColors = []ui.Color{ui.ColorRed, ui.ColorGreen}
	cpu.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorBlue)}
	cpu.NumStyles = []ui.Style{ui.NewStyle(ui.ColorYellow)}

	mem.Labels = []string{"Allocated", "Limits", "Usage"}
	mem.SetRect(0, 5, 50, 25)
	mem.BarWidth = 10
	mem.BarColors = []ui.Color{ui.ColorRed, ui.ColorGreen}
	mem.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorBlue)}
	mem.NumStyles = []ui.Style{ui.NewStyle(ui.ColorYellow)}

	ui.Render(cpu, mem)

	uiEvents := ui.PollEvents()
	for {
		e := <-uiEvents
		switch e.ID {
		case "q", ctlC:
			return
		}
	}
}

func pie(nm *NodeMap) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	cpu := widgets.NewPieChart()
	mem := widgets.NewPieChart()
	for k, v := range *nm {
		cpu.Title = k + " CPU"
		cpu.Data = []float64{
			float64(v.AllocatedCPUrequests.ScaledValue(resource.Micro)),
			float64(v.AllocatedCPULimits.ScaledValue(resource.Micro)),
			float64(v.AllocatedCPULimits.ScaledValue(resource.Micro))}
		mem.Title = k + " Memory"
		mem.Data = []float64{
			float64(v.AllocatedMemoryRequests.Value()),
			float64(v.AllocatedMemoryLimits.Value()),
			float64(v.UsageMemory.Value())}
	}

	cpu.SetRect(0, 0, 50, 5)
	mem.SetRect(0, 5, 50, 25)

	ui.Render(cpu, mem)

	uiEvents := ui.PollEvents()
	for {
		e := <-uiEvents
		switch e.ID {
		case "q", ctlC:
			return
		}
	}
}
