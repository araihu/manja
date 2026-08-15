package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestPreparedSchemaDetailBodyRendersNodeAndExampleLayout(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	fragment, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`class="manja-schema-detail-layout"`,
		`class="manja-schema-tree-panel"`,
		`id="schema-node-panel"`,
		`class="manja-schema-example-panel"`,
		`&lt;pod&gt;`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("schema detail body missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte("<pod>")) {
		t.Fatalf("schema detail body leaked unescaped example: %s", body)
	}
}

func TestPrepareSchemaDetailBodyFailsClosedOnInvalidOrMixedChildren(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	otherDetail, otherSchema, otherDocument := cloneDetail(detail), schema, document
	otherSchema.Name = otherDetail.Schema.Heading
	otherSchema.Description = otherDetail.Schema.Description
	otherDocument.Key = "other-v1"
	otherDetail.Schema.Href = "documents/other-v1/?selected=" + string(otherDetail.ID) + "#" + string(otherDetail.ID)
	otherNode, err := PrepareSchemaNode(otherDetail, projection.SchemaNode{Ordinal: 7, ID: "node-other", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/other-v1/")
	if err != nil {
		t.Fatal(err)
	}
	otherExample, err := PrepareSchemaDetailExample(otherDetail, otherSchema, otherDocument)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(**SchemaNodeFragment, **SchemaDetailExampleFragment)
	}{
		{name: "missing node", mutate: func(node **SchemaNodeFragment, _ **SchemaDetailExampleFragment) {
			*node = nil
		}},
		{name: "invalid node", mutate: func(node **SchemaNodeFragment, _ **SchemaDetailExampleFragment) {
			(*node).valid = false
		}},
		{name: "invalid node text", mutate: func(node **SchemaNodeFragment, _ **SchemaDetailExampleFragment) {
			(*node).data.Edges[0].Name = string([]byte{0xff})
		}},
		{name: "invalid example", mutate: func(_ **SchemaNodeFragment, example **SchemaDetailExampleFragment) {
			(*example).valid = false
		}},
		{name: "mixed node context", mutate: func(node **SchemaNodeFragment, _ **SchemaDetailExampleFragment) {
			*node = &otherNode
		}},
		{name: "mixed example context", mutate: func(_ **SchemaNodeFragment, example **SchemaDetailExampleFragment) {
			*example = &otherExample
		}},
		{name: "missing example", mutate: func(_ **SchemaNodeFragment, example **SchemaDetailExampleFragment) {
			*example = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodeValue, exampleValue := &node, &example
			test.mutate(&nodeValue, &exampleValue)
			fragment, err := PrepareSchemaDetailBody(detail, schema, document, nodeValue, exampleValue)
			if err == nil || !reflect.DeepEqual(fragment, SchemaDetailBodyFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedSchemaDetailBodyCopiesChildrenAndBytes(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	fragment, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	node.data.Edges[0].Name = "changed"
	example.data.JSON = `{"changed":true}`
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared schema detail body changed after child mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPrepareSchemaDetailBodyFailsClosedOnMismatchedPreparedExample(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	schema.Example.JSON = `{"type":"string"}`
	fragment, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err == nil || !reflect.DeepEqual(fragment, SchemaDetailBodyFragment{}) {
		t.Fatalf("mismatched prepared example = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestPreparedSchemaDetailBodyWithoutExampleRendersOnlyNode(t *testing.T) {
	detail, schema, document, node, _ := schemaDetailBodyFixture(t)
	detail.Schema.ExampleSchemaJSON = ""
	schema.Example.JSON = ""
	node.binding, _ = bindSchemaDetailPreparation(detail, document.Key)
	fragment, err := PrepareSchemaDetailBody(detail, schema, document, &node, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nodeBytes, err := node.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nodeBytes = append(nodeBytes, ' ')
	if !bytes.Equal(body, nodeBytes) {
		t.Fatalf("schema detail body without example changed node bytes:\nbody=%s\nnode=%s", body, nodeBytes)
	}
}

func TestPreparedSchemaDetailBodyRejectsOversizedOutput(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	huge := strings.Repeat("x", maximumHTMLFragmentBytes)
	detail.Schema.ExampleSchemaJSON = huge
	schema.Example.JSON = huge
	node.binding, _ = bindSchemaDetailPreparation(detail, document.Key)
	example, err := PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err == nil || !reflect.DeepEqual(fragment, SchemaDetailBodyFragment{}) {
		t.Fatalf("oversized body = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestZeroSchemaDetailBodyFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (SchemaDetailBodyFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func schemaDetailBodyFixture(t *testing.T) (catalog.DetailRecordV1, domain.Schema, catalog.DocumentDirectoryV1, SchemaNodeFragment, SchemaDetailExampleFragment) {
	t.Helper()
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
	exampleJSON := `{"type":"object","description":"<pod>"}`
	detail := catalog.DetailRecordV1{
		ID: detailID, Kind: "schema",
		Schema: &projection.SchemaDetail{
			ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
			HeadingID: string(detailID), Heading: "Pod", HeadingLevel: 2, Description: "Description", SchemaRef: 7,
			ExampleSchemaJSON: exampleJSON,
		},
	}
	document := catalog.DocumentDirectoryV1{Key: "core-v1", APIVersion: "v1"}
	node, err := PrepareSchemaNode(detail, projection.SchemaNode{
		Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object",
		Properties: []projection.SchemaNodeProperty{{Ordinal: 0, ID: "property-name", Name: "name", SchemaRef: 8}},
	}, []projection.SchemaNode{{Ordinal: 8, ID: "node-name", Name: "Name", Type: "string"}}, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description, Example: domain.SchemaExample{JSON: exampleJSON}}
	example, err := PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	return detail, schema, document, node, example
}
