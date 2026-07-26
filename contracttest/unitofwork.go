package contracttest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

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
			current, err := store.ReleaseTrack(ctx, track.ContractID, track.ID)
			if err != nil {
				return err
			}
			next := current
			next.Generation++
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

	t.Run("revision evidence is transaction readable", func(t *testing.T) {
		uow := factory(t)
		ctx := markedContext(t)
		revision := domain.ContractRevision{
			ID: "review-revision", ContractID: "contract", SourceID: "source",
			SpecDigest:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ContractDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}
		if err := uow.Within(ctx, func(callbackContext context.Context, store port.OperationalStore) error {
			requireSameContext(t, ctx, callbackContext)
			return store.SaveRevision(callbackContext, revision)
		}); err != nil {
			t.Fatalf("save revision evidence: %v", err)
		}
		if err := uow.Within(ctx, func(callbackContext context.Context, store port.OperationalStore) error {
			requireSameContext(t, ctx, callbackContext)
			got, err := store.ContractRevision(callbackContext, revision.ContractID, revision.ID)
			if err != nil {
				return err
			}
			if got != revision {
				t.Fatalf("revision evidence = %#v, want %#v", got, revision)
			}
			return nil
		}); err != nil {
			t.Fatalf("load revision evidence: %v", err)
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
		ctx := markedContext(t)
		track := domain.ReleaseTrack{ID: "concurrent", ContractID: "contract", Mode: domain.ReleaseModeFollowing}
		if err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
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
}

func incrementTrackWithRetry(ctx context.Context, uow port.UnitOfWork, contractID, trackID string) error {
	for attempt := 0; attempt < 100; attempt++ {
		err := uow.Within(ctx, func(ctx context.Context, store port.OperationalStore) error {
			current, err := store.ReleaseTrack(ctx, contractID, trackID)
			if err != nil {
				return err
			}
			next := current
			next.Generation++
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
