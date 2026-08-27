package localdocs

import (
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

// DescriptorV1 carries the immutable, non-secret inputs required to admit a
// public documentation projection for local enhancement.
type DescriptorV1 struct {
	SchemaVersion         uint32              `json:"schemaVersion"`
	CatalogID             string              `json:"catalogId"`
	PublicationKey        string              `json:"publicationKey"`
	Public                bool                `json:"public"`
	Anonymous             bool                `json:"anonymous"`
	PublicationBase       string              `json:"publicationBase"`
	SnapshotID            string              `json:"snapshotId"`
	RevisionID            string              `json:"revisionId"`
	ProjectionFormat      string              `json:"projectionFormat"`
	ProjectionDigest      string              `json:"projectionDigest"`
	ProjectionManifestURL string              `json:"projectionManifestUrl"`
	CatalogURL            string              `json:"catalogUrl"`
	SearchDataBase        string              `json:"searchDataBase"`
	ProjectionDataBase    string              `json:"projectionDataBase"`
	Static                *StaticDescriptorV1 `json:"static,omitempty"`
}

type StaticDescriptorV1 struct {
	DeploymentBase    string `json:"deploymentBase"`
	WorkerURL         string `json:"workerUrl"`
	WorkerScope       string `json:"workerScope"`
	OfflineShellURL   string `json:"offlineShellUrl"`
	ExportManifestURL string `json:"exportManifestUrl"`
}

// PublicEligibility is composition-owned authority. A snapshot cannot make
// itself eligible for local enhancement by changing its own projection data.
type PublicEligibility struct {
	CatalogID      string
	PublicationKey string
	Public         bool
	Anonymous      bool
}

// PrepareDescriptor copies only immutable, non-secret identity and route
// values into a descriptor. Any mismatch returns zero state and false so SSR
// remains the only response path.
func PrepareDescriptor(eligibility PublicEligibility, snapshot catalog.RuntimeSnapshot, publicationBase string) (DescriptorV1, bool) {
	if !eligibility.Public || !eligibility.Anonymous || domain.ValidateCatalogID(eligibility.CatalogID) != nil || domain.ValidateCatalogPublicationKey(eligibility.PublicationKey) != nil {
		return DescriptorV1{}, false
	}
	base, ok := descriptorPublicationBase(publicationBase)
	if !ok {
		return DescriptorV1{}, false
	}
	identity := snapshot.Manifest.Identity
	if snapshot.Manifest.SchemaVersion != 1 || identity.SchemaVersion != 1 || snapshot.Manifest.SnapshotID != snapshot.ID || identity.CatalogID != eligibility.CatalogID || snapshot.Directory.CatalogID != eligibility.CatalogID || !sameChildIdentities(snapshot.Manifest.Children, identity.Children) {
		return DescriptorV1{}, false
	}
	if domain.ValidateCanonicalIdentity("local docs revision id", identity.RevisionID, false) != nil || identity.Versions.ProjectionFormat != projectionFormatV2 || !lowerHexSHA256(identity.SourceManifestSHA256) {
		return DescriptorV1{}, false
	}
	identityBytes, err := marshalIdentity(identity)
	if err != nil {
		return DescriptorV1{}, false
	}
	projectionDigest := sha256Hex(identityBytes)
	if string(snapshot.ID) != "snapshot-sha256-"+projectionDigest {
		return DescriptorV1{}, false
	}
	manifestURL := descriptorURL(base, "snapshots", string(snapshot.ID), "manifest.json")
	catalogURL := descriptorURL(base, "snapshots", string(snapshot.ID), "catalog.json")
	searchDataBase := descriptorURL(base, "snapshots", string(snapshot.ID), "search-data") + "/"
	projectionDataBase := descriptorURL(base, "snapshots", string(snapshot.ID), "projection-data") + "/"
	return DescriptorV1{
		SchemaVersion: 1, CatalogID: eligibility.CatalogID, PublicationKey: eligibility.PublicationKey, Public: true, Anonymous: true, PublicationBase: base,
		SnapshotID: string(snapshot.ID), RevisionID: identity.RevisionID, ProjectionFormat: identity.Versions.ProjectionFormat,
		ProjectionDigest: projectionDigest, ProjectionManifestURL: manifestURL, CatalogURL: catalogURL,
		SearchDataBase: searchDataBase, ProjectionDataBase: projectionDataBase,
	}, true
}

// PrepareStaticDescriptor treats export invocation as disclosure authority.
// Catalog visibility is intentionally not an input.
func PrepareStaticDescriptor(catalogID string, snapshot catalog.RuntimeSnapshot, publicationBase, deploymentBase string) (DescriptorV1, bool) {
	descriptor, ok := PrepareDescriptor(PublicEligibility{CatalogID: catalogID, PublicationKey: catalogID, Public: true, Anonymous: true}, snapshot, publicationBase)
	deployment, deploymentOK := descriptorPublicationBase(deploymentBase)
	if !ok || !deploymentOK || !strings.HasPrefix(descriptor.PublicationBase, deployment) {
		return DescriptorV1{}, false
	}
	descriptor.Static = &StaticDescriptorV1{
		DeploymentBase:    deployment,
		WorkerURL:         descriptorURL(deployment, "sw.js"),
		WorkerScope:       deployment,
		OfflineShellURL:   descriptorURL(descriptor.PublicationBase, "_manja", "offline-shell") + "/",
		ExportManifestURL: descriptorURL(deployment, "_manja", "export.json"),
	}
	return descriptor, true
}
