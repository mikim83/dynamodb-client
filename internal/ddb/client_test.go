package ddb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func ids(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["id"].(*types.AttributeValueMemberN).Value
	}
	return out
}

func TestPagerFillsPagesAcrossShortScans(t *testing.T) {
	// 120 items, pages of 50, but each Scan call only returns 20 (1 MB cap).
	api := &fakeAPI{n: 120, perCall: 20}
	p := NewPager(api, "t", 50)
	ctx := context.Background()

	want := []struct {
		page, n int
		next    bool
		first   string
	}{
		{0, 50, true, "0"},
		{1, 50, true, "50"},
		{2, 20, false, "100"},
	}
	for _, w := range want {
		pg, err := p.Page(ctx, w.page)
		if err != nil {
			t.Fatalf("page %d: %v", w.page, err)
		}
		if len(pg.Items) != w.n || pg.HasNext != w.next || ids(pg.Items)[0] != w.first {
			t.Fatalf("page %d: got %d items, next=%v, first=%s", w.page, len(pg.Items), pg.HasNext, ids(pg.Items)[0])
		}
	}
}

func TestPagerCachesAndGoesBack(t *testing.T) {
	api := &fakeAPI{n: 120}
	p := NewPager(api, "t", 50)
	ctx := context.Background()
	for _, i := range []int{0, 1, 2} {
		if _, err := p.Page(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	before := api.scans
	pg, err := p.Page(ctx, 0)
	if err != nil || ids(pg.Items)[0] != "0" {
		t.Fatalf("back to page 0: %v %v", err, ids(pg.Items))
	}
	if api.scans != before {
		t.Fatalf("going back should not scan again (%d → %d calls)", before, api.scans)
	}
}

func TestPagerCannotSkipAhead(t *testing.T) {
	p := NewPager(&fakeAPI{n: 120}, "t", 50)
	if _, err := p.Page(context.Background(), 2); err == nil {
		t.Fatal("expected an error reading page 3 before page 2")
	}
}

func TestPagerExactMultipleEndsWithEmptyPage(t *testing.T) {
	// 100 items in pages of 50: DynamoDB still returns a key after item 99,
	// so page 2 says HasNext and page 3 comes back empty.
	p := NewPager(&fakeAPI{n: 100}, "t", 50)
	ctx := context.Background()
	if _, err := p.Page(ctx, 0); err != nil {
		t.Fatal(err)
	}
	pg, err := p.Page(ctx, 1)
	if err != nil || !pg.HasNext {
		t.Fatal("page 2 should report more (the key DynamoDB returns)")
	}
	pg, err = p.Page(ctx, 2)
	if err != nil || len(pg.Items) != 0 || pg.HasNext {
		t.Fatalf("page 3: err=%v items=%d next=%v", err, len(pg.Items), pg.HasNext)
	}
}

func TestScanAll(t *testing.T) {
	items, truncated, err := ScanAll(context.Background(), &fakeAPI{n: 120, perCall: 30}, "t", 1000)
	if err != nil || len(items) != 120 || truncated {
		t.Fatalf("got %d items, truncated=%v, err=%v", len(items), truncated, err)
	}
	items, truncated, err = ScanAll(context.Background(), &fakeAPI{n: 120, perCall: 30}, "t", 100)
	if err != nil || len(items) != 100 || !truncated {
		t.Fatalf("capped: got %d items, truncated=%v, err=%v", len(items), truncated, err)
	}
	_, truncated, _ = ScanAll(context.Background(), &fakeAPI{n: 100}, "t", 100)
	if truncated {
		t.Fatal("a table of exactly max items is not truncated")
	}
}

func TestDescribe(t *testing.T) {
	info, err := Describe(context.Background(), &fakeAPI{n: 7}, "t")
	if err != nil || info.HashKey != "id" || info.ItemCount != 7 || len(info.Keys()) != 1 {
		t.Fatalf("got %+v, %v", info, err)
	}
}
