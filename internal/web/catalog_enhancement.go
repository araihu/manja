package web

import (
	"net/http"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/localdocs"
	"github.com/araihu/manja/internal/web/templates"
)

// CatalogEnhancementPolicy carries only composition-authoritative public
// eligibility. Snapshot contents cannot grant themselves anonymous access.
type CatalogEnhancementPolicy struct {
	Disabled     bool
	Publications map[string]CatalogPublicEligibility
}

type CatalogPublicEligibility struct {
	CatalogID      string
	PublicationKey string
	Public         bool
	Anonymous      bool
}

func validCatalogEnhancementPolicy(policy CatalogEnhancementPolicy) bool {
	publicationKeys := make(map[string]struct{}, len(policy.Publications))
	for _, eligibility := range policy.Publications {
		if !eligibility.Public || !eligibility.Anonymous {
			continue
		}
		if domain.ValidateCatalogPublicationKey(eligibility.PublicationKey) != nil {
			return false
		}
		if _, exists := publicationKeys[eligibility.PublicationKey]; exists {
			return false
		}
		publicationKeys[eligibility.PublicationKey] = struct{}{}
	}
	return true
}

func (handler *CatalogHandler) catalogEnhancementDescriptor(snapshot catalog.RuntimeSnapshot, mount string) *templates.CatalogEnhancementDescriptorData {
	if handler.enhancement.Disabled {
		return nil
	}
	eligibility, exists := handler.enhancement.Publications[mount]
	if !exists || !eligibility.Public || !eligibility.Anonymous {
		return nil
	}
	descriptor, ok := localdocs.PrepareDescriptor(localdocs.PublicEligibility{
		CatalogID: eligibility.CatalogID, PublicationKey: eligibility.PublicationKey,
		Public: eligibility.Public, Anonymous: eligibility.Anonymous,
	}, snapshot, mount)
	if !ok {
		return nil
	}
	return &descriptor
}

// catalogEnhancementWithdrawalState makes a normal SSR response authoritative
// for a previously configured local-docs publication. The response remains a
// usable server-rendered page; the state only tells a known worker to tombstone
// its local generation before it considers an offline fallback.
func (handler *CatalogHandler) catalogEnhancementWithdrawalState(snapshot catalog.RuntimeSnapshot, mount string) string {
	if handler.enhancement.Disabled {
		return "revoked"
	}
	eligibility, exists := handler.enhancement.Publications[mount]
	if !exists || eligibility.CatalogID == "" && eligibility.PublicationKey == "" {
		return "deleted"
	}
	if !eligibility.Public || !eligibility.Anonymous {
		return "private"
	}
	if handler.catalogEnhancementDescriptor(snapshot, mount) == nil {
		return "revoked"
	}
	return ""
}

// serveOfflineShell is the production endpoint for the canonical anonymous
// reader shell. It is deliberately narrower than the normal catalog route:
// only a composition-authorized public+anonymous mount can emit shell bytes,
// while the global enhancement kill switch emits an authoritative tombstone
// response that a Service Worker can persist.
func (handler *CatalogHandler) serveOfflineShell(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	if handler.enhancement.Disabled {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Manja-Publication-State", "revoked")
		http.Error(response, "offline shell is revoked", http.StatusGone)
		return
	}
	eligibility, exists := handler.enhancement.Publications[mount]
	if !exists || !eligibility.Public || !eligibility.Anonymous {
		http.NotFound(response, request)
		return
	}
	handler.serveOverview(response, request, snapshot, mount)
}
