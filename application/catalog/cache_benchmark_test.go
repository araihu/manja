package catalog

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

func BenchmarkCatalogCache(b *testing.B) {
	cache := NewDetailCache()
	values := make([][]byte, 256)
	keys := make([]CacheKey, len(values))
	for index := range values {
		values[index] = []byte(fmt.Sprintf("bounded-catalog-shard-%03d", index))
		keys[index] = CacheKey{SnapshotID: SnapshotID("snapshot-sha256-" + repeatRuntimeHex("d")), Digest: sha256.Sum256(values[index])}
	}
	decoder := func(data []byte) (any, uint64, error) {
		return string(data), uint64(len(data)), nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		selected := index % len(values)
		_, err := cache.Load(context.Background(), keys[selected], uint64(len(values[selected])), func(context.Context) ([]byte, error) {
			return values[selected], nil
		}, decoder)
		if err != nil {
			b.Fatal(err)
		}
	}
	stats := cache.Stats()
	if stats.Entries > 128 || stats.Bytes > 64<<20 {
		b.Fatalf("cache exceeded bounds: %#v", stats)
	}
}
