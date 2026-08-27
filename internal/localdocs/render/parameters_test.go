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

func TestPreparedOperationParametersRenderCanonicalEscapedHTML(t *testing.T) {
	detail, operation, nodes := operationParametersFixture()
	fragment, err := PrepareOperationParameters(detail, operation, nodes)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`aria-labelledby="detail-sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-path-parameters-heading"`,
		`data-manja-parameter-list`,
		`data-required="true"`,
		`data-required="false"`,
		`&lt;namespace&gt;`,
		`array[string]`,
		`Header Parameters`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("parameter fragment missing %q: %s", want, body)
		}
	}
	if strings.Contains(string(body), `<namespace>`) || strings.Contains(string(body), "Cookie Parameters") {
		t.Fatalf("parameter fragment emitted unsafe or unsupported content: %s", body)
	}
}

func TestPrepareOperationParametersFailsClosedOnInconsistentInputs(t *testing.T) {
	baseDetail, baseOperation, baseNodes := operationParametersFixture()
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *[]projection.SchemaNode)
	}{
		{name: "wrong detail kind", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Kind = "schema"
		}},
		{name: "missing detail operation", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation = nil
		}},
		{name: "detail anchor", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Anchor = "changed"
		}},
		{name: "prepared anchor", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Anchor = "changed"
		}},
		{name: "missing parameter", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters = operation.Parameters[:len(operation.Parameters)-1]
		}},
		{name: "extra parameter", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters = append(operation.Parameters, operation.Parameters[0])
		}},
		{name: "reordered parameters", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0], operation.Parameters[1] = operation.Parameters[1], operation.Parameters[0]
		}},
		{name: "parameter ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Parameters[0].Ordinal = 1
		}},
		{name: "parameter id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *[]projection.SchemaNode) {
			detail.Operation.Parameters[0].ID = "parameter-changed"
		}},
		{name: "parameter location", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0].In = "query"
		}},
		{name: "parameter name", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0].Name = "changed"
		}},
		{name: "parameter required", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0].Required = false
		}},
		{name: "parameter description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0].Description = "changed"
		}},
		{name: "parameter example", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0].Example = "changed"
		}},
		{name: "schema summary", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *[]projection.SchemaNode) {
			operation.Parameters[0].Schema.Type = "integer"
		}},
		{name: "missing schema node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			*nodes = (*nodes)[:len(*nodes)-1]
		}},
		{name: "extra schema node", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			*nodes = append(*nodes, projection.SchemaNode{Ordinal: 9, ID: "node-unused", Type: "string"})
		}},
		{name: "duplicate schema ordinal", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1].Ordinal = (*nodes)[0].Ordinal
		}},
		{name: "duplicate schema id", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1].ID = (*nodes)[0].ID
		}},
		{name: "schema node field", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[0].Type = "integer"
		}},
		{name: "array item reference", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, nodes *[]projection.SchemaNode) {
			(*nodes)[1].Items[0].SchemaRef = 4
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := cloneParameterDetail(baseDetail)
			operation := cloneParameterOperation(baseOperation)
			nodes := cloneNodes(baseNodes)
			test.mutate(&detail, &operation, &nodes)
			fragment, err := PrepareOperationParameters(detail, operation, nodes)
			if err == nil || !reflect.DeepEqual(fragment, OperationParametersFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedOperationParametersCopiesInputsAndRendersDeterministically(t *testing.T) {
	detail, operation, nodes := operationParametersFixture()
	fragment, err := PrepareOperationParameters(detail, operation, nodes)
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Parameters[0].Name = "changed"
	operation.Parameters[0].Name = "changed"
	nodes[0].Type = "changed"
	first, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || bytes.Contains(first, []byte("changed")) {
		t.Fatalf("parameter fragment aliases input or is nondeterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	first[0] = 'x'
	third, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, third) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPreparedOperationParametersPreserveDisclosureFocusMarkup(t *testing.T) {
	detail, operation, nodes := operationParametersFixture()
	long := strings.Repeat("description ", 30)
	detail.Operation.Parameters[0].Description = long
	operation.Parameters[0].Description = long
	fragment, err := PrepareOperationParameters(detail, operation, nodes)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`x-data="{ expanded: false }"`, `x-bind:aria-expanded="expanded.toString()"`, `focus-visible:outline-2`, `aria-controls="detail-sha256-`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("disclosure contract missing %q: %s", want, body)
		}
	}
}

func TestPreparedOperationParametersRejectOutputAboveTwoMiBWithoutPartialBytes(t *testing.T) {
	detail, operation, nodes := operationParametersFixture()
	detail.Operation.Parameters[0].Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	operation.Parameters[0].Description = detail.Operation.Parameters[0].Description
	fragment, err := PrepareOperationParameters(detail, operation, nodes)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("oversized fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestZeroOperationParametersFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (OperationParametersFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestParameterSchemaInlineMatchesExistingSSRLabels(t *testing.T) {
	tests := []struct {
		name   string
		schema domain.SchemaSummary
		want   string
	}{
		{name: "plain", schema: domain.SchemaSummary{Name: "Pod", Type: "object"}, want: "Pod object"},
		{name: "formatted", schema: domain.SchemaSummary{Type: "string", Format: "uuid"}, want: "string<uuid>"},
		{name: "array", schema: domain.SchemaSummary{Type: "array", Items: schemaSummaryPointer(domain.SchemaSummary{Type: "string"})}, want: "array[string]"},
		{name: "named primitive enum", schema: domain.SchemaSummary{Name: "Phase", Type: "string", Enum: []string{"Ready"}}, want: "string<Phase>"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parameterSchemaInline(test.schema); got != test.want {
				t.Fatalf("parameterSchemaInline() = %q, want %q", got, test.want)
			}
		})
	}
}

func operationParametersFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	projected := []projection.Parameter{
		{Ordinal: 0, ID: operationParameterRecordID("path", "<namespace>"), Name: "<namespace>", In: "path", Required: true, Description: "Namespace <name>.", SchemaRef: 0},
		{Ordinal: 1, ID: operationParameterRecordID("query", "labels"), Name: "labels", In: "query", Description: "Label selector.", SchemaRef: 1},
		{Ordinal: 2, ID: operationParameterRecordID("header", "X-Trace"), Name: "X-Trace", In: "header", Description: "Trace ID.", SchemaRef: 3},
		{Ordinal: 3, ID: operationParameterRecordID("cookie", "session"), Name: "session", In: "cookie", SchemaRef: 4},
	}
	detail := catalog.DetailRecordV1{
		ID:   detailID,
		Kind: "operation",
		Operation: &projection.OperationDetail{
			ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2,
			Method: "GET", Path: "/api/v1/pods", Parameters: projected,
		},
	}
	operation := domain.Operation{
		Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/api/v1/pods",
		Parameters: []domain.OperationParameter{
			{Name: "<namespace>", In: "path", Required: true, Description: "Namespace <name>.", Schema: parameterSchemaSummary("string", "")},
			{Name: "labels", In: "query", Description: "Label selector.", Schema: domain.SchemaSummary{Type: "array", Constraints: []domain.SchemaConstraint{}, Items: schemaSummaryPointer(parameterSchemaSummary("string", ""))}},
			{Name: "X-Trace", In: "header", Description: "Trace ID.", Schema: parameterSchemaSummary("string", "uuid")},
			{Name: "session", In: "cookie", Schema: parameterSchemaSummary("boolean", "")},
		},
	}
	nodes := []projection.SchemaNode{
		{Ordinal: 0, ID: "node-namespace", Type: "string"},
		{Ordinal: 1, ID: "node-labels", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "item", SchemaRef: 2}}},
		{Ordinal: 2, ID: "node-label", Type: "string"},
		{Ordinal: 3, ID: "node-trace", Type: "string", Format: "uuid"},
		{Ordinal: 4, ID: "node-session", Type: "boolean"},
	}
	return detail, operation, nodes
}

func parameterSchemaSummary(typeName, format string) domain.SchemaSummary {
	return domain.SchemaSummary{Type: typeName, Format: format, Constraints: []domain.SchemaConstraint{}}
}

func schemaSummaryPointer(value domain.SchemaSummary) *domain.SchemaSummary { return &value }

func cloneParameterDetail(value catalog.DetailRecordV1) catalog.DetailRecordV1 {
	clone := value
	if value.Operation != nil {
		operation := *value.Operation
		operation.Parameters = append([]projection.Parameter(nil), value.Operation.Parameters...)
		for index := range operation.Parameters {
			operation.Parameters[index].Examples = append([]projection.Example(nil), operation.Parameters[index].Examples...)
		}
		clone.Operation = &operation
	}
	return clone
}

func cloneParameterOperation(value domain.Operation) domain.Operation {
	clone := value
	clone.Parameters = make([]domain.OperationParameter, len(value.Parameters))
	for index, parameter := range value.Parameters {
		clone.Parameters[index] = parameter
		clone.Parameters[index].Schema = cloneParameterSchema(parameter.Schema)
	}
	return clone
}

func cloneParameterSchema(value domain.SchemaSummary) domain.SchemaSummary {
	clone := value
	clone.Enum = append([]string(nil), value.Enum...)
	clone.Constraints = append([]domain.SchemaConstraint(nil), value.Constraints...)
	clone.Properties = make([]domain.SchemaProperty, len(value.Properties))
	for index, property := range value.Properties {
		clone.Properties[index] = property
		clone.Properties[index].Schema = cloneParameterSchema(property.Schema)
	}
	if value.Items != nil {
		items := cloneParameterSchema(*value.Items)
		clone.Items = &items
	}
	return clone
}
