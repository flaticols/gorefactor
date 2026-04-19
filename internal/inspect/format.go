package inspect

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.flaticols.dev/gorefactor/internal/graph"
)

// FormatOptions controls non-TTY output rendering.
type FormatOptions struct {
	ViolOnly bool
}

// FormatText returns a human-readable reference tree.
func FormatText(res *InspectResult, opts FormatOptions) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Target:  %s\n", res.Target))
	b.WriteString(fmt.Sprintf("Package: %s\n", res.PkgPath))
	if res.SymbolName != "" {
		b.WriteString(fmt.Sprintf("Symbol:  %s\n", res.SymbolName))
	}
	b.WriteString(fmt.Sprintf("Edges:   %d\n", len(res.Edges)))

	if len(res.Violations) > 0 {
		b.WriteString(fmt.Sprintf("\nViolations (%d):\n", len(res.Violations)))
		for _, v := range res.Violations {
			b.WriteString(fmt.Sprintf("  [DENY] %s → %s: %s\n", v.FromPkg, v.ToPkg, v.Rule.Reason))
		}
	}

	b.WriteString("\nReferences:\n")

	groups := groupEdgesByPkg(res.Edges)
	pkgs := sortedPkgKeys(groups)
	violPkgs := violationPkgSet(res.Violations)
	names := symNames(res)

	for _, pkg := range pkgs {
		edges := groups[pkg]
		if opts.ViolOnly && !violPkgs[pkg] {
			continue
		}
		viol := ""
		if violPkgs[pkg] {
			for _, v := range res.Violations {
				if v.FromPkg == pkg {
					viol = fmt.Sprintf(" [DENY: %s]", v.Rule.Reason)
					break
				}
			}
		}
		b.WriteString(fmt.Sprintf("\n  %s%s (%d refs)\n", pkg, viol, len(edges)))
		for _, e := range edges {
			marker := "✓"
			if e.SamePkg {
				marker = "~"
			} else if violPkgs[e.CallerPkg] {
				marker = "✗"
			}
			sym := symName(names, e.Callee)
			b.WriteString(fmt.Sprintf("    %s %s:%d  %s %s\n",
				marker, e.File, e.Line, string(e.Kind), sym))
		}
	}
	return b.String()
}

type jsonEdge struct {
	CallerPkg string `json:"callerPkg"`
	SymbolID  int    `json:"symbolId"`
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Col       int    `json:"col"`
	SamePkg   bool   `json:"samePkg"`
}

type jsonViolation struct {
	FromPkg string `json:"fromPkg"`
	ToPkg   string `json:"toPkg"`
	Reason  string `json:"reason"`
}

type jsonOutput struct {
	Target     string          `json:"target"`
	PkgPath    string          `json:"pkgPath"`
	SymbolName string          `json:"symbolName,omitempty"`
	Symbols    []graph.Symbol  `json:"symbols"`
	Edges      []jsonEdge      `json:"edges"`
	Violations []jsonViolation `json:"violations"`
}

// FormatJSON returns JSON bytes for the inspect result.
func FormatJSON(res *InspectResult) ([]byte, error) {
	names := symNames(res)
	edges := make([]jsonEdge, len(res.Edges))
	for i, e := range res.Edges {
		edges[i] = jsonEdge{
			CallerPkg: e.CallerPkg,
			SymbolID:  e.Callee,
			Symbol:    symName(names, e.Callee),
			Kind:      string(e.Kind),
			File:      e.File,
			Line:      e.Line,
			Col:       e.Col,
			SamePkg:   e.SamePkg,
		}
	}
	viols := make([]jsonViolation, len(res.Violations))
	for i, v := range res.Violations {
		viols[i] = jsonViolation{FromPkg: v.FromPkg, ToPkg: v.ToPkg, Reason: v.Rule.Reason}
	}
	return json.MarshalIndent(jsonOutput{
		Target:     res.Target,
		PkgPath:    res.PkgPath,
		SymbolName: res.SymbolName,
		Symbols:    res.Symbols,
		Edges:      edges,
		Violations: viols,
	}, "", "  ")
}

// FormatMarkdown returns Markdown output for the inspect result.
func FormatMarkdown(res *InspectResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %s\n\n", res.Target))
	b.WriteString(fmt.Sprintf("**Package:** `%s`  \n", res.PkgPath))
	b.WriteString(fmt.Sprintf("**References:** %d  \n\n", len(res.Edges)))

	if len(res.Violations) > 0 {
		b.WriteString("### Violations\n\n")
		b.WriteString("| From | To | Reason |\n|---|---|---|\n")
		for _, v := range res.Violations {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n",
				v.FromPkg, v.ToPkg, v.Rule.Reason))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Reference Tree\n\n```\n")
	groups := groupEdgesByPkg(res.Edges)
	names := symNames(res)
	for _, pkg := range sortedPkgKeys(groups) {
		b.WriteString(fmt.Sprintf("%s\n", pkg))
		for _, e := range groups[pkg] {
			sym := symName(names, e.Callee)
			b.WriteString(fmt.Sprintf("  %s:%d  %s\n", e.File, e.Line, sym))
		}
	}
	b.WriteString("```\n")
	return b.String()
}

// FormatQuickfix returns quickfix-format lines (file:line:col: message).
func FormatQuickfix(res *InspectResult) string {
	var b strings.Builder
	names := symNames(res)
	violPkgs := violationPkgSet(res.Violations)
	for _, e := range res.Edges {
		if e.File == "" || e.Line == 0 {
			continue
		}
		sym := symName(names, e.Callee)
		msg := fmt.Sprintf("%s.%s (%s)", res.PkgPath, sym, string(e.Kind))
		if violPkgs[e.CallerPkg] {
			msg += " [DENY]"
		}
		b.WriteString(fmt.Sprintf("%s:%d:%d: %s\n", e.File, e.Line, e.Col, msg))
	}
	return b.String()
}

// symNames builds a symbol-ID→name map for fast lookups.
func symNames(res *InspectResult) map[int]string {
	m := make(map[int]string, len(res.Symbols))
	for _, s := range res.Symbols {
		m[s.ID] = s.Name
	}
	return m
}

// symName returns the symbol name for id, or "(id=N)" if not found.
func symName(names map[int]string, id int) string {
	if n, ok := names[id]; ok {
		return n
	}
	return fmt.Sprintf("(id=%d)", id)
}

func groupEdgesByPkg(edges []graph.Edge) map[string][]graph.Edge {
	groups := make(map[string][]graph.Edge)
	for _, e := range edges {
		pkg := e.CallerPkg
		if pkg == "" {
			pkg = "(unknown)"
		}
		groups[pkg] = append(groups[pkg], e)
	}
	return groups
}

func sortedPkgKeys(m map[string][]graph.Edge) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func violationPkgSet(viols []PackageViolation) map[string]bool {
	s := make(map[string]bool, len(viols))
	for _, v := range viols {
		s[v.FromPkg] = true
	}
	return s
}
