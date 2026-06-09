package tui

import (
	"fmt"
	"sort"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/loader"
)

// importerRow is one navigable line in the importers pane (pane 3, package focus).
type importerRow struct {
	pkgPath    string
	label      string
	depth      int
	expandable bool
	expanded   bool
	ref        bool   // true = a reference-site child row ("func → Symbol")
	file       string // set for ref rows: jump target for `e`
	line       int
}

// buildImporterRowsFlat lists the direct importers of pkgPath as a flat list,
// annotating each with the count of distinct symbols it references (derived from
// edges grouped by CallerPkg). An importer with references is expandable; when
// expanded it reveals the individual reference sites ("CallerFunc → Symbol") so
// the user can see why the package is imported without selecting a symbol.
func buildImporterRowsFlat(pkgPath string, pg *loader.PackageGraph, edges []graph.Edge, syms []graph.Symbol, shortPkg func(string) string, expanded map[string]bool) []importerRow {
	if pg == nil {
		return nil
	}
	importers := pg.ImportersOf(pkgPath)
	if len(importers) == 0 {
		return []importerRow{{label: "(no importers)"}}
	}
	useCount := symbolsUsedByPkg(edges)
	symName := make(map[int]string, len(syms))
	for _, s := range syms {
		symName[s.ID] = s.Name
	}
	rows := make([]importerRow, 0, len(importers))
	for _, n := range importers {
		c := useCount[n.Path]
		label := shortPkg(n.Path)
		if c > 0 {
			label += fmt.Sprintf(" [%d]", c)
		}
		open := c > 0 && expanded[n.Path]
		rows = append(rows, importerRow{
			pkgPath:    n.Path,
			label:      label,
			expandable: c > 0,
			expanded:   open,
		})
		if open {
			rows = append(rows, importerRefRows(n.Path, edges, symName)...)
		}
	}
	return rows
}

// importerRefRows builds the child reference-site rows for one importer: every
// reference it makes into the target package, as "CallerFunc → Symbol  file:line",
// sorted by location.
func importerRefRows(importerPath string, edges []graph.Edge, symName map[int]string) []importerRow {
	var es []graph.Edge
	for _, e := range edges {
		if e.SamePkg || e.CallerPkg != importerPath {
			continue
		}
		es = append(es, e)
	}
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].File != es[j].File {
			return es[i].File < es[j].File
		}
		return es[i].Line < es[j].Line
	})
	rows := make([]importerRow, 0, len(es))
	for _, e := range es {
		caller := e.CallerFunc
		if caller == "" {
			caller = "(pkg-level)"
		}
		loc := e.File
		if e.Line > 0 {
			loc = fmt.Sprintf("%s:%d", e.File, e.Line)
		}
		rows = append(rows, importerRow{
			depth: 1,
			label: fmt.Sprintf("%s → %s  %s", caller, symName[e.Callee], loc),
			ref:   true,
			file:  e.File,
			line:  e.Line,
		})
	}
	return rows
}

// buildImporterRowsTree lays out the importers of pkgPath as a tree where each
// importer can expand into its own importers. expanded holds the set of open
// keys; ancestors break cycles.
func buildImporterRowsTree(pkgPath string, pg *loader.PackageGraph, edges []graph.Edge, shortPkg func(string) string, expanded map[string]bool) []importerRow {
	if pg == nil {
		return nil
	}
	rootImporters := pg.ImportersOf(pkgPath)
	if len(rootImporters) == 0 {
		return []importerRow{{label: "(no importers)"}}
	}
	useCount := symbolsUsedByPkg(edges)

	var rows []importerRow
	var walk func(path, key string, depth int, ancestors map[string]bool)
	walk = func(path, key string, depth int, ancestors map[string]bool) {
		up := pg.ImportersOf(path)
		expandable := len(up) > 0 && !ancestors[path]
		label := shortPkg(path)
		if depth == 0 {
			if c := useCount[path]; c > 0 {
				label += fmt.Sprintf(" [%d]", c)
			}
		}
		rows = append(rows, importerRow{
			pkgPath:    path,
			label:      label,
			depth:      depth,
			expandable: expandable,
			expanded:   expanded[key],
		})
		if !expandable || !expanded[key] {
			return
		}
		ancestors[path] = true
		for _, n := range up {
			walk(n.Path, key+"\x00"+n.Path, depth+1, ancestors)
		}
		delete(ancestors, path)
	}
	for _, n := range rootImporters {
		walk(n.Path, n.Path, 0, map[string]bool{})
	}
	return rows
}

// symbolsUsedByPkg counts the number of distinct callee symbols referenced from
// each caller package.
func symbolsUsedByPkg(edges []graph.Edge) map[string]int {
	perPkg := make(map[string]map[int]bool)
	for _, e := range edges {
		if e.SamePkg || e.CallerPkg == "" {
			continue
		}
		set := perPkg[e.CallerPkg]
		if set == nil {
			set = map[int]bool{}
			perPkg[e.CallerPkg] = set
		}
		set[e.Callee] = true
	}
	out := make(map[string]int, len(perPkg))
	for pkg, set := range perPkg {
		out[pkg] = len(set)
	}
	return out
}
