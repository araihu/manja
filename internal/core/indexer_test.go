package core_test

import (
	"context"
	"os"
	"strings"
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

func TestOpenAPIParserIndexesOverviewMetadata(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Overview API
  version: 1.0.0
  description: Overview description.
  termsOfService: https://example.test/terms
  contact:
    name: Contact Support
    url: https://example.test/support
    email: support@example.test
  license:
    name: MIT
    url: https://example.test/license
servers:
  - url: "{protocol}://{hostname}/api/v3"
    description: Live Server
    variables:
      protocol:
        default: https
        description: Server protocol.
      hostname:
        default: api.example.test
        description: Server host.
paths: {}
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "overview.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}

	if idx.Overview.Description != "Overview description." || idx.Overview.TermsOfService != "https://example.test/terms" {
		t.Fatalf("overview info = %#v", idx.Overview)
	}
	if idx.Overview.Contact.Name != "Contact Support" || idx.Overview.Contact.URL != "https://example.test/support" || idx.Overview.Contact.Email != "support@example.test" {
		t.Fatalf("overview contact = %#v", idx.Overview.Contact)
	}
	if idx.Overview.License.Name != "MIT" || idx.Overview.License.URL != "https://example.test/license" {
		t.Fatalf("overview license = %#v", idx.Overview.License)
	}
	if len(idx.Overview.Servers) != 1 {
		t.Fatalf("overview servers = %#v", idx.Overview.Servers)
	}
	server := idx.Overview.Servers[0]
	if server.URL != "{protocol}://{hostname}/api/v3" || server.Description != "Live Server" {
		t.Fatalf("overview server = %#v", server)
	}
	if len(server.Variables) != 2 || server.Variables[0].Name != "hostname" || server.Variables[0].Default != "api.example.test" || server.Variables[1].Name != "protocol" {
		t.Fatalf("overview server variables = %#v", server.Variables)
	}
}

func TestOpenAPIParserIndexesComponentSchemaTree(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Billing
  version: 1.0.0
paths: {}
components:
  schemas:
    Invoice:
      type: object
      description: Billing invoice.
      required: [id, customer, items]
      properties:
        id:
          type: string
          description: Stable invoice ID.
          example: inv_123
        customer:
          type: object
          description: Customer snapshot.
          required: [email]
          properties:
            email:
              type: string
              format: email
              description: Billing email.
              example: ada@example.test
            name:
              type: string
        items:
          type: array
          description: Purchased line items.
          items:
            type: object
            required: [sku, quantity]
            properties:
              quantity:
                type: integer
                format: int32
                example: 1
              sku:
                type: string
                description: Stock keeping unit.
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "billing.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Schemas) != 1 {
		t.Fatalf("schemas = %#v", idx.Schemas)
	}
	schema := idx.Schemas[0]
	if schema.Name != "Invoice" || schema.Summary.Type != "object" || schema.Summary.Description != "Billing invoice." {
		t.Fatalf("schema summary = %#v", schema)
	}
	if len(schema.Summary.Properties) != 3 {
		t.Fatalf("schema properties = %#v", schema.Summary.Properties)
	}
	customer := schema.Summary.Properties[0]
	if customer.Name != "customer" || !customer.Required || customer.Schema.Type != "object" {
		t.Fatalf("customer property = %#v", customer)
	}
	if len(customer.Schema.Properties) != 2 || customer.Schema.Properties[0].Name != "email" || customer.Schema.Properties[0].Schema.Format != "email" {
		t.Fatalf("customer nested properties = %#v", customer.Schema.Properties)
	}
	if customer.Schema.Properties[0].Schema.Example != "ada@example.test" {
		t.Fatalf("customer email example = %#v", customer.Schema.Properties[0].Schema)
	}
	id := schema.Summary.Properties[1]
	if id.Name != "id" || !id.Required || id.Schema.Example != "inv_123" {
		t.Fatalf("id property = %#v", id)
	}
	items := schema.Summary.Properties[2]
	if items.Name != "items" || !items.Required || items.Schema.Type != "array" || items.Schema.Items == nil {
		t.Fatalf("items property = %#v", items)
	}
	if len(items.Schema.Items.Properties) != 2 || items.Schema.Items.Properties[0].Name != "quantity" || items.Schema.Items.Properties[0].Schema.Format != "int32" {
		t.Fatalf("item nested properties = %#v", items.Schema.Items)
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

func TestOpenAPIParserIndexesOperationRequestAndResponses(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Todos
  version: 1.0.0
servers:
  - url: https://api.example.test
paths:
  /todos/{todoId}:
    put:
      operationId: updateTodo
      tags:
        - todos
      summary: Update Todo
      description: Updates a todo item.
      parameters:
        - name: todoId
          in: path
          required: true
          description: Todo identifier.
          schema:
            type: string
        - name: include
          in: query
          description: Include related resources.
          schema:
            type: string
            enum: [owner]
            default: owner
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/TodoInput"
      responses:
        "200":
          description: Updated todo.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Todo"
        "404":
          description: Not found.
      security:
        - bearerAuth: []
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
  schemas:
    TodoInput:
      type: object
      required: [name]
      properties:
        name:
          type: string
          description: Name of the task.
        completed:
          type: boolean
    Todo:
      type: object
      required: [id, name]
      properties:
        id:
          type: string
          description: ID of the task.
        name:
          type: string
          description: Name of the task.
        completed:
          type: boolean
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "todos.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	op := idx.Operations[0]
	if len(op.Parameters) != 2 {
		t.Fatalf("parameters = %#v", op.Parameters)
	}
	if op.Parameters[0].Name != "todoId" || op.Parameters[0].In != "path" || !op.Parameters[0].Required || op.Parameters[0].Schema.Type != "string" {
		t.Fatalf("path parameter = %#v", op.Parameters[0])
	}
	if op.Parameters[1].Name != "include" || op.Parameters[1].In != "query" || op.Parameters[1].Schema.Type != "string" || op.Parameters[1].Schema.Default != "owner" {
		t.Fatalf("query parameter = %#v", op.Parameters[1])
	}
	if op.RequestBody == nil || !op.RequestBody.Required || len(op.RequestBody.MediaTypes) != 1 {
		t.Fatalf("request body = %#v", op.RequestBody)
	}
	requestMedia := op.RequestBody.MediaTypes[0]
	if requestMedia.ContentType != "application/json" || requestMedia.Schema.Name != "TodoInput" {
		t.Fatalf("request media = %#v", requestMedia)
	}
	if !strings.Contains(requestMedia.Schema.JSON, `"required":["name"]`) {
		t.Fatalf("request schema JSON = %q", requestMedia.Schema.JSON)
	}
	if !strings.Contains(requestMedia.Example, `"name"`) || !strings.Contains(requestMedia.Example, `"completed"`) {
		t.Fatalf("request example = %q", requestMedia.Example)
	}
	if requestMedia.ExampleProvided {
		t.Fatalf("request media should mark generated fallback example as not provided: %#v", requestMedia)
	}
	if len(op.Responses) != 2 || op.Responses[0].Status != "200" || op.Responses[1].Status != "404" {
		t.Fatalf("responses = %#v", op.Responses)
	}
	responseMedia := op.Responses[0].MediaTypes[0]
	if responseMedia.ContentType != "application/json" || responseMedia.Schema.Name != "Todo" {
		t.Fatalf("response media = %#v", responseMedia)
	}
	if !strings.Contains(responseMedia.Schema.JSON, `"required":["id","name"]`) {
		t.Fatalf("response schema JSON = %q", responseMedia.Schema.JSON)
	}
	if responseMedia.Schema.Properties[0].Name != "completed" {
		t.Fatalf("response schema properties = %#v", responseMedia.Schema.Properties)
	}
	if responseMedia.ExampleProvided {
		t.Fatalf("response media should mark generated fallback example as not provided: %#v", responseMedia)
	}
	if idx.Schemas[0].Example.JSON == "" || !strings.Contains(idx.Schemas[0].Example.JSON, `"properties"`) {
		t.Fatalf("schema example payload = %#v", idx.Schemas[0].Example)
	}
	if !strings.Contains(idx.ExampleSpecJSON, `"#/components/schemas/Todo"`) && !strings.Contains(idx.ExampleSpecJSON, `"Todo"`) {
		t.Fatalf("example spec JSON = %q", idx.ExampleSpecJSON)
	}
	if len(op.Security) != 1 || op.Security[0].Name != "bearerAuth" {
		t.Fatalf("security = %#v", op.Security)
	}
	if len(op.Snippets) == 0 || op.Snippets[0].Language != "shell" || !strings.Contains(op.Snippets[0].Code, "https://api.example.test/todos/{todoId}") {
		t.Fatalf("snippets = %#v", op.Snippets)
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
	var workflow core.Schema
	for _, schema := range idx.Schemas {
		if schema.Name == "workflow" {
			workflow = schema
			break
		}
	}
	if workflow.Name == "" {
		t.Fatalf("workflow schema not indexed")
	}
	if workflow.Summary.Name != "Workflow" {
		t.Fatalf("workflow display name = %q, want Workflow", workflow.Summary.Name)
	}
	if len(idx.Search) != len(idx.Operations)+len(idx.Schemas)+1 {
		t.Fatalf("search documents = %d, want %d", len(idx.Search), len(idx.Operations)+len(idx.Schemas)+1)
	}
}
