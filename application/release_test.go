package application

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
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
	command := ReleaseCommand{
		ContractID: "payments",
		TrackID:    "v1",
		RevisionID: "revision-next",
		Accepted:   true,
		Review:     review,
		SyncRecord: domain.SyncRecord{ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess},
		PublicPath: "/payments/v1",
		ActorID:    "manager-1",
	}
	authorizeTestRelease(store, command)
	service, err := NewReleaseService(ReleaseDependencies{
		Revisions: store, Evidence: store, UnitOfWork: uow, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Coordinate(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Track.CurrentRevisionID != "revision-next" || result.Track.Generation != 4 {
		t.Fatalf("advanced track = %#v", result.Track)
	}
	wantCalls := []string{"evidence-read", "revision-read", "revision-read", "track-read", "track-write", "publication", "audit", "outbox"}
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
	for _, failAt := range []string{"track-write", "publication", "audit", "outbox"} {
		t.Run(failAt, func(t *testing.T) {
			store := newTestOperationalStore()
			original := domain.ReleaseTrack{
				ID: "v1", ContractID: "payments", BoundRef: "refs/heads/main",
				Mode: domain.ReleaseModeFollowing, Generation: 3, CurrentRevisionID: "revision-good",
			}
			store.tracks["payments/v1"] = original
			review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
			persistReleaseReviewRevisions(store, review)
			command := ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
				},
				PublicPath: "/payments/v1",
			}
			authorizeTestRelease(store, command)
			store.failAt = failAt
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions:  store,
				Evidence:   store,
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Coordinate(context.Background(), command)
			var appErr *Error
			if !errors.As(err, &appErr) || appErr.Kind != ErrorTransaction {
				t.Fatalf("Coordinate error = %#v, want transaction error", err)
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, original) {
				t.Fatalf("track changed after %s failure: %#v", failAt, got)
			}
			if len(store.reviews) != 1 || len(store.syncRecords) != 1 ||
				len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatalf("partial commit after %s failure", failAt)
			}
		})
	}
}

func TestReleaseCommandValidationOrderIsDeterministic(t *testing.T) {
	store := newTestOperationalStore()
	service, err := NewReleaseService(ReleaseDependencies{
		Revisions:  store,
		Evidence:   store,
		UnitOfWork: &testUnitOfWork{committed: store},
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

func TestReleaseServiceRejectsNonCanonicalCommandIdentityBeforePorts(t *testing.T) {
	now := time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*ReleaseCommand)
	}{
		{name: "contract padding", mutate: func(command *ReleaseCommand) { command.ContractID = " payments " }},
		{name: "track control", mutate: func(command *ReleaseCommand) { command.TrackID = "stable\x00shadow" }},
		{name: "revision invalid utf8", mutate: func(command *ReleaseCommand) { command.RevisionID = "revision-\xff" }},
		{name: "review control", mutate: func(command *ReleaseCommand) { command.Review.ID = "review\x00shadow" }},
		{name: "sync invalid utf8", mutate: func(command *ReleaseCommand) { command.SyncRecord.ID = "sync-\xff" }},
		{name: "actor padding", mutate: func(command *ReleaseCommand) { command.ActorID = " manager " }},
		{name: "public path control", mutate: func(command *ReleaseCommand) { command.PublicPath = "/payments\x00shadow" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _, command := authorizedReleaseFixture(t, now)
			clock := &testClock{now: now}
			uow := &testUnitOfWork{committed: store}
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions: store, Evidence: store, UnitOfWork: uow, Clock: clock,
			})
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&command)

			if _, err := service.Coordinate(context.Background(), command); err == nil {
				t.Fatal("Coordinate accepted a non-canonical command identity")
			}
			if len(store.calls) != 0 || len(clock.contexts) != 0 || uow.ctx != nil {
				t.Fatalf("non-canonical command reached ports: calls=%v clock=%d transaction=%v", store.calls, len(clock.contexts), uow.ctx != nil)
			}
		})
	}
}

func TestNewReleaseServiceRequiresRevisionReader(t *testing.T) {
	_, err := NewReleaseService(ReleaseDependencies{
		Evidence:   newTestOperationalStore(),
		UnitOfWork: &testUnitOfWork{committed: newTestOperationalStore()},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err == nil {
		t.Fatal("release service accepted missing revision reader")
	}
}

func TestNewReleaseServiceRequiresReleaseEvidenceReader(t *testing.T) {
	store := newTestOperationalStore()
	_, err := NewReleaseService(ReleaseDependencies{
		Revisions:  store,
		UnitOfWork: &testUnitOfWork{committed: store},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err == nil {
		t.Fatal("release service accepted missing release evidence reader")
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
				ID: "v1", ContractID: "payments", BoundRef: "refs/heads/main", Mode: domain.ReleaseModeFollowing,
				Generation: 3, CurrentRevisionID: "revision-good",
			}
			store := newTestOperationalStore()
			store.tracks["payments/v1"] = original
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions:  store,
				Evidence:   store,
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}
			review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
			persistReleaseReviewRevisions(store, review)
			tt.mutate(&review)

			command := ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
				},
				PublicPath: "/payments/v1",
			}
			authorizeTestRelease(store, command)
			_, err = service.Coordinate(context.Background(), command)
			if err == nil {
				t.Fatal("release advanced with mismatched review evidence")
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, original) {
				t.Fatalf("track changed after rejected evidence: %#v", got)
			}
			if len(store.reviews) != 1 || len(store.syncRecords) != 1 ||
				len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
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
		Revisions:  store,
		Evidence:   store,
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
	authorizeTestRelease(store, command)

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
		mutate func(*domain.ContractReview, *testOperationalStore)
	}{
		{
			name: "caller candidate spec digest",
			mutate: func(review *domain.ContractReview, _ *testOperationalStore) {
				review.CandidateSpecDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			},
		},
		{
			name: "caller baseline contract digest",
			mutate: func(review *domain.ContractReview, _ *testOperationalStore) {
				review.BaselineContractDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			},
		},
		{
			name: "persisted candidate spec digest",
			mutate: func(review *domain.ContractReview, store *testOperationalStore) {
				forged := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
				review.CandidateSpecDigest = forged
				review.Report.Comparisons[0].Candidate.SpecDigest = forged
				revision := store.revisions[review.CandidateRevisionID]
				revision.SpecDigest = forged
				store.revisions[review.CandidateRevisionID] = revision
			},
		},
		{
			name: "persisted baseline contract digest",
			mutate: func(review *domain.ContractReview, store *testOperationalStore) {
				forged := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
				review.BaselineContractDigest = forged
				review.Report.Comparisons[0].Baseline.ContractDigest = forged
				revision := store.revisions[review.BaselineRevisionID]
				revision.ContractDigest = forged
				store.revisions[review.BaselineRevisionID] = revision
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := domain.ReleaseTrack{
				ID: "v1", ContractID: "payments", BoundRef: "refs/heads/main", Mode: domain.ReleaseModeFollowing,
				Generation: 3, CurrentRevisionID: "revision-good",
			}
			store := newTestOperationalStore()
			store.tracks["payments/v1"] = original
			review := releaseReviewForTest("review-1", "payments", "revision-good", "revision-next")
			persistReleaseReviewRevisions(store, review)
			tt.mutate(&review, store)
			command := ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
				},
				PublicPath: "/payments/v1",
			}
			authorizeTestRelease(store, command)
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions:  store,
				Evidence:   store,
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = service.Coordinate(context.Background(), command)
			if err == nil {
				t.Fatal("release advanced with evidence that disagrees with persisted revisions")
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, original) {
				t.Fatalf("track changed after forged evidence: %#v", got)
			}
			if len(store.reviews) != 1 || len(store.syncRecords) != 1 ||
				len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatal("forged evidence produced release side effects")
			}
		})
	}
}

func TestReleaseServiceRejectsRewrittenCanonicalReviewResults(t *testing.T) {
	tests := []struct {
		name   string
		except bool
		mutate func(*domain.ContractReview)
	}{
		{
			name:   "rewritten finding",
			except: true,
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Findings[0].Description = "forged description"
			},
		},
		{
			name:   "removed finding",
			except: true,
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Findings = nil
			},
		},
		{
			name:   "rewritten decision",
			except: true,
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Policy.Decisions[0].Excepted = false
			},
		},
		{
			name:   "rewritten applied exception",
			except: true,
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Policy.AppliedExceptions[0].Reason = "forged reason"
			},
		},
		{
			name:   "rewritten exception disposition",
			except: true,
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Policy.ExceptionDispositions[0].Disposition = domain.ExceptionDispositionExpired
			},
		},
		{
			name: "forged pass result and verdict",
			mutate: func(review *domain.ContractReview) {
				review.Report.Comparisons[0].Findings = nil
				review.Report.Comparisons[0].Policy = domain.PolicyResult{Verdict: domain.VerdictPass}
				review.Report.Verdict = domain.VerdictPass
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalTrack := domain.ReleaseTrack{
				ID: "v1", ContractID: "payments", BoundRef: "refs/heads/main", Mode: domain.ReleaseModeFollowing,
				Generation: 3, CurrentRevisionID: "revision-good",
			}
			store := newTestOperationalStore()
			store.tracks["payments/v1"] = originalTrack
			review, baseline, candidate := releaseReviewWithFindingAtForTest(
				"review-1",
				"payments",
				"revision-good",
				"revision-next",
				time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
				tt.except,
			)
			persistReleaseReviewSnapshots(store, review, baseline, candidate)
			review = cloneContractReviewForTest(t, review)
			tt.mutate(&review)
			command := ReleaseCommand{
				ContractID: "payments", TrackID: "v1", RevisionID: "revision-next", Accepted: true,
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
				},
				PublicPath: "/payments/v1",
			}
			authorizeTestRelease(store, command)
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions:  store,
				Evidence:   store,
				UnitOfWork: &testUnitOfWork{committed: store},
				Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = service.Coordinate(context.Background(), command)
			if err == nil {
				t.Fatal("release advanced with rewritten canonical review result")
			}
			if got := store.tracks["payments/v1"]; !reflect.DeepEqual(got, originalTrack) {
				t.Fatalf("track changed after rewritten review: %#v", got)
			}
			if len(store.reviews) != 1 || len(store.syncRecords) != 1 ||
				len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatal("rewritten review produced release side effects")
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
	rejectedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	rejectedReview := releaseReviewAtForTest("review-rejected", "payments", "revision-good", "revision-next", rejectedAt)
	persistReleaseReviewRevisions(store, rejectedReview)
	service, err := NewReleaseService(ReleaseDependencies{
		Revisions:  store,
		Evidence:   store,
		UnitOfWork: &testUnitOfWork{committed: store},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := ReleaseCommand{
		ContractID: "payments", TrackID: "stable", RevisionID: "revision-next",
		Review: rejectedReview,
		SyncRecord: domain.SyncRecord{
			ID: "sync-1", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
		},
		PublicPath: "/payments/stable",
		ActorID:    "manager-1",
	}
	authorizeTestRelease(store, command)

	if _, err := service.Coordinate(context.Background(), command); err != nil {
		t.Fatalf("reject pinned candidate: %v", err)
	}
	command.Accepted = true
	command.Review = releaseReviewAtForTest(
		"review-accepted",
		"payments",
		"revision-good",
		"revision-next",
		rejectedAt.Add(time.Minute),
	)
	command.SyncRecord.ID = "sync-accepted"
	authorizeTestRelease(store, command)
	result, err := service.Coordinate(context.Background(), command)
	if err != nil {
		t.Fatalf("accept pinned candidate: %v", err)
	}
	if result.Track.Generation != 4 || result.Track.CandidateRevisionID != "revision-next" || result.Track.CurrentRevisionID != "revision-good" {
		t.Fatalf("reject then accept track = %#v", result.Track)
	}
	if result.Track.LastDecision == nil || !result.Track.LastDecision.Accepted || result.Track.LastDecision.ReviewID != command.Review.ID {
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
		Revisions:  store,
		Evidence:   store,
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
	authorizeTestRelease(store, command)
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

func TestReleaseServiceHistoricalAcceptanceReplayHasNoSideEffects(t *testing.T) {
	store := newTestOperationalStore()
	store.tracks["payments/stable"] = domain.ReleaseTrack{
		ID: "stable", ContractID: "payments", Mode: domain.ReleaseModePinned,
		Generation: 2, CurrentRevisionID: "revision-good",
	}
	acceptedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	acceptedReview := releaseReviewAtForTest(
		"review-accepted",
		"payments",
		"revision-good",
		"revision-next",
		acceptedAt,
	)
	persistReleaseReviewRevisions(store, acceptedReview)
	service, err := NewReleaseService(ReleaseDependencies{
		Revisions:  store,
		Evidence:   store,
		UnitOfWork: &testUnitOfWork{committed: store},
		Clock:      &testClock{now: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	acceptedCommand := ReleaseCommand{
		ContractID: "payments", TrackID: "stable", RevisionID: "revision-next",
		Accepted: true, Review: acceptedReview,
		SyncRecord: domain.SyncRecord{
			ID: "sync-accepted", ProjectID: "payments", RevisionID: "revision-next", Result: domain.SyncResultSuccess,
		},
		PublicPath: "/payments/stable",
	}
	authorizeTestRelease(store, acceptedCommand)
	if _, err := service.Coordinate(context.Background(), acceptedCommand); err != nil {
		t.Fatalf("apply acceptance: %v", err)
	}
	rejectedCommand := acceptedCommand
	rejectedCommand.Accepted = false
	rejectedCommand.Review = releaseReviewAtForTest(
		"review-rejected",
		"payments",
		"revision-good",
		"revision-next",
		acceptedAt.Add(time.Minute),
	)
	rejectedCommand.SyncRecord.ID = "sync-rejected"
	rejectedCommand.PublicPath = ""
	authorizeTestRelease(store, rejectedCommand)
	rejected, err := service.Coordinate(context.Background(), rejectedCommand)
	if err != nil {
		t.Fatalf("apply newer rejection: %v", err)
	}
	stateAfterRejection := store.clone()

	replayed, err := service.Coordinate(context.Background(), acceptedCommand)
	if err != nil {
		t.Fatalf("replay historical acceptance: %v", err)
	}
	if !reflect.DeepEqual(replayed.Track, rejected.Track) {
		t.Fatalf("historical replay changed track: replayed=%#v rejected=%#v", replayed.Track, rejected.Track)
	}
	if !reflect.DeepEqual(store.tracks, stateAfterRejection.tracks) ||
		!reflect.DeepEqual(store.reviews, stateAfterRejection.reviews) ||
		!reflect.DeepEqual(store.syncRecords, stateAfterRejection.syncRecords) ||
		!reflect.DeepEqual(store.publications, stateAfterRejection.publications) ||
		!reflect.DeepEqual(store.auditEvents, stateAfterRejection.auditEvents) ||
		!reflect.DeepEqual(store.outbox, stateAfterRejection.outbox) {
		t.Fatal("historical acceptance replay produced release side effects")
	}
	if len(store.reviews) != 2 || len(store.syncRecords) != 2 ||
		len(store.publications) != 0 || len(store.auditEvents) != 2 || len(store.outbox) != 2 {
		t.Fatalf(
			"historical replay side effects reviews=%d sync=%d publications=%d audit=%d outbox=%d",
			len(store.reviews), len(store.syncRecords), len(store.publications), len(store.auditEvents), len(store.outbox),
		)
	}
	if _, err := domain.PromoteReleaseRevision(replayed.Track, "revision-next"); err == nil {
		t.Fatal("historical acceptance replay restored promotion authorization")
	}
}

func TestReleaseServiceRejectsTrackRefPolicyAndReaderSubstitution(t *testing.T) {
	now := time.Date(2026, 7, 26, 14, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		mutate func(*domain.ReleaseEvidence, *testOperationalStore) port.RevisionReader
		track  string
	}{
		{
			name: "feature ref sync on main track",
			mutate: func(evidence *domain.ReleaseEvidence, _ *testOperationalStore) port.RevisionReader {
				evidence.SyncRecord.Ref = "refs/heads/feature"
				return nil
			},
		},
		{
			name:  "review reused across tracks",
			track: "nightly",
		},
		{
			name: "caller selected exception policy",
			mutate: func(evidence *domain.ReleaseEvidence, store *testOperationalStore) port.RevisionReader {
				review, baseline, candidate := releaseReviewWithFindingAtForTest(
					evidence.Review.ID,
					evidence.Review.ContractID,
					evidence.Review.BaselineRevisionID,
					evidence.Review.CandidateRevisionID,
					evidence.Review.Report.EvaluatedAt,
					true,
				)
				evidence.Review = review
				persistReleaseReviewSnapshots(store, review, baseline, candidate)
				for revisionID, revision := range store.revisions {
					revision.SourceID = evidence.Authorization.SourceID
					revision.Ref = evidence.Authorization.BoundRef
					store.revisions[revisionID] = revision
				}
				return nil
			},
		},
		{
			name: "custom revision reader substitutes feature ref",
			mutate: func(_ *domain.ReleaseEvidence, store *testOperationalStore) port.RevisionReader {
				return substitutingRevisionReader{delegate: store, revisionID: "revision-next", ref: "refs/heads/feature"}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newTestOperationalStore()
			track := domain.ReleaseTrack{
				ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main",
				Mode: domain.ReleaseModeFollowing, Generation: 3, CurrentRevisionID: "revision-good",
			}
			store.tracks["payments/stable"] = track
			store.tracks["payments/nightly"] = domain.ReleaseTrack{
				ID: "nightly", ContractID: "payments", BoundRef: "refs/heads/main",
				Mode: domain.ReleaseModeFollowing, Generation: 7, CurrentRevisionID: "revision-good",
			}
			review := releaseReviewAtForTest(
				"review-next", "payments", "revision-good", "revision-next", now.Add(-time.Minute),
			)
			persistReleaseReviewRevisions(store, review)
			for revisionID, revision := range store.revisions {
				revision.SourceID = "payments-git"
				revision.Ref = "refs/heads/main"
				store.revisions[revisionID] = revision
			}
			evidence := domain.ReleaseEvidence{
				Authorization: domain.ReleaseAuthorization{
					ContractID: "payments", TrackID: "stable",
					ReviewID: review.ID, SyncRecordID: "sync-next",
					BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
					SourceID: "payments-git", BoundRef: "refs/heads/main",
					PublicPath: "/payments/stable", PolicyDigest: review.Report.PolicyDigest,
				},
				Review: review,
				SyncRecord: domain.SyncRecord{
					ID: "sync-next", ProjectID: "payments", SourceID: "payments-git",
					RevisionID: "revision-next", Ref: "refs/heads/main", Result: domain.SyncResultSuccess,
				},
			}
			var revisions port.RevisionReader = store
			if test.mutate != nil {
				if substitute := test.mutate(&evidence, store); substitute != nil {
					revisions = substitute
				}
			}
			evidenceReader := &testReleaseEvidenceReader{evidence: evidence}
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions: revisions, Evidence: evidenceReader,
				UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now},
			})
			if err != nil {
				t.Fatal(err)
			}
			trackID := test.track
			if trackID == "" {
				trackID = "stable"
			}
			_, err = service.Coordinate(context.Background(), ReleaseCommand{
				ContractID: "payments", TrackID: trackID, RevisionID: "revision-next", Accepted: true,
				Review: review, SyncRecord: evidence.SyncRecord, PublicPath: "/caller-selected",
			})
			if err == nil {
				t.Fatal("release advanced with evidence outside its persisted track authorization")
			}
			if got := store.tracks["payments/"+trackID]; got.CurrentRevisionID != "revision-good" {
				t.Fatalf("substitution changed last-known-good track: %#v", got)
			}
			if len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatal("rejected substitution produced release side effects")
			}
		})
	}
}

func TestReleaseServiceAcceptsResolvedRevisionWithAuthorizedRefObservation(t *testing.T) {
	now := time.Date(2026, 7, 26, 14, 0, 0, 0, time.UTC)
	store, evidence, command := authorizedReleaseFixture(t, now.Add(-time.Minute))
	const commit = "0123456789abcdef0123456789abcdef01234567"
	candidate := store.revisions[evidence.Authorization.CandidateRevisionID]
	candidate.Ref = commit
	candidate.CommitSHA = commit
	store.revisions[candidate.ID] = candidate
	evidence.SyncRecord.CommitSHA = commit
	store.syncRecords[evidence.SyncRecord.ID] = evidence.SyncRecord
	command.SyncRecord = domain.SyncRecord{ID: evidence.SyncRecord.ID}

	service, err := NewReleaseService(ReleaseDependencies{
		Revisions: store, Evidence: &testReleaseEvidenceReader{evidence: evidence},
		UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Coordinate(context.Background(), command)
	if err != nil {
		t.Fatalf("resolved revision rejected despite authorized ref observation: %v", err)
	}
	if result.Track.CurrentRevisionID != candidate.ID {
		t.Fatalf("resolved revision did not advance track: %#v", result.Track)
	}
}

func TestReleaseServiceBoundsReviewTimeWithTrustedClock(t *testing.T) {
	now := time.Date(2026, 7, 26, 14, 0, 0, 0, time.UTC)

	t.Run("accepts exact future boundary and replays after restart", func(t *testing.T) {
		store, evidence, command := authorizedReleaseFixture(t, now.Add(5*time.Minute))
		service, err := NewReleaseService(ReleaseDependencies{
			Revisions: store, Evidence: &testReleaseEvidenceReader{evidence: evidence},
			UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now},
		})
		if err != nil {
			t.Fatal(err)
		}
		first, err := service.Coordinate(context.Background(), command)
		if err != nil {
			t.Fatalf("accept boundary review: %v", err)
		}
		if first.Track.CurrentRevisionID != "revision-next" {
			t.Fatalf("boundary review did not advance: %#v", first.Track)
		}
		if publication := store.publications["payments/revision-next"]; publication.Path != "/payments/stable" {
			t.Fatalf("release used caller path instead of authorized path: %#v", publication)
		}

		stateAfterFirst := store.clone()
		restarted, err := NewReleaseService(ReleaseDependencies{
			Revisions: store, Evidence: &testReleaseEvidenceReader{evidence: evidence},
			UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now.Add(time.Hour)},
		})
		if err != nil {
			t.Fatal(err)
		}
		replay, err := restarted.Coordinate(context.Background(), command)
		if err != nil {
			t.Fatalf("replay after restart: %v", err)
		}
		if !reflect.DeepEqual(replay.Track, first.Track) ||
			!reflect.DeepEqual(store.tracks, stateAfterFirst.tracks) ||
			!reflect.DeepEqual(store.publications, stateAfterFirst.publications) ||
			!reflect.DeepEqual(store.auditEvents, stateAfterFirst.auditEvents) ||
			!reflect.DeepEqual(store.outbox, stateAfterFirst.outbox) {
			t.Fatal("exact replay after restart changed release state")
		}
	})

	t.Run("rejects beyond future boundary without side effects", func(t *testing.T) {
		store, evidence, command := authorizedReleaseFixture(t, now.Add(5*time.Minute+time.Nanosecond))
		service, err := NewReleaseService(ReleaseDependencies{
			Revisions: store, Evidence: &testReleaseEvidenceReader{evidence: evidence},
			UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Coordinate(context.Background(), command); err == nil {
			t.Fatal("review beyond trusted future skew advanced")
		}
		if got := store.tracks["payments/stable"]; got.Generation != 3 || got.CurrentRevisionID != "revision-good" {
			t.Fatalf("future review changed last-known-good track: %#v", got)
		}
		if len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
			t.Fatal("future review produced release side effects")
		}
	})
}

func TestReleaseServiceRevalidatesAppliedExceptionExpiryAtTrustedReleaseTime(t *testing.T) {
	now := time.Date(2026, 7, 26, 14, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		expiresAt time.Time
		wantError bool
	}{
		{name: "expires before trusted now", expiresAt: now.Add(-time.Nanosecond), wantError: true},
		{name: "expires at trusted now", expiresAt: now, wantError: true},
		{name: "expires after trusted now", expiresAt: now.Add(time.Nanosecond)},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, evidence, command := authorizedReleaseWithAppliedException(t, test.expiresAt)
			original := domain.CloneReleaseTrack(store.tracks["payments/stable"])
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions: store, Evidence: &testReleaseEvidenceReader{evidence: evidence},
				UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Coordinate(context.Background(), command)
			if test.wantError {
				if err == nil {
					t.Fatal("release advanced with an exception that was not live at trusted release time")
				}
				if got := store.tracks["payments/stable"]; !reflect.DeepEqual(got, original) {
					t.Fatalf("expired exception changed last-known-good track: %#v", got)
				}
				if len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
					t.Fatal("expired exception produced release side effects")
				}
				return
			}
			if err != nil {
				t.Fatalf("release with live exception: %v", err)
			}
			if result.Track.CurrentRevisionID != "revision-next" {
				t.Fatalf("live exception did not advance release: %#v", result.Track)
			}
		})
	}

	t.Run("old review without applied exception remains valid", func(t *testing.T) {
		store, evidence, command := authorizedReleaseFixture(t, now.Add(-365*24*time.Hour))
		service, err := NewReleaseService(ReleaseDependencies{
			Revisions: store, Evidence: &testReleaseEvidenceReader{evidence: evidence},
			UnitOfWork: &testUnitOfWork{committed: store}, Clock: &testClock{now: now},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Coordinate(context.Background(), command)
		if err != nil {
			t.Fatalf("old review with no applied exception: %v", err)
		}
		if result.Track.CurrentRevisionID != "revision-next" {
			t.Fatalf("old review did not advance: %#v", result.Track)
		}
	})
}

func authorizedReleaseWithAppliedException(
	t *testing.T,
	expiresAt time.Time,
) (*testOperationalStore, domain.ReleaseEvidence, ReleaseCommand) {
	t.Helper()
	store := newTestOperationalStore()
	store.tracks["payments/stable"] = domain.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main",
		Mode: domain.ReleaseModeFollowing, Generation: 3, CurrentRevisionID: "revision-good",
	}
	review, baseline, candidate := releaseReviewWithFindingAtForTest(
		"review-next", "payments", "revision-good", "revision-next", expiresAt.Add(-time.Hour), true,
	)
	persistReleaseReviewSnapshots(store, review, baseline, candidate)
	for revisionID, revision := range store.revisions {
		revision.SourceID = "payments-git"
		revision.Ref = "refs/heads/main"
		store.revisions[revisionID] = revision
	}
	syncRecord := domain.SyncRecord{
		ID: "sync-next", ProjectID: "payments", SourceID: "payments-git",
		RevisionID: "revision-next", Ref: "refs/heads/main", Result: domain.SyncResultSuccess,
	}
	evidence := domain.ReleaseEvidence{
		Authorization: domain.ReleaseAuthorization{
			ContractID: "payments", TrackID: "stable", ReviewID: review.ID, SyncRecordID: syncRecord.ID,
			BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
			SourceID: "payments-git", BoundRef: "refs/heads/main", PublicPath: "/payments/stable",
			PolicyDigest: review.Report.PolicyDigest,
		},
		Review: review, SyncRecord: syncRecord,
	}
	command := ReleaseCommand{
		ContractID: "payments", TrackID: "stable", RevisionID: "revision-next", Accepted: true,
		Review: review, SyncRecord: syncRecord, PublicPath: "/caller-selected",
	}
	return store, evidence, command
}

func authorizedReleaseFixture(
	t *testing.T,
	evaluatedAt time.Time,
) (*testOperationalStore, domain.ReleaseEvidence, ReleaseCommand) {
	t.Helper()
	store := newTestOperationalStore()
	store.tracks["payments/stable"] = domain.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: "refs/heads/main",
		Mode: domain.ReleaseModeFollowing, Generation: 3, CurrentRevisionID: "revision-good",
	}
	review := releaseReviewAtForTest("review-next", "payments", "revision-good", "revision-next", evaluatedAt)
	persistReleaseReviewRevisions(store, review)
	for revisionID, revision := range store.revisions {
		revision.SourceID = "payments-git"
		revision.Ref = "refs/heads/main"
		store.revisions[revisionID] = revision
	}
	syncRecord := domain.SyncRecord{
		ID: "sync-next", ProjectID: "payments", SourceID: "payments-git",
		RevisionID: "revision-next", Ref: "refs/heads/main", Result: domain.SyncResultSuccess,
	}
	evidence := domain.ReleaseEvidence{
		Authorization: domain.ReleaseAuthorization{
			ContractID: "payments", TrackID: "stable",
			ReviewID: review.ID, SyncRecordID: syncRecord.ID,
			BaselineRevisionID: "revision-good", CandidateRevisionID: "revision-next",
			SourceID: "payments-git", BoundRef: "refs/heads/main",
			PublicPath: "/payments/stable", PolicyDigest: review.Report.PolicyDigest,
		},
		Review: review, SyncRecord: syncRecord,
	}
	store.reviews[review.ID] = review
	store.syncRecords[syncRecord.ID] = syncRecord
	command := ReleaseCommand{
		ContractID: "payments", TrackID: "stable", RevisionID: "revision-next", Accepted: true,
		Review:     domain.ContractReview{ID: review.ID},
		SyncRecord: domain.SyncRecord{ID: syncRecord.ID},
		PublicPath: "/caller-selected",
	}
	return store, evidence, command
}

func authorizeTestRelease(store *testOperationalStore, command ReleaseCommand) domain.ReleaseEvidence {
	track := store.tracks[command.ContractID+"/"+command.TrackID]
	if track.BoundRef == "" {
		track.BoundRef = "refs/heads/main"
		store.tracks[command.ContractID+"/"+command.TrackID] = track
	}
	sourceID := command.SyncRecord.SourceID
	if sourceID == "" {
		sourceID = "payments-git"
	}
	for revisionID, revision := range store.revisions {
		revision.SourceID = sourceID
		revision.Ref = track.BoundRef
		store.revisions[revisionID] = revision
	}
	syncRecord := command.SyncRecord
	syncRecord.ProjectID = command.ContractID
	syncRecord.SourceID = sourceID
	syncRecord.RevisionID = command.RevisionID
	syncRecord.Ref = track.BoundRef
	if syncRecord.Result == "" {
		syncRecord.Result = domain.SyncResultSuccess
	}
	publicPath := command.PublicPath
	if publicPath == "" {
		publicPath = "/" + command.ContractID + "/" + command.TrackID
	}
	authorization := domain.ReleaseAuthorization{
		ContractID: command.ContractID, TrackID: command.TrackID,
		ReviewID: command.Review.ID, SyncRecordID: syncRecord.ID,
		BaselineRevisionID:  command.Review.BaselineRevisionID,
		CandidateRevisionID: command.RevisionID,
		SourceID:            sourceID, BoundRef: track.BoundRef,
		PublicPath: publicPath, PolicyDigest: command.Review.Report.PolicyDigest,
	}
	store.authorizations[authorization.ReviewID] = authorization
	store.reviews[command.Review.ID] = command.Review
	store.syncRecords[syncRecord.ID] = syncRecord
	return domain.ReleaseEvidence{
		Authorization: authorization,
		Review:        command.Review,
		SyncRecord:    syncRecord,
	}
}

type testReleaseEvidenceReader struct {
	evidence domain.ReleaseEvidence
	ctx      context.Context
}

func (r *testReleaseEvidenceReader) ReleaseEvidence(ctx context.Context, _, _, _ string) (domain.ReleaseEvidence, error) {
	r.ctx = ctx
	return r.evidence, nil
}

type substitutingRevisionReader struct {
	delegate   port.RevisionReader
	revisionID string
	ref        string
}

func (r substitutingRevisionReader) ContractRevision(ctx context.Context, contractID, revisionID string) (domain.ContractRevision, error) {
	revision, err := r.delegate.ContractRevision(ctx, contractID, revisionID)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	if revisionID == r.revisionID {
		revision.Ref = r.ref
	}
	return revision, nil
}

func releaseReportForTest(contractID, baselineRevisionID, candidateRevisionID string) domain.ReviewReport {
	return releaseReportAtForTest(
		contractID,
		baselineRevisionID,
		candidateRevisionID,
		time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	)
}

func releaseReportAtForTest(contractID, baselineRevisionID, candidateRevisionID string, evaluatedAt time.Time) domain.ReviewReport {
	baseline := domain.NewContractSnapshot(contractID, baselineRevisionID, []byte("spec:"+baselineRevisionID), domain.SpecIndex{})
	candidate := domain.NewContractSnapshot(contractID, candidateRevisionID, []byte("spec:"+candidateRevisionID), domain.SpecIndex{})
	policy, err := domain.MergePolicy(domain.PolicyLayer{Name: "stable", Source: domain.PolicySourceRepository})
	if err != nil {
		panic(err)
	}
	report, err := domain.EvaluateReview(domain.ReviewRequest{
		ContractID: contractID, Target: baseline, Candidate: candidate, Release: &baseline,
		Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: "test",
	})
	if err != nil {
		panic(err)
	}
	report.Comparisons = []domain.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	return report
}

func releaseReviewForTest(id, contractID, baselineRevisionID, candidateRevisionID string) domain.ContractReview {
	return releaseReviewAtForTest(
		id,
		contractID,
		baselineRevisionID,
		candidateRevisionID,
		time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	)
}

func releaseReviewAtForTest(id, contractID, baselineRevisionID, candidateRevisionID string, evaluatedAt time.Time) domain.ContractReview {
	report := releaseReportAtForTest(contractID, baselineRevisionID, candidateRevisionID, evaluatedAt)
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

func releaseReviewWithFindingAtForTest(
	id, contractID, baselineRevisionID, candidateRevisionID string,
	evaluatedAt time.Time,
	exceptFinding bool,
) (domain.ContractReview, domain.ContractSnapshot, domain.ContractSnapshot) {
	baseline := domain.NewContractSnapshot(
		contractID,
		baselineRevisionID,
		[]byte("spec:"+baselineRevisionID),
		domain.SpecIndex{Operations: []domain.Operation{{Method: "GET", Path: "/payments"}}},
	)
	candidate := domain.NewContractSnapshot(
		contractID,
		candidateRevisionID,
		[]byte("spec:"+candidateRevisionID),
		domain.SpecIndex{},
	)
	layer := domain.PolicyLayer{Name: "stable", Source: domain.PolicySourceRepository}
	if exceptFinding {
		layer.Exceptions = []domain.PolicyException{{
			RuleID: domain.RuleOperationRemoved, Reason: "planned migration", Author: "api-team",
			ExpiresAt: evaluatedAt.Add(time.Hour), Source: domain.PolicySourceRepository,
		}}
	}
	policy, err := domain.MergePolicy(layer)
	if err != nil {
		panic(err)
	}
	report, err := domain.EvaluateReview(domain.ReviewRequest{
		ContractID: contractID, Target: baseline, Candidate: candidate, Release: &baseline,
		Policy: policy, EvaluatedAt: evaluatedAt, EngineVersion: "test",
	})
	if err != nil {
		panic(err)
	}
	report.Comparisons = []domain.ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	review := domain.ContractReview{
		ID:                      id,
		ContractID:              contractID,
		BaselineRevisionID:      baselineRevisionID,
		BaselineSpecDigest:      baseline.SpecDigest,
		BaselineContractDigest:  baseline.ContractDigest,
		CandidateRevisionID:     candidateRevisionID,
		CandidateSpecDigest:     candidate.SpecDigest,
		CandidateContractDigest: candidate.ContractDigest,
		Report:                  report,
	}
	return review, baseline, candidate
}

func cloneContractReviewForTest(t *testing.T, review domain.ContractReview) domain.ContractReview {
	t.Helper()
	encoded, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	var cloned domain.ContractReview
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func persistReleaseReviewSnapshots(
	store *testOperationalStore,
	review domain.ContractReview,
	baseline, candidate domain.ContractSnapshot,
) {
	store.revisions[review.BaselineRevisionID] = domain.ContractRevision{
		ID: review.BaselineRevisionID, ContractID: review.ContractID,
		SpecDigest: baseline.SpecDigest, ContractDigest: baseline.ContractDigest,
		ReviewSnapshot: &baseline,
	}
	store.revisions[review.CandidateRevisionID] = domain.ContractRevision{
		ID: review.CandidateRevisionID, ContractID: review.ContractID,
		SpecDigest: candidate.SpecDigest, ContractDigest: candidate.ContractDigest,
		ReviewSnapshot: &candidate,
	}
}

func persistReleaseReviewRevisions(store *testOperationalStore, review domain.ContractReview) {
	baseline := domain.NewContractSnapshot(
		review.ContractID,
		review.BaselineRevisionID,
		[]byte("spec:"+review.BaselineRevisionID),
		domain.SpecIndex{},
	)
	candidate := domain.NewContractSnapshot(
		review.ContractID,
		review.CandidateRevisionID,
		[]byte("spec:"+review.CandidateRevisionID),
		domain.SpecIndex{},
	)
	store.revisions[review.BaselineRevisionID] = domain.ContractRevision{
		ID: review.BaselineRevisionID, ContractID: review.ContractID,
		SpecDigest: review.BaselineSpecDigest, ContractDigest: review.BaselineContractDigest,
		ReviewSnapshot: &baseline,
	}
	store.revisions[review.CandidateRevisionID] = domain.ContractRevision{
		ID: review.CandidateRevisionID, ContractID: review.ContractID,
		SpecDigest: review.CandidateSpecDigest, ContractDigest: review.CandidateContractDigest,
		ReviewSnapshot: &candidate,
	}
}
