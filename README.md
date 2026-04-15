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

Install the binary first:

```bash
go install ./cmd/gorefact
```

This installs `gorefact` into `GOBIN` or `$(go env GOPATH)/bin`.

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
  rules = "rules.toml",
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
  rules = "rules.toml",
  patterns = { "./..." },
})
```

If you are iterating on a specific package area in a large monorepo, pass `filter_pkg`:

```lua
require("gorefact").setup({
  binary = vim.fn.exepath("gorefact"),
  rules = "rules.toml",
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
  rules = "rules.toml",
  patterns = { "./..." },
  server_args = {},
  keys = {
    explore = "<leader>ge",
    callers = "<leader>gc",
    callees = "<leader>gC",
    check = "<leader>gv",
  },
})
```

If you want in-editor help, run:

```vim
:helptags ALL
:help gorefact
```

## Rules

Example `rules.toml`:

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
gorefact validate-rules --rules rules.toml
```

## CLI

Check a repository:

```bash
gorefact check --rules rules.toml ./...
```

Quickfix output for Neovim:

```bash
gorefact check --rules rules.toml --format qf ./...
```

Scope loading to a package fragment:

```bash
gorefact check --rules rules.toml --filter-pkg tasks ./...
```

Run the RPC server:

```bash
gorefact serve --rules rules.toml ./...
```

Show the binary version:

```bash
gorefact version
```

## Neovim

Available commands:

- `:GorefactExplore`
- `:GorefactCallers`
- `:GorefactCallees`
- `:GorefactCheck`
- `:GorefactRestart`
- `:checkhealth gorefact`

Default keymaps:

- `<leader>ge` search
- `<leader>gc` callers
- `<leader>gC` alternate tree grouping entrypoint
- `<leader>gv` async check

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
