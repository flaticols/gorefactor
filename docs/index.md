# gorefact

**Go call-graph explorer with dependency rule checks.**

`gorefact` shows you everything that imports or references a package, type,
function, const, or var — and checks architectural rules defined in a TOML
file.

## What it does

- **`gorefact inspect`** — interactive TUI or structured output showing full
  reference trees for any package or symbol.
- **`gorefact check`** — batch dependency violation check against TOML
  `[[deny]]` rules.
- **`gorefact serve`** — long-lived JSON-RPC server consumed by the Neovim
  plugin.
- Output formats: **text**, **JSON**, **markdown**, **quickfix**.
- Optional `--filter-pkg` scoping for large repositories.

## Install

### Homebrew

```bash
brew tap flaticols/apps
brew install flaticols/apps/gorefact
```

### Go

Install globally with `go install`:

```bash
go install go.flaticols.dev/gorefactor/cmd/gorefact@latest
```

Or pin it per-module as a Go tool dependency (Go 1.24+):

```bash
go get -tool go.flaticols.dev/gorefactor/cmd/gorefact@latest
go tool gorefact inspect ./...
```

### Nix

```bash
nix profile install github:flaticols/gorefactor
```

Or add it as a flake input:

```nix
inputs.gorefactor.url = "github:flaticols/gorefactor";
```

### Release archives

Tagged releases publish `tar.gz` archives for macOS (`amd64`, `arm64`) on the
[releases page](https://github.com/flaticols/gorefactor/releases).

## Next steps

- [Inspect a package or symbol](../README.md#inspect)
- [Check dependency rules](../README.md#check)
- [Write rule files](../README.md#rules)
- [Neovim plugin](../README.md#neovim-plugin)
