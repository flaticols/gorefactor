package rules

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type report struct {
	Violations []reportViolation `json:"violations"`
	Summary    reportSummary     `json:"summary"`
}

type reportViolation struct {
	Rule   Rule           `json:"rule"`
	Edges  []reportEdge   `json:"edges"`
}

type reportEdge struct {
	Caller reportFunc `json:"caller"`
	Callee reportFunc `json:"callee"`
	Dynamic bool      `json:"dynamic"`
}

type reportFunc struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}

type reportSummary struct {
	TotalViolations int `json:"total_violations"`
	RulesViolated   int `json:"rules_violated"`
	RulesTotal      int `json:"rules_total"`
}

type groupedViolations struct {
	rule       Rule
	violations []Violation
}

// FormatText renders violations in a human-readable grouped format.
func FormatText(violations []Violation) string {
	if len(violations) == 0 {
		return "No dependency violations found.\n"
	}
	grouped := groupViolations(violations)
	var b strings.Builder
	for i, item := range grouped {
		fmt.Fprintf(&b, "VIOLATION %s -> %s: %s\n", item.rule.From, item.rule.To, item.rule.Reason)
		for _, v := range item.violations {
			fmt.Fprintf(&b, "  %s:%d\t%s -> %s\n", v.Caller.File, v.Caller.Line, v.Caller.Name, v.Callee.Name)
		}
		if i < len(grouped)-1 {
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "\n---\n%d violations across %d rules\n", len(violations), len(grouped))
	return b.String()
}

// FormatJSON renders the report as JSON.
func FormatJSON(violations []Violation, totalRules int) ([]byte, error) {
	rep := buildReport(violations, totalRules)
	return json.MarshalIndent(rep, "", "  ")
}

// FormatMarkdown renders the report for LLM consumption.
func FormatMarkdown(violations []Violation, totalRules int) string {
	if len(violations) == 0 {
		return "# Dependency Violations Report\n\nNo dependency violations found.\n"
	}
	grouped := groupViolations(violations)
	var b strings.Builder
	b.WriteString("# Dependency Violations Report\n\n")
	for _, item := range grouped {
		fmt.Fprintf(&b, "## Rule: `%s` must not depend on `%s`\n\n", item.rule.From, item.rule.To)
		b.WriteString("| # | Caller | Callee | File | Line |\n")
		b.WriteString("|---|--------|--------|------|------|\n")
		for i, v := range item.violations {
			fmt.Fprintf(&b, "| %d | `%s` | `%s` | `%s` | %d |\n", i+1, v.Caller.Name, v.Callee.Name, v.Caller.File, v.Caller.Line)
		}
		fmt.Fprintf(&b, "\n### Context for fix\n")
		fmt.Fprintf(&b, "- Caller package: `%s` — %d violations\n", item.rule.From, len(item.violations))
		fmt.Fprintf(&b, "- Callee package: `%s` — %d unique target functions\n", item.rule.To, countUniqueCallees(item.violations))
		if ctx := contextSuggestion(item.rule); ctx != "" {
			fmt.Fprintf(&b, "- Suggested approach: %s\n", ctx)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "**Summary**: %d violations across %d rules (%d rules total, %d clean)\n", len(violations), len(grouped), totalRules, totalRules-len(grouped))
	return b.String()
}

// FormatQuickfix renders quickfix-compatible lines.
func FormatQuickfix(violations []Violation) string {
	if len(violations) == 0 {
		return ""
	}
	var b strings.Builder
	for _, v := range violations {
		fmt.Fprintf(&b, "%s:%d:%d: VIOLATION %s→%s: %s → %s [%s]\n",
			v.Caller.File, v.Caller.Line, v.Caller.Col,
			v.Rule.From, v.Rule.To, v.Caller.Name, v.Callee.Name, v.Rule.Reason)
	}
	return b.String()
}

func buildReport(violations []Violation, totalRules int) report {
	grouped := groupViolations(violations)
	out := report{
		Violations: make([]reportViolation, 0, len(grouped)),
		Summary: reportSummary{
			TotalViolations: len(violations),
			RulesViolated:   len(grouped),
			RulesTotal:      totalRules,
		},
	}
	for _, item := range grouped {
		rv := reportViolation{Rule: item.rule, Edges: make([]reportEdge, 0, len(item.violations))}
		for _, v := range item.violations {
			rv.Edges = append(rv.Edges, reportEdge{
				Caller: reportFunc{
					Package: v.Caller.Package,
					Name:    v.Caller.Name,
					File:    v.Caller.File,
					Line:    v.Caller.Line,
				},
				Callee: reportFunc{
					Package: v.Callee.Package,
					Name:    v.Callee.Name,
					File:    v.Callee.File,
					Line:    v.Callee.Line,
				},
				Dynamic: v.Edge.Dynamic,
			})
		}
		out.Violations = append(out.Violations, rv)
	}
	return out
}

func groupViolations(violations []Violation) []groupedViolations {
	index := make(map[string]int)
	var groups []groupedViolations
	for _, v := range violations {
		key := v.Rule.From + "\x00" + v.Rule.To + "\x00" + v.Rule.Reason
		idx, ok := index[key]
		if !ok {
			index[key] = len(groups)
			groups = append(groups, groupedViolations{rule: v.Rule})
			idx = len(groups) - 1
		}
		groups[idx].violations = append(groups[idx].violations, v)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].rule.From != groups[j].rule.From {
			return groups[i].rule.From < groups[j].rule.From
		}
		if groups[i].rule.To != groups[j].rule.To {
			return groups[i].rule.To < groups[j].rule.To
		}
		return groups[i].rule.Reason < groups[j].rule.Reason
	})
	return groups
}

func countUniqueCallees(violations []Violation) int {
	seen := make(map[string]struct{})
	for _, v := range violations {
		seen[v.Callee.Package+"."+v.Callee.Name] = struct{}{}
	}
	return len(seen)
}

func contextSuggestion(rule Rule) string {
	switch {
	case strings.Contains(rule.To, "adapters"):
		return "introduce an interface in the caller package that the callee package implements"
	case strings.Contains(rule.To, "repository"):
		return "route calls through a service layer"
	default:
		return "introduce an abstraction boundary in the caller package"
	}
}
