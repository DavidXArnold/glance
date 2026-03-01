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
	core "gitlab.com/davidxarnold/glance/pkg/core"
	glanceutil "gitlab.com/davidxarnold/glance/pkg/util"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/clientcmd"

	// needed to authenticate with GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const (
	minBoxWidth     = 60
	maxBoxWidth     = 120
	defaultBoxWidth = 80

	outputFormatJSON   = "json"
	outputFormatPretty = "pretty"
)

const ctlC = "<C-c>"

// formatQuantity returns a human-readable or exact representation of a quantity pointer.
func formatQuantity(q *resource.Quantity) string {
	if q == nil {
		return ""
	}

	if viper.GetBool("exact") || viper.GetBool("show-raw") {
		// Exact value mode - show the raw value
		return q.String()
	}

	// Human-readable mode - determine if CPU or Memory based on scale
	// CPU values are typically in milli range, memory in bytes
	if q.MilliValue() < 100000 && q.Value() < 1000000 {
		// Likely CPU (small millivalue)
		return formatMilliCPU(q)
	}
	// Likely Memory (large byte value)
	return formatBytes(q)
}

// formatQuantityValue returns a human-readable or exact representation of a quantity value.
func formatQuantityValue(q resource.Quantity) string {
	if viper.GetBool("exact") || viper.GetBool("show-raw") {
		// Exact value mode - show the raw value
		return q.String()
	}

	// Human-readable mode
	if q.MilliValue() < 100000 && q.Value() < 1000000 {
		// Likely CPU
		return formatMilliCPU(&q)
	}
	// Likely Memory
	return formatBytes(&q)
}

// formatResourceRatioFromStrings formats resource strings as ratio (used / total).
// Converts string quantities to resource.Quantity then formats as ratio.
// nolint:unused // Reserved for future static view ratio formatting
func formatResourceRatioFromStrings(usedStr, totalStr string, isMemory bool, showRaw bool) string {
	used, err := resource.ParseQuantity(usedStr)
	if err != nil {
		return fmt.Sprintf("%s / %s", usedStr, totalStr)
	}
	total, err := resource.ParseQuantity(totalStr)
	if err != nil {
		return fmt.Sprintf("%s / %s", usedStr, totalStr)
	}
	// Call the function from live.go
	return formatResourceRatio(&used, &total, isMemory, showRaw)
}

func render(nm *core.NodeMap, c *core.Totals) error {
	switch viper.GetString("output") {
	case outputFormatJSON:
		return renderJSON(nm, c)
	case outputFormatPretty:
		return renderPretty(nm, c)
	case "chart":
		return chart(nm)
	case "dash":
		return dash(nm)
	case "pie":
		return pie(nm)
	default:
		table(nm, c)
		return nil
	}
}

// renderPodsStatic renders pod summaries according to the global output format.
func renderPodsStatic(rows []PodSummaryRow) error {
	output := viper.GetString("output")
	if output == outputFormatJSON {
		b, err := json.MarshalIndent(rows, "", "\t")
		if err != nil {
			log.Errorf("failed to marshal pods to JSON: %v", err)
			return fmt.Errorf("failed to render pods JSON output: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	// txt/pretty: use a simple table. We treat both the same for now.
	t := pt.NewWriter()
	t.SetOutputMirror(os.Stdout)
	if output == outputFormatPretty {
		t.SetStyle(pt.StyleRounded)
	} else {
		t.SetStyle(pt.StyleLight)
	}

	showGPU := viper.GetBool("show-gpu")

	headerRow := pt.Row{
		"NAMESPACE",
		"POD",
		"CPU REQUESTS/LIMITS",
		"CPU USAGE/LIMITS",
		"MEMORY REQUESTS/LIMITS",
		"MEMORY USAGE/LIMITS",
	}
	if showGPU {
		headerRow = append(headerRow, "GPU REQ/LIMIT")
	}
	headerRow = append(headerRow, "STATUS")
	t.AppendHeader(headerRow)

	showRaw := viper.GetBool("show-raw") || viper.GetBool("exact")

	for _, r := range rows {
		cpuReq := r.CPUReq
		cpuLimit := r.CPULimit
		cpuUsage := r.CPUUsage
		memReq := r.MemReq
		memLimit := r.MemLimit
		memUsage := r.MemUsage

		row := pt.Row{
			r.Namespace,
			r.Name,
			formatResourceRatio(cpuReq, cpuLimit, false, showRaw),
			formatResourceRatio(cpuUsage, cpuLimit, false, showRaw),
			formatResourceRatio(memReq, memLimit, true, showRaw),
			formatResourceRatio(memUsage, memLimit, true, showRaw),
		}
		if showGPU {
			if r.GPUReq != nil && r.GPUReq.Value() > 0 {
				gpuLimVal := int64(0)
				if r.GPULimit != nil {
					gpuLimVal = r.GPULimit.Value()
				}
				row = append(row, fmt.Sprintf("%d / %d", r.GPUReq.Value(), gpuLimVal))
			} else {
				row = append(row, "—")
			}
		}
		row = append(row, r.Status)
		t.AppendRow(row)
	}

	t.Render()
	return nil
}

// renderDeploymentsStatic renders deployment summaries according to the global output format.
func renderDeploymentsStatic(rows []DeploymentSummaryRow) error {
	output := viper.GetString("output")
	if output == outputFormatJSON {
		b, err := json.MarshalIndent(rows, "", "\t")
		if err != nil {
			log.Errorf("failed to marshal deployments to JSON: %v", err)
			return fmt.Errorf("failed to render deployments JSON output: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	t := pt.NewWriter()
	t.SetOutputMirror(os.Stdout)
	if output == outputFormatPretty {
		t.SetStyle(pt.StyleRounded)
	} else {
		t.SetStyle(pt.StyleLight)
	}

	showGPU := viper.GetBool("show-gpu")

	headerRow := pt.Row{
		"NAMESPACE",
		"DEPLOYMENT",
		"STATUS",
		"CPU REQUESTS/LIMITS",
		"MEMORY REQUESTS/LIMITS",
	}
	if showGPU {
		headerRow = append(headerRow, "GPU REQ/LIMIT")
	}
	headerRow = append(headerRow, "REPLICAS", "READY", "AVAILABLE")
	t.AppendHeader(headerRow)

	showRaw := viper.GetBool("show-raw") || viper.GetBool("exact")

	for _, r := range rows {
		cpuReq := r.CPUReq
		cpuLimit := r.CPULimit
		memReq := r.MemReq
		memLimit := r.MemLimit

		row := pt.Row{
			r.Namespace,
			r.Name,
			r.Status,
			formatResourceRatio(cpuReq, cpuLimit, false, showRaw),
			formatResourceRatio(memReq, memLimit, true, showRaw),
		}
		if showGPU {
			if r.GPUReq != nil && r.GPUReq.Value() > 0 {
				gpuLimVal := int64(0)
				if r.GPULimit != nil {
					gpuLimVal = r.GPULimit.Value()
				}
				row = append(row, fmt.Sprintf("%d / %d", r.GPUReq.Value(), gpuLimVal))
			} else {
				row = append(row, "—")
			}
		}
		row = append(row,
			fmt.Sprintf("%d", r.Replicas),
			fmt.Sprintf("%d", r.Ready),
			fmt.Sprintf("%d", r.Available),
		)
		t.AppendRow(row)
	}

	t.Render()
	return nil
}

func renderJSON(nm *core.NodeMap, c *core.Totals) error {
	snapshot := core.NewSnapshot(*nm, *c)
	g, err := json.MarshalIndent(snapshot, "", "\t")
	if err != nil {
		log.Errorf("failed to marshal snapshot to JSON: %v", err)
		return fmt.Errorf("failed to render JSON output: %w", err)
	}
	fmt.Println(string(g))
	return nil
}

// buildNodeRow creates a table row for a single node
func buildNodeRow(
	name string,
	v *core.NodeStats,
	status string,
	statusColor text.Colors,
	showVersion, showAge, showGroup, showGPU, showCloud bool,
) pt.Row {
	row := pt.Row{
		name,
		statusColor.Sprint(status),
	}
	if showVersion {
		row = append(row, v.NodeInfo.KubeletVersion)
	}
	if showAge {
		age := ""
		if !v.CreationTime.IsZero() {
			age = glanceutil.FormatAge(v.CreationTime)
		}
		row = append(row, age)
	}
	if showGroup {
		row = append(row, v.NodeGroup)
	}
	row = append(row,
		buildCPUUtilizationCell(v),
		buildMemUtilizationCell(v),
	)
	if showGPU {
		row = append(row, buildGPUUtilizationCell(v))
	}
	if showCloud {
		// Parse provider from ProviderID
		provider := ""
		if v.ProviderID != "" {
			cp, _ := glanceutil.ParseProviderID(v.ProviderID)
			provider = strings.ToUpper(cp)
		}
		row = append(row, provider, v.Region, v.InstanceType, v.CapacityType)
	}
	return row
}

func renderPretty(nm *core.NodeMap, c *core.Totals) error {
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

	// Decide which node columns to show. VERSION is shown by default to
	// preserve legacy behavior unless the user explicitly disables it via
	// config or flag.
	showVersion := true
	if viper.IsSet("show-node-version") {
		showVersion = viper.GetBool("show-node-version")
	}
	showAge := viper.GetBool("show-node-age")
	showGroup := viper.GetBool("show-node-group")
	showGPU := viper.GetBool("show-gpu")

	// Create main table
	t := pt.NewWriter()
	t.SetStyle(pt.StyleRounded)
	t.SetOutputMirror(os.Stdout)

	// Configure column colors (dynamic indices based on enabled columns).
	col := 1
	colNode := col
	col++
	colStatus := col
	col++
	colVersion := 0
	if showVersion {
		colVersion = col
		col++
	}
	colAge := 0
	if showAge {
		colAge = col
		col++
	}
	colGroup := 0
	if showGroup {
		colGroup = col
		col++
	}
	colCPU := col
	col++
	colMem := col
	col++
	colGPU := 0
	if showGPU {
		colGPU = col
		col++
	}

	showCloud := viper.GetBool("show-cloud-provider")
	colProvider, colRegion, colInstance, colCapacity := 0, 0, 0, 0
	if showCloud {
		colProvider = col
		col++
		colRegion = col
		col++
		colInstance = col
		col++
		colCapacity = col
		// No increment needed for last column
	}

	baseColumns := []pt.ColumnConfig{
		{Number: colNode, AutoMerge: false, Colors: text.Colors{text.FgHiWhite, text.Bold}},
		{Number: colStatus, AutoMerge: false},
	}
	if colVersion != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{
			Number:    colVersion,
			AutoMerge: false,
			Colors:    text.Colors{text.FgHiCyan},
		})
	}
	if colAge != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colAge, AutoMerge: false})
	}
	if colGroup != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colGroup, AutoMerge: false})
	}
	baseColumns = append(baseColumns,
		pt.ColumnConfig{Number: colCPU, AutoMerge: false},
		pt.ColumnConfig{Number: colMem, AutoMerge: false},
	)
	if colGPU != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colGPU, AutoMerge: false})
	}

	if showCloud {
		baseColumns = append(baseColumns,
			pt.ColumnConfig{Number: colProvider, AutoMerge: false},
			pt.ColumnConfig{Number: colRegion, AutoMerge: false},
			pt.ColumnConfig{Number: colInstance, AutoMerge: false},
			pt.ColumnConfig{Number: colCapacity, AutoMerge: false},
		)
	}
	t.SetColumnConfigs(baseColumns)

	headerRow := pt.Row{
		"NODE",
		"STATUS",
	}
	if showVersion {
		headerRow = append(headerRow, "VERSION")
	}
	if showAge {
		headerRow = append(headerRow, "AGE")
	}
	if showGroup {
		headerRow = append(headerRow, "GROUP")
	}
	headerRow = append(headerRow,
		"CPU UTILIZATION",
		"MEMORY UTILIZATION",
	)
	if showGPU {
		headerRow = append(headerRow, "GPU UTILIZATION")
	}
	if showCloud {
		headerRow = append(headerRow, "PROVIDER", "REGION", "INSTANCE TYPE", "CAPACITY")
	}
	t.AppendHeader(headerRow)

	// Add NotReady nodes first (if any) with red highlighting
	for _, name := range notReadyNodes {
		v := (*nm)[name]
		row := buildNodeRow(
			name,
			v,
			"⊘ "+v.Status,
			text.Colors{text.FgRed, text.Bold},
			showVersion,
			showAge,
			showGroup,
			showGPU,
			showCloud,
		)
		t.AppendRow(row)
	}

	// Add separator if we have both types
	if len(notReadyNodes) > 0 && len(readyNodes) > 0 {
		t.AppendSeparator()
	}

	// Add Ready nodes with green status
	for _, name := range readyNodes {
		v := (*nm)[name]
		row := buildNodeRow(
			name,
			v,
			"✓ "+v.Status,
			text.Colors{text.FgGreen, text.Bold},
			showVersion,
			showAge,
			showGroup,
			showGPU,
			showCloud,
		)
		t.AppendRow(row)
	}

	t.AppendSeparator()
	footerRow := pt.Row{
		text.Colors{text.FgHiWhite, text.Bold}.Sprint("CLUSTER TOTALS"),
		fmt.Sprintf("%d nodes", len(*nm)),
	}
	if showVersion {
		footerRow = append(footerRow, "")
	}
	if showAge {
		footerRow = append(footerRow, "")
	}
	if showGroup {
		footerRow = append(footerRow, "")
	}
	footerRow = append(footerRow,
		buildTotalCPUCell(c),
		buildTotalMemCell(c),
	)
	if showGPU {
		footerRow = append(footerRow, buildTotalGPUCell(c))
	}
	if showCloud {
		footerRow = append(footerRow, "", "", "", "")
	}
	t.AppendFooter(footerRow)

	fmt.Println()
	t.Render()

	// Print legend
	printLegend()

	return nil
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

	// Determine context and cloud information for header.
	ctxName, cloudProvider, cloudCluster := getHeaderContextAndCloud(nm)

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

	headerText := "glance (<) (<)"
	if ctxName != "" {
		headerText += fmt.Sprintf("   context: %s", ctxName)
	}
	if viper.GetBool("show-cloud-provider") && cloudProvider != "" {
		headerText += fmt.Sprintf("   cloud: %s", cloudProvider)
		if cloudCluster != "" {
			headerText += fmt.Sprintf("   cluster: %s", cloudCluster)
		}
	}

	titleLine := "║" + centerText(headerText, boxWidth) + "║"

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

	// GPU Allocated (only shown when GPUs are present in the cluster)
	if c.TotalAllocatableGPU != nil && !c.TotalAllocatableGPU.IsZero() {
		fmt.Println("║" + strings.Repeat(" ", boxWidth) + "║")
		gpuAllocPct := float64(0)
		if c.TotalAllocatableGPU.Value() > 0 {
			gpuAllocPct = float64(c.TotalAllocatedGPURequests.Value()) /
				float64(c.TotalAllocatableGPU.Value()) * 100
		}
		gpuAllocBar := buildColoredProgressBarDynamic(gpuAllocPct, barWidth)
		gpuAllocLine := fmt.Sprintf("║  GPU Allocated:  %s %5.1f%%  (%d / %d)",
			gpuAllocBar, gpuAllocPct,
			c.TotalAllocatedGPURequests.Value(),
			c.TotalAllocatableGPU.Value())
		fmt.Println(padRightDynamic(gpuAllocLine, boxWidth) + "║")
	}

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

// buildGPUUtilizationCell creates a GPU utilization cell showing requested / allocatable.
func buildGPUUtilizationCell(v *NodeStats) string {
	if v.AllocatableGPU == nil || v.AllocatableGPU.IsZero() {
		return "—"
	}

	alloc := v.AllocatableGPU.Value()
	req := v.AllocatedGPURequests.Value()
	pct := float64(req) / float64(alloc) * 100

	bar := buildMiniProgressBar(pct, 12)

	return fmt.Sprintf("%s %5.1f%%\nReq: %d / %d",
		bar, pct, req, alloc)
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
func buildTotalCPUCell(c *core.Totals) string {
	usagePct := calculatePercentage(c.TotalUsageCPU, c.TotalAllocatableCPU)
	reqPct := calculatePercentage(c.TotalAllocatedCPUrequests, c.TotalAllocatableCPU)

	return fmt.Sprintf("Usage: %5.1f%%  Req: %5.1f%%\n%s / %s",
		usagePct, reqPct,
		formatQuantity(c.TotalUsageCPU),
		formatQuantity(c.TotalAllocatableCPU))
}

// buildTotalGPUCell creates the totals GPU cell.
func buildTotalGPUCell(c *core.Totals) string {
	if c.TotalAllocatableGPU == nil || c.TotalAllocatableGPU.IsZero() {
		return "—"
	}
	reqPct := float64(c.TotalAllocatedGPURequests.Value()) /
		float64(c.TotalAllocatableGPU.Value()) * 100

	return fmt.Sprintf("Req: %5.1f%%\n%d / %d",
		reqPct,
		c.TotalAllocatedGPURequests.Value(),
		c.TotalAllocatableGPU.Value())
}

// buildTotalMemCell creates the totals memory cell
func buildTotalMemCell(c *core.Totals) string {
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

// getHeaderContextAndCloud derives context and cloud info for the summary header.
// - context: current kubeconfig context name (if available)
// - cloud: first detected provider from node ProviderIDs (AWS/GCE, etc.)
// - cluster: kubeconfig cluster name for the current context (best-effort)
func getHeaderContextAndCloud(nm *NodeMap) (contextName, cloudProvider, cloudCluster string) {
	// Derive context/cluster from kubeconfig when possible.
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		log.Debugf("Unable to determine kubeconfig context for header: %v", err)
	} else {
		ctxName := rawConfig.CurrentContext
		if ctxName != "" {
			contextName = ctxName
		}
		if ctx, ok := rawConfig.Contexts[ctxName]; ok && ctx.Cluster != "" {
			cloudCluster = ctx.Cluster
		}
	}

	// Derive cloud provider from the first node with a ProviderID.
	if nm != nil {
		for _, node := range *nm {
			if node.ProviderID == "" {
				continue
			}
			cp, _ := glanceutil.ParseProviderID(node.ProviderID)
			if cp != "" {
				cloudProvider = strings.ToUpper(cp)
				break
			}
		}
	}

	return contextName, cloudProvider, cloudCluster
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

// buildTableRow creates a standard table row for a single node (text output)
func buildTableRow(name string, v *core.NodeStats, showVersion, showAge, showGroup, showGPU, showCloud bool) pt.Row {
	// Calculate utilization percentages.
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

	// Format status with indicator.
	status := v.Status
	if status == "" {
		status = "Unknown"
	}

	row := pt.Row{name, status}
	if showVersion {
		row = append(row, v.NodeInfo.KubeletVersion)
	}
	if showAge {
		age := ""
		if !v.CreationTime.IsZero() {
			age = glanceutil.FormatAge(v.CreationTime)
		}
		row = append(row, age)
	}
	if showGroup {
		row = append(row, v.NodeGroup)
	}

	row = append(row,
		formatQuantityValue(v.AllocatedCPUrequests),
		formatQuantityValue(v.AllocatedCPULimits),
		formatQuantity(v.UsageCPU),
		cpuPct,
		formatQuantityValue(v.AllocatedMemoryRequests),
		formatQuantityValue(v.AllocatedMemoryLimits),
		formatQuantity(v.UsageMemory),
		memPct,
	)

	if showGPU {
		gpuReq := fmt.Sprintf("%d", v.AllocatedGPURequests.Value())
		gpuAlloc := "0"
		if v.AllocatableGPU != nil {
			gpuAlloc = fmt.Sprintf("%d", v.AllocatableGPU.Value())
		}
		row = append(row, gpuReq+" / "+gpuAlloc)
	}

	if showCloud {
		// Parse provider from ProviderID.
		provider := ""
		if v.ProviderID != "" {
			cp, _ := glanceutil.ParseProviderID(v.ProviderID)
			provider = strings.ToUpper(cp)
		}
		row = append(row, provider, v.Region, v.InstanceType, v.CapacityType)
	}

	return row
}

// buildTableFooter creates the footer row for the text table
func buildTableFooter(c *core.Totals, numNodes int, showVersion, showAge, showGroup, showGPU, showCloud bool) pt.Row {
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

	footerRow := pt.Row{
		"TOTALS",
		fmt.Sprintf("%d nodes", numNodes),
	}
	if showVersion {
		footerRow = append(footerRow, "")
	}
	if showAge {
		footerRow = append(footerRow, "")
	}
	if showGroup {
		footerRow = append(footerRow, "")
	}
	footerRow = append(footerRow,
		formatQuantity(c.TotalAllocatedCPUrequests),
		formatQuantity(c.TotalAllocatedCPULimits),
		formatQuantity(c.TotalUsageCPU),
		totalCPUPct,
		formatQuantity(c.TotalAllocatedMemoryRequests),
		formatQuantity(c.TotalAllocatedMemoryLimits),
		formatQuantity(c.TotalUsageMemory),
		totalMemPct,
	)
	if showGPU {
		gpuTotal := "0 / 0"
		if c.TotalAllocatableGPU != nil && !c.TotalAllocatableGPU.IsZero() {
			gpuTotal = fmt.Sprintf("%d / %d",
				c.TotalAllocatedGPURequests.Value(),
				c.TotalAllocatableGPU.Value())
		}
		footerRow = append(footerRow, gpuTotal)
	}
	if showCloud {
		footerRow = append(footerRow, "", "", "", "")
	}
	return footerRow
}

func table(nm *core.NodeMap, c *core.Totals) {
	// Sort nodes by name for consistent output.
	nodeNames := make([]string, 0, len(*nm))
	for name := range *nm {
		nodeNames = append(nodeNames, name)
	}
	sort.Strings(nodeNames)

	// Print header with cluster info.
	fmt.Println()
	fmt.Println(strings.Repeat("=", 120))

	ctxName, cloudProvider, cloudCluster := getHeaderContextAndCloud(nm)
	header := "glance"
	if ctxName != "" {
		header += fmt.Sprintf("   context: %s", ctxName)
	}
	if viper.GetBool("show-cloud-provider") && cloudProvider != "" {
		header += fmt.Sprintf("   cloud: %s", cloudProvider)
		if cloudCluster != "" {
			header += fmt.Sprintf("   cluster: %s", cloudCluster)
		}
	}
	fmt.Println("  " + header)

	if c.ClusterInfo.Host != "" {
		fmt.Printf("  Host: %s  |  Version: %s  |  Nodes: %d\n",
			c.ClusterInfo.Host, c.ClusterInfo.MasterVersion, len(*nm))
	}
	fmt.Println(strings.Repeat("=", 120))
	fmt.Println()

	// Decide which node columns to show. VERSION is shown by default to
	// preserve legacy behavior unless the user explicitly disables it via
	// config or flag.
	showVersion := true
	if viper.IsSet("show-node-version") {
		showVersion = viper.GetBool("show-node-version")
	}
	showAge := viper.GetBool("show-node-age")
	showGroup := viper.GetBool("show-node-group")
	showGPU := viper.GetBool("show-gpu")

	// Create main node table with borders.
	t := pt.NewWriter()
	t.SetStyle(pt.StyleLight)
	t.SetOutputMirror(os.Stdout)

	// Configure column alignment dynamically based on which optional node
	// columns are enabled.
	col := 1
	colNode := col
	col++
	colStatus := col
	col++
	colVersion := 0
	if showVersion {
		colVersion = col
		col++
	}
	colAge := 0
	if showAge {
		colAge = col
		col++
	}
	colGroup := 0
	if showGroup {
		colGroup = col
		col++
	}

	colCPUReq := col
	col++
	colCPULim := col
	col++
	colCPUUse := col
	col++
	colCPUPct := col
	col++
	colMemReq := col
	col++
	colMemLim := col
	col++
	colMemUse := col
	col++
	colMemPct := col
	col++
	colGPU := 0
	if showGPU {
		colGPU = col
		col++
	}

	showCloud := viper.GetBool("show-cloud-provider")
	colProvider, colRegion, colInstance, colCapacity := 0, 0, 0, 0
	if showCloud {
		colProvider = col
		col++
		colRegion = col
		col++
		colInstance = col
		col++
		colCapacity = col
		// col++ is not needed for the last column
	}

	var baseColumns []pt.ColumnConfig
	baseColumns = append(baseColumns,
		pt.ColumnConfig{Number: colNode, Align: text.AlignLeft},     // Name
		pt.ColumnConfig{Number: colStatus, Align: text.AlignCenter}, // Status
	)
	if colVersion != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colVersion, Align: text.AlignLeft}) // Version
	}
	if colAge != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colAge, Align: text.AlignLeft}) // Age
	}
	if colGroup != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colGroup, Align: text.AlignLeft}) // Group
	}

	baseColumns = append(baseColumns,
		pt.ColumnConfig{Number: colCPUReq, Align: text.AlignRight}, // CPU Req
		pt.ColumnConfig{Number: colCPULim, Align: text.AlignRight}, // CPU Lim
		pt.ColumnConfig{Number: colCPUUse, Align: text.AlignRight}, // CPU Use
		pt.ColumnConfig{Number: colCPUPct, Align: text.AlignRight}, // CPU %
		pt.ColumnConfig{Number: colMemReq, Align: text.AlignRight}, // Mem Req
		pt.ColumnConfig{Number: colMemLim, Align: text.AlignRight}, // Mem Lim
		pt.ColumnConfig{Number: colMemUse, Align: text.AlignRight}, // Mem Use
		pt.ColumnConfig{Number: colMemPct, Align: text.AlignRight}, // Mem %
	)
	if colGPU != 0 {
		baseColumns = append(baseColumns, pt.ColumnConfig{Number: colGPU, Align: text.AlignRight}) // GPU
	}

	if showCloud {
		baseColumns = append(baseColumns,
			pt.ColumnConfig{Number: colProvider, Align: text.AlignLeft}, // Provider
			pt.ColumnConfig{Number: colRegion, Align: text.AlignLeft},   // Region
			pt.ColumnConfig{Number: colInstance, Align: text.AlignLeft}, // Instance Type
			pt.ColumnConfig{Number: colCapacity, Align: text.AlignLeft}, // Capacity Type
		)
	}
	t.SetColumnConfigs(baseColumns)

	headerRow := pt.Row{"NODE", "STATUS"}
	if showVersion {
		headerRow = append(headerRow, "VERSION")
	}
	if showAge {
		headerRow = append(headerRow, "AGE")
	}
	if showGroup {
		headerRow = append(headerRow, "GROUP")
	}
	headerRow = append(headerRow,
		"CPU REQ", "CPU LIM", "CPU USE", "CPU %",
		"MEM REQ", "MEM LIM", "MEM USE", "MEM %",
	)
	if showGPU {
		headerRow = append(headerRow, "GPU REQ/ALLOC")
	}
	if showCloud {
		headerRow = append(headerRow, "PROVIDER", "REGION", "INSTANCE TYPE", "CAPACITY")
	}
	t.AppendHeader(headerRow)

	// Add node rows.
	for _, name := range nodeNames {
		v := (*nm)[name]
		row := buildTableRow(name, v, showVersion, showAge, showGroup, showGPU, showCloud)
		t.AppendRow(row)
	}

	// Add totals footer.
	footerRow := buildTableFooter(c, len(*nm), showVersion, showAge, showGroup, showGPU, showCloud)

	t.AppendSeparator()
	t.AppendFooter(footerRow)

	t.Render()

	// Print capacity summary.
	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("  CLUSTER CAPACITY")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  Allocatable CPU:    %s\n", formatQuantity(c.TotalAllocatableCPU))
	fmt.Printf("  Allocatable Memory: %s\n", formatQuantity(c.TotalAllocatableMemory))
	fmt.Printf("  Total Capacity CPU: %s\n", formatQuantity(c.TotalCapacityCPU))
	fmt.Printf("  Total Capacity Mem: %s\n", formatQuantity(c.TotalCapacityMemory))
	if c.TotalAllocatableGPU != nil && !c.TotalAllocatableGPU.IsZero() {
		fmt.Printf("  Allocatable GPU:    %d\n", c.TotalAllocatableGPU.Value())
		fmt.Printf("  Allocated GPU:      %d\n", c.TotalAllocatedGPURequests.Value())
	}
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()
}

func chart(_ *core.NodeMap) error {
	// Chart output is not yet implemented; return a clear error instead of exiting.
	return fmt.Errorf("chart output is not yet implemented")
}

func dash(nm *core.NodeMap) error {
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
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
			return nil
		}
	}
}

func pie(nm *core.NodeMap) error {
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
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
			return nil
		}
	}
}
