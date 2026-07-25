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

type fakePublicationReader struct {
	byPath map[string]domain.Publication
}

func (r fakePublicationReader) PublicPublicationByPath(_ context.Context, publicPath string) (domain.Publication, error) {
	publication, ok := r.byPath[publicPath]
	if !ok {
		return domain.Publication{}, fs.ErrNotExist
	}
	return publication, nil
}
