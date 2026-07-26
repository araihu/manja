package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	contracttest.RevisionReader(t, func(t testing.TB) contracttest.RevisionReaderFixture {
		store := NewFileStore(t.TempDir())
		revision := core.ContractRevision{
			ID: "revision-1", ContractID: "payments", SourceID: "source-main", Ref: "main",
		}
		if err := store.SaveRevision(context.Background(), revision); err != nil {
			t.Fatalf("seed revision reader: %v", err)
		}
		return contracttest.RevisionReaderFixture{
			Reader: store, ContractID: revision.ContractID, RevisionID: revision.ID, Want: revision,
		}
	})
}

func TestFileStoreReleaseTrackAliasesCannotMutatePersistedStateAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	decision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictFail, EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	want := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 1, CurrentRevisionID: "revision-good",
		CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: "payments"}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, want)
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		returned, err := operational.ReleaseTrack(ctx, want.ContractID, want.ID)
		if err != nil {
			return err
		}
		returned.LastDecision.Accepted = true
		returned.LastDecision.Verdict = core.VerdictPass
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		input := core.CloneReleaseTrack(want)
		if err := operational.SaveReleaseTrack(ctx, want.Generation, input); err != nil {
			return err
		}
		input.LastDecision.Accepted = true
		input.LastDecision.Verdict = core.VerdictPass
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, want.ContractID, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aliased mutation survived restart: got=%#v want=%#v", got, want)
	}
	state, err := restarted.loadOperationalState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Revisions) != 2 || len(state.ReleaseTracks) != 1 ||
		len(state.Reviews) != 0 || len(state.SyncRecords) != 0 || len(state.Publications) != 0 ||
		len(state.AuditEvents) != 0 || len(state.Outbox) != 0 {
		t.Fatalf("alias-only transactions created side effects: %#v", state)
	}
	if state.ReleaseTracks[releaseTrackKey(want.ContractID, want.ID)].Generation != want.Generation {
		t.Fatalf("alias-only transactions changed generation: %#v", state.ReleaseTracks)
	}
}

func TestFileStoreRejectsStrippedReleaseDecisionEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	decision := core.ReleaseDecision{
		RevisionID: "revision-current", ReviewID: "review-accepted",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	baseline := core.ReleaseTrack{ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned}
	accepted, changed, err := core.ConsiderReleaseDecision(baseline, decision)
	if err != nil || !changed {
		t.Fatalf("derive accepted candidate: changed=%t err=%v", changed, err)
	}
	track, err := core.PromoteReleaseRevision(accepted, decision.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveRevision(ctx, core.ContractRevision{ID: decision.RevisionID, ContractID: track.ContractID}); err != nil {
			return err
		}
		if err := operational.SaveReleaseTrack(ctx, 0, baseline); err != nil {
			return err
		}
		if err := operational.SaveReleaseTrack(ctx, baseline.Generation, accepted); err != nil {
			return err
		}
		return operational.SaveReleaseTrack(ctx, accepted.Generation, track)
	}); err != nil {
		t.Fatal(err)
	}

	err = store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		stripped, err := operational.ReleaseTrack(ctx, track.ContractID, track.ID)
		if err != nil {
			return err
		}
		stripped.LastDecision = nil
		return operational.SaveReleaseTrack(ctx, track.Generation, stripped)
	})
	if err == nil {
		t.Fatal("stripped release decision evidence was committed as legacy state")
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, track.ContractID, track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, track) {
		t.Fatalf("failed stripping attempt changed persisted track: got=%#v want=%#v", got, track)
	}
}

func TestFileStoreRejectsSupersededReleaseDecisionEvidence(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	newerDecision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-newer",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictFail, EvaluatedAt: evaluatedAt.Add(time.Minute),
	}
	track := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 1, CurrentRevisionID: "revision-good",
		CandidateRevisionID: newerDecision.RevisionID, LastDecision: &newerDecision,
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: track.ContractID}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, track)
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name        string
		evaluatedAt time.Time
	}{
		{name: "older", evaluatedAt: evaluatedAt},
		{name: "equal time different identity", evaluatedAt: newerDecision.EvaluatedAt},
	} {
		t.Run(test.name, func(t *testing.T) {
			acceptedDecision := core.ReleaseDecision{
				RevisionID: "revision-next", ReviewID: "review-superseded-" + test.name,
				ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Verdict:      core.VerdictPass, Accepted: true, EvaluatedAt: test.evaluatedAt,
			}
			superseded := core.CloneReleaseTrack(track)
			superseded.Generation++
			superseded.LastDecision = &acceptedDecision
			if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SaveReleaseTrack(ctx, track.Generation, superseded)
			}); err == nil {
				t.Fatal("superseded release decision evidence was committed")
			}
		})
	}
}

func TestFileStoreFailsClosedOnMalformedLoadedReleaseTrack(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	state := newOperationalState()
	state.Revisions["revision-next"] = core.ContractRevision{ID: "revision-next", ContractID: "payments"}
	state.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 3, CandidateRevisionID: "revision-next",
	}
	if err := store.publishOperationalState(ctx, state); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	if _, err := restarted.ReleaseTrack(ctx, "payments", "stable"); err == nil {
		t.Fatal("malformed loaded release track was treated as legacy state")
	}
	if err := restarted.Within(ctx, func(context.Context, port.OperationalStore) error { return nil }); err == nil {
		t.Fatal("malformed loaded release track was recommitted")
	}
}

func TestFileStoreFinalCommitRejectsBypassedReleaseTransition(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	decision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictFail, EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	want := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 1, CurrentRevisionID: "revision-good",
		CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: want.ContractID}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, want)
	}); err != nil {
		t.Fatal(err)
	}

	err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReview(ctx, core.ContractReview{
			ID: "review-side-effect", ContractID: want.ContractID,
			BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
		}); err != nil {
			return err
		}
		if err := operational.SaveSyncRecord(ctx, core.SyncRecord{
			ID: "sync-side-effect", ProjectID: want.ContractID, RevisionID: "revision-next", Result: core.SyncResultSuccess,
		}); err != nil {
			return err
		}
		if err := operational.SavePublication(ctx, core.Publication{
			ProjectID: want.ContractID, RevisionID: "revision-good", Public: true, Path: "/payments/stable",
		}); err != nil {
			return err
		}
		if err := operational.AppendAuditEvent(ctx, core.AuditEvent{
			ID: "audit-side-effect", ContractID: want.ContractID, TrackID: want.ID, RevisionID: "revision-next",
		}); err != nil {
			return err
		}
		if err := operational.Enqueue(ctx, core.OutboxMessage{
			ID: "outbox-side-effect", ContractID: want.ContractID, TrackID: want.ID, RevisionID: "revision-next",
		}); err != nil {
			return err
		}
		transaction := operational.(*operationalTransaction)
		forged := core.CloneReleaseTrack(want)
		forged.CurrentRevisionID = forged.CandidateRevisionID
		forged.Generation = 0
		transaction.state.ReleaseTracks[releaseTrackKey(want.ContractID, want.ID)] = forged
		return nil
	})
	if err == nil {
		t.Fatal("final commit accepted a bypassed rejected transition")
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, want.ContractID, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bypassed transition changed last known good track: got=%#v want=%#v", got, want)
	}
	state, err := restarted.loadOperationalState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Reviews) != 0 || len(state.SyncRecords) != 0 || len(state.Publications) != 0 ||
		len(state.AuditEvents) != 0 || len(state.Outbox) != 0 {
		t.Fatalf("failed transition leaked side effects: %#v", state)
	}
}

func TestFileStoreMigratesV1ReleaseAuthorityOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	legacy := newOperationalState()
	legacy.Version = 1
	legacy.Revisions["revision-good"] = core.ContractRevision{ID: "revision-good", ContractID: "payments"}
	legacy.Revisions["revision-unauthenticated"] = core.ContractRevision{ID: "revision-unauthenticated", ContractID: "payments"}
	legacy.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 5, CurrentRevisionID: "revision-good", CandidateRevisionID: "revision-unauthenticated",
	}
	if err := store.publishOperationalState(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	got, err := store.ReleaseTrack(ctx, "payments", "stable")
	if err != nil {
		t.Fatalf("migrate v1 release track: %v", err)
	}
	if got.Generation != 5 || got.CurrentRevisionID != "revision-good" || got.CandidateRevisionID != "" || got.LastDecision != nil {
		t.Fatalf("migrated v1 track = %#v", got)
	}
	first := readOperationalStateJSON(t, root)
	if first["version"] != float64(2) {
		t.Fatalf("migrated state version = %#v, want 2", first["version"])
	}
	authorities, ok := first["releaseTrackAuthorities"].(map[string]any)
	if !ok {
		t.Fatalf("migrated state lacks release authority map: %#v", first)
	}
	authority, ok := authorities[releaseTrackKey("payments", "stable")].(map[string]any)
	if !ok || authority["version"] != float64(1) || authority["generation"] != float64(5) || authority["decisionPresent"] != false {
		t.Fatalf("migrated release authority = %#v", authority)
	}

	firstBytes, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(root).ReleaseTrack(ctx, "payments", "stable"); err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("v1 migration was not idempotent across restart")
	}
}

func TestFileStoreV1MigrationSurvivesBusinessRollback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	legacy := newOperationalState()
	legacy.Version = 1
	legacy.Revisions["revision-good"] = core.ContractRevision{ID: "revision-good", ContractID: "payments"}
	legacy.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 4, CurrentRevisionID: "revision-good",
	}
	if err := store.publishOperationalState(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	rollback := errors.New("rollback after migration")
	err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SavePublication(ctx, core.Publication{
			ProjectID: "payments", RevisionID: "revision-good", Public: true, Path: "/payments/stable",
		}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v, want sentinel", err)
	}

	restarted := NewFileStore(root)
	state, err := restarted.loadOperationalState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 2 {
		t.Fatalf("migration rolled back to state version %d", state.Version)
	}
	if len(state.Publications) != 0 {
		t.Fatalf("business mutation escaped rollback: %#v", state.Publications)
	}
}

func TestFileStoreV2DetectsStrippedReleaseAuthority(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: "revision-good",
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: baseline.ContractID}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, baseline)
	}); err != nil {
		t.Fatal(err)
	}
	accepted, changed, err := core.ConsiderReleaseDecision(baseline, core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || !changed {
		t.Fatalf("derive acceptance: changed=%t err=%v", changed, err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, baseline.Generation, accepted)
	}); err != nil {
		t.Fatal(err)
	}
	promoted, err := core.PromoteReleaseRevision(accepted, "revision-next")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, accepted.Generation, promoted)
	}); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(root, "operational", "state.json")
	original, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "last decision",
			mutate: func(state map[string]any) {
				track := state["releaseTracks"].(map[string]any)[releaseTrackKey("payments", "stable")].(map[string]any)
				delete(track, "lastDecision")
			},
		},
		{
			name: "authority marker",
			mutate: func(state map[string]any) {
				delete(state["releaseTrackAuthorities"].(map[string]any), releaseTrackKey("payments", "stable"))
			},
		},
		{
			name: "generation chronology",
			mutate: func(state map[string]any) {
				track := state["releaseTracks"].(map[string]any)[releaseTrackKey("payments", "stable")].(map[string]any)
				track["generation"] = float64(0)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := readOperationalStateJSON(t, root)
			if _, ok := state["releaseTrackAuthorities"].(map[string]any); !ok {
				t.Fatalf("v2 state lacks release authority marker: %#v", state)
			}
			test.mutate(state)
			encoded, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := writeFileAtomically(statePath, append(encoded, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewFileStore(root).ReleaseTrack(ctx, "payments", "stable"); err == nil {
				t.Fatal("stripped v2 release authority was accepted as legacy")
			}
			if err := writeFileAtomically(statePath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := NewFileStore(root).ReleaseTrack(ctx, "payments", "stable")
			if err != nil {
				t.Fatalf("recover original v2 authority: %v", err)
			}
			if !reflect.DeepEqual(got, promoted) {
				t.Fatalf("recovered track = %#v, want %#v", got, promoted)
			}
		})
	}
}

func TestFileStoreLegacyReleaseHelperCannotRevokePersistedAcceptance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: "revision-good",
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: baseline.ContractID}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, baseline)
	}); err != nil {
		t.Fatal(err)
	}
	accepted, err := core.ConsiderReleaseRevision(baseline, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, baseline.Generation, accepted)
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, baseline.ContractID, baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, accepted) {
		t.Fatalf("restarted legacy acceptance = %#v, want %#v", got, accepted)
	}

	err = restarted.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		current, err := operational.ReleaseTrack(ctx, baseline.ContractID, baseline.ID)
		if err != nil {
			return err
		}
		if err := operational.SaveReview(ctx, core.ContractReview{
			ID: "denied-review", ContractID: baseline.ContractID,
			BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
		}); err != nil {
			return err
		}
		if err := operational.SaveSyncRecord(ctx, core.SyncRecord{
			ID: "denied-sync", ProjectID: baseline.ContractID, RevisionID: "revision-next", Result: core.SyncResultSuccess,
		}); err != nil {
			return err
		}
		if err := operational.SavePublication(ctx, core.Publication{
			ProjectID: baseline.ContractID, RevisionID: "revision-next", Path: "/payments/denied",
		}); err != nil {
			return err
		}
		if err := operational.AppendAuditEvent(ctx, core.AuditEvent{
			ID: "denied-audit", ContractID: baseline.ContractID, TrackID: baseline.ID, RevisionID: "revision-next",
		}); err != nil {
			return err
		}
		if err := operational.Enqueue(ctx, core.OutboxMessage{
			ID: "denied-outbox", ContractID: baseline.ContractID, TrackID: baseline.ID, RevisionID: "revision-next",
		}); err != nil {
			return err
		}
		_, err = core.ConsiderReleaseRevision(current, "revision-next", false)
		return err
	})
	if err == nil {
		t.Fatal("persisted legacy acceptance was revoked")
	}

	state, err := NewFileStore(root).loadOperationalState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.ReleaseTracks[releaseTrackKey(baseline.ContractID, baseline.ID)], accepted) {
		t.Fatalf("denied revocation changed accepted track: %#v", state.ReleaseTracks)
	}
	if len(state.Reviews) != 0 || len(state.SyncRecords) != 0 || len(state.Publications) != 0 || len(state.AuditEvents) != 0 || len(state.Outbox) != 0 {
		t.Fatalf("denied revocation leaked side effects: reviews=%d sync=%d publications=%d audit=%d outbox=%d",
			len(state.Reviews), len(state.SyncRecords), len(state.Publications), len(state.AuditEvents), len(state.Outbox))
	}

	replay, err := core.ConsiderReleaseRevision(accepted, "revision-next", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replay, accepted) {
		t.Fatalf("exact accepted replay changed track: got=%#v want=%#v", replay, accepted)
	}
	promoted, err := core.PromoteReleaseRevision(replay, "revision-next")
	if err != nil {
		t.Fatalf("promote persisted legacy acceptance: %v", err)
	}
	if err := NewFileStore(root).Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, accepted.Generation, promoted)
	}); err != nil {
		t.Fatal(err)
	}
	final, err := NewFileStore(root).ReleaseTrack(ctx, baseline.ContractID, baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(final, promoted) {
		t.Fatalf("persisted promotion = %#v, want %#v", final, promoted)
	}
}

func TestFileStoreRequiresExplicitReleaseTrackInitialization(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: "payments"}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	decision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	for _, track := range []core.ReleaseTrack{
		{
			ID: "implicit-history", ContractID: "payments", Mode: core.ReleaseModePinned,
			Generation: 5, CurrentRevisionID: "revision-good",
		},
		{
			ID: "implicit-decision-history", ContractID: "payments", Mode: core.ReleaseModePinned,
			Generation: 3, CurrentRevisionID: "revision-good",
			CandidateRevisionID: "revision-next", LastDecision: &decision,
		},
	} {
		if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			return operational.SaveReleaseTrack(ctx, 0, track)
		}); err == nil {
			t.Fatalf("implicit release history was initialized: %#v", track)
		}
	}

	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: "revision-good",
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, 0, baseline)
	}); err != nil {
		t.Fatalf("explicit generation-zero baseline: %v", err)
	}
	accepted, changed, err := core.ConsiderReleaseDecision(baseline, decision)
	if err != nil || !changed {
		t.Fatalf("derive first decision: changed=%t err=%v", changed, err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, baseline.Generation, accepted)
	}); err != nil {
		t.Fatalf("first authoritative decision: %v", err)
	}
}

func TestFileStoreRejectsCrossContractOperationalReferencesAtomically(t *testing.T) {
	for _, test := range []struct {
		name string
		save func(context.Context, port.OperationalStore) error
	}{
		{
			name: "track current",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SaveReleaseTrack(ctx, 0, core.ReleaseTrack{
					ID: "forged-current", ContractID: "payments", Mode: core.ReleaseModePinned,
					CurrentRevisionID: "orders-revision",
				})
			},
		},
		{
			name: "track candidate",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				decision := core.ReleaseDecision{
					RevisionID: "orders-revision", ReviewID: "forged-review",
					ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Verdict:      core.VerdictFail,
					EvaluatedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
				}
				return operational.SaveReleaseTrack(ctx, 0, core.ReleaseTrack{
					ID: "forged-candidate", ContractID: "payments", Mode: core.ReleaseModePinned,
					Generation: 1, CurrentRevisionID: "payments-good",
					CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
				})
			},
		},
		{
			name: "publication",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SavePublication(ctx, core.Publication{
					ProjectID: "payments", RevisionID: "orders-revision", Public: true, Path: "/payments/forged",
				})
			},
		},
		{
			name: "review baseline",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SaveReview(ctx, core.ContractReview{
					ID: "forged-baseline", ContractID: "payments",
					BaselineRevisionID: "orders-revision", CandidateRevisionID: "payments-next",
				})
			},
		},
		{
			name: "review candidate",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				return operational.SaveReview(ctx, core.ContractReview{
					ID: "forged-candidate", ContractID: "payments",
					BaselineRevisionID: "payments-good", CandidateRevisionID: "orders-revision",
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			store := NewFileStore(root)
			baseline := core.ReleaseTrack{
				ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
				CurrentRevisionID: "payments-good",
			}
			if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				for revisionID, contractID := range map[string]string{
					"payments-good": "payments", "payments-next": "payments", "orders-revision": "orders",
				} {
					if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: contractID}); err != nil {
						return err
					}
				}
				return operational.SaveReleaseTrack(ctx, 0, baseline)
			}); err != nil {
				t.Fatal(err)
			}

			err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				if err := operational.SaveReview(ctx, core.ContractReview{
					ID: "side-review", ContractID: "payments",
					BaselineRevisionID: "payments-good", CandidateRevisionID: "payments-next",
				}); err != nil {
					return err
				}
				if err := operational.SaveSyncRecord(ctx, core.SyncRecord{
					ID: "side-sync", ProjectID: "payments", RevisionID: "payments-next", Result: core.SyncResultSuccess,
				}); err != nil {
					return err
				}
				if err := operational.SavePublication(ctx, core.Publication{
					ProjectID: "payments", RevisionID: "payments-good", Public: true, Path: "/payments/stable",
				}); err != nil {
					return err
				}
				if err := operational.AppendAuditEvent(ctx, core.AuditEvent{
					ID: "side-audit", ContractID: "payments", TrackID: "stable", RevisionID: "payments-next",
				}); err != nil {
					return err
				}
				if err := operational.Enqueue(ctx, core.OutboxMessage{
					ID: "side-outbox", ContractID: "payments", TrackID: "stable", RevisionID: "payments-next",
				}); err != nil {
					return err
				}
				return test.save(ctx, operational)
			})
			if err == nil {
				t.Fatal("cross-contract operational reference committed")
			}

			restarted := NewFileStore(root)
			got, err := restarted.ReleaseTrack(ctx, baseline.ContractID, baseline.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, baseline) {
				t.Fatalf("cross-contract attempt changed last known good: got=%#v want=%#v", got, baseline)
			}
			state, err := restarted.loadOperationalState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.ReleaseTracks) != 1 || len(state.Reviews) != 0 || len(state.SyncRecords) != 0 ||
				len(state.Publications) != 0 || len(state.AuditEvents) != 0 || len(state.Outbox) != 0 {
				t.Fatalf("cross-contract rollback leaked side effects: %#v", state)
			}
		})
	}
}

func readOperationalStateJSON(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
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
	track := core.ReleaseTrack{ID: "stable", ContractID: "payments", Mode: core.ReleaseModeFollowing}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, 0, track)
	}); err != nil {
		t.Fatal(err)
	}
	err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		track.Generation = 1
		return operational.SaveReleaseTrack(ctx, 1, track)
	})
	if !errors.Is(err, port.ErrGenerationConflict) {
		t.Fatalf("stale generation error = %v, want %v", err, port.ErrGenerationConflict)
	}
	got, err := store.ReleaseTrack(ctx, "payments", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if got.Generation != 0 {
		t.Fatalf("generation after rejected transaction = %d, want 0", got.Generation)
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
			track := core.ReleaseTrack{ID: "stable", ContractID: "payments", Mode: core.ReleaseModeFollowing}
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
				next, changed, err := core.ConsiderReleaseDecision(current, core.ReleaseDecision{
					RevisionID: "revision-next", ReviewID: "review-next",
					ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Verdict:      core.VerdictFail,
					EvaluatedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
				})
				if err != nil || !changed {
					return fmt.Errorf("derive indeterminate decision: changed=%t err=%w", changed, err)
				}
				if err := operational.SaveRevision(ctx, core.ContractRevision{ID: "revision-next", ContractID: "payments"}); err != nil {
					return err
				}
				return operational.SaveReleaseTrack(ctx, current.Generation, next)
			})
			if !errors.Is(err, port.ErrCommitOutcomeUnknown) {
				t.Fatalf("post-rename error = %v, want %v", err, port.ErrCommitOutcomeUnknown)
			}

			restarted := NewFileStore(root)
			got, err := restarted.ReleaseTrack(ctx, "payments", "stable")
			if err != nil {
				t.Fatal(err)
			}
			if got.Generation != 1 {
				t.Fatalf("recovered generation = %d, want atomically published generation 1", got.Generation)
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

func TestFileStoreRejectsConflictingRevisionEvidence(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	raw := []byte("openapi: 3.1.0\n")
	key, err := store.Put(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := core.NewContractSnapshot("payments", "revision-1", raw, core.SpecIndex{})
	revision := core.ContractRevision{
		ID: "revision-1", ContractID: "payments", SourceID: "source-main",
		SpecBlobKey:    string(key),
		SpecDigest:     snapshot.SpecDigest,
		ContractDigest: snapshot.ContractDigest,
		ReviewSnapshot: &snapshot,
	}
	if err := store.SaveRevision(ctx, revision); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRevision(ctx, revision); err != nil {
		t.Fatalf("identical revision replay: %v", err)
	}
	conflicting := revision
	conflicting.SpecDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if err := store.SaveRevision(ctx, conflicting); err == nil {
		t.Fatal("conflicting immutable revision evidence was accepted")
	}
	got, err := store.Revision(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, revision) {
		t.Fatalf("conflicting replay changed revision: %#v", got)
	}
}

func TestFileStoreEnrichesMatchingLegacyRevisionEvidenceOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	raw := []byte("openapi: 3.1.0\n")
	key, err := store.Put(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy := core.ContractRevision{
		ID: "revision-1", SourceID: "source-main", Ref: "main", CommitSHA: "abc123",
		SpecBlobKey: string(key),
	}
	if err := store.SaveRevision(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	enriched := legacy
	enriched.ContractID = "payments"
	snapshot := core.NewContractSnapshot(enriched.ContractID, enriched.ID, raw, core.SpecIndex{})
	enriched.SpecDigest = snapshot.SpecDigest
	enriched.ContractDigest = snapshot.ContractDigest
	enriched.ReviewSnapshot = &snapshot
	if err := store.SaveRevision(ctx, enriched); err != nil {
		t.Fatalf("enrich matching legacy revision: %v", err)
	}
	restarted := NewFileStore(root)
	got, err := restarted.ContractRevision(ctx, enriched.ContractID, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, enriched) {
		t.Fatalf("enriched revision = %#v, want %#v", got, enriched)
	}
	changedAgain := enriched
	changedAgain.ContractDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := restarted.SaveRevision(ctx, changedAgain); err == nil {
		t.Fatal("second revision evidence change was accepted")
	}
}

func TestFileStoreRollsBackForgedLegacyRevisionEvidence(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*core.ContractRevision)
	}{
		{
			name: "spec digest is not the stored blob",
			mutate: func(revision *core.ContractRevision) {
				forged := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				revision.SpecDigest = forged
				revision.ReviewSnapshot.SpecDigest = forged
			},
		},
		{
			name: "contract digest is not the canonical surface",
			mutate: func(revision *core.ContractRevision) {
				forged := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				revision.ContractDigest = forged
				revision.ReviewSnapshot.ContractDigest = forged
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			store := NewFileStore(root)
			raw := []byte("openapi: 3.1.0\n")
			key, err := store.Put(ctx, raw)
			if err != nil {
				t.Fatal(err)
			}
			legacy := core.ContractRevision{
				ID: "revision-1", SourceID: "source-main", Ref: "main", CommitSHA: "abc123",
				SpecBlobKey: string(key),
			}
			if err := store.SaveRevision(ctx, legacy); err != nil {
				t.Fatal(err)
			}
			enriched := legacy
			enriched.ContractID = "payments"
			snapshot := core.NewContractSnapshot(enriched.ContractID, enriched.ID, raw, core.SpecIndex{})
			enriched.SpecDigest = snapshot.SpecDigest
			enriched.ContractDigest = snapshot.ContractDigest
			enriched.ReviewSnapshot = &snapshot
			test.mutate(&enriched)

			if err := store.SaveRevision(ctx, enriched); err == nil {
				t.Fatal("forged legacy evidence was accepted")
			}
			got, err := NewFileStore(root).Revision(ctx, legacy.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, legacy) {
				t.Fatalf("failed enrichment escaped rollback: %#v", got)
			}

			validSnapshot := core.NewContractSnapshot("payments", legacy.ID, raw, core.SpecIndex{})
			valid := legacy
			valid.ContractID = "payments"
			valid.SpecDigest = validSnapshot.SpecDigest
			valid.ContractDigest = validSnapshot.ContractDigest
			valid.ReviewSnapshot = &validSnapshot
			if err := NewFileStore(root).SaveRevision(ctx, valid); err != nil {
				t.Fatalf("recover with valid evidence: %v", err)
			}
			if _, err := NewFileStore(root).ContractRevision(ctx, "payments", legacy.ID); err != nil {
				t.Fatalf("read recovered evidence after restart: %v", err)
			}
		})
	}
}

func TestFileStoreAddsCanonicalSnapshotToPriorFlatEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	raw := []byte("openapi: 3.1.0\n")
	key, err := store.Put(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := core.NewContractSnapshot("payments", "revision-1", raw, core.SpecIndex{})
	prior := core.ContractRevision{
		ID: "revision-1", ContractID: snapshot.ContractID, SourceID: "source-main",
		SpecBlobKey: string(key), SpecDigest: snapshot.SpecDigest, ContractDigest: snapshot.ContractDigest,
	}
	if err := store.writeJSON(ctx, "revisions", prior.ID+".json", prior); err != nil {
		t.Fatal(err)
	}
	enriched := prior
	enriched.ReviewSnapshot = &snapshot
	if err := store.SaveRevision(ctx, enriched); err != nil {
		t.Fatalf("add canonical snapshot to prior flat evidence: %v", err)
	}
	got, err := NewFileStore(root).ContractRevision(ctx, prior.ContractID, prior.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, enriched) {
		t.Fatalf("enriched prior evidence = %#v, want %#v", got, enriched)
	}
}

func TestFileStoreReplacesUnboundFlatDigestsDuringOneTimeEnrichment(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	raw := []byte("openapi: 3.1.0\n")
	key, err := store.Put(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	prior := core.ContractRevision{
		ID: "revision-1", ContractID: "payments", SourceID: "source-main",
		SpecBlobKey:    string(key),
		SpecDigest:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContractDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if err := store.writeJSON(ctx, "revisions", prior.ID+".json", prior); err != nil {
		t.Fatal(err)
	}

	snapshot := core.NewContractSnapshot(prior.ContractID, prior.ID, raw, core.SpecIndex{})
	enriched := prior
	enriched.SpecDigest = snapshot.SpecDigest
	enriched.ContractDigest = snapshot.ContractDigest
	enriched.ReviewSnapshot = &snapshot
	if err := store.SaveRevision(ctx, enriched); err != nil {
		t.Fatalf("replace unbound flat evidence: %v", err)
	}
	got, err := NewFileStore(root).ContractRevision(ctx, prior.ContractID, prior.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, enriched) {
		t.Fatalf("verified evidence = %#v, want %#v", got, enriched)
	}
}

func TestFileStoreRejectsConflictingReviewEvidence(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	review := core.ContractReview{
		ID: "review-1", ContractID: "payments",
		BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
		Report: core.ReviewReport{SchemaVersion: core.ReviewSchemaVersion, EngineVersion: "engine-v1"},
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: "payments"}); err != nil {
				return err
			}
		}
		return operational.SaveReview(ctx, review)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReview(ctx, review)
	}); err != nil {
		t.Fatalf("identical review replay: %v", err)
	}
	conflicting := review
	conflicting.Report.EngineVersion = "engine-v2"
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReview(ctx, conflicting)
	}); err == nil {
		t.Fatal("conflicting immutable review evidence was accepted")
	}
}

func TestFileStorePersistsReleaseDecisionReplayIdentityAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	decision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-1",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	track := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 1, CurrentRevisionID: "revision-good",
		CandidateRevisionID: "revision-next", LastDecision: &decision,
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: "payments"}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, track)
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, "payments", "stable")
	if err != nil {
		t.Fatal(err)
	}
	next, changed, err := core.ConsiderReleaseDecision(got, decision)
	if err != nil {
		t.Fatal(err)
	}
	if changed || next.Generation != track.Generation || next.LastDecision == nil || *next.LastDecision != decision {
		t.Fatalf("restart replay changed track: next=%#v changed=%t", next, changed)
	}
	promoted, err := core.PromoteReleaseRevision(got, decision.RevisionID)
	if err != nil {
		t.Fatalf("promote persisted accepted decision: %v", err)
	}
	replayed, err := core.PromoteReleaseRevision(promoted, decision.RevisionID)
	if err != nil {
		t.Fatalf("replay persisted promotion: %v", err)
	}
	if !reflect.DeepEqual(replayed, promoted) || promoted.CurrentRevisionID != decision.RevisionID {
		t.Fatalf("persisted promotion replay changed track: promoted=%#v replayed=%#v", promoted, replayed)
	}
}

func TestFileStorePersistsHistoricalDecisionReplayProtectionAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	acceptedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	acceptedDecision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-accepted",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true, EvaluatedAt: acceptedAt,
	}
	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: "revision-good",
	}
	track, changed, err := core.ConsiderReleaseDecision(baseline, acceptedDecision)
	if err != nil || !changed {
		t.Fatalf("apply acceptance: changed=%t err=%v", changed, err)
	}
	rejectedDecision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-rejected",
		ReviewDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:      core.VerdictFail, Accepted: false, EvaluatedAt: acceptedAt.Add(time.Minute),
	}
	track, changed, err = core.ConsiderReleaseDecision(track, rejectedDecision)
	if err != nil || !changed {
		t.Fatalf("apply rejection: changed=%t err=%v", changed, err)
	}

	store := NewFileStore(root)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		for _, revisionID := range []string{"revision-good", "revision-next"} {
			if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: "payments"}); err != nil {
				return err
			}
		}
		return operational.SaveReleaseTrack(ctx, 0, baseline)
	}); err != nil {
		t.Fatal(err)
	}
	acceptedTrack, changed, err := core.ConsiderReleaseDecision(baseline, acceptedDecision)
	if err != nil || !changed {
		t.Fatalf("rederive acceptance: changed=%t err=%v", changed, err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, baseline.Generation, acceptedTrack)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, acceptedTrack.Generation, track)
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, "payments", "stable")
	if err != nil {
		t.Fatal(err)
	}
	replayed, changed, err := core.ConsiderReleaseDecision(got, acceptedDecision)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !reflect.DeepEqual(replayed, got) {
		t.Fatalf("historical acceptance changed restarted track: replayed=%#v got=%#v changed=%t", replayed, got, changed)
	}
	persisted, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte("decisionHistory")) {
		t.Fatalf("restarted track retained append-only decision history: %s", persisted)
	}
	if _, err := core.PromoteReleaseRevision(replayed, acceptedDecision.RevisionID); err == nil {
		t.Fatal("historical acceptance authorized promotion after restart")
	}
}

func TestFileStoreRestartPreservesCanonicalReleaseReviewValidation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	baselineSpec := []byte("baseline spec")
	candidateSpec := []byte("candidate spec")
	baseline := core.NewContractSnapshot("payments", "revision-good", baselineSpec, core.SpecIndex{
		Operations: []core.Operation{{Method: "GET", Path: "/payments"}},
	})
	candidate := core.NewContractSnapshot("payments", "revision-next", candidateSpec, core.SpecIndex{})
	policy, err := core.MergePolicy(core.PolicyLayer{
		Name: "stable", Source: core.PolicySourceRepository,
		Exceptions: []core.PolicyException{{
			RuleID: core.RuleOperationRemoved, Reason: "planned migration", Author: "api-team",
			ExpiresAt: evaluatedAt.Add(time.Hour), Source: core.PolicySourceRepository,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := core.EvaluateReview(core.ReviewRequest{
		ContractID: "payments", Target: baseline, Candidate: candidate, Release: &baseline,
		Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	report.Comparisons = []core.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	encoded, err := core.CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	var connectedReport core.ReviewReport
	if err := json.Unmarshal(encoded, &connectedReport); err != nil {
		t.Fatal(err)
	}

	store := NewFileStore(root)
	baselineKey, err := store.Put(ctx, baselineSpec)
	if err != nil {
		t.Fatal(err)
	}
	candidateKey, err := store.Put(ctx, candidateSpec)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveRevision(ctx, core.ContractRevision{
			ID: baseline.RevisionID, ContractID: baseline.ContractID,
			SpecBlobKey: string(baselineKey), SpecDigest: baseline.SpecDigest,
			ContractDigest: baseline.ContractDigest, ReviewSnapshot: &baseline,
		}); err != nil {
			return err
		}
		return operational.SaveRevision(ctx, core.ContractRevision{
			ID: candidate.RevisionID, ContractID: candidate.ContractID,
			SpecBlobKey: string(candidateKey), SpecDigest: candidate.SpecDigest,
			ContractDigest: candidate.ContractDigest, ReviewSnapshot: &candidate,
		})
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	persistedBaseline, err := restarted.ContractRevision(ctx, "payments", baseline.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	persistedCandidate, err := restarted.ContractRevision(ctx, "payments", candidate.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := core.ValidateReleaseReviewReportAgainstSnapshots(
			connectedReport,
			"payments",
			*persistedBaseline.ReviewSnapshot,
			*persistedCandidate.ReviewSnapshot,
		); err != nil {
			t.Fatalf("validate canonical report after restart attempt %d: %v", attempt, err)
		}
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
	if _, err := store.Publication(ctx, publication.ProjectID, publication.RevisionID); err != nil {
		t.Fatalf("load legacy publication through authenticated migration: %v", err)
	}
	if _, err := store.ContractRevision(ctx, publication.ProjectID, revision.ID); err != nil {
		t.Fatalf("legacy migration did not bind revision ownership: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "operational", "state.json")); err != nil {
		t.Fatalf("legacy read did not persist authenticated v2 state: %v", err)
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
