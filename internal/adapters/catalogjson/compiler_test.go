package catalogjson

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func TestCodecAcceptsEveryCompilerArtifact(t *testing.T) {
	t.Parallel()

	digest := sha256.Sum256([]byte("codec-compiler"))
	candidate := domain.CatalogCandidate{
		ID: "catalog", Title: "Catalog", DefaultDocumentKey: "doc", ProfileID: domain.CompatibilityProfileStrict,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "files-codec", ManifestDigest: hex.EncodeToString(digest[:])},
		Documents: []domain.CatalogDocument{{Key: "doc", SourcePath: "openapi.json", Format: domain.CatalogFormatJSON, Bytes: []byte(`{}`)}},
	}
	index := domain.CatalogIndex{
		CatalogID: "catalog", RevisionID: "files-codec", Title: "Catalog", ProfileID: domain.CompatibilityProfileStrict,
		Documents: []domain.CatalogDocumentIndex{{
			Key: "doc", SourcePath: "openapi.json",
			Index: domain.SpecIndex{
				RevisionID: "files-codec", Title: "Doc", Version: "v1", SpecDownload: domain.SpecDownload{Filename: "openapi.json"},
				Operations: []domain.Operation{{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets"}},
				Schemas:    []domain.Schema{{Name: "Pet", Summary: domain.SchemaSummary{Name: "Pet", Type: "object"}}},
			},
		}},
	}
	compiler, err := catalog.NewCompiler(catalog.DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	var directory catalog.CatalogArtifactV1
	var searchDirectory catalog.SearchDirectoryV1
	var manifest catalog.ManifestV1
	for _, child := range snapshot.Children {
		switch child.Kind {
		case "catalog":
			directory, err = DecodeCatalog(child.Bytes)
		case "detail":
			_, err = DecodeDetailShard(child.Bytes)
		case "schema-node":
			_, err = DecodeSchemaNodeShard(child.Bytes)
		case "search-directory":
			searchDirectory, err = DecodeSearchDirectory(child.Bytes)
		case "search-exact":
			_, err = DecodeSearchExactSegment(child.Bytes)
		case "search-posting", "search-trigram":
			_, err = DecodeSearchPostingSegment(child.Bytes)
		case "search-record":
			_, err = DecodeSearchRecordSegment(child.Bytes)
		case "manifest":
			manifest, err = DecodeManifest(child.Bytes)
		}
		if err != nil {
			t.Fatalf("decode %s %s: %v", child.Kind, child.Path, err)
		}
	}
	if err := ValidateCatalogManifest(directory, manifest); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSearchManifest(searchDirectory, manifest); err != nil {
		t.Fatal(err)
	}
}
