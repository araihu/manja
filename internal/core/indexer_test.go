package core_test

import (
	"context"
	"os"
	"testing"

	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	"github.com/araihu/manja/internal/core"
)

func TestOpenAPIParserBuildsSearchIndex(t *testing.T) {
	data, err := os.ReadFile("../adapters/openapi/testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "openapi.yaml",
		Format:   "yaml",
		Bytes:    data,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Title != "Petstore" || idx.Version != "1.0.0" {
		t.Fatalf("identity = %q %q", idx.Title, idx.Version)
	}
	if len(idx.Operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(idx.Operations))
	}
	if idx.Operations[0].Anchor != "operation-listpets" {
		t.Fatalf("first operation anchor = %q", idx.Operations[0].Anchor)
	}
	if idx.Search[0].Title != "GET /pets" {
		t.Fatalf("first search title = %q", idx.Search[0].Title)
	}
	if idx.Search[0].Href != "#"+idx.Operations[0].Anchor || idx.PublicRoutes[1].Path != idx.Search[0].Href {
		t.Fatalf("operation anchor mismatch: operation = %q, search = %q, route = %q", idx.Operations[0].Anchor, idx.Search[0].Href, idx.PublicRoutes[1].Path)
	}
	if len(idx.Schemas) != 1 || idx.Schemas[0].Name != "Pet" {
		t.Fatalf("schemas = %#v", idx.Schemas)
	}
}

func TestOpenAPIParserBuildsStableAnchorsWithoutOperationID(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Anonymous Operations
  version: 1.0.0
paths:
  /pets:
    get:
      summary: List pets
      responses:
        "200":
          description: ok
  /pets/{petId}:
    get:
      summary: Get pet
      parameters:
        - name: petId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
components:
  schemas: {}
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "anonymous.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Search) < 2 {
		t.Fatalf("search documents = %d, want at least 2", len(idx.Search))
	}
	if idx.Search[0].ID != "operation-get-pets" || idx.Search[0].Href != "#operation-get-pets" {
		t.Fatalf("first operation anchor = %q %q", idx.Search[0].ID, idx.Search[0].Href)
	}
	if idx.Operations[0].Anchor != idx.Search[0].ID {
		t.Fatalf("first operation anchor = %q, search id = %q", idx.Operations[0].Anchor, idx.Search[0].ID)
	}
	if idx.Search[1].ID != "operation-get-pets-petid" || idx.Search[1].Href != "#operation-get-pets-petid" {
		t.Fatalf("second operation anchor = %q %q", idx.Search[1].ID, idx.Search[1].Href)
	}
	if idx.Operations[1].Anchor != idx.Search[1].ID {
		t.Fatalf("second operation anchor = %q, search id = %q", idx.Operations[1].Anchor, idx.Search[1].ID)
	}
	if idx.PublicRoutes[1].Path != idx.Search[0].Href || idx.PublicRoutes[2].Path != idx.Search[1].Href {
		t.Fatalf("public route paths = %#v, search hrefs = %#v", idx.PublicRoutes, idx.Search[:2])
	}
}

func TestOpenAPIParserHandlesSpecWithoutComponents(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Componentless API
  version: 1.0.0
paths: {}
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "componentless.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Title != "Componentless API" {
		t.Fatalf("title = %q", idx.Title)
	}
	if len(idx.Schemas) != 0 {
		t.Fatalf("schemas = %#v", idx.Schemas)
	}
}

func TestOpenAPIParserIgnoresInvalidExamplesForDocsIndexing(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info:
  title: Example Tolerant API
  version: 1.0.0
paths:
  /events:
    get:
      tags:
        - events
      summary: List events
      responses:
        "200":
          description: ok
components:
  schemas:
    Event:
      type: object
      properties:
        published_at:
          type: string
          format: date-time
      example:
        published_at: not-a-date-time
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "examples.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Title != "Example Tolerant API" {
		t.Fatalf("title = %q", idx.Title)
	}
	if len(idx.Operations) != 1 || idx.Operations[0].Path != "/events" {
		t.Fatalf("operations = %#v", idx.Operations)
	}
	if len(idx.Schemas) != 1 || idx.Schemas[0].Name != "Event" {
		t.Fatalf("schemas = %#v", idx.Schemas)
	}
}

func TestOpenAPIParserIndexesGitHubRESTFixture(t *testing.T) {
	data, err := os.ReadFile("../adapters/openapi/testdata/github-v3-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "github-v3-rest.json",
		Format:   "json",
		Bytes:    data,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Title != "GitHub v3 REST API" || idx.Version != "1.1.4" {
		t.Fatalf("identity = %q %q", idx.Title, idx.Version)
	}
	if len(idx.Operations) != 674 {
		t.Fatalf("operations = %d, want 674", len(idx.Operations))
	}
	if len(idx.Schemas) != 278 {
		t.Fatalf("schemas = %d, want 278", len(idx.Schemas))
	}
	if len(idx.Search) != len(idx.Operations)+len(idx.Schemas)+1 {
		t.Fatalf("search documents = %d, want %d", len(idx.Search), len(idx.Operations)+len(idx.Schemas)+1)
	}
}
