package selfhosted

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogstore"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

func TestNewRendererLoadsAndActivatesConfiguredFileCatalog(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "payments.json"), []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "renderer.yaml")
	config := `version: 1
dataDir: data
catalogs:
  - id: payments
    mount: /
    title: Payments
    localDocs:
      public: true
      anonymous: true
      publicationKey: payments
    defaultDocument: payments
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [payments.json]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	handler, receipts, err := NewRenderer(context.Background(), RendererOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].CatalogID != "payments" || receipts[0].SnapshotID == "" {
		t.Fatalf("activation receipts = %#v", receipts)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments") || !strings.Contains(response.Body.String(), `id="manja-local-docs-descriptor"`) {
		t.Fatalf("GET / = %d %q", response.Code, response.Body.String())
	}
	disabled, _, err := NewRenderer(context.Background(), RendererOptions{ConfigPath: configPath, LocalDocsDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	disabledResponse := httptest.NewRecorder()
	disabled.ServeHTTP(disabledResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(disabledResponse.Body.String(), `id="manja-local-docs-descriptor"`) || !strings.Contains(disabledResponse.Body.String(), "Payments") {
		t.Fatalf("disabled local docs changed SSR or emitted descriptor: %q", disabledResponse.Body.String())
	}
}

func TestNewRendererServesRecoveredCatalogWhenRefreshSourcesFail(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	recovered := rendererCandidate("payments", "Payments")
	seedRendererSnapshot(t, dataDir, "/payments", recovered)

	configPath := filepath.Join(root, "renderer.yaml")
	config := `version: 1
dataDir: data
catalogs:
  - id: payments
    mount: /payments
    title: Payments
    defaultDocument: payments-v1
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [missing-payments.json]
  - id: orders
    mount: /orders
    title: Orders
    defaultDocument: orders-v1
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [missing-orders.json]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	handler, receipts, err := NewRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 2 {
		t.Fatalf("refresh receipts = %#v", receipts)
	}
	if !receipts[0].Degraded || receipts[0].SnapshotID == "" || len(receipts[0].Diagnostic) == 0 || len(receipts[0].Diagnostic) > 256 {
		t.Fatalf("recovered refresh receipt = %#v", receipts[0])
	}
	if !receipts[1].Degraded || receipts[1].SnapshotID != "" || len(receipts[1].Diagnostic) == 0 || len(receipts[1].Diagnostic) > 256 {
		t.Fatalf("unavailable refresh receipt = %#v", receipts[1])
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/payments/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments") {
		t.Fatalf("recovered GET /payments/ = %d %q", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/orders/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing GET /orders/ = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestNewRecoveredRendererRequiresCompletePrecompiledStateWithoutLoadingSources(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	seedRendererSnapshot(t, dataDir, "/", rendererCandidate("payments", "Payments"))

	configPath := filepath.Join(root, "renderer.yaml")
	config := `version: 1
catalogs:
  - id: payments
    mount: /
    title: Payments
    defaultDocument: payments-v1
    profile: strict-v1
    source:
      kind: files
      root: sources-that-are-not-in-the-runtime-image
      include: [payments.json]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	handler, receipts, err := NewRecoveredRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].CatalogID != "payments" || receipts[0].SnapshotID == "" || receipts[0].Degraded {
		t.Fatalf("recovery receipts = %#v", receipts)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments") {
		t.Fatalf("recovered GET / = %d %q", response.Code, response.Body.String())
	}
}

func TestNewRecoveredRendererFailsClosedWhenConfiguredCatalogHasNoActiveSnapshot(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "renderer.yaml")
	config := `version: 1
catalogs:
  - id: payments
    mount: /
    title: Payments
    defaultDocument: payments-v1
    profile: strict-v1
    source:
      kind: files
      root: missing
      include: [payments.json]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := NewRecoveredRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: filepath.Join(root, "empty-data")}); err == nil || !strings.Contains(err.Error(), `catalog "payments" has no active snapshot`) {
		t.Fatalf("empty recovery error = %v", err)
	}
}

func TestBuildRendererProducesStateConsumedWithoutSourceFiles(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "sources")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "payments.json"), []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "renderer.yaml")
	config := `version: 1
catalogs:
  - id: payments
    mount: /
    title: Payments
    defaultDocument: payments
    profile: strict-v1
    source:
      kind: files
      root: sources
      include: [payments.json]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "compiled")
	built, err := BuildRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sourceDir); err != nil {
		t.Fatal(err)
	}

	handler, recovered, err := NewRecoveredRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 || len(recovered) != 1 || built[0].SnapshotID != recovered[0].SnapshotID || built[0].RevisionID != recovered[0].RevisionID {
		t.Fatalf("built=%#v recovered=%#v", built, recovered)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments") {
		t.Fatalf("recovered GET / = %d %q", response.Code, response.Body.String())
	}
}

func TestBuildRendererRejectsDegradedSourceState(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "renderer.yaml")
	config := `version: 1
catalogs:
  - id: payments
    mount: /
    title: Payments
    defaultDocument: payments
    profile: strict-v1
    source:
      kind: files
      root: missing
      include: [payments.json]
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRenderer(context.Background(), RendererOptions{ConfigPath: configPath, DataDir: filepath.Join(root, "compiled")}); err == nil || !strings.Contains(err.Error(), `build catalog "payments"`) {
		t.Fatalf("degraded build error = %v", err)
	}
}

func seedRendererSnapshot(t *testing.T, dataDir, mount string, candidate domain.CatalogCandidate) {
	t.Helper()
	parser, err := openapiadapter.NewCatalogParser(nil)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := catalog.NewCompiler(catalog.DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	runtime := catalog.NewRuntime(1)
	coordinator, err := catalogstore.OpenActivationCoordinator(context.Background(), dataDir, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Activate(context.Background(), mount, "", 1, compiled); err != nil {
		_ = coordinator.Close()
		t.Fatal(err)
	}
	if err := coordinator.Close(); err != nil {
		t.Fatal(err)
	}
}

func rendererCandidate(id, title string) domain.CatalogCandidate {
	return domain.CatalogCandidate{
		ID: id, Title: title, ProfileID: domain.CompatibilityProfileStrict, DefaultDocumentKey: id + "-v1",
		Revision: domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "files-a", ManifestDigest: strings.Repeat("a", 64)},
		Documents: []domain.CatalogDocument{{
			Key: id + "-v1", SourcePath: id + ".json", Format: domain.CatalogFormatJSON,
			Bytes: []byte(`{"openapi":"3.0.3","info":{"title":"` + title + `","version":"v1"},"paths":{}}`),
		}},
	}
}
