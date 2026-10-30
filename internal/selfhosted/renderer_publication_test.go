//go:build !manja_runtime

package selfhosted

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
	"github.com/mxschmitt/playwright-go"
)

func TestPublicationMetadataSurvivesExportRecoveryAndChangesIdentity(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	spec := `{"openapi":"3.0.3","info":{"title":"Internal API","version":"0.2.0"},"paths":{"/audit":{"get":{"operationId":"listAudit","responses":{"200":{"description":"ok"}}}}}}`
	mappings := []map[string]any{}
	for _, channel := range []string{"next", "release"} {
		p := domain.CatalogPublication{API: "internal", Audience: "internal", Channel: channel, Ref: "refs/heads/main", Commit: strings.Repeat("a", 40), SourcePath: "api/internal.yaml"}
		if channel == "release" {
			p.Version = "0.2.0"
			p.Latest = true
			p.Ref = "refs/tags/openapi/internal/v0.2.0"
			p.Commit = strings.Repeat("b", 40)
		}
		name := channel + ".json"
		if err := os.WriteFile(filepath.Join(source, name), []byte(spec), 0600); err != nil {
			t.Fatal(err)
		}
		mappings = append(mappings, map[string]any{"path": name, "key": channel, "publication": p})
	}
	config := map[string]any{"version": 1, "catalogs": []any{map[string]any{"id": "example", "mount": "/example", "title": "Example", "profile": "strict-v1", "source": map[string]any{"kind": "files", "root": source, "include": []string{"*.json"}, "documents": mappings}}}}
	configPath := filepath.Join(root, "renderer.yaml")
	write := func() {
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	options := RendererOptions{ConfigPath: configPath, DataDir: filepath.Join(root, "data")}
	output := filepath.Join(root, "public")
	receipt, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: options, Output: output, BasePath: "/reference/"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyExport(context.Background(), output); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(output, "example/documents/next/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"data-publication-context", "next · development · API 0.2.0", `href="/reference/example/documents/release/"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("exported next page missing %q", want)
		}
	}
	for _, unwanted := range []string{"Publication source", "Upstream ref", "Catalog build revision", "data-catalog-provenance", "Active snapshot"} {
		if strings.Contains(string(page), unwanted) {
			t.Errorf("public document rendered management metadata %q", unwanted)
		}
	}
	directory, err := os.ReadFile(filepath.Join(output, "example/snapshots", string(receipt.Catalogs[0].SnapshotID), "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"refs/heads/main", strings.Repeat("a", 40), "api/internal.yaml"} {
		if !strings.Contains(string(directory), want) {
			t.Errorf("catalog lost publication metadata %q", want)
		}
	}
	search, err := os.ReadFile(filepath.Join(output, "_manja/search/directory.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"next · development", "release 0.2.0 · latest stable"} {
		if !strings.Contains(string(search), want) {
			t.Errorf("deployment search missing %q", want)
		}
	}
	if !testing.Short() {
		server := httptest.NewServer(http.StripPrefix("/reference", http.FileServer(http.Dir(output))))
		defer server.Close()
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
		if err = page.SetViewportSize(390, 844); err != nil {
			t.Fatal(err)
		}
		if err = page.Route("https://**", func(route playwright.Route) { _ = route.Abort() }); err != nil {
			t.Fatal(err)
		}
		if _, err = page.Goto(server.URL + "/reference/example/documents/next/"); err != nil {
			t.Fatal(err)
		}
		waitStaticExportReady(t, page)
		if err = page.Locator("#publication-version-trigger").Click(); err != nil {
			t.Fatal(err)
		}
		if err = page.GetByRole("option", playwright.PageGetByRoleOptions{Name: "release 0.2.0 · latest stable", Exact: playwright.Bool(true)}).Click(); err != nil {
			t.Fatal(err)
		}
		if _, err = page.WaitForFunction(`() => location.pathname.endsWith('/release/')`, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = page.GoBack(); err != nil {
			t.Fatal(err)
		}
		if _, err = page.WaitForFunction(`() => location.pathname.endsWith('/next/') && document.querySelector('#publication-version-trigger').textContent.includes('next')`, nil); err != nil {
			t.Fatal(err)
		}
		waitStaticExportReady(t, page)
		operationURL, err := page.Locator("[data-catalog-sidebar-operation]").First().Evaluate(`element => element.href`, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("opening operation %s", operationURL)
		if _, err = page.Goto(operationURL.(string)); err != nil {
			t.Fatal(err)
		}
		waitStaticExportReady(t, page)
		assertContext := func() {
			t.Helper()
			if _, err := page.WaitForFunction(`() => document.querySelector('[data-public-doc-identity]') && document.querySelector('[data-publication-back]') && document.querySelector('header [data-publication-controls] #publication-version-trigger') && !document.querySelector('[data-catalog-main-content] [data-publication-picker]') && document.title.includes('next · development')`, nil); err != nil {
				t.Fatal(err)
			}
		}
		assertContext()
		if _, err = page.Reload(); err != nil {
			t.Fatal(err)
		}
		waitStaticExportReady(t, page)
		assertContext()
	}
	handler, recovered, err := NewRecoveredRenderer(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/example/documents/release/", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "release 0.2.0 · latest stable") {
		t.Fatal("recovered runtime lost publication identity")
	}
	p := mappings[0]["publication"].(domain.CatalogPublication)
	p.Commit = strings.Repeat("c", 40)
	mappings[0]["publication"] = p
	write()
	_, changed, err := NewRenderer(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if changed[0].SnapshotID == recovered[0].SnapshotID || changed[0].SnapshotID == receipt.Catalogs[0].SnapshotID {
		t.Fatal("upstream metadata change reused the previous snapshot")
	}
}
