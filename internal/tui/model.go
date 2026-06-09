package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
	"go.flaticols.dev/gorefactor/internal/loader"
)

// graphMsg carries the T1 package graph loaded once at startup.
type graphMsg struct {
	pg  *loader.PackageGraph
	err error
}

// allSymbolsMsg carries the global symbol index loaded in the background.
type allSymbolsMsg []symbolEntry

// loadDoneMsg is sent when ResolveTargetWithGraph completes in the background.
// pkgPath identifies which package the result belongs to so a stale load that
// resolves after the user moved on does not clobber the active panes.
type loadDoneMsg struct {
	pkgPath string
	result  *inspect.InspectResult
	err     error
}

// structMembersMsg carries struct/interface members loaded for a type symbol.
type structMembersMsg struct {
	pkgPath  string
	typeName string
	members  []inspect.StructMember
}

type editorDoneMsg struct{ err error }

// treeRef is a (file, line) pointer for a rendered reference row.
type treeRef struct {
	file string
	line int
}

// GroupMode controls how reference edges / importers are grouped in pane 3.
type GroupMode int

const (
	GroupPkg  GroupMode = iota // group by caller package path
	GroupFile                  // group by file path
	GroupFunc                  // group by caller function
)

// pane identifies the focused column.
type pane int

const (
	panePicker pane = iota // pane 1: package picker
	paneAPI                // pane 2: public API
	paneRefs               // pane 3: importers / references
)

// symbolEntry is one row in the API symbol list (and the search index).
type symbolEntry struct {
	sym      graph.Symbol
	refCount int
}

// Model is the Bubble Tea model for the package-centric explorer.
type Model struct {
	cfg inspect.Config

	pg         *loader.PackageGraph
	allPkgs    []string                          // pg.AllPaths(), set after graph load
	cache      map[string]*inspect.InspectResult // ResolveTarget result per package path
	allSymbols []symbolEntry                     // global search index (names only, no positions)

	modulePrefix string // main module path — stripped from displayed paths
	moduleOnly   bool   // restrict picker/search to the main module

	// Picker (pane 1).
	pickMode    pickerMode
	pickRows    []pickerRow
	pickIdx     int
	pickExpand  map[string]bool // open folder/import keys
	selectedPkg string          // currently loaded package ("" = none)

	// API (pane 2).
	apiRows     []apiRow
	apiIdx      int
	apiExpand   map[string]bool                              // expanded type names
	members     map[string]map[string][]inspect.StructMember // pkgPath → typeName → members
	pendingLoad bool                                         // a ResolveTarget is in flight for selectedPkg
	detail      bool                                         // true = pane 3 shows the selected symbol's references

	// References / importers (pane 3).
	group     GroupMode
	refTree   bool            // pane-3 tree mode for importers (toggled by g in package focus)
	refExpand map[string]bool // open importer-tree keys
	refRows   []importerRow
	refLines  []string
	refRefs   []treeRef
	refIdx    int

	// Search (modal overlay over the picker).
	input         textinput.Model
	inputMode     bool
	searchResults []searchResult
	searchSel     int

	active        pane
	width, height int

	keys    keyMap
	help    help.Model
	pickVP  viewport.Model
	apiVP   viewport.Model
	refVP   viewport.Model
	spinner spinner.Model

	initialTarget string
	loadingGraph  bool
	flash         string // transient status (e.g. "copied …"), cleared on next key
	err           error
}

// New creates a Model. initialTarget, if non-empty, pre-selects a package once
// the graph finishes loading (suffix-matched against module package paths).
func New(initialTarget string, cfg inspect.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "filter packages…"

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	h := help.New()
	h.ShowAll = false

	return Model{
		cfg:           cfg,
		moduleOnly:    true, // module-only by default; 'm' includes external packages
		cache:         map[string]*inspect.InspectResult{},
		pickExpand:    map[string]bool{},
		apiExpand:     map[string]bool{},
		members:       map[string]map[string][]inspect.StructMember{},
		input:         ti,
		spinner:       sp,
		keys:          defaultKeys(),
		help:          h,
		active:        panePicker,
		pickVP:        viewport.New(0, 0),
		apiVP:         viewport.New(0, 0),
		refVP:         viewport.New(0, 0),
		initialTarget: strings.TrimSpace(initialTarget),
		loadingGraph:  true,
	}
}

// Run starts the full-screen explorer. Blocks until the user quits.
func Run(initialTarget string, cfg inspect.Config) error {
	m := New(initialTarget, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick, loadGraph(m.cfg), loadAllSymbols(m.cfg))
}

// loadGraph builds the T1 package graph once at startup.
func loadGraph(cfg inspect.Config) tea.Cmd {
	return func() tea.Msg {
		pg, err := inspect.BuildGraph(cfg)
		return graphMsg{pg: pg, err: err}
	}
}

func loadAllSymbols(cfg inspect.Config) tea.Cmd {
	return func() tea.Msg {
		syms, _ := inspect.LoadAllSymbols(cfg)
		entries := make([]symbolEntry, len(syms))
		for i, s := range syms {
			entries[i] = symbolEntry{sym: s}
		}
		return allSymbolsMsg(entries)
	}
}

// resolvePkg fires a full ResolveTarget for pkgPath using the cached graph.
func resolvePkg(cfg inspect.Config, pg *loader.PackageGraph, pkgPath string) tea.Cmd {
	return func() tea.Msg {
		res, err := inspect.ResolveTargetWithGraph(pkgPath, cfg, pg)
		return loadDoneMsg{pkgPath: pkgPath, result: res, err: err}
	}
}

func loadStructMembers(cfg inspect.Config, pg *loader.PackageGraph, pkgPath, typeName string) tea.Cmd {
	return func() tea.Msg {
		ms, _ := inspect.LoadStructMembersWithGraph(cfg, pg, pkgPath, typeName)
		return structMembersMsg{pkgPath: pkgPath, typeName: typeName, members: ms}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = m.width - 12
		m.help.Width = m.width
		return m, nil

	case graphMsg:
		m.loadingGraph = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.pg = msg.pg
		m.allPkgs = msg.pg.AllPaths()
		if m.modulePrefix == "" {
			m.modulePrefix = msg.pg.Module
		}
		m = m.rebuildPicker()
		// Auto-select an initial target if provided. Accept either an exact
		// package path or a path suffix (e.g. "internal/loader").
		if m.initialTarget != "" {
			pkgPath, _, _ := inspect.ParseTarget(m.initialTarget)
			m.initialTarget = ""
			if _, ok := m.pg.Nodes[pkgPath]; !ok {
				if r := suffixMatchPath(pkgPath, m.allPkgs); r != "" {
					pkgPath = r
				}
			}
			if _, ok := m.pg.Nodes[pkgPath]; ok {
				return m.selectPackage(pkgPath)
			}
		}
		return m, nil

	case allSymbolsMsg:
		m.allSymbols = []symbolEntry(msg)
		if m.inputMode {
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
		}
		return m, nil

	case loadDoneMsg:
		if msg.err == nil && msg.result != nil {
			m.cache[msg.pkgPath] = msg.result
		}
		// Only repaint if this load is still the active selection.
		if msg.pkgPath == m.selectedPkg {
			m.pendingLoad = false
			if msg.err != nil {
				m.err = msg.err
				return m, nil
			}
			m.err = nil
			m = m.applyResult(msg.result)
		}
		return m, nil

	case structMembersMsg:
		byType := m.members[msg.pkgPath]
		if byType == nil {
			byType = map[string][]inspect.StructMember{}
			m.members[msg.pkgPath] = byType
		}
		if msg.members == nil {
			byType[msg.typeName] = []inspect.StructMember{}
		} else {
			byType[msg.typeName] = msg.members
		}
		if msg.pkgPath == m.selectedPkg {
			m = m.rebuildAPI()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case editorDoneMsg:
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		}
		if m.inputMode {
			return m.updateSearch(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

// updateSearch handles keys while the package filter overlay is open.
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = false
		m.input.Blur()
		m.searchResults = nil
		return m, nil
	case "enter":
		if len(m.searchResults) > 0 {
			pkg := m.searchResults[m.searchSel].pkg
			m.inputMode = false
			m.input.Blur()
			m.searchResults = nil
			return m.selectPackage(pkg)
		}
		m.inputMode = false
		m.input.Blur()
		return m, nil
	case "down", "ctrl+n":
		if m.searchSel < len(m.searchResults)-1 {
			m.searchSel++
		}
		return m, nil
	case "up", "ctrl+p":
		if m.searchSel > 0 {
			m.searchSel--
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
		m.searchSel = 0
		return m, cmd
	}
}

// updateNormal handles vim-style keys when no overlay is active.
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = "" // any key dismisses a transient status; C re-sets it below
	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc":
		// Leave symbol-detail mode: pane 3 returns to importers.
		if m.detail {
			m.detail = false
			m.refIdx = 0
			m = m.rebuildRefs()
		}
		return m, nil

	case "?":
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case "/":
		m.inputMode = true
		m.input.Focus()
		m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
		m.searchSel = 0
		return m, nil

	case "t":
		switch m.pickMode {
		case pickerFlat:
			m.pickMode = pickerFolder
		case pickerFolder:
			m.pickMode = pickerImport
		default:
			m.pickMode = pickerFlat
		}
		m.pickExpand = map[string]bool{}
		m = m.rebuildPicker()
		return m, nil

	case "m":
		m.moduleOnly = !m.moduleOnly
		m.pickExpand = map[string]bool{}
		m = m.rebuildPicker()
		if m.inputMode {
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
		}
		return m, nil

	case "g":
		if m.active == paneRefs && m.symbolFocus() == (graph.Symbol{}) {
			m.refTree = !m.refTree
			m.refExpand = map[string]bool{}
			m.refIdx = 0
			m = m.rebuildRefs()
			return m, nil
		}
		switch m.group {
		case GroupPkg:
			m.group = GroupFile
		case GroupFile:
			m.group = GroupFunc
		default:
			m.group = GroupPkg
		}
		m = m.rebuildRefs()
		return m, nil

	case "e":
		file, line := m.currentRef()
		if file == "" {
			return m, nil
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		// Paths are stored relative to the working directory; resolve to an
		// absolute path so the editor opens the file regardless of its own cwd.
		abs := m.absPath(file)
		args := []string{abs}
		if line > 0 {
			args = []string{fmt.Sprintf("+%d", line), abs}
		}
		return m, tea.ExecProcess(exec.Command(editor, args...), func(err error) tea.Msg {
			return editorDoneMsg{err}
		})

	case "C":
		// Copy the working-directory-relative path of the focused file.
		file, _ := m.currentRef()
		if file == "" {
			return m, nil
		}
		m.flash = "copied " + file
		return m, copyToClipboard(file)

	case "tab", "l", "right":
		m.active = (m.active + 1) % 3
		return m, nil

	case "shift+tab", "h", "left":
		m.active = (m.active + 2) % 3
		return m, nil

	case "enter":
		return m.handleEnter()

	case "j", "down":
		m = m.moveCursor(1)
		return m, nil

	case "k", "up":
		m = m.moveCursor(-1)
		return m, nil
	}
	return m, nil
}

// handleEnter expands/selects depending on the active pane.
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.active {
	case panePicker:
		if m.pickIdx >= len(m.pickRows) {
			return m, nil
		}
		row := m.pickRows[m.pickIdx]
		if row.expandable {
			key := m.pickerKey(m.pickIdx)
			m.pickExpand[key] = !m.pickExpand[key]
			m = m.rebuildPicker()
			return m, nil
		}
		if row.pkgPath != "" {
			return m.selectPackage(row.pkgPath)
		}
		return m, nil

	case paneAPI:
		if m.apiIdx >= len(m.apiRows) {
			return m, nil
		}
		row := m.apiRows[m.apiIdx]
		if row.header {
			return m, nil
		}
		if row.member {
			// Members are leaf rows; focusing them keeps the parent type's
			// references in pane 3 (handled by symbolFocus returning zero here,
			// so importers stay shown). Nothing to expand.
			return m, nil
		}
		// Focus this symbol: pane 3 switches to its references.
		m.detail = true
		var cmd tea.Cmd
		if row.expandable {
			name := row.sym.Name
			m.apiExpand[name] = !m.apiExpand[name]
			// Lazy-load members on first expand.
			if m.apiExpand[name] {
				if _, ok := m.members[m.selectedPkg][name]; !ok {
					cmd = loadStructMembers(m.cfg, m.pg, m.selectedPkg, name)
				}
			}
		}
		m = m.rebuildAPI()
		m = m.rebuildRefs()
		return m, cmd

	case paneRefs:
		// Only the importer tree (package focus, tree mode) is expandable.
		if m.symbolFocus() != (graph.Symbol{}) {
			return m, nil
		}
		if m.refIdx < len(m.refRows) && m.refRows[m.refIdx].expandable {
			m.toggleRefRow(m.refIdx)
			m = m.rebuildRefs()
		}
		return m, nil
	}
	return m, nil
}

// moveCursor advances the active pane's selection by delta.
func (m Model) moveCursor(delta int) Model {
	switch m.active {
	case panePicker:
		m.pickIdx = clamp(m.pickIdx+delta, 0, len(m.pickRows)-1)
	case paneAPI:
		prev := m.apiIdx
		m.apiIdx = nextSelectable(m.apiRows, m.apiIdx, delta)
		// In detail mode pane 3 tracks the focused symbol; refresh on move.
		if m.detail && m.apiIdx != prev {
			m.refIdx = 0
			m = m.rebuildRefs()
		}
	case paneRefs:
		if m.symbolFocus() != (graph.Symbol{}) {
			m.refIdx = clamp(m.refIdx+delta, 0, len(m.refLines)-1)
		} else {
			m.refIdx = clamp(m.refIdx+delta, 0, len(m.refRows)-1)
		}
	}
	return m
}

// selectPackage loads (or shows from cache) the given package into panes 2/3.
func (m Model) selectPackage(pkgPath string) (tea.Model, tea.Cmd) {
	m.selectedPkg = pkgPath
	m.err = nil
	m.apiIdx = 0
	m.apiExpand = map[string]bool{}
	m.refIdx = 0
	m.refTree = false
	m.refExpand = map[string]bool{}
	m.detail = false
	m.active = paneAPI

	if res, ok := m.cache[pkgPath]; ok {
		m.pendingLoad = false
		m = m.applyResult(res)
		return m, nil
	}
	// Seed names instantly from the search index (no positions yet).
	m = m.seedAPIFromIndex(pkgPath)
	m = m.rebuildRefs()
	m.pendingLoad = true
	return m, resolvePkg(m.cfg, m.pg, pkgPath)
}

// seedAPIFromIndex populates pane 2 with exported symbol names from the global
// index (no File/Line) for instant feedback before ResolveTarget returns.
func (m Model) seedAPIFromIndex(pkgPath string) Model {
	var syms []symbolEntry
	for _, se := range m.allSymbols {
		if se.sym.Package == pkgPath {
			syms = append(syms, se)
		}
	}
	m.apiRows = buildAPIRows(syms, m.apiExpand, m.members[pkgPath])
	m.apiIdx = firstSelectable(m.apiRows)
	return m
}

// applyResult installs a resolved InspectResult and rebuilds panes 2/3.
func (m Model) applyResult(res *inspect.InspectResult) Model {
	if res == nil {
		return m
	}
	m = m.rebuildAPI()
	m = m.rebuildRefs()
	return m
}

// rebuildPicker recomputes pane-1 rows from the current mode and expand state.
func (m Model) rebuildPicker() Model {
	pkgs := modulePkgs(m.allPkgs, m.modulePrefix, m.moduleOnly)
	switch m.pickMode {
	case pickerFolder:
		m.pickRows = buildFolderRows(pkgs, m.shortPkg, m.pickExpand)
	case pickerImport:
		m.pickRows = buildImportRows(pkgs, m.pg, m.modulePrefix, m.moduleOnly, m.shortPkg, m.pickExpand)
	default:
		m.pickRows = buildFlatRows(pkgs, m.shortPkg)
	}
	m.pickIdx = clamp(m.pickIdx, 0, len(m.pickRows)-1)
	return m
}

// rebuildAPI recomputes pane-2 rows for the selected package.
func (m Model) rebuildAPI() Model {
	res := m.cache[m.selectedPkg]
	if res == nil {
		// keep whatever was seeded from the index
		m.apiIdx = clamp(m.apiIdx, 0, len(m.apiRows)-1)
		return m
	}
	counts := make(map[int]int, len(res.Edges))
	for _, e := range res.Edges {
		counts[e.Callee]++
	}
	syms := make([]symbolEntry, len(res.Symbols))
	for i, s := range res.Symbols {
		syms[i] = symbolEntry{sym: s, refCount: counts[s.ID]}
	}
	m.apiRows = buildAPIRows(syms, m.apiExpand, m.members[m.selectedPkg])
	m.apiIdx = clamp(m.apiIdx, 0, len(m.apiRows)-1)
	// Don't leave the cursor on a header row.
	if m.apiIdx < len(m.apiRows) && m.apiRows[m.apiIdx].header {
		m.apiIdx = nextSelectable(m.apiRows, m.apiIdx, 1)
	}
	return m
}

// rebuildRefs recomputes pane-3 contents based on focus (symbol vs package).
func (m Model) rebuildRefs() Model {
	res := m.cache[m.selectedPkg]
	sym := m.symbolFocus()
	if sym != (graph.Symbol{}) && res != nil {
		// Symbol focus: reference sites grouped by pkg/file/func.
		m.refLines, m.refRefs = buildTreeLines(res.Edges, sym.ID, m.group, m.shortPkg)
		m.refRows = nil
		m.refIdx = clamp(m.refIdx, 0, len(m.refLines)-1)
		return m
	}
	// Package focus: importers list/tree.
	var edges []graph.Edge
	var syms []graph.Symbol
	if res != nil {
		edges = res.Edges
		syms = res.Symbols
	}
	if m.refExpand == nil {
		m.refExpand = map[string]bool{}
	}
	if m.refTree {
		m.refRows = buildImporterRowsTree(m.selectedPkg, m.pg, edges, m.shortPkg, m.refExpand)
	} else {
		m.refRows = buildImporterRowsFlat(m.selectedPkg, m.pg, edges, syms, m.shortPkg, m.refExpand)
	}
	m.refLines = nil
	m.refRefs = nil
	m.refIdx = clamp(m.refIdx, 0, len(m.refRows)-1)
	return m
}

// symbolFocus returns the symbol currently selected in pane 2 (for the detailed
// reference view), or the zero Symbol when the API cursor is on a header, a
// member, or nothing.
// symbolFocus returns the symbol whose references pane 3 should show. This is
// the symbol selected in pane 2, but only while detail mode is on; otherwise
// pane 3 stays in package focus (importers). Returns the zero Symbol when the
// API cursor is on a header or member row.
func (m Model) symbolFocus() graph.Symbol {
	if !m.detail {
		return graph.Symbol{}
	}
	if m.apiIdx < 0 || m.apiIdx >= len(m.apiRows) {
		return graph.Symbol{}
	}
	row := m.apiRows[m.apiIdx]
	if row.header || row.member {
		return graph.Symbol{}
	}
	return row.sym
}

// pickerKey returns a stable expand key for the picker row at idx based on its
// visible ancestor chain.
func (m Model) pickerKey(idx int) string {
	if idx < 0 || idx >= len(m.pickRows) {
		return ""
	}
	row := m.pickRows[idx]
	switch m.pickMode {
	case pickerFolder:
		// Reconstruct the slash path from the visible ancestor chain.
		segs := []string{row.label}
		depth := row.depth
		for i := idx - 1; i >= 0 && depth > 0; i-- {
			if m.pickRows[i].depth == depth-1 {
				segs = append([]string{m.pickRows[i].label}, segs...)
				depth--
			}
		}
		return strings.Join(segs, "/")
	case pickerImport:
		// Reconstruct the \x00-joined path chain.
		parts := []string{row.pkgPath}
		depth := row.depth
		for i := idx - 1; i >= 0 && depth > 0; i-- {
			if m.pickRows[i].depth == depth-1 {
				parts = append([]string{m.pickRows[i].pkgPath}, parts...)
				depth--
			}
		}
		return strings.Join(parts, "\x00")
	}
	return row.pkgPath
}

// toggleRefRow flips the expand state for the importer-tree row at idx.
func (m *Model) toggleRefRow(idx int) {
	if m.refExpand == nil {
		m.refExpand = map[string]bool{}
	}
	if idx < 0 || idx >= len(m.refRows) {
		return
	}
	row := m.refRows[idx]
	parts := []string{row.pkgPath}
	depth := row.depth
	for i := idx - 1; i >= 0 && depth > 0; i-- {
		if m.refRows[i].depth == depth-1 {
			parts = append([]string{m.refRows[i].pkgPath}, parts...)
			depth--
		}
	}
	key := strings.Join(parts, "\x00")
	m.refExpand[key] = !m.refExpand[key]
}

// absPath resolves a working-directory-relative file path to an absolute path so
// external tools (the editor) can open it regardless of their own cwd.
func (m Model) absPath(file string) string {
	if file == "" || filepath.IsAbs(file) {
		return file
	}
	base := m.cfg.Loader.Dir
	if base == "" {
		base = "."
	}
	if abs, err := filepath.Abs(base); err == nil {
		return filepath.Join(abs, file)
	}
	return filepath.Join(base, file)
}

// copyToClipboard pipes text to the platform clipboard utility. Failures are
// silent — the flash already optimistically reports success.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		var name string
		var args []string
		switch runtime.GOOS {
		case "darwin":
			name = "pbcopy"
		case "windows":
			name = "clip"
		default:
			if _, err := exec.LookPath("wl-copy"); err == nil {
				name = "wl-copy"
			} else {
				name, args = "xclip", []string{"-selection", "clipboard"}
			}
		}
		c := exec.Command(name, args...)
		c.Stdin = strings.NewReader(text)
		_ = c.Run()
		return nil
	}
}

// currentRef returns the file and line for the focused element.
func (m Model) currentRef() (file string, line int) {
	switch m.active {
	case paneAPI:
		if m.apiIdx < len(m.apiRows) {
			return m.apiRows[m.apiIdx].file, m.apiRows[m.apiIdx].line
		}
	case paneRefs:
		if m.symbolFocus() != (graph.Symbol{}) {
			if m.refIdx < len(m.refRefs) {
				return m.refRefs[m.refIdx].file, m.refRefs[m.refIdx].line
			}
			return "", 0
		}
		// Package focus: an expanded importer's reference-site row.
		if m.refIdx < len(m.refRows) && m.refRows[m.refIdx].ref {
			return m.refRows[m.refIdx].file, m.refRows[m.refIdx].line
		}
	}
	return "", 0
}

// shortPkg strips the main module prefix from a package path.
func (m Model) shortPkg(p string) string {
	if m.modulePrefix == "" || p == "" {
		return p
	}
	if p == m.modulePrefix {
		return "."
	}
	if strings.HasPrefix(p, m.modulePrefix+"/") {
		return p[len(m.modulePrefix)+1:]
	}
	return p
}

// nextSelectable returns the next API-row index in the given direction (delta
// +1/-1) that is not a kind header, staying put if there is no such row ahead.
func nextSelectable(rows []apiRow, idx, delta int) int {
	if len(rows) == 0 {
		return 0
	}
	for next := idx + delta; next >= 0 && next < len(rows); next += delta {
		if !rows[next].header {
			return next
		}
	}
	return idx
}

// firstSelectable returns the first non-header API row index, or 0.
func firstSelectable(rows []apiRow) int {
	for i, r := range rows {
		if !r.header {
			return i
		}
	}
	return 0
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// suffixMatchPath returns the first path ending with "/"+suffix or equal to suffix.
func suffixMatchPath(suffix string, paths []string) string {
	for _, p := range paths {
		if p == suffix || strings.HasSuffix(p, "/"+suffix) {
			return p
		}
	}
	return ""
}

func ensureVisible(vp *viewport.Model, idx int) {
	if vp.Height <= 0 {
		return
	}
	if idx < vp.YOffset {
		vp.YOffset = idx
	} else if idx >= vp.YOffset+vp.Height {
		vp.YOffset = idx - vp.Height + 1
	}
	if vp.YOffset < 0 {
		vp.YOffset = 0
	}
}

func truncate(s string, max int) string {
	if max <= 3 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}
