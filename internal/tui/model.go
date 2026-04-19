package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go.flaticols.dev/gorefactor/internal/graph"
	"go.flaticols.dev/gorefactor/internal/inspect"
)

// GroupMode controls how reference edges are grouped in the tree pane.
type GroupMode int

const (
	GroupPkg  GroupMode = iota // group by caller package path
	GroupFile                  // group by file path
)

type pane int

const (
	paneSearch pane = iota
	paneList
	paneTree
)

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
type Model struct {
	cfg    inspect.Config
	result *inspect.InspectResult

	input   textinput.Model
	spinner spinner.Model

	symbols []symbolEntry
	listIdx int

	treeLines []string
	treeIdx   int

	group    GroupMode
	violOnly bool

	active        pane
	width, height int

	loading bool
	err     error
}

// New creates a Model with an optional pre-filled search target.
func New(initialTarget string, cfg inspect.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "package/symbol (e.g. github.com/acme/tasks)"
	ti.SetValue(initialTarget)
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		cfg:     cfg,
		input:   ti,
		spinner: sp,
		active:  paneSearch,
		loading: strings.TrimSpace(initialTarget) != "",
	}
}

// Run starts the full-screen Bubble Tea TUI. Blocks until the user quits.
func Run(initialTarget string, cfg inspect.Config) error {
	m := New(initialTarget, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if strings.TrimSpace(m.input.Value()) != "" {
		cmds = append(cmds, doLoad(m.input.Value(), m.cfg), m.spinner.Tick)
	}
	return tea.Batch(cmds...)
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

	case loadDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.result = msg.result
		m = buildSymbolList(m)
		m = updateTree(m)
		if m.active == paneSearch && len(m.symbols) > 0 {
			m.active = paneList
			m.input.Blur()
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
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit

		case "q":
			if m.active == paneSearch {
				return m, tea.Quit
			}
			m.active = paneSearch
			m.input.Focus()
			return m, nil

		case "tab":
			m = cyclePane(m)
			return m, nil

		case "esc":
			if m.active != paneSearch {
				m.active = paneSearch
				m.input.Focus()
			}
			return m, nil

		case "g":
			if m.active != paneSearch && m.result != nil {
				if m.group == GroupPkg {
					m.group = GroupFile
				} else {
					m.group = GroupPkg
				}
				m = updateTree(m)
			}
			return m, nil

		case "f":
			if m.active != paneSearch && m.result != nil {
				m.violOnly = !m.violOnly
				m = updateTree(m)
			}
			return m, nil

		case "enter":
			if m.active == paneSearch {
				q := strings.TrimSpace(m.input.Value())
				if q != "" {
					m.loading = true
					m.err = nil
					return m, tea.Batch(doLoad(q, m.cfg), m.spinner.Tick)
				}
			}
			return m, nil
		}

		// Per-pane navigation
		switch m.active {
		case paneSearch:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case paneList:
			switch msg.String() {
			case "up", "k":
				if m.listIdx > 0 {
					m.listIdx--
					m = updateTree(m)
				}
			case "down", "j":
				if m.listIdx < len(m.symbols)-1 {
					m.listIdx++
					m = updateTree(m)
				}
			}
		case paneTree:
			switch msg.String() {
			case "up", "k":
				if m.treeIdx > 0 {
					m.treeIdx--
				}
			case "down", "j":
				if m.treeIdx < len(m.treeLines)-1 {
					m.treeIdx++
				}
			}
		}
	}
	return m, nil
}

func cyclePane(m Model) Model {
	switch m.active {
	case paneSearch:
		if len(m.symbols) > 0 {
			m.active = paneList
			m.input.Blur()
		}
	case paneList:
		m.active = paneTree
	case paneTree:
		m.active = paneSearch
		m.input.Focus()
	}
	return m
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
	m.treeLines = buildTreeLines(m.result.Edges, sel.sym.ID, m.group, m.violOnly, violPkgs)
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

	contentH := m.height - 3
	if contentH < 2 {
		contentH = 2
	}

	leftW := m.width / 3
	midW := m.width / 3
	rightW := m.width - leftW - midW
	if rightW < 8 {
		rightW = 8
	}

	left := m.renderList(leftW, contentH)
	mid := m.renderTree(midW, contentH)
	right := m.renderDetail(rightW, contentH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	help := m.renderHelpBar()

	return lipgloss.JoinVertical(lipgloss.Left, searchBar, body, help)
}

func (m Model) renderSearch() string {
	label := "Search"
	if m.active == paneSearch {
		label = styleActive.Render(label)
	}
	return label + ": " + m.input.View()
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
		label := fmt.Sprintf("%s (%s) [%d]", s.sym.Name, s.sym.Kind, s.refCount)
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
	if m.group == GroupFile {
		groupName = "file"
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

func (m Model) renderDetail(w, h int) string {
	titleStr := styleTitle.Render("Detail")
	lines := make([]string, 0, h)
	lines = append(lines, padRight(titleStr, w))

	if m.result != nil && len(m.symbols) > 0 {
		sel := m.symbols[m.listIdx]
		s := sel.sym
		lines = append(lines,
			truncate(fmt.Sprintf("name:  %s", s.Name), w-1),
			truncate(fmt.Sprintf("kind:  %s", s.Kind), w-1),
			truncate(fmt.Sprintf("pkg:   %s", s.Package), w-1),
			truncate(fmt.Sprintf("file:  %s:%d", s.File, s.Line), w-1),
			truncate(fmt.Sprintf("refs:  %d", sel.refCount), w-1),
			"",
		)
		if len(m.result.Violations) > 0 {
			lines = append(lines, styleViolation.Render(fmt.Sprintf("violations (%d):", len(m.result.Violations))))
			for _, v := range m.result.Violations {
				if len(lines) >= h-1 {
					break
				}
				lines = append(lines, styleViolation.Render(truncate("  "+v.FromPkg, w-1)))
				lines = append(lines, styleDim.Render(truncate("  "+v.Rule.Reason, w-1)))
			}
		}
	}

	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHelpBar() string {
	violStr := "off"
	if m.violOnly {
		violStr = "on"
	}
	groupStr := "pkg"
	if m.group == GroupFile {
		groupStr = "file"
	}
	parts := []string{
		"[Tab] pane",
		"[↑↓/jk] nav",
		"[Enter] load",
		"[g] group=" + groupStr,
		"[f] violations=" + violStr,
		"[Esc] search",
		"[q] back/quit",
	}
	return styleHelp.Render(strings.Join(parts, "  "))
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
