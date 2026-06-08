package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	"github.com/araihu/manja/internal/core"
)

func TestPublicDocsRenderSearchAndOperations(t *testing.T) {
	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Tags: []string{"Pets"}},
		},
		Schemas: []core.Schema{{Name: "Pet", Description: "A pet"}},
		Search: []core.SearchDocument{
			{ID: "operation-listPets", Title: "GET /pets", Description: "List pets", Href: "#operation-listPets", Kind: "Operation", Section: "Pets"},
			{ID: "schema-Pet", Title: "Pet", Description: "A pet", Href: "#schema-pet", Kind: "Schema", Section: "Schemas"},
		},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Petstore", "GET", "/pets", "Search docs", "operation-listPets"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, want := range []string{
		`h-screen overflow-hidden`,
		`Manja`,
		`class="flex h-16`,
		`h-[calc(100vh-4rem)] overflow-hidden`,
		`aria-label="API sections"`,
		`aria-label="Documentation search"`,
		`href="/?selected=operation-listPets#operation-listPets"`,
		`href="/?selected=schema-pet#schema-pet"`,
		`id="main-content"`,
		`max-w-[100rem]`,
		`Operations`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs shell missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `<section id="operation-createPet"`) {
		t.Fatalf("default page should render only the selected sidebar item, got create operation content:\n%s", body)
	}
	sidebarMethodBadge := regexp.MustCompile(`<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*ml-auto shrink-0[^"]*border-info[^"]*bg-info[^"]*text-on-info[^"]*"[^>]*>GET</sup>`)
	if !sidebarMethodBadge.MatchString(body) {
		t.Fatalf("operation sidebar item should render flex path label with right-aligned Goshtoso method badge:\n%s", body)
	}
	postMethodBadge := regexp.MustCompile(`<a href="/\?selected=operation-createPet#operation-createPet"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*border-success[^"]*bg-success[^"]*text-on-success[^"]*"[^>]*>POST</sup>`)
	if !postMethodBadge.MatchString(body) {
		t.Fatalf("POST sidebar method badge should use Goshtoso success styling:\n%s", body)
	}
	pageMethodBadge := regexp.MustCompile(`<span class="[^"]*rounded-radius[^"]*w-fit[^"]*font-medium[^"]*text-\[10px\][^"]*px-1\.5[^"]*py-0\.5[^"]*border-info[^"]*bg-info[^"]*text-on-info[^"]*font-mono[^"]*font-bold[^"]*">GET</span>`)
	if !pageMethodBadge.MatchString(body) {
		t.Fatalf("operation cards should render methods with Goshtoso badge component classes:\n%s", body)
	}
	for _, reject := range []string{"bg-sky-700", "bg-sky-400", "bg-emerald-700", "bg-emerald-400", "bg-rose-700", "bg-rose-400", "text-white", "text-neutral-950"} {
		if strings.Contains(body, reject) {
			t.Fatalf("method badges should use Goshtoso badge variants, got custom class %q:\n%s", reject, body)
		}
	}
	sidebarTagGroup := regexp.MustCompile(`<div data-sidebar-section="Operations">.*<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">Pets</span>.*<div class="ml-4 flex flex-col">.*<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*>GET</sup>`)
	if !sidebarTagGroup.MatchString(body) {
		t.Fatalf("operation sidebar items should use Penguin-style tag sub-items:\n%s", body)
	}
	tagWithoutRail := regexp.MustCompile(`<a href="/\?selected=operation-listPets#operation-listPets" class="flex items-center gap-2 py-2\.5 pl-4[^"]*"><span class="min-w-0 flex-1 truncate">Pets</span>`)
	if !tagWithoutRail.MatchString(body) {
		t.Fatalf("operation tag parent should not render a leading rail:\n%s", body)
	}
	if strings.Contains(body, `<a href="#operations-heading" class="flex items-center gap-2 border-l`) {
		t.Fatalf("operation tag parent should not use endpoint rail classes:\n%s", body)
	}
	for _, reject := range []string{`x-data="{ open: true }"`, `x-on:click.prevent="open = !open"`, `x-bind:aria-expanded="open.toString()"`, `tag-pets-children`} {
		if strings.Contains(body, reject) {
			t.Fatalf("Penguin-style sidebar sub-items should be persistently visible, got collapsible marker %q:\n%s", reject, body)
		}
	}
	for _, want := range []string{`id="manja-theme"`, `name="theme"`, `manja-theme-trigger`, `theme: localStorage.getItem`, `theme = opt.value`} {
		if !strings.Contains(body, want) {
			t.Fatalf("header theme picker missing %q:\n%s", want, body)
		}
	}
	for _, want := range []string{
		`id="darkModeToggleBtn"`,
		`aria-label="dark mode toggle"`,
		`x-on:click="toggleDarkMode()"`,
		`x-bind:aria-pressed="darkMode.toString()"`,
		`x-show="darkMode"`,
		`x-show="!darkMode"`,
		`localStorage.getItem('darkMode')`,
		`classList.toggle('dark', on)`,
		`localStorage.setItem('darkMode'`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("header dark mode toggle missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `class="hidden w-36 sm:block"`) {
		t.Fatalf("theme picker should be visible in the nav, not hidden behind responsive utility classes:\n%s", body)
	}
	for _, theme := range []string{
		`value:&#39;goshtoso&#39;`,
		`value:&#39;arctic&#39;`,
		`value:&#39;minimal&#39;`,
		`value:&#39;modern&#39;`,
		`value:&#39;high-contrast&#39;`,
		`value:&#39;neo-brutalism&#39;`,
		`value:&#39;halloween&#39;`,
		`value:&#39;zombie&#39;`,
		`value:&#39;pastel&#39;`,
		`value:&#39;90s&#39;`,
		`value:&#39;christmas&#39;`,
		`value:&#39;prototype&#39;`,
		`value:&#39;news&#39;`,
		`value:&#39;industrial&#39;`,
		`value:&#39;dracula&#39;`,
	} {
		if !strings.Contains(body, theme) {
			t.Fatalf("theme picker missing Goshtoso theme option %q:\n%s", theme, body)
		}
	}
}

func TestPublicDocsRendersSelectedSidebarItemOnly(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Description: "Listing body", Tags: []string{"Pets"}},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Description: "Creation body", Tags: []string{"Pets"}},
		},
		Schemas: []core.Schema{{Name: "Pet", Description: "A pet schema body"}},
	}
	srv := NewPublicServer(idx)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?selected=operation-createPet", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("operation status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="operation-createPet"`, `Create pet`, `Creation body`, `href="/?selected=operation-createPet#operation-createPet"`, `<span class="sr-only">active</span>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("selected operation page missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{`<section id="operation-listPets"`, `Listing body`, `<section id="schema-pet"`} {
		if strings.Contains(body, reject) {
			t.Fatalf("selected operation page should not render %q:\n%s", reject, body)
		}
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/?selected=schema-pet", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("schema status = %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{`id="schema-pet"`, `Pet`, `A pet schema body`, `<span class="sr-only">active</span>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("selected schema page missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{`<section id="operation-listPets"`, `<section id="operation-createPet"`} {
		if strings.Contains(body, reject) {
			t.Fatalf("selected schema page should not render %q:\n%s", reject, body)
		}
	}
}

func TestPublicDocsSearchTargetsVisibleSectionsWithUniqueIDs(t *testing.T) {
	data, err := os.ReadFile("../adapters/openapi/testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := (openapiadapter.Parser{}).Parse(context.Background(), core.SpecFile{
		Path:  "../adapters/openapi/testdata/petstore.yaml",
		Bytes: data,
	}, core.Revision{ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	NewPublicServer(idx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, doc := range idx.Search {
		if doc.Href == "/" {
			continue
		}
		anchor, ok := strings.CutPrefix(doc.Href, "#")
		if !ok {
			t.Fatalf("search href %q is not an anchor", doc.Href)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/?selected="+url.QueryEscape(anchor), nil)

		NewPublicServer(idx).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("selected %q status = %d", anchor, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `<section id="`+anchor+`"`) {
			t.Fatalf("search href %q has no matching visible section:\n%s", doc.Href, body)
		}
	}

	idPattern := regexp.MustCompile(`\bid="([^"]+)"`)
	seen := map[string]bool{}
	for _, match := range idPattern.FindAllStringSubmatch(body, -1) {
		id := match[1]
		if seen[id] {
			t.Fatalf("duplicate DOM id %q in rendered public docs:\n%s", id, body)
		}
		seen[id] = true
	}
}

func TestPublicDocsRenderEndpointDetails(t *testing.T) {
	idx := core.SpecIndex{
		Title:           "Todos",
		ExampleSpecJSON: `{"components":{"schemas":{"Todo":{"type":"object","properties":{"id":{"type":"string"}}}}}}`,
		Operations: []core.Operation{{
			ID:          "updateTodo",
			Anchor:      "operation-updatetodo",
			Method:      "PUT",
			Path:        "/todos/{todoId}",
			Summary:     "Update Todo",
			Description: "Updates a todo item.",
			Tags:        []string{"Todos"},
			Parameters: []core.OperationParameter{{
				Name:        "todoId",
				In:          "path",
				Required:    true,
				Description: "Todo identifier.",
				Schema:      core.SchemaSummary{Type: "string"},
			}, {
				Name:        "include",
				In:          "query",
				Description: "Include related resources.",
				Schema:      core.SchemaSummary{Type: "string"},
			}},
			RequestBody: &core.OperationRequestBody{
				Required: true,
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema: core.SchemaSummary{
						Name: "TodoInput",
						Type: "object",
						Properties: []core.SchemaProperty{{
							Name:        "name",
							Required:    true,
							Schema:      core.SchemaSummary{Type: "string"},
							Description: "Name of the task.",
						}},
					},
					Example: "{\n  \"name\": \"string\"\n}",
				}},
			},
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Updated todo.",
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema:      core.SchemaSummary{Name: "Todo", Type: "object"},
					Example:     "{\n  \"id\": \"string\"\n}",
				}},
			}, {
				Status:      "404",
				Description: "Todo was not found.",
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema: core.SchemaSummary{
						Name: "Error",
						Type: "object",
						Properties: []core.SchemaProperty{{
							Name:        "message",
							Required:    true,
							Schema:      core.SchemaSummary{Type: "string"},
							Description: "Human-readable error.",
						}},
					},
				}},
			}},
			Security: []core.OperationSecurity{{Name: "bearerAuth"}},
			Snippets: []core.RequestSnippet{{
				Label:    "cURL",
				Language: "shell",
				Code:     "curl --request PUT --url https://api.example.test/todos/{todoId}",
			}},
		}},
		Search: []core.SearchDocument{{
			ID:    "operation-updatetodo",
			Title: "PUT /todos/{todoId}",
			Href:  "#operation-updatetodo",
			Kind:  "Operation",
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	NewPublicServer(idx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Update Todo`,
		`/todos/{todoId}`,
		`aria-label="Endpoint route"`,
		`xl:grid-cols-[minmax(0,50rem)_24rem]`,
		`Path Parameters`,
		`Query Parameters`,
		`Endpoint parameters`,
		`todoId`,
		`include`,
		`path`,
		`required`,
		`Request body`,
		`TodoInput`,
		`Responses`,
		`200`,
		`Updated todo.`,
		`404`,
		`Todo was not found.`,
		`Security`,
		`bearerAuth`,
		`aria-label="Endpoint examples"`,
		`lg:sticky`,
		`Request Sample: cURL`,
		`Response Example`,
		`cURL`,
		`curl --request PUT`,
		`&#34;name&#34;`,
		`aria-label="Copy Request Sample: cURL code"`,
		`class="codeblock overflow-x-auto"`,
		`id="operation-updatetodo-path-parameters"`,
		`id="operation-updatetodo-query-parameters"`,
		`id="operation-updatetodo-request-body-application-json-schema"`,
		`id="operation-updatetodo-responses"`,
		`role="tablist"`,
		`role="tab"`,
		`role="tabpanel"`,
		`tabpaneloperation-updatetodo-responsesresponse-200`,
		`tabpaneloperation-updatetodo-responsesresponse-404`,
		`caption class="sr-only">Path Parameters</caption>`,
		`caption class="sr-only">Request body schema for application/json</caption>`,
		`caption class="sr-only">Response 404 schema for application/json</caption>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("endpoint detail view missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{"Try It", "Send API Request", "Execute request", `aria-label="On this page"`} {
		if strings.Contains(body, reject) {
			t.Fatalf("endpoint detail view should be read-only, got %q:\n%s", reject, body)
		}
	}
}

func TestPublicDocsRenderGenericSchemaExamples(t *testing.T) {
	idx := core.SpecIndex{
		Title:           "Todos",
		ExampleSpecJSON: `{"components":{"schemas":{"Todo":{"type":"object","properties":{"id":{"type":"string"}}}}}}`,
		Operations: []core.Operation{{
			ID:      "updateTodo",
			Anchor:  "operation-updatetodo",
			Method:  "PUT",
			Path:    "/todos/{todoId}",
			Summary: "Update Todo",
			RequestBody: &core.OperationRequestBody{MediaTypes: []core.OperationMediaType{{
				ContentType: "application/json",
				Schema: core.SchemaSummary{
					Name: "TodoInput",
					Type: "object",
					JSON: `{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"done":{"type":"boolean"}}}`,
				},
				Example: "{\n  \"name\": \"fallback\"\n}",
			}}},
			Responses: []core.OperationResponse{{Status: "200", MediaTypes: []core.OperationMediaType{{
				ContentType: "application/json",
				Schema: core.SchemaSummary{
					Name: "Todo",
					Type: "object",
					JSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
				},
				Example: "{\n  \"id\": \"fallback\"\n}",
			}}}},
		}},
		Schemas: []core.Schema{{Name: "Todo", Description: "A todo.", Example: core.SchemaExample{
			JSON:    `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"name":{"type":"string"}}}`,
			Example: "{\n  \"id\": \"fallback\"\n}",
		}}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/")
	for _, want := range []string{
		`data-manja-example`,
		`id="operation-updatetodo-request-body-application-json-example"`,
		`Request Example: application/json`,
		`Response Example: 200 application/json`,
		`type="application/json"`,
		`"spec":{"components":{"schemas":{"Todo"`,
		`"skipNonRequired":false`,
		`"maxSampleDepth":3`,
		`/manja-assets/schema-example.js`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("generic endpoint example missing %q:\n%s", want, body)
		}
	}

	body = renderPublicDocs(t, NewPublicServer(idx), "/?selected=schema-todo")
	for _, want := range []string{
		`id="schema-todo-example"`,
		`Example: Todo`,
		`data-manja-example`,
		`"maxSampleDepth":2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("generic schema example missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{"Try It", "Send API Request", "<form", "Execute request"} {
		if strings.Contains(body, reject) {
			t.Fatalf("example component should stay read-only, got %q:\n%s", reject, body)
		}
	}
}

func renderPublicDocs(t *testing.T, handler http.Handler, target string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	return rec.Body.String()
}
