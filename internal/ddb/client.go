// Package ddb wraps the read-only DynamoDB calls the client needs: listing
// tables, describing them and scanning them page by page.
package ddb

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// API is the subset of the DynamoDB client used here. Every call is a read.
type API interface {
	ListTables(ctx context.Context, in *dynamodb.ListTablesInput, opts ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
	DescribeTable(ctx context.Context, in *dynamodb.DescribeTableInput, opts ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	Scan(ctx context.Context, in *dynamodb.ScanInput, opts ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

// Item is one DynamoDB item.
type Item = map[string]types.AttributeValue

// TableInfo is what the UI shows about a table.
type TableInfo struct {
	Name      string
	HashKey   string
	RangeKey  string
	ItemCount int64 // approximate: DynamoDB refreshes it about every six hours
	SizeBytes int64
	Status    string
}

// Keys returns the key attribute names, partition key first.
func (t TableInfo) Keys() []string {
	keys := []string{}
	if t.HashKey != "" {
		keys = append(keys, t.HashKey)
	}
	if t.RangeKey != "" {
		keys = append(keys, t.RangeKey)
	}
	return keys
}

// ListTables returns every table name in the account and region.
func ListTables(ctx context.Context, api API) ([]string, error) {
	var names []string
	p := dynamodb.NewListTablesPaginator(api, &dynamodb.ListTablesInput{})
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		names = append(names, out.TableNames...)
	}
	return names, nil
}

// Describe returns the key schema and size of a table.
func Describe(ctx context.Context, api API, table string) (TableInfo, error) {
	out, err := api.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})
	if err != nil {
		return TableInfo{}, err
	}
	t := out.Table
	info := TableInfo{
		Name:      aws.ToString(t.TableName),
		ItemCount: aws.ToInt64(t.ItemCount),
		SizeBytes: aws.ToInt64(t.TableSizeBytes),
		Status:    string(t.TableStatus),
	}
	for _, k := range t.KeySchema {
		switch k.KeyType {
		case types.KeyTypeHash:
			info.HashKey = aws.ToString(k.AttributeName)
		case types.KeyTypeRange:
			info.RangeKey = aws.ToString(k.AttributeName)
		}
	}
	return info, nil
}

// Page is one page of a scan.
type Page struct {
	Index   int
	Items   []Item
	HasNext bool
}

// Pager scans a table in fixed-size pages. DynamoDB only moves forward
// (ExclusiveStartKey), so the pager remembers the start key of every page it
// has reached and caches the pages already read: going back costs nothing.
type Pager struct {
	api    API
	table  string
	size   int
	mu     sync.Mutex
	starts []Item // starts[i] is the ExclusiveStartKey of page i (nil for page 0)
	cache  map[int]Page
}

// NewPager returns a pager over table with size items per page.
func NewPager(api API, table string, size int) *Pager {
	p := &Pager{api: api, table: table, size: size}
	p.Reset()
	return p
}

// Size is the page size.
func (p *Pager) Size() int { return p.size }

// Reset forgets every page read so far.
func (p *Pager) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts = []Item{nil}
	p.cache = map[int]Page{}
}

// Page returns page i. Pages are reached in order: page i+1 can be read once
// page i has been.
func (p *Pager) Page(ctx context.Context, i int) (Page, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pg, ok := p.cache[i]; ok {
		return pg, nil
	}
	if i < 0 || i >= len(p.starts) {
		return Page{}, fmt.Errorf("page %d has not been reached yet", i+1)
	}
	// Limit caps the items evaluated per call and a call also stops at 1 MB,
	// so keep asking until the page is full or the table ends.
	var items []Item
	key := p.starts[i]
	for len(items) < p.size {
		out, err := p.api.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(p.table),
			Limit:             aws.Int32(int32(p.size - len(items))),
			ExclusiveStartKey: key,
		})
		if err != nil {
			return Page{}, err
		}
		items = append(items, out.Items...)
		key = out.LastEvaluatedKey
		if len(key) == 0 {
			break
		}
	}
	pg := Page{Index: i, Items: items, HasNext: len(key) > 0}
	if pg.HasNext && len(p.starts) == i+1 {
		p.starts = append(p.starts, key)
	}
	p.cache[i] = pg
	return pg, nil
}

// ScanAll reads the whole table, stopping at max items. truncated reports
// whether the table has more items than were returned.
func ScanAll(ctx context.Context, api API, table string, max int) (items []Item, truncated bool, err error) {
	var key Item
	for {
		out, err := api.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(table),
			Limit:             aws.Int32(int32(min(max-len(items)+1, 1000))),
			ExclusiveStartKey: key,
		})
		if err != nil {
			return nil, false, err
		}
		items = append(items, out.Items...)
		key = out.LastEvaluatedKey
		if len(items) > max {
			return items[:max], true, nil
		}
		if len(key) == 0 {
			return items, false, nil
		}
	}
}
