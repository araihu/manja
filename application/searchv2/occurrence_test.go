package searchv2

import (
	"strings"
	"testing"
)

func TestSemanticRunExpandsIntoEveryCatalogMembership(t *testing.T) {
	topology, err := ResolveTopology(
		[]DeclaredCatalog{
			{Key: "internal", Title: "Internal", SpecKeys: []string{"pets"}},
			{Key: "public", Title: "Public", SpecKeys: []string{"pets"}},
		},
		[]Spec{{Key: "pets", Title: "Pets"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := NewBuildIdentity("manja-v2", strings.Repeat("a", 64))
	run, err := NewSemanticRun("pets", identity, semanticSpecRecords(t, "pets"))
	if err != nil {
		t.Fatal(err)
	}

	byCatalog := make(map[string][]Record)
	for _, catalog := range topology.Catalogs {
		records, err := ExpandSemanticRun(catalog, run, semanticLocations(run, catalog.Key))
		if err != nil {
			t.Fatal(err)
		}
		byCatalog[catalog.Key] = records
	}
	if len(byCatalog) != 2 {
		t.Fatalf("expanded catalog count = %d, want 2", len(byCatalog))
	}
	for index := range byCatalog["internal"] {
		internal := byCatalog["internal"][index]
		public := byCatalog["public"][index]
		if internal.ID == public.ID {
			t.Fatalf("catalog occurrences share id %q", internal.ID)
		}
		if internal.LogicalKey != public.LogicalKey || internal.Title != public.Title {
			t.Fatalf("semantic fields changed during expansion: %#v / %#v", internal, public)
		}
		if internal.CatalogKey != "internal" || public.CatalogKey != "public" {
			t.Fatalf("catalog metadata missing: %#v / %#v", internal, public)
		}
	}
	if run.BuildKey == "" {
		t.Fatal("semantic run has no reusable build key")
	}
}

func TestExpandSemanticRunRequiresMembershipAndCompleteLocations(t *testing.T) {
	identity, _ := NewBuildIdentity("manja-v2", strings.Repeat("a", 64))
	run, _ := NewSemanticRun("pets", identity, semanticSpecRecords(t, "pets"))
	nonMember := EffectiveCatalog{Key: "billing", Title: "Billing"}
	if _, err := ExpandSemanticRun(nonMember, run, semanticLocations(run, "billing")); err == nil {
		t.Fatal("expected non-member expansion to fail")
	}
	member := EffectiveCatalog{Key: "public", Title: "Public", Specs: []Spec{{Key: "pets", Title: "Pets"}}}
	locations := semanticLocations(run, "public")
	delete(locations, run.Records[0].ID)
	if _, err := ExpandSemanticRun(member, run, locations); err == nil {
		t.Fatal("expected incomplete locations to fail")
	}
}

func TestDeclaredEmptyCatalogHasNavigationRecord(t *testing.T) {
	topology, err := ResolveTopology(
		[]DeclaredCatalog{{Key: "future", Title: "Future APIs"}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewCatalogRecord(topology.Catalogs[0], "/catalogs/future")
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != KindCatalog || record.CatalogKey != "future" || record.PageHref != "/catalogs/future" {
		t.Fatalf("catalog record = %#v", record)
	}
}

func semanticLocations(run SemanticRun, catalogKey string) map[SemanticRecordID]RecordLocation {
	locations := make(map[SemanticRecordID]RecordLocation, len(run.Records))
	base := "/catalogs/" + catalogKey + "/specs/" + run.SpecKey
	for _, record := range run.Records {
		location := RecordLocation{PageHref: base}
		switch record.Kind {
		case KindOperation:
			location.PageHref += "/operations/list-pets"
			location.FragmentHref = base + "/fragments/operations/list-pets.html"
		case KindSchema:
			location.PageHref += "/schemas/pet"
			location.FragmentHref = base + "/fragments/schemas/pet.html"
		}
		locations[record.ID] = location
	}
	return locations
}
