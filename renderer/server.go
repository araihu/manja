package renderer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

var ErrActivationUnavailable = errors.New("catalog activation is not available until the compiler is installed")

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
	configByID map[string]CatalogConfig
	handler    http.Handler
}

func New(config Config) (Server, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	configured := make(map[string]CatalogConfig, len(config.Catalogs))
	mounts := make([]string, len(config.Catalogs))
	for index, catalog := range config.Catalogs {
		configured[catalog.ID] = catalog
		mounts[index] = catalog.Mount
	}
	return &server{configByID: configured, handler: &unavailableCatalogHandler{mounts: mounts}}, nil
}

func (s *server) Handler() http.Handler {
	return s.handler
}

func (s *server) Activate(ctx context.Context, candidate domain.CatalogCandidate) (ActivationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ActivationReceipt{}, err
	}
	if err := domain.ValidateCatalogCandidate(candidate); err != nil {
		return ActivationReceipt{}, err
	}
	configured, exists := s.configByID[candidate.ID]
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
	return ActivationReceipt{}, ErrActivationUnavailable
}

type unavailableCatalogHandler struct {
	mounts []string
}

func (handler unavailableCatalogHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	for _, mount := range handler.mounts {
		if mount == "/" || request.URL.Path == mount || strings.HasPrefix(request.URL.Path, mount+"/") {
			http.Error(response, "catalog unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	http.NotFound(response, request)
}

var _ Server = (*server)(nil)
