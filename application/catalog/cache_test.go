package catalog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheEvictsByEntryCountAndByteWeightInLRUOrder(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 2, Bytes: 12})
	loads := make(map[string]int)
	load := func(value string, decoded uint64) (CacheKey, ChildLoader, ChildDecoder) {
		bytes := []byte(value)
		key := cacheKeyFixture(value)
		return key, func(context.Context) ([]byte, error) {
			loads[value]++
			return bytes, nil
		}, func(data []byte) (any, uint64, error) { return string(data), decoded, nil }
	}
	aKey, aLoad, aDecode := load("a", 3)
	bKey, bLoad, bDecode := load("bb", 3)
	cKey, cLoad, cDecode := load("ccc", 3)
	for _, input := range []struct {
		key     CacheKey
		length  uint64
		loader  ChildLoader
		decoder ChildDecoder
	}{{aKey, 1, aLoad, aDecode}, {bKey, 2, bLoad, bDecode}} {
		if _, err := cache.Load(context.Background(), input.key, input.length, input.loader, input.decoder); err != nil {
			t.Fatal(err)
		}
	}
	// Touch A. C then evicts B by entry count while staying under byte weight.
	if _, err := cache.Load(context.Background(), aKey, 1, aLoad, aDecode); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), cKey, 3, cLoad, cDecode); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), bKey, 2, bLoad, bDecode); err != nil {
		t.Fatal(err)
	}
	if loads["a"] != 1 || loads["bb"] != 2 || cache.Stats().Entries > 2 || cache.Stats().Bytes > 12 {
		t.Fatalf("loads/stats = %#v / %#v", loads, cache.Stats())
	}
}

func TestCacheOversizedEntryBypassesAdmission(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 4, Bytes: 4})
	key, loader, decoder := cacheFixture("large", 20)
	for range 2 {
		if value, err := cache.Load(context.Background(), key, 5, loader, decoder); err != nil || value != "large" {
			t.Fatalf("oversized load = %v, %v", value, err)
		}
	}
	if cache.Stats().Entries != 0 || cache.Stats().Bypassed != 2 {
		t.Fatalf("oversized stats = %#v", cache.Stats())
	}
}

func TestCacheRejectsLengthAndDigestBeforeDecode(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 4, Bytes: 1024})
	key := cacheKeyFixture("expected")
	decoded := false
	decoder := func([]byte) (any, uint64, error) { decoded = true; return nil, 0, nil }
	if _, err := cache.Load(context.Background(), key, 99, func(context.Context) ([]byte, error) { return []byte("expected"), nil }, decoder); !errors.Is(err, ErrChildLengthMismatch) {
		t.Fatalf("length error = %v", err)
	}
	if _, err := cache.Load(context.Background(), key, 5, func(context.Context) ([]byte, error) { return []byte("other"), nil }, decoder); !errors.Is(err, ErrChildDigestMismatch) {
		t.Fatalf("digest error = %v", err)
	}
	if decoded {
		t.Fatal("decoder ran for unverified bytes")
	}
}

func TestCacheSingleflightAndCanceledWaiterDoesNotCancelObserver(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 4, Bytes: 1024})
	key := cacheKeyFixture("shared")
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	loader := func(ctx context.Context) ([]byte, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return []byte("shared"), nil
		}
	}
	decoder := func(data []byte) (any, uint64, error) { return string(data), 1, nil }
	canceledContext, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := cache.Load(canceledContext, key, 6, loader, decoder)
		first <- err
	}()
	<-started
	second := make(chan error, 1)
	go func() {
		value, err := cache.Load(context.Background(), key, 6, loader, decoder)
		if err == nil && value != "shared" {
			err = fmt.Errorf("value = %v", value)
		}
		second <- err
	}()
	for deadline := time.Now().Add(time.Second); cache.Stats().Waiters < 2 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || cache.Stats().Entries != 1 {
		t.Fatalf("singleflight calls/stats = %d / %#v", calls.Load(), cache.Stats())
	}
}

func TestCacheEvictsCompleteSnapshot(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 8, Bytes: 1024})
	first := SnapshotID("snapshot-sha256-" + repeatRuntimeHex("a"))
	second := SnapshotID("snapshot-sha256-" + repeatRuntimeHex("b"))
	for index, snapshot := range []SnapshotID{first, first, second} {
		data := fmt.Sprintf("entry-%d", index)
		key, loader, decoder := cacheFixture(data, 1)
		key.SnapshotID = snapshot
		if _, err := cache.Load(context.Background(), key, uint64(len(data)), loader, decoder); err != nil {
			t.Fatal(err)
		}
	}
	cache.EvictSnapshot(first)
	if stats := cache.Stats(); stats.Entries != 1 {
		t.Fatalf("snapshot eviction stats = %#v", stats)
	}
}

func TestCacheEvictionPreventsInFlightReadmission(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 8, Bytes: 1024})
	key := cacheKeyFixture("late")
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := cache.Load(context.Background(), key, 4, func(context.Context) ([]byte, error) {
			close(started)
			<-release
			return []byte("late"), nil
		}, func(data []byte) (any, uint64, error) { return string(data), 1, nil })
		result <- err
	}()
	<-started
	cache.EvictSnapshot(key.SnapshotID)
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if cache.Stats().Entries != 0 {
		t.Fatalf("evicted flight repopulated cache: %#v", cache.Stats())
	}
}

func TestCacheConcurrentLoadsRemainWithinLimits(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 8, Bytes: 80})
	var wait sync.WaitGroup
	for index := range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			data := fmt.Sprintf("value-%02d", index)
			key, loader, decoder := cacheFixture(data, 2)
			if _, err := cache.Load(context.Background(), key, uint64(len(data)), loader, decoder); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
	stats := cache.Stats()
	if stats.Entries > 8 || stats.Bytes > 80 {
		t.Fatalf("concurrent cache stats = %#v", stats)
	}
}

func TestDecodedWeightV1RejectsOverflow(t *testing.T) {
	t.Parallel()
	if value, err := DecodedWeightV1(10, 20, 30); err != nil || value != 60 {
		t.Fatalf("weight = %d, %v", value, err)
	}
	if _, err := DecodedWeightV1(^uint64(0), 1, 0); err == nil {
		t.Fatal("overflow was accepted")
	}
}

func cacheKeyFixture(value string) CacheKey {
	digest := sha256.Sum256([]byte(value))
	return CacheKey{SnapshotID: SnapshotID("snapshot-sha256-" + repeatRuntimeHex("c")), Digest: digest}
}

func cacheFixture(value string, decodedWeight uint64) (CacheKey, ChildLoader, ChildDecoder) {
	key := cacheKeyFixture(value)
	return key,
		func(context.Context) ([]byte, error) { return []byte(value), nil },
		func(data []byte) (any, uint64, error) { return string(data), decodedWeight, nil }
}
