package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web/templates"
)

const catalogProjectionFormat = "projection-v2"

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
	if domain.ValidateCatalogID(eligibility.CatalogID) != nil || domain.ValidateCatalogPublicationKey(eligibility.PublicationKey) != nil {
		return nil
	}
	identity := snapshot.Manifest.Identity
	if snapshot.Manifest.SchemaVersion != 1 || identity.SchemaVersion != 1 || snapshot.Manifest.SnapshotID != snapshot.ID || identity.CatalogID != eligibility.CatalogID || snapshot.Directory.CatalogID != eligibility.CatalogID {
		return nil
	}
	if !sameCatalogChildIdentities(snapshot.Manifest.Children, identity.Children) {
		return nil
	}
	if domain.ValidateCanonicalIdentity("local docs revision id", identity.RevisionID, false) != nil || identity.Versions.ProjectionFormat != catalogProjectionFormat || !isLowerHexDigest(identity.SourceManifestSHA256) {
		return nil
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return nil
	}
	digest := sha256.Sum256(encoded)
	projectionDigest := hex.EncodeToString(digest[:])
	if string(snapshot.ID) != "snapshot-sha256-"+projectionDigest {
		return nil
	}
	publicationBase := mount
	if publicationBase != "/" {
		publicationBase += "/"
	}
	projectionDataBase, err := catalogURL(mount, "snapshots", string(snapshot.ID), "projection-data")
	if err != nil {
		return nil
	}
	projectionManifestURL, err := catalogURL(mount, "snapshots", string(snapshot.ID), "manifest.json")
	if err != nil {
		return nil
	}
	return &templates.CatalogEnhancementDescriptorData{
		SchemaVersion: 1, PublicationKey: eligibility.PublicationKey, PublicationBase: publicationBase,
		SnapshotID: string(snapshot.ID), RevisionID: identity.RevisionID,
		ProjectionFormat: identity.Versions.ProjectionFormat, ProjectionDigest: projectionDigest,
		ProjectionManifestURL: projectionManifestURL, ProjectionDataBase: projectionDataBase + "/",
	}
}

func sameCatalogChildIdentities(left, right []catalog.ChildIdentityV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func isLowerHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
