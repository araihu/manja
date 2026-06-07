package store

import (
	"context"
	"testing"
	"time"

	"github.com/araihu/manja/internal/core"
)

func TestFileStorePersistsProjectRevisionPublicationAndBlob(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())

	project := core.Project{ID: "p1", Name: "Payments", Slug: "payments"}
	if err := fs.SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	gotProject, err := fs.Project(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if gotProject.Name != "Payments" {
		t.Fatalf("project name = %q", gotProject.Name)
	}

	rev := core.Revision{ID: "r1", SourceID: "s1", Ref: "main"}
	if err := fs.SaveRevision(ctx, rev); err != nil {
		t.Fatal(err)
	}
	gotRev, err := fs.Revision(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if gotRev.Ref != "main" {
		t.Fatalf("revision ref = %q", gotRev.Ref)
	}

	pub := core.Publication{ProjectID: "p1", RevisionID: "r1", Public: true, Path: "/acme/payments/v1"}
	if err := fs.SavePublication(ctx, pub); err != nil {
		t.Fatal(err)
	}
	var gotPub core.Publication
	if err := fs.readJSON(ctx, "publications", "p1-r1.json", &gotPub); err != nil {
		t.Fatal(err)
	}
	if gotPub.ProjectID != "p1" || gotPub.RevisionID != "r1" || !gotPub.Public || gotPub.Path != "/acme/payments/v1" {
		t.Fatalf("publication = %+v", gotPub)
	}

	if err := fs.Put(ctx, "specs/r1.yaml", []byte("openapi: 3.1.0")); err != nil {
		t.Fatal(err)
	}
	blob, err := fs.Get(ctx, "specs/r1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(blob) != "openapi: 3.1.0" {
		t.Fatalf("blob = %q", blob)
	}

	record := core.SyncRecord{
		ID:         "sync-1",
		ProjectID:  "p1",
		SourceID:   "s1",
		RevisionID: "r1",
		Trigger:    "manual",
		Ref:        "main",
		CommitSHA:  "abc123",
		SpecPath:   "openapi.yaml",
		Result:     core.SyncResultSuccess,
		StartedAt:  time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC),
		FinishedAt: time.Date(2026, 6, 7, 1, 2, 4, 0, time.UTC),
	}
	if err := fs.SaveSyncRecord(ctx, record); err != nil {
		t.Fatal(err)
	}
	var gotRecord core.SyncRecord
	if err := fs.readJSON(ctx, "sync-history", "sync-1.json", &gotRecord); err != nil {
		t.Fatal(err)
	}
	if gotRecord.ProjectID != "p1" || gotRecord.Result != core.SyncResultSuccess || gotRecord.CommitSHA != "abc123" {
		t.Fatalf("sync record = %+v", gotRecord)
	}
}

func TestFileStoreRejectsBlobNamespaceTraversal(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())

	project := core.Project{ID: "p1", Name: "Payments", Slug: "payments"}
	if err := fs.SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(ctx, "../projects/p1.json", []byte(`{"Name":"Owned"}`)); err == nil {
		t.Fatal("Put accepted blob key that escapes blob namespace")
	}
	if _, err := fs.Get(ctx, "../projects/p1.json"); err == nil {
		t.Fatal("Get accepted blob key that escapes blob namespace")
	}
	if err := fs.Put(ctx, "/projects/p1.json", []byte(`{"Name":"Owned"}`)); err == nil {
		t.Fatal("Put accepted absolute blob key")
	}
	if err := fs.Put(ctx, "specs/../projects/p1.json", []byte(`{"Name":"Owned"}`)); err == nil {
		t.Fatal("Put accepted blob key containing traversal")
	}
	gotProject, err := fs.Project(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if gotProject.Name != "Payments" {
		t.Fatalf("project was overwritten through blob namespace: %+v", gotProject)
	}
}

func TestFileStoreRejectsInvalidFlatIDs(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "save project separator",
			run:  func() error { return fs.SaveProject(ctx, core.Project{ID: "bad/id"}) },
		},
		{
			name: "save project absolute",
			run:  func() error { return fs.SaveProject(ctx, core.Project{ID: "/bad"}) },
		},
		{
			name: "get project traversal",
			run: func() error {
				_, err := fs.Project(ctx, "../p1")
				return err
			},
		},
		{
			name: "save revision separator",
			run:  func() error { return fs.SaveRevision(ctx, core.Revision{ID: "bad/id"}) },
		},
		{
			name: "save revision absolute",
			run:  func() error { return fs.SaveRevision(ctx, core.Revision{ID: "/bad"}) },
		},
		{
			name: "get revision traversal",
			run: func() error {
				_, err := fs.Revision(ctx, "../r1")
				return err
			},
		},
		{
			name: "save publication project separator",
			run:  func() error { return fs.SavePublication(ctx, core.Publication{ProjectID: "bad/id", RevisionID: "r1"}) },
		},
		{
			name: "save publication project absolute",
			run:  func() error { return fs.SavePublication(ctx, core.Publication{ProjectID: "/bad", RevisionID: "r1"}) },
		},
		{
			name: "save publication revision traversal",
			run:  func() error { return fs.SavePublication(ctx, core.Publication{ProjectID: "p1", RevisionID: "../r1"}) },
		},
		{
			name: "save sync record separator",
			run:  func() error { return fs.SaveSyncRecord(ctx, core.SyncRecord{ID: "bad/id"}) },
		},
		{
			name: "save sync record traversal",
			run:  func() error { return fs.SaveSyncRecord(ctx, core.SyncRecord{ID: "../sync"}) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("accepted invalid ID")
			}
		})
	}
}
