package catalog

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"sync"
)

var (
	ErrChildLengthMismatch = errors.New("catalog: child length mismatch")
	ErrChildDigestMismatch = errors.New("catalog: child digest mismatch")
)

type CacheKey struct {
	SnapshotID SnapshotID
	Digest     [sha256.Size]byte
}

type CacheLimits struct {
	Entries uint64
	Bytes   uint64
}

func NewDetailCache() *ByteCache {
	return NewByteCache(CacheLimits{Entries: 128, Bytes: 64 << 20})
}

func NewSearchCache() *ByteCache {
	return NewByteCache(CacheLimits{Entries: 64, Bytes: 8 << 20})
}

// DecodedWeightV1 conservatively counts retained string payloads, slice
// capacities, and fixed DTO storage. Encoded bytes are added by ByteCache.
func DecodedWeightV1(stringBytes, sliceCapacityBytes, fixedDTOBytes uint64) (uint64, error) {
	if stringBytes > math.MaxUint64-sliceCapacityBytes || stringBytes+sliceCapacityBytes > math.MaxUint64-fixedDTOBytes {
		return 0, fmt.Errorf("catalog: decoded cache weight overflow")
	}
	return stringBytes + sliceCapacityBytes + fixedDTOBytes, nil
}

type CacheStats struct {
	Entries  uint64
	Bytes    uint64
	Hits     uint64
	Misses   uint64
	Bypassed uint64
	Waiters  uint64
}

type ChildLoader func(context.Context) ([]byte, error)
type ChildDecoder func([]byte) (value any, decodedWeight uint64, err error)

type ByteCache struct {
	mutex   sync.Mutex
	limits  CacheLimits
	entries map[CacheKey]*list.Element
	lru     list.List
	flights map[CacheKey]*cacheFlight
	bytes   uint64
	hits    uint64
	misses  uint64
	bypass  uint64
	waiters uint64
}

type cacheEntry struct {
	key    CacheKey
	value  any
	weight uint64
}

type cacheFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters uint64
	value   any
	err     error
}

func NewByteCache(limits CacheLimits) *ByteCache {
	return &ByteCache{limits: limits, entries: make(map[CacheKey]*list.Element), flights: make(map[CacheKey]*cacheFlight)}
}

func (cache *ByteCache) Load(ctx context.Context, key CacheKey, expectedLength uint64, loader ChildLoader, decoder ChildDecoder) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if key.SnapshotID == "" || expectedLength == 0 || loader == nil || decoder == nil {
		return nil, fmt.Errorf("catalog: cache load contract is incomplete")
	}
	cache.mutex.Lock()
	if element, exists := cache.entries[key]; exists {
		cache.hits++
		cache.lru.MoveToFront(element)
		value := element.Value.(*cacheEntry).value
		cache.mutex.Unlock()
		return value, nil
	}
	cache.misses++
	flight, exists := cache.flights[key]
	if !exists {
		flightContext, cancel := context.WithCancel(context.Background())
		flight = &cacheFlight{done: make(chan struct{}), cancel: cancel}
		cache.flights[key] = flight
		go cache.execute(flightContext, key, expectedLength, loader, decoder, flight)
	}
	flight.waiters++
	cache.waiters++
	cache.mutex.Unlock()

	select {
	case <-ctx.Done():
		cache.releaseWaiter(key, flight)
		return nil, ctx.Err()
	case <-flight.done:
		cache.releaseWaiter(key, flight)
		return flight.value, flight.err
	}
}

func (cache *ByteCache) execute(
	ctx context.Context,
	key CacheKey,
	expectedLength uint64,
	loader ChildLoader,
	decoder ChildDecoder,
	flight *cacheFlight,
) {
	data, err := loader(ctx)
	var value any
	var weight uint64
	if err == nil && uint64(len(data)) != expectedLength {
		err = fmt.Errorf("%w: got %d, want %d", ErrChildLengthMismatch, len(data), expectedLength)
	}
	if err == nil {
		digest := sha256.Sum256(data)
		if digest != key.Digest {
			err = ErrChildDigestMismatch
		}
	}
	if err == nil {
		var decodedWeight uint64
		value, decodedWeight, err = decoder(data)
		if err == nil {
			if decodedWeight > math.MaxUint64-uint64(len(data)) {
				err = fmt.Errorf("catalog: decoded cache weight overflow")
			} else {
				weight = uint64(len(data)) + decodedWeight
			}
		}
	}

	cache.mutex.Lock()
	current, ownsFlight := cache.flights[key]
	ownsFlight = ownsFlight && current == flight
	if ownsFlight {
		delete(cache.flights, key)
	}
	if err == nil && ownsFlight {
		if weight > cache.limits.Bytes || cache.limits.Entries == 0 || cache.limits.Bytes == 0 {
			cache.bypass++
		} else {
			cache.admit(&cacheEntry{key: key, value: value, weight: weight})
		}
	}
	flight.value = value
	flight.err = err
	flight.cancel()
	close(flight.done)
	cache.mutex.Unlock()
}

func (cache *ByteCache) admit(entry *cacheEntry) {
	if existing, exists := cache.entries[entry.key]; exists {
		prior := existing.Value.(*cacheEntry)
		cache.bytes -= prior.weight
		existing.Value = entry
		cache.bytes += entry.weight
		cache.lru.MoveToFront(existing)
	} else {
		element := cache.lru.PushFront(entry)
		cache.entries[entry.key] = element
		cache.bytes += entry.weight
	}
	for uint64(len(cache.entries)) > cache.limits.Entries || cache.bytes > cache.limits.Bytes {
		oldest := cache.lru.Back()
		if oldest == nil {
			break
		}
		cached := oldest.Value.(*cacheEntry)
		delete(cache.entries, cached.key)
		cache.bytes -= cached.weight
		cache.lru.Remove(oldest)
	}
}

func (cache *ByteCache) releaseWaiter(key CacheKey, flight *cacheFlight) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if cache.waiters > 0 {
		cache.waiters--
	}
	if flight.waiters > 0 {
		flight.waiters--
	}
	if flight.waiters == 0 {
		if current, exists := cache.flights[key]; exists && current == flight {
			delete(cache.flights, key)
			flight.cancel()
		}
	}
}

func (cache *ByteCache) EvictSnapshot(snapshot SnapshotID) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	for key, element := range cache.entries {
		if key.SnapshotID != snapshot {
			continue
		}
		entry := element.Value.(*cacheEntry)
		cache.bytes -= entry.weight
		cache.lru.Remove(element)
		delete(cache.entries, key)
	}
	for key, flight := range cache.flights {
		if key.SnapshotID == snapshot {
			delete(cache.flights, key)
			flight.cancel()
		}
	}
}

func (cache *ByteCache) Stats() CacheStats {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	return CacheStats{
		Entries: uint64(len(cache.entries)), Bytes: cache.bytes, Hits: cache.hits,
		Misses: cache.misses, Bypassed: cache.bypass, Waiters: cache.waiters,
	}
}
