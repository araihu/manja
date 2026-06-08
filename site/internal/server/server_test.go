package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/araihu/manja/site/internal/server"
)

func TestRoutesRender(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(server.New())
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
			},
		},
		{
			path: "/demo",
			want: []string{
				"GitHub REST API demo",
				"GET",
				"/repos/{owner}/{repo}/teams",
			},
		},
		{
			path: "/docs",
			want: []string{
				"Setup docs",
				"go run ./cmd/manja",
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

	srv := httptest.NewServer(server.New())
	t.Cleanup(srv.Close)

	css := get(t, srv.URL+"/static/site.css", http.StatusOK)
	if !strings.Contains(css, "--accent: #18d6a7") {
		t.Fatalf("site.css did not include Manja accent token")
	}

	favicon := get(t, srv.URL+"/static/favicon.svg", http.StatusOK)
	if !strings.Contains(favicon, "<svg") {
		t.Fatalf("favicon.svg did not render as svg")
	}
}

func TestMissingRoute(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(server.New())
	t.Cleanup(srv.Close)

	get(t, srv.URL+"/missing", http.StatusNotFound)
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
