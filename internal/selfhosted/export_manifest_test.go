package selfhosted

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/web"
)

func TestExportManifestVerifiesExactCanonicalTree(t *testing.T) {
	root := t.TempDir()
	writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
	writeMinimalExport(t, &writer, []byte("<!doctype html><title>Docs</title>"))
	manifest := exportManifest{SchemaVersion: 1, BasePath: "/", Catalogs: []ExportCatalogReceipt{}, Files: writer.sortedEntries()}
	data, err := encodeExportManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.write(exportManifestPath, data, "application/json"); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyExport(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("extra"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyExport(context.Background(), root); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("extra file error = %v", err)
	}
}

func TestExportVerifierAcceptsPriorAssetInventoryForReplacement(t *testing.T) {
	for _, test := range []struct {
		name, mutate string
	}{
		{name: "intact"},
		{name: "changed asset", mutate: "changed"},
		{name: "missing asset", mutate: "missing"},
		{name: "wrong executable media type", mutate: "media type"},
		{name: "missing dependency", mutate: "dependency"},
		{name: "missing runtime core", mutate: "runtime"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
			// A previous renderer can have different asset names and need not
			// contain unused files introduced by the currently installed version.
			for name, body := range map[string]string{
				"index.html":                 `<!doctype html><script src="/assets/previous-runtime.js"></script>`,
				"search/index.html":          `<!doctype html><title>Search</title>`,
				"sw.js":                      "// previous worker",
				"assets/previous-runtime.js": "// previous runtime",
				"manja-assets/local-docs.js": "// previous runtime",
				"manja-assets/local-docs/runtime-assets.js": "// previous asset manifest",
				"manja-assets/local-docs/storage.js":        "// previous storage",
				"manja-assets/local-docs/sw.js":             "// previous runtime worker",
			} {
				if err := writer.write(name, []byte(body), mediaTypeForPath(name)); err != nil {
					t.Fatal(err)
				}
			}
			asset := "assets/previous-runtime.js"
			if test.mutate == "media type" {
				entry := writer.entries[asset]
				entry.MediaType = "text/plain"
				writer.entries[asset] = entry
			}
			if test.mutate == "dependency" || test.mutate == "runtime" {
				removed := asset
				if test.mutate == "runtime" {
					removed = "manja-assets/local-docs/storage.js"
				}
				delete(writer.entries, removed)
				if err := os.Remove(filepath.Join(root, removed)); err != nil {
					t.Fatal(err)
				}
			}
			data, err := encodeExportManifest(exportManifest{SchemaVersion: 1, Rendering: "html", BasePath: "/", Catalogs: []ExportCatalogReceipt{}, Files: writer.sortedEntries()})
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.write(exportManifestPath, data, "application/json"); err != nil {
				t.Fatal(err)
			}
			if test.mutate == "changed" {
				if err := os.WriteFile(filepath.Join(root, asset), []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if test.mutate == "missing" {
				if err := os.Remove(filepath.Join(root, asset)); err != nil {
					t.Fatal(err)
				}
			}
			_, err = VerifyExport(context.Background(), root)
			if test.mutate != "" {
				if err == nil {
					t.Fatal("VerifyExport accepted a broken prior export")
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyExport rejected prior asset inventory: %v", err)
			}
			if replace, err := prepareExportOutput(context.Background(), root); err != nil || !replace {
				t.Fatalf("prepareExportOutput = %t, %v; want verified replacement", replace, err)
			}
		})
	}
}

func TestExportManifestRejectsChangedAndNonCanonicalBytes(t *testing.T) {
	for _, mutate := range []func(string) error{
		func(root string) error {
			return os.WriteFile(filepath.Join(root, "index.html"), []byte("changed"), 0o600)
		},
		func(root string) error {
			name := filepath.Join(root, filepath.FromSlash(exportManifestPath))
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			return os.WriteFile(name, append([]byte(" "), data...), 0o600)
		},
	} {
		root := t.TempDir()
		writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
		writeMinimalExport(t, &writer, []byte("original"))
		data, err := encodeExportManifest(exportManifest{SchemaVersion: 1, BasePath: "/", Catalogs: []ExportCatalogReceipt{}, Files: writer.sortedEntries()})
		if err != nil {
			t.Fatal(err)
		}
		if err := writer.write(exportManifestPath, data, "application/json"); err != nil {
			t.Fatal(err)
		}
		if err := mutate(root); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyExport(context.Background(), root); err == nil {
			t.Fatal("VerifyExport succeeded after mutation")
		}
	}
}

func TestExportVerifierRejectsBrokenAndRuntimeOnlyHTMLReferences(t *testing.T) {
	for _, test := range []struct {
		name, base, body string
	}{
		{name: "root missing", base: "/", body: `<a href="/missing/">missing</a>`},
		{name: "subpath escape", base: "/group/project/", body: `<script src="/manja-assets/local-docs.js"></script>`},
		{name: "table row subpath escape", base: "/group/project/", body: `<div data-table-row-link="/documents/openapi/"></div>`},
		{name: "search subpath escape", base: "/group/project/", body: `<div data-catalog-search-href="/search"></div>`},
		{name: "embedded visit subpath escape", base: "/group/project/", body: `<script id="catalog-search-current-visit" type="application/json">{"href":"/documents/openapi/"}</script>`},
		{name: "runtime route", base: "/", body: `<div hx-get="/api/private"></div>`},
		{name: "post", base: "/", body: `<button hx-post="/">post</button>`},
		{name: "data get", base: "/", body: `<a data-hx-get="/">get</a>`},
		{name: "boost", base: "/", body: `<div hx-boost="true"></div>`},
		{name: "unhandled get form", base: "/", body: `<form method="get" action="/"></form>`},
		{name: "mutation form", base: "/", body: `<form method="POST" action="/"></form>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
			writeMinimalExport(t, &writer, []byte("<!doctype html>"+test.body))
			data, err := encodeExportManifest(exportManifest{SchemaVersion: 1, BasePath: test.base, Catalogs: []ExportCatalogReceipt{}, Files: writer.sortedEntries()})
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.write(exportManifestPath, data, "application/json"); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyExport(context.Background(), root); err == nil {
				t.Fatal("VerifyExport admitted invalid HTML reference")
			}
		})
	}
}

func TestExportVerifierRejectsWrongExecutableMediaType(t *testing.T) {
	root := t.TempDir()
	writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
	writeMinimalExport(t, &writer, []byte("<!doctype html><title>Docs</title>"))
	entry := writer.entries["sw.js"]
	entry.MediaType = "text/plain"
	writer.entries["sw.js"] = entry
	data, err := encodeExportManifest(exportManifest{SchemaVersion: 1, BasePath: "/", Catalogs: []ExportCatalogReceipt{}, Files: writer.sortedEntries()})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.write(exportManifestPath, data, "application/json"); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyExport(context.Background(), root); err == nil || !strings.Contains(err.Error(), "media type") {
		t.Fatalf("VerifyExport error = %v", err)
	}
}

func writeMinimalExport(t *testing.T, writer *exportTreeWriter, index []byte) {
	t.Helper()
	files := append([]string{"index.html", "search/index.html", "sw.js"}, trimExportAssetPaths(web.CatalogAssetPaths())...)
	for _, name := range files {
		data := []byte("asset")
		if name == "index.html" || name == "search/index.html" {
			data = index
		}
		if err := writer.write(name, data, mediaTypeForPath(name)); err != nil {
			t.Fatal(err)
		}
	}
}
