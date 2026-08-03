package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	uniqueSchemaNames := make(map[string]struct{})
	for _, document := range snapshot.Directory.Documents {
		operations += len(document.Operations)
		schemas += len(document.Schemas)
		for _, schema := range document.Schemas {
			uniqueSchemaNames[schema.Name] = struct{}{}
		}
	}
	if len(snapshot.Directory.Documents) != 65 || operations != 1202 || schemas != 1826 || len(uniqueSchemaNames) != 862 {
		t.Fatalf("compiled totals = documents:%d operations:%d schemas:%d unique-schema-names:%d", len(snapshot.Directory.Documents), operations, schemas, len(uniqueSchemaNames))
	}
	visibleHrefs := make(map[domain.DetailID]string, operations+schemas)
	requiredExactKeys := make(map[string]struct{}, operations+schemas)
	for _, document := range snapshot.Directory.Documents {
		for _, operation := range document.Operations {
			visibleHrefs[operation.DetailID] = operation.Href
			requiredExactKeys[string(operation.DetailID)] = struct{}{}
		}
		for _, schema := range document.Schemas {
			visibleHrefs[schema.DetailID] = schema.Href
			requiredExactKeys[string(schema.DetailID)] = struct{}{}
		}
	}

	searchDirectoryChild := childByKind(t, snapshot.Children, "search-directory")
	var searchDirectory SearchDirectoryV1
	if err := json.Unmarshal(searchDirectoryChild.Bytes, &searchDirectory); err != nil {
		t.Fatal(err)
	}
	if len(searchDirectory.Ranks) != 2130 {
		t.Fatalf("deduplicated Kubernetes search records = %d, want 2130", len(searchDirectory.Ranks))
	}
	exactKeys := make(map[string]struct{})
	searchRecordCount := 0
	for _, child := range snapshot.Children {
		switch child.Kind {
		case "search-exact":
			var segment SearchExactSegmentV1
			if err := json.Unmarshal(child.Bytes, &segment); err != nil {
				t.Fatal(err)
			}
			for _, entry := range segment.Entries {
				exactKeys[entry.Key] = struct{}{}
				for _, match := range entry.Matches {
					if int(match.Record) >= len(searchDirectory.Ranks) {
						t.Fatalf("exact key %q has invalid record %d", entry.Key, match.Record)
					}
				}
			}
		case "search-posting", "search-trigram":
			var segment SearchPostingSegmentV1
			if err := json.Unmarshal(child.Bytes, &segment); err != nil {
				t.Fatal(err)
			}
			for _, entry := range segment.Entries {
				for _, recordID := range entry.Records {
					if int(recordID) >= len(searchDirectory.Ranks) {
						t.Fatalf("posting %q has invalid record %d", entry.Key, recordID)
					}
				}
			}
		case "search-record":
			var segment SearchRecordSegmentV1
			if err := json.Unmarshal(child.Bytes, &segment); err != nil {
				t.Fatal(err)
			}
			searchRecordCount += len(segment.Records)
			for _, record := range segment.Records {
				if href, exists := visibleHrefs[record.DetailID]; !exists || href != record.Href {
					t.Fatalf("search record %q has no exact visible href", record.DetailID)
				}
			}
		}
	}
	if searchRecordCount != len(searchDirectory.Ranks) {
		t.Fatalf("search display record count = %d, ranks = %d", searchRecordCount, len(searchDirectory.Ranks))
	}
	for key := range requiredExactKeys {
		if _, exists := exactKeys[key]; !exists {
			t.Fatalf("canonical detail ID %q is not exact-searchable", key)
		}
	}

	service, err := NewSearchService(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var searchBytes uint64
	var searchSegments int
	for _, child := range snapshot.Children {
		if strings.HasPrefix(child.Kind, "search-") {
			searchBytes += child.Length
		}
		if strings.HasPrefix(child.Kind, "search-") && child.Kind != "search-directory" {
			searchSegments++
			if child.Length > options.Bounds.PostingSegmentBytes {
				t.Fatalf("search child %q bytes = %d", child.Path, child.Length)
			}
		}
	}
	if searchBytes > options.Bounds.SearchBytes {
		t.Fatalf("Kubernetes search bytes = %d, want <= %d", searchBytes, options.Bounds.SearchBytes)
	}
	t.Logf("Kubernetes search artifacts: bytes=%d segments=%d", searchBytes, searchSegments)
	for _, searchCase := range []struct {
		query, document, title string
	}{
		{query: "list deployment", document: "apps-v1", title: "deployment"},
		{query: "apps v1 deployment", document: "apps-v1", title: "deployment"},
		{query: "list pod", document: "core-v1", title: "pod"},
		{query: "storage class", document: "storage-v1", title: "storage"},
		{query: "resource claim", document: "resource-v1", title: "resource"},
	} {
		result, err := service.Search(context.Background(), snapshot.ID, searchCase.query)
		if err != nil {
			t.Fatalf("global Kubernetes search %q: %v", searchCase.query, err)
		}
		found := false
		for _, match := range result.Results {
			if match.DocumentKey == searchCase.document && strings.Contains(strings.ToLower(match.Title), searchCase.title) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("global Kubernetes search %q did not find %s target: %#v", searchCase.query, searchCase.document, result.Results)
		}
		if result.SegmentsDecoded == 0 || result.SegmentsDecoded > maxSearchSegments || result.PostingsScanned > maxSearchPostings || result.BytesDecoded > maxSearchDecodedBytes {
			t.Fatalf("global Kubernetes search receipt exceeds bounds: %#v", result)
		}
		t.Logf("Kubernetes search %q: results=%d postings=%d segments=%d bytes=%d duration=%s", searchCase.query, len(result.Results), result.PostingsScanned, result.SegmentsDecoded, result.BytesDecoded, result.Duration)
	}

	var schemaDetailID domain.DetailID
	for _, document := range snapshot.Directory.Documents {
		for _, schema := range document.Schemas {
			if shortSchemaName(schema.Name) == "PodSpec" {
				schemaDetailID = schema.DetailID
				break
			}
		}
		if schemaDetailID != "" {
			break
		}
	}
	if schemaDetailID == "" {
		t.Fatal("compiled Kubernetes catalog has no PodSpec schema")
	}
	for name, query := range map[string]string{
		"schema short name":   "PodSpec",
		"canonical detail ID": string(schemaDetailID),
	} {
		exact, err := service.Search(context.Background(), snapshot.ID, query)
		if err != nil {
			t.Fatalf("%s search: %v", name, err)
		}
		if len(exact.Results) == 0 {
			t.Fatalf("%s search returned no results", name)
		}
	}
}

func childByKind(t *testing.T, children []ChildArtifact, kind string) ChildArtifact {
	t.Helper()
	for _, child := range children {
		if child.Kind == kind {
			return child
		}
	}
	t.Fatalf("child kind %q is missing", kind)
	return ChildArtifact{}
}
