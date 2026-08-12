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

func TestPreparedOperationResponseDetailsRendersDescriptionAndHeaders(t *testing.T) {
	t.Parallel()

	detail, operation, nodes := operationResponseDetailFixture()
	fragment, err := PrepareOperationResponseDetails(detail, operation, nodes, "/documents/core-v1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.ResponseBytes(context.Background(), 0, operation.Anchor+"-201-headers", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-manja-response-section="description"`,
		`Created resource.`,
		`data-manja-response-section="headers"`,
		`aria-label="Response headers"`,
		`>Headers</h6>`,
		`data-schema-tree-row="X-Rate-Limit"`,
		`>X-Rate-Limit</span>`,
		`>array[array[string&lt;uuid&gt;]]</span>`,
		`>Header quota.</p>`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("response detail missing %q in %s", want, body)
		}
	}
}

func TestPrepareOperationResponseDetailsFailsClosedOnUnboundFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *[]projection.SchemaNode)
	}{
		{name: "response ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Responses[0].Ordinal = 1
		}},
		{name: "response status", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Status = "200"
		}},
		{name: "response description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Description = "Changed."
		}},
		{name: "header ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Responses[0].Headers[0].Ordinal = 1
		}},
		{name: "header location", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Responses[0].Headers[0].Name = "X-Rate Limit"
		}},
		{name: "header identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Responses[0].Headers[0].ID = "header"
		}},
		{name: "header name", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Headers[0].Name = "X-Changed"
		}},
		{name: "header description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Headers[0].Description = "Changed."
		}},
		{name: "header example", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Headers[0].Example = "changed"
		}},
		{name: "root schema field", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Headers[0].Schema.Type = "string"
		}},
		{name: "nested item schema field", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Responses[0].Headers[0].Schema.Items.Items.Format = "date-time"
		}},
		{name: "missing nested item", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1].Items = nil
		}},
		{name: "extra node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			*nodes = append(*nodes, projection.SchemaNode{Ordinal: 10, ID: "node-extra", Type: "string"})
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes := operationResponseDetailFixture()
			test.mutate(&detail, &operation, &nodes)
			fragment, err := PrepareOperationResponseDetails(detail, operation, nodes, "/documents/core-v1/", nil)
			if err == nil || !reflect.DeepEqual(fragment, OperationResponseDetailsFragment{}) {
				t.Fatalf("PrepareOperationResponseDetails() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPreparedOperationResponseDetailsCopiesInputsAndPreservesOrder(t *testing.T) {
	t.Parallel()

	detail, operation, nodes := operationResponseDetailFixture()
	detail.Operation.Responses = append(detail.Operation.Responses, projection.Response{Ordinal: 1, ID: "400", Status: "400", Description: "Bad request."})
	operation.Responses = append(operation.Responses, domain.OperationResponse{Status: "400", Description: "Bad request."})
	fragment, err := PrepareOperationResponseDetails(detail, operation, nodes, "/documents/core-v1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fragment.ResponseBytes(context.Background(), 0, operation.Anchor+"-201-headers", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.ResponseBytes(context.Background(), 1, operation.Anchor+"-400-headers", nil)
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Responses[0].Description = "Changed."
	operation.Responses[0].Headers[0].Name = "X-Changed"
	nodes[0].Type = "string"
	again, err := fragment.ResponseBytes(context.Background(), 0, operation.Anchor+"-201-headers", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, again) || !strings.Contains(string(first), "Created resource.") || !strings.Contains(string(second), "Bad request.") {
		t.Fatalf("prepared response details changed or order drifted: first=%s second=%s again=%s", first, second, again)
	}
}

func operationResponseDetailFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
	headerID := operationResponseHeaderID("X-Rate-Limit")
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
		Method: "POST", Path: "/api/v1/pods", Responses: []projection.Response{{
			Ordinal: 0, ID: "201", Status: "201", Description: "Created resource.", Headers: []projection.ResponseHeader{{
				Ordinal: 0, ID: headerID, Name: "X-Rate-Limit", Description: "Header quota.", SchemaRef: 7,
			}},
		}},
	}}
	operation := domain.Operation{Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/api/v1/pods", Responses: []domain.OperationResponse{{
		Status: "201", Description: "Created resource.", Headers: []domain.OperationResponseHeader{{
			Name: "X-Rate-Limit", Description: "Header quota.", Schema: domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "string", Format: "uuid"}}},
		}},
	}}}
	nodes := []projection.SchemaNode{
		{Ordinal: 7, ID: "node-header-root", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 8}}},
		{Ordinal: 8, ID: "node-header-array", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 9}}},
		{Ordinal: 9, ID: "node-header-value", Type: "string", Format: "uuid"},
	}
	return detail, operation, nodes
}
