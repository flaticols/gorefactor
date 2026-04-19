package tui

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
)

func TestBuildTreeLines_GroupPkg(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, Kind: graph.EdgeCall, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42},
		{Callee: 100, Kind: graph.EdgeTypeRef, CallerPkg: "example.com/service", File: "service/s.go", Line: 10},
	}
	violPkgs := map[string]bool{"example.com/handler": true}

	lines := buildTreeLines(edges, 100, GroupPkg, false, violPkgs)

	if len(lines) == 0 {
		t.Fatal("expected lines, got none")
	}
	foundViol := false
	foundClean := false
	for _, l := range lines {
		if strings.Contains(l, "handler") && strings.Contains(l, "✗") {
			foundViol = true
		}
		if strings.Contains(l, "service") && strings.Contains(l, "✓") {
			foundClean = true
		}
	}
	if !foundViol {
		t.Errorf("expected ✗ marker for handler, lines: %v", lines)
	}
	if !foundClean {
		t.Errorf("expected ✓ marker for service, lines: %v", lines)
	}
}

func TestBuildTreeLines_ViolOnly(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 10},
		{Callee: 100, CallerPkg: "example.com/service", File: "service/s.go", Line: 20},
	}
	violPkgs := map[string]bool{"example.com/handler": true}

	lines := buildTreeLines(edges, 100, GroupPkg, true, violPkgs)
	for _, l := range lines {
		if strings.Contains(l, "service") {
			t.Errorf("violOnly=true should hide clean package, but got: %q", l)
		}
	}
}

func TestBuildTreeLines_NoEdges(t *testing.T) {
	lines := buildTreeLines(nil, 100, GroupPkg, false, nil)
	if len(lines) == 0 {
		t.Fatal("expected at least one line (empty message), got none")
	}
}

func TestBuildTreeLines_GroupFile(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42},
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 55},
	}
	violPkgs := map[string]bool{}

	lines := buildTreeLines(edges, 100, GroupFile, false, violPkgs)
	// Both edges are in the same file — should appear under one group header.
	fileHeaders := 0
	for _, l := range lines {
		if strings.Contains(l, "handler/h.go") && !strings.Contains(l, ":42") && !strings.Contains(l, ":55") {
			fileHeaders++
		}
	}
	if fileHeaders != 1 {
		t.Errorf("expected 1 file header for handler/h.go, got %d; lines: %v", fileHeaders, lines)
	}
}
