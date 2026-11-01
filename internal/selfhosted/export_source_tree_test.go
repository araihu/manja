package selfhosted

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestExportPreservesReferencedSourceTree(t *testing.T) {
	for _, test := range []struct {
		name, filename         string
		support, selfReference bool
	}{
		{name: "support files", filename: "public.yaml", support: true},
		{name: "selected documents only", filename: "public.yaml"},
		{name: "space", filename: "public api.yaml"},
		{name: "unicode", filename: "públicá.yaml"},
		{name: "literal URL characters", filename: "public%?#.yaml"},
		{name: "self reference", filename: "original.yaml", selfReference: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			testExportPreservesReferencedSourceTree(t, test.filename, test.support, test.selfReference)
		})
	}
}

func testExportPreservesReferencedSourceTree(t *testing.T, filename string, support, selfReference bool) {
	t.Helper()
	root := t.TempDir()
	sources := filepath.Join(root, "specs")
	if err := os.MkdirAll(filepath.Join(sources, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	public := "openapi: 3.0.3\ninfo: {title: Public, version: 1.0.0}\npaths: {}\ncomponents:\n  schemas:\n    Widget: {type: object, properties: {id: {type: string}}}\n"
	if support {
		public = strings.Replace(public, "{type: object, properties: {id: {type: string}}}", "{$ref: '../schemas/widget.yaml'}", 1)
	}
	reference := "./" + filename + "#/components/schemas/Widget"
	if selfReference {
		public += "    Alias: {$ref: '" + reference + "'}\n"
	}
	files := map[string]string{"api/" + filename: public}
	documents := map[string]string{"public": "api/" + filename}
	if !selfReference {
		files["api/internal.yaml"] = "openapi: 3.0.3\ninfo: {title: Internal, version: 0.1.0}\npaths: {}\ncomponents:\n  schemas:\n    Widget: {$ref: '" + reference + "'}\n"
		// Encoded $refs remain forbidden by the source capture policy. Test
		// literal URL characters in a standalone source filename instead.
		if strings.ContainsAny(filename, "%?#") {
			files["api/internal.yaml"] = "openapi: 3.0.3\ninfo: {title: Internal, version: 0.1.0}\npaths: {}\n"
		}
		documents["internal"] = "api/internal.yaml"
	}
	if support {
		files["schemas/widget.yaml"] = "type: object\nproperties:\n  id: {type: string}\n"
	}
	for name, contents := range files {
		filename := filepath.Join(sources, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configuration := fmt.Sprintf(`version: 1
catalogs:
  - id: project
    mount: /catalogs/project
    title: Project
    defaultDocument: public
    profile: strict-v1
    source:
      kind: files
      root: %q
      include: [api/**]
      documents:
        - {path: %q, key: public}
`, sources, "api/"+filename)
	if !selfReference {
		configuration += "        - {path: api/internal.yaml, key: internal}\n"
	}
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	receipt, err := ExportRenderer(context.Background(), ExportOptions{
		RendererOptions: RendererOptions{ConfigPath: configPath, DataDir: filepath.Join(root, "data")},
		Output:          output, BasePath: "/docs/", FragmentWorkers: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyExport(context.Background(), output); err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Join(output, "catalogs", "project", "snapshots", receipt.Catalogs[0].SnapshotID, "source-files")
	for name, expected := range files {
		actual, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(name)))
		if err != nil || string(actual) != expected {
			t.Fatalf("source %s = %q, %v", name, actual, err)
		}
	}
	if filename == "public.yaml" && !support && !selfReference {
		for _, mutation := range []string{"missing", "changed"} {
			t.Run(mutation+" original", func(t *testing.T) {
				copyRoot := filepath.Join(t.TempDir(), "export")
				if err := os.CopyFS(copyRoot, os.DirFS(output)); err != nil {
					t.Fatal(err)
				}
				manifestPath := filepath.Join(copyRoot, filepath.FromSlash(exportManifestPath))
				data, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				var manifest exportManifest
				if err := json.Unmarshal(data, &manifest); err != nil {
					t.Fatal(err)
				}
				target, err := filepath.Rel(output, filepath.Join(sourceRoot, "api/public.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				target = filepath.ToSlash(target)
				entries := manifest.Files[:0]
				for _, entry := range manifest.Files {
					if entry.Path == target {
						if mutation == "missing" {
							if err := os.Remove(filepath.Join(copyRoot, filepath.FromSlash(target))); err != nil {
								t.Fatal(err)
							}
							continue
						}
						changed := []byte("changed original")
						if err := os.WriteFile(filepath.Join(copyRoot, filepath.FromSlash(target)), changed, 0o600); err != nil {
							t.Fatal(err)
						}
						digest := sha256.Sum256(changed)
						entry.Length, entry.SHA256 = uint64(len(changed)), hex.EncodeToString(digest[:])
					}
					entries = append(entries, entry)
				}
				manifest.Files = entries
				data, err = encodeExportManifest(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := VerifyExport(context.Background(), copyRoot); err == nil {
					t.Fatal("accepted a partial or changed source tree with an updated export inventory")
				}
			})
		}
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	for name, sourcePath := range documents {
		document, err := loader.LoadFromFile(filepath.Join(sourceRoot, filepath.FromSlash(sourcePath)))
		if err != nil {
			t.Fatalf("exported %s references no longer resolve: %v", name, err)
		}
		if err := document.Validate(context.Background()); err != nil {
			t.Fatal(err)
		}
		shell, err := os.ReadFile(filepath.Join(output, "catalogs/project/documents", name, "index.html"))
		want := (&url.URL{Path: "/docs/catalogs/project/snapshots/" + receipt.Catalogs[0].SnapshotID + "/source-files/" + sourcePath}).EscapedPath()
		if err != nil || !strings.Contains(string(shell), want) {
			t.Fatalf("%s download does not preserve its source location: %v", name, err)
		}
	}

	handler, _, err := NewRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	for name, expected := range files {
		response := httptest.NewRecorder()
		href := (&url.URL{Path: "/catalogs/project/snapshots/" + receipt.Catalogs[0].SnapshotID + "/source-files/" + name}).EscapedPath()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, href, nil))
		if response.Code != http.StatusOK || response.Body.String() != expected {
			t.Fatalf("GET %s = %d: %s", href, response.Code, response.Body.String())
		}
	}
}
