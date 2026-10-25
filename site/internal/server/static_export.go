package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	app "github.com/araihu/manja/internal/selfhosted"
	"github.com/araihu/manja/internal/web"
	"github.com/araihu/manja/site/internal/site"
)

const staticHeaders = "/*\n  Cache-Control: public, max-age=0, must-revalidate, no-transform\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n"

// ExportSite renders the landing and setup documentation without starting the
// demo workbench or writing publication state.
func ExportSite(output string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".manja-site-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := exportSiteFiles(stage); err != nil {
		return err
	}
	if err := os.RemoveAll(output); err != nil {
		return err
	}
	return os.Rename(stage, output)
}

func exportSiteFiles(output string) error {
	for name, key := range map[string]site.PageKey{"index.html": site.Home, "docs/index.html": site.Docs} {
		var body strings.Builder
		if err := site.Render(&body, key); err != nil {
			return err
		}
		if err := writeStaticFile(output, name, []byte(body.String())); err != nil {
			return err
		}
	}
	if err := fs.WalkDir(site.StaticFiles, "static", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := fs.ReadFile(site.StaticFiles, name)
		if err != nil {
			return err
		}
		return writeStaticFile(output, name, body)
	}); err != nil {
		return err
	}
	response := httptest.NewRecorder()
	web.NewCatalogAssetsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/manja-social.png", nil))
	if response.Code != http.StatusOK {
		return fmt.Errorf("social image returned HTTP %d", response.Code)
	}
	for name, body := range map[string][]byte{
		"manja-assets/manja-social.png": response.Body.Bytes(),
		"404.html":                      []byte(`<!doctype html><html lang="en"><meta charset="utf-8"><title>Page not found | Manja</title><h1>Page not found</h1><p><a href="/">Return to Manja</a></p></html>`),
		"_headers":                      []byte(staticHeaders),
		"_redirects":                    []byte("/demo " + site.DemoOrigin + "/ 302\n/demo/* " + site.DemoOrigin + "/:splat 302\n"),
	} {
		if err := writeStaticFile(output, name, body); err != nil {
			return err
		}
	}
	return nil
}

// ExportDemo publishes only the synthetic, published demo specs. Git history,
// candidate revisions, credentials and management handlers are never exported.
// The renderer exports and verifies all navigation, fragments and search data.
func ExportDemo(ctx context.Context, output string) error {
	root, err := os.MkdirTemp("", "manja-site-demo-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	var config strings.Builder
	fmt.Fprintf(&config, "version: 1\norganization:\n  title: Manja API demo\n  seo:\n    canonicalBase: %q\ncatalogs:\n", site.DemoOrigin)
	for _, seed := range demoSpecSeeds {
		name := seed.ID + ".yaml"
		if err := os.WriteFile(filepath.Join(root, name), []byte(seed.PublishedSpec), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(&config, "  - id: %q\n    mount: %q\n    title: %q\n    defaultDocument: %q\n    profile: strict-v1\n    seo:\n      canonicalBase: %q\n    source:\n      kind: files\n      root: .\n      include: [%q]\n", seed.ProjectID, seed.PublishedPath, seed.ProjectName, seed.ID, site.DemoOrigin+seed.PublishedPath, name)
	}
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config.String()), 0o600); err != nil {
		return err
	}
	// Keep the verified export separate from Cloudflare's hosting metadata so
	// subsequent exports can verify and reuse the complete original inventory.
	exportRoot := output + "-export"
	_, err = app.ExportRenderer(ctx, app.ExportOptions{
		RendererOptions: app.RendererOptions{ConfigPath: configPath, DataDir: filepath.Join(root, "data")},
		Output:          exportRoot, BasePath: "/",
	})
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".manja-demo-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.CopyFS(stage, os.DirFS(exportRoot)); err != nil {
		return err
	}
	if err := writeStaticFile(stage, "_headers", []byte(staticHeaders)); err != nil {
		return err
	}
	if err := os.RemoveAll(output); err != nil {
		return err
	}
	return os.Rename(stage, output)
}

func writeStaticFile(root, name string, body []byte) error {
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filename, body, 0o644)
}
