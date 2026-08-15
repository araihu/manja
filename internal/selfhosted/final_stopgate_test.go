package selfhosted

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type startupResult struct {
	handler http.Handler
	err     error
}

func startWithContext(ctx context.Context, start func(context.Context) (http.Handler, error)) (http.Handler, error) {
	results := make(chan startupResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler, err := start(ctx)
		results <- startupResult{handler: handler, err: err}
	}()

	select {
	case got := <-results:
		<-done
		return got.handler, got.err
	case <-ctx.Done():
		<-done
		return nil, ctx.Err()
	}
}

func TestStartWithContextWaitsForStartupBeforeReturningDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	finished := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := startWithContext(ctx, func(ctx context.Context) (http.Handler, error) {
			close(started)
			<-ctx.Done()
			close(finished)
			return nil, ctx.Err()
		})
		result <- err
	}()

	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startWithContext did not return after startup finished")
	}
	select {
	case <-finished:
	default:
		t.Fatal("startWithContext returned before startup finished")
	}
}

func TestDefaultGitHubFixtureStartsWithinDeadline(t *testing.T) {
	// The race detector substantially slows schema/example construction. Thirty
	// seconds keeps the startup contract bounded while leaving CI headroom.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dataDir := t.TempDir()
	handler, err := startWithContext(ctx, func(ctx context.Context) (http.Handler, error) {
		return NewWithOptions(ctx, Options{
			ProjectID: "github",
			SourceID:  "github-fixture",
			SpecPath: filepath.Join(
				"..", "adapters", "openapi", "testdata", "github-v3-rest.json",
			),
			DataDir: dataDir,
		})
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("default self-hosted fixture did not start within deadline: %v", ctx.Err())
		}
		t.Fatalf("start default self-hosted fixture: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("default fixture status = %d", recorder.Code)
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
