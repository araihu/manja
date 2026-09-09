package schemacache

import (
	"fmt"
	"github.com/araihu/manja/internal/localdocs"
	"sync"
	"testing"
)

func TestBudgetEvictionAndDisabledRetention(t *testing.T) {
	shard := localdocs.PreparedSchemaNodeShard{}
	cost := shard.RetainedBytes() + 2 + 512
	c := New(2 * cost)
	c.Add("a", shard)
	c.Add("b", shard)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("missing a")
	}
	c.Add("c", shard)
	if _, ok := c.Get("b"); ok {
		t.Fatal("least recently used entry survived")
	}
	c.Add("a", shard)
	if s := c.Stats(); s.RetainedBytes != 2*cost || s.PeakBytes > 2*cost || s.Evictions != 1 {
		t.Fatal(s)
	}
	for _, budget := range []uint64{0, cost - 1} {
		c := New(budget)
		c.Add("a", shard)
		if _, ok := c.Get("a"); ok {
			t.Fatal("oversized entry retained")
		}
		if c.Stats().RetainedBytes != 0 {
			t.Fatal(c.Stats())
		}
	}
}

func TestConcurrentRetentionStaysWithinBudget(t *testing.T) {
	c := New(4096)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Go(func() {
			for i := 0; i < 1000; i++ {
				k := fmt.Sprint(i % 31)
				c.Add(k, localdocs.PreparedSchemaNodeShard{})
				c.Get(k)
			}
		})
	}
	wg.Wait()
	if s := c.Stats(); s.PeakBytes > 4096 || s.Evictions == 0 {
		t.Fatal(s)
	}
}
