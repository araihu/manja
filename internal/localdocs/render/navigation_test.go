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

func TestPreparedOperationNavigationRendersCanonicalCatalogNeighbors(t *testing.T) {
	t.Parallel()

	detail, operation, document, documentHref := operationNavigationFixture()
	fragment, err := PrepareOperationNavigation(detail, operation, document, documentHref, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-manja-operation-navigation`,
		`data-manja-operation-group="core"`,
		`aria-label="More operations in core"`,
		`class="grid gap-3 lg:grid-cols-2"`,
		`data-manja-operation-neighbor="previous"`,
		`data-manja-operation-neighbor="next"`,
		`hx-target="#main-content"`,
		`hx-select="#main-content"`,
		`hx-select-oob="#catalog-navigation"`,
		`hx-swap="outerHTML show:#main-content:top"`,
		`focus-visible:outline-primary`,
		`dark:focus-visible:outline-primary-dark`,
		`sm:col-start-2`,
		`&lt;List&gt; Pods`,
		`Delete Pod`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("operation navigation missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte(`<List>`)) || bytes.Contains(body, []byte(`Other group`)) {
		t.Fatalf("operation navigation leaked unescaped or cross-group data: %s", body)
	}
}

func TestPrepareOperationNavigationFailsClosedOnInconsistentInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation, *catalog.DocumentDirectoryV1, *string, *map[string]struct{})
	}{
		{name: "schema detail", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, _ *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			detail.Kind, detail.Operation, detail.Schema = "schema", nil, &projection.SchemaDetail{}
		}},
		{name: "prepared operation", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation, _ *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			operation.Method = "PATCH"
		}},
		{name: "missing selected directory operation", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, document *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			document.Operations = append([]catalog.OperationDirectoryV1(nil), document.Operations[:1]...)
		}},
		{name: "duplicate detail id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation, document *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			document.Operations[3].DetailID = detail.ID
		}},
		{name: "directory method", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, document *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			document.Operations[1].Method = "PATCH"
		}},
		{name: "directory href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, document *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			document.Operations[1].Href = "core-v1/?selected=changed"
		}},
		{name: "invalid neighbor method", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, document *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			document.Operations[0].Method = "G ET"
		}},
		{name: "invalid neighbor UTF-8", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, document *catalog.DocumentDirectoryV1, _ *string, _ *map[string]struct{}) {
			document.Operations[0].Title = string([]byte{0xff})
		}},
		{name: "relative document href", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *catalog.DocumentDirectoryV1, href *string, _ *map[string]struct{}) {
			*href = "kubernetes/documents/core-v1/"
		}},
		{name: "unknown open group", mutate: func(_ *catalog.DetailRecordV1, _ *domain.Operation, _ *catalog.DocumentDirectoryV1, _ *string, groups *map[string]struct{}) {
			*groups = map[string]struct{}{"group-000000000000": {}}
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			detail, operation, document, documentHref := operationNavigationFixture()
			groups := map[string]struct{}(nil)
			test.mutate(&detail, &operation, &document, &documentHref, &groups)
			fragment, err := PrepareOperationNavigation(detail, operation, document, documentHref, groups)
			if err == nil || !reflect.DeepEqual(fragment, OperationNavigationFragment{}) {
				t.Fatalf("PrepareOperationNavigation() = (%#v, %v), want zero fragment and error", fragment, err)
			}
		})
	}
}

func TestPrepareOperationNavigationAcceptsTerminalWhitespaceInDirectoryTitle(t *testing.T) {
	t.Parallel()

	detail, operation, document, documentHref := operationNavigationFixture()
	document.Operations[1].Title += "\n"
	if _, err := PrepareOperationNavigation(detail, operation, document, documentHref, nil); err != nil {
		t.Fatalf("PrepareOperationNavigation rejected normalized directory title: %v", err)
	}
}

func TestPreparedOperationNavigationCopiesRenderInputs(t *testing.T) {
	t.Parallel()

	detail, operation, document, documentHref := operationNavigationFixture()
	fragment, err := PrepareOperationNavigation(detail, operation, document, documentHref, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Tags[0].Value = "changed"
	operation.Tags[0] = "changed"
	document.Operations[0].Title = "changed"
	document.Operations[3].Method = "PATCH"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared navigation bytes changed after input mutation\nwant=%s\n got=%s", want, got)
	}
}

func TestPrepareOperationNavigationRejectsOversizedOutput(t *testing.T) {
	t.Parallel()

	detail, operation, document, documentHref := operationNavigationFixture()
	document.Operations[0].Title = strings.Repeat("x", maximumHTMLFragmentBytes)
	fragment, err := PrepareOperationNavigation(detail, operation, document, documentHref, nil)
	if err == nil || !reflect.DeepEqual(fragment, OperationNavigationFragment{}) {
		t.Fatalf("PrepareOperationNavigation() = (%#v, %v), want zero fragment and error", fragment, err)
	}
}

func TestOperationNavigationRejectsZeroFragment(t *testing.T) {
	t.Parallel()

	body, err := (OperationNavigationFragment{}).Bytes(context.Background())
	if err == nil || len(body) != 0 {
		t.Fatalf("zero operation-navigation fragment Bytes() = (%d bytes, %v), want zero bytes and error", len(body), err)
	}
}

func operationNavigationFixture() (catalog.DetailRecordV1, domain.Operation, catalog.DocumentDirectoryV1, string) {
	ids := []domain.DetailID{
		domain.DetailID("detail-sha256-" + strings.Repeat("a", 64)),
		domain.DetailID("detail-sha256-" + strings.Repeat("b", 64)),
		domain.DetailID("detail-sha256-" + strings.Repeat("c", 64)),
		domain.DetailID("detail-sha256-" + strings.Repeat("d", 64)),
	}
	detail := catalog.DetailRecordV1{ID: ids[1], Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(ids[1]), Anchor: string(ids[1]), HeadingID: string(ids[1]), HeadingLevel: 2,
		Href:    "documents/core-v1/?selected=" + string(ids[1]) + "#" + string(ids[1]),
		Heading: "Get Pod", Method: "GET", Path: "/api/v1/pods/{name}", Summary: "Get Pod",
		Description: "Gets a pod.", Tags: []projection.TextRecord{{Ordinal: 0, ID: "core", Value: "core"}},
	}}
	operation := domain.Operation{
		ID: "getCoreV1Pod", Anchor: string(ids[1]), Title: "Get Pod", Method: "GET", Path: "/api/v1/pods/{name}",
		Summary: "Get Pod", Description: "Gets a pod.", Tags: []string{"core"},
	}
	document := catalog.DocumentDirectoryV1{Key: "core-v1", Operations: []catalog.OperationDirectoryV1{
		{DetailID: ids[0], OperationID: "listCoreV1Pod", Method: "GET", Path: "/api/v1/pods", Title: "<List> Pods", Href: "core-v1/?selected=" + string(ids[0]) + "#" + string(ids[0]), DetailChild: "details/core.json", Tags: []string{"core"}},
		{DetailID: ids[1], OperationID: operation.ID, Method: operation.Method, Path: operation.Path, Title: operation.Summary, Description: operation.Description, Tags: append([]string(nil), operation.Tags...), Href: "core-v1/?selected=" + string(ids[1]) + "#" + string(ids[1]), DetailChild: "details/core.json"},
		{DetailID: ids[2], OperationID: "listAppsV1Deployment", Method: "GET", Path: "/apis/apps/v1/deployments", Title: "Other group", Href: "core-v1/?selected=" + string(ids[2]) + "#" + string(ids[2]), DetailChild: "details/core.json", Tags: []string{"apps"}},
		{DetailID: ids[3], OperationID: "deleteCoreV1Pod", Method: "DELETE", Path: "/api/v1/pods/{name}", Title: "Delete Pod", Href: "core-v1/?selected=" + string(ids[3]) + "#" + string(ids[3]), DetailChild: "details/core.json", Tags: []string{"core"}},
	}}
	return detail, operation, document, "/kubernetes/documents/core-v1/"
}
