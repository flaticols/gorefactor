package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.toml")
	if err := os.WriteFile(path, []byte(`
[[deny]]
from = "tasks"
to = "adapters"
reason = "tasks must not depend on adapters"
`), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	rules, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if len(rules) != 1 || rules[0].From != "tasks" || rules[0].To != "adapters" {
		t.Fatalf("Parse() = %#v", rules)
	}
}

func TestCheckAndFormatters(t *testing.T) {
	baseDir := t.TempDir()
	g := &graph.Graph{
		Funcs: []graph.Func{
			{ID: 1, Module: "example.com/test", Name: "Execute", Package: "example.com/test/tasks", File: filepath.Join(baseDir, "tasks", "process.go"), Line: 123, Col: 5},
			{ID: 2, Module: "example.com/test", Name: "Calculate", Package: "example.com/test/adapters", File: filepath.Join(baseDir, "adapters", "engine.go"), Line: 45, Col: 3},
			{ID: 3, Module: "example.com/test", Name: "Compute", Package: "example.com/test/service", File: filepath.Join(baseDir, "service", "pricing.go"), Line: 67, Col: 2},
		},
		Edges: []graph.Edge{
			{Caller: 1, Callee: 2, File: filepath.Join(baseDir, "tasks", "process.go"), Line: 140, Col: 9},
			{Caller: 3, Callee: 2, File: filepath.Join(baseDir, "service", "pricing.go"), Line: 67, Col: 2},
		},
	}
	g.Index()

	rules := []Rule{
		{From: "example.com/test/tasks", To: "example.com/test/adapters", Reason: "tasks must not depend on adapters"},
		{From: "example.com/test/service", To: "example.com/test/adapters", Reason: "service must not depend on adapters"},
	}

	violations := Check(g, rules)
	if len(violations) != 2 {
		t.Fatalf("Check len = %d, want 2", len(violations))
	}
	if got := CheckEdge(g, g.Edges[0], rules); got == nil || got.From != "example.com/test/tasks" {
		t.Fatalf("CheckEdge = %#v", got)
	}
	if got := CheckEdge(g, g.Edges[1], rules); got == nil || got.From != "example.com/test/service" {
		t.Fatalf("CheckEdge = %#v", got)
	}
	if got := CheckEdge(g, graph.Edge{Caller: 2, Callee: 3}, rules); got != nil {
		t.Fatalf("CheckEdge unexpected hit = %#v", got)
	}

	opts := FormatOptions{BaseDir: baseDir}

	text := FormatText(violations, opts)
	if !strings.Contains(text, "VIOLATION example.com/test/tasks -> example.com/test/adapters") {
		t.Fatalf("FormatText missing rule header: %s", text)
	}
	if !strings.Contains(text, "at tasks/process.go:140:9") {
		t.Fatalf("FormatText missing relative callsite: %s", text)
	}

	jsonBytes, err := FormatJSON(violations, len(rules), opts)
	if err != nil {
		t.Fatalf("FormatJSON error = %v", err)
	}
	if !strings.Contains(string(jsonBytes), `"total_violations": 2`) {
		t.Fatalf("FormatJSON missing summary: %s", jsonBytes)
	}
	if !strings.Contains(string(jsonBytes), `"working_dir":`) || !strings.Contains(string(jsonBytes), `"module": "example.com/test"`) || !strings.Contains(string(jsonBytes), `"callsite":`) {
		t.Fatalf("FormatJSON missing metadata: %s", jsonBytes)
	}
	if strings.Contains(string(jsonBytes), filepath.ToSlash(filepath.Join(baseDir, "tasks", "process.go"))) || strings.Contains(string(jsonBytes), filepath.ToSlash(filepath.Join(baseDir, "adapters", "engine.go"))) {
		t.Fatalf("FormatJSON leaked absolute path: %s", jsonBytes)
	}

	md := FormatMarkdown(violations, len(rules), opts)
	if !strings.Contains(md, "# Dependency Violations Report") || !strings.Contains(md, "| # | Callsite | Caller | Callee | Dynamic |") {
		t.Fatalf("FormatMarkdown invalid output: %s", md)
	}
	if !strings.Contains(md, "`tasks/process.go:140:9`") {
		t.Fatalf("FormatMarkdown missing relative callsite: %s", md)
	}

	qf := FormatQuickfix(violations, opts)
	if !strings.Contains(qf, "tasks/process.go:140:9: VIOLATION example.com/test/tasks→example.com/test/adapters") {
		t.Fatalf("FormatQuickfix invalid output: %s", qf)
	}
}

func TestMatches(t *testing.T) {
	cases := []struct {
		pkg     string
		pattern string
		want    bool
	}{
		{"tasks", "tasks", true},
		{"tasks/sub", "tasks", true},
		{"tasks/sub", "tasks/*", true},
		{"adapters", "tasks", false},
		{"internal/core/db", "internal/core/*", true},
	}
	for _, tc := range cases {
		if got := matches(tc.pkg, tc.pattern); got != tc.want {
			t.Fatalf("matches(%q, %q) = %v, want %v", tc.pkg, tc.pattern, got, tc.want)
		}
	}
}
