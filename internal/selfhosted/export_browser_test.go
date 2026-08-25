package selfhosted

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestExportBrowserRunsFromGenericStaticServerAtRootAndSubpath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping static export browser acceptance in short mode")
	}
	root := t.TempDir()
	spec := `{"openapi":"3.0.3","info":{"title":"Private API","version":"v1"},"paths":{"/charges":{"get":{"operationId":"listCharges","summary":"List charges","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Charge"}}}}}}}},"components":{"schemas":{"Charge":{"type":"object","properties":{"id":{"type":"string"}}}}}}`
	if err := os.WriteFile(filepath.Join(root, "private.json"), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `version: 1
dataDir: data
catalogs:
  - id: private
    mount: /private
    title: Private
    defaultDocument: private
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [private.json]
`
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pw.Stop() })
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = browser.Close() })

	for _, basePath := range []string{"/", "/group/project/"} {
		t.Run(basePath, func(t *testing.T) {
			output := filepath.Join(root, "public-"+url.PathEscape(basePath))
			if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: basePath}); err != nil {
				t.Fatal(err)
			}
			var requestMu sync.Mutex
			var requests []string
			files := http.FileServer(http.Dir(output))
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				requestMu.Lock()
				requests = append(requests, request.URL.Path)
				requestMu.Unlock()
				if !strings.HasPrefix(request.URL.Path, basePath) {
					http.NotFound(response, request)
					return
				}
				clone := request.Clone(request.Context())
				clone.URL.Path = "/" + strings.TrimPrefix(request.URL.Path, basePath)
				if strings.HasSuffix(clone.URL.Path, ".wasm") {
					response.Header().Set("Content-Type", "application/wasm")
				}
				files.ServeHTTP(response, clone)
			}))
			t.Cleanup(server.Close)

			page, err := browser.NewPage()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = page.Close() })
			documentURL := server.URL + strings.TrimSuffix(basePath, "/") + "/private/documents/private/"
			if _, err := page.Goto(documentURL); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			if _, err := page.Reload(); err != nil {
				state, _ := page.Evaluate(`async () => ({controller: Boolean(navigator.serviceWorker.controller), caches: await caches.keys()})`)
				requestMu.Lock()
				snapshot := append([]string(nil), requests...)
				requestMu.Unlock()
				t.Fatalf("reload: %v; state=%#v requests=%#v", err, state, snapshot)
			}
			waitStaticExportReady(t, page)
			if err := page.Context().SetOffline(true); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = page.Context().SetOffline(false) })

			operation := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "List charges"}).First()
			if err := operation.Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.querySelector('[data-catalog-main-content]').textContent.includes('/charges')`, nil); err != nil {
				t.Fatal(err)
			}
			schema := page.Locator("#catalog-sidebar-groups").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Charge", Exact: playwright.Bool(true)}).First()
			if err := schema.Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.title === 'Charge' && document.querySelector('[data-catalog-main-content]').textContent.includes('Charge')`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5_000)}); err != nil {
				debug, _ := page.Evaluate(`() => ({title: document.title, href: location.href, main: document.querySelector('[data-catalog-main-content]').textContent, links: [...document.querySelectorAll('#catalog-sidebar-groups a')].map((value) => ({text: value.textContent, href: value.href}))})`)
				t.Fatalf("schema navigation: %v; debug=%#v", err, debug)
			}
			requestMu.Lock()
			defer requestMu.Unlock()
			for _, requestPath := range requests {
				if !strings.HasPrefix(requestPath, basePath) {
					t.Fatalf("static browser requested outside deployment base %q", requestPath)
				}
				if strings.Contains(requestPath, "/manage/") || strings.Contains(requestPath, "/api/") || strings.HasSuffix(requestPath, "/search.json") {
					t.Fatalf("static browser requested runtime-only route %q", requestPath)
				}
			}
		})
	}
}

func waitStaticExportReady(t *testing.T, page playwright.Page) {
	t.Helper()
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(60_000)}); err != nil {
		debug, _ := page.Evaluate(`() => ({state: document.documentElement.dataset.manjaLocalDocsState || '', reason: document.documentElement.dataset.manjaLocalDocsReason || '', worker: document.documentElement.dataset.manjaLocalDocsWorker || '', workerReason: document.documentElement.dataset.manjaLocalDocsWorkerReason || ''})`)
		t.Fatalf("static export did not become ready: %v; debug=%#v", err, debug)
	}
}
