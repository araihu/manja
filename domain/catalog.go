package domain

const (
	maxCatalogDocuments     = 256
	maxCatalogDocumentBytes = 8 << 20
)

type CompatibilityProfileID string

const (
	CompatibilityProfileStrict     CompatibilityProfileID = "strict-v1"
	CompatibilityProfileKubernetes CompatibilityProfileID = "kubernetes-v3-v1"
)

type CatalogRevisionKind string

const (
	CatalogRevisionFiles CatalogRevisionKind = "files"
	CatalogRevisionGit   CatalogRevisionKind = "git"
)

type CatalogFormat string

const (
	CatalogFormatJSON CatalogFormat = "json"
	CatalogFormatYAML CatalogFormat = "yaml"
)

type CatalogRevision struct {
	Kind           CatalogRevisionKind
	ID             string
	CommitSHA      string
	ManifestDigest string
}

type CatalogCandidate struct {
	ID                 string
	Title              string
	Branding           DocsBranding
	DefaultDocumentKey string
	ProfileID          CompatibilityProfileID
	Revision           CatalogRevision
	Documents          []CatalogDocument
}

type CatalogDocument struct {
	Key        string
	SourcePath string
	Format     CatalogFormat
	Bytes      []byte
}

type CatalogIndex struct {
	CatalogID  string
	RevisionID string
	Title      string
	Branding   DocsBranding
	ProfileID  CompatibilityProfileID
	Documents  []CatalogDocumentIndex
}

type CatalogDocumentIndex struct {
	Key        string
	SourcePath string
	Index      SpecIndex
}

type Facet struct {
	Name  string
	Value string
}
