package templates

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/araihu/goshtoso/components/badge"
	"github.com/araihu/goshtoso/components/icon"
	"github.com/araihu/goshtoso/components/icon/heroicons"
	"github.com/araihu/goshtoso/components/sidebar"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	localrender "github.com/araihu/manja/internal/localdocs/render"
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
	if !strings.Contains(body, `aria-hidden="true"`) || !strings.Contains(body, `fill="var(--color-primary)"`) || !strings.Contains(body, `aria-label="Manja"`) {
		t.Fatal("catalog header does not use the theme-aware inline Manja mark")
	}
	if strings.Contains(body, `src="/manja-assets/manja-mark.svg"`) {
		t.Fatal("catalog header still depends on an OS-themed external Manja mark")
	}
}

func TestCatalogOrganizationNavigationRendersCatalogAndSpecSections(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	data.Mount = "/"
	data.OrganizationNav = CatalogOrganizationNavData{
		Visible: true,
		Catalogs: []CatalogOrganizationItem{{
			ID: "catalog-kubernetes", Label: "Kubernetes", Description: "2 specs", Href: "/", Count: 2, Active: true,
		}},
		Specs: []CatalogOrganizationItem{{
			ID: "spec-kubernetes-core-v1", Label: "core-v1", Description: "Kubernetes", Href: "/documents/core-v1/",
		}},
	}
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{
		`id="catalog-organization-navigation"`,
		`aria-label="Catalogs and specs"`,
		`id="catalog-organization-section-catalogs"`,
		`id="catalog-organization-section-specs"`,
		`data-catalog-organization-item="catalog-kubernetes"`,
		`data-catalog-organization-item="spec-kubernetes-core-v1"`,
		`aria-current="page"`,
		`Search API...`,
		`heroicons.svg#hi-16-solid-rectangle-group`,
		`heroicons.svg#hi-16-solid-document-text`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("organization navigation missing %q", want)
		}
	}
	if strings.Count(body, `data-search-field`) != 1 {
		t.Fatalf("organization navigation rendered %d search fields, want 1", strings.Count(body, `data-search-field`))
	}
}

func TestCatalogShellUsesRouteSpecificNavigationLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       CatalogPageData
		label      string
		otherLabel string
	}{
		{
			name: "catalog root",
			data: func() CatalogPageData {
				data := catalogTemplateFixture()
				data.Mount = "/"
				data.OrganizationNav = CatalogOrganizationNavData{Visible: true}
				return data
			}(),
			label:      "Catalogs and specs",
			otherLabel: "API sections",
		},
		{
			name: "document",
			data: func() CatalogPageData {
				data := catalogTemplateFixture()
				document := data.Directory.Documents[0]
				data.Document = &document
				return data
			}(),
			label:      "API sections",
			otherLabel: "Catalogs and specs",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := renderCatalogTemplate(t, test.data)
			for _, want := range []string{
				`aria-label="Open ` + test.label + `"`,
				`aria-label="Close ` + test.label + `"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("%s shell missing %q", test.name, want)
				}
			}
			panel := regexp.MustCompile(`<aside[^>]*id="catalog-navigation"[^>]*>`).FindString(body)
			if panel == "" || !strings.Contains(panel, `aria-label="`+test.label+`"`) {
				t.Errorf("%s panel accessible label = %q, want %q", test.name, panel, test.label)
			}
			for _, reject := range []string{
				`aria-label="Open ` + test.otherLabel + `"`,
				`aria-label="Close ` + test.otherLabel + `"`,
			} {
				if strings.Contains(body, reject) {
					t.Errorf("%s shell retained wrong route label %q", test.name, reject)
				}
			}
		})
	}
}

func TestCatalogShellProvidesOneResponsiveSidebarWithMobileDrawerControls(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	body := renderCatalogTemplate(t, data)
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
	if strings.Contains(body, `>API sections</span>`) {
		t.Fatal("mobile drawer retains redundant API sections heading")
	}
	config := catalogSidebarConfig(data)
	if config.Logo != nil || config.LogoText != "" || config.LogoHref != "" {
		t.Fatalf("catalog sidebar retains redundant logo header: logo=%v text=%q href=%q", config.Logo != nil, config.LogoText, config.LogoHref)
	}
}

func TestCatalogOverviewOmitsEmptySidebarAndKeepsSearchInHeader(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	for _, unwanted := range []string{`id="catalog-navigation"`, `data-catalog-navigation-backdrop="true"`, `aria-controls="catalog-navigation"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("catalog overview retained empty sidebar shell %q", unwanted)
		}
	}
	searchAt := strings.Index(body, `data-catalog-header-search`)
	themeAt := strings.Index(body, `id="darkModeToggleBtn"`)
	if searchAt < 0 || themeAt < 0 || searchAt >= themeAt {
		t.Fatalf("catalog header search/theme order invalid: search=%d theme=%d", searchAt, themeAt)
	}
}

func TestCatalogShellProvidesGlobalSearchModal(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	for _, want := range []string{
		`src="/manja-assets/catalog-search.js"`,
		`data-search-field`,
		`data-catalog-header-search`,
		`data-search-id="catalog-search"`,
		`aria-controls="catalog-search-dialog"`,
		`Search API...`,
		`data-catalog-platform-shortcut`,
		`>Ctrl K</kbd>`,
		`Search operations, specs, and schemas`,
		`id="catalog-search-dialog"`,
		`role="dialog"`,
		`x-data="manjaCatalogSearch($el)"`,
		`data-search-child-base="/kubernetes/snapshots/snapshot-sha256-`,
		`/search-data/"`,
		`data-search-mount="/"`,
		`data-search-global="true"`,
		`data-search-context-mount="/kubernetes"`,
		`data-search-directory-path="search/directory.json"`,
		`data-search-directory-sha256="`,
		`data-search-fallback-url="/kubernetes/search.json"`,
		`x-on:keydown.window="handleWindowKey($event)"`,
		`x-on:goshtoso-search-open.window="if ($event.detail.id === 'catalog-search' && !open) openSearch()"`,
		`x-on:keydown.escape.prevent="closeSearch()"`,
		`Recently visited`,
		`id="catalog-search-current-visit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog search modal missing %q", want)
		}
	}
	if strings.Count(body, `data-search-field`) != 1 {
		t.Fatal("catalog rendered duplicate search fields")
	}
	if strings.Contains(body, `Ctrl/Cmd K`) {
		t.Fatal("catalog renders ambiguous shortcut label instead of platform enhancement")
	}
	if strings.Contains(body, `window.location.assign(href)`) && strings.Contains(body, `data-catalog-search-shortcut`) {
		t.Fatal("catalog Ctrl+K shortcut still navigates to a separate page")
	}
}

func TestCatalogSearchInputHasVisibleKeyboardFocusIndicator(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	input := regexp.MustCompile(`<input\b[^>]*id="catalog-search-input"[^>]*>`).FindString(body)
	if input == "" {
		t.Fatal("catalog search input not rendered")
	}
	for _, want := range []string{
		"focus-visible:rounded-radius",
		"focus-visible:outline-2",
		"focus-visible:outline-offset-2",
		"focus-visible:outline-primary",
		"dark:focus-visible:outline-primary-dark",
	} {
		if !strings.Contains(input, want) {
			t.Errorf("catalog search input missing keyboard-focus class %q: %s", want, input)
		}
	}
	for _, reject := range []string{
		"focus-visible:border-",
		"focus-visible:m-",
		"focus-visible:p-",
	} {
		if strings.Contains(input, reject) {
			t.Errorf("catalog search focus indicator uses layout-shifting class %q: %s", reject, input)
		}
	}
}

func TestCatalogOverviewUsesFilterableDocumentTable(t *testing.T) {
	t.Parallel()

	body := renderCatalogTemplate(t, catalogTemplateFixture())
	for _, want := range []string{
		`data-catalog-overview="true"`,
		`id="catalog-document-filter"`,
		`type="search"`,
		`x-model="filter"`,
		`Filter by name or version`,
		`<table`,
		`OpenAPI documents in this catalog`,
		`id="catalog-documents-table"`,
		`order_by=name`,
		`order_by=version`,
		`order_by=operations`,
		`order_by=schemas`,
		`Operations`,
		`Schemas`,
		`core-v1`,
		`apps-v1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog document table missing %q", want)
		}
	}
	if !strings.Contains(body, `rounded-full`) || !strings.Contains(body, `>CV<`) {
		t.Fatal("catalog document table does not render Goshtoso circular avatar initials")
	}
	for _, unwanted := range []string{`catalog-document-trigger`, `data-combobox`, `/_manja/catalog/document-combobox/options`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("catalog overview retains obsolete combobox control %q", unwanted)
		}
	}
	if !strings.Contains(body, `x-show="filter.trim()`) {
		t.Fatal("catalog document rows are not bound to the client-side filter")
	}
	for _, want := range []string{`data-catalog-document-row`, `No matching API documents.`, `aria-live="polite"`, `x-text=`} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog document filter missing %q", want)
		}
	}
}

func TestCatalogOverviewRendersOnlyDeclaredReadmeAndLicense(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	body := renderCatalogTemplate(t, data)
	for _, unwanted := range []string{`id="catalog-readme-heading"`, `id="catalog-license-heading"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("undeclared catalog metadata rendered %q", unwanted)
		}
	}

	data.CatalogReadme = "Kubernetes API documentation."
	data.CatalogLicense = CatalogOrganizationLicenseData{Name: "Apache-2.0", URL: "https://example.test/license"}
	body = renderCatalogTemplate(t, data)
	for _, want := range []string{`id="catalog-readme-heading"`, "Kubernetes API documentation.", `id="catalog-license-heading"`, "Apache-2.0", `href="https://example.test/license"`} {
		if !strings.Contains(body, want) {
			t.Errorf("declared catalog metadata missing %q", want)
		}
	}
}

func TestCatalogRootRendersStandaloneSpecsAndRootBreadcrumb(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	data.OrganizationRoot = true
	data.Mount = "/"
	data.SearchHref = "/"
	data.SearchJSONHref = "/search.json"
	data.SearchGlobal = true
	data.SearchMount = "/"
	data.SearchScopeLabel = "All catalogs"
	data.OrganizationNav = CatalogOrganizationNavData{
		Visible:  true,
		Catalogs: []CatalogOrganizationItem{{ID: "catalog-kubernetes", Label: "Kubernetes", Description: "2 specs", Href: "/kubernetes/"}},
		Specs:    []CatalogOrganizationItem{{ID: "spec-core-v1", Label: "core-v1", Description: "Kubernetes", Href: "/kubernetes/documents/core-v1/"}},
	}
	data.Organization = CatalogOrganizationPageData{
		Title:   "Manja",
		Readme:  "Fast, search-first OpenAPI documentation.",
		License: CatalogOrganizationLicenseData{Name: "No license declared"},
		Sources: []CatalogOrganizationSourceData{{Name: "Kubernetes definitions", Kind: "git", Location: "github.com/kubernetes/kubernetes", URL: "https://github.com/kubernetes/kubernetes"}},
	}
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{"Manja", "About", "Fast, search-first OpenAPI documentation.", "License", "No license declared", "Published sources", "Kubernetes definitions", `data-search-mount="/"`, `data-search-global="true"`, `data-search-fallback-url="/search.json"`} {
		if !strings.Contains(body, want) {
			t.Errorf("root overview missing %q", want)
		}
	}
	for _, duplicate := range []string{`id="organization-catalogs-heading"`, `id="organization-specs-heading"`} {
		if strings.Contains(body, duplicate) {
			t.Errorf("root main content duplicated sidebar section %q", duplicate)
		}
	}

	data.OrganizationRoot = false
	data.Document = &data.Directory.Documents[0]
	data.DocumentHref = "/kubernetes/documents/core-v1/"
	body = renderCatalogTemplate(t, data)
	if !strings.Contains(body, `href="/"`) || !strings.Contains(body, ">Catalogs</a>") {
		t.Fatal("nested catalog breadcrumb does not return to organization root")
	}
	for _, want := range []string{`href="/kubernetes/documents/core-v1/"`, `aria-current="page"`, `>core-v1</span>`} {
		if !strings.Contains(body, want) {
			t.Errorf("nested document breadcrumb missing %q", want)
		}
	}
}

func TestCatalogDocumentOverviewKeepsIdentityVersionAndDownloadInHeader(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	document.APIVersion = "v1"
	data.Document = &document
	data.DocumentHref = "/kubernetes/documents/core-v1/"
	data.DownloadHref = "/kubernetes/documents/core-v1/source.json"
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{
		`>OpenAPI document</p>`,
		`>core-v1</h1>`,
		`>v1</span>`,
		`href="/kubernetes/documents/core-v1/source.json"`,
		`>Download source</span>`,
		`sm:flex-row`,
		`sm:justify-between`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("document overview header missing %q", want)
		}
	}
	if !strings.Contains(body, document.SourcePath) {
		t.Fatalf("document overview omitted source path %q", document.SourcePath)
	}
}

func TestPreparedCatalogDocumentHeaderMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	document.APIVersion = "v1"
	document.SourceChild = "sources/core-v1.json"
	document.Overview.Description = "Document description."
	data.Document = &document
	data.DocumentHref = "/kubernetes/documents/core-v1/"
	data.DownloadHref = "/kubernetes/openapi/core-v1.json"

	legacy, err := renderLegacyCatalogDocumentHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := localrender.PrepareCatalogDocumentHeader(document, data.DocumentHref, data.DownloadHref)
	if err != nil {
		t.Fatal(err)
	}
	data.DocumentHeader = &fragment
	var delegated bytes.Buffer
	if err := catalogDocumentHeader(data).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy, delegated.Bytes()) {
		index := firstDifferentByte(legacy, delegated.Bytes())
		t.Fatalf("prepared document header changed SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy, index), nearbyBytes(delegated.Bytes(), index))
	}
}

func renderLegacyCatalogDocumentHeader(data CatalogPageData) ([]byte, error) {
	var output bytes.Buffer
	component := templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		document := *data.Document
		if _, err := io.WriteString(writer, `<header data-catalog-document-header class="mb-8 grid min-w-0 gap-4 border-b border-outline pb-8 dark:border-outline-dark"><div class="flex min-w-0 flex-wrap items-start justify-between gap-4"><div class="min-w-0"><p class="mb-2 text-sm font-semibold uppercase tracking-wide text-primary dark:text-primary-dark">OpenAPI document</p><div class="flex min-w-0 flex-wrap items-center gap-3"><h1 tabindex="-1" data-manja-settled-focus="true" class="manja-schema-title min-w-0 break-words font-title text-3xl font-bold text-on-surface-strong sm:text-4xl dark:text-on-surface-dark-strong" title="`); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, templ.EscapeString(catalogDocumentTitle(document))+`">`+templ.EscapeString(catalogDocumentTitle(document))+`</h1>`); err != nil {
			return err
		}
		if version := catalogDocumentVersionLabel(document); version != "" {
			if err := badge.Badge(badge.Config{Label: version, Tone: badge.ToneSecondary, Appearance: badge.AppearanceSoft, Size: badge.SizeSM, RootClass: "shrink-0"}).Render(ctx, writer); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(writer, `</div></div><nav aria-label="Specification actions" class="flex flex-wrap items-center gap-2"><a href="`+templ.EscapeString(data.DownloadHref)+`" download class="inline-flex min-h-9 items-center justify-center gap-2 rounded-radius border border-outline px-3 text-sm font-semibold text-on-surface-strong transition hover:bg-surface-alt focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary dark:border-outline-dark dark:text-on-surface-dark-strong dark:hover:bg-surface-dark-alt dark:focus-visible:outline-primary-dark">`); err != nil {
			return err
		}
		if err := icon.Icon(icon.Config{SpriteURL: heroicons.SpriteURL, Symbol: heroicons.Icon16SolidArrowDownTray, Size: icon.SizeSM, Decorative: true}).Render(ctx, writer); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, `<span>Download source</span></a></nav></div>`); err != nil {
			return err
		}
		if document.Overview.Description != "" {
			if _, err := io.WriteString(writer, `<p class="max-w-[70ch] whitespace-pre-wrap text-base leading-7 text-on-surface-muted dark:text-on-surface-dark-muted">`+templ.EscapeString(document.Overview.Description)+`</p>`); err != nil {
				return err
			}
		}
		if err := catalogSourceStatus(data).Render(ctx, writer); err != nil {
			return err
		}
		_, err := io.WriteString(writer, `</header>`)
		return err
	})
	if err := component.Render(context.Background(), &output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func TestCatalogDocumentOverviewRendersOnlyDeclaredOpenAPIInfo(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	data.DocumentHref = "/kubernetes/documents/core-v1/"
	body := renderCatalogTemplate(t, data)
	for _, unwanted := range []string{`aria-label="OpenAPI information"`, `>Contact</dt>`, `>License</dt>`, `>Terms of service</dt>`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("undeclared OpenAPI info rendered %q", unwanted)
		}
	}

	document.Overview.Description = "This is an example server for a pet store."
	document.Overview.TermsOfService = "https://example.test/terms"
	document.Overview.Contact = projection.Contact{Name: "API Support", URL: "https://example.test/support", Email: "support@example.test"}
	document.Overview.License = projection.License{Name: "Apache 2.0", URL: "https://example.test/license", Identifier: "Apache-2.0"}
	data.Document = &document
	body = renderCatalogTemplate(t, data)
	for _, want := range []string{
		`aria-label="OpenAPI information"`,
		"This is an example server for a pet store.",
		">Contact</dt>", "API Support", `href="https://example.test/support"`, `href="mailto:support@example.test"`,
		">License</dt>", "Apache 2.0", "Apache-2.0", `href="https://example.test/license"`,
		">Terms of service</dt>", `href="https://example.test/terms"`, ">View terms</a>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("declared OpenAPI info missing %q", want)
		}
	}
}

func TestCatalogDocumentSidebarGroupsOperationsUnderOnePathsItem(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	data.Groups = []CatalogSidebarGroupData{
		{ID: "group-actions", Kind: "operations", Label: "actions", Href: "?group=group-actions", Count: 72},
		{ID: "group-activity", Kind: "operations", Label: "activity", Href: "?group=group-activity", Count: 31},
		{ID: "schemas", Kind: "schemas", Label: "Schemas", Href: "?group=schemas", Count: 3},
	}
	config := catalogSidebarConfig(data)
	if len(config.Items) != 4 {
		t.Fatalf("sidebar item count = %d, want 4", len(config.Items))
	}
	specOverview := config.Items[1]
	if specOverview.ID != "spec-overview" || specOverview.Icon == nil {
		t.Fatalf("spec overview = %#v, want book icon in sidebar", specOverview)
	}
	paths := config.Items[2]
	if paths.ID != "catalog-paths" || paths.Label != "Paths" || paths.Icon == nil {
		t.Fatalf("paths parent = %#v, want visible Paths item with one icon", paths)
	}
	if paths.Href != data.DocumentHref || len(paths.Items) != 2 {
		t.Fatalf("paths parent href/items = %q/%d, want %q/2", paths.Href, len(paths.Items), data.DocumentHref)
	}
	for _, group := range paths.Items {
		if group.Icon != nil {
			t.Fatalf("operation group %q retained a repeated icon", group.Label)
		}
	}
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{
		`heroicons.svg#hi-16-solid-chevron-left`,
		`heroicons.svg#hi-16-solid-book-open`,
		`heroicons.svg#hi-16-solid-code-bracket`,
		`heroicons.svg#hi-16-solid-cube`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("document sidebar missing Goshtoso icon %q", want)
		}
	}
	if got := strings.Count(body, `heroicons.svg#hi-16-solid-code-bracket`); got != 1 {
		t.Fatalf("code bracket icon count = %d, want 1", got)
	}
	if strings.Contains(body, `class="mb-2 mt-8`) && strings.Contains(body, `>Kubernetes</h3>`) {
		t.Fatal("document sidebar retained redundant catalog title")
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
		{ID: "group-core", Kind: "operations", Label: "core/v1", Href: "/kubernetes/core-v1/?group=group-core", Count: 2, Open: true, Items: []CatalogSidebarItemData{{ID: "sidebar-one", Label: "List Pods", Href: "/kubernetes/core-v1/?selected=" + string(detailID) + "#" + string(detailID), Method: "GET", Active: true}}},
		{ID: "group-schemas", Kind: "schemas", Label: "Schemas", Href: "/kubernetes/core-v1/?group=group-schemas", Count: 500},
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

func TestCatalogSidebarUsesMethodHierarchyAndOverflowHooks(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	data.Groups = []CatalogSidebarGroupData{{
		ID: "group-core", Kind: "operations", Label: "core/v1", Href: "/kubernetes/documents/core-v1/?group=group-core", Count: 6,
		Items: []CatalogSidebarItemData{
			{ID: "get", Label: "Get resources", Method: "GET", Href: "#get"},
			{ID: "post", Label: "Create resource", Method: "POST", Href: "#post"},
			{ID: "delete", Label: "Delete resource", Method: "DELETE", Href: "#delete"},
			{ID: "put", Label: "Replace resource", Method: "PUT", Href: "#put"},
			{ID: "patch", Label: "Update resource", Method: "PATCH", Href: "#patch"},
			{ID: "options", Label: "List a very long operation name that must be discoverable in a tooltip", Method: "OPTIONS", Href: "#options"},
		},
	}}
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{
		`data-catalog-group-control="true"`,
		`data-catalog-sidebar-operation="true"`,
		`catalog-method-get`,
		`catalog-method-post`,
		`catalog-method-delete`,
		`catalog-method-warning`,
		`catalog-method-neutral`,
		`data-catalog-sidebar-overflow-tooltip="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog sidebar hierarchy missing %q", want)
		}
	}
	if got := strings.Count(body, `data-catalog-sidebar-operation="true"`); got != 6 {
		t.Fatalf("catalog sidebar operation hooks = %d, want 6", got)
	}
}

func TestCatalogSidebarItemsUseTargetedMainNavigation(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	detailID := document.Operations[0].DetailID
	href := "/kubernetes/documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID)
	data.Groups = []CatalogSidebarGroupData{{
		ID: "group-core", Kind: "operations", Label: "core/v1", Href: "/kubernetes/documents/core-v1/?group=group-core", Count: 1, Open: true,
		Items: []CatalogSidebarItemData{{ID: "sidebar-list-pods", Label: "List Pods", Method: "GET", Href: href}},
	}}

	body := renderCatalogTemplate(t, data)
	link := regexp.MustCompile(`<a[^>]*id="catalog-sidebar-item-sidebar-list-pods"[^>]*>`).FindString(body)
	if link == "" {
		t.Fatalf("catalog operation link missing:\n%s", body)
	}
	for _, want := range []string{
		`href="` + href + `"`,
		`hx-get="` + href + `"`,
		`hx-target="#catalog-main-content"`,
		`hx-select="#catalog-main-content"`,
		`hx-swap="outerHTML show:#main-content:top"`,
		`hx-push-url="true"`,
		`data-manja-sidebar-nav="true"`,
	} {
		if !strings.Contains(link, want) {
			t.Errorf("catalog operation link missing %q: %s", want, link)
		}
	}
	config := catalogSidebarConfig(data)
	var overview sidebar.Item
	for _, item := range config.Items {
		if item.ID == "spec-overview" {
			overview = item
			break
		}
	}
	if overview.ID == "" {
		t.Fatal("spec overview sidebar item missing")
	}
	for name, want := range catalogMainNavigationAttrs(data.DocumentHref) {
		if got := overview.LinkAttrs[name]; got != want {
			t.Errorf("spec overview %s = %#v, want %#v", name, got, want)
		}
	}
}

func TestCatalogSidebarSelectedItemHasDeterministicScrollTarget(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	data.Groups = []CatalogSidebarGroupData{{
		ID: "group-pulls", Kind: "operations", Label: "pulls", Href: "/kubernetes/documents/core-v1/?group=group-pulls", Count: 1, Open: true,
		Items: []CatalogSidebarItemData{{ID: "sidebar-selected-operation", Label: "Get pull request", Method: "GET", Href: "#selected", Active: true}},
	}}
	body := renderCatalogTemplate(t, data)
	for _, want := range []string{
		`data-catalog-navigation-trigger="true"`,
		`id="catalog-sidebar-item-sidebar-selected-operation"`,
		`data-catalog-sidebar-item="true"`,
		`data-catalog-sidebar-selected="true"`,
		`window.manjaCatalogScrollSidebarSelection`,
		`navigation.querySelector(".sidebar-scroll")`,
		`panel.scrollTop = Math.max(0, Math.min(maxTop, targetTop));`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog sidebar selection behavior missing %q", want)
		}
	}
	if got := strings.Count(body, `data-catalog-sidebar-selected="true"`); got != 1 {
		t.Fatalf("selected sidebar markers = %d, want 1", got)
	}
}

func TestCatalogSidebarGroupControlsExposeDisclosureState(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	data.Groups = []CatalogSidebarGroupData{
		{
			ID: "group-open", Kind: "operations", Label: "Operations", Href: "/documents/core-v1/?group=group-open", Count: 2,
			Open: true, Items: []CatalogSidebarItemData{{ID: "operation-one", Label: "List pods", Href: "#operation-one"}},
		},
		{ID: "group-closed", Kind: "schemas", Label: "Schemas", Href: "/documents/core-v1/?group=group-closed", Count: 3},
	}
	body := renderCatalogTemplate(t, data)
	if got := strings.Count(body, `data-catalog-group-control="true"`); got != 2 {
		t.Fatalf("catalog group controls = %d, want 2", got)
	}
	for _, want := range []string{
		`aria-expanded="true"`,
		`aria-expanded="false"`,
		`aria-controls="catalog-sidebar-groups"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog group control missing %q", want)
		}
	}
}

func TestCatalogOperationReusesRichPublicEndpointRenderer(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	detailID := document.Operations[0].DetailID
	data.Selected = &catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2, Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods",
		Summary: "Create Pod", Description: "Creates a Pod.",
	}}
	data.OperationView = &domain.Operation{
		ID: string(detailID), Anchor: string(detailID), Title: "Create Pod", Summary: "Create Pod", Description: "Creates a Pod.", Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods",
		Parameters: []domain.OperationParameter{
			{Name: "namespace", In: "path", Required: true, Description: "Namespace name.", Schema: domain.SchemaSummary{Type: "string"}},
			{Name: "dryRun", In: "query", Description: "Dry-run directive.", Schema: domain.SchemaSummary{Type: "string"}},
			{Name: "accept", In: "header", Required: true, Description: "Accepted response media type.", Schema: domain.SchemaSummary{Type: "string"}},
		},
		RequestBody: &domain.OperationRequestBody{Required: true, MediaTypes: []domain.OperationMediaType{{
			ContentType: "application/json", Example: "{\n  \"kind\": \"Pod\"\n}", ExampleProvided: true,
			Schema: domain.SchemaSummary{Name: "Pod", Type: "object", JSON: `{\"type\":\"object\"}`, Properties: []domain.SchemaProperty{{Name: "kind", Required: true, Schema: domain.SchemaSummary{Type: "string"}}}},
		}}},
		Responses: []domain.OperationResponse{{Status: "201", Description: "Created", MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object"}}}}},
		Security:  []domain.OperationSecurity{{Name: "BearerToken"}},
		Snippets:  []domain.RequestSnippet{{Label: "cURL", Language: "shell", Code: "curl --request POST /api/v1/namespaces/default/pods"}},
	}
	prepareCatalogOperationHeader(t, &data)

	body := renderCatalogTemplate(t, data)
	for _, want := range []string{
		`class="manja-endpoint-shell-layout"`,
		`aria-label="Request"`,
		"Path Parameters",
		"Query Parameters",
		"Header Parameters",
		`aria-label="Request body"`,
		"application/json",
		"Request body JSON",
		`class="manja-endpoint-responses-section grid gap-5"`,
		"Request Sample: Shell / cURL",
		"curl --request POST",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rich catalog operation missing %q", want)
		}
	}
}

func TestPreparedOperationHeaderMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	data.PageMarkdownHref = "/kubernetes/documents/core-v1/page.md?selected=operation"
	detailID := document.Operations[0].DetailID
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID),
		Href:      "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
		Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods", Summary: "Create Pod",
		Description: "Creates a Pod.", Deprecated: true,
	}}
	operation := domain.Operation{
		ID: "createCoreV1NamespacedPod", Anchor: string(detailID), Title: "Create Pod",
		Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods", Summary: "Create Pod",
		Description: "Creates a Pod.", Deprecated: true,
	}
	fragment, err := localrender.PrepareOperationHeader(detail, operation, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}

	data.Selected = &detail
	data.OperationView = &operation
	data.OperationHeader = &fragment
	var rendered bytes.Buffer
	if err := catalogOperationDetail(data).Render(context.Background(), &rendered); err != nil {
		t.Fatal(err)
	}
	legacy := rendered.Bytes()
	start := bytes.Index(legacy, []byte(`<header class="mb-8 min-w-0 border-b border-outline pb-6 dark:border-outline-dark" data-public-page-header="true">`))
	if start < 0 {
		t.Fatalf("SSR operation header absent: %s", legacy)
	}
	end := bytes.Index(legacy[start:], []byte(`</header>`))
	if end < 0 {
		t.Fatalf("SSR operation header unclosed: %s", legacy[start:])
	}
	legacy = legacy[start : start+end+len(`</header>`)]
	actions := copyPageAction(operation.Anchor, data.PageMarkdownHref)
	provenance := catalogProvenance(data, true)
	shared, err := fragment.Bytes(context.Background(), actions, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(shared, legacy) {
		t.Fatalf("prepared and SSR operation headers differ:\nprepared=%s\nSSR=%s", shared, legacy)
	}
}

func TestPreparedSchemaDetailHeaderMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	document.APIVersion = "v1"
	data.Document = &document
	data.DownloadHref = "/kubernetes/openapi/core-v1.json"
	data.PageMarkdownHref = "/kubernetes/documents/core-v1/page.md?selected=schema"
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "schema", Schema: &projection.SchemaDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Pod schema", HeadingLevel: 2, Description: "Schema description.", SchemaRef: 7,
	}}
	node, err := localrender.PrepareSchemaNode(detail, projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description}
	fragment, err := localrender.PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	data.Selected = &detail
	data.SchemaView = &schema
	data.SchemaNode = &node

	legacy, err := renderLegacySchemaDetailHeader(data, *detail.Schema, &node)
	if err != nil {
		t.Fatal(err)
	}
	data.SchemaDetailHeader = &fragment
	var delegatedOutput bytes.Buffer
	if err := catalogSchemaDetail(data).Render(context.Background(), &delegatedOutput); err != nil {
		t.Fatal(err)
	}
	delegated := extractSchemaDetailHeader(t, delegatedOutput.Bytes())
	if !bytes.Equal(delegated, legacy) {
		t.Fatalf("prepared and SSR schema detail headers differ:\nprepared=%s\nSSR=%s", delegated, legacy)
	}
}

func TestPreparedSchemaDetailExampleMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	document.APIVersion = "v1"
	data.Document = &document
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("e", 64))
	example := `{"type":"object","description":"<pod>"}`
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "schema", Schema: &projection.SchemaDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Pod schema", HeadingLevel: 2, Description: "Schema description.", SchemaRef: 7,
		ExampleSchemaJSON: example,
	}}
	node, err := localrender.PrepareSchemaNode(detail, projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description, Example: domain.SchemaExample{JSON: example}}
	fragment, err := localrender.PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	data.Selected = &detail
	data.SchemaView = &schema
	data.SchemaNode = &node
	data.SchemaDetailExample = &fragment

	legacy, err := renderLegacySchemaDetailExample(example)
	if err != nil {
		t.Fatal(err)
	}
	var delegatedOutput bytes.Buffer
	if err := catalogSchemaDetail(data).Render(context.Background(), &delegatedOutput); err != nil {
		t.Fatal(err)
	}
	delegated := extractSchemaDetailExample(t, delegatedOutput.Bytes())
	if !bytes.Equal(delegated, legacy) {
		t.Fatalf("prepared and SSR schema detail examples differ:\nprepared=%s\nSSR=%s", delegated, legacy)
	}
}

func TestPreparedSchemaDetailBodyMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	document.APIVersion = "v1"
	data.Document = &document
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("f", 64))
	example := `{"type":"object","description":"<pod>"}`
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "schema", Schema: &projection.SchemaDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Pod schema", HeadingLevel: 2, Description: "Schema description.", SchemaRef: 7,
		ExampleSchemaJSON: example,
	}}
	node, err := localrender.PrepareSchemaNode(detail, projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description, Example: domain.SchemaExample{JSON: example}}
	exampleFragment, err := localrender.PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	bodyFragment, err := localrender.PrepareSchemaDetailBody(detail, schema, document, &node, &exampleFragment)
	if err != nil {
		t.Fatal(err)
	}
	data.Selected = &detail
	data.SchemaView = &schema
	data.SchemaNode = &node
	data.SchemaDetailExample = &exampleFragment

	var legacy, delegated bytes.Buffer
	if err := catalogSchemaDetail(data).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	data.SchemaDetailBody = &bodyFragment
	if err := catalogSchemaDetail(data).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("prepared schema detail body changed complete SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedSchemaDetailBodyWithoutExampleMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("f", 64))
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "schema", Schema: &projection.SchemaDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Pod schema", HeadingLevel: 2, Description: "Schema description.", SchemaRef: 7,
	}}
	node, err := localrender.PrepareSchemaNode(detail, projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description}
	bodyFragment, err := localrender.PrepareSchemaDetailBody(detail, schema, document, &node, nil)
	if err != nil {
		t.Fatal(err)
	}
	data.Selected = &detail
	data.SchemaView = &schema
	data.SchemaNode = &node

	var legacy, delegated bytes.Buffer
	if err := catalogSchemaDetail(data).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	data.SchemaDetailBody = &bodyFragment
	if err := catalogSchemaDetail(data).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("prepared schema detail body without example changed complete SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedSchemaDetailMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	document.APIVersion = "v1"
	data.Document = &document
	data.DownloadHref = "/kubernetes/openapi/core-v1.json"
	data.PageMarkdownHref = "/kubernetes/documents/core-v1/page.md?selected=schema"
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	example := `{"type":"object","description":"<pod>"}`
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "schema", Schema: &projection.SchemaDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "Pod schema", HeadingLevel: 2, Description: "Schema description.", SchemaRef: 7,
		ExampleSchemaJSON: example,
	}}
	node, err := localrender.PrepareSchemaNode(detail, projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}, nil, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description, Example: domain.SchemaExample{JSON: example}}
	header, err := localrender.PrepareSchemaDetailHeader(detail, schema, document, &node)
	if err != nil {
		t.Fatal(err)
	}
	exampleFragment, err := localrender.PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := localrender.PrepareSchemaDetailBody(detail, schema, document, &node, &exampleFragment)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := localrender.PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err != nil {
		t.Fatal(err)
	}

	data.Selected = &detail
	data.SchemaView = &schema
	data.SchemaNode = &node
	data.SchemaDetailHeader = &header
	data.SchemaDetailExample = &exampleFragment
	data.SchemaDetailBody = &body
	var legacy, delegated bytes.Buffer
	if err := catalogSchemaDetail(data).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	data.SchemaDetail = &fragment
	if err := catalogSchemaDetail(data).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("prepared schema detail changed complete SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func renderLegacySchemaDetailExample(example string) ([]byte, error) {
	var output bytes.Buffer
	if err := codeExample("Root JSON Schema", "json", example).Render(context.Background(), &output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func extractSchemaDetailExample(t *testing.T, body []byte) []byte {
	t.Helper()
	marker := []byte(`<aside class="manja-schema-example-panel" aria-label="Root JSON Schema">`)
	start := bytes.Index(body, marker)
	if start < 0 {
		t.Fatalf("schema detail example absent: %s", body)
	}
	start += len(marker)
	relativeEnd := bytes.Index(body[start:], []byte(`</aside>`))
	if relativeEnd < 0 {
		t.Fatalf("schema detail example unclosed: %s", body[start:])
	}
	return body[start : start+relativeEnd]
}

func renderLegacySchemaDetailHeader(data CatalogPageData, schema projection.SchemaDetail, node *localrender.SchemaNodeFragment) ([]byte, error) {
	component := templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if _, err := io.WriteString(writer, `<header class="grid min-w-0 gap-4"><div class="flex flex-wrap items-center justify-between gap-3"><div class="flex min-w-0 flex-wrap items-center gap-2">`); err != nil {
			return err
		}
		if err := badge.Badge(badge.Config{Label: "Schema", Tone: badge.ToneSecondary, Appearance: badge.AppearanceSoft, Size: badge.SizeMD}).Render(ctx, writer); err != nil {
			return err
		}
		if version := catalogDocumentVersionLabel(*data.Document); version != "" {
			if err := badge.Badge(badge.Config{Label: version, Tone: badge.ToneDefault, Appearance: badge.AppearanceSoft, Size: badge.SizeSM}).Render(ctx, writer); err != nil {
				return err
			}
		}
		if node != nil && (node.Type() != "" || node.Format() != "") {
			if _, err := io.WriteString(writer, `<code class="rounded-radius bg-surface-alt px-3 py-1.5 text-sm text-on-surface-strong dark:bg-surface-dark-alt dark:text-on-surface-dark-strong">`+strings.TrimSpace(node.Type()+" "+node.Format())+`</code>`); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(writer, `</div><nav aria-label="Schema actions" class="flex flex-wrap items-center gap-2">`); err != nil {
			return err
		}
		if err := catalogSchemaDetailActions(data, schema.Anchor).Render(ctx, writer); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, `</nav></div><h1 tabindex="-1" data-manja-settled-focus="true" class="manja-schema-title font-title text-2xl font-bold text-on-surface-strong sm:text-3xl dark:text-on-surface-dark-strong">`+schema.Heading+`</h1>`); err != nil {
			return err
		}
		if schema.Description != "" {
			if _, err := io.WriteString(writer, `<p class="max-w-[70ch] whitespace-pre-wrap break-words text-on-surface-muted dark:text-on-surface-dark-muted">`+schema.Description+`</p>`); err != nil {
				return err
			}
		}
		if err := catalogSourceStatus(data).Render(ctx, writer); err != nil {
			return err
		}
		_, err := io.WriteString(writer, `</header>`)
		return err
	})
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func extractSchemaDetailHeader(t *testing.T, body []byte) []byte {
	t.Helper()
	start := bytes.Index(body, []byte(`<header class="grid min-w-0 gap-4">`))
	if start < 0 {
		t.Fatalf("schema detail header absent: %s", body)
	}
	relativeEnd := bytes.Index(body[start:], []byte(`</header>`))
	if relativeEnd < 0 {
		t.Fatalf("schema detail header unclosed: %s", body[start:])
	}
	end := start + relativeEnd + len(`</header>`)
	return body[start:end]
}

func TestPreparedOperationParametersMatchCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	for mask := 0; mask < 8; mask++ {
		mask := mask
		t.Run(fmt.Sprintf("path=%t/query=%t/header=%t", mask&4 != 0, mask&2 != 0, mask&1 != 0), func(t *testing.T) {
			t.Parallel()

			detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
			operation := domain.Operation{Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/api/v1/pods"}
			detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
				ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2,
				Method: "GET", Path: "/api/v1/pods",
			}}
			var nodes []projection.SchemaNode
			addParameter := func(name, location, description, example, schemaType, format string, required bool) {
				ordinal := uint32(len(operation.Parameters))
				operation.Parameters = append(operation.Parameters, domain.OperationParameter{
					Name: name, In: location, Required: required, Description: description, Example: example,
					Schema: parameterSummary(schemaType, format),
				})
				projected := projection.Parameter{
					Ordinal: ordinal, ID: parameterProjectionID(location, name), Name: name, In: location,
					Required: required, Description: description, SchemaRef: projection.SchemaRef(ordinal),
				}
				if example != "" {
					projected.Examples = []projection.Example{{Ordinal: 0, ID: "primary", Text: example, Provided: true}}
				}
				detail.Operation.Parameters = append(detail.Operation.Parameters, projected)
				nodes = append(nodes, projection.SchemaNode{Ordinal: ordinal, ID: "node-" + strings.ToLower(name), Type: schemaType, Format: format})
			}

			if mask&4 != 0 {
				addParameter("namespace", "path", "Namespace.", "", "string", "", true)
			}
			if mask&2 != 0 {
				addParameter("labels", "query", strings.Repeat("Filter labels. ", 20), "", "string", "", false)
			}
			if mask&1 != 0 {
				addParameter("X-Trace", "header", "", "trace-1", "string", "uuid", false)
			}
			// Cookie parameters keep the endpoint request renderable while remaining
			// deliberately outside this Path/Query/Header fragment contract.
			addParameter("session", "cookie", "", "", "boolean", "", false)

			fragment, err := localrender.PrepareOperationParameters(detail, operation, nodes)
			if err != nil {
				t.Fatal(err)
			}
			var originalPage, delegatedPage bytes.Buffer
			if err := endpointSection(operation, nil, "", PublicDocsOptions{}, OperationNavigationData{}).Render(context.Background(), &originalPage); err != nil {
				t.Fatal(err)
			}
			if err := endpointSection(operation, nil, "", PublicDocsOptions{OperationParameters: &fragment}, OperationNavigationData{}).Render(context.Background(), &delegatedPage); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(originalPage.Bytes(), delegatedPage.Bytes()) {
				index := firstDifferentByte(originalPage.Bytes(), delegatedPage.Bytes())
				t.Fatalf("delegated parameter fragment changed complete SSR endpoint bytes at byte %d:\noriginal=%q\ndelegated=%q", index, nearbyBytes(originalPage.Bytes(), index), nearbyBytes(delegatedPage.Bytes(), index))
			}
		})
	}
}

func TestPreparedOperationAuthorizationMatchesCompleteEndpointSSRBytes(t *testing.T) {
	t.Parallel()

	for securityCount := 0; securityCount <= 4; securityCount++ {
		securityCount := securityCount
		t.Run(fmt.Sprintf("security=%d", securityCount), func(t *testing.T) {
			t.Parallel()

			detailID := domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
			operation := domain.Operation{Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/api/v1/pods"}
			detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
				ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2,
				Method: "GET", Path: "/api/v1/pods",
			}}
			for index, security := range []domain.OperationSecurity{
				{Name: "bearer", Definition: domain.SecurityScheme{Name: "bearer", Type: "http", Description: "Bearer access.", Scheme: "bearer", BearerFormat: "JWT"}},
				{Name: "api-key", Definition: domain.SecurityScheme{Name: "api-key", Type: "apiKey", Description: "API key.", ParameterName: "X-API-Key", In: "header"}},
				{Name: "oauth", Scopes: []string{"pods:read", "pods:write"}, Definition: domain.SecurityScheme{Name: "oauth", Type: "oauth2", OpenIDConnectURL: "https://auth.example.test/.well-known/openid-configuration"}},
				{Name: "custom", Definition: domain.SecurityScheme{Name: "custom", Type: " ", Scheme: " ", BearerFormat: " ", OpenIDConnectURL: " "}},
			}[:securityCount] {
				operation.Security = append(operation.Security, security)
				projected := projection.SecurityRequirement{Ordinal: uint32(index), ID: security.Name, Name: security.Name, Definition: projection.SecurityScheme{
					Name: security.Definition.Name, Type: security.Definition.Type, Description: security.Definition.Description,
					ParameterName: security.Definition.ParameterName, In: security.Definition.In, Scheme: security.Definition.Scheme,
					BearerFormat: security.Definition.BearerFormat, OpenIDConnectURL: security.Definition.OpenIDConnectURL,
				}}
				for scopeIndex, scope := range security.Scopes {
					projected.Scopes = append(projected.Scopes, projection.TextRecord{Ordinal: uint32(scopeIndex), ID: authorizationProjectionID(scope), Value: scope})
				}
				detail.Operation.Security = append(detail.Operation.Security, projected)
			}

			fragment, err := localrender.PrepareOperationAuthorization(detail, operation)
			if err != nil {
				t.Fatal(err)
			}
			var originalPage, delegatedPage bytes.Buffer
			if err := endpointSection(operation, nil, "", PublicDocsOptions{}, OperationNavigationData{}).Render(context.Background(), &originalPage); err != nil {
				t.Fatal(err)
			}
			if err := endpointSection(operation, nil, "", PublicDocsOptions{OperationAuthorization: &fragment}, OperationNavigationData{}).Render(context.Background(), &delegatedPage); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(originalPage.Bytes(), delegatedPage.Bytes()) {
				index := firstDifferentByte(originalPage.Bytes(), delegatedPage.Bytes())
				t.Fatalf("delegated authorization changed complete SSR endpoint bytes at byte %d:\noriginal=%q\ndelegated=%q", index, nearbyBytes(originalPage.Bytes(), index), nearbyBytes(delegatedPage.Bytes(), index))
			}
		})
	}
}

func TestPreparedOperationRequestBodyMediaMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	for mediaCount := 0; mediaCount <= 3; mediaCount++ {
		mediaCount := mediaCount
		t.Run(fmt.Sprintf("media=%d", mediaCount), func(t *testing.T) {
			t.Parallel()

			detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
			documentHref := "/kubernetes/documents/core-v1/"
			schemaID := "detail-sha256-" + strings.Repeat("e", 64)
			operation := domain.Operation{
				Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/api/v1/pods",
				RequestBody: &domain.OperationRequestBody{Description: "Pod to create.", Required: true},
			}
			detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
				ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
				Method: "POST", Path: "/api/v1/pods", HasRequestBody: true,
				RequestBody: projection.RequestBody{Description: "Pod to create.", Required: true},
			}}
			var nodes []projection.SchemaNode
			if mediaCount >= 1 {
				operation.RequestBody.MediaTypes = append(operation.RequestBody.MediaTypes, domain.OperationMediaType{
					ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object", Properties: []domain.SchemaProperty{{Name: "kind", Required: true, Schema: domain.SchemaSummary{Type: "string"}}}},
					Example: `{"kind":"Pod"}`, ExampleProvided: true,
				})
				detail.Operation.RequestBody.MediaTypes = append(detail.Operation.RequestBody.MediaTypes, projection.MediaType{
					Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7,
					Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: `{"kind":"Pod"}`, Provided: true}},
				})
				nodes = append(nodes, projection.SchemaNode{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"})
			}
			if mediaCount >= 2 {
				operation.RequestBody.MediaTypes = append(operation.RequestBody.MediaTypes, domain.OperationMediaType{
					ContentType: "application/yaml", Schema: domain.SchemaSummary{Name: "Status", Type: "string", Enum: []string{"ready", "pending"}},
				})
				detail.Operation.RequestBody.MediaTypes = append(detail.Operation.RequestBody.MediaTypes, projection.MediaType{
					Ordinal: 1, ID: "application/yaml", ContentType: "application/yaml", SchemaRef: 8,
				})
				nodes = append(nodes, projection.SchemaNode{Ordinal: 8, ID: "node-status", Name: "Status", Type: "string", Enum: []string{"ready", "pending"}})
			}
			if mediaCount >= 3 {
				operation.RequestBody.MediaTypes = append(operation.RequestBody.MediaTypes, domain.OperationMediaType{
					ContentType: "application/problem+json", Schema: domain.SchemaSummary{
						Type: "array", Items: &domain.SchemaSummary{
							Type: "array", Items: &domain.SchemaSummary{Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}},
						},
					},
				})
				detail.Operation.RequestBody.MediaTypes = append(detail.Operation.RequestBody.MediaTypes, projection.MediaType{
					Ordinal: 2, ID: "application/problem+json", ContentType: "application/problem+json", SchemaRef: 9,
				})
				nodes = append(nodes,
					projection.SchemaNode{Ordinal: 9, ID: "node-array-root", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 10}}},
					projection.SchemaNode{Ordinal: 10, ID: "node-array-nested", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 11}}},
					projection.SchemaNode{Ordinal: 11, ID: "node-array-status", Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}},
				)
			}
			schemaLinks := map[string]string{
				"Pod":    documentHref + "?selected=" + schemaID + "#" + schemaID,
				"Status": documentHref + "?selected=" + schemaID + "#" + schemaID,
			}
			fragment, err := localrender.PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			baseOptions := PublicDocsOptions{
				SchemaLinks: schemaLinks, SchemaLinkTarget: "#catalog-main-content", SchemaLinkSelect: "#catalog-main-content", SchemaLinkSwap: "outerHTML show:#main-content:top",
			}
			var legacy, delegated bytes.Buffer
			if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
				t.Fatal(err)
			}
			baseOptions.OperationRequestBodyMedia = &fragment
			if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
				index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
				t.Fatalf("delegated request-body media summary changed complete SSR endpoint bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
			}
		})
	}
}

func TestPreparedOperationNavigationMatchesCompleteCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	ids := []domain.DetailID{
		domain.DetailID("detail-sha256-" + strings.Repeat("a", 64)),
		domain.DetailID("detail-sha256-" + strings.Repeat("b", 64)),
		domain.DetailID("detail-sha256-" + strings.Repeat("c", 64)),
	}
	detail := catalog.DetailRecordV1{ID: ids[1], Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(ids[1]), Anchor: string(ids[1]), HeadingID: string(ids[1]), HeadingLevel: 2,
		Href:    "documents/core-v1/?selected=" + string(ids[1]) + "#" + string(ids[1]),
		Heading: "Get Pod", Method: "GET", Path: "/api/v1/pods/{name}", Summary: "Get Pod",
		Tags: []projection.TextRecord{{Ordinal: 0, ID: "core", Value: "core"}},
	}}
	operation := domain.Operation{
		ID: "getCoreV1Pod", Anchor: string(ids[1]), Title: "Get Pod", Method: "GET", Path: "/api/v1/pods/{name}",
		Summary: "Get Pod", Tags: []string{"core"},
	}
	document := catalog.DocumentDirectoryV1{Key: "core-v1", Operations: []catalog.OperationDirectoryV1{
		{DetailID: ids[0], OperationID: "listCoreV1Pod", Method: "GET", Path: "/api/v1/pods", Title: "List Pods", Href: "core-v1/?selected=" + string(ids[0]) + "#" + string(ids[0]), DetailChild: "details/core.json", Tags: []string{"core"}},
		{DetailID: ids[1], OperationID: operation.ID, Method: operation.Method, Path: operation.Path, Title: operation.Summary, Href: "core-v1/?selected=" + string(ids[1]) + "#" + string(ids[1]), DetailChild: "details/core.json", Tags: append([]string(nil), operation.Tags...)},
		{DetailID: ids[2], OperationID: "deleteCoreV1Pod", Method: "DELETE", Path: "/api/v1/pods/{name}", Title: "Delete Pod", Href: "core-v1/?selected=" + string(ids[2]) + "#" + string(ids[2]), DetailChild: "details/core.json", Tags: []string{"core"}},
	}}
	documentHref := "/kubernetes/documents/core-v1/"
	fragment, err := localrender.PrepareOperationNavigation(detail, operation, document, documentHref, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyNavigation := OperationNavigationData{Group: "core", Catalog: true,
		Previous: &OperationNavigationItem{Title: "List Pods", Method: "GET", Href: documentHref + "?group=&selected=" + string(ids[0]) + "#" + string(ids[0])},
		Next:     &OperationNavigationItem{Title: "Delete Pod", Method: "DELETE", Href: documentHref + "?group=&selected=" + string(ids[2]) + "#" + string(ids[2])},
	}
	var legacy bytes.Buffer
	if err := endpointSection(operation, nil, "", PublicDocsOptions{}, legacyNavigation).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	var delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", PublicDocsOptions{OperationNavigation: &fragment}, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(delegated.Bytes(), legacy.Bytes()) {
		t.Fatalf("prepared operation navigation changed complete endpoint SSR bytes\nlegacy=%s\ndelegated=%s", legacy.Bytes(), delegated.Bytes())
	}
}

func TestPreparedOperationResponseMediaMatchesCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("f", 64))
	documentHref := "/kubernetes/documents/core-v1/"
	schemaID := "detail-sha256-" + strings.Repeat("e", 64)
	operation := domain.Operation{Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/api/v1/pods", Responses: []domain.OperationResponse{
		{Status: "200", Description: "Created resource.", Headers: []domain.OperationResponseHeader{{Name: "X-Rate-Limit", Description: "Quota remaining.", Example: "17", Schema: domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}}}}}}, MediaTypes: []domain.OperationMediaType{
			{ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object"}, Example: `{"kind":"Pod"}`, ExampleProvided: true},
			{ContentType: "application/problem+json", Schema: domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}}}}},
		}},
		{Status: "404", Description: "Missing resource.", Headers: []domain.OperationResponseHeader{{Name: "X-Request-ID", Schema: domain.SchemaSummary{Type: "string"}}}, MediaTypes: []domain.OperationMediaType{{ContentType: "text/plain", Schema: domain.SchemaSummary{Type: "string"}}}},
		{Status: "204", Description: "No content.", Headers: []domain.OperationResponseHeader{{Name: "X-Trace-ID", Schema: domain.SchemaSummary{Type: "string"}}}},
	}}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2, Method: "POST", Path: "/api/v1/pods",
		Responses: []projection.Response{
			{Ordinal: 0, ID: "200", Status: "200", Description: "Created resource.", Headers: []projection.ResponseHeader{{Ordinal: 0, ID: responseHeaderProjectionID("X-Rate-Limit"), Name: "X-Rate-Limit", Description: "Quota remaining.", SchemaRef: 12, Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: "17", Provided: true}}}}, MediaTypes: []projection.MediaType{
				{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7, Examples: []projection.Example{{Ordinal: 0, ID: "primary", Text: `{"kind":"Pod"}`, Provided: true}}},
				{Ordinal: 1, ID: "application/problem+json", ContentType: "application/problem+json", SchemaRef: 8},
			}},
			{Ordinal: 1, ID: "404", Status: "404", Description: "Missing resource.", Headers: []projection.ResponseHeader{{Ordinal: 0, ID: responseHeaderProjectionID("X-Request-ID"), Name: "X-Request-ID", SchemaRef: 13}}, MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "text/plain", ContentType: "text/plain", SchemaRef: 11}}},
			{Ordinal: 2, ID: "204", Status: "204", Description: "No content.", Headers: []projection.ResponseHeader{{Ordinal: 0, ID: responseHeaderProjectionID("X-Trace-ID"), Name: "X-Trace-ID", SchemaRef: 14}}},
		},
	}}
	nodes := []projection.SchemaNode{
		{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"},
		{Ordinal: 8, ID: "node-array-root", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 9}}},
		{Ordinal: 9, ID: "node-array-nested", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 10}}},
		{Ordinal: 10, ID: "node-status", Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}},
		{Ordinal: 11, ID: "node-text", Type: "string"},
		{Ordinal: 12, ID: "node-rate-limit", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 15}}},
		{Ordinal: 13, ID: "node-request-id", Type: "string"},
		{Ordinal: 14, ID: "node-trace-id", Type: "string"},
		{Ordinal: 15, ID: "node-rate-limit-items", Type: "array", Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 16}}},
		{Ordinal: 16, ID: "node-rate-limit-value", Name: "Status", Type: "string", Format: "uuid", Enum: []string{"ready", "pending"}},
	}
	schemaLinks := map[string]string{
		"Pod":    documentHref + "?selected=" + schemaID + "#" + schemaID,
		"Status": documentHref + "?selected=" + schemaID + "#" + schemaID,
	}
	fragment, err := localrender.PrepareOperationResponseMedia(detail, operation, nodes[:5], documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	responseDetails, err := localrender.PrepareOperationResponseDetails(detail, operation, []projection.SchemaNode{nodes[5], nodes[6], nodes[7], nodes[8], nodes[9]}, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	operationExamples, err := localrender.PrepareOperationExamples(detail, operation, nodes[:5])
	if err != nil {
		t.Fatal(err)
	}
	baseOptions := PublicDocsOptions{SchemaLinks: schemaLinks, SchemaLinkTarget: "#catalog-main-content", SchemaLinkSelect: "#catalog-main-content", SchemaLinkSwap: "outerHTML show:#main-content:top"}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	baseOptions.OperationResponseMedia = &fragment
	baseOptions.OperationResponseDetails = &responseDetails
	baseOptions.OperationExamples = &operationExamples
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("delegated response-media summary changed complete SSR endpoint bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
	operationSchemaTrees, err := localrender.PrepareOperationSchemaTrees(detail, operation, nodes[:5], documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	operationResponses, err := localrender.PrepareOperationResponses(detail, operation, fragment, responseDetails, operationExamples, operationSchemaTrees)
	if err != nil {
		t.Fatal(err)
	}
	baseOptions.OperationSchemaTrees = &operationSchemaTrees
	baseOptions.OperationResponses = &operationResponses
	delegated.Reset()
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("prepared complete responses changed SSR/no-JS endpoint bytes at byte %d:\nlegacy=%q\nprepared=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedOperationSchemaTreesMatchCompleteCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	schemaID := "detail-sha256-" + strings.Repeat("b", 64)
	documentHref := "/kubernetes/documents/core-v1/"
	phase := domain.SchemaSummary{
		Name: "Phase", Type: "string", Format: "uuid", Description: "Lifecycle <phase>.", Default: "Ready", Example: "Pending",
		Enum: []string{"Ready", "Pending"}, Constraints: []domain.SchemaConstraint{{Name: "minLength", Value: "1"}}, JSON: `{"type":"string"}`,
	}
	requestSchema := domain.SchemaSummary{
		Name: "Pod", Type: "object", Description: "Pod request.", JSON: `{"type":"object"}`,
		Properties: []domain.SchemaProperty{{Name: "phase", Required: true, Description: "Lifecycle <phase>.", Schema: phase}},
	}
	responseItem := domain.SchemaSummary{
		Name: "Envelope", Type: "object", Description: "Response envelope.", Nullable: true, Deprecated: true, JSON: `{"type":"object"}`,
		Properties: []domain.SchemaProperty{{Name: "phase", Description: "Lifecycle <phase>.", Schema: phase}},
	}
	responseSchema := domain.SchemaSummary{Name: "EnvelopeList", Type: "array", JSON: `{"type":"array"}`, Items: &responseItem}
	operation := domain.Operation{
		Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/pods",
		RequestBody: &domain.OperationRequestBody{Description: "Pod to create.", Required: true, MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: requestSchema}}},
		Responses:   []domain.OperationResponse{{Status: "201", Description: "Created.", MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: responseSchema}}}},
	}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2, Method: "POST", Path: "/pods", HasRequestBody: true,
		RequestBody: projection.RequestBody{Description: "Pod to create.", Required: true, MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7}}},
		Responses:   []projection.Response{{Ordinal: 0, ID: "201", Status: "201", Description: "Created.", MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 9}}}},
	}}
	nodes := []projection.SchemaNode{
		{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object", Description: "Pod request.", JSON: `{"type":"object"}`, Properties: []projection.SchemaNodeProperty{{Ordinal: 0, ID: "property-phase", Name: "phase", Required: true, SchemaRef: 8}}},
		{Ordinal: 8, ID: "node-phase", Name: "Phase", Type: "string", Format: "uuid", Description: "Lifecycle <phase>.", DefaultValue: "Ready", ExampleText: "Pending", Enum: []string{"Ready", "Pending"}, Constraints: []projection.SchemaConstraint{{Name: "minLength", Value: "1"}}, JSON: `{"type":"string"}`},
		{Ordinal: 9, ID: "node-envelope-list", Name: "EnvelopeList", Type: "array", JSON: `{"type":"array"}`, Items: []projection.SchemaNodeItem{{Ordinal: 0, ID: "items", SchemaRef: 10}}},
		{Ordinal: 10, ID: "node-envelope", Name: "Envelope", Type: "object", Description: "Response envelope.", Nullable: true, Deprecated: true, JSON: `{"type":"object"}`, Properties: []projection.SchemaNodeProperty{{Ordinal: 0, ID: "property-phase", Name: "phase", SchemaRef: 8}}},
	}
	schemaLinks := map[string]string{"Phase": documentHref + "?selected=" + schemaID + "#" + schemaID}
	fragment, err := localrender.PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	requestMedia, err := localrender.PrepareOperationRequestBodyMedia(detail, operation, nodes[:1], documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	requestBody, err := localrender.PrepareOperationRequestBody(detail, operation, requestMedia, fragment)
	if err != nil {
		t.Fatal(err)
	}
	baseOptions := PublicDocsOptions{SchemaLinks: schemaLinks, SchemaLinkTarget: "#catalog-main-content", SchemaLinkSelect: "#catalog-main-content", SchemaLinkSwap: "outerHTML show:#main-content:top"}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	baseOptions.OperationSchemaTrees = &fragment
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("delegated operation schema trees changed complete SSR endpoint bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
	delegated.Reset()
	baseOptions.OperationRequestBodyMedia = &requestMedia
	baseOptions.OperationRequestBody = &requestBody
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("prepared complete request-body changed SSR/no-JS endpoint bytes at byte %d:\nlegacy=%q\nprepared=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedOperationRequestSectionMatchesCompleteCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	schemaID := "detail-sha256-" + strings.Repeat("b", 64)
	documentHref := "/kubernetes/documents/core-v1/"
	operation := domain.Operation{
		Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/pods",
		RequestBody: &domain.OperationRequestBody{Description: "Pod to create.", Required: true, MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object"}}}},
		Responses:   []domain.OperationResponse{{Status: "201", Description: "Created."}},
	}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
		Method: "POST", Path: "/pods", HasRequestBody: true,
		RequestBody: projection.RequestBody{Description: "Pod to create.", Required: true, MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7}}},
		Responses:   []projection.Response{{Ordinal: 0, ID: "201", Status: "201", Description: "Created."}},
	}}
	nodes := []projection.SchemaNode{{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}}
	schemaLinks := map[string]string{"Pod": documentHref + "?selected=" + schemaID + "#" + schemaID}
	authorization, err := localrender.PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := localrender.PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	media, err := localrender.PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := localrender.PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	body, err := localrender.PrepareOperationRequestBody(detail, operation, media, trees)
	if err != nil {
		t.Fatal(err)
	}
	requestSection, err := localrender.PrepareOperationRequestSection(detail, operation, authorization, parameters, &body, documentHref, schemaLinks)
	if err != nil {
		t.Fatal(err)
	}
	baseOptions := PublicDocsOptions{SchemaLinks: schemaLinks, SchemaLinkTarget: "#catalog-main-content", SchemaLinkSelect: "#catalog-main-content", SchemaLinkSwap: "outerHTML show:#main-content:top"}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	baseOptions.OperationRequestSection = &requestSection
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("delegated request section changed complete SSR/no-JS endpoint bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedOperationRequestSectionAuthorizationOnlyMatchesSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("c", 64))
	security := domain.OperationSecurity{Name: "bearer", Definition: domain.SecurityScheme{Name: "bearer", Type: "http", Description: "Bearer token.", Scheme: "bearer", BearerFormat: "JWT"}}
	operation := domain.Operation{Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/pods", Security: []domain.OperationSecurity{security}}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), HeadingLevel: 2, Heading: "List Pods", Method: "GET", Path: "/pods",
		Security: []projection.SecurityRequirement{{Ordinal: 0, ID: "bearer", Name: "bearer", Definition: projection.SecurityScheme{Name: "bearer", Type: "http", Description: "Bearer token.", Scheme: "bearer", BearerFormat: "JWT"}}},
	}}
	authorization, err := localrender.PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := localrender.PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	requestSection, err := localrender.PrepareOperationRequestSection(detail, operation, authorization, parameters, nil, "/documents/core-v1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", PublicDocsOptions{}, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	if err := endpointSection(operation, nil, "", PublicDocsOptions{OperationRequestSection: &requestSection}, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("delegated authorization-only request section changed SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedOperationDetailSectionsMatchesCompleteEndpointSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
	security := domain.OperationSecurity{Name: "bearer", Definition: domain.SecurityScheme{Name: "bearer", Type: "http", Description: "Bearer token.", Scheme: "bearer", BearerFormat: "JWT"}}
	operation := domain.Operation{Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/pods", Security: []domain.OperationSecurity{security}}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), HeadingLevel: 2, Heading: "List Pods", Method: "GET", Path: "/pods",
		Security: []projection.SecurityRequirement{{Ordinal: 0, ID: "bearer", Name: "bearer", Definition: projection.SecurityScheme{Name: "bearer", Type: "http", Description: "Bearer token.", Scheme: "bearer", BearerFormat: "JWT"}}},
	}}
	authorization, err := localrender.PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := localrender.PrepareOperationParameters(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := localrender.PrepareOperationRequestSection(detail, operation, authorization, parameters, nil, "/documents/core-v1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	sections, err := localrender.PrepareOperationDetailSections(detail, operation, &request, nil)
	if err != nil {
		t.Fatal(err)
	}
	baseOptions := PublicDocsOptions{}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	baseOptions.OperationDetailSections = &sections
	if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("prepared operation detail sections changed complete endpoint SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedOperationDetailSectionsWithResponsesMatchesCompleteEndpointSSRBytes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		description string
	}{
		{name: "description", description: "Created."},
		{name: "empty description", description: ""},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			detailID := domain.DetailID("detail-sha256-" + strings.Repeat("e", 64))
			documentHref := "/documents/core-v1/"
			operation := domain.Operation{
				Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/pods",
				RequestBody: &domain.OperationRequestBody{Description: "Pod to create.", Required: true, MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: domain.SchemaSummary{Name: "Pod", Type: "object"}}}},
				Responses:   []domain.OperationResponse{{Status: "201", Description: test.description}},
			}
			detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
				ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", HeadingLevel: 2,
				Method: "POST", Path: "/pods", HasRequestBody: true,
				RequestBody: projection.RequestBody{Description: "Pod to create.", Required: true, MediaTypes: []projection.MediaType{{Ordinal: 0, ID: "application/json", ContentType: "application/json", SchemaRef: 7}}},
				Responses:   []projection.Response{{Ordinal: 0, ID: "201", Status: "201", Description: test.description}},
			}}
			nodes := []projection.SchemaNode{{Ordinal: 7, ID: "node-pod", Name: "Pod", Type: "object"}}
			schemaLinks := map[string]string{"Pod": documentHref + "?selected=detail-sha256-" + strings.Repeat("f", 64) + "#detail-sha256-" + strings.Repeat("f", 64)}
			authorization, err := localrender.PrepareOperationAuthorization(detail, operation)
			if err != nil {
				t.Fatal(err)
			}
			parameters, err := localrender.PrepareOperationParameters(detail, operation, nil)
			if err != nil {
				t.Fatal(err)
			}
			media, err := localrender.PrepareOperationRequestBodyMedia(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			trees, err := localrender.PrepareOperationSchemaTrees(detail, operation, nodes, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			body, err := localrender.PrepareOperationRequestBody(detail, operation, media, trees)
			if err != nil {
				t.Fatal(err)
			}
			request, err := localrender.PrepareOperationRequestSection(detail, operation, authorization, parameters, &body, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			responseMedia, err := localrender.PrepareOperationResponseMedia(detail, operation, nil, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			responseDetails, err := localrender.PrepareOperationResponseDetails(detail, operation, nil, documentHref, schemaLinks)
			if err != nil {
				t.Fatal(err)
			}
			examples, err := localrender.PrepareOperationExamples(detail, operation, nil)
			if err != nil {
				t.Fatal(err)
			}
			responses, err := localrender.PrepareOperationResponses(detail, operation, responseMedia, responseDetails, examples, trees)
			if err != nil {
				t.Fatal(err)
			}
			sections, err := localrender.PrepareOperationDetailSections(detail, operation, &request, &responses)
			if err != nil {
				t.Fatal(err)
			}

			baseOptions := PublicDocsOptions{SchemaLinks: schemaLinks, SchemaLinkTarget: "#catalog-main-content", SchemaLinkSelect: "#catalog-main-content", SchemaLinkSwap: "outerHTML show:#main-content:top"}
			var legacy, delegated bytes.Buffer
			if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
				t.Fatal(err)
			}
			baseOptions.OperationDetailSections = &sections
			if err := endpointSection(operation, nil, "", baseOptions, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
				index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
				t.Fatalf("prepared operation detail sections with responses changed complete endpoint SSR bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
			}
		})
	}
}

func TestPreparedOperationCodeSamplesMatchCatalogSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
	operation := domain.Operation{Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/pods", Snippets: []domain.RequestSnippet{
		{Label: "cURL", Language: "shell", Code: "curl <pods>"},
		{Label: "JavaScript", Language: "javascript", Code: "fetch('/pods')"},
	}}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), CodeSamples: []projection.CodeSample{
			{Ordinal: 0, ID: codeSampleProjectionID("shell", "cURL"), Label: "cURL", Language: "shell", Code: "curl <pods>"},
			{Ordinal: 1, ID: codeSampleProjectionID("javascript", "JavaScript"), Label: "JavaScript", Language: "javascript", Code: "fetch('/pods')"},
		},
	}}
	fragment, err := localrender.PrepareOperationExamples(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", PublicDocsOptions{}, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	if err := endpointSection(operation, nil, "", PublicDocsOptions{OperationExamples: &fragment}, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("delegated code samples changed complete SSR endpoint bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
}

func TestPreparedOperationCodeSamplesWithoutCurlMatchCompleteEndpointSSRBytes(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("d", 64))
	operation := domain.Operation{
		Anchor: string(detailID), Title: "Create Pod", Method: "POST", Path: "/pods",
		Parameters: []domain.OperationParameter{{Name: "trace", In: "header"}},
		Snippets: []domain.RequestSnippet{
			{Label: "JavaScript", Language: "javascript", Code: "fetch('/pods')"},
			{Label: "Python", Language: "python", Code: "requests.post('/pods')"},
		},
	}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), Method: "POST", Path: "/pods",
		CodeSamples: []projection.CodeSample{
			{Ordinal: 0, ID: codeSampleProjectionID("javascript", "JavaScript"), Label: "JavaScript", Language: "javascript", Code: "fetch('/pods')"},
			{Ordinal: 1, ID: codeSampleProjectionID("python", "Python"), Label: "Python", Language: "python", Code: "requests.post('/pods')"},
		},
	}}
	fragment, err := localrender.PrepareOperationExamples(detail, operation, nil)
	if err != nil {
		t.Fatal(err)
	}
	var legacy, delegated bytes.Buffer
	if err := endpointSection(operation, nil, "", PublicDocsOptions{}, OperationNavigationData{}).Render(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}
	if err := endpointSection(operation, nil, "", PublicDocsOptions{OperationExamples: &fragment}, OperationNavigationData{}).Render(context.Background(), &delegated); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy.Bytes(), delegated.Bytes()) {
		index := firstDifferentByte(legacy.Bytes(), delegated.Bytes())
		t.Fatalf("delegated non-cURL code samples changed complete SSR endpoint bytes at byte %d:\nlegacy=%q\ndelegated=%q", index, nearbyBytes(legacy.Bytes(), index), nearbyBytes(delegated.Bytes(), index))
	}
	for _, want := range []string{"Request Sample: Shell / cURL", "Request Sample: JavaScript", "Request Sample: Python"} {
		if !strings.Contains(delegated.String(), want) {
			t.Fatalf("delegated endpoint missing %q", want)
		}
	}
}

func firstDifferentByte(left, right []byte) int {
	limit := min(len(left), len(right))
	for index := range limit {
		if left[index] != right[index] {
			return index
		}
	}
	return limit
}

func nearbyBytes(value []byte, index int) []byte {
	start := max(0, index-80)
	end := min(len(value), index+80)
	return value[start:end]
}

func parameterProjectionID(location, name string) string {
	hash := sha256.New()
	hash.Write([]byte("parameter"))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range []string{strings.ToLower(location), name} {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		hash.Write(length[:])
		hash.Write([]byte(value))
	}
	return "parameter-" + hex.EncodeToString(hash.Sum(nil))
}

func responseHeaderProjectionID(name string) string {
	hash := sha256.New()
	hash.Write([]byte("response-header"))
	hash.Write([]byte{0})
	var length [8]byte
	value := strings.ToLower(name)
	binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
	hash.Write(length[:])
	hash.Write([]byte(value))
	return "response-header-" + hex.EncodeToString(hash.Sum(nil))
}

func codeSampleProjectionID(language, label string) string {
	hash := sha256.New()
	hash.Write([]byte("code-sample"))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range []string{language, label} {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		hash.Write(length[:])
		hash.Write([]byte(value))
	}
	return "code-sample-" + hex.EncodeToString(hash.Sum(nil))
}

func authorizationProjectionID(scope string) string {
	hash := sha256.New()
	hash.Write([]byte("scope"))
	hash.Write([]byte{0})
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len([]byte(scope))))
	hash.Write(length[:])
	hash.Write([]byte(scope))
	return "scope-" + hex.EncodeToString(hash.Sum(nil))
}

func parameterSummary(typeName, format string) domain.SchemaSummary {
	return domain.SchemaSummary{Type: typeName, Format: format, Constraints: []domain.SchemaConstraint{}}
}

func parameterSummaryPointer(value domain.SchemaSummary) *domain.SchemaSummary { return &value }

func TestParameterListUsesStackedRowsAndRequiredMarkers(t *testing.T) {
	t.Parallel()

	component := paramGroup("operation-test", "Path Parameters", []domain.OperationParameter{
		{Name: "namespace", In: "path", Required: true, Schema: domain.SchemaSummary{Type: "string"}},
		{Name: "dryRun", In: "query", Required: false, Schema: domain.SchemaSummary{Type: "string"}},
	}, PublicDocsOptions{})
	var body bytes.Buffer
	if err := component.Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}

	output := body.String()
	for _, want := range []string{`data-manja-parameter-list`, "namespace", "dryRun", "string", `data-required="true"`, `data-required="false"`} {
		if !strings.Contains(output, want) {
			t.Errorf("stacked parameter list missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, `>path</`) || strings.Contains(output, `>query</`) {
		t.Fatal("stacked parameter list retained redundant location labels")
	}
}

func TestCatalogSummarylessOperationUsesSemanticVisibleHeading(t *testing.T) {
	t.Parallel()

	data := catalogTemplateFixture()
	document := data.Directory.Documents[0]
	data.Document = &document
	detailID := document.Operations[0].DetailID
	data.Selected = &catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID),
		HeadingID: string(detailID), Heading: "listCoreV1NamespacedPod", HeadingLevel: 2, Method: "GET", Path: "/api/v1/namespaces/{namespace}/pods",
	}}
	data.OperationView = &domain.Operation{
		ID: string(detailID), Anchor: string(detailID), Title: "listCoreV1NamespacedPod", Method: "GET", Path: "/api/v1/namespaces/{namespace}/pods",
	}
	prepareCatalogOperationHeader(t, &data)

	body := renderCatalogTemplate(t, data)
	if !strings.Contains(body, ">listCoreV1NamespacedPod</h1>") {
		t.Fatalf("summary-less operation semantic heading missing: %s", body)
	}
	if strings.Contains(body, ">"+string(detailID)+"</h1>") {
		t.Fatalf("summary-less operation exposed immutable detail hash as heading: %s", body)
	}
}

func prepareCatalogOperationHeader(t *testing.T, data *CatalogPageData) {
	t.Helper()
	if data.Selected == nil || data.OperationView == nil {
		t.Fatal("operation fixture is incomplete")
	}
	fragment, err := localrender.PrepareOperationHeader(*data.Selected, *data.OperationView, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	data.OperationHeader = &fragment
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
		DownloadHref: "/kubernetes/catalog.json", SearchHref: "/kubernetes/search", SearchJSONHref: "/kubernetes/search.json", SearchGlobal: true, SearchContextMount: "/kubernetes", SearchMount: "/", SearchScopeLabel: "All catalogs",
		SearchChildBase:     "/kubernetes/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/search-data/",
		SearchDirectoryPath: "search/directory.json", SearchDirectoryLength: 42, SearchDirectorySHA256: strings.Repeat("c", 64),
	}
}
