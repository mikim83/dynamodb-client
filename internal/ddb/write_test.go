package ddb

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeDeleter struct {
	got []*dynamodb.DeleteItemInput
	err error
}

func (f *fakeDeleter) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.got = append(f.got, in)
	return &dynamodb.DeleteItemOutput{}, f.err
}

var orders = TableInfo{Name: "orders", HashKey: "pk", RangeKey: "sk"}

func order(pk, sk string) Item {
	return Item{
		"pk":    &types.AttributeValueMemberS{Value: pk},
		"sk":    &types.AttributeValueMemberN{Value: sk},
		"total": &types.AttributeValueMemberN{Value: "10"},
	}
}

func TestDeleteItemSendsKeyAndCondition(t *testing.T) {
	d := &fakeDeleter{}
	if err := DeleteItem(context.Background(), d, orders, order("a", "1")); err != nil {
		t.Fatal(err)
	}
	if len(d.got) != 1 {
		t.Fatalf("calls: %d", len(d.got))
	}
	in := d.got[0]
	if aws.ToString(in.TableName) != "orders" || len(in.Key) != 2 || in.Key["total"] != nil {
		t.Fatalf("key should be pk+sk only: %v", in.Key)
	}
	if aws.ToString(in.ConditionExpression) != "attribute_exists(#pk)" || in.ExpressionAttributeNames["#pk"] != "pk" {
		t.Fatalf("condition: %q %v", aws.ToString(in.ConditionExpression), in.ExpressionAttributeNames)
	}
}

func TestDeleteItemGone(t *testing.T) {
	d := &fakeDeleter{err: &types.ConditionalCheckFailedException{}}
	if err := DeleteItem(context.Background(), d, orders, order("a", "1")); !errors.Is(err, ErrGone) {
		t.Fatalf("want ErrGone, got %v", err)
	}
}

func TestDeleteItemMissingKey(t *testing.T) {
	d := &fakeDeleter{}
	item := order("a", "1")
	delete(item, "sk")
	if err := DeleteItem(context.Background(), d, orders, item); err == nil || len(d.got) != 0 {
		t.Fatalf("want an error and no call, got %v and %d calls", err, len(d.got))
	}
}

func TestSameKey(t *testing.T) {
	if !SameKey(orders, order("a", "1"), order("a", "1.0")) {
		t.Error("1 and 1.0 are the same number")
	}
	if SameKey(orders, order("a", "1"), order("a", "2")) {
		t.Error("different sort keys")
	}
	if SameKey(orders, order("a", "1"), Item{"pk": &types.AttributeValueMemberS{Value: "a"}}) {
		t.Error("missing sort key")
	}
}

func TestPagerRemoveKeepsNextPages(t *testing.T) {
	ctx := context.Background()
	p := NewPager(&fakeAPI{n: 120}, "t", 50)
	if _, err := p.Page(ctx, 0); err != nil {
		t.Fatal(err)
	}
	info := TableInfo{Name: "t", HashKey: "id"}
	p.Remove(0, info, Item{"id": &types.AttributeValueMemberN{Value: "3"}})

	pg, _ := p.Page(ctx, 0)
	if len(pg.Items) != 49 {
		t.Fatalf("page 1 should have 49 items, has %d", len(pg.Items))
	}
	for _, it := range pg.Items {
		if Display(it["id"]) == "3" {
			t.Fatal("id 3 still cached")
		}
	}
	pg, _ = p.Page(ctx, 1)
	if got := Display(pg.Items[0]["id"]); got != "50" {
		t.Fatalf("page 2 should still start at 50, starts at %s", got)
	}
}
