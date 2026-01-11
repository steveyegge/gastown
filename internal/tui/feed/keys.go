package feed

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the key bindings for the feed TUI.
type KeyMap struct {
	// Navigation
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding

	// Panel switching
	Tab         key.Binding
	ShiftTab    key.Binding
	FocusTree   key.Binding
	FocusConvoy key.Binding
	FocusFeed   key.Binding

	// Actions
	Enter   key.Binding
	Expand  key.Binding
	Refresh key.Binding

	// Search/Filter
	Search      key.Binding
	Filter      key.Binding
	ClearFilter key.Binding

	// General
	Help key.Binding
	Quit key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdn", "page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G", "bottom"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch panel"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("S-tab", "prev panel"),
		),
		FocusTree: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "agent tree"),
		),
		FocusConvoy: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "convoys"),
		),
		FocusFeed: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "event feed"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "expand/details"),
		),
		Expand: key.NewBinding(
			key.WithKeys("o", "l"),
			key.WithHelp("o", "toggle expand"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter"),
		),
		ClearFilter: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp returns key bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Tab, k.Search, k.Filter, k.Quit, k.Help}
}

// FullHelp returns key bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.Tab, k.FocusTree, k.FocusConvoy, k.FocusFeed, k.Enter, k.Expand},
		{k.Search, k.Filter, k.ClearFilter, k.Refresh},
		{k.Help, k.Quit},
	}
}
