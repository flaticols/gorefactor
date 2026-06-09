// Package tui implements the terminal user interface.
package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Search     key.Binding
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Enter      key.Binding
	Back       key.Binding
	PickerMode key.Binding
	Group      key.Binding
	ModuleOnly key.Binding
	Open       key.Binding
	Copy       key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Left:       key.NewBinding(key.WithKeys("h", "shift+tab"), key.WithHelp("h", "left pane")),
		Right:      key.NewBinding(key.WithKeys("l", "tab"), key.WithHelp("l", "right pane")),
		Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select/expand")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		PickerMode: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "picker mode")),
		Group:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "group/flat")),
		ModuleOnly: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "module-only")),
		Open:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in $EDITOR")),
		Copy:       key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "copy path")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c", "ctrl+q"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Up, k.Down, k.Enter, k.PickerMode, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Enter, k.Back},
		{k.Search, k.PickerMode, k.Group, k.ModuleOnly, k.Open, k.Copy},
		{k.Help, k.Quit},
	}
}
