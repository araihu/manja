package selfhosted

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments") {
		t.Fatalf("GET / = %d %q", response.Code, response.Body.String())
	}
}
