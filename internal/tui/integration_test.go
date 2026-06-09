package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
)

func smokeLoad(t *testing.T) Model {
	t.Helper()
	cfg := inspect.Config{Loader: loader.Config{Dir: "../..", Patterns: []string{"./..."}}}
	m := New("internal/loader", cfg)
	pg, err := inspect.BuildGraph(cfg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	nm, cmd := m.Update(graphMsg{pg: pg})
	m = nm.(Model)
	if m.selectedPkg == "" {
		t.Fatalf("expected auto-select of internal/loader")
	}
	if cmd != nil {
		nm, _ = m.Update(cmd())
		m = nm.(Model)
	}
	return m
}

func TestSmokeRealRepo(t *testing.T) {
	m := smokeLoad(t)
	t.Logf("picker rows: %d, API rows: %d, ref rows: %d", len(m.pickRows), len(m.apiRows), len(m.refRows))
	if len(m.apiRows) == 0 {
		t.Fatal("expected API rows for internal/loader")
	}
	if len(m.refRows) == 0 {
		t.Fatal("expected importer rows in package focus")
	}
	// Expand PackageGraph type → members appear.
	m.active = paneAPI
	for i, r := range m.apiRows {
		if !r.header && r.expandable && r.sym.Name == "PackageGraph" {
			m.apiIdx = i
			break
		}
	}
	nm, cmd := m.handleEnter()
	m = nm.(Model)
	if cmd != nil {
		nm, _ = m.Update(cmd())
		m = nm.(Model)
	}
	members := 0
	for _, r := range m.apiRows {
		if r.member && r.memberKind != "" {
			members++
		}
	}
	if members == 0 {
		t.Error("expected struct members after expanding PackageGraph")
	}
	t.Logf("members after expand: %d", members)
}

func TestSmokeDetailMode(t *testing.T) {
	m := smokeLoad(t)
	if m.symbolFocus() != (graph.Symbol{}) {
		t.Fatal("should start in package focus")
	}
	m.active = paneAPI
	for i, r := range m.apiRows {
		if !r.header && r.sym.Kind == "func" {
			m.apiIdx = i
			break
		}
	}
	nm, _ := m.handleEnter()
	m = nm.(Model)
	if !m.detail {
		t.Fatal("enter on a symbol should turn on detail mode")
	}
	t.Logf("detail refLines: %d for %s", len(m.refLines), m.apiRows[m.apiIdx].sym.Name)
	nm, _ = m.updateNormal(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.detail {
		t.Fatal("esc should leave detail mode")
	}
	if len(m.refRows) == 0 {
		t.Fatal("esc should restore importer rows")
	}
}
