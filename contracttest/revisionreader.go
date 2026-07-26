package contracttest

import (
	"context"
	"reflect"
	"testing"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type RevisionReaderFixture struct {
	Reader     port.RevisionReader
	ContractID string
	RevisionID string
	Want       domain.ContractRevision
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
	})

	t.Run("rejects the wrong contract", func(t *testing.T) {
		fixture := factory(t)
		if _, err := fixture.Reader.ContractRevision(context.Background(), fixture.ContractID+"-other", fixture.RevisionID); err == nil {
			t.Fatal("revision reader returned evidence for the wrong contract")
		}
	})
}
