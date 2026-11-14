<p align="center">
  <img src="assets/glance-logo.png" alt="Glance Logo">
</p>

# `glance` - kubectl plugin to view cluster resource allocation and usage

[![Go Version](https://img.shields.io/badge/Go-1.26-blue.svg)](https://golang.org)
[![client-go](https://img.shields.io/badge/client--go-v0.37-blue.svg)](https://github.com/kubernetes/client-go)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A kubectl plugin for viewing Kubernetes cluster resource allocation, utilization, and live monitoring. Provides quick insights into CPU, memory, and GPU usage across nodes, namespaces, pods, and deployments, and shows where GPUs are wasted.

> Note: Glance is developed on GitLab at https://gitlab.com/davidxarnold/glance and mirrored to GitHub at https://github.com/DavidXArnold/glance. Please open issues and feature requests on GitLab.

## Features

- 📊 **Multiple Output Formats** - Text, Pretty tables, JSON, Dashboard, Pie charts, and more
- 🔄 **Live Monitoring** - Real-time TUI with auto-refresh for continuous observation
- 🎯 **Multiple View Modes** - Nodes (default), Namespaces with navigation, Pods, Deployments, and GPUs
- 📈 **Resource Metrics** - CPU and memory requests, limits, and actual usage with ratio formatting
- 🎮 **GPU & Accelerator Support** - NVIDIA (including MIG and shared GPUs), AMD, Intel, AWS Neuron, Gaudi, and TPU resources with per-type counts, GPU model detection, and `--show-gpu` auto-detection
- ☁️ **Cloud Provider Integration** - Optional AWS/GCP node metadata columns, enabled explicitly via flags or live view toggles
- 🔍 **Flexible Filtering** - Label and field selectors for targeted views
- 📏 **Value Display Options** - Human-readable ratios or raw Kubernetes resource values
- 🛠️ **Performance Optimizations** - Parallel API fetching, watch cache, and batch operations for large clusters


## Table of Contents

- [Installation](#installation)
  - [Krew (Recommended)](#krew-kubectl-plugin-manager)
  - [From Source](#from-source)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Static View](#static-view)
  - [GPUs and other accelerators](#gpus-and-other-accelerators)
  - [Live View](#live-view)
- [Output Formats](#output-formats-1)
- [Filtering and Selection](#filtering-and-selection)
- [Cloud Provider Integration](#cloud-provider-integration)
- [CLI Flags Reference](#cli-flags-reference)
- [Configuration](#configuration)
- [Examples](#examples)
- [Requirements](#requirements-1)
- [Performance and Scaling](#performance-and-scaling)
- [Development](#development)
- [License](#license)

## Installation

### krew (kubectl plugin manager)

The easiest way to install glance is through [krew](https://krew.sigs.k8s.io/):

```shell
kubectl krew update
kubectl krew install glance
```

### From Source

Build from source with Go 1.26 or higher:

```shell
git clone https://gitlab.com/davidxarnold/glance.git
cd glance
make build
cp target/kubectl-glance /usr/local/bin/
```

## Quick Start

```shell
# View cluster resources (pretty output is now the default)
kubectl glance

# Start live monitoring with interactive TUI
kubectl glance live

# View with simple text output (no colors)
kubectl glance -o txt

# Monitor specific namespace pods in real-time
kubectl glance live  # then press 'p' and use ←→ to navigate namespaces
```



## Usage 

### Static View

**`kubectl glance`** provides a point-in-time snapshot of cluster resources, showing allocation and utilization across nodes.

```shell
kubectl glance
```

In addition to the top-level node view, there are dedicated static commands for
pods and deployments that mirror the live views but run once and exit:

```shell
# Static pods view (uses your kubeconfig context's namespace, like kubectl)
kubectl glance pods

# Static pods view for a specific namespace
kubectl glance pods -n default

# Static deployments view (kubeconfig context's namespace)
kubectl glance deployments

# Static deployments view for a specific namespace
kubectl glance deployments -n production

# GPU allocation and waste: free capacity, fragmentation, pending GPU pods
kubectl glance gpus
```

These commands support the same selectors and output formats as the root
command:

```shell
# Filter by labels and show JSON for scripting
kubectl glance pods -l app=myapp -o json

# Field selector + pretty output
kubectl glance deployments \
  --field-selector metadata.namespace=prod \
  -o pretty
```

**Example Output (nodes, `-o pretty`):**
```

╔══════════════════════════════════════════════════════════════════════════════╗
║                       glance (<) (<)   context: prod                         ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Host: https://k8s.example.com:6443  │  Version: v1.24.3+k3s1                ║
║  Nodes: ● 1 Ready                                                            ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  CPU Usage:      [░░░░░░░░░░░░░░░░░░░░░░░░░░]   2.0%  (118m / 6.0)           ║
║  CPU Allocated:  [░░░░░░░░░░░░░░░░░░░░░░░░░░]   3.3%  (200m / 6.0)           ║
║                                                                              ║
║  Mem Usage:      [██░░░░░░░░░░░░░░░░░░░░░░░░]  10.9%  (1.27Gi / 11.66Gi)     ║
║  Mem Allocated:  [░░░░░░░░░░░░░░░░░░░░░░░░░░]   1.2%  (140.00Mi / 11.66Gi)   ║
╚══════════════════════════════════════════════════════════════════════════════╝

╭────────────────┬─────────┬──────────────┬────────────────────────────┬──────────────────────────────╮
│ NODE           │ STATUS  │ VERSION      │ CPU UTILIZATION            │ MEMORY UTILIZATION           │
├────────────────┼─────────┼──────────────┼────────────────────────────┼──────────────────────────────┤
│ node-1         │ ✓ Ready │ v1.24.3+k3s1 │ ░░░░░░░░░░░░   2.0% █▅▁    │ ▓░░░░░░░░░░░  10.9% ▁█▁      │
│                │         │              │ Req: 200m  Limit: 0        │ Req: 140.00Mi  Lim: 170.00Mi │
├────────────────┼─────────┼──────────────┼────────────────────────────┼──────────────────────────────┤
│ CLUSTER TOTALS │ 1 NODES │              │ USAGE:   2.0%  REQ:   3.3% │ USAGE:  10.9%  REQ:   1.2%   │
│                │         │              │ 118M / 6.0                 │ 1.27GI / 11.66GI             │
╰────────────────┴─────────┴──────────────┴────────────────────────────┴──────────────────────────────╯

Legend: ■ <50%  ■ 50-75%  ■ 75-90%  ■ >90%  Sparkline: Req→Usage→Limit
```

#### Output Formats

Choose from multiple output formats to suit your workflow:

| Format | Flag | Description |
|--------|------|-------------|
| **Pretty** | `pretty` (default) | Colorful table with cluster summary, progress bars, and status icons |
| **Text** | `txt` | Clean ASCII table with borders, utilization percentages, and capacity summary |
| **JSON** | `json` | Machine-readable JSON for scripting/automation |
| **Dashboard** | `dash` | Terminal dashboard with visual elements |
| **Pie Chart** | `pie` | Resource allocation pie chart visualization |
| **Chart** | `chart` | Resource usage charts and graphs |

**Examples:**
```shell
# Default pretty output with cluster summary and visual indicators
kubectl glance

# Simple text output with ASCII borders
kubectl glance -o txt

# JSON output for automation
kubectl glance -o json | jq '.Totals.TotalUsageCPU'

# Visual dashboard
kubectl glance -o dash

# Pie chart visualization
kubectl glance -o pie
```

#### Static View Options

```shell
# Default: cloud provider columns are hidden
kubectl glance
# Show cloud provider columns (AWS/GCP node details)
kubectl glance --show-cloud-provider=true
# Explicitly hide cloud info columns (overriding config)
kubectl glance --show-cloud-provider=false

# Show raw Kubernetes values instead of human-readable (e.g., 1500m vs 1.5)
kubectl glance --raw            # --exact is an alias

# Control node columns in static output (top-level kubectl glance)
# VERSION is shown by default unless explicitly disabled.
# AGE and GROUP are only shown when enabled.
kubectl glance --show-node-age             # add AGE column
kubectl glance --show-node-group           # add GROUP column
kubectl glance --show-node-version=false   # hide VERSION column
kubectl glance --show-gpu                  # show GPU columns (auto-enabled when GPUs detected)
kubectl glance --gpu-resources 'example.com/npu,vendor.io/*'  # count extra device-plugin resources as GPUs

# Combine options
kubectl glance -c -o pretty --raw --show-node-age --show-node-group
```

### GPUs and other accelerators

Glance recognizes these extended resources as accelerators:

| Vendor | Resources |
|---|---|
| NVIDIA | `nvidia.com/gpu`, `nvidia.com/gpu.shared` (time-slicing/MPS), `nvidia.com/mig-*` (MIG slices) |
| AMD | `amd.com/gpu` |
| Intel | `gpu.intel.com/i915`, `gpu.intel.com/xe` |
| AWS | `aws.amazon.com/neuron`, `aws.amazon.com/neurondevice`, `aws.amazon.com/neuroncore` |
| Habana | `habana.ai/gaudi` |
| Google | `google.com/tpu` |
| Any | resources ending in `/gpu`, plus anything passed with `--gpu-resources` |

Counts are kept per resource type, so a node with one full GPU and seven MIG
slices shows `1 / 1 gpu, 3 / 7 mig-1g.10gb` rather than a misleading `4 / 8`.
Requests follow the scheduler's rules: init containers, sidecars, and pod
overhead are included.

#### Finding wasted GPUs

`kubectl glance gpus` (aliases `gpu`, `accelerators`) shows where GPUs are
allocated and where they are wasted. Press `G` in live mode for the same view.

```shell
kubectl glance gpus                  # pools, GPU nodes, pending GPU pods, findings
kubectl glance gpus -l pool=gpu      # only nodes matching a label selector
kubectl glance gpus -o json          # machine-readable report
```

It reports:

- **GPU pools**: per GPU model and accelerator type, how many devices are
  requested and free, and the **largest free block**: the most free devices
  on any one schedulable node. For example, eight nodes with one free GPU each
  have 8 free GPUs but cannot run a 4-GPU job.
- **GPU nodes**: per node, requested and allocatable devices, free devices,
  and how many GPU and non-GPU pods it runs. DaemonSet pods (device plugins,
  exporters) and static pods (such as kube-proxy on GKE) are not counted.
- **Pending GPU pods**: unscheduled pods requesting accelerators, how long
  they have waited, the scheduler's reason, and whether a single node has
  enough free devices of that type (this check ignores taints and affinity).
- **Findings**: idle GPU nodes, idle GPUs on nodes busy with non-GPU pods,
  fragmented capacity (enough free GPUs in total but not on one node), pods
  that need more GPUs than are free, and free GPUs stuck on cordoned or
  NotReady nodes.

This is based on pod requests, not measured GPU utilization: a GPU that is
requested but idle still counts as allocated.

The **GPU MODEL** column comes from node labels, checked in this order:
`nvidia.com/gpu.product` (NVIDIA GPU Feature Discovery / GPU Operator),
`cloud.google.com/gke-accelerator` (GKE), and
`karpenter.k8s.aws/instance-gpu-name` (Karpenter on AWS). Nodes without these
labels show `unknown`. The cluster summary in `-o pretty` breaks allocation
down by model and resource type.

### Live View

**`kubectl glance live`** provides a continuously updating TUI (Terminal User Interface) for real-time cluster monitoring, similar to `kubectl top` but with much richer information and multiple view modes.

```shell
kubectl glance live
```

**Features:**
- 🔄 Auto-refresh every 2 seconds (configurable)
- 🎯 Five view modes
- ⌨️ Keyboard-driven navigation
- 📊 Live resource metrics from metrics-server
- 📊 Visual progress bars with color indicators (🟢🟡🔴)
- 📈 Cluster summary dashboard showing aggregate stats
- ✓ Status icons for nodes, pods, and deployments
- 🎨 Color-coded rows based on utilization levels
- 🎛️ Interactive toggles for display options
- 📱 Terminal responsive

#### View Modes

Switch between views using keyboard shortcuts:

| Key | View | Description |
|-----|------|-------------|
| **o** | Nodes | Node capacity, allocation, and current usage across cluster (**default**) |
| **n** | Namespaces | Resource requests, limits, and usage per namespace (navigate with ↑↓, Enter to view) |
| **p** | Pods | Resource requests, limits, and usage per pod with namespace selection |
| **d** | Deployments | Deployment resources, replica counts, and availability status |
| **G** | GPUs | GPU pools, GPU nodes, pending GPU pods, and wasted-GPU findings (see [Finding wasted GPUs](#finding-wasted-gpus)) |

**Default View:** Nodes view shows cluster-wide node status on startup.

**Namespace Navigation:**
- In **Namespaces view**: Use ↑↓ arrows to select a namespace, press Enter to view pods in that namespace
- In **Pods/Deployments views**: Use ←→ arrows to cycle through namespaces
- Use kubectl's `-n` / `--namespace` flag to start in a specific namespace (default: your kubeconfig context's namespace)

**Sort Modes:**

You can configure sort order via both flags and keys:

```shell
# CLI flag (applies at startup)
kubectl glance live --sort-by=status   # or: name|cpu|memory
```

At runtime in live view, use keys `1`–`4` to switch sort mode:
- `1` – Status
- `2` – Name
- `3` – CPU
- `4` – Memory

#### Keyboard Controls

| Key | Action |
|-----|--------|
| `n` | Switch to **Namespaces** view |
| `p` | Switch to **Pods** view |
| `o` | Switch to **Nodes** view |
| `d` | Switch to **Deployments** view |
| `G` | Switch to **GPUs** view (pools, GPU nodes, pending GPU pods, findings) |
| `b` | Toggle **progress bars** on/off |
| `%` | Toggle **percentages** on progress bars |
| `r` | Toggle **raw data** display (e.g., "1500m" vs "1.5 / 2.0") |
| `c` | Toggle **compact mode** (hide summary and help) |
| `w` | Toggle **cloud info** columns (Provider, Region, Instance Type, Capacity) |
| `v` | Toggle **node version** column (Kubelet version) |
| `a` | Toggle **node age** column (time since creation) |
| `g` | Toggle **node group/pool** column |
| `u` | Toggle **GPU resource** columns (requests/allocatable) |
| `1` | Sort by **Status** |
| `2` | Sort by **Name** |
| `3` | Sort by **CPU** |
| `4` | Sort by **Memory** |
| `?` / `h` | Open the **settings panel** (all toggles, plus node and pod display limits); `s` saves, `Esc` closes |
| `↑↓` | Select namespace (in Namespaces view) |
| `Enter` | View pods for selected namespace (in Namespaces view) |
| `←→` | Navigate namespaces (in Pods/Deployments view) |
| `q` | Quit live view |

#### Display Features

**Progress Bars:**
- Visual bars under each resource metric (CPU request, limit, usage; Memory request, limit, usage)
- Shows resource utilization at a glance with filled/unfilled blocks
- Toggle with `b` key
- Optional percentage display with `%` key

**Menu Bar:**
- Bottom menu shows current toggle states with checkboxes (☑/☐)
- Indicates which features are enabled/disabled
- Always visible for quick reference
- Status bar also shows the active sort mode and the sort keybinds (`[1]status [2]name [3]cpu [4]memory`) so users can easily change ordering.

**Compact Mode:**
- Toggle with `c` key
- Hides detailed help text to maximize data display area
- Useful for smaller terminals or when focused on metrics

#### Live View Examples

```shell
# Start live view with default settings
kubectl glance live

# Custom refresh interval (5 seconds)
kubectl glance live -r 5
kubectl glance live --refresh 10

# Workflow examples:
# 1. Start in namespaces view, toggle bars and percentages
kubectl glance live
# Press 'b' to see progress bars
# Press '%' to add percentages to bars

# 2. Monitor specific namespace pods
kubectl glance live
# Press 'p' to switch to pods view
# Use ←→ to navigate to your namespace

# 3. Compact mode for smaller terminals
kubectl glance live
# Press 'c' to enable compact mode
# Press 'b' to hide bars if needed

# 4. Node monitoring with visual feedback
kubectl glance live
# Press 'o' for nodes view
# Progress bars show capacity utilization
```

#### Screen Layout

The live view has four parts, top to bottom:

1. **Summary bar**: context, cluster/provider, node and pod counts, and the
   current namespace (Pods and Deployments views).
2. **Table** for the current view. With progress bars on (`b`), each
   resource row gets bars underneath, colored by utilization.
3. **Status bar**: view name, refresh time, sort mode, and how many
   nodes/pods are shown when a limit applies (e.g. `Viewing Nodes: 20/150`).
4. **Menu**: view keys, toggles, and sort keys.

**Color Indicators** (live progress bars):
- 🟢 Green: < 50% utilization
- 🟡 Yellow: 50-90% utilization
- 🔴 Red: > 90% utilization

**Status Icons:**
- ✓ Ready / ⊘ NotReady (nodes)
- ● Running / ○ Pending / ✗ Failed (pods)
- ✓ Ready / ✗ NotReady / ○ Partial (deployments)

The bars use Unicode block characters (█ for filled, ░ for empty) to provide instant visual feedback on resource utilization.

#### What Each View Shows

**Namespaces View** (`n`)
- Lists all namespaces in the cluster
- CPU and memory requests/limits/usage per namespace
- Pod count per namespace
- Sorted alphabetically

**Pods View** (`p`)
- Lists all pods in selected namespace
- CPU and memory requests/limits/usage per pod
- Pod status (Running, Pending, etc.)
- Navigate namespaces with ←→ arrows

**Nodes View** (`o`)
- Lists all nodes in the cluster
- Node capacity (total available resources)
- Allocated resources (sum of pod requests)
- Actual usage from metrics-server
- Pod count per node

**Deployments View** (`d`)
- Lists all deployments in selected namespace
- Total resource requests/limits across all replicas
- Desired replica count
- Ready replica count
- Available replica count
- Navigate namespaces with ←→ arrows

**GPUs View** (`G`)
- GPU pools per model and accelerator type: requested, free, and the largest free block
- GPU nodes with free devices and GPU vs. non-GPU pod counts
- Pending GPU pods and whether they fit
- Findings: idle GPU nodes, fragmentation, and unavailable free GPUs

## Output Formats

Glance supports multiple output formats for different use cases:

### Text Format
Clean ASCII table with borders, status column, utilization percentages, and cluster capacity summary. Ideal for environments without color support or for piping to other tools.

```shell
kubectl glance -o txt
```

**Features:**
- ASCII box borders for clear structure
- Sorted node output for consistency
- CPU % and MEM % utilization columns
- Node status column (Ready/NotReady)
- Cluster capacity summary section

### Pretty Format (default)
Full-featured colored output with cluster summary dashboard, progress bars, and status indicators. Best for interactive terminal sessions.

```shell
kubectl glance
kubectl glance -o pretty
```

**Features:**
- 📊 Cluster summary dashboard with aggregate CPU/memory stats
- 📈 Visual progress bars with utilization percentages
- 🟢🟡🔴 Color-coded indicators based on thresholds (<50% green, 50-90% yellow, >90% red)
- ✓/⊘ Status icons for node health
- 📉 Sparkline trend indicators
- Grouped display (NotReady nodes shown first)
- Color-coded utilization cells

### JSON Format
Machine-readable JSON output perfect for automation, monitoring systems, and scripting.

```shell
kubectl glance -o json

# Example: Extract total CPU usage with jq
kubectl glance -o json | jq -r '.Totals.TotalUsageCPU'

# Example: Get all node stats
kubectl glance -o json | jq '.Nodes'

# Example: GPU allocation per model and accelerator type
kubectl glance -o json | jq '.Totals.GPUGroups'
```

**JSON Structure (simplified):**
```json
{
  "Nodes": {
    "node-1": {
      "Status": "Ready",
      "ProviderID": "aws:///us-west-2a/i-1234567890abcdef0",
      "Region": "us-west-2",
      "InstanceType": "m5.large",
      "NodeGroup": "eks-nodegroup-1",
      "CapacityType": "ON_DEMAND",
      "AllocatableCPU": "4",
      "AllocatableMemory": "8053040Ki",
      "AllocatedCPUrequests": "1.25",
      "AllocatedCPULimits": "2.0",
      "AllocatedMemoryRequests": "396Mi",
      "AllocatedMemoryLimits": "1024Mi",
      "UsageCPU": "0.186",
      "UsageMemory": "1172Mi"
    }
  },
  "Totals": {
    "TotalAllocatableCPU": "4",
    "TotalAllocatableMemory": "8053040Ki",
    "TotalUsageCPU": "0.186",
    "TotalUsageMemory": "1172Mi"
  }
}
```

### Dashboard Format
Terminal-based dashboard with visual elements and organized sections.

```shell
kubectl glance -o dash
```

### Chart Formats
Visual representations of resource utilization:

```shell
# Pie chart showing resource distribution
kubectl glance -o pie

# Bar charts and graphs
kubectl glance -o chart
```

## Filtering and Selection

Filter resources using Kubernetes label and field selectors:

### Label Selectors

```shell
# Filter nodes by environment label
kubectl glance --selector environment=production

# Multiple labels (AND logic)
kubectl glance --selector app=nginx,tier=frontend

# Short form
kubectl glance -l app=myapp
```

### Field Selectors

```shell
# Filter by specific field values
kubectl glance --field-selector metadata.name=node-1

# Field selectors apply to the listed resource: nodes for kubectl glance,
# pods for kubectl glance pods
kubectl glance pods --field-selector status.phase=Running,spec.nodeName=node-1
```

### Combined Filtering

```shell
# Combine label and field selectors
kubectl glance pods --selector app=nginx --field-selector status.phase=Running -o pretty
```

## Cloud Provider Integration

Glance can fetch additional metadata from cloud providers (AWS and GCP) to enrich node information. **Cloud provider columns are hidden by default** and are shown only when explicitly enabled via flag or configuration.

### Cloud Info Display

```shell
# Default: cloud info columns are hidden
kubectl glance
# Show cloud info columns
kubectl glance --show-cloud-provider=true
# Explicitly hide cloud info columns (overriding config)
kubectl glance --show-cloud-provider=false
```

**What You Get (as columns/fields):**
- **Common** (AWS & GCP):
  - Provider (AWS/GCE)
  - Region (from standard topology labels where available)
  - Instance type (e.g., `m5.large`, `n2-standard-4`)
  - Capacity type (e.g., `ON_DEMAND`, `SPOT`, `FARGATE`, `STANDARD`)
- **AWS-specific**:
  - Node group name (EKS)
  - Fargate profile (where applicable)
- **GCP-specific**:
  - Node pool name (GKE)

### Requirements

Cloud provider credentials must be configured:

**AWS:**
- AWS credentials in `~/.aws/credentials` or environment variables
- IAM permissions: `ec2:DescribeInstances`

**GCP:**
- Application Default Credentials or service account
- Permissions: `compute.instances.get`

**Behavior:**
- Glance hides cloud columns by default (no cloud API calls made)
- Use `--show-cloud-provider=true` to show columns, or `--show-cloud-provider=false` to force them off

### Example with Cloud Info

```shell
# View with cloud provider columns (if detected)
kubectl glance -o pretty
# Force cloud columns ON
kubectl glance --show-cloud-provider=true -o pretty
# Inspect cloud-related fields from JSON
kubectl glance -o json \
  | jq '.Nodes[] | {ProviderID, Region, InstanceType, CapacityType, NodeGroup, NodePool, FargateProfile}'
```

### CLI Flags Reference

### Static View Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `pretty` | Output format: `txt`, `pretty`, `json`, `dash`, `pie`, `chart` |
| `--selector` | `-l` | | Label selector (e.g., `app=nginx`) |
| `--field-selector` | | | Field selector (e.g., `metadata.name=node-1`) |
| `--show-cloud-provider` | `-c` | `false` | Show cloud provider columns (AWS/GCP instance type, region, capacity type) |
| `--raw` | | `false` | Show raw Kubernetes resource values (e.g., `1500m`, `2048Mi`) |
| `--exact` | | `false` | Alias for `--raw` |
| `--show-node-version` | | *unset* (shown) | VERSION column; set `false` to hide it |
| `--show-node-age` | | `false` | Show AGE column (node creation time) |
| `--show-node-group` | | `false` | Show GROUP column (cloud node group/pool, where available) |
| `--show-gpu` | | `false` | Show GPU columns (auto-enabled when GPU nodes are detected) |
| `--gpu-resources` | | | Extra extended resource names or glob patterns to count as GPUs/accelerators (e.g. `example.com/npu,vendor.io/*`) |
| `--config` | | `~/.glance/config` | Path to the glance config file |

Standard kubectl flags are also accepted: `--context`, `--kubeconfig`,
`-n/--namespace`, `--cluster`, `--user`, `--request-timeout`, and so on.

**Static subcommands** (`kubectl glance pods`, `kubectl glance deployments`,
`kubectl glance gpus`) accept the same flags. `pods` and `deployments` are scoped
to `-n/--namespace` (default: your kubeconfig context's namespace); `gpus` covers
all namespaces and applies the selectors to nodes.

### Live View Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--refresh` | `-r` | `2` | Refresh interval in seconds |
| `--namespace` | `-n` | context namespace | Initial namespace for the Pods/Deployments views (standard kubectl flag) |
| `--node-limit` | | `20` | Maximum number of nodes to display (0 = unlimited) |
| `--pod-limit` | | `100` | Maximum number of pods to display (0 = unlimited) |
| `--sort-by` | | `status` | Sort mode: `status`, `name`, `cpu`, `memory` |
| `--max-concurrent` | | `50` | Maximum concurrent API requests for parallel fetching |

**Notes:**
- `--node-limit` and `--pod-limit` are useful for large clusters (>100 nodes) to improve performance
- Sort mode can be changed dynamically in live view using keys `1`–`4`
- Namespace can be changed interactively using Left/Right arrow keys

## Configuration

### Environment Variables

Glance respects standard Kubernetes environment variables:

```shell
# Use specific kubeconfig
export KUBECONFIG=~/.kube/config-production
kubectl glance

# Set log level (trace, debug, info, warn, error, fatal)
# When set to debug or info, logs are written to ~/.glance/<level>-glance.log
export GLANCE_LOG_LEVEL=debug
kubectl glance
```

### Config File

Create `~/.glance/config` for persistent configuration (YAML format). This file is loaded automatically unless you point at another file with `--config <path>`. If it does not exist, Glance will create it on first write (for example, when you toggle live view settings).

```yaml
selector: "environment=production"
output: pretty
# show-cloud-provider defaults to false when omitted
# set explicitly to true/false to control behavior
show-cloud-provider: true
show-raw: false  # raw Kubernetes values (same as --raw/--exact)
log-level: warn  # trace, debug, info, warn, error, fatal

# Cloud info caching (live mode only)
cloud-cache-ttl: 5m      # TTL for cloud provider info cache (default: 5m)
cloud-cache-disk: false  # Persist cache to ~/.glance/cloud-cache.json (default: false)

# Column visibility (live mode and static view)
# For live mode, these are persisted when toggled with w/v/a/g keys.
# For static view (top-level kubectl glance), these control which
# columns are rendered when set explicitly. Flags override config.
#   - show-node-version: when unset, VERSION is shown by default; when
#     set to false (or passed as --show-node-version=false), the
#     VERSION column is hidden in static output.
#   - show-node-age: when true (or --show-node-age), add AGE column.
#   - show-node-group: when true (or --show-node-group), add GROUP column.
show-node-version: false
show-node-age: false
show-node-group: false
show-gpu: false           # auto-enabled when GPU nodes detected
gpu-resources: []         # extra accelerator resources/globs, e.g. ["example.com/npu"]
```

**Cloud Cache Settings:**
- `cloud-cache-ttl`: Duration to cache AWS/GCP instance type info. Valid units: s, m, h. Example: `300s`, `5m`, `1h`
- `cloud-cache-disk`: When `true`, cache persists between sessions in `~/.glance/cloud-cache.json`

**Column Visibility:**
- Initial state loaded from config file
- Changes made with `w` (cloud), `v` (version), `a` (age) keys are automatically saved
- Requires config file to exist for persistence

### Logging

By default, glance uses `warn` level logging which minimizes terminal output. For debugging:

- Set `GLANCE_LOG_LEVEL=debug` environment variable, or
- Add `log-level: debug` to your `~/.glance/config` config file

Log files are written to:
- `~/.glance/<level>-glance.log` (preferred), or
- `/tmp/<level>-glance.log` (fallback)

Log files are only created for `trace`, `debug`, or `info` levels.

## Examples

### Common Workflows

```shell
# Quick cluster overview
kubectl glance -o pretty

# Monitor production namespace in real-time
kubectl glance live
# Press 'p' for pods, then navigate to production namespace

# Check specific application resources
kubectl glance pods --selector app=nginx -o pretty

# Export cluster state for reporting
kubectl glance -o json > cluster-report-$(date +%Y%m%d).json


# Monitor cluster during deployment
kubectl glance live -r 1  # 1-second refresh

# Check nodes with cloud info
kubectl glance -c --field-selector metadata.name=node-1

# Get raw values for billing/capacity planning
kubectl glance --raw -o json > capacity-report.json

# Find idle or fragmented GPU capacity
kubectl glance gpus
```

### Automation Examples

```bash
#!/bin/bash
# Alert if cluster CPU usage exceeds 80% of allocatable
# (--raw prints Kubernetes quantities such as "4", "1500m", or "109600u")

pct=$(kubectl glance --raw -o json | jq -r '
  def cores:
    if endswith("n") then (.[:-1] | tonumber) / 1e9
    elif endswith("u") then (.[:-1] | tonumber) / 1e6
    elif endswith("m") then (.[:-1] | tonumber) / 1e3
    else tonumber end;
  (.Totals.TotalUsageCPU | cores) / (.Totals.TotalAllocatableCPU | cores) * 100 | floor')
if [ "$pct" -gt 80 ]; then
  echo "ALERT: cluster CPU usage at ${pct}%"
fi
```

```bash
#!/bin/bash
# Daily capacity report

kubectl glance --raw -o json | jq '{
  date: (now | strftime("%Y-%m-%d")),
  totalNodes: (.Nodes | length),
  totalCPU: .Totals.TotalAllocatableCPU,
  usedCPU: .Totals.TotalUsageCPU,
  totalMemory: .Totals.TotalAllocatableMemory,
  usedMemory: .Totals.TotalUsageMemory,
  gpus: .Totals.GPUGroups
}' > capacity-$(date +%Y%m%d).json
```

## Requirements

### Cluster Requirements

- **Kubernetes**: built with client-go v0.37 (Kubernetes 1.37 libraries). Glance only reads core
  APIs (nodes, pods, deployments, metrics), so older clusters generally work; development and
  testing currently use k3s 1.24.
- **Metrics Server**: required by the node view (`kubectl glance`) and for usage columns in live mode.
  `kubectl glance gpus`, `pods`, and `deployments` work without it (usage shows as 0).
  - Install: `kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml`
  - Or see the Kubernetes [metrics-server project] or your cloud provider's documentation for managed metrics add-ons

### Client Requirements

- **kubectl**: 1.12 or higher
- **Go**: 1.26 or higher (for building from source)
- **Terminal**: Supports ANSI colors (for pretty output and live view)

### Permissions

Required RBAC permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: glance-viewer
rules:
- apiGroups: [""]
  resources: ["nodes", "pods", "namespaces"]
  verbs: ["get", "list"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["nodes", "pods"]
  verbs: ["get", "list"]
```

## Performance and Scaling

Glance is optimized for large Kubernetes clusters with advanced performance features:

### Optimizations

- **Parallel API Fetching** - Uses Go errgroup for concurrent node, pod, and namespace queries
- **Watch Cache** - Leverages Kubernetes API server's watch cache (`resourceVersion="0"`) to reduce etcd load
- **Batch Operations** - Single pod list call with in-memory grouping eliminates N+1 query problem
- **Smart Limits** - Configurable node and pod limits prevent display overload in large clusters

### Performance Characteristics

| Cluster Size | Startup Time | Recommendations |
|--------------|--------------|-----------------|
| < 20 nodes | ~1-2 seconds | Default settings work great |
| 20-100 nodes | ~2-4 seconds | Use default `--node-limit=20` for live view |
| 100-500 nodes | ~5-10 seconds | Increase `--node-limit` as needed, use `--sort-by` strategically |
| 500+ nodes | ~10-20 seconds | Use a higher `--max-concurrent` and keep `--node-limit` modest |

### Large Cluster Detection

Glance automatically detects large clusters (>100 nodes) and warns about performance considerations:

```shell
# For clusters with 100+ nodes, glance logs a warning:
WARN Large cluster detected (150 nodes). Showing at most 20 nodes (--node-limit);
raise it with --node-limit or in the settings panel ([?]).
```

### Tuning for Large Clusters

```shell
# Increase display limits for larger terminals
kubectl glance live --node-limit=50 --pod-limit=200

# Increase API concurrency for faster fetching
kubectl glance live --max-concurrent=100

# Sort by specific criteria to focus on problem areas
kubectl glance live --sort-by=cpu  # Show highest CPU usage first
```

In live view, the summary/status bars will show how many nodes/pods are being displayed
(e.g., "Viewing Nodes: 20/150") so it’s clear when limits are applied on large clusters.

## Development

### Building

```shell
# Build binary
make build

# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# Check everything
make check
```

### Project Structure

```
glance/
├── cmd/                 # Main application entry point
├── pkg/
│   ├── cmd/            # CLI wiring and views
│   │   ├── glance.go   # Root command and static node view
│   │   ├── pods.go     # Static pods and deployments commands
│   │   ├── gpus.go     # Static gpus command and GPU report rendering
│   │   ├── live.go     # Live TUI implementation
│   │   ├── stats.go    # Shared pod/deployment aggregation (static + live)
│   │   ├── render.go   # Output formatting
│   │   ├── watch.go    # Informer-based cache (not yet wired in, see #41)
│   │   └── types.go    # Thin aliases over core types, shared constants
│   ├── core/           # Core domain types and aggregation (UI-agnostic)
│   │   ├── types.go    # NodeStats, Totals, Snapshot, etc.
│   │   ├── aggregate_nodes.go  # ComputeNodeSnapshot and helpers
│   │   ├── accelerators.go     # GPU/accelerator detection, pod requests, GPU models
│   │   └── gpu_report.go       # BuildGPUReport (pools, pending pods, findings)
│   ├── cloud/          # Cloud provider integration + caching
│   │   ├── aws.go      # AWS metadata provider
│   │   ├── gce.go      # GCP metadata provider
│   │   ├── cache.go    # Cloud metadata cache with TTL/disk support
│   │   ├── provider.go # Provider registry and lookup
│   │   └── types.go    # Provider-agnostic Metadata type
│   └── util/           # Utility functions (logging, helpers)
├── plugins/krew/       # Krew plugin manifest
└── version/            # Version information
```

### Testing

```shell
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific test
go test ./pkg/cmd -run TestLive
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `make check`
6. Submit a pull request

### Releasing

Releases are cut from `main` by pushing a semver tag; GitLab CI does the rest.
`AGENTS.md` has the full checklist.

#### Before tagging

1. Make sure `CHANGELOG.md` and `README.md` are up to date for everything in
   the release:
   - Move `[Unreleased]` entries into a new `## [X.Y.Z] - YYYY-MM-DD` section
     and leave an empty `[Unreleased]` section at the top.
   - Check the README sections the release touches: commands, the flags
     reference, keyboard shortcuts, config example, and requirements.
2. Update `version/version.go` to `X.Y.Z`.
3. Run `make check`.

#### Tag and push

```shell
git commit -am "chore: bump version to X.Y.Z"
git push origin main
git tag vX.Y.Z && git push origin vX.Y.Z
```

The CI pipeline then:
1. Runs lint and tests
2. Builds binaries for all platforms (darwin/linux amd64/arm64, windows amd64)
3. Creates `.tar.gz` archives with SHA256 checksums
4. Creates a GitLab Release with downloadable artifacts
5. Updates `plugins/krew/glance.yaml` on `main` with the new version and checksums
6. Mirrors the tag and `main` to GitHub, where the krew-release-bot workflow
   waits for the manifest and opens a PR in
   [kubernetes-sigs/krew-index](https://github.com/kubernetes-sigs/krew-index)

If the krew bot times out, re-run the `krew-release-bot` workflow on GitHub
with the release tag selected as the ref.

#### Release Artifacts

| Platform | Archive |
|----------|---------|
| macOS Intel | `kubectl-glance-VERSION-darwin-amd64.tar.gz` |
| macOS Apple Silicon | `kubectl-glance-VERSION-darwin-arm64.tar.gz` |
| Linux x86_64 | `kubectl-glance-VERSION-linux-amd64.tar.gz` |
| Linux ARM64 | `kubectl-glance-VERSION-linux-arm64.tar.gz` |
| Windows x86_64 | `kubectl-glance-VERSION-windows-amd64.tar.gz` |

#### Makefile Targets

| Target | Description |
|--------|-------------|
| `make build-all` | Build binaries for all platforms |
| `make archive-all` | Create archives (depends on build-all) |
| `make checksums` | Generate SHA256 checksums (depends on archive-all) |
| `make krew-plugin` | Update krew manifest with checksums (depends on checksums) |
| `make release` | Local release build (build → archive → checksum → manifest); CI does this on tags |
| `make krew-validate` | Test krew manifest locally |
| `make krew-reset` | Reset krew manifest to git version |
| `make clean` | Remove build artifacts |

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Support

- **Issues**: https://gitlab.com/davidxarnold/glance/-/issues
- **Repository**: https://gitlab.com/davidxarnold/glance

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [component-helpers](https://github.com/kubernetes/component-helpers) - Scheduler-accurate pod resource requests
- [termui](https://github.com/gizak/termui) - Terminal UI components
- [go-pretty](https://github.com/jedib0t/go-pretty) - Table rendering
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes API client