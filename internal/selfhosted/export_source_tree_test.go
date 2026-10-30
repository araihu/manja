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

	"github.com/getkin/kin-openapi/openapi3"
)

func TestExportPreservesReferencedSourceTree(t *testing.T) {
	root := t.TempDir()
	sources := filepath.Join(root, "specs")
	if err := os.MkdirAll(filepath.Join(sources, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	public := "openapi: 3.0.3\ninfo: {title: Public, version: 1.0.0}\npaths: {}\ncomponents:\n  schemas:\n    Widget: {$ref: '../schemas/widget.yaml'}\n"
	internal := "openapi: 3.0.3\ninfo: {title: Internal, version: 0.1.0}\npaths: {}\ncomponents:\n  schemas:\n    Widget: {$ref: './public.yaml#/components/schemas/Widget'}\n"
	files := map[string]string{"api/public.yaml": public, "api/internal.yaml": internal, "schemas/widget.yaml": "type: object\nproperties:\n  id: {type: string}\n"}
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
      include: [api/public.yaml, api/internal.yaml]
      documents:
        - {path: api/public.yaml, key: public}
        - {path: api/internal.yaml, key: internal}
`, sources)
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
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	for _, name := range []string{"public", "internal"} {
		document, err := loader.LoadFromFile(filepath.Join(sourceRoot, "api", name+".yaml"))
		if err != nil {
			t.Fatalf("exported %s references no longer resolve: %v", name, err)
		}
		if err := document.Validate(context.Background()); err != nil {
			t.Fatal(err)
		}
		shell, err := os.ReadFile(filepath.Join(output, "catalogs/project/documents", name, "index.html"))
		want := "/docs/catalogs/project/snapshots/" + receipt.Catalogs[0].SnapshotID + "/source-files/api/" + name + ".yaml"
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
		url := "/catalogs/project/snapshots/" + receipt.Catalogs[0].SnapshotID + "/source-files/" + name
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
		if response.Code != http.StatusOK || response.Body.String() != expected {
			t.Fatalf("GET %s = %d: %s", url, response.Code, response.Body.String())
		}
	}
}
