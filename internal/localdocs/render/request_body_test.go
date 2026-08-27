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

func TestPreparedOperationRequestBodyMediaRendersCanonicalSummary(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.MediaBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`application/json`,
		`href="/documents/core-v1/?selected=detail-sha256-cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc#detail-sha256-cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
		`hx-target="#catalog-main-content"`,
		`hx-select="#catalog-main-content"`,
		`hx-swap="outerHTML show:#main-content:top"`,
		`aria-label="Open schema Pod object"`,
		`focus-visible:outline-primary`,
		`>Pod object<`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request-body media summary missing %q in %s", want, body)
		}
	}
}

func TestPrepareOperationRequestBodyMediaFailsClosedOnInconsistentInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *[]projection.SchemaNode, *string, map[string]string)
	}{
		{name: "request-body declaration", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.HasRequestBody = false
		}},
		{name: "prepared body", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.RequestBody = nil
		}},
		{name: "description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.RequestBody.Description = "different"
		}},
		{name: "description utf8", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.Description = "\xff"
			operation.RequestBody.Description = "\xff"
		}},
		{name: "required", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.RequestBody.Required = false
		}},
		{name: "missing media", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			operation.RequestBody.MediaTypes = nil
		}},
		{name: "media ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].Ordinal = 1
		}},
		{name: "media id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].ID = "json"
		}},
		{name: "duplicate media", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			duplicate := detail.Operation.RequestBody.MediaTypes[0]
			duplicate.Ordinal = 1
			detail.Operation.RequestBody.MediaTypes = append(detail.Operation.RequestBody.MediaTypes, duplicate)
			operation.RequestBody.MediaTypes = append(operation.RequestBody.MediaTypes, operation.RequestBody.MediaTypes[0])
		}},
		{name: "media whitespace", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].ID = " application/json"
			detail.Operation.RequestBody.MediaTypes[0].ContentType = " application/json"
			operation.RequestBody.MediaTypes[0].ContentType = " application/json"
		}},
		{name: "media example text", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].Examples[0].Text = `{"kind":"Other"}`
		}},
		{name: "media example identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].Examples[0].ID = "other"
		}},
		{name: "media extra example", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, _ map[string]string) {
			detail.Operation.RequestBody.MediaTypes[0].Examples = append(detail.Operation.RequestBody.MediaTypes[0].Examples, projection.Example{Ordinal: 1, ID: "other", Text: "other", Provided: true})
		}},
		{name: "missing node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ *string, _ map[string]string) {
			*nodes = nil
		}},
		{name: "extra node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ *string, _ map[string]string) {
			*nodes = append(*nodes, projection.SchemaNode{Ordinal: 8, ID: "node-extra", Type: "string"})
		}},
		{name: "duplicate node ordinal", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode, _ *string, _ map[string]string) {
			*nodes = append(*nodes, (*nodes)[0])
		}},
		{name: "schema enum", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, nodes *[]projection.SchemaNode, _ *string, _ map[string]string) {
			(*nodes)[0].Enum = []string{"Pod"}
			operation.RequestBody.MediaTypes[0].Schema.Enum = []string{"Other"}
		}},
		{name: "schema href whitespace", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, links map[string]string) {
			links["Pod"] = " " + links["Pod"]
		}},
		{name: "schema href cross-document", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, links map[string]string) {
			links["Pod"] = strings.Replace(links["Pod"], "/documents/core-v1/", "/documents/other/", 1)
		}},
		{name: "schema href fragment", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, links map[string]string) {
			links["Pod"] += "-other"
		}},
		{name: "schema href extra query", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, _ *string, links map[string]string) {
			links["Pod"] = strings.Replace(links["Pod"], "#", "&other=value#", 1)
		}},
		{name: "document href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode, href *string, _ map[string]string) {
			*href = "/documents/../core-v1/"
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
			test.mutate(&detail, &operation, &nodes, &documentHref, schemaLinks)
			fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err == nil || !reflect.DeepEqual(fragment, OperationRequestBodyMediaFragment{}) {
				t.Fatalf("PrepareOperationRequestBodyMedia() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPrepareOperationRequestBodyMediaRequiresCompleteNestedItemsChain(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyNestedArrayFixture()
	fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.MediaBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := `array[array[string&lt;uuid&gt;&lt;Status&gt;]]`; !strings.Contains(string(body), want) {
		t.Fatalf("nested array summary missing %q in %s", want, body)
	}
}

func TestPrepareOperationRequestBodyMediaRejectsNestedItemsDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantField string
		mutate    func(*domain.Operation, *[]projection.SchemaNode)
	}{
		{name: "item type", wantField: "schema summary", mutate: func(operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.RequestBody.MediaTypes[0].Schema.Items.Items.Type = "integer"
		}},
		{name: "item name", wantField: "schema summary", mutate: func(operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.RequestBody.MediaTypes[0].Schema.Items.Items.Name = "Phase"
		}},
		{name: "item format", wantField: "schema summary", mutate: func(operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.RequestBody.MediaTypes[0].Schema.Items.Items.Format = "date-time"
		}},
		{name: "item enum", wantField: "schema summary", mutate: func(operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.RequestBody.MediaTypes[0].Schema.Items.Items.Enum = []string{"ready", "failed"}
		}},
		{name: "missing item node", wantField: "missing schema node", mutate: func(_ *domain.Operation, nodes *[]projection.SchemaNode) {
			*nodes = (*nodes)[:2]
		}},
		{name: "missing item edge", wantField: "schema summary", mutate: func(_ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1].Items = nil
		}},
		{name: "missing prepared item", wantField: "schema summary", mutate: func(operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.RequestBody.MediaTypes[0].Schema.Items.Items = nil
		}},
		{name: "extra prepared item", wantField: "schema summary", mutate: func(operation *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[2].Items = nil
			operation.RequestBody.MediaTypes[0].Schema.Items.Items.Items = &domain.SchemaSummary{Type: "boolean"}
		}},
		{name: "extra item node", wantField: "schema-node inventory", mutate: func(_ *domain.Operation, nodes *[]projection.SchemaNode) {
			*nodes = append(*nodes, projection.SchemaNode{Ordinal: 10, ID: "node-unused", Type: "string"})
		}},
		{name: "extra item edge", wantField: "schema-node inventory", mutate: func(_ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1].Items = append((*nodes)[1].Items, projection.SchemaNodeItem{Ordinal: 1, ID: "items-extra", SchemaRef: 10})
		}},
		{name: "reordered item nodes", wantField: "schema-node inventory", mutate: func(_ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1], (*nodes)[2] = (*nodes)[2], (*nodes)[1]
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyNestedArrayFixture()
			test.mutate(&operation, &nodes)
			fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err == nil || !reflect.DeepEqual(fragment, OperationRequestBodyMediaFragment{}) {
				t.Fatalf("PrepareOperationRequestBodyMedia() = (%#v, %v), want zero fragment and error", fragment, err)
			}
			if !strings.Contains(err.Error(), test.wantField) {
				t.Fatalf("PrepareOperationRequestBodyMedia() error = %q, want field %q", err, test.wantField)
			}
		})
	}
}

func TestPrepareOperationRequestBodyMediaRejectsUnboundPreparedItems(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"type", "name", "format", "enum"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyNestedArrayFixture()
			nodes = nodes[:1]
			switch field {
			case "type":
				operation.RequestBody.MediaTypes[0].Schema.Items.Type = "integer"
			case "name":
				operation.RequestBody.MediaTypes[0].Schema.Items.Name = "Changed"
			case "format":
				operation.RequestBody.MediaTypes[0].Schema.Items.Format = "uuid"
			case "enum":
				operation.RequestBody.MediaTypes[0].Schema.Items.Enum = []string{"changed"}
			}
			fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err == nil || !reflect.DeepEqual(fragment, OperationRequestBodyMediaFragment{}) {
				t.Fatalf("PrepareOperationRequestBodyMedia() = (%#v, %v), want zero fragment and error", fragment, err)
			}
			if !strings.Contains(err.Error(), "missing schema node") {
				t.Fatalf("PrepareOperationRequestBodyMedia() error = %q, want missing schema node", err)
			}
		})
	}
}

func TestPreparedOperationRequestBodyMediaCopiesInputsAndPreservesOrder(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	detail.Operation.RequestBody.MediaTypes = append(detail.Operation.RequestBody.MediaTypes, projection.MediaType{
		Ordinal: 1, ID: "application/yaml", ContentType: "application/yaml", SchemaRef: 8,
	})
	operation.RequestBody.MediaTypes = append(operation.RequestBody.MediaTypes, domain.OperationMediaType{
		ContentType: "application/yaml", Schema: domain.SchemaSummary{Type: "string"},
	})
	nodes = append(nodes, projection.SchemaNode{Ordinal: 8, ID: "node-yaml", Type: "string"})
	fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fragment.MediaBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.MediaBytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.RequestBody.MediaTypes[0].ContentType = "text/plain"
	operation.RequestBody.MediaTypes[0].ContentType = "text/plain"
	nodes[0].Name = "Changed"
	schemaLinks["Pod"] = "/changed"
	again, err := fragment.MediaBytes(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("prepared bytes changed after input mutation:\nfirst=%s\nagain=%s", first, again)
	}
	if !strings.Contains(string(first), "application/json") || !strings.Contains(string(second), "application/yaml") {
		t.Fatalf("media order changed: first=%s second=%s", first, second)
	}
	if strings.Contains(string(second), "href=") {
		t.Fatalf("primitive media summary unexpectedly linked: %s", second)
	}
}

func TestOperationRequestBodyMediaRejectsInvalidIndexAndOversizedOutputWithoutBytes(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	fragment, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{-1, 1} {
		body, renderErr := fragment.MediaBytes(context.Background(), index)
		if renderErr == nil || len(body) != 0 {
			t.Fatalf("MediaBytes(%d) = (%d bytes, %v), want zero bytes and error", index, len(body), renderErr)
		}
	}

	detail, operation, nodes, documentHref, schemaLinks = operationRequestBodyMediaFixture()
	huge := strings.Repeat("x", maximumHTMLFragmentBytes)
	detail.Operation.RequestBody.MediaTypes[0].ID = huge
	detail.Operation.RequestBody.MediaTypes[0].ContentType = huge
	operation.RequestBody.MediaTypes[0].ContentType = huge
	fragment, err = PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, renderErr := fragment.MediaBytes(context.Background(), 0)
	if renderErr == nil || len(body) != 0 {
		t.Fatalf("oversized MediaBytes() = (%d bytes, %v), want zero bytes and error", len(body), renderErr)
	}
}

func operationRequestBodyMediaFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode, string, map[string]string) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
	schemaID := "detail-sha256-" + strings.Repeat("c", 64)
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
		Method: "POST", Path: "/api/v1/pods", HasRequestBody: true,
		RequestBody: projection.RequestBody{
			Description: "Pod to create.", Required: true,
			MediaTypes: []projection.MediaType{{
				Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7,
				Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: `{"kind":"Pod"}`, Provided: true}},
			}},
		},
	}}
	operation := domain.Operation{
		Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/api/v1/pods",
		RequestBody: &domain.OperationRequestBody{
			Description: "Pod to create.", Required: true,
			MediaTypes: []domain.OperationMediaType{{
				ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object"},
				Example: `{"kind":"Pod"}`, ExampleProvided: true,
			}},
		},
	}
	nodes := []projection.SchemaNode{{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}}
	documentHref := "/documents/core-v1/"
	schemaLinks := map[string]string{"Pod": documentHref + "?selected=" + schemaID + "#" + schemaID}
	return detail, operation, nodes, documentHref, schemaLinks
}

func operationRequestBodyNestedArrayFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode, string, map[string]string) {
	detail, operation, _, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	detail.Operation.RequestBody.MediaTypes[0].SchemaRef = 7
	operation.RequestBody.MediaTypes[0].Schema = domain.SchemaSummary{
		Type: "array",
		Items: &domain.SchemaSummary{
			Type: "array",
			Items: &domain.SchemaSummary{
				Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"},
			},
		},
	}
	nodes := []projection.SchemaNode{
		{Ordinal: 7, ID: "node-array-root", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 8}}},
		{Ordinal: 8, ID: "node-array-nested", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 9}}},
		{Ordinal: 9, ID: "node-status", Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}},
	}
	return detail, operation, nodes, documentHref, schemaLinks
}
