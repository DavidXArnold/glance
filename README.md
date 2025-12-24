# `glance` - kubectl plugin to view cluster resource allocation and usage

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.31-blue.svg)](https://kubernetes.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A kubectl plugin for viewing Kubernetes cluster resource allocation, utilization, and live monitoring. Provides quick insights into CPU and memory usage across nodes, namespaces, pods, and deployments.

## Features

- 📊 **Multiple Output Formats** - Text, Pretty tables, JSON, Dashboard, Pie charts, and more
- 🔄 **Live Monitoring** - Real-time TUI with auto-refresh for continuous observation
- 🎯 **Multiple View Modes** - Namespaces, Pods, Nodes, and Deployments
- 📈 **Resource Metrics** - CPU and memory requests, limits, and actual usage
- ☁️ **Cloud Provider Integration** - Optional AWS and GCP node metadata
- 🔍 **Flexible Filtering** - Label and field selectors for targeted views
- 📏 **Value Display Options** - Human-readable or exact values


## Table of Contents

- [Installation](#installation)
  - [Krew (Recommended)](#krew-kubectl-plugin-manager)
  - [Homebrew (macOS)](#macos)
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

### macOS

On macOS, install via [Homebrew](https://brew.sh):

```shell
brew install davidxarnold/glance/glance
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
# View cluster resources (static snapshot)
kubectl glance

# Start live monitoring with interactive TUI
kubectl glance live

# View with pretty colored output
kubectl glance -o pretty

# Monitor specific namespace pods in real-time
kubectl glance live  # then press 'p' and use ←→ to navigate namespaces
```



## Usage 

### Static View

**`kubectl glance`** provides a point-in-time snapshot of cluster resources, showing allocation and utilization across nodes.

```shell
kubectl glance
```

**Example Output:**
```
INFO[0000] There are 3 node(s) in the cluster            Host="https://k8s.example.com:6443"
 NODE NAME       STATUS  PROVIDERID  ALLOCATABLE  ALLOCATABLE  ALLOCATED  ALLOCATED  ALLOCATED  ALLOCATED  USAGE   USAGE     
                                     CPU          MEM          CPU REQ    CPU LIM    MEM REQ    MEM LIM    CPU     MEM       
 node-1          Ready   aws://...   4            8053040Ki    1.250      2.000      396Mi      1024Mi     0.186   1172Mi 
 node-2          Ready   aws://...   4            8053040Ki    2.100      3.500      512Mi      2048Mi     1.420   1856Mi
 node-3          Ready   aws://...   8            16106080Ki   3.750      6.000      1024Mi     4096Mi     2.340   3240Mi
 TOTALS                              16           32212160Ki   7.100      11.500     1932Mi     7168Mi     3.946   6268Mi
```

#### Output Formats

Choose from multiple output formats to suit your workflow:

| Format | Flag | Description |
|--------|------|-------------|
| **Text** | `txt` (default) | Simple ASCII table output |
| **Pretty** | `pretty` | Colorful table with styling and borders |
| **JSON** | `json` | Machine-readable JSON for scripting/automation |
| **Dashboard** | `dash` | Terminal dashboard with visual elements |
| **Pie Chart** | `pie` | Resource allocation pie chart visualization |
| **Chart** | `chart` | Resource usage charts and graphs |

**Examples:**
```shell
# Pretty colored output
kubectl glance -o pretty

# JSON output for automation
kubectl glance -o json | jq '.totals.totalUsageCPU'

# Visual dashboard
kubectl glance -o dash

# Pie chart visualization
kubectl glance -o pie
```

#### Static View Options

```shell
# Include cloud provider information (AWS/GCP node details)
kubectl glance -c
kubectl glance --cloud-info

# Display pod-level resource details
kubectl glance -p
kubectl glance --pods

# Show exact values instead of human-readable (e.g., 1000m vs 1)
kubectl glance --exact

# Combine options
kubectl glance -c -p -o pretty --exact
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
- 📊 Visual progress bars for resource utilization
- 🎨 Clean, organized table display
- 🎛️ Interactive toggles for display options
- 📱 Terminal responsive

#### View Modes

Switch between views using keyboard shortcuts:

| Key | View | Description |
|-----|------|-------------|
| **n** | Namespaces | Resource requests, limits, and usage per namespace (default) |
| **p** | Pods | Resource requests, limits, and usage per pod with namespace selection |
| **o** | Nodes | Node capacity, allocation, and current usage across cluster |
| **d** | Deployments | Deployment resources, replica counts, and availability status |

#### Keyboard Controls

| Key | Action |
|-----|--------|
| `n` | Switch to **Namespaces** view |
| `p` | Switch to **Pods** view |
| `o` | Switch to **Nodes** view |
| `d` | Switch to **Deployments** view |
| `b` | Toggle **progress bars** on/off |
| `%` | Toggle **percentages** on progress bars |
| `c` | Toggle **compact mode** (hide help text) |
| `←` | Previous namespace (in Pods/Deployments view) |
| `→` | Next namespace (in Pods/Deployments view) |
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

When progress bars are enabled, each resource metric displays a visual indicator:

```
NAMESPACE    CPU REQ  CPU LIMIT  CPU USAGE  MEM REQ  MEM LIMIT  MEM USAGE  PODS
production   2.5      4.0        1.8        4Gi      8Gi        3.2Gi      42
             ██████░░  63%
             ████████  100%
             ████░░░░  45%
             █████░░░  50%
             ████████  100%
             ████░░░░  40%

development  1.0      2.0        0.5        2Gi      4Gi        1.5Gi      15
             █████░░░  50%
             ████████  100%
             ██░░░░░░  25%
             █████░░░  50%
             ████████  100%
             ████░░░░  38%
```

The bars use Unicode block characters (█ for filled, ░ for empty) to provide instant visual feedback on resource utilization.
kubectl glance live --refresh 10

# Quick workflow: Start live view, press 'p' for pods, use ←→ to browse namespaces
kubectl glance live
# (press 'p', then ← or → to navigate)
```

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

### Text Format (default)
Basic ASCII table output, ideal for quick terminal views and scripting.

```shell
kubectl glance
kubectl glance -o txt
```

### Pretty Format
Colorful, styled tables with borders and highlighting for better readability in interactive terminal sessions.

```shell
kubectl glance -o pretty
```

### JSON Format
Machine-readable JSON output perfect for automation, monitoring systems, and scripting.

```shell
kubectl glance -o json

# Example: Extract total CPU usage with jq
kubectl glance -o json | jq -r '.totals.totalUsageCPU'

# Example: Get all node stats
kubectl glance -o json | jq '.nodeMap'
```

**JSON Structure:**
```json
{
  "nodeMap": {
    "node-1": {
      "nodeName": "node-1",
      "status": "Ready",
      "allocatableCPU": "4",
      "allocatableMemory": "8053040Ki",
      "totalAllocatedCPUrequests": "1.25",
      "totalAllocatedCPULimits": "2.0",
      "totalAllocatedMemoryRequests": "396Mi",
      "totalAllocatedMemoryLimits": "1024Mi",
      "usageCPU": "0.186",
      "usageMemory": "1172Mi"
    }
  },
  "totals": {
    "totalAllocatableCPU": "4",
    "totalAllocatableMemory": "8053040Ki",
    ...
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

Glance can fetch additional metadata from cloud providers (AWS and GCP) to enrich node information.

### Enable Cloud Info

```shell
kubectl glance -c
kubectl glance --cloud-info
```

**What You Get:**
- **AWS EC2**: Instance type, availability zone, region, instance ID
- **GCP GCE**: Machine type, zone, project ID, instance ID

### Requirements

Cloud provider credentials must be configured:

**AWS:**
- AWS credentials in `~/.aws/credentials` or environment variables
- IAM permissions: `ec2:DescribeInstances`

**GCP:**
- Application Default Credentials or service account
- Permissions: `compute.instances.get`

### Example with Cloud Info

```shell
# View with cloud provider details
kubectl glance -c -o pretty

# JSON output with cloud metadata
kubectl glance -c -o json | jq '.nodeMap[].cloudInfo'
```

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
```

### Config File

Create `~/.glance` for persistent configuration (YAML format):

```yaml
selector: "environment=production"
output: pretty
cloud-info: true
exact: false
```

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
- **Metrics Server**: Required for usage statistics and live view
  - Install: `kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml`

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
│   ├── cmd/            # Command implementations
│   │   ├── glance.go   # Main command and static view
│   │   ├── live.go     # Live TUI implementation
│   │   ├── render.go   # Output formatting
│   │   ├── types.go    # Data structures
│   │   ├── aws.go      # AWS integration
│   │   └── gce.go      # GCP integration
│   └── util/           # Utility functions
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