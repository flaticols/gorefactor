package loader

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"

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
	// Map from "pkgpath.Name" -> Symbol for cross-package object matching.
	keyToSym := make(map[string]*graph.Symbol)

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
		key := tp.PkgPath + "." + name
		keyToSym[key] = sym
		nextID++
	}

	result := &RefResult{}
	for _, sym := range keyToSym {
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
			if obj.Pkg() == nil {
				continue
			}
			key := obj.Pkg().Path() + "." + obj.Name()
			sym, ok := keyToSym[key]
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
