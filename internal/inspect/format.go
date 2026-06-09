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
	// ModuleOnly restricts importers and the reference summary to packages in
	// the main module. When the result has no module path it is a no-op.
	ModuleOnly bool
}

// FormatText returns a human-readable package report: public API, importers,
// and a per-package reference summary.
func FormatText(res *InspectResult, opts FormatOptions) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Target:  %s\n", res.Target))
	b.WriteString(fmt.Sprintf("Package: %s\n", res.PkgPath))
	if res.SymbolName != "" {
		b.WriteString(fmt.Sprintf("Symbol:  %s\n", res.SymbolName))
	}

	importers := importerPaths(res, opts.ModuleOnly)
	edges := filterEdges(res.Edges, opts, modulePrefix(res))

	b.WriteString(fmt.Sprintf("Public API: %d  Importers: %d  References: %d\n",
		len(res.Symbols), len(importers), len(edges)))

	// Public API.
	b.WriteString("\nPublic API:\n")
	groups := groupSymbolsByKind(res.Symbols)
	for _, kind := range symbolKindOrder {
		syms := groups[kind]
		if len(syms) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\n  %s\n", kindHeading(kind)))
		for _, s := range syms {
			if s.File != "" {
				b.WriteString(fmt.Sprintf("    %s  %s:%d\n", s.Name, s.File, s.Line))
			} else {
				b.WriteString(fmt.Sprintf("    %s\n", s.Name))
			}
		}
	}

	// Importers.
	b.WriteString("\nImporters:\n")
	if len(importers) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, pkg := range importers {
			b.WriteString(fmt.Sprintf("  %s\n", pkg))
		}
	}

	// Reference summary.
	b.WriteString("\nReference Summary:\n")
	refGroups := groupEdgesByPkg(edges)
	pkgs := sortedPkgKeys(refGroups)
	names := symNames(res)
	for _, pkg := range pkgs {
		group := refGroups[pkg]
		b.WriteString(fmt.Sprintf("\n  %s (%d refs)\n", pkg, len(group)))
		for _, e := range group {
			marker := "✓"
			if e.SamePkg {
				marker = "~"
			}
			sym := symName(names, e.Callee)
			b.WriteString(fmt.Sprintf("    %s %s:%d  %s %s\n",
				marker, e.File, e.Line, string(e.Kind), sym))
		}
	}
	return b.String()
}

type jsonSymbol struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
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

type jsonOutput struct {
	Target     string       `json:"target"`
	PkgPath    string       `json:"pkgPath"`
	SymbolName string       `json:"symbolName,omitempty"`
	PublicAPI  []jsonSymbol `json:"publicApi"`
	Importers  []string     `json:"importers"`
	References []jsonEdge   `json:"references"`
}

// FormatJSON returns JSON bytes for the package report. The output contains no
// rules or violations fields.
func FormatJSON(res *InspectResult) ([]byte, error) {
	return FormatJSONWithOptions(res, FormatOptions{})
}

// FormatJSONWithOptions is like FormatJSON but honours the format options
// (e.g. ModuleOnly filtering of importers and references).
func FormatJSONWithOptions(res *InspectResult, opts FormatOptions) ([]byte, error) {
	names := symNames(res)
	prefix := modulePrefix(res)

	api := make([]jsonSymbol, 0, len(res.Symbols))
	for _, s := range res.Symbols {
		api = append(api, jsonSymbol{
			Kind: s.Kind,
			Name: s.Name,
			File: s.File,
			Line: s.Line,
		})
	}

	importers := importerPaths(res, opts.ModuleOnly)

	srcEdges := filterEdges(res.Edges, opts, prefix)
	edges := make([]jsonEdge, len(srcEdges))
	for i, e := range srcEdges {
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

	return json.MarshalIndent(jsonOutput{
		Target:     res.Target,
		PkgPath:    res.PkgPath,
		SymbolName: res.SymbolName,
		PublicAPI:  api,
		Importers:  importers,
		References: edges,
	}, "", "  ")
}

// FormatMarkdown returns a Markdown package report.
func FormatMarkdown(res *InspectResult) string {
	return FormatMarkdownWithOptions(res, FormatOptions{})
}

// FormatMarkdownWithOptions is like FormatMarkdown but honours the format options.
func FormatMarkdownWithOptions(res *InspectResult, opts FormatOptions) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %s\n\n", res.Target))
	b.WriteString(fmt.Sprintf("**Package:** `%s`  \n", res.PkgPath))
	if res.SymbolName != "" {
		b.WriteString(fmt.Sprintf("**Symbol:** `%s`  \n", res.SymbolName))
	}

	importers := importerPaths(res, opts.ModuleOnly)
	edges := filterEdges(res.Edges, opts, modulePrefix(res))
	b.WriteString(fmt.Sprintf("**Public API:** %d &nbsp; **Importers:** %d &nbsp; **References:** %d  \n\n",
		len(res.Symbols), len(importers), len(edges)))

	// Public API.
	b.WriteString("### Public API\n\n")
	groups := groupSymbolsByKind(res.Symbols)
	any := false
	for _, kind := range symbolKindOrder {
		syms := groups[kind]
		if len(syms) == 0 {
			continue
		}
		any = true
		b.WriteString(fmt.Sprintf("**%s**\n\n", kindHeading(kind)))
		for _, s := range syms {
			if s.File != "" {
				b.WriteString(fmt.Sprintf("- `%s` — `%s:%d`\n", s.Name, s.File, s.Line))
			} else {
				b.WriteString(fmt.Sprintf("- `%s`\n", s.Name))
			}
		}
		b.WriteString("\n")
	}
	if !any {
		b.WriteString("_None._\n\n")
	}

	// Importers.
	b.WriteString("### Importers\n\n")
	if len(importers) == 0 {
		b.WriteString("_None._\n\n")
	} else {
		for _, pkg := range importers {
			b.WriteString(fmt.Sprintf("- `%s`\n", pkg))
		}
		b.WriteString("\n")
	}

	// Reference summary.
	b.WriteString("### Reference Summary\n\n```\n")
	refGroups := groupEdgesByPkg(edges)
	names := symNames(res)
	for _, pkg := range sortedPkgKeys(refGroups) {
		b.WriteString(fmt.Sprintf("%s\n", pkg))
		for _, e := range refGroups[pkg] {
			sym := symName(names, e.Callee)
			b.WriteString(fmt.Sprintf("  %s:%d  %s\n", e.File, e.Line, sym))
		}
	}
	b.WriteString("```\n")
	return b.String()
}

// symbolKindOrder is the stable display order for the public API section.
var symbolKindOrder = []string{"const", "var", "func", "type"}

func kindHeading(kind string) string {
	switch kind {
	case "const":
		return "Constants"
	case "var":
		return "Variables"
	case "func":
		return "Functions"
	case "type":
		return "Types"
	default:
		return kind
	}
}

// groupSymbolsByKind buckets symbols by kind and sorts each bucket by name.
func groupSymbolsByKind(syms []graph.Symbol) map[string][]graph.Symbol {
	groups := make(map[string][]graph.Symbol)
	for _, s := range syms {
		groups[s.Kind] = append(groups[s.Kind], s)
	}
	for kind := range groups {
		sort.Slice(groups[kind], func(i, j int) bool {
			return groups[kind][i].Name < groups[kind][j].Name
		})
	}
	return groups
}

// importerPaths returns the import paths of packages that import the target,
// optionally restricted to the main module.
func importerPaths(res *InspectResult, moduleOnly bool) []string {
	if res.PkgGraph == nil {
		return nil
	}
	prefix := res.PkgGraph.Module
	var out []string
	for _, n := range res.PkgGraph.ImportersOf(res.PkgPath) {
		if moduleOnly && prefix != "" && !inModule(n.Path, prefix) {
			continue
		}
		out = append(out, n.Path)
	}
	sort.Strings(out)
	return out
}

// modulePrefix returns the main module path for the result, if known.
func modulePrefix(res *InspectResult) string {
	if res.PkgGraph == nil {
		return ""
	}
	return res.PkgGraph.Module
}

// filterEdges optionally drops edges whose caller package is outside the module.
func filterEdges(edges []graph.Edge, opts FormatOptions, prefix string) []graph.Edge {
	if !opts.ModuleOnly || prefix == "" {
		return edges
	}
	out := make([]graph.Edge, 0, len(edges))
	for _, e := range edges {
		if e.CallerPkg != "" && !inModule(e.CallerPkg, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// inModule reports whether pkg belongs to the module rooted at prefix.
func inModule(pkg, prefix string) bool {
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
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
