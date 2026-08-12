package render

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestPreparedOperationHeaderRendersCanonicalEscapedHTML(t *testing.T) {
	detail, operation := operationHeaderFixture()
	fragment, err := PrepareOperationHeader(detail, operation, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	actions := literalComponent(`<div data-test-actions="true">actions</div>`)
	provenance := literalComponent(`<dl data-test-provenance="true"><dt>snapshot</dt></dl>`)
	body, err := fragment.Bytes(context.Background(), actions, provenance)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<header class="mb-8 min-w-0 border-b border-outline pb-6 dark:border-outline-dark" data-public-page-header="true">`,
		`id="detail-sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-heading"`,
		`data-public-doc-identity="detail-sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`data-manja-settled-focus="true"`,
		`&lt;Create&gt; Pod`,
		`Deprecated`,
		`Creates &lt;Pod&gt;.`,
		`aria-label="Endpoint route"`,
		`POST`,
		`/api/v1/namespaces/{namespace}/pods`,
		`data-test-actions="true"`,
		`data-test-provenance="true"`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("operation header missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte("<Create>")) || bytes.Contains(body, []byte("Creates <Pod>.")) {
		t.Fatalf("operation header contains unescaped projection content: %s", body)
	}
}

func TestPrepareOperationHeaderFailsClosedOnInconsistentInputs(t *testing.T) {
	baseDetail, baseOperation := operationHeaderFixture()
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *string)
	}{
		{name: "schema detail", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *string) {
			detail.Kind, detail.Operation, detail.Schema = "schema", nil, &projection.SchemaDetail{}
		}},
		{name: "detail id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *string) {
			detail.ID = domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
		}},
		{name: "operation detail id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *string) {
			detail.Operation.ID = "changed"
		}},
		{name: "operation anchor", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *string) {
			detail.Operation.Anchor = "changed"
		}},
		{name: "operation heading id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *string) {
			detail.Operation.HeadingID = "changed"
		}},
		{name: "operation href", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *string) {
			detail.Operation.Href = "?selected=changed"
		}},
		{name: "empty heading", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			detail.Operation.Heading, operation.Title = "", ""
		}},
		{name: "empty method", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			detail.Operation.Method, operation.Method = "", ""
		}},
		{name: "non-absolute endpoint path", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			detail.Operation.Path, operation.Path = "relative", "relative"
		}},
		{name: "prepared anchor", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Anchor = "changed"
		}},
		{name: "prepared title", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Title = "changed"
		}},
		{name: "prepared method", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Method = "GET"
		}},
		{name: "prepared path", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Path = "/changed"
		}},
		{name: "prepared summary", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Summary = "changed"
		}},
		{name: "prepared description", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Description = "changed"
		}},
		{name: "prepared deprecated", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *string) {
			operation.Deprecated = false
		}},
		{name: "relative document href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, href *string) {
			*href = "kubernetes/documents/core-v1/"
		}},
		{name: "cross-origin document href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, href *string) {
			*href = "https://attacker.example/core-v1/"
		}},
		{name: "traversal document href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, href *string) {
			*href = "/kubernetes/../core-v1/"
		}},
		{name: "encoded traversal document href", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, href *string) {
			*href = "/kubernetes/documents/%2e%2e/"
			detail.Operation.Href = "documents/%2e%2e/?selected=" + string(detail.ID) + "#" + string(detail.ID)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := cloneDetail(baseDetail)
			operation := cloneOperation(baseOperation)
			href := "/kubernetes/documents/core-v1/"
			test.mutate(&detail, &operation, &href)

			fragment, err := PrepareOperationHeader(detail, operation, href)
			if err == nil || !reflect.DeepEqual(fragment, OperationHeaderFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedOperationHeaderCopiesInputsAndRendersDeterministically(t *testing.T) {
	detail, operation := operationHeaderFixture()
	fragment, err := PrepareOperationHeader(detail, operation, "/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Heading = "changed"
	operation.Title = "changed"
	operation.Tags[0] = "changed"

	first, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || bytes.Contains(first, []byte("changed")) || !bytes.Contains(first, []byte("&lt;Create&gt; Pod")) {
		t.Fatalf("operation header aliases inputs or is nondeterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	first[0] = 'x'
	third, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, third) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPrepareOperationHeaderPreservesCompilerAcceptedLowercaseMethod(t *testing.T) {
	detail, operation := operationHeaderFixture()
	detail.Operation.Method = "post"
	operation.Method = "post"
	fragment, err := PrepareOperationHeader(detail, operation, "/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(">POST</span>")) {
		t.Fatalf("lowercase method did not retain rendered badge contract: %s", body)
	}
}

func TestPreparedOperationHeaderRejectsOutputAboveTwoMiBWithoutPartialBytes(t *testing.T) {
	detail, operation := operationHeaderFixture()
	detail.Operation.Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	operation.Description = detail.Operation.Description
	fragment, err := PrepareOperationHeader(detail, operation, "/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background(), nil, nil)
	if err == nil || body != nil {
		t.Fatalf("oversized fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestZeroOperationHeaderFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (OperationHeaderFragment{}).Bytes(context.Background(), nil, nil)
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func operationHeaderFixture() (catalog.DetailRecordV1, domain.Operation) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	detail := catalog.DetailRecordV1{
		ID: detailID, Kind: "operation",
		Operation: &projection.OperationDetail{
			ID: string(detailID), Anchor: string(detailID),
			Href:      "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
			HeadingID: string(detailID), Heading: "<Create> Pod", HeadingLevel: 2,
			Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods", Summary: "Create Pod",
			Description: "Creates <Pod>.", Deprecated: true,
		},
	}
	operation := domain.Operation{
		ID: "createCoreV1NamespacedPod", Anchor: string(detailID), Title: "<Create> Pod",
		Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods", Summary: "Create Pod",
		Description: "Creates <Pod>.", Deprecated: true, Tags: []string{"core"},
	}
	return detail, operation
}

func cloneOperation(value domain.Operation) domain.Operation {
	clone := value
	clone.Tags = append([]string(nil), value.Tags...)
	return clone
}

func literalComponent(value string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, writer io.Writer) error {
		_, err := io.WriteString(writer, value)
		return err
	})
}
