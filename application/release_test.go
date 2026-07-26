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
	review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
	persistReleaseReviewRevisions(store, review)
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
		Review:     review,
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
	wantCalls := []string{"track-read", "revision-read", "revision-read", "review", "sync", "track-write", "publication", "audit", "outbox"}
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
			review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
			persistReleaseReviewRevisions(store, review)
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
				Review:     review,
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
			persistReleaseReviewRevisions(store, review)
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
	persistReleaseReviewRevisions(store, command.Review)

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
	if second.Track.LastDecision == nil || second.Track.LastDecision.ReviewID != command.Review.ID {
		t.Fatalf("replay identity = %#v, want review %q", second.Track.LastDecision, command.Review.ID)
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

func TestReleaseServiceRejectsCallerForgedPersistedEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.ContractReview)
	}{
		{
			name: "candidate spec digest",
			mutate: func(review *domain.ContractReview) {
				forged := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
				review.CandidateSpecDigest = forged
				review.Report.Comparisons[0].Candidate.SpecDigest = forged
			},
		},
		{
			name: "baseline contract digest",
			mutate: func(review *domain.ContractReview) {
				forged := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
				review.BaselineContractDigest = forged
				review.Report.Comparisons[0].Baseline.ContractDigest = forged
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
			review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
			persistReleaseReviewRevisions(store, review)
			tt.mutate(&review)
			service, err := NewReleaseService(ReleaseDependencies{
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = service.Coordinate(context.Background(), ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
				},
				PublicPath: "/payments/v1",
			})
			if err == nil {
				t.Fatal("release advanced with evidence that disagrees with persisted revisions")
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, original) {
				t.Fatalf("track changed after forged evidence: %#v", got)
			}
			if len(store.reviews) != 0 || len(store.syncRecords) != 0 || len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatal("forged evidence produced release side effects")
			}
		})
	}
}

func TestReleaseServiceAppliesPinnedRejectThenAcceptForSameRevision(t *testing.T) {
	store := newTestOperationalStore()
	store.tracks["payments/stable"] = domain.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: domain.ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
	persistReleaseReviewRevisions(store, review)
	service, err := NewReleaseService(ReleaseDependencies{
		UnitOfWork: &testUnitOfWork{committed: store},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := ReleaseCommand{
		ContractID: "payments", TrackID: "stable", RevisionID: "revision-next",
		Review: review,
		SyncRecord: domain.SyncRecord{
			ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
		},
		PublicPath: "/payments/stable",
		ActorID:    "manager-1",
	}

	if _, err := service.Coordinate(context.Background(), command); err != nil {
		t.Fatalf("reject pinned candidate: %v", err)
	}
	command.Accepted = true
	result, err := service.Coordinate(context.Background(), command)
	if err != nil {
		t.Fatalf("accept pinned candidate: %v", err)
	}
	if result.Track.Generation != 4 || result.Track.CandidateRevisionID != "revision-next" || result.Track.CurrentRevisionID != "revision-good" {
		t.Fatalf("reject then accept track = %#v", result.Track)
	}
	if result.Track.LastDecision == nil || !result.Track.LastDecision.Accepted || result.Track.LastDecision.ReviewID != review.ID {
		t.Fatalf("accepted decision not recorded: %#v", result.Track.LastDecision)
	}
	if len(store.auditEvents) != 2 || len(store.outbox) != 2 {
		t.Fatalf("reject then accept audit/outbox = %d/%d, want 2/2", len(store.auditEvents), len(store.outbox))
	}
}

func TestReleaseServiceRepeatedPinnedRejectionIsExactReplay(t *testing.T) {
	store := newTestOperationalStore()
	store.tracks["payments/stable"] = domain.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: domain.ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
	persistReleaseReviewRevisions(store, review)
	service, err := NewReleaseService(ReleaseDependencies{
		UnitOfWork: &testUnitOfWork{committed: store},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := ReleaseCommand{
		ContractID: "payments", TrackID: "stable", RevisionID: "revision-next",
		Review: review,
		SyncRecord: domain.SyncRecord{
			ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
		},
	}
	first, err := service.Coordinate(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	stateAfterFirst := store.clone()
	second, err := service.Coordinate(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.Track, first.Track) ||
		!reflect.DeepEqual(store.tracks, stateAfterFirst.tracks) ||
		!reflect.DeepEqual(store.reviews, stateAfterFirst.reviews) ||
		!reflect.DeepEqual(store.syncRecords, stateAfterFirst.syncRecords) ||
		!reflect.DeepEqual(store.publications, stateAfterFirst.publications) ||
		!reflect.DeepEqual(store.auditEvents, stateAfterFirst.auditEvents) ||
		!reflect.DeepEqual(store.outbox, stateAfterFirst.outbox) {
		t.Fatal("repeated rejection changed exact replay state")
	}
	if second.Track.LastDecision == nil || second.Track.LastDecision.ReviewID != review.ID {
		t.Fatalf("rejection replay identity = %#v", second.Track.LastDecision)
	}
}

func releaseReportForTest(contractID, baselineRevisionID, candidateRevisionID string) domain.ReviewReport {
	baseline := domain.NewContractSnapshot(contractID, baselineRevisionID, []byte("spec:"+baselineRevisionID), domain.SpecIndex{})
	candidate := domain.NewContractSnapshot(contractID, candidateRevisionID, []byte("spec:"+candidateRevisionID), domain.SpecIndex{})
	policy, err := domain.MergePolicy(domain.PolicyLayer{Name: "stable", Source: domain.PolicySourceRepository})
	if err != nil {
		panic(err)
	}
	report, err := domain.EvaluateReview(domain.ReviewRequest{
		ContractID: contractID, Target: baseline, Candidate: candidate, Release: &baseline,
		Policy: policy, EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), EngineVersion: "test",
	})
	if err != nil {
		panic(err)
	}
	report.Comparisons = []domain.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	return report
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

func persistReleaseReviewRevisions(store *testOperationalStore, review domain.ContractReview) {
	store.revisions[review.BaselineRevisionID] = domain.ContractRevision{
		ID: review.BaselineRevisionID, ContractID: review.ContractID,
		SpecDigest: review.BaselineSpecDigest, ContractDigest: review.BaselineContractDigest,
	}
	store.revisions[review.CandidateRevisionID] = domain.ContractRevision{
		ID: review.CandidateRevisionID, ContractID: review.ContractID,
		SpecDigest: review.CandidateSpecDigest, ContractDigest: review.CandidateContractDigest,
	}
}
