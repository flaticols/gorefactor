# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build and install
go install ./cmd/gorefact

# Run all tests
GOCACHE=/tmp/gocache go test ./...

# Run tests for a specific package
go test ./internal/rpc/...

# Run a single test
go test ./internal/rpc/ -run TestName

# Lint (uses standard go vet)
go vet ./...
```

## Architecture

**gorefact** is a Go call-graph explorer that checks architectural dependency rules. It has two modes: a one-shot CLI (`check`) and a long-lived JSON-RPC server (`serve`) consumed by a Neovim plugin.

### Go backend (`cmd/`, `internal/`)

Data flows in one direction:

1. **`internal/graph`** — builds a static call graph using `golang.org/x/tools/go/callgraph`. `Graph` holds `[]Func` and `[]Edge`; `Index()` must be called before lookups.
2. **`internal/rules`** — parses TOML `[[deny]]` rules and walks edges to produce `[]Violation`. Format functions (`FormatText`, `FormatJSON`, `FormatMarkdown`, `FormatQuickfix`) live here.
3. **`internal/treeview`** — builds caller-tree nodes from the graph for the Neovim tree buffer.
4. **`internal/rpc`** — a newline-delimited JSON-RPC 2.0 server over stdin/stdout. Methods: `gorefact.search`, `gorefact.tree`, `gorefact.detail`, `gorefact.funcAtPos`, `gorefact.check`. The server holds the graph and rules in memory after startup; the graph is never reloaded without restart.
5. **`cmd/gorefact/main.go`** — CLI dispatcher with subcommands `check`, `serve`, `version`, `validate-rules`.

### Neovim plugin (`nvim/`)

Lua plugin under `nvim/lua/` + vimdoc under `nvim/doc/`. The plugin spawns `gorefact serve` as a subprocess and communicates via JSON-RPC over its stdin/stdout. It manages three buffer types: search float, tree split, and detail split.

### Release pipeline

GoReleaser + GitHub Actions. Tag with `vX.Y.Z` to trigger `.github/workflows/release.yml`, which publishes macOS archives and pushes a Homebrew formula update to `flaticols/homebrew-apps`.

## Version control

This repo uses `jj` (jujutsu). Use `jj` commands for all VCS operations — see global CLAUDE.md for the cheatsheet.
