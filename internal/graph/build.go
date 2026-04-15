package graph

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// BuildConfig configures graph construction from Go packages.
type BuildConfig struct {
	Dir       string
	Tests     bool
	FilterPkg string
	Patterns  []string
	Progress  func(stage string)
}

// Build loads packages, constructs SSA, runs CHA, and returns the resulting graph.
func Build(cfg BuildConfig) (*Graph, error) {
	progress := cfg.Progress
	if progress == nil {
		progress = func(string) {}
	}
	patterns := cfg.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

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
	pkgs = filterPackages(pkgs, cfg.FilterPkg)
	if len(pkgs) == 0 {
		return &Graph{}, nil
	}
	packageModules := packageModuleMap(pkgs)

	progress("building ssa")
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	progress("building call graph")
	cg := cha.CallGraph(prog)

	g := &Graph{}
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
			g.Edges = append(g.Edges, Edge{
				Caller:  callerID,
				Callee:  calleeID,
				File:    cleanRelPath(cfg.Dir, pos.Filename),
				Line:    pos.Line,
				Col:     pos.Column,
				Dynamic: edge.Site.Common() != nil && edge.Site.Common().IsInvoke(),
			})
		}
	}

	if cfg.FilterPkg != "" {
		g = filterGraphByPackage(g, cfg.FilterPkg)
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
		if a.Receiver != b.Receiver {
			return a.Receiver < b.Receiver
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

func filterPackages(pkgs []*packages.Package, filter string) []*packages.Package {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return pkgs
	}
	out := make([]*packages.Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}
		if strings.Contains(pkg.PkgPath, filter) {
			out = append(out, pkg)
		}
	}
	return out
}

func filterGraphByPackage(g *Graph, filter string) *Graph {
	filter = strings.TrimSpace(filter)
	if filter == "" || g == nil {
		return g
	}
	allowed := make(map[int]struct{}, len(g.Funcs))
	out := &Graph{}
	for _, fn := range g.Funcs {
		if strings.Contains(fn.Package, filter) {
			allowed[fn.ID] = struct{}{}
			out.Funcs = append(out.Funcs, fn)
		}
	}
	for _, edge := range g.Edges {
		if _, ok := allowed[edge.Caller]; !ok {
			continue
		}
		if _, ok := allowed[edge.Callee]; !ok {
			continue
		}
		out.Edges = append(out.Edges, edge)
	}
	return out
}

func ssaFuncToFunc(fn *ssa.Function, fset *token.FileSet, id int, baseDir string, packageModules map[string]string) *Func {
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

	exported := ast.IsExported(name)
	if recv != "" {
		exported = ast.IsExported(name)
	}

	return &Func{
		ID:        id,
		Module:    packageModules[pkgPath],
		Name:      name,
		Receiver:  recv,
		Signature: fn.String(),
		Package:   pkgPath,
		File:      cleanRelPath(baseDir, pos.Filename),
		Line:      pos.Line,
		Col:       pos.Column,
		Exported:  exported,
	}
}

func packageModuleMap(pkgs []*packages.Package) map[string]string {
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

func cleanRelPath(base, file string) string {
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
