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

	lines, refs := buildTreeLines(edges, 100, GroupPkg, nil)
	if len(lines) != len(refs) {
		t.Fatalf("lines (%d) and refs (%d) length mismatch", len(lines), len(refs))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "handler") || !strings.Contains(joined, "service") {
		t.Errorf("expected both caller packages in output, got:\n%s", joined)
	}
	// A reference child row carries a non-zero treeRef.
	hasRef := false
	for _, r := range refs {
		if r.file != "" && r.line > 0 {
			hasRef = true
		}
	}
	if !hasRef {
		t.Error("expected at least one reference row with file/line")
	}
}

func TestBuildTreeLines_FiltersBySymbol(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/a", File: "a/a.go", Line: 1},
		{Callee: 200, CallerPkg: "example.com/b", File: "b/b.go", Line: 2},
	}
	lines, _ := buildTreeLines(edges, 100, GroupPkg, nil)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "example.com/b") {
		t.Errorf("symbol 100 result should not include edges to symbol 200:\n%s", joined)
	}
}

func TestBuildTreeLines_NoEdges(t *testing.T) {
	lines, refs := buildTreeLines(nil, 100, GroupPkg, nil)
	if len(lines) != 1 || !strings.Contains(lines[0], "no references") {
		t.Fatalf("expected a single (no references) line, got: %v", lines)
	}
	if len(refs) != len(lines) {
		t.Fatalf("refs length must match lines")
	}
}

func TestBuildTreeLines_GroupFile(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42},
		{Callee: 100, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 55},
	}
	lines, _ := buildTreeLines(edges, 100, GroupFile, nil)
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

func TestBuildTreeLines_GroupFunc(t *testing.T) {
	edges := []graph.Edge{
		{Callee: 100, CallerPkg: "example.com/handler", CallerFunc: "Serve", File: "handler/h.go", Line: 42},
		{Callee: 100, CallerPkg: "example.com/handler", CallerFunc: "Serve", File: "handler/h.go", Line: 55},
	}
	lines, _ := buildTreeLines(edges, 100, GroupFunc, nil)
	headers := 0
	for _, l := range lines {
		if strings.Contains(l, "Serve") && strings.Contains(l, "refs") {
			headers++
		}
	}
	if headers != 1 {
		t.Errorf("expected 1 func header for Serve, got %d; lines: %v", headers, lines)
	}
}
