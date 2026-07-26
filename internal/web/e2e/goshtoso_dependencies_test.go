package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/playwright-community/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

func TestPublicDocsGoshtosoCDNFailureUsesEmbeddedFallback(t *testing.T) {
	testPublicDocsGoshtosoDependencyJourney(t, true)
}

func TestPublicDocsGoshtosoCDNPrimaryJourney(t *testing.T) {
	testPublicDocsGoshtosoDependencyJourney(t, false)
}

func testPublicDocsGoshtosoDependencyJourney(t *testing.T, forceFallback bool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)
	server := httptestServer(t, web.NewPublicServer(goshtosoFallbackIndex()))

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
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{Width: 1440, Height: 900},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := page.AddInitScript(playwright.Script{Content: playwright.String(`
		window.__manjaDependencyEvents = { fallbacks: [], ready: 0, errors: [], rejections: [] };
		window.addEventListener("goshtoso:dependency-fallback", event => {
			window.__manjaDependencyEvents.fallbacks.push(event.detail.dependency);
		});
		window.addEventListener("goshtoso:dependencies-ready", () => {
			window.__manjaDependencyEvents.ready += 1;
		});
		window.addEventListener("goshtoso:dependency-error", event => {
			window.__manjaDependencyEvents.errors.push(event.detail && event.detail.dependency);
		});
		window.addEventListener("unhandledrejection", event => {
			window.__manjaDependencyEvents.rejections.push(String(event.reason));
		});
	`)}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var routeErrors []error
	if forceFallback {
		if err := page.Route("https://unpkg.com/**", func(route playwright.Route) {
			if err := route.Fulfill(playwright.RouteFulfillOptions{
				Status:      playwright.Int(503),
				Body:        "simulated CDN outage",
				ContentType: playwright.String("text/plain"),
			}); err != nil {
				mu.Lock()
				routeErrors = append(routeErrors, err)
				mu.Unlock()
			}
		}); err != nil {
			t.Fatal(err)
		}
	}

	var pageErrors []string
	var consoleErrors []string
	stage := "boot"
	setStage := func(next string) {
		mu.Lock()
		stage = next
		mu.Unlock()
	}
	page.OnPageError(func(err error) {
		mu.Lock()
		pageErrors = append(pageErrors, stage+": "+err.Error())
		mu.Unlock()
	})
	page.On("console", func(message playwright.ConsoleMessage) {
		if message.Type() != "error" {
			return
		}
		mu.Lock()
		consoleErrors = append(consoleErrors, message.Text())
		mu.Unlock()
	})

	_, err = page.Goto(server+"/?selected=operation-listpets#operation-listpets", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}

	type dependencyEvidence struct {
		Fallbacks  []string          `json:"fallbacks"`
		Ready      int               `json:"ready"`
		Errors     []string          `json:"errors"`
		Rejections []string          `json:"rejections"`
		Sources    map[string]string `json:"sources"`
	}
	raw, err := page.Evaluate(`() => JSON.stringify({
		fallbacks: window.__manjaDependencyEvents.fallbacks,
		ready: window.__manjaDependencyEvents.ready,
		errors: window.__manjaDependencyEvents.errors,
		rejections: window.__manjaDependencyEvents.rejections,
		sources: window.goshtosoDependencies.sources
	})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var evidence dependencyEvidence
	if err := json.Unmarshal([]byte(fmt.Sprint(raw)), &evidence); err != nil {
		t.Fatalf("decode browser dependency evidence: %v (%#v)", err, raw)
	}
	wantFallbacks := []string{}
	wantSource := "primary"
	if forceFallback {
		wantFallbacks = []string{"alpine-collapse", "alpine-focus", "alpine-mask", "alpine", "htmx"}
		wantSource = "fallback"
	}
	if fmt.Sprint(evidence.Fallbacks) != fmt.Sprint(wantFallbacks) {
		t.Errorf("fallback events = %v, want %v", evidence.Fallbacks, wantFallbacks)
	}
	if evidence.Ready != 1 {
		t.Errorf("ready event count = %d, want 1", evidence.Ready)
	}
	if len(evidence.Errors) != 0 {
		t.Errorf("dependency error events = %v, want none", evidence.Errors)
	}
	if len(evidence.Rejections) != 0 {
		t.Errorf("unhandled rejections = %v, want none", evidence.Rejections)
	}
	for _, name := range []string{"alpine-collapse", "alpine-focus", "alpine-mask", "alpine", "htmx"} {
		if evidence.Sources[name] != wantSource {
			t.Errorf("%s source = %q, want %s", name, evidence.Sources[name], wantSource)
		}
	}
	if evidence.Sources["combobox"] != "primary" {
		t.Errorf("combobox source = %q, want primary", evidence.Sources["combobox"])
	}

	setStage("disclosure")
	disclosure := page.Locator(`aside a[aria-controls="tag-pets-children"]`)
	if err := disclosure.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#tag-pets-children").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if expanded, err := disclosure.GetAttribute("aria-expanded"); err != nil || expanded != "false" {
		t.Fatalf("collapsed disclosure aria-expanded = %q, err = %v", expanded, err)
	}
	if err := disclosure.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#tag-pets-children").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}

	setStage("search")
	if err := page.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.activeElement && document.activeElement.id === "docs-search-input"`, nil); err != nil {
		t.Fatalf("search focus after CDN fallback: %v", err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#docs-search-dialog").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}

	setStage("htmx-navigation")
	openSidebarTagGroup(t, page, "tag-stores-children")
	storeLink := page.Locator(`aside a[href="/?selected=operation-createstore#operation-createstore"]`)
	if err := storeLink.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-createstore:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if got, want := page.URL(), server+"/?selected=operation-createstore#operation-createstore"; got != want {
		t.Fatalf("HTMX navigation URL = %q, want %q", got, want)
	}
	setStage("history-back")
	if _, err := page.GoBack(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-listpets:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	setStage("history-forward")
	if _, err := page.GoForward(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-createstore:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}

	overflow, err := page.Evaluate(`() => Math.max(document.documentElement.scrollWidth, document.body.scrollWidth) > window.innerWidth`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if overflow == true {
		t.Error("public docs page has horizontal overflow after dependency fallback interactions")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(routeErrors) != 0 {
		t.Errorf("CDN interception errors = %v", routeErrors)
	}
	if len(pageErrors) != 0 {
		t.Errorf("page errors after dependency journey = %v", pageErrors)
	}
	for _, message := range consoleErrors {
		if !strings.Contains(message, "Failed to load resource") {
			t.Errorf("unexpected console error after dependency journey: %s", message)
		}
	}
}

func goshtosoFallbackIndex() core.SpecIndex {
	return core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{
			{ID: "listPets", Anchor: "operation-listpets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createStore", Anchor: "operation-createstore", Method: "POST", Path: "/stores", Summary: "Create store", Tags: []string{"Stores"}},
		},
		Search: []core.SearchDocument{
			{ID: "operation-listpets", Title: "GET /pets", Description: "List pets", Href: "#operation-listpets", Kind: "Operation", Section: "Pets"},
			{ID: "operation-createstore", Title: "POST /stores", Description: "Create store", Href: "#operation-createstore", Kind: "Operation", Section: "Stores"},
		},
	}
}
