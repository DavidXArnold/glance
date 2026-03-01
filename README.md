<p align="center">
  <img src="assets/glance-logo.png" alt="Glance Logo">
</p>

# `glance` - kubectl plugin to view cluster resource allocation and usage

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.31-blue.svg)](https://kubernetes.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A kubectl plugin for viewing Kubernetes cluster resource allocation, utilization, and live monitoring. Provides quick insights into CPU and memory usage across nodes, namespaces, pods, and deployments.

> Note: Glance is developed on GitLab at https://gitlab.com/davidxarnold/glance and mirrored to GitHub at https://github.com/DavidXArnold/glance. Please open issues and feature requests on GitLab.

## Features

- 📊 **Multiple Output Formats** - Text, Pretty tables, JSON, Dashboard, Pie charts, and more
- 🔄 **Live Monitoring** - Real-time TUI with auto-refresh for continuous observation
- 🎯 **Multiple View Modes** - Nodes (default), Namespaces with navigation, Pods, and Deployments
- 📈 **Resource Metrics** - CPU and memory requests, limits, and actual usage with ratio formatting
- 🎮 **GPU Support** - NVIDIA, AMD, and other GPU resources with auto-detection and `--show-gpu` flag
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
  - [Live View](#live-view)
- [Output Formats](#output-formats)
- [Filtering and Selection](#filtering-and-selection)
- [Cloud Provider Integration](#cloud-provider-integration)
- [Configuration](#configuration)
- [Examples](#examples)
- [Requirements](#requirements)
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

Build from source with Go 1.25 or higher:

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
# Static pods view (all namespaces by default)
kubectl glance pods

# Static pods view for a specific namespace
kubectl glance pods -n default

# Static deployments view (all namespaces)
kubectl glance deployments

# Static deployments view for a specific namespace
kubectl glance deployments -n production
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

**Example Output (nodes):**
```
┌──────────────────────────────────────────────────────────────────────────────┐
│ CLUSTER: https://k8s.example.com:6443                                        │
│ CPU: ████████░░░░░░░░░░░░ 44.4% (7.1/16)    Nodes: 3 Ready                   │
│ MEM: ██████░░░░░░░░░░░░░░ 19.4% (6.1/31.4Gi)                                 │
└──────────────────────────────────────────────────────────────────────────────┘
 NODE NAME       STATUS  PROVIDERID  ALLOCATABLE  ALLOCATABLE  ALLOCATED  ALLOCATED  USAGE   USAGE     
                                     CPU          MEM          CPU REQ    CPU LIM    CPU     MEM       
 node-1          Ready   aws://...   4            8053040Ki    1.250      2.000      0.186   1172Mi 
 node-2          Ready   aws://...   4            8053040Ki    2.100      3.500      1.420   1856Mi
 node-3          Ready   aws://...   8            16106080Ki   3.750      6.000      2.340   3240Mi
 TOTALS                              16           32212160Ki   7.100      11.500     3.946   6268Mi
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

# JSON output for automation (Totals.TotalUsageCPU from snapshot)
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

# Display pod-level resource details
kubectl glance -p
kubectl glance --pods

# Show exact values instead of human-readable (e.g., 1000m vs 1)
kubectl glance --exact

# Control node columns in static output (top-level kubectl glance)
# VERSION is shown by default unless explicitly disabled.
# AGE and GROUP are only shown when enabled.
kubectl glance --show-node-age             # add AGE column
kubectl glance --show-node-group           # add GROUP column
kubectl glance --show-node-version=false   # hide VERSION column
kubectl glance --show-gpu                  # show GPU columns (auto-enabled when GPUs detected)

# Combine options
kubectl glance -c -p -o pretty --exact --show-node-age --show-node-group
```

### Live View

**`kubectl glance live`** provides a continuously updating TUI (Terminal User Interface) for real-time cluster monitoring, similar to `kubectl top` but with much richer information and multiple view modes.

```shell
kubectl glance live
```

**Features:**
- 🔄 Auto-refresh every 2 seconds (configurable)
- 🎯 Four different view modes
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

**Default View:** Nodes view shows cluster-wide node status on startup.

**Namespace Navigation:**
- In **Namespaces view**: Use ↑↓ arrows to select a namespace, press Enter to view pods in that namespace
- In **Pods/Deployments views**: Use ←→ arrows to cycle through namespaces
- Use `--namespace` or `-N` flag to start with a specific namespace

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

|| Key | Action |
||-----|--------|
|| `n` | Switch to **Namespaces** view |
|| `p` | Switch to **Pods** view |
|| `o` | Switch to **Nodes** view |
|| `d` | Switch to **Deployments** view |
|| `b` | Toggle **progress bars** on/off |
|| `%` | Toggle **percentages** on progress bars |
|| `r` | Toggle **raw data** display (e.g., "1500m" vs "1.5 / 2.0") |
|| `c` | Toggle **compact mode** (hide summary and help) |
|| `w` | Toggle **cloud info** columns (Provider, Region, Instance Type, Capacity) |
|| `v` | Toggle **node version** column (Kubelet version) |
|| `a` | Toggle **node age** column (time since creation) |
||| `g` | Toggle **node group/pool** column |
||| `u` | Toggle **GPU resource** columns (requests/allocatable) |
|| `1` | Sort by **Status** |
|| `2` | Sort by **Name** |
|| `3` | Sort by **CPU** |
|| `4` | Sort by **Memory** |
|| `?` | Open **settings modal** for advanced toggles |
|| `+/-` | Increase/decrease display **limits** (nodes or pods by 10) |
|| `↑↓` | Select namespace (in Namespaces view) |
|| `Enter` | View pods for selected namespace (in Namespaces view) |
|| `←→` | Navigate namespaces (in Pods/Deployments view) |
|| `q` | Quit live view |

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

#### Visual Example

The live view now includes a cluster summary dashboard and color-coded progress bars:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ CLUSTER SUMMARY                                                              │
│ CPU: 🟢 ████████░░░░░░░░ 45.2% (7.2/16 cores)  Nodes: ✓ 3 healthy          │
│ MEM: 🟡 ██████████████░░ 72.5% (23.2/32 Gi)    Pods: 42 running            │
└─────────────────────────────────────────────────────────────────────────────┘

NODE         STATUS      CPU CAP  CPU ALLOC  CPU USE  MEM CAP  MEM ALLOC  MEM USE  PODS
node-1       ✓ Ready     4        2.5        1.8      8Gi      4Gi        3.2Gi    15
             🟡 ████████████░░░░  45%
             🟡 ██████████████░░  63%
             🟢 ██████████░░░░░░  45%

node-2       ✓ Ready     4        3.0        2.1      8Gi      6Gi        5.8Gi    18
             🟡 ████████████████  75%
             🔴 ██████████████████ 95%
             🟡 ██████████████░░  73%
```

**Color Indicators:**
- 🟢 Green: < 50% utilization (healthy)
- 🟡 Yellow: 50-90% utilization (warning)
- 🔴 Red: > 90% utilization (critical)

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
kubectl glance -o json | jq -r '.totals.totalUsageCPU'

# Example: Get all node stats
kubectl glance -o json | jq '.nodeMap'
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

# Multiple field selectors
kubectl glance --field-selector status.phase=Running,spec.nodeName=node-1
```

### Combined Filtering

```shell
# Combine label and field selectors
kubectl glance --selector app=nginx --field-selector status.phase=Running -o pretty
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

|||| Flag | Short | Default | Description |
||||------|-------|---------|-------------|
|||| `--output` | `-o` | `pretty` | Output format: `txt`, `pretty`, `json`, `dash`, `pie`, `chart` |
|||| `--show-cloud-provider` | `-c` | `false` | Display cloud provider metadata (AWS/GCP instance types, regions) when set to true; off by default |
||| `--pods` | `-p` | `false` | Display pod-level resource details in static node view (root `kubectl glance`) |
||| `--exact` | | `false` | Show exact Kubernetes resource values instead of human-readable |
||| `--selector` | `-l` | | Label selector for filtering (e.g., `app=nginx`) |
||| `--field-selector` | | | Field selector for filtering (e.g., `status.phase=Running`) |
||| `--show-node-version` | | *unset* (treated as "show") | Control VERSION column in static output: when explicitly set to `false`, hides the VERSION column; when unset or `true`, VERSION is shown |
||| `--show-node-age` | | `false` | Show AGE column (node creation time) in static output |
|||| `--show-node-group` | | `false` | Show GROUP column (cloud node group/pool, where available) in static output |
|||| `--show-gpu` | | `false` | Show GPU resource columns (auto-enabled when GPU nodes are detected) |

**Static subcommands** (`kubectl glance pods`, `kubectl glance deployments`) reuse
these selectors and output flags, and additionally honor the global `--namespace`
flag from kubectl/genericclioptions.

### Live View Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--refresh` | `-r` | `2` | Refresh interval in seconds |
| `--namespace` | `-N` | | Initial namespace for pods/deployments view (empty = all namespaces) |
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

# Set default namespace
export KUBECTL_NAMESPACE=my-namespace
kubectl glance

# Set log level (trace, debug, info, warn, error, fatal)
# When set to debug or info, logs are written to ~/.glance/<level>-glance.log
export GLANCE_LOG_LEVEL=debug
kubectl glance
```

### Config File

Create `~/.glance/config` for persistent configuration (YAML format). This file is loaded automatically unless you override it with `--config`. If it does not exist, Glance will create it on first write (for example, when you toggle live view settings).

```yaml
selector: "environment=production"
output: pretty
# show-cloud-provider defaults to false when omitted
# set explicitly to true/false to control behavior
show-cloud-provider: true
exact: false
log-level: warn  # trace, debug, info, warn, error, fatal
namespace: ""    # Initial namespace for live view (empty = all namespaces)

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
kubectl glance --selector app=nginx -p -o pretty

# Export cluster state for reporting
kubectl glance -o json > cluster-report-$(date +%Y%m%d).json

# Compare resource requests vs usage
kubectl glance -o pretty | grep -E "ALLOCATED|USAGE"

# Monitor cluster during deployment
kubectl glance live -r 1  # 1-second refresh

# Check nodes with cloud info
kubectl glance -c --field-selector metadata.name=node-1

# Get exact values for billing/capacity planning
kubectl glance --exact -o json > capacity-report.json
```

### Automation Examples

```bash
#!/bin/bash
# Alert if CPU usage exceeds 80%

USAGE=$(kubectl glance -o json | jq -r '.totals.totalUsageCPU' | cut -d'.' -f1)
if [ "$USAGE" -gt 80 ]; then
  echo "ALERT: CPU usage at ${USAGE}%"
  # Send notification...
fi
```

```bash
#!/bin/bash
# Daily capacity report

kubectl glance --exact -o json | jq '{
  date: now | strftime("%Y-%m-%d"),
  totalNodes: .nodeMap | length,
  totalCPU: .totals.totalAllocatableCPU,
  usedCPU: .totals.totalUsageCPU,
  totalMemory: .totals.totalAllocatableMemory,
  usedMemory: .totals.totalUsageMemory
}' > capacity-$(date +%Y%m%d).json
```

## Requirements

### Cluster Requirements

- **Kubernetes**: 1.12 or higher (tested with 1.31)
- **Metrics Server**: **Required for glance to operate** (all modes)
  - Install: `kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml`
  - Or see the Kubernetes [metrics-server project] or your cloud provider's documentation for managed metrics add-ons

### Client Requirements

- **kubectl**: 1.12 or higher
- **Go**: 1.25 or higher (for building from source)
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
| 500+ nodes | ~10-20 seconds | Consider watch cache mode (future feature), use higher `--max-concurrent` |

### Large Cluster Detection

Glance automatically detects large clusters (>100 nodes) and warns about performance considerations:

```shell
# For clusters with 100+ nodes, glance shows a warning:
WARN Large cluster detected (150 nodes). Using --node-limit=20 for performance.
Consider using --watch mode for real-time updates with lower API load.
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
│   │   ├── glance.go   # Root command and static view
│   │   ├── live.go     # Live TUI implementation
│   │   ├── render.go   # Output formatting
│   │   └── types.go    # Thin aliases over core domain types
│   ├── core/           # Core domain types and aggregation (UI-agnostic)
│   │   ├── types.go    # NodeStats, Totals, Snapshot, etc.
│   │   └── aggregate_nodes.go  # ComputeNodeSnapshot and helpers
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

The project uses GitLab CI/CD to automate releases. Here's how to create a new release:

#### Prerequisites

- `GL_CI_TOKEN` CI/CD variable set in GitLab with write access to the repository
- Version updated in `version/version.go`

#### Manual Release (Local)

```shell
# 1. Update version in version/version.go
vim version/version.go

# 2. Build all platforms, create archives, generate checksums, update krew manifest
make release

# 3. Review the updated krew manifest
cat plugins/krew/glance.yaml

# 4. Commit and tag
git add -A && git commit -m "Release v$(make release_version)"
make tag-release
git push && git push --tags

# 5. Upload archives to GitLab release page
# Files are in: target/archives/
```

#### Automated Release (CI/CD)

Simply push a version tag to trigger the full release pipeline:

```shell
# 1. Update version in version/version.go and commit
vim version/version.go
git add -A && git commit -m "Prepare release v0.1.0"
git push

# 2. Create and push tag (triggers release)
git tag v0.1.0
git push --tags
```

The CI pipeline will automatically:
1. Run lint and tests
2. Build binaries for all platforms (darwin/linux amd64/arm64, windows amd64)
3. Create `.tar.gz` archives
4. Generate SHA256 checksums
5. Create a GitLab Release with downloadable artifacts
6. Update the krew manifest with new version and checksums

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
| `make release` | Full release pipeline (build → archive → checksum → manifest) |
| `make krew-validate` | Test krew manifest locally |
| `make krew-reset` | Reset krew manifest to git version |
| `make clean` | Remove build artifacts |

#### Submitting to Krew Index

After creating a release:

1. Fork [kubernetes-sigs/krew-index](https://github.com/kubernetes-sigs/krew-index)
2. Copy `plugins/krew/glance.yaml` to `plugins/glance.yaml` in your fork
3. Submit a PR to the krew-index repository
4. Wait for review and approval

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Support

- **Issues**: https://gitlab.com/davidxarnold/glance/-/issues
- **Repository**: https://gitlab.com/davidxarnold/glance

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [termui](https://github.com/gizak/termui) - Terminal UI components
- [go-pretty](https://github.com/jedib0t/go-pretty) - Table rendering
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes API client