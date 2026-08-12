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
	"github.com/araihu/manja/internal/web"
)

// ErrActivationUnavailable is returned by recovery-only servers. Those
// servers deliberately contain no parser or compiler and can only serve a
// previously published durable snapshot.
var ErrActivationUnavailable = errors.New("catalog activation is unavailable")

var ErrStartupProcessBudget = errors.New("renderer startup process budget exceeded")

type CatalogSource = port.CatalogSource

type ActivationReceipt struct {
	CatalogID           string
	Mount               string
	RevisionID          string
	SnapshotID          string
	StartupProcessBytes uint64
	Degraded            bool
	Diagnostic          string
}

type Server interface {
	Handler() http.Handler
	Recover(context.Context) error
	CheckStartupProcess() (uint64, error)
	Active(string) (ActivationReceipt, bool)
	Activate(context.Context, domain.CatalogCandidate) (ActivationReceipt, error)
}

type catalogParser interface {
	Parse(context.Context, domain.CatalogCandidate) (domain.CatalogIndex, error)
}

type catalogCompiler interface {
	Compile(context.Context, domain.CatalogCandidate, domain.CatalogIndex) (catalog.CompiledSnapshot, error)
}

type server struct {
	config             Config
	configByID         map[string]CatalogConfig
	parsers            map[string]catalogParser
	compilers          map[string]catalogCompiler
	handler            *catalogGateway
	initialize         sync.Mutex
	activation         chan struct{}
	runtime            *catalog.Runtime
	coordinator        *catalogstore.ActivationCoordinator
	generatedDir       string
	measureProcessPeak func() (uint64, error)
}

// NewRecoveryOnly constructs a serving runtime without parsers or compilers.
// Recover must restore every required active snapshot before traffic starts.
func NewRecoveryOnly(config Config) (Server, error) {
	return newServer(config)
}

func newServer(config Config) (*server, error) {
	if config.StartupProcessBytes == 0 {
		config.StartupProcessBytes = DefaultStartupProcessBytes
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	configured := make(map[string]CatalogConfig, len(config.Catalogs))
	parsers := make(map[string]catalogParser, len(config.Catalogs))
	compilers := make(map[string]catalogCompiler, len(config.Catalogs))
	mounts := make([]string, len(config.Catalogs))
	for index, configuredCatalog := range config.Catalogs {
		configuredCatalog.CompatibilityAllowlist = append([]byte(nil), configuredCatalog.CompatibilityAllowlist...)
		configured[configuredCatalog.ID] = configuredCatalog
		mounts[index] = configuredCatalog.Mount
	}
	config.Catalogs = append([]CatalogConfig(nil), config.Catalogs...)
	gateway := &catalogGateway{mounts: mounts, assets: web.NewCatalogAssetsHandler(), admissions: newCatalogAdmissionGate()}
	return &server{config: config, configByID: configured, parsers: parsers, compilers: compilers, handler: gateway, activation: make(chan struct{}, 1), measureProcessPeak: processPeakBytes}, nil
}

func (server *server) Handler() http.Handler {
	return server.handler
}

func (server *server) Recover(ctx context.Context) error {
	if _, err := server.CheckStartupProcess(); err != nil {
		return err
	}
	if err := server.ensureRuntime(ctx); err != nil {
		return err
	}
	_, err := server.CheckStartupProcess()
	return err
}

func (server *server) CheckStartupProcess() (uint64, error) {
	return server.checkStartupProcessWithReservation(0)
}

func (server *server) checkStartupProcessWithReservation(reserved uint64) (uint64, error) {
	peak, err := server.measureProcessPeak()
	if err != nil {
		return 0, fmt.Errorf("measure renderer startup process: %w", err)
	}
	limit := server.config.StartupProcessBytes
	if peak > limit || reserved > limit-peak {
		return peak, fmt.Errorf("%w: peak=%d reserved=%d limit=%d", ErrStartupProcessBudget, peak, reserved, limit)
	}
	return peak, nil
}

func (server *server) Active(catalogID string) (ActivationReceipt, bool) {
	configured, exists := server.configByID[catalogID]
	if !exists || server.runtime == nil {
		return ActivationReceipt{}, false
	}
	state, active := server.runtime.Table().Mounts[configured.Mount]
	if !active {
		return ActivationReceipt{CatalogID: catalogID, Mount: configured.Mount}, false
	}
	peak, _ := processPeakBytes()
	return ActivationReceipt{
		CatalogID: catalogID, Mount: configured.Mount,
		RevisionID: state.Active.Manifest.Identity.RevisionID,
		SnapshotID: string(state.Active.ID), StartupProcessBytes: peak,
	}, true
}

func (server *server) Activate(ctx context.Context, candidate domain.CatalogCandidate) (ActivationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if len(server.parsers) == 0 || len(server.compilers) == 0 {
		return ActivationReceipt{}, ErrActivationUnavailable
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
	select {
	case server.activation <- struct{}{}:
		defer func() { <-server.activation }()
	case <-ctx.Done():
		return ActivationReceipt{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if err := server.ensureRuntime(ctx); err != nil {
		return ActivationReceipt{}, err
	}
	if _, err := server.CheckStartupProcess(); err != nil {
		return ActivationReceipt{}, err
	}
	index, err := server.parsers[candidate.ID].Parse(ctx, candidate)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("parse catalog %q: %w", candidate.ID, err)
	}
	if _, err := server.CheckStartupProcess(); err != nil {
		return ActivationReceipt{}, err
	}
	compiled, err := server.compilers[candidate.ID].Compile(ctx, candidate, index)
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("compile catalog %q: %w", candidate.ID, err)
	}
	if _, err := server.CheckStartupProcess(); err != nil {
		return ActivationReceipt{}, err
	}
	table := server.runtime.Table()
	expectedOld := catalog.SnapshotID("")
	if state, active := table.Mounts[configured.Mount]; active {
		expectedOld = state.Active.ID
	}
	peak, err := server.CheckStartupProcess()
	if err != nil {
		return ActivationReceipt{}, err
	}
	promotionHeld := false
	defer func() {
		if promotionHeld {
			server.handler.endPromotion()
		}
	}()
	receipt, err := server.coordinator.ActivateAdmitted(ctx, configured.Mount, expectedOld, table.Generation, compiled, func() error {
		// Immutable candidate bytes are staged and preflighted. Drain existing
		// request/cache work before Runtime takes its writer lock, then exclude
		// new work through final admission and route publication.
		if err := server.handler.beginPromotion(ctx); err != nil {
			return err
		}
		promotionHeld = true
		return nil
	}, func() error {
		// Runtime has cloned the complete next table and catalogstore has encoded
		// its durable representation. This is the last allocation-heavy boundary.
		measured, admissionErr := server.checkStartupProcessWithReservation(server.handler.flightReservationBytes())
		if admissionErr == nil {
			peak = measured
		}
		return admissionErr
	})
	if err != nil {
		return ActivationReceipt{}, fmt.Errorf("activate catalog %q: %w", candidate.ID, err)
	}
	return ActivationReceipt{CatalogID: candidate.ID, Mount: configured.Mount, RevisionID: candidate.Revision.ID, SnapshotID: string(receipt.SnapshotID), StartupProcessBytes: peak}, nil
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
	presentation := make(map[string]web.CatalogPresentation, len(server.config.Catalogs))
	enhancement := web.CatalogEnhancementPolicy{Disabled: server.config.LocalDocsDisabled, Publications: make(map[string]web.CatalogPublicEligibility, len(server.config.Catalogs))}
	for _, configured := range server.config.Catalogs {
		presentation[configured.Mount] = web.CatalogPresentation{
			Description: configured.SEO.Description, Readme: configured.Readme,
			License:       web.CatalogLicensePresentation{Name: configured.License.Name, URL: configured.License.URL},
			CanonicalBase: configured.SEO.CanonicalBase,
			SocialImage:   configured.SEO.SocialImage, SocialImageMIMEType: socialImageMIMEType(configured.SEO.SocialImage), SocialImageAlt: configured.SEO.SocialImageAlt,
		}
		if configured.LocalDocs.Public || configured.LocalDocs.Anonymous || configured.LocalDocs.PublicationKey != "" {
			enhancement.Publications[configured.Mount] = web.CatalogPublicEligibility{
				CatalogID: configured.ID, PublicationKey: configured.LocalDocs.PublicationKey,
				Public: configured.LocalDocs.Public, Anonymous: configured.LocalDocs.Anonymous,
			}
		}
	}
	organization := web.OrganizationPresentation{
		Title:  server.config.Organization.Title,
		Readme: server.config.Organization.Readme,
		License: web.OrganizationLicensePresentation{
			Name: server.config.Organization.License.Name,
			URL:  server.config.Organization.License.URL,
		},
		Sources: make([]web.OrganizationSourcePresentation, len(server.config.Organization.Sources)),
		SEO: web.CatalogPresentation{
			Description: server.config.Organization.SEO.Description, CanonicalBase: server.config.Organization.SEO.CanonicalBase,
			SocialImage: server.config.Organization.SEO.SocialImage, SocialImageMIMEType: socialImageMIMEType(server.config.Organization.SEO.SocialImage), SocialImageAlt: server.config.Organization.SEO.SocialImageAlt,
		},
	}
	for index, source := range server.config.Organization.Sources {
		organization.Sources[index] = web.OrganizationSourcePresentation{
			Name: source.Name, Kind: source.Kind, Location: source.Location, URL: source.URL,
		}
	}
	server.handler.install(runtime, web.NewCatalogHandlerWithOrganizationAndEnhancement(runtime, coordinator.Store(), presentation, organization, enhancement))
	return nil
}

type catalogGateway struct {
	mutex      sync.RWMutex
	admissions *catalogAdmissionGate
	mounts     []string
	runtime    *catalog.Runtime
	delegate   http.Handler
	assets     http.Handler
}

type catalogFlightReservationReporter interface {
	CatalogFlightReservationBytes() uint64
}

type catalogAdmissionGate struct {
	mutex     sync.Mutex
	readers   uint64
	promoting bool
	changed   chan struct{}
}

func newCatalogAdmissionGate() *catalogAdmissionGate {
	return &catalogAdmissionGate{changed: make(chan struct{})}
}

func (gate *catalogAdmissionGate) notifyLocked() {
	close(gate.changed)
	gate.changed = make(chan struct{})
}

func (gate *catalogAdmissionGate) enter(ctx context.Context) error {
	for {
		gate.mutex.Lock()
		if err := ctx.Err(); err != nil {
			gate.mutex.Unlock()
			return err
		}
		if !gate.promoting {
			gate.readers++
			gate.mutex.Unlock()
			return nil
		}
		changed := gate.changed
		gate.mutex.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (gate *catalogAdmissionGate) leave() {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if gate.readers == 0 {
		panic("renderer: catalog admission gate reader underflow")
	}
	gate.readers--
	if gate.readers == 0 {
		gate.notifyLocked()
	}
}

func (gateway *catalogGateway) beginPromotion(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gate := gateway.admissions
	gate.mutex.Lock()
	if gate.promoting {
		gate.mutex.Unlock()
		return fmt.Errorf("renderer: concurrent catalog promotion")
	}
	gate.promoting = true
	gate.notifyLocked()
	for {
		if err := ctx.Err(); err != nil {
			gate.promoting = false
			gate.notifyLocked()
			gate.mutex.Unlock()
			return err
		}
		if gate.readers == 0 {
			gate.mutex.Unlock()
			return nil
		}
		changed := gate.changed
		gate.mutex.Unlock()
		select {
		case <-ctx.Done():
			gate.mutex.Lock()
		case <-changed:
			gate.mutex.Lock()
		}
	}
}

func (gateway *catalogGateway) endPromotion() {
	gate := gateway.admissions
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	if !gate.promoting {
		panic("renderer: catalog promotion gate is not held")
	}
	gate.promoting = false
	gate.notifyLocked()
}

func (gateway *catalogGateway) flightReservationBytes() uint64 {
	gateway.mutex.RLock()
	defer gateway.mutex.RUnlock()
	reporter, ok := gateway.delegate.(catalogFlightReservationReporter)
	if !ok {
		return 0
	}
	return reporter.CatalogFlightReservationBytes()
}

func (gateway *catalogGateway) install(runtime *catalog.Runtime, delegate http.Handler) {
	gateway.mutex.Lock()
	defer gateway.mutex.Unlock()
	gateway.runtime = runtime
	gateway.delegate = delegate
}

func (gateway *catalogGateway) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if err := gateway.admissions.enter(request.Context()); err != nil {
		http.Error(response, "request canceled", http.StatusRequestTimeout)
		return
	}
	defer gateway.admissions.leave()
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
	if request.URL.Path == "/" || request.URL.Path == "/search" || request.URL.Path == "/search.json" {
		if runtime == nil {
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
