package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

// pkgListMsg carries the T1 package list loaded in the background.
type pkgListMsg []string

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
	searchResults []searchResult
	searchSel     int

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
	m := Model{
		cfg:       cfg,
		input:     ti,
		spinner:   sp,
		active:    paneList,
		loading:   hasTarget,
		inputMode: hasTarget,
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
	cmds := []tea.Cmd{textinput.Blink, loadPkgList(m.cfg)}
	if strings.TrimSpace(m.input.Value()) != "" {
		cmds = append(cmds, doLoad(m.input.Value(), m.cfg), m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func loadPkgList(cfg inspect.Config) tea.Cmd {
	return func() tea.Msg {
		paths, _ := inspect.ListPackages(cfg)
		return pkgListMsg(paths)
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
		return m, nil

	case pkgListMsg:
		m.allPkgs = []string(msg)
		if m.inputMode {
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.symbols)
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
				m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.symbols)
				m.searchSel = 0
				return m, cmd
			}
		}

		// Normal mode: vim-style commands.
		switch msg.String() {
		case "q":
			return m, tea.Quit

		case "/":
			m.inputMode = true
			m.input.Focus()
			m.searchResults = filterResults(m.input.Value(), m.allPkgs, m.symbols)
			m.searchSel = 0
			return m, nil

		case "i":
			m.showDetail = !m.showDetail
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
				m = updateTree(m)
			}
			return m, nil

		case "f":
			if m.result != nil {
				m.violOnly = !m.violOnly
				m = updateTree(m)
			}
			return m, nil

		case "j", "down":
			switch m.active {
			case paneList:
				if m.listIdx < len(m.symbols)-1 {
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
				if m.listIdx > 0 {
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
	m.treeLines, m.treeRefs = buildTreeLines(m.result.Edges, sel.sym.ID, m.group, m.violOnly, violPkgs)
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

	help := m.renderHelpBar()

	// Reserve rows: 1 search + 1 help + dropdown (when in search mode).
	// The detail panel is overlaid on the body, not stacked.
	dropdownH := 0
	if m.inputMode && len(m.searchResults) > 0 {
		dropdownH = min(len(m.searchResults), 10)
	}
	contentH := m.height - 2 - dropdownH
	if contentH < 2 {
		contentH = 2
	}

	leftW := m.width / 3
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
	parts = append(parts, body, help)
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
		if r.sym != "" {
			// Symbol result: "SymName (kind)  pkg/path"
			if selected {
				plain := r.sym + " (" + r.kind + ")  " + r.pkg
				lines = append(lines, styleSelected.Width(w).Render("▶ "+truncate(plain, w-4)))
			} else {
				label := styleActive.Render(r.sym) + styleDim.Render(" ("+r.kind+")  "+r.pkg)
				lines = append(lines, "  "+label)
			}
		} else {
			// Package result: "prefix/lastSeg" — last segment highlighted, prefix dimmed
			if selected {
				lines = append(lines, styleSelected.Width(w).Render("▶ "+truncate(r.pkg, w-4)))
			} else {
				var label string
				if idx := strings.LastIndex(r.pkg, "/"); idx >= 0 {
					label = styleDim.Render(r.pkg[:idx+1]) + r.pkg[idx+1:]
				} else {
					label = r.pkg
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
	title := "Symbols"
	if m.active == paneList {
		title = styleActiveTitle.Render(title)
	} else {
		title = styleTitle.Render(title)
	}

	lines := make([]string, 0, h)
	lines = append(lines, padRight(title, w))

	for i, s := range m.symbols {
		if len(lines) >= h {
			break
		}
		pkgSeg := s.sym.Package
		if i := strings.LastIndex(pkgSeg, "/"); i >= 0 {
			pkgSeg = pkgSeg[i+1:]
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

	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
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

	lines := make([]string, 0, h)
	lines = append(lines, padRight(titleStr, w))

	visible := m.treeLines
	if m.treeIdx > 0 && m.treeIdx < len(visible) {
		visible = visible[m.treeIdx:]
	}

	for _, line := range visible {
		if len(lines) >= h {
			break
		}
		lines = append(lines, truncate(line, w-1))
	}

	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderDetailPanel() string {
	w := m.width
	sep := styleTitle.Render(strings.Repeat("─", w))

	if m.result == nil || len(m.symbols) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, sep, styleHelp.Render("  no symbol selected"))
	}

	sel := m.symbols[m.listIdx]
	s := sel.sym

	cols := []string{
		fmt.Sprintf("  %s  %s  %s:%d  refs:%d",
			styleActive.Render(s.Package+"."+s.Name),
			styleDim.Render("("+s.Kind+")"),
			s.File, s.Line,
			sel.refCount,
		),
	}

	if len(m.result.Violations) > 0 {
		violLine := styleViolation.Render(fmt.Sprintf("  violations (%d): ", len(m.result.Violations)))
		for i, v := range m.result.Violations {
			if i >= 3 {
				violLine += styleDim.Render(fmt.Sprintf("(+%d more)", len(m.result.Violations)-3))
				break
			}
			violLine += styleViolation.Render(v.FromPkg) + styleDim.Render(" "+v.Rule.Reason+"  ")
		}
		cols = append(cols, violLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, append([]string{sep}, cols...)...)
}

func (m Model) renderHelpBar() string {
	if m.inputMode {
		return styleHelp.Render("[Enter] search  [Esc] cancel")
	}
	violStr := "off"
	if m.violOnly {
		violStr = "on"
	}
	groupStr := "pkg"
	switch m.group {
	case GroupFile:
		groupStr = "file"
	case GroupFunc:
		groupStr = "func"
	}
	detailStr := "off"
	if m.showDetail {
		detailStr = "on"
	}
	parts := []string{
		"[/] search",
		"[hjkl] navigate",
		"[g] group=" + groupStr,
		"[f] violations=" + violStr,
		"[i] detail=" + detailStr,
		"[e] open",
		"[q] quit",
	}
	return styleHelp.Render(strings.Join(parts, "  "))
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
		if m.listIdx < len(m.symbols) {
			s := m.symbols[m.listIdx].sym
			return s.File, s.Line
		}
	}
	return "", 0
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
