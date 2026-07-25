package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/contracttest"
	core "github.com/araihu/manja/domain"
)

func TestFileStorePublicContracts(t *testing.T) {
	contracttest.UnitOfWork(t, func(t testing.TB) port.UnitOfWork {
		return NewFileStore(t.TempDir())
	})
	contracttest.BlobStore(t, func(t testing.TB) port.BlobStore {
		return NewFileStore(t.TempDir())
	})
}

func TestFileStoreUnitOfWorkRollsBackEveryOperationalMutation(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	key, err := store.Put(ctx, []byte("openapi: 3.1.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("stop transaction")
	err = store.Within(ctx, func(txCtx context.Context, operational port.OperationalStore) error {
		if txCtx != ctx {
			t.Fatal("unit of work replaced the incoming context")
		}
		if err := operational.SaveRevision(txCtx, core.ContractRevision{ID: "r1", SourceID: "s1", SpecBlobKey: string(key)}); err != nil {
			return err
		}
		if err := operational.SaveSyncRecord(txCtx, core.SyncRecord{ID: "sync-1", RevisionID: "r1"}); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Within error = %v, want %v", err, wantErr)
	}
	if _, err := store.Revision(ctx, "r1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back revision lookup error = %v, want not exist", err)
	}
	if _, err := store.SyncRecord(ctx, "sync-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back sync lookup error = %v, want not exist", err)
	}
}

func TestFileStoreUnitOfWorkRejectsStaleReleaseTrackGeneration(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	track := core.ReleaseTrack{ID: "stable", ContractID: "payments", Mode: core.ReleaseModeFollowing, Generation: 1}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, 0, track)
	}); err != nil {
		t.Fatal(err)
	}
	err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		track.Generation = 2
		return operational.SaveReleaseTrack(ctx, 0, track)
	})
	if !errors.Is(err, port.ErrGenerationConflict) {
		t.Fatalf("stale generation error = %v, want %v", err, port.ErrGenerationConflict)
	}
	got, err := store.ReleaseTrack(ctx, "payments", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if got.Generation != 1 {
		t.Fatalf("generation after rejected transaction = %d, want 1", got.Generation)
	}
}

func TestFileStoreReportsIndeterminateCommitAfterManifestRename(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(*FileStore)
	}{
		{
			name: "open parent directory",
			configure: func(store *FileStore) {
				store.openDirectory = func(string) (directorySyncer, error) {
					return nil, errors.New("forced directory open failure")
				}
			},
		},
		{
			name: "sync parent directory",
			configure: func(store *FileStore) {
				store.openDirectory = func(string) (directorySyncer, error) {
					return failingDirectorySyncer{err: errors.New("forced directory sync failure")}, nil
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			store := NewFileStore(root)
			track := core.ReleaseTrack{
				ID: "stable", ContractID: "payments", Mode: core.ReleaseModeFollowing, Generation: 1,
			}
			if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SaveReleaseTrack(ctx, 0, track)
			}); err != nil {
				t.Fatal(err)
			}
			tt.configure(store)

			err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				current, err := operational.ReleaseTrack(ctx, "payments", "stable")
				if err != nil {
					return err
				}
				current.Generation = 2
				return operational.SaveReleaseTrack(ctx, 1, current)
			})
			if !errors.Is(err, port.ErrCommitOutcomeUnknown) {
				t.Fatalf("post-rename error = %v, want %v", err, port.ErrCommitOutcomeUnknown)
			}

			restarted := NewFileStore(root)
			got, err := restarted.ReleaseTrack(ctx, "payments", "stable")
			if err != nil {
				t.Fatal(err)
			}
			if got.Generation != 2 {
				t.Fatalf("recovered generation = %d, want atomically published generation 2", got.Generation)
			}
		})
	}
}

type failingDirectorySyncer struct {
	err error
}

func (f failingDirectorySyncer) Sync() error  { return f.err }
func (f failingDirectorySyncer) Close() error { return nil }

func TestFileStoreContentAddressedBlobSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	want := []byte("openapi: 3.1.0\n")
	first := NewFileStore(root)
	key, err := first.Put(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	replayKey, err := first.Put(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if replayKey != key || key != port.ContentAddressedBlobKey(want) {
		t.Fatalf("blob keys = %q and %q, want %q", key, replayKey, port.ContentAddressedBlobKey(want))
	}
	restarted := NewFileStore(root)
	got, err := restarted.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("restarted blob = %q, want %q", got, want)
	}
}

func TestFileStoreDiscardsIncompleteOperationalStagingOnRestart(t *testing.T) {
	root := t.TempDir()
	stagingFiles := []string{
		filepath.Join(root, "operational", ".state-interrupted.tmp"),
		filepath.Join(root, "blobs", "sha256", ".write-interrupted.tmp"),
	}
	for _, staging := range stagingFiles {
		if err := os.MkdirAll(filepath.Dir(staging), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(staging, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = NewFileStore(root)
	for _, staging := range stagingFiles {
		if _, err := os.Stat(staging); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staging file %q after restart error = %v, want not exist", staging, err)
		}
	}
}

func TestFileStoreMigratesCommittedLegacyOperationalState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	revision := core.ContractRevision{ID: "legacy-revision", SourceID: "source", Ref: "v1"}
	publication := core.Publication{ProjectID: "payments", RevisionID: revision.ID, Public: true, Path: "/payments/v1"}
	record := core.SyncRecord{ID: "legacy-sync", ProjectID: "payments", RevisionID: revision.ID, Result: core.SyncResultSuccess}
	if err := store.writeJSON(ctx, "revisions", revision.ID+".json", revision); err != nil {
		t.Fatal(err)
	}
	if err := store.writeJSON(ctx, "publications", "payments-"+revision.ID+".json", publication); err != nil {
		t.Fatal(err)
	}
	if err := store.writeJSON(ctx, "sync-history", record.ID+".json", record); err != nil {
		t.Fatal(err)
	}

	publication.Path = "/payments/stable"
	if err := store.SavePublication(ctx, publication); err != nil {
		t.Fatalf("migrate and update legacy publication: %v", err)
	}
	for _, namespace := range []string{"revisions", "publications", "sync-history"} {
		if err := os.RemoveAll(filepath.Join(root, namespace)); err != nil {
			t.Fatal(err)
		}
	}

	restarted := NewFileStore(root)
	gotRevision, err := restarted.Revision(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRevision.Ref != revision.Ref {
		t.Fatalf("migrated revision = %#v", gotRevision)
	}
	gotPublication, err := restarted.Publication(ctx, publication.ProjectID, publication.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if gotPublication.Path != publication.Path {
		t.Fatalf("migrated publication = %#v", gotPublication)
	}
	if _, err := restarted.SyncRecord(ctx, record.ID); err != nil {
		t.Fatalf("migrated sync record: %v", err)
	}
}

func TestFileStoreValidatesMigratedLegacyRevisionBlobBeforeFirstCommit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	revision := core.ContractRevision{
		ID: "legacy-revision", SourceID: "source",
		SpecBlobKey: string(port.ContentAddressedBlobKey([]byte("missing legacy blob"))),
	}
	if err := store.writeJSON(ctx, "revisions", revision.ID+".json", revision); err != nil {
		t.Fatal(err)
	}
	err := store.SavePublication(ctx, core.Publication{
		ProjectID: "payments", RevisionID: revision.ID, Public: true, Path: "/payments/v1",
	})
	if err == nil {
		t.Fatal("first manifest commit accepted a migrated revision with a missing blob")
	}
	if _, statErr := os.Stat(filepath.Join(root, "operational", "state.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid migration published operational state: %v", statErr)
	}
}

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
	readPub, err := fs.Publication(ctx, "p1", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if readPub.ProjectID != "p1" || readPub.RevisionID != "r1" || !readPub.Public || readPub.Path != "/acme/payments/v1" {
		t.Fatalf("read publication = %+v", readPub)
	}

	key, err := fs.Put(ctx, []byte("openapi: 3.1.0"))
	if err != nil {
		t.Fatal(err)
	}
	blob, err := fs.Get(ctx, key)
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
	gotRecord, err := fs.SyncRecord(ctx, "sync-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotRecord.ProjectID != "p1" || gotRecord.Result != core.SyncResultSuccess || gotRecord.CommitSHA != "abc123" {
		t.Fatalf("sync record = %+v", gotRecord)
	}
}

func TestFileStoreReadsPublicPublicationByPath(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())
	if err := fs.SaveRevision(ctx, core.ContractRevision{ID: "r1", SourceID: "s1"}); err != nil {
		t.Fatal(err)
	}
	pub := core.Publication{
		ProjectID:  "p1",
		RevisionID: "r1",
		Public:     true,
		Path:       "/acme/payments/v1",
	}
	if err := fs.SavePublication(ctx, pub); err != nil {
		t.Fatal(err)
	}
	got, err := fs.PublicPublicationByPath(ctx, "/acme/payments/v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != "p1" || got.RevisionID != "r1" || !got.Public {
		t.Fatalf("publication = %#v", got)
	}
}

func TestFileStorePublicPublicationAdvancementRetiresPriorRoute(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	for _, revisionID := range []string{"r1", "r2"} {
		if err := store.SaveRevision(ctx, core.ContractRevision{ID: revisionID, SourceID: "s1"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, revisionID := range []string{"r1", "r2"} {
		if err := store.SavePublication(ctx, core.Publication{
			ProjectID: "p1", RevisionID: revisionID, Public: true, Path: "/acme/payments/v1",
		}); err != nil {
			t.Fatal(err)
		}
	}

	prior, err := store.Publication(ctx, "p1", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if prior.Public {
		t.Fatalf("prior revision remained public after route advancement: %#v", prior)
	}

	restarted := NewFileStore(root)
	for attempt := 0; attempt < 256; attempt++ {
		current, err := restarted.PublicPublicationByPath(ctx, "/acme/payments/v1")
		if err != nil {
			t.Fatal(err)
		}
		if current.RevisionID != "r2" {
			t.Fatalf("public lookup attempt %d resolved revision %q, want r2", attempt, current.RevisionID)
		}
	}
}

func TestFileStorePublicPublicationByPathRejectsPrivateAndInvalidPaths(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())
	if err := fs.SaveRevision(ctx, core.ContractRevision{ID: "r1", SourceID: "s1"}); err != nil {
		t.Fatal(err)
	}
	pub := core.Publication{
		ProjectID:  "p1",
		RevisionID: "r1",
		Public:     false,
		Path:       "/acme/payments/v1",
	}
	if err := fs.SavePublication(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.PublicPublicationByPath(ctx, "/acme/payments/v1"); err == nil {
		t.Fatal("private publication was returned for public path lookup")
	}
	if _, err := fs.PublicPublicationByPath(ctx, "acme/payments/v1"); err == nil {
		t.Fatal("relative publication path was accepted")
	}
	if _, err := fs.PublicPublicationByPath(ctx, "/../payments"); err == nil {
		t.Fatal("unsafe publication path was accepted")
	}
}

func TestFileStoreRejectsBlobNamespaceTraversal(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())

	project := core.Project{ID: "p1", Name: "Payments", Slug: "payments"}
	if err := fs.SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get(ctx, port.BlobKey("../projects/p1.json")); err == nil {
		t.Fatal("Get accepted blob key that escapes blob namespace")
	}
	if _, err := fs.Get(ctx, port.BlobKey("sha256:not-a-digest")); err == nil {
		t.Fatal("Get accepted malformed content-addressed key")
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
			name: "save publication relative path",
			run: func() error {
				return fs.SavePublication(ctx, core.Publication{ProjectID: "p1", RevisionID: "r1", Path: "acme/payments"})
			},
		},
		{
			name: "save publication traversal path",
			run: func() error {
				return fs.SavePublication(ctx, core.Publication{ProjectID: "p1", RevisionID: "r1", Path: "/../payments"})
			},
		},
		{
			name: "get publication project separator",
			run: func() error {
				_, err := fs.Publication(ctx, "bad/id", "r1")
				return err
			},
		},
		{
			name: "get publication revision traversal",
			run: func() error {
				_, err := fs.Publication(ctx, "p1", "../r1")
				return err
			},
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
