package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/araihu/manja/application/port"
	core "github.com/araihu/manja/domain"
)

func TestFileStoreRejectsAuthorizationAgainstNonCurrentTrackBaseline(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, true)
	fixture.baseline.CurrentRevisionID = "revision-current"
	var changed bool
	var err error
	fixture.next, changed, err = core.ConsiderReleaseDecision(fixture.baseline, *fixture.next.LastDecision)
	if err != nil || !changed {
		t.Fatalf("derive mismatched-baseline decision: changed=%t err=%v", changed, err)
	}
	if err := store.SaveRevision(ctx, core.ContractRevision{
		ID: "revision-current", ContractID: fixture.baseline.ContractID,
	}); err != nil {
		t.Fatal(err)
	}
	persistStoredReleaseBaseline(t, ctx, store, fixture, false)

	before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		transaction, ok := operational.(*operationalTransaction)
		if !ok {
			return errors.New("file-store transaction unavailable")
		}
		transaction.state.ReleaseAuthorizations[fixture.authorization.ReviewID] = fixture.authorization
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	})
	if err == nil {
		t.Fatal("release transition accepted a review against a non-current baseline")
	}
	after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("mismatched-baseline transition changed persisted state")
	}
	got, err := NewFileStore(root).ReleaseTrack(ctx, fixture.baseline.ContractID, fixture.baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fixture.baseline) {
		t.Fatalf("mismatched baseline changed last-known-good track: got=%#v want=%#v", got, fixture.baseline)
	}
}

func TestFileStoreRejectsAuthenticatedSchemaVersionDowngrade(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, false)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	}); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(root, "operational", "state.json")
	state := readOperationalStateJSON(t, root)
	state["version"] = float64(decisionOperationalStateVersion)
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := durableAtomicWrite(statePath, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "operational", "schema.json")); err != nil {
		t.Fatal(err)
	}
	downgraded, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileStore(root).ReleaseTrack(ctx, fixture.next.ContractID, fixture.next.ID); err == nil {
		t.Fatal("authenticated state masqueraded as a legacy schema")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, downgraded) {
		t.Fatal("rejected schema downgrade rewrote operational state")
	}
}

func TestFileStoreRecoversCurrentSchemaMarkerAfterInterruptedMarkerPublish(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	if err := store.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: "payments"}); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(root, "operational", "schema.json")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("current operational state has no durable schema marker: %v", err)
	}
	if err := os.Remove(markerPath); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(root).ContractRevision(ctx, "payments", "revision"); err != nil {
		t.Fatalf("recover current state after interrupted marker publish: %v", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("schema marker was not recovered: %v", err)
	}
}

func TestFileStoreMigratesAuthenticatedV3WithoutLosingAuthority(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, false)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	}); err != nil {
		t.Fatal(err)
	}

	state := readOperationalStateJSON(t, root)
	state["version"] = float64(authenticatedStateVersion)
	legacyRevisions := make(map[string]any)
	for _, raw := range state["revisions"].(map[string]any) {
		revision := raw.(map[string]any)
		legacyRevisions[revision["ID"].(string)] = revision
	}
	state["revisions"] = legacyRevisions
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := durableAtomicWrite(
		filepath.Join(root, "operational", "state.json"),
		append(encoded, '\n'),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "operational", "schema.json")); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, fixture.next.ContractID, fixture.next.ID)
	if err != nil {
		t.Fatalf("migrate authenticated v3 track: %v", err)
	}
	if !reflect.DeepEqual(got, fixture.next) {
		t.Fatalf("authenticated v3 migration changed decision authority: got=%#v want=%#v", got, fixture.next)
	}
	evidence, err := restarted.ReleaseEvidence(
		ctx,
		fixture.authorization.ContractID,
		fixture.authorization.TrackID,
		fixture.authorization.ReviewID,
	)
	if err != nil {
		t.Fatalf("load migrated authenticated evidence: %v", err)
	}
	if !reflect.DeepEqual(evidence.Authorization, fixture.authorization) {
		t.Fatalf("migrated authorization = %#v, want %#v", evidence.Authorization, fixture.authorization)
	}
	if state := readOperationalStateJSON(t, root); state["version"] != float64(operationalStateVersion) {
		t.Fatalf("migrated authenticated state version = %#v", state["version"])
	}
}

func TestFileStoreAuthenticatedV3MigrationIgnoresUnavailableNonAuthoritativeSyncHistory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	currentKey, err := store.Put(ctx, []byte("healthy last-known-good"))
	if err != nil {
		t.Fatal(err)
	}
	missingHistoricalKey := port.ContentAddressedBlobKey([]byte("missing historical sync blob"))
	state := newOperationalState()
	state.Version = authenticatedStateVersion
	state.Revisions["revision-current"] = core.ContractRevision{
		ID: "revision-current", ContractID: "payments", SpecBlobKey: string(currentKey),
	}
	state.Revisions["revision-historical"] = core.ContractRevision{
		ID: "revision-historical", ContractID: "payments", SpecBlobKey: string(missingHistoricalKey),
	}
	track := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: "revision-current", Generation: 4,
	}
	trackKey := releaseTrackKey(track.ContractID, track.ID)
	state.ReleaseTracks[trackKey] = track
	state.ReleaseTrackAuthorities[trackKey] = newReleaseTrackAuthority(track)
	state.Publications[publicationKey("payments", "revision-current")] = core.Publication{
		ProjectID: "payments", RevisionID: "revision-current", Public: true, Path: "/payments/stable",
	}
	state.SyncRecords["sync-historical"] = core.SyncRecord{
		ID: "sync-historical", ProjectID: "payments", SourceID: "payments-git",
		RevisionID: "revision-historical", Result: core.SyncResultSuccess,
	}
	if err := store.publishOperationalState(ctx, state); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	publication, err := restarted.PublicPublicationByPath(ctx, "/payments/stable")
	if err != nil {
		t.Fatalf("non-authoritative sync history blocked last-known-good migration: %v", err)
	}
	if publication.RevisionID != "revision-current" {
		t.Fatalf("public revision = %q, want revision-current", publication.RevisionID)
	}
	if _, err := restarted.ContractRevision(ctx, "payments", "revision-historical"); err == nil {
		t.Fatal("historical revision point read ignored its missing blob")
	}
	migrated := readOperationalStateJSON(t, root)
	if migrated["version"] != float64(operationalStateVersion) {
		t.Fatalf("migrated state version = %#v", migrated["version"])
	}
}

func TestFileStoreLegacyMigrationReconcilesDuplicatePublicPathToTrackCurrent(t *testing.T) {
	ctx := context.Background()
	for _, version := range []int{legacyOperationalStateVersion, decisionOperationalStateVersion, authenticatedStateVersion} {
		for _, order := range []string{"forward", "reverse"} {
			t.Run("v"+string(rune('0'+version))+"/"+order, func(t *testing.T) {
				root := t.TempDir()
				store := NewFileStore(root)
				state := duplicateLegacyPublicPathState(t, ctx, store, version, order)
				if err := store.publishOperationalState(ctx, state); err != nil {
					t.Fatal(err)
				}

				restarted := NewFileStore(root)
				for attempt := 0; attempt < 32; attempt++ {
					publication, err := restarted.PublicPublicationByPath(ctx, "/payments")
					if err != nil {
						t.Fatalf("read reconciled public path: %v", err)
					}
					if publication.RevisionID != "rev-b" {
						t.Fatalf("public path selected %q, want track current rev-b", publication.RevisionID)
					}
					restarted = NewFileStore(root)
				}
				migrated, err := restarted.loadOperationalState(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if migrated.Publications[publicationKey("payments", "rev-a")].Public {
					t.Fatal("legacy migration left losing publication public")
				}
				if !migrated.Publications[publicationKey("payments", "rev-b")].Public {
					t.Fatal("legacy migration demoted demonstrable last-known-good publication")
				}
			})
		}
	}
}

func TestFileStoreLegacyMigrationRejectsAmbiguousDuplicatePublicPathRecoverably(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	state := newOperationalState()
	state.Version = authenticatedStateVersion
	state.Revisions["rev-a"] = core.ContractRevision{ID: "rev-a", ContractID: "payments"}
	state.Revisions["rev-b"] = core.ContractRevision{ID: "rev-b", ContractID: "payments"}
	state.Publications[publicationKey("payments", "rev-a")] = core.Publication{
		ProjectID: "payments", RevisionID: "rev-a", Public: true, Path: "/payments",
	}
	state.Publications[publicationKey("payments", "rev-b")] = core.Publication{
		ProjectID: "payments", RevisionID: "rev-b", Public: true, Path: "/payments",
	}
	if err := store.publishOperationalState(ctx, state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(root).PublicPublicationByPath(ctx, "/payments"); err == nil {
		t.Fatal("ambiguous legacy public path was selected by map iteration")
	}
	after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed duplicate-path migration changed recoverable legacy state")
	}
	if _, err := os.Stat(filepath.Join(root, "operational", "schema.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed duplicate-path migration published a schema marker: %v", err)
	}
}

func duplicateLegacyPublicPathState(
	t *testing.T,
	ctx context.Context,
	store *FileStore,
	version int,
	order string,
) operationalState {
	t.Helper()
	state := newOperationalState()
	state.Version = version
	for _, revisionID := range []string{"rev-a", "rev-b"} {
		key, err := store.Put(ctx, []byte("spec "+revisionID))
		if err != nil {
			t.Fatal(err)
		}
		state.Revisions[revisionID] = core.ContractRevision{
			ID: revisionID, ContractID: "payments", SpecBlobKey: string(key),
		}
	}
	track := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: "rev-b", Generation: 9,
	}
	trackKey := releaseTrackKey(track.ContractID, track.ID)
	state.ReleaseTracks[trackKey] = track
	if version == authenticatedStateVersion {
		state.ReleaseTrackAuthorities[trackKey] = newReleaseTrackAuthority(track)
	}
	publications := []core.Publication{
		{ProjectID: "payments", RevisionID: "rev-a", Public: true, Path: "/payments"},
		{ProjectID: "payments", RevisionID: "rev-b", Public: true, Path: "/payments"},
	}
	if order == "reverse" {
		publications[0], publications[1] = publications[1], publications[0]
	}
	for _, publication := range publications {
		state.Publications[publicationKey(publication.ProjectID, publication.RevisionID)] = publication
	}
	return state
}

func TestFileStoreRejectsInvalidUTF8BeforeCommitAndWhileLoadingState(t *testing.T) {
	ctx := context.Background()
	invalidIdentities := []string{
		string([]byte("payments-\xff")),
		string([]byte("payments-\xfe")),
	}
	for _, identity := range invalidIdentities {
		if utf8.ValidString(identity) {
			t.Fatal("test identity unexpectedly contains valid UTF-8")
		}
		for name, operation := range map[string]func(*FileStore) error{
			"project id": func(store *FileStore) error {
				return store.SaveProject(ctx, core.Project{ID: identity})
			},
			"project source id": func(store *FileStore) error {
				return store.SaveProject(ctx, core.Project{ID: "payments", SourceIDs: []string{identity}})
			},
			"revision id": func(store *FileStore) error {
				return store.SaveRevision(ctx, core.ContractRevision{ID: identity, ContractID: "payments"})
			},
			"revision contract id": func(store *FileStore) error {
				return store.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: identity})
			},
			"revision source id": func(store *FileStore) error {
				return store.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: "payments", SourceID: identity})
			},
			"revision ref": func(store *FileStore) error {
				return store.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: "payments", Ref: identity})
			},
			"sync id": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: identity, ProjectID: "payments"})
			},
			"sync project id": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: "sync", ProjectID: identity})
			},
			"sync source id": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: "sync", ProjectID: "payments", SourceID: identity})
			},
			"sync revision id": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: "sync", ProjectID: "payments", RevisionID: identity})
			},
			"sync trigger": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: "sync", ProjectID: "payments", Trigger: identity})
			},
			"sync ref": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: "sync", ProjectID: "payments", Ref: identity})
			},
			"sync commit": func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{ID: "sync", ProjectID: "payments", CommitSHA: identity})
			},
		} {
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				if err := operation(NewFileStore(root)); err == nil {
					t.Fatal("invalid UTF-8 identity reached durable persistence")
				}
				if _, err := os.Stat(filepath.Join(root, "operational", "state.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("rejected invalid identity persisted operational state: %v", err)
				}
			})
		}
	}

	root := t.TempDir()
	state := newOperationalState()
	state.Version = operationalStateVersion
	for _, contractID := range []string{"payments-X", "payments-Y"} {
		revision := core.ContractRevision{ID: "revision", ContractID: contractID}
		state.Revisions[revisionStorageKey(revision)] = revision
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = bytes.ReplaceAll(encoded, []byte("payments-X"), []byte("payments-\xff"))
	encoded = bytes.ReplaceAll(encoded, []byte("payments-Y"), []byte("payments-\xfe"))
	if utf8.Valid(encoded) {
		t.Fatal("crafted operational snapshot unexpectedly contains valid UTF-8")
	}
	if err := os.MkdirAll(filepath.Join(root, "operational"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := durableAtomicWrite(filepath.Join(root, "operational", "state.json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(root).loadOperationalState(ctx); err == nil {
		t.Fatal("invalid UTF-8 snapshot was normalized into colliding persisted identities")
	}
}

func TestFileStoreLegacySchemaValidatesAuthoritativeBlobReferencesBeforeUpgrade(t *testing.T) {
	ctx := context.Background()
	roles := []string{"track current", "publication"}
	for _, version := range []int{legacyOperationalStateVersion, decisionOperationalStateVersion} {
		for _, role := range roles {
			t.Run(role+"/v"+string(rune('0'+version)), func(t *testing.T) {
				root := t.TempDir()
				store := NewFileStore(root)
				state := newOperationalState()
				state.Version = version
				missingKey := port.ContentAddressedBlobKey([]byte("missing legacy " + role))
				state.Revisions["referenced"] = core.ContractRevision{
					ID: "referenced", ContractID: "payments", SpecBlobKey: string(missingKey),
				}
				state.Revisions["other"] = core.ContractRevision{ID: "other", ContractID: "payments"}
				addLegacyOperationalReference(t, &state, role)
				if err := store.publishOperationalState(ctx, state); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := NewFileStore(root).loadOperationalState(ctx); err == nil {
					t.Fatalf("v%d migration accepted missing blob referenced by %s", version, role)
				}
				after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("failed legacy blob validation published an upgraded state")
				}
			})
		}
	}
}

func TestFileStoreLegacyMigrationDiscardsUnavailableRejectedCandidate(t *testing.T) {
	ctx := context.Background()
	for _, version := range []int{legacyOperationalStateVersion, decisionOperationalStateVersion} {
		t.Run("v"+string(rune('0'+version)), func(t *testing.T) {
			root := t.TempDir()
			store := NewFileStore(root)
			currentKey, err := store.Put(ctx, []byte("healthy current revision"))
			if err != nil {
				t.Fatal(err)
			}
			missingCandidateKey := port.ContentAddressedBlobKey([]byte("discarded rejected candidate"))
			state := newOperationalState()
			state.Version = version
			state.Revisions["revision-current"] = core.ContractRevision{
				ID: "revision-current", ContractID: "payments", SpecBlobKey: string(currentKey),
			}
			state.Revisions["revision-rejected"] = core.ContractRevision{
				ID: "revision-rejected", ContractID: "payments", SpecBlobKey: string(missingCandidateKey),
			}
			decision := core.ReleaseDecision{
				RevisionID: "revision-rejected", ReviewID: "review-untrusted",
				ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Verdict:      core.VerdictFail, Accepted: false,
				EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			}
			track := core.ReleaseTrack{
				ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
				CurrentRevisionID: "revision-current", CandidateRevisionID: "revision-rejected",
				LastDecision: &decision, Generation: 7,
			}
			trackKey := releaseTrackKey(track.ContractID, track.ID)
			state.ReleaseTracks[trackKey] = track
			if version == decisionOperationalStateVersion {
				state.ReleaseTrackAuthorities[trackKey] = newReleaseTrackAuthority(track)
			}
			state.Publications[publicationKey("payments", "revision-current")] = core.Publication{
				ProjectID: "payments", RevisionID: "revision-current", Public: true, Path: "/payments/stable",
			}
			state.SyncRecords["sync-rejected"] = core.SyncRecord{
				ID: "sync-rejected", ProjectID: "payments", SourceID: "payments-git",
				RevisionID: "revision-rejected", Result: core.SyncResultSuccess,
			}
			state.Reviews["review-untrusted"] = core.ContractReview{
				ID: "review-untrusted", ContractID: "payments",
				BaselineRevisionID: "revision-current", CandidateRevisionID: "revision-rejected",
			}
			state.AuditEvents["audit-rejected"] = core.AuditEvent{
				ID: "audit-rejected", ContractID: "payments", TrackID: "stable",
				RevisionID: "revision-rejected", Kind: "legacy.rejected",
			}
			state.Outbox["outbox-rejected"] = core.OutboxMessage{
				ID: "outbox-rejected", ContractID: "payments", TrackID: "stable",
				RevisionID: "revision-rejected", Topic: "legacy.rejected",
			}
			if err := store.publishOperationalState(ctx, state); err != nil {
				t.Fatal(err)
			}

			restarted := NewFileStore(root)
			publication, err := restarted.PublicPublicationByPath(ctx, "/payments/stable")
			if err != nil {
				t.Fatalf("discarded candidate blocked healthy public route: %v", err)
			}
			if publication.RevisionID != "revision-current" {
				t.Fatalf("public revision = %q, want revision-current", publication.RevisionID)
			}
			migrated, err := restarted.ReleaseTrack(ctx, "payments", "stable")
			if err != nil {
				t.Fatal(err)
			}
			if migrated.CurrentRevisionID != "revision-current" || migrated.CandidateRevisionID != "" || migrated.LastDecision != nil {
				t.Fatalf("legacy migration retained unauthenticated candidate authority: %#v", migrated)
			}
			if _, err := restarted.ContractRevision(ctx, "payments", "revision-rejected"); err == nil {
				t.Fatal("historical rejected candidate point read ignored missing immutable blob")
			}
			if _, err := restarted.ReleaseEvidence(ctx, "payments", "stable", "review-untrusted"); err == nil {
				t.Fatal("discarded legacy review became future release authority")
			}
		})
	}
}

func TestFileStoreAuthenticatedV3MigrationValidatesEveryRetainedBlobBeforeUpgrade(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name   string
		target func(storedReleaseFixture) string
		mutate func(string) error
	}{
		{
			name: "track current missing",
			target: func(fixture storedReleaseFixture) string {
				return fixture.authorization.BaselineRevisionID
			},
			mutate: os.Remove,
		},
		{
			name:   "public revision corrupt",
			target: func(storedReleaseFixture) string { return "revision-public" },
			mutate: func(path string) error {
				return os.WriteFile(path, []byte("corrupt public blob"), 0o600)
			},
		},
		{
			name: "authority candidate missing",
			target: func(fixture storedReleaseFixture) string {
				return fixture.authorization.CandidateRevisionID
			},
			mutate: os.Remove,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewFileStore(root)
			fixture := newStoredReleaseFixture(t, store, false)
			persistStoredReleaseBaseline(t, ctx, store, fixture, true)
			publicKey, err := store.Put(ctx, []byte("retained public revision"))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SaveRevision(ctx, core.ContractRevision{
				ID: "revision-public", ContractID: "payments", SpecBlobKey: string(publicKey),
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.SavePublication(ctx, core.Publication{
				ProjectID: "payments", RevisionID: "revision-public", Public: true, Path: "/payments/public",
			}); err != nil {
				t.Fatal(err)
			}

			state, err := store.loadOperationalState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			state.Version = authenticatedStateVersion
			flat := make(map[string]core.ContractRevision, len(state.Revisions))
			for _, revision := range state.Revisions {
				flat[revision.ID] = revision
			}
			state.Revisions = flat
			if err := store.publishOperationalState(ctx, state); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(root, "operational", "schema.json")
			markerBefore := []byte("{\n  \"version\": 3\n}\n")
			if err := durableAtomicWrite(markerPath, markerBefore, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			revision := state.Revisions[test.target(fixture)]
			blobPath, err := store.blobPath(port.BlobKey(revision.SpecBlobKey))
			if err != nil {
				t.Fatal(err)
			}
			originalBlob, err := os.ReadFile(blobPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.mutate(blobPath); err != nil {
				t.Fatal(err)
			}

			if _, err := NewFileStore(root).loadOperationalState(ctx); err == nil {
				t.Fatal("authenticated v3 migration published state with unavailable retained evidence")
			}
			after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("failed authenticated v3 migration advanced operational state")
			}
			markerAfter, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(markerAfter, markerBefore) {
				t.Fatalf("failed authenticated v3 migration advanced marker: got=%q want=%q", markerAfter, markerBefore)
			}

			if err := durableAtomicWrite(blobPath, originalBlob, 0o600); err != nil {
				t.Fatal(err)
			}
			recovered, err := NewFileStore(root).loadOperationalState(ctx)
			if err != nil {
				t.Fatalf("recover authenticated v3 migration after evidence repair: %v", err)
			}
			if recovered.Version != operationalStateVersion {
				t.Fatalf("recovered schema version = %d, want %d", recovered.Version, operationalStateVersion)
			}
			marker, present, err := NewFileStore(root).loadOperationalSchemaMarker(ctx)
			if err != nil || !present || marker.Version != operationalStateVersion {
				t.Fatalf("recovered marker = %#v present=%t err=%v", marker, present, err)
			}
		})
	}
}

func TestFileStoreSameHandleAdmissionHonorsContextDeadline(t *testing.T) {
	store := NewFileStore(t.TempDir())
	entered := make(chan struct{})
	release := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- store.Within(context.Background(), func(context.Context, port.OperationalStore) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	secondResult := make(chan error, 1)
	started := time.Now()
	go func() {
		secondResult <- store.Within(ctx, func(context.Context, port.OperationalStore) error {
			return errors.New("callback ran without in-process admission")
		})
	}()
	select {
	case err := <-secondResult:
		if !errors.Is(err, context.DeadlineExceeded) {
			close(release)
			<-firstResult
			t.Fatalf("contended same-handle error = %v, want deadline exceeded", err)
		}
		if elapsed := time.Since(started); elapsed > 120*time.Millisecond {
			close(release)
			<-firstResult
			t.Fatalf("same-handle deadline returned after %s", elapsed)
		}
	case <-time.After(150 * time.Millisecond):
		close(release)
		<-firstResult
		<-secondResult
		t.Fatal("same-handle admission ignored context deadline")
	}
	close(release)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}
}

func TestFileStoreSyncLogicalReplayPreservesFirstObservation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	first := core.SyncRecord{
		ID: "sync-stable", ProjectID: "payments", SourceID: "payments-git",
		RevisionID: "", Trigger: "webhook", Ref: "refs/heads/main", CommitSHA: "abc123",
		Result: core.SyncResultFailure, ErrorSummary: "source unavailable",
		StartedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 7, 25, 12, 0, 1, 0, time.UTC),
	}
	if err := store.SaveSyncRecord(ctx, first); err != nil {
		t.Fatal(err)
	}
	replay := first
	replay.StartedAt = replay.StartedAt.Add(time.Hour)
	replay.FinishedAt = replay.FinishedAt.Add(time.Hour)
	if err := store.SaveSyncRecord(ctx, replay); err != nil {
		t.Fatalf("timestamp-only logical replay conflicted: %v", err)
	}
	persisted, err := NewFileStore(root).SyncRecord(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, first) {
		t.Fatalf("logical replay replaced first observation: got=%#v want=%#v", persisted, first)
	}
	conflict := replay
	conflict.ErrorSummary = "different failure"
	if err := store.SaveSyncRecord(ctx, conflict); err == nil {
		t.Fatal("conflicting logical sync evidence was accepted")
	}
	persisted, err = NewFileStore(root).SyncRecord(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, first) {
		t.Fatal("conflicting logical replay changed durable evidence")
	}
}

func addLegacyOperationalReference(t *testing.T, state *operationalState, role string) {
	t.Helper()
	switch role {
	case "track current":
		state.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
			ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
			CurrentRevisionID: "referenced",
		}
	case "publication":
		state.Publications[publicationKey("payments", "referenced")] = core.Publication{
			ProjectID: "payments", RevisionID: "referenced", Public: true, Path: "/payments",
		}
	case "successful sync":
		state.SyncRecords["sync"] = core.SyncRecord{
			ID: "sync", ProjectID: "payments", RevisionID: "referenced", Result: core.SyncResultSuccess,
		}
	case "review baseline":
		state.Reviews["review"] = core.ContractReview{
			ID: "review", ContractID: "payments", BaselineRevisionID: "referenced", CandidateRevisionID: "other",
		}
	case "review candidate":
		state.Reviews["review"] = core.ContractReview{
			ID: "review", ContractID: "payments", BaselineRevisionID: "other", CandidateRevisionID: "referenced",
		}
	case "audit":
		state.AuditEvents["audit"] = core.AuditEvent{
			ID: "audit", ContractID: "payments", RevisionID: "referenced", Kind: "legacy",
		}
	case "outbox":
		state.Outbox["outbox"] = core.OutboxMessage{
			ID: "outbox", ContractID: "payments", RevisionID: "referenced", Topic: "legacy",
		}
	default:
		t.Fatalf("unknown legacy reference role %q", role)
	}
}

func TestFileStoreScopesEqualRevisionIDsByContractAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	payments := core.ContractRevision{ID: "shared", ContractID: "payments", SourceID: "payments-source", Ref: "main"}
	orders := core.ContractRevision{ID: "shared", ContractID: "orders", SourceID: "orders-source", Ref: "stable"}
	for _, revision := range []core.ContractRevision{payments, orders} {
		if err := store.SaveRevision(ctx, revision); err != nil {
			t.Fatalf("save %s revision: %v", revision.ContractID, err)
		}
		if err := store.SavePublication(ctx, core.Publication{
			ProjectID: revision.ContractID, RevisionID: revision.ID,
			Path: "/" + revision.ContractID, Public: true,
		}); err != nil {
			t.Fatalf("save %s publication: %v", revision.ContractID, err)
		}
	}

	restarted := NewFileStore(root)
	for _, want := range []core.ContractRevision{payments, orders} {
		got, err := restarted.ContractRevision(ctx, want.ContractID, want.ID)
		if err != nil {
			t.Fatalf("load %s revision after restart: %v", want.ContractID, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s revision = %#v, want %#v", want.ContractID, got, want)
		}
	}
	if _, err := restarted.Revision(ctx, "shared"); err == nil {
		t.Fatal("legacy unscoped revision lookup selected one of two contracts")
	}
}

func TestFileStorePublicReadIgnoresUnrelatedRejectedCandidateBlobFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	rejected := newStoredReleaseFixtureNamed(
		t, false, "review-rejected", "sync-rejected", time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	)
	persistStoredReleaseBaseline(t, ctx, store, rejected, true)
	accepted := newStoredReleaseFixtureNamed(
		t, true, "review-accepted", "sync-accepted", time.Date(2026, 7, 25, 12, 1, 0, 0, time.UTC),
	)
	persistStoredReleaseBaseline(t, ctx, store, accepted, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, rejected.baseline.Generation, rejected.next); err != nil {
			return err
		}
		if err := appendStoredReleaseEffects(ctx, operational, rejected); err != nil {
			return err
		}
		return operational.SavePublication(ctx, core.Publication{
			ProjectID:  rejected.baseline.ContractID,
			RevisionID: rejected.baseline.CurrentRevisionID,
			Public:     true, Path: "/payments/stable",
		})
	}); err != nil {
		t.Fatal(err)
	}
	candidate, err := store.ContractRevision(ctx, rejected.authorization.ContractID, rejected.authorization.CandidateRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	candidatePath, err := store.blobPath(port.BlobKey(candidate.SpecBlobKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(candidatePath); err != nil {
		t.Fatal(err)
	}

	publication, err := NewFileStore(root).PublicPublicationByPath(ctx, "/payments/stable")
	if err != nil {
		t.Fatalf("unrelated rejected candidate broke last-known-good public read: %v", err)
	}
	if publication.RevisionID != rejected.baseline.CurrentRevisionID {
		t.Fatalf("public read revision = %q, want %q", publication.RevisionID, rejected.baseline.CurrentRevisionID)
	}
	if _, err := NewFileStore(root).ReleaseEvidence(
		ctx,
		rejected.authorization.ContractID,
		rejected.authorization.TrackID,
		rejected.authorization.ReviewID,
	); err == nil {
		t.Fatal("corrupt rejected-candidate evidence remained authorizable")
	}

	accepted.next, _, err = core.ConsiderReleaseDecision(rejected.next, *accepted.next.LastDecision)
	if err != nil {
		t.Fatalf("derive later accepted decision: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = NewFileStore(root).Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, rejected.next.Generation, accepted.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, accepted)
	})
	if err == nil {
		t.Fatal("direct transition accepted corrupt candidate evidence")
	}
	after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed transition with corrupt evidence changed persisted state")
	}
	got, err := NewFileStore(root).ReleaseTrack(ctx, rejected.next.ContractID, rejected.next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, rejected.next) {
		t.Fatalf("failed transition changed rejected track: got=%#v want=%#v", got, rejected.next)
	}
}

func TestFileStoreRejectsControlCharactersAndMismatchedLookupIdentity(t *testing.T) {
	ctx := context.Background()
	bad := "safe\x00collision"
	for name, operation := range map[string]func(*FileStore) error{
		"project id": func(store *FileStore) error {
			return store.SaveProject(ctx, core.Project{ID: bad})
		},
		"revision id": func(store *FileStore) error {
			return store.SaveRevision(ctx, core.ContractRevision{ID: bad, ContractID: "payments"})
		},
		"revision contract": func(store *FileStore) error {
			return store.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: bad})
		},
		"revision ref": func(store *FileStore) error {
			return store.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: "payments", Ref: bad})
		},
		"track delimiter collision": func(store *FileStore) error {
			return store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SaveReleaseTrack(ctx, 0, core.ReleaseTrack{
					ID: "stable", ContractID: bad, Mode: core.ReleaseModePinned,
				})
			})
		},
		"publication path": func(store *FileStore) error {
			return store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				if err := operational.SaveRevision(ctx, core.ContractRevision{ID: "revision", ContractID: "payments"}); err != nil {
					return err
				}
				return operational.SavePublication(ctx, core.Publication{
					ProjectID: "payments", RevisionID: "revision", Path: "/payments/\x00stable",
				})
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(NewFileStore(t.TempDir())); err == nil {
				t.Fatal("control-bearing identity was accepted")
			}
		})
	}

	for name, check := range map[string]func(*FileStore) error{
		"project": func(store *FileStore) error {
			if err := store.writeJSON(ctx, "projects", "requested.json", core.Project{ID: "stored"}); err != nil {
				return err
			}
			_, err := store.Project(ctx, "requested")
			return err
		},
		"legacy revision": func(store *FileStore) error {
			if err := store.writeJSON(ctx, "revisions", "requested.json", core.ContractRevision{ID: "stored"}); err != nil {
				return err
			}
			_, err := store.Revision(ctx, "requested")
			return err
		},
		"legacy sync": func(store *FileStore) error {
			if err := store.writeJSON(ctx, "sync-history", "requested.json", core.SyncRecord{ID: "stored"}); err != nil {
				return err
			}
			_, err := store.SyncRecord(ctx, "requested")
			return err
		},
	} {
		t.Run("lookup mismatch/"+name, func(t *testing.T) {
			if err := check(NewFileStore(t.TempDir())); err == nil {
				t.Fatal("lookup returned a record with a different persisted identity")
			}
		})
	}
}
