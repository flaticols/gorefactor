package rules

import (
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// Violation records a rule hit for a specific edge.
type Violation struct {
	Rule   Rule
	Caller graph.Func
	Callee graph.Func
	Edge   graph.Edge
}

// Check evaluates all edges in the graph against the provided rules.
func Check(g *graph.Graph, rules []Rule) []Violation {
	if g == nil || len(rules) == 0 {
		return nil
	}
	g.Index()

	var out []Violation
	for _, edge := range g.Edges {
		rule, caller, callee, ok := checkEdge(g, edge, rules)
		if !ok {
			continue
		}
		out = append(out, Violation{
			Rule:   *rule,
			Caller: caller,
			Callee: callee,
			Edge:   edge,
		})
	}
	return out
}

// CheckEdge returns the first matching rule for the given edge or nil.
func CheckEdge(g *graph.Graph, edge graph.Edge, rules []Rule) *Rule {
	rule, _, _, ok := checkEdge(g, edge, rules)
	if !ok {
		return nil
	}
	return rule
}

func checkEdge(g *graph.Graph, edge graph.Edge, rules []Rule) (*Rule, graph.Func, graph.Func, bool) {
	caller, ok := g.FuncByID(edge.Caller)
	if !ok {
		return nil, graph.Func{}, graph.Func{}, false
	}
	callee, ok := g.FuncByID(edge.Callee)
	if !ok {
		return nil, graph.Func{}, graph.Func{}, false
	}
	for i := range rules {
		rule := rules[i]
		if matches(caller.Package, rule.From) && matches(callee.Package, rule.To) {
			return &rule, caller, callee, true
		}
	}
	return nil, graph.Func{}, graph.Func{}, false
}

func matches(pkg, pattern string) bool {
	pkg = strings.TrimSpace(pkg)
	pattern = strings.TrimSpace(pattern)
	if pkg == "" || pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
	}
	return strings.Contains(pkg, pattern)
}
