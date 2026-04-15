package treeview

import (
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/rules"
)

func TestBuildTreeMethodModeAnnotatesViolations(t *testing.T) {
	g := &graph.Graph{
		Funcs: []graph.Func{
			{ID: 1, Name: "Execute", Package: "tasks", File: "tasks/process.go", Line: 123},
			{ID: 2, Name: "Run", Package: "service", File: "service/run.go", Line: 42},
			{ID: 3, Name: "Calculate", Package: "adapters", File: "adapters/engine.go", Line: 55},
		},
		Edges: []graph.Edge{
			{Caller: 2, Callee: 1, File: "service/run.go", Line: 42},
			{Caller: 3, Callee: 1, File: "adapters/engine.go", Line: 55},
		},
	}
	g.Index()

	nodes := BuildTree(g, 1, GroupModeMethod, false, []rules.Rule{
		{From: "adapters", To: "tasks", Reason: "no direct adapter dependency"},
	})
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(nodes))
	}
	if nodes[0].Label != "Execute" {
		t.Fatalf("root label = %q", nodes[0].Label)
	}
	if nodes[0].CallerCount != 2 {
		t.Fatalf("caller count = %d, want 2", nodes[0].CallerCount)
	}
	if len(nodes[0].Children) != 2 {
		t.Fatalf("children = %d, want 2", len(nodes[0].Children))
	}
	var sawViolation bool
	for _, child := range nodes[0].Children {
		if child.Label == "adapters.Calculate" && child.Violation != nil {
			sawViolation = true
		}
	}
	if !sawViolation {
		t.Fatalf("expected a violation annotation in tree: %#v", nodes[0].Children)
	}
}

func TestBuildTreeViolationsOnlyFiltersCleanNodes(t *testing.T) {
	g := &graph.Graph{
		Funcs: []graph.Func{
			{ID: 1, Name: "Execute", Package: "tasks", File: "tasks/process.go", Line: 123},
			{ID: 2, Name: "Run", Package: "service", File: "service/run.go", Line: 42},
			{ID: 3, Name: "Calculate", Package: "adapters", File: "adapters/engine.go", Line: 55},
		},
		Edges: []graph.Edge{
			{Caller: 2, Callee: 1, File: "service/run.go", Line: 42},
			{Caller: 3, Callee: 1, File: "adapters/engine.go", Line: 55},
		},
	}
	g.Index()

	nodes := BuildTree(g, 1, GroupModeMethod, true, []rules.Rule{
		{From: "adapters", To: "tasks", Reason: "no direct adapter dependency"},
	})
	if len(nodes) != 1 || len(nodes[0].Children) != 1 {
		t.Fatalf("violations-only tree = %#v", nodes)
	}
}
