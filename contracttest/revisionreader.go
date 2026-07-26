package contracttest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type RevisionReaderFixture struct {
	Reader     port.RevisionReader
	ContractID string
	RevisionID string
	Want       domain.ContractRevision
	Observed   func() context.Context
}

type RevisionReaderFactory func(testing.TB) RevisionReaderFixture

func RevisionReader(t *testing.T, factory RevisionReaderFactory) {
	t.Helper()
	if factory == nil {
		t.Fatal("revision reader factory is required")
	}

	t.Run("preserves context and contract identity", func(t *testing.T) {
		fixture := factory(t)
		ctx := markedContext(t)
		got, err := fixture.Reader.ContractRevision(ctx, fixture.ContractID, fixture.RevisionID)
		if err != nil {
			t.Fatalf("read revision: %v", err)
		}
		if !reflect.DeepEqual(got, fixture.Want) {
			t.Fatalf("revision = %#v, want %#v", got, fixture.Want)
		}
		if fixture.Observed == nil {
			t.Fatal("revision reader fixture must expose its observed context")
		}
		requireSameContext(t, ctx, fixture.Observed())
	})

	t.Run("propagates cancellation", func(t *testing.T) {
		fixture := factory(t)
		ctx, cancel := context.WithCancel(markedContext(t))
		cancel()
		if _, err := fixture.Reader.ContractRevision(ctx, fixture.ContractID, fixture.RevisionID); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled revision read error = %v, want context canceled", err)
		}
		if fixture.Observed == nil {
			t.Fatal("revision reader fixture must expose its observed context")
		}
		requireSameContext(t, ctx, fixture.Observed())
	})

	t.Run("propagates deadline", func(t *testing.T) {
		fixture := factory(t)
		deadline := time.Now().Add(-time.Second)
		ctx, cancel := context.WithDeadline(markedContext(t), deadline)
		defer cancel()
		if _, err := fixture.Reader.ContractRevision(ctx, fixture.ContractID, fixture.RevisionID); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expired revision read error = %v, want deadline exceeded", err)
		}
		if fixture.Observed == nil {
			t.Fatal("revision reader fixture must expose its observed context")
		}
		observed := fixture.Observed()
		requireSameContext(t, ctx, observed)
		gotDeadline, ok := observed.Deadline()
		if !ok || !gotDeadline.Equal(deadline) {
			t.Fatalf("observed deadline = %v, %t; want %v", gotDeadline, ok, deadline)
		}
	})

	t.Run("rejects the wrong contract", func(t *testing.T) {
		fixture := factory(t)
		if _, err := fixture.Reader.ContractRevision(context.Background(), fixture.ContractID+"-other", fixture.RevisionID); err == nil {
			t.Fatal("revision reader returned evidence for the wrong contract")
		}
	})
}
