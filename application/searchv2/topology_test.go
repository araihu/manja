package searchv2

import "testing"

func TestResolveTopologyRetainsEmptyDeclaredCatalogAndAssignsOnlyUnreferencedToDefault(t *testing.T) {
	topology, err := ResolveTopology(
		[]DeclaredCatalog{
			{Key: "z-empty", Title: "Empty"},
			{Key: "apis", Title: "APIs", SpecKeys: []string{"pets"}},
		},
		[]Spec{{Key: "billing", Title: "Billing"}, {Key: "pets", Title: "Pets"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Catalogs) != 3 {
		t.Fatalf("catalog count = %d, want 3", len(topology.Catalogs))
	}
	if topology.Catalogs[0].Key != "apis" || topology.Catalogs[1].Key != "default" || topology.Catalogs[2].Key != "z-empty" {
		t.Fatalf("catalog order = %#v", topology.Catalogs)
	}
	if len(topology.Catalogs[2].Specs) != 0 || topology.Catalogs[2].Implicit {
		t.Fatalf("empty declared catalog changed: %#v", topology.Catalogs[2])
	}
	defaultCatalog := topology.Catalogs[1]
	if !defaultCatalog.Implicit || len(defaultCatalog.Specs) != 1 || defaultCatalog.Specs[0].Key != "billing" {
		t.Fatalf("default assignment = %#v", defaultCatalog)
	}
}

func TestResolveTopologyOmitsEmptyDefault(t *testing.T) {
	topology, err := ResolveTopology(
		[]DeclaredCatalog{{Key: "apis", Title: "APIs", SpecKeys: []string{"pets", "billing"}}},
		[]Spec{{Key: "pets", Title: "Pets"}, {Key: "billing", Title: "Billing"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Catalogs) != 1 || topology.Catalogs[0].Key != "apis" {
		t.Fatalf("topology = %#v", topology)
	}
}

func TestResolveTopologyIsDeterministicAndAReferenceInAnyCatalogExcludesDefault(t *testing.T) {
	first, err := ResolveTopology(
		[]DeclaredCatalog{
			{Key: "second", Title: "Second", SpecKeys: []string{"shared"}},
			{Key: "first", Title: "First", SpecKeys: []string{"zeta", "shared"}},
		},
		[]Spec{{Key: "zeta", Title: "Zeta"}, {Key: "shared", Title: "Shared"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveTopology(
		[]DeclaredCatalog{
			{Key: "first", Title: "First", SpecKeys: []string{"shared", "zeta"}},
			{Key: "second", Title: "Second", SpecKeys: []string{"shared"}},
		},
		[]Spec{{Key: "shared", Title: "Shared"}, {Key: "zeta", Title: "Zeta"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Catalogs) != 2 || len(second.Catalogs) != 2 {
		t.Fatalf("unexpected implicit default: %#v / %#v", first, second)
	}
	if len(first.Catalogs[0].Specs) != 2 || len(first.Catalogs[1].Specs) != 1 || first.Catalogs[1].Specs[0].Key != "shared" {
		t.Fatalf("multi-catalog membership was not retained: %#v", first.Catalogs)
	}
	for index := range first.Catalogs {
		if first.Catalogs[index].Key != second.Catalogs[index].Key {
			t.Fatalf("catalog order differs: %#v / %#v", first, second)
		}
		if len(first.Catalogs[index].Specs) != len(second.Catalogs[index].Specs) {
			t.Fatalf("member count differs: %#v / %#v", first, second)
		}
		for member := range first.Catalogs[index].Specs {
			if first.Catalogs[index].Specs[member] != second.Catalogs[index].Specs[member] {
				t.Fatalf("member order differs: %#v / %#v", first, second)
			}
		}
	}
}

func TestResolveTopologyRejectsUnknownAndReservedCatalogAssignments(t *testing.T) {
	tests := []struct {
		name     string
		catalogs []DeclaredCatalog
	}{
		{name: "unknown spec", catalogs: []DeclaredCatalog{{Key: "apis", Title: "APIs", SpecKeys: []string{"missing"}}}},
		{name: "reserved default", catalogs: []DeclaredCatalog{{Key: "default", Title: "Mine"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolveTopology(test.catalogs, []Spec{{Key: "pets", Title: "Pets"}}); err == nil {
				t.Fatal("expected invalid topology to fail")
			}
		})
	}
}
