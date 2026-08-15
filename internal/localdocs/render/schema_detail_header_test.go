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

func TestPreparedSchemaDetailHeaderRendersCanonicalEscapedHTML(t *testing.T) {
	detail, schema, document, node := schemaDetailHeaderFixture(t)
	fragment, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<header class="grid min-w-0 gap-4">`,
		`Schema`,
		`v1`,
		`object`,
		`Pod &lt;schema&gt;`,
		`Description &lt;escaped&gt;`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("schema detail header missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte("<schema>")) || bytes.Contains(body, []byte("<escaped>")) {
		t.Fatalf("schema detail header leaked unescaped projection content: %s", body)
	}
}

func TestPrepareSchemaDetailHeaderFailsClosedOnInconsistentInputs(t *testing.T) {
	baseDetail, baseSchema, baseDocument, baseNode := schemaDetailHeaderFixture(t)
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Schema, *catalog.DocumentDirectoryV1, *SchemaNodeFragment)
	}{
		{name: "operation detail", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Schema, _ *catalog.DocumentDirectoryV1, _ *SchemaNodeFragment) {
			detail.Kind, detail.Schema = "operation", nil
		}},
		{name: "schema identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Schema, _ *catalog.DocumentDirectoryV1, _ *SchemaNodeFragment) {
			detail.Schema.ID = "changed"
		}},
		{name: "prepared schema", mutate: func(_ *catalog.DetailRecordV1, schema *domain.Schema, _ *catalog.DocumentDirectoryV1, _ *SchemaNodeFragment) {
			schema.Name = "changed"
		}},
		{name: "invalid version utf8", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Schema, document *catalog.DocumentDirectoryV1, _ *SchemaNodeFragment) {
			document.APIVersion = string([]byte{0xff})
		}},
		{name: "invalid node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Schema, _ *catalog.DocumentDirectoryV1, node *SchemaNodeFragment) {
			node.valid = false
		}},
		{name: "missing node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Schema, _ *catalog.DocumentDirectoryV1, node *SchemaNodeFragment) {
			*node = SchemaNodeFragment{}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := cloneDetail(baseDetail)
			schema := baseSchema
			document := baseDocument
			node := baseNode
			test.mutate(&detail, &schema, &document, &node)
			fragment, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
			if err == nil || !reflect.DeepEqual(fragment, SchemaDetailHeaderFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedSchemaDetailHeaderCopiesRenderInputs(t *testing.T) {
	detail, schema, document, node := schemaDetailHeaderFixture(t)
	fragment, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	detail.Schema.Heading = "changed"
	schema.Name = "changed"
	document.APIVersion = "changed"
	node.data.Type = "changed"
	got, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared schema detail header changed after input mutation\nwant=%s\n got=%s", want, got)
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

func TestPreparedSchemaDetailHeaderRejectsOversizedOutputWithoutPartialBytes(t *testing.T) {
	detail, schema, document, node := schemaDetailHeaderFixture(t)
	detail.Schema.Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	schema.Description = detail.Schema.Description
	fragment, err := PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background(), nil, nil)
	if err == nil || body != nil {
		t.Fatalf("oversized fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestZeroSchemaDetailHeaderFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (SchemaDetailHeaderFragment{}).Bytes(context.Background(), nil, nil)
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func schemaDetailHeaderFixture(t *testing.T) (catalog.DetailRecordV1, domain.Schema, catalog.DocumentDirectoryV1, SchemaNodeFragment) {
	t.Helper()
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
	detail := catalog.DetailRecordV1{
		ID: detailID, Kind: "schema",
		Schema: &projection.SchemaDetail{
			ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
			HeadingID: string(detailID), Heading: "Pod <schema>", HeadingLevel: 2, Description: "Description <escaped>", SchemaRef: 7,
		},
	}
	document := catalog.DocumentDirectoryV1{Key: "core-v1", APIVersion: "v1"}
	node := projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}
	nodeFragment, err := PrepareSchemaNode(detail, node, nil, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description}
	return detail, schema, document, nodeFragment
}
