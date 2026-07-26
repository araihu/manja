package selfhosted

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultGitHubFixtureStartsWithinDeadline(t *testing.T) {
	// The race detector substantially slows schema/example construction. Thirty
	// seconds keeps the startup contract bounded while leaving CI headroom.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type result struct {
		handler http.Handler
		err     error
	}
	results := make(chan result, 1)
	dataDir := t.TempDir()
	go func() {
		handler, err := NewWithOptions(ctx, Options{
			ProjectID: "github",
			SourceID:  "github-fixture",
			SpecPath: filepath.Join(
				"..", "adapters", "openapi", "testdata", "github-v3-rest.json",
			),
			DataDir: dataDir,
		})
		results <- result{handler: handler, err: err}
	}()

	select {
	case <-ctx.Done():
		t.Fatalf("default self-hosted fixture did not start within deadline: %v", ctx.Err())
	case got := <-results:
		if got.err != nil {
			t.Fatalf("start default self-hosted fixture: %v", got.err)
		}
		recorder := httptest.NewRecorder()
		got.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("default fixture status = %d", recorder.Code)
		}
	}
}

func TestNewWithOptionsSyncsValidOperationParameterOverride(t *testing.T) {
	root := t.TempDir()
	specPath := filepath.Join(root, "openapi.yaml")
	writeTestFile(t, specPath, `
openapi: 3.1.0
info: {title: Override, version: 1.0.0}
paths:
  /payments/{id}:
    parameters:
      - {name: id, in: path, required: true, schema: {type: string}}
    get:
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer}}
      responses:
        "200": {description: ok}
`)
	handler, err := NewWithOptions(context.Background(), Options{
		ProjectID: "payments",
		SourceID:  "payments-file",
		SpecPath:  specPath,
		DataDir:   filepath.Join(root, "data"),
	})
	if err != nil {
		t.Fatalf("sync valid operation parameter override: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("synced docs status = %d", recorder.Code)
	}
}

func writeTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
