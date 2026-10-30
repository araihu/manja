//go:build !manja_runtime

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

func TestExportOrganizationBrandAndCatalogIcons(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "images")
	if err := os.Mkdir(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	image := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><path fill="currentColor" d="M0 0h32v32H0z"/></svg>`
	for _, name := range []string{"org.svg", "project.svg"} {
		if err := os.WriteFile(filepath.Join(assets, name), []byte(image), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	spec := filepath.Join(root, "specs")
	if err := os.Mkdir(spec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spec, "api.json"), []byte(`{"openapi":"3.0.3","info":{"title":"Project API","version":"1.0.0"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := fmt.Sprintf(`version: 1
assetDir: %q
organization:
  title: Example APIs
  branding:
    displayName: Example Org
    logo: {src: /brand-assets/org.svg, alt: Example Org, homeUrl: /}
    favicon: /brand-assets/org.svg
catalogs:
  - id: project
    mount: /catalogs/project
    title: Project
    profile: strict-v1
    branding:
      displayName: Project
      logo: {src: /brand-assets/project.svg, alt: Project}
    source:
      kind: files
      root: %q
      include: [api.json]
`, assets, spec)
	config := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(config, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	options := RendererOptions{ConfigPath: config, DataDir: filepath.Join(root, "data")}
	output := filepath.Join(root, "public")
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: options, Output: output, BasePath: "/docs/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyExport(context.Background(), output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"org.svg", "project.svg"} {
		data, err := os.ReadFile(filepath.Join(output, "brand-assets", name))
		if err != nil || string(data) != image {
			t.Fatalf("image %s = %q, %v", name, data, err)
		}
	}
	for _, page := range []string{"index.html", "catalogs/project/documents/api/index.html"} {
		data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(page)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `aria-label="Example Org"`) || !strings.Contains(string(data), `/docs/brand-assets/org.svg`) {
			t.Fatalf("organization brand missing from %s", page)
		}
		if page == "index.html" && strings.Count(string(data), `/docs/brand-assets/project.svg`) < 2 {
			t.Fatal("catalog icon was not inherited by its spec navigation item")
		}
	}
	handler, _, err := NewRecoveredRenderer(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	for route, status := range map[string]int{"/brand-assets/org.svg": http.StatusOK, "/brand-assets/private.svg": http.StatusNotFound} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != status {
			t.Fatalf("GET %s = %d", route, response.Code)
		}
	}
}

func TestRendererBrandAssetsRejectSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "private.svg")); err != nil {
		t.Fatal(err)
	}
	if _, err := withRendererBrandAssets(context.Background(), root, http.NotFoundHandler()); err == nil {
		t.Fatal("branding assets followed a symlink")
	}
}
