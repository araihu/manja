package server

import (
	"context"
	"testing"

	core "github.com/araihu/manja/domain"
	storeadapter "github.com/araihu/manja/internal/adapters/store"
	"github.com/araihu/manja/internal/web"
)

func TestDemoPublishedIndexLoaderDoesNotBypassContractScopedReadForMismatchedIndex(t *testing.T) {
	ctx := context.Background()
	store := storeadapter.NewFileStore(t.TempDir())
	raw := []byte("openapi: 3.1.0\ninfo:\n  title: Payments Published\n  version: v1\npaths: {}\n")
	key, err := store.Put(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRevision(ctx, core.ContractRevision{
		ID: "shared", ContractID: "payments", SourceID: "payments-git",
		Ref: "refs/heads/main", SpecBlobKey: string(key),
	}); err != nil {
		t.Fatal(err)
	}
	handler := &demoWorkbenchHandler{
		store: store,
		configBySpec: map[string]demoSpecConfig{
			"payments": {
				Seed:   demoSpecSeed{ProjectID: "payments"},
				Source: core.Source{ID: "payments-git", ProjectID: "payments", SpecPath: demoSpecPath},
			},
		},
	}
	index, ok, err := handler.publishedIndexLoader(ctx, web.ManagedSpec{
		ID:       "payments",
		Project:  core.Project{ID: "payments"},
		Revision: core.ContractRevision{ID: "shared", ContractID: "payments"},
		Index: core.SpecIndex{
			ProjectID: "orders", RevisionID: "shared", Title: "Orders Leaked Index",
		},
		Publication: core.Publication{
			ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || index.Title != "Payments Published" {
		t.Fatalf("mismatched index bypassed contract-scoped read: (%#v, %v)", index, ok)
	}
}
