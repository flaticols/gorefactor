package tui

import (
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/loader"
)

// pickerMode controls how the package picker (pane 1) lays out packages.
type pickerMode int

const (
	pickerFlat   pickerMode = iota // sorted flat list, module prefix stripped
	pickerFolder                   // nested by path segments (folder tree)
	pickerImport                   // nested by module-internal import edges
)

func (p pickerMode) String() string {
	switch p {
	case pickerFolder:
		return "folder"
	case pickerImport:
		return "import"
	default:
		return "flat"
	}
}

// pickerRow is one navigable line in the package picker.
type pickerRow struct {
	pkgPath    string // package this row selects/loads ("" for pure folder headers)
	label      string // display label (without indentation)
	depth      int    // indentation depth
	expandable bool   // folder header or importable package
	expanded   bool
}

// modulePkgs returns the package paths to show in the picker, filtered to the
// main module when moduleOnly is set, sorted.
func modulePkgs(allPkgs []string, modulePrefix string, moduleOnly bool) []string {
	out := make([]string, 0, len(allPkgs))
	for _, p := range allPkgs {
		if isNoisePkg(p) {
			continue
		}
		if moduleOnly && modulePrefix != "" {
			if p != modulePrefix && !strings.HasPrefix(p, modulePrefix+"/") {
				continue
			}
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// buildFlatRows renders the flat picker: one row per package, module prefix stripped.
func buildFlatRows(pkgs []string, shortPkg func(string) string) []pickerRow {
	rows := make([]pickerRow, 0, len(pkgs))
	for _, p := range pkgs {
		rows = append(rows, pickerRow{pkgPath: p, label: shortPkg(p)})
	}
	return rows
}

// folderNode is an intermediate tree node for the folder picker.
type folderNode struct {
	name     string
	pkgPath  string // non-empty if a real package lives exactly at this path
	children map[string]*folderNode
	order    []string // child names in insertion order, sorted on render
}

func newFolderNode(name string) *folderNode {
	return &folderNode{name: name, children: map[string]*folderNode{}}
}

func (n *folderNode) child(name string) *folderNode {
	if c, ok := n.children[name]; ok {
		return c
	}
	c := newFolderNode(name)
	n.children[name] = c
	n.order = append(n.order, name)
	return c
}

// buildFolderRows nests packages by path segments. expanded holds the set of
// folder keys (full segment path) that are open.
func buildFolderRows(pkgs []string, shortPkg func(string) string, expanded map[string]bool) []pickerRow {
	root := newFolderNode("")
	for _, p := range pkgs {
		short := shortPkg(p)
		segs := strings.Split(short, "/")
		cur := root
		for _, seg := range segs {
			cur = cur.child(seg)
		}
		cur.pkgPath = p
	}

	var rows []pickerRow
	var walk func(n *folderNode, prefix string, depth int)
	walk = func(n *folderNode, prefix string, depth int) {
		names := append([]string(nil), n.order...)
		sort.Strings(names)
		for _, name := range names {
			c := n.children[name]
			key := prefix + name
			hasChildren := len(c.children) > 0
			row := pickerRow{
				pkgPath:    c.pkgPath,
				label:      name,
				depth:      depth,
				expandable: hasChildren,
				expanded:   expanded[key],
			}
			rows = append(rows, row)
			if hasChildren && expanded[key] {
				walk(c, key+"/", depth+1)
			}
		}
	}
	walk(root, "", 0)
	return rows
}

// buildImportRows nests packages by module-internal import edges. Each row may
// expand into the packages it imports (within the module). expanded holds the
// set of import-path keys ("parentPath\x00childPath") that are open; cycles are
// broken by tracking the ancestor chain.
func buildImportRows(pkgs []string, pg *loader.PackageGraph, modulePrefix string, moduleOnly bool, shortPkg func(string) string, expanded map[string]bool) []pickerRow {
	if pg == nil {
		return buildFlatRows(pkgs, shortPkg)
	}
	inModule := func(p string) bool {
		if !moduleOnly || modulePrefix == "" {
			return true
		}
		return p == modulePrefix || strings.HasPrefix(p, modulePrefix+"/")
	}
	known := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		known[p] = true
	}

	moduleImports := func(path string) []string {
		node := pg.Nodes[path]
		if node == nil {
			return nil
		}
		var imps []string
		for _, imp := range node.Imports {
			if isNoisePkg(imp) || !inModule(imp) || !known[imp] {
				continue
			}
			imps = append(imps, imp)
		}
		sort.Strings(imps)
		return imps
	}

	var rows []pickerRow
	var walk func(path, key string, depth int, ancestors map[string]bool)
	walk = func(path, key string, depth int, ancestors map[string]bool) {
		imps := moduleImports(path)
		row := pickerRow{
			pkgPath:    path,
			label:      shortPkg(path),
			depth:      depth,
			expandable: len(imps) > 0 && !ancestors[path],
			expanded:   expanded[key],
		}
		rows = append(rows, row)
		if !row.expandable || !row.expanded {
			return
		}
		ancestors[path] = true
		for _, imp := range imps {
			walk(imp, key+"\x00"+imp, depth+1, ancestors)
		}
		delete(ancestors, path)
	}

	for _, p := range pkgs {
		walk(p, p, 0, map[string]bool{})
	}
	return rows
}
