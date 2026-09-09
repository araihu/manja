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
	spec := `{"openapi":"3.0.3","info":{"title":"Private API","version":"v1"},"paths":{"/charges":{"get":{"operationId":"listCharges","summary":"List charges","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Charge"}}}}}}},"/customers":{"get":{"operationId":"listCustomers","summary":"List customers with a deliberately long operation title that must truncate before the method badge","responses":{"200":{"description":"ok"}}}}},"components":{"schemas":{"Charge":{"type":"object","properties":{"id":{"type":"string"},"customer":{"$ref":"#/components/schemas/Customer"}}},"Customer":{"type":"object","properties":{"address":{"$ref":"#/components/schemas/Address"}}},"Address":{"type":"object","properties":{"city":{"type":"string"}}}}}}`
	spec = strings.Replace(spec, `"version":"v1"`, `"version":"v1","contact":{"name":"Support","url":"https://example.test/contact","email":"support@example.test"},"license":{"name":"License","url":"https://example.test/license"},"termsOfService":"https://example.test/terms"`, 1)

	if err := os.WriteFile(filepath.Join(root, "private.json"), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	otherSpec := `{"openapi":"3.0.3","info":{"title":"Other API","version":"v1"},"paths":{"/widgets":{"get":{"operationId":"listWidgets","summary":"List widgets","responses":{"200":{"description":"ok"}}}}}}`
	if err := os.WriteFile(filepath.Join(root, "other.json"), []byte(otherSpec), 0o600); err != nil {
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
  - id: other
    mount: /other
    title: Other
    defaultDocument: other
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [other.json]
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
			assertHTMLOnlyBundle(t, output)
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
			deployment := strings.TrimSuffix(basePath, "/")
			documentURL := server.URL + deployment + "/private/documents/private/"
			if _, err := page.Goto(server.URL + basePath); err != nil {
				t.Fatal(err)
			}
			for _, width := range []int{816, 1280} {
				if err := page.SetViewportSize(width, 994); err != nil {
					t.Fatal(err)
				}
				if width < 1024 {
					if err := page.Locator("[data-catalog-navigation-trigger]").Click(); err != nil {
						t.Fatal(err)
					}
				}
				for _, dark := range []bool{false, true} {
					centered, err := page.Evaluate(`dark => {
						document.documentElement.classList.toggle('dark', dark);
						const avatars = [...document.querySelectorAll('.manja-catalog-icon-avatar')];
						return avatars.length > 0 && avatars.every(el => {
							const a = el.getBoundingClientRect();
							const i = el.querySelector('svg').getBoundingClientRect();
							return a.width === 32 && a.height === 32 && i.width === 16 && i.height === 16 &&
								Math.abs(i.left + i.right - a.left - a.right) < 1 &&
								Math.abs(i.top + i.bottom - a.top - a.bottom) < 1;
						});
					}`, dark)
					if err != nil || centered != true {
						t.Fatalf("catalog avatar alignment at width=%d dark=%t: centered=%v err=%v", width, dark, centered, err)
					}
				}
			}
			if _, err := page.Goto(server.URL + deployment + "/private/"); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			catalogNavigation := page.Locator(`#catalog-navigation[aria-label="Catalog documents"]`)
			if visible, err := catalogNavigation.IsVisible(); err != nil || !visible {
				t.Fatalf("catalog overview sidebar visible = %t, %v", visible, err)
			}
			if current, err := catalogNavigation.Locator(`a[aria-current="page"]`).TextContent(); err != nil || !strings.Contains(current, "Catalog overview") {
				t.Fatalf("catalog overview current navigation item = %q, %v", current, err)
			}
			if count, err := catalogNavigation.Locator(`a[data-catalog-document-navigation][href="` + deployment + `/private/documents/private/"]`).Count(); err != nil || count != 1 {
				t.Fatalf("catalog document navigation links = %d, %v; want 1", count, err)
			}
			if count, err := page.Locator(`[data-catalog-navigation-trigger][aria-label="Open Catalog documents"]`).Count(); err != nil || count != 1 {
				t.Fatalf("catalog overview mobile navigation triggers = %d, %v; want 1", count, err)
			}
			if err := page.Locator(`[data-table-row-link]`).First().Click(); err != nil {
				t.Fatal(err)
			}
			if err := page.WaitForURL("**/private/documents/private/**"); err != nil {
				t.Fatalf("desktop document row: %v", err)
			}
			waitStaticExportReady(t, page)
			if reason, err := page.Locator("html").GetAttribute("data-manja-static-fragment-fallback-reason"); err != nil || reason != "" {
				t.Fatalf("pre-rendered initial route fell back: %q %v", reason, err)
			}
			operationsTab := page.Locator(`[role="tab"][data-manja-sidebar-tab="operations"]`)
			schemasTab := page.Locator(`[role="tab"][data-manja-sidebar-tab="schemas"]`)
			if err := operationsTab.Focus(); err != nil {
				t.Fatal(err)
			}
			if err := operationsTab.Press("ArrowRight"); err != nil {
				t.Fatal(err)
			}
			if focused, err := schemasTab.Evaluate("(el) => el === document.activeElement", nil); err != nil || focused != true {
				t.Fatalf("Goshtoso tab keyboard focus: %v, %v", focused, err)
			}
			if err := schemasTab.Press("Enter"); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-sidebar-tab="schemas"]').getAttribute('aria-selected') === 'true' && !document.querySelector('#manja-sidebar-panel-schemas').hidden`, nil); err != nil {
				t.Fatal(err)
			}
			if active, err := schemasTab.Evaluate("(el) => el.classList.contains('border-primary') && getComputedStyle(el).borderBottomWidth === '2px'", nil); err != nil || active != true {
				t.Fatalf("Goshtoso active tab underline: %v, %v", active, err)
			}
			if err := schemasTab.Press("ArrowLeft"); err != nil {
				t.Fatal(err)
			}
			if err := operationsTab.Press("Enter"); err != nil {
				t.Fatal(err)
			}
			groups := page.Locator(`#manja-sidebar-panel-operations section[data-manja-sidebar-group]`)
			// Intercept external destinations so the test never contacts third parties.
			if err := page.Context().Route("https://example.test/**", func(route playwright.Route) {
				_ = route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(200), Body: "External information"})
			}); err != nil {
				t.Fatal(err)
			}
			for _, destination := range []string{"contact", "license", "terms"} {
				href := "https://example.test/" + destination
				infoLink := page.Locator(`dl[aria-label="OpenAPI information"] a[href="` + href + `"]`)
				popup, err := page.ExpectPopup(func() error { return infoLink.Click() })
				if err != nil {
					t.Fatalf("open %s separately: %v", destination, err)
				}
				if err := popup.WaitForURL(href); err != nil {
					t.Fatal(err)
				}
				if opener, err := popup.Evaluate("window.opener === null"); err != nil || opener != true {
					t.Fatalf("unsafe popup opener: %v, %v", opener, err)
				}
				if page.URL() != documentURL {
					t.Fatalf("external link navigated documentation: %s", page.URL())
				}
				_ = popup.Close()
			}
			if count, err := groups.Count(); err != nil || count != 2 {
				t.Fatalf("initial operation groups = %d, %v; want 2", count, err)
			}
			firstExpanded, err := groups.Nth(0).Locator(`[data-manja-static-group]`).GetAttribute("aria-expanded")
			if err != nil || firstExpanded != "true" {
				t.Fatalf("first operation group expanded = %q, %v", firstExpanded, err)
			}
			secondControl := groups.Nth(1).Locator(`[data-manja-static-group]`)
			secondExpanded, err := secondControl.GetAttribute("aria-expanded")
			if err != nil || secondExpanded != "false" {
				t.Fatalf("second operation group expanded = %q, %v", secondExpanded, err)
			}
			longOperationTitle := "List customers with a deliberately long operation title that must truncate before the method badge"
			if count, err := page.Locator(`[data-catalog-sidebar-operation][title="` + longOperationTitle + `"]`).Count(); err != nil || count != 0 {
				t.Fatalf("collapsed group eagerly rendered %d operations: %v", count, err)
			}
			requestMu.Lock()
			requestsBeforeGroupOpen := len(requests)
			requestMu.Unlock()
			if err := secondControl.Click(); err != nil {
				t.Fatal(err)
			}
			longOperation := page.Locator(`[data-catalog-sidebar-operation][title="` + longOperationTitle + `"]`)
			if err := longOperation.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5_000)}); err != nil {
				t.Fatalf("lazy operation group swap: %v", err)
			}
			badgeLayout, err := page.Evaluate(`() => {
				const links = [...document.querySelectorAll('[data-catalog-sidebar-operation]')];
				const shortLink = links.find((link) => link.title === 'List charges');
				const longLink = links.find((link) => link.title.includes('deliberately long'));
				const inspect = (link) => {
					const label = link && link.querySelector('span:first-child');
					const badge = link && link.querySelector('[class*="catalog-method-"]');
					const linkBox = link && link.getBoundingClientRect();
					const badgeBox = badge && badge.getBoundingClientRect();
					return {linkRight: linkBox && linkBox.right, badgeRight: badgeBox && badgeBox.right, badgeLeft: badgeBox && badgeBox.left, labelTruncated: !!label && label.scrollWidth > label.clientWidth};
				};
				const short = inspect(shortLink);
				const long = inspect(longLink);
				return {
					short,
					long,
					aligned: Math.abs(short.badgeRight - long.badgeRight) < 0.5,
					contained: short.linkRight >= short.badgeRight && long.linkRight >= long.badgeRight,
					truncated: long.labelTruncated,
				};
			}`)
			if err != nil {
				t.Fatal(err)
			}
			layout := badgeLayout.(map[string]any)
			if layout["truncated"] != true || layout["aligned"] != true || layout["contained"] != true {
				t.Fatalf("operation badge columns are not stable: %#v", badgeLayout)
			}
			requestMu.Lock()
			requestsAfterGroupOpen := append([]string(nil), requests[requestsBeforeGroupOpen:]...)
			requestMu.Unlock()
			var groupHTML, groupSidecar bool
			for _, requestPath := range requestsAfterGroupOpen {
				if !strings.Contains(requestPath, "/_manja/fragments/sidebar/operations/sidebar-sha256-") {
					continue
				}
				groupHTML = groupHTML || strings.HasSuffix(requestPath, ".html")
				groupSidecar = groupSidecar || strings.HasSuffix(requestPath, ".html.meta.json")
			}
			if !groupHTML || !groupSidecar {
				t.Fatalf("group expansion did not fetch verified operation HTML and sidecar: %#v", requestsAfterGroupOpen)
			}
			operation := page.Locator(`[data-catalog-sidebar-operation][title="List charges"]`).First()
			schema := page.Locator(`#catalog-sidebar-groups [data-manja-static-sidebar-schemas] a[title="Charge"]`).First()
			operationHref, err := operation.GetAttribute("href")
			if err != nil {
				t.Fatal(err)
			}
			schemaHref, err := schema.GetAttribute("href")
			if err != nil {
				t.Fatal(err)
			}
			requestMu.Lock()
			initialRequests := append([]string(nil), requests...)
			requestMu.Unlock()
			for _, requestPath := range initialRequests {
				if strings.Contains(requestPath, "/projection-data/") || strings.Contains(requestPath, "/search-data/") {
					t.Fatalf("static activation eagerly loaded child %q", requestPath)
				}
			}
			assertStaticSidebarLayout(t, page, operation, schema)
			if _, err := page.Evaluate(`() => {
				const response = [...document.querySelectorAll('button')].find((button) => (button.textContent || '').includes('200'));
				if (response && response.getAttribute('aria-expanded') !== 'true') response.click();
				return !!response;
			}`); err != nil {
				t.Fatalf("open response schema: %v", err)
			}
			if err := page.Locator(`[data-manja-static-schema-fragment="true"]`).First().ScrollIntoViewIfNeeded(); err != nil {
				t.Fatalf("reveal lazy schema: %v", err)
			}
			if _, err := page.WaitForFunction(`() => {
				const placeholder = document.querySelector('[data-manja-static-schema-fragment="true"]');
				return placeholder && placeholder.dataset.manjaSchemaState === 'ready' && placeholder.querySelector('[data-schema-tree-node], [data-schema-tree-row]');
			}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(10_000)}); err != nil {
				debug, _ := page.Evaluate(`() => ({placeholder: document.querySelector('[data-manja-static-schema-fragment="true"]')?.outerHTML || '', main: document.querySelector('[data-catalog-main-content]')?.textContent || ''})`)
				t.Fatalf("lazy verified schema composition: %v debug=%#v", err, debug)
			}
			requestMu.Lock()
			lazySchemaRequests := append([]string(nil), requests...)
			requestMu.Unlock()
			var lazySchemaHTML, lazySchemaSidecar bool
			for _, requestPath := range lazySchemaRequests {
				if !strings.Contains(requestPath, "/_manja/fragments/schemas/schema-sha256-") {
					continue
				}
				if strings.HasSuffix(requestPath, ".html") {
					lazySchemaHTML = true
				}
				if strings.HasSuffix(requestPath, ".html.meta.json") {
					lazySchemaSidecar = true
				}
			}
			if !lazySchemaHTML || !lazySchemaSidecar {
				t.Fatalf("lazy schema did not fetch verified HTML and sidecar: %#v", lazySchemaRequests)
			}
			if _, err := page.Evaluate(`() => {
				const abi = window.ManjaLocalDocs;
				const metrics = window.__manjaStaticMetrics = {prepare: 0, admit: 0, admittedPaths: [], longTasks: 0};
				if (abi && typeof abi.prepare === 'function') {
					const prepare = abi.prepare;
					abi.prepare = function (...args) { metrics.prepare += 1; return prepare.apply(this, args); };
				}
				if (abi && typeof abi.admit === 'function') {
					const admit = abi.admit;
					abi.admit = function (path, ...args) { metrics.admit += 1; metrics.admittedPaths.push(path); return admit.call(this, path, ...args); };
				}
				window.__manjaStaticMainNode = document.querySelector('[data-catalog-main-content]');
				const primary = document.querySelector('[data-manja-primary-scroll]');
				const navigation = document.querySelector('[data-manja-local-sidebar]');
				window.__manjaStaticScrollBefore = {main: primary ? primary.scrollTop : 0, sidebar: navigation ? navigation.scrollTop : 0};
				if (window.PerformanceObserver) {
					try { new PerformanceObserver((list) => { metrics.longTasks += list.getEntries().length; }).observe({type: 'longtask', buffered: true}); } catch (_) {}
				}
				return true;
			}`); err != nil {
				t.Fatal(err)
			}
			performanceValues, err := page.Evaluate(`() => {
				const metrics = window.__manjaStaticMetrics || {};
				const primary = document.querySelector('[data-manja-primary-scroll]');
				const navigation = document.querySelector('[data-manja-local-sidebar]');
				const before = window.__manjaStaticScrollBefore || {};
				return {
					prepare: metrics.prepare || 0,
					admit: metrics.admit || 0,
					admittedPaths: metrics.admittedPaths || [],
					longTasks: metrics.longTasks || 0,
					mainNodeSame: window.__manjaStaticMainNode === document.querySelector('[data-catalog-main-content]'),
					mainScrollPreserved: primary ? primary.scrollTop === before.main : true,
					sidebarScrollPreserved: navigation ? navigation.scrollTop === before.sidebar : true,
				};
			}`)
			if err != nil {
				t.Fatal(err)
			}
			performanceMap, ok := performanceValues.(map[string]any)
			prepareCount, prepareOK := browserMetricInt(performanceMap["prepare"])
			if !ok || !prepareOK || prepareCount != 0 || performanceMap["mainNodeSame"] != true || performanceMap["mainScrollPreserved"] != true || performanceMap["sidebarScrollPreserved"] != true {
				t.Fatalf("static incremental navigation metrics = %#v", performanceValues)
			}
			searchField := page.Locator(`[data-search-id="catalog-search"] button`)
			if err := searchField.Click(); err != nil {
				t.Fatal(err)
			}
			searchInput := page.Locator("#catalog-search-input")
			if err := searchInput.Fill("List widgets"); err != nil {
				t.Fatal(err)
			}
			if _, err := searchInput.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
				t.Fatal(err)
			}
			crossCatalog := page.Locator("#catalog-search-dialog").GetByText("List widgets", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)})
			if err := crossCatalog.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5_000)}); err != nil {
				debug, _ := page.Evaluate(`async () => {
				  const dialog = document.querySelector('#catalog-search-dialog');
				  const url = dialog.dataset.searchDeploymentDirectoryUrl;
				  const response = await fetch(url, {headers: {Accept: 'application/json'}});
				  return {text: dialog.textContent || '', dataset: {...dialog.dataset}, status: response.status, directory: await response.text()};
				}`)
				requestMu.Lock()
				searchRequests := append([]string(nil), requests...)
				requestMu.Unlock()
				t.Fatalf("deployment-wide static search: %v debug=%#v requests=%#v", err, debug, searchRequests)
			}
			if err := searchInput.Fill("List charges"); err != nil {
				t.Fatal(err)
			}
			if _, err := searchInput.Evaluate(`element => element.dispatchEvent(new Event('input', { bubbles: true }))`, nil); err != nil {
				t.Fatal(err)
			}
			if err := page.Locator("#catalog-search-dialog").GetByText("List charges", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5_000)}); err != nil {
				debug, debugErr := page.Evaluate(`async () => {
					const dialog = document.querySelector('#catalog-search-dialog');
					const base = dialog.dataset.searchChildBase || '';
					const path = dialog.dataset.searchDirectoryPath || '';
					const url = base + path.split('/').map(encodeURIComponent).join('/');
					const response = await fetch(url, {headers: {Accept: 'application/json'}});
					const bytes = await response.arrayBuffer();
					const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))].map(value => value.toString(16).padStart(2, '0')).join('');
					return {base, path, expectedLength: dialog.dataset.searchDirectoryLength, expectedDigest: dialog.dataset.searchDirectorySha256, url, status: response.status, length: bytes.byteLength, digest, prefix: new TextDecoder().decode(new Uint8Array(bytes).slice(0, 80)), dialog: dialog.textContent || ''};
				}`)
				requestMu.Lock()
				snapshot := append([]string(nil), requests...)
				requestMu.Unlock()
				t.Fatalf("static subpath search: %v; debug=%#v debugErr=%v requests=%#v", err, debug, debugErr, snapshot)
			}
			if _, err := page.Evaluate(`() => { window.__staticSearchNavigationMarker = true; }`, nil); err != nil {
				t.Fatal(err)
			}
			requestMu.Lock()
			beforeSearchNavigation := append([]string(nil), requests...)
			requestMu.Unlock()
			searchResult := page.Locator("#catalog-search-dialog [data-catalog-search-result]").First()
			if err := searchResult.Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => {
				const dialog = document.querySelector('#catalog-search-dialog');
				const focus = document.activeElement;
				return window.__staticSearchNavigationMarker === true && location.search.includes('selected=') && document.title === 'List charges' && document.querySelector('[data-catalog-main-content]')?.textContent.includes('/charges') && dialog && getComputedStyle(dialog).display === 'none' && focus?.matches('[data-manja-settled-focus="true"]');
			}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5_000)}); err != nil {
				debug, _ := page.Evaluate(`() => ({title: document.title, href: location.href, active: document.activeElement && document.activeElement.outerHTML, dialog: document.querySelector('#catalog-search-dialog') && getComputedStyle(document.querySelector('#catalog-search-dialog')).display})`, nil)
				t.Fatalf("static search client navigation: %v; debug=%#v", err, debug)
			}
			requestMu.Lock()
			afterSearchNavigation := append([]string(nil), requests...)
			requestMu.Unlock()
			documentShellPath := strings.TrimSuffix(basePath, "/") + "/private/documents/private/"
			countDocumentShellRequests := func(values []string) int {
				count := 0
				for _, requestPath := range values {
					if requestPath == documentShellPath {
						count++
					}
				}
				return count
			}
			if got, want := countDocumentShellRequests(afterSearchNavigation), countDocumentShellRequests(beforeSearchNavigation); got != want {
				t.Fatalf("search result caused document shell navigation: before=%d after=%d requests=%#v", want, got, afterSearchNavigation)
			}
			newSearchRequests := afterSearchNavigation[len(beforeSearchNavigation):]
			fragmentLoaded := false
			for _, requestPath := range newSearchRequests {
				if strings.Contains(requestPath, "/_manja/fragments/operations/") && strings.HasSuffix(requestPath, ".html") {
					fragmentLoaded = true
				}
				if strings.Contains(requestPath, "/projection-data/") {
					t.Fatalf("pre-rendered operation navigation loaded projection child %q", requestPath)
				}
			}
			if !fragmentLoaded {
				t.Fatalf("operation navigation did not request a pre-rendered fragment: %#v", newSearchRequests)
			}
			if err := page.Keyboard().Press("Escape"); err != nil {
				t.Fatal(err)
			}

			if _, err := page.Goto(server.URL + operationHref); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			if _, err := page.WaitForFunction(`() => document.querySelector('[data-catalog-main-content]').textContent.includes('/charges')`, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := page.Reload(); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)

			if _, err := page.Goto(server.URL + schemaHref); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			if _, err := page.WaitForFunction(`() => document.title === 'Charge' && document.querySelector('[data-catalog-main-content]').textContent.includes('Charge')`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5_000)}); err != nil {
				debug, _ := page.Evaluate(`() => ({title: document.title, href: location.href, main: document.querySelector('[data-catalog-main-content]').textContent, links: [...document.querySelectorAll('#catalog-sidebar-groups a')].map((value) => ({text: value.textContent, href: value.href}))})`)
				t.Fatalf("direct schema navigation: %v; debug=%#v", err, debug)
			}
			if _, err := page.Reload(); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)

			if err := page.Locator(`a[data-catalog-schema-reference="true"]`).GetByText("Customer", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).First().Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => location.search.includes('node=') && document.title === 'Charge' && document.querySelector('#schema-node-panel [data-catalog-schema-property="address"]')`, nil); err != nil {
				t.Fatal(err)
			}
			if err := page.Locator(`#schema-node-panel a[data-catalog-schema-reference="true"]`).GetByText("Address", playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).First().Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.title === 'Charge' && document.querySelector('#schema-node-panel [data-catalog-schema-property="city"]')`, nil); err != nil {
				t.Fatal(err)
			}
			nodeURL := page.URL()
			if _, err := page.Reload(); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			if _, err := page.WaitForFunction(`() => document.querySelector('#schema-node-panel [data-catalog-schema-property="city"]')`, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := page.GoBack(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.querySelector('#schema-node-panel [data-catalog-schema-property="address"]')`, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := page.GoForward(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.querySelector('#schema-node-panel [data-catalog-schema-property="city"]')`, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := page.Goto(documentURL); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			if err := page.Context().SetOffline(true); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = page.Context().SetOffline(false) })

			operation = page.Locator(`[data-catalog-sidebar-operation][title="List charges"]`).First()
			if err := operation.Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.querySelector('[data-catalog-main-content]').textContent.includes('/charges')`, nil); err != nil {
				t.Fatal(err)
			}
			if err := page.Locator(`[role="tab"][data-manja-sidebar-tab="schemas"]`).Click(); err != nil {
				t.Fatalf("open offline schema tab: %v", err)
			}
			schema = page.Locator(`#catalog-sidebar-groups [data-manja-static-sidebar-schemas] a[title="Charge"]`).First()
			if err := schema.Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.title === 'Charge' && document.querySelector('[data-catalog-main-content]').textContent.includes('Charge')`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5_000)}); err != nil {
				debug, _ := page.Evaluate(`() => ({title: document.title, href: location.href, main: document.querySelector('[data-catalog-main-content]').textContent, links: [...document.querySelectorAll('#catalog-sidebar-groups a')].map((value) => ({text: value.textContent, href: value.href}))})`)
				t.Fatalf("offline schema navigation: %v; debug=%#v", err, debug)
			}
			if _, err := page.Goto(nodeURL); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			if _, err := page.WaitForFunction(`() => document.title === 'Charge' && document.querySelector('#schema-node-panel [data-catalog-schema-property="city"]')`, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := page.Reload(); err != nil {
				t.Fatal(err)
			}
			waitStaticExportReady(t, page)
			requestMu.Lock()
			defer requestMu.Unlock()
			for _, requestPath := range requests {
				if strings.Contains(requestPath, "/projection-data/") || strings.HasSuffix(requestPath, ".wasm") || strings.HasSuffix(requestPath, ".wasm.br") || strings.HasSuffix(requestPath, "/wasm_exec.js") {
					t.Fatalf("HTML-only navigation requested rendering input %q", requestPath)
				}
				if !strings.HasPrefix(requestPath, basePath) {
					t.Fatalf("static browser requested outside deployment base %q", requestPath)
				}
				if strings.Contains(requestPath, "/manage/") || strings.Contains(requestPath, "/api/") || strings.HasSuffix(requestPath, "/search") || strings.HasSuffix(requestPath, "/search.json") {
					t.Fatalf("static browser requested runtime-only route %q", requestPath)
				}
			}
		})
	}
}

func browserMetricInt(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int8:
		return int(number), true
	case int16:
		return int(number), true
	case int32:
		return int(number), true
	case int64:
		return int(number), true
	case uint:
		return int(number), uint64(number) <= uint64(^uint(0)>>1)
	case uint8:
		return int(number), true
	case uint16:
		return int(number), true
	case uint32:
		return int(number), uint64(number) <= uint64(^uint(0)>>1)
	case uint64:
		return int(number), number <= uint64(^uint(0)>>1)
	case float64:
		return int(number), number >= 0 && number == float64(int(number))
	default:
		return 0, false
	}
}

func assertStaticSidebarLayout(t *testing.T, page playwright.Page, operation, schema playwright.Locator) {
	t.Helper()
	values, err := page.Evaluate(`() => {
		const nav = document.querySelector('[data-manja-local-sidebar]');
		const operation = document.querySelector('[data-catalog-sidebar-operation]');
		const schema = [...document.querySelectorAll('[data-catalog-sidebar-item]')].find((item) => !item.hasAttribute('data-catalog-sidebar-operation') && item.textContent.trim() === 'Charge');
		const topLinks = [...document.querySelectorAll('[data-manja-static-sidebar-top-link]')];
		const operationSection = document.querySelector('[data-manja-static-sidebar-section="operations"]');
		const schemaSection = document.querySelector('[data-manja-static-sidebar-section="schemas"]');
		const overviewLink = topLinks.find((item) => item.textContent.trim() === 'Spec overview');
		const tabs = document.querySelectorAll('[role="tab"][data-manja-sidebar-tab]');
		const operationStyle = getComputedStyle(operation);
		const labelBox = operation.querySelector('.truncate').getBoundingClientRect();
		const methodBox = operation.querySelector('[data-manja-sidebar-method], [class*="catalog-method-"]').getBoundingClientRect();
		const operationBox = operation.getBoundingClientRect();
		const operationPanel = operation.closest('[data-manja-sidebar-tab-panel]');
		const operationItems = operation.closest('[data-manja-sidebar-items]');
		return {
			navClientWidth: nav.clientWidth,
			navScrollWidth: nav.scrollWidth,
			operationDisplay: operationStyle.display,
			operationMethod: operation.dataset.catalogMethod,
			hasOperationSection: !!operationSection,
			hasSchemaSection: !!schemaSection,
			topLinksPresent: topLinks.length === 1,
			hasBackLink: topLinks.some((item) => item.textContent.includes('Back to')),
			hasOverviewLink: !!overviewLink,
			overviewActive: overviewLink && overviewLink.getAttribute('aria-current') === 'page',
			hasResourceTabs: tabs.length === 2,
			methodBadgeAtRight: methodBox.left >= labelBox.right,
			operationVisible: operationBox.width > 0 && operationBox.height > 0,
			operationPanelHidden: operationPanel && operationPanel.hidden,
			operationItemsHidden: operationItems && operationItems.hidden,
			selectedTabs: [...tabs].filter((tab) => tab.getAttribute('aria-selected') === 'true').map((tab) => tab.dataset.manjaSidebarTab),
			noHorizontalOverflow: nav.scrollWidth <= nav.clientWidth,
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := values.(map[string]any)
	if !ok {
		t.Fatalf("static sidebar layout result = %#v", values)
	}
	if result["noHorizontalOverflow"] != true || result["operationDisplay"] != "grid" || result["operationVisible"] != true || result["operationMethod"] != "GET" || result["hasOperationSection"] != true || result["hasSchemaSection"] != true || result["topLinksPresent"] != true || result["hasBackLink"] != false || result["hasOverviewLink"] != true || result["hasResourceTabs"] != true || result["methodBadgeAtRight"] != true {
		t.Fatalf("static sidebar is not visually usable: %#v", result)
	}
	if err := operation.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-catalog-sidebar-operation][aria-current="page"][data-catalog-sidebar-selected="true"]`).WaitFor(); err != nil {
		t.Fatalf("selected operation state: %v", err)
	}
	if err := page.Locator(`[role="tab"][data-manja-sidebar-tab="schemas"]`).Click(); err != nil {
		t.Fatalf("open schema tab: %v", err)
	}
	if err := schema.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("schema remained discoverable: %v", err)
	}
}

func waitStaticExportReady(t *testing.T, page playwright.Page) {
	t.Helper()
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(60_000)}); err != nil {
		debug, _ := page.Evaluate(`() => ({state: document.documentElement.dataset.manjaLocalDocsState || '', reason: document.documentElement.dataset.manjaLocalDocsReason || '', worker: document.documentElement.dataset.manjaLocalDocsWorker || '', workerReason: document.documentElement.dataset.manjaLocalDocsWorkerReason || ''})`)
		t.Fatalf("static export did not become ready: %v; debug=%#v", err, debug)
	}
}
