package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

type CompilerOptions struct {
	Versions         CompilerVersions
	Bounds           Bounds
	ProfileAllowlist []byte
}

type Compiler struct {
	options CompilerOptions
}

func DefaultCompilerOptions() CompilerOptions {
	return CompilerOptions{
		Versions: CompilerVersions{
			KinOpenAPIModule:   "github.com/getkin/kin-openapi@v0.140.0",
			KinOpenAPIChecksum: "h1:JFn675aXRFjyiZKa/BFWploGldQlI0gobp4J5k0EZ2g=",
			CompilerFormat:     "catalog-compiler-v1", ProjectionFormat: "projection-v2",
			SearchFormat: "catalog-search-v1", PartitionPolicy: "catalog-partition-v1",
		},
		Bounds: DefaultBounds(),
	}
}

func NewCompiler(options CompilerOptions) (*Compiler, error) {
	versions := []string{
		options.Versions.KinOpenAPIModule, options.Versions.KinOpenAPIChecksum, options.Versions.CompilerFormat,
		options.Versions.ProjectionFormat, options.Versions.SearchFormat, options.Versions.PartitionPolicy,
	}
	for _, version := range versions {
		if strings.TrimSpace(version) == "" {
			return nil, fmt.Errorf("compiler version identities are required")
		}
	}
	if err := options.Bounds.Validate(BudgetUsage{}); err != nil {
		return nil, err
	}
	options.ProfileAllowlist = append([]byte(nil), options.ProfileAllowlist...)
	return &Compiler{options: options}, nil
}

func (compiler *Compiler) Compile(ctx context.Context, candidate domain.CatalogCandidate, index domain.CatalogIndex) (CompiledSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return CompiledSnapshot{}, err
	}
	if err := domain.ValidateCatalogCandidate(candidate); err != nil {
		return CompiledSnapshot{}, err
	}
	if err := domain.ValidateCatalogIndex(index); err != nil {
		return CompiledSnapshot{}, err
	}
	if err := validateCompilerAlignment(candidate, index); err != nil {
		return CompiledSnapshot{}, err
	}

	documents := append([]domain.CatalogDocument(nil), candidate.Documents...)
	sort.Slice(documents, func(i, j int) bool { return documents[i].Key < documents[j].Key })
	indexByKey := make(map[string]domain.CatalogDocumentIndex, len(index.Documents))
	for _, document := range index.Documents {
		indexByKey[document.Key] = document
	}
	directory := CatalogArtifactV1{
		SchemaVersion: 1, CatalogID: candidate.ID, Title: candidate.Title,
		Branding:           candidateBranding(candidate.Branding),
		DefaultDocumentKey: candidate.DefaultDocumentKey, ProfileID: candidate.ProfileID,
		Documents: make([]DocumentDirectoryV1, 0, len(documents)),
	}
	children := make([]ChildArtifact, 0, 4*len(documents)+len(candidate.SupportFiles)+2)
	sources := make([]SourceIdentityV1, 0, len(documents)+len(candidate.SupportFiles))
	usage := BudgetUsage{Catalogs: 1, Documents: uint64(len(documents)), Kubernetes: candidate.ProfileID == domain.CompatibilityProfileKubernetes}

	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return CompiledSnapshot{}, err
		}
		indexed := indexByKey[document.Key]
		sourcePath := "sources/" + document.Key + catalogSourceExtension(document.Format)
		sourceChild, err := newChild(sourcePath, "source", document.Bytes)
		if err != nil {
			return CompiledSnapshot{}, err
		}
		children = append(children, sourceChild)
		sources = append(sources, sourceIdentity("document", document.Key, document.SourcePath, document.Bytes))
		usage.SourceBytes += uint64(len(document.Bytes))
		if uint64(len(document.Bytes)) > usage.SourceDocumentBytes {
			usage.SourceDocumentBytes = uint64(len(document.Bytes))
		}

		projectedIndex := indexed.Index
		projectedIndex.ProjectID = candidate.ID + "-" + document.Key
		projectedIndex.RevisionID = candidate.Revision.ID
		projected, err := (projection.Builder{}).Build(ctx, projectedIndex)
		if err != nil {
			return CompiledSnapshot{}, fmt.Errorf("build projection for %q: %w", document.Key, err)
		}
		documentDirectory, err := buildDocumentDirectory(candidate.ID, document, indexed.Index, sourcePath)
		if err != nil {
			return CompiledSnapshot{}, err
		}
		documentDirectory.Overview = projected.Overview
		partitioned, err := PartitionDocument(document.Key, projected, documentDirectory, DefaultPartitionLimits(compiler.options.Bounds))
		if err != nil {
			return CompiledSnapshot{}, fmt.Errorf("partition projection for %q: %w", document.Key, err)
		}
		children = append(children, partitioned.Children...)
		usage.ProjectionBytes += partitioned.Usage.ProjectionBytes
		if partitioned.Usage.DetailShardRecords > usage.DetailShardRecords {
			usage.DetailShardRecords = partitioned.Usage.DetailShardRecords
		}
		if partitioned.Usage.DetailShardBytes > usage.DetailShardBytes {
			usage.DetailShardBytes = partitioned.Usage.DetailShardBytes
		}
		if partitioned.Usage.SchemaNodeShardRecords > usage.SchemaNodeShardRecords {
			usage.SchemaNodeShardRecords = partitioned.Usage.SchemaNodeShardRecords
		}
		if partitioned.Usage.SchemaNodeShardBytes > usage.SchemaNodeShardBytes {
			usage.SchemaNodeShardBytes = partitioned.Usage.SchemaNodeShardBytes
		}
		usage.Operations += uint64(len(indexed.Index.Operations))
		usage.Schemas += uint64(len(indexed.Index.Schemas))
		directory.Documents = append(directory.Documents, partitioned.Directory)
	}

	supports := append([]domain.CatalogSupportFile(nil), candidate.SupportFiles...)
	sort.Slice(supports, func(i, j int) bool { return supports[i].SourcePath < supports[j].SourcePath })
	for _, support := range supports {
		child, err := newChild("support/"+support.SourcePath, "support", support.Bytes)
		if err != nil {
			return CompiledSnapshot{}, err
		}
		children = append(children, child)
		sources = append(sources, sourceIdentity("support", "", support.SourcePath, support.Bytes))
		usage.SourceBytes += uint64(len(support.Bytes))
		if uint64(len(support.Bytes)) > usage.SourceDocumentBytes {
			usage.SourceDocumentBytes = uint64(len(support.Bytes))
		}
	}
	directoryBytes, err := json.Marshal(directory)
	if err != nil {
		return CompiledSnapshot{}, fmt.Errorf("encode catalog directory: %w", err)
	}
	directoryChild, err := newChild("catalog.json", "catalog", directoryBytes)
	if err != nil {
		return CompiledSnapshot{}, err
	}
	children = append(children, directoryChild)
	usage.DirectoryBytes = uint64(len(directoryBytes))
	usage.StartupCatalogBytes = uint64(len(directoryBytes))
	usage.Children = uint64(len(children) + 1)
	if err := compiler.options.Bounds.Validate(usage); err != nil {
		return CompiledSnapshot{}, err
	}

	identities, err := childIdentities(children)
	if err != nil {
		return CompiledSnapshot{}, err
	}
	profileAllowlist := []byte(nil)
	if candidate.ProfileID == domain.CompatibilityProfileKubernetes {
		if len(compiler.options.ProfileAllowlist) == 0 {
			return CompiledSnapshot{}, fmt.Errorf("Kubernetes compiler requires exact profile allowlist bytes")
		}
		profileAllowlist = compiler.options.ProfileAllowlist
	}
	allowlistDigest := sha256.Sum256(profileAllowlist)
	identity := SnapshotIdentityV1{
		SchemaVersion: 1, CatalogID: candidate.ID, CatalogTitle: candidate.Title,
		Branding:           candidateBranding(candidate.Branding),
		DefaultDocumentKey: candidate.DefaultDocumentKey, ProfileID: candidate.ProfileID,
		RevisionKind: candidate.Revision.Kind, RevisionID: candidate.Revision.ID,
		CommitSHA: candidate.Revision.CommitSHA, SourceManifestSHA256: candidate.Revision.ManifestDigest,
		ProfileAllowlistLength: uint64(len(profileAllowlist)),
		ProfileAllowlistSHA256: hex.EncodeToString(allowlistDigest[:]),
		Versions:               compiler.options.Versions, Bounds: compiler.options.Bounds,
		Sources: sources, Children: identities,
	}
	snapshotID, _, err := snapshotIdentity(identity)
	if err != nil {
		return CompiledSnapshot{}, err
	}
	manifestBytes, err := json.Marshal(ManifestV1{SchemaVersion: 1, SnapshotID: snapshotID, Identity: identity, Children: identities})
	if err != nil {
		return CompiledSnapshot{}, fmt.Errorf("encode snapshot manifest: %w", err)
	}
	manifestChild, err := newChild("manifest.json", "manifest", manifestBytes)
	if err != nil {
		return CompiledSnapshot{}, err
	}
	children = append(children, manifestChild)
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	for _, child := range children {
		usage.SnapshotBytes += child.Length
	}
	if err := compiler.options.Bounds.Validate(usage); err != nil {
		return CompiledSnapshot{}, err
	}
	return CompiledSnapshot{ID: snapshotID, Identity: identity, Directory: directory, Children: children}, nil
}

func validateCompilerAlignment(candidate domain.CatalogCandidate, index domain.CatalogIndex) error {
	if candidate.ID != index.CatalogID || candidate.Revision.ID != index.RevisionID || candidate.Title != index.Title || candidate.Branding != index.Branding || candidate.ProfileID != index.ProfileID || len(candidate.Documents) != len(index.Documents) {
		return fmt.Errorf("catalog candidate and parsed index do not align")
	}
	indexed := make(map[string]string, len(index.Documents))
	for _, document := range index.Documents {
		indexed[document.Key] = document.SourcePath
	}
	for _, document := range candidate.Documents {
		if sourcePath, exists := indexed[document.Key]; !exists || sourcePath != document.SourcePath {
			return fmt.Errorf("catalog candidate document %q does not align with parsed index", document.Key)
		}
	}
	return nil
}

func buildDocumentDirectory(catalogID string, document domain.CatalogDocument, index domain.SpecIndex, sourceChild string) (DocumentDirectoryV1, error) {
	result := DocumentDirectoryV1{
		Key: document.Key, SourcePath: document.SourcePath, Title: index.Title, APIVersion: index.Version,
		SourceChild: sourceChild, SchemaNodeShards: []ShardReferenceV1{},
		Operations: make([]OperationDirectoryV1, 0, len(index.Operations)), Schemas: make([]SchemaDirectoryV1, 0, len(index.Schemas)),
	}
	operations := append([]domain.Operation(nil), index.Operations...)
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path == operations[j].Path {
			return operations[i].Method < operations[j].Method
		}
		return operations[i].Path < operations[j].Path
	})
	for _, operation := range operations {
		detailID, err := domain.NewOperationDetailID(catalogID, document.Key, operation.Method, operation.Path)
		if err != nil {
			return DocumentDirectoryV1{}, err
		}
		title := firstCatalogText(operation.Summary, operation.ID, operation.Method+" "+operation.Path)
		result.Operations = append(result.Operations, OperationDirectoryV1{
			DetailID: detailID, OperationID: operation.ID, Method: operation.Method, Path: operation.Path,
			Title: title, Description: operation.Description, Href: catalogDetailHref(document.Key, detailID),
			Deprecated: operation.Deprecated, Tags: append([]string(nil), operation.Tags...), Facets: catalogFacets(operation.Facets),
		})
	}
	schemas := append([]domain.Schema(nil), index.Schemas...)
	sort.Slice(schemas, func(i, j int) bool { return schemas[i].Name < schemas[j].Name })
	for _, schema := range schemas {
		detailID, err := domain.NewSchemaDetailID(catalogID, document.Key, schema.Name)
		if err != nil {
			return DocumentDirectoryV1{}, err
		}
		result.Schemas = append(result.Schemas, SchemaDirectoryV1{
			DetailID: detailID, Name: schema.Name, Description: schema.Description, Href: catalogDetailHref(document.Key, detailID),
		})
	}
	return result, nil
}

func catalogDetailHref(documentKey string, detailID domain.DetailID) string {
	return "documents/" + documentKey + "/?selected=" + string(detailID) + "#" + string(detailID)
}

func sourceIdentity(role, key, sourcePath string, data []byte) SourceIdentityV1 {
	digest := sha256.Sum256(data)
	return SourceIdentityV1{Role: role, DocumentKey: key, SourcePath: sourcePath, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:])}
}

func catalogSourceExtension(format domain.CatalogFormat) string {
	if format == domain.CatalogFormatJSON {
		return ".json"
	}
	return ".yaml"
}

func firstCatalogText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "Untitled"
}

func candidateBranding(value domain.DocsBranding) BrandingV1 {
	return BrandingV1{
		DisplayName: value.DisplayName, LogoSrc: value.Logo.Src, LogoAlt: value.Logo.Alt,
		LogoHomeHref: value.Logo.HomeURL, FaviconHref: value.Favicon,
	}
}

func catalogFacets(values []domain.Facet) []FacetV1 {
	result := make([]FacetV1, len(values))
	for index, value := range values {
		result[index] = FacetV1{Name: value.Name, Value: value.Value}
	}
	return result
}
