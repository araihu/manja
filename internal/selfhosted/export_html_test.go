package selfhosted

import (
	"strings"
	"testing"

	"github.com/araihu/manja/internal/localdocs"
)

func TestRewriteExportHTMLPrefixesSubpathInjectsDescriptorAndRemovesRuntimeRoutes(t *testing.T) {
	descriptor := localdocs.DescriptorV1{SchemaVersion: 1, CatalogID: "payments", PublicationKey: "payments", Public: true, Anonymous: true, PublicationBase: "/group/project/payments/", Static: &localdocs.StaticDescriptorV1{DeploymentBase: "/group/project/"}}
	input := `<!doctype html><html><head><link rel="stylesheet" href="/assets/app.css"><script src="/manja-assets/catalog-search.js"></script></head><body><a href="/">Catalogs</a><a href="/payments/catalog.json">Catalog</a><a href="/payments/openapi/core.json">Source</a><a href="https://example.test/repo">Repo</a><div data-manja-copy-page><a href="/payments/documents/core/page.md?selected=x">Markdown</a></div><div hx-get="/payments/documents/core/?selected=x" data-search-fallback-url="/payments/search.json" data-search-child-base="/payments/snapshots/s/search-data/"></div></body></html>`
	output, err := rewriteExportHTML([]byte(input), "/group/project/", &exportHTMLCatalog{Mount: "/payments", SnapshotID: "snapshot", Descriptor: descriptor})
	if err != nil {
		t.Fatal(err)
	}
	body := string(output)
	for _, want := range []string{
		`href="/group/project/assets/app.css"`, `src="/group/project/manja-assets/catalog-search.js"`, `href="/group/project/"`,
		`href="/group/project/payments/snapshots/snapshot/catalog.json"`, `href="/group/project/payments/snapshots/snapshot/openapi/core.json"`,
		`href="https://example.test/repo"`, `data-search-child-base="/group/project/payments/snapshots/s/search-data/"`,
		`id="manja-local-docs-descriptor"`, `src="/group/project/manja-assets/local-docs.js"`, `"deploymentBase":"/group/project/"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("output missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"data-manja-copy-page", "hx-get", "data-search-fallback-url", "page.md"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("output contains %q:\n%s", unwanted, body)
		}
	}
}

func TestRewriteExportHTMLRejectsExternalResources(t *testing.T) {
	for _, input := range []string{
		`<html><head><script src="https://cdn.example/app.js"></script></head></html>`,
		`<html><head></head><body><img src="//cdn.example/logo.svg"></body></html>`,
		`<html><head></head><body><a href="http://example.test">insecure</a></body></html>`,
	} {
		if _, err := rewriteExportHTML([]byte(input), "/", nil); err == nil {
			t.Fatalf("rewriteExportHTML accepted %s", input)
		}
	}
}

func TestRewriteExportHTMLPinsRuntimeDependenciesToLocalSubpath(t *testing.T) {
	input := `<html><head><script src="/assets/js/dependency-loader.js" data-goshtoso-dependencies='{"dependencies":[{"primary_url":"https://cdn.example/runtime.js","fallback_url":"/assets/js/runtime.js"},{"primary_url":"/assets/js/goshtoso.min.js","fallback_url":"/assets/js/goshtoso.min.js"}]}'></script></head></html>`
	output, err := rewriteExportHTML([]byte(input), "/group/project/", nil)
	if err != nil {
		t.Fatal(err)
	}
	body := string(output)
	if strings.Contains(body, "cdn.example") || !strings.Contains(body, `/group/project/assets/js/runtime.js`) || !strings.Contains(body, `/group/project/assets/js/goshtoso.min.js`) {
		t.Fatalf("runtime dependencies were not pinned locally: %s", body)
	}
}
