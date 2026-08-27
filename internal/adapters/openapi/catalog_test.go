package openapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestCatalogParserParsesCompleteLockedKubernetesCatalog(t *testing.T) {
	candidate, allowlist := lockedKubernetesCandidate(t)
	parser, err := NewCatalogParser(allowlist)
	if err != nil {
		t.Fatal(err)
	}

	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Documents) != 65 {
		t.Fatalf("document count = %d, want 65", len(index.Documents))
	}
	if !sort.SliceIsSorted(index.Documents, func(i, j int) bool { return index.Documents[i].Key < index.Documents[j].Key }) {
		t.Fatal("catalog documents are not sorted by document key")
	}
	operations, schemas := 0, 0
	sourceByKey := make(map[string][]byte, len(candidate.Documents))
	for _, document := range candidate.Documents {
		sourceByKey[document.Key] = document.Bytes
	}
	for _, document := range index.Documents {
		operations += len(document.Index.Operations)
		schemas += len(document.Index.Schemas)
		if !bytes.Equal(document.Index.SpecDownload.JSON, sourceByKey[document.Key]) {
			t.Fatalf("document %q download does not preserve exact locked source bytes", document.Key)
		}
	}
	if operations != 1202 || schemas != 1826 {
		t.Fatalf("catalog totals = %d operations, %d schemas; want 1202, 1826", operations, schemas)
	}
	if index.CatalogID != "kubernetes" || index.RevisionID != candidate.Revision.ID || index.ProfileID != domain.CompatibilityProfileKubernetes {
		t.Fatalf("catalog authority = %#v", index)
	}
}

func TestCatalogParserAllowsDuplicateOperationIDAcrossDocuments(t *testing.T) {
	t.Parallel()

	candidate := catalogParserCandidate(
		catalogParserDocument("alpha-v1", "alpha.json", `{"openapi":"3.0.3","info":{"title":"Alpha","version":"v1"},"paths":{"/alpha":{"get":{"operationId":"shared","responses":{"200":{"description":"ok"}}}}}}`),
		catalogParserDocument("beta-v1", "beta.json", `{"openapi":"3.0.3","info":{"title":"Beta","version":"v1"},"paths":{"/beta":{"get":{"operationId":"shared","responses":{"200":{"description":"ok"}}}}}}`),
	)
	parser, err := NewCatalogParser(nil)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Documents) != 2 || index.Documents[0].Index.Operations[0].ID != "shared" || index.Documents[1].Index.Operations[0].ID != "shared" {
		t.Fatalf("duplicate cross-document operationId was not preserved: %#v", index.Documents)
	}
}

func TestCatalogParserRejectsDuplicatePathBeforeJSONCollapse(t *testing.T) {
	t.Parallel()

	candidate := catalogParserCandidate(catalogParserDocument("alpha-v1", "alpha.json", `{
  "openapi":"3.0.3",
  "info":{"title":"Alpha","version":"v1"},
  "paths":{
    "/alpha":{"get":{"operationId":"first","responses":{"200":{"description":"ok"}}}},
    "/alpha":{"get":{"operationId":"second","responses":{"200":{"description":"ok"}}}}
  }
}`))
	parser, err := NewCatalogParser(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(context.Background(), candidate); err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("duplicate path error = %v", err)
	}
}

func TestCatalogParserProjectsKubernetesOperationFacets(t *testing.T) {
	t.Parallel()

	document := catalogParserDocument("apps-v1", "apps.json", `{
  "openapi":"3.0.3",
  "info":{"title":"Apps","version":"v1"},
  "paths":{"/apis/apps/v1/deployments":{"get":{
    "operationId":"listAppsV1Deployment",
    "x-kubernetes-action":"list",
    "x-kubernetes-group-version-kind":{"group":"apps","version":"v1","kind":"Deployment"},
    "responses":{"200":{"description":"ok"}}
  }}}
}`)
	candidate := catalogParserCandidate(document)
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	allowlist, err := BuildKubernetesDefaultAllowlist(context.Background(), []domain.CatalogDocument{document})
	if err != nil {
		t.Fatal(err)
	}
	parser, err := NewCatalogParser(allowlist)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Facet{
		{Name: "action", Value: "list"},
		{Name: "group", Value: "apps"},
		{Name: "kind", Value: "Deployment"},
		{Name: "version", Value: "v1"},
	}
	if !reflect.DeepEqual(index.Documents[0].Index.Operations[0].Facets, want) {
		t.Fatalf("facets = %#v, want %#v", index.Documents[0].Index.Operations[0].Facets, want)
	}
}

func TestCatalogParserResolvesOnlyCapturedRelativeReferences(t *testing.T) {
	t.Parallel()

	candidate := catalogParserCandidate(catalogParserDocument("alpha-v1", "specs/alpha.json", `{
  "openapi":"3.0.3",
  "info":{"title":"Alpha","version":"v1"},
  "paths":{},
  "components":{"schemas":{"Thing":{"$ref":"./common.yaml#/components/schemas/Thing"}}}
}`))
	candidate.SupportFiles = []domain.CatalogSupportFile{{
		SourcePath: "specs/common.yaml",
		Bytes:      []byte("components:\n  schemas:\n    Thing:\n      type: string\n"),
	}}
	parser, err := NewCatalogParser(nil)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Documents[0].Index.Schemas[0].Name; got != "Thing" {
		t.Fatalf("resolved schema name = %q", got)
	}
}

func TestCatalogParserNeverDialsRemoteReference(t *testing.T) {
	t.Parallel()

	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests <- struct{}{} }))
	defer server.Close()
	candidate := catalogParserCandidate(catalogParserDocument("alpha-v1", "alpha.json", `{
  "openapi":"3.0.3",
  "info":{"title":"Alpha","version":"v1"},
  "paths":{},
  "components":{"schemas":{"Thing":{"$ref":"`+server.URL+`/common.yaml#/Thing"}}}
}`))
	parser, err := NewCatalogParser(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(context.Background(), candidate); err == nil {
		t.Fatal("remote catalog reference was accepted")
	}
	select {
	case <-requests:
		t.Fatal("catalog parser dialed a remote reference")
	default:
	}
}

func TestCatalogParserRejectsKubernetesSupportFilesOutsideExactDefaultAudit(t *testing.T) {
	t.Parallel()

	document := catalogParserDocument("core-v1", "core.json", `{"openapi":"3.0.3","info":{"title":"Core","version":"v1"},"paths":{}}`)
	candidate := catalogParserCandidate(document)
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	candidate.SupportFiles = []domain.CatalogSupportFile{{SourcePath: "common.yaml", Bytes: []byte("type: string\n")}}
	allowlist, err := BuildKubernetesDefaultAllowlist(context.Background(), []domain.CatalogDocument{document})
	if err != nil {
		t.Fatal(err)
	}
	parser, err := NewCatalogParser(allowlist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(context.Background(), candidate); err == nil || !strings.Contains(err.Error(), "support files") {
		t.Fatalf("Kubernetes support-file audit error = %v", err)
	}
}

func catalogParserCandidate(documents ...domain.CatalogDocument) domain.CatalogCandidate {
	digest := sha256.Sum256([]byte("catalog-parser-test"))
	return domain.CatalogCandidate{
		ID: "catalog", Title: "Catalog", ProfileID: domain.CompatibilityProfileStrict,
		Revision: domain.CatalogRevision{
			Kind: domain.CatalogRevisionFiles, ID: "file-catalog-parser-test", ManifestDigest: hex.EncodeToString(digest[:]),
		},
		Documents: documents,
	}
}

func catalogParserDocument(key, path, data string) domain.CatalogDocument {
	return domain.CatalogDocument{Key: key, SourcePath: path, Format: domain.CatalogFormatJSON, Bytes: []byte(data)}
}

func lockedKubernetesCandidate(t *testing.T) (domain.CatalogCandidate, []byte) {
	t.Helper()
	root := filepath.Join("..", "..", "renderer", "testdata", "kubernetes")
	catalogBytes, err := os.ReadFile(filepath.Join(root, "catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Documents []struct {
			Key          string `json:"key"`
			UpstreamPath string `json:"upstreamPath"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
		t.Fatal(err)
	}
	documents := make([]domain.CatalogDocument, len(catalog.Documents))
	for index, document := range catalog.Documents {
		data, err := os.ReadFile(filepath.Join(root, "specs", filepath.Base(document.UpstreamPath)))
		if err != nil {
			t.Fatal(err)
		}
		documents[index] = domain.CatalogDocument{
			Key: document.Key, SourcePath: "specs/" + filepath.Base(document.UpstreamPath), Format: domain.CatalogFormatJSON, Bytes: data,
		}
	}
	allowlist, err := os.ReadFile(filepath.Join(root, "default-allowlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(catalogBytes)
	return domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1",
		ProfileID: domain.CompatibilityProfileKubernetes,
		Revision: domain.CatalogRevision{
			Kind: domain.CatalogRevisionFiles, ID: "file-kubernetes-a818af18", ManifestDigest: hex.EncodeToString(manifestDigest[:]),
		},
		Documents: documents,
	}, allowlist
}
