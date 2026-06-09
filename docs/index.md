# gorefact

**Package-centric explorer for Go import and reference graphs.**

`gorefact` lets you browse a module's packages, inspect a package's public API,
and see exactly who depends on it — built for planning refactors.

## What it does

- **Browse packages** — flat list, folder tree, or import tree.
- **Public API** — exported `const`, `var`, `func`, and types; structs and
  interfaces expand inline into their exported methods and fields.
- **Importers** — who imports the selected package, flat or as a tree.
- **Symbol-level usage** — `CallerPkg.CallerFunc → Symbol` reference sites with
  `file:line`.
- Interactive TUI on a terminal, or structured **text**, **JSON**, **markdown**
  output for pipelines.
- Module-only by default, with a toggle for external packages; optional
  `--filter-pkg` scoping for large repositories.

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
go tool gorefact ./...
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

- [Usage and flags](../README.md#usage)
- [TUI keybindings](../README.md#tui-keybindings)
