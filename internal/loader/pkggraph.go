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
