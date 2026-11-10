package ddb

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Deleter is the only write the client can make. The UI gets one only when
// started with --edit; otherwise it holds just the read-only API.
type Deleter interface {
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// ErrGone means the item was already deleted by someone else.
var ErrGone = errors.New("the item no longer exists")

// KeyOf returns the primary key of item.
func KeyOf(info TableInfo, item Item) (Item, error) {
	key := Item{}
	for _, k := range info.Keys() {
		v, ok := item[k]
		if !ok {
			return nil, fmt.Errorf("item has no key attribute %q", k)
		}
		key[k] = v
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("table %s has no key schema", info.Name)
	}
	return key, nil
}

// SameKey reports whether a and b have the same primary key.
func SameKey(info TableInfo, a, b Item) bool {
	for _, k := range info.Keys() {
		if a[k] == nil || b[k] == nil || Compare(a[k], b[k]) != 0 {
			return false
		}
	}
	return true
}

// Without returns items minus the one with item's primary key.
func Without(items []Item, info TableInfo, item Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if !SameKey(info, it, item) {
			out = append(out, it)
		}
	}
	return out
}

// DeleteItem deletes item by its primary key. The delete is conditional on
// the item still existing, so a row deleted meanwhile reports ErrGone
// instead of a silent success.
func DeleteItem(ctx context.Context, d Deleter, info TableInfo, item Item) error {
	key, err := KeyOf(info, item)
	if err != nil {
		return err
	}
	_, err = d.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:                aws.String(info.Name),
		Key:                      key,
		ConditionExpression:      aws.String("attribute_exists(#pk)"),
		ExpressionAttributeNames: map[string]string{"#pk": info.HashKey},
	})
	var cond *types.ConditionalCheckFailedException
	if errors.As(err, &cond) {
		return ErrGone
	}
	return err
}
