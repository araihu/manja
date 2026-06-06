package e2e

import (
	"context"
	"net"
	"net/http"
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
	if got := page.URL(); got != server+"/#operation-listpets" {
		t.Fatalf("page URL = %q, want %q", got, server+"/#operation-listpets")
	}
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
