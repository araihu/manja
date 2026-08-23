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
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

type staticCatalogSource struct {
	candidate domain.CatalogCandidate
}

func (source staticCatalogSource) Load(context.Context) (domain.CatalogCandidate, error) {
	return source.candidate, nil
}

var _ CatalogSource = staticCatalogSource{}

type observedDoneContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

type reservedFlightHandler struct {
	http.Handler
	reserved uint64
}

func (handler reservedFlightHandler) CatalogFlightReservationBytes() uint64 {
	return handler.reserved
}

func (ctx *observedDoneContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Done()
}

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

func TestRecoveryOnlyServerConstructsNoCompilerAndRejectsActivation(t *testing.T) {
	t.Parallel()

	serverAPI, err := NewRecoveryOnly(Config{Version: 1, DataDir: t.TempDir(), Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)
	if len(implementation.parsers) != 0 || len(implementation.compilers) != 0 {
		t.Fatalf("recovery-only parser/compiler construction = %d/%d, want 0/0", len(implementation.parsers), len(implementation.compilers))
	}
	if _, err := serverAPI.Activate(context.Background(), rendererTestCandidate("payments")); !errors.Is(err, ErrActivationUnavailable) {
		t.Fatalf("recovery-only Activate error = %v, want %v", err, ErrActivationUnavailable)
	}
}

func TestServerRejectsStartupWhenProcessPeakExceedsConfiguredBudget(t *testing.T) {
	t.Parallel()

	server, err := New(Config{Version: 1, StartupProcessBytes: 1, ResourceLimits: true, Catalogs: []CatalogConfig{
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
	serverAPI, err := New(Config{Version: 1, StartupProcessBytes: 64 << 20, ResourceLimits: true, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)
	implementation.measureProcessPeak = func() (uint64, error) { return (64 << 20) + 1, nil }
	_, err = serverAPI.Activate(context.Background(), rendererTestCandidate("payments"))
	if !errors.Is(err, ErrStartupProcessBudget) {
		t.Fatalf("over-budget activation error = %v, want %v", err, ErrStartupProcessBudget)
	}
	if _, active := serverAPI.Active("payments"); active {
		t.Fatal("over-budget activation published a catalog")
	}
	response := httptest.NewRecorder()
	serverAPI.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/documents/payments-v1/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("over-budget route status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestActivateBudgetFailureLeavesPublishedRoutesExactlyUnchanged(t *testing.T) {
	dataDir := t.TempDir()
	serverAPI, err := New(Config{Version: 1, DataDir: dataDir, ResourceLimits: true, Catalogs: []CatalogConfig{
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

func TestActivateRejectsPressureObservedAfterStagingWithoutChangingRoutes(t *testing.T) {
	dataDir := t.TempDir()
	serverAPI, err := New(Config{Version: 1, DataDir: dataDir, ResourceLimits: true, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/payments", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)

	paymentsV1 := rendererTestCandidateVersion("payments", "Payments v1", "file-manifest-payments-v1", "c")
	if _, err := serverAPI.Activate(context.Background(), paymentsV1); err != nil {
		t.Fatalf("seed %s: %v", paymentsV1.Revision.ID, err)
	}
	beforeTable := implementation.runtime.Table()
	beforeRoutes, err := os.ReadFile(filepath.Join(dataDir, "state", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}

	measurements := 0
	promotionExclusive := false
	implementation.measureProcessPeak = func() (uint64, error) {
		measurements++
		if measurements < 5 {
			return 1, nil
		}
		implementation.handler.admissions.mutex.Lock()
		promotionExclusive = implementation.handler.admissions.promoting && implementation.handler.admissions.readers == 0
		implementation.handler.admissions.mutex.Unlock()
		return DefaultStartupProcessBytes + 1, nil
	}
	paymentsV2 := rendererTestCandidateVersion("payments", "Payments v2", "file-manifest-payments-v2", "e")
	_, err = serverAPI.Activate(context.Background(), paymentsV2)
	if !errors.Is(err, ErrStartupProcessBudget) {
		t.Fatalf("activation error = %v, want %v", err, ErrStartupProcessBudget)
	}
	if measurements != 5 {
		t.Fatalf("startup measurements = %d, want 5", measurements)
	}
	if !promotionExclusive {
		t.Fatal("final process admission did not own the exclusive request/cache boundary")
	}

	afterTable := implementation.runtime.Table()
	beforePayments := beforeTable.Mounts["/payments"]
	afterPayments := afterTable.Mounts["/payments"]
	if afterTable.Generation != beforeTable.Generation || afterPayments.Active.ID != beforePayments.Active.ID || afterPayments.Previous != beforePayments.Previous {
		t.Errorf("runtime route changed: before=%#v after=%#v", beforePayments, afterPayments)
	}
	afterRoutes, readErr := os.ReadFile(filepath.Join(dataDir, "state", "routes.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(afterRoutes, beforeRoutes) {
		t.Errorf("durable route table changed:\nbefore=%s\nafter=%s", beforeRoutes, afterRoutes)
	}
	if _, statErr := os.Stat(filepath.Join(dataDir, "state", "activation-journal.json")); !os.IsNotExist(statErr) {
		t.Errorf("activation journal after rejected promotion = %v, want absent", statErr)
	}
	response := httptest.NewRecorder()
	serverAPI.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/payments/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Payments v1") || strings.Contains(response.Body.String(), "Payments v2") {
		t.Errorf("route after rejected activation = %d %q, want Payments v1 only", response.Code, response.Body.String())
	}
}

func TestActivateReservesInFlightCacheCapacityBeforePublishing(t *testing.T) {
	const processLimit = 64 << 20
	serverAPI, err := New(Config{Version: 1, DataDir: t.TempDir(), StartupProcessBytes: processLimit, ResourceLimits: true, Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/payments", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)
	implementation.measureProcessPeak = func() (uint64, error) { return 1, nil }
	paymentsV1 := rendererTestCandidateVersion("payments", "Payments v1", "file-manifest-payments-v1", "c")
	if _, err := serverAPI.Activate(context.Background(), paymentsV1); err != nil {
		t.Fatal(err)
	}
	before := implementation.runtime.Table()

	implementation.handler.mutex.Lock()
	implementation.handler.delegate = reservedFlightHandler{Handler: implementation.handler.delegate, reserved: processLimit}
	implementation.handler.mutex.Unlock()
	measurements := 0
	implementation.measureProcessPeak = func() (uint64, error) {
		measurements++
		return 1, nil
	}
	paymentsV2 := rendererTestCandidateVersion("payments", "Payments v2", "file-manifest-payments-v2", "e")
	_, err = serverAPI.Activate(context.Background(), paymentsV2)
	if !errors.Is(err, ErrStartupProcessBudget) {
		t.Fatalf("activation error = %v, want %v", err, ErrStartupProcessBudget)
	}
	if measurements != 5 {
		t.Fatalf("startup measurements = %d, want 5", measurements)
	}
	after := implementation.runtime.Table()
	if after.Generation != before.Generation || after.Mounts["/payments"].Active.ID != before.Mounts["/payments"].Active.ID {
		t.Fatalf("cache-reservation rejection changed runtime: before=%#v after=%#v", before, after)
	}
}

func TestActivateWaitsForInFlightCatalogRequestBeforeFinalAdmission(t *testing.T) {
	serverAPI, err := New(Config{Version: 1, DataDir: t.TempDir(), Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/payments", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)
	paymentsV1 := rendererTestCandidateVersion("payments", "Payments v1", "file-manifest-payments-v1", "c")
	if _, err := serverAPI.Activate(context.Background(), paymentsV1); err != nil {
		t.Fatal(err)
	}

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseRequest) }) })
	implementation.handler.mutex.Lock()
	original := implementation.handler.delegate
	implementation.handler.delegate = http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-releaseRequest
		original.ServeHTTP(response, request)
	})
	implementation.handler.mutex.Unlock()

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response := httptest.NewRecorder()
		serverAPI.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/payments/", nil))
	}()
	<-requestStarted

	prePromotion := make(chan struct{})
	finalAdmission := make(chan struct{})
	measurements := 0
	implementation.measureProcessPeak = func() (uint64, error) {
		measurements++
		switch measurements {
		case 4:
			close(prePromotion)
		case 5:
			close(finalAdmission)
		}
		return 1, nil
	}
	paymentsV2 := rendererTestCandidateVersion("payments", "Payments v2", "file-manifest-payments-v2", "e")
	activationDone := make(chan error, 1)
	go func() {
		_, err := serverAPI.Activate(context.Background(), paymentsV2)
		activationDone <- err
	}()
	<-prePromotion

	deadline := time.Now().Add(2 * time.Second)
	for {
		implementation.handler.admissions.mutex.Lock()
		promoting := implementation.handler.admissions.promoting
		readers := implementation.handler.admissions.readers
		implementation.handler.admissions.mutex.Unlock()
		if promoting && readers == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("activation did not wait behind the in-flight catalog request")
		}
		runtime.Gosched()
	}
	select {
	case <-finalAdmission:
		t.Fatal("final process admission ran before the in-flight request drained")
	default:
	}

	releaseOnce.Do(func() { close(releaseRequest) })
	<-requestDone
	if err := <-activationDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-finalAdmission:
	default:
		t.Fatal("final process admission did not run after the request drained")
	}
	if active, ok := serverAPI.Active("payments"); !ok || active.RevisionID != paymentsV2.Revision.ID {
		t.Fatalf("active catalog after drained promotion = %#v, %t", active, ok)
	}
}

func TestActivationSerializationHonorsContextCancellation(t *testing.T) {
	serverAPI, err := New(Config{Version: 1, DataDir: t.TempDir(), Catalogs: []CatalogConfig{
		{ID: "payments", Mount: "/payments", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict},
	}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := serverAPI.(*server)
	implementation.activation <- struct{}{}
	defer func() { <-implementation.activation }()
	base, cancel := context.WithCancel(context.Background())
	ctx := &observedDoneContext{Context: base, observed: make(chan struct{})}
	activationDone := make(chan error, 1)
	go func() {
		_, activationErr := serverAPI.Activate(ctx, rendererTestCandidate("payments"))
		activationDone <- activationErr
	}()
	<-ctx.observed
	cancel()
	err = <-activationDone
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("activation serialization error = %v, want %v", err, context.Canceled)
	}
	if _, active := serverAPI.Active("payments"); active {
		t.Fatal("canceled activation changed runtime state")
	}
}

func TestCatalogRequestWaitingForPromotionReportsCancellation(t *testing.T) {
	gateway := &catalogGateway{admissions: newCatalogAdmissionGate(), assets: web.NewCatalogAssetsHandler()}
	if err := gateway.beginPromotion(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gateway.endPromotion()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	if response.Code != http.StatusRequestTimeout {
		t.Fatalf("canceled request status = %d, want %d", response.Code, http.StatusRequestTimeout)
	}
}

func TestPromotionGateRejectsCanceledContextWithoutBlockingRequests(t *testing.T) {
	gateway := &catalogGateway{admissions: newCatalogAdmissionGate(), assets: web.NewCatalogAssetsHandler()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gateway.beginPromotion(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("promotion error = %v, want %v", err, context.Canceled)
	}
	gateway.admissions.mutex.Lock()
	promoting := gateway.admissions.promoting
	gateway.admissions.mutex.Unlock()
	if promoting {
		t.Fatal("canceled promotion left request admissions blocked")
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
	if receipt.StartupProcessBytes == 0 {
		t.Fatal("startup process receipt is zero")
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

func TestActivateResourceLimitsAreOptIn(t *testing.T) {
	candidate := rendererTestCandidate("payments")
	candidate.Documents[0].Bytes = []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{},"x-padding":"` + strings.Repeat("a", 8<<20) + `"}`)

	for name, resourceLimits := range map[string]bool{"default off": false, "opted in": true} {
		t.Run(name, func(t *testing.T) {
			server, err := New(Config{Version: 1, DataDir: t.TempDir(), ResourceLimits: resourceLimits, Catalogs: []CatalogConfig{{
				ID: "payments", Mount: "/", Title: "Payments", DefaultDocumentKey: "payments-v1", ProfileID: domain.CompatibilityProfileStrict,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = server.Activate(context.Background(), candidate)
			if resourceLimits && err == nil {
				t.Fatal("bounded activation accepted an oversized source document")
			}
			if !resourceLimits && err != nil {
				t.Fatalf("default activation rejected an oversized source document: %v", err)
			}
		})
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
