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

	// _ "github.com/go-echarts/go-echarts/v2"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	pt "github.com/jedib0t/go-pretty/v6/table"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"

	// needed to authenticate with GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const ctlC = "<C-c>"

// formatQuantity returns a human-readable or exact representation of a quantity pointer
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

// formatQuantityValue returns a human-readable or exact representation of a quantity value
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
	t := pt.NewWriter()
	t.SetStyle(pt.StyleColoredBright)
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(pt.Row{
		"Node Name",
		"Status",
		"Kubelet Version",
		"Allocated CPU Req",
		"Allocated CPU Limit",
		"Allocated MEM Req",
		"Allocated MEM Limit",
		"Used CPU",
		"Used MEM",
		"Available CPU",
		"Available MEM",
	})

	for k, v := range *nm {
		status := v.Status
		if status == "" {
			status = "Unknown"
		}

		t.AppendRow(pt.Row{
			k,
			status,
			v.NodeInfo.KubeletVersion,
			formatQuantityValue(v.AllocatedCPUrequests),
			formatQuantityValue(v.AllocatedCPULimits),
			formatQuantityValue(v.AllocatedMemoryRequests),
			formatQuantityValue(v.AllocatedMemoryLimits),
			formatQuantity(v.UsageCPU),
			formatQuantity(v.UsageMemory),
			formatQuantity(v.AllocatableCPU),
			formatQuantity(v.AllocatableMemory),
		})
	}

	t.AppendSeparator()
	t.AppendFooter(pt.Row{
		"TOTALS",
		"",
		"",
		formatQuantity(c.TotalAllocatedCPUrequests),
		formatQuantity(c.TotalAllocatedCPULimits),
		formatQuantity(c.TotalAllocatedMemoryRequests),
		formatQuantity(c.TotalAllocatedMemoryLimits),
		formatQuantity(c.TotalUsageCPU),
		formatQuantity(c.TotalUsageMemory),
		formatQuantity(c.TotalAllocatableCPU),
		formatQuantity(c.TotalAllocatableMemory),
	})

	t.Render()
	os.Exit(0)
}

func table(nm *NodeMap, c *Totals) {
	t := pt.NewWriter()
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateColumns = false
	t.Style().Options.SeparateFooter = false
	t.Style().Options.SeparateHeader = false
	t.Style().Options.SeparateRows = false
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(pt.Row{
		"Name", "Kubelet-Version", "ProviderID", "Allocated-CPU Req", "Allocated-CPU Lim",
		"Allocated-MEM Req", "Allocated-MEM Lim", "Usage-CPU", "Usage-Mem", "Available-CPU", "Available-MEM",
	})

	for k, v := range *nm {
		t.AppendRow([]interface{}{k, v.NodeInfo.KubeletVersion, v.ProviderID,
			formatQuantityValue(v.AllocatedCPUrequests),
			formatQuantityValue(v.AllocatedCPULimits),
			formatQuantityValue(v.AllocatedMemoryRequests),
			formatQuantityValue(v.AllocatedMemoryLimits),
			formatQuantity(v.UsageCPU),
			formatQuantity(v.UsageMemory),
			formatQuantity(v.AllocatableCPU),
			formatQuantity(v.AllocatableMemory)})
	}

	t.AppendFooter(pt.Row{
		"Totals",
		"",
		"",
		formatQuantity(c.TotalAllocatedCPUrequests),
		formatQuantity(c.TotalAllocatedCPULimits),
		formatQuantity(c.TotalAllocatedMemoryRequests),
		formatQuantity(c.TotalAllocatedMemoryLimits),
		formatQuantity(c.TotalUsageCPU),
		formatQuantity(c.TotalUsageMemory),
		formatQuantity(c.TotalAllocatableCPU),
		formatQuantity(c.TotalAllocatableMemory),
	})

	t.Render()
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
