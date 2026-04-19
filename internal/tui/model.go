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
	paneList pane = iota
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
		inputMode: hasTarget, // start in search mode when target pre-filled
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
		m.inputMode = false
		m.input.Blur()
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

		// Search/insert mode: all keystrokes go to textinput.
		if m.inputMode {
			switch msg.String() {
			case "esc":
				m.inputMode = false
				m.input.Blur()
				return m, nil
			case "enter":
				q := strings.TrimSpace(m.input.Value())
				if q != "" {
					m.loading = true
					m.err = nil
					m.inputMode = false
					m.input.Blur()
					return m, tea.Batch(doLoad(q, m.cfg), m.spinner.Tick)
				}
				m.inputMode = false
				m.input.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
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
			return m, nil

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
				if m.group == GroupPkg {
					m.group = GroupFile
				} else {
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
	if m.inputMode {
		return styleHelp.Render("[Enter] search  [Esc] cancel")
	}
	violStr := "off"
	if m.violOnly {
		violStr = "on"
	}
	groupStr := "pkg"
	if m.group == GroupFile {
		groupStr = "file"
	}
	parts := []string{
		"[/] search",
		"[hjkl] navigate",
		"[g] group=" + groupStr,
		"[f] violations=" + violStr,
		"[q] quit",
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
