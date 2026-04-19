# gorefact Engine Rebuild & Inspection Feature — Design Spec

**Date:** 2026-04-19  
**Status:** Draft

---

## Problem

The existing tool is a call-graph explorer backed by SSA/CHA. The target codebase is a Go service with ~90K functions, import cycles, and layering violations. The primary workflow is:

1. Pick a package, type, function, const, or var.
2. See everything that imports or references it — full tree — to understand *why* a coupling exists.
3. Define and enforce layering rules (`tasks/` must never import `adapters/`).
4. Plan and validate a refactoring.

SSA loaded for 90K functions at startup is too slow (60–120s). The existing engine's call-graph-only model misses consts, vars, and types.

---

## Goals

- Sub-5s startup for a 90K-function service.
- Full reference tracking: calls, reads, writes, type usages, returns.
- Same-package usages highlighted separately from cross-package usages.
- Grouping by caller, callee, or package.
- Interactive TUI (full-screen, auto-detected on TTY) + structured CLI output (JSON, Markdown, quickfix) for non-TTY.
- Rule management through the tool (TUI keybinding + CLI subcommand).
- SSA/CHA kept for scoped deep analysis and the existing `check` command.
- Neovim RPC protocol extended, not replaced.

---

## Architecture

### Three-Tier Engine

```
Tier 1 — Package graph    (always loaded, ~1s)
Tier 2 — Reference graph  (on demand, per target, ~0.5s per package)
Tier 3 — SSA call graph   (opt-in, scoped, existing behaviour)
```

**Tier 1** uses `go/packages` with `NeedImports | NeedName | NeedModule`. Loads the full package dependency graph for the entire service. Supports: import cycle detection, package-level layering violations, "who imports this package."

**Tier 2** uses `go/packages` with `NeedSyntax | NeedTypes | NeedTypesInfo` scoped to a specific package (and optionally its direct importers). Walks AST with `go/types` to collect every reference to every exported declaration in the target package. Produces `Symbol` nodes and `Edge` records with `Kind` (call, read, write, typeref). Loads on first query for a given package, cached in memory.

**Tier 3** is the existing SSA/CHA engine, invoked when `--depth=full` is passed or when the TUI requests a precise call tree. Always scoped via `--filter-pkg` to avoid loading the full 90K-function graph.

### Data Model

```go
// EdgeKind classifies the nature of a reference.
type EdgeKind string
const (
    EdgeCall    EdgeKind = "call"
    EdgeRead    EdgeKind = "read"
    EdgeWrite   EdgeKind = "write"
    EdgeTypeRef EdgeKind = "typeref"
)

// Symbol represents a non-function declaration (var, const, type).
type Symbol struct {
    ID       int
    Kind     string // "var" | "const" | "type"
    Name     string
    Package  string
    File     string
    Line     int
    Exported bool
}

// Edge now carries kind and same-package flag.
type Edge struct {
    Caller  int      // node ID (Func or Symbol)
    Callee  int      // node ID (Func or Symbol)
    Kind    EdgeKind
    SamePkg bool
    File    string
    Line    int
    Col     int
    Dynamic bool
}
```

Funcs and Symbols share a single integer ID counter so edges are uniform. Existing consumers filter `edge.Kind == EdgeCall` — no breaking changes.

---

## Entry Points and TTY Routing

TTY detection uses `golang.org/x/term` (`term.IsTerminal(int(os.Stdout.Fd()))`).

### Bare `gorefact` (no subcommand)

- **TTY**: launches TUI directly — equivalent to `gorefact inspect` with an empty search query.
- **Non-TTY**: prints root help. Piping `gorefact` into anything never starts an interactive UI.

### `gorefact serve`

Starts the long-lived JSON-RPC server over stdin/stdout for the Neovim plugin. Never starts the TUI. The Neovim plugin always uses this mode — spawns `gorefact serve` as a subprocess and communicates exclusively via the RPC protocol. TTY state is irrelevant.

### `gorefact inspect [target] [flags]`

`target` follows Go qualified-name notation. When a name is ambiguous (e.g. `tasks.Run` could be a func or a type), Tier 2 resolves it via `go/types` and returns all matching declarations; the TUI lets you pick, non-TTY output includes all matches.

- `github.com/acme/tasks` — entire package
- `github.com/acme/tasks.Run` — function, var, const, or type named `Run`
- `github.com/acme/tasks.Engine.Calculate` — method

**Flags:**
- `--format text|json|md|qf` (non-TTY output)
- `--group caller|callee|pkg` (grouping mode, default: pkg)
- `--depth fast|full` (fast = T1+T2, full = T1+T2+T3 SSA)
- `--same-pkg` (include same-package references, default: shown but marked)
- `--rules path/to/gorefact.rules.toml`
- `--no-tui` (force CLI output even on TTY)

**TTY routing:** TTY → launches TUI with `target` pre-filled. Non-TTY → prints structured output.

---

## TUI

Built with **Bubble Tea** (charmbracelet/bubbletea) + **Lip Gloss** for styling.

### Layout

```
┌─ search ──────────────────────────────────────────────────────────┐
│ > tasks.Engine                                                     │
├─ results ─────────────────┬─ tree ──────────────────┬─ detail ───┤
│ tasks.Engine       [8 ref] │ ▼ tasks.Engine          │ pkg: tasks  │
│ tasks.EngineConfig [3 ref] │   ✗ handler.Process     │ kind: type  │
│                            │     handler/proc.go:42  │ file: ...   │
│                            │   ✓ service.Run          │             │
│                            │   ~ tasks.NewEngine     │ violations: │
│                            │     (same pkg)          │ handler →   │
│                            │                         │ tasks [DENY]│
└────────────────────────────┴─────────────────────────┴─────────────┘
[g] group  [d] depth  [r] add rule  [f] filter violations  [?] help
```

- **Search pane**: fuzzy filter across all packages and symbols. Pre-filled from CLI arg if provided.
- **Tree pane**: callers/importers of selected symbol, grouped by `--group` mode. Violation markers: `✗` = deny rule hit, `~` = same-package, `✓` = clean.
- **Detail pane**: symbol metadata, file/line, active violations.
- **Group toggle** (`g`): cycles through caller / callee / pkg groupings.
- **Depth toggle** (`d`): upgrades current view from T2 to T3 (SSA), shows spinner during load.
- **Add rule** (`r`): opens a small form pre-filled with caller pkg → callee pkg, choose deny/allow, enter reason, writes to `gorefact.rules.toml` immediately.
- **Filter violations** (`f`): hides clean edges, shows only violation subtree.

---

## Rule Management

### TUI flow (`r` key)
Opens an inline form:
```
Add rule
  from: handler          [editable]
  to:   tasks            [editable]
  type: [deny] / allow
  reason: handlers must go through service layer
  [Enter] save   [Esc] cancel
```
Appends to `gorefact.rules.toml` (or creates it). Reloads rules in memory without restart.

### CLI subcommands

```bash
# Add a deny rule
gorefact rules add --deny handler tasks "handlers must go through service layer"

# Add an allow rule  
gorefact rules add --allow tasks service ""

# List all rules
gorefact rules list [--format text|json]

# Remove a rule
gorefact rules remove --from handler --to tasks
```

---

## Extended RPC Protocol (Neovim)

New methods added, existing methods unchanged:

| Method | Description |
|---|---|
| `gorefact.inspectSymbol` | Returns T2 reference tree for a qualified name |
| `gorefact.packageTree` | Returns T1 import tree for a package path |
| `gorefact.rulesAdd` | Appends a rule to the rules file |
| `gorefact.rulesList` | Returns current rule set |
| `gorefact.rulesRemove` | Removes a rule by from/to pair |

Existing methods (`gorefact.search`, `gorefact.tree`, `gorefact.detail`, `gorefact.funcAtPos`, `gorefact.check`) are unchanged.

---

## Output Formats (non-TTY)

All non-TTY output from `inspect` follows the existing format flag convention.

**text**: indented tree with violation markers  
**json**: `{ "target": "...", "nodes": [...], "violations": [...] }`  
**md**: fenced tree block + violations table  
**qf**: quickfix lines pointing to reference sites (for Neovim `:cfile`)

---

## Internal Package Layout (target)

```
internal/
  graph/         — Graph, Func, Symbol, Edge, EdgeKind (existing + extended)
  loader/        — NEW: three-tier loader (T1 PackageGraph, T2 ReferenceWalker, T3 SSALoader)
  rules/         — check, parse, format, write (add WriteRule func)
  treeview/      — tree builder (extended for Symbol nodes)
  rpc/           — JSON-RPC server (new methods added)
  tui/           — NEW: Bubble Tea app (search, tree, detail, rule form panes)
cmd/
  gorefact/      — inspect subcommand + rules subcommands added
```

The existing `internal/graph/build.go` (SSA engine) moves into `internal/loader/ssa.go`. Tier 1 and Tier 2 loaders live in `internal/loader/pkggraph.go` and `internal/loader/refs.go`.

---

## Help System

The `help` command is updated to be comprehensive and machine-readable — useful both for humans running the CLI and for LLMs consuming tool output (e.g. via `gorefact help --format json`).

### Structure

Every subcommand (`inspect`, `check`, `serve`, `rules`, `validate-rules`, `version`) gets:
- **One-line summary** — shown in root `help` listing
- **Purpose paragraph** — what problem it solves, when to use it
- **Flags table** — name, type, default, description
- **Examples** — at least 2 concrete invocations with expected output described
- **Related commands** — cross-references

Root `gorefact help` lists all commands with summaries and a short description of the overall tool purpose (call-graph and import dependency explorer for Go refactoring).

Help output is plain text/Markdown — no JSON format needed. LLMs parse structured Markdown reliably. Each subcommand's help follows a consistent Markdown-ish layout (summary, description, flags table, examples, related commands) so it is both human-scannable and LLM-parseable without extra tooling.

---

## What Is Not Changing

- `gorefact check` — behaviour and output unchanged; still T3 SSA.
- `gorefact serve` — existing RPC methods unchanged; new methods are additive.
- `gorefact validate-rules` — unchanged.
- `gorefact.rules.toml` format — extended with an `[[allow]]` stanza alongside existing `[[deny]]`; the file remains valid TOML and backward-compatible (files with only `[[deny]]` entries continue to work).
- Homebrew / release pipeline — unchanged.
