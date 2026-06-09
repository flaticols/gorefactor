package tui

import (
	"fmt"
	"sort"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

// apiRow is one navigable line in the public-API pane (pane 2).
type apiRow struct {
	header     bool         // kind group header (const/var/func/type) — not selectable as a symbol
	sym        graph.Symbol // the symbol this row represents (when !header and !member)
	member     bool         // true when this row is a struct/interface member
	memberKind string       // "method" or "field"
	label      string       // display text (without indentation)
	depth      int
	expandable bool // a type row that can expand into members
	expanded   bool
	file       string
	line       int
	refCount   int
}

// kindOrder controls the grouping order of symbol kinds in the API pane.
var kindOrder = []string{"const", "var", "func", "type"}

func kindRank(kind string) int {
	for i, k := range kindOrder {
		if k == kind {
			return i
		}
	}
	return len(kindOrder)
}

// buildAPIRows lays out a package's exported symbols grouped by kind. A type
// symbol whose path is in expanded is expanded inline into its members (loaded
// lazily into the members map, keyed by symbol name).
func buildAPIRows(
	syms []symbolEntry,
	expanded map[string]bool,
	members map[string][]inspect.StructMember,
) []apiRow {
	if len(syms) == 0 {
		return nil
	}
	ordered := append([]symbolEntry(nil), syms...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, rj := kindRank(ordered[i].sym.Kind), kindRank(ordered[j].sym.Kind)
		if ri != rj {
			return ri < rj
		}
		return ordered[i].sym.Name < ordered[j].sym.Name
	})

	var rows []apiRow
	lastKind := ""
	for _, se := range ordered {
		s := se.sym
		if s.Kind != lastKind {
			lastKind = s.Kind
			rows = append(rows, apiRow{header: true, label: s.Kind, depth: 0})
		}
		isType := s.Kind == "type"
		exp := expanded[s.Name]
		refSuffix := ""
		if se.refCount > 0 {
			refSuffix = fmt.Sprintf(" [%d]", se.refCount)
		}
		rows = append(rows, apiRow{
			sym:        s,
			label:      s.Name + refSuffix,
			depth:      1,
			expandable: isType,
			expanded:   exp,
			file:       s.File,
			line:       s.Line,
			refCount:   se.refCount,
		})
		if isType && exp {
			mems, ok := members[s.Name]
			if !ok {
				rows = append(rows, apiRow{member: true, label: "loading…", depth: 2})
				continue
			}
			if len(mems) == 0 {
				rows = append(rows, apiRow{member: true, label: "(no exported members)", depth: 2})
				continue
			}
			for _, mem := range mems {
				var body string
				switch mem.Kind {
				case "method":
					body = mem.Name + mem.Signature
				case "field":
					body = mem.Name + ": " + mem.Type
				default:
					body = mem.Name
				}
				refSuffix := ""
				if n := len(mem.Edges); n > 0 {
					refSuffix = fmt.Sprintf(" [%d]", n)
				}
				rows = append(rows, apiRow{
					member:     true,
					memberKind: mem.Kind,
					label:      body + refSuffix,
					depth:      2,
					file:       mem.File,
					line:       mem.Line,
					refCount:   len(mem.Edges),
				})
			}
		}
	}
	return rows
}
