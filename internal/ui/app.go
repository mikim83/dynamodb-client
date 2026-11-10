// Package ui is the terminal interface: a table list, a paged item view with
// sorting and an item detail view. It never writes to DynamoDB.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mikim83/dynamodb-client/internal/ddb"
)

const callTimeout = 60 * time.Second

// Options configure a session against one account and region.
type Options struct {
	API      ddb.API
	Profile  string // shown in the status bar only
	Region   string
	PageSize int
	MaxItems int    // cap for "load all"
	Table    string // open this table straight away
}

// Result tells the caller what to do once the UI exits.
type Result struct {
	SwitchProfile bool
}

type screen int

const (
	scrTables screen = iota
	scrItems
	scrDetail
)

type tableItem string

func (t tableItem) FilterValue() string { return string(t) }
func (t tableItem) Title() string       { return string(t) }
func (t tableItem) Description() string { return "" }

type (
	tablesMsg struct {
		names []string
		err   error
	}
	openMsg struct {
		info  ddb.TableInfo
		pager *ddb.Pager
		page  ddb.Page
		err   error
	}
	pageMsg struct {
		page ddb.Page
		err  error
	}
	allMsg struct {
		items     []ddb.Item
		truncated bool
		err       error
	}
)

// Model is the root Bubble Tea model.
type Model struct {
	opt    Options
	keys   keyMap
	help   help.Model
	width  int
	height int
	screen screen

	tables list.Model
	items  itemsState
	detail viewport.Model

	loading string
	err     error
	result  Result
}

// New returns the root model.
func New(opt Options) Model {
	if opt.PageSize < 1 {
		opt.PageSize = 50
	}
	if opt.MaxItems < 1 {
		opt.MaxItems = 10000
	}
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	l := list.New(nil, d, 0, 0)
	l.Title = "Tables"
	l.Styles.Title = titleStyle
	l.KeyMap.Quit.SetEnabled(false)
	l.SetStatusBarItemName("table", "tables")

	return Model{
		opt:     opt,
		keys:    newKeyMap(),
		help:    help.New(),
		tables:  l,
		detail:  viewport.New(0, 0),
		loading: "Loading tables…",
	}
}

// Run starts the UI and blocks until it exits.
func Run(opt Options) (Result, error) {
	final, err := tea.NewProgram(New(opt), tea.WithAltScreen()).Run()
	if err != nil {
		return Result{}, err
	}
	return final.(Model).result, nil
}

func (m Model) Init() tea.Cmd { return m.loadTables() }

func (m Model) loadTables() tea.Cmd {
	api := m.opt.API
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		names, err := ddb.ListTables(ctx, api)
		return tablesMsg{names, err}
	}
}

func (m Model) openTable(name string) tea.Cmd {
	api, size := m.opt.API, m.opt.PageSize
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		info, err := ddb.Describe(ctx, api, name)
		if err != nil {
			return openMsg{err: err}
		}
		pager := ddb.NewPager(api, name, size)
		pg, err := pager.Page(ctx, 0)
		return openMsg{info: info, pager: pager, page: pg, err: err}
	}
}

func fetchPage(p *ddb.Pager, i int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		pg, err := p.Page(ctx, i)
		return pageMsg{pg, err}
	}
}

func (m Model) loadAll() tea.Cmd {
	api, table, max := m.opt.API, m.items.info.Name, m.opt.MaxItems
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*callTimeout)
		defer cancel()
		items, truncated, err := ddb.ScanAll(ctx, api, table, max)
		return allMsg{items, truncated, err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm := next.(Model)
	if nm.screen == scrItems && nm.width > 0 {
		nm.items.adjust(nm.width, nm.itemsHeight())
	}
	return nm, cmd
}

// itemsHeight is the number of lines left for the item table: the screen
// minus the title, the help and the status line.
func (m Model) itemsHeight() int {
	helpLines := strings.Count(m.help.View(itemsHelp{m.keys}), "\n") + 1
	return max(m.height-2-helpLines, 3)
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.tables.SetSize(msg.Width, msg.Height-2)
		m.detail.Width, m.detail.Height = msg.Width, max(msg.Height-3, 1)
		return m, nil

	case tablesMsg:
		m.loading, m.err = "", msg.err
		if msg.err == nil {
			items := make([]list.Item, len(msg.names))
			for i, n := range msg.names {
				items[i] = tableItem(n)
			}
			cmd := m.tables.SetItems(items)
			if m.opt.Table != "" {
				name := m.opt.Table
				m.opt.Table = ""
				m.loading = "Opening " + name + "…"
				return m, tea.Batch(cmd, m.openTable(name))
			}
			return m, cmd
		}
		return m, nil

	case openMsg:
		m.loading, m.err = "", msg.err
		if msg.err == nil {
			m.items = newItemsState(msg.info, msg.pager)
			m.items.showPage(msg.page)
			m.screen = scrItems
		}
		return m, nil

	case pageMsg:
		m.loading, m.err = "", msg.err
		if msg.err == nil {
			if len(msg.page.Items) == 0 && msg.page.Index > 0 {
				m.items.hasNext = false
				m.items.note = "No more items."
			} else {
				m.items.showPage(msg.page)
			}
		}
		return m, nil

	case allMsg:
		m.loading, m.err = "", msg.err
		if msg.err == nil {
			m.items.setAll(msg.items, msg.truncated, m.opt.MaxItems)
		}
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) && msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.loading != "" {
			return m, nil // one call at a time
		}
		switch m.screen {
		case scrTables:
			return m.updateTables(msg)
		case scrItems:
			return m.updateItems(msg)
		case scrDetail:
			return m.updateDetail(msg)
		}
	}

	if m.screen == scrTables {
		var cmd tea.Cmd
		m.tables, cmd = m.tables.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateTables(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tables.FilterState() != list.Filtering {
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Profile):
			m.result.SwitchProfile = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Reload):
			m.loading, m.err = "Loading tables…", nil
			return m, m.loadTables()
		case key.Matches(msg, m.keys.Open):
			if t, ok := m.tables.SelectedItem().(tableItem); ok {
				m.loading, m.err = "Opening "+string(t)+"…", nil
				return m, m.openTable(string(t))
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.tables, cmd = m.tables.Update(msg)
	return m, cmd
}

func (m Model) updateItems(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	it := &m.items
	m.err, it.note = nil, ""
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back):
		m.screen = scrTables
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(msg, m.keys.Up):
		it.moveRow(-1)
	case key.Matches(msg, m.keys.Down):
		it.moveRow(1)
	case key.Matches(msg, m.keys.NextCol):
		it.moveCol(1)
	case key.Matches(msg, m.keys.PrevCol):
		it.moveCol(-1)
	case key.Matches(msg, m.keys.Sort):
		it.cycleSort()
	case key.Matches(msg, m.keys.ClearSort):
		it.clearSort()
	case key.Matches(msg, m.keys.NextPage):
		if it.allMode {
			it.gotoAllPage(it.page + 1)
		} else if !it.hasNext {
			it.note = "This is the last page."
		} else {
			m.loading = fmt.Sprintf("Loading page %d…", it.page+2)
			return m, fetchPage(it.pager, it.page+1)
		}
	case key.Matches(msg, m.keys.PrevPage):
		if it.page == 0 {
			it.note = "This is the first page."
		} else if it.allMode {
			it.gotoAllPage(it.page - 1)
		} else {
			return m, fetchPage(it.pager, it.page-1)
		}
	case key.Matches(msg, m.keys.FirstPage):
		if it.allMode {
			it.gotoAllPage(0)
		} else if it.page > 0 {
			return m, fetchPage(it.pager, 0)
		}
	case key.Matches(msg, m.keys.LastPage):
		if it.allMode {
			it.gotoAllPage(it.lastAllPage())
		} else {
			it.note = "Press a to load the whole table first."
		}
	case key.Matches(msg, m.keys.LoadAll):
		if !it.allMode {
			m.loading = fmt.Sprintf("Loading up to %d items…", m.opt.MaxItems)
			return m, m.loadAll()
		}
	case key.Matches(msg, m.keys.Reload):
		it.reset()
		m.loading = "Reloading…"
		return m, fetchPage(it.pager, 0)
	case key.Matches(msg, m.keys.Open):
		if row, ok := it.selected(); ok {
			m.detail.SetContent(ddb.ItemJSON(row))
			m.detail.GotoTop()
			m.screen = scrDetail
		}
	}
	return m, nil
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back):
		m.screen = scrItems
		return m, nil
	}
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	var body string
	switch m.screen {
	case scrTables:
		body = m.tables.View()
	case scrItems:
		body = titleStyle.Render(m.items.title()) + "\n" +
			m.items.render(m.width, m.itemsHeight()) + "\n" +
			m.help.View(itemsHelp{m.keys})
	case scrDetail:
		row, _ := m.items.selected()
		body = titleStyle.Render(m.items.info.Name+" · "+keyLabel(row, m.items.info.Keys())) + "\n" +
			m.detail.View() + "\n" +
			dimStyle.Render("↑/↓ scroll · esc back · q quit")
	}
	return body + "\n" + m.statusLine()
}

func (m Model) statusLine() string {
	ctx := m.opt.Region
	if m.opt.Profile != "" {
		ctx = m.opt.Profile + " · " + ctx
	}
	switch {
	case m.loading != "":
		return dimStyle.Render(ctx+" · ") + noteStyle.Render(m.loading)
	case m.err != nil:
		return errStyle.Render(oneLine(m.err.Error()))
	case m.screen != scrTables && m.items.note != "":
		return dimStyle.Render(ctx+" · ") + noteStyle.Render(m.items.note)
	case m.screen == scrTables:
		return dimStyle.Render(ctx + " · enter open · / filter · r reload · P profile · q quit")
	default:
		return dimStyle.Render(ctx + " · " + m.items.status())
	}
}

func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " ") }

func keyLabel(row ddb.Item, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+ddb.Display(row[k]))
	}
	return strings.Join(parts, " ")
}
