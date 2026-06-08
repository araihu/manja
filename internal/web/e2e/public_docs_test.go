package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/playwright-community/playwright-go"

	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web"
)

func TestPublicDocsSearchKeyboard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	const operationAnchor = "operation-listpets"
	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{{
			ID:      "listPets",
			Method:  "GET",
			Path:    "/pets",
			Summary: "List pets",
			Tags:    []string{"Pets"},
			Anchor:  operationAnchor,
		}},
		Search: []core.SearchDocument{{
			ID:          operationAnchor,
			Title:       "GET /pets",
			Description: "List pets",
			Href:        "#" + operationAnchor,
			Kind:        "Operation",
			Section:     "Pets",
		}},
	}
	server := httptestServer(t, web.NewPublicServer(idx))

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
	if _, err := page.Goto(server); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	input := page.Locator("#docs-search-input")
	if err := input.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := input.Fill("pets"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#search-operation-listpets:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-listpets:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if got := page.URL(); got != server+"/?selected=operation-listpets#operation-listpets" {
		t.Fatalf("page URL = %q, want %q", got, server+"/?selected=operation-listpets#operation-listpets")
	}
}

func TestPublicDocsThemeSelectDropdownOverlaysContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{{
			ID:      "listPets",
			Method:  "GET",
			Path:    "/pets",
			Summary: "List pets",
			Tags:    []string{"Pets"},
			Anchor:  "operation-listpets",
		}},
	}
	server := httptestServer(t, web.NewPublicServer(idx))

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
		Viewport: &playwright.Size{Width: 1024, Height: 768},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#manja-theme-trigger").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#manja-theme-trigger ~ div[role='listbox']").WaitFor(); err != nil {
		t.Fatal(err)
	}

	result, err := page.Evaluate(`() => {
		const trigger = document.getElementById('manja-theme-trigger');
		const menu = trigger.parentElement.querySelector('[role="listbox"]');
		const header = document.querySelector('.manja-docs-header');
		const menuRect = menu.getBoundingClientRect();
		const headerRect = header.getBoundingClientRect();
		const x = Math.min(menuRect.left + 12, menuRect.right - 1);
		const y = Math.min(headerRect.bottom + 12, menuRect.bottom - 1);
		const hit = document.elementFromPoint(x, y);

		return {
			headerOverflow: getComputedStyle(header).overflow,
			headerBottom: headerRect.bottom,
			menuTop: menuRect.top,
			menuBottom: menuRect.bottom,
			probeX: x,
			probeY: y,
			hitTag: hit ? hit.tagName : '',
			hitText: hit ? hit.textContent.trim() : '',
			menuContainsHit: !!hit && menu.contains(hit),
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if resultMap, ok := result.(map[string]any); !ok || resultMap["menuContainsHit"] != true {
		t.Fatalf("theme dropdown should overlay content below the header, got %#v", result)
	}
}

func TestPublicDocsSidebarNavigationSwapsMainContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	const operationAnchor = "operation-target"
	operations := make([]core.Operation, 0, 45)
	for i := 0; i < 44; i++ {
		operations = append(operations, core.Operation{
			ID:      fmt.Sprintf("filler%d", i),
			Method:  "GET",
			Path:    fmt.Sprintf("/filler/%d", i),
			Summary: fmt.Sprintf("Filler operation %d", i),
			Tags:    []string{"Pets"},
			Anchor:  fmt.Sprintf("operation-filler-%d", i),
		})
	}
	operations = append(operations, core.Operation{
		ID:          "target",
		Method:      "GET",
		Path:        "/target",
		Summary:     "Target operation",
		Description: "Target body",
		Tags:        []string{"Pets"},
		Anchor:      operationAnchor,
	})
	idx := core.SpecIndex{
		Title:      "Petstore",
		Version:    "1.0.0",
		Operations: operations,
		Search: []core.SearchDocument{{
			ID:          operationAnchor,
			Title:       "GET /target",
			Description: "Target operation",
			Href:        "#" + operationAnchor,
			Kind:        "Operation",
			Section:     "Pets",
		}},
	}
	server := httptestServer(t, web.NewPublicServer(idx))

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
	if _, err := page.Goto(server); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate("() => { window.__manjaReloadSentinel = 'kept'; }"); err != nil {
		t.Fatal(err)
	}
	scrolled, err := page.Evaluate(`() => {
		const sidebar = document.querySelector('.sidebar-scroll');
		if (!sidebar) return false;
		sidebar.scrollTop = sidebar.scrollHeight;
		return sidebar.scrollTop > 0;
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if scrolled != true {
		t.Fatalf("test setup could not scroll the sidebar")
	}

	link := page.Locator(`aside a[href="/?selected=operation-target#operation-target"]`).Last()
	if err := link.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-target:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	kept, err := page.Evaluate("() => window.__manjaReloadSentinel === 'kept'")
	if err != nil {
		t.Fatal(err)
	}
	if kept != true {
		t.Fatalf("sidebar navigation performed a full page reload instead of preserving page state")
	}
	scrollPreserved, err := page.Evaluate(`() => {
		const sidebar = document.querySelector('.sidebar-scroll');
		return !!sidebar && sidebar.scrollTop > 0;
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if scrollPreserved != true {
		t.Fatalf("sidebar navigation reset the sidebar scroll position")
	}
	operationActive, err := page.Evaluate(`() => Array.from(document.querySelectorAll('aside a[href="/?selected=operation-target#operation-target"]')).some((link) => link.querySelector('.sr-only')?.textContent.trim() === 'active')`)
	if err != nil {
		t.Fatal(err)
	}
	if operationActive != true {
		t.Fatalf("sidebar navigation did not update the selected operation active marker")
	}
	overviewActive, err := page.Evaluate(`() => document.querySelector('aside a[href="/?selected=overview#overview"] .sr-only')?.textContent.trim() === 'active'`)
	if err != nil {
		t.Fatal(err)
	}
	if overviewActive == true {
		t.Fatalf("sidebar navigation left the previous overview active marker in place")
	}
	if got := page.URL(); got != server+"/?selected=operation-target#operation-target" {
		t.Fatalf("page URL = %q, want %q", got, server+"/?selected=operation-target#operation-target")
	}
}

func TestPublicDocsContainsScrollInDocsPanes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	idx := scrollStressIndex()
	const selectedAnchor = "operation-listteams-00"
	server := httptestServer(t, web.NewPublicServer(idx))

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
		Viewport: &playwright.Size{Width: 1024, Height: 768},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/?selected=" + selectedAnchor + "#" + selectedAnchor); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#" + selectedAnchor + ":visible").WaitFor(); err != nil {
		t.Fatal(err)
	}

	result, err := page.Evaluate(`() => {
		const documentScroller = document.scrollingElement;
		const body = document.body;
		const shell = body.firstElementChild;
		const main = document.querySelector('main');
		const aside = document.querySelector('aside');
		const sidebar = document.querySelector('.sidebar-scroll');
		const overflows = (scrollSize, clientSize) => scrollSize > clientSize + 1;

		return {
			documentScrollableX: overflows(documentScroller.scrollWidth, documentScroller.clientWidth),
			documentScrollableY: overflows(documentScroller.scrollHeight, documentScroller.clientHeight),
			bodyScrollableX: overflows(body.scrollWidth, window.innerWidth),
			bodyScrollableY: overflows(body.scrollHeight, window.innerHeight),
			mainScrollableY: main ? overflows(main.scrollHeight, main.clientHeight) : false,
			sidebarScrollableY: sidebar ? overflows(sidebar.scrollHeight, sidebar.clientHeight) : false,
			mainRectHeight: main ? Math.round(main.getBoundingClientRect().height) : 0,
			asideRectHeight: aside ? Math.round(aside.getBoundingClientRect().height) : 0,
			windowInnerHeight: window.innerHeight,
			shellOverflow: shell ? getComputedStyle(shell).overflow : '',
			mainOverflowY: main ? getComputedStyle(main).overflowY : '',
			sidebarOverflowY: sidebar ? getComputedStyle(sidebar).overflowY : '',
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("scroll metrics should be a map, got %#v", result)
	}
	for _, key := range []string{"documentScrollableX", "bodyScrollableX"} {
		if metrics[key] == true {
			t.Fatalf("public docs should not create document-level scrollbars; %s=true in metrics %#v", key, metrics)
		}
	}
	for _, key := range []string{"documentScrollableY", "bodyScrollableY"} {
		if metrics[key] == true {
			t.Fatalf("public docs should contain vertical scrolling inside docs panes; %s=true in metrics %#v", key, metrics)
		}
	}
	if metrics["mainScrollableY"] != true {
		t.Fatalf("public docs should scroll endpoint content inside the main pane, got metrics %#v", metrics)
	}
	if metrics["sidebarScrollableY"] != true {
		t.Fatalf("test setup should keep long navigation scrollable inside the sidebar, got metrics %#v", metrics)
	}
	wantPaneHeight := metricNumber(t, metrics, "windowInnerHeight") - 64
	for _, key := range []string{"mainRectHeight", "asideRectHeight"} {
		if got := metricNumber(t, metrics, key); got != wantPaneHeight {
			t.Fatalf("%s should fill the viewport below the header; want %v got %v, metrics %#v", key, wantPaneHeight, got, metrics)
		}
	}
}

func metricNumber(t *testing.T, metrics map[string]any, key string) float64 {
	t.Helper()
	switch value := metrics[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		t.Fatalf("metric %s should be numeric, got %#v", key, value)
		return 0
	}
}

func scrollStressIndex() core.SpecIndex {
	operations := make([]core.Operation, 0, 48)
	search := make([]core.SearchDocument, 0, 48)
	for i := 0; i < 48; i++ {
		anchor := fmt.Sprintf("operation-listteams-%02d", i)
		operations = append(operations, core.Operation{
			ID:          fmt.Sprintf("listTeams%02d", i),
			Anchor:      anchor,
			Method:      "GET",
			Path:        fmt.Sprintf("/orgs/{org}/teams/%02d", i),
			Summary:     fmt.Sprintf("List teams %02d", i),
			Description: "Returns teams for an organization with membership metadata, permissions, and pagination details.",
			Tags:        []string{"Teams"},
			Parameters: []core.OperationParameter{{
				Name:        "org",
				In:          "path",
				Required:    true,
				Description: "The organization name.",
				Schema:      core.SchemaSummary{Type: "string"},
			}, {
				Name:        "per_page",
				In:          "query",
				Description: "The number of results per page.",
				Schema:      core.SchemaSummary{Type: "integer"},
			}, {
				Name:        "page",
				In:          "query",
				Description: "The page number of the results to fetch.",
				Schema:      core.SchemaSummary{Type: "integer"},
			}},
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "OK",
			}, {
				Status:      "404",
				Description: "Not Found",
			}},
		})
		search = append(search, core.SearchDocument{
			ID:          anchor,
			Title:       fmt.Sprintf("GET /orgs/{org}/teams/%02d", i),
			Description: fmt.Sprintf("List teams %02d", i),
			Href:        "#" + anchor,
			Kind:        "Operation",
			Section:     "Teams",
		})
	}

	return core.SpecIndex{
		Title:      "Teams API",
		Version:    "1.0.0",
		Operations: operations,
		Search:     search,
	}
}

func chdirRepoRoot(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("find repo root from %s: %v", cwd, err)
	}
	t.Chdir(root)
}

func httptestServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return "http://" + listener.Addr().String()
}
