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
	g := &graph.Graph{
		Funcs: []graph.Func{
			{ID: 1, Name: "Execute", Package: "tasks", File: "tasks/process.go", Line: 123, Col: 5},
			{ID: 2, Name: "Calculate", Package: "adapters", File: "adapters/engine.go", Line: 45, Col: 3},
			{ID: 3, Name: "Compute", Package: "service", File: "service/pricing.go", Line: 67, Col: 2},
		},
		Edges: []graph.Edge{
			{Caller: 1, Callee: 2, File: "tasks/process.go", Line: 123, Col: 5},
			{Caller: 3, Callee: 2, File: "service/pricing.go", Line: 67, Col: 2},
		},
	}
	g.Index()

	rules := []Rule{
		{From: "tasks", To: "adapters", Reason: "tasks must not depend on adapters"},
		{From: "service", To: "adapters", Reason: "service must not depend on adapters"},
	}

	violations := Check(g, rules)
	if len(violations) != 2 {
		t.Fatalf("Check len = %d, want 2", len(violations))
	}
	if got := CheckEdge(g, g.Edges[0], rules); got == nil || got.From != "tasks" {
		t.Fatalf("CheckEdge = %#v", got)
	}
	if got := CheckEdge(g, g.Edges[1], rules); got == nil || got.From != "service" {
		t.Fatalf("CheckEdge = %#v", got)
	}
	if got := CheckEdge(g, graph.Edge{Caller: 2, Callee: 3}, rules); got != nil {
		t.Fatalf("CheckEdge unexpected hit = %#v", got)
	}

	text := FormatText(violations)
	if !strings.Contains(text, "VIOLATION tasks -> adapters") {
		t.Fatalf("FormatText missing rule header: %s", text)
	}

	jsonBytes, err := FormatJSON(violations, len(rules))
	if err != nil {
		t.Fatalf("FormatJSON error = %v", err)
	}
	if !strings.Contains(string(jsonBytes), `"total_violations": 2`) {
		t.Fatalf("FormatJSON missing summary: %s", jsonBytes)
	}

	md := FormatMarkdown(violations, len(rules))
	if !strings.Contains(md, "# Dependency Violations Report") || !strings.Contains(md, "| # | Caller | Callee | File | Line |") {
		t.Fatalf("FormatMarkdown invalid output: %s", md)
	}

	qf := FormatQuickfix(violations)
	if !strings.Contains(qf, "tasks/process.go:123:5: VIOLATION tasks→adapters") {
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
