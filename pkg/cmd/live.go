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
	"fmt"
	"sort"
	"sync"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	metricsV1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// ViewMode represents the current display mode
type ViewMode int

const (
	ViewNamespaces ViewMode = iota
	ViewPods
	ViewNodes
	ViewDeployments
)

const (
	checkboxChecked   = "☑"
	checkboxUnchecked = "☐"
	defaultNamespace  = "default"

	// Status icons
	statusReady    = "✓"
	statusNotReady = "⊘"
	statusRunning  = "●"
	statusPending  = "○"
	statusFailed   = "✗"

	// Status text labels
	nodeStatusReady    = "Ready"
	nodeStatusNotReady = "NotReady"

	// Sort mode string constant
	sortByStatus = "status"

	// Progress bar thresholds
	thresholdLow      = 50.0
	thresholdMedium   = 75.0
	thresholdHigh     = 90.0
	thresholdCritical = 100.0

	// Default limits for large cluster support
	defaultNodeLimit      = 20
	defaultPodLimit       = 100
	defaultMaxConcurrent  = 50
	largeClusterThreshold = 100
)

// SortMode represents the current sort mode
type SortMode int

const (
	SortByName SortMode = iota
	SortByStatus
	SortByCPU
	SortByMemory
)

// ResourceMetrics holds the resource values and capacity for progress bars.
type ResourceMetrics struct {
	CPURequest  float64
	CPULimit    float64
	CPUUsage    float64
	CPUCapacity float64
	MemRequest  float64
	MemLimit    float64
	MemUsage    float64
	MemCapacity float64
}

// LiveState holds the state for the live TUI
type LiveState struct {
	mode              ViewMode
	selectedNamespace string
	refreshInterval   time.Duration
	lastUpdate        time.Time
	table             *widgets.Table
	statusBar         *widgets.Paragraph
	helpBar           *widgets.Paragraph
	menuBar           *widgets.Paragraph
	showBars          bool
	showPercentages   bool
	compactMode       bool
	// Scaling options
	nodeLimit     int
	podLimit      int
	maxConcurrent int
	sortMode      SortMode
	totalNodes    int // Track total for "showing X of Y" display
	totalPods     int
}

// NewLiveCmd creates the live subcommand
func NewLiveCmd(gc *GlanceConfig) *cobra.Command {
	var refreshInterval int
	var nodeLimit int
	var podLimit int
	var maxConcurrent int
	var sortBy string

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Display live resource monitoring with interactive TUI",
		Long: `Display a live, continuously updating terminal UI showing Kubernetes resource allocation and usage.

View modes:
  - Namespaces (default): Shows resource requests, limits, and usage per namespace
  - Pods: Shows resource requests, limits, and usage per pod (namespace-scoped)
  - Nodes: Shows node capacity, allocation, and usage
  - Deployments: Shows deployment resource requests and replica status

Scaling options:
  - Use --node-limit to limit displayed nodes (default: 20)
  - Use --pod-limit to limit displayed pods (default: 100)
  - Use --sort-by to sort by status, cpu, memory, or name (default: status)

Controls will be displayed at the bottom of the screen.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := kubernetes.NewForConfig(gc.restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			// Parse sort mode
			sortMode := SortByStatus
			switch sortBy {
			case "name":
				sortMode = SortByName
			case "cpu":
				sortMode = SortByCPU
			case "memory", "mem":
				sortMode = SortByMemory
			case sortByStatus:
				sortMode = SortByStatus
			}

			return runLive(k8sClient, gc, time.Duration(refreshInterval)*time.Second,
				nodeLimit, podLimit, maxConcurrent, sortMode)
		},
	}

	cmd.Flags().IntVarP(&refreshInterval, "refresh", "r", 2, "Refresh interval in seconds")
	cmd.Flags().IntVarP(&nodeLimit, "node-limit", "n", defaultNodeLimit,
		"Maximum nodes to display (0 for unlimited)")
	cmd.Flags().IntVar(&podLimit, "pod-limit", defaultPodLimit,
		"Maximum pods to display per view (0 for unlimited)")
	cmd.Flags().IntVar(&maxConcurrent, "max-concurrent", defaultMaxConcurrent,
		"Maximum concurrent API requests")
	cmd.Flags().StringVar(&sortBy, "sort-by", sortByStatus,
		"Sort by: status, name, cpu, memory")

	return cmd
}

func runLive(
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	refreshInterval time.Duration,
	nodeLimit, podLimit, maxConcurrent int,
	sortMode SortMode,
) error {
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
	}
	defer ui.Close()

	state := &LiveState{
		mode:              ViewNamespaces,
		selectedNamespace: "",
		refreshInterval:   refreshInterval,
		lastUpdate:        time.Now(),
		showBars:          true,
		showPercentages:   true,
		compactMode:       false,
		nodeLimit:         nodeLimit,
		podLimit:          podLimit,
		maxConcurrent:     maxConcurrent,
		sortMode:          sortMode,
	}

	// Check cluster size and warn for large clusters
	ctx := context.Background()
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		ResourceVersion: "0", // Use watch cache for faster response
	})
	if err == nil {
		state.totalNodes = len(nodes.Items)
		if state.totalNodes > largeClusterThreshold {
			log.Warnf("Large cluster detected (%d nodes). Using --node-limit=%d for performance. "+
				"Consider using --watch mode for real-time updates with lower API load.",
				state.totalNodes, nodeLimit)
		}
	}

	// Initialize UI components
	state.table = widgets.NewTable()
	state.statusBar = widgets.NewParagraph()
	state.helpBar = widgets.NewParagraph()
	state.menuBar = widgets.NewParagraph()

	state.helpBar.Text = getHelpText(state.mode)
	state.helpBar.Border = false
	state.menuBar.Border = false

	// Initial render
	if err := updateDisplay(k8sClient, gc, state); err != nil {
		return err
	}

	// Set up event handling
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return nil
			case "n":
				state.mode = ViewNamespaces
				state.selectedNamespace = ""
				state.helpBar.Text = getHelpText(state.mode)
			case "p":
				state.mode = ViewPods
				if state.selectedNamespace == "" {
					state.selectedNamespace = defaultNamespace
				}
				state.helpBar.Text = getHelpText(state.mode)
			case "o":
				state.mode = ViewNodes
				state.helpBar.Text = getHelpText(state.mode)
			case "d":
				state.mode = ViewDeployments
				if state.selectedNamespace == "" {
					state.selectedNamespace = defaultNamespace
				}
				state.helpBar.Text = getHelpText(state.mode)
			case "<Left>":
				if state.mode == ViewPods || state.mode == ViewDeployments {
					state.selectedNamespace = getPreviousNamespace(k8sClient, state.selectedNamespace)
				}
			case "<Right>":
				if state.mode == ViewPods || state.mode == ViewDeployments {
					state.selectedNamespace = getNextNamespace(k8sClient, state.selectedNamespace)
				}
			case "b":
				state.showBars = !state.showBars
			case "%":
				state.showPercentages = !state.showPercentages
			case "c":
				state.compactMode = !state.compactMode
			case "s":
				// Cycle through sort modes
				state.sortMode = (state.sortMode + 1) % 4
			case "<Resize>":
				// Handle terminal resize
			}

			if err := updateDisplay(k8sClient, gc, state); err != nil {
				log.Errorf("Failed to update display: %v", err)
			}

		case <-ticker.C:
			state.lastUpdate = time.Now()
			if err := updateDisplay(k8sClient, gc, state); err != nil {
				log.Errorf("Failed to update display: %v", err)
			}
		}
	}
}

func updateDisplay(k8sClient *kubernetes.Clientset, gc *GlanceConfig, state *LiveState) error {
	termWidth, termHeight := ui.TerminalDimensions()

	var data [][]string
	var header []string
	var metrics []ResourceMetrics
	var err error

	ctx := context.Background()

	switch state.mode {
	case ViewNamespaces:
		header, data, metrics, err = fetchNamespaceData(ctx, k8sClient, gc, state)
	case ViewPods:
		header, data, metrics, err = fetchPodData(ctx, k8sClient, gc, state.selectedNamespace, state)
	case ViewNodes:
		header, data, metrics, err = fetchNodeData(ctx, k8sClient, gc, state)
	case ViewDeployments:
		header, data, metrics, err = fetchDeploymentData(ctx, k8sClient, state.selectedNamespace)
	}

	if err != nil {
		return err
	}

	// Add progress bars to data if enabled
	if state.showBars && len(metrics) > 0 {
		data = addProgressBars(data, metrics, state.showPercentages)
	}

	// Calculate summary stats
	summaryStats := calculateSummaryStats(metrics)

	// Calculate table height based on compact mode (leave room for summary)
	summaryHeight := 3
	tableHeight := termHeight - 6 - summaryHeight
	if state.compactMode {
		tableHeight = termHeight - 5
		summaryHeight = 0
	}

	// Update table
	state.table.Rows = append([][]string{header}, data...)
	state.table.TextStyle = ui.NewStyle(ui.ColorWhite)
	state.table.RowSeparator = false
	state.table.BorderStyle = ui.NewStyle(ui.ColorCyan)
	state.table.SetRect(0, summaryHeight, termWidth, tableHeight+summaryHeight)
	state.table.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

	// Apply row coloring based on utilization
	applyRowColors(state.table, metrics, state.showBars)

	// Render summary bar at the top (if not compact)
	if !state.compactMode {
		renderSummaryBar(summaryStats, termWidth)
	}

	// Update status bar
	modeStr := getModeString(state.mode)
	if state.selectedNamespace != "" && (state.mode == ViewPods || state.mode == ViewDeployments) {
		modeStr += fmt.Sprintf(" [%s]", state.selectedNamespace)
	}
	state.statusBar.Text = fmt.Sprintf(" %s | Updated: %s | Refresh: %v | Items: %d",
		modeStr,
		state.lastUpdate.Format("15:04:05"),
		state.refreshInterval,
		len(data)/getRowMultiplier(state.showBars))
	state.statusBar.Border = false
	state.statusBar.SetRect(0, tableHeight+summaryHeight, termWidth, tableHeight+summaryHeight+2)

	// Update menu bar with toggles
	state.menuBar.Text = getMenuBar(state)
	state.menuBar.SetRect(0, tableHeight+summaryHeight+2, termWidth, tableHeight+summaryHeight+3)

	// Update help bar
	if !state.compactMode {
		state.helpBar.SetRect(0, tableHeight+summaryHeight+3, termWidth, termHeight)
	}

	if state.compactMode {
		ui.Render(state.table, state.statusBar, state.menuBar)
	} else {
		ui.Render(state.table, state.statusBar, state.menuBar, state.helpBar)
	}
	return nil
}

// SummaryStats holds aggregate statistics for the summary bar.
type SummaryStats struct {
	TotalItems    int
	AvgCPUUsage   float64
	AvgMemUsage   float64
	MaxCPUUsage   float64
	MaxMemUsage   float64
	CriticalCount int
	WarningCount  int
	HealthyCount  int
}

// calculateSummaryStats computes aggregate statistics from metrics
func calculateSummaryStats(metrics []ResourceMetrics) SummaryStats {
	stats := SummaryStats{TotalItems: len(metrics)}
	if len(metrics) == 0 {
		return stats
	}

	var totalCPU, totalMem float64
	for _, m := range metrics {
		cpuPct := safePercentage(m.CPUUsage, m.CPUCapacity)
		memPct := safePercentage(m.MemUsage, m.MemCapacity)

		totalCPU += cpuPct
		totalMem += memPct

		if cpuPct > stats.MaxCPUUsage {
			stats.MaxCPUUsage = cpuPct
		}
		if memPct > stats.MaxMemUsage {
			stats.MaxMemUsage = memPct
		}

		maxPct := cpuPct
		if memPct > maxPct {
			maxPct = memPct
		}

		switch {
		case maxPct >= thresholdHigh:
			stats.CriticalCount++
		case maxPct >= thresholdMedium:
			stats.WarningCount++
		default:
			stats.HealthyCount++
		}
	}

	stats.AvgCPUUsage = totalCPU / float64(len(metrics))
	stats.AvgMemUsage = totalMem / float64(len(metrics))

	return stats
}

// renderSummaryBar renders a summary bar at the top of the screen
func renderSummaryBar(stats SummaryStats, width int) {
	summary := widgets.NewParagraph()
	summary.Border = true
	summary.BorderStyle = ui.NewStyle(ui.ColorCyan)
	summary.Title = " 🔍 Cluster Summary "
	summary.TitleStyle = ui.NewStyle(ui.ColorCyan, ui.ColorBlack, ui.ModifierBold)

	// Build summary text with status indicators
	healthIcon := fmt.Sprintf("[%s %d](fg:green)", statusReady, stats.HealthyCount)
	warnIcon := fmt.Sprintf("[%s %d](fg:yellow)", statusPending, stats.WarningCount)
	critIcon := fmt.Sprintf("[%s %d](fg:red)", statusNotReady, stats.CriticalCount)

	cpuBar := makeColoredBar(stats.AvgCPUUsage, 15)
	memBar := makeColoredBar(stats.AvgMemUsage, 15)

	summary.Text = fmt.Sprintf(
		" Status: %s %s %s │ CPU: %s %.0f%% │ Mem: %s %.0f%%",
		healthIcon, warnIcon, critIcon,
		cpuBar, stats.AvgCPUUsage,
		memBar, stats.AvgMemUsage,
	)

	summary.SetRect(0, 0, width, 3)
	ui.Render(summary)
}

// makeColoredBar creates a colored progress bar string for termui
func makeColoredBar(percentage float64, width int) string {
	if percentage > 100 {
		percentage = 100
	}
	if percentage < 0 {
		percentage = 0
	}

	filled := int((percentage / 100) * float64(width))
	empty := width - filled

	var color string
	switch {
	case percentage >= thresholdHigh:
		color = "red"
	case percentage >= thresholdMedium:
		color = "yellow"
	case percentage >= thresholdLow:
		color = "yellow"
	default:
		color = "green"
	}

	filledStr := ""
	for i := 0; i < filled; i++ {
		filledStr += "█"
	}
	emptyStr := ""
	for i := 0; i < empty; i++ {
		emptyStr += "░"
	}

	return fmt.Sprintf("[%s](fg:%s)[%s](fg:black)", filledStr, color, emptyStr)
}

// applyRowColors applies color coding to table rows based on utilization
func applyRowColors(table *widgets.Table, metrics []ResourceMetrics, showBars bool) {
	multiplier := getRowMultiplier(showBars)

	for i, m := range metrics {
		rowIdx := (i * multiplier) + 1 // +1 for header

		cpuPct := safePercentage(m.CPUUsage, m.CPUCapacity)
		memPct := safePercentage(m.MemUsage, m.MemCapacity)
		maxPct := cpuPct
		if memPct > maxPct {
			maxPct = memPct
		}

		var style ui.Style
		switch {
		case maxPct >= thresholdHigh:
			style = ui.NewStyle(ui.ColorRed)
		case maxPct >= thresholdMedium:
			style = ui.NewStyle(ui.ColorYellow)
		default:
			style = ui.NewStyle(ui.ColorWhite)
		}

		table.RowStyles[rowIdx] = style
	}
}

// getRowMultiplier returns 2 if progress bars are shown (data + bar rows), 1 otherwise
func getRowMultiplier(showBars bool) int {
	if showBars {
		return 2
	}
	return 1
}

// safePercentage calculates percentage safely handling zero capacity
func safePercentage(value, capacity float64) float64 {
	if capacity == 0 {
		return 0
	}
	return (value / capacity) * 100
}

// nsRowData holds data for a single namespace row for parallel processing.
type nsRowData struct {
	row      []string
	metrics  ResourceMetrics
	cpuUsage float64
}

func fetchNamespaceData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	state *LiveState,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"NAMESPACE", "CPU REQ", "CPU LIMIT", "CPU USAGE", "MEM REQ", "MEM LIMIT", "MEM USAGE", "PODS"}

	// Use watch cache for faster response
	namespaces, err := k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		ResourceVersion: "0",
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return header, [][]string{}, []ResourceMetrics{}, nil
	}

	// Fetch ALL pods and metrics in parallel (instead of per-namespace queries)
	g, gCtx := errgroup.WithContext(ctx)

	var allPods *v1.PodList
	var allPodMetrics *metricsV1beta1api.PodMetricsList

	g.Go(func() error {
		var err error
		allPods, err = k8sClient.CoreV1().Pods("").List(gCtx, metav1.ListOptions{
			ResourceVersion: "0",
		})
		return err
	})

	g.Go(func() error {
		var err error
		allPodMetrics, err = metricsClient.MetricsV1beta1().PodMetricses("").List(gCtx, metav1.ListOptions{
			ResourceVersion: "0",
		})
		if err != nil {
			log.Debugf("Failed to fetch pod metrics: %v", err)
		}
		return nil // Don't fail on metrics error
	})

	if err := g.Wait(); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch pods: %w", err)
	}

	// Group pods by namespace
	podsByNS := make(map[string][]v1.Pod)
	if allPods != nil {
		for _, pod := range allPods.Items {
			podsByNS[pod.Namespace] = append(podsByNS[pod.Namespace], pod)
		}
	}

	// Group metrics by namespace+pod
	metricsByPod := make(map[string]*metricsV1beta1api.PodMetrics)
	if allPodMetrics != nil {
		for i := range allPodMetrics.Items {
			pm := &allPodMetrics.Items[i]
			key := pm.Namespace + "/" + pm.Name
			metricsByPod[key] = pm
		}
	}

	// Process namespaces in parallel
	nsData := make([]nsRowData, len(namespaces.Items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, state.maxConcurrent)

	for i, ns := range namespaces.Items {
		wg.Add(1)
		go func(idx int, ns v1.Namespace) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
			cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
			memReq := resource.NewQuantity(0, resource.BinarySI)
			memLimit := resource.NewQuantity(0, resource.BinarySI)
			cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
			memUsage := resource.NewQuantity(0, resource.BinarySI)

			pods := podsByNS[ns.Name]
			podCount := len(pods)

			for _, pod := range pods {
				for _, container := range pod.Spec.Containers {
					if req := container.Resources.Requests.Cpu(); req != nil {
						cpuReq.Add(*req)
					}
					if lim := container.Resources.Limits.Cpu(); lim != nil {
						cpuLimit.Add(*lim)
					}
					if req := container.Resources.Requests.Memory(); req != nil {
						memReq.Add(*req)
					}
					if lim := container.Resources.Limits.Memory(); lim != nil {
						memLimit.Add(*lim)
					}
				}

				// Get metrics for this pod
				key := pod.Namespace + "/" + pod.Name
				if pm, ok := metricsByPod[key]; ok {
					for _, container := range pm.Containers {
						cpuUsage.Add(container.Usage[v1.ResourceCPU])
						memUsage.Add(container.Usage[v1.ResourceMemory])
					}
				}
			}

			cpuUsagePct := float64(0)
			if cpuLimit.MilliValue() > 0 {
				cpuUsagePct = float64(cpuUsage.MilliValue()) / float64(cpuLimit.MilliValue()) * 100
			}

			row := []string{
				ns.Name,
				formatMilliCPU(cpuReq),
				formatMilliCPU(cpuLimit),
				formatMilliCPU(cpuUsage),
				formatBytes(memReq),
				formatBytes(memLimit),
				formatBytes(memUsage),
				fmt.Sprintf("%d", podCount),
			}

			metrics := ResourceMetrics{
				CPURequest:  float64(cpuReq.MilliValue()) / 1000.0,
				CPULimit:    float64(cpuLimit.MilliValue()) / 1000.0,
				CPUUsage:    float64(cpuUsage.MilliValue()) / 1000.0,
				CPUCapacity: float64(cpuLimit.MilliValue()) / 1000.0,
				MemRequest:  float64(memReq.Value()),
				MemLimit:    float64(memLimit.Value()),
				MemUsage:    float64(memUsage.Value()),
				MemCapacity: float64(memLimit.Value()),
			}

			nsData[idx] = nsRowData{
				row:      row,
				metrics:  metrics,
				cpuUsage: cpuUsagePct,
			}
		}(i, ns)
	}
	wg.Wait()

	// Sort based on sort mode
	switch state.sortMode {
	case SortByName:
		sort.Slice(nsData, func(i, j int) bool {
			return nsData[i].row[0] < nsData[j].row[0]
		})
	case SortByCPU:
		sort.Slice(nsData, func(i, j int) bool {
			return nsData[i].cpuUsage > nsData[j].cpuUsage
		})
	default:
		// Default: sort by name
		sort.Slice(nsData, func(i, j int) bool {
			return nsData[i].row[0] < nsData[j].row[0]
		})
	}

	// Build final rows and metrics
	rows := make([][]string, 0, len(nsData))
	metrics := make([]ResourceMetrics, 0, len(nsData))
	for _, nd := range nsData {
		rows = append(rows, nd.row)
		metrics = append(metrics, nd.metrics)
	}

	return header, rows, metrics, nil
}

// podRowData holds data for a single pod row for sorting and limiting.
type podRowData struct {
	row       []string
	metrics   ResourceMetrics
	isRunning bool
	cpuUsage  float64
	memUsage  float64
}

func fetchPodData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	namespace string,
	state *LiveState,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"POD", "CPU REQ", "CPU LIMIT", "CPU USAGE", "MEM REQ", "MEM LIMIT", "MEM USAGE", "STATUS"}

	// Fetch pods and metrics in parallel using watch cache
	g, gCtx := errgroup.WithContext(ctx)

	var pods *v1.PodList
	var podMetricsList *metricsV1beta1api.PodMetricsList

	g.Go(func() error {
		var err error
		pods, err = k8sClient.CoreV1().Pods(namespace).List(gCtx, metav1.ListOptions{
			ResourceVersion: "0",
		})
		return err
	})

	metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
	if err == nil {
		g.Go(func() error {
			var err error
			podMetricsList, err = metricsClient.MetricsV1beta1().PodMetricses(namespace).List(gCtx, metav1.ListOptions{
				ResourceVersion: "0",
			})
			if err != nil {
				log.Debugf("Failed to fetch pod metrics: %v", err)
			}
			return nil // Don't fail on metrics error
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list pods: %w", err)
	}

	state.totalPods = len(pods.Items)

	metricsMap := make(map[string]*metricsV1beta1api.PodMetrics)
	if podMetricsList != nil {
		for i := range podMetricsList.Items {
			metricsMap[podMetricsList.Items[i].Name] = &podMetricsList.Items[i]
		}
	}

	podData := make([]podRowData, 0, len(pods.Items))
	for _, pod := range pods.Items {
		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLimit := resource.NewQuantity(0, resource.BinarySI)
		cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
		memUsage := resource.NewQuantity(0, resource.BinarySI)

		for _, container := range pod.Spec.Containers {
			if req := container.Resources.Requests.Cpu(); req != nil {
				cpuReq.Add(*req)
			}
			if lim := container.Resources.Limits.Cpu(); lim != nil {
				cpuLimit.Add(*lim)
			}
			if req := container.Resources.Requests.Memory(); req != nil {
				memReq.Add(*req)
			}
			if lim := container.Resources.Limits.Memory(); lim != nil {
				memLimit.Add(*lim)
			}
		}

		if pm, ok := metricsMap[pod.Name]; ok {
			for _, container := range pm.Containers {
				cpuUsage.Add(container.Usage[v1.ResourceCPU])
				memUsage.Add(container.Usage[v1.ResourceMemory])
			}
		}

		// Format status with icon
		podStatus := string(pod.Status.Phase)
		statusIcon := getStatusIcon(podStatus)
		isRunning := pod.Status.Phase == v1.PodRunning

		cpuUsagePct := float64(0)
		if cpuLimit.MilliValue() > 0 {
			cpuUsagePct = float64(cpuUsage.MilliValue()) / float64(cpuLimit.MilliValue()) * 100
		}
		memUsagePct := float64(0)
		if memLimit.Value() > 0 {
			memUsagePct = float64(memUsage.Value()) / float64(memLimit.Value()) * 100
		}

		row := []string{
			pod.Name,
			formatMilliCPU(cpuReq),
			formatMilliCPU(cpuLimit),
			formatMilliCPU(cpuUsage),
			formatBytes(memReq),
			formatBytes(memLimit),
			formatBytes(memUsage),
			statusIcon + podStatus,
		}

		metrics := ResourceMetrics{
			CPURequest:  float64(cpuReq.MilliValue()) / 1000.0,
			CPULimit:    float64(cpuLimit.MilliValue()) / 1000.0,
			CPUUsage:    float64(cpuUsage.MilliValue()) / 1000.0,
			CPUCapacity: float64(cpuLimit.MilliValue()) / 1000.0,
			MemRequest:  float64(memReq.Value()),
			MemLimit:    float64(memLimit.Value()),
			MemUsage:    float64(memUsage.Value()),
			MemCapacity: float64(memLimit.Value()),
		}

		podData = append(podData, podRowData{
			row:       row,
			metrics:   metrics,
			isRunning: isRunning,
			cpuUsage:  cpuUsagePct,
			memUsage:  memUsagePct,
		})
	}

	// Sort based on sort mode
	switch state.sortMode {
	case SortByStatus:
		// Non-running pods first (Pending, Failed, etc.)
		sort.Slice(podData, func(i, j int) bool {
			if podData[i].isRunning != podData[j].isRunning {
				return !podData[i].isRunning
			}
			return podData[i].cpuUsage > podData[j].cpuUsage
		})
	case SortByName:
		sort.Slice(podData, func(i, j int) bool {
			return podData[i].row[0] < podData[j].row[0]
		})
	case SortByCPU:
		sort.Slice(podData, func(i, j int) bool {
			return podData[i].cpuUsage > podData[j].cpuUsage
		})
	case SortByMemory:
		sort.Slice(podData, func(i, j int) bool {
			return podData[i].memUsage > podData[j].memUsage
		})
	}

	// Apply pod limit
	limit := len(podData)
	if state.podLimit > 0 && state.podLimit < limit {
		limit = state.podLimit
	}

	// Build final rows and metrics
	rows := make([][]string, 0, limit)
	metrics := make([]ResourceMetrics, 0, limit)
	for i := 0; i < limit; i++ {
		rows = append(rows, podData[i].row)
		metrics = append(metrics, podData[i].metrics)
	}

	return header, rows, metrics, nil
}

// nodeRowData holds data for a single node row for parallel processing.
type nodeRowData struct {
	row      []string
	metrics  ResourceMetrics
	isReady  bool
	cpuUsage float64
	memUsage float64
}

func fetchNodeData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	state *LiveState,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"NODE", "STATUS", "CPU CAP", "CPU ALLOC", "CPU USAGE", "MEM CAP", "MEM ALLOC", "MEM USAGE", "PODS"}

	// Use watch cache for faster response (resourceVersion="0")
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		ResourceVersion: "0",
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	state.totalNodes = len(nodes.Items)

	metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return header, [][]string{}, []ResourceMetrics{}, nil
	}

	// Fetch all data in parallel using errgroup
	g, gCtx := errgroup.WithContext(ctx)

	var nodeMetrics *metricsV1beta1api.NodeMetricsList
	var allPods *v1.PodList

	// Fetch node metrics in parallel
	g.Go(func() error {
		var err error
		nodeMetrics, err = metricsClient.MetricsV1beta1().NodeMetricses().List(gCtx, metav1.ListOptions{
			ResourceVersion: "0",
		})
		return err
	})

	// Fetch ALL pods once (instead of per-node queries) using watch cache
	g.Go(func() error {
		var err error
		allPods, err = k8sClient.CoreV1().Pods("").List(gCtx, metav1.ListOptions{
			ResourceVersion: "0",
			FieldSelector:   "status.phase!=Succeeded,status.phase!=Failed",
		})
		return err
	})

	// Wait for parallel fetches to complete
	if err := g.Wait(); err != nil {
		log.Debugf("Error fetching metrics or pods: %v", err)
	}

	// Build metrics map
	metricsMap := make(map[string]*metricsV1beta1api.NodeMetrics)
	if nodeMetrics != nil {
		for i := range nodeMetrics.Items {
			metricsMap[nodeMetrics.Items[i].Name] = &nodeMetrics.Items[i]
		}
	}

	// Group pods by node name (O(n) instead of O(n*m) API calls)
	podsByNode := make(map[string][]v1.Pod)
	if allPods != nil {
		for _, pod := range allPods.Items {
			nodeName := pod.Spec.NodeName
			if nodeName != "" {
				podsByNode[nodeName] = append(podsByNode[nodeName], pod)
			}
		}
	}

	// Process nodes in parallel with semaphore for concurrency limit
	nodeData := make([]nodeRowData, len(nodes.Items))
	var mu sync.Mutex
	sem := make(chan struct{}, state.maxConcurrent)

	var wg sync.WaitGroup
	for i, node := range nodes.Items {
		wg.Add(1)
		go func(idx int, node v1.Node) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Get node status
			isReady := false
			nodeStatus := "Unknown"
			for _, condition := range node.Status.Conditions {
				if condition.Type == v1.NodeReady {
					if condition.Status == v1.ConditionTrue {
						nodeStatus = statusReady + " " + nodeStatusReady
						isReady = true
					} else {
						nodeStatus = statusNotReady + " " + nodeStatusNotReady
					}
					break
				}
			}

			cpuCap := node.Status.Capacity.Cpu()
			memCap := node.Status.Capacity.Memory()
			cpuAlloc := resource.NewMilliQuantity(0, resource.DecimalSI)
			memAlloc := resource.NewQuantity(0, resource.BinarySI)
			cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
			memUsage := resource.NewQuantity(0, resource.BinarySI)

			// Use pre-fetched pods grouped by node
			pods := podsByNode[node.Name]
			podCount := len(pods)
			for _, pod := range pods {
				for _, container := range pod.Spec.Containers {
					if req := container.Resources.Requests.Cpu(); req != nil {
						cpuAlloc.Add(*req)
					}
					if req := container.Resources.Requests.Memory(); req != nil {
						memAlloc.Add(*req)
					}
				}
			}

			if nm, ok := metricsMap[node.Name]; ok {
				cpuUsage.Add(nm.Usage[v1.ResourceCPU])
				memUsage.Add(nm.Usage[v1.ResourceMemory])
			}

			cpuUsagePct := float64(0)
			if cpuCap.MilliValue() > 0 {
				cpuUsagePct = float64(cpuUsage.MilliValue()) / float64(cpuCap.MilliValue()) * 100
			}
			memUsagePct := float64(0)
			if memCap.Value() > 0 {
				memUsagePct = float64(memUsage.Value()) / float64(memCap.Value()) * 100
			}

			row := []string{
				node.Name,
				nodeStatus,
				formatMilliCPU(cpuCap),
				formatMilliCPU(cpuAlloc),
				formatMilliCPU(cpuUsage),
				formatBytes(memCap),
				formatBytes(memAlloc),
				formatBytes(memUsage),
				fmt.Sprintf("%d", podCount),
			}

			metrics := ResourceMetrics{
				CPURequest:  float64(cpuAlloc.MilliValue()) / 1000.0,
				CPULimit:    float64(cpuCap.MilliValue()) / 1000.0,
				CPUUsage:    float64(cpuUsage.MilliValue()) / 1000.0,
				CPUCapacity: float64(cpuCap.MilliValue()) / 1000.0,
				MemRequest:  float64(memAlloc.Value()),
				MemLimit:    float64(memCap.Value()),
				MemUsage:    float64(memUsage.Value()),
				MemCapacity: float64(memCap.Value()),
			}

			mu.Lock()
			nodeData[idx] = nodeRowData{
				row:      row,
				metrics:  metrics,
				isReady:  isReady,
				cpuUsage: cpuUsagePct,
				memUsage: memUsagePct,
			}
			mu.Unlock()
		}(i, node)
	}
	wg.Wait()

	// Sort based on sort mode
	sortNodeData(nodeData, state.sortMode)

	// Apply node limit
	limit := len(nodeData)
	if state.nodeLimit > 0 && state.nodeLimit < limit {
		limit = state.nodeLimit
	}

	// Build final rows and metrics
	rows := make([][]string, 0, limit)
	metrics := make([]ResourceMetrics, 0, limit)
	for i := 0; i < limit; i++ {
		rows = append(rows, nodeData[i].row)
		metrics = append(metrics, nodeData[i].metrics)
	}

	return header, rows, metrics, nil
}

// sortNodeData sorts node data based on sort mode
func sortNodeData(data []nodeRowData, mode SortMode) {
	switch mode {
	case SortByStatus:
		// NotReady nodes first, then by CPU usage descending
		sort.Slice(data, func(i, j int) bool {
			if data[i].isReady != data[j].isReady {
				return !data[i].isReady // NotReady first
			}
			return data[i].cpuUsage > data[j].cpuUsage
		})
	case SortByName:
		sort.Slice(data, func(i, j int) bool {
			return data[i].row[0] < data[j].row[0]
		})
	case SortByCPU:
		sort.Slice(data, func(i, j int) bool {
			return data[i].cpuUsage > data[j].cpuUsage
		})
	case SortByMemory:
		sort.Slice(data, func(i, j int) bool {
			return data[i].memUsage > data[j].memUsage
		})
	}
}

func fetchDeploymentData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	namespace string,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{
		"DEPLOYMENT", "STATUS", "CPU REQ", "CPU LIMIT", "MEM REQ", "MEM LIMIT",
		"REPLICAS", "READY", "AVAILABLE",
	}

	deployments, err := k8sClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		ResourceVersion: "0",
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	var rows [][]string
	var metrics []ResourceMetrics
	for _, deploy := range deployments.Items {
		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLimit := resource.NewQuantity(0, resource.BinarySI)

		for _, container := range deploy.Spec.Template.Spec.Containers {
			if req := container.Resources.Requests.Cpu(); req != nil {
				cpuReq.Add(*req)
			}
			if lim := container.Resources.Limits.Cpu(); lim != nil {
				cpuLimit.Add(*lim)
			}
			if req := container.Resources.Requests.Memory(); req != nil {
				memReq.Add(*req)
			}
			if lim := container.Resources.Limits.Memory(); lim != nil {
				memLimit.Add(*lim)
			}
		}

		// Multiply by replica count for per-deployment totals
		replicas := int32(1)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}

		cpuReq = multiplyQuantity(cpuReq, int(replicas))
		cpuLimit = multiplyQuantity(cpuLimit, int(replicas))
		memReq = multiplyQuantity(memReq, int(replicas))
		memLimit = multiplyQuantity(memLimit, int(replicas))

		// Determine deployment status
		deployStatus := statusReady + " " + nodeStatusReady
		if deploy.Status.ReadyReplicas < replicas {
			if deploy.Status.ReadyReplicas == 0 {
				deployStatus = statusFailed + " " + nodeStatusNotReady
			} else {
				deployStatus = statusPending + " Partial"
			}
		}

		rows = append(rows, []string{
			deploy.Name,
			deployStatus,
			formatMilliCPU(cpuReq),
			formatMilliCPU(cpuLimit),
			formatBytes(memReq),
			formatBytes(memLimit),
			fmt.Sprintf("%d", replicas),
			fmt.Sprintf("%d", deploy.Status.ReadyReplicas),
			fmt.Sprintf("%d", deploy.Status.AvailableReplicas),
		})

		metrics = append(metrics, ResourceMetrics{
			CPURequest:  float64(cpuReq.MilliValue()) / 1000.0,
			CPULimit:    float64(cpuLimit.MilliValue()) / 1000.0,
			CPUUsage:    0, // Deployments don't have direct usage metrics
			CPUCapacity: float64(cpuLimit.MilliValue()) / 1000.0,
			MemRequest:  float64(memReq.Value()),
			MemLimit:    float64(memLimit.Value()),
			MemUsage:    0,
			MemCapacity: float64(memLimit.Value()),
		})
	}

	return header, rows, metrics, nil
}

func getHelpText(mode ViewMode) string {
	base := "[n]NS [p]Pods [o]Nodes [d]Deploy | [b]Bars [%]Pct [s]Sort [c]Compact | [q]Quit"
	if mode == ViewPods || mode == ViewDeployments {
		return base + " | [←→]NS"
	}
	return base
}

func getSortModeString(mode SortMode) string {
	switch mode {
	case SortByStatus:
		return sortByStatus
	case SortByName:
		return "name"
	case SortByCPU:
		return "cpu"
	case SortByMemory:
		return "memory"
	default:
		return sortByStatus
	}
}

func getMenuBar(state *LiveState) string {
	barsIcon := checkboxUnchecked
	if state.showBars {
		barsIcon = checkboxChecked
	}
	percIcon := checkboxUnchecked
	if state.showPercentages {
		percIcon = checkboxChecked
	}
	compactIcon := checkboxUnchecked
	if state.compactMode {
		compactIcon = checkboxChecked
	}

	sortStr := getSortModeString(state.sortMode)

	// Show limit info if applicable
	limitInfo := ""
	if state.nodeLimit > 0 && state.totalNodes > state.nodeLimit {
		limitInfo = fmt.Sprintf(" | Showing %d/%d nodes", state.nodeLimit, state.totalNodes)
	}

	return fmt.Sprintf(" %s Bars | %s Pct | %s Compact | Sort: %s%s",
		barsIcon, percIcon, compactIcon, sortStr, limitInfo)
}

func getModeString(mode ViewMode) string {
	switch mode {
	case ViewNamespaces:
		return "NAMESPACES"
	case ViewPods:
		return "PODS"
	case ViewNodes:
		return "NODES"
	case ViewDeployments:
		return "DEPLOYMENTS"
	default:
		return "UNKNOWN"
	}
}

func getPreviousNamespace(k8sClient *kubernetes.Clientset, current string) string {
	ctx := context.Background()
	namespaces, err := k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil || len(namespaces.Items) == 0 {
		return current
	}

	names := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		names = append(names, ns.Name)
	}
	sort.Strings(names)

	for i, name := range names {
		if name == current {
			if i == 0 {
				return names[len(names)-1]
			}
			return names[i-1]
		}
	}

	return names[0]
}

func getNextNamespace(k8sClient *kubernetes.Clientset, current string) string {
	ctx := context.Background()
	namespaces, err := k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil || len(namespaces.Items) == 0 {
		return current
	}

	names := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		names = append(names, ns.Name)
	}
	sort.Strings(names)

	for i, name := range names {
		if name == current {
			if i == len(names)-1 {
				return names[0]
			}
			return names[i+1]
		}
	}

	return names[0]
}

func formatMilliCPU(q *resource.Quantity) string {
	if q == nil || q.IsZero() {
		return "0"
	}
	return fmt.Sprintf("%.2f", float64(q.MilliValue())/1000.0)
}

func formatBytes(q *resource.Quantity) string {
	if q == nil || q.IsZero() {
		return "0"
	}

	bytes := q.Value()
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2fGi", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2fMi", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2fKi", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func multiplyQuantity(q *resource.Quantity, multiplier int) *resource.Quantity {
	if q == nil {
		return resource.NewQuantity(0, resource.BinarySI)
	}
	result := q.DeepCopy()
	val := result.Value()
	result.Set(val * int64(multiplier))
	return &result
}

// addProgressBars adds visual progress bars under resource values
func addProgressBars(data [][]string, metrics []ResourceMetrics, showPercentages bool) [][]string {
	if len(data) == 0 || len(metrics) == 0 {
		return data
	}

	result := make([][]string, 0, len(data)*2)

	for i, row := range data {
		result = append(result, row)

		if i >= len(metrics) {
			continue
		}

		m := metrics[i]
		bars := make([]string, len(row))

		// First column is usually name, skip it
		bars[0] = ""

		// Generate bars for each metric column
		// Assuming columns are: NAME, CPU REQ, CPU LIMIT, CPU USAGE, MEM REQ, MEM LIMIT, MEM USAGE, ...
		if len(row) >= 4 {
			// CPU Request bar (column 1)
			bars[1] = makeProgressBar(m.CPURequest, m.CPUCapacity, 10, showPercentages)
			// CPU Limit bar (column 2)
			bars[2] = makeProgressBar(m.CPULimit, m.CPUCapacity, 10, showPercentages)
			// CPU Usage bar (column 3)
			bars[3] = makeProgressBar(m.CPUUsage, m.CPUCapacity, 10, showPercentages)
		}

		if len(row) >= 7 {
			// Memory Request bar (column 4)
			bars[4] = makeProgressBar(m.MemRequest, m.MemCapacity, 10, showPercentages)
			// Memory Limit bar (column 5)
			bars[5] = makeProgressBar(m.MemLimit, m.MemCapacity, 10, showPercentages)
			// Memory Usage bar (column 6)
			bars[6] = makeProgressBar(m.MemUsage, m.MemCapacity, 10, showPercentages)
		}

		// Fill remaining columns
		for j := 7; j < len(row); j++ {
			bars[j] = ""
		}

		result = append(result, bars)
	}

	return result
}

// makeProgressBar creates a visual progress bar with color indicator
func makeProgressBar(value, max float64, width int, showPercentage bool) string {
	if max == 0 {
		if showPercentage {
			return "  0%"
		}
		return ""
	}

	percentage := (value / max) * 100
	if percentage > 100 {
		percentage = 100
	}

	filled := int((percentage / 100) * float64(width))
	if filled > width {
		filled = width
	}

	// Color indicator based on percentage
	colorIndicator := getColorIndicator(percentage)

	bar := ""
	// Use block characters for a smooth gradient effect
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	if showPercentage {
		return fmt.Sprintf("%s%s %3.0f%%", colorIndicator, bar, percentage)
	}
	return colorIndicator + bar
}

// getColorIndicator returns a color indicator character based on percentage
func getColorIndicator(percentage float64) string {
	switch {
	case percentage >= thresholdHigh:
		return "🔴"
	case percentage >= thresholdMedium:
		return "🟡"
	case percentage >= thresholdLow:
		return "🟢"
	default:
		return "🟢"
	}
}

// getStatusIcon returns an appropriate status icon for a given status string
func getStatusIcon(status string) string {
	switch status {
	case "Running":
		return statusRunning + " "
	case "Pending":
		return statusPending + " "
	case "Failed", "Error", "CrashLoopBackOff":
		return statusFailed + " "
	case nodeStatusReady:
		return statusReady + " "
	case nodeStatusNotReady:
		return statusNotReady + " "
	default:
		return ""
	}
}
