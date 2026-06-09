package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/site/internal/server"
)

func TestRoutesRender(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	tests := []struct {
		path string
		want []string
	}{
		{
			path: "/",
			want: []string{
				"Point Manja at your spec.",
				"Source-connected OpenAPI publishing",
				"Versions stay close to source",
				`href="/demo" target="_blank" rel="noopener"`,
				`href="/demo" target="_blank" rel="noopener">View live demo</a>`,
			},
		},
		{
			path: "/demo",
			want: []string{
				"GitHub v3 REST API",
				"Search docs",
				"Operations",
				`href="/demo/?selected=overview#overview"`,
				`href="/demo/manja-assets/manja.css"`,
				`data-search-source-url="/demo/search.json"`,
			},
		},
		{
			path: "/docs",
			want: []string{
				"Setup docs",
				"go run ./cmd/manja",
				"ghcr.io/araihu/manja:main",
				`href="#run-with-docker"`,
				"last-known-good",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			body := get(t, srv.URL+tt.path, http.StatusOK)
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Fatalf("expected %s to contain %q", tt.path, want)
				}
			}
		})
	}
}

func TestStaticAssetsRender(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	css := get(t, srv.URL+"/static/site.css", http.StatusOK)
	if !strings.Contains(css, "--accent: #18d6a7") {
		t.Fatalf("site.css did not include Manja accent token")
	}
	for _, want := range []string{
		".docs-content section {\n  min-width: 0;",
		"overflow-wrap: anywhere;",
		".code-panel {\n  background: #101920;",
		"overflow-x: auto;",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("site.css did not include docs overflow guard %q", want)
		}
	}

	favicon := get(t, srv.URL+"/static/favicon.svg", http.StatusOK)
	if !strings.Contains(favicon, "<svg") {
		t.Fatalf("favicon.svg did not render as svg")
	}

	demoCSS := get(t, srv.URL+"/demo/manja-assets/manja.css", http.StatusOK)
	if !strings.Contains(demoCSS, "--color-surface") {
		t.Fatalf("demo renderer CSS did not render")
	}

	searchJSON := get(t, srv.URL+"/demo/search.json", http.StatusOK)
	if !strings.Contains(searchJSON, `"href":"/demo/?selected=`) {
		t.Fatalf("demo search index did not keep result hrefs under /demo")
	}
}

func TestMissingRoute(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	get(t, srv.URL+"/missing", http.StatusNotFound)
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	return server.NewWithOptions(t.Context(), server.Options{
		SpecPath:  filepath.Join("..", "..", "..", "internal", "adapters", "openapi", "testdata", "github-v3-rest.json"),
		DataDir:   t.TempDir(),
		StaticDir: filepath.Join("..", "..", "..", "internal", "web", "static"),
	})
}

func get(t *testing.T, url string, wantStatus int) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d; body: %s", url, resp.StatusCode, wantStatus, body)
	}
	return string(body)
}
