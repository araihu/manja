package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	configadapter "github.com/araihu/manja/internal/adapters/config"
	app "github.com/araihu/manja/internal/selfhosted"
	"github.com/araihu/manja/internal/web"
	"github.com/araihu/manja/site/internal/site"
	"gopkg.in/yaml.v3"
)

// Keep integrity-checked HTML, manifests, fragments and the root service worker
// revalidated and protected from edge rewriting. Only browser asset directories
// get a bounded fresh lifetime and allow transport compression. Their stable
// URLs can change between builds, so they must not use immutable caching.
// Detach the inherited header first: Cloudflare joins matching header values.
const staticHeaders = `/*
  Cache-Control: public, max-age=0, must-revalidate, no-transform
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/static/*
  ! Cache-Control
  Cache-Control: public, max-age=3600, must-revalidate

/assets/*
  ! Cache-Control
  Cache-Control: public, max-age=3600, must-revalidate

/manja-assets/*
  ! Cache-Control
  Cache-Control: public, max-age=3600, must-revalidate

/manja-assets/local-docs/sw.js
  ! Cache-Control
  Cache-Control: public, max-age=0, must-revalidate, no-transform

/manja-assets/local-docs/runtime-assets.js
  ! Cache-Control
  Cache-Control: public, max-age=0, must-revalidate, no-transform
`

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

// ExportDemo publishes the pinned GitHub, Stripe and Twilio demo catalogs.
func ExportDemo(ctx context.Context, output string) error {
	configPath := firstExistingPath(
		"demo/renderer.yaml",
		"site/demo/renderer.yaml",
		"../../demo/renderer.yaml",
	)
	return exportDemo(ctx, output, configPath, func(event app.ExportProgressEvent) {
		fmt.Fprintln(os.Stderr, app.FormatExportProgress(event))
	})
}

// ExportDemoFromConfig supports bounded fixtures and alternate local catalogs.
func ExportDemoFromConfig(ctx context.Context, output, configPath string) error {
	return exportDemo(ctx, output, configPath, nil)
}

func exportDemo(ctx context.Context, output, sourceConfigPath string, progress app.ExportProgress) error {
	root, err := os.MkdirTemp("", "manja-site-demo-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	configPath, err := writeDemoConfig(sourceConfigPath, root)
	if err != nil {
		return err
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	// Renderer state retains previously activated catalogs. Separate it by
	// configuration so removing a demo catalog also removes it from navigation
	// and global search, while repeated builds can reuse the same snapshots.
	dataDir := filepath.Join(output+"-data", fmt.Sprintf("%x", sha256.Sum256(configBytes)))
	// Keep the verified export separate from Cloudflare's hosting metadata so
	// subsequent exports can verify and reuse the complete original inventory.
	exportRoot := output + "-export"
	_, err = app.ExportRenderer(ctx, app.ExportOptions{
		RendererOptions: app.RendererOptions{ConfigPath: configPath, DataDir: dataDir},
		Output:          exportRoot, BasePath: "/", Progress: progress,
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

func writeDemoConfig(sourcePath, outputDir string) (string, error) {
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", err
	}
	config, err := configadapter.LoadRenderer(sourcePath)
	if err != nil {
		return "", err
	}
	sourceDir := filepath.Dir(sourcePath)
	resolve := func(name string) string {
		if name == "" || filepath.IsAbs(name) {
			return name
		}
		return filepath.Join(sourceDir, name)
	}
	updateSEO := func(seo *configadapter.RendererSEOConfig) {
		seo.CanonicalBase = strings.ReplaceAll(seo.CanonicalBase, "https://manja.araihu.com", site.DemoOrigin)
		seo.SocialImage = strings.ReplaceAll(seo.SocialImage, "https://manja.araihu.com", site.DemoOrigin)
	}
	updateSEO(&config.Organization.SEO)
	for i := range config.Catalogs {
		catalog := &config.Catalogs[i]
		updateSEO(&catalog.SEO)
		catalog.CompatibilityAllowlist = resolve(catalog.CompatibilityAllowlist)
		if catalog.Source.Kind == configadapter.RendererSourceFiles {
			catalog.Source.Root = resolve(catalog.Source.Root)
		}
		if catalog.Source.IntegrityReceipt != nil {
			name := *catalog.Source.IntegrityReceipt
			data, err := os.ReadFile(resolve(name))
			if err != nil {
				return "", err
			}
			name = filepath.Base(name)
			if err := os.WriteFile(filepath.Join(outputDir, name), data, 0o600); err != nil {
				return "", err
			}
			catalog.Source.IntegrityReceipt = &name
		}
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}
	path := filepath.Join(outputDir, "renderer.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func writeStaticFile(root, name string, body []byte) error {
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filename, body, 0o644)
}
