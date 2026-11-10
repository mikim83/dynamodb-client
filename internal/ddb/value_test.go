package ddb

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func s(v string) types.AttributeValue { return &types.AttributeValueMemberS{Value: v} }
func n(v string) types.AttributeValue { return &types.AttributeValueMemberN{Value: v} }

func TestDisplay(t *testing.T) {
	cases := []struct {
		in   types.AttributeValue
		want string
	}{
		{s("a\nb"), "a b"},
		{n("12.50"), "12.50"},
		{&types.AttributeValueMemberBOOL{Value: true}, "true"},
		{&types.AttributeValueMemberNULL{Value: true}, "null"},
		{&types.AttributeValueMemberB{Value: []byte("hi")}, "aGk="},
		{&types.AttributeValueMemberSS{Value: []string{"x", "y"}}, "[x, y]"},
		{&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{"b": n("2"), "a": s("<x>")}}, `{"a":"<x>","b":2}`},
		{&types.AttributeValueMemberL{Value: []types.AttributeValue{n("1"), s("z")}}, `[1,"z"]`},
	}
	for _, c := range cases {
		if got := Display(c.in); got != c.want {
			t.Errorf("Display(%T) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestItemJSONKeepsNumberPrecision(t *testing.T) {
	got := ItemJSON(Item{"big": n("12345678901234567890.123456789")})
	if !strings.Contains(got, "12345678901234567890.123456789") {
		t.Fatalf("precision lost: %s", got)
	}
}

func TestColumnsKeysFirst(t *testing.T) {
	items := []Item{{"z": s(""), "pk": s(""), "a": s("")}, {"sk": s(""), "m": s("")}}
	got := Columns(items, []string{"pk", "sk"})
	want := []string{"pk", "sk", "a", "m", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSortItems(t *testing.T) {
	items := []Item{
		{"id": s("a"), "v": n("10")},
		{"id": s("b")}, // no v: always last
		{"id": s("c"), "v": n("9")},
		{"id": s("d"), "v": n("-1.5")},
	}
	order := func() string {
		var b strings.Builder
		for _, it := range items {
			b.WriteString(Display(it["id"]))
		}
		return b.String()
	}
	SortItems(items, "v", false)
	if got := order(); got != "dcab" {
		t.Fatalf("asc: got %s, want dcab (numeric, missing last)", got)
	}
	SortItems(items, "v", true)
	if got := order(); got != "acdb" {
		t.Fatalf("desc: got %s, want acdb (missing still last)", got)
	}
}

func TestCompareMixedTypes(t *testing.T) {
	if Compare(n("100"), s("1")) >= 0 {
		t.Fatal("numbers sort before strings")
	}
	if Compare(s("b"), s("a")) <= 0 {
		t.Fatal("strings sort lexicographically")
	}
}

func TestGuessTimeColumn(t *testing.T) {
	cases := []struct {
		cols []string
		want string
	}{
		{[]string{"id", "name"}, ""},
		{[]string{"id", "Created_At", "updatedAt"}, "Created_At"},
		{[]string{"id", "updatedAt", "timestamp"}, "timestamp"},
		{[]string{"pk", "sk", "lastModified"}, "lastModified"},
		{[]string{"pk", "TS"}, "TS"},
		{[]string{"pk", "created-on"}, "created-on"},
	}
	for _, c := range cases {
		if got := GuessTimeColumn(c.cols); got != c.want {
			t.Errorf("%v: got %q, want %q", c.cols, got, c.want)
		}
	}
}
