package render

import (
	"bytes"
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestPreparedOperationResponsesRendersCanonicalSection(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	operation.Responses[0].MediaTypes[0].Example = `{"kind":"Envelope"}`
	operation.Responses[0].MediaTypes[0].ExampleProvided = true
	detail.Operation.Responses[0].MediaTypes[0].Examples = []projection.Example{{Ordinal: 0, ID: "primary", Text: `{"kind":"Envelope"}`, Provided: true}}
	responseNodes := nodes[3:]
	media, err := PrepareOperationResponseMedia(detail, operation, responseNodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	details, err := PrepareOperationResponseDetails(detail, operation, nil, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	examples, err := PrepareOperationExamples(detail, operation, responseNodes)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationResponses(detail, operation, media, details, examples, trees)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`aria-label="Responses"`,
		`>Responses</h4>`,
		`id="` + operation.Anchor + `-responses"`,
		` Created</span>`,
		`>201<`,
		`data-manja-response-section="media-type"`,
		`aria-label="Response 201 application/json example"`,
		`dark:border-outline-dark`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("prepared responses missing %q in %s", want, body)
		}
	}
}

func TestPrepareOperationResponsesFailsClosedOnInconsistentInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *OperationResponseMediaFragment, *OperationResponseDetailsFragment, *OperationExamplesFragment, *OperationSchemaTreesFragment)
	}{
		{name: "prepared response", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationResponseMediaFragment, _ *OperationResponseDetailsFragment, _ *OperationExamplesFragment, _ *OperationSchemaTreesFragment) {
			operation.Responses[0].Status = "202"
		}},
		{name: "projected response", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationResponseMediaFragment, _ *OperationResponseDetailsFragment, _ *OperationExamplesFragment, _ *OperationSchemaTreesFragment) {
			detail.Operation.Responses[0].ID = "202"
			detail.Operation.Responses[0].Status = "202"
		}},
		{name: "media child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, media *OperationResponseMediaFragment, _ *OperationResponseDetailsFragment, _ *OperationExamplesFragment, _ *OperationSchemaTreesFragment) {
			media.media[0][0].ContentType = "application/xml"
		}},
		{name: "details child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationResponseMediaFragment, details *OperationResponseDetailsFragment, _ *OperationExamplesFragment, _ *OperationSchemaTreesFragment) {
			details.responses[0].Description = "changed"
		}},
		{name: "example child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationResponseMediaFragment, _ *OperationResponseDetailsFragment, examples *OperationExamplesFragment, _ *OperationSchemaTreesFragment) {
			examples.responses[0][0].Status = "202"
		}},
		{name: "schema-tree child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationResponseMediaFragment, _ *OperationResponseDetailsFragment, _ *OperationExamplesFragment, trees *OperationSchemaTreesFragment) {
			trees.responses[0][0].ID = "changed"
		}},
		{name: "zero media child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, media *OperationResponseMediaFragment, _ *OperationResponseDetailsFragment, _ *OperationExamplesFragment, _ *OperationSchemaTreesFragment) {
			*media = OperationResponseMediaFragment{}
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
			media, details, examples, trees := prepareOperationResponsesChildren(t, detail, operation, nodes, documentHref, schemaLinks)
			test.mutate(&detail, &operation, &media, &details, &examples, &trees)
			fragment, err := PrepareOperationResponses(detail, operation, media, details, examples, trees)
			if err == nil || !reflect.DeepEqual(fragment, OperationResponsesFragment{}) {
				t.Fatalf("PrepareOperationResponses() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPrepareOperationResponsesRejectsMixedChildPreparationContexts(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	media, details, examples, _ := prepareOperationResponsesChildren(t, detail, operation, nodes, documentHref, schemaLinks)
	otherDocumentHref := "/documents/other/"
	otherSchemaLinks := map[string]string{"Phase": strings.Replace(schemaLinks["Phase"], documentHref, otherDocumentHref, 1)}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, otherDocumentHref, otherSchemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationResponses(detail, operation, media, details, examples, trees)
	if err == nil || !reflect.DeepEqual(fragment, OperationResponsesFragment{}) {
		t.Fatalf("PrepareOperationResponses() = (%#v, %v), want zero fragment and error for mixed contexts", fragment, err)
	}
}

func TestPreparedOperationResponsesCopiesRenderInputs(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	media, details, examples, trees := prepareOperationResponsesChildren(t, detail, operation, nodes, documentHref, schemaLinks)
	fragment, err := PrepareOperationResponses(detail, operation, media, details, examples, trees)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Responses[0].Status = "mutated"
	operation.Responses[0].Description = "mutated"
	media.media[0][0].ContentType = "mutated"
	details.responses[0].Description = "mutated"
	examples.responses[0][0].Schema = []byte(`{"mutated":true}`)
	trees.responses[0][0].Caption = "mutated"
	schemaLinks["Phase"] = "/mutated/"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared responses bytes changed after input mutation\nwant=%s\n got=%s", want, got)
	}
}

func TestPrepareOperationResponsesRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	huge := strings.Repeat("x", maximumHTMLFragmentBytes)
	detail.Operation.Responses[0].Description = huge
	operation.Responses[0].Description = huge
	media, details, examples, trees := prepareOperationResponsesChildren(t, detail, operation, nodes, documentHref, schemaLinks)
	fragment, err := PrepareOperationResponses(detail, operation, media, details, examples, trees)
	if err == nil || !reflect.DeepEqual(fragment, OperationResponsesFragment{}) {
		t.Fatalf("PrepareOperationResponses() = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestOperationResponsesRejectsZeroFragment(t *testing.T) {
	t.Parallel()

	body, err := (OperationResponsesFragment{}).Bytes(context.Background())
	if err == nil || len(body) != 0 {
		t.Fatalf("zero responses Bytes() = (%d bytes, %v), want zero bytes and error", len(body), err)
	}
}

func TestOperationResponseStatusTextMatchesSSRContract(t *testing.T) {
	t.Parallel()

	for code := 100; code <= 599; code++ {
		if got, want := operationResponseStatusText(code), http.StatusText(code); got != want {
			t.Errorf("operationResponseStatusText(%d) = %q, want %q", code, got, want)
		}
	}
}

func prepareOperationResponsesChildren(
	t *testing.T,
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	nodes []projection.SchemaNode,
	documentHref string,
	schemaLinks map[string]string,
) (OperationResponseMediaFragment, OperationResponseDetailsFragment, OperationExamplesFragment, OperationSchemaTreesFragment) {
	t.Helper()
	responseNodes := nodes[3:]
	media, err := PrepareOperationResponseMedia(detail, operation, responseNodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	details, err := PrepareOperationResponseDetails(detail, operation, nil, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	examples, err := PrepareOperationExamples(detail, operation, responseNodes)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	return media, details, examples, trees
}
