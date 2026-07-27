package projectionjson

import (
	"reflect"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestCodecRejectsNoncanonicalRecordSemantics(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*projection.Document)
	}{
		{"overview heading differs from document title", func(document *projection.Document) {
			document.Overview.Heading = "forged"
		}},
		{"server ordinal is not dense", func(document *projection.Document) {
			document.Overview.Servers[0].Ordinal = 7
		}},
		{"server ID is not derived from URL", func(document *projection.Document) {
			document.Overview.Servers[0].ID = "server-forged"
		}},
		{"server variable ordinal is not dense", func(document *projection.Document) {
			document.Overview.Servers[0].Variables[0].Ordinal = 7
		}},
		{"server variable ID is not derived", func(document *projection.Document) {
			document.Overview.Servers[0].Variables[0].ID = "server-variable-forged"
		}},
		{"server enum ordinal is not dense", func(document *projection.Document) {
			document.Overview.Servers[0].Variables[0].Enum[0].Ordinal = 7
		}},
		{"server enum ID is not derived", func(document *projection.Document) {
			document.Overview.Servers[0].Variables[0].Enum[0].ID = "server-enum-forged"
		}},
		{"sidebar section ID is not derived", func(document *projection.Document) {
			document.SidebarSections[0].ID = "operation-tag-forged"
		}},
		{"sidebar section href is external", func(document *projection.Document) {
			document.SidebarSections[0].Href = "https://evil.example/?section=forged"
		}},
		{"sidebar item ordinal does not identify operation", func(document *projection.Document) {
			document.SidebarSections[0].Items[0].Ordinal = 7
		}},
		{"sidebar item identity differs from operation", func(document *projection.Document) {
			document.SidebarSections[0].Items[0].ID = "operation-forged"
		}},
		{"operation directory method differs from detail", func(document *projection.Document) {
			document.Operations[0].Method = "PATCH"
		}},
		{"operation directory title differs from detail heading", func(document *projection.Document) {
			document.Operations[0].Title = "forged"
		}},
		{"operation section ordinal is not dense", func(document *projection.Document) {
			document.Operations[0].Sections[0].Ordinal = 7
		}},
		{"operation section ID is not derived", func(document *projection.Document) {
			document.Operations[0].Sections[0].ID = "operation-section-forged"
		}},
		{"operation section does not match sidebar membership", func(document *projection.Document) {
			document.Operations[0].Sections[0].Value = document.SidebarSections[1].ID
		}},
		{"tag ordinal is not dense", func(document *projection.Document) {
			document.OperationDetails[0].Tags[0].Ordinal = 7
		}},
		{"tag ID is not derived", func(document *projection.Document) {
			document.OperationDetails[0].Tags[0].ID = "tag-forged"
		}},
		{"parameter ordinal is not dense", func(document *projection.Document) {
			document.OperationDetails[0].Parameters[0].Ordinal = 7
		}},
		{"parameter ID is not derived", func(document *projection.Document) {
			document.OperationDetails[0].Parameters[0].ID = "parameter-forged"
		}},
		{"parameter example ID is not canonical", func(document *projection.Document) {
			document.OperationDetails[0].Parameters[0].Examples[0].ID = "forged"
		}},
		{"request body presence disagrees with populated body", func(document *projection.Document) {
			document.OperationDetails[0].HasRequestBody = false
		}},
		{"absent request body retains description", func(document *projection.Document) {
			document.OperationDetails[1].RequestBody.Description = "forged"
		}},
		{"request media ordinal is not dense", func(document *projection.Document) {
			document.OperationDetails[0].RequestBody.MediaTypes[0].Ordinal = 7
		}},
		{"request media ID differs from content type", func(document *projection.Document) {
			document.OperationDetails[0].RequestBody.MediaTypes[0].ID = "text/plain"
		}},
		{"media example ID is not canonical", func(document *projection.Document) {
			document.OperationDetails[0].RequestBody.MediaTypes[0].Examples[0].ID = "forged"
		}},
		{"response ordinal is not dense", func(document *projection.Document) {
			document.OperationDetails[0].Responses[0].Ordinal = 7
		}},
		{"response ID differs from status", func(document *projection.Document) {
			document.OperationDetails[0].Responses[0].ID = "200"
		}},
		{"response media ID differs from content type", func(document *projection.Document) {
			document.OperationDetails[0].Responses[0].MediaTypes[0].ID = "text/plain"
		}},
		{"security requirement ID differs from name", func(document *projection.Document) {
			document.OperationDetails[0].Security[0].ID = "forged"
		}},
		{"security scope ID is not derived", func(document *projection.Document) {
			document.OperationDetails[0].Security[0].Scopes[0].ID = "scope-forged"
		}},
		{"code sample ordinal is not dense", func(document *projection.Document) {
			document.OperationDetails[0].CodeSamples[0].Ordinal = 7
		}},
		{"code sample ID is not derived", func(document *projection.Document) {
			document.OperationDetails[0].CodeSamples[0].ID = "code-sample-forged"
		}},
		{"schema directory name differs from title", func(document *projection.Document) {
			document.Schemas[0].Name = "forged"
		}},
		{"schema detail heading differs from directory", func(document *projection.Document) {
			document.SchemaDetails[0].Heading = "forged"
		}},
		{"schema example ID is not canonical", func(document *projection.Document) {
			document.SchemaDetails[1].Examples[0].ID = "forged"
		}},
		{"schema sidebar item differs from directory", func(document *projection.Document) {
			document.SidebarSections[len(document.SidebarSections)-1].Items[0].Label = "forged"
		}},
		{"search result ID is not derived", func(document *projection.Document) {
			document.Search[0].ResultID = "search-result-forged"
		}},
		{"search ID no longer matches result ID", func(document *projection.Document) {
			document.Search[0].ID = "operation-lisz"
		}},
		{"search keyword ordinal is not dense", func(document *projection.Document) {
			document.Search[0].Keywords[0].Ordinal = 7
		}},
		{"search keyword ID is not derived", func(document *projection.Document) {
			document.Search[0].Keywords[0].ID = "keyword-forged"
		}},
		{"search href is absolute external URL", func(document *projection.Document) {
			document.Search[0].Href = "https://evil.example?selected=operation-list-pets#operation-list-pets"
		}},
		{"public route path is absolute external URL", func(document *projection.Document) {
			document.PublicRoutes[len(document.PublicRoutes)-1].Path = "https://evil.example?selected=schema-pet#schema-pet"
		}},
		{"public route targets unknown anchor", func(document *projection.Document) {
			document.PublicRoutes[len(document.PublicRoutes)-1].Path = "/z?selected=unknown#unknown"
		}},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			document := mustBuild(t, semanticValidationFixture())
			mutation.mutate(&document)

			if encoded, err := Marshal(document); err == nil {
				t.Errorf("Marshal accepted noncanonical document (%d bytes)", len(encoded))
			}

			decoded, err := Unmarshal(marshalUnchecked(t, document))
			if err == nil {
				t.Error("Unmarshal accepted noncanonical document")
			}
			if !reflect.ValueOf(decoded).IsZero() {
				t.Fatalf("Unmarshal returned nonzero document: formatVersion=%d", decoded.FormatVersion)
			}
		})
	}
}

func semanticValidationFixture() domain.SpecIndex {
	input := fullFixture()
	input.Operations[0].Responses[0].MediaTypes = []domain.OperationMediaType{{
		ContentType:     "application/problem+json",
		Schema:          domain.SchemaSummary{Type: "object"},
		Example:         "{}",
		ExampleProvided: true,
	}}
	input.Search[1].Keywords = []string{"pets", "animals"}
	return input
}
