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
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
)

// ResourceMetrics holds the resource values and capacity for progress bars
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
}

// NewLiveCmd creates the live subcommand
func NewLiveCmd(gc *GlanceConfig) *cobra.Command {
	var refreshInterval int

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Display live resource monitoring with interactive TUI",
		Long: `Display a live, continuously updating terminal UI showing Kubernetes resource allocation and usage.

View modes:
  - Namespaces (default): Shows resource requests, limits, and usage per namespace
  - Pods: Shows resource requests, limits, and usage per pod (namespace-scoped)
  - Nodes: Shows node capacity, allocation, and usage
  - Deployments: Shows deployment resource requests and replica status

Controls will be displayed at the bottom of the screen.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := kubernetes.NewForConfig(gc.restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			return runLive(k8sClient, gc, time.Duration(refreshInterval)*time.Second)
		},
	}

	cmd.Flags().IntVarP(&refreshInterval, "refresh", "r", 2, "Refresh interval in seconds")

	return cmd
}

func runLive(k8sClient *kubernetes.Clientset, gc *GlanceConfig, refreshInterval time.Duration) error {
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
		header, data, metrics, err = fetchNamespaceData(ctx, k8sClient, gc)
	case ViewPods:
		header, data, metrics, err = fetchPodData(ctx, k8sClient, gc, state.selectedNamespace)
	case ViewNodes:
		header, data, metrics, err = fetchNodeData(ctx, k8sClient, gc)
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

	// Calculate table height based on compact mode
	tableHeight := termHeight - 6
	if state.compactMode {
		tableHeight = termHeight - 5
	}

	// Update table
	state.table.Rows = append([][]string{header}, data...)
	state.table.TextStyle = ui.NewStyle(ui.ColorWhite)
	state.table.RowSeparator = false
	state.table.BorderStyle = ui.NewStyle(ui.ColorCyan)
	state.table.SetRect(0, 0, termWidth, tableHeight)
	state.table.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

	// Update status bar
	modeStr := getModeString(state.mode)
	if state.selectedNamespace != "" && (state.mode == ViewPods || state.mode == ViewDeployments) {
		modeStr += fmt.Sprintf(" [%s]", state.selectedNamespace)
	}
	state.statusBar.Text = fmt.Sprintf(" %s | Updated: %s | Refresh: %v | Items: %d",
		modeStr,
		state.lastUpdate.Format("15:04:05"),
		state.refreshInterval,
		len(data))
	state.statusBar.Border = false
	state.statusBar.SetRect(0, tableHeight, termWidth, tableHeight+2)

	// Update menu bar with toggles
	state.menuBar.Text = getMenuBar(state)
	state.menuBar.SetRect(0, tableHeight+2, termWidth, tableHeight+3)

	// Update help bar
	if !state.compactMode {
		state.helpBar.SetRect(0, tableHeight+3, termWidth, termHeight)
	}

	if state.compactMode {
		ui.Render(state.table, state.statusBar, state.menuBar)
	} else {
		ui.Render(state.table, state.statusBar, state.menuBar, state.helpBar)
	}
	return nil
}

func fetchNamespaceData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"NAMESPACE", "CPU REQ", "CPU LIMIT", "CPU USAGE", "MEM REQ", "MEM LIMIT", "MEM USAGE", "PODS"}

	namespaces, err := k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return header, [][]string{}, []ResourceMetrics{}, nil // Continue without metrics
	}

	var rows [][]string
	var metrics []ResourceMetrics
	for _, ns := range namespaces.Items {
		cpuReq := resource.NewMilliQuantity(0, resource.DecimalSI)
		cpuLimit := resource.NewMilliQuantity(0, resource.DecimalSI)
		memReq := resource.NewQuantity(0, resource.BinarySI)
		memLimit := resource.NewQuantity(0, resource.BinarySI)
		cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
		memUsage := resource.NewQuantity(0, resource.BinarySI)

		pods, err := k8sClient.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		podCount := len(pods.Items)

		for _, pod := range pods.Items {
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
		}

		// Get metrics for pods in namespace
		podMetrics, err := metricsClient.MetricsV1beta1().PodMetricses(ns.Name).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, pm := range podMetrics.Items {
				for _, container := range pm.Containers {
					cpuUsage.Add(container.Usage[v1.ResourceCPU])
					memUsage.Add(container.Usage[v1.ResourceMemory])
				}
			}
		}

		rows = append(rows, []string{
			ns.Name,
			formatMilliCPU(cpuReq),
			formatMilliCPU(cpuLimit),
			formatMilliCPU(cpuUsage),
			formatBytes(memReq),
			formatBytes(memLimit),
			formatBytes(memUsage),
			fmt.Sprintf("%d", podCount),
		})

		// Store metrics for progress bars
		metrics = append(metrics, ResourceMetrics{
			CPURequest:  float64(cpuReq.MilliValue()) / 1000.0,
			CPULimit:    float64(cpuLimit.MilliValue()) / 1000.0,
			CPUUsage:    float64(cpuUsage.MilliValue()) / 1000.0,
			CPUCapacity: float64(cpuLimit.MilliValue()) / 1000.0,
			MemRequest:  float64(memReq.Value()),
			MemLimit:    float64(memLimit.Value()),
			MemUsage:    float64(memUsage.Value()),
			MemCapacity: float64(memLimit.Value()),
		})
	}

	// Sort by namespace name
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	return header, rows, metrics, nil
}

func fetchPodData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	namespace string,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"POD", "CPU REQ", "CPU LIMIT", "CPU USAGE", "MEM REQ", "MEM LIMIT", "MEM USAGE", "STATUS"}

	pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list pods: %w", err)
	}

	metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return header, [][]string{}, []ResourceMetrics{}, nil
	}

	podMetrics, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	metricsMap := make(map[string]*metricsV1beta1api.PodMetrics)
	if err == nil {
		for i := range podMetrics.Items {
			metricsMap[podMetrics.Items[i].Name] = &podMetrics.Items[i]
		}
	}

	var rows [][]string
	var metrics []ResourceMetrics
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

		rows = append(rows, []string{
			pod.Name,
			formatMilliCPU(cpuReq),
			formatMilliCPU(cpuLimit),
			formatMilliCPU(cpuUsage),
			formatBytes(memReq),
			formatBytes(memLimit),
			formatBytes(memUsage),
			string(pod.Status.Phase),
		})

		metrics = append(metrics, ResourceMetrics{
			CPURequest:  float64(cpuReq.MilliValue()) / 1000.0,
			CPULimit:    float64(cpuLimit.MilliValue()) / 1000.0,
			CPUUsage:    float64(cpuUsage.MilliValue()) / 1000.0,
			CPUCapacity: float64(cpuLimit.MilliValue()) / 1000.0,
			MemRequest:  float64(memReq.Value()),
			MemLimit:    float64(memLimit.Value()),
			MemUsage:    float64(memUsage.Value()),
			MemCapacity: float64(memLimit.Value()),
		})
	}

	return header, rows, metrics, nil
}

func fetchNodeData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"NODE", "CPU CAP", "CPU ALLOC", "CPU USAGE", "MEM CAP", "MEM ALLOC", "MEM USAGE", "PODS"}

	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	metricsClient, err := metricsclientset.NewForConfig(gc.restConfig)
	if err != nil {
		return header, [][]string{}, []ResourceMetrics{}, nil
	}

	nodeMetrics, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	metricsMap := make(map[string]*metricsV1beta1api.NodeMetrics)
	if err == nil {
		for i := range nodeMetrics.Items {
			metricsMap[nodeMetrics.Items[i].Name] = &nodeMetrics.Items[i]
		}
	}

	var rows [][]string
	var metrics []ResourceMetrics
	for _, node := range nodes.Items {
		cpuCap := node.Status.Capacity.Cpu()
		memCap := node.Status.Capacity.Memory()
		cpuAlloc := resource.NewMilliQuantity(0, resource.DecimalSI)
		memAlloc := resource.NewQuantity(0, resource.BinarySI)
		cpuUsage := resource.NewMilliQuantity(0, resource.DecimalSI)
		memUsage := resource.NewQuantity(0, resource.BinarySI)

		pods, err := k8sClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
		})

		podCount := 0
		if err == nil {
			podCount = len(pods.Items)
			for _, pod := range pods.Items {
				for _, container := range pod.Spec.Containers {
					if req := container.Resources.Requests.Cpu(); req != nil {
						cpuAlloc.Add(*req)
					}
					if req := container.Resources.Requests.Memory(); req != nil {
						memAlloc.Add(*req)
					}
				}
			}
		}

		if nm, ok := metricsMap[node.Name]; ok {
			cpuUsage.Add(nm.Usage[v1.ResourceCPU])
			memUsage.Add(nm.Usage[v1.ResourceMemory])
		}

		rows = append(rows, []string{
			node.Name,
			formatMilliCPU(cpuCap),
			formatMilliCPU(cpuAlloc),
			formatMilliCPU(cpuUsage),
			formatBytes(memCap),
			formatBytes(memAlloc),
			formatBytes(memUsage),
			fmt.Sprintf("%d", podCount),
		})

		metrics = append(metrics, ResourceMetrics{
			CPURequest:  float64(cpuAlloc.MilliValue()) / 1000.0,
			CPULimit:    float64(cpuCap.MilliValue()) / 1000.0,
			CPUUsage:    float64(cpuUsage.MilliValue()) / 1000.0,
			CPUCapacity: float64(cpuCap.MilliValue()) / 1000.0,
			MemRequest:  float64(memAlloc.Value()),
			MemLimit:    float64(memCap.Value()),
			MemUsage:    float64(memUsage.Value()),
			MemCapacity: float64(memCap.Value()),
		})
	}

	return header, rows, metrics, nil
}

func fetchDeploymentData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	namespace string,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := []string{"DEPLOYMENT", "CPU REQ", "CPU LIMIT", "MEM REQ", "MEM LIMIT", "REPLICAS", "READY", "AVAILABLE"}

	deployments, err := k8sClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
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

		rows = append(rows, []string{
			deploy.Name,
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
	base := "[n] Namespaces | [p] Pods | [o] Nodes | [d] Deployments | [b] Bars | [%] Percentages | [c] Compact | [q] Quit"
	if mode == ViewPods || mode == ViewDeployments {
		return base + " | [←→] Switch Namespace"
	}
	return base
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

	return fmt.Sprintf(" %s Bars [b] | %s Percentages [%%] | %s Compact [c]",
		barsIcon, percIcon, compactIcon)
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

// makeProgressBar creates a visual progress bar
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
		return fmt.Sprintf("%s %3.0f%%", bar, percentage)
	}
	return bar
}
