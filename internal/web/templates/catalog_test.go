package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestCatalogPageRendersOverviewCountsAndMountAwareDocuments(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{"Kubernetes", "Documents", "Operations", "Schemas", `/kubernetes/core-v1/`, `/kubernetes/apps-v1/`, "API groups and versions"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if strings.Contains(body, `href="/core-v1/`) {
		t.Fatal("nested catalog emitted root-relative document href")
	}
}

func TestCatalogDocumentRendersOnlyExpandedGroupAndSelectedVisibleAnchor(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	detailID := document.Operations[0].DetailID
	data.Selected = &catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "List Pods", Method: "GET", Path: "/api/v1/pods",
	}}
	data.Groups = []CatalogSidebarGroupData{
		{ID: "group-core", Label: "core/v1", Href: "/kubernetes/core-v1/?group=group-core", Count: 2, Open: true, Items: []CatalogSidebarItemData{{ID: "sidebar-one", Label: "List Pods", Href: "/kubernetes/core-v1/?selected=" + string(detailID) + "#" + string(detailID), Method: "GET", Active: true}}},
		{ID: "group-schemas", Label: "Schemas", Href: "/kubernetes/core-v1/?group=group-schemas", Count: 500},
	}
	body := renderCatalogTemplate(t, data)
	if strings.Count(body, `href="/kubernetes/core-v1/?selected=`) != 1 {
		t.Fatalf("selected link was duplicated across shell variants")
	}
	if !strings.Contains(body, `id="`+string(detailID)+`"`) || !strings.Contains(body, "List Pods") {
		t.Fatal("selected detail has no visible target")
	}
	if strings.Contains(body, "Schema item that must stay lazy") {
		t.Fatal("collapsed group materialized hidden items")
	}
	if len(body) > 512<<10 || strings.Count(body, "<") > 2500 {
		t.Fatalf("initial page bounds = %d bytes, %d elements", len(body), strings.Count(body, "<"))
	}
}

func renderCatalogTemplate(t *testing.T, data CatalogPageData) string {
	t.Helper()
	var output bytes.Buffer
	if err := CatalogPage(data).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func catalogTemplateFixture() CatalogPageData {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	directory := catalog.CatalogArtifactV1{
		SchemaVersion: 1, CatalogID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1", SearchChild: "search/directory.json",
		Branding: catalog.BrandingV1{DisplayName: "Manja"},
		Documents: []catalog.DocumentDirectoryV1{
			{Key: "core-v1", Title: "Kubernetes Core v1", SourcePath: "api/openapi.json", Operations: []catalog.OperationDirectoryV1{{DetailID: detailID, Title: "List Pods", Method: "GET", Path: "/api/v1/pods"}}, Schemas: make([]catalog.SchemaDirectoryV1, 500)},
			{Key: "apps-v1", Title: "Kubernetes Apps v1", SourcePath: "apis/apps/openapi.json"},
		},
	}
	return CatalogPageData{
		Mount: "/kubernetes", SnapshotID: catalog.SnapshotID("snapshot-sha256-" + strings.Repeat("b", 64)), Directory: directory,
		Documents:    []CatalogDocumentOption{{Key: "core-v1", Label: "Kubernetes Core v1", Href: "/kubernetes/core-v1/"}, {Key: "apps-v1", Label: "Kubernetes Apps v1", Href: "/kubernetes/apps-v1/"}},
		DownloadHref: "/kubernetes/catalog.json", SearchHref: "/kubernetes/search",
	}
}
