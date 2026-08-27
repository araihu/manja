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

func TestPreparedOperationSchemaTreesRenderBoundRecursiveData(t *testing.T) {
	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	fragment, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}

	request, err := fragment.RequestBodyBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="detail-sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-request-body-application-json-schema"`,
		`aria-label="Request body schema for application/json schema tree"`,
		`data-schema-tree-row="kind"`, `data-schema-tree-row="phase"`,
		`data-required="true">required`, `data-required="false">optional`,
		`Child &lt;description&gt;.`, `Default:</span> <code>Pod`, `Example:</span> <code>pod-1`,
		`Allowed:</span> <code>Pod</code><code>Service`, `minLength:</span><code>1`,
		`data-manja-schema-enum-reference="true"`, `hx-target="#catalog-main-content"`,
		`hx-select="#catalog-main-content"`, `hx-swap="outerHTML show:#main-content:top"`,
	} {
		if !bytes.Contains(request, []byte(want)) {
			t.Errorf("request schema tree missing %q:\n%s", want, request)
		}
	}

	response, err := fragment.ResponseBytes(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="detail-sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-response-201-application-json-schema"`,
		`aria-label="Response body schema tree"`, `data-schema-tree-row="items"`,
		`data-schema-tree-row="name"`, `nullable`, `data-deprecated="true">deprecated`,
	} {
		if !bytes.Contains(response, []byte(want)) {
			t.Errorf("response schema tree missing %q:\n%s", want, response)
		}
	}
}

func TestPrepareOperationSchemaTreesFailsClosedOnUnboundInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *[]projection.SchemaNode, map[string]string)
	}{
		{name: "request media identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].ID = "text/plain"
		}},
		{name: "duplicate request media", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			media := detail.Operation.RequestBody.MediaTypes[0]
			media.Ordinal = 1
			detail.Operation.RequestBody.MediaTypes = append(detail.Operation.RequestBody.MediaTypes, media)
			operation.RequestBody.MediaTypes = append(operation.RequestBody.MediaTypes, operation.RequestBody.MediaTypes[0])
		}},
		{name: "response identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			detail.Operation.Responses[0].ID = "200"
		}},
		{name: "response media order", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			detail.Operation.Responses[0].MediaTypes[0].Ordinal = 1
		}},
		{name: "schema ref", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].SchemaRef = 99
		}},
		{name: "schema node id", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			(*nodes)[0].ID = ""
		}},
		{name: "duplicate schema node id", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			(*nodes)[1].ID = (*nodes)[0].ID
		}},
		{name: "property id", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			(*nodes)[0].Properties[0].ID = ""
		}},
		{name: "property order", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			(*nodes)[0].Properties[0].Ordinal = 1
		}},
		{name: "item id", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			(*nodes)[3].Items[0].ID = ""
		}},
		{name: "missing node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			*nodes = (*nodes)[:len(*nodes)-1]
		}},
		{name: "extra node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			*nodes = append(*nodes, projection.SchemaNode{Ordinal: 99, ID: "node-extra", Type: "string"})
		}},
		{name: "name", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Name = "Other"
		}},
		{name: "type", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Type = "string"
		}},
		{name: "format", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Format = "date"
		}},
		{name: "description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Description = "changed"
		}},
		{name: "default", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Default = "changed"
		}},
		{name: "example", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Example = "changed"
		}},
		{name: "enum", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Enum[0] = "changed"
		}},
		{name: "constraint", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Constraints[0].Value = "2"
		}},
		{name: "nullable", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.Responses[0].MediaTypes[0].Schema.Items.Nullable = false
		}},
		{name: "deprecated", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.Responses[0].MediaTypes[0].Schema.Items.Deprecated = false
		}},
		{name: "property name", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Name = "other"
		}},
		{name: "property required", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Required = false
		}},
		{name: "property description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Description = "changed"
		}},
		{name: "items", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.Responses[0].MediaTypes[0].Schema.Items = nil
		}},
		{name: "json", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ map[string]string) {
			operation.RequestBody.MediaTypes[0].Schema.JSON = "changed"
		}},
		{name: "invalid link", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, links map[string]string) {
			links["Phase"] = "https://example.com/schema"
		}},
		{name: "bounded output", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, nodes *[]projection.SchemaNode, _ map[string]string) {
			huge := strings.Repeat("x", maximumHTMLFragmentBytes)
			operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Example = huge
			operation.Responses[0].MediaTypes[0].Schema.Items.Properties[0].Schema.Example = huge
			(*nodes)[1].ExampleText = huge
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
			test.mutate(&detail, &operation, &nodes, schemaLinks)
			fragment, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
			if err == nil || !reflect.DeepEqual(fragment, OperationSchemaTreesFragment{}) {
				t.Fatalf("PrepareOperationSchemaTrees() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPreparedOperationSchemaTreesCopyInputs(t *testing.T) {
	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	fragment, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	before, err := fragment.RequestBodyBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	operation.RequestBody.MediaTypes[0].Schema.Properties[0].Schema.Enum[0] = "mutated"
	nodes[1].Enum[0] = "mutated"
	nodes[0].Properties[0].Name = "mutated"
	schemaLinks["Phase"] = "/mutated/"
	after, err := fragment.RequestBodyBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("prepared fragment aliases inputs:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestPrepareOperationSchemaTreesAdmitsBoundProjectionCycle(t *testing.T) {
	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	operation.Responses[0].MediaTypes[0].Schema.Items = &domain.SchemaSummary{Name: "EnvelopeList", Type: "array", JSON: `{"type":"array"}`}
	nodes[3].Items[0].SchemaRef = 10
	nodes = nodes[:4]

	fragment, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.ResponseBytes(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`EnvelopeList array[EnvelopeList array]`)) {
		t.Fatalf("bound recursive schema label missing from %s", body)
	}
}

func TestZeroOperationSchemaTreesFragmentFailsWithoutBytes(t *testing.T) {
	fragment := OperationSchemaTreesFragment{}
	if body, err := fragment.RequestBodyBytes(context.Background(), 0); err == nil || len(body) != 0 {
		t.Fatalf("zero request tree = (%q, %v), want no bytes and error", body, err)
	}
	if body, err := fragment.ResponseBytes(context.Background(), 0, 0); err == nil || len(body) != 0 {
		t.Fatalf("zero response tree = (%q, %v), want no bytes and error", body, err)
	}
}

func operationSchemaTreeFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode, string, map[string]string) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	phaseID := "detail-sha256-" + strings.Repeat("b", 64)
	documentHref := "/kubernetes/documents/core-v1/"
	kind := domain.SchemaSummary{
		Type: "string", Format: "uuid", Description: "Child <description>.", Default: "Pod", Example: "pod-1",
		Enum: []string{"Pod", "Service"}, Constraints: []domain.SchemaConstraint{{Name: "minLength", Value: "1"}}, JSON: `{"type":"string"}`,
	}
	phase := domain.SchemaSummary{Name: "Phase", Type: "string", Description: "Lifecycle phase.", Enum: []string{"Ready", "Pending"}, JSON: `{"type":"string"}`}
	requestSchema := domain.SchemaSummary{
		Name: "Pod", Type: "object", Description: "Pod root.", Default: `{}`, Example: `{"kind":"Pod"}`,
		Constraints: []domain.SchemaConstraint{{Name: "maxProperties", Value: "8"}}, JSON: `{"type":"object"}`,
		Properties: []domain.SchemaProperty{
			{Name: "kind", Required: true, Description: "Kind override.", Schema: kind},
			{Name: "phase", Required: false, Description: "Lifecycle phase.", Schema: phase},
		},
	}
	responseItem := domain.SchemaSummary{
		Name: "Envelope", Type: "object", Description: "Response item.", Nullable: true, Deprecated: true, JSON: `{"type":"object"}`,
		Properties: []domain.SchemaProperty{{Name: "name", Required: true, Description: "Child <description>.", Schema: kind}},
	}
	responseSchema := domain.SchemaSummary{Name: "EnvelopeList", Type: "array", JSON: `{"type":"array"}`, Items: &responseItem}
	operation := domain.Operation{
		Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/pods",
		RequestBody: &domain.OperationRequestBody{Description: "Pod to create.", Required: true, MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: requestSchema}}},
		Responses:   []domain.OperationResponse{{Status: "201", Description: "Created.", MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: responseSchema}}}},
	}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2, Method: "POST", Path: "/pods", HasRequestBody: true,
		RequestBody: projection.RequestBody{Description: "Pod to create.", Required: true, MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7}}},
		Responses:   []projection.Response{{Ordinal: 0, ID: "201", Status: "201", Description: "Created.", MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 10}}}},
	}}
	nodes := []projection.SchemaNode{
		{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object", Description: "Pod root.", DefaultValue: `{}`, ExampleText: `{"kind":"Pod"}`, Constraints: []projection.SchemaConstraint{{Name: "maxProperties", Value: "8"}}, JSON: `{"type":"object"}`, Properties: []projection.SchemaNodeProperty{
			{Ordinal: 0, ID: "property-kind", Name: "kind", Required: true, Description: "Kind override.", SchemaRef: 8},
			{Ordinal: 1, ID: "property-phase", Name: "phase", Description: "", SchemaRef: 9},
		}},
		{Ordinal: 8, ID: "node-kind", Type: "string", Format: "uuid", Description: "Child <description>.", DefaultValue: "Pod", ExampleText: "pod-1", Enum: []string{"Pod", "Service"}, Constraints: []projection.SchemaConstraint{{Name: "minLength", Value: "1"}}, JSON: `{"type":"string"}`},
		{Ordinal: 9, ID: "node-phase", Name: "Phase", Type: "string", Description: "Lifecycle phase.", Enum: []string{"Ready", "Pending"}, JSON: `{"type":"string"}`},
		{Ordinal: 10, ID: "node-envelope-list", Name: "EnvelopeList", Type: "array", JSON: `{"type":"array"}`, Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 11}}},
		{Ordinal: 11, ID: "node-envelope", Name: "Envelope", Type: "object", Description: "Response item.", Nullable: true, Deprecated: true, JSON: `{"type":"object"}`, Properties: []projection.SchemaNodeProperty{{Ordinal: 0, ID: "property-name", Name: "name", Required: true, SchemaRef: 8}}},
	}
	schemaLinks := map[string]string{"Phase": documentHref + "?selected=" + phaseID + "#" + phaseID}
	return detail, operation, nodes, documentHref, schemaLinks
}
