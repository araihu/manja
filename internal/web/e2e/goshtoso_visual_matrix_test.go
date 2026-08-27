package e2e

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
	"github.com/araihu/manja/internal/web/templates"
)

func TestGoshtosoAffectedSurfaceVisualMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)
	partialIndex := managementCandidateIndex()
	partialIndex.ProjectID = "partial"
	partialIndex.RevisionID = "rev-last-known-good"
	partialIndex.Title = "Partial API"

	primaryHandler := web.NewServerWithOptions(goshtosoFallbackIndex(), web.Options{
		Management: web.ManagementOptions{
			Store: &visualMatrixPublicationStore{},
			SyncAction: func(_ context.Context, spec web.ManagedSpec, _ string) (web.ManagedSpec, error) {
				time.Sleep(750 * time.Millisecond)
				return spec, errors.New("visual matrix application failure")
			},
			Specs: []web.ManagedSpec{{
				ID:    "payments-api",
				Index: managementCandidateIndex(),
				Project: core.Project{
					ID:   "payments",
					Name: "Acme Payments API",
				},
				Source: core.Source{
					ID:        "local-git/payments-api.git",
					ProjectID: "payments",
					Kind:      "git",
					SpecPath:  "openapi.yaml",
				},
				Revision: core.Revision{
					ID:        "rev-candidate",
					SourceID:  "local-git/payments-api.git",
					Ref:       "main",
					CommitSHA: "64d8e2a013f76b5f28c9f14881065d3f6c4f8e17",
				},
				Candidates: []core.RevisionCandidate{{
					SourceID:  "local-git/payments-api.git",
					Ref:       "main",
					Kind:      "branch",
					CommitSHA: "64d8e2a013f76b5f28c9f14881065d3f6c4f8e17",
				}},
				Publication: core.Publication{
					ProjectID:  "payments",
					RevisionID: "rev-live",
					Public:     true,
					Path:       "/payments/v1",
				},
				SyncRecord: core.SyncRecord{
					ProjectID:  "payments",
					SourceID:   "local-git/payments-api.git",
					RevisionID: "rev-candidate",
					Result:     core.SyncResultSuccess,
					Trigger:    "e2e",
				},
			}, {
				ID:    "partial-api",
				Index: partialIndex,
				Project: core.Project{
					ID:   "partial",
					Name: "Partial API",
				},
				Source:      core.Source{ID: "local-git/partial-api.git", ProjectID: "partial", Kind: "git", SpecPath: "openapi.yaml"},
				Revision:    core.Revision{ID: "rev-last-known-good", SourceID: "local-git/partial-api.git", Ref: "main", CommitSHA: "1111111111111111111111111111111111111111"},
				Candidates:  []core.RevisionCandidate{{SourceID: "local-git/partial-api.git", Ref: "main", Kind: "branch", CommitSHA: "2222222222222222222222222222222222222222"}},
				Publication: core.Publication{ProjectID: "partial", RevisionID: "rev-last-known-good", Public: true, Path: "/partial/v1"},
				SyncRecord:  core.SyncRecord{ProjectID: "partial", SourceID: "local-git/partial-api.git", RevisionID: "rev-last-known-good", Result: core.SyncResultFailure, Trigger: "e2e", ErrorSummary: "latest source refresh failed; serving last-known-good publication"},
			}},
		},
	})
	server := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("HX-Request") == "true" && r.URL.Query().Get("visual_hold") == "true" {
			time.Sleep(2 * time.Second)
		}
		primaryHandler.ServeHTTP(w, r)
	}))
	emptyManagementServer := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manage/specs" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := templates.ManagementSpecsPage(templates.ManagementOverviewModel{}).Render(r.Context(), w); err != nil {
				t.Fatalf("render empty management visual fixture: %v", err)
			}
			return
		}
		primaryHandler.ServeHTTP(w, r)
	}))

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
	browserContext, err := browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer browserContext.Close()
	visualDir := os.Getenv("MANJA_VISUAL_DIR")
	if visualDir != "" {
		if err := os.MkdirAll(visualDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	surfaces := []struct {
		name           string
		serverURL      string
		path           string
		root           string
		kind           string
		state          string
		expectedStatus int
		includeLegacy  bool
	}{
		{name: "public-detail", serverURL: server, path: "/?selected=operation-listpets#operation-listpets", root: "#main-content", kind: "public", state: "success", expectedStatus: 200, includeLegacy: true},
		{name: "public-loading", serverURL: server, path: "/", root: "#main-content", kind: "public", state: "public-loading", expectedStatus: 200},
		{name: "public-not-found", serverURL: server, path: "/?selected=operation-does-not-exist", root: `[data-public-docs-not-found="true"]`, kind: "public", state: "public-not-found", expectedStatus: 200},
		{name: "management-detail", serverURL: server, path: "/manage/spec/payments-api", root: "#management-main-content", kind: "management", state: "success", expectedStatus: 200, includeLegacy: true},
		{name: "management-list", serverURL: server, path: "/manage/specs", root: "#management-main-content", kind: "management", state: "list", expectedStatus: 200},
		{name: "management-empty", serverURL: emptyManagementServer, path: "/manage/specs", root: `[data-management-empty="true"]`, kind: "management", state: "true-empty", expectedStatus: 200},
		{name: "management-filtered-empty", serverURL: server, path: "/manage/specs?q=does-not-exist", root: `[data-management-filtered-empty="true"]`, kind: "management", state: "filtered-empty", expectedStatus: 200},
		{name: "management-not-found", serverURL: server, path: "/manage/spec/does-not-exist", root: `[data-management-spec-not-found="true"]`, kind: "management", state: "not-found", expectedStatus: 404},
		{name: "management-partial", serverURL: server, path: "/manage/spec/partial-api", root: `[data-management-partial="true"]`, kind: "management", state: "partial", expectedStatus: 200},
		{name: "management-loading", serverURL: server, path: "/manage/spec/payments-api", root: "#management-main-content", kind: "management", state: "loading", expectedStatus: 200},
		{name: "management-application-error", serverURL: server, path: "/manage/spec/payments-api", root: "#management-main-content", kind: "management", state: "application-error", expectedStatus: 200},
		{name: "management-transport-error", serverURL: server, path: "/manage/spec/payments-api", root: "#management-main-content", kind: "management", state: "transport-error", expectedStatus: 200},
	}
	for _, surface := range surfaces {
		for _, width := range []int{390, 1440} {
			themes := []string{"araihu", "goshtoso", "minimal"}
			if surface.includeLegacy {
				themes = append(themes, "manja")
			}
			for _, theme := range themes {
				for _, dark := range []bool{false, true} {
					name := fmt.Sprintf("%s/%d/%s/dark-%t", surface.name, width, theme, dark)
					t.Run(name, func(t *testing.T) {
						page, err := browserContext.NewPage()
						if err != nil {
							t.Fatal(err)
						}
						defer page.Close()
						height := 900
						if width == 390 {
							height = 844
						}
						if err := page.SetViewportSize(width, height); err != nil {
							t.Fatal(err)
						}
						if err := page.AddInitScript(playwright.Script{Content: playwright.String(fmt.Sprintf(`
							localStorage.setItem('theme', %q);
							localStorage.setItem('darkMode', %q);
						`, theme, fmt.Sprintf("%t", dark)))}); err != nil {
							t.Fatal(err)
						}

						var mu sync.Mutex
						var pageErrors []string
						var consoleErrors []visualMatrixConsoleError
						page.OnPageError(func(err error) {
							mu.Lock()
							pageErrors = append(pageErrors, err.Error())
							mu.Unlock()
						})
						page.On("console", func(message playwright.ConsoleMessage) {
							if message.Type() != "error" {
								return
							}
							mu.Lock()
							location := message.Location()
							locationURL := ""
							if location != nil {
								locationURL = location.URL
							}
							consoleErrors = append(consoleErrors, visualMatrixConsoleError{Text: message.Text(), URL: locationURL})
							mu.Unlock()
						})

						response, err := page.Goto(surface.serverURL+surface.path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
						if err != nil {
							t.Fatal(err)
						}
						if response == nil || response.Status() != surface.expectedStatus {
							t.Fatalf("GET %s status = %v, want %d", surface.path, response, surface.expectedStatus)
						}
						if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
							t.Fatalf("await Goshtoso dependency readiness: %v", err)
						}
						if _, err := page.WaitForFunction(`() => !document.documentElement.classList.contains('boot')`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
							t.Fatalf("wait for visual boot state to settle: %v", err)
						}
						if err := page.Locator(surface.root).WaitFor(); err != nil {
							t.Fatal(err)
						}
						if surface.state == "not-found" {
							mu.Lock()
							consoleErrors, pageErrors = visualMatrixUnexpectedDiagnostics(visualMatrixConsolePolicy{
								Mode:         "direct",
								State:        surface.state,
								RequestedURL: response.URL(),
								Status:       response.Status(),
							}, consoleErrors, pageErrors)
							mu.Unlock()
						}
						prepareManagementVisualState(t, page, surface.state)
						if surface.state == "transport-error" {
							// An intentionally aborted request emits HTMX/network console errors before
							// the tested recovery UI settles. Start the post-recovery cleanliness window here.
							mu.Lock()
							consoleErrors = nil
							mu.Unlock()
						}
						if width == 390 && surface.name == "management-detail" {
							assertOpenManagementVisualDrawer(t, page, height, visualDir, theme, dark)
						}

						metrics, err := page.Evaluate(`({theme, dark, surface, width}) => {
							const root = document.documentElement;
							const publicSurface = surface === 'public';
							const desktopSidebar = document.querySelector(publicSurface ? 'aside[aria-label="API sections"]' : 'aside[aria-label="Management sections"]');
							const mobileSidebarTrigger = document.querySelector(publicSurface ? '[aria-label="Open API sections"]' : '[aria-label="Open management sections"]');
							const visible = (element) => Boolean(element && element.getClientRects().length > 0);
							const triggerBox = mobileSidebarTrigger?.getBoundingClientRect();
							const main = document.querySelector('#main-content');
							const mainBox = main?.getBoundingClientRect();
							return {
								themeMatches: root.dataset.theme === theme,
								darkMatches: root.classList.contains('dark') === dark,
								overflow: Math.max(root.scrollWidth, document.body.scrollWidth) > window.innerWidth,
								viewportMatches: window.innerWidth === width,
								responsiveShell: width === 390
									? !visible(desktopSidebar) && visible(mobileSidebarTrigger)
									: visible(desktopSidebar) && !visible(mobileSidebarTrigger),
								mobileTarget: width !== 390 || Boolean(triggerBox && triggerBox.width >= 44 && triggerBox.height >= 44),
								primaryScrollOwners: document.querySelectorAll('[data-manja-primary-scroll="true"]').length,
								nestedInteractive: document.querySelectorAll('a a, a button, button a, button button').length,
								mainHorizontalClipping: main?.scrollWidth > main?.clientWidth,
								wideElements: !mainBox ? [] : Array.from(main.querySelectorAll('*')).filter(element => {
									const box = element.getBoundingClientRect();
									return box.right > mainBox.right + 1 || box.width > mainBox.width + 1;
								}).slice(0, 8).map(element => ({
									tag: element.tagName,
									className: element.className?.toString().slice(0, 160),
									width: Math.round(element.getBoundingClientRect().width),
									right: Math.round(element.getBoundingClientRect().right),
								})),
							};
						}`, map[string]any{"theme": theme, "dark": dark, "surface": surface.kind, "width": width})
						if err != nil {
							t.Fatal(err)
						}
						values, ok := metrics.(map[string]any)
						if !ok {
							t.Fatalf("visual matrix metrics should be a map, got %#v", metrics)
						}
						for _, key := range []string{"themeMatches", "darkMatches", "viewportMatches", "responsiveShell", "mobileTarget"} {
							if values[key] != true {
								t.Errorf("%s = %#v, want true; metrics=%#v", key, values[key], values)
							}
						}
						if values["overflow"] == true {
							t.Errorf("horizontal viewport overflow; metrics=%#v", values)
						}
						if values["mainHorizontalClipping"] == true {
							t.Errorf("main workspace clips horizontal content; metrics=%#v", values)
						}
						if fmt.Sprint(values["primaryScrollOwners"]) != "1" {
							t.Errorf("primary scroll owner count = %#v, want 1; metrics=%#v", values["primaryScrollOwners"], values)
						}
						if fmt.Sprint(values["nestedInteractive"]) != "0" {
							t.Errorf("nested interactive count = %#v, want 0; metrics=%#v", values["nestedInteractive"], values)
						}
						if visualDir != "" {
							path := filepath.Join(visualDir, fmt.Sprintf("%s-%d-%s-dark-%t.png", surface.name, width, theme, dark))
							if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(path)}); err != nil {
								t.Fatalf("capture visual evidence: %v", err)
							}
						}
						if surface.state == "public-loading" {
							if _, err := page.WaitForFunction(`() => {
								const loading = document.querySelector('[data-public-docs-loading="true"]');
								const content = document.querySelector('[data-public-docs-content="true"]');
								return Boolean(content?.dataset.selectedDoc === 'operation-listpets' && (!loading || loading.getClientRects().length === 0));
							}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
								t.Fatalf("public loading request did not settle after visual evidence: %v", err)
							}
						}

						mu.Lock()
						defer mu.Unlock()
						if len(pageErrors) != 0 {
							t.Errorf("page errors = %v", pageErrors)
						}
						if len(consoleErrors) != 0 {
							t.Errorf("console errors = %v", consoleErrors)
						}
					})
				}
			}
		}
	}
}

type visualMatrixConsoleError struct {
	Text string
	URL  string
}

type visualMatrixConsolePolicy struct {
	Mode         string
	State        string
	RequestedURL string
	Status       int
}

func visualMatrixUnexpectedConsoleErrors(policy visualMatrixConsolePolicy, captured []visualMatrixConsoleError) []visualMatrixConsoleError {
	unexpected := make([]visualMatrixConsoleError, 0, len(captured))
	for _, consoleError := range captured {
		expectedDirectNotFound := policy.Mode == "direct" &&
			policy.State == "not-found" &&
			policy.Status == 404 &&
			consoleError.URL == policy.RequestedURL &&
			consoleError.Text == "Failed to load resource: the server responded with a status of 404 (Not Found)"
		if !expectedDirectNotFound {
			unexpected = append(unexpected, consoleError)
		}
	}
	return unexpected
}

func visualMatrixUnexpectedDiagnostics(policy visualMatrixConsolePolicy, consoleErrors []visualMatrixConsoleError, pageErrors []string) ([]visualMatrixConsoleError, []string) {
	return visualMatrixUnexpectedConsoleErrors(policy, consoleErrors), append([]string(nil), pageErrors...)
}

func TestVisualMatrixConsoleClassifierFailsClosed(t *testing.T) {
	requestedURL := "http://127.0.0.1:8080/manage/spec/does-not-exist"
	exact404 := visualMatrixConsoleError{
		Text: "Failed to load resource: the server responded with a status of 404 (Not Found)",
		URL:  requestedURL,
	}
	base := visualMatrixConsolePolicy{Mode: "direct", State: "not-found", RequestedURL: requestedURL, Status: 404}
	if got := visualMatrixUnexpectedConsoleErrors(base, []visualMatrixConsoleError{exact404}); len(got) != 0 {
		t.Fatalf("exact requested direct 404 should be scoped, got %#v", got)
	}

	negativeControls := []struct {
		name   string
		policy visualMatrixConsolePolicy
		error  visualMatrixConsoleError
	}{
		{name: "unexpected-first-party-503", policy: visualMatrixConsolePolicy{Mode: "direct", State: "not-found", RequestedURL: requestedURL, Status: 503}, error: visualMatrixConsoleError{Text: "Failed to load resource: the server responded with a status of 503 (Service Unavailable)", URL: requestedURL}},
		{name: "unrelated-javascript-error", policy: base, error: visualMatrixConsoleError{Text: "Uncaught Error: unrelated first-party failure", URL: requestedURL}},
		{name: "wrong-document", policy: base, error: visualMatrixConsoleError{Text: exact404.Text, URL: "http://127.0.0.1:8080/unrelated.js"}},
		{name: "htmx-mode", policy: visualMatrixConsolePolicy{Mode: "htmx", State: "not-found", RequestedURL: requestedURL, Status: 404}, error: exact404},
	}
	for _, control := range negativeControls {
		t.Run(control.name, func(t *testing.T) {
			got := visualMatrixUnexpectedConsoleErrors(control.policy, []visualMatrixConsoleError{control.error})
			if len(got) != 1 || got[0] != control.error {
				t.Fatalf("negative control was suppressed: got %#v, want %#v", got, control.error)
			}
		})
	}

	pageError := "Uncaught Error: permanent pageerror negative control"
	_, gotPageErrors := visualMatrixUnexpectedDiagnostics(base, nil, []string{pageError})
	if len(gotPageErrors) != 1 || gotPageErrors[0] != pageError {
		t.Fatalf("pageerror negative control was suppressed: %#v", gotPageErrors)
	}
}

func prepareManagementVisualState(t *testing.T, page playwright.Page, state string) {
	t.Helper()
	if state == "public-loading" {
		if _, err := page.Evaluate(`() => {
			const href = '/?selected=operation-listpets&visual_hold=true#operation-listpets';
			const trigger = document.createElement('button');
			trigger.type = 'button';
			trigger.textContent = 'Load visual operation';
			trigger.setAttribute('hx-get', href);
			trigger.setAttribute('hx-target', '#main-content');
			trigger.setAttribute('hx-swap', 'innerHTML');
			trigger.setAttribute('hx-indicator', '#public-docs-loading');
			document.body.appendChild(trigger);
			window.htmx.process(trigger);
			trigger.click();
		}`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := page.WaitForFunction(`() => {
			const loading = document.querySelector('[data-public-docs-loading="true"]');
			return Boolean(loading && loading.getClientRects().length > 0 && loading.getAttribute('aria-busy') === 'true');
		}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
			t.Fatalf("wait for public visual loading state: %v", err)
		}
		if _, err := page.Evaluate(`() => document.querySelector('#main-content')?.scrollTo({ top: 0 })`, nil); err != nil {
			t.Fatal(err)
		}
		return
	}
	if state != "loading" && state != "application-error" && state != "transport-error" {
		return
	}
	if err := page.Locator(`[role="tab"]:has-text("Sync")`).Click(); err != nil {
		t.Fatal(err)
	}
	button := page.Locator(`#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]`)
	if state == "transport-error" {
		if err := page.Route("**/manage/sync", func(route playwright.Route) {
			_ = route.Abort()
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := button.Evaluate(`button => button.click()`, nil); err != nil {
		t.Fatal(err)
	}
	switch state {
	case "loading":
		if _, err := page.WaitForFunction(`() => {
			const button = document.querySelector('#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]');
			return Boolean(button && button.disabled && button.textContent.includes('Syncing ref'));
		}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
			t.Fatalf("wait for visual loading state: %v", err)
		}
		if err := button.ScrollIntoViewIfNeeded(); err != nil {
			t.Fatalf("bring visual loading state into view: %v", err)
		}
	case "application-error":
		if err := page.Locator(`[data-management-application-error="true"]`).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5000)}); err != nil {
			t.Fatalf("wait for visual application error: %v", err)
		}
	case "transport-error":
		if err := page.Locator(`[data-management-transport-recovery="true"]`).WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: playwright.Float(5000)}); err != nil {
			t.Fatalf("wait for visual transport recovery: %v", err)
		}
	}
}

func assertOpenManagementVisualDrawer(t *testing.T, page playwright.Page, viewportHeight int, visualDir, theme string, dark bool) {
	t.Helper()
	trigger := page.Locator(`button[aria-label="Open management sections"]`)
	if err := trigger.Click(); err != nil {
		t.Fatal(err)
	}
	panel := page.Locator(`#management-sidebar-panel`)
	if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => {
		const panel = document.querySelector('#management-sidebar-panel');
		if (!panel) return false;
		const box = panel.getBoundingClientRect();
		return box.left >= -1 && box.right > 0 && box.left < innerWidth && box.bottom > 0 && box.top < innerHeight;
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("management drawer did not enter the viewport: %v", err)
	}
	page.WaitForTimeout(250)
	panelBox, err := panel.BoundingBox()
	if err != nil {
		t.Fatal(err)
	}
	headerBox, err := page.Locator(`header[data-boot-anim="header"]`).BoundingBox()
	if err != nil {
		t.Fatal(err)
	}
	if panelBox == nil || panelBox.X < -1 || panelBox.X >= 390 || panelBox.X+panelBox.Width <= 0 || panelBox.Y >= float64(viewportHeight) || panelBox.Y+panelBox.Height <= 0 {
		t.Fatalf("management drawer must positively intersect the viewport, box=%#v", panelBox)
	}
	if headerBox == nil || panelBox.Y < headerBox.Y+headerBox.Height-1 {
		t.Fatalf("management drawer must be viewport-owned below the header, panel=%#v header=%#v", panelBox, headerBox)
	}
	if visualDir != "" {
		path := filepath.Join(visualDir, fmt.Sprintf("management-drawer-390-%s-dark-%t.png", theme, dark))
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(path)}); err != nil {
			t.Fatalf("capture management drawer visual evidence: %v", err)
		}
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => {
		const trigger = document.querySelector('button[aria-label="Open management sections"]');
		const panel = document.querySelector('#management-sidebar-panel');
		return trigger?.getAttribute('aria-expanded') === 'false' && document.activeElement === trigger && (!panel || panel.getClientRects().length === 0);
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("management drawer did not close and restore focus on Escape: %v", err)
	}
}

type visualMatrixPublicationStore struct{}

func (*visualMatrixPublicationStore) SavePublication(context.Context, core.Publication) error {
	return nil
}
