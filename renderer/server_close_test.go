package renderer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/araihu/manja/domain"
)

func closeTestServer(t *testing.T, dataDir string) (Server, io.Closer, Config) {
	t.Helper()
	config := Config{Version: 1, DataDir: dataDir, Catalogs: []CatalogConfig{validCatalogConfig("payments", "/payments")}}
	api, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	closer, ok := api.Handler().(io.Closer)
	if !ok {
		t.Fatal("handler does not expose its owner's Close")
	}
	t.Cleanup(func() { _ = closer.Close() })
	return api, closer, config
}

func TestServerCloseReleasesConfiguredStateAndRejectsReuse(t *testing.T) {
	dataDir := t.TempDir()
	api, closer, config := closeTestServer(t, dataDir)
	candidate := rendererTestCandidate("payments")
	receipt, err := api.Activate(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("configured state directory was removed: %v", err)
	}
	if err := api.Recover(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Recover after Close = %v", err)
	}
	if _, err := api.Activate(context.Background(), candidate); !errors.Is(err, ErrClosed) {
		t.Fatalf("Activate after Close = %v", err)
	}
	if _, active := api.Active("payments"); active {
		t.Fatal("closed owner still advertises an active catalog")
	}
	recovered, err := NewRecoveryOnly(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.(io.Closer).Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := recovered.Recover(ctx); err != nil {
		t.Fatalf("recover released state: %v", err)
	}
	if got, active := recovered.Active("payments"); !active || got.SnapshotID != receipt.SnapshotID {
		t.Fatalf("durable snapshot after Close = %#v, %t", got, active)
	}
}

func TestServerCloseRemovesOnlyOwnedTemporaryState(t *testing.T) {
	api, closer, _ := closeTestServer(t, "")
	if err := api.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	generated := api.(*server).generatedDir
	if generated == "" {
		t.Fatal("renderer did not allocate temporary state")
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(generated); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary state after Close: %v", err)
	}
	if err := api.Recover(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed temporary renderer reopened: %v", err)
	}
}

func TestServerConcurrentCloseDrainsRequestsAndRejectsNewAdmissions(t *testing.T) {
	api, closer, _ := closeTestServer(t, t.TempDir())
	if _, err := api.Activate(context.Background(), rendererTestCandidate("payments")); err != nil {
		t.Fatal(err)
	}
	implementation := api.(*server)
	gateway := implementation.handler
	original := gateway.delegate
	started, release := make(chan struct{}), make(chan struct{})
	var startOnce, releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	gateway.install(implementation.runtime, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() { close(started) })
		<-release
		original.ServeHTTP(w, r)
	}))
	first, firstDone := httptest.NewRecorder(), make(chan struct{})
	go func() {
		api.Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/payments/", nil))
		close(firstDone)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter the renderer")
	}
	closed := make(chan error, 4)
	for range 4 {
		go func() { closed <- closer.Close() }()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		gateway.admissions.mutex.Lock()
		stopped, changed := gateway.admissions.closed, gateway.admissions.changed
		gateway.admissions.mutex.Unlock()
		if stopped {
			break
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal("Close did not stop new admissions")
		}
	}
	second, secondDone := httptest.NewRecorder(), make(chan struct{})
	go func() {
		api.Handler().ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/payments/", nil))
		close(secondDone)
	}()
	select {
	case <-secondDone:
		if second.Code != http.StatusServiceUnavailable {
			t.Fatalf("request during Close = %d", second.Code)
		}
	case <-ctx.Done():
		t.Fatal("new request blocked behind a closing renderer")
	}
	select {
	case err := <-closed:
		t.Fatalf("Close completed before the admitted request: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-firstDone:
		if first.Code != http.StatusOK {
			t.Fatalf("admitted request = %d", first.Code)
		}
	case <-ctx.Done():
		t.Fatal("admitted request did not finish")
	}
	for range 4 {
		select {
		case err := <-closed:
			if err != nil {
				t.Fatalf("concurrent Close: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent Close did not return")
		}
	}
}

type closeTestParser struct {
	catalogParser
	entered chan struct{}
	release chan struct{}
}

func (parser closeTestParser) Parse(ctx context.Context, candidate domain.CatalogCandidate) (domain.CatalogIndex, error) {
	close(parser.entered)
	select {
	case <-parser.release:
		return parser.catalogParser.Parse(ctx, candidate)
	case <-ctx.Done():
		return domain.CatalogIndex{}, ctx.Err()
	}
}

func TestServerCloseWaitsForActivationToPublish(t *testing.T) {
	api, closer, config := closeTestServer(t, t.TempDir())
	implementation := api.(*server)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	implementation.parsers["payments"] = closeTestParser{catalogParser: implementation.parsers["payments"], entered: entered, release: release}
	activated := make(chan error, 1)
	activationCtx, cancelActivation := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelActivation()
	go func() {
		_, err := api.Activate(activationCtx, rendererTestCandidate("payments"))
		activated <- err
	}()
	select {
	case <-entered:
	case <-activationCtx.Done():
		t.Fatal("activation did not enter the parser")
	}
	closed := make(chan error, 1)
	go func() { closed <- closer.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close interrupted an in-flight activation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	for _, completion := range []<-chan error{activated, closed} {
		select {
		case err := <-completion:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("activation/Close did not finish")
		}
	}
	recovered, err := NewRecoveryOnly(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.(io.Closer).Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := recovered.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if _, active := recovered.Active("payments"); !active {
		t.Fatal("Close lost the in-flight durable activation")
	}
}
