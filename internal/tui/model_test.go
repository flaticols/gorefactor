package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

func resultFor(pkg string, syms ...graph.Symbol) *inspect.InspectResult {
	return &inspect.InspectResult{PkgPath: pkg, Target: pkg, Symbols: syms}
}

func TestSelectPackage_FiresLoadAndShowsSpinner(t *testing.T) {
	m := newTestModel(testGraph())
	pkg := testModule + "/internal/graph"
	nm, cmd := m.selectPackage(pkg)
	mm := nm.(Model)
	if mm.selectedPkg != pkg {
		t.Fatalf("selectedPkg = %q", mm.selectedPkg)
	}
	if !mm.pendingLoad {
		t.Error("expected pendingLoad after selecting an uncached package")
	}
	if cmd == nil {
		t.Error("expected a ResolveTarget command to be fired")
	}
	if mm.active != paneAPI {
		t.Errorf("active pane should move to API on select, got %v", mm.active)
	}
}

func TestSelectPackage_UsesCache(t *testing.T) {
	m := newTestModel(testGraph())
	pkg := testModule + "/internal/graph"
	m.cache[pkg] = resultFor(pkg, graph.Symbol{ID: 1, Kind: "func", Name: "Index"})

	nm, cmd := m.selectPackage(pkg)
	mm := nm.(Model)
	if cmd != nil {
		t.Error("cached package should not fire a load command")
	}
	if mm.pendingLoad {
		t.Error("cached package should not be pending")
	}
	if len(mm.apiRows) == 0 {
		t.Error("cached package should render API rows immediately")
	}
}

func TestLoadDoneMsg_StaleLoadDoesNotClobber(t *testing.T) {
	m := newTestModel(testGraph())
	pkgA := testModule + "/internal/graph"
	pkgB := testModule + "/internal/loader"

	// User selected B.
	m.selectedPkg = pkgB
	m.pendingLoad = true

	// A's load (stale) arrives.
	msgA := loadDoneMsg{pkgPath: pkgA, result: resultFor(pkgA, graph.Symbol{ID: 1, Kind: "func", Name: "Old"})}
	nm, _ := m.Update(msgA)
	mm := nm.(Model)

	if _, ok := mm.cache[pkgA]; !ok {
		t.Error("stale result should still be cached for later reuse")
	}
	if !mm.pendingLoad {
		t.Error("stale load must not clear pendingLoad for the active package B")
	}
	if mm.selectedPkg != pkgB {
		t.Errorf("selectedPkg changed to %q on stale load", mm.selectedPkg)
	}

	// B's load arrives → panes update.
	msgB := loadDoneMsg{pkgPath: pkgB, result: resultFor(pkgB, graph.Symbol{ID: 2, Kind: "func", Name: "New"})}
	nm2, _ := mm.Update(msgB)
	mm2 := nm2.(Model)
	if mm2.pendingLoad {
		t.Error("active load should clear pendingLoad")
	}
}

func TestGraphMsg_AutoSelectsInitialTarget(t *testing.T) {
	pg := testGraph()
	m := New("internal/loader", inspect.Config{})
	nm, cmd := m.Update(graphMsg{pg: pg})
	mm := nm.(Model)
	if mm.selectedPkg != testModule+"/internal/loader" {
		t.Errorf("initial target should auto-select internal/loader, got %q", mm.selectedPkg)
	}
	if cmd == nil {
		t.Error("auto-select should fire a load command")
	}
}

func TestPickerModeCycle(t *testing.T) {
	m := newTestModel(testGraph())
	m = m.rebuildPicker()
	if m.pickMode != pickerFlat {
		t.Fatalf("default picker mode should be flat")
	}
	nm, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	mm := nm.(Model)
	if mm.pickMode != pickerFolder {
		t.Errorf("t should cycle flat → folder, got %v", mm.pickMode)
	}
	nm2, _ := mm.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if nm2.(Model).pickMode != pickerImport {
		t.Errorf("t should cycle folder → import")
	}
}

func TestDetailMode_EnterAndEsc(t *testing.T) {
	m := newTestModel(testGraph())
	pkg := testModule + "/internal/graph"
	m.cache[pkg] = resultFor(pkg,
		graph.Symbol{ID: 1, Kind: "func", Name: "Index", Package: pkg},
	)
	nm, _ := m.selectPackage(pkg)
	m = nm.(Model)

	// Package focus by default: pane 3 shows importers, symbolFocus is zero.
	if m.detail {
		t.Fatal("selecting a package must start in package focus")
	}
	if m.symbolFocus() != (graph.Symbol{}) {
		t.Fatal("symbolFocus must be zero in package focus")
	}

	// Enter on the func row → detail mode.
	m.active = paneAPI
	m.apiIdx = firstSelectable(m.apiRows)
	nm, _ = m.handleEnter()
	m = nm.(Model)
	if !m.detail {
		t.Fatal("enter on a symbol should enable detail mode")
	}
	if m.symbolFocus().Name != "Index" {
		t.Errorf("symbolFocus = %q, want Index", m.symbolFocus().Name)
	}

	// Esc → back to package focus.
	nm, _ = m.updateNormal(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.detail {
		t.Error("esc should clear detail mode")
	}
}

func TestCopyPath_SetsFlashAndCommand(t *testing.T) {
	m := newTestModel(testGraph())
	m.active = paneRefs
	// Package focus with an expanded importer's reference-site row selected.
	m.refRows = []importerRow{
		{pkgPath: "x", expandable: true, expanded: true},
		{ref: true, depth: 1, file: "internal/loader/loader.go", line: 42},
	}
	m.refIdx = 1

	nm, cmd := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	mm := nm.(Model)
	if cmd == nil {
		t.Fatal("C should return a clipboard command")
	}
	if mm.flash != "copied internal/loader/loader.go" {
		t.Errorf("flash = %q, want copied internal/loader/loader.go", mm.flash)
	}
	// A subsequent key dismisses the flash.
	nm2, _ := mm.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if nm2.(Model).flash != "" {
		t.Errorf("flash should clear on the next key, got %q", nm2.(Model).flash)
	}
}

func TestOpenEditor_AbsolutizesPath(t *testing.T) {
	m := newTestModel(testGraph())
	m.cfg.Loader.Dir = "/repo/root"
	got := m.absPath("internal/loader/loader.go")
	if got != "/repo/root/internal/loader/loader.go" {
		t.Errorf("absPath = %q, want /repo/root/internal/loader/loader.go", got)
	}
	if abs := m.absPath("/already/abs.go"); abs != "/already/abs.go" {
		t.Errorf("absolute input should pass through, got %q", abs)
	}
}

func TestModuleOnlyToggle(t *testing.T) {
	m := newTestModel(testGraph())
	m.moduleOnly = false
	m = m.rebuildPicker()
	withStdlib := len(m.pickRows)

	nm, _ := m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	mm := nm.(Model)
	if !mm.moduleOnly {
		t.Fatal("m should enable moduleOnly")
	}
	if len(mm.pickRows) >= withStdlib {
		t.Errorf("module-only should reduce package rows: before=%d after=%d", withStdlib, len(mm.pickRows))
	}
}
