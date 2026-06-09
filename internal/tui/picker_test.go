package tui

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
)

const testModule = "example.com/m"

func newTestModel(pg *loader.PackageGraph) Model {
	m := New("", inspect.Config{})
	m.pg = pg
	m.allPkgs = pg.AllPaths()
	m.modulePrefix = pg.Module
	return m
}

func testGraph() *loader.PackageGraph {
	return &loader.PackageGraph{
		Module: testModule,
		Nodes: map[string]*loader.PackageNode{
			testModule + "/internal/graph":  {Path: testModule + "/internal/graph", Name: "graph"},
			testModule + "/internal/loader": {Path: testModule + "/internal/loader", Name: "loader", Imports: []string{testModule + "/internal/graph", "github.com/ext/dep"}},
			testModule + "/cmd/app":         {Path: testModule + "/cmd/app", Name: "main", Imports: []string{testModule + "/internal/loader"}},
			"github.com/ext/dep":            {Path: "github.com/ext/dep", Name: "dep"},
			"fmt":                           {Path: "fmt", Name: "fmt"},
		},
	}
}

func TestModulePkgs_ModuleOnlyDropsStdlib(t *testing.T) {
	pg := testGraph()
	pkgs := modulePkgs(pg.AllPaths(), pg.Module, true)
	for _, p := range pkgs {
		if p == "fmt" {
			t.Fatalf("module-only should exclude stdlib fmt, got: %v", pkgs)
		}
	}
	if len(pkgs) != 3 {
		t.Errorf("expected 3 module packages, got %d: %v", len(pkgs), pkgs)
	}
}

func TestBuildFlatRows_StripsPrefix(t *testing.T) {
	m := newTestModel(testGraph())
	pkgs := modulePkgs(m.allPkgs, m.modulePrefix, true)
	rows := buildFlatRows(pkgs, m.shortPkg)
	found := false
	for _, r := range rows {
		if r.label == "internal/graph" && r.pkgPath == testModule+"/internal/graph" {
			found = true
		}
		if strings.HasPrefix(r.label, testModule) {
			t.Errorf("flat row label should be module-stripped, got %q", r.label)
		}
	}
	if !found {
		t.Errorf("expected internal/graph row, got: %v", rows)
	}
}

func TestBuildFolderRows_NestsAndExpands(t *testing.T) {
	m := newTestModel(testGraph())
	pkgs := modulePkgs(m.allPkgs, m.modulePrefix, true)

	// Collapsed: only top-level segments (cmd, internal).
	rows := buildFolderRows(pkgs, m.shortPkg, map[string]bool{})
	tops := map[string]bool{}
	for _, r := range rows {
		if r.depth == 0 {
			tops[r.label] = true
		}
	}
	if !tops["cmd"] || !tops["internal"] {
		t.Errorf("expected top-level cmd and internal folders, got: %v", tops)
	}

	// Expand "internal" → its children appear at depth 1.
	rows = buildFolderRows(pkgs, m.shortPkg, map[string]bool{"internal": true})
	sawGraph := false
	for _, r := range rows {
		if r.label == "graph" && r.depth == 1 {
			sawGraph = true
		}
	}
	if !sawGraph {
		t.Errorf("expanding internal should reveal graph at depth 1, got: %v", rows)
	}
}

func TestBuildImportRows_ExpandsImports(t *testing.T) {
	m := newTestModel(testGraph())
	pkgs := modulePkgs(m.allPkgs, m.modulePrefix, true)

	// cmd/app imports internal/loader; expand its key.
	rows := buildImportRows(pkgs, m.pg, m.modulePrefix, true, m.shortPkg, map[string]bool{
		testModule + "/cmd/app": true,
	})
	sawChild := false
	for _, r := range rows {
		if r.pkgPath == testModule+"/internal/loader" && r.depth == 1 {
			sawChild = true
		}
	}
	if !sawChild {
		t.Errorf("expanding cmd/app should reveal internal/loader at depth 1, got: %v", rows)
	}
}
