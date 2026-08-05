package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/renderer"
)

func TestCatalogDocumentComboboxSearchSelectAndClientFirstModal(t *testing.T) {
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
	if err := page.Locator("#catalog-document-trigger").Click(); err != nil {
		t.Fatal(err)
	}
	apps := page.Locator(`#catalog-document-options [data-value="/documents/apps-v1/"]`)
	if err := apps.WaitFor(); err != nil {
		t.Fatalf("first-open options: %v", err)
	}
	core := page.Locator(`#catalog-document-options [data-value="/documents/core-v1/"]`)
	if err := core.WaitFor(); err != nil {
		t.Fatalf("first-open core option: %v", err)
	}
	if err := page.Locator(`#catalog-document-body input[name="q"]`).Fill("apps"); err != nil {
		t.Fatal(err)
	}
	if err := core.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatalf("filtered core option remained visible: %v", err)
	}
	if count, err := page.Locator(`#catalog-document-options [data-combobox-option]`).Count(); err != nil || count != 1 {
		t.Fatalf("filtered option count = %d, err=%v", count, err)
	}
	if err := apps.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(baseURL + "/documents/apps-v1/"); err != nil {
		t.Fatalf("selection navigation: %v (url=%s)", err, page.URL())
	}
	if err := page.Route("**/search.json*", func(route playwright.Route) { _ = route.Abort() }); err != nil {
		t.Fatal(err)
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
	if err := modal.Locator(`[data-catalog-search-source]`).GetByText("Browser index", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("client search source: %v", err)
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
	if err := modal.Locator(`[data-catalog-search-source]`).GetByText("Browser index", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatalf("fuzzy client search source: %v", err)
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
	if source, err := modal.Locator(`[data-catalog-search-source]`).TextContent(); err != nil || !strings.Contains(source, "Browser index") {
		t.Fatalf("client search source = %q, err=%v", source, err)
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
	if err := panel.Locator(`[data-catalog-sidebar-search]`).WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("mobile drawer search field: %v", err)
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
	if source, err := fallbackModal.Locator(`[data-catalog-search-source]`).TextContent(); err != nil || !strings.Contains(source, "Server fallback") {
		t.Fatalf("fallback search source = %q, err=%v", source, err)
	}
}

func TestCatalogSidebarExpansionPreservesKeyboardFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	server, err := renderer.New(renderer.Config{Version: 1, DataDir: t.TempDir(), Catalogs: []renderer.CatalogConfig{{
		ID: "kubernetes", Mount: "/", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	spec := []byte(`{"openapi":"3.0.3","info":{"title":"Kubernetes Core","version":"v1"},"paths":{"/api/v1/pods":{"get":{"operationId":"listCorePods","tags":["core_v1"],"summary":"List core pods","responses":{"200":{"description":"OK"}}}}}}`)
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
	if expanded, err := control.GetAttribute("aria-expanded"); err != nil || expanded != "false" {
		t.Fatalf("collapsed group aria-expanded = %q, err=%v", expanded, err)
	}
	if err := control.Focus(); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatal(err)
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
	focused, err := replacement.Evaluate(`element => document.activeElement === element`, nil)
	if err != nil || focused != true {
		t.Fatalf("expanded group retained focus = %v, err=%v", focused, err)
	}
}
