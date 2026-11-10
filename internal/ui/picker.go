package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mikim83/dynamodb-client/internal/awsconfig"
)

type profileItem struct{ p awsconfig.Profile }

func (i profileItem) Title() string       { return i.p.Name }
func (i profileItem) FilterValue() string { return i.p.Name }
func (i profileItem) Description() string {
	var parts []string
	if i.p.Region != "" {
		parts = append(parts, i.p.Region)
	}
	if k := i.p.Kind(); k != "" {
		parts = append(parts, k)
	}
	return strings.Join(parts, " · ")
}

type pickerModel struct {
	list   list.Model
	chosen string
	done   bool
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "enter":
				if it, ok := m.list.SelectedItem().(profileItem); ok {
					m.chosen, m.done = it.p.Name, true
				}
				return m, tea.Quit
			case "q", "esc", "ctrl+c":
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string { return m.list.View() }

// PickProfile shows the profiles and returns the one chosen. ok is false if
// the user quit without choosing.
func PickProfile(profiles []awsconfig.Profile, current string) (name string, ok bool, err error) {
	items := make([]list.Item, len(profiles))
	sel := 0
	for i, p := range profiles {
		items[i] = profileItem{p}
		if p.Name == current {
			sel = i
		}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "AWS profile"
	l.Styles.Title = titleStyle
	l.KeyMap.Quit.SetEnabled(false)
	l.Select(sel)

	final, err := tea.NewProgram(pickerModel{list: l}, tea.WithAltScreen()).Run()
	if err != nil {
		return "", false, fmt.Errorf("profile picker: %w", err)
	}
	m := final.(pickerModel)
	return m.chosen, m.done, nil
}
