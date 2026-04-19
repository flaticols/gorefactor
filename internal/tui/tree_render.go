package tui

import (
	"fmt"
	"sort"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// buildTreeLines converts reference edges for a specific symbol into renderable
// text lines for the tree pane, grouped by pkg or file.
// The returned refs slice is parallel to lines: non-zero entries point to the
// source file and line of the reference (child rows); header rows have zero values.
func buildTreeLines(
	edges []graph.Edge,
	symID int,
	group GroupMode,
	violOnly bool,
	violPkgs map[string]bool,
	shortPkg func(string) string,
) ([]string, []treeRef) {
	if shortPkg == nil {
		shortPkg = func(s string) string { return s }
	}
	var relevant []graph.Edge
	for _, e := range edges {
		if e.Callee == symID {
			relevant = append(relevant, e)
		}
	}
	if len(relevant) == 0 {
		return []string{"  (no references)"}, []treeRef{{}}
	}
	switch group {
	case GroupFile:
		return treeByFile(relevant, violOnly, violPkgs)
	case GroupFunc:
		return treeByFunc(relevant, violOnly, violPkgs, shortPkg)
	default:
		return treeByPkg(relevant, violOnly, violPkgs, shortPkg)
	}
}

func treeByPkg(edges []graph.Edge, violOnly bool, violPkgs map[string]bool, shortPkg func(string) string) ([]string, []treeRef) {
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
	var refs []treeRef
	for _, pkg := range pkgs {
		es := groups[pkg]
		isViol := violPkgs[pkg]
		if violOnly && !isViol {
			continue
		}
		marker := "✓"
		if isViol {
			marker = styleViolation.Render("✗")
		}
		lines = append(lines, fmt.Sprintf("  %s %s (%d refs)", marker, shortPkg(pkg), len(es)))
		refs = append(refs, treeRef{})
		for _, e := range es {
			sameMark := ""
			if e.SamePkg {
				sameMark = " ~"
			}
			lines = append(lines, fmt.Sprintf("      %s:%d%s", e.File, e.Line, sameMark))
			refs = append(refs, treeRef{file: e.File, line: e.Line})
		}
	}
	if len(lines) == 0 {
		return []string{"  (no violations)"}, []treeRef{{}}
	}
	return lines, refs
}

func treeByFunc(edges []graph.Edge, violOnly bool, violPkgs map[string]bool, shortPkg func(string) string) ([]string, []treeRef) {
	type funcKey struct{ pkg, fn string }
	groups := make(map[funcKey][]graph.Edge)
	for _, e := range edges {
		fn := e.CallerFunc
		if fn == "" {
			fn = "(unknown)"
		}
		groups[funcKey{pkg: e.CallerPkg, fn: fn}] = append(groups[funcKey{pkg: e.CallerPkg, fn: fn}], e)
	}
	keys := make([]funcKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].pkg != keys[j].pkg {
			return keys[i].pkg < keys[j].pkg
		}
		return keys[i].fn < keys[j].fn
	})

	var lines []string
	var refs []treeRef
	for _, key := range keys {
		es := groups[key]
		pkg := key.pkg
		isViol := violPkgs[pkg]
		if violOnly && !isViol {
			continue
		}
		marker := "✓"
		if isViol {
			marker = styleViolation.Render("✗")
		}
		lines = append(lines, fmt.Sprintf("  %s %s.%s (%d refs)", marker, shortPkg(pkg), key.fn, len(es)))
		refs = append(refs, treeRef{})
		for _, e := range es {
			sameMark := ""
			if e.SamePkg {
				sameMark = " ~"
			}
			lines = append(lines, fmt.Sprintf("      %s:%d%s", e.File, e.Line, sameMark))
			refs = append(refs, treeRef{file: e.File, line: e.Line})
		}
	}
	if len(lines) == 0 {
		return []string{"  (no violations)"}, []treeRef{{}}
	}
	return lines, refs
}

func treeByFile(edges []graph.Edge, violOnly bool, violPkgs map[string]bool) ([]string, []treeRef) {
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
	var refs []treeRef
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
		refs = append(refs, treeRef{})
		for _, e := range es {
			lines = append(lines, fmt.Sprintf("      :%d  %s", e.Line, string(e.Kind)))
			refs = append(refs, treeRef{file: e.File, line: e.Line})
		}
	}
	if len(lines) == 0 {
		return []string{"  (no violations)"}, []treeRef{{}}
	}
	return lines, refs
}
