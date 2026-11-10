package ddb

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// fakeAPI serves a table of n items keyed by "id" (N, 0..n-1). Like
// DynamoDB, a scan returns at most Limit items and at most perCall items
// (standing in for the 1 MB cap), and may return a LastEvaluatedKey even
// when nothing is left.
type fakeAPI struct {
	n       int
	perCall int
	scans   int
}

func (f *fakeAPI) ListTables(context.Context, *dynamodb.ListTablesInput, ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	return &dynamodb.ListTablesOutput{TableNames: []string{"t"}}, nil
}

func (f *fakeAPI) DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	return &dynamodb.DescribeTableOutput{Table: &types.TableDescription{
		TableName: aws.String("t"),
		ItemCount: aws.Int64(int64(f.n)),
		KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash}},
	}}, nil
}

func (f *fakeAPI) Scan(_ context.Context, in *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	f.scans++
	start := 0
	if k, ok := in.ExclusiveStartKey["id"].(*types.AttributeValueMemberN); ok {
		start, _ = strconv.Atoi(k.Value)
		start++
	}
	limit := f.n
	if in.Limit != nil {
		limit = int(*in.Limit)
	}
	if f.perCall > 0 {
		limit = min(limit, f.perCall)
	}
	end := min(start+limit, f.n)
	out := &dynamodb.ScanOutput{}
	for i := start; i < end; i++ {
		out.Items = append(out.Items, Item{"id": &types.AttributeValueMemberN{Value: strconv.Itoa(i)}})
	}
	// DynamoDB returns a key whenever it stopped because of Limit, even if
	// the table happens to end exactly there.
	if end > start && end-start == limit {
		out.LastEvaluatedKey = Item{"id": &types.AttributeValueMemberN{Value: strconv.Itoa(end - 1)}}
	}
	return out, nil
}
