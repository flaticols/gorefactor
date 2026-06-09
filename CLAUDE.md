# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build and install
go install ./cmd/gorefact

# Run all tests
GOCACHE=/tmp/gocache go test ./...

# Run tests for a specific package
go test ./internal/inspect/...

# Run a single test
go test ./internal/inspect/ -run TestName

# Lint (uses standard go vet)
go vet ./...
```

## Architecture

**gorefact** is a package-centric explorer for Go import and reference graphs. It opens an interactive TUI on a terminal, or prints a structured package report (text / JSON / markdown) for non-TTY use.

### Go backend (`cmd/`, `internal/`)

Data flows in one direction:

1. **`internal/graph`** — type definitions (`Func`, `Edge`, `Symbol`, `Graph`) shared by the loader, inspect, and TUI layers.
2. **`internal/loader`** — builds the package import graph (`pkggraph.go`: `BuildPackageGraph`, `PackageGraph.ImportersOf`, `AllPaths`) and walks source references (`refs.go`: `WalkRefs`); each edge records its enclosing `CallerPkg`/`CallerFunc`.
3. **`internal/inspect`** — resolves a target into a package report: `ResolveTarget` (public API with positions + reference edges), `ListPackages`, `LoadAllSymbols`, `LoadStructMembers`. Format functions `FormatText`, `FormatJSON`, `FormatMarkdown` render the report.
4. **`internal/tui`** — the BubbleTea three-pane explorer (package picker, public API, importers/usage) built on the inspect + loader data.
5. **`cmd/gorefact/main.go`** — CLI dispatcher. The bare command (with an optional target) opens the explorer on a TTY or prints a report otherwise. Subcommands: `pkg <list|get|imports|importers>` (scriptable graph queries, never the TUI), `version`, `help`.

### Release pipeline

GoReleaser + GitHub Actions. Tag with `vX.Y.Z` to trigger `.github/workflows/release.yml`, which publishes macOS archives and pushes a Homebrew formula update to `flaticols/homebrew-apps`.

## Version control

This repo uses `jj` (jujutsu). Use `jj` commands for all VCS operations — see global CLAUDE.md for the cheatsheet.
