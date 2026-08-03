package renderer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/araihu/manja/domain"
)

type staticCatalogSource struct {
	candidate domain.CatalogCandidate
}

func (source staticCatalogSource) Load(context.Context) (domain.CatalogCandidate, error) {
	return source.candidate, nil
}

var _ CatalogSource = staticCatalogSource{}

func TestServerExposesStableHandlerAndBoundedUnavailableRoutes(t *testing.T) {
	t.Parallel()

	server, err := New(Config{Version: 1, Catalogs: []CatalogConfig{
		{ID: "kubernetes", Mount: "/kubernetes", Title: "Kubernetes", ProfileID: domain.CompatibilityProfileKubernetes},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if server.Handler() != server.Handler() {
		t.Fatal("Handler returned a different instance")
	}

	for requestPath, wantStatus := range map[string]int{
		"/kubernetes":         http.StatusServiceUnavailable,
		"/kubernetes/":        http.StatusServiceUnavailable,
		"/kubernetes/core-v1": http.StatusServiceUnavailable,
		"/kubernetesx":        http.StatusNotFound,
		"/other":              http.StatusNotFound,
	} {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != wantStatus {
			t.Errorf("GET %s status = %d, want %d", requestPath, response.Code, wantStatus)
		}
		if response.Body.Len() > 256 {
			t.Errorf("GET %s diagnostic bytes = %d, want <= 256", requestPath, response.Body.Len())
		}
	}
}

func TestActivateValidatesConfiguredCandidateBeforeCompilerExists(t *testing.T) {
	t.Parallel()

	server, err := New(Config{Version: 1, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := rendererTestCandidate("payments")
	if _, err := server.Activate(context.Background(), candidate); !errors.Is(err, ErrActivationUnavailable) {
		t.Fatalf("Activate error = %v, want ErrActivationUnavailable", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.Activate(canceled, candidate); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Activate error = %v, want context.Canceled", err)
	}

	candidate.ID = "unknown"
	if _, err := server.Activate(context.Background(), candidate); err == nil || errors.Is(err, ErrActivationUnavailable) {
		t.Fatalf("unconfigured candidate error = %v, want configuration error", err)
	}

	candidate = rendererTestCandidate("payments")
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	if _, err := server.Activate(context.Background(), candidate); err == nil || errors.Is(err, ErrActivationUnavailable) {
		t.Fatalf("profile mismatch error = %v, want configuration error", err)
	}

	candidate = rendererTestCandidate("payments")
	candidate.Documents[0].Key = "other-v1"
	candidate.DefaultDocumentKey = "other-v1"
	if _, err := server.Activate(context.Background(), candidate); err == nil || errors.Is(err, ErrActivationUnavailable) {
		t.Fatalf("missing configured default error = %v, want configuration error", err)
	}

	candidate = rendererTestCandidate("payments")
	candidate.Documents[0].Bytes = nil
	if _, err := server.Activate(context.Background(), candidate); err == nil || errors.Is(err, ErrActivationUnavailable) {
		t.Fatalf("invalid candidate error = %v, want domain validation error", err)
	}
}

func rendererTestCandidate(id string) domain.CatalogCandidate {
	return domain.CatalogCandidate{
		ID: id, Title: "Payments", ProfileID: domain.CompatibilityProfileStrict,
		DefaultDocumentKey: "payments-v1",
		Revision: domain.CatalogRevision{
			Kind: domain.CatalogRevisionFiles, ID: "file-manifest-a",
			ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Documents: []domain.CatalogDocument{{
			Key: "payments-v1", SourcePath: "payments.json", Format: domain.CatalogFormatJSON, Bytes: []byte("{}"),
		}},
	}
}
