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
				`src="/static/manja-logo.svg" alt="Manja" width="160" height="40"`,
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
				"Acme Payments API",
				"Search docs",
				"Operations",
				`href="/demo/payments/v1/?selected=overview#overview"`,
				`href="/demo/payments/v1/manja-assets/manja.css"`,
				`data-search-source-url="/demo/payments/v1/search.json"`,
			},
		},
		{
			path: "/demo/manage",
			want: []string{
				"Management",
				"Spec roster",
				"3 specs",
				"Acme Payments API",
				"Acme Identity API",
				"Acme Notifications API",
				"Production docs",
				"Publish candidate",
				"Route settings",
				"Release history",
				"Spec diff",
				"Contract check",
				"6 breaking",
				"Ada Lovelace",
				"Removed endpoint",
				"GET /customers",
				"Required parameter added",
				"api-version (header)",
				"Added schema",
				"Refund",
				`role="tab"`,
				`id="management-specs-table"`,
				`href="/demo/manage/specs"`,
				`hx-post="/demo/manage/publication"`,
				`hx-target="#management-main-content"`,
				`placeholder="/docs/v1"`,
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

func TestDemoManagementRedirectsStayMounted(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	form := strings.NewReader("visibility=public&path=%2Fpayments%2Fv1&request_id=site-demo-request-token")
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/manage/publication", form)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusSeeOther, body)
	}
	if got := resp.Header.Get("Location"); !strings.HasPrefix(got, "/demo/manage/spec/") {
		t.Fatalf("Location = %q, want /demo/manage/spec/...", got)
	}

	body := get(t, srv.URL+"/demo/payments/v1", http.StatusOK)
	if !strings.Contains(body, "Acme Payments API") {
		t.Fatalf("published demo route did not render docs:\n%s", body)
	}
}

func TestDemoManagementHTMXMutationStaysMounted(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	form := strings.NewReader("visibility=public&path=%2Fpayments%2Fv1&request_id=site-demo-request-token")
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/manage/publication", form)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, body)
	}
	if got := resp.Header.Get("HX-Push-Url"); !strings.HasPrefix(got, "/demo/manage/spec/") {
		t.Fatalf("HX-Push-Url = %q, want /demo/manage/spec/...", got)
	}
	bodyText := string(body)
	for _, want := range []string{
		`id="management-main-content"`,
		`href="/demo/payments/v1"`,
		`hx-post="/demo/manage/publication"`,
		`placeholder="/docs/v1"`,
	} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("fragment missing %q:\n%s", want, bodyText)
		}
	}
	for _, reject := range []string{
		`placeholder="/demo/docs/v1"`,
		"<!doctype html>",
	} {
		if strings.Contains(bodyText, reject) {
			t.Fatalf("fragment should not include %q:\n%s", reject, bodyText)
		}
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

	for _, path := range []string{"/static/favicon.svg", "/static/manja-mark.svg", "/static/manja-logo.svg"} {
		asset := get(t, srv.URL+path, http.StatusOK)
		for _, want := range []string{
			`class="araihu-brand-v11"`,
			"--araihu-logo-surface",
			"--araihu-logo-ink",
			"--araihu-logo-signal",
			"@media (prefers-color-scheme: dark)",
		} {
			if !strings.Contains(asset, want) {
				t.Fatalf("%s did not preserve v11 adaptive logo contract %q", path, want)
			}
		}
	}

	demoCSS := get(t, srv.URL+"/demo/payments/v1/manja-assets/manja.css", http.StatusOK)
	if !strings.Contains(demoCSS, "--color-surface") {
		t.Fatalf("demo renderer CSS did not render")
	}

	searchJSON := get(t, srv.URL+"/demo/payments/v1/search.json", http.StatusOK)
	if !strings.Contains(searchJSON, `"href":"/demo/payments/v1/?selected=`) {
		t.Fatalf("demo search index did not keep result hrefs under /demo")
	}
}

func TestDemoManagementShowsPerSpecContractOutcomes(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	payments := get(t, srv.URL+"/demo/manage/spec/payments-api", http.StatusOK)
	for _, want := range []string{
		"Acme Payments API",
		"Publish",
		"Diff",
		"Route",
		"Sync",
		"History",
		"Details",
		"Added schema",
		"Refund",
		`role="tab"`,
	} {
		if !strings.Contains(payments, want) {
			t.Fatalf("payments detail missing %q:\n%s", want, payments)
		}
	}
	if strings.Contains(payments, "and 1 more changes") {
		t.Fatalf("payments detail should render the full diff instead of a hidden-count placeholder:\n%s", payments)
	}

	identity := get(t, srv.URL+"/demo/manage/spec/identity-api", http.StatusOK)
	for _, want := range []string{
		"Acme Identity API",
		"Only additive contract changes",
		"Bruno Dias",
		"Add group directory endpoints",
		"Added endpoint",
		"GET /groups",
		"Response status added",
		"GET /users 206",
	} {
		if !strings.Contains(identity, want) {
			t.Fatalf("identity detail missing %q:\n%s", want, identity)
		}
	}

	notifications := get(t, srv.URL+"/demo/manage/spec/notifications-api", http.StatusOK)
	for _, want := range []string{
		"Acme Notifications API",
		"3 breaking",
		"Carla Mendes",
		"Require delivery templates for message sends",
		"Required parameter added",
		"POST /messages X-Delivery-Policy (header)",
		"Request body became required",
		"Response status removed",
		"POST /messages 400",
	} {
		if !strings.Contains(notifications, want) {
			t.Fatalf("notifications detail missing %q:\n%s", want, notifications)
		}
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
