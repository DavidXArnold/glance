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
7. Update the Homebrew formula

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