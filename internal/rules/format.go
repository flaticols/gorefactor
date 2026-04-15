package rules

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// FormatOptions controls report rendering.
type FormatOptions struct {
	BaseDir string
}

type report struct {
	Metadata   reportMetadata    `json:"metadata"`
	Violations []reportViolation `json:"violations"`
	Summary    reportSummary     `json:"summary"`
}

type reportMetadata struct {
	WorkingDir string `json:"working_dir,omitempty"`
}

type reportViolation struct {
	Rule  Rule         `json:"rule"`
	Count int          `json:"count"`
	Edges []reportEdge `json:"edges"`
}

type reportEdge struct {
	Callsite reportLocation `json:"callsite"`
	Caller   reportFunc     `json:"caller"`
	Callee   reportFunc     `json:"callee"`
	Dynamic  bool           `json:"dynamic"`
}

type reportFunc struct {
	Module    string         `json:"module,omitempty"`
	Package   string         `json:"package"`
	Name      string         `json:"name"`
	Receiver  string         `json:"receiver,omitempty"`
	Signature string         `json:"signature,omitempty"`
	Location  reportLocation `json:"location"`
}

type reportLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
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
func FormatText(violations []Violation, opts FormatOptions) string {
	violations = normalizeViolations(violations, opts)
	if len(violations) == 0 {
		return "No dependency violations found.\n"
	}
	grouped := groupViolations(violations)
	var b strings.Builder
	for i, item := range grouped {
		fmt.Fprintf(&b, "VIOLATION %s -> %s: %s\n", item.rule.From, item.rule.To, item.rule.Reason)
		for _, v := range item.violations {
			fmt.Fprintf(&b, "  at %s:%d:%d\n", v.Edge.File, v.Edge.Line, v.Edge.Col)
			fmt.Fprintf(&b, "    caller: %s.%s\n", v.Caller.Package, fullName(v.Caller))
			fmt.Fprintf(&b, "    callee: %s.%s\n", v.Callee.Package, fullName(v.Callee))
		}
		if i < len(grouped)-1 {
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "\n---\n%d violations across %d rules\n", len(violations), len(grouped))
	return b.String()
}

// FormatJSON renders the report as JSON.
func FormatJSON(violations []Violation, totalRules int, opts FormatOptions) ([]byte, error) {
	rep := buildReport(violations, totalRules, opts)
	return json.MarshalIndent(rep, "", "  ")
}

// FormatMarkdown renders the report for LLM consumption.
func FormatMarkdown(violations []Violation, totalRules int, opts FormatOptions) string {
	violations = normalizeViolations(violations, opts)
	if len(violations) == 0 {
		return "# Dependency Violations Report\n\nNo dependency violations found.\n"
	}
	grouped := groupViolations(violations)
	var b strings.Builder
	b.WriteString("# Dependency Violations Report\n\n")
	if opts.BaseDir != "" {
		fmt.Fprintf(&b, "- Working directory: `%s`\n", filepath.ToSlash(filepath.Clean(opts.BaseDir)))
	}
	fmt.Fprintf(&b, "- Total violations: **%d**\n", len(violations))
	fmt.Fprintf(&b, "- Rules violated: **%d/%d**\n\n", len(grouped), totalRules)
	for _, item := range grouped {
		fmt.Fprintf(&b, "## Rule: `%s` must not depend on `%s`\n\n", item.rule.From, item.rule.To)
		if item.rule.Reason != "" {
			fmt.Fprintf(&b, "- Reason: %s\n", item.rule.Reason)
		}
		fmt.Fprintf(&b, "- Violation count: %d\n", len(item.violations))
		fmt.Fprintf(&b, "- Unique target functions: %d\n\n", countUniqueCallees(item.violations))
		b.WriteString("| # | Callsite | Caller | Callee | Dynamic |\n")
		b.WriteString("|---|----------|--------|--------|---------|\n")
		for i, v := range item.violations {
			fmt.Fprintf(&b, "| %d | `%s:%d:%d` | `%s.%s` | `%s.%s` | `%t` |\n",
				i+1,
				v.Edge.File,
				v.Edge.Line,
				v.Edge.Col,
				v.Caller.Package,
				fullName(v.Caller),
				v.Callee.Package,
				fullName(v.Callee),
				v.Edge.Dynamic,
			)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "**Summary**: %d violations across %d rules (%d rules total, %d clean)\n", len(violations), len(grouped), totalRules, totalRules-len(grouped))
	return b.String()
}

// FormatQuickfix renders quickfix-compatible lines.
func FormatQuickfix(violations []Violation, opts FormatOptions) string {
	violations = normalizeViolations(violations, opts)
	if len(violations) == 0 {
		return ""
	}
	var b strings.Builder
	for _, v := range violations {
		fmt.Fprintf(&b, "%s:%d:%d: VIOLATION %s→%s: %s.%s → %s.%s [%s]\n",
			v.Edge.File, v.Edge.Line, v.Edge.Col,
			v.Rule.From, v.Rule.To,
			v.Caller.Package, fullName(v.Caller),
			v.Callee.Package, fullName(v.Callee),
			v.Rule.Reason)
	}
	return b.String()
}

func buildReport(violations []Violation, totalRules int, opts FormatOptions) report {
	violations = normalizeViolations(violations, opts)
	grouped := groupViolations(violations)
	out := report{
		Metadata: reportMetadata{
			WorkingDir: filepath.ToSlash(filepath.Clean(strings.TrimSpace(opts.BaseDir))),
		},
		Violations: make([]reportViolation, 0, len(grouped)),
		Summary: reportSummary{
			TotalViolations: len(violations),
			RulesViolated:   len(grouped),
			RulesTotal:      totalRules,
		},
	}
	for _, item := range grouped {
		rv := reportViolation{
			Rule:  item.rule,
			Count: len(item.violations),
			Edges: make([]reportEdge, 0, len(item.violations)),
		}
		for _, v := range item.violations {
			rv.Edges = append(rv.Edges, reportEdge{
				Callsite: reportLocation{
					File:   v.Edge.File,
					Line:   v.Edge.Line,
					Column: v.Edge.Col,
				},
				Caller: reportFunc{
					Module:    v.Caller.Module,
					Package:   v.Caller.Package,
					Name:      v.Caller.Name,
					Receiver:  v.Caller.Receiver,
					Signature: v.Caller.Signature,
					Location: reportLocation{
						File:   v.Caller.File,
						Line:   v.Caller.Line,
						Column: v.Caller.Col,
					},
				},
				Callee: reportFunc{
					Module:    v.Callee.Module,
					Package:   v.Callee.Package,
					Name:      v.Callee.Name,
					Receiver:  v.Callee.Receiver,
					Signature: v.Callee.Signature,
					Location: reportLocation{
						File:   v.Callee.File,
						Line:   v.Callee.Line,
						Column: v.Callee.Col,
					},
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

func normalizeViolations(violations []Violation, opts FormatOptions) []Violation {
	if len(violations) == 0 {
		return nil
	}
	out := make([]Violation, len(violations))
	for i, violation := range violations {
		violation.Caller.File = cleanPath(opts.BaseDir, violation.Caller.File)
		violation.Callee.File = cleanPath(opts.BaseDir, violation.Callee.File)
		violation.Edge.File = cleanPath(opts.BaseDir, violation.Edge.File)
		out[i] = violation
	}
	return out
}

func cleanPath(baseDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if filepath.IsAbs(path) && strings.TrimSpace(baseDir) != "" {
		if rel, err := filepath.Rel(baseDir, path); err == nil {
			path = rel
		}
	}
	return filepath.ToSlash(path)
}

func fullName(fn graph.Func) string {
	name := strings.TrimSpace(fn.Name)
	recv := strings.TrimSpace(fn.Receiver)
	if recv == "" {
		return name
	}
	return recv + "." + name
}
