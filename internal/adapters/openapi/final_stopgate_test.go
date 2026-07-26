package openapi_test

import (
	"context"
	"os"
	"testing"

	core "github.com/araihu/manja/domain"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

func TestSupportedGitHubFixtureFitsBoundedSchemaValidation(t *testing.T) {
	raw, err := os.ReadFile("testdata/github-v3-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	index, err := (openapiadapter.Parser{}).Parse(
		context.Background(),
		core.SpecFile{Path: "github-v3-rest.json", Format: "json", Bytes: raw},
		core.ContractRevision{ID: "github-v3-rest"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.ValidateSpecIndex(index); err != nil {
		t.Fatalf("supported default fixture failed bounded validation: %v", err)
	}
}

func TestOpenAPIParserAppliesOperationParameterOverrides(t *testing.T) {
	raw := []byte(`
openapi: 3.1.0
info: {title: Override, version: 1.0.0}
paths:
  /payments/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema: {type: string}
      - name: locale
        in: query
        schema: {type: string}
    get:
      parameters:
        - name: id
          in: path
          required: true
          description: operation override
          schema: {type: integer}
      responses:
        "200": {description: ok}
`)
	index, err := (openapiadapter.Parser{}).Parse(
		context.Background(),
		core.SpecFile{Path: "openapi.yaml", Format: "yaml", Bytes: raw},
		core.ContractRevision{ID: "revision"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Operations) != 1 || len(index.Operations[0].Parameters) != 2 {
		t.Fatalf("effective parameters = %#v", index.Operations)
	}
	parameters := index.Operations[0].Parameters
	if parameters[0].Name != "id" ||
		parameters[0].In != "path" ||
		parameters[0].Description != "operation override" ||
		parameters[0].Schema.Type != "integer" {
		t.Fatalf("path parameter was not overridden: %#v", parameters[0])
	}
	if parameters[1].Name != "locale" || parameters[1].In != "query" {
		t.Fatalf("unshadowed path parameter was not retained: %#v", parameters[1])
	}
	if err := core.ValidateSpecIndex(index); err != nil {
		t.Fatalf("valid OpenAPI parameter override became ambiguous: %v", err)
	}
}

func TestOpenAPIParserRejectsTrueParameterDuplicates(t *testing.T) {
	tests := []struct {
		name       string
		parameters string
	}{
		{
			name: "path item",
			parameters: `
    parameters:
      - {name: id, in: query, schema: {type: string}}
      - {name: id, in: query, schema: {type: integer}}
    get:
`,
		},
		{
			name: "operation",
			parameters: `
    get:
      parameters:
        - {name: id, in: query, schema: {type: string}}
        - {name: id, in: query, schema: {type: integer}}
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(`
openapi: 3.1.0
info: {title: Duplicate, version: 1.0.0}
paths:
  /payments:
` + test.parameters + `
      responses:
        "200": {description: ok}
`)
			if _, err := (openapiadapter.Parser{}).Parse(
				context.Background(),
				core.SpecFile{Path: "openapi.yaml", Format: "yaml", Bytes: raw},
				core.ContractRevision{ID: "revision"},
			); err == nil {
				t.Fatal("parser accepted a truly ambiguous parameter duplicate")
			}
		})
	}
}

func TestOpenAPIParserDoesNotHideNonCanonicalOverrideIdentity(t *testing.T) {
	raw := []byte(`
openapi: 3.1.0
info: {title: Invalid Override, version: 1.0.0}
paths:
  /payments:
    parameters:
      - name: " id"
        in: query
        schema: {type: string}
    get:
      parameters:
        - name: id
          in: query
          schema: {type: integer}
      responses:
        "200": {description: ok}
`)
	if _, err := (openapiadapter.Parser{}).Parse(
		context.Background(),
		core.SpecFile{Path: "openapi.yaml", Format: "yaml", Bytes: raw},
		core.ContractRevision{ID: "revision"},
	); err == nil {
		t.Fatal("parser normalized a non-canonical path parameter into a valid override")
	}
}

func BenchmarkValidateSupportedGitHubFixture(b *testing.B) {
	raw, err := os.ReadFile("testdata/github-v3-rest.json")
	if err != nil {
		b.Fatal(err)
	}
	index, err := (openapiadapter.Parser{}).Parse(
		context.Background(),
		core.SpecFile{Path: "github-v3-rest.json", Format: "json", Bytes: raw},
		core.ContractRevision{ID: "github-v3-rest"},
	)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := core.ValidateSpecIndex(index); err != nil {
			b.Fatal(err)
		}
	}
}
