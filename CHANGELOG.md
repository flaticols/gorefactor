# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.17] - 2026-04-19

### Fixed
- Intra-package references are now included in both the symbol reference tree and per-member reference tree: the target package is added to the walk set alongside its importers (previously only importers were walked, so same-pkg uses were silently missing)

## [0.0.16] - 2026-04-19

### Added
- Live per-member references: navigating through struct/interface members with `j`/`k` rebuilds the right panel to show references to the selected method or field (via `types.Info.Selections` over importer packages)
- `?` toggles full help (bubbles/help + bubbles/key): help bar auto-generated from the key map with short/expanded modes

### Changed
- Both panes now use `bubbles/viewport` for real scrolling (mouse wheel, `ctrl+d/u`); long reference lists no longer truncate at the fold — selection keeps the cursor line in view
- Struct-member panel rendered with `lipgloss/tree` (proper `├─ └─` branching) instead of hand-rolled `|-` prefixes; reference counts `[N]` shown per member

## [0.0.15] - 2026-04-19

### Added
- `m` key toggles module-only search: exclude third-party packages and only show entries under the main module path
- Auto-open struct/interface member view when a single type symbol loads (no `Enter` needed); also works for interfaces

### Changed
- Reference file paths are now module-relative (was absolute): `-dir` default of `.` is resolved to cwd before relativizing
- Detail panel shows the fully-qualified `module/pkg.Name` and wraps long lines across multiple rows (via lipgloss `Width`)

## [0.0.14] - 2026-04-19

### Added
- Struct member view: `Enter` on a `type` symbol opens a new panel listing its exported methods and fields (`|-Method(args) (result) (func)` / `|-Field:Type (field)`); `Esc` returns to the symbol list; `e` opens the focused member in `$EDITOR`
- `inspect.LoadStructMembers(cfg, pkgPath, typeName)` enumerates exported methods (via `types.NewMethodSet` on pointer receiver) and struct fields

### Changed
- Displayed package paths are stripped of the main module prefix in all panels (symbol list, search dropdown, reference tree, detail panel) — e.g. `go.flaticols.dev/gorefactor/internal/graph` is shown as `internal/graph`
- `inspect.ListPackages` now returns `(paths, module, error)`; `loader.PackageGraph` exposes the main module path
- `treeByFunc` renders `pkg.Func` from short pkg + caller func (previously concatenated full pkg path)

## [0.0.13] - 2026-04-19

### Added
- Global symbol index: on TUI startup, all exported symbols across the entire workspace are loaded in the background (NeedTypes, no SSA); searching by name (e.g. `Edge`, `Handler`, `(*Recv).Method`) works immediately without loading a package first
- Results show `SymName (kind)  full/pkg/path` so you can see which package each symbol belongs to

## [0.0.12] - 2026-04-19

### Added
- `e` key opens the focused reference (tree pane) or symbol declaration (list pane) in `$EDITOR` (falls back to `vi`)
- Detail panel is now a true overlay: rendered on top of the two-pane body without shrinking the content area

### Changed
- Symbol list shows `pkg.Name (kind) [N]` (last segment only); full qualified name still shown in search dropdown and detail panel

## [0.0.11] - 2026-04-19

### Changed
- Search results exclude stdlib and `golang.org/` / `google.golang.org/` / `gopkg.in/` packages to reduce noise

## [0.0.10] - 2026-04-19

### Added
- Fuzzy search with live results in TUI: type in search mode to see matching packages and symbols; `j`/`k` navigate the dropdown; `Enter` picks a result
- Search matches by package last-segment, full path, subsequence, and symbol name; symbols shown as `Name (kind)  pkg/path`
- Package list loaded in background on TUI startup (T1 only, fast)

### Fixed
- Version shows as `(devel)` when installed from Nix flake: `buildGoModule` now injects version via `-ldflags "-X main.Version=..."`, GoReleaser does the same

## [0.0.9] - 2026-04-19

### Added
- `GroupFunc` grouping mode in TUI: `g` now cycles `pkg → file → func`, grouping references by caller function with full qualified name (`pkg/path.(*Recv).Method`)
- `CallerFunc` field on `graph.Edge`, populated by T2 AST walk detecting the enclosing function declaration at each reference site
- Symbol list shows full qualified name: `github.com/acme/tasks.Run (func) [12]`
- Detail panel toggled by `i`: compact bottom overlay showing symbol info and violations summary; main layout is now two panes (Symbols + References) using full width

## [0.0.8] - 2026-04-19

### Added
- Vim-style modal TUI navigation: `/` to enter search, `hjkl` for movement, `g`/`f` work in normal mode (no longer eaten by search box)
- Suffix package matching: `gorefact inspect graph` resolves to the first package ending in `/graph`
- `nix/gorefact.nix` binary derivation, auto-updated by GoReleaser on each release
- `flake.nix` now exposes `packages.default` (pre-built binary) and `packages.source` (build from source)
- GoReleaser nix publisher wired up in `.goreleaser.yaml`

## [0.0.7] - 2026-04-19

### Added
- `--format` flag implicitly disables TUI (no need for `--no-tui`)
- Package paths and directory args (`.`, `./...`, `github.com/...`) route directly to inspect without the `inspect` subcommand

## [0.0.6] - 2026-04-19

### Added
- `gorefact inspect [target]` command: show everything that imports or references a package, type, function, const, or var using the T1+T2 engine
- Full-screen Bubble Tea TUI on TTY: 3-pane layout (symbol list, reference tree, detail) with pkg/file grouping, violation markers, and keyboard navigation
- Structured non-TTY output via `--format text|json|md|qf`; passing `--format` auto-disables TUI
- Bare `gorefact .` or `gorefact github.com/acme/pkg` routes directly to inspect without typing the `inspect` subcommand
- `graph.Edge.CallerPkg` field enables T2 tree grouping by caller package
- `rules.CheckPackageImport` for package-level violation detection without SSA

### Fixed
- Fix handling of nil values in RPC responses. Vim's NIL sentinel is now properly converted to Lua nil in both error and result messages, preventing unexpected behavior when the plugin receives responses from the refactoring server.
