package selfhosted

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/web"
)

func TestExportBasePathValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"/", "/group/project/", "/a-b_1/"} {
		if err := canonicalExportBasePath(value); err != nil {
			t.Errorf("canonicalExportBasePath(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"", "project/", "/project", "//project/", "/group//project/", "/group/../project/", "/group/./project/", `/group\project/`, "/project%2f/", "/project/?x=1", "/project/#x", "/project name/", "/project\n/"} {
		if err := canonicalExportBasePath(value); err == nil {
			t.Errorf("canonicalExportBasePath(%q) succeeded", value)
		}
	}
}

func TestExportRendererIncludesCatalogWithoutLocalDocsVisibility(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "private.json"), []byte(`{"openapi":"3.0.3","info":{"title":"Private API","version":"v1"},"paths":{"/charges":{"get":{"operationId":"listCharges","responses":{"200":{"description":"ok"}}}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `version: 1
dataDir: data
catalogs:
  - id: private
    mount: /private
    title: Private
    defaultDocument: private
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [private.json]
`
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	receipt, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Catalogs) != 1 || receipt.Catalogs[0].CatalogID != "private" || receipt.Catalogs[0].PublicationKey != "private" {
		t.Fatalf("receipt = %#v", receipt)
	}
	for _, name := range []string{"index.html", "private/index.html", "private/documents/private/index.html", "private/_manja/offline-shell/index.html", "sw.js", exportManifestPath} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	if _, err := VerifyExport(context.Background(), output); err != nil {
		t.Fatalf("VerifyExport: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(output, "private/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `id="manja-local-docs-descriptor"`) || !strings.Contains(string(body), `"publicationKey":"private"`) || !strings.Contains(string(body), `src="/manja-assets/local-docs.js"`) {
		t.Fatalf("visibility-disabled static shell lacks export authority: %s", body)
	}
}

func TestExportRendererRewritesSubpathDeployment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "api.json"), []byte(`{"openapi":"3.0.3","info":{"title":"API","version":"v1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `version: 1
dataDir: data
catalogs:
  - id: api
    mount: /api
    title: API
    defaultDocument: api
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [api.json]
`
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/group/project/"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(output, "api/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`src="/group/project/manja-assets/local-docs.js"`, `"deploymentBase":"/group/project/"`, `"publicationBase":"/group/project/api/"`, `href="/group/project/"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("subpath shell missing %q", want)
		}
	}
}

func TestExportWorkerChangesWithShellsAndRemainsDeterministic(t *testing.T) {
	exportWorker := func(shell string) []byte {
		t.Helper()
		assets := web.NewCatalogAssetsHandler()
		handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/" {
				response.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = response.Write([]byte(shell))
				return
			}
			assets.ServeHTTP(response, request)
		})
		output := filepath.Join(t.TempDir(), "public")
		if _, err := exportFromHandler(context.Background(), handler, nil, output, "/"); err != nil {
			t.Fatal(err)
		}
		worker, err := os.ReadFile(filepath.Join(output, "sw.js"))
		if err != nil {
			t.Fatal(err)
		}
		return worker
	}

	first := exportWorker(`<!doctype html><title>First</title>`)
	changed := exportWorker(`<!doctype html><title>Changed</title>`)
	repeated := exportWorker(`<!doctype html><title>First</title>`)
	if string(first) == string(changed) {
		t.Fatal("exported Service Worker did not change with cacheable HTML")
	}
	if string(first) != string(repeated) {
		t.Fatal("identical export shells produced different Service Worker bytes")
	}
}

func TestExportRejectsNonEmptyOutputWithoutMutation(t *testing.T) {
	output := t.TempDir()
	marker := filepath.Join(output, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareExportOutput(output); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("prepareExportOutput error = %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("marker changed: %q %v", data, err)
	}
}

func TestExportCaptureRejectsRedirect(t *testing.T) {
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/target", http.StatusTemporaryRedirect)
	})
	if _, err := captureHTTP(context.Background(), handler, "/source", 0, ""); err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("captureHTTP error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/source", nil))
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("fixture status = %d", response.Code)
	}
}
