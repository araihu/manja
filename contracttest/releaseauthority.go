package contracttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type ReleaseAuthorityUnitOfWorkFixture struct {
	UnitOfWork     port.UnitOfWork
	Blobs          port.BlobStore
	Authorizations port.ReleaseAuthorizationWriter
}

type ReleaseAuthorityUnitOfWorkFactory func(testing.TB) ReleaseAuthorityUnitOfWorkFixture

// ReleaseAuthorityUnitOfWork verifies the optional authenticated release
// persistence boundary without expanding OperationalStore's eight methods.
func ReleaseAuthorityUnitOfWork(t *testing.T, factory ReleaseAuthorityUnitOfWorkFactory) {
	t.Helper()
	if factory == nil {
		t.Fatal("release authority unit of work factory is required")
	}

	t.Run("invented authority fails atomically", func(t *testing.T) {
		fixture := factory(t)
		requireReleaseAuthorityFixture(t, fixture)
		ctx := markedContext(t)
		evidence := seedReleaseAuthorityFixture(t, ctx, fixture, false)
		err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveReleaseTrack(ctx, evidence.baseline.Generation, evidence.next); err != nil {
				return err
			}
			return appendReleaseAuthorityEffects(ctx, store, evidence)
		})
		if err == nil {
			t.Fatal("invented release authority committed")
		}
		assertReleaseAuthorityTrack(t, ctx, fixture.UnitOfWork, evidence.baseline)
	})

	t.Run("missing deterministic effects fails atomically", func(t *testing.T) {
		fixture := factory(t)
		requireReleaseAuthorityFixture(t, fixture)
		ctx := markedContext(t)
		evidence := seedReleaseAuthorityFixture(t, ctx, fixture, true)
		err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			return store.SaveReleaseTrack(ctx, evidence.baseline.Generation, evidence.next)
		})
		if err == nil {
			t.Fatal("release transition without audit and outbox committed")
		}
		assertReleaseAuthorityTrack(t, ctx, fixture.UnitOfWork, evidence.baseline)
	})

	t.Run("nested release state is isolated on read and save", func(t *testing.T) {
		fixture := factory(t)
		requireReleaseAuthorityFixture(t, fixture)
		ctx := markedContext(t)
		evidence := seedReleaseAuthorityFixture(t, ctx, fixture, true)
		input := domain.CloneReleaseTrack(evidence.next)
		if err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveReleaseTrack(ctx, evidence.baseline.Generation, input); err != nil {
				return err
			}
			input.LastDecision.ReviewDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			return appendReleaseAuthorityEffects(ctx, store, evidence)
		}); err != nil {
			t.Fatalf("commit authenticated release transition: %v", err)
		}
		persisted, err := loadTrack(ctx, fixture.UnitOfWork, evidence.next.ContractID, evidence.next.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(persisted, evidence.next) {
			t.Fatalf("saved release track retained caller alias: got=%#v want=%#v", persisted, evidence.next)
		}
		if err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			returned, err := store.ReleaseTrack(ctx, evidence.next.ContractID, evidence.next.ID)
			if err != nil {
				return err
			}
			returned.LastDecision.Accepted = true
			returned.LastDecision.Verdict = domain.VerdictFail
			return nil
		}); err != nil {
			t.Fatalf("mutate returned nested release state: %v", err)
		}
		assertReleaseAuthorityTrack(t, ctx, fixture.UnitOfWork, evidence.next)
	})

	t.Run("exact authenticated replay is concurrency safe", func(t *testing.T) {
		fixture := factory(t)
		requireReleaseAuthorityFixture(t, fixture)
		ctx := markedContext(t)
		evidence := seedReleaseAuthorityFixture(t, ctx, fixture, true)
		const attempts = 8
		start := make(chan struct{})
		results := make(chan error, attempts)
		var wait sync.WaitGroup
		for index := 0; index < attempts; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				results <- fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
					if err := store.SaveReleaseTrack(ctx, evidence.baseline.Generation, evidence.next); err != nil {
						return err
					}
					return appendReleaseAuthorityEffects(ctx, store, evidence)
				})
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Errorf("authenticated exact replay: %v", err)
			}
		}
		assertReleaseAuthorityTrack(t, ctx, fixture.UnitOfWork, evidence.next)
	})

	t.Run("authenticated decision evidence cannot be stripped", func(t *testing.T) {
		fixture := factory(t)
		requireReleaseAuthorityFixture(t, fixture)
		ctx := markedContext(t)
		evidence := seedReleaseAuthorityFixture(t, ctx, fixture, true)
		if err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveReleaseTrack(ctx, evidence.baseline.Generation, evidence.next); err != nil {
				return err
			}
			return appendReleaseAuthorityEffects(ctx, store, evidence)
		}); err != nil {
			t.Fatal(err)
		}
		err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			stripped, err := store.ReleaseTrack(ctx, evidence.next.ContractID, evidence.next.ID)
			if err != nil {
				return err
			}
			stripped.LastDecision = nil
			stripped.CandidateRevisionID = ""
			return store.SaveReleaseTrack(ctx, evidence.next.Generation, stripped)
		})
		if err == nil {
			t.Fatal("authenticated release decision evidence was stripped")
		}
		assertReleaseAuthorityTrack(t, ctx, fixture.UnitOfWork, evidence.next)
	})

	t.Run("rejected authenticated decision cannot bypass publication", func(t *testing.T) {
		fixture := factory(t)
		requireReleaseAuthorityFixture(t, fixture)
		ctx := markedContext(t)
		evidence := seedReleaseAuthorityFixture(t, ctx, fixture, true)
		if err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveReleaseTrack(ctx, evidence.baseline.Generation, evidence.next); err != nil {
				return err
			}
			return appendReleaseAuthorityEffects(ctx, store, evidence)
		}); err != nil {
			t.Fatal(err)
		}
		err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			forged, err := store.ReleaseTrack(ctx, evidence.next.ContractID, evidence.next.ID)
			if err != nil {
				return err
			}
			forged.CurrentRevisionID = forged.CandidateRevisionID
			forged.CandidateRevisionID = ""
			forged.Generation++
			return store.SaveReleaseTrack(ctx, evidence.next.Generation, forged)
		})
		if err == nil {
			t.Fatal("rejected authenticated candidate bypassed its decision")
		}
		assertReleaseAuthorityTrack(t, ctx, fixture.UnitOfWork, evidence.next)
	})
}

type releaseAuthorityEvidence struct {
	baselineSpec  []byte
	candidateSpec []byte
	baseline      domain.ReleaseTrack
	next          domain.ReleaseTrack
	review        domain.ContractReview
	syncRecord    domain.SyncRecord
	authorization domain.ReleaseAuthorization
}

func seedReleaseAuthorityFixture(
	t *testing.T,
	ctx context.Context,
	fixture ReleaseAuthorityUnitOfWorkFixture,
	withAuthorization bool,
) releaseAuthorityEvidence {
	t.Helper()
	evidence := newReleaseAuthorityEvidence(t)
	baselineKey, err := fixture.Blobs.Put(ctx, evidence.baselineSpec)
	if err != nil {
		t.Fatal(err)
	}
	candidateKey, err := fixture.Blobs.Put(ctx, evidence.candidateSpec)
	if err != nil {
		t.Fatal(err)
	}
	baselineSnapshot := domain.NewContractSnapshot(
		evidence.review.ContractID,
		evidence.review.BaselineRevisionID,
		evidence.baselineSpec,
		domain.SpecIndex{},
	)
	candidateSnapshot := domain.NewContractSnapshot(
		evidence.review.ContractID,
		evidence.review.CandidateRevisionID,
		evidence.candidateSpec,
		domain.SpecIndex{},
	)
	if err := fixture.UnitOfWork.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
		if err := store.SaveRevision(ctx, domain.ContractRevision{
			ID: baselineSnapshot.RevisionID, ContractID: baselineSnapshot.ContractID,
			SourceID: evidence.authorization.SourceID, Ref: evidence.authorization.BoundRef,
			SpecBlobKey: string(baselineKey), SpecDigest: baselineSnapshot.SpecDigest,
			ContractDigest: baselineSnapshot.ContractDigest, ReviewSnapshot: &baselineSnapshot,
		}); err != nil {
			return err
		}
		if err := store.SaveRevision(ctx, domain.ContractRevision{
			ID: candidateSnapshot.RevisionID, ContractID: candidateSnapshot.ContractID,
			SourceID: evidence.authorization.SourceID, Ref: evidence.authorization.BoundRef,
			SpecBlobKey: string(candidateKey), SpecDigest: candidateSnapshot.SpecDigest,
			ContractDigest: candidateSnapshot.ContractDigest, ReviewSnapshot: &candidateSnapshot,
		}); err != nil {
			return err
		}
		if err := store.SaveReview(ctx, evidence.review); err != nil {
			return err
		}
		if err := store.SaveSyncRecord(ctx, evidence.syncRecord); err != nil {
			return err
		}
		return store.SaveReleaseTrack(ctx, 0, evidence.baseline)
	}); err != nil {
		t.Fatalf("seed release authority fixture: %v", err)
	}
	if withAuthorization {
		if err := fixture.Authorizations.SaveReleaseAuthorization(ctx, evidence.authorization); err != nil {
			t.Fatalf("save release authorization: %v", err)
		}
	}
	return evidence
}

func newReleaseAuthorityEvidence(t *testing.T) releaseAuthorityEvidence {
	t.Helper()
	evaluatedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	baselineSpec := []byte("contracttest baseline")
	candidateSpec := []byte("contracttest candidate")
	baselineSnapshot := domain.NewContractSnapshot("contract", "revision-good", baselineSpec, domain.SpecIndex{})
	candidateSnapshot := domain.NewContractSnapshot("contract", "revision-next", candidateSpec, domain.SpecIndex{})
	policy, err := domain.MergePolicy(domain.PolicyLayer{
		Name: "repository-default", Source: domain.PolicySourceRepository,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := domain.EvaluateReview(domain.ReviewRequest{
		ContractID: "contract", Target: baselineSnapshot, Candidate: candidateSnapshot, Release: &baselineSnapshot,
		Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: "contracttest",
	})
	if err != nil {
		t.Fatal(err)
	}
	report.Comparisons = []domain.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	review := domain.ContractReview{
		ID: "review-next", ContractID: "contract",
		BaselineRevisionID: baselineSnapshot.RevisionID,
		BaselineSpecDigest: baselineSnapshot.SpecDigest, BaselineContractDigest: baselineSnapshot.ContractDigest,
		CandidateRevisionID: candidateSnapshot.RevisionID,
		CandidateSpecDigest: candidateSnapshot.SpecDigest, CandidateContractDigest: candidateSnapshot.ContractDigest,
		Report: report,
	}
	canonical, err := domain.CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	decision := domain.ReleaseDecision{
		RevisionID: review.CandidateRevisionID, ReviewID: review.ID,
		ReviewDigest: hex.EncodeToString(digest[:]), Verdict: report.Verdict,
		EvaluatedAt: evaluatedAt,
	}
	baseline := domain.ReleaseTrack{
		ID: "stable", ContractID: "contract", BoundRef: "refs/heads/main",
		Mode: domain.ReleaseModePinned, CurrentRevisionID: review.BaselineRevisionID,
	}
	next, changed, err := domain.ConsiderReleaseDecision(baseline, decision)
	if err != nil || !changed {
		t.Fatalf("derive release authority fixture: changed=%t err=%v", changed, err)
	}
	syncRecord := domain.SyncRecord{
		ID: "sync-next", ProjectID: "contract", SourceID: "source-main",
		RevisionID: review.CandidateRevisionID, Ref: baseline.BoundRef,
		Result: domain.SyncResultSuccess, StartedAt: evaluatedAt.Add(-time.Minute), FinishedAt: evaluatedAt,
	}
	return releaseAuthorityEvidence{
		baselineSpec: baselineSpec, candidateSpec: candidateSpec,
		baseline: baseline, next: next, review: review, syncRecord: syncRecord,
		authorization: domain.ReleaseAuthorization{
			ContractID: "contract", TrackID: baseline.ID, ReviewID: review.ID, SyncRecordID: syncRecord.ID,
			BaselineRevisionID: review.BaselineRevisionID, CandidateRevisionID: review.CandidateRevisionID,
			SourceID: syncRecord.SourceID, BoundRef: baseline.BoundRef,
			PublicPath: "/contract/stable", PolicyDigest: report.PolicyDigest,
		},
	}
}

func appendReleaseAuthorityEffects(
	ctx context.Context,
	store port.OperationalStore,
	evidence releaseAuthorityEvidence,
) error {
	eventID := releaseAuthorityEvidenceID(
		evidence.authorization,
		evidence.next.LastDecision.Accepted,
		evidence.next.Generation,
	)
	if err := store.AppendAuditEvent(ctx, domain.AuditEvent{
		ID: eventID, ContractID: evidence.authorization.ContractID, TrackID: evidence.authorization.TrackID,
		RevisionID: evidence.authorization.CandidateRevisionID,
		Kind:       "release.track.considered", OccurredAt: evidence.next.LastDecision.EvaluatedAt,
	}); err != nil {
		return err
	}
	return store.Enqueue(ctx, domain.OutboxMessage{
		ID: "outbox-" + eventID[len("audit-"):], ContractID: evidence.authorization.ContractID,
		TrackID: evidence.authorization.TrackID, RevisionID: evidence.authorization.CandidateRevisionID,
		Topic: "release.track.updated", CreatedAt: evidence.next.LastDecision.EvaluatedAt,
	})
}

func releaseAuthorityEvidenceID(
	authorization domain.ReleaseAuthorization,
	accepted bool,
	generation uint64,
) string {
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

func requireReleaseAuthorityFixture(t *testing.T, fixture ReleaseAuthorityUnitOfWorkFixture) {
	t.Helper()
	if fixture.UnitOfWork == nil || fixture.Blobs == nil || fixture.Authorizations == nil {
		t.Fatal("release authority fixture requires unit of work, blob store, and authorization writer")
	}
}

func assertReleaseAuthorityTrack(
	t *testing.T,
	ctx context.Context,
	uow port.UnitOfWork,
	want domain.ReleaseTrack,
) {
	t.Helper()
	got, err := loadTrack(ctx, uow, want.ContractID, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release track = %#v, want %#v", got, want)
	}
}
