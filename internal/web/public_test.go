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
		Title:      "Petstore",
		Version:    "1.0.0",
		Operations: []core.Operation{{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}}},
		Search:     []core.SearchDocument{{ID: "operation-listPets", Title: "GET /pets", Description: "List pets", Href: "#operation-listPets", Kind: "Operation", Section: "Pets"}},
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
