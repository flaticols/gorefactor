# gorefact

`gorefact` is a Go call-graph and dependency explorer. It shows you everything that imports or references a package, type, function, const, or var — and checks architectural rules defined in a TOML file.

## Features

- `gorefact inspect` — interactive TUI or structured output showing full reference trees for any package/symbol
- `gorefact check` — batch dependency violation check against TOML deny rules
- `gorefact serve` — long-lived JSON-RPC server for the Neovim plugin
- text, JSON, markdown, and quickfix output formats
- optional `--filter-pkg` scoping for large repositories
- Neovim plugin with search, tree, and detail buffers

## Install

### Nix

```bash
nix profile install github:flaticols/gorefactor
```

Or add to your flake inputs:

```nix
inputs.gorefactor.url = "github:flaticols/gorefactor";
```

### Homebrew

```bash
brew tap flaticols/apps
brew install flaticols/apps/gorefact
```

### Go

```bash
go install go.flaticols.dev/gorefactor/cmd/gorefact@latest
```

### Release archives

Tagged releases publish `tar.gz` archives for macOS (`amd64`, `arm64`) on the [releases page](https://github.com/flaticols/gorefactor/releases).

---

## inspect

`gorefact inspect` answers: *who imports this, and where exactly?*

```bash
# Open TUI (on a TTY)
gorefact inspect github.com/acme/tasks
gorefact inspect github.com/acme/tasks.Engine
gorefact inspect github.com/acme/tasks.Engine.Calc

# Short suffix — resolves to the first matching package
gorefact inspect tasks
gorefact inspect graph

# Structured output (auto-disables TUI)
gorefact inspect github.com/acme/tasks --format json | jq .
gorefact inspect github.com/acme/tasks --format text | less
gorefact inspect github.com/acme/tasks --format md
gorefact inspect github.com/acme/tasks --format qf

# Bare invocation on a TTY opens the TUI with an empty search box
gorefact
```

### TUI keybindings

The TUI uses vim-style modal navigation.

| Key | Action |
|-----|--------|
| `/` | Enter search mode — type a package path or suffix |
| `Enter` | Submit search |
| `Esc` | Exit search mode |
| `j` / `k` | Navigate down / up in active pane |
| `h` / `l` or `Tab` | Switch between Symbols and References panes |
| `g` | Toggle grouping: by package ↔ by file |
| `f` | Toggle violations-only filter |
| `q` | Quit |
| `ctrl+q` | Quit (from any mode) |

---

## check

```bash
gorefact check --rules gorefact.rules.toml ./...
gorefact check --rules gorefact.rules.toml --format json ./...
gorefact check --rules gorefact.rules.toml --format qf ./...
gorefact check --rules gorefact.rules.toml --filter-pkg tasks ./...
```

---

## Rules

`gorefact.rules.toml`:

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

Validate without loading packages:

```bash
gorefact validate-rules --rules gorefact.rules.toml
```

---

## Neovim plugin

### Install (Neovim 0.12+)

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

### Default config

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

### Commands

- `:GorefactExplore`
- `:GorefactCallers`
- `:GorefactCallees`
- `:GorefactCheck`
- `:GorefactRestart`
- `:checkhealth gorefact`

---

## serve

Starts the JSON-RPC server consumed by the Neovim plugin:

```bash
gorefact serve --rules gorefact.rules.toml ./...
```

---

## Release pipeline

- `.github/workflows/release.yml` triggers on pushed tags and runs GoReleaser
- GoReleaser publishes macOS archives, updates the Homebrew formula in `flaticols/homebrew-apps`, and updates `nix/gorefact.nix` in this repo
- Required GitHub secret: `GORELEASER_GITHUB_TOKEN` (access to this repo and `flaticols/homebrew-apps`)

To publish a release:

```bash
jj tag create v0.1.0
jj git push --tag v0.1.0
```

---

## Development

```bash
# Run tests
GOCACHE=/tmp/gocache go test ./...

# Build and install locally
go install ./cmd/gorefact

# Enter nix dev shell
nix develop
```
