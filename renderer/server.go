package renderer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogstore"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	"github.com/araihu/manja/internal/web"
)

// ErrActivationUnavailable remains for source compatibility with the early
// facade, but complete renderer builds now install the compiler at first use.
var ErrActivationUnavailable = errors.New("catalog activation is unavailable")

type CatalogSource = port.CatalogSource

type ActivationReceipt struct {
	CatalogID  string
	Mount      string
	RevisionID string
	SnapshotID string
}

type Server interface {
	Handler() http.Handler
	Activate(context.Context, domain.CatalogCandidate) (ActivationReceipt, error)
}

type server struct {
	config       Config
	configByID   map[string]CatalogConfig
	parsers      map[string]*openapiadapter.CatalogParser
	compilers    map[string]*catalog.Compiler
	handler      *catalogGateway
	initialize   sync.Mutex
	runtime      *catalog.Runtime
	coordinator  *catalogstore.ActivationCoordinator
	generatedDir string
}

func New(config Config) (Server, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	configured := make(map[string]CatalogConfig, len(config.Catalogs))
	parsers := make(map[string]*openapiadapter.CatalogParser, len(config.Catalogs))
	compilers := make(map[string]*catalog.Compiler, len(config.Catalogs))
	mounts := make([]string, len(config.Catalogs))
	for index, configuredCatalog := range config.Catalogs {
		configuredCatalog.CompatibilityAllowlist = append([]byte(nil), configuredCatalog.CompatibilityAllowlist...)
		configured[configuredCatalog.ID] = configuredCatalog
		mounts[index] = configuredCatalog.Mount
		parser, err := openapiadapter.NewCatalogParser(configuredCatalog.CompatibilityAllowlist)
		if err != nil {
			return nil, fmt.Errorf("catalog %q compatibility allowlist: %w", configuredCatalog.ID, err)
		}
		options := catalog.DefaultCompilerOptions()
		options.ProfileAllowlist = configuredCatalog.CompatibilityAllowlist
		compiler, err := catalog.NewCompiler(options)
		if err != nil {
			return nil, fmt.Errorf("catalog %q compiler: %w", configuredCatalog.ID, err)
		}
		parsers[configuredCatalog.ID] = parser
		compilers[configuredCatalog.ID] = compiler
	}
	config.Catalogs = append([]CatalogConfig(nil), config.Catalogs...)
	gateway := &catalogGateway{mounts: mounts, assets: web.NewCatalogAssetsHandler()}
	return &server{config: config, configByID: configured, parsers: parsers, compilers: compilers, handler: gateway}, nil
}

func (server *server) Handler() http.Handler {
	return server.handler
}

func (server *server) Activate(ctx context.Context, candidate domain.CatalogCandidate) (ActivationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if err := domain.ValidateCatalogCandidate(candidate); err != nil {
		return ActivationReceipt{}, err
	}
	configured, exists := server.configByID[candidate.ID]
	if !exists {
		return ActivationReceipt{}, fmt.Errorf("catalog %q is not configured", candidate.ID)
	}
	if candidate.ProfileID != configured.ProfileID {
		return ActivationReceipt{}, fmt.Errorf("catalog %q profile %q does not match configured profile %q", candidate.ID, candidate.ProfileID, configured.ProfileID)
	}
	if configured.DefaultDocumentKey != "" {
		found := false
		for _, document := range candidate.Documents {
			if document.Key == configured.DefaultDocumentKey {
				found = true
				break
			}
		}
		if !found {
			return ActivationReceipt{}, fmt.Errorf("catalog %q does not contain configured default document %q", candidate.ID, configured.DefaultDocumentKey)
		}
	}
	if err := server.ensureRuntime(ctx); err != nil {
		return ActivationReceipt{}, err
	}
	index, err := server.parsers[candidate.ID].Parse(ctx, candidate)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("parse catalog %q: %w", candidate.ID, err)
	}
	compiled, err := server.compilers[candidate.ID].Compile(ctx, candidate, index)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("compile catalog %q: %w", candidate.ID, err)
	}
	table := server.runtime.Table()
	expectedOld := catalog.SnapshotID("")
	if state, active := table.Mounts[configured.Mount]; active {
		expectedOld = state.Active.ID
	}
	receipt, err := server.coordinator.Activate(ctx, configured.Mount, expectedOld, table.Generation, compiled)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("activate catalog %q: %w", candidate.ID, err)
	}
	return ActivationReceipt{CatalogID: candidate.ID, Mount: configured.Mount, RevisionID: candidate.Revision.ID, SnapshotID: string(receipt.SnapshotID)}, nil
}

func (server *server) ensureRuntime(ctx context.Context) error {
	server.initialize.Lock()
	defer server.initialize.Unlock()
	if server.runtime != nil {
		return nil
	}
	dataDir := server.config.DataDir
	if dataDir == "" {
		generated, err := os.MkdirTemp("", "manja-renderer-")
		if err != nil {
			return fmt.Errorf("create renderer data directory: %w", err)
		}
		dataDir = generated
		server.generatedDir = generated
	}
	runtime := catalog.NewRuntime(1)
	coordinator, err := catalogstore.OpenActivationCoordinator(ctx, dataDir, runtime)
	if err != nil {
		if server.generatedDir != "" {
			_ = os.RemoveAll(server.generatedDir)
			server.generatedDir = ""
		}
		return err
	}
	server.runtime = runtime
	server.coordinator = coordinator
	server.handler.install(runtime, web.NewCatalogHandler(runtime, coordinator.Store()))
	return nil
}

type catalogGateway struct {
	mutex    sync.RWMutex
	mounts   []string
	runtime  *catalog.Runtime
	delegate http.Handler
	assets   http.Handler
}

func (gateway *catalogGateway) install(runtime *catalog.Runtime, delegate http.Handler) {
	gateway.mutex.Lock()
	defer gateway.mutex.Unlock()
	gateway.runtime = runtime
	gateway.delegate = delegate
}

func (gateway *catalogGateway) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/assets/") || strings.HasPrefix(request.URL.Path, "/manja-assets/") {
		gateway.assets.ServeHTTP(response, request)
		return
	}
	gateway.mutex.RLock()
	runtime, delegate := gateway.runtime, gateway.delegate
	gateway.mutex.RUnlock()
	if web.IsCatalogComponentPath(request.URL.Path) {
		if runtime == nil || delegate == nil {
			http.Error(response, "catalog unavailable", http.StatusServiceUnavailable)
			return
		}
		delegate.ServeHTTP(response, request)
		return
	}
	for _, mount := range gateway.mounts {
		if mount != "/" && request.URL.Path != mount && !strings.HasPrefix(request.URL.Path, mount+"/") {
			continue
		}
		if runtime == nil {
			http.Error(response, "catalog unavailable", http.StatusServiceUnavailable)
			return
		}
		if !runtime.HasMount(mount) {
			http.Error(response, "catalog unavailable", http.StatusServiceUnavailable)
			return
		}
		delegate.ServeHTTP(response, request)
		return
	}
	http.NotFound(response, request)
}

var _ Server = (*server)(nil)
