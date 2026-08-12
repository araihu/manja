package templates

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/araihu/goshtoso/components/sidebar"
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
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "Create Pod", Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods",
	}}
	data.OperationView = &domain.Operation{
		ID: string(detailID), Anchor: string(detailID), Summary: "Create Pod", Description: "Creates a Pod.", Method: "POST", Path: "/api/v1/namespaces/{namespace}/pods",
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
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "listCoreV1NamespacedPod", Method: "GET", Path: "/api/v1/namespaces/{namespace}/pods",
	}}
	data.OperationView = &domain.Operation{
		ID: string(detailID), Anchor: string(detailID), Title: "listCoreV1NamespacedPod", Method: "GET", Path: "/api/v1/namespaces/{namespace}/pods",
	}

	body := renderCatalogTemplate(t, data)
	if !strings.Contains(body, ">listCoreV1NamespacedPod</h1>") {
		t.Fatalf("summary-less operation semantic heading missing: %s", body)
	}
	if strings.Contains(body, ">"+string(detailID)+"</h1>") {
		t.Fatalf("summary-less operation exposed immutable detail hash as heading: %s", body)
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
		DownloadHref: "/kubernetes/catalog.json", SearchHref: "/kubernetes/search", SearchJSONHref: "/kubernetes/search.json", SearchGlobal: true, SearchContextMount: "/kubernetes", SearchMount: "/", SearchScopeLabel: "All catalogs",
		SearchChildBase:     "/kubernetes/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/search-data/",
		SearchDirectoryPath: "search/directory.json", SearchDirectoryLength: 42, SearchDirectorySHA256: strings.Repeat("c", 64),
	}
}
