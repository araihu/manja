package web

import (
	"context"
	"encoding/json"
	"html"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"

	core "github.com/araihu/manja/domain"
	markdownadapter "github.com/araihu/manja/internal/adapters/markdown"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

func TestPublicDocsUsesGoshtosoCDNFirstDependencyFallbackContract(t *testing.T) {
	type dependencyEntry struct {
		Name                string `json:"name"`
		PrimaryURL          string `json:"primary_url"`
		FallbackURL         string `json:"fallback_url,omitempty"`
		Integrity           string `json:"integrity,omitempty"`
		WaitForWindowLoaded bool   `json:"wait_for_window_loaded,omitempty"`
	}
	type dependencyConfig struct {
		Dependencies []dependencyEntry `json:"dependencies"`
	}

	srv := NewPublicServer(core.SpecIndex{Title: "Petstore", Version: "1.0.0"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(templ.WithNonce(req.Context(), "manja-csp-nonce"))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	loaderTag := regexp.MustCompile(`<script[^>]*src="/assets/js/dependency-loader\.js"[^>]*>`).FindString(body)
	if loaderTag == "" {
		t.Fatalf("public docs head missing Goshtoso dependency loader:\n%s", body)
	}
	if !strings.Contains(loaderTag, `nonce="manja-csp-nonce"`) {
		t.Fatalf("Goshtoso dependency loader did not inherit the request CSP nonce: %s", loaderTag)
	}

	match := regexp.MustCompile(`data-goshtoso-dependencies="([^"]+)"`).FindStringSubmatch(loaderTag)
	if len(match) != 2 {
		t.Fatalf("dependency loader missing public configuration: %s", loaderTag)
	}
	var config dependencyConfig
	if err := json.Unmarshal([]byte(html.UnescapeString(match[1])), &config); err != nil {
		t.Fatalf("decode dependency loader configuration: %v\n%s", err, loaderTag)
	}

	want := []dependencyEntry{
		{Name: "alpine-collapse", PrimaryURL: "https://unpkg.com/@alpinejs/collapse@3.14.9/dist/cdn.min.js", FallbackURL: "/assets/js/runtime/alpinejs-collapse/3.14.9/alpine-collapse.min.js"},
		{Name: "alpine-focus", PrimaryURL: "https://unpkg.com/@alpinejs/focus@3.14.9/dist/cdn.min.js", FallbackURL: "/assets/js/runtime/alpinejs-focus/3.14.9/alpine-focus.min.js"},
		{Name: "alpine-mask", PrimaryURL: "https://unpkg.com/@alpinejs/mask@3.14.9/dist/cdn.min.js", FallbackURL: "/assets/js/runtime/alpinejs-mask/3.14.9/alpine-mask.min.js"},
		{Name: "alpine", PrimaryURL: "https://unpkg.com/alpinejs@3.14.9/dist/cdn.min.js", FallbackURL: "/assets/js/runtime/alpinejs/3.14.9/alpine.min.js"},
		{Name: "htmx", PrimaryURL: "https://unpkg.com/htmx.org@2.0.8/dist/htmx.min.js", FallbackURL: "/assets/js/runtime/htmx.org/2.0.8/htmx.min.js", WaitForWindowLoaded: true},
		{Name: "combobox", PrimaryURL: "/assets/js/combobox.js"},
	}
	if len(config.Dependencies) != len(want) {
		t.Fatalf("dependency count = %d, want %d: %#v", len(config.Dependencies), len(want), config.Dependencies)
	}
	for i, expected := range want {
		got := config.Dependencies[i]
		if got.Name != expected.Name || got.PrimaryURL != expected.PrimaryURL || got.FallbackURL != expected.FallbackURL {
			t.Errorf("dependency %d = (%q, %q, %q), want (%q, %q, %q)", i, got.Name, got.PrimaryURL, got.FallbackURL, expected.Name, expected.PrimaryURL, expected.FallbackURL)
		}
		if got.WaitForWindowLoaded != expected.WaitForWindowLoaded {
			t.Errorf("%s wait_for_window_loaded = %t, want %t", got.Name, got.WaitForWindowLoaded, expected.WaitForWindowLoaded)
		}
		if got.Name != "combobox" && !strings.HasPrefix(got.Integrity, "sha384-") {
			t.Errorf("%s integrity = %q, want SHA-384 SRI", got.Name, got.Integrity)
		}
	}

	assetURLs := []string{"/assets/styles.css", "/assets/js/dependency-loader.js", "/assets/js/combobox.js"}
	for _, dependency := range want {
		if dependency.FallbackURL != "" {
			assetURLs = append(assetURLs, dependency.FallbackURL)
		}
	}
	for _, assetURL := range assetURLs {
		t.Run(assetURL, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, assetURL, nil))
			if rec.Code != http.StatusOK {
				t.Errorf("GET %s status = %d, want %d", assetURL, rec.Code, http.StatusOK)
			}
		})
	}
}

func TestPublicDocsRenderSearchAndOperations(t *testing.T) {
	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Tags: []string{"Pets"}},
			{ID: "listStores", Method: "GET", Path: "/stores", Summary: "List stores", Tags: []string{"Stores"}},
		},
		Schemas: []core.Schema{{Name: "Pet", Description: "A pet"}},
		Search: []core.SearchDocument{
			{ID: "operation-listPets", Title: "GET /pets", Description: "List pets", Href: "#operation-listPets", Kind: "Operation", Section: "Pets"},
			{ID: "operation-listStores", Title: "GET /stores", Description: "List stores", Href: "#operation-listStores", Kind: "Operation", Section: "Stores"},
			{ID: "schema-Pet", Title: "Pet", Description: "A pet", Href: "#schema-pet", Kind: "Schema", Section: "Schemas"},
		},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?selected=operation-listPets", nil)

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
		`fixed inset-0 flex flex-col overflow-clip`,
		`Manja`,
		`class="flex h-16`,
		`flex min-h-0 flex-1 overflow-clip`,
		`hidden h-full w-72 shrink-0 lg:block`,
		`class="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden p-6`,
		`document.documentElement.classList.add('boot')`,
		`data-boot-anim="header"`,
		`data-boot-anim="sidebar"`,
		`data-boot-anim="main"`,
		`aria-label="API sections"`,
		`aria-label="Documentation search"`,
		`href="/?selected=operation-listPets#operation-listPets"`,
		`href="/?selected=schema-pet#schema-pet"`,
		`id="main-content"`,
		`max-w-[100rem]`,
		`Operations`,
		`data-search-source-url="/search.json"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs shell missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `id="search-operation-listPets"`) || strings.Contains(body, `data-search-title="GET /pets"`) {
		t.Fatalf("search records should load from /search.json instead of pre-rendering every result:\n%s", body)
	}
	if strings.Contains(body, `<section id="operation-createPet"`) {
		t.Fatalf("operation page should render only the selected sidebar item, got create operation content:\n%s", body)
	}
	sidebarMethodBadge := regexp.MustCompile(`<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span[^>]*>List pets</span>\s*<sup[^>]*>GET</sup>`)
	if !sidebarMethodBadge.MatchString(body) {
		t.Fatalf("operation sidebar link should associate its label with the GET method text:\n%s", body)
	}
	postMethodBadge := regexp.MustCompile(`<a href="/\?selected=operation-createPet#operation-createPet"[^>]*><span[^>]*>Create pet</span>\s*<sup[^>]*>POST</sup>`)
	if !postMethodBadge.MatchString(body) {
		t.Fatalf("operation sidebar link should associate its label with the POST method text:\n%s", body)
	}
	pageMethodBadge := regexp.MustCompile(`<div aria-label="Endpoint route"[^>]*>\s*<span[^>]*>GET</span>\s*<p[^>]*>/pets</p>`)
	if !pageMethodBadge.MatchString(body) {
		t.Fatalf("selected endpoint should expose its method and path in the labelled native route group:\n%s", body)
	}
	sidebarTagGroup := regexp.MustCompile(`<div data-sidebar-section="Operations">.*<div x-data="\{ open: true \}">.*<a href="#"[^>]*x-on:click\.prevent="open = !open"[^>]*aria-controls="tag-pets-children"[^>]*><span class="min-w-0 flex-1 truncate">Pets</span>.*<div id="tag-pets-children" x-show="open" class="ml-4 flex flex-col">.*<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">List pets</span>\s*<sup[^>]*>GET</sup>`)
	if !sidebarTagGroup.MatchString(body) {
		t.Fatalf("operation sidebar items should open the selected tag group on full page render:\n%s", body)
	}
	petsControl := regexp.MustCompile(`<a [^>]*aria-controls="tag-pets-children"[^>]*>`).FindString(body)
	if petsControl == "" {
		t.Fatalf("operation tag group should render a Pets disclosure control:\n%s", body)
	}
	if !strings.Contains(petsControl, `href="#"`) {
		t.Fatalf("operation tag disclosure should not navigate to the first endpoint, got:\n%s", petsControl)
	}
	if strings.Contains(petsControl, `hx-get=`) {
		t.Fatalf("operation tag disclosure should not issue HTMX navigation, got:\n%s", petsControl)
	}
	storesGroup := regexp.MustCompile(`<div x-data="\{ open: false \}">.*<a href="#"[^>]*aria-controls="tag-stores-children"[^>]*><span class="min-w-0 flex-1 truncate">Stores</span>.*<div id="tag-stores-children" x-show="open" style="display: none;" class="ml-4 flex flex-col">`)
	if !storesGroup.MatchString(body) {
		t.Fatalf("unselected operation tag groups should stay collapsed:\n%s", body)
	}
	for _, want := range []string{`aria-controls="tag-pets-children"`, `aria-controls="tag-stores-children"`, `x-bind:aria-expanded="open.toString()"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("operation tag group missing collapsible marker %q:\n%s", want, body)
		}
	}
	for _, want := range []string{
		`data-theme="manja"`,
		`id="manja-theme"`,
		`name="theme"`,
		`manja-theme-trigger`,
		`theme: localStorage.getItem('theme') || 'manja'`,
		`localStorage.getItem('theme') || 'manja'`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("header theme picker or default theme missing %q:\n%s", want, body)
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
		`value:&#39;manja&#39;`,
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
			t.Fatalf("theme picker missing theme option %q:\n%s", theme, body)
		}
	}
	if !regexp.MustCompile(`allOptions:\s*\[\{value:&#39;manja&#39;,label:&#39;Manja&#39;\},\{value:&#39;goshtoso&#39;,label:&#39;Goshtoso&#39;\}`).MatchString(body) {
		t.Fatalf("Manja theme option should be first and Goshtoso should remain available:\n%s", body)
	}
	if !strings.Contains(body, `selectedValues: [&#39;manja&#39;]`) {
		t.Fatalf("Manja theme option should be selected by default:\n%s", body)
	}
	if strings.Contains(body, `data-theme="goshtoso"`) || strings.Contains(body, `|| 'goshtoso'`) {
		t.Fatalf("public docs should default to the Manja theme, not Goshtoso:\n%s", body)
	}

	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`html.boot [data-boot-anim="header"]`,
		`html.boot [data-boot-anim="sidebar"]`,
		`html.boot [data-boot-anim="main"]`,
		`#main-content.htmx-swapping`,
		`#main-content > .htmx-added`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(string(css), want) {
			t.Fatalf("public docs transition CSS missing %q:\n%s", want, css)
		}
	}
}

func TestPublicDocsOperationTagDisclosuresDoNotDrawRails(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Tags: []string{"Pets"}},
		},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=operation-listPets")
	petsControl := regexp.MustCompile(`<a href="#"[^>]*aria-controls="tag-pets-children"[^>]*>`).FindString(body)
	if petsControl == "" {
		t.Fatalf("operation tag group should render a Pets disclosure control:\n%s", body)
	}
	if !strings.Contains(petsControl, `data-manja-sidebar-tag="true"`) {
		t.Fatalf("operation tag disclosure should be marked for rail-free styling:\n%s", petsControl)
	}

	childLink := regexp.MustCompile(`<a href="/\?selected=operation-listPets#operation-listPets"[^>]*>`).FindString(body)
	if childLink == "" {
		t.Fatalf("operation tag children should contain the selected endpoint link:\n%s", body)
	}
	if strings.Contains(childLink, `data-manja-sidebar-tag="true"`) {
		t.Fatalf("operation child links should not be styled as tag disclosure controls:\n%s", childLink)
	}
	if !strings.Contains(childLink, `border-l`) {
		t.Fatalf("operation child links should keep sidebar rails:\n%s", childLink)
	}

	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`(?s)#sidebar-nav-content\s+a\[data-manja-sidebar-tag="true"\]\s*\{[^}]*\}`).FindString(string(css))
	if rule == "" {
		t.Fatalf("missing rail-free sidebar tag disclosure CSS rule")
	}
	for _, want := range []string{
		`border-left-width: 0;`,
		`padding-left: 0;`,
	} {
		if !strings.Contains(rule, want) {
			t.Fatalf("sidebar tag disclosure CSS should include %q:\n%s", want, rule)
		}
	}
}

func TestPublicDocsSchemaSidebarChildrenIndentLikeTagChildren(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Schema Labels",
		Schemas: []core.Schema{
			{Name: "workflow", Summary: core.SchemaSummary{Name: "Workflow"}},
		},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=schema-workflow")
	schemaSection := regexp.MustCompile(`(?s)<div data-sidebar-section="Schemas">.*?</div></div>`).FindString(body)
	if schemaSection == "" {
		t.Fatalf("schema sidebar section should render:\n%s", body)
	}
	if !strings.Contains(schemaSection, `<div class="ml-4 flex flex-col">`) {
		t.Fatalf("schema sidebar children should use Goshtoso nested item indentation:\n%s", schemaSection)
	}
}

func TestPublicDocsServeSearchIndexJSON(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Search: []core.SearchDocument{{
			ID:          "operation-listPets",
			Title:       "GET /pets",
			Description: "List pets",
			Href:        "#operation-listPets",
			Kind:        "Operation",
			Section:     "Pets",
			Keywords:    []string{"listPets", "GET", "/pets", "Pets"},
		}},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search.json", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	contentType, _, err := mime.ParseMediaType(rec.Header().Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}
	var got []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Href        string   `json:"href"`
		Section     string   `json:"section"`
		Keywords    []string `json:"keywords"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("search documents = %d, want 1: %#v", len(got), got)
	}
	if got[0].ID != "search-operation-listPets" || got[0].Title != "GET /pets" || got[0].Href != "/?selected=operation-listPets#operation-listPets" {
		t.Fatalf("search item = %#v", got[0])
	}
	if strings.Join(got[0].Keywords, " ") != "listPets GET /pets Pets" {
		t.Fatalf("keywords = %#v", got[0].Keywords)
	}
}

func TestPublicDocsServeSearchIndexJSONPrefersPublicRouteForMatchingAnchor(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Search: []core.SearchDocument{{
			ID:          "operation-listPets",
			Title:       "GET /pets",
			Description: "List pets",
			Href:        "#operation-listPets",
			Kind:        "Operation",
			Section:     "Pets",
		}},
		PublicRoutes: []core.PublicRoute{{
			Path:        "/docs?selected=operation-listPets#operation-listPets",
			Title:       "GET /pets",
			Description: "List pets",
		}},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search.json", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []struct {
		ID   string `json:"id"`
		Href string `json:"href"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("search documents = %d, want 1: %#v", len(got), got)
	}
	if got[0].Href != "/docs?selected=operation-listPets#operation-listPets" {
		t.Fatalf("search href = %q, want route-index href", got[0].Href)
	}
}

func TestPublicDocsSearchIndexUsesPublicRoutes(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Widget API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: getWidgets
      summary: List widgets
      responses:
        "200":
          description: ok
components:
  schemas:
    Widget:
      type: object
      description: A widget resource.
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "widgets.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}

	wantOperation := "/?selected=operation-getwidgets#operation-getwidgets"
	wantSchema := "/?selected=schema-widget#schema-widget"
	for _, want := range []string{wantOperation, wantSchema} {
		found := false
		for _, route := range idx.PublicRoutes {
			if route.Path == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("public routes missing %q: %#v", want, idx.PublicRoutes)
		}
	}

	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search.json", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Href        string `json:"href"`
		Kind        string `json:"kind"`
		Method      string `json:"method"`
		Path        string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	items := make(map[string]struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Href        string `json:"href"`
		Kind        string `json:"kind"`
		Method      string `json:"method"`
		Path        string `json:"path"`
	}, len(got))
	for _, item := range got {
		items[item.ID] = item
	}
	operation := items["search-operation-getwidgets"]
	if operation.Href != wantOperation {
		t.Fatalf("operation search href = %q, want %q; items = %#v", operation.Href, wantOperation, got)
	}
	if operation.Title != "List widgets" {
		t.Fatalf("operation search title = %q, want List widgets", operation.Title)
	}
	if operation.Description != "" {
		t.Fatalf("operation search description = %q, want empty when it duplicates title", operation.Description)
	}
	if operation.Kind != "Operation" || operation.Method != "GET" || operation.Path != "/widgets" {
		t.Fatalf("operation search metadata = %#v, want kind Operation, method GET, path /widgets", operation)
	}
	schema := items["search-schema-Widget"]
	if schema.Href != wantSchema {
		t.Fatalf("schema search href = %q, want %q; items = %#v", schema.Href, wantSchema, got)
	}
	if schema.Kind != "Schema" {
		t.Fatalf("schema search kind = %q, want Schema; item = %#v", schema.Kind, schema)
	}
	if schema.Method != "" || schema.Path != "" {
		t.Fatalf("schema search should not emit method/path metadata: %#v", schema)
	}
}

func TestPublicDocsRenderOverviewByDefault(t *testing.T) {
	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Overview: core.SpecOverview{
			Description:    "GitHub's v3 REST API.",
			TermsOfService: "https://docs.example.test/terms",
			Contact: core.SpecContact{
				Name: "Contact Support",
				URL:  "https://support.example.test",
			},
			License: core.SpecLicense{
				Name: "MIT",
				URL:  "https://license.example.test",
			},
			Servers: []core.SpecServer{{
				URL:         "{protocol}://{hostname}/api/v3",
				Description: "Live Server",
				Variables: []core.SpecServerVariable{{
					Name:        "hostname",
					Default:     "HOSTNAME",
					Description: "Self-hosted Enterprise Server or Enterprise Cloud hostname",
				}, {
					Name:        "protocol",
					Default:     "http",
					Description: "Self-hosted Enterprise Server or Enterprise Cloud protocol",
				}},
			}},
		},
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Tags: []string{"Pets"}},
		},
		Schemas: []core.Schema{{Name: "Pet", Description: "A pet"}},
		Search: []core.SearchDocument{
			{ID: "overview", Title: "Petstore", Description: "API overview", Href: "#overview", Kind: "Overview"},
			{ID: "operation-listPets", Title: "GET /pets", Description: "List pets", Href: "#operation-listPets", Kind: "Operation", Section: "Pets"},
			{ID: "schema-Pet", Title: "Pet", Description: "A pet", Href: "#schema-pet", Kind: "Schema", Section: "Schemas"},
		},
	}

	body := renderPublicDocs(t, NewPublicServer(idx))

	for _, want := range []string{
		`<section id="overview"`,
		`Petstore`,
		`v1.0.0`,
		`Endpoints`,
		`2 endpoints`,
		`Schemas`,
		`1 schema`,
		`API Base URL`,
		`Live Server`,
		`{protocol}://{hostname}/api/v3`,
		`hostname`,
		`Self-hosted Enterprise Server or Enterprise Cloud hostname`,
		`Default:`,
		`HOSTNAME`,
		`protocol`,
		`http`,
		`Additional Information`,
		`href="https://support.example.test"`,
		`Contact Support`,
		`href="https://license.example.test"`,
		`MIT`,
		`href="https://docs.example.test/terms"`,
		`Terms of Service`,
		`GitHub&#39;s v3 REST API.`,
		`href="/?selected=overview#overview"`,
		`>Overview</span>`,
		`<span class="sr-only">active</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("default overview page missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `<section id="operation-listPets"`) {
		t.Fatalf("default overview page should not render endpoint content:\n%s", body)
	}
	for _, reject := range []string{`data-sidebar-section=""`} {
		if strings.Contains(body, reject) {
			t.Fatalf("default overview page should not render %q:\n%s", reject, body)
		}
	}
}

func TestPublicDocsBrandingDefaultsToManjaAssets(t *testing.T) {
	idx := core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
	}

	body := renderPublicDocs(t, NewPublicServer(idx))

	for _, want := range []string{
		`<title>Petstore</title>`,
		`<link rel="icon" href="/manja-assets/favicon.svg">`,
		`href="/" class="flex min-w-0 items-center gap-2`,
		`<img src="/manja-assets/manja-mark.svg" alt="" width="32" height="32"`,
		`<span>Manja</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("default branding missing %q:\n%s", want, body)
		}
	}
}

func TestPublicDocsBrandingUsesOptionsOverSpecAndDefaults(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Spec API",
		Branding: core.DocsBranding{
			DisplayName: "Spec Brand",
			Logo: core.DocsBrandingLogo{
				Src:     "https://cdn.example.test/spec-logo.svg",
				Alt:     "Spec Brand logo",
				HomeURL: "https://spec.example.test",
			},
			Favicon: "https://cdn.example.test/spec-favicon.svg",
		},
	}

	body := renderPublicDocs(t, NewPublicServerWithOptions(idx, PublicOptions{
		Branding: core.DocsBranding{
			DisplayName: "Acme Developers",
			Logo: core.DocsBrandingLogo{
				Src:     "https://cdn.example.test/acme-logo.svg",
				Alt:     "Acme Developers",
				HomeURL: "https://developers.example.test",
			},
			Favicon: "https://cdn.example.test/acme-favicon.svg",
		},
	}))

	for _, want := range []string{
		`<title>Spec API</title>`,
		`<link rel="icon" href="https://cdn.example.test/acme-favicon.svg">`,
		`href="https://developers.example.test" class="flex min-w-0 items-center gap-2`,
		`<img src="https://cdn.example.test/acme-logo.svg" alt="Acme Developers" width="32" height="32"`,
		`<span>Acme Developers</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("custom branding missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{
		`Spec Brand`,
		`https://cdn.example.test/spec-logo.svg`,
		`https://cdn.example.test/spec-favicon.svg`,
		`/manja-assets/manja-mark.svg`,
	} {
		if strings.Contains(body, reject) {
			t.Fatalf("custom branding should override %q:\n%s", reject, body)
		}
	}
}

func TestPublicDocsRenderMarkdownDescriptions(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Markdown API",
		Overview: core.SpecOverview{
			Description: "Use **REST** docs.\n\nSee [guides](https://docs.example.test).",
			Servers: []core.SpecServer{{
				URL:         "https://api.example.test",
				Description: "Use the **production** base URL.",
				Variables: []core.SpecServerVariable{{
					Name:        "tenant",
					Default:     "demo",
					Description: "Choose a **tenant** value.",
				}},
			}},
		},
		Operations: []core.Operation{{
			ID:          "listPets",
			Anchor:      "operation-listpets",
			Method:      "GET",
			Path:        "/pets",
			Summary:     "List pets",
			Description: "Returns **pets**.\n\n- Fast\n- Safe",
			Parameters: []core.OperationParameter{{
				Name:        "kind",
				In:          "query",
				Description: "Filter by **kind**.",
				Example:     "cat",
				Schema:      core.SchemaSummary{Type: "string"},
			}},
			RequestBody: &core.OperationRequestBody{
				Description: "Send a **pet** payload.",
			},
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Returns **matching** pets.",
			}},
		}},
		Schemas: []core.Schema{{
			Name:        "Pet",
			Description: "A **pet** resource.",
			Summary: core.SchemaSummary{
				Name:        "Pet",
				Type:        "object",
				Description: "A **pet** resource.",
				Properties: []core.SchemaProperty{{
					Name:        "name",
					Description: "The **display** name.",
					Schema: core.SchemaSummary{
						Type:        "string",
						Description: "The **display** name.",
					},
				}},
			},
		}},
		Search: []core.SearchDocument{{
			ID:          "operation-listpets",
			Title:       "GET /pets",
			Description: "Returns **pets**.",
			Href:        "#operation-listpets",
			Kind:        "Operation",
		}, {
			ID:          "schema-Pet",
			Title:       "Pet",
			Description: "A **pet** resource.",
			Href:        "#schema-pet",
			Kind:        "Schema",
		}},
	}
	srv := NewPublicServerWithOptions(idx, PublicOptions{
		MarkdownRenderer: markdownadapter.NewRenderer(),
	})

	overview := renderPublicDocs(t, srv)
	for _, want := range []string{
		`<div class="manja-markdown">`,
		`<strong>REST</strong>`,
		`<a href="https://docs.example.test">guides</a>`,
		`<strong>production</strong>`,
		`<strong>tenant</strong>`,
	} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview markdown missing %q:\n%s", want, overview)
		}
	}
	for _, reject := range []string{`**REST**`, `[guides](https://docs.example.test)`} {
		if strings.Contains(overview, reject) {
			t.Fatalf("overview markdown should render %q instead of preserving markdown syntax:\n%s", reject, overview)
		}
	}

	endpoint := renderPublicDocs(t, srv, "/?selected=operation-listpets")
	for _, want := range []string{
		`<div class="manja-markdown">`,
		`<strong>pets</strong>`,
		`<ul>`,
		`<li>Fast</li>`,
		`<li>Safe</li>`,
		`<strong>kind</strong>`,
		`Example:`,
		`cat`,
		`<strong>pet</strong>`,
		`<strong>matching</strong>`,
	} {
		if !strings.Contains(endpoint, want) {
			t.Fatalf("endpoint markdown missing %q:\n%s", want, endpoint)
		}
	}
	if strings.Contains(endpoint, `**pets**`) {
		t.Fatalf("endpoint markdown should not preserve markdown syntax:\n%s", endpoint)
	}

	schema := renderPublicDocs(t, srv, "/?selected=schema-pet")
	for _, want := range []string{
		`<strong>pet</strong>`,
		`<strong>display</strong>`,
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema markdown missing %q:\n%s", want, schema)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search.json", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d", rec.Code)
	}
	searchBody := rec.Body.String()
	for _, want := range []string{`"description":"Returns pets."`, `"description":"A pet resource."`} {
		if !strings.Contains(searchBody, want) {
			t.Fatalf("search markdown plain text missing %q:\n%s", want, searchBody)
		}
	}
	for _, reject := range []string{`**pets**`, `**pet**`} {
		if strings.Contains(searchBody, reject) {
			t.Fatalf("search descriptions should use markdown plain text, got %q in %s", reject, searchBody)
		}
	}
}

func TestPublicDocsExposeJSONSpecDownload(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Downloadable API
  version: 1.0.0
paths: {}
`)
	idx, err := (openapiadapter.Parser{}).Parse(context.Background(), core.SpecFile{
		Path:   "downloadable.yaml",
		Format: "yaml",
		Bytes:  spec,
	}, core.Revision{ID: "downloadable"})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewPublicServer(idx)

	body := renderPublicDocs(t, srv)
	for _, want := range []string{
		`href="/openapi.json"`,
		`download="downloadable.json"`,
		`Download JSON`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("overview page missing JSON download button marker %q:\n%s", want, body)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("download content type = %q", contentType)
	}
	_, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("download content disposition = %q: %v", rec.Header().Get("Content-Disposition"), err)
	}
	if params["filename"] != "downloadable.json" {
		t.Fatalf("download filename = %q", params["filename"])
	}
	var payload struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("download body is not JSON: %v\n%s", err, rec.Body.String())
	}
	if payload.Info.Title != "Downloadable API" {
		t.Fatalf("download title = %q", payload.Info.Title)
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
	breadcrumb := regexp.MustCompile(`(?s)<nav[^>]*aria-label="breadcrumb"[^>]*>.*?</nav>`).FindString(body)
	if breadcrumb == "" {
		t.Fatalf("selected operation page should render breadcrumb navigation:\n%s", body)
	}
	for _, want := range []string{`href="/?selected=overview#overview"`, `>Home<`, `Pets`, `aria-current="page"`, `Create pet`} {
		if !strings.Contains(breadcrumb, want) {
			t.Fatalf("selected operation breadcrumb missing %q:\n%s", want, breadcrumb)
		}
	}
	if strings.Contains(breadcrumb, `href="/?selected=operation-createPet#operation-createPet"`) {
		t.Fatalf("selected operation breadcrumb should not link the current page:\n%s", breadcrumb)
	}
	if strings.Contains(body, `class="rounded-radius border border-outline px-2 py-1 text-xs font-medium text-on-surface-muted dark:border-outline-dark dark:text-on-surface-dark-muted">Pets</span>`) {
		t.Fatalf("selected operation page should use breadcrumbs instead of the old tag pill:\n%s", body)
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

func TestPublicDocsFragmentRequestReturnsOnlyMainContent(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createPet", Method: "POST", Path: "/pets", Summary: "Create pet", Description: "Creation body", Tags: []string{"Pets"}},
		},
	}
	srv := NewPublicServer(idx)

	full := renderPublicDocs(t, srv, "/?selected=operation-createPet")
	for _, want := range []string{
		`id="main-content"`,
		`id="sidebar-nav-content"`,
		`hx-get="/?selected=operation-createPet#operation-createPet"`,
		`hx-target="#main-content"`,
		`hx-swap="innerHTML swap:120ms settle:240ms"`,
		`hx-push-url="true"`,
		`hx-history="false"`,
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("full page missing fragment navigation marker %q:\n%s", want, full)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?selected=operation-createPet", nil)
	req.Header.Set("HX-Request", "true")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fragment status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Creation body`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fragment response missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{`<!doctype html>`, `<html`, `id="main-content"`, `hx-swap-oob`, `id="sidebar-nav-content"`} {
		if strings.Contains(body, reject) {
			t.Fatalf("fragment response should not include shell/sidebar marker %q:\n%s", reject, body)
		}
	}

	historyRec := httptest.NewRecorder()
	historyReq := httptest.NewRequest(http.MethodGet, "/?selected=operation-listPets", nil)
	historyReq.Header.Set("HX-Request", "true")
	historyReq.Header.Set("HX-History-Restore-Request", "true")
	srv.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history restore status = %d", historyRec.Code)
	}
	historyBody := historyRec.Body.String()
	for _, want := range []string{`<!doctype html>`, `<html`, `id="main-content"`, `id="sidebar-nav-content"`, `hx-history="false"`} {
		if !strings.Contains(historyBody, want) {
			t.Fatalf("history restore should return a complete public document with %q:\n%s", want, historyBody)
		}
	}
}

func TestPublicDocsRendersPoweredByFooterInFullPageAndFragments(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
		},
	}
	srv := NewPublicServer(idx)

	full := renderPublicDocs(t, srv, "/?selected=operation-listPets")
	for _, want := range []string{
		`<footer aria-label="Powered by Manja"`,
		`Powered by`,
		`href="https://manja.araihu.com"`,
		`>Manja</a>`,
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("full page missing powered-by footer marker %q:\n%s", want, full)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?selected=operation-listPets", nil)
	req.Header.Set("HX-Request", "true")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fragment status = %d", rec.Code)
	}
	fragment := rec.Body.String()
	for _, want := range []string{
		`<footer aria-label="Powered by Manja"`,
		`href="https://manja.araihu.com"`,
	} {
		if !strings.Contains(fragment, want) {
			t.Fatalf("fragment response missing powered-by footer marker %q:\n%s", want, fragment)
		}
	}
}

func TestPublicDocsEndpointSidebarLabelMode(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Operations: []core.Operation{
			{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "deletePet", Method: "DELETE", Path: "/pets/{petId}", Tags: []string{"Pets"}},
		},
	}

	t.Run("auto uses endpoint name with path fallback", func(t *testing.T) {
		body := renderPublicDocs(t, NewPublicServer(idx))

		listLabel := regexp.MustCompile(`<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">List pets</span>\s*<sup[^>]*>GET</sup>`)
		if !listLabel.MatchString(body) {
			t.Fatalf("auto sidebar label should prefer endpoint name:\n%s", body)
		}
		if !strings.Contains(body, `title="List pets"`) {
			t.Fatalf("auto sidebar label should expose full endpoint name via title:\n%s", body)
		}
		if strings.Contains(body, `aria-label="List pets"`) {
			t.Fatalf("auto sidebar label should not generate aria-label by default:\n%s", body)
		}
		deleteLabel := regexp.MustCompile(`<a href="/\?selected=operation-deletePet#operation-deletePet"[^>]*><span class="min-w-0 flex-1 truncate">/pets/{petId}</span>\s*<sup[^>]*>DELETE</sup>`)
		if !deleteLabel.MatchString(body) {
			t.Fatalf("auto sidebar label should fall back to endpoint path:\n%s", body)
		}
		if !strings.Contains(body, `title="/pets/{petId}"`) {
			t.Fatalf("path fallback sidebar label should expose full path via title:\n%s", body)
		}
		if strings.Contains(body, `aria-label="/pets/{petId}"`) {
			t.Fatalf("path fallback sidebar label should not generate aria-label by default:\n%s", body)
		}
	})

	t.Run("path option enforces endpoint path", func(t *testing.T) {
		body := renderPublicDocs(t, NewPublicServerWithOptions(idx, PublicOptions{
			EndpointSidebarLabel: EndpointSidebarLabelPath,
		}))

		pathLabel := regexp.MustCompile(`<a href="/\?selected=operation-listPets#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*>GET</sup>`)
		if !pathLabel.MatchString(body) {
			t.Fatalf("path sidebar label option should use endpoint path:\n%s", body)
		}
	})
}

func TestPublicDocsSchemaSidebarLabelPrefersDisplayName(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Schema Labels",
		Schemas: []core.Schema{
			{
				Name:    "workflow",
				Summary: core.SchemaSummary{Name: "Workflow"},
			},
			{Name: "raw-schema-name"},
		},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=schema-workflow")

	displayLabel := regexp.MustCompile(`<a href="/\?selected=schema-workflow#schema-workflow"[^>]*title="Workflow"[^>]*><span class="min-w-0 flex-1 truncate">Workflow</span>`)
	if !displayLabel.MatchString(body) {
		t.Fatalf("schema sidebar label should prefer schema display name:\n%s", body)
	}
	fallbackLabel := regexp.MustCompile(`<a href="/\?selected=schema-raw-schema-name#schema-raw-schema-name"[^>]*title="raw-schema-name"[^>]*><span class="min-w-0 flex-1 truncate">raw-schema-name</span>`)
	if !fallbackLabel.MatchString(body) {
		t.Fatalf("schema sidebar label should fall back to schema name:\n%s", body)
	}
}

func TestPublicDocsRendersSchemaTree(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Todos",
		Schemas: []core.Schema{{
			Name:        "Todo",
			Description: "A todo schema body",
			Summary: core.SchemaSummary{
				Name:        "Todo",
				Type:        "object",
				Description: "A todo schema body",
				Properties: []core.SchemaProperty{{
					Name:        "id",
					Required:    true,
					Description: "Stable todo ID.",
					Schema: core.SchemaSummary{
						Type:    "string",
						Example: "todo_123",
					},
				}, {
					Name:        "labels",
					Description: "Display labels.",
					Schema: core.SchemaSummary{
						Type: "array",
						Items: &core.SchemaSummary{
							Type: "string",
						},
					},
				}, {
					Name:     "owner",
					Required: true,
					Schema: core.SchemaSummary{
						Name: "User",
						Type: "object",
						Properties: []core.SchemaProperty{{
							Name:     "email",
							Required: true,
							Schema: core.SchemaSummary{
								Type:   "string",
								Format: "email",
							},
						}},
					},
				}},
			},
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?selected=schema-todo", nil)

	NewPublicServer(idx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="schema-todo-tree"`,
		`aria-label="Todo schema tree"`,
		`manja-doc-title`,
		`class="manja-schema-tree"`,
		`class="manja-schema-root"`,
		`data-schema-tree-row="id"`,
		`data-schema-tree-row="labels"`,
		`data-schema-tree-row="owner"`,
		`data-schema-tree-row="email"`,
		`manja-schema-row`,
		`manja-schema-caret`,
		`manja-schema-branch`,
		`manja-schema-children`,
		`required`,
		`string`,
		`array[string]`,
		`User object`,
		`string&lt;email&gt;`,
		`Example:`,
		`todo_123`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("schema tree missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `<table`) {
		t.Fatalf("schema page should render a tree component, not a table:\n%s", body)
	}
	if strings.Contains(body, `class="overflow-hidden rounded-radius border border-outline bg-surface text-sm`) {
		t.Fatalf("schema tree should not use the heavy bordered-panel wrapper:\n%s", body)
	}
	if strings.Contains(body, `data-schema-tree-node="Todo"`) {
		t.Fatalf("schema tree should start at the root object properties, not render the root object row:\n%s", body)
	}
}

func TestPublicDocsRendersLongSchemaExamplesAsBlocks(t *testing.T) {
	longExample := strings.Repeat("Contributor Covenant Code of Conduct ", 12)
	idx := core.SpecIndex{
		Title: "Todos",
		Schemas: []core.Schema{{
			Name: "Repository",
			Summary: core.SchemaSummary{
				Name: "Repository",
				Type: "object",
				Properties: []core.SchemaProperty{{
					Name: "code_of_conduct",
					Schema: core.SchemaSummary{
						Name:        "Code Of Conduct",
						Type:        "object",
						Description: "Code Of Conduct",
						Properties: []core.SchemaProperty{{
							Name: "body",
							Schema: core.SchemaSummary{
								Type:    "string",
								Example: longExample,
							},
						}},
					},
				}},
			},
		}},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?selected=schema-repository", nil)

	NewPublicServer(idx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-schema-tree-row="body"`,
		`class="manja-schema-example manja-schema-example-block"`,
		`<pre><code>`,
		`Contributor Covenant Code of Conduct`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("long schema example missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `<p class="manja-schema-example"><span>Example:</span><code>`) {
		t.Fatalf("long schema example should not render as an inline chip:\n%s", body)
	}
}

func TestPublicDocsSchemaTreeCSSConnectsBranchRails(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	childrenRule := regexp.MustCompile(`(?s)\.manja-schema-children\s*\{[^}]*\}`)
	rule := childrenRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-schema-children rule")
	}
	if !strings.Contains(string(css), `--manja-schema-child-indent-x: 1.5rem;`) {
		t.Fatalf("schema tree should indent child rails enough to read as nested ownership")
	}
	if !strings.Contains(string(css), `--manja-schema-root-child-indent-x: 2.5rem;`) {
		t.Fatalf("schema tree root-property children should have a tab-like indent")
	}
	if !strings.Contains(rule, `margin-left: var(--manja-schema-child-indent-x);`) {
		t.Fatalf("schema children rail should use the child indent coordinate:\n%s", rule)
	}
	rootChildrenRule := regexp.MustCompile(`(?s)\.manja-schema-root\s*>\s*\.manja-schema-node\s*>\s*\.manja-schema-children\s*\{[^}]*\}`)
	rootRule := rootChildrenRule.FindString(string(css))
	if rootRule == "" {
		t.Fatalf("missing root-property child rail rule")
	}
	if !strings.Contains(rootRule, `margin-left: var(--manja-schema-root-child-indent-x);`) {
		t.Fatalf("schema root-property child rail should use the stronger root child indent:\n%s", rootRule)
	}
	if !strings.Contains(rule, `padding: 0.125rem 0 0.25rem 0;`) {
		t.Fatalf("schema children rail should start at the child row branch origin:\n%s", rule)
	}
	nameRule := regexp.MustCompile(`(?s)\.manja-schema-name\s*\{[^}]*\}`).FindString(string(css))
	if nameRule == "" {
		t.Fatalf("missing .manja-schema-name rule")
	}
	if !strings.Contains(nameRule, `min-width: 0;`) || !strings.Contains(nameRule, `overflow-wrap: anywhere;`) {
		t.Fatalf("schema property names should wrap inside their grid column instead of overlapping required labels or examples:\n%s", nameRule)
	}
}

func TestPublicDocsSchemaTreeCSSSeparatesCaretFromBranch(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	caretRule := regexp.MustCompile(`(?s)\.manja-schema-caret::before\s*\{[^}]*\}`)
	rule := caretRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-schema-caret::before rule")
	}
	if strings.Contains(rule, `left: -`) || strings.Contains(rule, "\n  right:") {
		t.Fatalf("schema tree caret should be anchored at the elbow right tip, not offset outside the elbow lane:\n%s", rule)
	}
	if !strings.Contains(string(css), `--manja-schema-caret-gap: 0.4rem;`) {
		t.Fatalf("schema tree should reserve a small gap between elbow tips and carets")
	}
	if !strings.Contains(rule, `left: calc(var(--manja-schema-elbow-tip-x) + var(--manja-schema-caret-gap));`) {
		t.Fatalf("schema tree caret should sit after the elbow right tip with spacing:\n%s", rule)
	}
	if !strings.Contains(rule, `top: 0.55rem;`) {
		t.Fatalf("schema tree caret should be vertically centered on the elbow line:\n%s", rule)
	}
	branchRule := regexp.MustCompile(`(?s)\.manja-schema-branch::before\s*\{[^}]*\}`).FindString(string(css))
	if !strings.Contains(branchRule, `left: 0;`) {
		t.Fatalf("schema tree branch should pass through the rail before reaching the elbow tip:\n%s", branchRule)
	}
	if !strings.Contains(branchRule, `width: var(--manja-schema-elbow-tip-x);`) {
		t.Fatalf("schema tree branch should extend to the elbow tip where the caret sits:\n%s", branchRule)
	}
	if strings.Contains(string(css), `.manja-schema-node > summary .manja-schema-branch::after`) {
		t.Fatalf("schema tree caret should not be drawn on the branch connector")
	}
}

func TestPublicDocsSchemaTitleCSSWrapsLongNames(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	titleRule := regexp.MustCompile(`(?s)\.manja-doc-title\s*\{[^}]*\}`)
	rule := titleRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-doc-title rule")
	}
	if !strings.Contains(rule, `overflow-wrap: anywhere;`) {
		t.Fatalf("schema title should wrap long API identifiers:\n%s", rule)
	}
}

func TestPublicDocsSchemaInlineExampleCSSWrapsLongValues(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	exampleRule := regexp.MustCompile(`(?s)\.manja-schema-example-inline code\s*\{[^}]*\}`)
	rule := exampleRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-schema-example-inline code rule")
	}
	if !strings.Contains(rule, `overflow-wrap: anywhere;`) {
		t.Fatalf("inline schema examples should wrap long URLs:\n%s", rule)
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
				Schema:      core.SchemaSummary{Type: "string", Default: "owner"},
			}, {
				Name:        "accept",
				In:          "header",
				Required:    true,
				Description: "Accept header.",
				Schema:      core.SchemaSummary{Type: "string", Default: "application/vnd.example+json"},
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
	req := httptest.NewRequest(http.MethodGet, "/?selected=operation-updatetodo", nil)

	NewPublicServer(idx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Update Todo`,
		`/todos/{todoId}`,
		`aria-label="Endpoint route"`,
		`<div class="manja-endpoint-shell-layout">`,
		`<div class="manja-endpoint-detail-layout">`,
		`<section class="grid gap-8" aria-label="Request">`,
		`<section class="manja-endpoint-responses-section grid gap-5" aria-label="Responses">`,
		`<section class="grid gap-4">`,
		`Path Parameters`,
		`Query Parameters`,
		`Header Parameters`,
		`Request configuration`,
		`data-manja-request-config-panel`,
		`allowMultiple: true`,
		`px-1 text-sm font-semibold text-on-surface-strong dark:text-on-surface-dark-strong`,
		`bg-surface-alt/40 dark:bg-surface-dark-alt/50`,
		`x-collapse`,
		`Server Variables`,
		`Parameters`,
		`Body`,
		`data-manja-request-config-body`,
		`border border-outline bg-surface px-3 py-2 font-mono text-xs leading-5 text-on-surface-strong outline-none placeholder:text-on-surface-muted focus:ring-2 focus:ring-primary dark:border-outline-dark dark:bg-surface-dark dark:text-on-surface-dark-strong dark:placeholder:text-on-surface-dark-muted dark:focus:ring-primary-dark`,
		`name="server.hostname"`,
		`value="HOSTNAME"`,
		`name="server.protocol"`,
		`value="http"`,
		`name="parameters.include"`,
		`value="owner"`,
		`name="parameters.accept"`,
		`value="application/vnd.example+json"`,
		`todoId`,
		`include`,
		`accept`,
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
		`<aside class="manja-endpoint-examples-rail" aria-label="Endpoint examples">`,
		`<div class="manja-endpoint-examples-rail-content">`,
		`aria-label="200"`,
		`bg-success`,
		`text-on-success`,
		`border border-success bg-success text-on-success`,
		`bg-warning`,
		`text-on-warning`,
		`border border-warning bg-warning text-on-warning`,
		`Request Sample: Shell / cURL`,
		`Response Example`,
		`cURL`,
		`data-manja-request-sample-target`,
		`name="requestSampleTarget"`,
		`Shell / cURL`,
		`JavaScript / fetch`,
		`Python / Requests`,
		`Go / NewRequest`,
		`curl --request PUT`,
		`&#34;name&#34;`,
		`aria-label="Copy Request Sample: Shell / cURL code"`,
		`class="codeblock overflow-x-auto"`,
		`id="operation-updatetodo-path-parameters"`,
		`id="operation-updatetodo-query-parameters"`,
		`id="operation-updatetodo-request-body-application-json-schema"`,
		`id="operation-updatetodo-responses"`,
		`role="tablist"`,
		`role="tab"`,
		`role="tabpanel"`,
		`tabpaneloperation-updatetodo-responsesresponse-200`,
		`id="tabpaneloperation-updatetodo-responsesresponse-200" role="tabpanel" aria-label="200"><section class="grid gap-4">`,
		`class="manja-response-panel-main grid gap-4"`,
		`class="manja-response-panel-example"`,
		`class="manja-response-media-block border-t border-outline pt-6 pb-5 dark:border-outline-dark"><div class="manja-response-panel-layout">`,
		`tabpaneloperation-updatetodo-responsesresponse-404`,
		`caption class="sr-only">Path Parameters</caption>`,
		`aria-label="Request body schema for application/json schema tree"`,
		`aria-label="Response 404 schema for application/json schema tree"`,
		`class="manja-schema-root"`,
		`data-schema-tree-row="message"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("endpoint detail view missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `data-schema-tree-node="Error"`) {
		t.Fatalf("endpoint schema tree should start at the root object properties, not render the root object row:\n%s", body)
	}
	for _, reject := range []string{
		`size-1.5 rounded-full bg-success`,
		`size-1.5 rounded-full bg-danger`,
		`class="dark grid min-w-0 gap-2" data-manja-request-config-panel`,
		`font-mono text-on-surface-dark-strong">security</span>`,
		`<span class="text-on-surface-dark-muted">:</span>`,
		`<span class="truncate text-on-surface-dark-muted">bearerAuth</span>`,
		`border border-outline-dark bg-surface-dark px-3 py-2 font-mono text-xs leading-5 text-on-surface-dark-strong`,
	} {
		if strings.Contains(body, reject) {
			t.Fatalf("endpoint detail view should not render stale dark-only styling %q:\n%s", reject, body)
		}
	}
	for status, statusBadge := range map[string]*regexp.Regexp{
		"200": regexp.MustCompile(`<span class="[^"]*border border-success bg-success text-on-success[^"]*">200</span>`),
		"404": regexp.MustCompile(`<span class="[^"]*border border-warning bg-warning text-on-warning[^"]*">404</span>`),
	} {
		if count := len(statusBadge.FindAllString(body, -1)); count != 1 {
			t.Fatalf("response status badge %q should render once in the tab, got %d:\n%s", status, count, body)
		}
	}
	rail := htmlBetween(t, body, `<aside class="manja-endpoint-examples-rail"`, `</aside>`)
	if strings.Contains(rail, `Response Example`) {
		t.Fatalf("endpoint examples rail should not include response examples:\n%s", rail)
	}
	for _, reject := range []string{"Try It", "Send API Request", "Execute request", `aria-label="On this page"`} {
		if strings.Contains(body, reject) {
			t.Fatalf("endpoint detail view should be read-only, got %q:\n%s", reject, body)
		}
	}
}

func TestPublicDocsEndpointDetailCSSStacksRequestAndResponses(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	layoutRule := regexp.MustCompile(`(?s)\.manja-endpoint-detail-layout\s*\{[^}]*\}`)
	rule := layoutRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-endpoint-detail-layout rule")
	}
	for _, want := range []string{
		`display: grid;`,
		`grid-template-columns: minmax(0, 1fr);`,
	} {
		if !strings.Contains(rule, want) {
			t.Fatalf("endpoint detail layout should stack by default with %q:\n%s", want, rule)
		}
	}
	if !strings.Contains(string(css), `.manja-endpoint-responses-section`) {
		t.Fatalf("endpoint responses section should own stacked-layout divider spacing")
	}
	if !strings.Contains(string(css), `.manja-endpoint-detail-layout-single`) {
		t.Fatalf("endpoint detail layout should have a single-child class for response-only endpoints")
	}
	for _, reject := range []string{
		`@container endpoint-main`,
		`.manja-endpoint-detail-layout:not(.manja-endpoint-detail-layout-single)`,
		`grid-template-columns: repeat(2, minmax(0, 1fr));`,
	} {
		if strings.Contains(string(css), reject) {
			t.Fatalf("endpoint detail layout should keep Request and Responses stacked, got %q in CSS", reject)
		}
	}
}

func TestPublicDocsRequestSampleHighlightCSSUsesThemeTokens(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	source := string(css)
	for _, want := range []string{
		`.codeblock .hljs-keyword`,
		`.codeblock .hljs-string`,
		`.codeblock .hljs-comment`,
		`display: contents;`,
		`var(--color-purple-700)`,
		`var(--color-blue-700)`,
		`var(--color-on-surface-muted)`,
		`var(--color-purple-400)`,
		`var(--color-blue-400)`,
		`var(--color-on-surface-dark-muted)`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("request sample highlight CSS should use theme token marker %q", want)
		}
	}
}

func TestPublicDocsManjaThemeCSSDefinesBrandTokens(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	body := string(css)
	for _, want := range []string{
		`[data-theme=manja]`,
		`--color-surface: #f7f4ec;`,
		`--color-surface-alt: #fffdf8;`,
		`--color-primary: #0d8f73;`,
		`--color-secondary: #18d6a7;`,
		`--color-surface-dark: #101513;`,
		`--color-primary-dark: #68f0c8;`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Manja theme CSS missing %q:\n%s", want, body)
		}
	}
}

func TestPublicDocsMarkdownCSSUsesThemeTokens(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	source := string(css)
	for _, want := range []string{
		`.manja-markdown`,
		`var(--color-on-surface)`,
		`var(--color-on-surface-muted)`,
		`var(--color-outline)`,
		`var(--color-surface-alt)`,
		`var(--color-primary)`,
		`var(--font-title)`,
		`var(--radius-radius)`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("markdown CSS should use theme token marker %q", want)
		}
	}
	if strings.Contains(source, `prose`) {
		t.Fatalf("markdown CSS should not depend on Tailwind Typography prose classes")
	}
}

func TestPublicDocsEndpointResponsesOnlyUsesSingleDetailColumn(t *testing.T) {
	idx := core.SpecIndex{
		Title: "GitHub",
		Operations: []core.Operation{{
			ID:          "root",
			Anchor:      "operation-root",
			Method:      "GET",
			Path:        "/",
			Summary:     "GitHub API Root",
			Description: "Get Hypermedia links to resources accessible in GitHub's REST API.",
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Response",
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema: core.SchemaSummary{
						Type: "object",
						Properties: []core.SchemaProperty{{
							Name:     "authorizations_url",
							Required: true,
							Schema:   core.SchemaSummary{Type: "string"},
						}},
					},
				}},
			}},
			Snippets: []core.RequestSnippet{{
				Label:    "cURL",
				Language: "shell",
				Code:     "curl --request GET --url https://api.example.test/",
			}},
		}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=operation-root")
	for _, want := range []string{
		`<div class="manja-endpoint-detail-layout manja-endpoint-detail-layout-single">`,
		`aria-label="200"`,
		`bg-success`,
		`text-on-success`,
		`Request Sample: cURL`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response-only endpoint layout missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `rounded-radius px-3 py-1 font-mono text-xs font-bold bg-success`) {
		t.Fatalf("response status should use Goshtoso solid badge, not custom status pill:\n%s", body)
	}
	if strings.Contains(body, `size-1.5 rounded-full bg-success`) {
		t.Fatalf("response status should use one solid badge treatment without dot indicators:\n%s", body)
	}
	if count := len(regexp.MustCompile(`<span class="[^"]*border border-success bg-success text-on-success[^"]*">200</span>`).FindAllString(body, -1)); count != 1 {
		t.Fatalf("response status badge should render once in the tab, got %d:\n%s", count, body)
	}
}

func TestPublicDocsResponseStatusBadgesUseStatusClassHierarchy(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Status API",
		Operations: []core.Operation{{
			ID:      "statuses",
			Anchor:  "operation-statuses",
			Method:  "GET",
			Path:    "/statuses",
			Summary: "Get statuses",
			Responses: []core.OperationResponse{{
				Status:      "102",
				Description: "Processing.",
			}, {
				Status:      "200",
				Description: "OK.",
			}, {
				Status:      "302",
				Description: "Redirect.",
			}, {
				Status:      "404",
				Description: "Missing.",
			}, {
				Status:      "500",
				Description: "Server error.",
			}},
		}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=operation-statuses")
	for _, want := range []string{
		`aria-label="102"`,
		`aria-label="200"`,
		`aria-label="302"`,
		`aria-label="404"`,
		`aria-label="500"`,
		`border border-outline bg-surface-alt text-on-surface`,
		`border border-success bg-success text-on-success`,
		`border border-primary bg-primary text-on-primary`,
		`border border-warning bg-warning text-on-warning`,
		`border border-danger bg-danger text-on-danger`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("status hierarchy missing %q:\n%s", want, body)
		}
	}
}

func TestPublicDocsMethodBadgesAssociateMethodsWithOperations(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Method API",
		Operations: []core.Operation{{
			ID:      "read",
			Anchor:  "operation-read",
			Method:  "GET",
			Path:    "/resource",
			Summary: "Read resource",
		}, {
			ID:      "create",
			Anchor:  "operation-create",
			Method:  "POST",
			Path:    "/resource",
			Summary: "Create resource",
		}, {
			ID:      "replace",
			Anchor:  "operation-replace",
			Method:  "PUT",
			Path:    "/resource",
			Summary: "Replace resource",
		}, {
			ID:      "update",
			Anchor:  "operation-update",
			Method:  "PATCH",
			Path:    "/resource",
			Summary: "Update resource",
		}, {
			ID:      "delete",
			Anchor:  "operation-delete",
			Method:  "DELETE",
			Path:    "/resource",
			Summary: "Delete resource",
		}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/")
	for _, item := range []struct {
		anchor  string
		method  string
		summary string
	}{
		{"operation-read", "GET", "Read resource"},
		{"operation-create", "POST", "Create resource"},
		{"operation-replace", "PUT", "Replace resource"},
		{"operation-update", "PATCH", "Update resource"},
		{"operation-delete", "DELETE", "Delete resource"},
	} {
		sidebarPattern := regexp.MustCompile(`<a href="/\?selected=` + regexp.QuoteMeta(item.anchor) + `#` + regexp.QuoteMeta(item.anchor) + `"[^>]*><span[^>]*>` + regexp.QuoteMeta(item.summary) + `</span>\s*<sup[^>]*>` + regexp.QuoteMeta(item.method) + `</sup>`)
		if !sidebarPattern.MatchString(body) {
			t.Fatalf("sidebar operation %s should associate %q with method %s:\n%s", item.anchor, item.summary, item.method, body)
		}
		endpointBody := renderPublicDocs(t, NewPublicServer(idx), "/?selected="+item.anchor)
		endpointPattern := regexp.MustCompile(`(?s)<section id="` + regexp.QuoteMeta(item.anchor) + `"[^>]*>.*?<div aria-label="Endpoint route"[^>]*>\s*<span[^>]*>` + regexp.QuoteMeta(item.method) + `</span>\s*<p[^>]*>/resource</p>`)
		if !endpointPattern.MatchString(endpointBody) {
			t.Fatalf("endpoint %s should expose method %s and its path in the labelled route group:\n%s", item.anchor, item.method, endpointBody)
		}
	}
}

func TestPublicDocsEndpointResponseExamplesRenderInsideMatchingTabPanel(t *testing.T) {
	idx := core.SpecIndex{
		Title:           "Todos",
		ExampleSpecJSON: `{"components":{"schemas":{"Todo":{"type":"object","properties":{"id":{"type":"string"},"message":{"type":"string"}}}}}}`,
		Operations: []core.Operation{{
			ID:      "getTodo",
			Anchor:  "operation-gettodo",
			Method:  "GET",
			Path:    "/todos/{todoId}",
			Summary: "Get Todo",
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Todo response.",
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema: core.SchemaSummary{
						Name: "Todo",
						Type: "object",
						JSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
					},
					Example:         "{\n  \"id\": \"todo-1\"\n}",
					ExampleProvided: true,
				}},
			}, {
				Status:      "404",
				Description: "Missing todo.",
				MediaTypes: []core.OperationMediaType{{
					ContentType: "application/json",
					Schema: core.SchemaSummary{
						Name: "Error",
						Type: "object",
						JSON: `{"type":"object","required":["message"],"properties":{"message":{"type":"string"}}}`,
					},
					Example:         "{\n  \"message\": \"not found\"\n}",
					ExampleProvided: true,
				}},
			}},
		}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=operation-gettodo")
	if strings.Contains(body, `<aside class="manja-endpoint-examples-rail"`) {
		t.Fatalf("response examples should not create the endpoint examples rail:\n%s", body)
	}
	response200Panel := htmlBetween(t, body, `id="tabpaneloperation-gettodo-responsesresponse-200"`, `id="tabpaneloperation-gettodo-responsesresponse-404"`)
	for _, want := range []string{
		`class="manja-response-media-block border-t border-outline pt-6 pb-5 dark:border-outline-dark"><div class="manja-response-panel-layout">`,
		`class="manja-response-panel-main grid gap-4"`,
		`class="manja-response-panel-example"`,
		`Response Example: 200 application/json`,
		`id="operation-gettodo-response-200-application-json-example"`,
	} {
		if !strings.Contains(response200Panel, want) {
			t.Fatalf("200 response panel missing %q:\n%s", want, response200Panel)
		}
	}
	if strings.Contains(response200Panel, `Response Example: 404 application/json`) {
		t.Fatalf("200 response panel should not include the 404 example:\n%s", response200Panel)
	}
	if strings.Contains(response200Panel, `<section class="manja-response-panel-layout">`) {
		t.Fatalf("response example grid should live inside the media block under its divider:\n%s", response200Panel)
	}
}

func TestPublicDocsEndpointShellCSSUsesResponsiveExamplesRail(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	layoutRule := regexp.MustCompile(`(?s)\.manja-endpoint-shell-layout\s*\{[^}]*\}`)
	rule := layoutRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-endpoint-shell-layout rule")
	}
	for _, want := range []string{
		`display: grid;`,
		`grid-template-columns: minmax(0, 1fr);`,
	} {
		if !strings.Contains(rule, want) {
			t.Fatalf("endpoint shell should stack by default with %q:\n%s", want, rule)
		}
	}
	largeRule := regexp.MustCompile(`(?s)@media\s*\(min-width:\s*1280px\)\s*\{.*?\.manja-endpoint-shell-layout\s*\{[^}]*\}`).FindString(string(css))
	if largeRule == "" {
		t.Fatalf("missing large-screen endpoint shell media rule")
	}
	if !strings.Contains(largeRule, `grid-template-columns: minmax(0, 1fr) minmax(20rem, 28rem);`) {
		t.Fatalf("endpoint shell should split content and examples rail on large screens:\n%s", largeRule)
	}
	railRule := regexp.MustCompile(`(?s)\.manja-endpoint-examples-rail\s*\{[^}]*\}`).FindString(string(css))
	if railRule == "" {
		t.Fatalf("endpoint examples rail should use named CSS instead of breakpoint-only utility classes")
	}
	if !strings.Contains(railRule, `align-self: stretch;`) {
		t.Fatalf("endpoint examples rail should stretch to the endpoint row so sticky examples keep working:\n%s", railRule)
	}
	railContentRule := regexp.MustCompile(`(?s)\.manja-endpoint-examples-rail-content\s*>\s*\*[^}]*\}`).FindString(string(css))
	if railContentRule == "" {
		t.Fatalf("endpoint examples rail content should constrain grid children")
	}
	if !strings.Contains(railContentRule, `min-width: 0;`) || !strings.Contains(railContentRule, `max-width: 100%;`) {
		t.Fatalf("endpoint examples rail children should not overflow their column:\n%s", railContentRule)
	}
}

func TestPublicDocsEndpointResponsePanelCSSUsesResponsiveExampleColumn(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	mediaBlockRule := regexp.MustCompile(`(?s)\.manja-response-media-block\s*\{[^}]*\}`).FindString(string(css))
	if mediaBlockRule == "" {
		t.Fatalf("missing .manja-response-media-block rule")
	}
	if !strings.Contains(mediaBlockRule, `container-type: inline-size;`) {
		t.Fatalf("response media blocks should provide the container width for nested schema/example splits:\n%s", mediaBlockRule)
	}
	layoutRule := regexp.MustCompile(`(?s)\.manja-response-panel-layout\s*\{[^}]*\}`).FindString(string(css))
	if layoutRule == "" {
		t.Fatalf("missing .manja-response-panel-layout rule")
	}
	for _, want := range []string{
		`display: grid;`,
		`grid-template-columns: minmax(0, 1fr);`,
	} {
		if !strings.Contains(layoutRule, want) {
			t.Fatalf("response panel should stack by default with %q:\n%s", want, layoutRule)
		}
	}
	childrenRule := regexp.MustCompile(`(?s)\.manja-response-panel-layout\s*>\s*\*[^}]*\}`).FindString(string(css))
	if childrenRule == "" || !strings.Contains(childrenRule, `min-width: 0;`) {
		t.Fatalf("response panel children should be constrained in their grid columns:\n%s", childrenRule)
	}
	containerRule := regexp.MustCompile(`(?s)@container\s*\(min-width:\s*58rem\)\s*\{[^{}]*\.manja-response-panel-layout\s*\{[^}]*\}`).FindString(string(css))
	if containerRule == "" {
		t.Fatalf("missing response panel container query layout rule")
	}
	if !strings.Contains(containerRule, `grid-template-columns: minmax(24rem, 1fr) minmax(20rem, 28rem);`) {
		t.Fatalf("response panel should split schema and matching example only when the media block is wide enough:\n%s", containerRule)
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
				Example:         "{\n  \"name\": \"fallback\"\n}",
				ExampleProvided: true,
			}}},
			Responses: []core.OperationResponse{{Status: "200", MediaTypes: []core.OperationMediaType{{
				ContentType: "application/json",
				Schema: core.SchemaSummary{
					Name: "Todo",
					Type: "object",
					JSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
				},
				Example:         "{\n  \"id\": \"fallback\"\n}",
				ExampleProvided: true,
			}}}},
		}},
		Schemas: []core.Schema{{Name: "Todo", Description: "A todo.", Example: core.SchemaExample{
			JSON:     `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"name":{"type":"string"}}}`,
			Example:  "{\n  \"id\": \"fallback\"\n}",
			Provided: true,
		}}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/?selected=operation-updatetodo")
	for _, want := range []string{
		`data-manja-example`,
		`data-manja-request-config-body`,
		`id="operation-updatetodo-request-body-input"`,
		`Body`,
		`Response Example: 200 application/json`,
		`type="application/json"`,
		`"hasExplicitExample":true`,
		`"spec":{"components":{"schemas":{"Todo"`,
		`"skipNonRequired":false`,
		`"maxSampleDepth":3`,
		`/manja-assets/schema-example.js`,
		`/manja-assets/request-composer.js`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("generic endpoint example missing %q:\n%s", want, body)
		}
	}

	body = renderPublicDocs(t, NewPublicServer(idx), "/?selected=schema-todo")
	for _, want := range []string{
		`class="manja-schema-detail-layout"`,
		`class="manja-schema-tree-panel"`,
		`aria-label="Schema example"`,
		`class="manja-schema-example-panel"`,
		`id="schema-todo-example"`,
		`Example: Todo`,
		`data-manja-example`,
		`"maxSampleDepth":4`,
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

func TestPublicDocsSchemaDetailCSSUsesResponsiveSplitLayout(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	layoutRule := regexp.MustCompile(`(?s)\.manja-schema-detail-layout\s*\{[^}]*\}`)
	rule := layoutRule.FindString(string(css))
	if rule == "" {
		t.Fatalf("missing .manja-schema-detail-layout rule")
	}
	for _, want := range []string{
		`display: grid;`,
		`grid-template-columns: minmax(0, 1fr);`,
	} {
		if !strings.Contains(rule, want) {
			t.Fatalf("schema detail layout should stack by default with %q:\n%s", want, rule)
		}
	}
	largeRule := regexp.MustCompile(`(?s)@media\s*\(min-width:\s*1280px\)\s*\{[^{}]*\.manja-schema-detail-layout\s*\{[^}]*\}`).FindString(string(css))
	if largeRule == "" {
		t.Fatalf("missing large-screen schema detail layout media rule")
	}
	if !strings.Contains(largeRule, `grid-template-columns: minmax(0, 1fr) minmax(20rem, 28rem);`) {
		t.Fatalf("schema detail layout should split tree and example into two columns on large screens:\n%s", largeRule)
	}
	if !strings.Contains(string(css), `.manja-schema-detail-layout .manja-schema-tree`) {
		t.Fatalf("schema detail layout should reset schema tree top margin so columns align")
	}
}

func renderPublicDocs(t *testing.T, handler http.Handler, targets ...string) string {
	t.Helper()
	target := "/"
	if len(targets) > 0 {
		target = targets[0]
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	return rec.Body.String()
}

func htmlBetween(t *testing.T, body string, start string, end string) string {
	t.Helper()
	startIndex := strings.Index(body, start)
	if startIndex == -1 {
		t.Fatalf("missing start marker %q:\n%s", start, body)
	}
	remainder := body[startIndex:]
	endIndex := strings.Index(remainder, end)
	if endIndex == -1 {
		t.Fatalf("missing end marker %q after %q:\n%s", end, start, remainder)
	}
	return remainder[:endIndex]
}
