package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Search     key.Binding
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Back       key.Binding
	Group      key.Binding
	Violations key.Binding
	ModuleOnly key.Binding
	Detail     key.Binding
	Open       key.Binding
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
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Group:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "group")),
		Violations: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "violations")),
		ModuleOnly: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "module-only")),
		Detail:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "detail")),
		Open:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in $EDITOR")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c", "ctrl+q"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Up, k.Down, k.Open, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Search, k.Up, k.Down, k.Left, k.Right, k.Back},
		{k.Group, k.Violations, k.ModuleOnly, k.Detail, k.Open},
		{k.Help, k.Quit},
	}
}
