package render

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func TestPreparedOperationRequestBodyRendersCompleteCanonicalSection(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-manja-request-body`,
		`aria-label="Request body"`,
		`>Body Params<`,
		`data-required="true">required</span>`,
		`Pod to create.`,
		`data-manja-request-body-media="application/json"`,
		`hx-target="#catalog-main-content"`,
		`aria-label="Request body schema for application/json schema tree"`,
		`dark:border-outline-dark`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request-body section missing %q in %s", want, body)
		}
	}
}

func TestPrepareOperationRequestBodyFailsClosedOnInconsistentInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *OperationRequestBodyMediaFragment, *OperationSchemaTreesFragment)
	}{
		{name: "request body declaration", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationRequestBodyMediaFragment, _ *OperationSchemaTreesFragment) {
			detail.Operation.HasRequestBody = false
		}},
		{name: "prepared body", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationRequestBodyMediaFragment, _ *OperationSchemaTreesFragment) {
			operation.RequestBody = nil
		}},
		{name: "description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationRequestBodyMediaFragment, _ *OperationSchemaTreesFragment) {
			operation.RequestBody.Description = "changed"
		}},
		{name: "required", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationRequestBodyMediaFragment, _ *OperationSchemaTreesFragment) {
			operation.RequestBody.Required = false
		}},
		{name: "media identity", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *OperationRequestBodyMediaFragment, _ *OperationSchemaTreesFragment) {
			detail.Operation.RequestBody.MediaTypes[0].ID = "application/xml"
			detail.Operation.RequestBody.MediaTypes[0].ContentType = "application/xml"
			operation.RequestBody.MediaTypes[0].ContentType = "application/xml"
		}},
		{name: "invalid media fragment", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, media *OperationRequestBodyMediaFragment, _ *OperationSchemaTreesFragment) {
			*media = OperationRequestBodyMediaFragment{}
		}},
		{name: "invalid schema trees", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *OperationRequestBodyMediaFragment, trees *OperationSchemaTreesFragment) {
			*trees = OperationSchemaTreesFragment{}
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
			media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&detail, &operation, &media, &trees)
			fragment, err := PrepareOperationRequestBody(detail, operation, media, trees)
			if err == nil || !reflect.DeepEqual(fragment, OperationRequestBodyFragment{}) {
				t.Fatalf("PrepareOperationRequestBody() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPreparedOperationRequestBodyCopiesRenderInputs(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	detail.Operation.RequestBody.Description = "mutated"
	operation.RequestBody.Description = "mutated"
	media.media[0].ContentType = "application/xml"
	trees.request[0].Caption = "mutated"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared request-body bytes changed after input mutation\nwant=%s\n got=%s", want, got)
	}
}

func TestPrepareOperationRequestBodyRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	detail, operation, nodes, documentHref, schemaLinks := operationRequestBodyMediaFixture()
	huge := strings.Repeat("x", maximumHTMLFragmentBytes)
	detail.Operation.RequestBody.Description = huge
	operation.RequestBody.Description = huge
	media, err := PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := PrepareOperationRequestBody(detail, operation, media, trees)
	if err == nil || !reflect.DeepEqual(fragment, OperationRequestBodyFragment{}) {
		t.Fatalf("PrepareOperationRequestBody() = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestOperationRequestBodyRejectsZeroFragment(t *testing.T) {
	t.Parallel()

	body, err := (OperationRequestBodyFragment{}).Bytes(context.Background())
	if err == nil || len(body) != 0 {
		t.Fatalf("zero request-body fragment Bytes() = (%d bytes, %v), want zero bytes and error", len(body), err)
	}
}
