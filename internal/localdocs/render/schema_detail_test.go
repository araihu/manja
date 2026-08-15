package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/projection"
)

func TestPreparedSchemaDetailRendersArticleWithHeaderAndBody(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<article id="` + string(detail.ID) + `" data-catalog-detail="schema" class="space-y-6">`,
		`<header class="grid min-w-0 gap-4">`,
		`class="manja-schema-detail-layout"`,
		`class="manja-schema-example-panel"`,
	} {
		if !bytes.Contains(rendered, []byte(want)) {
			t.Errorf("prepared schema detail missing %q:\n%s", want, rendered)
		}
	}
}

func TestPrepareSchemaDetailFailsClosedOnInvalidOrMixedChildren(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	otherDetail := cloneDetail(detail)
	otherDocument := document
	otherDocument.Key = "other-v1"
	otherDetail.Schema.Href = "documents/other-v1/?selected=" + string(otherDetail.ID) + "#" + string(otherDetail.ID)
	otherNode, err := PrepareSchemaNode(otherDetail, projection.SchemaNode{Ordinal: 7, ID: "node-other", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/other-v1/")
	if err != nil {
		t.Fatal(err)
	}
	otherHeader, err := PrepareSchemaDetailHeader(otherDetail, schema, otherDocument, &otherNode)
	if err != nil {
		t.Fatal(err)
	}
	otherBody, err := PrepareSchemaDetailBody(otherDetail, schema, otherDocument, &otherNode, &example)
	if err == nil {
		t.Fatal("mixed example unexpectedly accepted")
	}
	_ = otherBody

	tests := []struct {
		name          string
		mutateHeader  func(*SchemaDetailHeaderFragment)
		mutateBody    func(*SchemaDetailBodyFragment)
		headerPresent bool
		bodyPresent   bool
	}{
		{name: "invalid header", mutateHeader: func(value *SchemaDetailHeaderFragment) { value.valid = false }, headerPresent: true, bodyPresent: true},
		{name: "invalid body", mutateBody: func(value *SchemaDetailBodyFragment) { value.valid = false }, headerPresent: true, bodyPresent: true},
		{name: "mixed header", mutateHeader: func(value *SchemaDetailHeaderFragment) { *value = otherHeader }, headerPresent: true, bodyPresent: true},
		{name: "missing header", headerPresent: false, bodyPresent: true},
		{name: "missing body", headerPresent: true, bodyPresent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			headerValue, bodyValue := header, body
			if test.mutateHeader != nil {
				test.mutateHeader(&headerValue)
			}
			if test.mutateBody != nil {
				test.mutateBody(&bodyValue)
			}
			var headerPtr *SchemaDetailHeaderFragment
			if test.headerPresent {
				headerPtr = &headerValue
			}
			var bodyPtr *SchemaDetailBodyFragment
			if test.bodyPresent {
				bodyPtr = &bodyValue
			}
			fragment, err := PrepareSchemaDetail(detail, schema, document, headerPtr, bodyPtr)
			if err == nil || !reflect.DeepEqual(fragment, SchemaDetailFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedSchemaDetailRejectsMixedBodyContext(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	otherDetail := cloneDetail(detail)
	otherDocument := document
	otherDocument.Key = "other-v1"
	otherDetail.Schema.Href = "documents/other-v1/?selected=" + string(otherDetail.ID) + "#" + string(otherDetail.ID)
	_ = example

	// Mutating a child binding is the direct fail-closed check for a body that
	// was prepared for another document while retaining otherwise valid bytes.
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	body.binding, err = bindSchemaDetailPreparation(otherDetail, otherDocument.Key)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err == nil || !reflect.DeepEqual(fragment, SchemaDetailFragment{}) {
		t.Fatalf("mixed body context = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestPreparedSchemaDetailCopiesChildrenAndBytes(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	header.data.Title = "changed"
	body.data.Node.data.Edges[0].Name = "changed"
	example.data.JSON = `{"changed":true}`
	got, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared schema detail changed after child mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPreparedSchemaDetailWithoutExamplePreservesBodySpacing(t *testing.T) {
	detail, schema, document, node, _ := schemaDetailBodyFixture(t)
	detail.Schema.ExampleSchemaJSON = ""
	schema.Example.JSON = ""
	node.binding, _ = bindSchemaDetailPreparation(detail, document.Key)
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, nil)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	nodeBytes, err := node.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rendered, append(nodeBytes, ' ')) {
		t.Fatalf("prepared schema detail lost legacy node separator:\n%s", rendered)
	}
}

func TestPreparedSchemaDetailRejectsOversizedCombinedOutput(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	detail.Schema.Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	schema.Description = detail.Schema.Description
	binding, _ := bindSchemaDetailPreparation(detail, document.Key)
	node.binding = binding
	example.binding = binding
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err == nil || !reflect.DeepEqual(fragment, SchemaDetailFragment{}) {
		t.Fatalf("oversized schema detail = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestZeroSchemaDetailFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (SchemaDetailFragment{}).Bytes(context.Background(), nil, nil)
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestPreparedSchemaDetailUsesSchemaIdentityAsArticleAnchor(t *testing.T) {
	detail, schema, document, node, example := schemaDetailBodyFixture(t)
	header, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareSchemaDetailBody(detail, schema, document, &node, &example)
	if err != nil {
		t.Fatal(err)
	}
	detail.Schema.Anchor = "changed"
	fragment, err := PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err == nil || !reflect.DeepEqual(fragment, SchemaDetailFragment{}) {
		t.Fatalf("changed article anchor = (%#v, %v), want zero fragment and error", fragment, err)
	}
}
