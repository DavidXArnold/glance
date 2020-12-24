/*
Copyright 2020 David Arnold
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
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	pt "github.com/jedib0t/go-pretty/v6/table"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"

	// needed to authenticate with GCP
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const ctlC = "<C-c>"

func render(nm *NodeMap, c *Totals) {
	switch viper.GetString("output") {
	case "json":
		o := &Glance{
			*nm,
			*c,
		}
		g, err := json.MarshalIndent(o, "", "\t")
		if err != nil {
			log.Error(err)
		}
		fmt.Println(string(g))

		os.Exit(0)
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
			v.AllocatedCPUrequests.AsDec().String(), v.AllocatedCPULimits.AsDec().String(),
			v.AllocatedMemoryRequests.String(), v.AllocatedMemoryLimits.String(),
			v.UsageCPU.AsDec().String(), v.UsageMemory.String(), v.AllocatableCPU.AsDec().String(),
			v.AllocatableMemory.String()})
	}

	t.AppendFooter(pt.Row{
		"Totals", "", "", c.TotalAllocatedCPUrequests.AsDec(), c.TotalAllocatedCPULimits.AsDec(), c.TotalAllocatedMemoryRequests,
		c.TotalAllocatedMemoryLimits, c.TotalUsageCPU.AsDec(), c.TotalUsageMemory, c.TotalAllocatableCPU, c.TotalAllocatableMemory,
	})

	t.Render()
}

func chart(nm *NodeMap) {
	log.Fatalf("Not yet implemented")
	// create a new bar instance
	bar := charts.NewBar()
	// set some global options like Title/Legend/ToolTip or anything else
	bar.SetGlobalOptions(charts.WithTitleOpts(opts.Title{
		Title:    "Glance ",
		Subtitle: "It's extremely easy to use, right?",
	}))

	cpu := make([]opts.BarData, 0)

	x := make([]string, 0)

	for k, v := range *nm {
		x = append(x, k)
		cpu = append(cpu,
			opts.BarData{Name: "CPU (Allocated)", Value: v.AllocatedCPUrequests.AsDec().String(),
				Label: &opts.Label{Show: true, Color: "blue", Position: "left"}},
			opts.BarData{Name: "CPU (Limit)", Value: v.AllocatedCPULimits.AsDec().String(),
				Label: &opts.Label{Show: true, Color: "green", Position: "right"}},
			opts.BarData{Name: "CPU (Usage)", Value: v.UsageCPU.AsDec().String(),
				Label: &opts.Label{Show: true, Color: "red", Position: "left"}})
	}

	// Put data into instance
	bar.SetXAxis(x).
		AddSeries("CPU (Allocated)", cpu)
	// Where the magic happens
	f, _ := os.Create("bar.html")
	err := bar.Render(f)
	if err != nil {
		log.Fatalf("error rendering: %v", err)
	}
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
