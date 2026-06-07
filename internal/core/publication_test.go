package core

import (
	"context"
	"errors"
	"testing"
)

func TestPublicResolverReturnsOnlyPublicPublicationByPath(t *testing.T) {
	ctx := context.Background()
	store := fakePublicationStore{
		byPath: map[string]Publication{
			"/acme/payments/v1": {ProjectID: "p1", RevisionID: "r1", Public: true, Path: "/acme/payments/v1"},
			"/acme/private/v1":  {ProjectID: "p1", RevisionID: "r2", Public: false, Path: "/acme/private/v1"},
		},
	}
	resolver := PublicResolver{Store: store}

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
}

type fakePublicationStore struct {
	byPath map[string]Publication
}

func (s fakePublicationStore) SaveProject(context.Context, Project) error {
	return nil
}

func (s fakePublicationStore) Project(context.Context, string) (Project, error) {
	return Project{}, nil
}

func (s fakePublicationStore) SaveRevision(context.Context, Revision) error {
	return nil
}

func (s fakePublicationStore) Revision(context.Context, string) (Revision, error) {
	return Revision{}, nil
}

func (s fakePublicationStore) SavePublication(context.Context, Publication) error {
	return nil
}

func (s fakePublicationStore) Publication(context.Context, string, string) (Publication, error) {
	return Publication{}, errors.ErrUnsupported
}

func (s fakePublicationStore) PublicPublicationByPath(_ context.Context, publicPath string) (Publication, error) {
	p, ok := s.byPath[publicPath]
	if !ok {
		return Publication{}, errors.ErrUnsupported
	}
	return p, nil
}

func (s fakePublicationStore) SaveSyncRecord(context.Context, SyncRecord) error {
	return nil
}
