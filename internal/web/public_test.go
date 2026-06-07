package web

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		`href="#operation-listPets"`,
		`href="#schema-pet"`,
		`id="main-content"`,
		`aria-label="On this page"`,
		`Operations`,
		`Schemas`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("docs shell missing %q:\n%s", want, body)
		}
	}
	sidebarMethodBadge := regexp.MustCompile(`<a href="#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*ml-auto shrink-0[^"]*border-info[^"]*bg-info[^"]*text-on-info[^"]*"[^>]*>GET</sup>`)
	if !sidebarMethodBadge.MatchString(body) {
		t.Fatalf("operation sidebar item should render flex path label with right-aligned Goshtoso method badge:\n%s", body)
	}
	postMethodBadge := regexp.MustCompile(`<a href="#operation-createPet"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*border-success[^"]*bg-success[^"]*text-on-success[^"]*"[^>]*>POST</sup>`)
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
	sidebarTagGroup := regexp.MustCompile(`<div data-sidebar-section="Operations">.*<a href="#operations-heading"[^>]*><span class="min-w-0 flex-1 truncate">Pets</span>.*<div class="ml-4 flex flex-col">.*<a href="#operation-listPets"[^>]*><span class="min-w-0 flex-1 truncate">/pets</span>\s*<sup[^>]*>GET</sup>`)
	if !sidebarTagGroup.MatchString(body) {
		t.Fatalf("operation sidebar items should use Penguin-style tag sub-items:\n%s", body)
	}
	tagWithoutRail := regexp.MustCompile(`<a href="#operations-heading" class="flex items-center gap-2 py-2\.5 pl-4[^"]*"><span class="min-w-0 flex-1 truncate">Pets</span>`)
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
