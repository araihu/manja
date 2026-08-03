package catalog

import "github.com/araihu/manja/domain"

type SnapshotID string

type CompilerVersions struct {
	KinOpenAPIModule   string `json:"kinOpenapiModule"`
	KinOpenAPIChecksum string `json:"kinOpenapiChecksum"`
	CompilerFormat     string `json:"compilerFormat"`
	ProjectionFormat   string `json:"projectionFormat"`
	SearchFormat       string `json:"searchFormat"`
	PartitionPolicy    string `json:"partitionPolicy"`
}

type SourceIdentityV1 struct {
	Role        string `json:"role"`
	DocumentKey string `json:"documentKey"`
	SourcePath  string `json:"sourcePath"`
	Length      uint64 `json:"length"`
	SHA256      string `json:"sha256"`
}

type ChildIdentityV1 struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Length uint64 `json:"length"`
	SHA256 string `json:"sha256"`
}

type SnapshotIdentityV1 struct {
	SchemaVersion          uint32                        `json:"schemaVersion"`
	CatalogID              string                        `json:"catalogId"`
	CatalogTitle           string                        `json:"catalogTitle"`
	Branding               BrandingV1                    `json:"branding"`
	DefaultDocumentKey     string                        `json:"defaultDocumentKey"`
	ProfileID              domain.CompatibilityProfileID `json:"profileId"`
	RevisionKind           domain.CatalogRevisionKind    `json:"revisionKind"`
	RevisionID             string                        `json:"revisionId"`
	CommitSHA              string                        `json:"commitSha"`
	SourceManifestSHA256   string                        `json:"sourceManifestSha256"`
	ProfileAllowlistLength uint64                        `json:"profileAllowlistLength"`
	ProfileAllowlistSHA256 string                        `json:"profileAllowlistSha256"`
	Versions               CompilerVersions              `json:"versions"`
	Bounds                 Bounds                        `json:"bounds"`
	Sources                []SourceIdentityV1            `json:"sources"`
	Children               []ChildIdentityV1             `json:"children"`
}

type ChildArtifact struct {
	Path   string
	Kind   string
	Length uint64
	SHA256 string
	Bytes  []byte
}

type CompiledSnapshot struct {
	ID        SnapshotID
	Identity  SnapshotIdentityV1
	Directory CatalogArtifactV1
	Children  []ChildArtifact
}

type CatalogArtifactV1 struct {
	SchemaVersion      uint32                        `json:"schemaVersion"`
	CatalogID          string                        `json:"catalogId"`
	Title              string                        `json:"title"`
	Branding           BrandingV1                    `json:"branding"`
	DefaultDocumentKey string                        `json:"defaultDocumentKey"`
	ProfileID          domain.CompatibilityProfileID `json:"profileId"`
	Documents          []DocumentDirectoryV1         `json:"documents"`
}

type BrandingV1 struct {
	DisplayName  string `json:"displayName"`
	LogoSrc      string `json:"logoSrc"`
	LogoAlt      string `json:"logoAlt"`
	LogoHomeHref string `json:"logoHomeHref"`
	FaviconHref  string `json:"faviconHref"`
}

type DocumentDirectoryV1 struct {
	Key             string                 `json:"key"`
	SourcePath      string                 `json:"sourcePath"`
	Title           string                 `json:"title"`
	APIVersion      string                 `json:"apiVersion"`
	SourceChild     string                 `json:"sourceChild"`
	ProjectionChild string                 `json:"projectionChild"`
	Operations      []OperationDirectoryV1 `json:"operations"`
	Schemas         []SchemaDirectoryV1    `json:"schemas"`
}

type OperationDirectoryV1 struct {
	DetailID    domain.DetailID `json:"detailId"`
	OperationID string          `json:"operationId"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Href        string          `json:"href"`
	Deprecated  bool            `json:"deprecated"`
	Tags        []string        `json:"tags"`
	Facets      []FacetV1       `json:"facets"`
}

type FacetV1 struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SchemaDirectoryV1 struct {
	DetailID    domain.DetailID `json:"detailId"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Href        string          `json:"href"`
}

type ManifestV1 struct {
	SchemaVersion uint32             `json:"schemaVersion"`
	SnapshotID    SnapshotID         `json:"snapshotId"`
	Identity      SnapshotIdentityV1 `json:"identity"`
	Children      []ChildIdentityV1  `json:"children"`
}
