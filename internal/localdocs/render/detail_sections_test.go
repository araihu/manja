package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func TestPreparedOperationDetailSectionsRendersRequestAndResponses(t *testing.T) {
	t.Parallel()

	detail, operation, request, responses, _ := preparedOperationDetailSectionsFixture(t)
	fragment, err := PrepareOperationDetailSections(detail, operation, &request, &responses)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`class="manja-endpoint-detail-layout"`,
		`aria-label="Request"`,
		`aria-label="Responses"`,
	} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("prepared operation detail sections missing %q in %s", want, rendered)
		}
	}
}

func TestPrepareOperationDetailSectionsFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*OperationRequestSectionFragment, *OperationResponsesFragment, *bool, *bool)
	}{
		{name: "invalid request child", mutate: func(request *OperationRequestSectionFragment, _ *OperationResponsesFragment, _ *bool, _ *bool) {
			request.valid = false
		}},
		{name: "invalid response child", mutate: func(_ *OperationRequestSectionFragment, responses *OperationResponsesFragment, _ *bool, _ *bool) {
			responses.valid = false
		}},
		{name: "missing request child", mutate: func(_ *OperationRequestSectionFragment, _ *OperationResponsesFragment, requestPresent *bool, _ *bool) {
			*requestPresent = false
		}},
		{name: "missing response child", mutate: func(_ *OperationRequestSectionFragment, _ *OperationResponsesFragment, _ *bool, responsePresent *bool) {
			*responsePresent = false
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, request, responses, _ := preparedOperationDetailSectionsFixture(t)
			requestPresent, responsePresent := true, true
			test.mutate(&request, &responses, &requestPresent, &responsePresent)
			var requestPtr *OperationRequestSectionFragment
			if requestPresent {
				requestPtr = &request
			}
			var responsePtr *OperationResponsesFragment
			if responsePresent {
				responsePtr = &responses
			}
			fragment, err := PrepareOperationDetailSections(detail, operation, requestPtr, responsePtr)
			if err == nil || !reflect.DeepEqual(fragment, OperationDetailSectionsFragment{}) {
				t.Fatalf("PrepareOperationDetailSections() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPreparedOperationDetailSectionsCopiesChildren(t *testing.T) {
	t.Parallel()

	detail, operation, request, responses, schemaLinks := preparedOperationDetailSectionsFixture(t)
	fragment, err := PrepareOperationDetailSections(detail, operation, &request, &responses)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request.data.Body.data.Description = "mutated request"
	responses.data.Responses[0].Details.Description = "mutated response"
	schemaLinks["Phase"] = "/mutated/"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared operation detail sections changed after child mutation\nwant=%s\n got=%s", want, got)
	}
}

func TestPrepareOperationDetailSectionsRejectsMixedPreparationContexts(t *testing.T) {
	t.Parallel()

	detail, operation, request, _, requestLinks := preparedOperationDetailSectionsFixture(t)
	_, _, nodes, _, _ := operationSchemaTreeFixture()
	otherDocumentHref := "/documents/other/"
	otherSchemaLinks := map[string]string{
		"Phase": strings.Replace(requestLinks["Phase"], "/kubernetes/documents/core-v1/", otherDocumentHref, 1),
		"Pod":   otherDocumentHref + "?selected=" + string(detail.ID) + "#" + string(detail.ID),
	}
	media, err := PrepareOperationResponseMedia(detail, operation, nodes[3:], otherDocumentHref, otherSchemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	details, err := PrepareOperationResponseDetails(detail, operation, nil, otherDocumentHref, otherSchemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	examples, err := PrepareOperationExamples(detail, operation, nodes[3:])
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, otherDocumentHref, otherSchemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	responses, err := PrepareOperationResponses(detail, operation, media, details, examples, trees)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationDetailSections(detail, operation, &request, &responses)
	if err == nil || !reflect.DeepEqual(fragment, OperationDetailSectionsFragment{}) {
		t.Fatalf("PrepareOperationDetailSections() = (%#v, %v), want zero fragment and error for mixed contexts", fragment, err)
	}
}

func TestPrepareOperationDetailSectionsRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	detail, operation, request, responses, _ := preparedOperationDetailSectionsFixture(t)
	request.data.Body.data.Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	fragment, err := PrepareOperationDetailSections(detail, operation, &request, &responses)
	if err == nil || !reflect.DeepEqual(fragment, OperationDetailSectionsFragment{}) {
		t.Fatalf("PrepareOperationDetailSections() = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func preparedOperationDetailSectionsFixture(t *testing.T) (catalog.DetailRecordV1, domain.Operation, OperationRequestSectionFragment, OperationResponsesFragment, map[string]string) {
	t.Helper()
	detail, operation, nodes, documentHref, schemaLinks := operationSchemaTreeFixture()
	requestMedia, err := PrepareOperationRequestBodyMedia(detail, operation, nodes[:1], documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	requestBody, err := PrepareOperationRequestBody(detail, operation, requestMedia, trees)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, &requestBody, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	media, details, examples, responseTrees := prepareOperationResponsesChildren(t, detail, operation, nodes, documentHref, schemaLinks)
	responses, err := PrepareOperationResponses(detail, operation, media, details, examples, responseTrees)
	if err != nil {
		t.Fatal(err)
	}
	return detail, operation, request, responses, schemaLinks
}
