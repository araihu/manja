package contracttest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type UnitOfWorkFactory func(testing.TB) port.UnitOfWork

func UnitOfWork(t *testing.T, factory UnitOfWorkFactory) {
	t.Helper()
	if factory == nil {
		t.Fatal("unit of work factory is required")
	}

	t.Run("preserves context", func(t *testing.T) {
		uow := factory(t)
		ctx := markedContext(t)
		if err := uow.Within(ctx, func(callbackContext context.Context, _ port.OperationalStore) error {
			requireSameContext(t, ctx, callbackContext)
			return nil
		}); err != nil {
			t.Fatalf("context transaction: %v", err)
		}
	})

	t.Run("rollback is atomic", func(t *testing.T) {
		uow := factory(t)
		ctx := markedContext(t)
		track := domain.ReleaseTrack{ID: "stable", ContractID: "contract", Mode: domain.ReleaseModeFollowing}
		if err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			return store.SaveReleaseTrack(ctx, 0, track)
		}); err != nil {
			t.Fatalf("seed rollback track: %v", err)
		}
		rollback := errors.New("rollback sentinel")
		err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveRevision(ctx, domain.ContractRevision{ID: "rollback-revision", ContractID: track.ContractID}); err != nil {
				return err
			}
			current, err := store.ReleaseTrack(ctx, track.ContractID, track.ID)
			if err != nil {
				return err
			}
			next, changed, err := domain.ConsiderReleaseDecision(current, domain.ReleaseDecision{
				RevisionID: "rollback-revision", ReviewID: "rollback-review",
				ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Verdict:      domain.VerdictFail,
				EvaluatedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			})
			if err != nil || !changed {
				return fmt.Errorf("derive rollback decision: changed=%t err=%w", changed, err)
			}
			if err := store.SaveReleaseTrack(ctx, current.Generation, next); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("rollback error = %v, want sentinel", err)
		}
		got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Generation != 0 {
			t.Errorf("rollback exposed generation %d, want 0", got.Generation)
		}
	})

	t.Run("rejects missing blob reference", func(t *testing.T) {
		uow := factory(t)
		ctx := markedContext(t)
		missing := port.ContentAddressedBlobKey([]byte("blob that was never written"))
		err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			return store.SaveRevision(ctx, domain.ContractRevision{ID: "missing-blob-revision", SourceID: "source", SpecBlobKey: string(missing)})
		})
		if err == nil {
			t.Error("missing blob metadata commit succeeded")
		}
	})

	t.Run("concurrent updates do not lose generations", func(t *testing.T) {
		uow := factory(t)
		if _, authenticated := uow.(port.ReleaseAuthorizationWriter); authenticated {
			return
		}
		ctx := markedContext(t)
		track := domain.ReleaseTrack{ID: "concurrent", ContractID: "contract", Mode: domain.ReleaseModeFollowing}
		if err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveRevision(ctx, domain.ContractRevision{ID: "concurrent-revision", ContractID: track.ContractID}); err != nil {
				return err
			}
			return store.SaveReleaseTrack(ctx, 0, track)
		}); err != nil {
			t.Fatalf("seed concurrent track: %v", err)
		}

		const updates = 16
		start := make(chan struct{})
		errorsByUpdate := make(chan error, updates)
		var wait sync.WaitGroup
		for index := 0; index < updates; index++ {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				errorsByUpdate <- incrementTrackWithRetry(ctx, uow, track.ContractID, track.ID)
			}()
		}
		close(start)
		wait.Wait()
		close(errorsByUpdate)
		for err := range errorsByUpdate {
			if err != nil {
				t.Errorf("concurrent update: %v", err)
			}
		}
		got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Generation != updates {
			t.Errorf("generation after concurrent updates = %d, want %d; adapter lost an update", got.Generation, updates)
		}
	})

	t.Run("release track reads do not alias transactional state", func(t *testing.T) {
		uow := factory(t)
		if _, authenticated := uow.(port.ReleaseAuthorizationWriter); authenticated {
			return
		}
		ctx := markedContext(t)
		track := releaseTrackIsolationFixture()
		if err := seedReleaseTrackIsolation(ctx, uow, track); err != nil {
			t.Fatalf("seed release track isolation: %v", err)
		}
		if err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			returned, err := store.ReleaseTrack(ctx, track.ContractID, track.ID)
			if err != nil {
				return err
			}
			returned.LastDecision.Accepted = true
			returned.LastDecision.Verdict = domain.VerdictPass
			return nil
		}); err != nil {
			t.Fatalf("mutate returned track without save: %v", err)
		}
		got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, track) {
			t.Fatalf("returned release track aliased persisted state: got=%#v want=%#v", got, track)
		}
	})

	t.Run("release track saves retain an isolated value", func(t *testing.T) {
		uow := factory(t)
		if _, authenticated := uow.(port.ReleaseAuthorizationWriter); authenticated {
			return
		}
		ctx := markedContext(t)
		track := releaseTrackIsolationFixture()
		if err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			if err := store.SaveRevision(ctx, domain.ContractRevision{ID: track.CandidateRevisionID, ContractID: track.ContractID}); err != nil {
				return err
			}
			if err := store.SaveReleaseTrack(ctx, 0, track); err != nil {
				return err
			}
			track.LastDecision.Accepted = true
			track.LastDecision.Verdict = domain.VerdictPass
			return nil
		}); err != nil {
			t.Fatalf("save isolated release track: %v", err)
		}
		got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := releaseTrackIsolationFixture()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("saved release track retained caller alias: got=%#v want=%#v", got, want)
		}
	})

	t.Run("release track decision evidence cannot be stripped", func(t *testing.T) {
		uow := factory(t)
		if _, authenticated := uow.(port.ReleaseAuthorizationWriter); authenticated {
			return
		}
		ctx := markedContext(t)
		track := releaseTrackIsolationFixture()
		track.LastDecision.Accepted = true
		track.LastDecision.Verdict = domain.VerdictPass
		var err error
		track, err = domain.PromoteReleaseRevision(track, track.LastDecision.RevisionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := seedReleaseTrackIsolation(ctx, uow, track); err != nil {
			t.Fatalf("seed decision evidence stripping track: %v", err)
		}

		err = uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			stripped, err := store.ReleaseTrack(ctx, track.ContractID, track.ID)
			if err != nil {
				return err
			}
			stripped.LastDecision = nil
			return store.SaveReleaseTrack(ctx, track.Generation, stripped)
		})
		if err == nil {
			t.Fatal("stripped release decision evidence was committed as legacy state")
		}
		got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, track) {
			t.Fatalf("failed stripping attempt changed persisted track: got=%#v want=%#v", got, track)
		}
	})

	t.Run("release track state cannot bypass its decision", func(t *testing.T) {
		uow := factory(t)
		if _, authenticated := uow.(port.ReleaseAuthorizationWriter); authenticated {
			return
		}
		ctx := markedContext(t)
		track := releaseTrackIsolationFixture()
		if err := seedReleaseTrackIsolation(ctx, uow, track); err != nil {
			t.Fatalf("seed release transition: %v", err)
		}

		err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			forged, err := store.ReleaseTrack(ctx, track.ContractID, track.ID)
			if err != nil {
				return err
			}
			forged.CurrentRevisionID = forged.CandidateRevisionID
			forged.Generation = 0
			return store.SaveReleaseTrack(ctx, track.Generation, forged)
		})
		if err == nil {
			t.Fatal("rejected candidate bypassed its decision and generation")
		}
		got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, track) {
			t.Fatalf("failed transition changed persisted track: got=%#v want=%#v", got, track)
		}
	})

	t.Run("cross-contract operational references fail atomically", func(t *testing.T) {
		cases := map[string]func(context.Context, port.OperationalStore, domain.ReleaseTrack) error{
			"track current": func(ctx context.Context, store port.OperationalStore, _ domain.ReleaseTrack) error {
				return store.SaveReleaseTrack(ctx, 0, domain.ReleaseTrack{
					ID: "substituted-current", ContractID: "payments", Mode: domain.ReleaseModeFollowing,
					CurrentRevisionID: "orders-revision",
				})
			},
			"track candidate": func(ctx context.Context, store port.OperationalStore, track domain.ReleaseTrack) error {
				next, changed, err := domain.ConsiderReleaseDecision(track, domain.ReleaseDecision{
					RevisionID: "orders-revision", ReviewID: "cross-contract-review",
					ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Verdict:      domain.VerdictFail, EvaluatedAt: time.Date(2026, 7, 25, 14, 0, 0, 0, time.UTC),
				})
				if err != nil || !changed {
					return fmt.Errorf("derive cross-contract candidate: changed=%t err=%w", changed, err)
				}
				return store.SaveReleaseTrack(ctx, track.Generation, next)
			},
			"publication": func(ctx context.Context, store port.OperationalStore, _ domain.ReleaseTrack) error {
				return store.SavePublication(ctx, domain.Publication{
					ProjectID: "payments", RevisionID: "orders-revision", Path: "/payments",
				})
			},
			"review baseline": func(ctx context.Context, store port.OperationalStore, _ domain.ReleaseTrack) error {
				return store.SaveReview(ctx, domain.ContractReview{
					ID: "cross-baseline", ContractID: "payments",
					BaselineRevisionID: "orders-revision", CandidateRevisionID: "payments-next",
				})
			},
			"review candidate": func(ctx context.Context, store port.OperationalStore, _ domain.ReleaseTrack) error {
				return store.SaveReview(ctx, domain.ContractReview{
					ID: "cross-candidate", ContractID: "payments",
					BaselineRevisionID: "payments-good", CandidateRevisionID: "orders-revision",
				})
			},
		}
		for name, attempt := range cases {
			t.Run(name, func(t *testing.T) {
				uow := factory(t)
				ctx := markedContext(t)
				track := domain.ReleaseTrack{
					ID: "stable-owner", ContractID: "payments", Mode: domain.ReleaseModeFollowing,
					CurrentRevisionID: "payments-good",
				}
				if err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
					for _, revision := range []domain.ContractRevision{
						{ID: "payments-good", ContractID: "payments"},
						{ID: "payments-next", ContractID: "payments"},
						{ID: "orders-revision", ContractID: "orders"},
					} {
						if err := store.SaveRevision(ctx, revision); err != nil {
							return err
						}
					}
					return store.SaveReleaseTrack(ctx, 0, track)
				}); err != nil {
					t.Fatalf("seed ownership fixture: %v", err)
				}

				err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
					return attempt(ctx, store, track)
				})
				if err == nil {
					t.Fatal("cross-contract reference committed")
				}
				got, err := loadTrack(ctx, uow, track.ContractID, track.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, track) {
					t.Fatalf("cross-contract rollback changed last-known-good track: got=%#v want=%#v", got, track)
				}
			})
		}
	})
}

func releaseTrackIsolationFixture() domain.ReleaseTrack {
	decision := domain.ReleaseDecision{
		RevisionID: "isolation-revision", ReviewID: "isolation-review",
		ReviewDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Verdict:      domain.VerdictFail,
		EvaluatedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	return domain.ReleaseTrack{
		ID: "isolation", ContractID: "contract", Mode: domain.ReleaseModePinned,
		Generation: 1, CandidateRevisionID: decision.RevisionID, LastDecision: &decision,
	}
}

func seedReleaseTrackIsolation(ctx context.Context, uow port.UnitOfWork, track domain.ReleaseTrack) error {
	return uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
		revisionID := track.CandidateRevisionID
		if revisionID == "" && track.LastDecision != nil {
			revisionID = track.LastDecision.RevisionID
		}
		if err := store.SaveRevision(ctx, domain.ContractRevision{ID: revisionID, ContractID: track.ContractID}); err != nil {
			return err
		}
		if track.Generation <= 1 {
			return store.SaveReleaseTrack(ctx, 0, track)
		}
		if track.Generation != 2 || track.LastDecision == nil || !track.LastDecision.Accepted || track.CandidateRevisionID != "" {
			return fmt.Errorf("unsupported release isolation seed: %#v", track)
		}
		baseline := domain.CloneReleaseTrack(track)
		baseline.Generation = 0
		baseline.CurrentRevisionID = ""
		baseline.LastDecision = nil
		if err := store.SaveReleaseTrack(ctx, 0, baseline); err != nil {
			return err
		}
		candidate, changed, err := domain.ConsiderReleaseDecision(baseline, *track.LastDecision)
		if err != nil || !changed {
			return fmt.Errorf("derive isolation candidate: changed=%t err=%w", changed, err)
		}
		if err := store.SaveReleaseTrack(ctx, baseline.Generation, candidate); err != nil {
			return err
		}
		return store.SaveReleaseTrack(ctx, candidate.Generation, track)
	})
}

func incrementTrackWithRetry(ctx context.Context, uow port.UnitOfWork, contractID, trackID string) error {
	for attempt := 0; attempt < 100; attempt++ {
		err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			current, err := store.ReleaseTrack(ctx, contractID, trackID)
			if err != nil {
				return err
			}
			sequence := current.Generation + 1
			next, changed, err := domain.ConsiderReleaseDecision(current, domain.ReleaseDecision{
				RevisionID: "concurrent-revision", ReviewID: fmt.Sprintf("concurrent-review-%04d", sequence),
				ReviewDigest: fmt.Sprintf("%064x", sequence), Verdict: domain.VerdictFail,
				EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC).Add(time.Duration(sequence) * time.Second),
			})
			if err != nil || !changed {
				return fmt.Errorf("derive concurrent decision: changed=%t err=%w", changed, err)
			}
			return store.SaveReleaseTrack(ctx, current.Generation, next)
		})
		if errors.Is(err, port.ErrGenerationConflict) {
			continue
		}
		return err
	}
	return fmt.Errorf("generation conflict retry budget exhausted")
}

func loadTrack(ctx context.Context, uow port.UnitOfWork, contractID, trackID string) (domain.ReleaseTrack, error) {
	var track domain.ReleaseTrack
	err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
		var err error
		track, err = store.ReleaseTrack(ctx, contractID, trackID)
		return err
	})
	return track, err
}
