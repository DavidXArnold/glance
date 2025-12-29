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
│   │   ├── live.go        # Live TUI monitoring mode (termui)
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

### Completed
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

### Architecture Notes
- `pkg/cmd/render.go` has dynamic width functions:
  - `getTerminalWidth()` - returns terminal width bounded 60-120
  - `buildColoredProgressBarDynamic()` - progress bar scaled to width
  - `padRightDynamic()` - padding scaled to width
- `pkg/cmd/types.go` has `ClusterInfo` struct (Host, MasterVersion)
- `pkg/cmd/glance.go` has `configureLogging()` function

### Key Files Changed
- `pkg/cmd/glance.go` - logging config, cluster info population, lint fixes
- `pkg/cmd/render.go` - dynamic width, cluster info display, removed unused funcs
- `pkg/cmd/types.go` - ClusterInfo struct added to Totals
- `pkg/cmd/types_test.go` - tests for ClusterInfo
- `pkg/cmd/render_test.go` - tests for dynamic width functions
- `README.md` - logging configuration documentation

### Next Steps
1. Merge MR #21 to main
2. Bump version in `version/version.go`
3. Tag and release (v0.1.18)
4. Submit Krew plugin PR to kubernetes-sigs/krew-index


