package tui

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
)

func TestBuildImporterRowsFlat_ListsImportersWithUseCount(t *testing.T) {
	m := newTestModel(testGraph())
	target := testModule + "/internal/graph"
	edges := []graph.Edge{
		{Callee: 1, CallerPkg: testModule + "/internal/loader", File: "l.go", Line: 1},
		{Callee: 2, CallerPkg: testModule + "/internal/loader", File: "l.go", Line: 2},
	}
	rows := buildImporterRowsFlat(target, m.pg, edges, nil, m.shortPkg, nil)
	if len(rows) != 1 {
		t.Fatalf("expected 1 importer of internal/graph, got %d: %v", len(rows), rows)
	}
	if rows[0].pkgPath != testModule+"/internal/loader" {
		t.Errorf("importer path = %q", rows[0].pkgPath)
	}
	if !strings.Contains(rows[0].label, "[2]") {
		t.Errorf("expected use count [2] in label, got %q", rows[0].label)
	}
	if !rows[0].expandable {
		t.Errorf("an importer with references should be expandable")
	}
}

func TestBuildImporterRowsFlat_ExpandsToRefSites(t *testing.T) {
	m := newTestModel(testGraph())
	target := testModule + "/internal/graph"
	importer := testModule + "/internal/loader"
	syms := []graph.Symbol{{ID: 1, Name: "Graph"}, {ID: 2, Name: "Edge"}}
	edges := []graph.Edge{
		{Callee: 1, CallerPkg: importer, CallerFunc: "Build", File: "loader.go", Line: 10},
		{Callee: 2, CallerPkg: importer, CallerFunc: "Build", File: "loader.go", Line: 12},
	}
	rows := buildImporterRowsFlat(target, m.pg, edges, syms, m.shortPkg, map[string]bool{importer: true})
	// 1 importer header + 2 reference-site children.
	if len(rows) != 3 {
		t.Fatalf("expected importer + 2 ref rows, got %d: %v", len(rows), rows)
	}
	child := rows[1]
	if !child.ref || child.depth != 1 {
		t.Fatalf("expected a depth-1 ref child, got %+v", child)
	}
	if !strings.Contains(child.label, "Build → Graph") || child.file != "loader.go" || child.line != 10 {
		t.Errorf("ref child row mismatch: %+v", child)
	}
}

func TestBuildImporterRowsFlat_NoImporters(t *testing.T) {
	m := newTestModel(testGraph())
	// cmd/app is imported by nobody.
	rows := buildImporterRowsFlat(testModule+"/cmd/app", m.pg, nil, nil, m.shortPkg, nil)
	if len(rows) != 1 || !strings.Contains(rows[0].label, "no importers") {
		t.Errorf("expected a single (no importers) row, got: %v", rows)
	}
}

func TestBuildImporterRowsTree_Expands(t *testing.T) {
	m := newTestModel(testGraph())
	target := testModule + "/internal/graph"
	// internal/loader imports internal/graph; cmd/app imports internal/loader.
	// The tree roots at the importers of target, so the loader row's key is its
	// own path (no target prefix).
	expand := map[string]bool{
		testModule + "/internal/loader": true,
	}
	rows := buildImporterRowsTree(target, m.pg, nil, m.shortPkg, expand)
	sawApp := false
	for _, r := range rows {
		if r.pkgPath == testModule+"/cmd/app" && r.depth == 1 {
			sawApp = true
		}
	}
	if !sawApp {
		t.Errorf("expanding internal/loader should reveal cmd/app at depth 1, got: %v", rows)
	}
}

func TestSymbolsUsedByPkg(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 1, CallerPkg: "a"},
		{Callee: 1, CallerPkg: "a"}, // duplicate symbol from same pkg → counted once
		{Callee: 2, CallerPkg: "a"},
		{Callee: 1, CallerPkg: "b", SamePkg: true}, // same-pkg edges ignored
	}
	got := symbolsUsedByPkg(edges)
	if got["a"] != 2 {
		t.Errorf("pkg a should use 2 distinct symbols, got %d", got["a"])
	}
	if _, ok := got["b"]; ok {
		t.Errorf("same-pkg edges must not produce a use count: %v", got)
	}
}
