//go:build !manja_runtime

package selfhosted_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/internal/selfhosted"
)

const lifecycleSpec = `{"openapi":"3.0.3","info":{"title":"Lifecycle API","version":"1.0.0"},"paths":{}}`

type lifecycleFixture struct {
	options selfhosted.RendererOptions
	assets  string
	spec    string
}

func newLifecycleFixture(t *testing.T) lifecycleFixture {
	t.Helper()
	root := t.TempDir()
	assets := filepath.Join(root, "brand")
	if err := os.Mkdir(assets, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "logo.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(root, "api.json")
	if err := os.WriteFile(spec, []byte(lifecycleSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := `version: 1
assetDir: brand
organization:
  branding:
    displayName: Lifecycle
    logo: {src: /brand-assets/logo.svg}
catalogs:
  - id: lifecycle
    mount: /lifecycle
    title: Lifecycle
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [api.json]
`
	config := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(config, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	return lifecycleFixture{options: selfhosted.RendererOptions{ConfigPath: config, DataDir: filepath.Join(root, "state")}, assets: assets, spec: spec}
}

func lifecycleContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func owningLifecycleHandler(t *testing.T, handler http.Handler) io.Closer {
	t.Helper()
	closer, ok := handler.(io.Closer)
	if !ok {
		t.Fatal("owning renderer handler does not implement io.Closer")
	}
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Errorf("close renderer: %v", err)
		}
	})
	return closer
}

func requireLifecycleResponse(t *testing.T, handler http.Handler, route, content string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), content) {
		t.Fatalf("GET %s = %d %q", route, response.Code, response.Body.String())
	}
}

func TestRendererLifecycleBrandedHandlerCloseReleasesState(t *testing.T) {
	fixture := newLifecycleFixture(t)
	handler, built, err := selfhosted.NewRenderer(lifecycleContext(t), fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	closer := owningLifecycleHandler(t, handler)
	requireLifecycleResponse(t, handler, "/brand-assets/logo.svg", "<svg")
	requireLifecycleResponse(t, handler, "/lifecycle/", "Lifecycle")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, recovered, err := selfhosted.NewRecoveredRenderer(lifecycleContext(t), fixture.options)
	if err != nil {
		t.Fatalf("recover after branded handler Close: %v", err)
	}
	owningLifecycleHandler(t, reopened)
	if len(built) != 1 || len(recovered) != 1 || built[0].SnapshotID != recovered[0].SnapshotID {
		t.Fatalf("built=%#v recovered=%#v", built, recovered)
	}
	requireLifecycleResponse(t, reopened, "/brand-assets/logo.svg", "<svg")
	requireLifecycleResponse(t, reopened, "/lifecycle/", "Lifecycle")
}

func TestRendererLifecycleBrandDirectoryFailureReleasesState(t *testing.T) {
	for _, recoveryOnly := range []bool{false, true} {
		name := "NewRenderer"
		if recoveryOnly {
			name = "NewRecoveredRenderer"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newLifecycleFixture(t)
			if recoveryOnly {
				if _, err := selfhosted.BuildRenderer(lifecycleContext(t), fixture.options); err != nil {
					t.Fatal(err)
				}
			}
			moved := fixture.assets + "-away"
			if err := os.Rename(fixture.assets, moved); err != nil {
				t.Fatal(err)
			}
			constructor := selfhosted.NewRenderer
			if recoveryOnly {
				constructor = selfhosted.NewRecoveredRenderer
			}
			handler, _, err := constructor(lifecycleContext(t), fixture.options)
			if err == nil || !strings.Contains(err.Error(), "branding asset directory") {
				if handler != nil {
					owningLifecycleHandler(t, handler)
				}
				t.Fatalf("missing asset directory error = %v", err)
			}
			if err := os.Rename(moved, fixture.assets); err != nil {
				t.Fatal(err)
			}
			reopened, _, err := selfhosted.NewRecoveredRenderer(lifecycleContext(t), fixture.options)
			if err != nil {
				t.Fatalf("recovery after %s asset failure: %v", name, err)
			}
			owningLifecycleHandler(t, reopened)
			requireLifecycleResponse(t, reopened, "/lifecycle/", "Lifecycle")
		})
	}
}

func TestRendererLifecycleIncompleteRecoveryReleasesStateBeforeBuild(t *testing.T) {
	fixture := newLifecycleFixture(t)
	handler, _, err := selfhosted.NewRecoveredRenderer(lifecycleContext(t), fixture.options)
	if err == nil || !strings.Contains(err.Error(), "has no active snapshot") {
		if handler != nil {
			owningLifecycleHandler(t, handler)
		}
		t.Fatalf("incomplete recovery error = %v", err)
	}
	if _, err := selfhosted.BuildRenderer(lifecycleContext(t), fixture.options); err != nil {
		t.Fatalf("build after incomplete recovery: %v", err)
	}
}

func TestRendererLifecycleExportWorkerFailureReleasesState(t *testing.T) {
	fixture := newLifecycleFixture(t)
	_, err := selfhosted.ExportRenderer(lifecycleContext(t), selfhosted.ExportOptions{RendererOptions: fixture.options, Output: filepath.Join(t.TempDir(), "public"), BasePath: "/", FragmentWorkers: 33})
	if err == nil || !strings.Contains(err.Error(), "fragment workers must be between 1 and 32") {
		t.Fatalf("invalid export workers error = %v", err)
	}
	handler, _, err := selfhosted.NewRecoveredRenderer(lifecycleContext(t), fixture.options)
	if err != nil {
		t.Fatalf("recover after export worker failure: %v", err)
	}
	owningLifecycleHandler(t, handler)
	requireLifecycleResponse(t, handler, "/lifecycle/", "Lifecycle")
}

func TestRendererLifecycleDegradedBuildReleasesState(t *testing.T) {
	fixture := newLifecycleFixture(t)
	if err := os.Remove(fixture.spec); err != nil {
		t.Fatal(err)
	}
	_, err := selfhosted.BuildRenderer(lifecycleContext(t), fixture.options)
	if err == nil || !strings.Contains(err.Error(), `build catalog "lifecycle"`) {
		t.Fatalf("degraded build error = %v", err)
	}
	if err := os.WriteFile(fixture.spec, []byte(lifecycleSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := selfhosted.BuildRenderer(lifecycleContext(t), fixture.options); err != nil {
		t.Fatalf("build after degraded source failure: %v", err)
	}
}
