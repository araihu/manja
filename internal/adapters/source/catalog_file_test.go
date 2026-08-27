package source

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestCatalogFileSourceLoadsSortedStableManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCatalogFile(t, root, "z.json", `{"openapi":"3.0.0","info":{"title":"Z","version":"1"},"paths":{}}`)
	writeCatalogFile(t, root, "a.json", `{"openapi":"3.0.0","info":{"title":"A","version":"1"},"paths":{},"components":{"schemas":{"Thing":{"$ref":"./common.yaml#/Thing"}}}}`)
	writeCatalogFile(t, root, "common.yaml", "Thing:\n  type: string\n")

	source := FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "*.json")}
	candidate, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Documents) != 2 || candidate.Documents[0].Key != "a" || candidate.Documents[1].Key != "z" {
		t.Fatalf("documents = %#v", candidate.Documents)
	}
	if len(candidate.SupportFiles) != 1 || candidate.SupportFiles[0].SourcePath != "common.yaml" || string(candidate.SupportFiles[0].Bytes) != "Thing:\n  type: string\n" {
		t.Fatalf("support files = %#v", candidate.SupportFiles)
	}
	if candidate.Revision.Kind != domain.CatalogRevisionFiles || !strings.HasPrefix(candidate.Revision.ID, "files-sha256-") || len(candidate.Revision.ManifestDigest) != 64 {
		t.Fatalf("revision = %#v", candidate.Revision)
	}
	again, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Revision != again.Revision || !bytes.Equal(candidate.Documents[0].Bytes, again.Documents[0].Bytes) {
		t.Fatalf("stable reload changed candidate: %#v / %#v", candidate.Revision, again.Revision)
	}
}

func TestCatalogFileSourceReservesRecursiveSupportBytesBeforeRead(t *testing.T) {
	root := t.TempDir()
	writeCatalogFile(t, root, "openapi.json", `{"openapi":"3.0.3","info":{"title":"Support budget","version":"v1"},"paths":{},"components":{"schemas":{"Root":{"$ref":"support-00.json"}}}}`)
	for index := 0; index < 3; index++ {
		ref := ""
		if index < 2 {
			ref = `"$ref":"support-0` + string(rune('1'+index)) + `.json",`
		}
		prefix := []byte(`{` + ref + `"padding":"`)
		suffix := []byte(`"}`)
		data := make([]byte, 0, maxCatalogSourceFileBytes-1024)
		data = append(data, prefix...)
		data = append(data, bytes.Repeat([]byte{'a'}, maxCatalogSourceFileBytes-1024-len(prefix)-len(suffix))...)
		data = append(data, suffix...)
		writeCatalogBytes(t, root, "support-0"+string(rune('0'+index))+".json", data)
	}

	var reads []string
	_, err := (FileCatalogSource{
		Root: root,
		Manifest: CatalogManifest{
			ID: "catalog", Title: "Catalog", ProfileID: domain.CompatibilityProfileKubernetes, Includes: []string{"openapi.json"},
		},
		beforeFileRead: func(path string) { reads = append(reads, path) },
	}).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "catalog source bytes") {
		t.Fatalf("recursive support aggregate error = %v", err)
	}
	for _, read := range reads {
		if read == "support-02.json" {
			t.Fatalf("aggregate-crossing recursive support was read: %v", reads)
		}
	}
}

func TestCatalogFileSourceRejectsUnsafeSelection(t *testing.T) {
	t.Parallel()

	for name, configure := range map[string]func(*testing.T, string) FileCatalogSource{
		"zero matches": func(t *testing.T, root string) FileCatalogSource {
			return FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "*.json")}
		},
		"absolute include": func(t *testing.T, root string) FileCatalogSource {
			return FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "/openapi.json")}
		},
		"include escape": func(t *testing.T, root string) FileCatalogSource {
			return FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "../*.json")}
		},
		"symlink escape": func(t *testing.T, root string) FileCatalogSource {
			outside := filepath.Join(t.TempDir(), "outside.json")
			if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "openapi.json")); err != nil {
				t.Fatal(err)
			}
			return FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "*.json")}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if _, err := configure(t, root).Load(context.Background()); err == nil {
				t.Fatal("unsafe file catalog selection was accepted")
			}
		})
	}
}

func TestCatalogFileSourceRejectsChangesBetweenStablePasses(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*testing.T, string){
		"same-size bytes": func(t *testing.T, root string) { writeCatalogFile(t, root, "one.json", `{"mutate":2}`) },
		"addition":        func(t *testing.T, root string) { writeCatalogFile(t, root, "two.json", `{}`) },
		"removal": func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "one.json")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeCatalogFile(t, root, "one.json", `{"stable":1}`)
			source := FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "*.json")}
			source.afterFirstPass = func() { mutate(t, root) }
			if _, err := source.Load(context.Background()); err == nil {
				t.Fatal("unstable file catalog was accepted")
			}
		})
	}
}

func TestCatalogFileSourceRejectsGrowthDuringBoundedRead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCatalogFile(t, root, "openapi.json", `{"openapi":"3.0.3","info":{"title":"Stable","version":"v1"},"paths":{}}`)
	source := FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "*.json")}
	source.beforeFileRead = func(sourcePath string) {
		if sourcePath != "openapi.json" {
			return
		}
		file, err := os.OpenFile(filepath.Join(root, sourcePath), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(" "); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := source.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "changed while reading") {
		t.Fatalf("growing file error = %v", err)
	}
}

func TestCatalogFileSourceEnforcesDocumentAndKubernetesAggregateLimits(t *testing.T) {
	t.Parallel()

	t.Run("document", func(t *testing.T) {
		root := t.TempDir()
		writeCatalogBytes(t, root, "huge.json", validLargeJSON((8<<20)+1))
		if _, err := (FileCatalogSource{Root: root, Manifest: testCatalogManifest("strict-v1", "*.json")}).Load(context.Background()); err == nil {
			t.Fatal("oversized source document was accepted")
		}
	})

	t.Run("Kubernetes aggregate", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"one.json", "two.json", "three.json"} {
			writeCatalogBytes(t, root, name, validLargeJSON(6<<20))
		}
		if _, err := (FileCatalogSource{Root: root, Manifest: testCatalogManifest("kubernetes-v3-v1", "*.json")}).Load(context.Background()); err == nil {
			t.Fatal("oversized Kubernetes source group was accepted")
		}
	})
}

func TestCatalogFileSourceLoadsCompleteLockedKubernetesManifest(t *testing.T) {
	root := filepath.Join("..", "..", "renderer", "testdata", "kubernetes")
	authorityBytes, err := os.ReadFile(filepath.Join(root, "catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var authority struct {
		Documents []struct {
			Key          string `json:"key"`
			UpstreamPath string `json:"upstreamPath"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(authorityBytes, &authority); err != nil {
		t.Fatal(err)
	}
	keys := make([]CatalogDocumentKey, len(authority.Documents))
	for index, document := range authority.Documents {
		keys[index] = CatalogDocumentKey{SourcePath: filepath.Base(document.UpstreamPath), Key: document.Key}
	}
	candidate, err := (FileCatalogSource{
		Root: filepath.Join(root, "specs"),
		Manifest: CatalogManifest{
			ID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1",
			ProfileID: domain.CompatibilityProfileKubernetes, Includes: []string{"*_openapi.json"}, DocumentKeys: keys,
		},
	}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, document := range candidate.Documents {
		total += len(document.Bytes)
	}
	if len(candidate.Documents) != 65 || total != 12625155 || len(candidate.SupportFiles) != 0 {
		t.Fatalf("locked source totals = documents:%d bytes:%d support:%d", len(candidate.Documents), total, len(candidate.SupportFiles))
	}
}

func testCatalogManifest(profile string, includes ...string) CatalogManifest {
	return CatalogManifest{
		ID: "catalog", Title: "Catalog", ProfileID: domain.CompatibilityProfileID(profile), Includes: includes,
	}
}

func writeCatalogFile(t *testing.T, root, name, data string) {
	t.Helper()
	writeCatalogBytes(t, root, name, []byte(data))
}

func writeCatalogBytes(t *testing.T, root, name string, data []byte) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func validLargeJSON(size int) []byte {
	prefix := []byte(`{"value":"`)
	suffix := []byte(`"}`)
	if size < len(prefix)+len(suffix) {
		return []byte(`{}`)
	}
	result := make([]byte, 0, size)
	result = append(result, prefix...)
	result = append(result, bytes.Repeat([]byte{'a'}, size-len(prefix)-len(suffix))...)
	return append(result, suffix...)
}
