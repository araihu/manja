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
		{name: "runtime route", base: "/", body: `<div hx-get="/api/private"></div>`},
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
