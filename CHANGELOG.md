# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Cloud info caching with TTL (default: 5 minutes) for improved performance
- Disk-based cache persistence option (`cloud-cache-disk: true` in config)
- Column visibility persistence: toggle states saved to config file
- Config options: `cloud-cache-ttl`, `cloud-cache-disk`, `show-node-version`, `show-node-age`
- Cache stored in `~/.glance/cloud-cache.json` when disk persistence enabled
- New live view columns: version (`v`), age (`a`), and cloud info (`i`) toggles
- Dynamic limit controls: `+/-` keys to adjust node/pod limits, `l` to cycle presets (20/50/100/500/1000)
- Limit display in status bar and summary bar showing "X/Y nodes" or "X/Y pods"
- Async cloud info retrieval for non-blocking performance
- Progress bar improvements: only shown on resource columns, not metadata columns

### Fixed
- Panic when toggling cloud info with invalid provider IDs (index out of range)
- Progress bars incorrectly appearing on version, age, and cloud provider columns
- Cloud info now properly validates provider ID format before parsing
- Lint error in test: addProgressBars now requires baseColCount parameter

### Changed
- Default `maxConcurrent` increased from 50 to 100 for better performance on large clusters
- Cloud provider instance type lookups now cached with configurable TTL
- Column visibility changes automatically persisted to config file
- Refactored `fetchNodeData` function for reduced cyclomatic complexity (extracted helper functions)
- Improved code quality: replaced if-else chains with tagged switch statements for better performance

## [0.1.20] - 2025-12-29

### Fixed
- AWS API error handling to prevent panic with improved error unwrapping using `errors.As()`

## [0.1.19] - 2025-12-29

### Added
- Comprehensive live view enhancements
- Namespace navigation with up/down arrow keys and Enter to select in Namespaces view
- Standardized CPU/memory formatting to ratio display (e.g., "10.2 / 17" for cores, "44.7Gi / 66Gi" for memory)
- Raw resource toggle (`r` key) with 'Raw Data' label to show Kubernetes native values
- Expanded column headers: "CPU REQUESTS/LIMITS" format for better clarity
- `--namespace` / `-N` flag for initial namespace selection in live view
- Display current namespace in summary bar with `[←→]` toggle hints
- Glance logo added to README

### Changed
- **Default view changed from Pods to Nodes** for live mode
- `--cloud-info` flag renamed to `--show-cloud-provider` (default: `true`, backwards compatible)
- Column headers consolidated to ratio format for consistency

### Fixed
- Deployment view colors: removed misleading red coloring at 100% ready replicas by clearing row styles

## [0.1.18] - 2025-12-28

### Added
- Configurable logging with `GLANCE_LOG_LEVEL` environment variable
- `log-level` config file option supporting trace, debug, info, warn (default), error, fatal
- Log file output to `~/.glance/<level>-glance.log` for debug/info/trace levels

### Fixed
- Node status display: now correctly shows "Ready" for ready nodes (was showing raw status)
- Dynamic terminal width detection (60-120 char bounds) for better display scaling
- Cluster info moved from INFO logs to summary display box for cleaner output
- Removed noisy INFO/debug log statements from terminal output

### Changed
- Cluster metadata (host, version) now displayed in summary box instead of logs
- Default log level is `warn` for minimal terminal clutter

## [0.1.17] - 2025-12-28

### Fixed
- CI YAML parsing issues in GitLab pipeline configuration
- Multiple releases to stabilize CI automation

## [0.1.16] - 2025-12-27

### Added
- Live view performance optimizations for large clusters (>100 nodes)
- Parallel API fetching with errgroup pattern
- Watch cache optimization with `resourceVersion="0"` for faster API responses
- Batch pod fetching to eliminate N+1 query problem
- Node/pod limits with smart defaults (`--node-limit`, `--pod-limit`)
- Large cluster detection (>100 nodes) with warning and recommendations
- Sorting modes: `--sort-by` flag (status, name, cpu, memory)
- Press `s` key to cycle through sort modes in live UI
- Informer-based watch mode in `watch.go` for real-time updates (new file, 288 lines)
- New types for parallel processing: `nsRowData`, `podRowData`, `nodeRowData`
- `SummaryStats` type for aggregate display

### Changed
- Small clusters (<20 nodes): No noticeable performance change
- Medium clusters (20-100 nodes): 2-3x faster startup with parallel fetching
- Large clusters (>100 nodes): 5-10x faster with watch cache + batch fetching
- Very large clusters (500+ nodes): Use watch mode for real-time updates without polling overhead

## [0.1.15] and earlier

### Added
- Initial release with static and live monitoring modes
- Multiple output formats (text, pretty, JSON, dashboard, pie, chart)
- Cloud provider integration (AWS EC2, GCP Compute)
- Resource metrics from metrics-server
- Interactive TUI with keyboard navigation
- Progress bars and visual indicators
- Namespace, pod, node, and deployment views

---

## Release Process

**When creating a new release:**

1. Update `version/version.go` with new version number
2. Update this `CHANGELOG.md` file:
   - Move items from `[Unreleased]` to new version section
   - Add release date
   - Create new empty `[Unreleased]` section at top
3. Commit changes: `git commit -m "chore: bump version to X.Y.Z"`
4. Create and push tag: `git tag vX.Y.Z && git push origin main && git push origin vX.Y.Z`
5. CI automatically builds, creates release, and updates Krew manifest

**Version naming:**
- **Patch** (0.1.X): Bug fixes, minor improvements
- **Minor** (0.X.0): New features, non-breaking changes
- **Major** (X.0.0): Breaking changes, major refactors

**Changelog categories:**
- **Added**: New features
- **Changed**: Changes to existing functionality
- **Deprecated**: Soon-to-be removed features
- **Removed**: Removed features
- **Fixed**: Bug fixes
- **Security**: Security fixes
