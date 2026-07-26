package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type releaseTrackOverrideStore struct {
	*testOperationalStore
	track domain.ReleaseTrack
}

func (s *releaseTrackOverrideStore) ReleaseTrack(
	ctx context.Context,
	contractID, trackID string,
) (domain.ReleaseTrack, error) {
	if err := s.record(ctx, "track-read"); err != nil {
		return domain.ReleaseTrack{}, err
	}
	return domain.CloneReleaseTrack(s.track), nil
}

type releaseTrackOverrideUnitOfWork struct {
	store *releaseTrackOverrideStore
}

func (u *releaseTrackOverrideUnitOfWork) Within(
	ctx context.Context,
	callback func(context.Context, port.OperationalStore) error,
) error {
	return callback(ctx, u.store)
}

func TestReleaseServiceRejectsZeroGenerationExactReplayFromExternalStore(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store, evidence, command := authorizedReleaseFixture(t, evaluatedAt)
	decision, err := releaseDecision(evidence.Review, true)
	if err != nil {
		t.Fatal(err)
	}
	malformed := domain.ReleaseTrack{
		ID: "stable", ContractID: "payments", BoundRef: evidence.Authorization.BoundRef,
		Mode: domain.ReleaseModeFollowing, CurrentRevisionID: decision.RevisionID,
		LastDecision: &decision,
	}
	override := &releaseTrackOverrideStore{testOperationalStore: store, track: malformed}
	clock := &testClock{now: evaluatedAt.Add(time.Minute)}
	service, err := NewReleaseService(ReleaseDependencies{
		Revisions:  store,
		Evidence:   &testReleaseEvidenceReader{evidence: evidence},
		UnitOfWork: &releaseTrackOverrideUnitOfWork{store: override},
		Clock:      clock,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Coordinate(context.Background(), command); err == nil {
		t.Fatal("release service accepted zero-generation exact replay")
	}
	if len(clock.contexts) != 0 {
		t.Fatalf("invalid exact replay reached trusted clock %d times", len(clock.contexts))
	}
	for _, call := range store.calls {
		if strings.HasSuffix(call, "write") || call == "publication" || call == "audit" || call == "outbox" {
			t.Fatalf("invalid exact replay caused side effect %q", call)
		}
	}
}

func TestReleaseServiceRejectsExactReplayWithOutstandingCandidateFromAnotherBaseline(t *testing.T) {
	evaluatedAt := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name     string
		accepted bool
	}{
		{name: "rejected", accepted: false},
		{name: "accepted pending promotion", accepted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, evidence, command := authorizedReleaseFixture(t, evaluatedAt)
			command.Accepted = test.accepted
			decision, err := releaseDecision(evidence.Review, test.accepted)
			if err != nil {
				t.Fatal(err)
			}
			malformed := domain.ReleaseTrack{
				ID: "stable", ContractID: "payments", BoundRef: evidence.Authorization.BoundRef,
				Mode: domain.ReleaseModePinned, Generation: 4,
				CurrentRevisionID: "revision-unrelated", CandidateRevisionID: decision.RevisionID,
				LastDecision: &decision,
			}
			if err := domain.ValidateReleaseTrack(malformed); err != nil {
				t.Fatalf("external-store fixture must remain domain-valid: %v", err)
			}
			override := &releaseTrackOverrideStore{testOperationalStore: store, track: malformed}
			clock := &testClock{now: evaluatedAt.Add(time.Minute)}
			service, err := NewReleaseService(ReleaseDependencies{
				Revisions:  store,
				Evidence:   &testReleaseEvidenceReader{evidence: evidence},
				UnitOfWork: &releaseTrackOverrideUnitOfWork{store: override},
				Clock:      clock,
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = service.Coordinate(context.Background(), command)
			const want = "coordinate release: release track baseline \"revision-unrelated\" does not match authorized review baseline \"revision-good\""
			if err == nil || err.Error() != want {
				t.Fatalf("Coordinate error = %v, want %q", err, want)
			}
			if len(clock.contexts) != 0 {
				t.Fatalf("invalid exact replay reached trusted clock %d times", len(clock.contexts))
			}
			for _, call := range store.calls {
				if strings.HasSuffix(call, "write") || call == "publication" || call == "audit" || call == "outbox" {
					t.Fatalf("invalid exact replay caused side effect %q", call)
				}
			}
			if len(store.publications) != 0 || len(store.auditEvents) != 0 || len(store.outbox) != 0 {
				t.Fatalf(
					"invalid exact replay persisted side effects: publications=%d audit=%d outbox=%d",
					len(store.publications), len(store.auditEvents), len(store.outbox),
				)
			}
		})
	}
}
