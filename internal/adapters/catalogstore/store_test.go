package catalogstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

func TestStorePublishesAndPreflightsImmutableSnapshot(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	snapshot := compiledFixture(t)
	materialized, err := store.Publish(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := store.Preflight(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if verified.ID != snapshot.ID || verified.Location != materialized.Location || verified.Directory.CatalogID != "catalog" || verified.Search.SearchVersion != 1 {
		t.Fatalf("verified snapshot = %#v", verified)
	}
	second, err := store.Publish(context.Background(), snapshot)
	if err != nil || second.Location != materialized.Location {
		t.Fatalf("idempotent publication = %#v, %v", second, err)
	}
}

func TestCompiledVariableExactPrefixesSurviveDecodeAndActivation(t *testing.T) {
	t.Parallel()

	snapshot := compiledVariableExactPrefixFixture(t)
	var encodedDirectory []byte
	for _, child := range snapshot.Children {
		if child.Kind == "search-directory" {
			encodedDirectory = child.Bytes
			break
		}
	}
	if len(encodedDirectory) == 0 {
		t.Fatal("compiled snapshot lacks search directory")
	}
	directory, err := catalogjson.DecodeSearchDirectory(encodedDirectory)
	if err != nil {
		t.Fatalf("decode compiled variable exact prefixes: %v", err)
	}
	if encoded, err := catalogjson.EncodeSearchDirectory(directory); err != nil {
		t.Fatalf("re-encode compiled variable exact prefixes: %v", err)
	} else if !bytes.Equal(encoded, encodedDirectory) {
		t.Fatal("compiled search directory is not canonical after decode")
	}
	deepPrefixes := 0
	depths := make(map[int]struct{})
	siblingCounts := make(map[string]int)
	for index, bucket := range directory.ExactBuckets {
		if index > 0 && directory.ExactBuckets[index-1].Prefix >= bucket.Prefix {
			t.Fatalf("compiled exact prefixes are not strictly sorted: %q then %q", directory.ExactBuckets[index-1].Prefix, bucket.Prefix)
		}
		depths[len(bucket.Prefix)] = struct{}{}
		if len(bucket.Prefix) > 1 {
			deepPrefixes++
			siblingCounts[bucket.Prefix[:len(bucket.Prefix)-1]]++
		}
	}
	if deepPrefixes < 2 {
		t.Fatalf("compiled exact directory has %d deep prefixes, want sibling shards", deepPrefixes)
	}
	sawSiblings := false
	for _, count := range siblingCounts {
		if count > 1 {
			sawSiblings = true
			break
		}
	}
	if !sawSiblings {
		t.Fatal("compiled exact directory lacks sibling shards")
	}
	if len(depths) < 2 {
		t.Fatalf("compiled exact directory has %d prefix depths, want variable depths", len(depths))
	}

	root := t.TempDir()
	runtime := catalog.NewRuntime(1)
	coordinator, err := OpenActivationCoordinator(context.Background(), root, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer coordinator.Close()
	if _, err := coordinator.Activate(context.Background(), "/catalog", "", 1, snapshot); err != nil {
		t.Fatalf("activate compiled variable exact prefixes: %v", err)
	}
	if active := runtime.Table().Mounts["/catalog"].Active; active.ID != snapshot.ID {
		t.Fatalf("active snapshot = %q, want %q", active.ID, snapshot.ID)
	}
}

func compiledVariableExactPrefixFixture(t *testing.T) catalog.CompiledSnapshot {
	t.Helper()

	const documentKey = "large-v1"
	const operationCount = 320
	source := []byte(`{}`)
	operations := make([]domain.Operation, 0, operationCount)
	for candidate := 0; len(operations) < operationCount; candidate++ {
		literalPath := fmt.Sprintf("/operations/%05d", candidate)
		detailID, err := domain.NewOperationDetailID("catalog", documentKey, "GET", literalPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(strings.TrimPrefix(string(detailID), "detail-sha256-"), "0") {
			continue
		}
		operations = append(operations, domain.Operation{
			ID: fmt.Sprintf("operation-%05d", candidate), Method: "GET", Path: literalPath,
			Summary: "Synthetic operation for a recursively sharded exact bucket",
		})
	}
	revisionDigest := sha256.Sum256(source)
	candidate := domain.CatalogCandidate{
		ID: "catalog", Title: "Catalog", DefaultDocumentKey: documentKey, ProfileID: domain.CompatibilityProfileStrict,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "files-large-exact", ManifestDigest: hex.EncodeToString(revisionDigest[:])},
		Documents: []domain.CatalogDocument{{Key: documentKey, SourcePath: "large.json", Format: domain.CatalogFormatJSON, Bytes: source}},
	}
	index := domain.CatalogIndex{
		CatalogID: "catalog", RevisionID: "files-large-exact", Title: "Catalog", ProfileID: domain.CompatibilityProfileStrict,
		Documents: []domain.CatalogDocumentIndex{{
			Key: documentKey, SourcePath: "large.json",
			Index: domain.SpecIndex{RevisionID: "files-large-exact", Title: "Large", Version: "v1", SpecDownload: domain.SpecDownload{Filename: "large.json"}, Operations: operations},
		}},
	}
	options := catalog.DefaultCompilerOptions()
	// Keep the synthetic token/posting entries below the per-entry floor while
	// still forcing the 320-entry exact hot bucket to recurse by digest nibble.
	options.Bounds.PostingSegmentBytes = 8 << 10
	compiler, err := catalog.NewCompiler(options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatalf("compile variable exact prefix fixture: %v", err)
	}
	return snapshot
}

func TestStorePublishesFullLockedKubernetesSnapshotWithinBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("full locked Kubernetes receipt")
	}

	snapshot := compiledKubernetesFixture(t)
	store := New(t.TempDir())
	materialized, err := store.Publish(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := store.Preflight(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	operations, schemas := 0, 0
	for _, document := range verified.Directory.Documents {
		operations += len(document.Operations)
		schemas += len(document.Schemas)
	}
	if len(verified.Directory.Documents) != 65 || operations != 1202 || schemas != 1826 {
		t.Fatalf("Kubernetes catalog = %d documents, %d operations, %d schemas", len(verified.Directory.Documents), operations, schemas)
	}
	if materialized.Bytes > MaxSnapshotBytes {
		t.Fatalf("Kubernetes materialization = %d bytes", materialized.Bytes)
	}
}

func TestPreflightStreamsButDoesNotDecodeDetailShards(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	snapshot := compiledFixtureWithInvalidDetailBytes(t)
	if _, err := store.Publish(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Preflight(context.Background(), snapshot.ID); err != nil {
		t.Fatalf("preflight decoded an intentionally invalid detail shard: %v", err)
	}
}

func TestPreflightClassifiesChangedChildAsDeterministicCorruption(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())
	snapshot := compiledFixture(t)
	materialized, err := store.Publish(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	child := snapshot.Children[0]
	path := filepath.Join(materialized.Location, filepath.FromSlash(child.Path))
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 1
	if err := os.WriteFile(path, data, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Preflight(context.Background(), snapshot.ID); !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("corruption error = %v", err)
	}
}

func TestPublishRejectsUnsafeChildBeforeFilesystemMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := New(root)
	snapshot := compiledFixture(t)
	snapshot.Children[0].Path = "../escape"
	if _, err := store.Publish(context.Background(), snapshot); !errors.Is(err, ErrCorruptSnapshot) {
		t.Fatalf("unsafe path error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unsafe child escaped root: %v", err)
	}
}

func compiledFixture(t *testing.T) catalog.CompiledSnapshot {
	return compiledFixtureVersion(t, "things")
}

func compiledFixtureVersion(t *testing.T, suffix string) catalog.CompiledSnapshot {
	return compiledFixtureCatalogVersion(t, "catalog", suffix)
}

func compiledFixtureCatalogVersion(t *testing.T, catalogID, suffix string) catalog.CompiledSnapshot {
	t.Helper()
	source := []byte(`{"openapi":"3.0.3","info":{"title":"Fixture","version":"v1"},"paths":{"/` + suffix + `":{"get":{"operationId":"list` + suffix + `","responses":{"200":{"description":"ok"}}}}}}`)
	revisionDigest := sha256.Sum256(source)
	candidate := domain.CatalogCandidate{
		ID: catalogID, Title: "Catalog", DefaultDocumentKey: "fixture-v1", ProfileID: domain.CompatibilityProfileStrict,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "files-fixture", ManifestDigest: hex.EncodeToString(revisionDigest[:])},
		Documents: []domain.CatalogDocument{{Key: "fixture-v1", SourcePath: "fixture.json", Format: domain.CatalogFormatJSON, Bytes: source}},
	}
	parser, err := openapiadapter.NewCatalogParser(nil)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := catalog.NewCompiler(catalog.DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func compiledFixtureWithInvalidDetailBytes(t *testing.T) catalog.CompiledSnapshot {
	t.Helper()
	snapshot := compiledFixture(t)
	manifestIndex := -1
	changedPath := ""
	for index := range snapshot.Children {
		switch snapshot.Children[index].Kind {
		case "detail":
			snapshot.Children[index].Bytes = []byte("not-json")
			snapshot.Children[index].Length = uint64(len(snapshot.Children[index].Bytes))
			digest := sha256.Sum256(snapshot.Children[index].Bytes)
			snapshot.Children[index].SHA256 = hex.EncodeToString(digest[:])
			changedPath = snapshot.Children[index].Path
		case "manifest":
			manifestIndex = index
		}
	}
	if manifestIndex < 0 || changedPath == "" {
		t.Fatal("fixture lacks manifest or detail child")
	}
	manifest, err := catalogjson.DecodeManifest(snapshot.Children[manifestIndex].Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Children {
		if manifest.Children[index].Path == changedPath {
			for _, child := range snapshot.Children {
				if child.Path == changedPath {
					manifest.Children[index].Length = child.Length
					manifest.Children[index].SHA256 = child.SHA256
					manifest.Identity.Children[index] = manifest.Children[index]
				}
			}
		}
	}
	identityBytes, err := json.Marshal(manifest.Identity)
	if err != nil {
		t.Fatal(err)
	}
	identityDigest := sha256.Sum256(identityBytes)
	manifest.SnapshotID = catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(identityDigest[:]))
	manifestBytes, err := catalogjson.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	snapshot.Children[manifestIndex].Bytes = manifestBytes
	snapshot.Children[manifestIndex].Length = uint64(len(manifestBytes))
	snapshot.Children[manifestIndex].SHA256 = hex.EncodeToString(manifestDigest[:])
	snapshot.ID = manifest.SnapshotID
	snapshot.Identity = manifest.Identity
	return snapshot
}

func compiledKubernetesFixture(t *testing.T) catalog.CompiledSnapshot {
	t.Helper()
	root := filepath.Join("..", "..", "renderer", "testdata", "kubernetes")
	catalogBytes, err := os.ReadFile(filepath.Join(root, "catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Documents []struct {
			Key          string `json:"key"`
			UpstreamPath string `json:"upstreamPath"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(catalogBytes, &source); err != nil {
		t.Fatal(err)
	}
	documents := make([]domain.CatalogDocument, len(source.Documents))
	for index, document := range source.Documents {
		data, err := os.ReadFile(filepath.Join(root, "specs", filepath.Base(document.UpstreamPath)))
		if err != nil {
			t.Fatal(err)
		}
		documents[index] = domain.CatalogDocument{Key: document.Key, SourcePath: "specs/" + filepath.Base(document.UpstreamPath), Format: domain.CatalogFormatJSON, Bytes: data}
	}
	allowlist, err := os.ReadFile(filepath.Join(root, "default-allowlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(catalogBytes)
	candidate := domain.CatalogCandidate{
		ID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1", ProfileID: domain.CompatibilityProfileKubernetes,
		Revision:  domain.CatalogRevision{Kind: domain.CatalogRevisionFiles, ID: "files-kubernetes-a818af18", ManifestDigest: hex.EncodeToString(manifestDigest[:])},
		Documents: documents,
	}
	parser, err := openapiadapter.NewCatalogParser(allowlist)
	if err != nil {
		t.Fatal(err)
	}
	index, err := parser.Parse(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	options := catalog.DefaultCompilerOptions()
	options.ProfileAllowlist = allowlist
	compiler, err := catalog.NewCompiler(options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
