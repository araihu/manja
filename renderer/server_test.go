package renderer

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		"/_manja/catalog/document-combobox/options?catalog-mount=%2Fkubernetes": http.StatusServiceUnavailable,
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

func TestServerRejectsStartupWhenProcessPeakExceedsConfiguredBudget(t *testing.T) {
	t.Parallel()

	server, err := New(Config{Version: 1, StartupProcessBytes: 1, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/", Title: "Payments", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Recover(context.Background()); !errors.Is(err, ErrStartupProcessBudget) {
		t.Fatalf("startup process budget error = %v, want %v", err, ErrStartupProcessBudget)
	}
}

func TestActivateRejectsOverBudgetProcessBeforePublishing(t *testing.T) {
	start, err := processPeakBytes()
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{Version: 1, StartupProcessBytes: start + (8 << 20), Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	pressure := make([]byte, 64<<20)
	for index := 0; index < len(pressure); index += 4096 {
		pressure[index] = 1
	}
	_, err = server.Activate(context.Background(), rendererTestCandidate("payments"))
	runtime.KeepAlive(pressure)
	if !errors.Is(err, ErrStartupProcessBudget) {
		t.Fatalf("over-budget activation error = %v, want %v", err, ErrStartupProcessBudget)
	}
	if _, active := server.Active("payments"); active {
		t.Fatal("over-budget activation published a catalog")
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("over-budget route status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestActivateBudgetFailureLeavesPublishedRoutesExactlyUnchanged(t *testing.T) {
	dataDir := t.TempDir()
	serverAPI, err := New(Config{Version: 1, DataDir: dataDir, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/payments", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
		{ID: "inventory", Mount: "/inventory", Title: "Inventory", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)

	inventory := rendererTestCandidateVersion("inventory", "Inventory v1", "file-manifest-inventory-v1", "d")
	paymentsV0 := rendererTestCandidateVersion("payments", "Payments v0", "file-manifest-payments-v0", "b")
	paymentsV1 := rendererTestCandidateVersion("payments", "Payments v1", "file-manifest-payments-v1", "c")
	for _, candidate := range []domain.CatalogCandidate{inventory, paymentsV0, paymentsV1} {
		if _, err := serverAPI.Activate(context.Background(), candidate); err != nil {
			t.Fatalf("seed %s %s: %v", candidate.ID, candidate.Revision.ID, err)
		}
	}

	beforeTable := implementation.runtime.Table()
	beforePayments := beforeTable.Mounts["/payments"]
	if beforePayments.Previous == nil || beforePayments.Previous.Manifest.Identity.RevisionID != paymentsV0.Revision.ID {
		t.Fatalf("seed previous route = %#v, want %s", beforePayments.Previous, paymentsV0.Revision.ID)
	}
	beforeRoutes, err := os.ReadFile(filepath.Join(dataDir, "state", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}

	paymentsV2 := rendererTestCandidateVersion("payments", "Payments v2", "file-manifest-payments-v2", "e")
	measurements := 0
	transientStatus := 0
	transientBody := ""
	implementation.measureProcessPeak = func() (uint64, error) {
		measurements++
		if measurements < 4 {
			return 1, nil
		}
		response := httptest.NewRecorder()
		serverAPI.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/payments/", nil))
		transientStatus = response.Code
		transientBody = response.Body.String()
		return DefaultStartupProcessBytes + 1, nil
	}

	_, err = serverAPI.Activate(context.Background(), paymentsV2)
	if !errors.Is(err, ErrStartupProcessBudget) {
		t.Fatalf("activation error = %v, want %v", err, ErrStartupProcessBudget)
	}
	if measurements != 4 {
		t.Fatalf("startup measurements = %d, want 4", measurements)
	}
	if transientStatus != http.StatusOK || !strings.Contains(transientBody, "Payments v1") || strings.Contains(transientBody, "Payments v2") {
		t.Errorf("route observed during rejecting measurement = %d %q, want Payments v1 only", transientStatus, transientBody)
	}

	afterTable := implementation.runtime.Table()
	afterPayments := afterTable.Mounts["/payments"]
	if afterTable.Generation != beforeTable.Generation || afterPayments.Active.ID != beforePayments.Active.ID || afterPayments.Previous == nil || afterPayments.Previous.ID != beforePayments.Previous.ID {
		t.Errorf("payments route changed: before=%#v after=%#v", beforePayments, afterPayments)
	}
	beforeInventory := beforeTable.Mounts["/inventory"]
	afterInventory := afterTable.Mounts["/inventory"]
	if afterInventory.Active.ID != beforeInventory.Active.ID || afterInventory.Previous != beforeInventory.Previous {
		t.Errorf("inventory route changed: before=%#v after=%#v", beforeInventory, afterInventory)
	}
	afterRoutes, readErr := os.ReadFile(filepath.Join(dataDir, "state", "routes.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(afterRoutes, beforeRoutes) {
		t.Errorf("durable route table changed:\nbefore=%s\nafter=%s", beforeRoutes, afterRoutes)
	}
	response := httptest.NewRecorder()
	serverAPI.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/payments/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments v1") || strings.Contains(response.Body.String(), "Payments v2") {
		t.Errorf("route after rejected activation = %d %q, want Payments v1 only", response.Code, response.Body.String())
	}
}

func TestActivateCompilesAndPublishesConfiguredCandidate(t *testing.T) {
	t.Parallel()

	server, err := New(Config{Version: 1, DataDir: t.TempDir(), Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	candidate := rendererTestCandidate("payments")
	receipt, err := server.Activate(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CatalogID != "payments" || receipt.Mount != "/" || receipt.RevisionID != candidate.Revision.ID || receipt.SnapshotID == "" {
		t.Fatalf("activation receipt = %#v", receipt)
	}
	if receipt.StartupProcessBytes == 0 || receipt.StartupProcessBytes > DefaultStartupProcessBytes {
		t.Fatalf("startup process receipt = %d, want 1..%d", receipt.StartupProcessBytes, DefaultStartupProcessBytes)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments") {
		t.Fatalf("activated handler = %d %s", response.Code, response.Body.String())
	}
	asset := httptest.NewRecorder()
	server.Handler().ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/manja-assets/manja.css", nil))
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("catalog asset = %d %q", asset.Code, asset.Header().Get("Content-Type"))
	}
	combo := httptest.NewRecorder()
	server.Handler().ServeHTTP(combo, httptest.NewRequest(http.MethodGet, "/_manja/catalog/document-combobox/options?catalog-mount=%2F&q=payments", nil))
	if combo.Code != http.StatusOK || !strings.Contains(combo.Body.String(), ">payments-v1</span>") {
		t.Fatalf("catalog combobox = %d %q", combo.Code, combo.Body.String())
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.Activate(canceled, candidate); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Activate error = %v, want context.Canceled", err)
	}

	candidate.ID = "unknown"
	if _, err := server.Activate(context.Background(), candidate); err == nil {
		t.Fatalf("unconfigured candidate error = %v, want configuration error", err)
	}

	candidate = rendererTestCandidate("payments")
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	if _, err := server.Activate(context.Background(), candidate); err == nil {
		t.Fatalf("profile mismatch error = %v, want configuration error", err)
	}

	candidate = rendererTestCandidate("payments")
	candidate.Documents[0].Key = "other-v1"
	candidate.DefaultDocumentKey = "other-v1"
	if _, err := server.Activate(context.Background(), candidate); err == nil {
		t.Fatalf("missing configured default error = %v, want configuration error", err)
	}

	candidate = rendererTestCandidate("payments")
	candidate.Documents[0].Bytes = nil
	if _, err := server.Activate(context.Background(), candidate); err == nil {
		t.Fatalf("invalid candidate error = %v, want domain validation error", err)
	}
}

func TestRendererWiresValidatedSocialImageMIMETypeIntoPresentation(t *testing.T) {
	t.Parallel()

	server, err := New(Config{Version: 1, DataDir: t.TempDir(), Catalogs: []CatalogConfig{{
		ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict,
		SEO: CatalogSEO{
			CanonicalBase: "https://docs.example.test", SocialImage: "https://docs.example.test/social.jpeg", SocialImageAlt: "Payments API reference",
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Activate(context.Background(), rendererTestCandidate("payments")); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("catalog route = %d body=%q", response.Code, response.Body.String())
	}
	for _, want := range []string{
		`<meta property="og:image" content="https://docs.example.test/social.jpeg">`,
		`<meta property="og:image:type" content="image/jpeg">`,
	} {
		if count := strings.Count(response.Body.String(), want); count != 1 {
			t.Errorf("presentation metadata %q count = %d, want 1", want, count)
		}
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
			Key: "payments-v1", SourcePath: "payments.json", Format: domain.CatalogFormatJSON, Bytes: []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{}}`),
		}},
	}
}

func rendererTestCandidateVersion(id, title, revisionID, digestCharacter string) domain.CatalogCandidate {
	candidate := rendererTestCandidate(id)
	candidate.Title = title
	candidate.Revision.ID = revisionID
	candidate.Revision.ManifestDigest = strings.Repeat(digestCharacter, 64)
	candidate.Documents[0].Bytes = []byte(strings.Replace(string(candidate.Documents[0].Bytes), "Payments", title, 1))
	return candidate
}
