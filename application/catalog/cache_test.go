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
		if _, err := cache.Load(context.Background(), input.key, input.length, 3, input.loader, input.decoder); err != nil {
			t.Fatal(err)
		}
	}
	// Touch A. C then evicts B by entry count while staying under byte weight.
	if _, err := cache.Load(context.Background(), aKey, 1, 3, aLoad, aDecode); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), cKey, 3, 3, cLoad, cDecode); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), bKey, 2, 3, bLoad, bDecode); err != nil {
		t.Fatal(err)
	}
	if loads["a"] != 1 || loads["bb"] != 2 || cache.Stats().Entries > 2 || cache.Stats().Bytes > 12 {
		t.Fatalf("loads/stats = %#v / %#v", loads, cache.Stats())
	}
}

func TestCacheOversizedEntryBypassesAdmission(t *testing.T) {
	t.Parallel()

	cache := NewByteCache(CacheLimits{Entries: 4, Bytes: 4, FlightEntries: 1, FlightBytes: 32})
	key, loader, decoder := cacheFixture("large", 20)
	for range 2 {
		if value, err := cache.Load(context.Background(), key, 5, 20, loader, decoder); err != nil || value != "large" {
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
	if _, err := cache.Load(context.Background(), key, 99, 0, func(context.Context) ([]byte, error) { return []byte("expected"), nil }, decoder); !errors.Is(err, ErrChildLengthMismatch) {
		t.Fatalf("length error = %v", err)
	}
	if _, err := cache.Load(context.Background(), key, 5, 0, func(context.Context) ([]byte, error) { return []byte("other"), nil }, decoder); !errors.Is(err, ErrChildDigestMismatch) {
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
		_, err := cache.Load(canceledContext, key, 6, 1, loader, decoder)
		first <- err
	}()
	<-started
	second := make(chan error, 1)
	go func() {
		value, err := cache.Load(context.Background(), key, 6, 1, loader, decoder)
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
		if _, err := cache.Load(context.Background(), key, uint64(len(data)), 1, loader, decoder); err != nil {
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
		_, err := cache.Load(context.Background(), key, 4, 1, func(context.Context) ([]byte, error) {
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
	if stats := cache.Stats(); stats.Entries != 0 || stats.InFlightEntries != 0 || stats.InFlightBytes != 0 {
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
			if _, err := cache.Load(context.Background(), key, uint64(len(data)), 2, loader, decoder); err != nil {
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

func TestCacheBoundsDistinctLoadersByEncodedAndDecodedReservation(t *testing.T) {
	const (
		flightCount  = 64
		encodedBytes = 1 << 20
		decodedBytes = 1 << 20
		flightBytes  = 4 << 20
	)
	cache := NewByteCache(CacheLimits{Entries: 2, Bytes: flightBytes, FlightEntries: flightCount, FlightBytes: flightBytes})
	release := make(chan struct{})
	var started atomic.Int32
	var active atomic.Int32
	var peakActive atomic.Int32
	var wait sync.WaitGroup
	errorsFound := make(chan error, flightCount)
	keys := make([]CacheKey, flightCount)
	for index := range flightCount {
		keys[index] = cacheKeyFixture(string(cacheMegabyteFixture(index)))
	}
	for index := range flightCount {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := cache.Load(context.Background(), keys[index], encodedBytes, decodedBytes, func(context.Context) ([]byte, error) {
				started.Add(1)
				current := active.Add(1)
				for previous := peakActive.Load(); current > previous && !peakActive.CompareAndSwap(previous, current); previous = peakActive.Load() {
				}
				<-release
				active.Add(-1)
				return cacheMegabyteFixture(index), nil
			}, func([]byte) (any, uint64, error) {
				return index, decodedBytes, nil
			})
			errorsFound <- err
		}()
	}

	waitForCacheCondition(t, time.Second, func() bool { return started.Load() == 2 })
	time.Sleep(25 * time.Millisecond)
	stats := cache.Stats()
	if started.Load() != 2 || active.Load() != 2 || stats.InFlightEntries != 2 || stats.InFlightBytes != flightBytes {
		t.Fatalf("initial admission started=%d active=%d stats=%#v", started.Load(), active.Load(), stats)
	}
	for completed := 0; completed < flightCount; completed++ {
		release <- struct{}{}
		if completed < flightCount-2 {
			wantStarted := int32(completed + 3)
			waitForCacheCondition(t, time.Second, func() bool { return started.Load() == wantStarted })
		}
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	stats = cache.Stats()
	if started.Load() != flightCount || peakActive.Load() != 2 || stats.InFlightEntries != 0 || stats.InFlightBytes != 0 || stats.PeakInFlightEntries != 2 || stats.PeakInFlightBytes != flightBytes {
		t.Fatalf("bounded flight receipt started=%d peak-active=%d stats=%#v", started.Load(), peakActive.Load(), stats)
	}
}

func TestCacheReleasesReservationForEveryFlightOutcome(t *testing.T) {
	testCases := []struct {
		name    string
		loader  ChildLoader
		decoder ChildDecoder
		wantErr error
	}{
		{name: "success", loader: func(context.Context) ([]byte, error) { return []byte("x"), nil }, decoder: func(data []byte) (any, uint64, error) { return string(data), 3, nil }},
		{name: "loader error", loader: func(context.Context) ([]byte, error) { return nil, errors.New("load failed") }, decoder: func([]byte) (any, uint64, error) { return nil, 0, nil }, wantErr: errors.New("load failed")},
		{name: "decoder error", loader: func(context.Context) ([]byte, error) { return []byte("x"), nil }, decoder: func([]byte) (any, uint64, error) { return nil, 0, errors.New("decode failed") }, wantErr: errors.New("decode failed")},
		{name: "loader panic", loader: func(context.Context) ([]byte, error) { panic("loader panic") }, decoder: func([]byte) (any, uint64, error) { return nil, 0, nil }, wantErr: ErrCacheFlightPanic},
		{name: "decoder panic", loader: func(context.Context) ([]byte, error) { return []byte("x"), nil }, decoder: func([]byte) (any, uint64, error) { panic("decoder panic") }, wantErr: ErrCacheFlightPanic},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cache := NewByteCache(CacheLimits{Entries: 1, Bytes: 4})
			_, err := cache.Load(context.Background(), cacheKeyFixture("x"), 1, 3, testCase.loader, testCase.decoder)
			if testCase.wantErr == nil && err != nil {
				t.Fatal(err)
			}
			if testCase.wantErr != nil && (err == nil || !errors.Is(err, testCase.wantErr) && err.Error() != testCase.wantErr.Error()) {
				t.Fatalf("flight error = %v, want %v", err, testCase.wantErr)
			}
			if stats := cache.Stats(); stats.InFlightEntries != 0 || stats.InFlightBytes != 0 {
				t.Fatalf("flight reservation leaked after %s: %#v", testCase.name, stats)
			}
			key, loader, decoder := cacheFixture("y", 3)
			if _, err := cache.Load(context.Background(), key, 1, 3, loader, decoder); err != nil {
				t.Fatalf("follow-up load after %s: %v", testCase.name, err)
			}
		})
	}
}

func TestCacheCanceledAdmissionWaiterNeverStartsLoader(t *testing.T) {
	cache := NewByteCache(CacheLimits{Entries: 1, Bytes: 4})
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		_, err := cache.Load(context.Background(), cacheKeyFixture("a"), 1, 3, func(context.Context) ([]byte, error) {
			close(firstStarted)
			<-firstRelease
			return []byte("a"), nil
		}, func(data []byte) (any, uint64, error) { return string(data), 3, nil })
		firstResult <- err
	}()
	<-firstStarted

	secondContext, cancelSecond := context.WithCancel(context.Background())
	var secondStarted atomic.Bool
	secondResult := make(chan error, 1)
	go func() {
		_, err := cache.Load(secondContext, cacheKeyFixture("b"), 1, 3, func(context.Context) ([]byte, error) {
			secondStarted.Store(true)
			return []byte("b"), nil
		}, func(data []byte) (any, uint64, error) { return string(data), 3, nil })
		secondResult <- err
	}()
	waitForCacheCondition(t, time.Second, func() bool { return cache.Stats().Misses == 2 })
	if secondStarted.Load() {
		t.Fatal("admission waiter started while flight budget was full")
	}
	cancelSecond()
	if err := <-secondResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission waiter error = %v", err)
	}
	close(firstRelease)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}
	if secondStarted.Load() {
		t.Fatal("canceled admission waiter started later")
	}
	if stats := cache.Stats(); stats.InFlightEntries != 0 || stats.InFlightBytes != 0 {
		t.Fatalf("reservation after cancellation = %#v", stats)
	}
}

func TestCacheCanceledOnlyWaiterReleasesActiveReservation(t *testing.T) {
	cache := NewByteCache(CacheLimits{Entries: 1, Bytes: 4})
	loadContext, cancelLoad := context.WithCancel(context.Background())
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := cache.Load(loadContext, cacheKeyFixture("x"), 1, 3, func(ctx context.Context) ([]byte, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}, func([]byte) (any, uint64, error) { return nil, 0, nil })
		result <- err
	}()
	<-started
	cancelLoad()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled only waiter error = %v", err)
	}
	waitForCacheCondition(t, time.Second, func() bool {
		stats := cache.Stats()
		return stats.InFlightEntries == 0 && stats.InFlightBytes == 0
	})
	key, loader, decoder := cacheFixture("y", 3)
	if _, err := cache.Load(context.Background(), key, 1, 3, loader, decoder); err != nil {
		t.Fatalf("follow-up after active cancellation: %v", err)
	}
}

func TestCacheFailedFlightWakesBlockedAdmission(t *testing.T) {
	cache := NewByteCache(CacheLimits{Entries: 1, Bytes: 4})
	failedStarted := make(chan struct{})
	fail := make(chan struct{})
	failedResult := make(chan error, 1)
	go func() {
		_, err := cache.Load(context.Background(), cacheKeyFixture("x"), 1, 3, func(context.Context) ([]byte, error) {
			close(failedStarted)
			<-fail
			return nil, errors.New("failed")
		}, func([]byte) (any, uint64, error) { return nil, 0, nil })
		failedResult <- err
	}()
	<-failedStarted

	blockedStarted := make(chan struct{})
	blockedResult := make(chan error, 1)
	go func() {
		_, err := cache.Load(context.Background(), cacheKeyFixture("y"), 1, 3, func(context.Context) ([]byte, error) {
			close(blockedStarted)
			return []byte("y"), nil
		}, func(data []byte) (any, uint64, error) { return string(data), 3, nil })
		blockedResult <- err
	}()
	waitForCacheCondition(t, time.Second, func() bool { return cache.Stats().Misses == 2 })
	select {
	case <-blockedStarted:
		t.Fatal("blocked loader started before failed flight released reservation")
	default:
	}
	close(fail)
	if err := <-failedResult; err == nil || err.Error() != "failed" {
		t.Fatalf("failed flight error = %v", err)
	}
	select {
	case <-blockedStarted:
	case <-time.After(time.Second):
		t.Fatal("blocked admission deadlocked after failed flight")
	}
	if err := <-blockedResult; err != nil {
		t.Fatal(err)
	}
}

func TestCacheRejectsFlightAndDecodedWeightsOutsideReservation(t *testing.T) {
	cache := NewByteCache(CacheLimits{Entries: 1, Bytes: 4})
	loaderStarted := false
	if _, err := cache.Load(context.Background(), cacheKeyFixture("x"), 1, 4, func(context.Context) ([]byte, error) {
		loaderStarted = true
		return []byte("x"), nil
	}, func(data []byte) (any, uint64, error) { return string(data), 4, nil }); !errors.Is(err, ErrCacheFlightTooLarge) {
		t.Fatalf("oversized flight error = %v", err)
	}
	if loaderStarted {
		t.Fatal("oversized flight started loader")
	}

	value, err := cache.Load(context.Background(), cacheKeyFixture("y"), 1, 1, func(context.Context) ([]byte, error) {
		return []byte("y"), nil
	}, func(data []byte) (any, uint64, error) { return string(data), 2, nil })
	if !errors.Is(err, ErrChildDecodedWeightExceeded) {
		t.Fatalf("decoded reservation error = %v", err)
	}
	if value != nil {
		t.Fatalf("rejected decoded value remained reachable after reservation release: %v", value)
	}
	if stats := cache.Stats(); stats.Entries != 0 || stats.InFlightEntries != 0 || stats.InFlightBytes != 0 {
		t.Fatalf("rejected reservation stats = %#v", stats)
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

func cacheMegabyteFixture(index int) []byte {
	data := make([]byte, 1<<20)
	data[0] = byte(index)
	return data
}

func waitForCacheCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		t.Fatal("timed out waiting for cache condition")
	}
}
