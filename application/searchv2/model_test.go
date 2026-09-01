package searchv2

import (
	"testing"

	"github.com/araihu/manja/domain"
)

func TestRecordIDsAreStableAndOccurrenceScoped(t *testing.T) {
	first := mustRecord(t, RecordInput{
		Kind: KindSchema, CatalogKey: "apis", CatalogTitle: "APIs",
		SpecKey: "pets-v1", SpecTitle: "Pets v1",
		LogicalKey: "#/components/schemas/Pet", Title: "Pet",
		PageHref:     "/catalogs/apis/specs/pets-v1/schemas/pet",
		FragmentHref: "/catalogs/apis/specs/pets-v1/fragments/schemas/pet.html",
	})
	repeated := mustRecord(t, RecordInput{
		Kind: KindSchema, CatalogKey: "apis", CatalogTitle: "Renamed catalog",
		SpecKey: "pets-v1", SpecTitle: "Renamed spec",
		LogicalKey: "#/components/schemas/Pet", Title: "Renamed Pet",
		Description: "Changed display text",
		PageHref:    "/new/page", FragmentHref: "/new/fragment",
	})
	otherSpec := mustRecord(t, RecordInput{
		Kind: KindSchema, CatalogKey: "apis", CatalogTitle: "APIs",
		SpecKey: "pets-v2", SpecTitle: "Pets v2",
		LogicalKey: "#/components/schemas/Pet", Title: "Pet",
		PageHref:     "/catalogs/apis/specs/pets-v2/schemas/pet",
		FragmentHref: "/catalogs/apis/specs/pets-v2/fragments/schemas/pet.html",
	})

	if first.ID != repeated.ID {
		t.Fatalf("display metadata changed stable identity: %q != %q", first.ID, repeated.ID)
	}
	if first.ID == otherSpec.ID {
		t.Fatalf("identical schema occurrences in different specs share id %q", first.ID)
	}
}

func TestSemanticRecordIdentityIsCatalogIndependentAndSpecScoped(t *testing.T) {
	first := mustSemanticRecord(t, SemanticRecordInput{
		Kind: KindSchema, SpecKey: "pets-v1", SpecTitle: "Pets v1",
		LogicalKey: "#/components/schemas/Pet", Title: "Pet", Description: "A pet",
	})
	renamed := mustSemanticRecord(t, SemanticRecordInput{
		Kind: KindSchema, SpecKey: "pets-v1", SpecTitle: "Pets renamed",
		LogicalKey: "#/components/schemas/Pet", Title: "Animal", Description: "Changed",
	})
	otherSpec := mustSemanticRecord(t, SemanticRecordInput{
		Kind: KindSchema, SpecKey: "pets-v2", SpecTitle: "Pets v2",
		LogicalKey: "#/components/schemas/Pet", Title: "Pet",
	})
	if first.ID != renamed.ID {
		t.Fatalf("display metadata changed semantic identity: %q != %q", first.ID, renamed.ID)
	}
	if first.ID == otherSpec.ID {
		t.Fatalf("identical schemas in different specs share semantic id %q", first.ID)
	}
}

func TestFixedQueryOperationVariantsHaveDistinctSemanticIdentity(t *testing.T) {
	reset := mustSemanticRecord(t, SemanticRecordInput{
		Kind: KindOperation, SpecKey: "vcenter", SpecTitle: "vCenter",
		LogicalKey: "POST /appliance/networking?action=reset", Title: "Reset networking",
		OperationID: "Appliance.Networking_reset", Method: "POST", Path: "/appliance/networking",
		RequestTarget: "/appliance/networking?action=reset",
		FixedQuery:    []domain.FixedQueryParameter{{Name: "action", Value: "reset"}},
	})
	change := mustSemanticRecord(t, SemanticRecordInput{
		Kind: KindOperation, SpecKey: "vcenter", SpecTitle: "vCenter",
		LogicalKey: "POST /appliance/networking?action=change", Title: "Change networking",
		OperationID: "Appliance.Networking_change", Method: "POST", Path: "/appliance/networking",
		RequestTarget: "/appliance/networking?action=change",
		FixedQuery:    []domain.FixedQueryParameter{{Name: "action", Value: "change"}},
	})
	if reset.ID == change.ID {
		t.Fatalf("fixed-query variants share semantic id %q", reset.ID)
	}
	if reset.Path != "/appliance/networking" || reset.RequestTarget != "/appliance/networking?action=reset" || len(reset.FixedQuery) != 1 {
		t.Fatalf("fixed-query semantic record = %#v", reset)
	}
}

func TestNewRecordRepresentsEverySearchKind(t *testing.T) {
	tests := []RecordInput{
		{
			Kind: KindCatalog, CatalogKey: "platform", CatalogTitle: "Platform",
			LogicalKey: "platform", Title: "Platform", PageHref: "/catalogs/platform",
		},
		{
			Kind: KindSpec, CatalogKey: "platform", CatalogTitle: "Platform",
			SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "pets", Title: "Pets",
			PageHref: "/catalogs/platform/specs/pets",
		},
		{
			Kind: KindPath, CatalogKey: "platform", CatalogTitle: "Platform",
			SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "/pets/{id}",
			Title: "/pets/{id}", Description: "A pet resource", Path: "/pets/{id}",
			PageHref:     "/catalogs/platform/specs/pets/paths/pets-id",
			FragmentHref: "/catalogs/platform/specs/pets/fragments/paths/pets-id.html",
		},
		{
			Kind: KindOperation, CatalogKey: "platform", CatalogTitle: "Platform",
			SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "GET /pets/{id}",
			Title: "Get pet", Description: "Find a pet", OperationID: "getPet",
			Method: "get", Path: "/pets/{id}",
			PageHref:     "/catalogs/platform/specs/pets/operations/get-pet",
			FragmentHref: "/catalogs/platform/specs/pets/fragments/operations/get-pet.html",
		},
		{
			Kind: KindSchema, CatalogKey: "platform", CatalogTitle: "Platform",
			SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "#/components/schemas/Pet",
			Title: "Pet", Description: "A pet",
			PageHref:     "/catalogs/platform/specs/pets/schemas/pet",
			FragmentHref: "/catalogs/platform/specs/pets/fragments/schemas/pet.html",
		},
	}
	for _, input := range tests {
		record := mustRecord(t, input)
		if record.ID == "" {
			t.Fatalf("kind %q has empty id", input.Kind)
		}
		if input.Kind == KindOperation && record.Method != "GET" {
			t.Fatalf("operation method = %q, want GET", record.Method)
		}
	}
}

func TestSemanticRecordsRejectNonCanonicalResourceKeys(t *testing.T) {
	tests := []SemanticRecordInput{
		{Kind: KindPath, SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "/pets", Title: "Pets", Path: "/pets"},
		{Kind: KindOperation, SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "get /pets", Title: "List pets", Method: "get", Path: "/pets"},
		{Kind: KindSchema, SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "Pet", Title: "Pet"},
		{Kind: KindSchema, SpecKey: "pets", SpecTitle: "Pets", LogicalKey: "#/components/schemas/Foo/Bar", Title: "Foo"},
	}
	for _, input := range tests {
		if _, err := NewSemanticRecord(input); err == nil {
			t.Fatalf("expected non-canonical input to fail: %#v", input)
		}
	}
}

func mustRecord(t *testing.T, input RecordInput) Record {
	t.Helper()
	record, err := NewRecord(input)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
