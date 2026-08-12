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

func TestPreparedOperationExamplesRenderCanonicalEscapedResponseAndCodeSamples(t *testing.T) {
	t.Parallel()

	detail, operation, nodes := operationExamplesFixture()
	fragment, err := PrepareOperationExamples(detail, operation, nodes)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, err := fragment.ResponseExampleBytes(context.Background(), 0, 0, operation.Anchor)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-manja-example`,
		`Response Example: 201 application/json`,
		`&lt;script&gt;alert(&#39;response&#39;)&lt;/script&gt;`,
		`"hasExplicitExample":true`,
		`aria-live="polite"`,
	} {
		if !strings.Contains(string(responseBody), want) {
			t.Errorf("response example missing %q in %s", want, responseBody)
		}
	}
	if strings.Contains(string(responseBody), `<script>alert('response')</script>`) {
		t.Fatalf("response example emitted unescaped HTML: %s", responseBody)
	}

	first, err := fragment.CodeSampleBytes(context.Background(), 0, "Request Sample: cURL")
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.CodeSampleBytes(context.Background(), 1, "Request Sample: JavaScript")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), `Request Sample: cURL`) || !strings.Contains(string(first), `curl &lt;unsafe&gt;`) || !strings.Contains(string(second), `Request Sample: JavaScript`) || !strings.Contains(string(second), `&#39;/pods&#39;`) {
		t.Fatalf("code-sample order or escaping drifted: first=%s second=%s", first, second)
	}
}

func TestPrepareOperationExamplesFailsClosedOnMutationInventoryOrderSizeAndUTF8(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation)
	}{
		{name: "response reorder", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0], detail.Operation.Responses[1] = detail.Operation.Responses[1], detail.Operation.Responses[0]
		}},
		{name: "response status", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Responses[0].Status = "200"
		}},
		{name: "header example missing", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0].Headers[0].Examples = nil
		}},
		{name: "header example extra", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0].Headers[0].Examples = append(detail.Operation.Responses[0].Headers[0].Examples, projection.Example{Ordinal: 1, ID: "secondary", Text: "18", Provided: true})
		}},
		{name: "header example invalid utf8", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0].Headers[0].Examples[0].Text = string([]byte{0xff})
		}},
		{name: "prepared header example", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Responses[0].Headers[0].Example = "18"
		}},
		{name: "media reorder", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0].MediaTypes[0], detail.Operation.Responses[0].MediaTypes[1] = detail.Operation.Responses[0].MediaTypes[1], detail.Operation.Responses[0].MediaTypes[0]
		}},
		{name: "media example missing", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0].MediaTypes[0].Examples = nil
		}},
		{name: "media example extra", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Responses[0].MediaTypes[0].Examples = append(detail.Operation.Responses[0].MediaTypes[0].Examples, projection.Example{Ordinal: 1, ID: "secondary", Text: "extra", Provided: true})
		}},
		{name: "prepared media example", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Responses[0].MediaTypes[0].Example = "changed"
		}},
		{name: "prepared media schema JSON", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Responses[0].MediaTypes[0].Schema.JSON = `{"changed":true}`
		}},
		{name: "code sample missing", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.CodeSamples = detail.Operation.CodeSamples[:1]
		}},
		{name: "code sample extra", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.CodeSamples = append(detail.Operation.CodeSamples, projection.CodeSample{Ordinal: 2, ID: operationCodeSampleID("ruby", "Ruby"), Label: "Ruby", Language: "ruby", Code: "puts :ok"})
		}},
		{name: "code sample reorder", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.CodeSamples[0], detail.Operation.CodeSamples[1] = detail.Operation.CodeSamples[1], detail.Operation.CodeSamples[0]
		}},
		{name: "code sample id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.CodeSamples[0].ID = "curl"
		}},
		{name: "prepared code", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Snippets[0].Code = "changed"
		}},
		{name: "invalid code utf8", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation) {
			invalid := string([]byte{0xff})
			detail.Operation.CodeSamples[0].Code = invalid
			operation.Snippets[0].Code = invalid
		}},
		{name: "oversized response example", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation) {
			huge := strings.Repeat("x", maximumHTMLFragmentBytes+1)
			detail.Operation.Responses[0].MediaTypes[0].Examples[0].Text = huge
			operation.Responses[0].MediaTypes[0].Example = huge
		}},
		{name: "oversized code sample", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation) {
			huge := strings.Repeat("x", maximumHTMLFragmentBytes+1)
			detail.Operation.CodeSamples[0].Code = huge
			operation.Snippets[0].Code = huge
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes := operationExamplesFixture()
			test.mutate(&detail, &operation)
			fragment, err := PrepareOperationExamples(detail, operation, nodes)
			if err == nil || !reflect.DeepEqual(fragment, OperationExamplesFragment{}) {
				t.Fatalf("PrepareOperationExamples() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPreparedOperationExamplesCopyInputsPreserveExplicitEmptyAndRejectInvalidIndexes(t *testing.T) {
	t.Parallel()

	detail, operation, nodes := operationExamplesFixture()
	fragment, err := PrepareOperationExamples(detail, operation, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if !fragment.HasResponseExample(0, 0) || fragment.HasResponseExample(0, 1) || fragment.HasResponseExample(1, 0) {
		t.Fatalf("response example visibility drifted")
	}
	before, err := fragment.ResponseExampleBytes(context.Background(), 0, 0, operation.Anchor)
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Responses[0].MediaTypes[0].Examples[0].Text = "changed"
	operation.Responses[0].MediaTypes[0].Example = "changed"
	detail.Operation.CodeSamples[0].Code = "changed"
	operation.Snippets[0].Code = "changed"
	after, err := fragment.ResponseExampleBytes(context.Background(), 0, 0, operation.Anchor)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("prepared response example changed after source mutation")
	}
	for _, indexes := range [][2]int{{-1, 0}, {0, -1}, {0, 2}, {2, 0}} {
		body, renderErr := fragment.ResponseExampleBytes(context.Background(), indexes[0], indexes[1], operation.Anchor)
		if renderErr == nil || len(body) != 0 {
			t.Fatalf("ResponseExampleBytes(%d, %d) = (%d bytes, %v), want zero bytes and error", indexes[0], indexes[1], len(body), renderErr)
		}
	}
	body, renderErr := fragment.CodeSampleBytes(context.Background(), 2, "Request Sample")
	if renderErr == nil || len(body) != 0 {
		t.Fatalf("CodeSampleBytes(2) = (%d bytes, %v), want zero bytes and error", len(body), renderErr)
	}
}

func operationExamplesFixture() (catalog.DetailRecordV1, domain.Operation, []projection.SchemaNode) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), Responses: []projection.Response{
			{Ordinal: 0, ID: "201", Status: "201", Headers: []projection.ResponseHeader{{Ordinal: 0, ID: operationResponseHeaderID("X-Quota"), Name: "X-Quota", Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: "17", Provided: true}}}}, MediaTypes: []projection.MediaType{
				{Ordinal: 0, ID: "application/json", ContentType: "application/json", Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: `<script>alert('response')</script>`, Provided: true}}},
				{Ordinal: 1, ID: "text/plain", ContentType: "text/plain", Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: "", Provided: true}}},
			}},
			{Ordinal: 1, ID: "204", Status: "204"},
		},
		CodeSamples: []projection.CodeSample{
			{Ordinal: 0, ID: operationCodeSampleID("shell", "cURL"), Label: "cURL", Language: "shell", Code: "curl <unsafe>"},
			{Ordinal: 1, ID: operationCodeSampleID("javascript", "JavaScript"), Label: "JavaScript", Language: "javascript", Code: "fetch('/pods')"},
		},
	}}
	operation := domain.Operation{Anchor: string(detailID), Responses: []domain.OperationResponse{
		{Status: "201", Headers: []domain.OperationResponseHeader{{Name: "X-Quota", Example: "17"}}, MediaTypes: []domain.OperationMediaType{
			{ContentType: "application/json", Example: `<script>alert('response')</script>`, ExampleProvided: true},
			{ContentType: "text/plain", Example: "", ExampleProvided: true},
		}},
		{Status: "204"},
	}, Snippets: []domain.RequestSnippet{
		{Label: "cURL", Language: "shell", Code: "curl <unsafe>"},
		{Label: "JavaScript", Language: "javascript", Code: "fetch('/pods')"},
	}}
	return detail, operation, []projection.SchemaNode{{Ordinal: 0, ID: "node-response-example"}}
}
