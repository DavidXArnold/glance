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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	glanceutil "gitlab.com/davidxarnold/glance/pkg/util"
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

// CloudCacheEntry holds cached cloud provider information with TTL.
type CloudCacheEntry struct {
	InstanceType   string
	NodeGroup      string // EKS node group or GKE node pool
	FargateProfile string // AWS Fargate profile
	CapacityType   string // ON_DEMAND, SPOT, FARGATE
	Timestamp      time.Time
}

// CloudCache holds the in-memory cloud info cache with TTL.
type CloudCache struct {
	mu      sync.RWMutex
	cache   map[string]*CloudCacheEntry
	ttl     time.Duration
	useDisk bool
}

// NewCloudCache creates a new cloud cache with the specified TTL.
func NewCloudCache(ttl time.Duration, useDisk bool) *CloudCache {
	c := &CloudCache{
		cache:   make(map[string]*CloudCacheEntry),
		ttl:     ttl,
		useDisk: useDisk,
	}
	if useDisk {
		c.loadFromDisk()
	}
	return c
}

// CloudMetadata holds cloud provider metadata for a node.
type CloudMetadata struct {
	InstanceType   string
	NodeGroup      string
	FargateProfile string
	CapacityType   string
}

// Get retrieves a cached cloud info entry if it exists and is not expired.
func (c *CloudCache) Get(providerID string) (*CloudMetadata, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[providerID]
	if !ok {
		return nil, false
	}
	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}
	return &CloudMetadata{
		InstanceType:   entry.InstanceType,
		NodeGroup:      entry.NodeGroup,
		FargateProfile: entry.FargateProfile,
		CapacityType:   entry.CapacityType,
	}, true
}

// Set stores a cloud info entry in the cache.
func (c *CloudCache) Set(providerID string, metadata *CloudMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[providerID] = &CloudCacheEntry{
		InstanceType:   metadata.InstanceType,
		NodeGroup:      metadata.NodeGroup,
		FargateProfile: metadata.FargateProfile,
		CapacityType:   metadata.CapacityType,
		Timestamp:      time.Now(),
	}
	if c.useDisk {
		// Save to disk in background
		go c.saveToDisk()
	}
}

// getCachePath returns the path to the disk cache file.
func (c *CloudCache) getCachePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Debugf("failed to get home directory: %v", err)
		return ""
	}
	return filepath.Join(homeDir, ".glance", "cloud-cache.json")
}

// loadFromDisk loads cached cloud info from disk.
func (c *CloudCache) loadFromDisk() {
	cachePath := c.getCachePath()
	if cachePath == "" {
		return
	}

	// #nosec G304 - cachePath is computed from user home directory, not user input
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debugf("failed to read cloud cache from disk: %v", err)
		}
		return
	}

	var diskCache map[string]*CloudCacheEntry
	if err := json.Unmarshal(data, &diskCache); err != nil {
		log.Debugf("failed to unmarshal cloud cache: %v", err)
		return
	}

	// Load entries that haven't expired
	c.mu.Lock()
	defer c.mu.Unlock()
	for providerID, entry := range diskCache {
		if time.Since(entry.Timestamp) <= c.ttl {
			c.cache[providerID] = entry
		}
	}
	log.Debugf("loaded %d cloud cache entries from disk", len(c.cache))
}

// saveToDisk saves the current cache to disk.
func (c *CloudCache) saveToDisk() {
	cachePath := c.getCachePath()
	if cachePath == "" {
		return
	}

	// Ensure directory exists
	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		log.Debugf("failed to create cache directory: %v", err)
		return
	}

	c.mu.RLock()
	data, err := json.Marshal(c.cache)
	c.mu.RUnlock()

	if err != nil {
		log.Debugf("failed to marshal cloud cache: %v", err)
		return
	}

	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		log.Debugf("failed to write cloud cache to disk: %v", err)
	}
}

// LiveState holds the state for the live TUI
type LiveState struct {
	mode                   ViewMode
	selectedNamespace      string
	selectedNamespaceIndex int // For up/down navigation in namespace view
	refreshInterval        time.Duration
	lastUpdate             time.Time
	table                  *widgets.Table
	statusBar              *widgets.Paragraph
	menuBar                *widgets.Paragraph
	showBars               bool
	showPercentages        bool
	compactMode            bool
	showRawResources       bool   // Toggle between ratio format and raw resource values
	showCloudInfo          bool   // Toggle cloud provider information display
	showNodeVersion        bool   // Toggle node version display
	showNodeAge            bool   // Toggle node age display
	showNodeGroup          bool   // Toggle node group/pool display
	filterNodeGroup        string // Filter by node group/pool (empty = all)
	filterCapacityType     string // Filter by capacity type: on-demand, spot, fargate (empty = all)
	// Scaling options
	nodeLimit     int
	podLimit      int
	maxConcurrent int
	sortMode      SortMode
	totalNodes    int // Track total for "showing X of Y" display
	totalPods     int
	// Namespace list for navigation
	namespaceList []string
	// Cloud info caching
	cloudCache *CloudCache
	// Settings modal state
	showSettingsModal  bool
	settingsModal      *widgets.Table
	modalSelectedRow   int
	modalScrollOffset  int
	modalDirty         bool
	showConfirmDiscard bool
	// Pending settings (staged changes before save)
	pendingShowBars         bool
	pendingShowPercentages  bool
	pendingCompactMode      bool
	pendingShowRawResources bool
	pendingShowCloudInfo    bool
	pendingShowNodeVersion  bool
	pendingShowNodeAge      bool
	pendingShowNodeGroup    bool
	pendingFilterNodeGroup  string
	pendingFilterCapacity   string
	pendingSortMode         SortMode
	pendingNodeLimit        int
	pendingPodLimit         int
}

// NewLiveCmd creates the live subcommand
func NewLiveCmd(gc *GlanceConfig) *cobra.Command {
	var refreshInterval int
	var nodeLimit int
	var podLimit int
	var maxConcurrent int
	var sortBy string
	var namespace string

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Display live resource monitoring with interactive TUI",
		Long: `Display a live, continuously updating terminal UI showing Kubernetes resource allocation and usage.

View modes:
  - Nodes (default): Shows node capacity, allocation, and usage
  - Namespaces: Shows resource requests, limits, and usage per namespace (navigate with ↑↓, Enter to view)
  - Pods: Shows resource requests, limits, and usage per pod (namespace-scoped)
  - Deployments: Shows deployment resource requests and replica status

Scaling options:
  - Use --node-limit to limit displayed nodes (default: 20)
  - Use --pod-limit to limit displayed pods (default: 100)
  - Use --sort-by to sort by status, cpu, memory, or name (default: status)

Namespace navigation:
  - In Namespaces view: Press ↑↓ to select, Enter to view pods in that namespace
  - In Pods/Deployments views: Press ←→ to cycle through namespaces
  - Use -N/--namespace flag to set initial namespace (default: all namespaces)

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
				nodeLimit, podLimit, maxConcurrent, sortMode, namespace)
		},
	}

	cmd.Flags().IntVarP(&refreshInterval, "refresh", "r", 2, "Refresh interval in seconds")
	cmd.Flags().IntVar(&nodeLimit, "node-limit", defaultNodeLimit,
		"Maximum nodes to display (0 for unlimited)")
	cmd.Flags().IntVar(&podLimit, "pod-limit", defaultPodLimit,
		"Maximum pods to display per view (0 for unlimited)")
	cmd.Flags().IntVar(&maxConcurrent, "max-concurrent", defaultMaxConcurrent,
		"Maximum concurrent API requests")
	cmd.Flags().StringVar(&sortBy, "sort-by", sortByStatus,
		"Sort by: status, name, cpu, memory")
	cmd.Flags().StringVarP(&namespace, "namespace", "N", "",
		"Initial namespace for scoped views (pods, deployments). Empty string means all namespaces.")

	// Bind to viper for config file support
	_ = viper.BindPFlag("namespace", cmd.Flags().Lookup("namespace"))

	return cmd
}

func runLive(
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	refreshInterval time.Duration,
	nodeLimit, podLimit, maxConcurrent int,
	sortMode SortMode,
	initialNamespace string,
) error {
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
	}
	defer ui.Close()

	state := &LiveState{
		mode:                   ViewNodes,
		selectedNamespace:      initialNamespace,
		selectedNamespaceIndex: 0,
		refreshInterval:        refreshInterval,
		lastUpdate:             time.Now(),
		showBars:               true,
		showPercentages:        true,
		compactMode:            false,
		nodeLimit:              nodeLimit,
		podLimit:               podLimit,
		maxConcurrent:          maxConcurrent,
		sortMode:               sortMode,
		cloudCache:             NewCloudCache(viper.GetDuration("cloud-cache-ttl"), viper.GetBool("cloud-cache-disk")),
		showCloudInfo:          viper.GetBool("show-cloud-provider"),
		showNodeVersion:        viper.GetBool("show-node-version"),
		showNodeAge:            viper.GetBool("show-node-age"),
		showNodeGroup:          viper.GetBool("show-node-group"),
		filterNodeGroup:        viper.GetString("filter-node-group"),
		filterCapacityType:     viper.GetString("filter-capacity-type"),
		showSettingsModal:      false,
		modalSelectedRow:       0,
		modalScrollOffset:      0,
		modalDirty:             false,
		showConfirmDiscard:     false,
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
	state.menuBar = widgets.NewParagraph()

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
			if handleUIEvent(e, k8sClient, gc, state) {
				return nil
			}

		case <-ticker.C:
			state.lastUpdate = time.Now()
			if err := updateDisplay(k8sClient, gc, state); err != nil {
				log.Errorf("Failed to update display: %v", err)
			}
		}
	}
}

// handleUIEvent processes UI events and returns true if the app should exit.
func handleUIEvent(e ui.Event, k8sClient *kubernetes.Clientset, gc *GlanceConfig, state *LiveState) bool {
	// Handle confirmation dialog first if active
	if state.showConfirmDiscard {
		handleConfirmEvent(e, state)
		// Only re-render modal and confirmation, don't fetch data
		updateModalDisplay(state)
		return false
	}

	// Handle settings modal if active
	if state.showSettingsModal {
		handleModalEvent(e, state)
		// Only re-render modal, don't fetch data from Kubernetes
		updateModalDisplay(state)
		return false
	}

	// Main UI event handling
	switch e.ID {
	case "q", "<C-c>":
		return true
	case "?", "h":
		// Open settings modal
		initPendingState(state)
		state.showSettingsModal = true
		state.modalSelectedRow = 3 // First selectable row (Progress Bars)
		state.modalScrollOffset = 0
		// Render modal immediately without fetching data
		updateModalDisplay(state)
		return false
	case "n":
		state.mode = ViewNamespaces
		state.selectedNamespace = ""
		state.selectedNamespaceIndex = 0
	case "p":
		state.mode = ViewPods
	case "o":
		state.mode = ViewNodes
	case "d":
		state.mode = ViewDeployments
	case "<Up>":
		handleUpArrow(state)
	case "<Down>":
		handleDownArrow(state)
	case "<Enter>":
		handleEnterKey(state)
	case "<Left>":
		handleLeftArrow(k8sClient, state)
	case "<Right>":
		handleRightArrow(k8sClient, state)
	case "<Resize>":
		// Handle terminal resize - update modal if open
		if state.showSettingsModal {
			updateModalDisplay(state)
			return false
		}
	}

	if err := updateDisplay(k8sClient, gc, state); err != nil {
		log.Errorf("Failed to update display: %v", err)
	}
	return false
}

// handleUpArrow handles up arrow key navigation in namespace view.
func handleUpArrow(state *LiveState) {
	if state.mode == ViewNamespaces && len(state.namespaceList) > 0 {
		if state.selectedNamespaceIndex > 0 {
			state.selectedNamespaceIndex--
		}
	}
}

// handleDownArrow handles down arrow key navigation in namespace view.
func handleDownArrow(state *LiveState) {
	if state.mode == ViewNamespaces && len(state.namespaceList) > 0 {
		if state.selectedNamespaceIndex < len(state.namespaceList)-1 {
			state.selectedNamespaceIndex++
		}
	}
}

// handleEnterKey handles enter key to select namespace and switch to pods view.
func handleEnterKey(state *LiveState) {
	if state.mode == ViewNamespaces && len(state.namespaceList) > 0 {
		state.selectedNamespace = state.namespaceList[state.selectedNamespaceIndex]
		state.mode = ViewPods
	}
}

// handleLeftArrow handles left arrow key to cycle to previous namespace.
func handleLeftArrow(k8sClient *kubernetes.Clientset, state *LiveState) {
	if state.mode == ViewPods || state.mode == ViewDeployments {
		state.selectedNamespace = getPreviousNamespace(k8sClient, state.selectedNamespace)
	}
}

// handleRightArrow handles right arrow key to cycle to next namespace.
func handleRightArrow(k8sClient *kubernetes.Clientset, state *LiveState) {
	if state.mode == ViewPods || state.mode == ViewDeployments {
		state.selectedNamespace = getNextNamespace(k8sClient, state.selectedNamespace)
	}
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		// Calculate base column count (number of non-resource columns to skip)
		baseColCount := 1 // Default for namespace/pod/deployment views (just name column)

		if state.mode == ViewNodes {
			// NODE, STATUS are the base columns
			baseColCount = 2
			// Add optional columns that come before resource columns
			if state.showNodeVersion {
				baseColCount++
			}
			if state.showNodeAge {
				baseColCount++
			}
		}
		data = addProgressBars(data, metrics, state.showPercentages, baseColCount)
	}

	// Calculate summary stats
	summaryStats := calculateSummaryStats(metrics)

	// Calculate table height based on compact mode (leave room for summary)
	summaryHeight := 3
	tableHeight := termHeight - 4 - summaryHeight // 4 = status bar (1) + borders (3)
	if state.compactMode {
		tableHeight = termHeight - 4
		summaryHeight = 0
	}

	// Update table
	state.table.Rows = append([][]string{header}, data...)
	state.table.TextStyle = ui.NewStyle(ui.ColorWhite)
	state.table.RowSeparator = false
	state.table.BorderStyle = ui.NewStyle(ui.ColorCyan)
	state.table.SetRect(0, summaryHeight, termWidth, tableHeight+summaryHeight)
	state.table.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

	// Apply row coloring based on utilization (skip for deployments - they don't have usage metrics)
	if state.mode != ViewDeployments {
		applyRowColors(state.table, metrics, state.showBars)
	} else {
		// Clear any previous row styles for deployments
		for i := 1; i < len(state.table.Rows); i++ {
			delete(state.table.RowStyles, i)
		}
	}

	// Highlight selected namespace row if in namespace view
	if state.mode == ViewNamespaces && len(state.namespaceList) > 0 {
		rowMultiplier := getRowMultiplier(state.showBars)
		selectedRow := (state.selectedNamespaceIndex * rowMultiplier) + 1 // +1 for header
		if selectedRow < len(state.table.Rows) {
			state.table.RowStyles[selectedRow] = ui.NewStyle(ui.ColorBlack, ui.ColorCyan, ui.ModifierBold)
		}
	}

	// Render summary bar at the top (if not compact)
	if !state.compactMode {
		renderSummaryBar(
			summaryStats, termWidth, state.mode, state.selectedNamespace,
			state.nodeLimit, state.podLimit, state.totalNodes, state.totalPods,
		)
	}

	// Update status bar with limit information
	modeStr := getModeString(state.mode)
	limitInfo := ""
	switch state.mode {
	case ViewNodes:
		limitInfo = fmt.Sprintf(" | Limits: %d/%d nodes", min(state.nodeLimit, state.totalNodes), state.totalNodes)
	case ViewPods:
		limitInfo = fmt.Sprintf(" | Limits: %d/%d pods", min(state.podLimit, state.totalPods), state.totalPods)
	}

	// Add filter info if active
	filterInfo := ""
	if state.filterNodeGroup != "" {
		filterInfo = fmt.Sprintf(" | Filters: NodeGroup=%s", state.filterNodeGroup)
	}
	if state.filterCapacityType != "" {
		if filterInfo == "" {
			filterInfo = " | Filters: "
		} else {
			filterInfo += ", "
		}
		filterInfo += fmt.Sprintf("Capacity=%s", state.filterCapacityType)
	}

	// Add dirty indicator if modal is open
	dirtyIndicator := ""
	if state.modalDirty {
		dirtyIndicator = " | [⚠ Unsaved Changes](fg:yellow)"
	}

	sortInfo := fmt.Sprintf(" | Sort: %s", getSortModeString(state.sortMode))

	state.statusBar.Text = fmt.Sprintf(" %s | Updated: %s%s%s%s%s | [?]Settings [q]Quit",
		modeStr,
		state.lastUpdate.Format("15:04:05"),
		limitInfo,
		filterInfo,
		sortInfo,
		dirtyIndicator)
	state.statusBar.Border = false
	state.statusBar.SetRect(0, tableHeight+summaryHeight, termWidth, tableHeight+summaryHeight+1)

	// Render base UI
	ui.Render(state.table, state.statusBar)

	// Render modal overlay if open
	if state.showSettingsModal {
		if state.settingsModal == nil || state.modalDirty {
			state.settingsModal = createSettingsModal(state, termWidth, termHeight)
		}
		ui.Render(state.settingsModal)

		// Render confirmation dialog if needed
		if state.showConfirmDiscard {
			confirmDialog := createConfirmDialog(termWidth, termHeight)
			ui.Render(confirmDialog)
		}
	}

	return nil
}

// updateModalDisplay updates only the modal without fetching data from Kubernetes.
// This is much faster than updateDisplay and should be used when the modal is open.
func updateModalDisplay(state *LiveState) {
	if !state.showSettingsModal {
		return
	}

	termWidth, termHeight := ui.TerminalDimensions()

	// Recreate modal with updated state
	state.settingsModal = createSettingsModal(state, termWidth, termHeight)
	ui.Render(state.settingsModal)

	// Render confirmation dialog if needed
	if state.showConfirmDiscard {
		confirmDialog := createConfirmDialog(termWidth, termHeight)
		ui.Render(confirmDialog)
	}
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
func renderSummaryBar(
	stats SummaryStats, width int, mode ViewMode, selectedNamespace string,
	nodeLimit, podLimit, totalNodes, totalPods int,
) {
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

	// Add namespace display for scoped views
	namespaceInfo := ""
	if mode == ViewPods || mode == ViewDeployments {
		nsDisplay := "All Namespaces"
		if selectedNamespace != "" {
			nsDisplay = selectedNamespace
		}
		namespaceInfo = fmt.Sprintf(" │ [Namespace: [←→]](fg:cyan,mod:bold) [%s](fg:white,mod:bold)", nsDisplay)
	}

	// Add limit display for nodes and pods
	limitInfo := ""
	if mode == ViewNodes && totalNodes > 0 {
		limitInfo = fmt.Sprintf(" │ [Limit:](fg:cyan,mod:bold) [%d/%d](fg:white)", min(nodeLimit, totalNodes), totalNodes)
	} else if mode == ViewPods && totalPods > 0 {
		limitInfo = fmt.Sprintf(" │ [Limit:](fg:cyan,mod:bold) [%d/%d](fg:white)", min(podLimit, totalPods), totalPods)
	}

	summary.Text = fmt.Sprintf(
		" Status: %s %s %s │ CPU: %s %.0f%%%% │ Mem: %s %.0f%%%s%s",
		healthIcon, warnIcon, critIcon,
		cpuBar, stats.AvgCPUUsage,
		memBar, stats.AvgMemUsage,
		namespaceInfo,
		limitInfo,
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
	header := []string{
		"NAMESPACE",
		"CPU REQUESTS/LIMITS",
		"CPU USAGE/LIMITS",
		"MEMORY REQUESTS/LIMITS",
		"MEMORY USAGE/LIMITS",
		"PODS",
	}

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
				formatResourceRatio(cpuReq, cpuLimit, false, state.showRawResources),
				formatResourceRatio(cpuUsage, cpuLimit, false, state.showRawResources),
				formatResourceRatio(memReq, memLimit, true, state.showRawResources),
				formatResourceRatio(memUsage, memLimit, true, state.showRawResources),
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
	namespaceList := make([]string, 0, len(nsData))
	for _, nd := range nsData {
		rows = append(rows, nd.row)
		metrics = append(metrics, nd.metrics)
		namespaceList = append(namespaceList, nd.row[0])
	}

	// Store namespace list for navigation
	state.namespaceList = namespaceList

	// Ensure selected index is within bounds
	if state.selectedNamespaceIndex >= len(namespaceList) {
		state.selectedNamespaceIndex = 0
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
	header := []string{
		"POD",
		"CPU REQUESTS/LIMITS",
		"CPU USAGE/LIMITS",
		"MEMORY REQUESTS/LIMITS",
		"MEMORY USAGE/LIMITS",
		"STATUS",
	}

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
			formatResourceRatio(cpuReq, cpuLimit, false, state.showRawResources),
			formatResourceRatio(cpuUsage, cpuLimit, false, state.showRawResources),
			formatResourceRatio(memReq, memLimit, true, state.showRawResources),
			formatResourceRatio(memUsage, memLimit, true, state.showRawResources),
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
	row            []string
	metrics        ResourceMetrics
	isReady        bool
	cpuUsage       float64
	memUsage       float64
	creationTime   time.Time
	nodeVersion    string
	providerID     string
	region         string
	instanceType   string
	nodeGroup      string
	fargateProfile string
	capacityType   string
}

// buildNodeHeader constructs the table header based on toggle states.
func buildNodeHeader(state *LiveState) []string {
	header := []string{"NODE", "STATUS"}

	if state.showNodeVersion {
		header = append(header, "VERSION")
	}
	if state.showNodeAge {
		header = append(header, "AGE")
	}
	if state.showNodeGroup {
		header = append(header, "NODE GROUP/POOL")
	}

	header = append(header,
		"CPU ALLOCATED/CAPACITY",
		"CPU USAGE/CAPACITY",
		"MEMORY ALLOCATED/CAPACITY",
		"MEMORY USAGE/CAPACITY",
		"PODS",
	)

	if state.showCloudInfo {
		header = append(header, "PROVIDER", "REGION", "INSTANCE TYPE", "CAPACITY")
	}

	return header
}

// fetchNodeMetricsAndPods fetches node metrics and pods in parallel.
func fetchNodeMetricsAndPods(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	metricsClient *metricsclientset.Clientset,
) (*metricsV1beta1api.NodeMetricsList, *v1.PodList, error) {
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

	if err := g.Wait(); err != nil {
		return nodeMetrics, allPods, err
	}

	return nodeMetrics, allPods, nil
}

// processNodeRow builds a single node row with metrics.
func processNodeRow(
	node v1.Node,
	pods []v1.Pod,
	metricsMap map[string]*metricsV1beta1api.NodeMetrics,
	state *LiveState,
) ([]string, ResourceMetrics, nodeRowData) {
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

	// Calculate allocated resources from pods
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

	// Build row with all columns
	row := []string{node.Name, nodeStatus}

	// Add optional columns based on toggles
	if state.showNodeVersion {
		row = append(row, node.Status.NodeInfo.KubeletVersion)
	}
	if state.showNodeAge {
		row = append(row, glanceutil.FormatAge(node.CreationTimestamp.Time))
	}

	// Extract node group/pool from labels
	nodeGroup := extractNodeGroupFromLabels(node.Labels)

	if state.showNodeGroup {
		row = append(row, nodeGroup)
	}

	// Add resource columns
	row = append(row,
		formatResourceRatio(cpuAlloc, cpuCap, false, state.showRawResources),
		formatResourceRatio(cpuUsage, cpuCap, false, state.showRawResources),
		formatResourceRatio(memAlloc, memCap, true, state.showRawResources),
		formatResourceRatio(memUsage, memCap, true, state.showRawResources),
		fmt.Sprintf("%d", podCount),
	)

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

	rowData := nodeRowData{
		row:          row,
		metrics:      metrics,
		isReady:      isReady,
		cpuUsage:     cpuUsagePct,
		memUsage:     memUsagePct,
		creationTime: node.CreationTimestamp.Time,
		nodeVersion:  node.Status.NodeInfo.KubeletVersion,
		providerID:   node.Spec.ProviderID,
		nodeGroup:    nodeGroup,
	}

	// Get region from labels
	if region, ok := node.Labels["topology.kubernetes.io/region"]; ok {
		rowData.region = region
	}

	// Extract capacity type from labels
	capacityType := extractCapacityTypeFromLabels(node.Labels)
	rowData.capacityType = capacityType

	// Extract Fargate profile if present
	if profile, ok := node.Labels["eks.amazonaws.com/fargate-profile"]; ok {
		rowData.fargateProfile = profile
		rowData.capacityType = "FARGATE"
	}

	return row, metrics, rowData
}

// extractNodeGroupFromLabels extracts node group/pool name from node labels.
func extractNodeGroupFromLabels(labels map[string]string) string {
	// EKS node group
	if ng, ok := labels["eks.amazonaws.com/nodegroup"]; ok {
		return ng
	}
	// GKE node pool
	if np, ok := labels["cloud.google.com/gke-nodepool"]; ok {
		return np
	}
	// AKS node pool
	if np, ok := labels["agentpool"]; ok {
		return np
	}
	// Kops instance group
	if ig, ok := labels["kops.k8s.io/instancegroup"]; ok {
		return ig
	}
	return ""
}

// extractCapacityTypeFromLabels extracts capacity type from node labels.
func extractCapacityTypeFromLabels(labels map[string]string) string {
	// EKS capacity type
	if ct, ok := labels["eks.amazonaws.com/capacityType"]; ok {
		return strings.ToUpper(ct)
	}
	// karpenter capacity type
	if ct, ok := labels["karpenter.sh/capacity-type"]; ok {
		return strings.ToUpper(ct)
	}
	// GKE spot
	if spot, ok := labels["cloud.google.com/gke-spot"]; ok {
		if spot == "true" {
			return capacityTypeSpot
		}
	}
	// GKE preemptible (legacy)
	if pre, ok := labels["cloud.google.com/gke-preemptible"]; ok {
		if pre == "true" {
			return capacityTypeSpot
		}
	}
	return "ON_DEMAND"
}

// fetchCloudInfoForNodes fetches cloud provider info for all nodes with caching.
func fetchCloudInfoForNodes(nodeData []nodeRowData, state *LiveState) {
	if !viper.GetBool("show-cloud-provider") || !state.showCloudInfo {
		return
	}

	var cloudWg sync.WaitGroup
	var mu sync.Mutex

	for i := range nodeData {
		if nodeData[i].providerID == "" {
			continue
		}

		cp, id := glanceutil.ParseProviderID(nodeData[i].providerID)
		// Skip if id array is too short
		if len(id) < 2 {
			log.Debugf("invalid provider ID format: %s", nodeData[i].providerID)
			continue
		}

		// Check cache first
		if cachedMetadata, ok := state.cloudCache.Get(nodeData[i].providerID); ok {
			mu.Lock()
			nodeData[i].instanceType = cachedMetadata.InstanceType
			// Override with cached cloud info if available (takes precedence over labels)
			if cachedMetadata.NodeGroup != "" {
				nodeData[i].nodeGroup = cachedMetadata.NodeGroup
			}
			if cachedMetadata.FargateProfile != "" {
				nodeData[i].fargateProfile = cachedMetadata.FargateProfile
			}
			if cachedMetadata.CapacityType != "" {
				nodeData[i].capacityType = cachedMetadata.CapacityType
			}
			mu.Unlock()
			continue
		}

		// Prepare instance ID based on provider format
		// AWS: aws:///zone/i-instanceid -> id = ["", "zone", "i-instanceid"]
		// GCE: gce://project/zone/instance -> id = ["project", "zone", "instance"]
		var instanceID string
		switch cp {
		case "aws":
			// For AWS, we need the actual instance ID (i-xxx), which is id[2]
			if len(id) < 3 {
				log.Debugf("invalid AWS provider ID format: %s", nodeData[i].providerID)
				continue
			}
			instanceID = id[2]
		case "gce":
			// For GCE, we need the full path "project/zone/instance"
			instanceID = strings.Join(id, "/")
		default:
			log.Debugf("unsupported cloud provider: %s", cp)
			continue
		}

		cloudWg.Add(1)
		go func(idx int, provider string, instanceID string, providerID string) {
			defer cloudWg.Done()
			var metadata *CloudMetadata
			var err error
			switch provider {
			case "aws":
				awsMetadata, awsErr := getAWSNodeInfo(instanceID)
				if awsErr == nil && awsMetadata != nil {
					metadata = &CloudMetadata{
						InstanceType:   awsMetadata.InstanceType,
						NodeGroup:      awsMetadata.NodeGroup,
						FargateProfile: awsMetadata.FargateProfile,
						CapacityType:   awsMetadata.CapacityType,
					}
				}
				err = awsErr
			case "gce":
				gceMetadata, gceErr := getGCENodeInfo(instanceID)
				if gceErr == nil && gceMetadata != nil {
					metadata = &CloudMetadata{
						InstanceType: gceMetadata.InstanceType,
						NodeGroup:    gceMetadata.NodePool,
						CapacityType: gceMetadata.CapacityType,
					}
				}
				err = gceErr
			}
			if err == nil && metadata != nil {
				// Cache the result
				state.cloudCache.Set(providerID, metadata)
				mu.Lock()
				nodeData[idx].instanceType = metadata.InstanceType
				// Override with cloud API info if available (more accurate than labels)
				if metadata.NodeGroup != "" {
					nodeData[idx].nodeGroup = metadata.NodeGroup
				}
				if metadata.FargateProfile != "" {
					nodeData[idx].fargateProfile = metadata.FargateProfile
				}
				if metadata.CapacityType != "" {
					nodeData[idx].capacityType = metadata.CapacityType
				}
				mu.Unlock()
			}
		}(i, cp, instanceID, nodeData[i].providerID)
	}
	cloudWg.Wait()

	// Add cloud columns to rows if cloud info is enabled
	if state.showCloudInfo {
		for i := range nodeData {
			provider := ""
			if nodeData[i].providerID != "" {
				cp, _ := glanceutil.ParseProviderID(nodeData[i].providerID)
				provider = strings.ToUpper(cp)
			}
			nodeData[i].row = append(nodeData[i].row,
				provider, nodeData[i].region, nodeData[i].instanceType, nodeData[i].capacityType)
		}
	}
}

func fetchNodeData(
	ctx context.Context,
	k8sClient *kubernetes.Clientset,
	gc *GlanceConfig,
	state *LiveState,
) ([]string, [][]string, []ResourceMetrics, error) {
	header := buildNodeHeader(state)

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

	// Fetch all data in parallel
	nodeMetrics, allPods, err := fetchNodeMetricsAndPods(ctx, k8sClient, metricsClient)
	if err != nil {
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

			pods := podsByNode[node.Name]
			_, _, rowData := processNodeRow(node, pods, metricsMap, state)

			mu.Lock()
			nodeData[idx] = rowData
			mu.Unlock()
		}(i, node)
	}
	wg.Wait()

	// Fetch cloud info asynchronously if enabled
	fetchCloudInfoForNodes(nodeData, state)

	// Apply filters
	nodeData = filterNodeData(nodeData, state)

	// Sort based on sort mode
	sortNodeData(nodeData, state.sortMode)

	// Update total after filtering
	state.totalNodes = len(nodeData)

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

// filterNodeData filters node data based on state filters.
func filterNodeData(data []nodeRowData, state *LiveState) []nodeRowData {
	if state.filterNodeGroup == "" && state.filterCapacityType == "" {
		return data // No filtering needed
	}

	filtered := make([]nodeRowData, 0, len(data))
	for _, row := range data {
		// Filter by node group/pool if specified
		if state.filterNodeGroup != "" {
			if row.nodeGroup == "" || !strings.Contains(strings.ToLower(row.nodeGroup), strings.ToLower(state.filterNodeGroup)) {
				lowerFilter := strings.ToLower(state.filterNodeGroup)
				if row.fargateProfile == "" ||
					!strings.Contains(strings.ToLower(row.fargateProfile), lowerFilter) {
					continue
				}
			}
		}

		// Filter by capacity type if specified
		if state.filterCapacityType != "" {
			if !strings.EqualFold(row.capacityType, state.filterCapacityType) {
				continue
			}
		}

		filtered = append(filtered, row)
	}
	return filtered
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
		"DEPLOYMENT", "STATUS", "CPU REQUESTS/LIMITS", "MEMORY REQUESTS/LIMITS",
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
			formatResourceRatio(cpuReq, cpuLimit, false, false),
			formatResourceRatio(memReq, memLimit, true, false),
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

// formatResourceRatio formats CPU or memory as "used / total" ratio
// For CPU: "10.2 / 17" (cores)
// For memory: "44.7Gi / 66Gi" (binary units)
func formatResourceRatio(used, total *resource.Quantity, isMemory bool, showRaw bool) string {
	if showRaw {
		// Raw mode: show Kubernetes resource strings
		usedStr := "0"
		totalStr := "0"
		if used != nil && !used.IsZero() {
			usedStr = used.String()
		}
		if total != nil && !total.IsZero() {
			totalStr = total.String()
		}
		return fmt.Sprintf("%s / %s", usedStr, totalStr)
	}

	// Ratio mode: human-readable format
	if isMemory {
		return fmt.Sprintf("%s / %s", formatBytes(used), formatBytes(total))
	}
	return fmt.Sprintf("%s / %s", formatMilliCPU(used), formatMilliCPU(total))
}

func formatMilliCPU(q *resource.Quantity) string {
	if q == nil || q.IsZero() {
		return "0"
	}
	return fmt.Sprintf("%.1f", float64(q.MilliValue())/1000.0)
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
func addProgressBars(data [][]string, metrics []ResourceMetrics, showPercentages bool, baseColCount int) [][]string {
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

		// Initialize all bars as empty
		for j := range bars {
			bars[j] = ""
		}

		// baseColCount tells us where resource columns start
		// For namespace view: NAMESPACE (1 col) -> resources start at col 1
		// For node view: NODE, STATUS (2 cols) + optional cols -> resources start after baseColCount
		resourceStartCol := baseColCount

		// Only add progress bars to resource columns (CPU and Memory)
		// We expect at least 4 resource columns after baseColCount
		if len(row) >= resourceStartCol+4 {
			// CPU Allocated bar
			bars[resourceStartCol] = makeProgressBar(m.CPURequest, m.CPUCapacity, 10, showPercentages)
			// CPU Usage bar
			bars[resourceStartCol+1] = makeProgressBar(m.CPUUsage, m.CPUCapacity, 10, showPercentages)
			// Memory Allocated bar
			bars[resourceStartCol+2] = makeProgressBar(m.MemRequest, m.MemCapacity, 10, showPercentages)
			// Memory Usage bar
			bars[resourceStartCol+3] = makeProgressBar(m.MemUsage, m.MemCapacity, 10, showPercentages)
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

// initPendingState copies current settings to pending state when modal opens.
func initPendingState(state *LiveState) {
	state.pendingShowBars = state.showBars
	state.pendingShowPercentages = state.showPercentages
	state.pendingCompactMode = state.compactMode
	state.pendingShowRawResources = state.showRawResources
	state.pendingShowCloudInfo = state.showCloudInfo
	state.pendingShowNodeVersion = state.showNodeVersion
	state.pendingShowNodeAge = state.showNodeAge
	state.pendingShowNodeGroup = state.showNodeGroup
	state.pendingFilterNodeGroup = state.filterNodeGroup
	state.pendingFilterCapacity = state.filterCapacityType
	state.pendingSortMode = state.sortMode
	state.pendingNodeLimit = state.nodeLimit
	state.pendingPodLimit = state.podLimit
}

// applyPendingState copies pending settings to current state and saves.
func applyPendingState(state *LiveState) {
	state.showBars = state.pendingShowBars
	state.showPercentages = state.pendingShowPercentages
	state.compactMode = state.pendingCompactMode
	state.showRawResources = state.pendingShowRawResources
	state.showCloudInfo = state.pendingShowCloudInfo
	state.showNodeVersion = state.pendingShowNodeVersion
	state.showNodeAge = state.pendingShowNodeAge
	state.showNodeGroup = state.pendingShowNodeGroup
	state.filterNodeGroup = state.pendingFilterNodeGroup
	state.filterCapacityType = state.pendingFilterCapacity
	state.sortMode = state.pendingSortMode
	state.nodeLimit = state.pendingNodeLimit
	state.podLimit = state.pendingPodLimit

	// Save to viper
	saveAllSettings(state)
	state.modalDirty = false
}

// saveAllSettings saves all settings to viper configuration.
func saveAllSettings(state *LiveState) {
	viper.Set("show-bars", state.showBars)
	viper.Set("show-percentages", state.showPercentages)
	viper.Set("compact-mode", state.compactMode)
	viper.Set("show-raw-resources", state.showRawResources)
	viper.Set("show-cloud-provider", state.showCloudInfo)
	viper.Set("show-node-version", state.showNodeVersion)
	viper.Set("show-node-age", state.showNodeAge)
	viper.Set("show-node-group", state.showNodeGroup)
	viper.Set("filter-node-group", state.filterNodeGroup)
	viper.Set("filter-capacity-type", state.filterCapacityType)
	viper.Set("sort-by", getSortModeString(state.sortMode))
	viper.Set("node-limit", state.nodeLimit)
	viper.Set("pod-limit", state.podLimit)

	if err := viper.WriteConfig(); err != nil {
		log.Debugf("Failed to write config: %v", err)
	}
}

// updateModalScroll adjusts scroll offset to keep selected row visible.
func updateModalScroll(state *LiveState, totalRows, visibleRows int) {
	if state.modalSelectedRow < state.modalScrollOffset {
		state.modalScrollOffset = state.modalSelectedRow
	}
	if state.modalSelectedRow >= state.modalScrollOffset+visibleRows {
		state.modalScrollOffset = state.modalSelectedRow - visibleRows + 1
	}
	if state.modalScrollOffset < 0 {
		state.modalScrollOffset = 0
	}
	if state.modalScrollOffset > totalRows-visibleRows && totalRows > visibleRows {
		state.modalScrollOffset = totalRows - visibleRows
	}
}

// buildSettingsRows creates the table rows for the settings modal.
func buildSettingsRows(state *LiveState) [][]string {
	rows := [][]string{
		{"Category", "Setting", "Key", "Value"},
		{},
		{"[Display Options](fg:cyan,mod:bold)", "", "", ""},
		{"", "Progress Bars", "[b]", boolToCheckbox(state.pendingShowBars)},
		{"", "Percentages", "[%]", boolToCheckbox(state.pendingShowPercentages)},
		{"", "Compact Mode", "[c]", boolToCheckbox(state.pendingCompactMode)},
		{"", "Raw Resources", "[r]", boolToCheckbox(state.pendingShowRawResources)},
		{},
		{"[Node Columns](fg:cyan,mod:bold)", "", "", ""},
		{"", "Cloud Provider Info", "[i]", boolToCheckbox(state.pendingShowCloudInfo)},
		{"", "Node Version", "[v]", boolToCheckbox(state.pendingShowNodeVersion)},
		{"", "Node Age", "[a]", boolToCheckbox(state.pendingShowNodeAge)},
		{"", "Node Group/Pool", "[g]", boolToCheckbox(state.pendingShowNodeGroup)},
		{},
		{"[Sorting](fg:cyan,mod:bold)", "", "", ""},
		{"", "Sort by Status", "", sortModeRadio(state.pendingSortMode, SortByStatus)},
		{"", "Sort by Name", "", sortModeRadio(state.pendingSortMode, SortByName)},
		{"", "Sort by CPU", "", sortModeRadio(state.pendingSortMode, SortByCPU)},
		{"", "Sort by Memory", "", sortModeRadio(state.pendingSortMode, SortByMemory)},
		{},
		{"[Limits](fg:cyan,mod:bold)", "", "", ""},
		{"", "Node Limit", "[←/→]", fmt.Sprintf("%d", state.pendingNodeLimit)},
		{"", "Pod Limit", "[←/→]", fmt.Sprintf("%d", state.pendingPodLimit)},
		{},
		{"[Filters](fg:cyan,mod:bold)", "", "", ""},
		{"", "Node Group Filter", "[Enter]", filterValue(state.pendingFilterNodeGroup)},
		{"", "Capacity Type Filter", "[Enter]", filterValue(state.pendingFilterCapacity)},
	}
	return rows
}

// boolToCheckbox converts boolean to checkbox symbol.
func boolToCheckbox(b bool) string {
	if b {
		return checkboxChecked
	}
	return checkboxUnchecked
}

// sortModeRadio returns radio button symbol for sort mode.
func sortModeRadio(current, target SortMode) string {
	if current == target {
		return "●"
	}
	return "○"
}

// filterValue formats filter value for display.
func filterValue(s string) string {
	if s == "" {
		return "[none]"
	}
	return fmt.Sprintf("[%s]", s)
}

// createSettingsModal creates the settings modal table widget.
func createSettingsModal(state *LiveState, termWidth, termHeight int) *widgets.Table {
	table := widgets.NewTable()

	// Styling
	table.Border = true
	table.BorderStyle = ui.NewStyle(ui.ColorCyan)

	// Calculate total and visible rows
	allRows := buildSettingsRows(state)
	totalRows := len(allRows)
	visibleRows := termHeight - 10
	if visibleRows < 15 {
		visibleRows = 15
	}
	if visibleRows > totalRows {
		visibleRows = totalRows
	}

	// Update scroll if needed
	updateModalScroll(state, totalRows, visibleRows)

	// Slice rows for scrolling
	endRow := state.modalScrollOffset + visibleRows
	if endRow > totalRows {
		endRow = totalRows
	}
	table.Rows = allRows[state.modalScrollOffset:endRow]

	// Title with scroll position
	scrollInfo := ""
	if totalRows > visibleRows {
		scrollInfo = fmt.Sprintf(" (%d/%d)", state.modalSelectedRow+1, totalRows)
	}
	table.Title = fmt.Sprintf(" ⚙️  Settings%s ", scrollInfo)
	table.TitleStyle = ui.NewStyle(ui.ColorCyan, ui.ColorBlack, ui.ModifierBold)

	// Center the modal
	modalWidth := 70
	modalHeight := visibleRows + 2 // +2 for borders
	modalX := (termWidth - modalWidth) / 2
	modalY := (termHeight - modalHeight) / 2
	table.SetRect(modalX, modalY, modalX+modalWidth, modalY+modalHeight)

	// Column widths
	table.ColumnWidths = []int{20, 25, 10, 15}

	// Style header row
	table.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

	// Highlight selected row (adjust for scroll offset)
	selectedIdx := state.modalSelectedRow - state.modalScrollOffset + 1
	if selectedIdx >= 0 && selectedIdx < len(table.Rows) {
		// Only highlight non-empty, non-category rows
		row := allRows[state.modalSelectedRow]
		if len(row) > 1 && row[1] != "" && row[0] == "" {
			table.RowStyles[selectedIdx] = ui.NewStyle(ui.ColorBlack, ui.ColorCyan, ui.ModifierBold)
		}
	}

	table.TextStyle = ui.NewStyle(ui.ColorWhite)

	return table
}

// createConfirmDialog creates a confirmation dialog for unsaved changes.
func createConfirmDialog(termWidth, termHeight int) *widgets.Paragraph {
	dialog := widgets.NewParagraph()

	dialog.Border = true
	dialog.BorderStyle = ui.NewStyle(ui.ColorYellow)
	dialog.Title = " ⚠ Confirm "
	dialog.TitleStyle = ui.NewStyle(ui.ColorYellow, ui.ColorBlack, ui.ModifierBold)

	dialog.Text = "[Discard unsaved changes?](fg:yellow,mod:bold)\n\n" +
		"[Y] Yes, discard\n" +
		"[N] No, return to settings"

	// Center the dialog
	dialogWidth := 50
	dialogHeight := 7
	dialogX := (termWidth - dialogWidth) / 2
	dialogY := (termHeight - dialogHeight) / 2
	dialog.SetRect(dialogX, dialogY, dialogX+dialogWidth, dialogY+dialogHeight)

	return dialog
}

// handleModalEvent processes keyboard events when settings modal is open.
func handleModalEvent(e ui.Event, state *LiveState) {
	_, termHeight := ui.TerminalDimensions()
	allRows := buildSettingsRows(state)
	totalRows := len(allRows)
	visibleRows := termHeight - 10
	if visibleRows < 15 {
		visibleRows = 15
	}
	if visibleRows > totalRows {
		visibleRows = totalRows
	}

	// Find next/previous selectable row
	findNextSelectableRow := func(current, direction int) int {
		for i := current + direction; i >= 0 && i < totalRows; i += direction {
			row := allRows[i]
			// Selectable if: not empty, has content in column 1, not a category header
			if len(row) > 1 && row[1] != "" && row[0] == "" {
				return i
			}
		}
		return current
	}

	switch e.ID {
	case "<Escape>", "q", "Q":
		if state.modalDirty {
			state.showConfirmDiscard = true
		} else {
			state.showSettingsModal = false
		}

	case "s", "S":
		applyPendingState(state)
		state.showSettingsModal = false

	case "<Up>":
		state.modalSelectedRow = findNextSelectableRow(state.modalSelectedRow, -1)
		updateModalScroll(state, totalRows, visibleRows)

	case "<Down>":
		state.modalSelectedRow = findNextSelectableRow(state.modalSelectedRow, 1)
		updateModalScroll(state, totalRows, visibleRows)

	case "<PageUp>":
		newRow := state.modalSelectedRow - visibleRows
		if newRow < 0 {
			newRow = 0
		}
		state.modalSelectedRow = findNextSelectableRow(newRow, 1)
		updateModalScroll(state, totalRows, visibleRows)

	case "<PageDown>":
		newRow := state.modalSelectedRow + visibleRows
		if newRow >= totalRows {
			newRow = totalRows - 1
		}
		state.modalSelectedRow = findNextSelectableRow(newRow, -1)
		updateModalScroll(state, totalRows, visibleRows)

	case "<Space>", "<Enter>":
		// Toggle setting at selected row
		toggleModalSetting(state, allRows[state.modalSelectedRow])

	case "<Left>":
		// Decrease limits
		adjustModalLimit(state, allRows[state.modalSelectedRow], -10)

	case "<Right>":
		// Increase limits
		adjustModalLimit(state, allRows[state.modalSelectedRow], 10)
	}
}

// toggleModalSetting toggles a setting in the modal based on the selected row.
func toggleModalSetting(state *LiveState, row []string) {
	if len(row) < 2 || row[1] == "" {
		return
	}

	settingName := row[1]
	switch settingName {
	case "Progress Bars":
		state.pendingShowBars = !state.pendingShowBars
		state.modalDirty = true
	case "Percentages":
		state.pendingShowPercentages = !state.pendingShowPercentages
		state.modalDirty = true
	case "Compact Mode":
		state.pendingCompactMode = !state.pendingCompactMode
		state.modalDirty = true
	case "Raw Resources":
		state.pendingShowRawResources = !state.pendingShowRawResources
		state.modalDirty = true
	case "Cloud Provider Info":
		state.pendingShowCloudInfo = !state.pendingShowCloudInfo
		state.modalDirty = true
	case "Node Version":
		state.pendingShowNodeVersion = !state.pendingShowNodeVersion
		state.modalDirty = true
	case "Node Age":
		state.pendingShowNodeAge = !state.pendingShowNodeAge
		state.modalDirty = true
	case "Node Group/Pool":
		state.pendingShowNodeGroup = !state.pendingShowNodeGroup
		state.modalDirty = true
	case "Sort by Status":
		state.pendingSortMode = SortByStatus
		state.modalDirty = true
	case "Sort by Name":
		state.pendingSortMode = SortByName
		state.modalDirty = true
	case "Sort by CPU":
		state.pendingSortMode = SortByCPU
		state.modalDirty = true
	case "Sort by Memory":
		state.pendingSortMode = SortByMemory
		state.modalDirty = true
	}
}

// adjustModalLimit adjusts node or pod limit in the modal.
func adjustModalLimit(state *LiveState, row []string, delta int) {
	if len(row) < 2 {
		return
	}

	switch row[1] {
	case "Node Limit":
		state.pendingNodeLimit += delta
		if state.pendingNodeLimit < 10 {
			state.pendingNodeLimit = 10
		}
		if state.pendingNodeLimit > 1000 {
			state.pendingNodeLimit = 1000
		}
		state.modalDirty = true
	case "Pod Limit":
		state.pendingPodLimit += delta
		if state.pendingPodLimit < 10 {
			state.pendingPodLimit = 10
		}
		if state.pendingPodLimit > 10000 {
			state.pendingPodLimit = 10000
		}
		state.modalDirty = true
	}
}

// handleConfirmEvent processes keyboard events for the confirmation dialog.
func handleConfirmEvent(e ui.Event, state *LiveState) {
	switch e.ID {
	case "y", "Y":
		// Discard changes and close modal
		state.showConfirmDiscard = false
		state.showSettingsModal = false
		state.modalDirty = false
	case "n", "N", "<Escape>":
		// Return to modal
		state.showConfirmDiscard = false
	}
}
