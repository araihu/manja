package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/renderer"
)

func TestCatalogNavigationKeepsLastSpecReachableAndLabelsEachRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	documents := make([]domain.CatalogDocument, 0, 18)
	for index := 1; index <= 18; index++ {
		key := fmt.Sprintf("spec-%02d", index)
		documents = append(documents, domain.CatalogDocument{
			Key: key, SourcePath: key + ".json", Format: domain.CatalogFormatJSON,
			Bytes: []byte(fmt.Sprintf(`{"openapi":"3.0.3","info":{"title":"Spec %02d","version":"v1"},"paths":{}}`, index)),
		})
	}
	server, err := renderer.New(renderer.Config{Version: 1, DataDir: t.TempDir(), Catalogs: []renderer.CatalogConfig{{
		ID: "kubernetes", Mount: "/catalogs/kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Activate(context.Background(), domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "file-manifest-root-navigation", ManifestDigest: strings.Repeat("d", 64)},
		Documents: documents,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseURL := httptestServer(t, server.Handler())

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}

	for _, viewport := range []struct {
		name          string
		width, height int
	}{
		{name: "mobile", width: 390, height: 844},
		{name: "desktop", width: 1440, height: 900},
	} {
		t.Run(viewport.name, func(t *testing.T) {
			if err := page.SetViewportSize(viewport.width, viewport.height); err != nil {
				t.Fatal(err)
			}
			if _, err := page.Goto(baseURL+"/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
				t.Fatal(err)
			}
			trigger := page.Locator(`[x-ref="catalogNavTrigger"]`)
			if label, err := trigger.GetAttribute("aria-label"); err != nil || label != "Open Catalogs and specs" {
				t.Errorf("%s root trigger label = %q, err=%v", viewport.name, label, err)
			}
			panel := page.Locator("#catalog-navigation")
			if label, err := panel.GetAttribute("aria-label"); err != nil || label != "Catalogs and specs" {
				t.Errorf("%s root panel label = %q, err=%v", viewport.name, label, err)
			}
			if viewport.width < 1024 {
				if err := trigger.Click(); err != nil {
					t.Fatal(err)
				}
			}
			if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
				t.Fatalf("%s root navigation panel: %v", viewport.name, err)
			}
			if label, err := panel.Locator(`[x-ref="catalogNavClose"]`).GetAttribute("aria-label"); err != nil || label != "Close Catalogs and specs" {
				t.Errorf("%s root close label = %q, err=%v", viewport.name, label, err)
			}
			if label, err := page.Locator(`[data-catalog-navigation-backdrop]`).GetAttribute("aria-label"); err != nil || label != "Close Catalogs and specs" {
				t.Errorf("%s root backdrop label = %q, err=%v", viewport.name, label, err)
			}
			metricsValue, err := page.Evaluate(`() => {
				const panel = document.querySelector('#catalog-navigation');
				const nav = document.querySelector('#catalog-organization-navigation [data-goshtoso-scroll-viewport]');
				const last = document.querySelector('[data-catalog-organization-item="spec-kubernetes-spec-18"]');
				nav.scrollTop = nav.scrollHeight;
				const panelRect = panel.getBoundingClientRect();
				const navRect = nav.getBoundingClientRect();
				const lastRect = last.getBoundingClientRect();
				return {
					panelBottom: panelRect.bottom,
					navBottom: navRect.bottom,
					navTop: navRect.top,
					lastBottom: lastRect.bottom,
					lastTop: lastRect.top,
					viewportBottom: window.innerHeight,
				};
			}`)
			if err != nil {
				t.Fatal(err)
			}
			metrics, ok := metricsValue.(map[string]any)
			if !ok {
				t.Fatalf("%s root navigation metrics = %#v", viewport.name, metricsValue)
			}
			visibleBottom := min(metricNumber(t, metrics, "panelBottom"), metricNumber(t, metrics, "viewportBottom"))
			navBottom := metricNumber(t, metrics, "navBottom")
			lastBottom := metricNumber(t, metrics, "lastBottom")
			lastTop := metricNumber(t, metrics, "lastTop")
			navTop := metricNumber(t, metrics, "navTop")
			t.Logf("%dx%d root labels=%q/%q/%q nav=%.1f..%.1f last=%.1f..%.1f visible-bottom=%.1f", viewport.width, viewport.height, "Open Catalogs and specs", "Catalogs and specs", "Close Catalogs and specs", navTop, navBottom, lastTop, lastBottom, visibleBottom)
			if navBottom > visibleBottom+1 {
				t.Errorf("%s organization navigation bottom %.1f exceeds visible drawer bottom %.1f: %#v", viewport.name, navBottom, visibleBottom, metrics)
			}
			if lastBottom > visibleBottom+1 || lastTop < navTop-1 {
				t.Errorf("%s last spec remains clipped after maximum navigation scroll: %#v", viewport.name, metrics)
			}

			if _, err := page.Goto(baseURL+"/catalogs/kubernetes/documents/spec-01/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
				t.Fatal(err)
			}
			if label, err := page.Locator(`[x-ref="catalogNavTrigger"]`).GetAttribute("aria-label"); err != nil || label != "Open API sections" {
				t.Errorf("%s document trigger label = %q, err=%v", viewport.name, label, err)
			}
			if label, err := page.Locator("#catalog-navigation").GetAttribute("aria-label"); err != nil || label != "API sections" {
				t.Errorf("%s document panel label = %q, err=%v", viewport.name, label, err)
			}
			if label, err := page.Locator(`[x-ref="catalogNavClose"]`).GetAttribute("aria-label"); err != nil || label != "Close API sections" {
				t.Errorf("%s document close label = %q, err=%v", viewport.name, label, err)
			}
			t.Logf("%dx%d document labels=%q/%q/%q", viewport.width, viewport.height, "Open API sections", "API sections", "Close API sections")
		})
	}
}

func TestCatalogDocumentSearchUsesGlobalModal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	server, err := renderer.New(renderer.Config{Version: 1, DataDir: t.TempDir(), Catalogs: []renderer.CatalogConfig{{
		ID: "kubernetes", Mount: "/", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	appsSpec := []byte(`{"openapi":"3.0.3","info":{"title":"Kubernetes Apps","version":"v1"},"paths":{"/apis/apps/v1/deployments":{"get":{"operationId":"listAppsDeployments","summary":"List app deployments","description":"Returns deployments managed by the apps controller.","responses":{"200":{"description":"OK"}}}}}}`)
	coreSpec := []byte(`{"openapi":"3.0.3","info":{"title":"Kubernetes Core","version":"v1"},"paths":{"/api/v1/pods":{"get":{"operationId":"listCorePods","summary":"List core pods","description":"Returns pods scheduled on cluster nodes.","responses":{"200":{"description":"OK"}}}}}}`)
	_, err = server.Activate(context.Background(), domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
		Revision: domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "file-manifest-catalog-combobox", ManifestDigest: strings.Repeat("a", 64)},
		Documents: []domain.CatalogDocument{
			{Key: "apps-v1", SourcePath: "apis/apps/v1.json", Format: domain.CatalogFormatJSON, Bytes: appsSpec},
			{Key: "core-v1", SourcePath: "api/v1.json", Format: domain.CatalogFormatJSON, Bytes: coreSpec},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	baseURL := httptestServer(t, server.Handler())

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(baseURL, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	if count, err := page.Locator(`header a[href="/search"]`).Count(); err != nil || count != 0 {
		t.Fatalf("header Search links = %d, err=%v", count, err)
	}
	if count, err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Open", Exact: playwright.Bool(true)}).Count(); err != nil || count != 0 {
		t.Fatalf("header Open buttons = %d, err=%v", count, err)
	}
	if _, err := page.Goto(baseURL+"/documents/apps-v1/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatalf("document navigation: %v", err)
	}
	documentHeader := page.Locator(`main[data-catalog-document] > header[data-catalog-document-header]`)
	if err := documentHeader.WaitFor(); err != nil {
		t.Fatalf("catalog document header: %v", err)
	}
	if heading, err := documentHeader.Locator("h1").TextContent(); err != nil || strings.TrimSpace(heading) != "apps-v1" {
		t.Fatalf("catalog document heading = %q, err=%v", heading, err)
	}
	if count, err := documentHeader.Locator("a").Count(); err != nil || count != 1 {
		t.Fatalf("catalog document header links = %d, err=%v; only source download should remain", count, err)
	}
	if count, err := documentHeader.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Download source", Exact: playwright.Bool(true)}).Count(); err != nil || count != 1 {
		t.Fatalf("catalog source download links = %d, err=%v", count, err)
	}
	sourcePath := documentHeader.Locator("[data-catalog-provenance] > div").First().Locator("dd code")
	if source, err := sourcePath.TextContent(); err != nil || strings.TrimSpace(source) != "apis/apps/v1.json" {
		t.Fatalf("catalog source path = %q, err=%v", source, err)
	}
	if order, err := documentHeader.Evaluate(`element => {
		const action = element.querySelector('a');
		const source = element.querySelector('[data-catalog-provenance] > div:first-child dd code');
		return Boolean(action && source && (action.compareDocumentPosition(source) & Node.DOCUMENT_POSITION_FOLLOWING));
	}`, nil); err != nil || order != true {
		t.Fatalf("catalog source path should follow download action: %v, err=%v", order, err)
	}
	if _, err := page.Evaluate(`() => { window.__catalogSearchLoadMarker = "unchanged"; return true; }`, nil); err != nil {
		t.Fatal(err)
	}
	beforeSearch := page.URL()
	searchField := page.Locator(`[data-search-id="catalog-search"] button`)
	if err := searchField.Click(); err != nil {
		t.Fatal(err)
	}
	modal := page.Locator("#catalog-search-dialog")
	if err := modal.WaitFor(); err != nil {
		t.Fatalf("sidebar search opened modal: %v", err)
	}
	if err := page.SetViewportSize(884, 790); err != nil {
		t.Fatal(err)
	}
	page.WaitForTimeout(250)
	searchPanelBox, err := modal.Locator(":scope > div").BoundingBox()
	if err != nil {
		t.Fatal(err)
	}
	if searchPanelBox == nil {
		t.Fatal("catalog search panel has no bounding box")
	}
	panelCenter := searchPanelBox.Y + searchPanelBox.Height/2
	if delta := panelCenter - 395; delta < -2 || delta > 2 {
		t.Fatalf("catalog search panel vertical center = %.1f, want 395±2; box=%#v", panelCenter, searchPanelBox)
	}
	if searchPanelBox.Y < 16 || searchPanelBox.Y+searchPanelBox.Height > 774 {
		t.Fatalf("catalog search panel must retain 16px viewport margins, box=%#v", searchPanelBox)
	}
	if expanded, err := searchField.Evaluate(`element => element.getAttribute('aria-expanded')`, nil); err != nil || expanded != "true" {
		t.Fatalf("sidebar search expanded state = %v, err=%v", expanded, err)
	}
	if page.URL() != beforeSearch {
		t.Fatalf("sidebar search navigated from %q to %q", beforeSearch, page.URL())
	}
	if err := modal.Locator(`[data-catalog-search-recent-result="true"]`).GetByText("apps-v1", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("recently visited apps document: %v", err)
	}
	input := page.Locator("#catalog-search-input")
	focused, err := input.Evaluate(`element => document.activeElement === element`, nil)
	if err != nil || focused != true {
		t.Fatalf("search query focused = %v, err=%v", focused, err)
	}
	if err := input.Fill("controller"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
		t.Fatal(err)
	}
	if err := modal.Locator(`[data-catalog-search-source]`).GetByText("Global search", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(false)}).WaitFor(); err != nil {
		t.Fatalf("global search source: %v", err)
	}
	if err := modal.GetByText("List app deployments", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("client search result with server fallback blocked: %v", err)
	}
	if role, err := input.GetAttribute("role"); err != nil || role != "combobox" {
		t.Fatalf("search input role = %q, err=%v", role, err)
	}
	if inputType, err := input.GetAttribute("type"); err != nil || inputType != "text" {
		t.Fatalf("search input type = %q, err=%v; native and custom clear affordances must not coexist", inputType, err)
	}
	if controls, err := input.GetAttribute("aria-controls"); err != nil || controls != "catalog-search-results" {
		t.Fatalf("search input aria-controls = %q, err=%v", controls, err)
	}
	if expanded, err := input.GetAttribute("aria-expanded"); err != nil || expanded != "true" {
		t.Fatalf("search input aria-expanded = %q, err=%v", expanded, err)
	}
	activeOptionID, err := input.GetAttribute("aria-activedescendant")
	if err != nil || activeOptionID == "" {
		t.Fatalf("search input aria-activedescendant = %q, err=%v", activeOptionID, err)
	}
	activeOption := modal.Locator("#" + activeOptionID)
	if selected, err := activeOption.GetAttribute("aria-selected"); err != nil || selected != "true" {
		t.Fatalf("active search option aria-selected = %q, err=%v", selected, err)
	}
	if count, err := modal.Locator(`button[aria-label="Clear search"]`).Count(); err != nil || count != 1 {
		t.Fatalf("custom search clear controls = %d, err=%v", count, err)
	}
	if err := modal.Locator(`.search-highlight`).GetByText("controller", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("exact content match was not highlighted: %v", err)
	}
	highlighted, err := modal.Evaluate(`element => Alpine.$data(element).highlight('<img data-search-xss="true"> controller')`, nil)
	if err != nil {
		t.Fatal(err)
	}
	highlightedHTML, ok := highlighted.(string)
	if !ok || strings.Contains(highlightedHTML, "<img") || !strings.Contains(highlightedHTML, "&lt;img") || !strings.Contains(highlightedHTML, `class="search-highlight"`) {
		t.Fatalf("unsafe or missing highlight markup: %#v", highlighted)
	}
	if err := input.Fill("list"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
		t.Fatal(err)
	}
	if err := modal.GetByText("List core pods", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("second keyboard search result: %v", err)
	}
	firstActiveID, err := input.GetAttribute("aria-activedescendant")
	if err != nil || firstActiveID == "" {
		t.Fatalf("initial keyboard option id = %q, err=%v", firstActiveID, err)
	}
	if err := input.Press("ArrowDown"); err != nil {
		t.Fatal(err)
	}
	secondActiveID, err := input.GetAttribute("aria-activedescendant")
	if err != nil || secondActiveID == "" || secondActiveID == firstActiveID {
		t.Fatalf("ArrowDown active option = %q, initial=%q, err=%v", secondActiveID, firstActiveID, err)
	}
	if selected, err := modal.Locator("#" + firstActiveID).GetAttribute("aria-selected"); err != nil || selected != "false" {
		t.Fatalf("previous option aria-selected = %q, err=%v", selected, err)
	}
	if selected, err := modal.Locator("#" + secondActiveID).GetAttribute("aria-selected"); err != nil || selected != "true" {
		t.Fatalf("ArrowDown option aria-selected = %q, err=%v", selected, err)
	}
	if err := input.Fill("deplxoyments"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
		t.Fatal(err)
	}
	if err := modal.Locator(`[data-catalog-search-source]`).GetByText("Global search", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(false)}).WaitFor(); err != nil {
		t.Fatalf("fuzzy global search source: %v", err)
	}
	if err := modal.GetByText("List app deployments", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("fuzzy client search result: %v", err)
	}
	if count, err := modal.Locator(`.search-highlight`).Count(); err != nil || count < 1 {
		t.Fatalf("fuzzy result highlight count = %d, err=%v", count, err)
	}
	fuzzyHighlight, err := modal.Locator(`.search-highlight`).First().TextContent()
	if err != nil || (fuzzyHighlight != "dep" && fuzzyHighlight != "oym" && fuzzyHighlight != "nts") {
		t.Fatalf("fuzzy result highlight = %q, err=%v", fuzzyHighlight, err)
	}
	if source, err := modal.Locator(`[data-catalog-search-source]`).TextContent(); err != nil || !strings.Contains(source, "Global search") {
		t.Fatalf("global search source = %q, err=%v", source, err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	if err := modal.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatalf("Escape did not close modal: %v", err)
	}
	if expanded, err := searchField.Evaluate(`element => element.getAttribute('aria-expanded')`, nil); err != nil || expanded != "false" {
		t.Fatalf("sidebar search collapsed state = %v, err=%v", expanded, err)
	}
	if expanded, err := input.GetAttribute("aria-expanded"); err != nil || expanded != "false" {
		t.Fatalf("search combobox collapsed state = %q, err=%v", expanded, err)
	}
	if page.URL() != beforeSearch {
		t.Fatalf("Escape navigated from %q to %q", beforeSearch, page.URL())
	}
	if err := page.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	if err := modal.WaitFor(); err != nil {
		t.Fatalf("Ctrl+K did not reopen modal: %v", err)
	}
	if expanded, err := searchField.Evaluate(`element => element.getAttribute('aria-expanded')`, nil); err != nil || expanded != "true" {
		t.Fatalf("Ctrl+K sidebar search state = %v, err=%v", expanded, err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	marker, err := page.Evaluate(`() => window.__catalogSearchLoadMarker`, nil)
	if err != nil || marker != "unchanged" {
		t.Fatalf("search modal reloaded the page: marker=%v err=%v", marker, err)
	}
	if err := page.SetViewportSize(884, 781); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Open API sections", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatal(err)
	}
	panel := page.Locator("#catalog-navigation")
	if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("mobile catalog drawer: %v", err)
	}
	if count, err := panel.GetByText("API sections", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).Count(); err != nil || count != 0 {
		t.Fatalf("mobile drawer API sections headings = %d, err=%v", count, err)
	}
	if count, err := panel.Locator(`nav[aria-label="sidebar navigation"] > div.shrink-0.border-b`).Count(); err != nil || count != 0 {
		t.Fatalf("mobile drawer sidebar logo headers = %d, err=%v", count, err)
	}
	if count, err := panel.Locator(`[data-catalog-sidebar-search]`).Count(); err != nil || count != 0 {
		t.Fatalf("mobile drawer should not duplicate the global search field: count=%d err=%v", count, err)
	}
	if err := panel.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close API sections", Exact: playwright.Bool(true)}).Click(); err != nil {
		t.Fatal(err)
	}

	fallbackPage, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	defer fallbackPage.Close()
	if err := fallbackPage.Route("**/snapshots/**/search-data/**", func(route playwright.Route) { _ = route.Abort() }); err != nil {
		t.Fatal(err)
	}
	if _, err := fallbackPage.Goto(baseURL+"/documents/core-v1/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	groupControl := fallbackPage.Locator(`#catalog-sidebar-groups a[data-catalog-group-control]`).First()
	coreOperation := fallbackPage.Locator(`[data-catalog-sidebar-operation][title="List core pods"]`)
	if count, err := coreOperation.Count(); err != nil {
		t.Fatalf("core operation count: %v", err)
	} else if count == 0 {
		if err := groupControl.Click(); err != nil {
			t.Fatalf("open core operation group: %v", err)
		}
	}
	if err := coreOperation.WaitFor(); err != nil {
		t.Fatalf("core operation link: %v", err)
	}
	if err := coreOperation.Click(); err != nil {
		t.Fatalf("open core operation: %v", err)
	}
	if !strings.Contains(fallbackPage.URL(), "/documents/core-v1/?selected=") {
		t.Fatalf("core operation navigation url = %s", fallbackPage.URL())
	}
	operationHeader := fallbackPage.Locator(`[data-catalog-detail="operation"] [data-public-page-header]`)
	if err := operationHeader.WaitFor(); err != nil {
		t.Fatalf("catalog operation header: %v", err)
	}
	if heading, err := operationHeader.Locator(".manja-doc-title").TextContent(); err != nil || strings.TrimSpace(heading) != "List core pods" {
		t.Fatalf("catalog operation heading = %q, err=%v", heading, err)
	}
	if count, err := operationHeader.Locator(".manja-doc-title + p").Count(); err != nil || count != 0 {
		t.Fatalf("catalog operation duplicate route subtitle count = %d, err=%v", count, err)
	}
	if path, err := fallbackPage.Locator(`[data-catalog-detail="operation"] [aria-label="Endpoint route"] code`).TextContent(); err != nil || strings.TrimSpace(path) != "/api/v1/pods" {
		t.Fatalf("catalog operation route badge = %q, err=%v", path, err)
	}
	if _, err := fallbackPage.Goto(baseURL+"/documents/core-v1/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	if err := fallbackPage.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	fallbackInput := fallbackPage.Locator("#catalog-search-input")
	if err := fallbackInput.Fill("listCorePods"); err != nil {
		t.Fatal(err)
	}
	if _, err := fallbackInput.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
		t.Fatal(err)
	}
	fallbackModal := fallbackPage.Locator("#catalog-search-dialog")
	if err := fallbackModal.GetByText("List core pods", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("server fallback result: %v", err)
	}
	if source, err := fallbackModal.Locator(`[data-catalog-search-source]`).TextContent(); err != nil || !strings.Contains(source, "Global search") {
		t.Fatalf("global search source = %q, err=%v", source, err)
	}
}

func TestOrganizationRootSearchKeepsNestedCatalogMount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	server, err := renderer.New(renderer.Config{
		Version: 1,
		DataDir: t.TempDir(),
		Catalogs: []renderer.CatalogConfig{
			{ID: "alpha", Mount: "/catalogs/alpha", Title: "Alpha Catalog", ProfileID: domain.CompatibilityProfileStrict},
			{ID: "beta", Mount: "/catalogs/beta", Title: "Beta Catalog", ProfileID: domain.CompatibilityProfileStrict},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	alphaSpec := []byte(`{"openapi":"3.0.3","info":{"title":"Alpha API","version":"v1"},"paths":{"/alpha":{"get":{"operationId":"listAlphaThings","summary":"List Alpha things","description":"Alpha browser index result.","responses":{"200":{"description":"OK"}}}}}}`)
	betaSpec := []byte(`{"openapi":"3.0.3","info":{"title":"Beta API","version":"v1"},"paths":{"/beta":{"get":{"operationId":"listBetaThings","summary":"List Beta things","description":"Beta browser index result.","responses":{"200":{"description":"OK"}}}}}}`)
	for _, candidate := range []domain.CatalogCandidate{
		{ID: "alpha", Title: "Alpha Catalog", ProfileID: domain.CompatibilityProfileStrict, Revision: domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "root-multi-mount-alpha", ManifestDigest: strings.Repeat("a", 64)}, Documents: []domain.CatalogDocument{{Key: "alpha-v1", SourcePath: "alpha.json", Format: domain.CatalogFormatJSON, Bytes: alphaSpec}}},
		{ID: "beta", Title: "Beta Catalog", ProfileID: domain.CompatibilityProfileStrict, Revision: domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "root-multi-mount-beta", ManifestDigest: strings.Repeat("b", 64)}, Documents: []domain.CatalogDocument{{Key: "beta-v1", SourcePath: "beta.json", Format: domain.CatalogFormatJSON, Bytes: betaSpec}}},
	} {
		if _, err := server.Activate(context.Background(), candidate); err != nil {
			t.Fatalf("activate %s: %v", candidate.ID, err)
		}
	}
	baseURL := httptestServer(t, server.Handler())

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(baseURL+"/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	if count, err := page.GetByText("Alpha Catalog", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).Count(); err != nil || count == 0 {
		t.Fatalf("root Alpha Catalog navigation count = %d, err=%v", count, err)
	}
	if count, err := page.GetByText("Beta Catalog", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).Count(); err != nil || count == 0 {
		t.Fatalf("root Beta Catalog navigation count = %d, err=%v", count, err)
	}
	if count, err := page.Locator(`#catalog-organization-section-specs`).Count(); err != nil || count != 1 {
		t.Fatalf("root Specs heading count = %d, err=%v", count, err)
	}
	if count, err := page.Locator(`[data-search-mount="/"]`).Count(); err != nil || count != 1 {
		t.Fatalf("root global search mount count = %d, err=%v", count, err)
	}
	searchField := page.Locator(`[data-search-id="catalog-search"] button`)
	if err := searchField.Click(); err != nil {
		t.Fatal(err)
	}
	modal := page.Locator("#catalog-search-dialog")
	if err := modal.WaitFor(); err != nil {
		t.Fatalf("root search modal: %v", err)
	}
	input := page.Locator("#catalog-search-input")
	if err := input.Fill("Alpha things"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
		t.Fatal(err)
	}
	result := modal.GetByText("List Alpha things", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)})
	if err := result.WaitFor(); err != nil {
		t.Fatalf("root global search result: %v", err)
	}
	if err := result.Click(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.URL(), "/catalogs/alpha/documents/alpha-v1/") {
		t.Fatalf("root search navigation url = %q, want nested alpha mount", page.URL())
	}
	if err := page.Locator(`main[data-catalog-document="alpha-v1"]`).WaitFor(); err != nil {
		t.Fatalf("nested alpha document after root search: %v", err)
	}
}

func TestCatalogSidebarExpansionAndNavigationPreserveContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	server, err := renderer.New(renderer.Config{Version: 1, DataDir: t.TempDir(), Catalogs: []renderer.CatalogConfig{{
		ID: "kubernetes", Mount: "/", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	spec := []byte(`{"openapi":"3.0.3","info":{"title":"Kubernetes Core","version":"v1"},"paths":{"/api/v1/pods":{"get":{"operationId":"listCorePods","tags":["core_v1"],"summary":"List core pods in every namespace with a deliberately long title for overflow verification","responses":{"200":{"description":"OK"}}}},"/api/v1/widgets":{"post":{"operationId":"createCoreWidget","tags":["core_v1"],"summary":"Create widget","responses":{"201":{"description":"Created"}}}},"/api/v1/widgets/replace":{"put":{"operationId":"replaceCoreWidget","tags":["core_v1"],"summary":"Replace widget","responses":{"200":{"description":"OK"}}}},"/api/v1/widgets/status":{"patch":{"operationId":"patchCoreWidget","tags":["core_v1"],"summary":"Patch widget","responses":{"200":{"description":"OK"}}}},"/api/v1/widgets/archive":{"delete":{"operationId":"deleteCoreWidget","tags":["core_v1"],"summary":"Delete widget","responses":{"204":{"description":"Deleted"}}}},"/api/v1/widgets/options":{"options":{"operationId":"optionsCoreWidget","tags":["core_v1"],"summary":"Inspect widget options","responses":{"200":{"description":"OK"}}}}}}`)
	_, err = server.Activate(context.Background(), domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "file-manifest-sidebar-focus", ManifestDigest: strings.Repeat("b", 64)},
		Documents: []domain.CatalogDocument{{Key: "core-v1", SourcePath: "api/v1.json", Format: domain.CatalogFormatJSON, Bytes: spec}},
	})
	if err != nil {
		t.Fatal(err)
	}
	baseURL := httptestServer(t, server.Handler())

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(baseURL+"/documents/core-v1/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	control := page.Locator(`#catalog-sidebar-groups a[hx-target="#catalog-sidebar-groups"]`).First()
	if expanded, err := control.GetAttribute("aria-expanded"); err != nil || expanded != "true" {
		t.Fatalf("initial group aria-expanded = %q, err=%v", expanded, err)
	}
	// aria-expanded changes before the sidebar outerHTML swap settles. Wait for
	// the replacement control to finish HTMX processing before activating it.
	if _, err := page.Evaluate(`() => {
		window.__manjaCatalogSidebarSettleCount = 0;
		document.body.addEventListener('htmx:afterSettle', () => {
			window.__manjaCatalogSidebarSettleCount += 1;
		});
		return true;
	}`); err != nil {
		t.Fatal(err)
	}
	if err := control.Focus(); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('#catalog-sidebar-groups a[hx-target="#catalog-sidebar-groups"]')?.getAttribute('aria-expanded') === 'false'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("group should collapse after keyboard activation: %v", err)
	}
	if _, err := page.WaitForFunction(`() => window.__manjaCatalogSidebarSettleCount >= 1`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("group collapse should finish HTMX settling: %v", err)
	}
	collapsed := page.Locator(`#catalog-sidebar-groups a[hx-target="#catalog-sidebar-groups"]`).First()
	if err := collapsed.WaitFor(); err != nil {
		t.Fatalf("collapsed group control replacement: %v", err)
	}
	if err := collapsed.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("collapsed group control visible: %v", err)
	}
	if expanded, err := collapsed.GetAttribute("aria-expanded"); err != nil || expanded != "false" {
		t.Fatalf("collapsed group aria-expanded = %q, err=%v", expanded, err)
	}
	if err := collapsed.Focus(); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('#catalog-sidebar-groups a[hx-target="#catalog-sidebar-groups"]')?.getAttribute('aria-expanded') === 'true'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("group should expand after keyboard activation: %v", err)
	}
	if _, err := page.WaitForFunction(`() => window.__manjaCatalogSidebarSettleCount >= 2`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("group expansion should finish HTMX settling: %v", err)
	}
	replacement := page.Locator(`#catalog-sidebar-groups a[hx-target="#catalog-sidebar-groups"]`).First()
	if err := replacement.WaitFor(); err != nil {
		t.Fatalf("expanded group control replacement: %v", err)
	}
	if err := replacement.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("expanded group control visible: %v", err)
	}
	if expanded, err := replacement.GetAttribute("aria-expanded"); err != nil || expanded != "true" {
		t.Fatalf("expanded group aria-expanded = %q, err=%v", expanded, err)
	}
	if _, err := page.WaitForFunction(`() => {
		const control = document.querySelector('#catalog-sidebar-groups a[hx-target="#catalog-sidebar-groups"][aria-expanded="true"]');
		return control && document.activeElement === control;
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("expanded group should retain focus: %v", err)
	}
	groupStyle, err := replacement.Evaluate(`element => { const style = getComputedStyle(element); return { borderLeftWidth: style.borderLeftWidth, fontWeight: style.fontWeight, fontSize: style.fontSize }; }`, nil)
	if err != nil {
		t.Fatal(err)
	}
	style, ok := groupStyle.(map[string]interface{})
	if !ok || style["borderLeftWidth"] != "0px" || style["fontWeight"] != "700" || style["fontSize"] != "16px" {
		t.Fatalf("group hierarchy style = %#v, want no rail, 700 weight, 16px text", groupStyle)
	}
	for _, test := range []struct {
		title string
		class string
	}{
		{title: "List core pods in every namespace with a deliberately long title for overflow verification", class: "catalog-method-get"},
		{title: "Create widget", class: "catalog-method-post"},
		{title: "Replace widget", class: "catalog-method-warning"},
		{title: "Patch widget", class: "catalog-method-warning"},
		{title: "Delete widget", class: "catalog-method-delete"},
		{title: "Inspect widget options", class: "catalog-method-neutral"},
	} {
		link := page.Locator(`[data-catalog-sidebar-operation][title="` + test.title + `"]`)
		if err := link.WaitFor(); err != nil {
			t.Fatalf("sidebar operation %q: %v", test.title, err)
		}
		badgeClass, err := link.Locator("sup, span:not(.min-w-0):not(.sr-only)").First().GetAttribute("class")
		if err != nil || !strings.Contains(badgeClass, test.class) {
			t.Fatalf("sidebar operation %q badge class = %q, want %q; err=%v", test.title, badgeClass, test.class, err)
		}
	}
	longLink := page.Locator(`[data-catalog-sidebar-operation][title="List core pods in every namespace with a deliberately long title for overflow verification"]`)
	directHref, err := longLink.GetAttribute("href")
	if err != nil || directHref == "" {
		t.Fatalf("direct operation href = %q, err=%v", directHref, err)
	}
	directPage, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := directPage.Goto(baseURL+directHref, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad}); err != nil {
		t.Fatal(err)
	}
	directActive := directPage.Locator(`[data-catalog-sidebar-operation][title="List core pods in every namespace with a deliberately long title for overflow verification"][aria-current="page"]`)
	badgeLayersScript := `element => {
		const layers = Array.from(element.querySelectorAll(':scope > :is(span, sup)[class*="catalog-method-"]'))
			.map(badge => badge.textContent.trim());
		const pseudo = getComputedStyle(element, '::after').content;
		if (pseudo !== 'none') layers.push(pseudo.replace(/^['"]|['"]$/g, ''));
		return layers;
	}`
	badgeLayers, err := directActive.Evaluate(badgeLayersScript, nil)
	if err != nil || fmt.Sprint(badgeLayers) != "[GET]" {
		t.Fatalf("direct active operation badge layers = %v, want [GET]; err=%v", badgeLayers, err)
	}
	directNext := directPage.Locator(`[data-catalog-sidebar-operation][title="Create widget"]`)
	if err := directNext.Click(); err != nil {
		t.Fatalf("direct operation transition: %v", err)
	}
	if _, err := directPage.WaitForFunction(`() => {
		const previous = document.querySelector('[data-catalog-sidebar-operation][title="List core pods in every namespace with a deliberately long title for overflow verification"]');
		const current = document.querySelector('[data-catalog-sidebar-operation][title="Create widget"]');
		return previous && !previous.hasAttribute('aria-current') && current?.getAttribute('aria-current') === 'page';
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("direct operation transition state: %v", err)
	}
	directPrevious := directPage.Locator(`[data-catalog-sidebar-operation][title="List core pods in every namespace with a deliberately long title for overflow verification"]`)
	badgeLayers, err = directPrevious.Evaluate(badgeLayersScript, nil)
	if err != nil || fmt.Sprint(badgeLayers) != "[GET]" {
		t.Fatalf("previous active operation badge layers = %v, want [GET]; err=%v", badgeLayers, err)
	}
	if err := directPage.Close(); err != nil {
		t.Fatal(err)
	}
	overflow, err := longLink.Locator(".truncate").Evaluate(`element => element.scrollWidth > element.clientWidth`, nil)
	if err != nil || overflow != true {
		t.Fatalf("long sidebar label overflow = %v, err=%v", overflow, err)
	}
	if err := longLink.Hover(); err != nil {
		t.Fatal(err)
	}
	tooltip := page.Locator(`#catalog-sidebar-overflow-tooltip`)
	if err := tooltip.WaitFor(); err != nil {
		t.Fatalf("overflow tooltip: %v", err)
	}
	if hidden, err := tooltip.IsHidden(); err != nil || hidden {
		t.Fatalf("overflow tooltip hidden = %v, err=%v", hidden, err)
	}
	if text, err := tooltip.TextContent(); err != nil || text != "List core pods in every namespace with a deliberately long title for overflow verification" {
		t.Fatalf("overflow tooltip text = %q, err=%v", text, err)
	}
	if describedBy, err := longLink.GetAttribute("aria-describedby"); err != nil || describedBy != "catalog-sidebar-overflow-tooltip" {
		t.Fatalf("overflow tooltip aria-describedby = %q, err=%v", describedBy, err)
	}

	target := page.Locator(`[data-catalog-sidebar-operation][title="Create widget"]`)
	targetHref, err := target.GetAttribute("href")
	if err != nil || targetHref == "" {
		t.Fatalf("target operation href = %q, err=%v", targetHref, err)
	}
	if _, err := page.Evaluate(`() => {
		window.__manjaCatalogNavigationSentinel = "kept";
		window.__manjaCatalogNavigationSettled = false;
		document.getElementById("catalog-sidebar-groups").dataset.navigationSentinel = "kept";
		document.body.addEventListener("htmx:afterSettle", function onSettle(event) {
			if (event.detail && event.detail.target && event.detail.target.id === "catalog-main-content") {
				window.__manjaCatalogNavigationSettled = true;
			}
		}, { once: true });
		return true;
	}`); err != nil {
		t.Fatal(err)
	}
	if err := target.Click(); err != nil {
		t.Fatalf("catalog operation navigation: %v", err)
	}
	if err := page.Locator(`[data-catalog-detail="operation"] .manja-doc-title`).WaitFor(); err != nil {
		t.Fatalf("catalog operation detail: %v", err)
	}
	if _, err := page.WaitForFunction(`() => window.__manjaCatalogNavigationSettled === true || window.__manjaCatalogNavigationSentinel !== "kept"`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("catalog operation navigation settlement: %v", err)
	}
	kept, err := page.Evaluate(`() => window.__manjaCatalogNavigationSentinel === "kept" && document.getElementById("catalog-sidebar-groups")?.dataset.navigationSentinel === "kept"`, nil)
	if err != nil || kept != true {
		t.Fatalf("catalog operation navigation replaced persistent context: kept=%v err=%v", kept, err)
	}
	if got := page.URL(); got != baseURL+targetHref {
		t.Fatalf("catalog operation URL = %q, want %q", got, baseURL+targetHref)
	}
	active := page.Locator(`[data-catalog-sidebar-operation][title="Create widget"][aria-current="page"][data-catalog-sidebar-selected="true"]`)
	if count, err := active.Count(); err != nil || count != 1 {
		t.Fatalf("active catalog operation count = %d, err=%v", count, err)
	}
	if count, err := active.Locator(".catalog-method-post").Count(); err != nil || count != 1 {
		t.Fatalf("active catalog operation rendered badge count = %d, want 1; err=%v", count, err)
	}
	pseudoBadge, err := active.Evaluate(`element => getComputedStyle(element, "::after").content`, nil)
	if err != nil || pseudoBadge != "none" {
		t.Fatalf("active catalog operation pseudo badge = %#v, want none; err=%v", pseudoBadge, err)
	}
	identity, err := page.Evaluate(`() => ({
		title: document.title,
		focused: document.activeElement?.hasAttribute("data-manja-settled-focus") === true,
		mainTitle: document.getElementById("catalog-main-content")?.dataset.documentTitle,
	})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantIdentity := map[string]any{"title": "Create widget · Manja", "focused": true, "mainTitle": "Create widget · Manja"}
	if fmt.Sprint(identity) != fmt.Sprint(wantIdentity) {
		t.Fatalf("catalog operation identity = %#v, want %#v", identity, wantIdentity)
	}
}
