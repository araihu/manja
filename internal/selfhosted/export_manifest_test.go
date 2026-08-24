package selfhosted

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportManifestVerifiesExactCanonicalTree(t *testing.T) {
	root := t.TempDir()
	writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
	if err := writer.write("index.html", []byte("<!doctype html><title>Docs</title>"), "text/html"); err != nil {
		t.Fatal(err)
	}
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
		if err := writer.write("index.html", []byte("original"), "text/html"); err != nil {
			t.Fatal(err)
		}
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
