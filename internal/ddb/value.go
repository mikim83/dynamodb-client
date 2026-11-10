package ddb

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Display renders a value on one line for a table cell.
func Display(av types.AttributeValue) string {
	switch v := av.(type) {
	case nil:
		return ""
	case *types.AttributeValueMemberS:
		return oneLine(v.Value)
	case *types.AttributeValueMemberN:
		return v.Value
	case *types.AttributeValueMemberBOOL:
		if v.Value {
			return "true"
		}
		return "false"
	case *types.AttributeValueMemberNULL:
		return "null"
	case *types.AttributeValueMemberB:
		return base64.StdEncoding.EncodeToString(v.Value)
	case *types.AttributeValueMemberSS:
		return "[" + strings.Join(v.Value, ", ") + "]"
	case *types.AttributeValueMemberNS:
		return "[" + strings.Join(v.Value, ", ") + "]"
	default: // BS, L, M
		return oneLine(compactJSON(ToJSON(av)))
	}
}

func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
}

func compactJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// ToJSON converts a value to plain JSON types. Numbers stay json.Number, so
// no precision is lost; binary values become base64 strings.
func ToJSON(av types.AttributeValue) any {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return json.Number(v.Value)
	case *types.AttributeValueMemberBOOL:
		return v.Value
	case *types.AttributeValueMemberNULL:
		return nil
	case *types.AttributeValueMemberB:
		return base64.StdEncoding.EncodeToString(v.Value)
	case *types.AttributeValueMemberSS:
		return v.Value
	case *types.AttributeValueMemberNS:
		out := make([]json.Number, len(v.Value))
		for i, n := range v.Value {
			out[i] = json.Number(n)
		}
		return out
	case *types.AttributeValueMemberBS:
		out := make([]string, len(v.Value))
		for i, b := range v.Value {
			out[i] = base64.StdEncoding.EncodeToString(b)
		}
		return out
	case *types.AttributeValueMemberL:
		out := make([]any, len(v.Value))
		for i, e := range v.Value {
			out[i] = ToJSON(e)
		}
		return out
	case *types.AttributeValueMemberM:
		out := make(map[string]any, len(v.Value))
		for k, e := range v.Value {
			out[k] = ToJSON(e)
		}
		return out
	default:
		return nil
	}
}

// ItemJSON renders an item as indented JSON with sorted keys.
func ItemJSON(item Item) string {
	out := make(map[string]any, len(item))
	for k, v := range item {
		out[k] = ToJSON(v)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err.Error()
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// Columns lists the attribute names found in items: the key attributes
// first, in the order given, then the rest alphabetically.
func Columns(items []Item, keys []string) []string {
	seen := map[string]bool{}
	cols := []string{}
	for _, k := range keys {
		seen[k] = true
		cols = append(cols, k)
	}
	var rest []string
	for _, it := range items {
		for name := range it {
			if !seen[name] {
				seen[name] = true
				rest = append(rest, name)
			}
		}
	}
	sort.Strings(rest)
	return append(cols, rest...)
}

// typeRank orders values of different types: numbers, strings, booleans,
// binary, then sets, lists and maps (compared by their text).
func typeRank(av types.AttributeValue) int {
	switch av.(type) {
	case *types.AttributeValueMemberN:
		return 0
	case *types.AttributeValueMemberS:
		return 1
	case *types.AttributeValueMemberBOOL:
		return 2
	case *types.AttributeValueMemberB:
		return 3
	case *types.AttributeValueMemberNULL:
		return 5
	default:
		return 4
	}
}

// Compare orders two values: numerically for numbers, by text otherwise.
func Compare(a, b types.AttributeValue) int {
	ra, rb := typeRank(a), typeRank(b)
	if ra != rb {
		return ra - rb
	}
	switch x := a.(type) {
	case *types.AttributeValueMemberN:
		fa, okA := new(big.Float).SetString(x.Value)
		fb, okB := new(big.Float).SetString(b.(*types.AttributeValueMemberN).Value)
		if okA && okB {
			return fa.Cmp(fb)
		}
	case *types.AttributeValueMemberBOOL:
		y := b.(*types.AttributeValueMemberBOOL)
		switch {
		case x.Value == y.Value:
			return 0
		case !x.Value:
			return -1
		default:
			return 1
		}
	case *types.AttributeValueMemberB:
		return bytes.Compare(x.Value, b.(*types.AttributeValueMemberB).Value)
	}
	return strings.Compare(Display(a), Display(b))
}

// SortItems sorts items in place by attribute col. Items without the
// attribute always go last, whatever the direction. The sort is stable.
func SortItems(items []Item, col string, desc bool) {
	sort.SliceStable(items, func(i, j int) bool {
		a, okA := items[i][col]
		b, okB := items[j][col]
		switch {
		case !okA && !okB:
			return false
		case !okA:
			return false
		case !okB:
			return true
		}
		c := Compare(a, b)
		if desc {
			return c > 0
		}
		return c < 0
	})
}
