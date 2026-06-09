package inspect

import (
	"go/types"
	"path/filepath"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/loader"
	"golang.org/x/tools/go/packages"
)

// InspectResult holds the resolved reference data for a target.
type InspectResult struct {
	Target     string
	PkgPath    string
	SymbolName string // empty = whole package
	Symbols    []graph.Symbol
	Edges      []graph.Edge
	PkgGraph   *loader.PackageGraph
}

// Config configures the inspect resolver.
type Config struct {
	Loader loader.Config
}

// BuildGraph runs a T1 load and returns the resulting package graph. The TUI
// builds this once at startup and threads it into ResolveTargetWithGraph and
// LoadStructMembersWithGraph to avoid rebuilding the graph on every selection.
func BuildGraph(cfg Config) (*loader.PackageGraph, error) {
	if cfg.Loader.Progress != nil {
		cfg.Loader.Progress("loading package graph")
	}
	return loader.BuildPackageGraph(cfg.Loader)
}

// ResolveTarget parses target, loads T1+T2 data, and returns an InspectResult.
// target may be a package path, "pkg.Symbol", or "pkg.Receiver.Method".
// If target contains no "/" it is treated as a suffix and matched against all
// known packages (e.g. "graph" matches "go.flaticols.dev/gorefactor/internal/graph").
//
// It builds the package graph once and delegates to ResolveTargetWithGraph.
func ResolveTarget(target string, cfg Config) (*InspectResult, error) {
	pg, err := BuildGraph(cfg)
	if err != nil {
		return nil, err
	}
	return ResolveTargetWithGraph(target, cfg, pg)
}

// ResolveTargetWithGraph is like ResolveTarget but reuses an already-built
// package graph instead of rebuilding it. Callers that resolve many targets
// (e.g. an interactive browser) should build the graph once via BuildGraph and
// pass it here to avoid repeated T1 loads.
func ResolveTargetWithGraph(target string, cfg Config, pg *loader.PackageGraph) (*InspectResult, error) {
	target = strings.TrimSpace(target)
	pkgPath, symbolName, _ := ParseTarget(target) // method-level filtering not yet implemented

	// Resolve to a known package. If pkgPath is not an exact graph node (e.g. a
	// bare suffix "loader" or a partial path "internal/loader"), match it against
	// the first package whose path ends with "/"+pkgPath or equals pkgPath.
	if resolved := ResolvePackagePath(pg, pkgPath); resolved != "" {
		pkgPath = resolved
	}

	importers := pg.ImportersOf(pkgPath)
	importerPaths := make([]string, 0, len(importers)+1)
	for _, n := range importers {
		importerPaths = append(importerPaths, n.Path)
	}
	// Also walk the target package itself so intra-package references are captured.
	importerPaths = append(importerPaths, pkgPath)

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

	return &InspectResult{
		Target:     target,
		PkgPath:    pkgPath,
		SymbolName: symbolName,
		Symbols:    syms,
		Edges:      edges,
		PkgGraph:   pg,
	}, nil
}

// ResolvePackagePath resolves target to a known package path in pg: an exact
// node match wins, otherwise the first package whose path ends with "/"+target
// or equals target (so bare suffixes like "loader" and partial paths like
// "internal/loader" resolve). Returns "" when nothing matches.
func ResolvePackagePath(pg *loader.PackageGraph, target string) string {
	if pg == nil || target == "" {
		return ""
	}
	if _, ok := pg.Nodes[target]; ok {
		return target
	}
	return suffixMatch(target, pg.AllPaths())
}

// ListPackages returns all package paths in the workspace and the main module path.
func ListPackages(cfg Config) ([]string, string, error) {
	pg, err := loader.BuildPackageGraph(cfg.Loader)
	if err != nil {
		return nil, "", err
	}
	return pg.AllPaths(), pg.Module, nil
}

// StructMember is one exported method or field of a named type.
type StructMember struct {
	Name      string // method or field name
	Kind      string // "method" or "field"
	Signature string // for methods: "(args) (results)"; for fields: ""
	Type      string // for fields: type string; for methods: result type
	File      string
	Line      int
	Edges     []graph.Edge // references to this member across importer packages
}

// LoadStructMembers returns the exported methods and (for struct types) fields
// of the named type pkgPath.typeName. Returns nil if the type is not found.
//
// It builds the package graph once (to attach member references) and delegates
// to LoadStructMembersWithGraph.
func LoadStructMembers(cfg Config, pkgPath, typeName string) ([]StructMember, error) {
	pg, err := loader.BuildPackageGraph(cfg.Loader)
	if err != nil {
		// Member positions/signatures don't need the graph; only the reference
		// edges do. Fall back to a nil graph so callers still get members.
		pg = nil
	}
	return LoadStructMembersWithGraph(cfg, pg, pkgPath, typeName)
}

// LoadStructMembersWithGraph is like LoadStructMembers but reuses an
// already-built package graph instead of rebuilding it. A nil graph skips the
// reference-edge collection (members are still returned).
func LoadStructMembersWithGraph(cfg Config, pg *loader.PackageGraph, pkgPath, typeName string) ([]StructMember, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedSyntax,
		Dir:   cfg.Loader.Dir,
		Tests: cfg.Loader.Tests,
	}, pkgPath)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 || pkgs[0].Types == nil {
		return nil, nil
	}
	pkg := pkgs[0]
	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return nil, nil
	}
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil, nil
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		return nil, nil
	}

	var members []StructMember

	if st, ok := named.Underlying().(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if !f.Exported() {
				continue
			}
			pos := pkg.Fset.Position(f.Pos())
			members = append(members, StructMember{
				Name: f.Name(),
				Kind: "field",
				Type: f.Type().String(),
				File: relPath(cfg.Loader.Dir, pos.Filename),
				Line: pos.Line,
			})
		}
	}

	ms := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < ms.Len(); i++ {
		sel := ms.At(i)
		fn, ok := sel.Obj().(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig, _ := fn.Type().(*types.Signature)
		pos := pkg.Fset.Position(fn.Pos())
		members = append(members, StructMember{
			Name:      fn.Name(),
			Kind:      "method",
			Signature: formatSignature(sig),
			File:      relPath(cfg.Loader.Dir, pos.Filename),
			Line:      pos.Line,
		})
	}

	// Collect references to each member across importer packages and the
	// target package itself (so intra-package refs are included) and attach.
	if pg != nil {
		importers := pg.ImportersOf(pkgPath)
		paths := make([]string, 0, len(importers)+1)
		for _, n := range importers {
			paths = append(paths, n.Path)
		}
		paths = append(paths, pkgPath)
		if refs, err := loader.WalkMemberRefs(pkgPath, typeName, paths, cfg.Loader); err == nil {
			for i := range members {
				members[i].Edges = refs[members[i].Name]
			}
		}
	}

	return members, nil
}

func relPath(base, file string) string {
	if file == "" {
		return file
	}
	if base == "" {
		return file
	}
	if !filepath.IsAbs(base) {
		if abs, err := filepath.Abs(base); err == nil {
			base = abs
		}
	}
	if rel, err := filepath.Rel(base, file); err == nil {
		return filepath.ToSlash(rel)
	}
	return file
}

func formatSignature(sig *types.Signature) string {
	if sig == nil {
		return ""
	}
	var b strings.Builder
	b.WriteByte('(')
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		p := params.At(i)
		if p.Name() != "" {
			b.WriteString(p.Name())
			b.WriteByte(' ')
		}
		b.WriteString(p.Type().String())
	}
	b.WriteByte(')')
	res := sig.Results()
	if res.Len() == 0 {
		return b.String()
	}
	b.WriteByte(' ')
	if res.Len() > 1 {
		b.WriteByte('(')
	}
	for i := 0; i < res.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(res.At(i).Type().String())
	}
	if res.Len() > 1 {
		b.WriteByte(')')
	}
	return b.String()
}

// LoadAllSymbols enumerates every exported symbol across all workspace packages.
// It uses NeedTypes (no SSA) so it is significantly faster than a full T2 load,
// but still slower than T1 — run it in a background goroutine.
func LoadAllSymbols(cfg Config) ([]graph.Symbol, error) {
	patterns := cfg.Loader.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.NeedName | packages.NeedTypes | packages.NeedModule,
		Dir:   cfg.Loader.Dir,
		Tests: cfg.Loader.Tests,
	}, patterns...)
	if err != nil {
		return nil, err
	}

	var syms []graph.Symbol
	id := 1
	for _, pkg := range pkgs {
		if pkg.Types == nil || pkg.PkgPath == "" {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			kind := loader.SymKind(obj)
			if kind == "" {
				continue
			}
			syms = append(syms, graph.Symbol{
				ID:       id,
				Kind:     kind,
				Name:     name,
				Package:  pkg.PkgPath,
				Exported: true,
			})
			id++
		}
	}
	return syms, nil
}

// suffixMatch returns the first path in paths that ends with "/"+suffix or equals suffix.
func suffixMatch(suffix string, paths []string) string {
	for _, p := range paths {
		if p == suffix || strings.HasSuffix(p, "/"+suffix) {
			return p
		}
	}
	return ""
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
