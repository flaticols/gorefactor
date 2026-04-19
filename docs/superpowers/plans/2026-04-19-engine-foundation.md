# Engine Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the graph engine with a three-tier architecture — T1 (package import graph), T2 (AST reference walker), T3 (SSA call graph) — and extend the data model to support `Symbol` nodes and typed `EdgeKind` edges.

**Architecture:** T1 uses `go/packages` with import-only loading for fast full-service package graphs. T2 uses `go/packages` with full type info (`NeedSyntax | NeedTypes | NeedTypesInfo`) for on-demand symbol reference extraction per target package. T3 is the existing SSA/CHA engine moved to `internal/loader/ssa.go`. All three are wired through a unified `loader.Load()` entry point that replaces `graph.Build()`.

**Tech Stack:** `go/packages`, `go/ast`, `go/types`, `golang.org/x/tools/go/callgraph/cha`, `golang.org/x/tools/go/ssa`, `golang.org/x/tools/go/ssautil`

---

## File Map

| Action | Path | Responsibility |
|--------|------|---------------|
| Modify | `internal/graph/types.go` | Add `EdgeKind`, `Symbol`; extend `Edge` with `Kind`+`SamePkg`; extend `Graph` with `Symbols` + symbol index |
| Modify | `internal/graph/types_test.go` | Tests for new Symbol indexing and EdgeKind filtering |
| Create | `internal/loader/pkggraph.go` | T1: fast package import graph (`PackageNode`, `PackageGraph`, `buildPackageGraph`) |
| Create | `internal/loader/pkggraph_test.go` | Tests for T1 ImportersOf, AllPaths |
| Create | `internal/loader/refs.go` | T2: AST reference walker (`walkRefs`, `enclosingFunc`, `classifyRef`) |
| Create | `internal/loader/refs_test.go` | Tests for T2 symbol extraction and edge classification |
| Create | `internal/loader/ssa.go` | T3: SSA/CHA engine (content of `internal/graph/build.go`, adapted) |
| Delete | `internal/graph/build.go` | Replaced by `internal/loader/ssa.go` |
| Create | `internal/loader/loader.go` | Unified `Config`, `Depth`, `Result`, `Load()` entry point |
| Modify | `cmd/gorefact/main.go` | Replace `graph.Build(graph.BuildConfig{…})` with `loader.Load(loader.Config{…})` |

---

## Task 1: Extend graph data model

**Files:**
- Modify: `internal/graph/types.go`
- Modify: `internal/graph/types_test.go`

- [ ] **Step 1: Write failing tests for EdgeKind and Symbol**

Add to `internal/graph/types_test.go`:

```go
func TestEdgeKindOnGraph(t *testing.T) {
	g := &Graph{
		Funcs: []Func{
			{ID: 1, Name: "Caller", Package: "a", File: "a/a.go", Line: 1},
			{ID: 2, Name: "Callee", Package: "b", File: "b/b.go", Line: 1},
		},
		Edges: []Edge{
			{Caller: 1, Callee: 2, Kind: EdgeCall, File: "a/a.go", Line: 5},
		},
	}
	g.Index()
	callers := g.CallersOf(g.Funcs[1])
	if len(callers) != 1 {
		t.Fatalf("CallersOf len = %d, want 1", len(callers))
	}
}

func TestSymbolIndexing(t *testing.T) {
	g := &Graph{
		Symbols: []Symbol{
			{ID: 10, Kind: "var", Name: "ErrNotFound", Package: "tasks", File: "tasks/errors.go", Line: 5, Exported: true},
			{ID: 11, Kind: "const", Name: "MaxRetries", Package: "tasks", File: "tasks/config.go", Line: 3, Exported: true},
		},
	}
	g.Index()

	sym, ok := g.SymbolByID(10)
	if !ok || sym.Name != "ErrNotFound" {
		t.Fatalf("SymbolByID(10) = %v, %v", sym, ok)
	}

	syms := g.SymbolsInPackage("tasks")
	if len(syms) != 2 {
		t.Fatalf("SymbolsInPackage len = %d, want 2", len(syms))
	}
}

func TestEdgeSamePkgFlag(t *testing.T) {
	g := &Graph{
		Funcs: []Func{
			{ID: 1, Name: "A", Package: "pkg", File: "pkg/a.go", Line: 1},
			{ID: 2, Name: "B", Package: "pkg", File: "pkg/b.go", Line: 1},
		},
		Edges: []Edge{
			{Caller: 1, Callee: 2, Kind: EdgeCall, SamePkg: true, File: "pkg/a.go", Line: 5},
		},
	}
	g.Index()
	if !g.Edges[0].SamePkg {
		t.Fatal("expected SamePkg=true")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
GOCACHE=/tmp/gocache go test ./internal/graph/ -run "TestEdgeKindOnGraph|TestSymbolIndexing|TestEdgeSamePkgFlag" -v
```

Expected: compile error — `EdgeCall`, `Symbol`, `SamePkg` undefined.

- [ ] **Step 3: Add EdgeKind, Symbol, update Edge in types.go**

At the top of `internal/graph/types.go`, after the package declaration, add:

```go
// EdgeKind classifies the nature of a reference edge.
type EdgeKind string

const (
	EdgeCall    EdgeKind = "call"
	EdgeRead    EdgeKind = "read"
	EdgeWrite   EdgeKind = "write"
	EdgeTypeRef EdgeKind = "typeref"
)

// Symbol represents a non-function declaration: var, const, or type.
type Symbol struct {
	ID       int
	Kind     string // "var" | "const" | "type"
	Name     string
	Package  string
	File     string
	Line     int
	Exported bool
}
```

Update the `Edge` struct to add two fields after `Callee int`:

```go
type Edge struct {
	Caller  int
	Callee  int
	Kind    EdgeKind // call, read, write, typeref; empty treated as call
	SamePkg bool     // true when caller and callee share the same package
	File    string
	Line    int
	Col     int
	Dynamic bool
}
```

- [ ] **Step 4: Add Symbols to Graph and extend Index()**

In the `Graph` struct, add after `Edges []Edge`:

```go
Symbols []Symbol

bySymbolID  map[int]int
bySymbolPkg map[string][]int
```

In `Index()`, after the existing edge-indexing loop, add:

```go
g.bySymbolID = make(map[int]int, len(g.Symbols))
g.bySymbolPkg = make(map[string][]int)
for i := range g.Symbols {
	s := g.Symbols[i]
	g.bySymbolID[s.ID] = i
	g.bySymbolPkg[s.Package] = append(g.bySymbolPkg[s.Package], s.ID)
}
for pkg, ids := range g.bySymbolPkg {
	sort.Ints(ids)
	g.bySymbolPkg[pkg] = uniqueInts(ids)
}
```

- [ ] **Step 5: Add SymbolByID and SymbolsInPackage methods**

```go
// SymbolByID returns the symbol for the given identifier.
func (g *Graph) SymbolByID(id int) (Symbol, bool) {
	if g.bySymbolID == nil {
		g.Index()
	}
	i, ok := g.bySymbolID[id]
	if !ok {
		return Symbol{}, false
	}
	return g.Symbols[i], true
}

// SymbolsInPackage returns all symbols declared in the given package.
func (g *Graph) SymbolsInPackage(pkg string) []Symbol {
	if g.bySymbolPkg == nil {
		g.Index()
	}
	ids := g.bySymbolPkg[pkg]
	out := make([]Symbol, 0, len(ids))
	for _, id := range ids {
		if s, ok := g.SymbolByID(id); ok {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 6: Run tests to confirm pass**

```bash
GOCACHE=/tmp/gocache go test ./internal/graph/ -v
```

Expected: all tests pass including the new three.

- [ ] **Step 7: Commit**

```bash
jj describe -m "graph: add EdgeKind, Symbol, extend Edge and Graph index"
jj new
```

---

## Task 2: T1 — Package import graph loader

**Files:**
- Create: `internal/loader/pkggraph.go`
- Create: `internal/loader/pkggraph_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/loader/pkggraph_test.go`:

```go
package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/loader"
)

func TestBuildPackageGraph(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

var ErrNotFound = fmt.Errorf("not found")
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() error { return alpha.ErrNotFound }
`)

	pg, err := loader.BuildPackageGraph(loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	})
	if err != nil {
		t.Fatalf("BuildPackageGraph() error = %v", err)
	}

	importers := pg.ImportersOf("example.com/test/alpha")
	if len(importers) != 1 || importers[0].Path != "example.com/test/beta" {
		t.Fatalf("ImportersOf = %v", importers)
	}

	paths := pg.AllPaths()
	if len(paths) < 2 {
		t.Fatalf("AllPaths len = %d, want >= 2", len(paths))
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
GOCACHE=/tmp/gocache go test ./internal/loader/ -run TestBuildPackageGraph -v
```

Expected: compile error — package `loader` does not exist.

- [ ] **Step 3: Create internal/loader/pkggraph.go**

```go
package loader

import (
	"sort"

	"golang.org/x/tools/go/packages"
)

// PackageNode is one node in the T1 import graph.
type PackageNode struct {
	Path    string
	Name    string
	Module  string
	Imports []string // direct import paths, sorted
}

// PackageGraph is the result of a T1 load.
type PackageGraph struct {
	Nodes map[string]*PackageNode // keyed by import path
}

// ImportersOf returns all packages that directly import targetPath, sorted by path.
func (pg *PackageGraph) ImportersOf(targetPath string) []*PackageNode {
	var out []*PackageNode
	for _, node := range pg.Nodes {
		for _, imp := range node.Imports {
			if imp == targetPath {
				out = append(out, node)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// AllPaths returns all package paths in the graph, sorted.
func (pg *PackageGraph) AllPaths() []string {
	paths := make([]string, 0, len(pg.Nodes))
	for p := range pg.Nodes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// BuildPackageGraph runs a T1 load: package names and imports only.
// Fast enough for 90K-function services.
func BuildPackageGraph(cfg Config) (*PackageGraph, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedModule,
		Dir:  cfg.Dir,
	}, cfg.patterns()...)
	if err != nil {
		return nil, err
	}

	pg := &PackageGraph{Nodes: make(map[string]*PackageNode)}
	seen := make(map[string]bool)

	var walk func(pkg *packages.Package)
	walk = func(pkg *packages.Package) {
		if pkg == nil || seen[pkg.PkgPath] || pkg.PkgPath == "" {
			return
		}
		seen[pkg.PkgPath] = true

		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}
		sort.Strings(imports)

		module := ""
		if pkg.Module != nil {
			module = pkg.Module.Path
		}

		pg.Nodes[pkg.PkgPath] = &PackageNode{
			Path:    pkg.PkgPath,
			Name:    pkg.Name,
			Module:  module,
			Imports: imports,
		}

		for _, dep := range pkg.Imports {
			walk(dep)
		}
	}

	for _, pkg := range pkgs {
		walk(pkg)
	}

	return pg, nil
}
```

- [ ] **Step 4: Create internal/loader/loader.go (Config only, for now)**

```go
package loader

import "strings"

// Config configures any loader tier.
type Config struct {
	Dir       string
	Tests     bool
	FilterPkg string
	Patterns  []string
	Progress  func(string)
}

func (c Config) patterns() []string {
	if len(c.Patterns) == 0 {
		return []string{"./..."}
	}
	return c.Patterns
}

func (c Config) progress(stage string) {
	if c.Progress != nil {
		c.Progress(stage)
	}
}

func (c Config) filterPkg() string {
	return strings.TrimSpace(c.FilterPkg)
}
```

- [ ] **Step 5: Run tests to confirm pass**

```bash
GOCACHE=/tmp/gocache go test ./internal/loader/ -run TestBuildPackageGraph -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
jj describe -m "loader: add T1 package import graph (BuildPackageGraph)"
jj new
```

---

## Task 3: T2 — AST reference walker

**Files:**
- Create: `internal/loader/refs.go`
- Create: `internal/loader/refs_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/loader/refs_test.go`:

```go
package loader_test

import (
	"path/filepath"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/loader"
)

func TestWalkRefs_FuncCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

func DoWork() {}
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Run() { alpha.DoWork() }
`)

	result, err := loader.WalkRefs("example.com/test/alpha", []string{"example.com/test/beta"}, loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, 100)
	if err != nil {
		t.Fatalf("WalkRefs() error = %v", err)
	}

	var foundSym bool
	for _, s := range result.Symbols {
		if s.Name == "DoWork" && s.Kind == "func" {
			foundSym = true
		}
	}
	if !foundSym {
		t.Fatalf("expected DoWork symbol, got %v", result.Symbols)
	}

	var foundEdge bool
	for _, e := range result.Edges {
		if e.Kind == graph.EdgeCall {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected a call edge, got %v", result.Edges)
	}
}

func TestWalkRefs_VarRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

import "errors"

var ErrNotFound = errors.New("not found")
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func Check(err error) bool { return err == alpha.ErrNotFound }
`)

	result, err := loader.WalkRefs("example.com/test/alpha", []string{"example.com/test/beta"}, loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, 100)
	if err != nil {
		t.Fatalf("WalkRefs() error = %v", err)
	}

	var foundVar bool
	for _, s := range result.Symbols {
		if s.Name == "ErrNotFound" && s.Kind == "var" {
			foundVar = true
		}
	}
	if !foundVar {
		t.Fatalf("expected ErrNotFound symbol, got %v", result.Symbols)
	}

	var foundEdge bool
	for _, e := range result.Edges {
		if e.Kind == graph.EdgeRead {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected a read edge, got %v", result.Edges)
	}
}

func TestWalkRefs_TypeRef(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.26.2\n")
	writeFile(t, filepath.Join(dir, "alpha", "alpha.go"), `package alpha

type Config struct{ Timeout int }
`)
	writeFile(t, filepath.Join(dir, "beta", "beta.go"), `package beta

import "example.com/test/alpha"

func New(cfg alpha.Config) {}
`)

	result, err := loader.WalkRefs("example.com/test/alpha", []string{"example.com/test/beta"}, loader.Config{
		Dir:      dir,
		Patterns: []string{"./..."},
	}, 100)
	if err != nil {
		t.Fatalf("WalkRefs() error = %v", err)
	}

	var foundType bool
	for _, s := range result.Symbols {
		if s.Name == "Config" && s.Kind == "type" {
			foundType = true
		}
	}
	if !foundType {
		t.Fatalf("expected Config type symbol, got %v", result.Symbols)
	}

	var foundEdge bool
	for _, e := range result.Edges {
		if e.Kind == graph.EdgeTypeRef {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected a typeref edge, got %v", result.Edges)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
GOCACHE=/tmp/gocache go test ./internal/loader/ -run "TestWalkRefs" -v
```

Expected: compile error — `loader.WalkRefs` undefined.

- [ ] **Step 3: Create internal/loader/refs.go**

```go
package loader

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
	"golang.org/x/tools/go/packages"
)

// RefResult holds symbols declared in targetPkg and reference edges pointing to them.
type RefResult struct {
	Symbols []graph.Symbol
	Edges   []graph.Edge
}

// WalkRefs loads importer packages and collects all references to exported
// symbols declared in targetPkg. startID is the first integer ID to assign
// to new Symbol nodes (must not overlap with existing Func IDs in the Graph).
func WalkRefs(targetPkg string, importerPaths []string, cfg Config, startID int) (*RefResult, error) {
	if len(importerPaths) == 0 {
		return &RefResult{}, nil
	}

	// Load the target package to enumerate its exported symbols.
	targetPkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  cfg.Dir,
	}, targetPkg)
	if err != nil {
		return nil, err
	}
	if len(targetPkgs) == 0 || targetPkgs[0].Types == nil {
		return &RefResult{}, nil
	}

	tp := targetPkgs[0]
	nextID := startID
	objToSym := make(map[types.Object]*graph.Symbol)

	scope := tp.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		kind := symKind(obj)
		if kind == "" {
			continue
		}
		pos := tp.Fset.Position(obj.Pos())
		sym := &graph.Symbol{
			ID:       nextID,
			Kind:     kind,
			Name:     name,
			Package:  tp.PkgPath,
			File:     cleanPath(cfg.Dir, pos.Filename),
			Line:     pos.Line,
			Exported: true,
		}
		objToSym[obj] = sym
		nextID++
	}

	result := &RefResult{}
	for _, sym := range objToSym {
		result.Symbols = append(result.Symbols, *sym)
	}
	sort.Slice(result.Symbols, func(i, j int) bool {
		return result.Symbols[i].ID < result.Symbols[j].ID
	})

	// Load importer packages with full type info.
	importerPkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports,
		Dir: cfg.Dir,
	}, importerPaths...)
	if err != nil {
		return nil, err
	}

	for _, ipkg := range importerPkgs {
		if ipkg.TypesInfo == nil {
			continue
		}
		samePkg := ipkg.PkgPath == targetPkg

		for ident, obj := range ipkg.TypesInfo.Uses {
			sym, ok := objToSym[obj]
			if !ok {
				continue
			}
			pos := ipkg.Fset.Position(ident.Pos())

			// callerFuncID is 0 for Plan 1: edges record the reference site
			// (File/Line) but not the enclosing Func ID, which requires
			// either SSA (T3) or a separate AST func-extraction pass added
			// in Plan 2 (inspect command).
			callerFuncID := 0

			result.Edges = append(result.Edges, graph.Edge{
				Caller:  callerFuncID,
				Callee:  sym.ID,
				Kind:    classifyRef(obj),
				SamePkg: samePkg,
				File:    cleanPath(cfg.Dir, pos.Filename),
				Line:    pos.Line,
				Col:     pos.Column,
			})
		}
	}

	return result, nil
}

// symKind maps a types.Object to the Symbol.Kind string.
func symKind(obj types.Object) string {
	switch obj.(type) {
	case *types.Func:
		return "func"
	case *types.Var:
		return "var"
	case *types.Const:
		return "const"
	case *types.TypeName:
		return "type"
	}
	return ""
}

// classifyRef decides the EdgeKind for a reference to obj.
func classifyRef(obj types.Object) graph.EdgeKind {
	switch obj.(type) {
	case *types.Func:
		return graph.EdgeCall
	case *types.TypeName:
		return graph.EdgeTypeRef
	case *types.Const:
		return graph.EdgeRead
	case *types.Var:
		return graph.EdgeRead
	}
	return graph.EdgeRead
}

// enclosingFuncDecl returns the innermost *ast.FuncDecl covering pos, or nil.
func enclosingFuncDecl(file *ast.File, pos token.Pos) *ast.FuncDecl {
	var result *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if n.Pos() > pos || n.End() < pos {
			return false
		}
		if fd, ok := n.(*ast.FuncDecl); ok {
			result = fd
		}
		return true
	})
	return result
}

// cleanPath is defined in loader.go — do not redeclare here.
```

- [ ] **Step 4: Run tests to confirm pass**

```bash
GOCACHE=/tmp/gocache go test ./internal/loader/ -run "TestWalkRefs" -v
```

Expected: all three WalkRefs tests pass.

- [ ] **Step 5: Commit**

```bash
jj describe -m "loader: add T2 AST reference walker (WalkRefs)"
jj new
```

---

## Task 4: Migrate T3 SSA loader

**Files:**
- Create: `internal/loader/ssa.go`
- Delete: `internal/graph/build.go`
- Modify: `internal/loader/loader.go` (extend with Load + Result)

- [ ] **Step 1: Create internal/loader/ssa.go**

Move the entire content of `internal/graph/build.go` into a new file `internal/loader/ssa.go`, with these changes:
- Change `package graph` → `package loader`
- Change import `"go.flaticols.dev/gorefactor/internal/graph"` added, all `Func`/`Edge`/`Graph` references prefixed with `graph.`
- Rename `Build` → `buildSSA`
- Rename `BuildConfig` → use `Config` (already defined in loader.go)
- Set `Kind: graph.EdgeCall` on every `graph.Edge` created in the edge-building loop
- Set `SamePkg` on each edge: `SamePkg: callerFunc.Package == calleeFunc.Package` (look up both funcs after `addFunc`)

Full file:

```go
package loader

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func buildSSA(cfg Config) (*graph.Graph, error) {
	progress := cfg.progress
	patterns := cfg.patterns()

	progress("loading packages")
	fset := token.NewFileSet()
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.LoadAllSyntax | packages.NeedModule,
		Dir:   cfg.Dir,
		Tests: cfg.Tests,
		Fset:  fset,
	}, patterns...)
	if err != nil {
		return nil, err
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		return nil, fmt.Errorf("package load failed: %d errors", n)
	}
	pkgs = filterPkgs(pkgs, cfg.filterPkg())
	if len(pkgs) == 0 {
		return &graph.Graph{}, nil
	}
	packageModules := pkgModuleMap(pkgs)

	progress("building ssa")
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	progress("building call graph")
	cg := cha.CallGraph(prog)

	g := &graph.Graph{}
	funcIDs := make(map[*ssa.Function]int)
	nextID := 1

	addFunc := func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		if _, ok := funcIDs[fn]; ok {
			return
		}
		decl := ssaFuncToFunc(fn, fset, nextID, cfg.Dir, packageModules)
		if decl == nil {
			return
		}
		funcIDs[fn] = nextID
		g.Funcs = append(g.Funcs, *decl)
		nextID++
	}

	for _, pkg := range ssaPkgs {
		if pkg == nil {
			continue
		}
		for _, mem := range pkg.Members {
			if fn, ok := mem.(*ssa.Function); ok {
				addFunc(fn)
			}
		}
	}

	for fn := range ssautil.AllFunctions(prog) {
		addFunc(fn)
	}

	lookup := func(fn *ssa.Function) (int, bool) {
		if fn == nil {
			return 0, false
		}
		id, ok := funcIDs[fn]
		return id, ok
	}

	// Build a temporary pkg lookup for SamePkg detection.
	funcPkg := make(map[int]string, len(g.Funcs))
	for _, f := range g.Funcs {
		funcPkg[f.ID] = f.Package
	}

	for _, node := range cg.Nodes {
		if node == nil || node.Func == nil {
			continue
		}
		callerID, ok := lookup(node.Func)
		if !ok {
			continue
		}
		for _, edge := range node.Out {
			if edge == nil || edge.Callee == nil || edge.Callee.Func == nil || edge.Site == nil {
				continue
			}
			calleeID, ok := lookup(edge.Callee.Func)
			if !ok {
				continue
			}
			pos := fset.Position(edge.Site.Pos())
			g.Edges = append(g.Edges, graph.Edge{
				Caller:  callerID,
				Callee:  calleeID,
				Kind:    graph.EdgeCall,
				SamePkg: funcPkg[callerID] == funcPkg[calleeID],
				File:    cleanPath(cfg.Dir, pos.Filename),
				Line:    pos.Line,
				Col:     pos.Column,
				Dynamic: edge.Site.Common() != nil && edge.Site.Common().IsInvoke(),
			})
		}
	}

	if cfg.filterPkg() != "" {
		g = filterGraphByPkg(g, cfg.filterPkg())
	}

	sort.Slice(g.Funcs, func(i, j int) bool {
		a, b := g.Funcs[i], g.Funcs[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})

	oldToNew := make(map[int]int, len(g.Funcs))
	for i := range g.Funcs {
		oldID := g.Funcs[i].ID
		newID := i + 1
		oldToNew[oldID] = newID
		g.Funcs[i].ID = newID
	}
	for i := range g.Edges {
		if newID, ok := oldToNew[g.Edges[i].Caller]; ok {
			g.Edges[i].Caller = newID
		}
		if newID, ok := oldToNew[g.Edges[i].Callee]; ok {
			g.Edges[i].Callee = newID
		}
	}

	g.Index()
	progress("done")
	return g, nil
}

func filterPkgs(pkgs []*packages.Package, filter string) []*packages.Package {
	if filter == "" {
		return pkgs
	}
	out := make([]*packages.Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if pkg != nil && strings.Contains(pkg.PkgPath, filter) {
			out = append(out, pkg)
		}
	}
	return out
}

func filterGraphByPkg(g *graph.Graph, filter string) *graph.Graph {
	if filter == "" || g == nil {
		return g
	}
	allowed := make(map[int]struct{})
	out := &graph.Graph{}
	for _, fn := range g.Funcs {
		if strings.Contains(fn.Package, filter) {
			allowed[fn.ID] = struct{}{}
			out.Funcs = append(out.Funcs, fn)
		}
	}
	for _, edge := range g.Edges {
		_, callerOK := allowed[edge.Caller]
		_, calleeOK := allowed[edge.Callee]
		if callerOK && calleeOK {
			out.Edges = append(out.Edges, edge)
		}
	}
	return out
}

func ssaFuncToFunc(fn *ssa.Function, fset *token.FileSet, id int, baseDir string, packageModules map[string]string) *graph.Func {
	if fn == nil || fn.Pos() == token.NoPos {
		return nil
	}
	pos := fset.Position(fn.Pos())
	if pos.Filename == "" {
		return nil
	}
	pkgPath := ""
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		pkgPath = fn.Pkg.Pkg.Path()
	}

	name := fn.Name()
	if strings.HasPrefix(name, "init#") {
		name = "init"
	}

	recv := ""
	if sig := fn.Signature; sig != nil && sig.Recv() != nil {
		recv = formatRecv(sig.Recv().Type())
	}

	if pkgPath == "" && name != "init" {
		return nil
	}

	return &graph.Func{
		ID:        id,
		Module:    packageModules[pkgPath],
		Name:      name,
		Receiver:  recv,
		Signature: fn.String(),
		Package:   pkgPath,
		File:      cleanPath(baseDir, pos.Filename),
		Line:      pos.Line,
		Col:       pos.Column,
		Exported:  ast.IsExported(name),
	}
}

// cleanPath is defined in loader.go — shared across the loader package.

func pkgModuleMap(pkgs []*packages.Package) map[string]string {
	out := make(map[string]string)
	seen := make(map[*packages.Package]struct{})

	var walk func(pkg *packages.Package)
	walk = func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		if _, ok := seen[pkg]; ok {
			return
		}
		seen[pkg] = struct{}{}
		if pkg.PkgPath != "" && pkg.Module != nil {
			out[pkg.PkgPath] = pkg.Module.Path
		}
		for _, imported := range pkg.Imports {
			walk(imported)
		}
	}
	for _, pkg := range pkgs {
		walk(pkg)
	}
	return out
}

func formatRecv(t types.Type) string {
	switch tt := t.(type) {
	case *types.Pointer:
		return "*" + formatRecv(tt.Elem())
	case *types.Named:
		return tt.Obj().Name()
	default:
		s := t.String()
		s = strings.TrimPrefix(s, "type ")
		return filepath.Base(s)
	}
}
```

- [ ] **Step 2: Delete internal/graph/build.go**

```bash
rm /path/to/internal/graph/build.go
```

(In jj, tracked files are deleted when removed from disk. Use `rm internal/graph/build.go`.)

- [ ] **Step 3: Extend loader.go with Result and Load()**

Append to `internal/loader/loader.go`:

```go
// Depth controls which tiers are activated.
type Depth int

const (
	// DepthFull runs T1 + T3 (SSA). Used by check and serve commands.
	DepthFull Depth = iota
	// DepthFast runs T1 only. T2 is invoked separately via WalkRefs.
	DepthFast
)

// Result is the output of Load.
type Result struct {
	// Packages is the T1 package import graph. Always populated.
	Packages *PackageGraph
	// Graph is the T3 SSA call graph. Populated when Depth == DepthFull.
	Graph *graph.Graph
}

// Load runs T1 always. When cfg.Depth == DepthFull, also runs T3 (SSA/CHA).
func Load(cfg Config, depth Depth) (*Result, error) {
	cfg.progress("loading package graph")
	pg, err := BuildPackageGraph(cfg)
	if err != nil {
		return nil, err
	}

	result := &Result{Packages: pg}

	if depth == DepthFull {
		g, err := buildSSA(cfg)
		if err != nil {
			return nil, err
		}
		result.Graph = g
	}

	return result, nil
}
```

Replace the entire `internal/loader/loader.go` content with (adds `graph` import and moves `cleanPath` here so it is shared by `refs.go` and `ssa.go` without duplication):

```go
package loader

import (
	"path/filepath"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// Config configures any loader tier.
type Config struct {
	Dir       string
	Tests     bool
	FilterPkg string
	Patterns  []string
	Progress  func(string)
}

func (c Config) patterns() []string {
	if len(c.Patterns) == 0 {
		return []string{"./..."}
	}
	return c.Patterns
}

func (c Config) progress(stage string) {
	if c.Progress != nil {
		c.Progress(stage)
	}
}

func (c Config) filterPkg() string {
	return strings.TrimSpace(c.FilterPkg)
}

// cleanPath makes file paths relative to base for stable output.
func cleanPath(base, file string) string {
	if file == "" {
		return ""
	}
	file = filepath.Clean(file)
	if base == "" {
		return filepath.ToSlash(file)
	}
	if rel, err := filepath.Rel(base, file); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}

// Depth controls which tiers are activated.
type Depth int

const (
	// DepthFull runs T1 + T3 (SSA). Used by check and serve commands.
	DepthFull Depth = iota
	// DepthFast runs T1 only. T2 is invoked separately via WalkRefs.
	DepthFast
)

// Result is the output of Load.
type Result struct {
	// Packages is the T1 package import graph. Always populated.
	Packages *PackageGraph
	// Graph is the T3 SSA call graph. Populated when Depth == DepthFull.
	Graph *graph.Graph
}

// Load runs T1 always. When depth == DepthFull, also runs T3 (SSA/CHA).
func Load(cfg Config, depth Depth) (*Result, error) {
	cfg.progress("loading package graph")
	pg, err := BuildPackageGraph(cfg)
	if err != nil {
		return nil, err
	}

	result := &Result{Packages: pg}

	if depth == DepthFull {
		g, err := buildSSA(cfg)
		if err != nil {
			return nil, err
		}
		result.Graph = g
	}

	return result, nil
}
```

- [ ] **Step 4: Run all existing tests to confirm nothing broken**

```bash
GOCACHE=/tmp/gocache go test ./... -v 2>&1 | tail -30
```

Expected: all graph and loader tests pass. The build_test.go tests that used `graph.Build()` will now fail to compile — that's the signal to move to Task 5.

- [ ] **Step 5: Commit**

```bash
jj describe -m "loader: migrate T3 SSA engine from graph/build.go to loader/ssa.go"
jj new
```

---

## Task 5: Update CLI to use loader.Load()

**Files:**
- Modify: `cmd/gorefact/main.go`

The `runCheck` and `runServe` functions call `graph.Build(graph.BuildConfig{…})`. Replace with `loader.Load(loader.Config{…}, loader.DepthFull)`.

- [ ] **Step 1: Update runCheck in main.go**

Find the block in `runCheck`:
```go
g, err := graph.Build(graph.BuildConfig{
    Dir:       *dir,
    Tests:     *tests,
    FilterPkg: *filterPkg,
    Patterns:  fs.Args(),
    Progress:  progress,
})
if err != nil {
    fmt.Fprintf(stderr, "build failed: %v\n", err)
    return 1
}
```

Replace with:
```go
res, err := loader.Load(loader.Config{
    Dir:       *dir,
    Tests:     *tests,
    FilterPkg: *filterPkg,
    Patterns:  fs.Args(),
    Progress:  progress,
}, loader.DepthFull)
if err != nil {
    fmt.Fprintf(stderr, "build failed: %v\n", err)
    return 1
}
g := res.Graph
```

- [ ] **Step 2: Update runServe in main.go**

Find the block in `runServe`:
```go
g, err := graph.Build(graph.BuildConfig{
    Dir:       *dir,
    Tests:     *tests,
    FilterPkg: *filterPkg,
    Patterns:  fs.Args(),
    Progress:  progress,
})
```

Replace with:
```go
res, err := loader.Load(loader.Config{
    Dir:       *dir,
    Tests:     *tests,
    FilterPkg: *filterPkg,
    Patterns:  fs.Args(),
    Progress:  progress,
}, loader.DepthFull)
if err != nil {
    fmt.Fprintf(stderr, "build failed: %v\n", err)
    return 1
}
g := res.Graph
```

- [ ] **Step 3: Update imports in main.go**

Remove `"go.flaticols.dev/gorefactor/internal/graph"` from the import if it's only used for `graph.Build`/`graph.BuildConfig`. Add `"go.flaticols.dev/gorefactor/internal/loader"`. Keep the `graph` import if it's used for `graph.Func` or other types elsewhere in the file.

- [ ] **Step 4: Build and run all tests**

```bash
GOCACHE=/tmp/gocache go build ./cmd/gorefact && GOCACHE=/tmp/gocache go test ./... 2>&1 | tail -20
```

Expected: build succeeds, all tests pass.

- [ ] **Step 5: Smoke test the binary**

```bash
go run ./cmd/gorefact version
go run ./cmd/gorefact check --help
```

Expected: version prints, help text appears.

- [ ] **Step 6: Commit**

```bash
jj describe -m "cmd: migrate check and serve to loader.Load()"
jj new
```

---

## After Plan 1

Plans 2 and 3 will be written before execution:

- **Plan 2:** `gorefact inspect` command, TTY detection via `golang.org/x/term`, Bubble Tea TUI (search/tree/detail panes)
- **Plan 3:** `gorefact rules` subcommands, TUI rule editor, RPC protocol extensions, help system rewrite
