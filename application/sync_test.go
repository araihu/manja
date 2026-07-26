package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
	storeadapter "github.com/araihu/manja/internal/adapters/store"
)

type syncContextKey struct{}

func TestNewSyncServiceRequiresCanonicalSyncRecordReader(t *testing.T) {
	_, err := NewSyncService(SyncDependencies{
		Source: &syncSourceFake{}, Parser: &syncParserFake{},
		UnitOfWork: &testUnitOfWork{committed: newTestOperationalStore()},
		Blobs:      newSyncBlobFake(nil), Clock: &testClock{now: time.Now()},
	})
	if err == nil {
		t.Fatal("sync service accepted a missing canonical sync record reader")
	}
}

func TestSyncRejectsMalformedCommandBeforeAnyEffect(t *testing.T) {
	invalidUTF8 := string([]byte("payments-\xff"))
	tests := []struct {
		name    string
		command SyncCommand
	}{
		{name: "contract padding", command: SyncCommand{ContractID: " payments", SourceID: "source-main"}},
		{name: "contract control", command: SyncCommand{ContractID: "payments\x00shadow", SourceID: "source-main"}},
		{name: "contract invalid UTF-8", command: SyncCommand{ContractID: invalidUTF8, SourceID: "source-main"}},
		{name: "source padding", command: SyncCommand{ContractID: "payments", SourceID: "source-main "}},
		{name: "source control", command: SyncCommand{ContractID: "payments", SourceID: "source\nmain"}},
		{name: "source invalid UTF-8", command: SyncCommand{ContractID: "payments", SourceID: invalidUTF8}},
		{name: "trigger padding", command: SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: " webhook"}},
		{name: "trigger control", command: SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: "web\thook"}},
		{name: "trigger invalid UTF-8", command: SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: invalidUTF8}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			source := &syncSourceFake{events: &events}
			parser := &syncParserFake{events: &events}
			blobs := newSyncBlobFake(&events)
			store := newTestOperationalStore()
			uow := &testUnitOfWork{committed: store}
			clock := &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)}
			service, err := NewSyncService(SyncDependencies{
				Source: source, Parser: parser, UnitOfWork: uow, SyncRecords: uow,
				Blobs: blobs, Clock: clock,
			})
			if err != nil {
				t.Fatal(err)
			}

			if _, err := service.Sync(context.Background(), test.command); err == nil {
				t.Fatal("malformed sync command was accepted")
			}
			if len(events) != 0 || source.ctx != nil || parser.ctx != nil || blobs.ctx != nil || uow.ctx != nil {
				t.Fatalf("malformed command caused effects: events=%v source=%v parser=%v blob=%v uow=%v", events, source.ctx, parser.ctx, blobs.ctx, uow.ctx)
			}
			if len(clock.contexts) != 0 || len(store.calls) != 0 || len(blobs.data) != 0 {
				t.Fatalf("malformed command changed dependencies: clock=%d store=%v blobs=%d", len(clock.contexts), store.calls, len(blobs.data))
			}
		})
	}
}

func TestSyncUsesExplicitCanonicalReaderAfterCommit(t *testing.T) {
	ctx := context.WithValue(context.Background(), syncContextKey{}, "canonical")
	store := newTestOperationalStore()
	uow := &testUnitOfWork{committed: store}
	reader := &separateSyncRecordReader{store: store}
	dependencies := SyncDependencies{
		Source: &syncSourceFake{}, Parser: &syncParserFake{},
		UnitOfWork: uow, SyncRecords: reader, Blobs: newSyncBlobFake(nil),
		Clock: &advancingSyncClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC), step: time.Minute},
	}
	service, err := NewSyncService(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	command := SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: "webhook"}
	first, err := service.Sync(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Sync(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.Record, first.Record) {
		t.Fatalf("explicit reader replay = %#v, want first observation %#v", second.Record, first.Record)
	}
	if reader.calls != 2 {
		t.Fatalf("canonical reader calls = %d, want 2", reader.calls)
	}
	assertSameContexts(t, ctx, reader.ctx)
}

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
		Source: source, Parser: parser, UnitOfWork: uow, SyncRecords: uow,
		Blobs: blobs, Clock: clock, Cache: cache,
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
	if revision.ContractID != "payments" {
		t.Fatalf("committed revision contract id = %q, want payments", revision.ContractID)
	}
	if revision.SpecDigest != "f39db8e8ede3dc2457c613e2a304e6d478f6e5ec660e4746464f41e76ac77006" {
		t.Fatalf("committed revision spec digest = %q", revision.SpecDigest)
	}
	if revision.ContractDigest != "e4400417ff2796c6684383ba52313f2e4f8a0ba0365fa94986bed503138b95e0" {
		t.Fatalf("committed revision contract digest = %q", revision.ContractDigest)
	}
	if revision.ReviewSnapshot == nil {
		t.Fatal("committed revision is missing its canonical review snapshot")
	}
	if revision.ReviewSnapshot.ContractID != revision.ContractID ||
		revision.ReviewSnapshot.RevisionID != revision.ID ||
		revision.ReviewSnapshot.SpecDigest != revision.SpecDigest ||
		revision.ReviewSnapshot.ContractDigest != revision.ContractDigest {
		t.Fatalf("committed revision review snapshot = %#v, revision = %#v", revision.ReviewSnapshot, revision)
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
	uow := &testUnitOfWork{committed: store, failCommit: true}
	service, err := NewSyncService(SyncDependencies{
		Source:      &syncSourceFake{events: &events},
		Parser:      &syncParserFake{events: &events},
		UnitOfWork:  uow,
		SyncRecords: uow,
		Blobs:       blobs,
		Clock:       &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)},
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
	uow := &testUnitOfWork{committed: store}
	service, err := NewSyncService(SyncDependencies{
		Source: &syncSourceFake{}, Parser: &syncParserFake{},
		UnitOfWork: uow, SyncRecords: uow, Blobs: newSyncBlobFake(nil),
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
	uow := &testUnitOfWork{committed: store}
	service, err := NewSyncService(SyncDependencies{
		Source:      &syncSourceFake{},
		Parser:      &syncParserFake{},
		UnitOfWork:  uow,
		SyncRecords: uow,
		Blobs:       blobs,
		Clock:       &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)},
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

func TestSyncReplayWithAdvancingClockReusesDurableLogicalEvidence(t *testing.T) {
	for _, trigger := range []string{"webhook", "poll", "manual", "ci"} {
		t.Run(trigger+" success", func(t *testing.T) {
			ctx := context.Background()
			store := storeadapter.NewFileStore(t.TempDir())
			clock := &advancingSyncClock{
				now:  time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC),
				step: time.Minute,
			}
			service, err := NewSyncService(SyncDependencies{
				Source: &syncSourceFake{}, Parser: &syncParserFake{},
				UnitOfWork: store, SyncRecords: store, Blobs: store, Clock: clock,
			})
			if err != nil {
				t.Fatal(err)
			}
			command := SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: trigger}
			first, err := service.Sync(ctx, command)
			if err != nil {
				t.Fatalf("first sync: %v", err)
			}
			second, err := service.Sync(ctx, command)
			if err != nil {
				t.Fatalf("replay sync: %v", err)
			}
			if !reflect.DeepEqual(second.Record, first.Record) {
				t.Fatalf("replay record = %#v, want first durable evidence %#v", second.Record, first.Record)
			}
			persisted, err := store.SyncRecord(ctx, first.Record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted, first.Record) {
				t.Fatalf("persisted replay evidence = %#v, want %#v", persisted, first.Record)
			}
		})

		t.Run(trigger+" failure", func(t *testing.T) {
			ctx := context.Background()
			store := storeadapter.NewFileStore(t.TempDir())
			cause := errors.New("source temporarily unavailable")
			service, err := NewSyncService(SyncDependencies{
				Source: failingSyncSource{err: cause}, Parser: &syncParserFake{},
				UnitOfWork: store, SyncRecords: store, Blobs: store,
				Clock: &advancingSyncClock{
					now:  time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC),
					step: time.Minute,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			command := SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: trigger}
			var firstPersisted domain.SyncRecord
			for attempt := 0; attempt < 2; attempt++ {
				_, err := service.Sync(ctx, command)
				var appErr *Error
				if !errors.As(err, &appErr) || appErr.Kind != ErrorSource || !errors.Is(err, cause) {
					t.Fatalf("failure replay %d error = %#v, want original source failure", attempt+1, err)
				}
				persisted, err := store.SyncRecord(ctx, syncRecordID(command, domain.ContractRevision{}, domain.SyncResultFailure))
				if err != nil {
					t.Fatal(err)
				}
				if attempt == 0 {
					firstPersisted = persisted
				} else if !reflect.DeepEqual(persisted, firstPersisted) {
					t.Fatalf("failure replay evidence = %#v, want first durable evidence %#v", persisted, firstPersisted)
				}
			}
			if firstPersisted.StartedAt.IsZero() || firstPersisted.FinishedAt.IsZero() {
				t.Fatalf("canonical failure evidence lost first observation times: %#v", firstPersisted)
			}
		})
	}
}

func TestFailureReplayNormalizesUTF8AtSummaryBoundaryAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cause := errors.New(strings.Repeat("a", 511) + "é")
	command := SyncCommand{ContractID: "payments", SourceID: "source-main", Trigger: "webhook"}

	for attempt := 0; attempt < 2; attempt++ {
		store := storeadapter.NewFileStore(root)
		service, err := NewSyncService(SyncDependencies{
			Source: failingSyncSource{err: cause}, Parser: &syncParserFake{},
			UnitOfWork: store, SyncRecords: store, Blobs: store,
			Clock: &advancingSyncClock{
				now:  time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC).Add(time.Duration(attempt) * time.Hour),
				step: time.Minute,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.Sync(ctx, command)
		var appErr *Error
		if !errors.As(err, &appErr) || appErr.Kind != ErrorSource || !errors.Is(err, cause) {
			t.Fatalf("failure replay %d error = %#v, want original source error", attempt+1, err)
		}
	}

	store := storeadapter.NewFileStore(root)
	persisted, err := store.SyncRecord(ctx, syncRecordID(command, domain.ContractRevision{}, domain.SyncResultFailure))
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(persisted.ErrorSummary) || len(persisted.ErrorSummary) > 512 {
		t.Fatalf("persisted error summary is not valid bounded UTF-8: %q (%d bytes)", persisted.ErrorSummary, len(persisted.ErrorSummary))
	}
	if persisted.ErrorSummary != strings.Repeat("a", 511) {
		t.Fatalf("persisted boundary summary = %q, want complete-rune prefix", persisted.ErrorSummary)
	}

	changed := errors.New(strings.Repeat("a", 510) + "different")
	service, err := NewSyncService(SyncDependencies{
		Source: failingSyncSource{err: changed}, Parser: &syncParserFake{},
		UnitOfWork: store, SyncRecords: store, Blobs: store,
		Clock: &testClock{now: time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Sync(ctx, command); err == nil {
		t.Fatal("semantically different failure reused immutable sync identity")
	} else {
		var appErr *Error
		if !errors.As(err, &appErr) || appErr.Kind != ErrorTransaction {
			t.Fatalf("changed failure error = %#v, want transaction conflict", err)
		}
	}
}

func TestErrorSummaryNormalizesInvalidUTF8(t *testing.T) {
	got := errorSummary(errors.New(string([]byte{'x', 0xff, 'y'})))
	if got != "x\uFFFDy" || !utf8.ValidString(got) {
		t.Fatalf("normalized summary = %q, want valid replacement form", got)
	}
}

func TestSyncRejectsNonContentAddressedBlobIdentity(t *testing.T) {
	store := newTestOperationalStore()
	uow := &testUnitOfWork{committed: store}
	service, err := NewSyncService(SyncDependencies{
		Source:      &syncSourceFake{},
		Parser:      &syncParserFake{},
		UnitOfWork:  uow,
		SyncRecords: uow,
		Blobs:       invalidKeyBlobStore{},
		Clock:       &testClock{now: time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)},
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

type failingSyncSource struct{ err error }

func (s failingSyncSource) Fetch(context.Context) (domain.SpecFile, domain.ContractRevision, error) {
	return domain.SpecFile{}, domain.ContractRevision{}, s.err
}

type advancingSyncClock struct {
	now  time.Time
	step time.Duration
}

type separateSyncRecordReader struct {
	store *testOperationalStore
	ctx   context.Context
	calls int
}

func (r *separateSyncRecordReader) SyncRecord(ctx context.Context, id string) (domain.SyncRecord, error) {
	r.ctx = ctx
	r.calls++
	record, ok := r.store.syncRecords[id]
	if !ok {
		return domain.SyncRecord{}, errors.New("sync record not found")
	}
	return record, nil
}

func (c *advancingSyncClock) Now(context.Context) time.Time {
	current := c.now
	c.now = c.now.Add(c.step)
	return current
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
