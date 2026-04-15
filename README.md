# gorefact

`gorefact` is a Go call-graph explorer with dependency rule checks and a Neovim UI for browsing callers, violations, and function details.

## Features

- `gorefact check` for batch dependency violation checks
- `gorefact serve` for a long-lived JSON-RPC server used by Neovim
- TOML deny rules
- text, json, markdown, and quickfix output formats
- Neovim search, tree, and detail buffers
- optional `--filter-pkg` scoping for large repositories

## Install

### Homebrew

```bash
brew tap flaticols/apps
brew install flaticols/apps/gorefact
```

### Go

Install the latest tagged module version:

```bash
go install go.flaticols.dev/gorefactor/cmd/gorefact@latest
```

From a local checkout, use:

```bash
go install ./cmd/gorefact
```

Both forms install `gorefact` into `GOBIN` or `$(go env GOPATH)/bin`.

### Release assets

Tagged releases also publish prebuilt archives and Linux packages:

- `tar.gz` and `zip` archives for macOS, Linux, and Windows
- `.deb`, `.rpm`, and `.apk` packages as release assets

The Linux packages are published as downloadable artifacts. They are not a full APT repository yet.

Required external tools:

- `gorefact` on `PATH`
- `go` on `PATH`

### Neovim 0.12 with `vim.pack`

Install from the remote repository:

```lua
local plug = vim.pack.add({
  { src = "https://github.com/flaticols/gorefactor", name = "gorefactor" },
})[1]

vim.opt.rtp:append(plug.path .. "/nvim")

require("gorefact").setup({
  binary = vim.fn.exepath("gorefact"),
  rules = "gorefact.rules.toml",
  patterns = { "./..." },
})
```

For local development from a checkout on disk:

```lua
local plug = vim.pack.add({
  { src = "/Users/flaticols/Developer/gorefactor", name = "gorefactor-dev" },
})[1]

vim.opt.rtp:append(plug.path .. "/nvim")

require("gorefact").setup({
  binary = vim.fn.exepath("gorefact"),
  dir = vim.fn.getcwd(),
  rules = "gorefact.rules.toml",
  patterns = { "./..." },
})
```

If you are iterating on a specific package area in a large monorepo, pass `filter_pkg`:

```lua
require("gorefact").setup({
  binary = vim.fn.exepath("gorefact"),
  rules = "gorefact.rules.toml",
  patterns = { "./..." },
  filter_pkg = "tasks",
})
```

`vim.pack.add()` installs and loads the Git repository, and `plug.path .. "/nvim"` adds the actual plugin runtime directory from this repo layout.

The plugin module name is:

```lua
require("gorefact")
```

Default config:

```lua
require("gorefact").setup({
  binary = vim.fn.exepath("gorefact"),
  dir = vim.fn.getcwd(),
  tests = false,
  filter_pkg = "",
  rules = "gorefact.rules.toml",
  patterns = { "./..." },
  server_args = {},
  keys = {
    explore = "<leader>Re",
    callers = "<leader>Rc",
    callees = "<leader>RC",
    check = "<leader>Rv",
  },
})
```

If you want in-editor help, run:

```vim
:helptags ALL
:help gorefact
```

## Rules

Example `gorefact.rules.toml`:

```toml
[[deny]]
from = "tasks"
to = "adapters"
reason = "tasks must not depend on adapters"

[[deny]]
from = "handler"
to = "repository"
reason = "handlers must go through service layer"
```

Validate rules without building the graph:

```bash
gorefact validate-rules --rules gorefact.rules.toml
```

## CLI

Check a repository:

```bash
gorefact check --rules gorefact.rules.toml ./...
```

Quickfix output for Neovim:

```bash
gorefact check --rules gorefact.rules.toml --format qf ./...
```

Scope loading to a package fragment:

```bash
gorefact check --rules gorefact.rules.toml --filter-pkg tasks ./...
```

Run the RPC server:

```bash
gorefact serve --rules gorefact.rules.toml ./...
```

Show the binary version:

```bash
gorefact version
```

`gorefact version` reads Go build metadata via `debug.ReadBuildInfo()`.

- `go install ...@version` builds usually report the module version
- release binaries built from a tagged checkout may fall back to the embedded VCS revision when the module version is unavailable

## Release

This repo ships with a release pipeline modeled on `flaticols/bump`:

- `.github/workflows/test-and-tag.yml` runs tests and builds on PRs and pushes to `main`
- `.github/workflows/release.yml` runs on pushed tags and invokes GoReleaser
- `.goreleaser.yaml` publishes release archives, Linux packages, and a Homebrew formula update to `flaticols/homebrew-apps`

To publish a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Required GitHub secret:

- `GORELEASER_GITHUB_TOKEN` with access to this repo and `flaticols/homebrew-apps`

## Neovim

Available commands:

- `:GorefactExplore`
- `:GorefactCallers`
- `:GorefactCallees`
- `:GorefactCheck`
- `:GorefactRestart`
- `:checkhealth gorefact`

Default keymaps:

- `<leader>Re` search
- `<leader>Rc` callers
- `<leader>RC` alternate tree grouping entrypoint
- `<leader>Rv` async check

Statusline helper:

```lua
require("gorefact").statusline()
```

## UI Preview

Search float:

```text
gorefact search
TaxEng

Results:
  github.com/acme/adapters.(*TaxEngine).Calculate  [12 callers]
  github.com/acme/adapters.*TaxEngine              [5 methods]
```

Tree and detail:

```text
gorefact-tree                    gorefact-detail
▼ Calculate                      Package: github.com/acme/tasks
  ⚠ github.com/acme/tasks.Run    Function: Run
  ✓ github.com/acme/service.Do   File: tasks/run.go:42
```

## Development

Run tests:

```bash
GOCACHE=/tmp/gocache go test ./...
```
