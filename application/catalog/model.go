package catalog

import (
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

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
	SearchChild        string                        `json:"searchChild"`
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
	Key              string                      `json:"key"`
	SourcePath       string                      `json:"sourcePath"`
	Title            string                      `json:"title"`
	APIVersion       string                      `json:"apiVersion"`
	Branding         BrandingV1                  `json:"branding"`
	Overview         projection.Overview         `json:"overview"`
	SourceChild      string                      `json:"sourceChild"`
	SchemaNodeShards []ShardReferenceV1          `json:"schemaNodeShards"`
	Operations       []OperationDirectoryV1      `json:"operations"`
	Schemas          []SchemaDirectoryV1         `json:"schemas"`
	SecuritySchemes  []SecuritySchemeDirectoryV1 `json:"securitySchemes,omitempty"`
}

type SecuritySchemeDirectoryV1 struct {
	Name             string `json:"name"`
	Anchor           string `json:"anchor"`
	Type             string `json:"type"`
	Description      string `json:"description,omitempty"`
	ParameterName    string `json:"parameterName,omitempty"`
	In               string `json:"in,omitempty"`
	Scheme           string `json:"scheme,omitempty"`
	BearerFormat     string `json:"bearerFormat,omitempty"`
	OpenIDConnectURL string `json:"openIdConnectUrl,omitempty"`
}

type OperationDirectoryV1 struct {
	DetailID    domain.DetailID `json:"detailId"`
	OperationID string          `json:"operationId"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Href        string          `json:"href"`
	DetailChild string          `json:"detailChild"`
	Deprecated  bool            `json:"deprecated"`
	Tags        []string        `json:"tags"`
	Facets      []FacetV1       `json:"facets"`
}

type FacetV1 struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SchemaDirectoryV1 struct {
	DetailID         domain.DetailID `json:"detailId"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Href             string          `json:"href"`
	DetailChild      string          `json:"detailChild"`
	CanonicalSHA256  string          `json:"canonicalSHA256"`
	ProjectionSHA256 string          `json:"projectionSHA256"`
}

type ShardReferenceV1 struct {
	Path         string `json:"path"`
	FirstOrdinal uint32 `json:"firstOrdinal"`
	LastOrdinal  uint32 `json:"lastOrdinal"`
	Records      uint32 `json:"records"`
	Length       uint64 `json:"length"`
	SHA256       string `json:"sha256"`
}

type DetailShardV1 struct {
	SchemaVersion uint32           `json:"schemaVersion"`
	DocumentKey   string           `json:"documentKey"`
	Records       []DetailRecordV1 `json:"records"`
}

type DetailRecordV1 struct {
	ID        domain.DetailID             `json:"id"`
	Kind      string                      `json:"kind"`
	Operation *projection.OperationDetail `json:"operation,omitempty"`
	Schema    *projection.SchemaDetail    `json:"schema,omitempty"`
}

type SchemaNodeShardV1 struct {
	SchemaVersion uint32                  `json:"schemaVersion"`
	DocumentKey   string                  `json:"documentKey"`
	FirstOrdinal  uint32                  `json:"firstOrdinal"`
	Nodes         []projection.SchemaNode `json:"nodes"`
}

type DocumentArtifacts struct {
	Directory DocumentDirectoryV1
	Children  []ChildArtifact
	Usage     BudgetUsage
}

type SearchDirectoryV1 struct {
	SchemaVersion   uint32                           `json:"schemaVersion"`
	SearchVersion   uint32                           `json:"searchVersion"`
	ExactBuckets    []SearchExactBucketReferenceV1   `json:"exactBuckets"`
	TokenRoutes     []SearchPostingRouteV1           `json:"tokenRoutes"`
	TrigramRoutes   []SearchPostingRouteV1           `json:"trigramRoutes"`
	PostingSegments []SearchSegmentReferenceV1       `json:"postingSegments"`
	TrigramSegments []SearchSegmentReferenceV1       `json:"trigramSegments"`
	RecordSegments  []SearchRecordSegmentReferenceV1 `json:"recordSegments"`
	Ranks           []SearchRankRecordV1             `json:"ranks"`
}

type SearchRankRecordV1 struct {
	Title string `json:"t"`
}

type SearchExactEntryV1 struct {
	Key     string               `json:"key"`
	Matches []SearchExactMatchV1 `json:"matches"`
}

type SearchExactMatchV1 struct {
	Record   uint32 `json:"record"`
	Priority uint8  `json:"priority"`
}

type SearchExactBucketReferenceV1 struct {
	Prefix string `json:"prefix"`
	SearchSegmentReferenceV1
}

type SearchPostingRouteV1 struct {
	Key     string `json:"key"`
	Segment uint16 `json:"segment"`
}

type SearchSegmentReferenceV1 struct {
	Path     string `json:"path"`
	Entries  uint32 `json:"entries"`
	Postings uint32 `json:"postings"`
	Length   uint64 `json:"length"`
	SHA256   string `json:"sha256"`
}

type SearchRecordSegmentReferenceV1 struct {
	Path        string `json:"path"`
	FirstRecord uint32 `json:"firstRecord"`
	Records     uint32 `json:"records"`
	Length      uint64 `json:"length"`
	SHA256      string `json:"sha256"`
}

type SearchExactSegmentV1 struct {
	SchemaVersion uint32               `json:"schemaVersion"`
	SearchVersion uint32               `json:"searchVersion"`
	Entries       []SearchExactEntryV1 `json:"entries"`
}

type SearchPostingSegmentV1 struct {
	SchemaVersion uint32                 `json:"schemaVersion"`
	SearchVersion uint32                 `json:"searchVersion"`
	Entries       []SearchPostingEntryV1 `json:"entries"`
}

type SearchPostingEntryV1 struct {
	Key     string   `json:"key"`
	Records []uint32 `json:"records"`
}

type SearchRecordSegmentV1 struct {
	SchemaVersion uint32           `json:"schemaVersion"`
	SearchVersion uint32           `json:"searchVersion"`
	FirstRecord   uint32           `json:"firstRecord"`
	Records       []SearchRecordV1 `json:"records"`
}

type SearchRecordV1 struct {
	DetailID    domain.DetailID `json:"detailId"`
	DocumentKey string          `json:"documentKey"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Href        string          `json:"href"`
	OperationID string          `json:"operationId"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	SchemaName  string          `json:"schemaName"`
	Occurrences uint32          `json:"occurrences"`
	Documents   []string        `json:"documents"`
}

type SearchArtifacts struct {
	Directory SearchDirectoryV1
	Children  []ChildArtifact
	Usage     BudgetUsage
}

type ManifestV1 struct {
	SchemaVersion uint32             `json:"schemaVersion"`
	SnapshotID    SnapshotID         `json:"snapshotId"`
	Identity      SnapshotIdentityV1 `json:"identity"`
	Children      []ChildIdentityV1  `json:"children"`
}
