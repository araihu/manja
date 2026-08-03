package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/renderer"
)

func TestCatalogDocumentComboboxSearchSelectAndGlobalShortcut(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	server, err := renderer.New(renderer.Config{Version: 1, DataDir: t.TempDir(), Catalogs: []renderer.CatalogConfig{{
		ID: "kubernetes", Mount: "/", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	spec := []byte(`{"openapi":"3.0.3","info":{"title":"Kubernetes","version":"v1"},"paths":{}}`)
	_, err = server.Activate(context.Background(), domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileStrict,
		Revision: domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "file-manifest-catalog-combobox", ManifestDigest: strings.Repeat("a", 64)},
		Documents: []domain.CatalogDocument{
			{Key: "apps-v1", SourcePath: "apis/apps/v1.json", Format: domain.CatalogFormatJSON, Bytes: spec},
			{Key: "core-v1", SourcePath: "api/v1.json", Format: domain.CatalogFormatJSON, Bytes: spec},
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
	if err := page.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL(baseURL + "/search"); err != nil {
		t.Fatalf("global search shortcut: %v (url=%s)", err, page.URL())
	}
	focused, err := page.Locator("#catalog-search-query").Evaluate(`element => document.activeElement === element`, nil)
	if err != nil || focused != true {
		t.Fatalf("search query focused = %v, err=%v", focused, err)
	}
}
