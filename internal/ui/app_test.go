package ui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mikim83/dynamodb-client/internal/ddb"
)

// fakeAPI is a table "events" of n items: id (N) and score = (id*7) % n.
type fakeAPI struct{ n int }

func (f fakeAPI) ListTables(context.Context, *dynamodb.ListTablesInput, ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	return &dynamodb.ListTablesOutput{TableNames: []string{"events"}}, nil
}

func (f fakeAPI) DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	return &dynamodb.DescribeTableOutput{Table: &types.TableDescription{
		TableName: aws.String("events"),
		ItemCount: aws.Int64(int64(f.n)),
		KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash}},
	}}, nil
}

func (f fakeAPI) Scan(_ context.Context, in *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	start := 0
	if k, ok := in.ExclusiveStartKey["id"].(*types.AttributeValueMemberN); ok {
		start, _ = strconv.Atoi(k.Value)
		start++
	}
	end := min(start+int(aws.ToInt32(in.Limit)), f.n)
	out := &dynamodb.ScanOutput{}
	for i := start; i < end; i++ {
		out.Items = append(out.Items, ddb.Item{
			"id":    &types.AttributeValueMemberN{Value: strconv.Itoa(i)},
			"score": &types.AttributeValueMemberN{Value: strconv.Itoa(i * 7 % f.n)},
		})
	}
	if end < f.n {
		out.LastEvaluatedKey = ddb.Item{"id": &types.AttributeValueMemberN{Value: strconv.Itoa(end - 1)}}
	}
	return out, nil
}

// drive feeds msg to the model and runs the commands it returns until none
// is left, the way the Bubble Tea runtime would.
func drive(t *testing.T, m tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		var cmd tea.Cmd
		m, cmd = m.Update(queue[0])
		queue = queue[1:]
		queue = append(queue, run(cmd)...)
	}
	return m
}

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, run(c)...)
		}
		return out
	case tablesMsg, openMsg, pageMsg, allMsg, deleteMsg:
		return []tea.Msg{msg}
	default:
		return nil // list status timers and the like
	}
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func open(t *testing.T, n int) Model {
	t.Helper()
	return openWith(t, Options{API: fakeAPI{n: n}})
}

func openWith(t *testing.T, opt Options) Model {
	t.Helper()
	opt.Region, opt.PageSize, opt.Table = "eu-west-1", 50, "events"
	var m tea.Model = New(opt)
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = drive(t, m, m.Init()())
	got := m.(Model)
	if got.screen != scrItems {
		t.Fatalf("table did not open: err=%v", got.err)
	}
	return got
}

func firstID(m Model) string { return ddb.Display(m.items.rows[0]["id"]) }

func TestPagingForwardAndBack(t *testing.T) {
	m := open(t, 120)
	if len(m.items.rows) != 50 || firstID(m) != "0" {
		t.Fatalf("page 1: %d rows, first %s", len(m.items.rows), firstID(m))
	}
	var tm tea.Model = m
	tm = drive(t, tm, keyMsg("right"))
	tm = drive(t, tm, keyMsg("n"))
	m = tm.(Model)
	if m.items.page != 2 || len(m.items.rows) != 20 || firstID(m) != "100" {
		t.Fatalf("page 3: page=%d rows=%d first=%s", m.items.page, len(m.items.rows), firstID(m))
	}
	tm = drive(t, tm, keyMsg("right"))
	if !strings.Contains(tm.(Model).items.note, "last page") {
		t.Fatalf("expected last-page note, got %q", tm.(Model).items.note)
	}
	tm = drive(t, tm, keyMsg("left"))
	if m = tm.(Model); m.items.page != 1 || firstID(m) != "50" {
		t.Fatalf("back: page=%d first=%s", m.items.page, firstID(m))
	}
	if !strings.Contains(m.View(), "page 2") {
		t.Fatal("status bar should show page 2")
	}
}

func TestSortPageThenGlobal(t *testing.T) {
	var tm tea.Model = open(t, 120)
	tm = drive(t, tm, keyMsg("tab")) // select "score"
	tm = drive(t, tm, keyMsg("s"))   // ascending, this page only
	m := tm.(Model)
	if m.items.sortCol != "score" || m.items.allMode {
		t.Fatalf("sort=%q all=%v", m.items.sortCol, m.items.allMode)
	}
	if got := ddb.Display(m.items.rows[0]["score"]); got != "0" {
		t.Fatalf("page sort: lowest score on page 1 is 0, got %s", got)
	}
	tm = drive(t, tm, keyMsg("s")) // descending
	m = tm.(Model)
	pageMax := ddb.Display(m.items.rows[0]["score"])

	tm = drive(t, tm, keyMsg("a")) // load all: global sort
	m = tm.(Model)
	if !m.items.allMode || len(m.items.all) != 120 {
		t.Fatalf("load all: mode=%v items=%d", m.items.allMode, len(m.items.all))
	}
	if got := ddb.Display(m.items.rows[0]["score"]); got != "119" {
		t.Fatalf("global desc sort should start at 119, got %s (page max was %s)", got, pageMax)
	}
	tm = drive(t, tm, keyMsg("G"))
	if m = tm.(Model); m.items.page != 2 || len(m.items.rows) != 20 {
		t.Fatalf("last page: page=%d rows=%d", m.items.page, len(m.items.rows))
	}
}

func TestDetailView(t *testing.T) {
	var tm tea.Model = open(t, 10)
	tm = drive(t, tm, keyMsg("enter"))
	m := tm.(Model)
	if m.screen != scrDetail || !strings.Contains(m.View(), `"score": 0`) {
		t.Fatalf("detail view not shown:\n%s", m.View())
	}
}

// fakeDeleter records the ids it was asked to delete.
type fakeDeleter struct {
	ids []string
	err error
}

func (f *fakeDeleter) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.ids = append(f.ids, ddb.Display(in.Key["id"]))
	return &dynamodb.DeleteItemOutput{}, f.err
}

func ids(m Model) string {
	var out []string
	for _, r := range m.items.rows[:min(3, len(m.items.rows))] {
		out = append(out, ddb.Display(r["id"]))
	}
	return strings.Join(out, ",")
}

func TestReadOnlyIgnoresDelete(t *testing.T) {
	var tm tea.Model = open(t, 10)
	tm = drive(t, tm, keyMsg("d"))
	m := tm.(Model)
	if m.confirm != nil || strings.Contains(m.View(), "EDIT") || strings.Contains(m.View(), "delete") {
		t.Fatalf("read-only session offered a delete:\n%s", m.View())
	}
}

func TestDeleteConfirmed(t *testing.T) {
	d := &fakeDeleter{}
	var tm tea.Model = openWith(t, Options{API: fakeAPI{n: 120}, Deleter: d})
	if !strings.Contains(tm.View(), "EDIT") {
		t.Fatal("status bar should say EDIT")
	}
	tm = drive(t, tm, keyMsg("j"))
	tm = drive(t, tm, keyMsg("d"))
	if v := tm.View(); !strings.Contains(v, "Delete id=1?") {
		t.Fatalf("no confirmation prompt:\n%s", v)
	}
	tm = drive(t, tm, keyMsg("y"))
	m := tm.(Model)
	if len(d.ids) != 1 || d.ids[0] != "1" {
		t.Fatalf("deleted %v", d.ids)
	}
	if len(m.items.rows) != 49 || ids(m) != "0,2,3" || m.items.note != "Deleted id=1" {
		t.Fatalf("rows=%d ids=%s note=%q", len(m.items.rows), ids(m), m.items.note)
	}
	// The cached page must not bring the row back.
	tm = drive(t, tm, keyMsg("right"))
	tm = drive(t, tm, keyMsg("left"))
	if m = tm.(Model); ids(m) != "0,2,3" {
		t.Fatalf("after paging back: %s", ids(m))
	}
}

func TestDeleteCancelled(t *testing.T) {
	d := &fakeDeleter{}
	var tm tea.Model = openWith(t, Options{API: fakeAPI{n: 10}, Deleter: d})
	tm = drive(t, tm, keyMsg("d"))
	tm = drive(t, tm, keyMsg("n"))
	m := tm.(Model)
	if len(d.ids) != 0 || len(m.items.rows) != 10 || m.items.note != "Delete cancelled." {
		t.Fatalf("deleted=%v rows=%d note=%q", d.ids, len(m.items.rows), m.items.note)
	}
}

func TestDeleteFromDetail(t *testing.T) {
	d := &fakeDeleter{}
	var tm tea.Model = openWith(t, Options{API: fakeAPI{n: 10}, Deleter: d})
	tm = drive(t, tm, keyMsg("enter"))
	tm = drive(t, tm, keyMsg("d"))
	tm = drive(t, tm, keyMsg("y"))
	m := tm.(Model)
	if m.screen != scrItems || len(d.ids) != 1 || d.ids[0] != "0" || ids(m) != "1,2,3" {
		t.Fatalf("screen=%v deleted=%v ids=%s", m.screen, d.ids, ids(m))
	}
}

func TestDeleteAlreadyGone(t *testing.T) {
	d := &fakeDeleter{err: &types.ConditionalCheckFailedException{}}
	var tm tea.Model = openWith(t, Options{API: fakeAPI{n: 10}, Deleter: d})
	tm = drive(t, tm, keyMsg("d"))
	tm = drive(t, tm, keyMsg("y"))
	m := tm.(Model)
	if m.err != nil || len(m.items.rows) != 9 || !strings.Contains(m.items.note, "Already deleted") {
		t.Fatalf("err=%v rows=%d note=%q", m.err, len(m.items.rows), m.items.note)
	}
}

func TestDeleteInLoadAllMode(t *testing.T) {
	d := &fakeDeleter{}
	var tm tea.Model = openWith(t, Options{API: fakeAPI{n: 120}, Deleter: d})
	tm = drive(t, tm, keyMsg("a"))
	tm = drive(t, tm, keyMsg("d"))
	tm = drive(t, tm, keyMsg("y"))
	if m := tm.(Model); len(m.items.all) != 119 || ids(m) != "1,2,3" {
		t.Fatalf("all=%d ids=%s", len(m.items.all), ids(m))
	}
}
