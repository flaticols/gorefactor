package tui

import (
	"fmt"
	"sort"

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
	case GroupFunc:
		return treeByFunc(relevant, violOnly, violPkgs)
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
		marker := "✓"
		if isViol {
			marker = styleViolation.Render("✗")
		}
		lines = append(lines, fmt.Sprintf("  %s %s (%d refs)", marker, pkg, len(es)))
		for _, e := range es {
			sameMark := ""
			if e.SamePkg {
				sameMark = " ~"
			}
			lines = append(lines, fmt.Sprintf("      %s:%d%s", e.File, e.Line, sameMark))
		}
	}
	if len(lines) == 0 {
		return []string{"  (no violations)"}
	}
	return lines
}

func treeByFunc(edges []graph.Edge, violOnly bool, violPkgs map[string]bool) []string {
	groups := make(map[string][]graph.Edge)
	for _, e := range edges {
		fn := e.CallerFunc
		if fn == "" {
			fn = "(unknown)"
		}
		key := e.CallerPkg + "." + fn
		groups[key] = append(groups[key], e)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, key := range keys {
		es := groups[key]
		pkg := es[0].CallerPkg
		isViol := violPkgs[pkg]
		if violOnly && !isViol {
			continue
		}
		marker := "✓"
		if isViol {
			marker = styleViolation.Render("✗")
		}
		lines = append(lines, fmt.Sprintf("  %s %s (%d refs)", marker, key, len(es)))
		for _, e := range es {
			sameMark := ""
			if e.SamePkg {
				sameMark = " ~"
			}
			lines = append(lines, fmt.Sprintf("      %s:%d%s", e.File, e.Line, sameMark))
		}
	}
	if len(lines) == 0 {
		return []string{"  (no violations)"}
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
			lines = append(lines, fmt.Sprintf("      :%d  %s", e.Line, string(e.Kind)))
		}
	}
	if len(lines) == 0 {
		return []string{"  (no violations)"}
	}
	return lines
}
