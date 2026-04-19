# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
