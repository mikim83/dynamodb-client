package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up, Down            key.Binding
	NextPage, PrevPage  key.Binding
	FirstPage, LastPage key.Binding
	NextCol, PrevCol    key.Binding
	Sort, ClearSort     key.Binding
	LoadAll             key.Binding
	Open, Back          key.Binding
	Reload, Profile     key.Binding
	Help, Quit          key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		NextPage:  key.NewBinding(key.WithKeys("right", "n", "pgdown", "l"), key.WithHelp("→/n", "next page")),
		PrevPage:  key.NewBinding(key.WithKeys("left", "p", "pgup", "h"), key.WithHelp("←/p", "prev page")),
		FirstPage: key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "first page")),
		LastPage:  key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "last page (all loaded)")),
		NextCol:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next column")),
		PrevCol:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev column")),
		Sort:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort ▲/▼/off")),
		ClearSort: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "clear sort")),
		LoadAll:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "load all (global sort)")),
		Open:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:      key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back")),
		Reload:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Profile:   key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "switch profile")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// itemsHelp adapts keyMap to help.KeyMap for the items screen.
type itemsHelp struct{ k keyMap }

func (h itemsHelp) ShortHelp() []key.Binding {
	return []key.Binding{h.k.NextPage, h.k.PrevPage, h.k.NextCol, h.k.Sort, h.k.Open, h.k.Back, h.k.Help}
}

func (h itemsHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{h.k.Up, h.k.Down, h.k.Open, h.k.Back},
		{h.k.NextPage, h.k.PrevPage, h.k.FirstPage, h.k.LastPage},
		{h.k.NextCol, h.k.PrevCol, h.k.Sort, h.k.ClearSort},
		{h.k.LoadAll, h.k.Reload, h.k.Profile, h.k.Quit},
	}
}
