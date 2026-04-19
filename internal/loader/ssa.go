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
	patterns := cfg.patterns()

	cfg.progress("loading packages")
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

	cfg.progress("building ssa")
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	cfg.progress("building call graph")
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
	cfg.progress("done")
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
