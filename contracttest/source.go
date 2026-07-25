package contracttest

import (
	"context"
	"reflect"
	"testing"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type SourceFixture struct {
	Fetcher        port.SourceFetcher
	WantSpec       domain.SpecFile
	WantRevision   domain.ContractRevision
	WantCandidates []domain.RevisionCandidate
}

type SourceFactory func(testing.TB) SourceFixture

func SourceFetcher(t *testing.T, factory SourceFactory) {
	t.Helper()
	if factory == nil {
		t.Fatal("source factory is required")
	}

	t.Run("fetch is deterministic", func(t *testing.T) {
		fixture := factory(t)
		if fixture.Fetcher == nil {
			t.Fatal("source factory returned nil fetcher")
		}
		ctx := markedContext(t)
		firstSpec, firstRevision, err := fixture.Fetcher.Fetch(ctx)
		if err != nil {
			t.Fatal(err)
		}
		secondSpec, secondRevision, err := fixture.Fetcher.Fetch(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(firstSpec, secondSpec) || !reflect.DeepEqual(firstRevision, secondRevision) {
			t.Errorf("source fetch is not deterministic")
		}
		if !reflect.DeepEqual(firstSpec, fixture.WantSpec) {
			t.Errorf("spec = %#v, want %#v", firstSpec, fixture.WantSpec)
		}
		if !reflect.DeepEqual(firstRevision, fixture.WantRevision) {
			t.Errorf("revision = %#v, want %#v", firstRevision, fixture.WantRevision)
		}
	})

	t.Run("discovery is deterministic", func(t *testing.T) {
		fixture := factory(t)
		discoverer, ok := fixture.Fetcher.(port.SourceDiscoverer)
		if !ok {
			if fixture.WantCandidates != nil {
				t.Error("source fixture supplied candidates but fetcher does not implement discovery")
			}
			return
		}
		ctx := markedContext(t)
		first, err := discoverer.Discover(ctx)
		if err != nil {
			t.Fatal(err)
		}
		second, err := discoverer.Discover(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Errorf("source discovery is not deterministic: first %#v, second %#v", first, second)
		}
		if !reflect.DeepEqual(first, fixture.WantCandidates) {
			t.Errorf("discovery = %#v, want %#v", first, fixture.WantCandidates)
		}
	})

	t.Run("honors cancellation", func(t *testing.T) {
		fixture := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := fixture.Fetcher.Fetch(ctx); err == nil {
			t.Error("Fetch ignored cancelled context")
		}
		if discoverer, ok := fixture.Fetcher.(port.SourceDiscoverer); ok {
			if _, err := discoverer.Discover(ctx); err == nil {
				t.Error("Discover ignored cancelled context")
			}
		}
	})
}
