# Surfacing gorefact in the Zed editor

A ranked, executable plan for exposing gorefact's import/reference graph data
inside Zed to help plan refactors.

## Context

**What gorefact produces.** gorefact is a package-centric Go analyzer. Its
engine (`internal/inspect`) resolves a target into structured data and the CLI
already emits it on stdout via `--format json|md|text`:

- `inspect.ResolveTarget(target, cfg)` → `InspectResult` with `PkgPath`,
  `Symbols` (exported public API, each with `File`/`Line`), and `Edges`
  (reference edges, each carrying `CallerPkg`, `CallerFunc`, `Kind` =
  call/read/write/typeref, `File`, `Line`, `Col`, `SamePkg`).
- `inspect.ListPackages(cfg)` → all workspace package paths + the module path.
- `inspect.LoadStructMembers(cfg, pkg, type)` → exported methods/fields of a
  named type, each with position and its own reference edges.
- `inspect.BuildGraph(cfg)` → the `PackageGraph` (`ImportersOf`, `AllPaths`).

The `--format json` output keys are: `target`, `pkgPath`, `symbolName`,
`publicApi` (`{kind,name,file,line}`), `importers` (`[]string`), `references`
(`{callerPkg,symbolId,symbol,kind,file,line,col,samePkg}`). These map 1:1 onto
any structured integration surface — no new analysis logic is required.

**Goal.** Let a developer inside Zed answer refactor questions grounded in the
real graph: "who imports this package?", "what uses `PkgB.SymY`, and from
where?", "what's the import path from A to B?" — instead of guessing.

**Hard constraint.** Zed has **no custom-panel / custom-UI extension API**.
gorefact data can only surface through channels Zed already renders: the Agent
Panel (LLM-callable MCP tools), the integrated terminal (tasks), or
language-server features Zed natively displays (hover, code actions, code lens).
A bespoke tree view like the old Neovim plugin is not possible.

## Ranked options

| # | Vector | Effort | Value | What Zed can ACTUALLY display today |
|---|--------|:------:|:-----:|-------------------------------------|
| 1 | **MCP context server** (`gorefact mcp` stdio server, registered in `settings.json` `context_servers`) | 3 | 5 | Agent-callable **Tools** in the Agent Panel (key format `mcp:gorefact:<tool>`). The agent calls them mid-conversation and grounds its refactor plan in real edges. No panel, no resources, no prompts (see below). Cross-client: the same binary works in Claude Code / Cursor / any MCP host. |
| 2 | **Zed Tasks** (`.zed/tasks.json` running the CLI) | 1 | 3 | Plain text/markdown report in the **integrated terminal** scrollback. Runnable from the `task: spawn` modal, a keybinding, or gutter runnables. No clickable file:line, no editor decorations — read-only text. Zero build. |
| 3 | **Additional LSP server** alongside gopls (Rust→WASM extension launching a new `gorefact-lsp` binary) | 4 | 4 | Native rendering of **hover** popovers, **code actions** (lightbulb), and **code lens** ("N importers / N references" above decls). Reference edges → LSP `Location[]` shown in Zed's native multi-location picker. Zed-only; requires a new long-lived LSP server. |
| — | MCP **Prompts** / extension slash commands | — | 0 | **Nothing.** Dead surface in the current agent UI (rejected — see below). |
| — | ACP agent server | 2 | 1 | Overkill / wrong shape (rejected — see below). |

Effort/value are relative (1 = trivial, 5 = large/high). Verdicts respect the
source docs current as of this writing — see citations per row.

## Recommendation

1. **Primary — `gorefact mcp` MCP context server.** Highest value (5) and the
   only vector with a cross-client multiplier: one stdio MCP server reused
   unchanged across Zed, Claude Code, Cursor, and any MCP host. It wraps the
   existing `internal/inspect` engine — no new analysis. Registered purely via
   `settings.json` (`context_servers`), **no extension/WASM needed**.
   ([zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp))

2. **Quick win — ship example `.zed/tasks.json`.** Effort 1, zero build, works
   today. Ranked above LSP despite lower value because it is immediate: paste a
   file, get gorefact reports in the terminal. A good stopgap and a useful
   complement to MCP for users who don't drive the Agent Panel.

3. **Only if feasible — minimal `gorefact-lsp`.** A second Go language server
   beside gopls, surfaced through hover, code actions, and code lens. Real and
   supported, but it is net-new long-lived transport (the deleted `internal/rpc`
   was a bespoke JSON-RPC daemon, **not** LSP — no reuse there), it is Zed-only,
   and code lens requires the user to opt in (`code_lens: "on"`). Build only
   after the MCP server proves the value.

---

## Primary implementation sketch: `gorefact mcp`

### Where it lives

- **`cmd/gorefact/main.go`** — add `case "mcp"` to the dispatch in `run()`
  (currently only `help`/`version` are reserved; everything else is treated as
  a package target). Note: this **reserves the word `mcp`** — `gorefact mcp`
  will no longer inspect a package literally named `mcp` (use the full path or
  `--dir` form for that edge case).
- **`internal/mcp`** (new package) — the stdio MCP server. It depends only on
  `internal/inspect` (and `internal/loader` for the graph). This mirrors the
  deleted `internal/rpc` package as structural precedent (a transport layer over
  the analysis engine) — but it is **net-new code**: `internal/rpc` spoke a
  bespoke JSON-RPC protocol for the old Neovim plugin, whereas this speaks the
  MCP wire protocol. Use a Go MCP SDK
  (e.g. `github.com/modelcontextprotocol/go-sdk` or `github.com/mark3labs/mcp-go`)
  for the stdio JSON-RPC transport and tool registration.

### Protocol

stdio JSON-RPC (MCP standard transport). The subcommand reads requests on
stdin, writes responses on stdout, logs to stderr. The working directory is the
Zed project root, so a `dir` tool argument defaults to `.` (cwd). Build the
`PackageGraph` lazily/once per dir and keep it warm for the process lifetime to
avoid re-loading on every tool call.

### Tool set

All gorefact data is exposed as **MCP Tools** (Zed surfaces Tools, not Prompts
or Resources — see Rejected). Each tool calls an existing `inspect` function and
returns the JSON gorefact already produces.

| Tool | Inputs | Backed by | Output |
|------|--------|-----------|--------|
| `list_packages` | `dir?` | `inspect.ListPackages` | `{ module, packages: []string }` |
| `package_api` | `dir?`, `pkg` | `inspect.ResolveTarget` (`.Symbols`, `.PkgGraph.ImportersOf`) | `{ pkgPath, publicApi:[{kind,name,file,line}], importers:[]string }` |
| `list_importers` | `dir?`, `pkg` | `inspect.ResolveTarget(...).PkgGraph.ImportersOf` | `{ pkgPath, importers:[]string }` |
| `symbol_uses` | `dir?`, `pkg`, `symbol` | `inspect.ResolveTarget("pkg.Symbol")` (already filters `Edges` by symbol) | `{ symbol, references:[{callerPkg,callerFunc,kind,file,line}] }` |
| `struct_members` | `dir?`, `pkg`, `type` | `inspect.LoadStructMembers` | `{ members:[{name,kind,signature,type,file,line,edges}] }` |
| `import_path` | `dir?`, `from`, `to` | `PackageGraph.AllPaths` | `{ paths: [][]string }` |

Each handler builds a `inspect.Config{Loader: loader.Config{Dir: dir}}`, calls
the function, and serializes the same shapes `internal/inspect/format.go`
already emits. `symbol_uses` reuses `ResolveTarget`'s built-in symbol filtering
(pass `pkg.Symbol` as the target). Wire-format reuse keeps the MCP layer thin.

### Zed registration (`settings.json`)

Exact flat shape verified against
[zed.dev/docs/ai/mcp](https://zed.dev/docs/ai/mcp) — `command` is a plain
string; siblings are `args` (array) and `env` (object). No extension required.

```json
{
  "context_servers": {
    "gorefact": {
      "command": "gorefact",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

`gorefact` must be on `PATH` (otherwise use an absolute path in `command`).
Then enable it in an Agent Profile (`enable_all_context_servers: true`, or list
`gorefact` under the profile's `context_servers`), and confirm any tool-call
permission prompts. The Agent Panel's "Add Custom Server" button writes this
same config.
([zed.dev/docs/ai/agent-panel](https://zed.dev/docs/ai/agent-panel),
[zed.dev/docs/ai/agent-profiles](https://zed.dev/docs/ai/agent-profiles))

**Optional discoverability later:** ship a thin Rust→WASM extension declaring
`[context_servers.gorefact]` in `extension.toml` with a `context_server_command`
returning `zed::Command { command: "gorefact", args: vec!["mcp"], env }`. Pure
launcher plumbing — no analysis in Rust. Only needed for marketplace install;
the `settings.json` path needs none of it.
([zed.dev/docs/extensions/mcp-extensions](https://zed.dev/docs/extensions/mcp-extensions))

---

## Quick win: `.zed/tasks.json`

Drop this at the worktree root. Uses only binary-verified `$ZED_*` variables.
([zed.dev/docs/tasks](https://zed.dev/docs/tasks))

```json
[
  {
    "label": "gorefact: package report (current dir)",
    "command": "gorefact",
    "args": ["--dir", "${ZED_WORKTREE_ROOT}", "${ZED_RELATIVE_DIR}", "--format", "md"],
    "reveal": "always",
    "allow_concurrent_runs": true
  },
  {
    "label": "gorefact: explore current package (TUI)",
    "command": "gorefact",
    "args": ["--dir", "${ZED_WORKTREE_ROOT}", "${ZED_RELATIVE_DIR}"],
    "use_new_terminal": true
  },
  {
    "label": "gorefact: uses of symbol ${ZED_SYMBOL}",
    "command": "gorefact",
    "args": ["--dir", "${ZED_WORKTREE_ROOT}", "${ZED_RELATIVE_DIR}.${ZED_SYMBOL}", "--format", "md"],
    "reveal": "always"
  }
]
```

Optional keybinding (`keymap.json`):

```json
{ "context": "Editor", "bindings": { "alt-g": ["task::Spawn", { "task_name": "gorefact: package report (current dir)" }] } }
```

**Variable rules (verified against the built binary):**

- Use **`${ZED_RELATIVE_DIR}`** as the target (e.g. `internal/loader`). It is
  non-path-like, so gorefact keeps it as the target and resolves it via
  bare-suffix package matching.
- **Do NOT** use `${ZED_RELATIVE_FILE}` (has a `.go` extension),
  `${ZED_DIRNAME}` / `${ZED_FILE}` (absolute paths — gorefact swallows an
  absolute first positional as `--dir`, leaving the target empty → exit 2
  "a target package is required"), or `${ZED_STEM}` (filename stem, not package
  identity).
- **Always pass `--format md` (or `json`/`text`)** for a report. The integrated
  terminal is a TTY, so omitting `--format` launches the interactive Bubbletea
  TUI in the terminal pane instead (that is the deliberate "explore" task above).
- `${ZED_SYMBOL}` is reliable for top-level funcs/types; for **methods** the
  breadcrumb may yield only the bare method name, which gorefact resolves to a
  package with 0 references (silent miss). Treat the symbol task as best-effort.
- Assumes **worktree root == Go module root**. If `go.mod` is in a subdirectory,
  `--dir ${ZED_WORKTREE_ROOT}` + the relative-dir suffix won't align with import
  paths.

---

## Optional: minimal `gorefact-lsp`

Build only after MCP proves valuable. Two parts:

1. **Go LSP server** (new binary `cmd/gorefact-lsp`): a long-lived
   JSON-RPC-over-stdio language server (e.g. `go.lsp.dev/protocol`). On
   `initialize` advertise `hoverProvider`, `codeActionProvider`,
   `codeLensProvider`. Build the `PackageGraph` once and keep it warm; on a
   per-file request, map the file back to its package and call `ResolveTarget`.
   - **Hover** on an import/exported symbol → markdown ("Imported by N packages;
     used by `PkgA.FuncX`, `PkgB.FuncY`") assembled from `References`.
   - **Code actions** ("Show usages of SymY") → return reference edges as LSP
     **`Location[]`** (each `Edge` has `File`/`Line`), surfaced in Zed's native
     multi-location picker. Navigate via `Location[]`, **not** `executeCommand`
     (Zed has no custom panel to open, and the executeCommand-filtering behavior
     is unverified).
   - **Code lens** ("N importers" / "N references") above the package clause and
     exported decls, counts from `len(Importers)` / per-symbol edge counts.

2. **Zed extension (Rust→WASM)**: `extension.toml` with
   `[language_servers.gorefact]` `name = "gorefact"`, `languages = ["Go"]`
   (the language name must match Go's `config.toml` `name`). Implement the Rust
   `language_server_command` trait method returning `zed::Command` pointing at
   the `gorefact-lsp` binary. The WASM only launches the native binary — no LSP
   logic in WASM. Users opt in with:
   ```json
   { "languages": { "Go": { "language_servers": ["gopls", "gorefact", "..."] } }, "code_lens": "on" }
   ```
   The `language_servers` list runs servers concurrently; `"..."` is a wildcard,
   `!name` disables.
   ([zed.dev/docs/extensions/languages](https://zed.dev/docs/extensions/languages),
   [zed.dev/docs/configuring-languages](https://zed.dev/docs/configuring-languages))

**Code lens — capability and limitations.** Code lens **does render** today
(PR #54100, merged 2026-04-22, closed the reference-count issue #11565). The
`code_lens` setting is `off` (default) / `on` / `menu`, described as showing
"reference counts, implementations, and other metadata provided by the language
server." gopls' `test` lens proves the path end-to-end. The honest caveats are
operational, not "it doesn't work":
- default is **off** — the user must set `code_lens: "on"` (or `"menu"`);
- it needs a **new long-lived position-addressable LSP server** (net-new
  transport, no reuse from the deleted `internal/rpc`);
- it is **Zed-only** — no cross-client reuse, which is why this sits below MCP.

([zed.dev/docs/reference/all-settings](https://zed.dev/docs/reference/all-settings),
[PR #54100](https://github.com/zed-industries/zed/pull/54100),
[issue #11565](https://github.com/zed-industries/zed/issues/11565),
[zed.dev/docs/languages/go](https://zed.dev/docs/languages/go))

---

## Verification

**MCP server (primary).**
- Stand-alone, before Zed: run the MCP Inspector against the binary —
  `npx @modelcontextprotocol/inspector gorefact mcp` — and confirm the tool list
  (`list_packages`, `package_api`, `symbol_uses`, …) and that calling a tool
  returns the expected JSON.
- Sanity-check the underlying engine output it wraps (nushell, piped):
  ```nu
  gorefact --format json internal/loader | from json | get importers
  gorefact --format json internal/loader.WalkRefs | from json | get references | length
  ```
- In Zed: add the `context_servers` snippet, restart, open the Agent Panel,
  enable the `gorefact` server in the active Agent Profile, and prompt e.g.
  "what packages import `internal/loader`, and what uses `WalkRefs`?" Confirm the
  agent invokes `mcp:gorefact:list_importers` / `mcp:gorefact:symbol_uses` and
  cites real file:line edges.

**Tasks (quick win).**
- Add `.zed/tasks.json`, open a `.go` file inside a package, run `task: spawn`
  → "gorefact: package report (current dir)", and confirm a markdown report
  appears in the terminal. Verify `${ZED_RELATIVE_DIR}` resolved to the package
  path (not a `.go` file). With nothing selected, the symbol task should hide
  (no `ZED_SYMBOL`).

**LSP (optional).**
- Run `gorefact-lsp` against a hand-written `initialize` + `textDocument/hover`
  request over stdio first. In Zed, install the dev extension, set the
  `language_servers` list + `code_lens: "on"`, open a Go file, and confirm
  hover/code-action/lens render alongside gopls.

---

## Out of scope / rejected

- **MCP Prompts / extension slash commands** — **dead surface.** Zed's docs list
  "Tools and Prompts," but text threads were removed
  ([#53760](https://github.com/zed-industries/zed/issues/53760), closed
  not-planned) and MCP prompts are not available in agent threads
  ([#31324](https://github.com/zed-industries/zed/discussions/31324)). Authoring
  a "plan-refactor" prompt as a Zed slash command would target a surface that
  does not render. Expose **everything as Tools**.
- **MCP Resources** — Zed does not surface them (no `@`-mention of resources).
  Don't model gorefact output as Resources.
- **ACP agent servers / external agents** — these host a *full* external coding
  agent that owns its own runtime/auth/model/tools over the Agent Client
  Protocol. gorefact is a read-only single-shot analyzer; wrapping it as an ACP
  agent is a massive impedance mismatch, and the extension-based ACP path is
  being deprecated for the ACP Registry.
  ([zed.dev/docs/ai/external-agents](https://zed.dev/docs/ai/external-agents))
- **Any custom panel / tree view** — Zed has no custom-UI extension API; the old
  Neovim-style tree pane cannot be reproduced.
