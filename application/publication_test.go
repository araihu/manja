package application

import (
	"context"
	"io/fs"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestPublicResolverReturnsOnlyPublicPublicationByPath(t *testing.T) {
	ctx := context.Background()
	reader := fakePublicationReader{byPath: map[string]domain.Publication{
		"/acme/payments/v1": {ProjectID: "p1", RevisionID: "r1", Public: true, Path: "/acme/payments/v1"},
		"/acme/private/v1":  {ProjectID: "p1", RevisionID: "r2", Public: false, Path: "/acme/private/v1"},
	}}
	resolver, err := NewPublicResolver(reader)
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolver.PublicationByPath(ctx, "/acme/payments/v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != "p1" || got.RevisionID != "r1" || !got.Public {
		t.Fatalf("publication = %#v", got)
	}
	if _, err := resolver.PublicationByPath(ctx, "/acme/private/v1"); err == nil {
		t.Fatal("private publication was visible to anonymous resolver")
	}
	if _, err := resolver.PublicationByPath(ctx, "/missing"); err == nil {
		t.Fatal("missing publication resolved")
	}
}

func TestPublicResolverRejectsNonCanonicalPathBeforeReader(t *testing.T) {
	for _, publicPath := range []string{" /payments ", "/payments\x00shadow", "/payments-\xff"} {
		t.Run(publicPath, func(t *testing.T) {
			calls := 0
			resolver, err := NewPublicResolver(fakePublicationReader{calls: &calls})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := resolver.PublicationByPath(context.Background(), publicPath); err == nil {
				t.Fatal("PublicationByPath accepted non-canonical path")
			}
			if calls != 0 {
				t.Fatal("non-canonical public path reached publication reader")
			}
		})
	}
}

func TestPublicResolverRejectsMalformedReaderOutput(t *testing.T) {
	for _, test := range []struct {
		name        string
		publication domain.Publication
	}{
		{name: "mismatched path", publication: domain.Publication{ProjectID: "payments", RevisionID: "revision-1", Public: true, Path: "/other"}},
		{name: "project control", publication: domain.Publication{ProjectID: "payments\x00shadow", RevisionID: "revision-1", Public: true, Path: "/payments"}},
		{name: "revision invalid utf8", publication: domain.Publication{ProjectID: "payments", RevisionID: "revision-\xff", Public: true, Path: "/payments"}},
		{name: "hostname padding", publication: domain.Publication{ProjectID: "payments", RevisionID: "revision-1", Public: true, Path: "/payments", Hostname: " docs.example.com "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			resolver, err := NewPublicResolver(fakePublicationReader{
				byPath: map[string]domain.Publication{"/payments": test.publication}, calls: &calls,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := resolver.PublicationByPath(context.Background(), "/payments"); err == nil {
				t.Fatal("PublicationByPath accepted malformed reader output")
			}
			if calls != 1 {
				t.Fatalf("publication reader calls = %d, want 1", calls)
			}
		})
	}
}

type fakePublicationReader struct {
	byPath map[string]domain.Publication
	calls  *int
}

func (r fakePublicationReader) PublicPublicationByPath(_ context.Context, publicPath string) (domain.Publication, error) {
	if r.calls != nil {
		*r.calls++
	}
	publication, ok := r.byPath[publicPath]
	if !ok {
		return domain.Publication{}, fs.ErrNotExist
	}
	return publication, nil
}
