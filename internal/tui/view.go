package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"go.flaticols.dev/gorefactor/internal/graph"
)

func (m Model) View() string {
	if m.err != nil {
		return styleError.Render(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
	}
	if m.width == 0 {
		return "loading…"
	}

	helpView := m.help.View(m.keys)
	helpH := lipgloss.Height(helpView)

	dropdownH := 0
	if m.inputMode && len(m.searchResults) > 0 {
		dropdownH = min(len(m.searchResults), 10)
	}

	contentH := max(m.height-1-helpH-dropdownH, 3)

	// Three columns. Picker gets a third, the rest split between API and refs.
	pickW := max(m.width/3, 18)
	rest := m.width - pickW
	apiW := max(rest/2, 12)
	refW := max(rest-apiW, 8)

	pickCol := m.renderPicker(pickW, contentH)
	apiCol := m.renderAPI(apiW, contentH)
	refCol := m.renderRefs(refW, contentH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, pickCol, apiCol, refCol)

	var parts []string
	parts = append(parts, m.renderTopBar())
	if dropdownH > 0 {
		parts = append(parts, m.renderSearchDropdown(dropdownH))
	}
	parts = append(parts, body, helpView)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderTopBar() string {
	if m.inputMode {
		return styleActive.Render("/") + " " + m.input.View()
	}
	status := "Packages"
	if m.loadingGraph {
		status = m.spinner.View() + " loading package graph"
	} else if m.selectedPkg != "" {
		status = styleActive.Render(m.shortPkg(m.selectedPkg))
		if m.pendingLoad {
			status += " " + m.spinner.View()
		}
	}
	if m.flash != "" {
		status += styleHelp.Render("   " + m.flash)
	}
	return "  " + status
}

func (m Model) renderPicker(w, h int) string {
	title := fmt.Sprintf("Packages [%s]", m.pickMode)
	title = m.titleStyled(title, panePicker)

	lines := make([]string, 0, len(m.pickRows))
	for i, r := range m.pickRows {
		indent := strings.Repeat("  ", r.depth)
		marker := "  "
		if r.expandable {
			if r.expanded {
				marker = "▾ "
			} else {
				marker = "▸ "
			}
		}
		label := indent + marker + r.label
		lines = append(lines, m.styleRow(label, w, i == m.pickIdx, m.active == panePicker))
	}
	if len(lines) == 0 {
		lines = append(lines, styleHelp.Render("  (no packages)"))
	}
	return m.framePane(title, lines, &m.pickVP, w, h, m.pickIdx)
}

func (m Model) renderAPI(w, h int) string {
	title := "Public API"
	title = m.titleStyled(title, paneAPI)

	if m.selectedPkg == "" {
		body := styleHelp.Render("  select a package")
		return lipgloss.JoinVertical(lipgloss.Left, padRight(title, w), body)
	}

	lines := make([]string, 0, len(m.apiRows))
	for i, r := range m.apiRows {
		var label string
		if r.header {
			label = styleDim.Render(strings.ToUpper(r.label))
		} else {
			indent := strings.Repeat("  ", r.depth)
			marker := "  "
			if r.expandable {
				if r.expanded {
					marker = "▾ "
				} else {
					marker = "▸ "
				}
			}
			label = indent + marker + r.label
		}
		selected := i == m.apiIdx && !r.header
		lines = append(lines, m.styleRow(label, w, selected, m.active == paneAPI))
	}
	if len(lines) == 0 {
		if m.pendingLoad {
			lines = append(lines, styleHelp.Render("  "+m.spinner.View()+" loading…"))
		} else {
			lines = append(lines, styleHelp.Render("  (no exported symbols)"))
		}
	}
	return m.framePane(title, lines, &m.apiVP, w, h, m.apiIdx)
}

func (m Model) renderRefs(w, h int) string {
	sym := m.symbolFocus()
	if sym != (graph.Symbol{}) {
		groupName := "pkg"
		switch m.group {
		case GroupFile:
			groupName = "file"
		case GroupFunc:
			groupName = "func"
		}
		title := fmt.Sprintf("Uses of %s [group=%s]", sym.Name, groupName)
		title = m.titleStyled(title, paneRefs)
		lines := make([]string, len(m.refLines))
		for i, line := range m.refLines {
			lines[i] = m.styleRow(line, w, i == m.refIdx, m.active == paneRefs)
		}
		return m.framePane(title, lines, &m.refVP, w, h, m.refIdx)
	}

	mode := "flat"
	if m.refTree {
		mode = "tree"
	}
	title := fmt.Sprintf("Importers [%s]", mode)
	title = m.titleStyled(title, paneRefs)
	if m.selectedPkg == "" {
		body := styleHelp.Render("  select a package")
		return lipgloss.JoinVertical(lipgloss.Left, padRight(title, w), body)
	}
	lines := make([]string, 0, len(m.refRows))
	for i, r := range m.refRows {
		indent := strings.Repeat("  ", r.depth)
		marker := "  "
		if r.expandable {
			if r.expanded {
				marker = "▾ "
			} else {
				marker = "▸ "
			}
		}
		label := indent + marker + r.label
		lines = append(lines, m.styleRow(label, w, i == m.refIdx, m.active == paneRefs))
	}
	return m.framePane(title, lines, &m.refVP, w, h, m.refIdx)
}

// styleRow applies selection styling to a single rendered line.
func (m Model) styleRow(label string, w int, selected, paneActive bool) string {
	label = truncate(label, w-1)
	switch {
	case selected && paneActive:
		return styleSelected.Width(w).Render(label)
	case selected:
		return styleCurrent.Render(padRight(label, w))
	default:
		return styleItem.Render(padRight(label, w))
	}
}

// framePane assembles a titled, scrolled column.
func (m Model) framePane(title string, lines []string, vp *viewport.Model, w, h, idx int) string {
	vp.Width = w
	vp.Height = h - 1
	vp.SetContent(strings.Join(lines, "\n"))
	ensureVisible(vp, idx)
	return lipgloss.JoinVertical(lipgloss.Left, padRight(title, w), vp.View())
}

func (m Model) titleStyled(s string, p pane) string {
	if m.active == p {
		return styleActiveTitle.Render(s)
	}
	return styleTitle.Render(s)
}

func (m Model) renderSearchDropdown(h int) string {
	visible := m.searchResults
	if len(visible) > h {
		visible = visible[:h]
	}
	w := m.width
	lines := make([]string, 0, len(visible))
	for i, r := range visible {
		shortP := m.shortPkg(r.pkg)
		if i == m.searchSel {
			lines = append(lines, styleSelected.Width(w).Render("▶ "+truncate(shortP, w-4)))
			continue
		}
		var label string
		if idx := strings.LastIndex(shortP, "/"); idx >= 0 {
			label = styleDim.Render(shortP[:idx+1]) + shortP[idx+1:]
		} else {
			label = shortP
		}
		lines = append(lines, "  "+label)
	}
	return strings.Join(lines, "\n")
}
