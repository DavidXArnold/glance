/*
Copyright 2026 David Arnold
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
	"io"
	"os"
	"time"

	pt "github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	core "gitlab.com/davidxarnold/glance/pkg/core"
	glanceutil "gitlab.com/davidxarnold/glance/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// nonTerminatedPods selects pods that are running or waiting to run.
const nonTerminatedPods = "status.phase!=Succeeded,status.phase!=Failed"

// NewGPUsCmd creates the static "glance gpus" subcommand.
func NewGPUsCmd(gc *GlanceConfig) *cobra.Command {
	return &cobra.Command{
		Use:     "gpus",
		Aliases: []string{"gpu", "accelerators"},
		Short:   "Show GPU allocation, free capacity, and waste",
		Long: `Show where GPUs and other accelerators are allocated and where they are wasted.

Reports, per GPU model and accelerator type:
  - free devices and the largest free block on a single node (fragmentation)
  - GPU nodes with nothing allocated, including ones only running non-GPU pods
  - pending pods waiting for accelerators, and whether they would fit
  - free devices stuck on cordoned or NotReady nodes

Allocation is based on pod requests; it does not measure GPU utilization.
--selector and --field-selector filter nodes. Supports -o txt|pretty|json.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			rc, err := gc.configFlags.ToRESTConfig()
			if err != nil {
				return fmt.Errorf("failed to get kubernetes config: %w", err)
			}
			k8sClient, err := kubernetes.NewForConfig(rc)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			report, err := collectGPUReport(context.Background(), k8sClient)
			if err != nil {
				return err
			}
			return renderGPUReport(os.Stdout, &report, viper.GetString("output"))
		},
	}
}

// collectGPUReport lists nodes (filtered by the selector flags) and all
// non-terminated pods, then builds the GPU report.
func collectGPUReport(ctx context.Context, k8sClient kubernetes.Interface) (core.GPUReport, error) {
	labelSelector, fieldSelector, err := getSelectors()
	if err != nil {
		return core.GPUReport{}, err
	}

	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, buildListOptions(labelSelector, fieldSelector))
	if err != nil {
		return core.GPUReport{}, fmt.Errorf("failed to list nodes: %w", err)
	}
	pods, err := k8sClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		ResourceVersion: "0",
		FieldSelector:   nonTerminatedPods,
	})
	if err != nil {
		return core.GPUReport{}, fmt.Errorf("failed to list pods: %w", err)
	}

	return core.BuildGPUReport(nodes.Items, pods.Items, time.Now()), nil
}

// gpuFindingTitles maps finding kinds to short human-readable titles.
var gpuFindingTitles = map[string]string{
	core.FindingIdleNode:       "Idle GPU node",
	core.FindingIdleBusyNode:   "Idle GPUs on busy node",
	core.FindingFragmentation:  "Fragmented capacity",
	core.FindingCordonedFree:   "Unavailable free GPUs",
	core.FindingPendingNoSpace: "Not enough GPUs",
}

// renderGPUReport writes the report as txt, pretty, or json.
func renderGPUReport(w io.Writer, r *core.GPUReport, output string) error {
	if output == outputFormatJSON {
		b, err := json.MarshalIndent(r, "", "\t")
		if err != nil {
			return fmt.Errorf("failed to render gpu report JSON output: %w", err)
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}

	if len(r.Nodes) == 0 {
		_, err := fmt.Fprintln(w, "No GPU or accelerator nodes found.")
		return err
	}

	style := pt.StyleLight
	if output == outputFormatPretty {
		style = pt.StyleRounded
	}
	newTable := func(title string) pt.Writer {
		t := pt.NewWriter()
		t.SetOutputMirror(w)
		t.SetStyle(style)
		t.SetTitle(title)
		return t
	}

	pools := newTable("GPU POOLS")
	pools.AppendHeader(pt.Row{"MODEL", "RESOURCE", "NODES", "REQUESTED", headerFree, "LARGEST FREE BLOCK", "UNAVAILABLE"})
	for i := range r.Pools {
		p := &r.Pools[i]
		pools.AppendRow(pt.Row{
			p.Model, core.ShortAcceleratorName(p.Resource), p.Nodes,
			fmt.Sprintf("%d / %d", p.Requests, p.Allocatable),
			fmt.Sprintf("%d on %d node(s)", p.Free, p.NodesWithFree),
			p.LargestFreeBlock, p.UnschedulableFree,
		})
	}
	pools.Render()
	_, _ = fmt.Fprintln(w)

	nodes := newTable("GPU NODES")
	nodes.AppendHeader(pt.Row{headerNode, headerGPUModel, "STATE", "GPU REQ/ALLOC", headerFree, "GPU PODS", "OTHER PODS"})
	for i := range r.Nodes {
		n := &r.Nodes[i]
		var free int64
		for _, acc := range n.Accelerators {
			free += acc.Free()
		}
		state := n.State
		if output == outputFormatPretty {
			state = colorGPUNodeState(state)
		}
		nodes.AppendRow(pt.Row{
			n.Name, n.Model, state,
			formatAcceleratorRatios(n.Accelerators, ", "),
			free, n.GPUPods, n.OtherPods,
		})
	}
	nodes.Render()

	if len(r.Pending) > 0 {
		_, _ = fmt.Fprintln(w)
		pending := newTable("PENDING GPU PODS")
		pending.AppendHeader(pt.Row{headerNamespace, headerPod, "REQUEST", "WAITING", "FITS", "REASON"})
		for i := range r.Pending {
			p := &r.Pending[i]
			fits := "yes"
			if !p.Fits {
				fits = fmt.Sprintf("no (largest free %d)", p.LargestFreeBlock)
			}
			pending.AppendRow(pt.Row{
				p.Namespace, p.Name,
				fmt.Sprintf("%d %s", p.Requested, core.ShortAcceleratorName(p.Resource)),
				glanceutil.FormatDuration(p.Age), fits, p.Reason,
			})
		}
		pending.Render()
	}

	if len(r.Findings) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "FINDINGS")
		for _, f := range r.Findings {
			title := gpuFindingTitles[f.Kind]
			if output == outputFormatPretty {
				title = text.Colors{text.FgYellow, text.Bold}.Sprint(title)
			}
			_, _ = fmt.Fprintf(w, "  • %s: %s — %s\n", title, f.Subject, f.Message)
		}
	}
	return nil
}

func colorGPUNodeState(state string) string {
	switch state {
	case core.GPUNodeReady:
		return text.Colors{text.FgGreen}.Sprint(state)
	case core.GPUNodeCordoned:
		return text.Colors{text.FgYellow}.Sprint(state)
	default:
		return text.Colors{text.FgRed, text.Bold}.Sprint(state)
	}
}
