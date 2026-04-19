# Inspect Command & TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `gorefact inspect <target>` command with structured non-TTY output and a full-screen Bubble Tea TUI for interactive reference exploration.

**Architecture:** T1+T2 loader already exists in `internal/loader/`; this plan wires it to a new `internal/inspect/` resolver, non-TTY formatters, a CLI entry point with TTY detection, and a `internal/tui/` Bubble Tea model. Bare `gorefact` on a TTY launches the TUI directly.

**Tech Stack:** `golang.org/x/term` (TTY detection), `github.com/charmbracelet/bubbletea` (TUI framework), `github.com/charmbracelet/lipgloss` (styles), `github.com/charmbracelet/bubbles` (textinput, spinner components)

---

## File Map

**New files:**
- `internal/inspect/resolve.go` — `InspectResult`, `ResolveTarget`, `parseTarget`
- `internal/inspect/resolve_test.go`
- `internal/inspect/format.go` — `FormatText`, `FormatJSON`, `FormatMarkdown`, `FormatQuickfix`
- `internal/inspect/format_test.go`
- `internal/tui/styles.go` — Lip Gloss style vars
- `internal/tui/tree_render.go` — pure tree-line builder (testable without Bubble Tea)
- `internal/tui/tree_render_test.go`
- `internal/tui/model.go` — Bubble Tea `Model`, `New`, `Run`, `Init`, `Update`, `View`
- `cmd/gorefact/inspect.go` — `runInspect`, `isTTY`, help text

**Modified files:**
- `internal/graph/types.go` — add `CallerPkg string` to `Edge`
- `internal/loader/refs.go` — populate `CallerPkg` in `WalkRefs`
- `internal/rules/check.go` — add exported `CheckPackageImport`
- `internal/rules/rules_test.go` — add `TestCheckPackageImport`
- `cmd/gorefact/main.go` — add `inspect` case; bare-command TTY routing
- `go.mod` / `go.sum` — new dependencies

---

### Task 1: Add dependencies and extend Edge with CallerPkg

**Files:**
- Modify: `internal/graph/types.go`
- Modify: `internal/loader/refs.go`
- Modify: `go.mod`, `go.sum` (via go get)

- [ ] **Step 1: Add `CallerPkg string` to `Edge` in `internal/graph/types.go`**

In the `Edge` struct, add the field after `SamePkg bool`:

```go
type Edge struct {
	Caller    int
	Callee    int
	Kind      EdgeKind // call, read, write, typeref; empty treated as call
	SamePkg   bool     // true when caller and callee share the same package
	CallerPkg string   // package path of the caller; populated by T2 WalkRefs
	File      string
	Line      int
	Col       int
	Dynamic   bool
}
```

- [ ] **Step 2: Populate `CallerPkg` in `WalkRefs` in `internal/loader/refs.go`**

In the `result.Edges = append(...)` block (around line 110), add `CallerPkg: ipkg.PkgPath`:

```go
result.Edges = append(result.Edges, graph.Edge{
    Caller:    callerFuncID,
    Callee:    sym.ID,
    Kind:      classifyRef(obj),
    SamePkg:   samePkg,
    CallerPkg: ipkg.PkgPath,
    File:      cleanPath(cfg.Dir, pos.Filename),
    Line:      pos.Line,
    Col:       pos.Column,
})
```

- [ ] **Step 3: Add Go module dependencies**

Run in the repo root:

```bash
go get golang.org/x/term@latest
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
go mod tidy
```

- [ ] **Step 4: Verify build**

```bash
GOCACHE=/tmp/gocache go build ./...
```

Expected: no errors. `go.mod` now lists the four new dependencies.

- [ ] **Step 5: Run tests to confirm no regressions**

```bash
GOCACHE=/tmp/gocache go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
jj describe -m "graph: add CallerPkg to Edge; add bubbletea/lipgloss/bubbles/term deps"
jj new
```

---

### Task 2: Export `CheckPackageImport` from rules package

**Files:**
- Modify: `internal/rules/check.go`
- Modify: `internal/rules/rules_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/rules/rules_test.go`:

```go
func TestCheckPackageImport(t *testing.T) {
	rs := []Rule{
		{From: "handler", To: "tasks", Reason: "test deny"},
	}
	got := CheckPackageImport("example.com/app/handler", "example.com/app/tasks", rs)
	if got == nil {
		t.Fatal("expected rule match, got nil")
	}
	if got.Reason != "test deny" {
		t.Fatalf("got reason %q, want %q", got.Reason, "test deny")
	}

	got2 := CheckPackageImport("example.com/app/service", "example.com/app/tasks", rs)
	if got2 != nil {
		t.Fatalf("expected no match for service→tasks, got %v", got2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOCACHE=/tmp/gocache go test ./internal/rules/ -run TestCheckPackageImport -v
```

Expected: `FAIL` — `CheckPackageImport undefined`.

- [ ] **Step 3: Implement `CheckPackageImport` in `internal/rules/check.go`**

Add after the `CheckEdge` function:

```go
// CheckPackageImport returns the first matching deny rule for a package-level
// import from fromPkg to toPkg. Uses the same matching logic as edge checks.
func CheckPackageImport(fromPkg, toPkg string, rs []Rule) *Rule {
	for i := range rs {
		if matches(fromPkg, rs[i].From) && matches(toPkg, rs[i].To) {
			return &rs[i]
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOCACHE=/tmp/gocache go test ./internal/rules/ -run TestCheckPackageImport -v
```

Expected: `PASS`.

- [ ] **Step 5: Run full test suite**

```bash
GOCACHE=/tmp/gocache go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
jj describe -m "rules: export CheckPackageImport for package-level violation checks"
jj new
```

---

### Task 3: Inspect resolver (`internal/inspect/resolve.go`)

**Files:**
- Create: `internal/inspect/resolve.go`
- Create: `internal/inspect/resolve_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/inspect/resolve_test.go`:

```go
package inspect_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input  string
		pkg    string
		sym    string
		method string
	}{
		{"github.com/acme/tasks", "github.com/acme/tasks", "", ""},
		{"github.com/acme/tasks.Run", "github.com/acme/tasks", "Run", ""},
		{"github.com/acme/tasks.Engine.Calculate", "github.com/acme/tasks", "Engine", "Calculate"},
		{"tasks", "tasks", "", ""},
		{"tasks.Run", "tasks", "Run", ""},
	}
	for _, tc := range tests {
		pkg, sym, meth := inspect.ParseTarget(tc.input)
		if pkg != tc.pkg || sym != tc.sym || meth != tc.method {
			t.Errorf("ParseTarget(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.input, pkg, sym, meth, tc.pkg, tc.sym, tc.method)
		}
	}
}

func TestResolveTarget_Package(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Engine struct{}

func NewEngine() Engine { return Engine{} }

const Version = "1.0"
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() alpha.Engine { return alpha.NewEngine() }
`)

	cfg := inspect.Config{
		Loader: loader.Config{Dir: dir, Patterns: []string{"./..."}},
	}
	res, err := inspect.ResolveTarget("example.com/test/alpha", cfg)
	if err != nil {
		t.Fatalf("ResolveTarget error = %v", err)
	}
	if res.PkgPath != "example.com/test/alpha" {
		t.Errorf("PkgPath = %q", res.PkgPath)
	}
	if len(res.Symbols) == 0 {
		t.Fatal("expected symbols, got none")
	}
	if len(res.Edges) == 0 {
		t.Fatal("expected edges from beta→alpha, got none")
	}
	for _, e := range res.Edges {
		if e.CallerPkg == "" {
			t.Error("edge missing CallerPkg")
		}
	}
}

func TestResolveTarget_Symbol(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Engine struct{}

func NewEngine() Engine { return Engine{} }
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() alpha.Engine { return alpha.NewEngine() }
`)

	cfg := inspect.Config{
		Loader: loader.Config{Dir: dir, Patterns: []string{"./..."}},
	}
	res, err := inspect.ResolveTarget("example.com/test/alpha.Engine", cfg)
	if err != nil {
		t.Fatalf("ResolveTarget error = %v", err)
	}
	if res.SymbolName != "Engine" {
		t.Errorf("SymbolName = %q, want Engine", res.SymbolName)
	}
	for _, s := range res.Symbols {
		if s.Name != "Engine" {
			t.Errorf("unexpected symbol %q in filtered result", s.Name)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOCACHE=/tmp/gocache go test ./internal/inspect/... -v 2>&1 | head -20
```

Expected: build error — `package inspect does not exist`.

- [ ] **Step 3: Create `internal/inspect/resolve.go`**

```go
package inspect

import (
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/rules"
)

// InspectResult holds the resolved reference data for a target.
type InspectResult struct {
	Target     string
	PkgPath    string
	SymbolName string // empty = whole package
	Symbols    []graph.Symbol
	Edges      []graph.Edge
	PkgGraph   *loader.PackageGraph
	Violations []PackageViolation
}

// PackageViolation records a deny-rule hit at the package-import level.
type PackageViolation struct {
	FromPkg string
	ToPkg   string
	Rule    rules.Rule
}

// Config configures the inspect resolver.
type Config struct {
	Loader loader.Config
	Rules  []rules.Rule
}

// ResolveTarget parses target, loads T1+T2 data, and returns an InspectResult.
// target may be a package path, "pkg.Symbol", or "pkg.Receiver.Method".
func ResolveTarget(target string, cfg Config) (*InspectResult, error) {
	target = strings.TrimSpace(target)
	pkgPath, symbolName, _ := ParseTarget(target)

	if cfg.Loader.Progress != nil {
		cfg.Loader.Progress("loading package graph")
	}
	pg, err := loader.BuildPackageGraph(cfg.Loader)
	if err != nil {
		return nil, err
	}

	importers := pg.ImportersOf(pkgPath)
	importerPaths := make([]string, len(importers))
	for i, n := range importers {
		importerPaths[i] = n.Path
	}

	if cfg.Loader.Progress != nil {
		cfg.Loader.Progress("walking references")
	}
	refs, err := loader.WalkRefs(pkgPath, importerPaths, cfg.Loader, 10000)
	if err != nil {
		return nil, err
	}

	syms := refs.Symbols
	edges := refs.Edges

	if symbolName != "" {
		var filtSyms []graph.Symbol
		for _, s := range syms {
			if s.Name == symbolName {
				filtSyms = append(filtSyms, s)
			}
		}
		ids := make(map[int]bool, len(filtSyms))
		for _, s := range filtSyms {
			ids[s.ID] = true
		}
		var filtEdges []graph.Edge
		for _, e := range edges {
			if ids[e.Callee] {
				filtEdges = append(filtEdges, e)
			}
		}
		syms = filtSyms
		edges = filtEdges
	}

	var viols []PackageViolation
	for _, imp := range importers {
		if r := rules.CheckPackageImport(imp.Path, pkgPath, cfg.Rules); r != nil {
			viols = append(viols, PackageViolation{
				FromPkg: imp.Path,
				ToPkg:   pkgPath,
				Rule:    *r,
			})
		}
	}

	return &InspectResult{
		Target:     target,
		PkgPath:    pkgPath,
		SymbolName: symbolName,
		Symbols:    syms,
		Edges:      edges,
		PkgGraph:   pg,
		Violations: viols,
	}, nil
}

// ParseTarget splits a qualified target into package path, symbol name, and method name.
//
//	"github.com/acme/tasks"             → ("github.com/acme/tasks", "", "")
//	"github.com/acme/tasks.Run"         → ("github.com/acme/tasks", "Run", "")
//	"github.com/acme/tasks.Engine.Calc" → ("github.com/acme/tasks", "Engine", "Calc")
func ParseTarget(target string) (pkgPath, symbol, method string) {
	lastSlash := strings.LastIndex(target, "/")
	suffix := target
	if lastSlash >= 0 {
		suffix = target[lastSlash+1:]
	}
	dotIdx := strings.Index(suffix, ".")
	if dotIdx < 0 {
		return target, "", ""
	}
	base := lastSlash + 1
	pkgPath = target[:base+dotIdx]
	rest := suffix[dotIdx+1:]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) == 1 {
		return pkgPath, parts[0], ""
	}
	return pkgPath, parts[0], parts[1]
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOCACHE=/tmp/gocache go test ./internal/inspect/... -v
```

Expected: `TestParseTarget` and `TestResolveTarget_Package` and `TestResolveTarget_Symbol` all PASS.

- [ ] **Step 5: Commit**

```bash
jj describe -m "inspect: add ResolveTarget and ParseTarget"
jj new
```

---

### Task 4: Inspect non-TTY formatters (`internal/inspect/format.go`)

**Files:**
- Create: `internal/inspect/format.go`
- Create: `internal/inspect/format_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/inspect/format_test.go`:

```go
package inspect_test

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/rules"
)

func makeTestResult() *inspect.InspectResult {
	return &inspect.InspectResult{
		Target:  "example.com/tasks",
		PkgPath: "example.com/tasks",
		Symbols: []graph.Symbol{
			{ID: 100, Kind: "type", Name: "Engine", Package: "example.com/tasks", File: "tasks/engine.go", Line: 5, Exported: true},
		},
		Edges: []graph.Edge{
			{Caller: 0, Callee: 100, Kind: graph.EdgeCall, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42, Col: 5, SamePkg: false},
			{Caller: 0, Callee: 100, Kind: graph.EdgeTypeRef, CallerPkg: "example.com/service", File: "service/s.go", Line: 10, Col: 3, SamePkg: false},
		},
		PkgGraph: &loader.PackageGraph{Nodes: map[string]*loader.PackageNode{
			"example.com/tasks":   {Path: "example.com/tasks", Name: "tasks"},
			"example.com/handler": {Path: "example.com/handler", Name: "handler", Imports: []string{"example.com/tasks"}},
			"example.com/service": {Path: "example.com/service", Name: "service", Imports: []string{"example.com/tasks"}},
		}},
		Violations: []inspect.PackageViolation{
			{FromPkg: "example.com/handler", ToPkg: "example.com/tasks", Rule: rules.Rule{From: "handler", To: "tasks", Reason: "use service layer"}},
		},
	}
}

func TestFormatText(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatText(res, inspect.FormatOptions{})
	if !strings.Contains(out, "example.com/handler") {
		t.Errorf("FormatText missing handler pkg, got:\n%s", out)
	}
	if !strings.Contains(out, "DENY") {
		t.Errorf("FormatText missing DENY marker, got:\n%s", out)
	}
	if !strings.Contains(out, "handler/h.go") {
		t.Errorf("FormatText missing file reference, got:\n%s", out)
	}
}

func TestFormatJSON(t *testing.T) {
	res := makeTestResult()
	data, err := inspect.FormatJSON(res)
	if err != nil {
		t.Fatalf("FormatJSON error = %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"target"`) {
		t.Errorf("FormatJSON missing target field: %s", s)
	}
	if !strings.Contains(s, `"example.com/handler"`) {
		t.Errorf("FormatJSON missing callerPkg: %s", s)
	}
}

func TestFormatMarkdown(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatMarkdown(res)
	if !strings.Contains(out, "## ") {
		t.Errorf("FormatMarkdown missing heading: %s", out)
	}
	if !strings.Contains(out, "example.com/tasks") {
		t.Errorf("FormatMarkdown missing package: %s", out)
	}
}

func TestFormatQuickfix(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatQuickfix(res)
	if !strings.Contains(out, "handler/h.go:42") {
		t.Errorf("FormatQuickfix missing file:line, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOCACHE=/tmp/gocache go test ./internal/inspect/... -run "TestFormat" -v 2>&1 | head -10
```

Expected: build error — format functions undefined.

- [ ] **Step 3: Create `internal/inspect/format.go`**

```go
package inspect

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rules"
)

// FormatOptions controls non-TTY output rendering.
type FormatOptions struct {
	BaseDir  string
	ViolOnly bool
}

// FormatText returns a human-readable reference tree.
func FormatText(res *InspectResult, opts FormatOptions) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Target:  %s\n", res.Target))
	b.WriteString(fmt.Sprintf("Package: %s\n", res.PkgPath))
	if res.SymbolName != "" {
		b.WriteString(fmt.Sprintf("Symbol:  %s\n", res.SymbolName))
	}
	b.WriteString(fmt.Sprintf("Edges:   %d\n", len(res.Edges)))

	if len(res.Violations) > 0 {
		b.WriteString(fmt.Sprintf("\nViolations (%d):\n", len(res.Violations)))
		for _, v := range res.Violations {
			b.WriteString(fmt.Sprintf("  [DENY] %s → %s: %s\n", v.FromPkg, v.ToPkg, v.Rule.Reason))
		}
	}

	b.WriteString("\nReferences:\n")

	groups := groupEdgesByPkg(res.Edges)
	pkgs := sortedPkgKeys(groups)
	violPkgs := violationPkgSet(res.Violations)

	for _, pkg := range pkgs {
		edges := groups[pkg]
		if opts.ViolOnly && !violPkgs[pkg] {
			continue
		}
		viol := ""
		if violPkgs[pkg] {
			for _, v := range res.Violations {
				if v.FromPkg == pkg {
					viol = fmt.Sprintf(" [DENY: %s]", v.Rule.Reason)
					break
				}
			}
		}
		b.WriteString(fmt.Sprintf("\n  %s%s (%d refs)\n", pkg, viol, len(edges)))
		for _, e := range edges {
			marker := "✓"
			if e.SamePkg {
				marker = "~"
			} else if violPkgs[e.CallerPkg] {
				marker = "✗"
			}
			sym := symbolName(res, e.Callee)
			b.WriteString(fmt.Sprintf("    %s %s:%d  %s %s\n",
				marker, e.File, e.Line, string(e.Kind), sym))
		}
	}
	return b.String()
}

type jsonEdge struct {
	CallerPkg string `json:"callerPkg"`
	SymbolID  int    `json:"symbolId"`
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Col       int    `json:"col"`
	SamePkg   bool   `json:"samePkg"`
}

type jsonViolation struct {
	FromPkg string `json:"fromPkg"`
	ToPkg   string `json:"toPkg"`
	Reason  string `json:"reason"`
}

type jsonOutput struct {
	Target     string          `json:"target"`
	PkgPath    string          `json:"pkgPath"`
	SymbolName string          `json:"symbolName,omitempty"`
	Symbols    []graph.Symbol  `json:"symbols"`
	Edges      []jsonEdge      `json:"edges"`
	Violations []jsonViolation `json:"violations"`
}

// FormatJSON returns JSON bytes for the inspect result.
func FormatJSON(res *InspectResult) ([]byte, error) {
	edges := make([]jsonEdge, len(res.Edges))
	for i, e := range res.Edges {
		edges[i] = jsonEdge{
			CallerPkg: e.CallerPkg,
			SymbolID:  e.Callee,
			Symbol:    symbolName(res, e.Callee),
			Kind:      string(e.Kind),
			File:      e.File,
			Line:      e.Line,
			Col:       e.Col,
			SamePkg:   e.SamePkg,
		}
	}
	viols := make([]jsonViolation, len(res.Violations))
	for i, v := range res.Violations {
		viols[i] = jsonViolation{FromPkg: v.FromPkg, ToPkg: v.ToPkg, Reason: v.Rule.Reason}
	}
	return json.MarshalIndent(jsonOutput{
		Target:     res.Target,
		PkgPath:    res.PkgPath,
		SymbolName: res.SymbolName,
		Symbols:    res.Symbols,
		Edges:      edges,
		Violations: viols,
	}, "", "  ")
}

// FormatMarkdown returns Markdown output for the inspect result.
func FormatMarkdown(res *InspectResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %s\n\n", res.Target))
	b.WriteString(fmt.Sprintf("**Package:** `%s`  \n", res.PkgPath))
	b.WriteString(fmt.Sprintf("**References:** %d  \n\n", len(res.Edges)))

	if len(res.Violations) > 0 {
		b.WriteString("### Violations\n\n")
		b.WriteString("| From | To | Reason |\n|---|---|---|\n")
		for _, v := range res.Violations {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n",
				v.FromPkg, v.ToPkg, v.Rule.Reason))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Reference Tree\n\n```\n")
	groups := groupEdgesByPkg(res.Edges)
	for _, pkg := range sortedPkgKeys(groups) {
		b.WriteString(fmt.Sprintf("%s\n", pkg))
		for _, e := range groups[pkg] {
			sym := symbolName(res, e.Callee)
			b.WriteString(fmt.Sprintf("  %s:%d  %s\n", e.File, e.Line, sym))
		}
	}
	b.WriteString("```\n")
	return b.String()
}

// FormatQuickfix returns quickfix-format lines (file:line:col: message).
func FormatQuickfix(res *InspectResult) string {
	var b strings.Builder
	violPkgs := violationPkgSet(res.Violations)
	for _, e := range res.Edges {
		if e.File == "" || e.Line == 0 {
			continue
		}
		sym := symbolName(res, e.Callee)
		msg := fmt.Sprintf("%s.%s (%s)", res.PkgPath, sym, string(e.Kind))
		if violPkgs[e.CallerPkg] {
			msg += " [DENY]"
		}
		b.WriteString(fmt.Sprintf("%s:%d:%d: %s\n", e.File, e.Line, e.Col, msg))
	}
	return b.String()
}

func symbolName(res *InspectResult, id int) string {
	for _, s := range res.Symbols {
		if s.ID == id {
			return s.Name
		}
	}
	return fmt.Sprintf("(id=%d)", id)
}

func groupEdgesByPkg(edges []graph.Edge) map[string][]graph.Edge {
	groups := make(map[string][]graph.Edge)
	for _, e := range edges {
		pkg := e.CallerPkg
		if pkg == "" {
			pkg = "(unknown)"
		}
		groups[pkg] = append(groups[pkg], e)
	}
	return groups
}

func sortedPkgKeys(m map[string][]graph.Edge) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func violationPkgSet(viols []PackageViolation) map[string]bool {
	s := make(map[string]bool, len(viols))
	for _, v := range viols {
		s[v.FromPkg] = true
	}
	return s
}

// Ensure rules import is used (Rule type is embedded in PackageViolation).
var _ rules.Rule
```

Wait, the last line `var _ rules.Rule` will cause a compile error. Remove it. The `rules` import is used via `PackageViolation.Rule rules.Rule` in `resolve.go`. But `format.go` doesn't reference `rules` directly. Let me remove the `rules` import from `format.go`.

- [ ] **Step 4: Fix the `format.go` import**

The `format.go` file above has an unused import `"go.flaticols.dev/gorefactor/internal/rules"`. Remove that import and the `var _ rules.Rule` line. The final imports in `format.go` should be:

```go
import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
GOCACHE=/tmp/gocache go test ./internal/inspect/... -v
```

Expected: all PASS.

- [ ] **Step 6: Run full test suite**

```bash
GOCACHE=/tmp/gocache go test ./...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
jj describe -m "inspect: add FormatText, FormatJSON, FormatMarkdown, FormatQuickfix"
jj new
```

---

### Task 5: CLI entry point and TTY routing

**Files:**
- Create: `cmd/gorefact/inspect.go`
- Modify: `cmd/gorefact/main.go`

- [ ] **Step 1: Create `cmd/gorefact/inspect.go`**

```go
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/rules"
	"go.flaticols.dev/gorefactor/internal/tui"
)

func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		printInspectHelp(fs.Output())
		printFlagDefaults(fs)
	}

	var (
		rulesPath = fs.String("rules", defaultRulesFile, "path to gorefact.rules.toml")
		dir       = fs.String("dir", ".", "working directory")
		format    = fs.String("format", "text", "output format: text|json|md|qf (non-TTY only)")
		tests     = fs.Bool("tests", false, "include test packages")
		filterPkg = fs.String("filter-pkg", "", "only include packages containing this path fragment")
		noTUI     = fs.Bool("no-tui", false, "force CLI output even on TTY")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	target := ""
	if len(fs.Args()) > 0 {
		target = strings.TrimSpace(fs.Args()[0])
	}

	resolvedRulesPath := resolvePath(*dir, *rulesPath)
	var ruleSet []rules.Rule
	if resolvedRulesPath != "" {
		var err error
		ruleSet, err = rules.Parse(resolvedRulesPath)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "parse rules failed: %v\n", err)
			return 1
		}
	}

	cfg := inspect.Config{
		Loader: loader.Config{
			Dir:       *dir,
			Tests:     *tests,
			FilterPkg: *filterPkg,
		},
		Rules: ruleSet,
	}

	if !*noTUI && isTTY() {
		if err := tui.Run(target, cfg); err != nil {
			fmt.Fprintf(stderr, "tui error: %v\n", err)
			return 1
		}
		return 0
	}

	cfg.Loader.Progress = func(stage string) {
		fmt.Fprintf(stderr, "%s...\n", title(stage))
	}

	res, err := inspect.ResolveTarget(target, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "inspect failed: %v\n", err)
		return 1
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "text":
		_, _ = io.WriteString(stdout, inspect.FormatText(res, inspect.FormatOptions{BaseDir: *dir}))
	case "json":
		data, err := inspect.FormatJSON(res)
		if err != nil {
			fmt.Fprintf(stderr, "format json failed: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(append(data, '\n'))
	case "md", "markdown":
		_, _ = io.WriteString(stdout, inspect.FormatMarkdown(res))
	case "qf", "quickfix":
		_, _ = io.WriteString(stdout, inspect.FormatQuickfix(res))
	default:
		fmt.Fprintf(stderr, "unknown format %q\n", *format)
		return 2
	}
	return 0
}

// isTTY returns true when stdout is an interactive terminal.
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func printInspectHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gorefact inspect [target] [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Show everything that imports or references a given package, type, function,")
	fmt.Fprintln(w, "const, or var — full reference tree using T1+T2 engine.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Target formats:")
	fmt.Fprintln(w, "  github.com/acme/tasks              — entire package")
	fmt.Fprintln(w, "  github.com/acme/tasks.Run          — symbol named Run")
	fmt.Fprintln(w, "  github.com/acme/tasks.Engine.Calc  — method Calc on type Engine")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Without a target on a TTY, opens the TUI with an empty search box.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gorefact inspect github.com/acme/tasks")
	fmt.Fprintln(w, "  gorefact inspect github.com/acme/tasks.Engine --format json")
	fmt.Fprintln(w, "  gorefact inspect github.com/acme/tasks --no-tui --format text")
	fmt.Fprintln(w)
}
```

- [ ] **Step 2: Modify `cmd/gorefact/main.go` — add `inspect` case and bare-command TTY routing**

In the `run` function, change the `if len(args) == 0` block (lines 28–31) from:

```go
	if len(args) == 0 {
		printRootHelp(stderr)
		return 2
	}
```

to:

```go
	if len(args) == 0 {
		if isTTY() {
			return runInspect(nil, stdout, stderr)
		}
		printRootHelp(stderr)
		return 2
	}
```

Then in the `switch args[0]` block, add a new case before `"help"`:

```go
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
```

Also add `"inspect"` to `printRootHelp` in `main.go`. Find the line:

```go
	fmt.Fprintln(w, "  check           build the graph and report rule violations")
```

And insert before it:

```go
	fmt.Fprintln(w, "  inspect         explore what imports or references a package/symbol (TUI or text)")
```

And in `runHelp`'s switch, add:

```go
	case "inspect":
		printInspectHelp(stdout)
		return 0
```

- [ ] **Step 3: Verify the build compiles** (tui package doesn't exist yet — expect a compile error referencing `tui.Run`)

```bash
GOCACHE=/tmp/gocache go build ./cmd/gorefact/ 2>&1 | head -5
```

Expected: error about `tui` package not found. That's correct — tui is added in Task 6.

- [ ] **Step 4: Commit (the inspect.go file; main.go changes)**

```bash
jj describe -m "cmd: add inspect subcommand, TTY routing for bare gorefact"
jj new
```

---

### Task 6: TUI styles and core model

**Files:**
- Create: `internal/tui/styles.go`
- Create: `internal/tui/model.go`

- [ ] **Step 1: Create `internal/tui/styles.go`**

```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleTitle       = lipgloss.NewStyle().Bold(true)
	styleActiveTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleActive      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleSelected    = lipgloss.NewStyle().Background(lipgloss.Color("238")).Bold(true)
	styleCurrent     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	styleItem        = lipgloss.NewStyle()
	styleViolation   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleHelp        = lipgloss.NewStyle().Faint(true)
	styleDim         = lipgloss.NewStyle().Faint(true)
	styleDetail      = lipgloss.NewStyle()
	styleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)
```

- [ ] **Step 2: Create `internal/tui/model.go`**

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

// GroupMode controls how reference edges are grouped in the tree pane.
type GroupMode int

const (
	GroupPkg  GroupMode = iota // group by caller package path
	GroupFile                  // group by file path
)

type pane int

const (
	paneSearch pane = iota
	paneList
	paneTree
)

// symbolEntry is one row in the symbol list pane.
type symbolEntry struct {
	sym      graph.Symbol
	refCount int
}

// loadDoneMsg is sent when ResolveTarget completes in the background.
type loadDoneMsg struct {
	result *inspect.InspectResult
	err    error
}

// Model is the Bubble Tea model for the inspect TUI.
type Model struct {
	cfg     inspect.Config
	result  *inspect.InspectResult

	input   textinput.Model
	spinner spinner.Model

	symbols []symbolEntry
	listIdx int

	treeLines []string
	treeIdx   int

	group    GroupMode
	violOnly bool

	active        pane
	width, height int

	loading bool
	err     error
}

// New creates a Model with an optional pre-filled search target.
func New(initialTarget string, cfg inspect.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "package/symbol (e.g. github.com/acme/tasks)"
	ti.SetValue(initialTarget)
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		cfg:     cfg,
		input:   ti,
		spinner: sp,
		active:  paneSearch,
	}
}

// Run starts the full-screen Bubble Tea TUI. Blocks until the user quits.
func Run(initialTarget string, cfg inspect.Config) error {
	m := New(initialTarget, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if strings.TrimSpace(m.input.Value()) != "" {
		m.loading = true
		cmds = append(cmds, doLoad(m.input.Value(), m.cfg), m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func doLoad(target string, cfg inspect.Config) tea.Cmd {
	return func() tea.Msg {
		res, err := inspect.ResolveTarget(target, cfg)
		return loadDoneMsg{result: res, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = m.width - 12
		return m, nil

	case loadDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.result = msg.result
		m = buildSymbolList(m)
		m = updateTree(m)
		if m.active == paneSearch && len(m.symbols) > 0 {
			m.active = paneList
			m.input.Blur()
		}
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.active == paneSearch {
				return m, tea.Quit
			}
			// q in non-search pane goes back to search
			m.active = paneSearch
			m.input.Focus()
			return m, nil

		case "ctrl+q":
			return m, tea.Quit

		case "tab":
			m = cyclePane(m)
			return m, nil

		case "esc":
			if m.active != paneSearch {
				m.active = paneSearch
				m.input.Focus()
			}
			return m, nil

		case "g":
			if m.active != paneSearch && m.result != nil {
				if m.group == GroupPkg {
					m.group = GroupFile
				} else {
					m.group = GroupPkg
				}
				m = updateTree(m)
			}
			return m, nil

		case "f":
			if m.active != paneSearch && m.result != nil {
				m.violOnly = !m.violOnly
				m = updateTree(m)
			}
			return m, nil

		case "enter":
			if m.active == paneSearch {
				q := strings.TrimSpace(m.input.Value())
				if q != "" {
					m.loading = true
					m.err = nil
					return m, tea.Batch(doLoad(q, m.cfg), m.spinner.Tick)
				}
			}
			return m, nil
		}

		// Per-pane navigation
		switch m.active {
		case paneSearch:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case paneList:
			switch msg.String() {
			case "up", "k":
				if m.listIdx > 0 {
					m.listIdx--
					m = updateTree(m)
				}
			case "down", "j":
				if m.listIdx < len(m.symbols)-1 {
					m.listIdx++
					m = updateTree(m)
				}
			}
		case paneTree:
			switch msg.String() {
			case "up", "k":
				if m.treeIdx > 0 {
					m.treeIdx--
				}
			case "down", "j":
				if m.treeIdx < len(m.treeLines)-1 {
					m.treeIdx++
				}
			}
		}
	}
	return m, nil
}

func cyclePane(m Model) Model {
	switch m.active {
	case paneSearch:
		if len(m.symbols) > 0 {
			m.active = paneList
			m.input.Blur()
		}
	case paneList:
		m.active = paneTree
	case paneTree:
		m.active = paneSearch
		m.input.Focus()
	}
	return m
}

func buildSymbolList(m Model) Model {
	if m.result == nil {
		m.symbols = nil
		return m
	}
	counts := make(map[int]int, len(m.result.Edges))
	for _, e := range m.result.Edges {
		counts[e.Callee]++
	}
	m.symbols = make([]symbolEntry, len(m.result.Symbols))
	for i, s := range m.result.Symbols {
		m.symbols[i] = symbolEntry{sym: s, refCount: counts[s.ID]}
	}
	return m
}

func updateTree(m Model) Model {
	if m.result == nil || len(m.symbols) == 0 {
		m.treeLines = nil
		return m
	}
	sel := m.symbols[m.listIdx]
	violPkgs := make(map[string]bool, len(m.result.Violations))
	for _, v := range m.result.Violations {
		violPkgs[v.FromPkg] = true
	}
	m.treeLines = buildTreeLines(m.result.Edges, sel.sym.ID, m.group, m.violOnly, violPkgs)
	m.treeIdx = 0
	return m
}

func (m Model) View() string {
	if m.err != nil {
		return styleError.Render(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
	}

	searchBar := m.renderSearch()

	if m.loading {
		loadLine := fmt.Sprintf("  %s Loading %s...", m.spinner.View(), strings.TrimSpace(m.input.Value()))
		return lipgloss.JoinVertical(lipgloss.Left, searchBar, "", loadLine, "", styleHelp.Render("[ctrl+q] quit"))
	}

	contentH := m.height - 3
	if contentH < 2 {
		contentH = 2
	}

	leftW := m.width / 3
	midW := m.width / 3
	rightW := m.width - leftW - midW
	if rightW < 8 {
		rightW = 8
	}

	left := m.renderList(leftW, contentH)
	mid := m.renderTree(midW, contentH)
	right := m.renderDetail(rightW, contentH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	help := m.renderHelpBar()

	return lipgloss.JoinVertical(lipgloss.Left, searchBar, body, help)
}

func (m Model) renderSearch() string {
	label := "Search"
	if m.active == paneSearch {
		label = styleActive.Render(label)
	}
	return label + ": " + m.input.View()
}

func (m Model) renderList(w, h int) string {
	title := "Symbols"
	if m.active == paneList {
		title = styleActiveTitle.Render(title)
	} else {
		title = styleTitle.Render(title)
	}

	lines := make([]string, 0, h)
	lines = append(lines, padRight(title, w))

	for i, s := range m.symbols {
		if len(lines) >= h {
			break
		}
		label := fmt.Sprintf("%s (%s) [%d]", s.sym.Name, s.sym.Kind, s.refCount)
		label = truncate(label, w-1)
		switch {
		case i == m.listIdx && m.active == paneList:
			lines = append(lines, styleSelected.Width(w).Render(label))
		case i == m.listIdx:
			lines = append(lines, styleCurrent.Render(padRight(label, w)))
		default:
			lines = append(lines, styleItem.Render(padRight(label, w)))
		}
	}

	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderTree(w, h int) string {
	groupName := "pkg"
	if m.group == GroupFile {
		groupName = "file"
	}
	titleStr := fmt.Sprintf("References [group=%s]", groupName)
	if m.active == paneTree {
		titleStr = styleActiveTitle.Render(titleStr)
	} else {
		titleStr = styleTitle.Render(titleStr)
	}

	lines := make([]string, 0, h)
	lines = append(lines, padRight(titleStr, w))

	visible := m.treeLines
	if m.treeIdx > 0 && m.treeIdx < len(visible) {
		visible = visible[m.treeIdx:]
	}

	for _, line := range visible {
		if len(lines) >= h {
			break
		}
		lines = append(lines, truncate(line, w-1))
	}

	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderDetail(w, h int) string {
	titleStr := styleTitle.Render("Detail")
	lines := make([]string, 0, h)
	lines = append(lines, padRight(titleStr, w))

	if m.result != nil && len(m.symbols) > 0 {
		sel := m.symbols[m.listIdx]
		s := sel.sym
		lines = append(lines,
			truncate(fmt.Sprintf("name:  %s", s.Name), w-1),
			truncate(fmt.Sprintf("kind:  %s", s.Kind), w-1),
			truncate(fmt.Sprintf("pkg:   %s", s.Package), w-1),
			truncate(fmt.Sprintf("file:  %s:%d", s.File, s.Line), w-1),
			truncate(fmt.Sprintf("refs:  %d", sel.refCount), w-1),
			"",
		)
		if len(m.result.Violations) > 0 {
			lines = append(lines, styleViolation.Render(fmt.Sprintf("violations (%d):", len(m.result.Violations))))
			for _, v := range m.result.Violations {
				if len(lines) >= h-1 {
					break
				}
				lines = append(lines, styleViolation.Render(truncate("  "+v.FromPkg, w-1)))
				lines = append(lines, styleDim.Render(truncate("  "+v.Rule.Reason, w-1)))
			}
		}
	}

	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHelpBar() string {
	violStr := "off"
	if m.violOnly {
		violStr = "on"
	}
	groupStr := "pkg"
	if m.group == GroupFile {
		groupStr = "file"
	}
	parts := []string{
		"[Tab] pane",
		"[↑↓/jk] nav",
		"[Enter] load",
		"[g] group=" + groupStr,
		"[f] violations=" + violStr,
		"[Esc] search",
		"[q] back/quit",
	}
	return styleHelp.Render(strings.Join(parts, "  "))
}

func truncate(s string, max int) string {
	if max <= 3 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}
```

- [ ] **Step 3: Verify the build**

```bash
GOCACHE=/tmp/gocache go build ./...
```

Expected: may fail with `buildTreeLines undefined` (from `tree_render.go`, added in Task 7). Alternatively, add a stub in `model.go`:

```go
// Temporary stub — replaced by tree_render.go in Task 7.
func buildTreeLines(edges []graph.Edge, symID int, group GroupMode, violOnly bool, violPkgs map[string]bool) []string {
	return []string{"(tree rendering coming in Task 7)"}
}
```

Add this stub at the bottom of `model.go` temporarily so the build succeeds.

- [ ] **Step 4: Verify build succeeds with the stub**

```bash
GOCACHE=/tmp/gocache go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
jj describe -m "tui: add Bubble Tea model with search, list, tree, and detail panes"
jj new
```

---

### Task 7: TUI tree rendering and keybindings

**Files:**
- Create: `internal/tui/tree_render.go`
- Create: `internal/tui/tree_render_test.go`
- Modify: `internal/tui/model.go` — remove the temporary stub

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/tree_render_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
)

func TestBuildTreeLines_GroupPkg(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, Kind: graph.EdgeCall, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42},
		{Callee: 100, Kind: graph.EdgeTypeRef, CallerPkg: "example.com/service", File: "service/s.go", Line: 10},
	}
	violPkgs := map[string]bool{"example.com/handler": true}

	lines := buildTreeLines(edges, 100, GroupPkg, false, violPkgs)

	if len(lines) == 0 {
		t.Fatal("expected lines, got none")
	}
	foundViol := false
	foundClean := false
	for _, l := range lines {
		if strings.Contains(l, "handler") && strings.Contains(l, "✗") {
			foundViol = true
		}
		if strings.Contains(l, "service") && strings.Contains(l, "✓") {
			foundClean = true
		}
	}
	if !foundViol {
		t.Errorf("expected ✗ marker for handler, lines: %v", lines)
	}
	if !foundClean {
		t.Errorf("expected ✓ marker for service, lines: %v", lines)
	}
}

func TestBuildTreeLines_ViolOnly(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 10},
		{Callee: 100, CallerPkg: "example.com/service", File: "service/s.go", Line: 20},
	}
	violPkgs := map[string]bool{"example.com/handler": true}

	lines := buildTreeLines(edges, 100, GroupPkg, true, violPkgs)
	for _, l := range lines {
		if strings.Contains(l, "service") {
			t.Errorf("violOnly=true should hide clean package, but got: %q", l)
		}
	}
}

func TestBuildTreeLines_NoEdges(t *testing.T) {
	lines := buildTreeLines(nil, 100, GroupPkg, false, nil)
	if len(lines) == 0 {
		t.Fatal("expected at least one line (empty message), got none")
	}
}

func TestBuildTreeLines_GroupFile(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42},
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 55},
	}
	violPkgs := map[string]bool{}

	lines := buildTreeLines(edges, 100, GroupFile, false, violPkgs)
	// Both edges are in the same file — should appear under one group header.
	fileHeaders := 0
	for _, l := range lines {
		if strings.Contains(l, "handler/h.go") && !strings.Contains(l, ":42") && !strings.Contains(l, ":55") {
			fileHeaders++
		}
	}
	if fileHeaders != 1 {
		t.Errorf("expected 1 file header for handler/h.go, got %d; lines: %v", fileHeaders, lines)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOCACHE=/tmp/gocache go test ./internal/tui/... -run "TestBuildTreeLines" -v 2>&1 | head -15
```

Expected: FAIL because `buildTreeLines` returns the stub `"(tree rendering coming in Task 7)"` and doesn't have the real logic.

- [ ] **Step 3: Create `internal/tui/tree_render.go`**

```go
package tui

import (
	"fmt"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// buildTreeLines converts reference edges for a specific symbol into renderable
// text lines for the tree pane, grouped by pkg or file.
func buildTreeLines(
	edges []graph.Edge,
	symID int,
	group GroupMode,
	violOnly bool,
	violPkgs map[string]bool,
) []string {
	var relevant []graph.Edge
	for _, e := range edges {
		if e.Callee == symID {
			relevant = append(relevant, e)
		}
	}
	if len(relevant) == 0 {
		return []string{"  (no references)"}
	}
	switch group {
	case GroupFile:
		return treeByFile(relevant, violOnly, violPkgs)
	default:
		return treeByPkg(relevant, violOnly, violPkgs)
	}
}

func treeByPkg(edges []graph.Edge, violOnly bool, violPkgs map[string]bool) []string {
	groups := make(map[string][]graph.Edge)
	for _, e := range edges {
		pkg := e.CallerPkg
		if pkg == "" {
			pkg = "(unknown)"
		}
		groups[pkg] = append(groups[pkg], e)
	}
	pkgs := make([]string, 0, len(groups))
	for p := range groups {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	var lines []string
	for _, pkg := range pkgs {
		es := groups[pkg]
		isViol := violPkgs[pkg]
		if violOnly && !isViol {
			continue
		}
		marker := "✓ "
		if isViol {
			marker = styleViolation.Render("✗") + " "
		}
		lines = append(lines, fmt.Sprintf("  %s%s (%d refs)", marker, pkg, len(es)))
		for _, e := range es {
			sameMark := ""
			if e.SamePkg {
				sameMark = " ~"
			}
			lines = append(lines, fmt.Sprintf("      %s:%d%s", e.File, e.Line, sameMark))
		}
	}
	return lines
}

func treeByFile(edges []graph.Edge, violOnly bool, violPkgs map[string]bool) []string {
	type fileKey struct{ file, pkg string }
	groups := make(map[fileKey][]graph.Edge)
	for _, e := range edges {
		k := fileKey{file: e.File, pkg: e.CallerPkg}
		groups[k] = append(groups[k], e)
	}
	keys := make([]fileKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].file != keys[j].file {
			return keys[i].file < keys[j].file
		}
		return keys[i].pkg < keys[j].pkg
	})

	var lines []string
	for _, k := range keys {
		es := groups[k]
		isViol := violPkgs[k.pkg]
		if violOnly && !isViol {
			continue
		}
		marker := "✓"
		if isViol {
			marker = styleViolation.Render("✗")
		}
		lines = append(lines, fmt.Sprintf("  %s %s", marker, k.file))
		for _, e := range es {
			lines = append(lines, fmt.Sprintf("      :%d  %s", e.Line, strings.TrimSpace(string(e.Kind))))
		}
	}
	return lines
}
```

- [ ] **Step 4: Remove the stub from `internal/tui/model.go`**

Find and delete these lines at the bottom of `model.go`:

```go
// Temporary stub — replaced by tree_render.go in Task 7.
func buildTreeLines(edges []graph.Edge, symID int, group GroupMode, violOnly bool, violPkgs map[string]bool) []string {
	return []string{"(tree rendering coming in Task 7)"}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
GOCACHE=/tmp/gocache go test ./internal/tui/... -v
```

Expected: `TestBuildTreeLines_GroupPkg`, `TestBuildTreeLines_ViolOnly`, `TestBuildTreeLines_NoEdges`, `TestBuildTreeLines_GroupFile` all PASS.

- [ ] **Step 6: Run full test suite**

```bash
GOCACHE=/tmp/gocache go test ./...
```

Expected: all green.

- [ ] **Step 7: Smoke-test the inspect command**

```bash
go install ./cmd/gorefact
gorefact inspect --no-tui --format text go.flaticols.dev/gorefactor/internal/graph 2>/dev/null
```

Expected: output showing packages that import `internal/graph`, their reference edges, and any violations. Output should not be empty (the project itself imports `internal/graph` in multiple places).

- [ ] **Step 8: Commit**

```bash
jj describe -m "tui: add tree_render with pkg/file grouping and violation markers"
jj new
```
