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

type SyncRecordReaderFixture struct {
	Reader   port.SyncRecordReader
	RecordID string
	Want     domain.SyncRecord
	Observed func() context.Context
}

type SyncRecordReaderFactory func(testing.TB) SyncRecordReaderFixture

// SyncRecordReader verifies the separate canonical sync-evidence read boundary
// without adding methods to OperationalStore.
func SyncRecordReader(t *testing.T, factory SyncRecordReaderFactory) {
	t.Helper()
	if factory == nil {
		t.Fatal("sync record reader factory is required")
	}

	t.Run("preserves context and canonical evidence", func(t *testing.T) {
		fixture := factory(t)
		ctx := markedContext(t)
		got, err := fixture.Reader.SyncRecord(ctx, fixture.RecordID)
		if err != nil {
			t.Fatalf("read sync record: %v", err)
		}
		if !reflect.DeepEqual(got, fixture.Want) {
			t.Fatalf("sync record = %#v, want %#v", got, fixture.Want)
		}
		requireObservedSyncContext(t, fixture, ctx)
	})

	t.Run("propagates cancellation", func(t *testing.T) {
		fixture := factory(t)
		ctx, cancel := context.WithCancel(markedContext(t))
		cancel()
		if _, err := fixture.Reader.SyncRecord(ctx, fixture.RecordID); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled sync record read error = %v, want context canceled", err)
		}
		requireObservedSyncContext(t, fixture, ctx)
	})

	t.Run("propagates deadline", func(t *testing.T) {
		fixture := factory(t)
		deadline := time.Now().Add(-time.Second)
		ctx, cancel := context.WithDeadline(markedContext(t), deadline)
		defer cancel()
		if _, err := fixture.Reader.SyncRecord(ctx, fixture.RecordID); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expired sync record read error = %v, want deadline exceeded", err)
		}
		requireObservedSyncContext(t, fixture, ctx)
		gotDeadline, ok := fixture.Observed().Deadline()
		if !ok || !gotDeadline.Equal(deadline) {
			t.Fatalf("observed deadline = %v, %t; want %v", gotDeadline, ok, deadline)
		}
	})

	t.Run("rejects unknown identity", func(t *testing.T) {
		fixture := factory(t)
		if _, err := fixture.Reader.SyncRecord(context.Background(), fixture.RecordID+"-other"); err == nil {
			t.Fatal("sync record reader returned evidence for an unknown identity")
		}
	})
}

func requireObservedSyncContext(t *testing.T, fixture SyncRecordReaderFixture, want context.Context) {
	t.Helper()
	if fixture.Observed == nil {
		t.Fatal("sync record reader fixture must expose its observed context")
	}
	requireSameContext(t, want, fixture.Observed())
}
