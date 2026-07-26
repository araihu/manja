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
	if err := writeFileAtomically(statePath, append(encoded, '\n'), 0o600); err != nil {
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
	if err := writeFileAtomically(
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

func TestFileStoreLegacySchemaValidatesEveryReferencedBlobBeforeUpgrade(t *testing.T) {
	ctx := context.Background()
	roles := []string{
		"track current",
		"track candidate",
		"publication",
		"successful sync",
		"review baseline",
		"review candidate",
		"audit",
		"outbox",
	}
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

func addLegacyOperationalReference(t *testing.T, state *operationalState, role string) {
	t.Helper()
	switch role {
	case "track current":
		state.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
			ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
			CurrentRevisionID: "referenced",
		}
	case "track candidate":
		decision := core.ReleaseDecision{
			RevisionID: "referenced", ReviewID: "legacy-review",
			ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Verdict:      core.VerdictFail, EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		}
		state.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
			ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
			CandidateRevisionID: "referenced", LastDecision: &decision, Generation: 1,
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
