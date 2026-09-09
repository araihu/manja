package render

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestPreparedSchemaNodeFragmentRendersCanonicalEscapedHTML(t *testing.T) {
	detail, node, references := schemaNodeFixture()

	fragment, err := PrepareSchemaNode(detail, node, references, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="schema-node-panel"`,
		`aria-label="Pod schema node"`,
		`data-catalog-schema-property="metadata"`,
		`href="/kubernetes/documents/core-v1/?selected=detail-sha256-cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc&amp;node=8#schema-node-panel"`,
		`&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("fragment missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "<script>alert") {
		t.Fatalf("fragment contains unescaped projection content: %s", body)
	}
}

func TestPrepareSchemaNodeFailsClosedOnInconsistentInputs(t *testing.T) {
	baseDetail, baseNode, baseReferences := schemaNodeFixture()
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *projection.SchemaNode, *[]projection.SchemaNode, *string)
	}{
		{name: "operation detail", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, _ *string) {
			detail.Kind, detail.Operation, detail.Schema = "operation", &projection.OperationDetail{}, nil
		}},
		{name: "detail id", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, _ *string) {
			detail.ID = domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
		}},
		{name: "schema anchor", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, _ *string) {
			detail.Schema.Anchor = "changed"
		}},
		{name: "schema heading id", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, _ *string) {
			detail.Schema.HeadingID = "changed"
		}},
		{name: "schema href", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, _ *string) {
			detail.Schema.Href = "?selected=changed"
		}},
		{name: "node id", mutate: func(_ *catalog.DetailRecordV1, node *projection.SchemaNode, _ *[]projection.SchemaNode, _ *string) {
			node.ID = ""
		}},
		{name: "duplicate reference ordinal", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, references *[]projection.SchemaNode, _ *string) {
			*references = append(*references, (*references)[0])
		}},
		{name: "missing reference", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, references *[]projection.SchemaNode, _ *string) {
			*references = (*references)[:1]
		}},
		{name: "extra reference", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, references *[]projection.SchemaNode, _ *string) {
			*references = append(*references, projection.SchemaNode{Ordinal: 10, ID: "node-extra", Name: "Extra"})
		}},
		{name: "reference id", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, references *[]projection.SchemaNode, _ *string) {
			(*references)[0].ID = ""
		}},
		{name: "relative document href", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = "kubernetes/documents/core-v1/"
		}},
		{name: "absolute document href", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = "https://attacker.example/core-v1/"
		}},
		{name: "traversal document href", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = "/kubernetes/../core-v1/"
		}},
		{name: "query document href", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = "/kubernetes/core-v1/?changed=1"
		}},
		{name: "backslash document href", mutate: func(_ *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = `/kubernetes\core-v1/`
		}},
		{name: "encoded traversal document href", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = "/kubernetes/documents/%2e%2e/"
			detail.Schema.Href = "documents/%2e%2e/?selected=" + string(detail.ID) + "#" + string(detail.ID)
		}},
		{name: "invalid document key", mutate: func(detail *catalog.DetailRecordV1, _ *projection.SchemaNode, _ *[]projection.SchemaNode, href *string) {
			*href = "/kubernetes/documents/Core-V1/"
			detail.Schema.Href = "documents/Core-V1/?selected=" + string(detail.ID) + "#" + string(detail.ID)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := cloneDetail(baseDetail)
			node := cloneNode(baseNode)
			references := cloneNodes(baseReferences)
			href := "/kubernetes/documents/core-v1/"
			test.mutate(&detail, &node, &references, &href)

			fragment, err := PrepareSchemaNode(detail, node, references, href)
			if err == nil || !reflect.DeepEqual(fragment, SchemaNodeFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedSchemaNodeRequiresExactReferenceCoverageButAcceptsSelfReference(t *testing.T) {
	detail, node, _ := schemaNodeFixture()
	node.Properties = []projection.SchemaNodeProperty{{Ordinal: 0, ID: "self", Name: "self", SchemaRef: projection.SchemaRef(node.Ordinal)}}

	fragment, err := PrepareSchemaNode(detail, node, nil, "/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `data-catalog-schema-property="self"`) || !strings.Contains(string(body), `node=7#schema-node-panel`) {
		t.Fatalf("self reference fragment invalid: %s", body)
	}
}

func TestPreparedSchemaNodeCopiesInputsAndRendersDeterministically(t *testing.T) {
	detail, node, references := schemaNodeFixture()
	fragment, err := PrepareSchemaNode(detail, node, references, "/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	detail.Schema.Heading = "changed"
	node.Name = "changed"
	node.Properties[0].Name = "changed"
	references[0].Name = "changed"
	references[1].Enum[0] = "changed"

	first, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || strings.Contains(string(first), "changed") || !strings.Contains(string(first), "ObjectMeta") || !strings.Contains(string(first), "Pending") {
		t.Fatalf("fragment aliases inputs or is nondeterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	first[0] = 'x'
	third, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second, third) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPreparedSchemaNodeRejectsFragmentAboveTwoMiBWithoutPartialBytes(t *testing.T) {
	detail, node, references := schemaNodeFixture()
	references[1].Enum = []string{strings.Repeat("x", maximumHTMLFragmentBytes)}
	fragment, err := PrepareSchemaNode(detail, node, references, "/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("oversized fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestZeroSchemaNodeFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (SchemaNodeFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func schemaNodeFixture() (catalog.DetailRecordV1, projection.SchemaNode, []projection.SchemaNode) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("c", 64))
	detail := catalog.DetailRecordV1{
		ID: detailID, Kind: "schema",
		Schema: &projection.SchemaDetail{
			ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
			HeadingID: string(detailID), Heading: "Pod schema", HeadingLevel: 2, SchemaRef: 7,
		},
	}
	node := projection.SchemaNode{
		Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object",
		Description: "<script>alert('x')</script>",
		Properties: []projection.SchemaNodeProperty{
			{Ordinal: 0, ID: "property-metadata", Name: "metadata", Required: true, SchemaRef: 8},
			{Ordinal: 1, ID: "property-phase", Name: "phase", SchemaRef: 9},
		},
	}
	references := []projection.SchemaNode{
		{Ordinal: 8, ID: "node-metadata", Name: "ObjectMeta", Type: "object"},
		{Ordinal: 9, ID: "node-phase", Name: "Phase", Type: "string", Enum: []string{"Pending", "Running"}},
	}
	return detail, node, references
}

func cloneDetail(value catalog.DetailRecordV1) catalog.DetailRecordV1 {
	clone := value
	if value.Schema != nil {
		schema := *value.Schema
		clone.Schema = &schema
	}
	if value.Operation != nil {
		operation := *value.Operation
		clone.Operation = &operation
	}
	return clone
}

func cloneNode(value projection.SchemaNode) projection.SchemaNode {
	clone := value
	clone.Enum = append([]string(nil), value.Enum...)
	clone.Constraints = append([]projection.SchemaConstraint(nil), value.Constraints...)
	clone.Properties = append([]projection.SchemaNodeProperty(nil), value.Properties...)
	clone.Items = append([]projection.SchemaNodeItem(nil), value.Items...)
	return clone
}

func cloneNodes(values []projection.SchemaNode) []projection.SchemaNode {
	result := make([]projection.SchemaNode, len(values))
	for index := range values {
		result[index] = cloneNode(values[index])
	}
	return result
}
