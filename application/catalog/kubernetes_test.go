package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/araihu/manja/domain"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

func TestCompilerCompilesCompleteLockedKubernetesCatalog(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "renderer", "testdata", "kubernetes")
	catalogBytes, err := os.ReadFile(filepath.Join(root, "catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var authority struct {
		Documents []struct {
			Key          string `json:"key"`
			UpstreamPath string `json:"upstreamPath"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(catalogBytes, &authority); err != nil {
		t.Fatal(err)
	}
	documents := make([]domain.CatalogDocument, len(authority.Documents))
	for index, document := range authority.Documents {
		data, err := os.ReadFile(filepath.Join(root, "specs", filepath.Base(document.UpstreamPath)))
		if err != nil {
			t.Fatal(err)
		}
		documents[index] = domain.CatalogDocument{
			Key: document.Key, SourcePath: "specs/" + filepath.Base(document.UpstreamPath),
			Format: domain.CatalogFormatJSON, Bytes: data,
		}
	}
	manifestDigest := sha256.Sum256(catalogBytes)
	candidate := domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1",
		ProfileID: domain.CompatibilityProfileKubernetes,
		Revision: domain.CatalogRevision{
			Kind: domain.CatalogRevisionFiles, ID: "files-kubernetes-a818af18", ManifestDigest: hex.EncodeToString(manifestDigest[:]),
		},
		Documents: documents,
	}
	allowlist, err := os.ReadFile(filepath.Join(root, "default-allowlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	parser, err := openapiadapter.NewCatalogParser(allowlist)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultCompilerOptions()
	options.ProfileAllowlist = allowlist
	compiler, err := NewCompiler(options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	operations, schemas := 0, 0
	for _, document := range snapshot.Directory.Documents {
		operations += len(document.Operations)
		schemas += len(document.Schemas)
	}
	if len(snapshot.Directory.Documents) != 65 || operations != 1202 || schemas != 1826 {
		t.Fatalf("compiled totals = documents:%d operations:%d schemas:%d", len(snapshot.Directory.Documents), operations, schemas)
	}
}
