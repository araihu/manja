package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/araihu/manja/domain"
)

func TestReleaseServiceCommitsAcceptedFollowingTrackInvariant(t *testing.T) {
	ctx := context.Background()
	store := newTestOperationalStore()
	store.tracks["payments/v1"] = domain.ReleaseTrack{ID: "v1", ContractID: "payments", Mode: domain.ReleaseModeFollowing, Generation: 3, CurrentRevisionID: "revision-good"}
	uow := &testUnitOfWork{committed: store}
	clock := &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)}
	service, err := NewReleaseService(ReleaseDependencies{UnitOfWork: uow, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Coordinate(ctx, ReleaseCommand{
		ContractID: "payments",
		TrackID:    "v1",
		RevisionID: "revision-next",
		Accepted:   true,
		Review:     releaseReviewForTest("review-1", "payments", "revision-good", "revision-next"),
		SyncRecord: domain.SyncRecord{ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess},
		PublicPath: "/payments/v1",
		ActorID:    "manager-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Track.CurrentRevisionID != "revision-next" || result.Track.Generation != 4 {
		t.Fatalf("advanced track = %#v", result.Track)
	}
	wantCalls := []string{"track-read", "review", "sync", "track-write", "publication", "audit", "outbox"}
	if !reflect.DeepEqual(store.calls, wantCalls) {
		t.Fatalf("transaction calls = %#v, want %#v", store.calls, wantCalls)
	}
	if len(store.publications) != 1 || len(store.auditEvents) != 1 || len(store.outbox) != 1 {
		t.Fatalf("release evidence incomplete: publications=%d audit=%d outbox=%d", len(store.publications), len(store.auditEvents), len(store.outbox))
	}
	if event := store.auditEvents[0]; event.TrackID != "v1" || event.RevisionID != "revision-next" {
		t.Fatalf("audit event lacks release identity: %#v", event)
	}
	if message := store.outbox[0]; message.TrackID != "v1" || message.RevisionID != "revision-next" {
		t.Fatalf("outbox message lacks release identity: %#v", message)
	}
	for _, got := range store.contexts {
		assertSameContexts(t, ctx, got)
	}
}

func TestReleaseServiceRollsBackEveryWriteStage(t *testing.T) {
	for _, failAt := range []string{"review", "sync", "track-write", "publication", "audit", "outbox"} {
		t.Run(failAt, func(t *testing.T) {
			store := newTestOperationalStore()
			original := domain.ReleaseTrack{ID: "v1", ContractID: "payments", Mode: domain.ReleaseModeFollowing, Generation: 3, CurrentRevisionID: "revision-good"}
			store.tracks["payments/v1"] = original
			store.failAt = failAt
			service, err := NewReleaseService(ReleaseDependencies{
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Coordinate(context.Background(), ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review:     releaseReviewForTest("review-1", "payments", "revision-good", "revision-next"),
				SyncRecord: domain.SyncRecord{ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess}, PublicPath: "/payments/v1",
			})
			var appErr *Error
			if !errors.As(err, &appErr) || appErr.Kind != ErrorTransaction {
				t.Fatalf("Coordinate error = %#v, want transaction error", err)
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, original) {
				t.Fatalf("track changed after %s failure: %#v", failAt, got)
			}
			if len(store.reviews) != 0 || len(store.syncRecords) != 0 || len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatalf("partial commit after %s failure", failAt)
			}
		})
	}
}

func TestReleaseCommandValidationOrderIsDeterministic(t *testing.T) {
	service, err := NewReleaseService(ReleaseDependencies{
		UnitOfWork: &testUnitOfWork{committed: newTestOperationalStore()},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 256; attempt++ {
		_, err := service.Coordinate(context.Background(), ReleaseCommand{})
		if err == nil || err.Error() != "coordinate release: contract id is required" {
			t.Fatalf("validation attempt %d error = %v", attempt, err)
		}
	}
}

func TestReleaseServiceRejectsMismatchedReviewEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.ContractReview)
	}{
		{
			name: "report contract",
			mutate: func(review *domain.ContractReview) {
				review.Report.ContractID = "inventory"
			},
		},
		{
			name: "schema version",
			mutate: func(review *domain.ContractReview) {
				review.Report.SchemaVersion = "manja.review/v0"
			},
		},
		{
			name: "candidate revision",
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Candidate.RevisionID = "revision-other"
			},
		},
		{
			name: "candidate digest",
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Candidate.SpecDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			},
		},
		{
			name: "baseline digest",
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Baseline.ContractDigest = "9999999999999999999999999999999999999999999999999999999999999999"
			},
		},
		{
			name: "comparison kind",
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Kind = domain.ComparisonPullRequest
			},
		},
		{
			name: "comparison verdict",
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Policy.Verdict = domain.VerdictFail
			},
		},
		{
			name: "stale release baseline",
			mutate: func(review *domain.ContractReview) {
				review.BaselineRevisionID = "revision-stale"
				review.Report.Comparisons[0].Baseline.RevisionID = "revision-stale"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := domain.ReleaseTrack{
				ID: "v1", ContractID: "payments", Mode: domain.ReleaseModeFollowing,
				Generation: 3, CurrentRevisionID: "revision-good",
			}
			store := newTestOperationalStore()
			store.tracks["payments/v1"] = original
			service, err := NewReleaseService(ReleaseDependencies{
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}
			review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
			tt.mutate(&review)

			_, err = service.Coordinate(context.Background(), ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
				},
				PublicPath: "/payments/v1",
			})
			if err == nil {
				t.Fatal("release advanced with mismatched review evidence")
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, original) {
				t.Fatalf("track changed after rejected evidence: %#v", got)
			}
			if len(store.reviews) != 0 || len(store.syncRecords) != 0 || len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatal("rejected evidence produced release side effects")
			}
		})
	}
}

func TestReleaseServiceReplayIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newTestOperationalStore()
	store.tracks["payments/v1"] = domain.ReleaseTrack{
		ID: "v1", ContractID: "payments", Mode: domain.ReleaseModeFollowing,
		Generation: 3, CurrentRevisionID: "revision-good",
	}
	service, err := NewReleaseService(ReleaseDependencies{
		UnitOfWork: &testUnitOfWork{committed: store},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := ReleaseCommand{
		ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
		Review: releaseReviewForTest("review-1", "payments", "revision-good", "revision-next"),
		SyncRecord: domain.SyncRecord{
			ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
		},
		PublicPath: "/payments/v1",
		ActorID:    "manager-1",
	}

	first, err := service.Coordinate(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	stateAfterFirst := store.clone()
	second, err := service.Coordinate(ctx, command)
	if err != nil {
		t.Fatalf("replay accepted release: %v", err)
	}
	if !reflect.DeepEqual(second.Track, first.Track) {
		t.Fatalf("replay track = %#v, want %#v", second.Track, first.Track)
	}
	if second.Track.Generation != 4 {
		t.Fatalf("replay generation = %d, want 4", second.Track.Generation)
	}
	if !reflect.DeepEqual(store.tracks, stateAfterFirst.tracks) ||
		!reflect.DeepEqual(store.reviews, stateAfterFirst.reviews) ||
		!reflect.DeepEqual(store.syncRecords, stateAfterFirst.syncRecords) ||
		!reflect.DeepEqual(store.publications, stateAfterFirst.publications) ||
		!reflect.DeepEqual(store.auditEvents, stateAfterFirst.auditEvents) ||
		!reflect.DeepEqual(store.outbox, stateAfterFirst.outbox) {
		t.Fatalf("replay changed committed release state: %#v", store)
	}
}

func releaseReportForTest(contractID, baselineRevisionID, candidateRevisionID string) domain.ReviewReport {
	return domain.ReviewReport{
		SchemaVersion: domain.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    contractID,
		PolicyDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:       domain.VerdictPass,
		Comparisons: []domain.ComparisonReport{{
			Kind: domain.ComparisonReleaseImpact,
			Baseline: domain.SnapshotRef{
				RevisionID:     baselineRevisionID,
				SpecDigest:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				ContractDigest: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			},
			Candidate: domain.SnapshotRef{
				RevisionID:     candidateRevisionID,
				SpecDigest:     "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				ContractDigest: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
			},
			Policy: domain.PolicyResult{Verdict: domain.VerdictPass},
		}},
	}
}

func releaseReviewForTest(id, contractID, baselineRevisionID, candidateRevisionID string) domain.ContractReview {
	report := releaseReportForTest(contractID, baselineRevisionID, candidateRevisionID)
	return domain.ContractReview{
		ID:                      id,
		ContractID:              contractID,
		BaselineRevisionID:      baselineRevisionID,
		BaselineSpecDigest:      report.Comparisons[0].Baseline.SpecDigest,
		BaselineContractDigest:  report.Comparisons[0].Baseline.ContractDigest,
		CandidateRevisionID:     candidateRevisionID,
		CandidateSpecDigest:     report.Comparisons[0].Candidate.SpecDigest,
		CandidateContractDigest: report.Comparisons[0].Candidate.ContractDigest,
		Report:                  report,
	}
}
