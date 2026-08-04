package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestCompilerEmitsCompleteAcyclicSnapshot(t *testing.T) {
	t.Parallel()

	candidate, index := compilerFixture()
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(snapshot.ID), "snapshot-sha256-") || len(snapshot.ID) != len("snapshot-sha256-")+64 {
		t.Fatalf("snapshot ID = %q", snapshot.ID)
	}
	if len(snapshot.Directory.Documents) != 2 || len(snapshot.Identity.Sources) != 2 {
		t.Fatalf("snapshot directory/identity = %#v / %#v", snapshot.Directory, snapshot.Identity)
	}
	detailIDs := make(map[domain.DetailID]struct{})
	for _, document := range snapshot.Directory.Documents {
		for _, operation := range document.Operations {
			detailIDs[operation.DetailID] = struct{}{}
		}
		for _, schema := range document.Schemas {
			detailIDs[schema.DetailID] = struct{}{}
		}
	}
	if len(detailIDs) != 4 {
		t.Fatalf("detail coverage = %d, want 4", len(detailIDs))
	}
	paths := make(map[string]struct{}, len(snapshot.Children))
	for _, child := range snapshot.Children {
		if _, exists := paths[child.Path]; exists {
			t.Fatalf("duplicate child path %q", child.Path)
		}
		paths[child.Path] = struct{}{}
		if child.Path != "manifest.json" && bytes.Contains(child.Bytes, []byte(snapshot.ID)) {
			t.Fatalf("child %q embeds final snapshot ID", child.Path)
		}
	}
	if _, exists := paths["manifest.json"]; !exists {
		t.Fatal("manifest child is missing")
	}
}

func TestCompilerRejectsCandidateIndexMismatch(t *testing.T) {
	t.Parallel()

	candidate, index := compilerFixture()
	index.Documents[0].SourcePath = "other.json"
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.Compile(context.Background(), candidate, index); err == nil {
		t.Fatal("candidate/index mismatch was accepted")
	}
}

func compilerFixture() (domain.CatalogCandidate, domain.CatalogIndex) {
	documents := []domain.CatalogDocument{
		{Key: "beta-v1", SourcePath: "beta.json", Format: domain.CatalogFormatJSON, Bytes: []byte(`{"beta":true}`)},
		{Key: "alpha-v1", SourcePath: "alpha.json", Format: domain.CatalogFormatJSON, Bytes: []byte(`{"alpha":true}`)},
	}
	digest := sha256.Sum256([]byte("compiler-fixture"))
	candidate := domain.CatalogCandidate{
		ID: "catalog", Title: "Catalog", DefaultDocumentKey: "alpha-v1", ProfileID: domain.CompatibilityProfileStrict,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "files-fixture", ManifestDigest: hex.EncodeToString(digest[:])},
		Documents: documents,
	}
	indexes := []domain.CatalogDocumentIndex{
		compilerDocumentIndex("beta-v1", "beta.json", "GET", "/beta", "Beta"),
		compilerDocumentIndex("alpha-v1", "alpha.json", "POST", "/alpha", "Alpha"),
	}
	return candidate, domain.CatalogIndex{
		CatalogID: "catalog", RevisionID: "files-fixture", Title: "Catalog", ProfileID: domain.CompatibilityProfileStrict,
		Documents: indexes,
	}
}

func compilerDocumentIndex(key, sourcePath, method, literalPath, schemaName string) domain.CatalogDocumentIndex {
	return domain.CatalogDocumentIndex{
		Key: key, SourcePath: sourcePath,
		Index: domain.SpecIndex{
			RevisionID: "files-fixture", Title: strings.Title(schemaName), Version: "v1",
			SpecDownload: domain.SpecDownload{Filename: key + ".json"},
			Operations:   []domain.Operation{{ID: "operation-" + key, Method: method, Path: literalPath, Summary: schemaName}},
			Schemas:      []domain.Schema{{Name: schemaName, Summary: domain.SchemaSummary{Name: schemaName, Type: "object"}}},
		},
	}
}

func sortedChildren(children []ChildArtifact) []ChildArtifact {
	result := append([]ChildArtifact(nil), children...)
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func assertSnapshotsEqual(t *testing.T, left, right CompiledSnapshot) {
	t.Helper()
	if left.ID != right.ID || !reflect.DeepEqual(left.Identity, right.Identity) || !reflect.DeepEqual(sortedChildren(left.Children), sortedChildren(right.Children)) {
		t.Fatalf("snapshots differ:\nleft=%#v\nright=%#v", left, right)
	}
}
