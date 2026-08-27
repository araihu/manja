package projection

import (
	"context"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestAnchorValidationAndRouteResolution(t *testing.T) {
	accepted := []string{"operation-list/pets", "operation.list~pets", "operation-list_pets"}
	for _, anchor := range accepted {
		input := minimalIndex()
		input.Operations = []domain.Operation{{Anchor: anchor, Method: "GET", Path: "/"}}
		if _, err := (Builder{}).Build(context.Background(), input); err != nil {
			t.Errorf("anchor %q rejected: %v", anchor, err)
		}
	}

	rejected := []string{"operation-list%2Fpets", "operation#list", "operation list", "opération-list", "operation\\list", "overview", "main-content", "operations-heading", "schemas-heading"}
	for _, anchor := range rejected {
		input := minimalIndex()
		input.Operations = []domain.Operation{{Anchor: anchor, Method: "GET", Path: "/"}}
		if _, err := (Builder{}).Build(context.Background(), input); err == nil {
			t.Errorf("anchor %q accepted", anchor)
		}
	}
}

func TestAnchorRejectsInvalidSearchAndPublicRoutes(t *testing.T) {
	badSearch := []string{"", "missing", "https://example.com/#overview", "//example.com/#overview", "?selected=wrong#overview", "#unknown"}
	for _, href := range badSearch {
		input := minimalIndex()
		input.Search = []domain.SearchDocument{{ID: "search", Kind: "overview", Href: href}}
		if _, err := (Builder{}).Build(context.Background(), input); err == nil {
			t.Errorf("search href %q accepted", href)
		}
	}

	badRoutes := []string{"?selected=overview#overview", "https://example.com/?selected=overview#overview", "//example.com/?selected=overview#overview", "/a/../?selected=overview#overview", "/\\bad?selected=overview#overview", "/?selected=overview&extra=1#overview", "/#overview", "/?selected=overview"}
	for _, path := range badRoutes {
		input := minimalIndex()
		input.PublicRoutes = []domain.PublicRoute{{Path: path, Title: "bad"}}
		if _, err := (Builder{}).Build(context.Background(), input); err == nil {
			t.Errorf("public route %q accepted", path)
		}
	}
}

func TestAnchorGeneratedNamespacesDoNotCollide(t *testing.T) {
	input := minimalIndex()
	input.Operations = []domain.Operation{{Anchor: "op", Method: "GET", Path: "/", Tags: []string{"Pets"}}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	sectionID := document.SidebarSections[0].ID
	if !strings.HasPrefix(sectionID, "operation-tag-") || len(sectionID) != len("operation-tag-")+64 {
		t.Fatalf("section ID = %q", sectionID)
	}

	input.Operations[0].Anchor = sectionID
	if _, err := (Builder{}).Build(context.Background(), input); err == nil {
		t.Fatal("operation anchor colliding with section ID accepted")
	}

	input = minimalIndex()
	input.Search = []domain.SearchDocument{{ID: "overview", Kind: "overview", Href: "#overview"}}
	document, err = (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if document.Search[0].ResultID == "overview" || !strings.HasPrefix(document.Search[0].ResultID, "search-result-") {
		t.Fatalf("search result ID = %q", document.Search[0].ResultID)
	}
}
