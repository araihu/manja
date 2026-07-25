package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type syncContextKey struct{}

func TestSyncWritesBlobBeforeAtomicOperationalState(t *testing.T) {
	ctx := context.WithValue(context.Background(), syncContextKey{}, "sync")
	events := []string{}
	source := &syncSourceFake{events: &events}
	parser := &syncParserFake{events: &events}
	blobs := newSyncBlobFake(&events)
	store := newTestOperationalStore()
	uow := &testUnitOfWork{committed: store}
	clock := &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)}
	cache := &syncCacheFake{}
	service, err := NewSyncService(SyncDependencies{
		Source: source, Parser: parser, UnitOfWork: uow, Blobs: blobs, Clock: clock, Cache: cache,
	})
	if err != nil {
		t.Fatalf("construct sync service: %v", err)
	}

	result, err := service.Sync(ctx, SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: "webhook"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	events = append(events, store.calls...)
	wantPrefix := []string{"source", "parse", "blob", "revision", "sync"}
	if !reflect.DeepEqual(events, wantPrefix) {
		t.Fatalf("operation order = %#v, want %#v", events, wantPrefix)
	}
	revision := store.revisions["revision-1"]
	if revision.SpecBlobKey == "" || revision.SpecBlobKey != string(result.BlobKey) {
		t.Fatalf("committed revision blob key = %q, result = %q", revision.SpecBlobKey, result.BlobKey)
	}
	if result.Record.Result != domain.SyncResultSuccess || len(store.syncRecords) != 1 {
		t.Fatalf("sync result/records = %#v / %#v", result.Record, store.syncRecords)
	}
	if !cache.deleted["public:payments"] || !cache.deleted["search:payments:revision-1"] {
		t.Fatalf("cache deletes = %#v", cache.deleted)
	}
	assertSameContexts(t, ctx, source.ctx, parser.ctx, blobs.ctx, uow.ctx, cache.contexts[0], cache.contexts[1])
	for _, got := range store.contexts {
		assertSameContexts(t, ctx, got)
	}
	for _, got := range clock.contexts {
		assertSameContexts(t, ctx, got)
	}
}

func TestSyncCommitFailureLeavesOnlyReplaySafeBlob(t *testing.T) {
	events := []string{}
	blobs := newSyncBlobFake(&events)
	store := newTestOperationalStore()
	service, err := NewSyncService(SyncDependencies{
		Source:     &syncSourceFake{events: &events},
		Parser:     &syncParserFake{events: &events},
		UnitOfWork: &testUnitOfWork{committed: store, failCommit: true},
		Blobs:      blobs,
		Clock:      &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Sync(context.Background(), SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: "webhook"})
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Kind != ErrorTransaction {
		t.Fatalf("sync error = %#v, want transaction classification", err)
	}
	if len(blobs.data) != 1 {
		t.Fatalf("blob writes = %d, want one replay-safe orphan", len(blobs.data))
	}
	if len(store.revisions) != 0 || len(store.syncRecords) != 0 {
		t.Fatalf("transaction leaked state: revisions=%#v sync=%#v", store.revisions, store.syncRecords)
	}
}

func TestSyncPreservesCommittedResultAndAttemptsEveryCacheInvalidation(t *testing.T) {
	store := newTestOperationalStore()
	cache := &syncCacheFake{fail: map[string]error{
		"public:payments":            errors.New("public cache unavailable"),
		"search:payments:revision-1": errors.New("search cache unavailable"),
	}}
	service, err := NewSyncService(SyncDependencies{
		Source: &syncSourceFake{}, Parser: &syncParserFake{},
		UnitOfWork: &testUnitOfWork{committed: store}, Blobs: newSyncBlobFake(nil),
		Clock: &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)}, Cache: cache,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Sync(context.Background(), SyncCommand{ContractID: "payments", SourceID: "source-main"})
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Kind != ErrorCache {
		t.Fatalf("Sync error = %#v, want cache warning", err)
	}
	if result.Revision.ID != "revision-1" || result.Record.Result != domain.SyncResultSuccess {
		t.Fatalf("cache warning discarded committed result: %#v", result)
	}
	if len(store.revisions) != 1 || len(store.syncRecords) != 1 {
		t.Fatalf("cache warning changed committed state: revisions=%d sync=%d", len(store.revisions), len(store.syncRecords))
	}
	if !cache.deleted["public:payments"] || !cache.deleted["search:payments:revision-1"] {
		t.Fatalf("cache invalidation stopped early: %#v", cache.deleted)
	}
}

func TestSyncReplayUsesStableRecordAndBlobIdentity(t *testing.T) {
	store := newTestOperationalStore()
	blobs := newSyncBlobFake(nil)
	service, err := NewSyncService(SyncDependencies{
		Source:     &syncSourceFake{},
		Parser:     &syncParserFake{},
		UnitOfWork: &testUnitOfWork{committed: store},
		Blobs:      blobs,
		Clock:      &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: "webhook"}
	first, err := service.Sync(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Sync(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.ID != second.Record.ID || first.BlobKey != second.BlobKey {
		t.Fatalf("replay identities differ: %#v / %#v", first, second)
	}
	if len(store.syncRecords) != 1 || len(blobs.data) != 1 {
		t.Fatalf("replay duplicated state: records=%d blobs=%d", len(store.syncRecords), len(blobs.data))
	}
}

func TestSyncRejectsNonContentAddressedBlobIdentity(t *testing.T) {
	store := newTestOperationalStore()
	service, err := NewSyncService(SyncDependencies{
		Source:     &syncSourceFake{},
		Parser:     &syncParserFake{},
		UnitOfWork: &testUnitOfWork{committed: store},
		Blobs:      invalidKeyBlobStore{},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Sync(context.Background(), SyncCommand{ContractID: "payments", SourceID: "source-main"})
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Kind != ErrorIntegrity {
		t.Fatalf("Sync error = %#v, want integrity error", err)
	}
	if len(store.revisions) != 0 || len(store.syncRecords) != 0 {
		t.Fatal("invalid blob identity reached operational transaction")
	}
}

type invalidKeyBlobStore struct{}

func (invalidKeyBlobStore) Put(context.Context, []byte) (port.BlobKey, error) {
	return port.BlobKey("caller-selected-key"), nil
}

func (invalidKeyBlobStore) Get(context.Context, port.BlobKey) ([]byte, error) {
	return nil, errors.New("not implemented")
}

type syncSourceFake struct {
	ctx    context.Context
	events *[]string
}

func (s *syncSourceFake) Fetch(ctx context.Context) (domain.SpecFile, domain.ContractRevision, error) {
	s.ctx = ctx
	if s.events != nil {
		*s.events = append(*s.events, "source")
	}
	return domain.SpecFile{SourceID: "source-main", Path: "openapi.yaml", Format: "yaml", Bytes: []byte("openapi: 3.1.0\n")}, domain.ContractRevision{ID: "revision-1", SourceID: "source-main", Ref: "main", CommitSHA: "abc123"}, nil
}

type syncParserFake struct {
	ctx    context.Context
	events *[]string
}

func (p *syncParserFake) Parse(ctx context.Context, _ domain.SpecFile, revision domain.ContractRevision) (domain.SpecIndex, error) {
	p.ctx = ctx
	if p.events != nil {
		*p.events = append(*p.events, "parse")
	}
	return domain.SpecIndex{RevisionID: revision.ID, Title: "Payments"}, nil
}

type syncBlobFake struct {
	ctx    context.Context
	events *[]string
	data   map[port.BlobKey][]byte
}

func newSyncBlobFake(events *[]string) *syncBlobFake {
	return &syncBlobFake{events: events, data: map[port.BlobKey][]byte{}}
}

func (s *syncBlobFake) Put(ctx context.Context, data []byte) (port.BlobKey, error) {
	s.ctx = ctx
	if s.events != nil {
		*s.events = append(*s.events, "blob")
	}
	sum := sha256.Sum256(data)
	key := port.BlobKey("sha256:" + hex.EncodeToString(sum[:]))
	s.data[key] = append([]byte(nil), data...)
	return key, nil
}

func (s *syncBlobFake) Get(ctx context.Context, key port.BlobKey) ([]byte, error) {
	s.ctx = ctx
	data, ok := s.data[key]
	if !ok {
		return nil, errors.New("blob not found")
	}
	return append([]byte(nil), data...), nil
}

type syncCacheFake struct {
	deleted  map[string]bool
	contexts []context.Context
	fail     map[string]error
}

func (c *syncCacheFake) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (c *syncCacheFake) Set(context.Context, string, []byte) error         { return nil }
func (c *syncCacheFake) Delete(ctx context.Context, key string) error {
	c.contexts = append(c.contexts, ctx)
	if c.deleted == nil {
		c.deleted = map[string]bool{}
	}
	c.deleted[key] = true
	return c.fail[key]
}
