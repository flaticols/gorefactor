# gorefact

`gorefact` is a package-centric explorer for Go import and reference graphs. It is built for planning refactors: browse the packages in a module, pick one, and see its public API and exactly who depends on it.

## Features

- **Package picker** — browse a module's packages as a flat list, a folder tree (nested by path segments), or an import tree (nested by import edges).
- **Public API view** — the package's exported `const`, `var`, `func`, and types. A struct or interface expands inline into its exported methods and fields as a sub-tree.
- **Importers view** — who imports the selected package, as a flat list or a tree, annotated with how each importer uses it.
- **Symbol-level usage** — select a symbol to see its reference sites as `CallerPkg.CallerFunc → Symbol` rows with `file:line`, so you can read off exactly which functions touch a symbol.
- Module-only by default, with a toggle to include external packages.
- Interactive TUI on a terminal, or scriptable **text / JSON / markdown** output for pipelines.
- Optional `--filter-pkg` scoping for large repositories.

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

Or pin it per-module as a Go tool dependency (Go 1.24+):

```bash
go get -tool go.flaticols.dev/gorefactor/cmd/gorefact@latest
go tool gorefact ./...
```

### Release archives

Tagged releases publish `tar.gz` archives for macOS (`x86_64`, `arm64`) on the [releases page](https://github.com/flaticols/gorefactor/releases).

---

## Usage

`gorefact` answers: *what does this package export, and who depends on it?*

```bash
# Open the interactive explorer (on a TTY) with the package picker
gorefact

# Open the explorer focused on a specific target
gorefact github.com/acme/tasks
gorefact github.com/acme/tasks.Engine
gorefact github.com/acme/tasks.Engine.Calc

# Short suffix — matched against the module's packages
gorefact tasks
gorefact graph

# Structured output for a target (non-TTY, or force with --format)
gorefact --format json github.com/acme/tasks | jq .
gorefact --format text github.com/acme/tasks | less
gorefact --format md   github.com/acme/tasks
```

A non-TTY invocation, or any invocation with an explicit `--format`, prints a
package report — its public API, its importers, and a reference summary — instead
of opening the TUI. A target package is required for non-TTY output.

### `pkg` — scriptable queries (never opens the TUI)

```bash
gorefact pkg list                              # packages in the module
gorefact pkg importers internal/loader         # who imports this package
gorefact pkg imports tui --format json         # what this package imports
gorefact pkg imports tui --module-only=false   # include stdlib + third-party
gorefact pkg get internal/loader --format md   # full report (API + importers + refs)
```

`list`, `imports`, and `importers` print one package path per line (or
`--format json`); `get` prints the same report as the bare command and also
accepts `--format md`. Targets take full import paths or bare suffixes. All
subcommands accept `--dir`, `--tests`, and `--module-only`.

### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--dir` | `.` | Working directory to analyze |
| `--format` | `text` | Output format `text\|json\|md` (non-TTY, or forces non-TTY when set) |
| `--tests` | `false` | Include test packages |
| `--filter-pkg` | `""` | Only include packages containing this path fragment |
| `--module-only` | `true` | Restrict importers and references to the main module |

### TUI keybindings

| Key | Action |
|-----|--------|
| `t` | Cycle picker mode — flat / folder tree / import tree |
| `g` | Toggle importers grouping — flat ↔ tree |
| `m` | Toggle module-only (include or exclude external packages) |
| `Enter` | Select a package / expand a struct or tree node |
| `/` | Fuzzy-search the package picker |
| `e` | Open the selected symbol or reference site in `$EDITOR` |
| `j` / `k` (or `↓` / `↑`) | Move down / up in the active pane |
| `h` / `l` (or `Tab` / `Shift+Tab`) | Move focus left / right between panes |
| `Esc` | Back / exit search |
| `?` | Toggle help |
| `q` (or `Ctrl+C` / `Ctrl+Q`) | Quit |

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
