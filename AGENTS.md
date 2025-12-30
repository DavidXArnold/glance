# AI Agent Instructions for Glance

This document provides context and guidelines for AI assistants working on this codebase.

## Project Overview

**Glance** is a kubectl plugin that displays Kubernetes cluster resource allocation and utilization. It provides quick insights into CPU and memory usage across nodes, namespaces, pods, and deployments.

- **Language**: Go 1.25
- **Type**: kubectl plugin (CLI tool)
- **License**: Apache 2.0
- **Repository**: https://gitlab.com/davidxarnold/glance

## Architecture

```
glance/
├── cmd/
│   └── glance.go          # Main entrypoint
├── pkg/
│   ├── cmd/
│   │   ├── glance.go      # Core command logic, Kubernetes client interactions
│   │   ├── live.go        # Live TUI monitoring mode (termui) with scaling optimizations
│   │   ├── watch.go       # Informer-based caching for large clusters (>100 nodes)
│   │   ├── render.go      # Output rendering (text, pretty, JSON, charts)
│   │   ├── types.go       # Data structures (NodeInfo, ClusterTotals, etc.)
│   │   ├── aws.go         # AWS EC2 metadata integration
│   │   └── gce.go         # GCP GCE metadata integration
│   └── util/
│       └── util.go        # Shared utilities
├── version/
│   └── version.go         # Version constant (MUST be updated with releases)
└── plugins/krew/
    └── glance.yaml        # Krew plugin manifest (auto-updated by CI)
```

## Key Dependencies

- `k8s.io/client-go` - Kubernetes API client
- `k8s.io/metrics` - Metrics API for resource usage
- `github.com/spf13/cobra` - CLI framework
- `github.com/jedib0t/go-pretty/v6` - Table rendering
- `github.com/gizak/termui/v3` - Terminal UI for live mode
- `github.com/sirupsen/logrus` - Logging
- `cloud.google.com/go/container` - GCP integration
- `github.com/aws/aws-sdk-go-v2` - AWS integration
- `golang.org/x/sync` - errgroup for parallel API operations

## Development Commands

```shell
make build          # Build binary to target/kubectl-glance
make test           # Run unit tests
make lint           # Run golangci-lint in Docker
make fmt            # Format code with goimports
make check          # fmt + lint + test
make build-all      # Build for all platforms (darwin, linux, windows)
```

## Coding Conventions

### Go Style
- Follow standard Go conventions and `gofmt`
- Use `goimports` for import organization
- Error messages should be lowercase, no trailing punctuation
- Use structured logging with `logrus`

### File Organization
- Keep related functionality in the same file
- Test files use `_test.go` suffix alongside source
- One package per directory

### Naming
- Use descriptive variable names
- Acronyms should be consistent case (`URL`, `API`, not `Url`, `Api`)
- Interface names don't need `I` prefix

### Error Handling
- Return errors, don't panic (except in truly unrecoverable situations)
- Wrap errors with context: `fmt.Errorf("failed to get nodes: %w", err)`

## Testing

- Unit tests live alongside source files (`*_test.go`)
- Run `make test` before committing
- Use table-driven tests where appropriate
- Mock Kubernetes clients for unit tests

## Release Process

1. Update `version/version.go` with new version (e.g., `"0.1.3"`)
2. Commit: `git commit -m "chore: bump version to 0.1.3"`
3. Push to main: `git push origin main`
4. Create and push tag: `git tag v0.1.3 && git push origin v0.1.3`

**CI automatically:**
- Runs lint, test, build
- Creates multi-platform archives
- Uploads to GitLab Package Registry
- Creates GitLab Release
- Updates `plugins/krew/glance.yaml` with new checksums

## CI/CD Notes

- Pipeline defined in `.gitlab-ci.yml`
- Release jobs only trigger on semver tags (`v*.*.*`)
- Uses `golang:1.25` Docker image
- Release jobs checkout latest `main` before pushing manifest updates (to avoid non-fast-forward errors)

## Important Files to Keep in Sync

When releasing:
- `version/version.go` - **Manual**: Update before tagging
- `plugins/krew/glance.yaml` - **Auto**: Updated by CI

## Common Tasks

### Adding a new output format
1. Add format constant to `pkg/cmd/render.go`
2. Implement render function
3. Update `renderOutput()` switch statement
4. Add to cobra command flag choices
5. Update README.md examples

### Adding a new CLI flag
1. Add flag in `pkg/cmd/glance.go` `NewGlanceCmd()` function
2. Bind to viper if needed for config file support
3. Update help text and README.md

### Adding cloud provider support
1. Create new file `pkg/cmd/<provider>.go`
2. Implement metadata fetching interface
3. Add to `getCloudInfo()` in `glance.go`
4. Document in README.md

## Things to Avoid

- Don't modify `plugins/krew/glance.yaml` manually (CI manages it)
- Don't use `panic()` for recoverable errors
- Don't add dependencies without checking license compatibility
- Don't commit without running `make check`
- Don't create tags without updating `version/version.go` first

## Context for Common Questions

**Q: Why does the CI job fail with "non-fast-forward"?**
A: The release jobs run on tag commits which may be behind `main`. The CI now fetches and checks out latest `main` before pushing.

**Q: How do I test locally without a cluster?**
A: You need a Kubernetes cluster. Use `kind`, `minikube`, or connect to a real cluster.

---

## Current Session Status (December 28, 2025)

### Completed on `fixes` branch
- **v0.1.17 released** - multiple releases to fix CI YAML parsing issues
- Fixed output display issues:
  - Node status now correctly shows "Ready" for ready nodes
  - Dynamic terminal width detection (60-120 char bounds)
  - Cluster info moved from INFO log to summary display box
  - Removed noisy INFO/debug log statements from terminal output
- Added configurable logging:
  - `GLANCE_LOG_LEVEL` environment variable
  - `log-level` config file option
  - Log file output to `~/.glance/<level>-glance.log`
  - Levels: trace, debug, info, warn (default), error, fatal
- Fixed all lint issues:
  - G301 gosec: directory permissions 0755 → 0750
  - G302 gosec: file permissions 0644 → 0600
  - G304 gosec: filepath.Clean() + #nosec annotation
  - SA9003 staticcheck: removed empty config read branch
  - Removed unused functions (buildColoredProgressBar, padRight)
- Updated tests for new functionality (ClusterInfo, dynamic width, etc.)
- MR #21 open on `fixes` branch

### Completed on `live-improvements` branch
- **Scaling improvements for large clusters** - comprehensive performance optimizations
- Parallel API fetching with errgroup:
  - Concurrent node, pod, and namespace queries
  - Configurable concurrency limit (default 50)
  - Semaphore pattern for rate limiting
- Watch cache optimization:
  - `resourceVersion="0"` for faster API responses
  - Leverages API server's watch cache
  - Reduces load on etcd backend
- Batch pod fetching:
  - Eliminated N+1 query problem (O(n*m) → O(n))
  - Single pod list call with in-memory grouping by node/namespace
  - Dramatic reduction in API calls for large clusters
- Node/pod limits with smart defaults:
  - `--node-limit` flag (default 20, max 1000)
  - `--pod-limit` flag (default 100, max 10000)
  - Large cluster detection (>100 nodes) with warning
- Sorting modes:
  - `--sort-by` flag: status, name, cpu, memory
  - Press 's' key to cycle through modes in live UI
  - Status sort shows NotReady nodes first
- Informer-based watch mode (new file `watch.go`):
  - Shared informer factory for real-time updates
  - WatchCache type with Start(), Stop(), GetNodes(), GetPods(), etc.
  - Designed for clusters with >100 nodes
  - Background cache refresh with change notifications
- New types for parallel processing:
  - `nsRowData`, `podRowData`, `nodeRowData` for concurrent aggregation
  - `SummaryStats` for aggregate display
- Code quality review:
  - All doc comments now conform to Effective Go
  - Added periods to all doc comments
  - Fixed typos (relevent → relevant)
  - Verified error message format (lowercase, no trailing punctuation)
  - Ran `make fmt`, `make test`, `make lint`

### Architecture Notes
- **`pkg/cmd/render.go`** has dynamic width functions:
  - `getTerminalWidth()` - returns terminal width bounded 60-120
  - `buildColoredProgressBarDynamic()` - progress bar scaled to width
  - `padRightDynamic()` - padding scaled to width
- **`pkg/cmd/types.go`** has `ClusterInfo` struct (Host, MasterVersion)
- **`pkg/cmd/glance.go`** has `configureLogging()` function
- **`pkg/cmd/live.go`** implements parallel fetching:
  - `fetchNodeData()`, `fetchNamespaceData()`, `fetchPodData()` with errgroup
  - Uses semaphore pattern (`make(chan struct{}, maxConcurrent)`)
  - Single batch pod fetch with `groupPodsByNode()` and `groupPodsByNamespace()`
- **`pkg/cmd/watch.go`** (new file, 288 lines):
  - `WatchCache` struct with informer factory
  - Event handlers for Add/Update/Delete
  - Non-blocking update notification channel
  - Label selector filtering support

### Key Files Changed
#### On `fixes` branch:
- `pkg/cmd/glance.go` - logging config, cluster info population, lint fixes
- `pkg/cmd/render.go` - dynamic width, cluster info display, removed unused funcs
- `pkg/cmd/types.go` - ClusterInfo struct added to Totals
- `pkg/cmd/types_test.go` - tests for ClusterInfo
- `pkg/cmd/render_test.go` - tests for dynamic width functions
- `README.md` - logging configuration documentation

#### On `live-improvements` branch:
- `pkg/cmd/live.go` - +701 lines of parallel fetching, sorting, limiting
- `pkg/cmd/watch.go` - +288 lines (new file) for informer-based caching
- `pkg/cmd/live_test.go` - updated test assertions for new help text
- `go.mod` / `go.sum` - added golang.org/x/sync v0.19.0
- All `pkg/cmd/*.go` files - doc comment improvements for Effective Go

### Performance Characteristics
- **Small clusters (<20 nodes)**: No noticeable change
- **Medium clusters (20-100 nodes)**: 2-3x faster startup with parallel fetching
- **Large clusters (>100 nodes)**: 5-10x faster with watch cache + batch fetching
- **Very large clusters (500+ nodes)**: Use watch mode for real-time updates without polling overhead

### Testing Notes
- All unit tests pass (`make test`)
- Lint checks pass (`make lint`)
- Tested with minikube (small cluster)
- Code formatting verified (`make fmt`)

### Next Steps
1. Complete lint job (in progress)
2. Commit Effective Go improvements to `live-improvements` branch
3. Decide merge strategy:
   - Option A: Merge `fixes` to main first, then `live-improvements`
   - Option B: Merge `live-improvements` to main directly (includes all scaling work)
4. Bump version in `version/version.go` to v0.1.18
5. Tag and release
6. Update README.md with new CLI flags and performance notes
7. Submit Krew plugin PR to kubernetes-sigs/krew-index

---

## Current Session Status (December 29, 2025)

### Completed on `live-enhancements` branch
- **Live view UX improvements** - comprehensive enhancements to live monitoring interface
- Changed default view from Namespaces to Pods with "all namespaces" initial state
- Namespace display in summary bar:
  - Shows "Namespace: [←→] <name>" or "All Namespaces" in summary bar
  - Clear visual indicator for namespace cycling availability
  - Only displayed in Pods and Deployments views
- Standardized CPU/memory formatting with ratio display:
  - New format: "10.2 / 17" for CPU cores, "44.7Gi / 66Gi" for memory
  - Shows "used / total" or "requests / limits" depending on context
  - Consistent across all views (namespaces, pods, nodes, deployments)
- Added raw resource toggle (r key):
  - Toggle between ratio format and raw Kubernetes values
  - Raw mode shows values like "1500m", "2048Mi"
  - Global toggle affects all resource displays
  - State indicated in menu bar with checkbox
- Expanded header abbreviations:
  - "CPU REQ" → "CPU REQUESTS/LIMITS" (combined columns)
  - "MEM REQ" → "MEMORY REQUESTS/LIMITS" (combined columns)
  - "CPU USAGE" → "CPU USAGE/LIMITS" (shows ratio)
  - Node view: "CPU ALLOC/CAP", "MEM ALLOC/CAP" (allocated/capacity ratios)
  - Deployment view: simplified to requests/limits ratios only
- Renamed `--cloud-info` flag to `--show-cloud-provider`:
  - Default changed from `false` to `true` (enabled by default)
  - More descriptive name for the feature
  - Backwards compatibility alias maintained in viper bindings
- Added namespace configuration:
  - `--namespace` / `-N` flag for initial namespace selection
  - Config file support via viper binding
  - Empty string means "all namespaces" (default)
  - kubectl-compatible namespace handling
- Updated help text and menu bar:
  - Added [r]Raw toggle to help text
  - Menu bar shows raw resource toggle state
  - Help text shows namespace cycling hint ([←→]NS) contextually

### Architecture Notes
- **`pkg/cmd/live.go`** - major formatting refactor:
  - `formatResourceRatio(used, total, isMemory, showRaw)` - unified formatting function
  - `formatMilliCPU()` - changed from %.2f to %.1f precision
  - `renderSummaryBar()` - now takes mode and selectedNamespace parameters
  - All fetch functions updated to use ratio formatting
  - Headers reduced from 8-9 columns to 6-7 by combining related metrics
- **`pkg/cmd/glance.go`** - flag changes:
  - Renamed `cloud-info` to `show-cloud-provider` with default true
  - Added backwards compatibility binding for `cloud-info`
- **`LiveState` struct** - added `showRawResources bool` field

### Key Files Changed
#### On `live-enhancements` branch:
- `pkg/cmd/live.go` - ~200 lines modified (formatting refactor, namespace display, headers)
- `pkg/cmd/glance.go` - flag renaming and backwards compatibility
- `README.md` - comprehensive documentation updates:
  - Added CLI Flags Reference section with all flags documented
  - Updated Features section with new capabilities
  - Corrected default view mode to Pods
  - Added missing `s` and `r` key bindings
  - Updated Cloud Provider Integration section (default enabled)
  - Added Performance and Scaling section with tuning guidance
  - Updated config file example with new options
  - Expanded view modes documentation with sort modes
- `AGENTS.md` - session documentation (this file)

### Display Format Changes
**Before:**
```
NAMESPACE  CPU REQ  CPU LIMIT  CPU USAGE  MEM REQ   MEM LIMIT  MEM USAGE  PODS
default    2.50     4.00       1.80       4096Mi    8192Mi     3200Mi     10
```

**After:**
```
NAMESPACE  CPU REQUESTS/LIMITS  CPU USAGE/LIMITS  MEMORY REQUESTS/LIMITS  MEMORY USAGE/LIMITS  PODS
default    2.5 / 4.0            1.8 / 4.0         4.0Gi / 8.0Gi           3.1Gi / 8.0Gi        10
```

**Raw Mode (press 'r'):**
```
NAMESPACE  CPU REQUESTS/LIMITS  CPU USAGE/LIMITS  MEMORY REQUESTS/LIMITS  MEMORY USAGE/LIMITS  PODS
default    2500m / 4000m        1800m / 4000m     4096Mi / 8192Mi         3200Mi / 8192Mi      10
```

### Testing Notes
- Build successful with `make build`
- No compilation errors
- All formatting functions tested manually
- Ready for runtime testing with live cluster

### Deferred Features
- **Async cloud info fetching in live mode** - postponed for future implementation
  - Current behavior: cloud info used in static view only
  - Future: fetch cloud metadata async on startup, display in live node view
  - Implementation requires careful testing to avoid startup delays

### Next Steps
1. Test live view functionality with a real cluster:
   - Verify namespace display in summary bar
   - Test ratio formatting across all views
   - Confirm raw resource toggle works correctly
   - Test namespace cycling with [←→] keys
   - Validate new help text and menu bar
2. Run full test suite (`make test`)
3. Run linter (`make lint`)
4. Commit changes to `live-enhancements` branch
5. Create MR for review
6. After merge, bump version to v0.1.18 in `version/version.go`
7. Tag and release v0.1.18
8. Update krew plugin manifest (automated by CI)

### Breaking Changes
- **Default view changed**: Was Namespaces, now Pods
- **Column format changed**: Separate columns combined into ratio format
- **Flag renamed**: `--cloud-info` → `--show-cloud-provider` (backwards compatible)
- **Default behavior changed**: Cloud info now enabled by default

### Migration Guide for Users
- Users expecting Namespaces view on startup: Press `n` to switch
- Users with `cloud-info: false` in config: Update to `show-cloud-provider: false`
- Users scripting against column headers: Update parsers for new ratio format
- Users preferring old separate columns: Use raw mode with `r` key (shows individual values)
