package web

import (
	"context"
	"testing"

	core "github.com/araihu/manja/domain"
)

func TestManagementBaselineRequiresCompleteContractScopedIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ManagedSpec)
	}{
		{
			name: "current revision belongs to another contract",
			mutate: func(spec *ManagedSpec) {
				spec.Revision.ContractID = "orders"
				spec.Index.ProjectID = "orders"
			},
		},
		{
			name: "current index belongs to another project",
			mutate: func(spec *ManagedSpec) {
				spec.Index.ProjectID = "orders"
			},
		},
		{
			name: "cached published index belongs to another project",
			mutate: func(spec *ManagedSpec) {
				spec.Index.ProjectID = "orders"
				spec.PublishedIndex = core.SpecIndex{
					ProjectID: "orders", RevisionID: "shared",
					Operations: []core.Operation{{Method: "GET", Path: "/orders"}},
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := &managementServer{
				publishedIndexLoader: func(context.Context, ManagedSpec) (core.SpecIndex, bool, error) {
					calls++
					return core.SpecIndex{
						ProjectID: "payments", RevisionID: "shared",
						Operations: []core.Operation{{Method: "GET", Path: "/payments"}},
					}, true, nil
				},
			}
			spec := ManagedSpec{
				ID:      "payments",
				Project: core.Project{ID: "payments"},
				Revision: core.ContractRevision{
					ID: "shared", ContractID: "payments",
				},
				Index: core.SpecIndex{
					ProjectID: "payments", RevisionID: "shared",
					Operations: []core.Operation{{Method: "GET", Path: "/payments-candidate"}},
				},
				Publication: core.Publication{
					ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments",
				},
			}
			test.mutate(&spec)

			index, ok, err := server.managementBaselineIndex(context.Background(), spec)
			if err != nil {
				t.Fatal(err)
			}
			if !ok || calls != 1 {
				t.Fatalf("contract-scoped loader result ok=%t calls=%d index=%#v", ok, calls, index)
			}
			if index.ProjectID != "payments" || index.Operations[0].Path != "/payments" {
				t.Fatalf("baseline bypassed the contract-scoped loader: %#v", index)
			}
		})
	}
}
