package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

// pkgListMsg carries the T1 package list loaded in the background.
type pkgListMsg struct {
	paths  []string
	module string
}

// structMembersMsg carries the struct members loaded for an inspected struct.
type structMembersMsg struct {
	pkgPath  string
	typeName string
	members  []inspect.StructMember
}

// allSymbolsMsg carries the global symbol index loaded in the background.
type allSymbolsMsg []symbolEntry

// treeRef is a (file, line) pointer for a rendered tree child row.
// Zero value means the row is a group header with no direct reference.
type treeRef struct {
	file string
	line int
}

type editorDoneMsg struct{ err error }

// GroupMode controls how reference edges are grouped in the tree pane.
type GroupMode int

const (
	GroupPkg  GroupMode = iota // group by caller package path
	GroupFile                  // group by file path
	GroupFunc                  // group by caller function (pkg.Func or pkg.(*Recv).Method)
)

type pane int

const (
	paneList pane = iota
	paneTree
)

type viewMode int

const (
	viewSymbols viewMode = iota
	viewStruct
)

const detailPanelHeight = 9

// symbolEntry is one row in the symbol list pane.
type symbolEntry struct {
	sym      graph.Symbol
	refCount int
}

// loadDoneMsg is sent when ResolveTarget completes in the background.
type loadDoneMsg struct {
	result *inspect.InspectResult
	err    error
}

// Model is the Bubble Tea model for the inspect TUI.
// Navigation is modal: normal mode uses vim keys; "/" enters search/insert mode.
type Model struct {
	cfg    inspect.Config
	result *inspect.InspectResult

	input     textinput.Model
	spinner   spinner.Model
	inputMode bool // true = search/insert mode, false = normal mode

	symbols []symbolEntry
	listIdx int

	treeLines []string
	treeRefs  []treeRef // parallel to treeLines: file+line for reference child rows
	treeIdx   int

	group      GroupMode
	violOnly   bool
	showDetail bool // bottom detail panel toggled by i

	active        pane
	width, height int

	allPkgs       []string
	allSymbols    []symbolEntry // accumulated across all loaded targets
	searchResults []searchResult
	searchSel     int

	modulePrefix string // main module path — stripped from displayed package paths
	moduleOnly   bool   // search shows only packages/symbols inside the main module

	view          viewMode
	structMembers []inspect.StructMember
	structPkg     string
	structName    string
	structIdx     int

	keys    keyMap
	help    help.Model
	listVP  viewport.Model
	treeVP  viewport.Model

	loading bool
	err     error
}

// New creates a Model with an optional pre-filled search target.
func New(initialTarget string, cfg inspect.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "/ to search  (e.g. tasks  or  github.com/acme/tasks)"
	ti.SetValue(initialTarget)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	hasTarget := strings.TrimSpace(initialTarget) != ""
	h := help.New()
	h.ShowAll = false
	m := Model{
		cfg:       cfg,
		input:     ti,
		spinner:   sp,
		active:    paneList,
		loading:   hasTarget,
		inputMode: hasTarget,
		keys:      defaultKeys(),
		help:      h,
		listVP:    viewport.New(0, 0),
		treeVP:    viewport.New(0, 0),
	}
	if hasTarget {
		m.input.Focus()
	}
	return m
}

// Run starts the full-screen Bubble Tea TUI. Blocks until the user quits.
func Run(initialTarget string, cfg inspect.Config) error {
	m := New(initialTarget, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, loadPkgList(m.cfg), loadAllSymbols(m.cfg)}
	if strings.TrimSpace(m.input.Value()) != "" {
		cmds = append(cmds, doLoad(m.input.Value(), m.cfg), m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func loadPkgList(cfg inspect.Config) tea.Cmd {
	return func() tea.Msg {
		paths, module, _ := inspect.ListPackages(cfg)
		return pkgListMsg{paths: paths, module: module}
	}
}

func loadStructMembers(cfg inspect.Config, pkgPath, typeName string) tea.Cmd {
	return func() tea.Msg {
		ms, _ := inspect.LoadStructMembers(cfg, pkgPath, typeName)
		return structMembersMsg{pkgPath: pkgPath, typeName: typeName, members: ms}
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

func doLoad(target string, cfg inspect.Config) tea.Cmd {
	return func() tea.Msg {
		res, err := inspect.ResolveTarget(target, cfg)
		return loadDoneMsg{result: res, err: err}
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

	case pkgListMsg:
		m.allPkgs = msg.paths
		if m.modulePrefix == "" {
			m.modulePrefix = msg.module
		}
		if m.inputMode {
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
		}
		return m, nil

	case structMembersMsg:
		if m.view == viewStruct && m.structPkg == msg.pkgPath && m.structName == msg.typeName {
			m.structMembers = msg.members
			m.structIdx = 0
			m = updateMemberTree(m)
		}
		return m, nil

	case allSymbolsMsg:
		m.allSymbols = []symbolEntry(msg)
		if m.inputMode {
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
		}
		return m, nil

	case loadDoneMsg:
		m.loading = false
		m.inputMode = false
		m.input.Blur()
		m.searchResults = nil
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.result = msg.result
		m = buildSymbolList(m)
		m = updateTree(m)
		m.active = paneList
		m.view = viewSymbols
		m.structMembers = nil
		// Auto-open struct/interface member view if the load resolved to a
		// single type symbol (typical when user picks pkg.StructName from search).
		if len(m.symbols) == 1 && m.symbols[0].sym.Kind == "type" {
			sel := m.symbols[0].sym
			m.view = viewStruct
			m.structPkg = sel.Package
			m.structName = sel.Name
			m.structIdx = 0
			return m, loadStructMembers(m.cfg, sel.Package, sel.Name)
		}
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		// Global keys always handled first.
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		}

		// Search/insert mode.
		if m.inputMode {
			switch msg.String() {
			case "esc":
				m.inputMode = false
				m.input.Blur()
				m.searchResults = nil
				return m, nil
			case "enter":
				var q string
				if len(m.searchResults) > 0 {
					sel := m.searchResults[m.searchSel]
					if sel.sym != "" {
						q = sel.pkg + "." + sel.sym
					} else {
						q = sel.pkg
					}
					m.input.SetValue(q)
				} else {
					q = strings.TrimSpace(m.input.Value())
				}
				if q != "" {
					m.loading = true
					m.err = nil
					m.inputMode = false
					m.input.Blur()
					m.searchResults = nil
					return m, tea.Batch(doLoad(q, m.cfg), m.spinner.Tick)
				}
				m.inputMode = false
				m.input.Blur()
				return m, nil
			case "j", "down":
				if m.searchSel < len(m.searchResults)-1 {
					m.searchSel++
				}
				return m, nil
			case "k", "up":
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

		// Normal mode: vim-style commands.
		switch msg.String() {
		case "q":
			return m, tea.Quit

		case "esc":
			if m.view == viewStruct {
				m.view = viewSymbols
				m.structMembers = nil
				m.structPkg = ""
				m.structName = ""
				m.structIdx = 0
				m = updateTree(m)
				return m, nil
			}
			return m, nil

		case "enter":
			if m.view == viewSymbols && m.active == paneList && m.listIdx < len(m.symbols) {
				sel := m.symbols[m.listIdx].sym
				if sel.Kind == "type" {
					m.view = viewStruct
					m.structPkg = sel.Package
					m.structName = sel.Name
					m.structMembers = nil
					m.structIdx = 0
					return m, loadStructMembers(m.cfg, sel.Package, sel.Name)
				}
			}
			return m, nil

		case "/":
			m.inputMode = true
			m.input.Focus()
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
			m.searchSel = 0
			return m, nil

		case "i":
			m.showDetail = !m.showDetail
			return m, nil

		case "?":
			m.help.ShowAll = !m.help.ShowAll
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
			args := []string{file}
			if line > 0 {
				args = []string{fmt.Sprintf("+%d", line), file}
			}
			return m, tea.ExecProcess(exec.Command(editor, args...), func(err error) tea.Msg {
				return editorDoneMsg{err}
			})

		case "tab", "l":
			if m.active == paneList {
				m.active = paneTree
			} else {
				m.active = paneList
			}
			return m, nil

		case "shift+tab", "h":
			if m.active == paneTree {
				m.active = paneList
			} else {
				m.active = paneTree
			}
			return m, nil

		case "g":
			if m.result != nil {
				switch m.group {
				case GroupPkg:
					m.group = GroupFile
				case GroupFile:
					m.group = GroupFunc
				default:
					m.group = GroupPkg
				}
				if m.view == viewStruct {
					m = updateMemberTree(m)
				} else {
					m = updateTree(m)
				}
			}
			return m, nil

		case "f":
			if m.result != nil {
				m.violOnly = !m.violOnly
				if m.view == viewStruct {
					m = updateMemberTree(m)
				} else {
					m = updateTree(m)
				}
			}
			return m, nil

		case "m":
			m.moduleOnly = !m.moduleOnly
			if m.inputMode {
				m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.allSymbols, m.modulePrefix, m.moduleOnly)
			}
			return m, nil

		case "j", "down":
			switch m.active {
			case paneList:
				if m.view == viewStruct {
					if m.structIdx < len(m.structMembers) {
						m.structIdx++
						m = updateMemberTree(m)
					}
				} else if m.listIdx < len(m.symbols)-1 {
					m.listIdx++
					m = updateTree(m)
				}
			case paneTree:
				if m.treeIdx < len(m.treeLines)-1 {
					m.treeIdx++
				}
			}
			return m, nil

		case "k", "up":
			switch m.active {
			case paneList:
				if m.view == viewStruct {
					if m.structIdx > 0 {
						m.structIdx--
						m = updateMemberTree(m)
					}
				} else if m.listIdx > 0 {
					m.listIdx--
					m = updateTree(m)
				}
			case paneTree:
				if m.treeIdx > 0 {
					m.treeIdx--
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func buildSymbolList(m Model) Model {
	if m.result == nil {
		m.symbols = nil
		return m
	}
	counts := make(map[int]int, len(m.result.Edges))
	for _, e := range m.result.Edges {
		counts[e.Callee]++
	}
	m.symbols = make([]symbolEntry, len(m.result.Symbols))
	for i, s := range m.result.Symbols {
		m.symbols[i] = symbolEntry{sym: s, refCount: counts[s.ID]}
	}
	return m
}


func updateMemberTree(m Model) Model {
	violPkgs := map[string]bool{}
	if m.result != nil {
		for _, v := range m.result.Violations {
			violPkgs[v.FromPkg] = true
		}
	}
	// structIdx 0 = whole struct; 1..N = members[i-1]
	var edges []graph.Edge
	if m.structIdx == 0 && m.result != nil && len(m.symbols) > 0 {
		edges = m.result.Edges
		m.treeLines, m.treeRefs = buildTreeLines(edges, m.symbols[0].sym.ID, m.group, m.violOnly, violPkgs, m.shortPkg)
	} else {
		idx := m.structIdx - 1
		if idx < 0 || idx >= len(m.structMembers) {
			m.treeLines = nil
			m.treeRefs = nil
			return m
		}
		edges = m.structMembers[idx].Edges
		m.treeLines, m.treeRefs = buildTreeLines(edges, 0, m.group, m.violOnly, violPkgs, m.shortPkg)
	}
	m.treeIdx = 0
	return m
}

func updateTree(m Model) Model {
	if m.result == nil || len(m.symbols) == 0 {
		m.treeLines = nil
		return m
	}
	sel := m.symbols[m.listIdx]
	violPkgs := make(map[string]bool, len(m.result.Violations))
	for _, v := range m.result.Violations {
		violPkgs[v.FromPkg] = true
	}
	m.treeLines, m.treeRefs = buildTreeLines(m.result.Edges, sel.sym.ID, m.group, m.violOnly, violPkgs, m.shortPkg)
	m.treeIdx = 0
	return m
}

func (m Model) View() string {
	if m.err != nil {
		return styleError.Render(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
	}

	searchBar := m.renderSearch()

	if m.loading {
		loadLine := fmt.Sprintf("  %s Loading %s...", m.spinner.View(), strings.TrimSpace(m.input.Value()))
		return lipgloss.JoinVertical(lipgloss.Left, searchBar, "", loadLine, "", styleHelp.Render("[ctrl+q] quit"))
	}

	helpView := m.help.View(m.keys)
	helpH := lipgloss.Height(helpView)

	dropdownH := 0
	if m.inputMode && len(m.searchResults) > 0 {
		dropdownH = min(len(m.searchResults), 10)
	}
	contentH := m.height - 1 - helpH - dropdownH
	if contentH < 2 {
		contentH = 2
	}

	leftW := m.width / 3
	if leftW < 20 {
		leftW = 20
	}
	rightW := m.width - leftW
	if rightW < 8 {
		rightW = 8
	}

	left := m.renderList(leftW, contentH)
	right := m.renderTree(rightW, contentH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if m.showDetail {
		body = overlayBottom(body, m.renderDetailPanel())
	}

	parts := []string{searchBar}
	if dropdownH > 0 {
		parts = append(parts, m.renderSearchDropdown(dropdownH))
	}
	parts = append(parts, body, helpView)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderSearchDropdown(h int) string {
	visible := m.searchResults
	if len(visible) > h {
		visible = visible[:h]
	}
	w := m.width
	lines := make([]string, 0, len(visible))
	for i, r := range visible {
		selected := i == m.searchSel
		shortP := m.shortPkg(r.pkg)
		if r.sym != "" {
			// Symbol result: "SymName (kind)  pkg/path"
			if selected {
				plain := r.sym + " (" + r.kind + ")  " + shortP
				lines = append(lines, styleSelected.Width(w).Render("▶ "+truncate(plain, w-4)))
			} else {
				label := styleActive.Render(r.sym) + styleDim.Render(" ("+r.kind+")  "+shortP)
				lines = append(lines, "  "+label)
			}
		} else {
			// Package result: "prefix/lastSeg" — last segment highlighted, prefix dimmed
			if selected {
				lines = append(lines, styleSelected.Width(w).Render("▶ "+truncate(shortP, w-4)))
			} else {
				var label string
				if idx := strings.LastIndex(shortP, "/"); idx >= 0 {
					label = styleDim.Render(shortP[:idx+1]) + shortP[idx+1:]
				} else {
					label = shortP
				}
				lines = append(lines, "  "+label)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSearch() string {
	prefix := "  "
	if m.inputMode {
		prefix = styleActive.Render("/") + " "
		return prefix + m.input.View()
	}
	if m.result != nil {
		return prefix + styleDim.Render(m.result.Target) + styleHelp.Render("  / to search")
	}
	return prefix + styleHelp.Render("Press / to search")
}

func (m Model) renderList(w, h int) string {
	if m.view == viewStruct {
		return m.renderStructList(w, h)
	}
	title := "Symbols"
	if m.active == paneList {
		title = styleActiveTitle.Render(title)
	} else {
		title = styleTitle.Render(title)
	}

	lines := make([]string, 0, len(m.symbols))
	for i, s := range m.symbols {
		pkgSeg := s.sym.Package
		if j := strings.LastIndex(pkgSeg, "/"); j >= 0 {
			pkgSeg = pkgSeg[j+1:]
		}
		label := fmt.Sprintf("%s.%s (%s) [%d]", pkgSeg, s.sym.Name, s.sym.Kind, s.refCount)
		label = truncate(label, w-1)
		switch {
		case i == m.listIdx && m.active == paneList:
			lines = append(lines, styleSelected.Width(w).Render(label))
		case i == m.listIdx:
			lines = append(lines, styleCurrent.Render(padRight(label, w)))
		default:
			lines = append(lines, styleItem.Render(padRight(label, w)))
		}
	}
	m.listVP.Width = w
	m.listVP.Height = h - 1
	m.listVP.SetContent(strings.Join(lines, "\n"))
	ensureVisible(&m.listVP, m.listIdx)
	return lipgloss.JoinVertical(lipgloss.Left, padRight(title, w), m.listVP.View())
}

func (m Model) renderStructList(w, h int) string {
	title := m.structName
	if m.active == paneList {
		title = styleActiveTitle.Render(title)
	} else {
		title = styleTitle.Render(title)
	}
	if m.structMembers == nil {
		return lipgloss.JoinVertical(lipgloss.Left, padRight(title, w), styleHelp.Render("  loading..."))
	}

	labels := make([]string, len(m.structMembers))
	for i, mem := range m.structMembers {
		kind := styleDim.Render(" (" + mem.Kind + ")")
		var body string
		switch mem.Kind {
		case "method":
			body = mem.Name + styleDim.Render(mem.Signature)
		case "field":
			body = mem.Name + styleDim.Render(":"+mem.Type)
		default:
			body = mem.Name
		}
		refN := ""
		if n := len(mem.Edges); n > 0 {
			refN = styleDim.Render(fmt.Sprintf(" [%d]", n))
		}
		labels[i] = body + kind + refN
	}

	// Root (whole-struct) row label: shows total ref count.
	rootRefs := 0
	if m.result != nil {
		rootRefs = len(m.result.Edges)
	}
	rootLabel := m.structName + styleDim.Render(fmt.Sprintf(" [%d]", rootRefs))
	active := m.active == paneList
	if m.structIdx == 0 && active {
		rootLabel = styleSelected.Render(rootLabel)
	} else if m.structIdx == 0 {
		rootLabel = lipgloss.NewStyle().Bold(true).Render(rootLabel)
	}

	// Selected member index in the tree.Child list (structIdx 1..N → 0..N-1).
	selChild := m.structIdx - 1
	t := tree.New().Root(rootLabel).Enumerator(tree.RoundedEnumerator)
	for _, lbl := range labels {
		t.Child(lbl)
	}
	t.ItemStyleFunc(func(_ tree.Children, i int) lipgloss.Style {
		if i == selChild && active {
			return lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("230"))
		}
		if i == selChild {
			return lipgloss.NewStyle().Bold(true)
		}
		return lipgloss.NewStyle()
	})

	content := t.String()
	m.listVP.Width = w
	m.listVP.Height = h - 1
	m.listVP.SetContent(content)
	ensureVisible(&m.listVP, m.structIdx)
	return lipgloss.JoinVertical(lipgloss.Left, padRight(title, w), m.listVP.View())
}

func (m Model) renderTree(w, h int) string {
	groupName := "pkg"
	switch m.group {
	case GroupFile:
		groupName = "file"
	case GroupFunc:
		groupName = "func"
	}
	titleStr := fmt.Sprintf("References [group=%s]", groupName)
	if m.active == paneTree {
		titleStr = styleActiveTitle.Render(titleStr)
	} else {
		titleStr = styleTitle.Render(titleStr)
	}

	lines := make([]string, len(m.treeLines))
	for i, line := range m.treeLines {
		line = truncate(line, w-1)
		if i == m.treeIdx && m.active == paneTree {
			lines[i] = styleSelected.Width(w).Render(line)
		} else {
			lines[i] = line
		}
	}
	m.treeVP.Width = w
	m.treeVP.Height = h - 1
	m.treeVP.SetContent(strings.Join(lines, "\n"))
	ensureVisible(&m.treeVP, m.treeIdx)
	return lipgloss.JoinVertical(lipgloss.Left, padRight(titleStr, w), m.treeVP.View())
}

func (m Model) renderDetailPanel() string {
	w := m.width
	sep := styleTitle.Render(strings.Repeat("─", w))

	if m.result == nil || len(m.symbols) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, sep, styleHelp.Render("  no symbol selected"))
	}

	sel := m.symbols[m.listIdx]
	s := sel.sym

	wrap := lipgloss.NewStyle().Width(w - 2)
	nameLine := wrap.Render(fmt.Sprintf("  %s %s",
		styleActive.Render(s.Package+"."+s.Name),
		styleDim.Render("("+s.Kind+")"),
	))
	locLine := wrap.Render(fmt.Sprintf("  %s  refs:%d",
		styleDim.Render(fmt.Sprintf("%s:%d", s.File, s.Line)),
		sel.refCount,
	))

	cols := []string{nameLine, locLine}

	if len(m.result.Violations) > 0 {
		var b strings.Builder
		b.WriteString(styleViolation.Render(fmt.Sprintf("  violations (%d): ", len(m.result.Violations))))
		for i, v := range m.result.Violations {
			if i >= 3 {
				b.WriteString(styleDim.Render(fmt.Sprintf("(+%d more)", len(m.result.Violations)-3)))
				break
			}
			b.WriteString(styleViolation.Render(v.FromPkg))
			b.WriteString(styleDim.Render(" " + v.Rule.Reason + "  "))
		}
		cols = append(cols, wrap.Render(b.String()))
	}

	return lipgloss.JoinVertical(lipgloss.Left, append([]string{sep}, cols...)...)
}

// currentRef returns the file and line for the focused element.
func (m Model) currentRef() (file string, line int) {
	switch m.active {
	case paneTree:
		if m.treeIdx < len(m.treeRefs) {
			r := m.treeRefs[m.treeIdx]
			return r.file, r.line
		}
	case paneList:
		if m.view == viewStruct {
			if m.structIdx == 0 {
				if len(m.symbols) > 0 {
					s := m.symbols[0].sym
					return s.File, s.Line
				}
				return "", 0
			}
			idx := m.structIdx - 1
			if idx >= 0 && idx < len(m.structMembers) {
				mem := m.structMembers[idx]
				return mem.File, mem.Line
			}
			return "", 0
		}
		if m.listIdx < len(m.symbols) {
			s := m.symbols[m.listIdx].sym
			return s.File, s.Line
		}
	}
	return "", 0
}

// shortPkg strips the main module prefix from a package path so only the
// intra-module portion is shown (e.g. "go.flaticols.dev/gorefactor/internal/graph"
// → "internal/graph"). Packages outside the module are returned unchanged.
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

// ensureVisible scrolls vp so that line `idx` (zero-based in the content) is within view.
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

// overlayBottom renders panel on top of the last lines of base.
func overlayBottom(base, panel string) string {
	bLines := strings.Split(base, "\n")
	pLines := strings.Split(panel, "\n")
	if len(pLines) >= len(bLines) {
		return panel
	}
	out := make([]string, len(bLines))
	copy(out, bLines)
	copy(out[len(bLines)-len(pLines):], pLines)
	return strings.Join(out, "\n")
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
