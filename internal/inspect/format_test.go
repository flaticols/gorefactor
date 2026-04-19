package inspect_test

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
	"go.flaticols.dev/gorefactor/internal/rules"
)

func makeTestResult() *inspect.InspectResult {
	return &inspect.InspectResult{
		Target:  "example.com/tasks",
		PkgPath: "example.com/tasks",
		Symbols: []graph.Symbol{
			{ID: 100, Kind: "type", Name: "Engine", Package: "example.com/tasks", File: "tasks/engine.go", Line: 5, Exported: true},
		},
		Edges: []graph.Edge{
			{Caller: 0, Callee: 100, Kind: graph.EdgeCall, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42, Col: 5, SamePkg: false},
			{Caller: 0, Callee: 100, Kind: graph.EdgeTypeRef, CallerPkg: "example.com/service", File: "service/s.go", Line: 10, Col: 3, SamePkg: false},
		},
		PkgGraph: &loader.PackageGraph{Nodes: map[string]*loader.PackageNode{
			"example.com/tasks":   {Path: "example.com/tasks", Name: "tasks"},
			"example.com/handler": {Path: "example.com/handler", Name: "handler", Imports: []string{"example.com/tasks"}},
			"example.com/service": {Path: "example.com/service", Name: "service", Imports: []string{"example.com/tasks"}},
		}},
		Violations: []inspect.PackageViolation{
			{FromPkg: "example.com/handler", ToPkg: "example.com/tasks", Rule: rules.Rule{From: "handler", To: "tasks", Reason: "use service layer"}},
		},
	}
}

func TestFormatText(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatText(res, inspect.FormatOptions{})
	if !strings.Contains(out, "example.com/handler") {
		t.Errorf("FormatText missing handler pkg, got:\n%s", out)
	}
	if !strings.Contains(out, "DENY") {
		t.Errorf("FormatText missing DENY marker, got:\n%s", out)
	}
	if !strings.Contains(out, "handler/h.go") {
		t.Errorf("FormatText missing file reference, got:\n%s", out)
	}
}

func TestFormatJSON(t *testing.T) {
	res := makeTestResult()
	data, err := inspect.FormatJSON(res)
	if err != nil {
		t.Fatalf("FormatJSON error = %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"target"`) {
		t.Errorf("FormatJSON missing target field: %s", s)
	}
	if !strings.Contains(s, `"example.com/handler"`) {
		t.Errorf("FormatJSON missing callerPkg: %s", s)
	}
}

func TestFormatMarkdown(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatMarkdown(res)
	if !strings.Contains(out, "## ") {
		t.Errorf("FormatMarkdown missing heading: %s", out)
	}
	if !strings.Contains(out, "example.com/tasks") {
		t.Errorf("FormatMarkdown missing package: %s", out)
	}
}

func TestFormatQuickfix(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatQuickfix(res)
	if !strings.Contains(out, "handler/h.go:42") {
		t.Errorf("FormatQuickfix missing file:line, got:\n%s", out)
	}
}
