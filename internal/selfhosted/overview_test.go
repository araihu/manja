package selfhosted

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const overviewReadme = "## Getting started\n\nBrowse [documentation](https://example.com/docs/).\n\n- **Install** the client\n- Browse the API\n\nPlain text & more.\n\n<script>alert(1)</script><img src=x onerror=alert(2)>\n\n[unsafe](javascript:alert(3)) [data](data:text/html,unsafe)"

func overviewRendererConfig(t *testing.T, readme string) string {
	t.Helper()
	root := t.TempDir()
	spec := `{"openapi":"3.0.3","info":{"title":"Example API","version":"v1"},"paths":{"/ping":{"get":{"operationId":"ping","summary":"Ping","responses":{"200":{"description":"OK"}}}}}}`
	if err := os.WriteFile(filepath.Join(root, "example.json"), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`version: 1
dataDir: data
organization:
  title: Example APIs
  readme: |-
    %s
catalogs:
  - id: example
    mount: /example
    title: Example
    readme: |-
      %s
    defaultDocument: example
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [example.json]
`, strings.ReplaceAll(readme, "\n", "\n    "), strings.ReplaceAll(readme, "\n", "\n      "))
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func TestOverviewReadmeMarkdownInHostedAndStaticRendering(t *testing.T) {
	configPath := overviewRendererConfig(t, overviewReadme)
	handler, _, err := NewRenderer(context.Background(), RendererOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "public")
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath, DataDir: t.TempDir()}, Output: output, BasePath: "/reference/"}); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/", "/example/"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", route, response.Code)
		}
		static, err := os.ReadFile(filepath.Join(output, route, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		for mode, body := range map[string]string{"hosted": response.Body.String(), "static": string(static)} {
			for _, want := range []string{`class="manja-markdown"`, `href="https://example.com/docs/"`, "Getting started</h2>", "<ul>", "<strong>Install</strong>", "<p>Plain text &amp; more.</p>"} {
				if !strings.Contains(body, want) {
					t.Errorf("%s %s missing %q", mode, route, want)
				}
			}
			for _, unsafe := range []string{"<script>alert", "<img src=x", "onerror=", "javascript:alert", "data:text/html,unsafe"} {
				if strings.Contains(body, unsafe) {
					t.Errorf("%s %s contains unsafe README markup %q", mode, route, unsafe)
				}
			}
		}
	}
}
