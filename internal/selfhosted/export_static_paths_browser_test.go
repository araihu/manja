package selfhosted

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestStaticExportSearchSortAndNavigationWithoutBrowserStorage(t *testing.T) {
	if testing.Short() {
		t.Skip("browser acceptance")
	}
	root := t.TempDir()
	for _, item := range []struct {
		key, title string
		operations int
	}{{"alpha", "Alpha API", 2}, {"zeta", "Zeta API", 10}} {
		paths := map[string]any{}
		for index := 0; index < item.operations; index++ {
			paths[fmt.Sprintf("/items/%d", index)] = map[string]any{"get": map[string]any{"operationId": fmt.Sprintf("%sOperation%d", item.key, index), "summary": fmt.Sprintf("%s operation %d", item.key, index), "responses": map[string]any{"200": map[string]any{"description": "OK"}}}}
		}
		payload, err := json.Marshal(map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": item.title, "version": "1.0.0"}, "paths": paths})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, item.key+".json"), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := `version: 1
dataDir: data
catalogs:
  - id: project
    mount: /project
    title: Project
    defaultDocument: alpha
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [alpha.json, zeta.json]
`
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	const base = "/static/docs/"
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: base}); err != nil {
		t.Fatal(err)
	}
	files := http.StripPrefix(strings.TrimSuffix(base, "/"), http.FileServer(http.Dir(output)))
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		files.ServeHTTP(w, r)
	}))
	defer server.Close()
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
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{Viewport: &playwright.Size{Width: 1280, Height: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()
	if err := page.AddInitScript(playwright.Script{Content: playwright.String(`Object.defineProperty(window,'caches',{get(){throw new Error('Storage denied')}});`)}); err != nil {
		t.Fatal(err)
	}
	if err := page.Route("**/favicon.ico", func(route playwright.Route) {
		_ = route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(204)})
	}); err != nil {
		t.Fatal(err)
	}
	catalogURL := server.URL + base + "project/"
	if _, err := page.Goto(catalogURL); err != nil {
		t.Fatal(err)
	}
	waitStaticExportReady(t, page)
	checkRows := func(want string) {
		t.Helper()
		value, err := page.Evaluate(`() => [...document.querySelectorAll('#catalog-documents-table [data-catalog-document-row]')].map(row=>row.cells[2].textContent.trim()).join(',')`)
		if err != nil || value != want {
			t.Fatalf("table order=%v, want %s: %v", value, want, err)
		}
	}
	sort := page.Locator(`[data-manja-static-sort="operations"] button`)
	if err := sort.Press("Enter"); err != nil {
		t.Fatal(err)
	}
	checkRows("2,10")
	if err := sort.Press("Space"); err != nil {
		t.Fatal(err)
	}
	checkRows("10,2")
	if direction, err := page.Locator(`[data-manja-static-sort="operations"]`).GetAttribute("aria-sort"); err != nil || direction != "descending" {
		t.Fatalf("sort state=%s %v", direction, err)
	}
	if err := sort.Click(); err != nil {
		t.Fatal(err)
	}
	checkRows("2,10")
	// Both a bookmarked query and a form submission must use the browser index.
	if _, err := page.Goto(catalogURL + "search/?q=alphaOperation0"); err != nil {
		t.Fatal(err)
	}
	alphaResult := page.Locator("#catalog-search-dialog").GetByText("alpha operation 0", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)})
	if err := alphaResult.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#catalog-search-query").Fill("zetaOperation0"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`() => {window.__staticSearchFormMarker=true}`); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("[data-manja-static-search-form] button").Click(); err != nil {
		t.Fatal(err)
	}
	zetaResult := page.Locator("#catalog-search-dialog").GetByText("zeta operation 0", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)})
	if err := zetaResult.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if retained, err := page.Evaluate(`() => window.__staticSearchFormMarker === true`); err != nil || retained != true {
		t.Fatalf("form reloaded static shell: %v %v", retained, err)
	}
	if err := zetaResult.Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => location.search.includes('selected=') && document.querySelector('[data-catalog-main-content]')?.textContent.includes('/items/0')`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`() => {const main=document.querySelector('[data-catalog-main-content]');main.insertAdjacentHTML('beforeend','<a id="anchor-check" href="#native-anchor">Parameters</a><div id="native-anchor">Anchor target</div>')}`); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#anchor-check").Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => location.hash === '#native-anchor' && !!document.querySelector('#anchor-check')`, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, request := range requests {
		if !strings.HasPrefix(request, "GET "+base) || strings.Contains(request, "table_id=") || strings.Contains(request, "/search.json") || strings.Contains(request, "/manage/") || strings.Contains(request, "/api/") {
			t.Fatalf("runtime-dependent request %q", request)
		}
	}
}
