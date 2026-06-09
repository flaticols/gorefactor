package inspect_test

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
)

func makeTestResult() *inspect.InspectResult {
	return &inspect.InspectResult{
		Target:  "example.com/tasks",
		PkgPath: "example.com/tasks",
		Symbols: []graph.Symbol{
			{ID: 100, Kind: "type", Name: "Engine", Package: "example.com/tasks", File: "tasks/engine.go", Line: 5, Exported: true},
			{ID: 101, Kind: "func", Name: "Run", Package: "example.com/tasks", File: "tasks/run.go", Line: 8, Exported: true},
		},
		Edges: []graph.Edge{
			{Caller: 0, Callee: 100, Kind: graph.EdgeCall, CallerPkg: "example.com/handler", File: "handler/h.go", Line: 42, Col: 5, SamePkg: false},
			{Caller: 0, Callee: 100, Kind: graph.EdgeTypeRef, CallerPkg: "example.com/service", File: "service/s.go", Line: 10, Col: 3, SamePkg: false},
		},
		PkgGraph: &loader.PackageGraph{
			Module: "example.com",
			Nodes: map[string]*loader.PackageNode{
				"example.com/tasks":   {Path: "example.com/tasks", Name: "tasks"},
				"example.com/handler": {Path: "example.com/handler", Name: "handler", Imports: []string{"example.com/tasks"}},
				"example.com/service": {Path: "example.com/service", Name: "service", Imports: []string{"example.com/tasks"}},
			},
		},
	}
}

func TestFormatText(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatText(res, inspect.FormatOptions{})
	for _, want := range []string{"Public API:", "Importers:", "Engine", "Run", "example.com/handler", "handler/h.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatText missing %q, got:\n%s", want, out)
		}
	}
}

func TestFormatTextModuleOnly(t *testing.T) {
	res := makeTestResult()
	// Add an external importer/edge that module-only should drop.
	res.PkgGraph.Nodes["other.com/ext"] = &loader.PackageNode{
		Path: "other.com/ext", Name: "ext", Imports: []string{"example.com/tasks"},
	}
	res.Edges = append(res.Edges, graph.Edge{
		Caller: 0, Callee: 100, Kind: graph.EdgeCall, CallerPkg: "other.com/ext", File: "ext/e.go", Line: 1,
	})

	out := inspect.FormatText(res, inspect.FormatOptions{ModuleOnly: true})
	if strings.Contains(out, "other.com/ext") {
		t.Errorf("module-only output should drop external importer, got:\n%s", out)
	}
	full := inspect.FormatText(res, inspect.FormatOptions{ModuleOnly: false})
	if !strings.Contains(full, "other.com/ext") {
		t.Errorf("non-module-only output should keep external importer, got:\n%s", full)
	}
}

func TestFormatJSON(t *testing.T) {
	res := makeTestResult()
	data, err := inspect.FormatJSON(res)
	if err != nil {
		t.Fatalf("FormatJSON error = %v", err)
	}
	s := string(data)
	for _, want := range []string{`"target"`, `"publicApi"`, `"importers"`, `"references"`, `"example.com/handler"`} {
		if !strings.Contains(s, want) {
			t.Errorf("FormatJSON missing %q: %s", want, s)
		}
	}
	for _, banned := range []string{"violation", "Violation", "rules", "Rules", "deny"} {
		if strings.Contains(s, banned) {
			t.Errorf("FormatJSON should not contain %q: %s", banned, s)
		}
	}
}

func TestFormatMarkdown(t *testing.T) {
	res := makeTestResult()
	out := inspect.FormatMarkdown(res)
	for _, want := range []string{"## ", "### Public API", "### Importers", "example.com/tasks"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatMarkdown missing %q: %s", want, out)
		}
	}
}
