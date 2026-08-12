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

func TestPreparedOperationResponseMediaRendersCanonicalSummary(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationResponseMediaFixture()
	fragment, err := PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.MediaBytes(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`>application/json<`,
		`href="/documents/core-v1/?selected=detail-sha256-cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc#detail-sha256-cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
		`hx-target="#catalog-main-content"`,
		`hx-select="#catalog-main-content"`,
		`hx-swap="outerHTML show:#main-content:top"`,
		`aria-label="Open schema Pod object"`,
		`>Pod object<`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("response media summary missing %q in %s", want, body)
		}
	}
}

func TestPrepareOperationResponseMediaFailsClosedOnInconsistentInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *[]projection.SchemaNode, *string, map[string]string)
	}{
		{name: "response inventory", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.Responses = nil
		}},
		{name: "response ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.Responses[0].Ordinal = 1
		}},
		{name: "response id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.Responses[0].ID = "created"
		}},
		{name: "response status", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.Responses[0].Status = "200"
		}},
		{name: "missing media", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.Responses[0].MediaTypes = nil
		}},
		{name: "media ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.Responses[0].MediaTypes[0].Ordinal = 1
		}},
		{name: "media id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.Responses[0].MediaTypes[0].ID = "json"
		}},
		{name: "media whitespace", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.Responses[0].MediaTypes[0].ID = " application/json"
			detail.Operation.Responses[0].MediaTypes[0].ContentType = " application/json"
			operation.Responses[0].MediaTypes[0].ContentType = " application/json"
		}},
		{name: "media example", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.Responses[0].MediaTypes[0].Examples[0].Text = `{"kind":"Other"}`
		}},
		{name: "missing node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ *string, _ map[string]string) {
			*nodes = nil
		}},
		{name: "extra node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ *string, _ map[string]string) {
			*nodes = append(*nodes, projection.SchemaNode{Ordinal: 8, ID: "node-extra", Type: "string"})
		}},
		{name: "schema summary", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.Responses[0].MediaTypes[0].Schema.Type = "string"
		}},
		{name: "schema href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, links map[string]string) {
			links["Pod"] = " " + links["Pod"]
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationResponseMediaFixture()
			test.mutate(&detail, &operation, &nodes, &documentHref, schemaLinks)
			fragment, err := PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err == nil || !reflect.DeepEqual(fragment, OperationResponseMediaFragment{}) {
				t.Fatalf("PrepareOperationResponseMedia() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPrepareOperationResponseMediaRequiresCompleteNestedItemsChain(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationResponseMediaNestedArrayFixture()
	fragment, err := PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.MediaBytes(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := `array[array[string&lt;uuid&gt;&lt;Status&gt;]]`; !strings.Contains(string(body), want) {
		t.Fatalf("nested array summary missing %q in %s", want, body)
	}

	operation.Responses[0].MediaTypes[0].Schema.Items.Items.Enum = []string{"changed"}
	if fragment, err := PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks); err == nil || !reflect.DeepEqual(fragment, OperationResponseMediaFragment{}) {
		t.Fatalf("unbound nested item mutation = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestPreparedOperationResponseMediaCopiesInputsPreservesResponseAndMediaOrder(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationResponseMediaFixture()
	detail.Operation.Responses = append(detail.Operation.Responses, projection.Response{Ordinal: 1, ID: "400", Status: "400", MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "text/plain", ContentType: "text/plain", SchemaRef: 8}}})
	operation.Responses = append(operation.Responses, domain.OperationResponse{Status: "400", MediaTypes: []domain.OperationMediaType{{ContentType: "text/plain", Schema: domain.SchemaSummary{Type: "string"}}}})
	nodes = append(nodes, projection.SchemaNode{Ordinal: 8, ID: "node-text", Type: "string"})
	fragment, err := PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fragment.MediaBytes(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.MediaBytes(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Responses[0].MediaTypes[0].ContentType = "text/plain"
	operation.Responses[0].MediaTypes[0].Schema.Name = "Changed"
	nodes[0].Name = "Changed"
	schemaLinks["Pod"] = "/changed"
	again, err := fragment.MediaBytes(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, again) || !strings.Contains(string(first), "application/json") || !strings.Contains(string(second), "text/plain") {
		t.Fatalf("prepared response media changed or order drifted: first=%s second=%s again=%s", first, second, again)
	}
}

func TestOperationResponseMediaRejectsInvalidIndexAndOversizedOutputWithoutBytes(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationResponseMediaFixture()
	fragment, err := PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	for _, indexes := range [][2]int{{-1, 0}, {0, -1}, {0, 1}, {1, 0}} {
		body, renderErr := fragment.MediaBytes(context.Background(), indexes[0], indexes[1])
		if renderErr == nil || len(body) != 0 {
			t.Fatalf("MediaBytes(%d, %d) = (%d bytes, %v), want zero bytes and error", indexes[0], indexes[1], len(body), renderErr)
		}
	}

	huge := strings.Repeat("x", maximumHTMLFragmentBytes)
	detail.Operation.Responses[0].MediaTypes[0].ID = huge
	detail.Operation.Responses[0].MediaTypes[0].ContentType = huge
	operation.Responses[0].MediaTypes[0].ContentType = huge
	fragment, err = PrepareOperationResponseMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, renderErr := fragment.MediaBytes(context.Background(), 0, 0)
	if renderErr == nil || len(body) != 0 {
		t.Fatalf("oversized MediaBytes() = (%d bytes, %v), want zero bytes and error", len(body), renderErr)
	}
}

func operationResponseMediaFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode, string, map[string]string) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
	schemaID := "detail-sha256-" + strings.Repeat("c", 64)
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
		Method: "POST", Path: "/api/v1/pods", Responses: []projection.Response{{
			Ordinal: 0, ID: "201", Status: "201", MediaTypes: []projection.MediaType{{
				Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7,
				Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: `{"kind":"Pod"}`, Provided: true}},
			}},
		}},
	}}
	operation := domain.Operation{
		Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/api/v1/pods", Responses: []domain.OperationResponse{{
			Status: "201", MediaTypes: []domain.OperationMediaType{{
				ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object"},
				Example: `{"kind":"Pod"}`, ExampleProvided: true,
			}},
		}},
	}
	nodes := []projection.SchemaNode{{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}}
	documentHref := "/documents/core-v1/"
	schemaLinks := map[string]string{"Pod": documentHref + "?selected=" + schemaID + "#" + schemaID}
	return detail, operation, nodes, documentHref, schemaLinks
}

func operationResponseMediaNestedArrayFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode, string, map[string]string) {
	detail, operation, _, documentHref, schemaLinks := operationResponseMediaFixture()
	operation.Responses[0].MediaTypes[0].Schema = domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}}}}
	nodes := []projection.SchemaNode{
		{Ordinal: 7, ID: "node-array-root", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 8}}},
		{Ordinal: 8, ID: "node-array-nested", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 9}}},
		{Ordinal: 9, ID: "node-status", Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}},
	}
	return detail, operation, nodes, documentHref, schemaLinks
}
