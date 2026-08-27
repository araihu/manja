package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"

	core "github.com/araihu/manja/domain"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
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

func TestPublicDocsSearchMatchesFuzzyQueries(t *testing.T) {
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
	if err := input.Fill("lst pts"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#search-operation-listpets:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestComposerUpdatesRequestSample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	const operationAnchor = "operation-list-global-webhooks"
	idx := core.SpecIndex{
		Title: "GitHub REST",
		Overview: core.SpecOverview{
			Servers: []core.SpecServer{{
				URL: "{protocol}://{hostname}/api/v3",
				Variables: []core.SpecServerVariable{{
					Name:    "hostname",
					Default: "HOSTNAME",
				}, {
					Name:    "protocol",
					Default: "http",
				}},
			}},
		},
		Operations: []core.Operation{{
			ID:      "listGlobalWebhooks",
			Anchor:  operationAnchor,
			Method:  "POST",
			Path:    "/admin/hooks",
			Summary: "List global webhooks",
			Tags:    []string{"enterprise-admin"},
			Parameters: []core.OperationParameter{{
				Name:     "page",
				In:       "query",
				Schema:   core.SchemaSummary{Type: "integer", Default: "1"},
				Example:  "1",
				Required: false,
			}, {
				Name:     "accept",
				In:       "header",
				Required: true,
				Schema:   core.SchemaSummary{Type: "string", Default: "application/vnd.github.superpro-preview+json"},
			}},
			RequestBody: &core.OperationRequestBody{
				MediaTypes: []core.OperationMediaType{{
					ContentType:     "application/json",
					Schema:          core.SchemaSummary{Type: "object", JSON: `{"type":"object","properties":{"name":{"type":"string"}}}`},
					Example:         "{\n  \"name\": \"web\"\n}",
					ExampleProvided: true,
				}},
			},
			Snippets: []core.RequestSnippet{{
				Label:    "cURL",
				Language: "shell",
				Code:     "curl --request POST --url {protocol}://{hostname}/api/v3/admin/hooks",
			}},
		}},
		Search: []core.SearchDocument{{
			ID:      operationAnchor,
			Title:   "POST /admin/hooks",
			Href:    "#" + operationAnchor,
			Kind:    "Operation",
			Section: "enterprise-admin",
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
		Viewport: &playwright.Size{Width: 1440, Height: 900},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/?selected=" + operationAnchor + "#" + operationAnchor); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-config-root]')?.getAttribute('data-manja-request-config-enhanced') === 'true'`, nil); err != nil {
		t.Fatalf("request configuration enhancement: %v", err)
	}
	if err := page.Locator(".manja-request-config-trigger-bar").Click(); err != nil {
		t.Fatal(err)
	}

	sample := page.Locator(".manja-endpoint-examples-rail [data-manja-request-sample] .codeblock")
	if err := sample.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".manja-endpoint-examples-rail [data-manja-request-config-panel]").WaitFor(); err != nil {
		t.Fatal(err)
	}
	themeResult, err := page.Evaluate(`() => {
		const panel = document.querySelector('[data-manja-request-config-panel]');
		const heading = panel?.querySelector('[id$="-request-config-heading"]');
		const editor = panel?.querySelector('[data-manja-request-body-editor]');
		const textarea = panel?.querySelector('[data-manja-request-body-input]');
		const bodyHighlight = panel?.querySelector('[data-manja-request-body-highlight]');
		const readStyles = () => ({
			headingColor: getComputedStyle(heading).color,
			editorBackground: getComputedStyle(editor).backgroundColor,
			editorBorderColor: getComputedStyle(editor).borderColor,
			textareaCaretColor: getComputedStyle(textarea).caretColor,
		});
		document.documentElement.classList.remove('dark');
		const light = readStyles();
		document.documentElement.classList.add('dark');
		const dark = readStyles();
		return {
			panelForcedDark: panel.classList.contains('dark'),
			lightHeadingColor: light.headingColor,
			darkHeadingColor: dark.headingColor,
			lightEditorBackground: light.editorBackground,
			darkEditorBackground: dark.editorBackground,
			lightEditorBorderColor: light.editorBorderColor,
			darkEditorBorderColor: dark.editorBorderColor,
			lightTextareaCaretColor: light.textareaCaretColor,
			darkTextareaCaretColor: dark.textareaCaretColor,
			bodyHighlighted: editor?.dataset.manjaRequestBodyHighlighted === 'true',
			bodyTokenCount: bodyHighlight?.querySelectorAll('[class^="hljs-"], [class*=" hljs-"]').length || 0,
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	themeStyles, ok := themeResult.(map[string]any)
	if !ok {
		t.Fatalf("request config theme styles should be a map, got %#v", themeResult)
	}
	if themeStyles["panelForcedDark"] == true {
		t.Fatalf("request config panel should not force dark mode, got %#v", themeStyles)
	}
	if themeStyles["lightHeadingColor"] == themeStyles["darkHeadingColor"] {
		t.Fatalf("request config heading should react to dark mode, got %#v", themeStyles)
	}
	if themeStyles["lightEditorBackground"] == themeStyles["darkEditorBackground"] || themeStyles["lightEditorBorderColor"] == themeStyles["darkEditorBorderColor"] || themeStyles["lightTextareaCaretColor"] == themeStyles["darkTextareaCaretColor"] {
		t.Fatalf("request config body editor should react to dark mode, got %#v", themeStyles)
	}
	bodyTokenCount := numericValue(themeStyles["bodyTokenCount"])
	if themeStyles["bodyHighlighted"] != true || bodyTokenCount == 0 {
		t.Fatalf("request config body should be syntax highlighted, got %#v", themeStyles)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('.manja-endpoint-examples-rail [data-manja-request-sample] .codeblock')?.textContent.includes("HOSTNAME/api/v3/admin/hooks?page=1")`, nil); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`.manja-endpoint-examples-rail [name="server.hostname"]`).Fill("github.example.test"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`.manja-endpoint-examples-rail [name="parameters.page"]`).Fill("2"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`.manja-endpoint-examples-rail [data-manja-request-body-input]`).Fill("{\n  \"name\": \"changed\"\n}"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => {
		const text = document.querySelector('.manja-endpoint-examples-rail [data-manja-request-sample] .codeblock')?.textContent || '';
		return text.includes("github.example.test/api/v3/admin/hooks?page=2") &&
			text.includes("accept: application/vnd.github.superpro-preview+json") &&
			text.includes("content-type: application/json") &&
			text.includes('"name": "changed"');
	}`, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRequestComposerAccordionContentStaysInsideRail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	const operationAnchor = "operation-list-global-webhooks"
	idx := core.SpecIndex{
		Title: "GitHub REST",
		Overview: core.SpecOverview{
			Servers: []core.SpecServer{{
				URL: "{protocol}://{hostname}/api/v3",
				Variables: []core.SpecServerVariable{{
					Name:        "hostname",
					Description: "Self-hosted Enterprise Server or Enterprise Cloud hostname",
					Default:     "HOSTNAME",
				}, {
					Name:        "protocol",
					Description: "Self-hosted Enterprise Server or Enterprise Cloud protocol",
					Default:     "http",
				}},
			}},
		},
		Operations: []core.Operation{{
			ID:      "listGlobalWebhooks",
			Anchor:  operationAnchor,
			Method:  "GET",
			Path:    "/admin/hooks",
			Summary: "List global webhooks",
			Tags:    []string{"enterprise-admin"},
			Parameters: []core.OperationParameter{{
				Name:        "accept",
				In:          "header",
				Required:    true,
				Description: "This API is under preview and subject to change.",
				Schema:      core.SchemaSummary{Type: "string", Default: "application/vnd.github.superpro-preview+json"},
			}, {
				Name:        "per_page",
				In:          "query",
				Description: "Results per page (max 100)",
				Schema:      core.SchemaSummary{Type: "integer", Default: "30"},
			}, {
				Name:        "page",
				In:          "query",
				Description: "Page number of the results to fetch.",
				Schema:      core.SchemaSummary{Type: "integer", Default: "1"},
			}},
			Snippets: []core.RequestSnippet{{
				Label:    "cURL",
				Language: "shell",
				Code:     "curl --request GET --url {protocol}://{hostname}/api/v3/admin/hooks",
			}},
		}},
		Search: []core.SearchDocument{{
			ID:      operationAnchor,
			Title:   "GET /admin/hooks",
			Href:    "#" + operationAnchor,
			Kind:    "Operation",
			Section: "enterprise-admin",
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
		Viewport: &playwright.Size{Width: 1440, Height: 900},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/?selected=" + operationAnchor + "#" + operationAnchor); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-config-root]')?.getAttribute('data-manja-request-config-enhanced') === 'true'`, nil); err != nil {
		t.Fatalf("request configuration enhancement: %v", err)
	}
	if err := page.Locator(".manja-request-config-trigger-bar").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(".manja-endpoint-examples-rail [data-manja-request-config-panel]").WaitFor(); err != nil {
		t.Fatal(err)
	}

	result, err := page.Evaluate(`() => {
		const composer = document.querySelector('[data-manja-request-composer]');
		if (!composer) return { missing: true };
		const bounds = composer.getBoundingClientRect();
		const offenders = [];
		for (const el of composer.querySelectorAll('*')) {
			const rect = el.getBoundingClientRect();
			if (!rect.width || !rect.height) continue;
			if (rect.left < bounds.left - 1 || rect.right > bounds.right + 1) {
				offenders.push({
					tag: el.tagName.toLowerCase(),
					id: el.id || '',
					name: el.getAttribute('name') || '',
					role: el.getAttribute('role') || '',
					className: String(el.className || ''),
					left: Math.round(rect.left),
					right: Math.round(rect.right),
					boundsLeft: Math.round(bounds.left),
					boundsRight: Math.round(bounds.right),
				});
			}
		}
		return {
			composerWidth: Math.round(bounds.width),
			composerScrollWidth: composer.scrollWidth,
			offenders,
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("request composer overflow metrics should be a map, got %#v", result)
	}
	if metrics["missing"] == true {
		t.Fatalf("request composer missing from page")
	}
	if offenders, _ := metrics["offenders"].([]any); len(offenders) > 0 {
		t.Fatalf("request composer descendants should stay inside the examples rail, got metrics %#v", metrics)
	}
}

func TestRequestComposerRemainsReachableWithoutJavaScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewPublicServer(requestComposerFallbackIndex()))

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

	for _, width := range []int{390, 1440} {
		t.Run(fmt.Sprintf("%dpx", width), func(t *testing.T) {
			page, err := browser.NewPage(playwright.BrowserNewPageOptions{
				JavaScriptEnabled: playwright.Bool(false),
				Viewport:          &playwright.Size{Width: width, Height: 900},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer page.Close()

			if _, err := page.Goto(server + "/?selected=" + requestComposerFallbackOperationAnchor + "#" + requestComposerFallbackOperationAnchor); err != nil {
				t.Fatal(err)
			}
			assertStaticRequestComposerFallback(t, page, width, "without JavaScript")
		})
	}
}

func TestRequestComposerRemainsReachableWhenAlpineFailsToLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewPublicServer(requestComposerFallbackIndex()))

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

	for _, width := range []int{390, 1440} {
		t.Run(fmt.Sprintf("%dpx", width), func(t *testing.T) {
			page, err := browser.NewPage(playwright.BrowserNewPageOptions{
				Viewport: &playwright.Size{Width: width, Height: 900},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer page.Close()

			for _, url := range []string{
				"https://unpkg.com/alpinejs@3.14.9/dist/cdn.min.js",
				server + "/assets/js/runtime/alpinejs/3.14.9/alpine.min.js",
			} {
				if err := page.Route(url, func(route playwright.Route) {
					if err := route.Fulfill(playwright.RouteFulfillOptions{
						Status:      playwright.Int(http.StatusServiceUnavailable),
						Body:        "simulated Alpine outage",
						ContentType: playwright.String("text/plain"),
					}); err != nil {
						t.Errorf("fail Alpine asset %s: %v", route.Request().URL(), err)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := page.Goto(server+"/?selected="+requestComposerFallbackOperationAnchor+"#"+requestComposerFallbackOperationAnchor, playwright.PageGotoOptions{
				WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-composer]')?.dataset.manjaRequestComposerHydrated === 'true'`, nil); err != nil {
				t.Fatalf("%dpx request composer should hydrate while Alpine is unavailable: %v", width, err)
			}
			if alpine, err := page.Evaluate(`() => typeof window.Alpine`); err != nil {
				t.Fatal(err)
			} else if alpine != "undefined" {
				t.Fatalf("%dpx Alpine should remain unavailable, got %v", width, alpine)
			}

			assertStaticRequestComposerFallback(t, page, width, "without Alpine")
		})
	}
}

const requestComposerFallbackOperationAnchor = "operation-create-widget"

func requestComposerFallbackIndex() core.SpecIndex {
	return core.SpecIndex{
		Title: "Widget API",
		Overview: core.SpecOverview{
			Servers: []core.SpecServer{{URL: "https://api.example.test"}},
		},
		Operations: []core.Operation{{
			ID:      "createWidget",
			Anchor:  requestComposerFallbackOperationAnchor,
			Method:  "POST",
			Path:    "/widgets/{widgetID}",
			Summary: "Create a widget",
			Parameters: []core.OperationParameter{{
				Name:     "widgetID",
				In:       "path",
				Required: true,
				Schema:   core.SchemaSummary{Type: "string"},
			}},
			Snippets: []core.RequestSnippet{{
				Label:    "cURL",
				Language: "shell",
				Code:     "curl --request POST --url https://api.example.test/widgets/example",
			}},
		}},
		Search: []core.SearchDocument{{
			ID:      requestComposerFallbackOperationAnchor,
			Title:   "POST /widgets/{widgetID}",
			Href:    "#" + requestComposerFallbackOperationAnchor,
			Kind:    "Operation",
			Section: "widgets",
		}},
	}
}

func assertStaticRequestComposerFallback(t *testing.T, page playwright.Page, width int, reason string) {
	t.Helper()
	requestConfigRoot := page.Locator("[data-manja-request-config-root]")
	if enhanced, err := requestConfigRoot.GetAttribute("data-manja-request-config-enhanced"); err != nil {
		t.Fatal(err)
	} else if enhanced != "" {
		t.Fatalf("%dpx request configuration should not be marked enhanced %s, got %q", width, reason, enhanced)
	}
	composer := page.Locator("[data-manja-request-composer]")
	if got, err := composer.Count(); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("%dpx should render one server-side request composer %s, got %d", width, reason, got)
	}
	if visible, err := composer.IsVisible(); err != nil {
		t.Fatal(err)
	} else if !visible {
		t.Fatalf("%dpx server-side request composer should remain visible %s", width, reason)
	}
	if err := composer.ScrollIntoViewIfNeeded(); err != nil {
		t.Fatalf("%dpx server-side request composer should remain viewport reachable %s: %v", width, reason, err)
	}
	if bounds, err := composer.BoundingBox(); err != nil {
		t.Fatal(err)
	} else if bounds == nil || bounds.Width <= 0 || bounds.Height <= 0 {
		t.Fatalf("%dpx server-side request composer should have visible bounds %s, got %#v", width, reason, bounds)
	}
	if visible, err := composer.Locator(`[data-manja-request-sample]`).IsVisible(); err != nil {
		t.Fatal(err)
	} else if !visible {
		t.Fatalf("%dpx server-rendered request sample should remain visible %s", width, reason)
	}
	for _, selector := range []string{
		"[data-manja-request-config-trigger]",
		".manja-request-config-desktop-toggle",
		".manja-request-config-mobile-close",
	} {
		if visible, err := page.Locator(selector).IsVisible(); err != nil {
			t.Fatal(err)
		} else if visible {
			t.Fatalf("%dpx non-functional request configuration control %q should stay hidden %s", width, selector, reason)
		}
	}
}

func TestRichOperationDetailsKeepHorizontalOverflowLocal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	const operationAnchor = "operation-create-namespaced-deployment"
	idx := core.SpecIndex{
		Title: "Kubernetes",
		Operations: []core.Operation{{
			ID:      "createNamespacedDeployment",
			Anchor:  operationAnchor,
			Method:  "POST",
			Path:    "/apis/apps/v1/namespaces/{namespace}/deployments",
			Summary: "Create a namespaced Deployment",
			Tags:    []string{"apps_v1"},
			Parameters: []core.OperationParameter{{
				Name:        "namespace",
				In:          "path",
				Required:    true,
				Description: "Object name and auth scope, such as for teams and projects.",
				Schema:      core.SchemaSummary{Type: "string"},
			}, {
				Name:        "fieldManager",
				In:          "query",
				Description: "Name associated with the actor or entity making these changes.",
				Schema:      core.SchemaSummary{Type: "io.k8s.apimachinery.pkg.apis.meta.v1.FieldManagerIdentifier"},
			}, {
				Name:        "pretty",
				In:          "query",
				Description: strings.Repeat("If true, the output is pretty printed and the server may include additional context in the response. ", 4),
				Schema:      core.SchemaSummary{Type: "string"},
			}},
			RequestBody: &core.OperationRequestBody{
				Required: true,
				MediaTypes: []core.OperationMediaType{{
					ContentType:     "application/json",
					Schema:          core.SchemaSummary{Type: "object", JSON: `{"type":"object","properties":{"apiVersion":{"type":"string"},"kind":{"type":"string"},"metadata":{"type":"object"},"spec":{"type":"object"}}}`},
					Example:         "{\n  \"apiVersion\": \"apps/v1\",\n  \"kind\": \"Deployment\"\n}",
					ExampleProvided: true,
				}},
			},
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "OK",
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema:      core.SchemaSummary{Type: "object", JSON: `{"type":"object","properties":{"apiVersion":{"type":"string"},"kind":{"type":"string"},"metadata":{"type":"object"}}}`},
				}},
			}},
			Snippets: []core.RequestSnippet{{
				Label:    "cURL",
				Language: "shell",
				Code:     "curl --request POST --url https://kubernetes.example.test/apis/apps/v1/namespaces/default/deployments",
			}},
		}},
		Search: []core.SearchDocument{{
			ID:      operationAnchor,
			Title:   "POST /apis/apps/v1/namespaces/{namespace}/deployments",
			Href:    "#" + operationAnchor,
			Kind:    "Operation",
			Section: "apps_v1",
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
	if _, err := page.Goto(server + "/?selected=" + operationAnchor + "#" + operationAnchor); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`() => {
		const description = document.querySelector('[data-public-page-header="true"] .manja-doc-title + p');
		description.style.fontFamily = 'monospace';
		description.style.fontSize = '18px';
	}`); err != nil {
		t.Fatal(err)
	}
	parameterListID := operationAnchor + "-query-parameters"
	if err := page.Locator("#" + parameterListID).WaitFor(); err != nil {
		t.Fatal(err)
	}
	parameterPresentation, err := page.Evaluate(`(operationAnchor) => {
		const read = (location) => {
			const group = document.getElementById(operationAnchor + '-' + location + '-parameters');
			const rows = [...(group?.querySelectorAll('[data-manja-parameter-row]') || [])];
			return {
				headers: [...(group?.querySelectorAll('thead th') || [])].map((cell) => cell.textContent.trim()).join('|'),
				required: rows.map((row) => Boolean(row.querySelector('[data-required="true"]'))).join('|'),
				tables: group?.querySelectorAll('table').length || 0,
			};
		};
		return { path: read('path'), query: read('query') };
	}`, operationAnchor)
	if err != nil {
		t.Fatal(err)
	}
	presentation, ok := parameterPresentation.(map[string]any)
	if !ok {
		t.Fatalf("parameter presentation should be a map, got %#v", parameterPresentation)
	}
	for location, wantRequired := range map[string]string{"path": "true", "query": "false|false"} {
		group, ok := presentation[location].(map[string]any)
		if !ok {
			t.Fatalf("%s parameter presentation should be a map, got %#v", location, presentation[location])
		}
		if got, want := group["headers"], ""; got != want {
			t.Fatalf("%s parameter headers = %q, want no table headers", location, got)
		}
		if got := group["required"]; got != wantRequired {
			t.Fatalf("%s required values = %q, want %q", location, got, wantRequired)
		}
		if got := metricNumber(t, group, "tables"); got != 0 {
			t.Fatalf("%s parameter tables = %v, want stacked rows", location, got)
		}
	}
	if err := page.SetViewportSize(1280, 900); err != nil {
		t.Fatal(err)
	}
	desktopVisibility, err := page.Evaluate(`() => ({
		enhanced: document.querySelector('[data-manja-request-config-root]')?.getAttribute('data-manja-request-config-enhanced'),
		rail: getComputedStyle(document.querySelector('.manja-endpoint-examples-rail')).display,
		trigger: getComputedStyle(document.querySelector('.manja-request-config-trigger-bar')).display,
		open: document.querySelector('[data-manja-request-config-sheet]')?.getAttribute('data-open'),
		ariaHidden: document.querySelector('[data-manja-request-config-sheet]')?.getAttribute('aria-hidden'),
		content: getComputedStyle(document.querySelector('.manja-endpoint-examples-rail-content')).display,
		contentAriaHidden: document.querySelector('.manja-endpoint-examples-rail-content')?.getAttribute('aria-hidden'),
		toggle: getComputedStyle(document.querySelector('.manja-request-config-desktop-toggle')).display,
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := desktopVisibility.(map[string]any); !ok || got["enhanced"] != "true" || got["rail"] != "none" || got["trigger"] != "flex" || got["open"] != "false" || got["ariaHidden"] != nil || got["content"] != "grid" || got["contentAriaHidden"] != "true" || got["toggle"] == "none" {
		t.Fatalf("desktop request configuration should start collapsed behind its trigger, got %#v", desktopVisibility)
	}
	desktopToggle := page.Locator(".manja-request-config-desktop-toggle")
	if err := page.Locator(".manja-request-config-trigger-bar").Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-config-sheet]')?.getAttribute('data-open') === 'true'`, nil); err != nil {
		t.Fatalf("desktop request configuration should open from its header toggle: %v", err)
	}
	desktopOpenState, err := page.Evaluate(`() => {
		const sheet = document.querySelector('[data-manja-request-config-sheet]');
		const content = document.querySelector('.manja-endpoint-examples-rail-content');
		const footer = document.querySelector('footer');
		const sheetBottom = sheet ? sheet.getBoundingClientRect().bottom + window.scrollY : 0;
		const footerTop = footer ? footer.getBoundingClientRect().top + window.scrollY : sheetBottom;
		return {
			open: sheet?.getAttribute('data-open'),
			content: getComputedStyle(content).display,
			expanded: document.querySelector('.manja-request-config-desktop-toggle')?.getAttribute('aria-expanded'),
			footerGap: footerTop - sheetBottom,
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if state, ok := desktopOpenState.(map[string]any); !ok || state["open"] != "true" || state["content"] == "none" || state["expanded"] != "true" || metricNumber(t, state, "footerGap") < 16 {
		t.Fatalf("desktop request configuration open state should preserve footer breathing room, got %#v", desktopOpenState)
	}
	if err := desktopToggle.Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-config-sheet]')?.getAttribute('data-open') === 'false'`, nil); err != nil {
		t.Fatalf("desktop request configuration should collapse from its header toggle: %v", err)
	}

	for _, width := range []int{320, 390, 414, 768} {
		t.Run(fmt.Sprintf("%dpx", width), func(t *testing.T) {
			if err := page.SetViewportSize(width, 900); err != nil {
				t.Fatal(err)
			}
			trigger := page.Locator("[data-manja-request-config-trigger]")
			if err := trigger.WaitFor(); err != nil {
				t.Fatal(err)
			}
			if hidden, err := trigger.IsHidden(); err != nil || hidden {
				if err != nil {
					t.Fatal(err)
				}
				t.Fatalf("%dpx request configuration trigger should be visible", width)
			}
			result, err := page.Evaluate(`(parameterListID) => {
				const main = document.querySelector('#main-content');
			const list = document.getElementById(parameterListID);
			const rows = [...(list?.querySelectorAll('[data-manja-parameter-row]') || [])];
			const maxRowClientWidth = rows.reduce((width, row) => Math.max(width, row.clientWidth || 0), 0);
			const maxRowScrollWidth = rows.reduce((width, row) => Math.max(width, row.scrollWidth || 0), 0);
			const headerDescription = document.querySelector('[data-catalog-detail="operation"] [data-public-page-header] .manja-doc-title + p');
			return {
				mainClientWidth: main?.clientWidth || 0,
				mainScrollWidth: main?.scrollWidth || 0,
				headerDescriptionClientWidth: headerDescription?.clientWidth || 0,
				headerDescriptionScrollWidth: headerDescription?.scrollWidth || 0,
				listClientWidth: list?.clientWidth || 0,
				listScrollWidth: list?.scrollWidth || 0,
				parameterRowCount: rows.length,
				maxRowClientWidth,
				maxRowScrollWidth,
				tables: list?.querySelectorAll('table').length || 0,
				hasRequestBody: Boolean(document.querySelector('[aria-label="Request body"]')),
				hasRequestSample: Boolean(document.querySelector('[data-manja-request-sample]')),
			};
		}`, parameterListID)
			if err != nil {
				t.Fatal(err)
			}
			metrics, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("%dpx operation layout metrics should be a map, got %#v", width, result)
			}
			t.Logf("%dpx rich operation layout: main=%v/%v list=%v/%v rows=%v", width, metrics["mainScrollWidth"], metrics["mainClientWidth"], metrics["listScrollWidth"], metrics["listClientWidth"], metrics["parameterRowCount"])
			if metrics["hasRequestBody"] != true || metrics["hasRequestSample"] != true {
				t.Fatalf("%dpx operation must preserve rich request content, got %#v", width, metrics)
			}
			if got, want := metricNumber(t, metrics, "mainScrollWidth"), metricNumber(t, metrics, "mainClientWidth"); got != want {
				t.Fatalf("%dpx main must not own horizontal overflow: scrollWidth=%v clientWidth=%v, metrics %#v", width, got, want, metrics)
			}
			if got, want := metricNumber(t, metrics, "headerDescriptionScrollWidth"), metricNumber(t, metrics, "headerDescriptionClientWidth"); got != want {
				t.Fatalf("%dpx catalog operation header must wrap its route description: scrollWidth=%v clientWidth=%v, metrics %#v", width, got, want, metrics)
			}
			if got, max := metricNumber(t, metrics, "listClientWidth"), metricNumber(t, metrics, "mainClientWidth"); got > max {
				t.Fatalf("%dpx parameter list must stay inside main, metrics %#v", width, metrics)
			}
			if got, want := metricNumber(t, metrics, "listScrollWidth"), metricNumber(t, metrics, "listClientWidth"); got != want {
				t.Fatalf("%dpx parameter list should not own horizontal overflow, metrics %#v", width, metrics)
			}
			if got, want := metricNumber(t, metrics, "maxRowScrollWidth"), metricNumber(t, metrics, "maxRowClientWidth"); got != want {
				t.Fatalf("%dpx parameter rows should wrap without horizontal overflow, metrics %#v", width, metrics)
			}
			if got := metricNumber(t, metrics, "parameterRowCount"); got != 2 {
				t.Fatalf("%dpx query parameter list should expose two stacked rows, metrics %#v", width, metrics)
			}
			if got := metricNumber(t, metrics, "tables"); got != 0 {
				t.Fatalf("%dpx parameter list should not render a table, metrics %#v", width, metrics)
			}

			if width == 390 {
				descriptionToggle := page.Locator("#" + parameterListID + " .manja-description-toggle").First()
				if err := descriptionToggle.WaitFor(); err != nil {
					t.Fatal(err)
				}
				if got, err := descriptionToggle.GetAttribute("aria-expanded"); err != nil || got != "false" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("long parameter description should start collapsed, aria-expanded=%q", got)
				}
				if got, err := descriptionToggle.TextContent(); err != nil || strings.TrimSpace(got) != "Show more" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("collapsed parameter description label = %q, want Show more", got)
				}
				if err := descriptionToggle.Click(); err != nil {
					t.Fatal(err)
				}
				if got, err := descriptionToggle.GetAttribute("aria-expanded"); err != nil || got != "true" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("long parameter description should expand, aria-expanded=%q", got)
				}
				if got, err := descriptionToggle.TextContent(); err != nil || strings.TrimSpace(got) != "Show less" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("expanded parameter description label = %q, want Show less", got)
				}
				if err := descriptionToggle.Click(); err != nil {
					t.Fatal(err)
				}
				if got, err := descriptionToggle.GetAttribute("aria-expanded"); err != nil || got != "false" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("long parameter description should collapse again, aria-expanded=%q", got)
				}
				if err := page.Locator("#darkModeToggleBtn").Focus(); err != nil {
					t.Fatal(err)
				}
				if err := page.Keyboard().Press("Escape"); err != nil {
					t.Fatal(err)
				}
				closedEscapeFocus, err := page.Evaluate(`() => document.activeElement?.id`)
				if err != nil {
					t.Fatal(err)
				}
				if closedEscapeFocus != "darkModeToggleBtn" {
					t.Fatalf("Escape with a closed request configuration should preserve focus, got %v", closedEscapeFocus)
				}
				if err := trigger.Click(); err != nil {
					t.Fatal(err)
				}
				drawerID := operationAnchor + "-request-config"
				drawer := page.Locator(`[data-manja-request-config-sheet][role="dialog"][aria-labelledby="` + drawerID + `-title"]`)
				if err := drawer.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
					t.Fatal(err)
				}
				if _, err := page.WaitForFunction(`() => {
					const sheet = document.querySelector('[data-manja-request-config-sheet]');
					return Boolean(sheet && sheet.contains(document.activeElement));
				}`, nil); err != nil {
					t.Fatalf("mobile request configuration should move focus into the sheet: %v", err)
				}
				if got, err := drawer.Locator(`[data-manja-request-composer]`).Count(); err != nil {
					t.Fatal(err)
				} else if got == 0 {
					t.Fatalf("mobile request drawer should contain request configuration")
				}
				mobileState, err := page.Evaluate(`() => {
					const ids = [...document.querySelectorAll('[id]')].map((element) => element.id);
					const duplicates = ids.filter((id, index) => ids.indexOf(id) !== index);
					const sheet = document.querySelector('[data-manja-request-config-sheet]');
					const triggerButton = document.querySelector('[data-manja-request-config-trigger]');
					const active = document.activeElement;
					return {
						duplicateIDs: [...new Set(duplicates)],
						triggerExpanded: triggerButton?.getAttribute('aria-expanded'),
						triggerPosition: getComputedStyle(document.querySelector('[data-manja-request-config-trigger]')).position,
						sheetPosition: getComputedStyle(sheet).position,
						activeInsideSheet: Boolean(active && sheet?.contains(active)),
						composerCount: document.querySelectorAll('[data-manja-request-composer]').length,
						fieldDescriptions: [...(sheet?.querySelectorAll('[data-manja-request-config-fields] [data-field-path]') || [])].map((field) => {
							const control = field.querySelector('input[id], select[id], textarea[id]');
							const helper = field.querySelector('p.text-xs:not([data-manja-request-body-status])');
							return { control: control?.id || '', describedBy: control?.getAttribute('aria-describedby') || '', helper: helper?.id || '' };
						}),
					};
				}`)
				if err != nil {
					t.Fatal(err)
				}
				state, ok := mobileState.(map[string]any)
				if !ok {
					t.Fatalf("mobile request configuration state should be a map, got %#v", mobileState)
				}
				if duplicates, ok := state["duplicateIDs"].([]any); !ok || len(duplicates) != 0 {
					t.Fatalf("mobile request configuration must not duplicate ids, got %#v", state["duplicateIDs"])
				}
				if state["triggerExpanded"] != "true" || state["triggerPosition"] != "fixed" || state["sheetPosition"] != "fixed" || state["activeInsideSheet"] != true || numericValue(state["composerCount"]) != 1 {
					t.Fatalf("mobile request configuration semantics = %#v", state)
				}
				fields, fieldsOK := state["fieldDescriptions"].([]any)
				if !fieldsOK || len(fields) == 0 {
					t.Fatalf("request configuration should expose field helper descriptions, state=%#v", state)
				}
				for _, rawField := range fields {
					field, ok := rawField.(map[string]any)
					control, controlOK := field["control"].(string)
					describedBy, describedByOK := field["describedBy"].(string)
					helper, helperOK := field["helper"].(string)
					if !ok || !controlOK || !describedByOK || !helperOK || control == "" || helper == "" || !strings.Contains(describedBy, helper) {
						t.Fatalf("request configuration field helper association = %#v", rawField)
					}
				}
				configDescriptionToggle := drawer.Locator(`[data-manja-request-config-fields] [data-manja-description-toggle]`).First()
				if err := configDescriptionToggle.WaitFor(); err != nil {
					t.Fatal(err)
				}
				if got, err := configDescriptionToggle.GetAttribute("aria-expanded"); err != nil || got != "false" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("long request configuration description should start collapsed, aria-expanded=%q", got)
				}
				if got, err := configDescriptionToggle.TextContent(); err != nil || strings.TrimSpace(got) != "Show more" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("collapsed request configuration label = %q, want Show more", got)
				}
				if err := configDescriptionToggle.Click(); err != nil {
					t.Fatal(err)
				}
				if got, err := configDescriptionToggle.GetAttribute("aria-expanded"); err != nil || got != "true" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("long request configuration description should expand, aria-expanded=%q", got)
				}
				if got, err := configDescriptionToggle.TextContent(); err != nil || strings.TrimSpace(got) != "Show less" {
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("expanded request configuration label = %q, want Show less", got)
				}
				darkVisuals, err := page.Evaluate(`() => {
					const root = document.documentElement;
					const sheet = document.querySelector('[data-manja-request-config-sheet]');
					const triggerBar = document.querySelector('[data-manja-request-config-trigger]');
					const wasDark = root.classList.contains('dark');
					root.classList.remove('dark');
					const light = {
						sheet: getComputedStyle(sheet).backgroundColor,
						bar: getComputedStyle(triggerBar).backgroundColor,
					};
					root.classList.add('dark');
					const dark = {
						sheet: getComputedStyle(sheet).backgroundColor,
						bar: getComputedStyle(triggerBar).backgroundColor,
					};
					root.classList.toggle('dark', wasDark);
					return { light, dark };
				}`)
				if err != nil {
					t.Fatal(err)
				}
				visuals, ok := darkVisuals.(map[string]any)
				if !ok {
					t.Fatalf("mobile request configuration theme styles should be a map, got %#v", darkVisuals)
				}
				light, lightOK := visuals["light"].(map[string]any)
				dark, darkOK := visuals["dark"].(map[string]any)
				if !lightOK || !darkOK || light["sheet"] == dark["sheet"] || light["bar"] == dark["bar"] {
					t.Fatalf("mobile request configuration should switch sheet and bar surfaces with dark mode, got %#v", visuals)
				}
				if err := page.Keyboard().Press("Escape"); err != nil {
					t.Fatal(err)
				}
				if err := drawer.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
					t.Fatal(err)
				}
				focusedTrigger, err := page.Evaluate(`() => document.activeElement === document.querySelector('[data-manja-request-config-trigger]')`)
				if err != nil {
					t.Fatal(err)
				}
				if focusedTrigger != true {
					t.Fatalf("closing request configuration with Escape should restore focus to its trigger")
				}
				if err := trigger.Click(); err != nil {
					t.Fatal(err)
				}
				if err := drawer.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
					t.Fatal(err)
				}
				if err := page.Mouse().Click(2, 50); err != nil {
					t.Fatal(err)
				}
				if err := drawer.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
					t.Fatal(err)
				}
				focusedTrigger, err = page.Evaluate(`() => document.activeElement === document.querySelector('[data-manja-request-config-trigger]')`)
				if err != nil {
					t.Fatal(err)
				}
				if focusedTrigger != true {
					t.Fatalf("clicking the request configuration backdrop should restore focus to its trigger")
				}
				if err := trigger.Click(); err != nil {
					t.Fatal(err)
				}
				if err := drawer.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
					t.Fatal(err)
				}
				if err := drawer.Locator(`[aria-label="Close request configuration"]`).Click(); err != nil {
					t.Fatal(err)
				}
				if err := drawer.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
					t.Fatal(err)
				}
				focusedTrigger, err = page.Evaluate(`() => document.activeElement === document.querySelector('[data-manja-request-config-trigger]')`)
				if err != nil {
					t.Fatal(err)
				}
				if focusedTrigger != true {
					t.Fatalf("clicking the request configuration close button should restore focus to its trigger")
				}
			}
		})
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
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	if err := page.Locator("#manja-theme-trigger").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#manja-theme-listbox[role='listbox']").WaitFor(); err != nil {
		t.Fatal(err)
	}

	result, err := page.Evaluate(`() => {
		const trigger = document.getElementById('manja-theme-trigger');
		const menu = document.getElementById('manja-theme-listbox');
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
	if err := page.Locator(`#manja-theme-listbox [role='option']:has-text("Goshtoso")`).Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.theme === 'goshtoso' && localStorage.getItem('theme') === 'goshtoso'`, nil); err != nil {
		t.Fatalf("theme selection should update the live document and persistence: %v", err)
	}
}

func TestPublicDocsAraiHuDefaultRetainsThemeAndModeSwitching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewPublicServer(core.SpecIndex{Title: "Petstore", Version: "1.0.0"}))
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

	var consoleErrors []string
	page.On("console", func(message playwright.ConsoleMessage) {
		if message.Type() == "error" {
			consoleErrors = append(consoleErrors, message.Text())
		}
	})
	if _, err := page.Goto(server); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.theme === 'araihu' && !document.documentElement.classList.contains('dark')`, nil); err != nil {
		t.Fatalf("Arai Hu should be default light mode: %v", err)
	}
	if err := page.Locator("#darkModeToggleBtn").Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.theme === 'araihu' && document.documentElement.classList.contains('dark') && localStorage.getItem('darkMode') === 'true'`, nil); err != nil {
		t.Fatalf("dark mode should preserve Arai Hu theme and persist: %v", err)
	}
	if err := page.Locator("#manja-theme-trigger").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`#manja-theme-listbox [role='option']:has-text("Goshtoso")`).Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.theme === 'goshtoso' && document.documentElement.classList.contains('dark') && localStorage.getItem('theme') === 'goshtoso'`, nil); err != nil {
		t.Fatalf("theme selection should preserve dark mode and persist: %v", err)
	}
	if err := page.Locator("#manja-theme-trigger").Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Locator(`#manja-theme-listbox [role='option']:has-text("Arai Hû")`).Evaluate(`option => option.click()`, nil); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#darkModeToggleBtn").Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.theme === 'araihu' && !document.documentElement.classList.contains('dark') && localStorage.getItem('theme') === 'araihu' && localStorage.getItem('darkMode') === 'false'`, nil); err != nil {
		t.Fatalf("Arai Hu and light mode should remain independently switchable: %v", err)
	}
	if len(consoleErrors) != 0 {
		t.Fatalf("unexpected browser console errors: %v", consoleErrors)
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
	if _, err := page.Goto(server + "/?selected=operation-filler-0#operation-filler-0"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	if err := page.Locator("#operation-filler-0:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate("() => { window.__manjaReloadSentinel = 'kept'; }"); err != nil {
		t.Fatal(err)
	}
	openSidebarTagGroup(t, page, "tag-pets-children")
	if _, err := page.Evaluate("() => { document.getElementById('sidebar-nav-content').dataset.sidebarSentinel = 'kept'; }"); err != nil {
		t.Fatal(err)
	}
	scrolled, err := page.Evaluate(`() => {
		const sidebar = document.querySelector('aside[aria-label="API sections"] .sidebar-scroll');
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
	if _, err := page.WaitForFunction(`() => document.activeElement?.dataset.publicDocIdentity === 'operation-target'`, nil); err != nil {
		t.Fatalf("selected operation focus sentinel: %v", err)
	}
	kept, err := page.Evaluate("() => window.__manjaReloadSentinel === 'kept'")
	if err != nil {
		t.Fatal(err)
	}
	if kept != true {
		t.Fatalf("sidebar navigation performed a full page reload instead of preserving page state")
	}
	scrollPreserved, err := page.Evaluate(`() => {
		const sidebar = document.querySelector('aside[aria-label="API sections"] .sidebar-scroll');
		return !!sidebar && sidebar.scrollTop > 0;
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if scrollPreserved != true {
		t.Fatalf("sidebar navigation reset the sidebar scroll position")
	}
	sidebarUntouched, err := page.Evaluate(`() => document.getElementById('sidebar-nav-content')?.dataset.sidebarSentinel === 'kept'`)
	if err != nil {
		t.Fatal(err)
	}
	if sidebarUntouched != true {
		t.Fatalf("sidebar navigation replaced the sidebar instead of swapping only main content")
	}
	groupStillOpen, err := page.Evaluate(`() => document.querySelector('#tag-pets-children a')?.offsetParent !== null`)
	if err != nil {
		t.Fatal(err)
	}
	if groupStillOpen != true {
		t.Fatalf("sidebar navigation collapsed the open sidebar tag group")
	}
	targetActive, err := page.Evaluate(`() => document.querySelector('aside a[href="/?selected=operation-target#operation-target"] .sr-only')?.textContent.trim() === 'active'`)
	if err != nil {
		t.Fatal(err)
	}
	if targetActive != true {
		t.Fatalf("sidebar navigation did not move the active marker to the selected operation")
	}
	initialActive, err := page.Evaluate(`() => document.querySelector('aside a[href="/?selected=operation-filler-0#operation-filler-0"] .sr-only')?.textContent.trim() === 'active'`)
	if err != nil {
		t.Fatal(err)
	}
	if initialActive == true {
		t.Fatalf("sidebar navigation left the previous operation active marker in place")
	}
	if got := page.URL(); got != server+"/?selected=operation-target#operation-target" {
		t.Fatalf("page URL = %q, want %q", got, server+"/?selected=operation-target#operation-target")
	}
	identity, err := page.Evaluate(`() => ({
		selected: document.querySelector('main')?.dataset.selectedDoc,
		content: document.querySelector('[data-public-docs-content]')?.dataset.selectedDoc,
		focus: document.activeElement?.dataset.publicDocIdentity,
		current: Array.from(document.querySelectorAll('aside[aria-label="API sections"] [aria-current="page"]')).map(link => link.getAttribute('href')),
		title: document.title
	})`)
	if err != nil {
		t.Fatal(err)
	}
	wantIdentity := map[string]any{
		"selected": "operation-target",
		"content":  "operation-target",
		"focus":    "operation-target",
		"current":  []any{"/?selected=operation-target#operation-target"},
		"title":    "Target operation · Manja",
	}
	if fmt.Sprint(identity) != fmt.Sprint(wantIdentity) {
		t.Fatalf("HTMX selected identity = %#v, want %#v", identity, wantIdentity)
	}

	if _, err := page.GoBack(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-filler-0:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	backIdentity, err := page.Evaluate(`() => document.querySelector('main')?.dataset.selectedDoc + '|' + document.querySelector('[data-public-docs-content]')?.dataset.selectedDoc + '|' + document.querySelector('aside[aria-label="API sections"] [aria-current="page"]')?.getAttribute('href')`)
	if err != nil {
		t.Fatal(err)
	}
	if backIdentity != "operation-filler-0|operation-filler-0|/?selected=operation-filler-0#operation-filler-0" {
		t.Fatalf("Back selected identity = %#v", backIdentity)
	}

	if _, err := page.GoForward(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-target:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	forwardIdentity, err := page.Evaluate(`() => document.querySelector('main')?.dataset.selectedDoc + '|' + document.querySelector('[data-public-docs-content]')?.dataset.selectedDoc + '|' + document.querySelector('aside[aria-label="API sections"] [aria-current="page"]')?.getAttribute('href')`)
	if err != nil {
		t.Fatal(err)
	}
	if forwardIdentity != "operation-target|operation-target|/?selected=operation-target#operation-target" {
		t.Fatalf("Forward selected identity = %#v", forwardIdentity)
	}
}

func TestPublicDocsSidebarTagGroupsToggleIndependently(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}, Anchor: "operation-listpets"},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Tags: []string{"Pets"}, Anchor: "operation-createpet"},
			{ID: "listStores", Method: "GET", Path: "/stores", Summary: "List stores", Tags: []string{"Stores"}, Anchor: "operation-liststores"},
		},
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
	initialURL := page.URL()
	petsControl := page.Locator(`aside a[aria-controls="tag-pets-children"]`)
	count, err := petsControl.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("Pets tag disclosure count = %d, want 1", count)
	}
	storesControl := page.Locator(`aside a[aria-controls="tag-stores-children"]`)
	count, err = storesControl.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("Stores tag disclosure count = %d, want 1", count)
	}
	petsChild := page.Locator(`#tag-pets-children a`).First()
	storesChild := page.Locator(`#tag-stores-children a`).First()
	if err := petsChild.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if err := storesChild.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}

	if err := petsControl.Click(); err != nil {
		t.Fatal(err)
	}
	if err := petsChild.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}
	if err := storesChild.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if got := page.URL(); got != initialURL {
		t.Fatalf("tag disclosure should not navigate, got URL %q want %q", got, initialURL)
	}

	if err := petsControl.Click(); err != nil {
		t.Fatal(err)
	}
	if err := petsChild.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if got := page.URL(); got != initialURL {
		t.Fatalf("closing tag disclosure should not navigate, got URL %q want %q", got, initialURL)
	}
}

func TestPublicDocsSidebarOverlayOnSmallDevices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	const createAnchor = "operation-createpet"
	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}, Anchor: "operation-listpets"},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Description: "Creation body", Tags: []string{"Pets"}, Anchor: createAnchor},
		},
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
		Viewport: &playwright.Size{Width: 390, Height: 740},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server); err != nil {
		t.Fatal(err)
	}

	trigger := page.Locator(`button[aria-label="Open API sections"]`)
	if err := trigger.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}
	triggerBox, err := trigger.BoundingBox()
	if err != nil {
		t.Fatal(err)
	}
	if triggerBox == nil || triggerBox.Width < 44 || triggerBox.Height < 44 {
		t.Fatalf("mobile navigation trigger must be at least 44x44, box=%#v", triggerBox)
	}
	desktopAsideHidden, err := page.Locator(`aside[aria-label="API sections"]`).IsHidden()
	if err != nil {
		t.Fatal(err)
	}
	if !desktopAsideHidden {
		t.Fatalf("desktop sidebar should be hidden on small screens")
	}

	if err := trigger.Click(); err != nil {
		t.Fatal(err)
	}
	panel := page.Locator(`#public-docs-sidebar-panel`)
	if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => {
		const panel = document.querySelector('#public-docs-sidebar-panel');
		if (!panel) return false;
		const box = panel.getBoundingClientRect();
		return box.right > 0 && box.left < innerWidth && box.bottom > 0 && box.top < innerHeight;
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("mobile drawer did not enter the viewport: %v", err)
	}
	panelBox, err := panel.BoundingBox()
	if err != nil {
		t.Fatal(err)
	}
	if panelBox == nil || panelBox.X >= 390 || panelBox.X+panelBox.Width <= 0 || panelBox.Y >= 740 || panelBox.Y+panelBox.Height <= 0 {
		t.Fatalf("mobile drawer must positively intersect the 390px viewport, box=%#v", panelBox)
	}
	headerBox, err := page.Locator(`header[data-boot-anim="header"]`).BoundingBox()
	if err != nil {
		t.Fatal(err)
	}
	if headerBox == nil || panelBox.Y < headerBox.Y+headerBox.Height-1 {
		t.Fatalf("mobile drawer must be viewport-owned below the header, panel=%#v header=%#v", panelBox, headerBox)
	}
	focusedSkipLink, err := page.Evaluate(`() => {
		const active = document.activeElement;
		return active instanceof HTMLAnchorElement &&
			active.closest('#public-docs-sidebar-panel') !== null &&
			active.getAttribute('href') === '#main-content';
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if focusedSkipLink == true {
		t.Fatalf("mobile sidebar should open like the Goshtoso docs drawer without forcing focus to the skip link")
	}
	expanded, err := trigger.GetAttribute("aria-expanded")
	if err != nil {
		t.Fatal(err)
	}
	if expanded != "true" {
		t.Fatalf("mobile sidebar trigger aria-expanded = %q, want true", expanded)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.activeElement?.getAttribute('aria-label') === 'Open API sections'`, nil); err != nil {
		t.Fatalf("Escape should restore focus to the mobile navigation trigger: %v", err)
	}
	if err := trigger.Click(); err != nil {
		t.Fatal(err)
	}
	if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}

	tagControl := panel.Locator(`a[aria-controls="mobile-tag-pets-children"]`)
	if err := tagControl.Click(); err != nil {
		t.Fatal(err)
	}
	if err := panel.Locator(`#mobile-tag-pets-children a`).First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}

	link := panel.Locator(`a[href="/?selected=` + createAnchor + `#` + createAnchor + `"]`).Last()
	if err := link.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#" + createAnchor + ":visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := panel.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if got := page.URL(); got != server+"/?selected="+createAnchor+"#"+createAnchor {
		t.Fatalf("page URL = %q, want %q", got, server+"/?selected="+createAnchor+"#"+createAnchor)
	}
}

func TestPublicDocsScrollsMainContentInsideShell(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	const selectedAnchor = "operation-oauth-authorizations-list-grants"
	const fixturePath = "internal/adapters/openapi/testdata/github-v3-rest.json"
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := (openapiadapter.Parser{}).Parse(context.Background(), core.SpecFile{
		Path:  fixturePath,
		Bytes: data,
	}, core.Revision{ID: "test"})
	if err != nil {
		t.Fatal(err)
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
			const header = document.querySelector('.manja-docs-header');
			const main = document.querySelector('main');
			const aside = document.querySelector('aside[aria-label="API sections"]');
			const sidebar = document.querySelector('aside[aria-label="API sections"] .sidebar-scroll');
			const overflows = (scrollSize, clientSize) => scrollSize > clientSize + 1;
			const headerRect = header ? header.getBoundingClientRect() : null;
			const asideRect = aside ? aside.getBoundingClientRect() : null;

			return {
				documentScrollableX: overflows(documentScroller.scrollWidth, documentScroller.clientWidth),
				documentScrollableY: overflows(documentScroller.scrollHeight, documentScroller.clientHeight),
				bodyScrollableX: overflows(body.scrollWidth, window.innerWidth),
				bodyScrollableY: overflows(body.scrollHeight, window.innerHeight),
				mainScrollableY: main ? overflows(main.scrollHeight, main.clientHeight) : false,
				sidebarScrollableY: sidebar ? overflows(sidebar.scrollHeight, sidebar.clientHeight) : false,
				mainRectHeight: main ? Math.round(main.getBoundingClientRect().height) : 0,
				asideRectHeight: asideRect ? Math.round(asideRect.height) : 0,
				asideRectTop: asideRect ? Math.round(asideRect.top) : 0,
				headerRectBottom: headerRect ? Math.round(headerRect.bottom) : 0,
				headerRectHeight: headerRect ? Math.round(headerRect.height) : 0,
				headerDisplay: header ? getComputedStyle(header).display : '',
				headerChildHeight: header?.firstElementChild ? Math.round(header.firstElementChild.getBoundingClientRect().height) : 0,
				mainRectTop: main ? Math.round(main.getBoundingClientRect().top) : 0,
				asideParentTop: aside?.parentElement ? Math.round(aside.parentElement.getBoundingClientRect().top) : 0,
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
	for _, key := range []string{"documentScrollableX", "documentScrollableY", "bodyScrollableX", "bodyScrollableY"} {
		if metrics[key] == true {
			t.Fatalf("public docs should keep scrolling inside the docs shell; %s=true in metrics %#v", key, metrics)
		}
	}
	if metrics["mainScrollableY"] != true {
		t.Fatalf("public docs should scroll selected content inside the main pane, got metrics %#v", metrics)
	}
	if metrics["sidebarScrollableY"] != true {
		t.Fatalf("test setup should keep long navigation scrollable inside the sidebar, got metrics %#v", metrics)
	}
	wantPaneHeight := metricNumber(t, metrics, "windowInnerHeight") - metricNumber(t, metrics, "headerRectBottom")
	if got := metricNumber(t, metrics, "asideRectHeight"); got != wantPaneHeight {
		t.Fatalf("aside should keep the viewport-height navigation rail; want %v got %v, metrics %#v", wantPaneHeight, got, metrics)
	}
	if got := metricNumber(t, metrics, "mainRectHeight"); got != wantPaneHeight {
		t.Fatalf("main should match the docs shell height and scroll internally; want %v got %v, metrics %#v", wantPaneHeight, got, metrics)
	}
	if got := metricNumber(t, metrics, "asideRectTop"); got != metricNumber(t, metrics, "headerRectBottom") {
		t.Fatalf("sidebar rail should connect to the header bottom; metrics %#v", metrics)
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

func openSidebarTagGroup(t *testing.T, page playwright.Page, childrenID string) {
	t.Helper()

	open, err := page.Evaluate(`(childrenID) => document.querySelector("#" + childrenID + " a")?.offsetParent !== null`, childrenID)
	if err != nil {
		t.Fatal(err)
	}
	if open == true {
		return
	}
	if err := page.Locator(`aside a[aria-controls="` + childrenID + `"]`).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#" + childrenID + " a").First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(3000),
	}); err != nil {
		t.Fatal(err)
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

func numericValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
