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
	for _, want := range []string{"Kubernetes", "Documents", "Operations", "Schemas", `/kubernetes/documents/core-v1/`, `/kubernetes/documents/apps-v1/`, "API groups and versions"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if strings.Contains(body, `href="/core-v1/`) {
		t.Fatal("nested catalog emitted root-relative document href")
	}
}

func TestCatalogHeaderOmitsThemeSelectorButKeepsDarkMode(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	if strings.Contains(body, `id="manja-theme-trigger"`) || strings.Contains(body, `aria-label="Theme"`) {
		t.Fatal("catalog header still renders theme selector")
	}
	if !strings.Contains(body, `id="darkModeToggleBtn"`) {
		t.Fatal("catalog header removed dark mode toggle")
	}
}

func TestCatalogShellProvidesOneResponsiveSidebarWithMobileDrawerControls(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	for _, want := range []string{
		`x-data="{ catalogNavOpen: false }"`,
		`aria-label="Open API sections"`,
		`aria-controls="catalog-navigation"`,
		`x-bind:aria-expanded="catalogNavOpen.toString()"`,
		`id="catalog-navigation"`,
		`x-bind:style="catalogNavOpen ? &#39;display: block&#39; : &#39;&#39;"`,
		`x-trap.noscroll="catalogNavOpen"`,
		`aria-label="Close API sections"`,
		`data-catalog-navigation-backdrop="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("responsive catalog shell missing %q", want)
		}
	}
	if strings.Count(body, `id="catalog-sidebar-groups"`) != 1 {
		t.Fatal("catalog rendered duplicate sidebar trees")
	}
}

func TestCatalogShellProvidesClientFirstSearchModalWithServerFallback(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	for _, want := range []string{
		`src="/manja-assets/catalog-search.js"`,
		`id="catalog-search-dialog"`,
		`role="dialog"`,
		`x-data="manjaCatalogSearch($el)"`,
		`data-search-child-base="/kubernetes/snapshots/snapshot-sha256-`,
		`/search-data/"`,
		`data-search-directory-path="search/directory.json"`,
		`data-search-directory-sha256="`,
		`data-search-fallback-url="/kubernetes/search.json"`,
		`x-on:keydown.window="handleWindowKey($event)"`,
		`x-on:keydown.escape.prevent="closeSearch()"`,
		`Recently visited`,
		`id="catalog-search-current-visit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog search modal missing %q", want)
		}
	}
	if strings.Contains(body, `window.location.assign(href)`) && strings.Contains(body, `data-catalog-search-shortcut`) {
		t.Fatal("catalog Ctrl+K shortcut still navigates to a separate page")
	}
}

func TestCatalogHeaderUsesSearchableGoshtosoDocumentCombobox(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	for _, want := range []string{
		`data-combobox`,
		`id="catalog-document"`,
		`role="combobox"`,
		`data-combobox-search`,
		`hx-get="/_manja/catalog/document-combobox/options"`,
		`hx-trigger="click once"`,
		`core-v1`,
		`apps-v1`,
		`combobox:change`,
		`window.location.assign(value)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog document combobox missing %q", want)
		}
	}
	for _, unwanted := range []string{`<select id="catalog-document"`, `>Open</button>`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("catalog document combobox retains obsolete control %q", unwanted)
		}
	}
	if strings.Contains(body, `data-combobox-option`) {
		t.Fatal("catalog overview eagerly renders document options before the combobox opens")
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
		Documents:    []CatalogDocumentOption{{Key: "core-v1", Label: "core-v1", Href: "/kubernetes/documents/core-v1/"}, {Key: "apps-v1", Label: "apps-v1", Href: "/kubernetes/documents/apps-v1/"}},
		DownloadHref: "/kubernetes/catalog.json", SearchHref: "/kubernetes/search",
		SearchChildBase: "/kubernetes/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/search-data/",
		SearchDirectoryPath: "search/directory.json", SearchDirectoryLength: 42, SearchDirectorySHA256: strings.Repeat("c", 64),
	}
}
