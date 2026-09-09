// Package schemacache provides byte-budgeted decoded schema reuse for exports.
package schemacache

import (
	"sync"

	"github.com/araihu/manja/internal/localdocs"
	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type entry struct {
	shard localdocs.PreparedSchemaNodeShard
	cost  uint64
}
type Stats struct {
	Hits          uint64 `json:"hits"`
	Misses        uint64 `json:"misses"`
	Evictions     uint64 `json:"evictions"`
	Oversized     uint64 `json:"oversized"`
	RetainedBytes uint64 `json:"retainedBytes"`
	PeakBytes     uint64 `json:"peakBytes"`
}

type Cache struct {
	mu     sync.Mutex
	lru    *simplelru.LRU[string, entry]
	budget uint64
	stats  Stats
}

func New(bytes uint64) *Cache {
	c := &Cache{budget: bytes}
	// Every entry is charged at least 512 bytes. A fixed count ceiling also
	// bounds map/list metadata independently of decoded object estimates.
	count := bytes / 512
	if count < 1 {
		count = 1
	}
	if count > uint64(^uint(0)>>1) {
		count = uint64(^uint(0) >> 1)
	}
	c.lru, _ = simplelru.NewLRU[string, entry](int(count), func(_ string, e entry) {
		c.stats.RetainedBytes -= e.cost
		c.stats.Evictions++
	})
	return c
}

func (c *Cache) Get(key string) (localdocs.PreparedSchemaNodeShard, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.lru.Get(key)
	if ok {
		c.stats.Hits++
	} else {
		c.stats.Misses++
	}
	return e.shard, ok
}

func (c *Cache) Add(key string, shard localdocs.PreparedSchemaNodeShard) {
	cost := shard.RetainedBytes() + 2*uint64(len(key)) + 512
	c.mu.Lock()
	defer c.mu.Unlock()
	if cost > c.budget {
		c.stats.Oversized++
		return
	}
	// Concurrent misses may prepare the same immutable shard. Keep one copy.
	if _, exists := c.lru.Get(key); exists {
		return
	}
	for c.stats.RetainedBytes > c.budget-cost {
		c.lru.RemoveOldest()
	}
	c.stats.RetainedBytes += cost
	c.lru.Add(key, entry{shard: shard, cost: cost})
	if c.stats.RetainedBytes > c.stats.PeakBytes {
		c.stats.PeakBytes = c.stats.RetainedBytes
	}
}

func (c *Cache) Stats() Stats { c.mu.Lock(); defer c.mu.Unlock(); return c.stats }
