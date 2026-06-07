package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSyncerPersistsGoodRevisionAndRecordsSuccess(t *testing.T) {
	ctx := context.Background()
	source := fakeSource{
		spec: SpecFile{SourceID: "src1", Path: "openapi.yaml", Format: "yaml", Bytes: []byte("openapi: 3.1.0")},
		rev:  Revision{ID: "rev1", SourceID: "src1", Ref: "main", CommitSHA: "abc123"},
	}
	parser := fakeParser{idx: SpecIndex{Title: "Payments API", PublicRoutes: []PublicRoute{{Path: "/", Title: "Payments API"}}}}
	store := &fakeSyncStore{}
	blobs := &fakeBlobStore{}
	cache := &fakeCache{}

	result, err := Syncer{
		Source: source,
		Parser: parser,
		Store:  store,
		Blobs:  blobs,
		Cache:  cache,
	}.Sync(ctx, SyncRequest{ProjectID: "p1", SourceID: "src1", Trigger: "manual"})
	if err != nil {
		t.Fatal(err)
	}

	if result.Revision.ID != "rev1" || result.Index.ProjectID != "p1" || result.Index.RevisionID != "rev1" {
		t.Fatalf("result = %#v", result)
	}
	if store.savedRevision.ID != "rev1" {
		t.Fatalf("saved revision = %#v", store.savedRevision)
	}
	if got := string(blobs.data["specs/rev1.yaml"]); got != "openapi: 3.1.0" {
		t.Fatalf("blob = %q", got)
	}
	if !cache.deleted["public:p1"] || !cache.deleted["search:p1:rev1"] {
		t.Fatalf("cache deletes = %#v", cache.deleted)
	}
	if len(store.records) != 1 {
		t.Fatalf("records = %#v", store.records)
	}
	record := store.records[0]
	if record.Result != SyncResultSuccess || record.ProjectID != "p1" || record.SourceID != "src1" || record.RevisionID != "rev1" {
		t.Fatalf("record = %#v", record)
	}
	if record.Trigger != "manual" || record.Ref != "main" || record.CommitSHA != "abc123" || record.SpecPath != "openapi.yaml" {
		t.Fatalf("record metadata = %#v", record)
	}
	if record.ErrorSummary != "" || record.ID == "" || record.StartedAt.IsZero() || record.FinishedAt.IsZero() {
		t.Fatalf("record lifecycle = %#v", record)
	}
}

func TestSyncerRecordsParseFailureWithoutReplacingLastGoodState(t *testing.T) {
	ctx := context.Background()
	source := fakeSource{
		spec: SpecFile{SourceID: "src1", Path: "openapi.yaml", Format: "yaml", Bytes: []byte("not openapi")},
		rev:  Revision{ID: "rev1", SourceID: "src1", Ref: "main", CommitSHA: "abc123"},
	}
	parser := fakeParser{err: errors.New("parse failed")}
	store := &fakeSyncStore{}
	blobs := &fakeBlobStore{}
	cache := &fakeCache{}

	_, err := Syncer{
		Source: source,
		Parser: parser,
		Store:  store,
		Blobs:  blobs,
		Cache:  cache,
	}.Sync(ctx, SyncRequest{ProjectID: "p1", SourceID: "src1", Trigger: "manual"})
	if err == nil {
		t.Fatal("expected parse error")
	}

	if store.savedRevision.ID != "" {
		t.Fatalf("saved revision after parse failure = %#v", store.savedRevision)
	}
	if len(blobs.data) != 0 {
		t.Fatalf("blobs after parse failure = %#v", blobs.data)
	}
	if len(cache.deleted) != 0 {
		t.Fatalf("cache deletes after parse failure = %#v", cache.deleted)
	}
	if len(store.records) != 1 {
		t.Fatalf("records = %#v", store.records)
	}
	record := store.records[0]
	if record.Result != SyncResultFailure || record.RevisionID != "rev1" || record.CommitSHA != "abc123" {
		t.Fatalf("failure record = %#v", record)
	}
	if !strings.Contains(record.ErrorSummary, "parse failed") {
		t.Fatalf("failure summary = %q", record.ErrorSummary)
	}
}

type fakeSource struct {
	spec SpecFile
	rev  Revision
	err  error
}

func (s fakeSource) Fetch(context.Context) (SpecFile, Revision, error) {
	return s.spec, s.rev, s.err
}

type fakeParser struct {
	idx SpecIndex
	err error
}

func (p fakeParser) Parse(context.Context, SpecFile, Revision) (SpecIndex, error) {
	return p.idx, p.err
}

type fakeSyncStore struct {
	savedRevision Revision
	records       []SyncRecord
}

func (s *fakeSyncStore) SaveProject(context.Context, Project) error {
	return nil
}

func (s *fakeSyncStore) Project(context.Context, string) (Project, error) {
	return Project{}, nil
}

func (s *fakeSyncStore) SaveRevision(_ context.Context, rev Revision) error {
	s.savedRevision = rev
	return nil
}

func (s *fakeSyncStore) Revision(context.Context, string) (Revision, error) {
	return Revision{}, nil
}

func (s *fakeSyncStore) SavePublication(context.Context, Publication) error {
	return nil
}

func (s *fakeSyncStore) SaveSyncRecord(_ context.Context, record SyncRecord) error {
	s.records = append(s.records, record)
	return nil
}

type fakeBlobStore struct {
	data map[string][]byte
}

func (s *fakeBlobStore) Put(_ context.Context, key string, data []byte) error {
	if s.data == nil {
		s.data = map[string][]byte{}
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *fakeBlobStore) Get(context.Context, string) ([]byte, error) {
	return nil, nil
}

type fakeCache struct {
	deleted map[string]bool
}

func (c *fakeCache) Get(string) ([]byte, bool) {
	return nil, false
}

func (c *fakeCache) Set(string, []byte) {}

func (c *fakeCache) Delete(key string) {
	if c.deleted == nil {
		c.deleted = map[string]bool{}
	}
	c.deleted[key] = true
}
