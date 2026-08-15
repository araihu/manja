package web

import (
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
