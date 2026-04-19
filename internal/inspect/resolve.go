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
// If target contains no "/" it is treated as a suffix and matched against all
// known packages (e.g. "graph" matches "go.flaticols.dev/gorefactor/internal/graph").
func ResolveTarget(target string, cfg Config) (*InspectResult, error) {
	target = strings.TrimSpace(target)
	pkgPath, symbolName, _ := ParseTarget(target) // method-level filtering not yet implemented

	if cfg.Loader.Progress != nil {
		cfg.Loader.Progress("loading package graph")
	}
	pg, err := loader.BuildPackageGraph(cfg.Loader)
	if err != nil {
		return nil, err
	}

	// Suffix match: if pkgPath has no "/" resolve it to the first package whose
	// path ends with "/"+pkgPath or equals pkgPath exactly.
	if !strings.Contains(pkgPath, "/") {
		if resolved := suffixMatch(pkgPath, pg.AllPaths()); resolved != "" {
			pkgPath = resolved
		}
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
