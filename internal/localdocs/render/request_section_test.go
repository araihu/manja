package render

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func TestPreparedOperationRequestSectionRendersCanonicalChildren(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	authorization, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, &body, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fragment.Bytes(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedOperationRequestSectionRendersCanonicalMarkup(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	authorization, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, &body, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<section class="grid gap-8" aria-label="Request">`,
		`<h4 class="font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">Request</h4>`,
		`data-manja-request-body`,
		`data-manja-request-body-media="application/json"`,
		`dark:border-outline-dark`,
	} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("request section missing %q in %s", want, rendered)
		}
	}
}

func TestPrepareOperationRequestSectionFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *OperationAuthorizationFragment, *OperationParametersFragment, **OperationRequestBodyFragment)
	}{
		{name: "invalid authorization", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, authorization *OperationAuthorizationFragment, _ *OperationParametersFragment, _ **OperationRequestBodyFragment) {
			*authorization = OperationAuthorizationFragment{}
		}},
		{name: "invalid parameters", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationAuthorizationFragment, parameters *OperationParametersFragment, _ **OperationRequestBodyFragment) {
			*parameters = OperationParametersFragment{}
		}},
		{name: "missing body child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationAuthorizationFragment, _ *OperationParametersFragment, body **OperationRequestBodyFragment) {
			*body = nil
		}},
		{name: "changed operation parent", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationAuthorizationFragment, _ *OperationParametersFragment, _ **OperationRequestBodyFragment) {
			operation.Path = "/api/v1/changed"
		}},
		{name: "changed detail request declaration", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationAuthorizationFragment, _ *OperationParametersFragment, _ **OperationRequestBodyFragment) {
			detail.Operation.HasRequestBody = false
		}},
		{name: "invalid UTF-8 child", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationAuthorizationFragment, _ *OperationParametersFragment, body **OperationRequestBodyFragment) {
			(*body).data.Description = string([]byte{0xff})
		}},
		{name: "mixed body context", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationAuthorizationFragment, _ *OperationParametersFragment, body **OperationRequestBodyFragment) {
			otherDetail, otherOperation, nodes, _, _ := operationRequestBodyMediaFixture()
			otherDetail.ID = detail.ID
			otherDetail.Operation.ID = detail.Operation.ID
			otherDetail.Operation.Anchor = detail.Operation.Anchor
			otherOperation.Anchor = operation.Anchor
			otherDocumentHref := "/documents/other/"
			otherSchemaLinks := map[string]string{"Pod": otherDocumentHref + "?selected=detail-sha256-" + strings.Repeat("c", 64) + "#detail-sha256-" + strings.Repeat("c", 64)}
			media, err := PrepareOperationRequestBodyMedia(otherDetail, otherOperation, nodes, otherDocumentHref, otherSchemaLinks)
			if err != nil {
				t.Fatalf("prepare mixed media: %v", err)
			}
			trees, err := PrepareOperationSchemaTrees(otherDetail, otherOperation, nodes, otherDocumentHref, otherSchemaLinks)
			if err != nil {
				t.Fatalf("prepare mixed trees: %v", err)
			}
			otherBody, err := PrepareOperationRequestBody(otherDetail, otherOperation, media, trees)
			if err != nil {
				t.Fatalf("prepare mixed body: %v", err)
			}
			*body = &otherBody
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
			authorization, err := PrepareOperationAuthorization(detail, operation)
			if err != nil {
				t.Fatal(err)
			}
			parameters, err := PrepareOperationParameters(detail, operation, nil)
			if err != nil {
				t.Fatal(err)
			}
			media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			body, err := PrepareOperationRequestBody(detail, operation, media, trees)
			if err != nil {
				t.Fatal(err)
			}
			bodyPtr := &body
			test.mutate(&detail, &operation, &authorization, &parameters, &bodyPtr)
			fragment, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, bodyPtr, documentHref, schemaLinks)
			if err == nil || !reflect.DeepEqual(fragment, OperationRequestSectionFragment{}) {
				t.Fatalf("PrepareOperationRequestSection() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPreparedOperationRequestSectionCopiesRenderInputs(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	authorization, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, &body, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.RequestBody.Description = "mutated"
	operation.RequestBody.Description = "mutated"
	body.data.Description = "mutated"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared request-section bytes changed after input mutation\nwant=%s\n got=%s", want, got)
	}
}

func TestPrepareOperationRequestSectionRejectsInvalidDocumentHref(t *testing.T) {
	t.Parallel()

	detail, operation, _, _, _ := operationRequestBodyMediaFixture()
	authorization, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, nil, "/invalid/", nil)
	if err == nil || !strings.Contains(err.Error(), "document href") || !reflect.DeepEqual(fragment, OperationRequestSectionFragment{}) {
		t.Fatalf("PrepareOperationRequestSection() = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestPreparedOperationRequestSectionRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	authorization, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	body.data.Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	fragment, err := PrepareOperationRequestSection(detail, operation, authorization, parameters, &body, documentHref, schemaLinks)
	if err == nil || !reflect.DeepEqual(fragment, OperationRequestSectionFragment{}) {
		t.Fatalf("PrepareOperationRequestSection() = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestOperationRequestSectionRejectsZeroFragment(t *testing.T) {
	t.Parallel()

	body, err := (OperationRequestSectionFragment{}).Bytes(context.Background())
	if err == nil || len(body) != 0 {
		t.Fatalf("zero request-section fragment Bytes() = (%d bytes, %v), want zero bytes and error", len(body), err)
	}
}
