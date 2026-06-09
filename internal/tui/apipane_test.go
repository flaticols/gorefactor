package tui

import (
	"strings"
	"testing"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

func TestBuildAPIRows_GroupsByKind(t *testing.T) {
	syms := []symbolEntry{
		{sym: graph.Symbol{ID: 1, Kind: "type", Name: "Engine"}},
		{sym: graph.Symbol{ID: 2, Kind: "func", Name: "Run"}, refCount: 3},
		{sym: graph.Symbol{ID: 3, Kind: "const", Name: "Version"}},
	}
	rows := buildAPIRows(syms, map[string]bool{}, nil)

	// Headers must appear in kind order: const, func, type.
	var headerSeq []string
	for _, r := range rows {
		if r.header {
			headerSeq = append(headerSeq, r.label)
		}
	}
	want := []string{"const", "func", "type"}
	if strings.Join(headerSeq, ",") != strings.Join(want, ",") {
		t.Errorf("header order = %v, want %v", headerSeq, want)
	}

	// Run row should carry its ref count in the label.
	found := false
	for _, r := range rows {
		if !r.header && r.sym.Name == "Run" {
			found = true
			if !strings.Contains(r.label, "[3]") {
				t.Errorf("Run label should show ref count, got %q", r.label)
			}
		}
	}
	if !found {
		t.Error("Run row not found")
	}
}

func TestBuildAPIRows_TypeExpandsToMembers(t *testing.T) {
	syms := []symbolEntry{
		{sym: graph.Symbol{ID: 1, Kind: "type", Name: "Engine"}},
	}
	members := map[string][]inspect.StructMember{
		"Engine": {
			{Name: "Start", Kind: "method", Signature: "()", File: "e.go", Line: 5},
			{Name: "Name", Kind: "field", Type: "string", File: "e.go", Line: 2},
		},
	}

	// Collapsed: only the type row, no members.
	rows := buildAPIRows(syms, map[string]bool{}, members)
	for _, r := range rows {
		if r.member {
			t.Fatalf("collapsed type must not emit member rows: %v", r)
		}
	}

	// Expanded: members appear as depth-2 rows.
	rows = buildAPIRows(syms, map[string]bool{"Engine": true}, members)
	memberLabels := map[string]bool{}
	for _, r := range rows {
		if r.member && r.depth == 2 {
			memberLabels[r.memberKind] = true
		}
	}
	if !memberLabels["method"] || !memberLabels["field"] {
		t.Errorf("expanded type should show method and field members, rows: %v", rows)
	}
}

func TestBuildAPIRows_ExpandedTypeWithoutMembersShowsLoading(t *testing.T) {
	syms := []symbolEntry{{sym: graph.Symbol{ID: 1, Kind: "type", Name: "Engine"}}}
	rows := buildAPIRows(syms, map[string]bool{"Engine": true}, nil)
	sawLoading := false
	for _, r := range rows {
		if r.member && strings.Contains(r.label, "loading") {
			sawLoading = true
		}
	}
	if !sawLoading {
		t.Errorf("expanded type without loaded members should show a loading row, got: %v", rows)
	}
}
