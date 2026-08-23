package projection

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestBuilderInternsSchemasAcrossAllRoots(t *testing.T) {
	shared := domain.SchemaSummary{
		Name: "Shared", Type: "object", Description: "same schema",
		Properties: []domain.SchemaProperty{{Name: "id", Required: true, Schema: domain.SchemaSummary{Type: "string"}}},
	}
	input := minimalIndex()
	input.Operations = []domain.Operation{{
		Anchor: "operation-shared", Method: "POST", Path: "/shared",
		Parameters:  []domain.OperationParameter{{Name: "shared", In: "query", Schema: shared}},
		RequestBody: &domain.OperationRequestBody{MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: shared}}},
		Responses:   []domain.OperationResponse{{Status: "200", MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: shared}}}},
	}}
	input.Schemas = []domain.Schema{{Name: "Shared", Summary: shared}}

	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.SchemaNodes) != 2 {
		t.Fatalf("schema nodes = %d, want shared root and child", len(document.SchemaNodes))
	}
	operation := document.OperationDetails[0]
	refs := []SchemaRef{
		operation.Parameters[0].SchemaRef,
		operation.RequestBody.MediaTypes[0].SchemaRef,
		operation.Responses[0].MediaTypes[0].SchemaRef,
		document.SchemaDetails[0].SchemaRef,
	}
	for _, ref := range refs[1:] {
		if ref != refs[0] {
			t.Fatalf("root refs = %#v, want one interned root", refs)
		}
	}
}

func TestBuilderSchemaNodeIdentityUsesChildDigest(t *testing.T) {
	input := minimalIndex()
	input.Operations = []domain.Operation{{
		Anchor: "operation-child-digests", Method: "GET", Path: "/children",
		Parameters: []domain.OperationParameter{
			{Name: "text", In: "query", Schema: domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "string"}}},
			{Name: "count", In: "query", Schema: domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "integer"}}},
		},
	}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	parameters := document.OperationDetails[0].Parameters
	first := document.SchemaNodes[parameters[0].SchemaRef]
	second := document.SchemaNodes[parameters[1].SchemaRef]
	if first.ID == second.ID {
		t.Fatalf("parents with different child digests share ID %q", first.ID)
	}
}

func TestBuilderSchemaNodeOrderIgnoresTraversalOrder(t *testing.T) {
	base := minimalIndex()
	base.Operations = []domain.Operation{
		{Anchor: "operation-z", Method: "GET", Path: "/z", Parameters: []domain.OperationParameter{{Name: "z", In: "query", Schema: domain.SchemaSummary{Type: "integer"}}}},
		{Anchor: "operation-a", Method: "GET", Path: "/a", Parameters: []domain.OperationParameter{{Name: "a", In: "query", Schema: domain.SchemaSummary{Type: "string"}}}},
	}
	reversed := base
	reversed.Operations = []domain.Operation{base.Operations[1], base.Operations[0]}

	first, err := (Builder{}).Build(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (Builder{}).Build(context.Background(), reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.SchemaNodes, second.SchemaNodes) {
		t.Fatalf("schema node order changed with traversal\nfirst:  %#v\nsecond: %#v", first.SchemaNodes, second.SchemaNodes)
	}
	for index, node := range first.SchemaNodes {
		if node.Ordinal != uint32(index) || index > 0 && first.SchemaNodes[index-1].ID >= node.ID {
			t.Fatalf("schemaNodes[%d] is not canonical: %#v", index, node)
		}
	}
}

func TestBuilderSchemaHashCollisionFailsClosed(t *testing.T) {
	input := minimalIndex()
	input.Operations = []domain.Operation{{
		Anchor: "operation-collision", Method: "GET", Path: "/collision",
		Parameters: []domain.OperationParameter{
			{Name: "one", In: "query", Schema: domain.SchemaSummary{Type: "string"}},
			{Name: "two", In: "query", Schema: domain.SchemaSummary{Type: "integer"}},
		},
	}}
	document, err := buildWithSchemaHasher(context.Background(), input, func([]byte) [32]byte {
		return [32]byte{1}
	}, true)
	if err == nil || !reflect.ValueOf(document).IsZero() || !strings.Contains(err.Error(), "hash_collision") {
		t.Fatalf("collision build = %#v, %v; want zero document hash_collision", document, err)
	}
}

func TestBuilderPreservesAllInternedSchemaContent(t *testing.T) {
	input := minimalIndex()
	input.Schemas = []domain.Schema{{
		Name: "Envelope", Description: "detail description",
		Summary: domain.SchemaSummary{
			Name: "Envelope summary", Type: "object", Format: "custom", Description: "node description",
			Default: "default text", Example: "example text", JSON: `{"z":1e+03,"a":"<tag>"}`,
			Properties: []domain.SchemaProperty{{
				Name: "value", Required: true, Description: "edge description",
				Schema: domain.SchemaSummary{Name: "Value", Type: "string", Format: "uuid", Description: "child description", Default: "child default", Example: "child example", JSON: `{"child":1.0}`},
			}},
			Items: &domain.SchemaSummary{Name: "Item", Type: "integer", Format: "int64", Description: "item description", Default: "0", Example: "1", JSON: `{"item":-0}`},
		},
		Example: domain.SchemaExample{JSON: `{"shape":1e-3}`, Example: "explicit example", Provided: true},
	}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	detail := document.SchemaDetails[0]
	root := document.SchemaNodes[detail.SchemaRef]
	if root.Name != "Envelope summary" || root.Type != "object" || root.Format != "custom" ||
		root.Description != "node description" || root.DefaultValue != "default text" ||
		root.ExampleText != "example text" || root.JSON != `{"a":"\u003ctag\u003e","z":1000}` {
		t.Fatalf("root content changed: %#v", root)
	}
	if len(root.Properties) != 1 || root.Properties[0].Description != "edge description" || !root.Properties[0].Required {
		t.Fatalf("property edge changed: %#v", root.Properties)
	}
	property := document.SchemaNodes[root.Properties[0].SchemaRef]
	if property.Name != "Value" || property.Format != "uuid" || property.JSON != `{"child":1}` {
		t.Fatalf("property node changed: %#v", property)
	}
	if len(root.Items) != 1 || root.Items[0].ID != "items" {
		t.Fatalf("item edge changed: %#v", root.Items)
	}
	item := document.SchemaNodes[root.Items[0].SchemaRef]
	if item.Name != "Item" || item.Format != "int64" || item.JSON != `{"item":0}` {
		t.Fatalf("item node changed: %#v", item)
	}
	if detail.ExampleSchemaJSON != `{"shape":0.001}` || len(detail.Examples) != 1 || detail.Examples[0].Text != "explicit example" {
		t.Fatalf("schema detail content changed: %#v", detail)
	}
}
