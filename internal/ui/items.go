package ui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/mikim83/dynamodb-client/internal/ddb"
)

const (
	maxColWidth = 40
	colGap      = "  "
)

// itemsState is the paged view of one table.
//
// By default it scans page by page, so sorting only reorders the page on
// screen. "Load all" reads the whole table (up to a cap) into memory; then
// sorting is global and pages are slices of the sorted items.
type itemsState struct {
	info  ddb.TableInfo
	pager *ddb.Pager

	page    int
	hasNext bool
	raw     []ddb.Item // current page as read
	rows    []ddb.Item // current page as shown (sorted)

	allMode   bool
	all       []ddb.Item // whole table, sorted, when allMode
	allRaw    []ddb.Item
	truncated bool

	columns           []string
	colSel, colOffset int
	rowSel, rowOffset int
	sortCol           string
	sortDesc          bool
	note              string
}

func newItemsState(info ddb.TableInfo, pager *ddb.Pager) itemsState {
	return itemsState{info: info, pager: pager}
}

func (s *itemsState) size() int { return s.pager.Size() }

func (s *itemsState) reset() {
	s.pager.Reset()
	s.allMode, s.all, s.allRaw, s.truncated = false, nil, nil, false
	s.page, s.rowSel, s.rowOffset = 0, 0, 0
}

// showPage displays a page read from DynamoDB (paged mode).
func (s *itemsState) showPage(pg ddb.Page) {
	if pg.Index != s.page {
		s.rowSel, s.rowOffset = 0, 0
	}
	s.page, s.hasNext = pg.Index, pg.HasNext
	s.raw = pg.Items
	s.refresh()
}

// setAll switches to the in-memory mode with the whole table.
func (s *itemsState) setAll(items []ddb.Item, truncated bool, cap int) {
	s.allMode, s.allRaw, s.truncated = true, items, truncated
	s.page, s.rowSel, s.rowOffset = 0, 0, 0
	s.refresh()
	if truncated {
		s.note = fmt.Sprintf("Loaded the first %d items only (--max-items).", cap)
	} else {
		s.note = fmt.Sprintf("Loaded all %d items: sorting now applies to the whole table.", len(items))
	}
}

func (s *itemsState) lastAllPage() int {
	if len(s.all) == 0 {
		return 0
	}
	return (len(s.all) - 1) / s.size()
}

func (s *itemsState) gotoAllPage(p int) {
	last := s.lastAllPage()
	switch {
	case p < 0:
		s.note = "This is the first page."
		return
	case p > last:
		s.note = "This is the last page."
		return
	}
	if p != s.page {
		s.rowSel, s.rowOffset = 0, 0
	}
	s.page = p
	s.slicePage()
}

// refresh re-applies the sort and recomputes the columns.
func (s *itemsState) refresh() {
	selName := ""
	if s.colSel < len(s.columns) {
		selName = s.columns[s.colSel]
	}
	if s.allMode {
		s.all = append([]ddb.Item(nil), s.allRaw...)
		if s.sortCol != "" {
			ddb.SortItems(s.all, s.sortCol, s.sortDesc)
		}
		s.columns = ddb.Columns(s.all, s.info.Keys())
		s.slicePage()
	} else {
		s.rows = append([]ddb.Item(nil), s.raw...)
		if s.sortCol != "" {
			ddb.SortItems(s.rows, s.sortCol, s.sortDesc)
		}
		s.columns = ddb.Columns(s.rows, s.info.Keys())
	}
	s.colSel = 0
	for i, c := range s.columns {
		if c == selName {
			s.colSel = i
		}
	}
	s.clampRow()
}

func (s *itemsState) slicePage() {
	from := min(s.page*s.size(), len(s.all))
	to := min(from+s.size(), len(s.all))
	s.rows = s.all[from:to]
	s.clampRow()
}

func (s *itemsState) clampRow() {
	if s.rowSel >= len(s.rows) {
		s.rowSel = max(len(s.rows)-1, 0)
	}
}

func (s *itemsState) moveRow(d int) {
	s.rowSel = min(max(s.rowSel+d, 0), max(len(s.rows)-1, 0))
}

func (s *itemsState) moveCol(d int) {
	if len(s.columns) == 0 {
		return
	}
	s.colSel = (s.colSel + d + len(s.columns)) % len(s.columns)
}

// cycleSort goes ascending → descending → off on the selected column.
func (s *itemsState) cycleSort() {
	if len(s.columns) == 0 {
		return
	}
	col := s.columns[s.colSel]
	switch {
	case s.sortCol != col:
		s.sortCol, s.sortDesc = col, false
	case !s.sortDesc:
		s.sortDesc = true
	default:
		s.sortCol = ""
	}
	if s.allMode {
		s.page, s.rowSel, s.rowOffset = 0, 0, 0
	}
	s.refresh()
}

func (s *itemsState) clearSort() {
	s.sortCol = ""
	s.refresh()
}

func (s *itemsState) selected() (ddb.Item, bool) {
	if s.rowSel < len(s.rows) {
		return s.rows[s.rowSel], true
	}
	return nil, false
}

func (s *itemsState) title() string {
	return fmt.Sprintf("%s  %s", s.info.Name, dimStyle.Render("("+strings.Join(s.info.Keys(), ", ")+")"))
}

func (s *itemsState) status() string {
	first := s.page*s.size() + 1
	last := first + len(s.rows) - 1
	if len(s.rows) == 0 {
		first, last = 0, 0
	}
	var parts []string
	if s.allMode {
		parts = append(parts, fmt.Sprintf("page %d/%d", s.page+1, s.lastAllPage()+1),
			fmt.Sprintf("rows %d–%d of %d", first, last, len(s.all)))
		if s.truncated {
			parts = append(parts, "truncated")
		}
	} else {
		more := ""
		if s.hasNext {
			more = " (more →)"
		}
		parts = append(parts, fmt.Sprintf("page %d%s", s.page+1, more),
			fmt.Sprintf("rows %d–%d", first, last),
			fmt.Sprintf("~%d items", s.info.ItemCount))
	}
	if s.sortCol != "" {
		dir := "▲"
		if s.sortDesc {
			dir = "▼"
		}
		scope := "this page"
		if s.allMode {
			scope = "all loaded"
		}
		parts = append(parts, fmt.Sprintf("sort %s %s (%s)", s.sortCol, dir, scope))
	}
	return strings.Join(parts, " · ")
}

func (s *itemsState) colWidths() []int {
	widths := make([]int, len(s.columns))
	for i, c := range s.columns {
		w := runewidth.StringWidth(c) + 2 // room for the sort arrow
		for _, r := range s.rows {
			w = max(w, runewidth.StringWidth(ddb.Display(r[c])))
		}
		widths[i] = min(max(w, 3), maxColWidth)
	}
	return widths
}

// offsets returns the first visible column and row so the selected cell is
// on screen, moving the previous offsets as little as possible.
func (s *itemsState) offsets(widths []int, width, bodyH int) (colOff, rowOff int) {
	colOff, rowOff = min(s.colOffset, max(len(widths)-1, 0)), s.rowOffset
	if s.colSel < colOff {
		colOff = s.colSel
	}
	for colOff < s.colSel && span(widths[colOff:s.colSel+1]) > width {
		colOff++
	}
	if s.rowSel < rowOff {
		rowOff = s.rowSel
	}
	if s.rowSel >= rowOff+bodyH {
		rowOff = s.rowSel - bodyH + 1
	}
	return colOff, rowOff
}

// adjust stores the scroll offsets for a screen of the given size. Update
// calls it after every change, since View cannot keep state.
func (s *itemsState) adjust(width, height int) {
	s.colOffset, s.rowOffset = s.offsets(s.colWidths(), width, max(height-1, 1))
}

// render draws the header and the visible rows, scrolled so the selected
// column and row are in view.
func (s *itemsState) render(width, height int) string {
	if len(s.rows) == 0 {
		return dimStyle.Render("No items.") + strings.Repeat("\n", max(height-1, 0))
	}
	widths := s.colWidths()
	bodyH := max(height-1, 1) // one line goes to the header
	colOff, rowOff := s.offsets(widths, width, bodyH)

	var vis []int
	used := 0
	for i := colOff; i < len(s.columns); i++ {
		need := widths[i]
		if len(vis) > 0 {
			need += len(colGap)
		}
		if len(vis) > 0 && used+need > width {
			break
		}
		vis = append(vis, i)
		used += need
	}

	var b strings.Builder
	head := make([]string, len(vis))
	for j, i := range vis {
		name := s.columns[i]
		if name == s.sortCol {
			if s.sortDesc {
				name += " ▼"
			} else {
				name += " ▲"
			}
		}
		cell := fit(name, widths[i])
		if i == s.colSel {
			head[j] = selColStyle.Render(cell)
		} else {
			head[j] = headerStyle.Render(cell)
		}
	}
	b.WriteString(strings.Join(head, colGap))

	end := min(rowOff+bodyH, len(s.rows))
	for r := rowOff; r < end; r++ {
		cells := make([]string, len(vis))
		for j, i := range vis {
			v, ok := s.rows[r][s.columns[i]]
			text := "·"
			if ok {
				text = ddb.Display(v)
			}
			cells[j] = fit(text, widths[i])
		}
		line := runewidth.Truncate(strings.Join(cells, colGap), width, "")
		b.WriteString("\n")
		if r == s.rowSel {
			b.WriteString(selRowStyle.Render(runewidth.FillRight(line, width)))
		} else {
			b.WriteString(line)
		}
	}
	b.WriteString(strings.Repeat("\n", bodyH-(end-rowOff)))
	return b.String()
}

func span(ws []int) int {
	t := 0
	for i, w := range ws {
		if i > 0 {
			t += len(colGap)
		}
		t += w
	}
	return t
}

func fit(s string, w int) string {
	return runewidth.FillRight(runewidth.Truncate(s, w, "…"), w)
}
