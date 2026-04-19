package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleTitle       = lipgloss.NewStyle().Bold(true)
	styleActiveTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleActive      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleSelected    = lipgloss.NewStyle().Background(lipgloss.Color("238")).Bold(true)
	styleCurrent     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	styleItem        = lipgloss.NewStyle()
	styleViolation   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleHelp        = lipgloss.NewStyle().Faint(true)
	styleDim         = lipgloss.NewStyle().Faint(true)
	styleDetail      = lipgloss.NewStyle()
	styleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)
