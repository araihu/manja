package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/application"
	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/contracttest"
	core "github.com/araihu/manja/domain"
)

type storedReleaseFixture struct {
	baselineSpec  []byte
	candidateSpec []byte
	baseline      core.ReleaseTrack
	next          core.ReleaseTrack
	review        core.ContractReview
	syncRecord    core.SyncRecord
	authorization core.ReleaseAuthorization
}

type observingRevisionReader struct {
	delegate port.RevisionReader
	observed context.Context
}

type fixedReleaseClock struct{ now time.Time }

func (c fixedReleaseClock) Now(context.Context) time.Time { return c.now }

type fixedReleaseEvidenceReader struct{ evidence core.ReleaseEvidence }

func (r fixedReleaseEvidenceReader) ReleaseEvidence(
	context.Context,
	string,
	string,
	string,
) (core.ReleaseEvidence, error) {
	return r.evidence, nil
}

func (r *observingRevisionReader) ContractRevision(
	ctx context.Context,
	contractID, revisionID string,
) (core.ContractRevision, error) {
	r.observed = ctx
	return r.delegate.ContractRevision(ctx, contractID, revisionID)
}

func newStoredReleaseFixture(t *testing.T, _ *FileStore, accepted bool) storedReleaseFixture {
	return newStoredReleaseFixtureNamed(
		t,
		accepted,
		"review-next",
		"sync-next",
		time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	)
}

func newStoredReleaseFixtureNamed(
	t *testing.T,
	accepted bool,
	reviewID, syncID string,
	evaluatedAt time.Time,
) storedReleaseFixture {
	t.Helper()
	baselineSpec := []byte("baseline release authority")
	candidateSpec := []byte("candidate release authority")
	baselineSnapshot := core.NewContractSnapshot("payments", "revision-good", baselineSpec, core.SpecIndex{})
	candidateSnapshot := core.NewContractSnapshot("payments", "revision-next", candidateSpec, core.SpecIndex{})
	policy, err := core.MergePolicy(core.PolicyLayer{
		Name: "repository-default", Source: core.PolicySourceRepository,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := core.EvaluateReview(core.ReviewRequest{
		ContractID: "payments", Target: baselineSnapshot, Candidate: candidateSnapshot, Release: &baselineSnapshot,
		Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: "store-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	report.Comparisons = []core.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	review := core.ContractReview{
		ID: reviewID, ContractID: "payments",
		BaselineRevisionID: "revision-good", BaselineSpecDigest: baselineSnapshot.SpecDigest,
		BaselineContractDigest: baselineSnapshot.ContractDigest,
		CandidateRevisionID:    "revision-next", CandidateSpecDigest: candidateSnapshot.SpecDigest,
		CandidateContractDigest: candidateSnapshot.ContractDigest, Report: report,
	}
	canonicalReview, err := core.CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	reviewDigest := sha256.Sum256(canonicalReview)
	decision := core.ReleaseDecision{
		RevisionID: review.CandidateRevisionID, ReviewID: review.ID,
		ReviewDigest: hex.EncodeToString(reviewDigest[:]), Verdict: report.Verdict,
		Accepted: accepted, EvaluatedAt: evaluatedAt,
	}
	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main",
		Mode: core.ReleaseModePinned, CurrentRevisionID: review.BaselineRevisionID,
	}
	next, changed, err := core.ConsiderReleaseDecision(baseline, decision)
	if err != nil || !changed {
		t.Fatalf("derive stored release fixture: changed=%t err=%v", changed, err)
	}
	syncRecord := core.SyncRecord{
		ID: syncID, ProjectID: "payments", SourceID: "payments-git",
		RevisionID: review.CandidateRevisionID, Ref: baseline.BoundRef,
		Result: core.SyncResultSuccess, StartedAt: evaluatedAt.Add(-time.Minute), FinishedAt: evaluatedAt,
	}
	return storedReleaseFixture{
		baselineSpec: baselineSpec, candidateSpec: candidateSpec,
		baseline: baseline, next: next, review: review, syncRecord: syncRecord,
		authorization: core.ReleaseAuthorization{
			ContractID: "payments", TrackID: baseline.ID, ReviewID: review.ID, SyncRecordID: syncRecord.ID,
			BaselineRevisionID: review.BaselineRevisionID, CandidateRevisionID: review.CandidateRevisionID,
			SourceID: syncRecord.SourceID, BoundRef: baseline.BoundRef,
			PublicPath: "/payments/stable", PolicyDigest: report.PolicyDigest,
		},
	}
}

func persistStoredReleaseBaseline(
	t *testing.T,
	ctx context.Context,
	store *FileStore,
	fixture storedReleaseFixture,
	withAuthorization bool,
) {
	t.Helper()
	baselineKey, err := store.Put(ctx, fixture.baselineSpec)
	if err != nil {
		t.Fatal(err)
	}
	candidateKey, err := store.Put(ctx, fixture.candidateSpec)
	if err != nil {
		t.Fatal(err)
	}
	baselineSnapshot := core.NewContractSnapshot(
		fixture.review.ContractID,
		fixture.review.BaselineRevisionID,
		fixture.baselineSpec,
		core.SpecIndex{},
	)
	candidateSnapshot := core.NewContractSnapshot(
		fixture.review.ContractID,
		fixture.review.CandidateRevisionID,
		fixture.candidateSpec,
		core.SpecIndex{},
	)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveRevision(ctx, core.ContractRevision{
			ID: baselineSnapshot.RevisionID, ContractID: baselineSnapshot.ContractID,
			SourceID: fixture.authorization.SourceID, Ref: fixture.authorization.BoundRef,
			SpecBlobKey: string(baselineKey), SpecDigest: baselineSnapshot.SpecDigest,
			ContractDigest: baselineSnapshot.ContractDigest, ReviewSnapshot: &baselineSnapshot,
		}); err != nil {
			return err
		}
		if err := operational.SaveRevision(ctx, core.ContractRevision{
			ID: candidateSnapshot.RevisionID, ContractID: candidateSnapshot.ContractID,
			SourceID: fixture.authorization.SourceID, Ref: fixture.authorization.BoundRef,
			SpecBlobKey: string(candidateKey), SpecDigest: candidateSnapshot.SpecDigest,
			ContractDigest: candidateSnapshot.ContractDigest, ReviewSnapshot: &candidateSnapshot,
		}); err != nil {
			return err
		}
		if err := operational.SaveReview(ctx, fixture.review); err != nil {
			return err
		}
		if err := operational.SaveSyncRecord(ctx, fixture.syncRecord); err != nil {
			return err
		}
		return operational.SaveReleaseTrack(ctx, 0, fixture.baseline)
	}); err != nil {
		t.Fatalf("persist stored release baseline: %v", err)
	}
	if withAuthorization {
		if err := store.SaveReleaseAuthorization(ctx, fixture.authorization); err != nil {
			t.Fatalf("persist release authorization: %v", err)
		}
	}
}

func appendStoredReleaseEffects(
	ctx context.Context,
	operational port.OperationalStore,
	fixture storedReleaseFixture,
) error {
	eventID := storedReleaseEvidenceID(fixture.authorization, fixture.next.LastDecision.Accepted, fixture.next.Generation)
	if err := operational.AppendAuditEvent(ctx, core.AuditEvent{
		ID: eventID, ContractID: fixture.authorization.ContractID, TrackID: fixture.authorization.TrackID,
		RevisionID: fixture.authorization.CandidateRevisionID, Kind: "release.track.considered",
		OccurredAt: fixture.next.LastDecision.EvaluatedAt,
	}); err != nil {
		return err
	}
	return operational.Enqueue(ctx, core.OutboxMessage{
		ID: "outbox-" + eventID[len("audit-"):], ContractID: fixture.authorization.ContractID,
		TrackID: fixture.authorization.TrackID, RevisionID: fixture.authorization.CandidateRevisionID,
		Topic: "release.track.updated", CreatedAt: fixture.next.LastDecision.EvaluatedAt,
	})
}

func storedReleaseEvidenceID(authorization core.ReleaseAuthorization, accepted bool, generation uint64) string {
	value := fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%d\x00%t",
		authorization.ContractID,
		authorization.TrackID,
		authorization.CandidateRevisionID,
		authorization.ReviewID,
		generation,
		accepted,
	)
	sum := sha256.Sum256([]byte(value))
	return "audit-" + hex.EncodeToString(sum[:])[:24]
}

func assertStoredReleaseBaseline(
	t *testing.T,
	ctx context.Context,
	store *FileStore,
	fixture storedReleaseFixture,
) {
	t.Helper()
	got, err := store.ReleaseTrack(ctx, fixture.baseline.ContractID, fixture.baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fixture.baseline) {
		t.Fatalf("last-known-good release track = %#v, want %#v", got, fixture.baseline)
	}
	state, err := store.loadOperationalState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.AuditEvents) != 0 || len(state.Outbox) != 0 || len(state.Publications) != 0 {
		t.Fatalf("failed release transition leaked side effects: %#v", state)
	}
}

func TestFileStorePublicContracts(t *testing.T) {
	contracttest.UnitOfWork(t, func(t testing.TB) port.UnitOfWork {
		return NewFileStore(t.TempDir())
	})
	contracttest.BlobStore(t, func(t testing.TB) port.BlobStore {
		return NewFileStore(t.TempDir())
	})
	contracttest.ReleaseAuthorityUnitOfWork(t, func(t testing.TB) contracttest.ReleaseAuthorityUnitOfWorkFixture {
		store := NewFileStore(t.TempDir())
		return contracttest.ReleaseAuthorityUnitOfWorkFixture{
			UnitOfWork: store, Blobs: store, Authorizations: store,
		}
	})
	contracttest.RevisionReader(t, func(t testing.TB) contracttest.RevisionReaderFixture {
		store := NewFileStore(t.TempDir())
		revision := core.ContractRevision{
			ID: "revision-1", ContractID: "payments", SourceID: "source-main", Ref: "main",
		}
		if err := store.SaveRevision(context.Background(), revision); err != nil {
			t.Fatalf("seed revision reader: %v", err)
		}
		reader := &observingRevisionReader{delegate: store}
		return contracttest.RevisionReaderFixture{
			Reader: reader, ContractID: revision.ContractID, RevisionID: revision.ID, Want: revision,
			Observed: func() context.Context { return reader.observed },
		}
	})
}

func TestFileStoreReleaseTrackAliasesCannotMutatePersistedStateAcrossRestart(t *testing.T) {
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
	want := fixture.next
	before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
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
	after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("alias-only transactions changed persisted operational state")
	}
}

func TestFileStoreRejectsStrippedReleaseDecisionEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, true)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	}); err != nil {
		t.Fatal(err)
	}
	track := fixture.next

	err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
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
	track := fixture.next
	evaluatedAt := track.LastDecision.EvaluatedAt

	for _, test := range []struct {
		name        string
		evaluatedAt time.Time
	}{
		{name: "older", evaluatedAt: evaluatedAt.Add(-time.Minute)},
		{name: "equal time different identity", evaluatedAt: evaluatedAt},
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
	want := fixture.next
	before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	err = store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
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
	after, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed transition changed persisted operational state")
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
	unauthenticatedDecision := core.ReleaseDecision{
		RevisionID: "revision-unauthenticated", ReviewID: "review-missing",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	legacy.ReleaseTracks[releaseTrackKey("payments", "stable")] = core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		Generation: 5, CurrentRevisionID: "revision-good", CandidateRevisionID: "revision-unauthenticated",
		LastDecision: &unauthenticatedDecision,
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
	if first["version"] != float64(operationalStateVersion) {
		t.Fatalf("migrated state version = %#v, want %d", first["version"], operationalStateVersion)
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
	if state.Version != operationalStateVersion {
		t.Fatalf("migration rolled back to state version %d", state.Version)
	}
	if len(state.Publications) != 0 {
		t.Fatalf("business mutation escaped rollback: %#v", state.Publications)
	}
}

func TestFileStoreMigratesV2DecisionAuthorityToSafeV3Baseline(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	legacy := newOperationalState()
	legacy.Version = decisionOperationalStateVersion
	for _, revisionID := range []string{"revision-good", "revision-unauthenticated"} {
		legacy.Revisions[revisionID] = core.ContractRevision{ID: revisionID, ContractID: "payments"}
	}
	decision := core.ReleaseDecision{
		RevisionID: "revision-unauthenticated", ReviewID: "review-missing",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictPass, Accepted: true,
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	track := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main", Mode: core.ReleaseModePinned,
		Generation: 6, CurrentRevisionID: "revision-good",
		CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
	}
	key := releaseTrackKey(track.ContractID, track.ID)
	legacy.ReleaseTracks[key] = track
	legacy.ReleaseTrackAuthorities[key] = newReleaseTrackAuthority(track)
	if err := store.publishOperationalState(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	got, err := NewFileStore(root).ReleaseTrack(ctx, track.ContractID, track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Generation != track.Generation ||
		got.CurrentRevisionID != track.CurrentRevisionID ||
		got.CandidateRevisionID != "" ||
		got.LastDecision != nil {
		t.Fatalf("migrated v2 track = %#v", got)
	}
	state, err := NewFileStore(root).loadOperationalState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != operationalStateVersion || len(state.ReleaseAuthorizations) != 0 {
		t.Fatalf("migrated v2 operational state = %#v", state)
	}
}

func TestFileStoreV3DetectsStrippedReleaseAuthority(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, true)
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
				t.Fatalf("v3 state lacks release authority marker: %#v", state)
			}
			test.mutate(state)
			encoded, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := durableAtomicWrite(statePath, append(encoded, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewFileStore(root).ReleaseTrack(ctx, "payments", "stable"); err == nil {
				t.Fatal("stripped v3 release authority was accepted as legacy")
			}
			if err := durableAtomicWrite(statePath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := NewFileStore(root).ReleaseTrack(ctx, "payments", "stable")
			if err != nil {
				t.Fatalf("recover original v3 authority: %v", err)
			}
			if !reflect.DeepEqual(got, fixture.next) {
				t.Fatalf("recovered track = %#v, want %#v", got, fixture.next)
			}
		})
	}
}

func TestFileStoreRejectsUnauthenticatedLegacyReleaseHelperDecision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main", Mode: core.ReleaseModePinned,
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
	}); err == nil {
		t.Fatal("legacy helper decision without persisted review authority committed")
	}

	restarted := NewFileStore(root)
	got, err := restarted.ReleaseTrack(ctx, baseline.ContractID, baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, baseline) {
		t.Fatalf("restarted track = %#v, want last-known-good %#v", got, baseline)
	}

	_, err = core.ConsiderReleaseRevision(accepted, "revision-next", false)
	if err == nil {
		t.Fatal("legacy accepted decision was revoked")
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
		t.Fatalf("derive legacy promotion: %v", err)
	}
	if promoted.CurrentRevisionID != "revision-next" {
		t.Fatalf("legacy promotion = %#v", promoted)
	}
}

func TestFileStoreImmutableOperationalIDsRejectConflictsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	revision := core.ContractRevision{ID: "revision-good", ContractID: "payments"}
	track := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: core.ReleaseModePinned,
		CurrentRevisionID: revision.ID,
	}
	syncRecord := core.SyncRecord{
		ID: "sync-1", ProjectID: "payments", RevisionID: revision.ID,
		Result: core.SyncResultSuccess, Trigger: "manual",
	}
	audit := core.AuditEvent{
		ID: "audit-1", ContractID: "payments", TrackID: track.ID,
		RevisionID: revision.ID, Kind: "release.seeded",
		OccurredAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
	outbox := core.OutboxMessage{
		ID: "outbox-1", ContractID: "payments", TrackID: track.ID,
		RevisionID: revision.ID, Topic: "release.seeded", Payload: []byte(`{"generation":0}`),
		CreatedAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveRevision(ctx, revision); err != nil {
			return err
		}
		if err := operational.SaveReleaseTrack(ctx, 0, track); err != nil {
			return err
		}
		if err := operational.SaveSyncRecord(ctx, syncRecord); err != nil {
			return err
		}
		if err := operational.AppendAuditEvent(ctx, audit); err != nil {
			return err
		}
		return operational.Enqueue(ctx, outbox)
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewFileStore(root)
	before, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveSyncRecord(ctx, syncRecord); err != nil {
			return err
		}
		if err := operational.AppendAuditEvent(ctx, audit); err != nil {
			return err
		}
		return operational.Enqueue(ctx, outbox)
	}); err != nil {
		t.Fatalf("identical immutable replay: %v", err)
	}
	afterReplay, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterReplay) {
		t.Fatal("identical immutable replay changed persisted bytes")
	}

	for _, test := range []struct {
		name string
		save func(context.Context, port.OperationalStore) error
	}{
		{
			name: "sync",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				conflict := syncRecord
				conflict.Trigger = "webhook"
				return operational.SaveSyncRecord(ctx, conflict)
			},
		},
		{
			name: "audit",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				conflict := audit
				conflict.Kind = "release.rewritten"
				return operational.AppendAuditEvent(ctx, conflict)
			},
		},
		{
			name: "outbox",
			save: func(ctx context.Context, operational port.OperationalStore) error {
				conflict := outbox
				conflict.Payload = []byte(`{"generation":1}`)
				return operational.Enqueue(ctx, conflict)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := NewFileStore(root).Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
				if err := operational.SavePublication(ctx, core.Publication{
					ProjectID: "payments", RevisionID: revision.ID, Path: "/should-roll-back",
				}); err != nil {
					return err
				}
				return test.save(ctx, operational)
			})
			if err == nil {
				t.Fatal("conflicting immutable ID overwrote durable evidence")
			}
			state, err := NewFileStore(root).loadOperationalState(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Publications) != 0 ||
				!reflect.DeepEqual(state.SyncRecords[syncRecord.ID], syncRecord) ||
				!reflect.DeepEqual(state.AuditEvents[audit.ID], audit) ||
				!reflect.DeepEqual(state.Outbox[outbox.ID], outbox) {
				t.Fatalf("immutable conflict escaped atomic rollback: %#v", state)
			}
		})
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

	authorizedStore := NewFileStore(t.TempDir())
	fixture := newStoredReleaseFixture(t, authorizedStore, true)
	persistStoredReleaseBaseline(t, ctx, authorizedStore, fixture, true)
	if err := authorizedStore.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
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

func TestFileStoreImmutableOperationalIDsUseCanonicalReplayAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	now := time.Now()
	syncRecord := core.SyncRecord{
		ID: "sync-canonical", ProjectID: "payments", Result: core.SyncResultFailure,
		StartedAt: now, FinishedAt: now.Add(time.Second),
	}
	audit := core.AuditEvent{
		ID: "audit-canonical", ContractID: "payments", Kind: "canonical-replay", OccurredAt: now,
	}
	outbox := core.OutboxMessage{
		ID: "outbox-canonical", ContractID: "payments", Topic: "canonical-replay",
		Payload: []byte(`{"ok":true}`), CreatedAt: now,
	}
	first := NewFileStore(root)
	if err := first.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveSyncRecord(ctx, syncRecord); err != nil {
			return err
		}
		if err := operational.AppendAuditEvent(ctx, audit); err != nil {
			return err
		}
		return operational.Enqueue(ctx, outbox)
	}); err != nil {
		t.Fatal(err)
	}
	if err := NewFileStore(root).Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveSyncRecord(ctx, syncRecord); err != nil {
			return err
		}
		if err := operational.AppendAuditEvent(ctx, audit); err != nil {
			return err
		}
		return operational.Enqueue(ctx, outbox)
	}); err != nil {
		t.Fatalf("canonical immutable replay after restart: %v", err)
	}
}

func TestFileStoreRejectsInventedReleaseAuthorityAndMissingTransitionEffects(t *testing.T) {
	ctx := context.Background()

	t.Run("invented authority", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		fixture := newStoredReleaseFixture(t, store, false)
		persistStoredReleaseBaseline(t, ctx, store, fixture, false)

		err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
				return err
			}
			return appendStoredReleaseEffects(ctx, operational, fixture)
		})
		if err == nil {
			t.Fatal("release decision with no persisted track authorization committed")
		}
		assertStoredReleaseBaseline(t, ctx, store, fixture)
	})

	t.Run("missing effects", func(t *testing.T) {
		root := t.TempDir()
		store := NewFileStore(root)
		fixture := newStoredReleaseFixture(t, store, false)
		persistStoredReleaseBaseline(t, ctx, store, fixture, true)

		err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			return operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next)
		})
		if err == nil {
			t.Fatal("release decision with no deterministic audit and outbox effects committed")
		}
		assertStoredReleaseBaseline(t, ctx, NewFileStore(root), fixture)
	})

	t.Run("authorization is immutable across restart", func(t *testing.T) {
		root := t.TempDir()
		store := NewFileStore(root)
		fixture := newStoredReleaseFixture(t, store, false)
		persistStoredReleaseBaseline(t, ctx, store, fixture, true)
		restarted := NewFileStore(root)
		if err := restarted.SaveReleaseAuthorization(ctx, fixture.authorization); err != nil {
			t.Fatalf("identical authorization replay: %v", err)
		}
		conflicting := fixture.authorization
		conflicting.PublicPath = "/payments/other"
		if err := restarted.SaveReleaseAuthorization(ctx, conflicting); err == nil {
			t.Fatal("conflicting release authorization overwrote immutable evidence")
		}
		evidence, err := NewFileStore(root).ReleaseEvidence(
			ctx,
			fixture.authorization.ContractID,
			fixture.authorization.TrackID,
			fixture.authorization.ReviewID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(evidence.Authorization, fixture.authorization) {
			t.Fatalf("conflict changed persisted authorization: %#v", evidence.Authorization)
		}
	})

	t.Run("complete bundle survives restart", func(t *testing.T) {
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
			t.Fatalf("persist complete release authority bundle: %v", err)
		}
		restarted := NewFileStore(root)
		got, err := restarted.ReleaseTrack(ctx, fixture.next.ContractID, fixture.next.ID)
		if err != nil {
			t.Fatalf("load complete release authority bundle after restart: %v", err)
		}
		if !reflect.DeepEqual(got, fixture.next) {
			t.Fatalf("restarted release track = %#v, want %#v", got, fixture.next)
		}

		statePath := filepath.Join(root, "operational", "state.json")
		original, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct {
			name   string
			mutate func(map[string]any)
			probe  func(*FileStore) error
		}{
			{
				name: "authorization",
				mutate: func(state map[string]any) {
					delete(state["releaseAuthorizations"].(map[string]any), fixture.review.ID)
				},
			},
			{
				name: "review",
				mutate: func(state map[string]any) {
					delete(state["reviews"].(map[string]any), fixture.review.ID)
				},
			},
			{
				name: "review digest",
				mutate: func(state map[string]any) {
					track := state["releaseTracks"].(map[string]any)[releaseTrackKey("payments", "stable")].(map[string]any)
					track["lastDecision"].(map[string]any)["reviewDigest"] =
						"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				},
			},
			{
				name: "candidate blob binding",
				mutate: func(state map[string]any) {
					revisions := state["revisions"].(map[string]any)
					baseline := revisions[revisionKey(
						fixture.authorization.ContractID,
						fixture.authorization.BaselineRevisionID,
					)].(map[string]any)
					candidate := revisions[revisionKey(
						fixture.authorization.ContractID,
						fixture.authorization.CandidateRevisionID,
					)].(map[string]any)
					candidate["specBlobKey"] = baseline["specBlobKey"]
				},
				probe: func(store *FileStore) error {
					_, err := store.ReleaseEvidence(
						ctx,
						fixture.authorization.ContractID,
						fixture.authorization.TrackID,
						fixture.authorization.ReviewID,
					)
					return err
				},
			},
			{
				name: "sync",
				mutate: func(state map[string]any) {
					delete(state["syncRecords"].(map[string]any), fixture.syncRecord.ID)
				},
			},
			{
				name: "audit",
				mutate: func(state map[string]any) {
					for id := range state["auditEvents"].(map[string]any) {
						delete(state["auditEvents"].(map[string]any), id)
					}
				},
			},
			{
				name: "outbox",
				mutate: func(state map[string]any) {
					for id := range state["outbox"].(map[string]any) {
						delete(state["outbox"].(map[string]any), id)
					}
				},
			},
		} {
			t.Run("restart rejects stripped "+test.name, func(t *testing.T) {
				state := readOperationalStateJSON(t, root)
				test.mutate(state)
				encoded, err := json.MarshalIndent(state, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := durableAtomicWrite(statePath, append(encoded, '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
				probe := test.probe
				if probe == nil {
					probe = func(store *FileStore) error {
						_, err := store.ReleaseTrack(ctx, fixture.next.ContractID, fixture.next.ID)
						return err
					}
				}
				if err := probe(NewFileStore(root)); err == nil {
					t.Fatalf("stripped %s was accepted after restart", test.name)
				}
				if err := durableAtomicWrite(statePath, original, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := NewFileStore(root).ReleaseTrack(ctx, fixture.next.ContractID, fixture.next.ID); err != nil {
					t.Fatalf("restore last-known-good after stripped %s: %v", test.name, err)
				}
			})
		}
	})

	t.Run("pinned promotion requires deterministic effects", func(t *testing.T) {
		root := t.TempDir()
		store := NewFileStore(root)
		fixture := newStoredReleaseFixture(t, store, true)
		persistStoredReleaseBaseline(t, ctx, store, fixture, true)
		if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
				return err
			}
			return appendStoredReleaseEffects(ctx, operational, fixture)
		}); err != nil {
			t.Fatal(err)
		}
		promoted, err := core.PromoteReleaseRevision(fixture.next, fixture.authorization.CandidateRevisionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			return operational.SaveReleaseTrack(ctx, fixture.next.Generation, promoted)
		}); err == nil {
			t.Fatal("pinned promotion without publication/audit/outbox committed")
		}
		if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			if err := operational.SaveReleaseTrack(ctx, fixture.next.Generation, promoted); err != nil {
				return err
			}
			if err := operational.SavePublication(ctx, core.Publication{
				ProjectID: fixture.authorization.ContractID, RevisionID: fixture.authorization.CandidateRevisionID,
				Public: true, Path: fixture.authorization.PublicPath,
			}); err != nil {
				return err
			}
			eventID := releaseTransitionEvidenceID(
				fixture.authorization,
				true,
				promoted.Generation,
				"release.track.promoted",
			)
			if err := operational.AppendAuditEvent(ctx, core.AuditEvent{
				ID: eventID, ContractID: fixture.authorization.ContractID, TrackID: fixture.authorization.TrackID,
				RevisionID: fixture.authorization.CandidateRevisionID,
				Kind:       "release.track.promoted", OccurredAt: promoted.LastDecision.EvaluatedAt.Add(time.Minute),
			}); err != nil {
				return err
			}
			return operational.Enqueue(ctx, core.OutboxMessage{
				ID:         "outbox-" + strings.TrimPrefix(eventID, "audit-"),
				ContractID: fixture.authorization.ContractID, TrackID: fixture.authorization.TrackID,
				RevisionID: fixture.authorization.CandidateRevisionID,
				Topic:      "release.track.promoted", CreatedAt: promoted.LastDecision.EvaluatedAt.Add(time.Minute),
			})
		}); err != nil {
			t.Fatalf("persist authenticated pinned promotion: %v", err)
		}
		got, err := NewFileStore(root).ReleaseTrack(ctx, promoted.ContractID, promoted.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, promoted) {
			t.Fatalf("restarted promoted track = %#v, want %#v", got, promoted)
		}
	})
}

func TestReleaseServiceUsesFileStoreAuthenticatedEvidenceBoundary(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)

	t.Run("persisted authorization advances and survives restart", func(t *testing.T) {
		root := t.TempDir()
		store := NewFileStore(root)
		fixture := newStoredReleaseFixture(t, store, false)
		persistStoredReleaseBaseline(t, ctx, store, fixture, true)
		service, err := application.NewReleaseService(application.ReleaseDependencies{
			Revisions: store, Evidence: store, UnitOfWork: store, Clock: fixedReleaseClock{now: now},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Coordinate(ctx, application.ReleaseCommand{
			ContractID: fixture.authorization.ContractID, TrackID: fixture.authorization.TrackID,
			RevisionID: fixture.authorization.CandidateRevisionID,
			Review:     core.ContractReview{ID: fixture.authorization.ReviewID},
			SyncRecord: core.SyncRecord{ID: fixture.authorization.SyncRecordID},
		})
		if err != nil {
			t.Fatalf("coordinate authenticated file-store release: %v", err)
		}
		if !reflect.DeepEqual(result.Track, fixture.next) {
			t.Fatalf("coordinated track = %#v, want %#v", result.Track, fixture.next)
		}
		restarted := NewFileStore(root)
		got, err := restarted.ReleaseTrack(ctx, fixture.next.ContractID, fixture.next.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, fixture.next) {
			t.Fatalf("restarted coordinated track = %#v, want %#v", got, fixture.next)
		}
		evidence, err := restarted.ReleaseEvidence(
			ctx,
			fixture.authorization.ContractID,
			fixture.authorization.TrackID,
			fixture.authorization.ReviewID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(evidence.Authorization, fixture.authorization) ||
			!reflect.DeepEqual(evidence.Review, fixture.review) ||
			!reflect.DeepEqual(evidence.SyncRecord, fixture.syncRecord) {
			t.Fatalf("restarted release evidence = %#v", evidence)
		}
	})

	t.Run("custom evidence reader cannot invent authority", func(t *testing.T) {
		root := t.TempDir()
		store := NewFileStore(root)
		fixture := newStoredReleaseFixture(t, store, false)
		persistStoredReleaseBaseline(t, ctx, store, fixture, true)
		forged := core.ReleaseEvidence{
			Authorization: fixture.authorization,
			Review:        fixture.review,
			SyncRecord:    fixture.syncRecord,
		}
		forged.Authorization.ReviewID = "review-forged"
		forged.Authorization.SyncRecordID = "sync-forged"
		forged.Review.ID = forged.Authorization.ReviewID
		forged.SyncRecord.ID = forged.Authorization.SyncRecordID
		service, err := application.NewReleaseService(application.ReleaseDependencies{
			Revisions: store, Evidence: fixedReleaseEvidenceReader{evidence: forged},
			UnitOfWork: store, Clock: fixedReleaseClock{now: now},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Coordinate(ctx, application.ReleaseCommand{
			ContractID: forged.Authorization.ContractID, TrackID: forged.Authorization.TrackID,
			RevisionID: forged.Authorization.CandidateRevisionID,
			Review:     core.ContractReview{ID: forged.Authorization.ReviewID},
			SyncRecord: core.SyncRecord{ID: forged.Authorization.SyncRecordID},
		}); err == nil {
			t.Fatal("custom reader invented release authority")
		}
		assertStoredReleaseBaseline(t, ctx, NewFileStore(root), fixture)
	})
}

func TestFileStoreSerializesGenerationCASAcrossHandles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first := NewFileStore(root)
	second := NewFileStore(root)
	fixtureA := newStoredReleaseFixtureNamed(
		t, false, "review-a", "sync-a", time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	)
	fixtureB := newStoredReleaseFixtureNamed(
		t, false, "review-b", "sync-b", time.Date(2026, 7, 26, 12, 1, 0, 0, time.UTC),
	)
	persistStoredReleaseBaseline(t, ctx, first, fixtureA, true)
	persistStoredReleaseBaseline(t, ctx, first, fixtureB, true)

	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		results <- first.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			if _, err := operational.ReleaseTrack(ctx, "payments", "stable"); err != nil {
				return err
			}
			close(firstEntered)
			select {
			case <-secondEntered:
			case <-time.After(200 * time.Millisecond):
			}
			if err := operational.SaveReleaseTrack(ctx, 0, fixtureA.next); err != nil {
				return err
			}
			return appendStoredReleaseEffects(ctx, operational, fixtureA)
		})
	}()
	<-firstEntered
	go func() {
		results <- second.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
			if _, err := operational.ReleaseTrack(ctx, "payments", "stable"); err != nil {
				return err
			}
			close(secondEntered)
			if err := operational.SaveReleaseTrack(ctx, 0, fixtureB.next); err != nil {
				return err
			}
			return appendStoredReleaseEffects(ctx, operational, fixtureB)
		})
	}()

	errs := []error{<-results, <-results}
	successes := 0
	conflicts := 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, port.ErrGenerationConflict):
			conflicts++
		default:
			t.Fatalf("concurrent writer error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("two-handle CAS results successes=%d conflicts=%d errors=%v", successes, conflicts, errs)
	}
	got, err := NewFileStore(root).ReleaseTrack(ctx, "payments", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if got.Generation != 1 {
		t.Fatalf("two-handle CAS persisted generation %d, want 1", got.Generation)
	}
	winner := fixtureA
	if got.LastDecision != nil && got.LastDecision.ReviewID == fixtureB.review.ID {
		winner = fixtureB
	}
	beforeReplay, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, 0, got); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, winner)
	}); err != nil {
		t.Fatalf("identical stale-CAS replay: %v", err)
	}
	afterReplay, err := os.ReadFile(filepath.Join(root, "operational", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeReplay, afterReplay) {
		t.Fatal("identical stale-CAS replay changed operational state")
	}
}

func TestFileStoreOperationalLockAcquisitionHonorsContext(t *testing.T) {
	root := t.TempDir()
	first := NewFileStore(root)
	second := NewFileStore(root)
	locked := make(chan struct{})
	release := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- first.Within(context.Background(), func(context.Context, port.OperationalStore) error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	callbackRan := false
	err := second.Within(ctx, func(context.Context, port.OperationalStore) error {
		callbackRan = true
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended lock error = %v, want deadline exceeded", err)
	}
	if callbackRan {
		t.Fatal("callback ran without acquiring the operational lock")
	}
	close(release)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}
}

func TestFileStoreRejectsPaddedSecurityIdentities(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name string
		run  func(*FileStore) error
	}{
		{
			name: "revision id",
			run: func(store *FileStore) error {
				return store.SaveRevision(ctx, core.ContractRevision{ID: " revision-next"})
			},
		},
		{
			name: "revision ref",
			run: func(store *FileStore) error {
				return store.SaveRevision(ctx, core.ContractRevision{ID: "revision-next", Ref: "refs/heads/main "})
			},
		},
		{
			name: "review id",
			run: func(store *FileStore) error {
				return store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
					for _, revisionID := range []string{"revision-good", "revision-next"} {
						if err := operational.SaveRevision(ctx, core.ContractRevision{ID: revisionID, ContractID: "payments"}); err != nil {
							return err
						}
					}
					return operational.SaveReview(ctx, core.ContractReview{
						ID: "review-next ", ContractID: "payments",
						BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
					})
				})
			},
		},
		{
			name: "sync ref",
			run: func(store *FileStore) error {
				return store.SaveSyncRecord(ctx, core.SyncRecord{
					ID: "sync-next", ProjectID: "payments", Ref: "refs/heads/main ",
					Result: core.SyncResultFailure,
				})
			},
		},
		{
			name: "audit actor",
			run: func(store *FileStore) error {
				return store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
					return operational.AppendAuditEvent(ctx, core.AuditEvent{
						ID: "audit-next", ContractID: "payments", ActorID: " operator",
					})
				})
			},
		},
		{
			name: "outbox contract",
			run: func(store *FileStore) error {
				return store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
					return operational.Enqueue(ctx, core.OutboxMessage{
						ID: "outbox-next", ContractID: "payments ",
					})
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(NewFileStore(t.TempDir())); err == nil ||
				!strings.Contains(err.Error(), "whitespace") {
				t.Fatalf("padded identity error = %v, want whitespace rejection", err)
			}
		})
	}
}

func TestFileStoreRejectsReleaseGenerationOverflowAcrossRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	legacy := newOperationalState()
	legacy.Version = decisionOperationalStateVersion
	legacy.Revisions["revision-good"] = core.ContractRevision{ID: "revision-good", ContractID: "payments"}
	legacy.Revisions["revision-next"] = core.ContractRevision{ID: "revision-next", ContractID: "payments"}
	baseline := core.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main",
		Mode: core.ReleaseModePinned, Generation: ^uint64(0), CurrentRevisionID: "revision-good",
	}
	legacy.ReleaseTracks[releaseTrackKey(baseline.ContractID, baseline.ID)] = baseline
	legacy.ReleaseTrackAuthorities[releaseTrackKey(baseline.ContractID, baseline.ID)] =
		newReleaseTrackAuthority(baseline)
	if err := store.publishOperationalState(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(root).ReleaseTrack(ctx, baseline.ContractID, baseline.ID); err != nil {
		t.Fatalf("migrate exhausted legacy baseline: %v", err)
	}

	decision := core.ReleaseDecision{
		RevisionID: "revision-next", ReviewID: "review-next",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      core.VerdictFail,
		EvaluatedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	if _, _, err := core.ConsiderReleaseDecision(baseline, decision); err == nil {
		t.Fatal("domain release decision wrapped exhausted generation")
	}
	forged := core.CloneReleaseTrack(baseline)
	forged.Generation = 0
	forged.CandidateRevisionID = decision.RevisionID
	forged.LastDecision = &decision
	if err := NewFileStore(root).Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		return operational.SaveReleaseTrack(ctx, baseline.Generation, forged)
	}); err == nil {
		t.Fatal("store persisted wrapped release generation")
	}
	got, err := NewFileStore(root).ReleaseTrack(ctx, baseline.ContractID, baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, baseline) {
		t.Fatalf("overflow attempt changed restarted track: got=%#v want=%#v", got, baseline)
	}
}

func TestFileStoreReportsIndeterminateCommitAfterManifestRename(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewFileStore(root)
	fixture := newStoredReleaseFixture(t, store, false)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	store.confirmReplacement = func(string) error {
		return errors.New("forced replacement confirmation failure")
	}

	err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		current, err := operational.ReleaseTrack(ctx, "payments", "stable")
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current, fixture.baseline) {
			return fmt.Errorf("loaded release baseline = %#v, want %#v", current, fixture.baseline)
		}
		if err := operational.SaveReleaseTrack(ctx, current.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
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
}

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
	fixture := newStoredReleaseFixture(t, store, true)
	persistStoredReleaseBaseline(t, ctx, store, fixture, true)
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, fixture.baseline.Generation, fixture.next); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, fixture)
	}); err != nil {
		t.Fatal(err)
	}
	track := fixture.next
	decision := *track.LastDecision

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
	store := NewFileStore(root)
	acceptedFixture := newStoredReleaseFixtureNamed(t, true, "review-accepted", "sync-accepted", acceptedAt)
	rejectedFixture := newStoredReleaseFixtureNamed(t, false, "review-rejected", "sync-rejected", acceptedAt.Add(time.Minute))
	persistStoredReleaseBaseline(t, ctx, store, acceptedFixture, true)
	persistStoredReleaseBaseline(t, ctx, store, rejectedFixture, true)
	baseline := acceptedFixture.baseline
	acceptedTrack := acceptedFixture.next
	acceptedDecision := *acceptedTrack.LastDecision
	track, changed, err := core.ConsiderReleaseDecision(acceptedTrack, *rejectedFixture.next.LastDecision)
	if err != nil || !changed {
		t.Fatalf("apply rejection: changed=%t err=%v", changed, err)
	}
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, baseline.Generation, acceptedTrack); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, acceptedFixture)
	}); err != nil {
		t.Fatal(err)
	}
	rejectedFixture.next = track
	if err := store.Within(ctx, func(ctx context.Context, operational port.OperationalStore) error {
		if err := operational.SaveReleaseTrack(ctx, acceptedTrack.Generation, track); err != nil {
			return err
		}
		return appendStoredReleaseEffects(ctx, operational, rejectedFixture)
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
	operationalStaging := filepath.Join(root, "operational", ".write-interrupted.tmp")
	blobStaging := filepath.Join(root, "blobs", "sha256", ".write-interrupted.tmp")
	stagingFiles := []string{operationalStaging, blobStaging}
	for _, staging := range stagingFiles {
		if err := os.MkdirAll(filepath.Dir(staging), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(staging, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := NewFileStore(root)
	if err := store.Within(context.Background(), func(context.Context, port.OperationalStore) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(operationalStaging); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("operational staging file after restart error = %v, want not exist", err)
	}
	if _, err := os.Stat(blobStaging); err != nil {
		t.Fatalf("unrelated blob staging file should not be removed: %v", err)
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
		t.Fatalf("legacy read did not persist authenticated current state: %v", err)
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
