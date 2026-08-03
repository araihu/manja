package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/domain"
)

func TestSearchFindsExactKeysBeforeTokenRanking(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	alpha := snapshot.Directory.Documents[0]
	operation := alpha.Operations[0]
	schema := alpha.Schemas[0]
	for name, query := range map[string]string{
		"detail ID":    string(operation.DetailID),
		"operation ID": operation.OperationID,
		"literal path": operation.Path,
		"method path":  operation.Method + " " + operation.Path,
		"schema":       schema.Name,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := service.Search(context.Background(), snapshot.ID, query)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Results) == 0 {
				t.Fatalf("exact query %q returned no result", query)
			}
		})
	}
}

func TestRuntimeSearchLoadsVerifiedChildrenThroughBoundedCache(t *testing.T) {
	t.Parallel()

	_, snapshot := compiledSearchFixture(t)
	children := make(map[string]ChildArtifact, len(snapshot.Children))
	manifest := ManifestV1{SchemaVersion: 1, SnapshotID: snapshot.ID}
	var directory SearchDirectoryV1
	for _, child := range snapshot.Children {
		children[child.Path] = child
		manifest.Children = append(manifest.Children, ChildIdentityV1{Path: child.Path, Kind: child.Kind, Length: child.Length, SHA256: child.SHA256})
		if child.Path == snapshot.Directory.SearchChild {
			if err := json.Unmarshal(child.Bytes, &directory); err != nil {
				t.Fatal(err)
			}
		}
	}
	runtimeSnapshot := RuntimeSnapshot{ID: snapshot.ID, Directory: snapshot.Directory, Search: directory, Manifest: manifest}
	cache := NewSearchCache()
	loads := 0
	service, err := NewRuntimeSearchService(runtimeSnapshot, cache, func(_ context.Context, path string) ([]byte, ChildIdentityV1, error) {
		loads++
		child, ok := children[path]
		if !ok {
			return nil, ChildIdentityV1{}, fmt.Errorf("missing %s", path)
		}
		return append([]byte(nil), child.Bytes...), ChildIdentityV1{Path: child.Path, Kind: child.Kind, Length: child.Length, SHA256: child.SHA256}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	query := snapshot.Directory.Documents[0].Operations[0].OperationID
	first, err := service.Search(context.Background(), snapshot.ID, query)
	if err != nil || len(first.Results) == 0 || loads == 0 {
		t.Fatalf("runtime search = %#v loads=%d err=%v", first, loads, err)
	}
	firstLoads := loads
	if _, err := service.Search(context.Background(), snapshot.ID, query); err != nil {
		t.Fatal(err)
	}
	if loads != firstLoads || cache.Stats().Hits == 0 {
		t.Fatalf("cache reuse loads=%d/%d stats=%#v", firstLoads, loads, cache.Stats())
	}
}

func TestSearchExactLookupRunsBeforeTokenCountLimit(t *testing.T) {
	t.Parallel()

	candidate, index := compilerFixture()
	exactOperationID := "one two three four five six seven eight nine"
	index.Documents[0].Index.Operations[0].ID = exactOperationID
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewSearchService(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Search(context.Background(), snapshot.ID, exactOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].OperationID != exactOperationID {
		t.Fatalf("exact over-token result = %#v", result.Results)
	}
}

func TestSearchGroupsOnlySemanticallyIdenticalSchemas(t *testing.T) {
	t.Parallel()

	for name, secondType := range map[string]string{"identical": "object", "different": "string"} {
		t.Run(name, func(t *testing.T) {
			candidate, index := compilerFixture()
			for documentIndex := range index.Documents {
				index.Documents[documentIndex].Index.Schemas[0].Name = "SharedSchema"
				index.Documents[documentIndex].Index.Schemas[0].Summary = domain.SchemaSummary{Name: "SharedSchema", Type: "object"}
			}
			index.Documents[1].Index.Schemas[0].Summary.Type = secondType
			compiler, err := NewCompiler(DefaultCompilerOptions())
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := compiler.Compile(context.Background(), candidate, index)
			if err != nil {
				t.Fatal(err)
			}
			service, err := NewSearchService(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Search(context.Background(), snapshot.ID, "SharedSchema")
			if err != nil {
				t.Fatal(err)
			}
			wantResults := 1
			if name == "different" {
				wantResults = 2
			}
			if len(result.Results) != wantResults {
				t.Fatalf("schema group results = %#v", result.Results)
			}
			if name == "identical" {
				grouped := result.Results[0]
				if grouped.DocumentKey != "alpha-v1" || grouped.Occurrences != 2 || !reflect.DeepEqual(grouped.Documents, []string{"alpha-v1", "beta-v1"}) {
					t.Fatalf("grouped schema = %#v", grouped)
				}
				for _, document := range snapshot.Directory.Documents {
					exact, err := service.Search(context.Background(), snapshot.ID, string(document.Schemas[0].DetailID))
					if err != nil || len(exact.Results) != 1 || exact.Results[0].DetailID != grouped.DetailID {
						t.Fatalf("source detail ID lookup for %s = %#v, %v", document.Key, exact, err)
					}
				}
			}
		})
	}
}

func TestSearchRanksMultiDocumentTokensDeterministically(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	first, err := service.Search(context.Background(), snapshot.ID, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Search(context.Background(), snapshot.ID, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Results) == 0 || len(first.Results) > 20 || !equalSearchResults(first.Results, second.Results) {
		t.Fatalf("unstable bounded results = %#v / %#v", first.Results, second.Results)
	}
	if first.SnapshotID != snapshot.ID || first.SearchVersion != 1 || first.PostingsScanned == 0 || first.SegmentsDecoded == 0 {
		t.Fatalf("search receipt = %#v", first)
	}
}

func TestSearchRejectsWrongSnapshotCancellationAndBroadWork(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	if _, err := service.Search(context.Background(), SnapshotID("snapshot-sha256-"+strings.Repeat("f", 64)), "alpha"); err == nil {
		t.Fatal("wrong snapshot was accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Search(canceled, snapshot.ID, "alpha"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled search error = %v", err)
	}

	broad := *service
	broad.directory.ExactBuckets = nil
	broad.directory.TokenRoutes = make([]SearchPostingRouteV1, maxSearchTokenSegments+1)
	broad.directory.PostingSegments = make([]SearchSegmentReferenceV1, maxSearchTokenSegments+1)
	for index := range broad.directory.TokenRoutes {
		broad.directory.TokenRoutes[index] = SearchPostingRouteV1{Key: fmt.Sprintf("alpha%d", index), Segment: uint16(index)}
	}
	if _, err := broad.Search(context.Background(), snapshot.ID, "alpha"); !errors.Is(err, ErrQueryTooBroad) {
		t.Fatalf("broad segment error = %v", err)
	}
}

func TestSearchRejectsOverLimitHumanKeyButCanonicalDetailIDSucceeds(t *testing.T) {
	t.Parallel()

	candidate, index := compilerFixture()
	overLimitPath := "/" + strings.Repeat("a", 256)
	index.Documents[0].Index.Operations[0].Path = overLimitPath
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewSearchService(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Search(context.Background(), snapshot.ID, overLimitPath); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("over-limit query error = %v", err)
	}
	var detailID domain.DetailID
	for _, document := range snapshot.Directory.Documents {
		for _, operation := range document.Operations {
			if operation.Path == overLimitPath {
				detailID = operation.DetailID
			}
		}
	}
	if detailID == "" {
		t.Fatal("over-limit human key operation is missing")
	}
	result, err := service.Search(context.Background(), snapshot.ID, string(detailID))
	if err != nil || len(result.Results) == 0 {
		t.Fatalf("canonical detail ID lookup = %#v, %v", result, err)
	}
}

func TestSearchUsesBoundedTrigramFallback(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	result, err := service.Search(context.Background(), snapshot.ID, "alphx")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) == 0 || !strings.Contains(strings.ToLower(result.Results[0].Title), "alpha") {
		t.Fatalf("fuzzy alpha results = %#v", result.Results)
	}
	if result.SegmentsDecoded > maxSearchSegments || result.BytesDecoded > maxSearchDecodedBytes || result.PostingsScanned > maxSearchPostings {
		t.Fatalf("fuzzy receipt exceeds bounds: %#v", result)
	}
}

func TestSearchRejectsPostingAndDecodedWorkOverLimits(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	for name, reference := range map[string]SearchSegmentReferenceV1{
		"postings": {Path: "search/postings/oversized.json", Entries: 1, Postings: maxSearchPostings + 1, Length: 1, SHA256: strings.Repeat("a", 64)},
		"bytes":    {Path: "search/postings/oversized.json", Entries: 1, Postings: 1, Length: maxSearchDecodedBytes + 1, SHA256: strings.Repeat("a", 64)},
	} {
		t.Run(name, func(t *testing.T) {
			bounded := *service
			bounded.directory.ExactBuckets = nil
			bounded.directory.TokenRoutes = []SearchPostingRouteV1{{Key: "limit", Segment: 0}}
			bounded.directory.PostingSegments = []SearchSegmentReferenceV1{reference}
			if _, err := bounded.Search(context.Background(), snapshot.ID, "limit"); !errors.Is(err, ErrQueryTooBroad) {
				t.Fatalf("over-limit %s error = %v", name, err)
			}
		})
	}
}

func TestSearchRejectsMoreThanSixteenDecodedSegments(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	recordIDs := make([]uint32, maxSearchSegments+1)
	for index := range recordIDs {
		recordIDs[index] = uint32(index)
	}
	segmentBytes, err := json.Marshal(SearchPostingSegmentV1{SchemaVersion: 1, SearchVersion: searchVersion, Entries: []SearchPostingEntryV1{{Key: "spread", Records: recordIDs}}})
	if err != nil {
		t.Fatal(err)
	}
	postingChild, err := contentAddressedSearchChild("postings", "search-posting", segmentBytes)
	if err != nil {
		t.Fatal(err)
	}
	broad := *service
	broad.directory.ExactBuckets = nil
	broad.directory.TokenRoutes = []SearchPostingRouteV1{{Key: "spread", Segment: 0}}
	broad.directory.PostingSegments = []SearchSegmentReferenceV1{searchSegmentReference(postingChild, 1, uint32(len(recordIDs)))}
	broad.directory.Ranks = make([]SearchRankRecordV1, len(recordIDs))
	broad.directory.RecordSegments = make([]SearchRecordSegmentReferenceV1, len(recordIDs))
	for index := range recordIDs {
		broad.directory.Ranks[index] = SearchRankRecordV1{Title: fmt.Sprintf("Record %02d", index)}
		broad.directory.RecordSegments[index] = SearchRecordSegmentReferenceV1{
			Path: fmt.Sprintf("search/records/%064x.json", index+1), FirstRecord: uint32(index), Records: 1,
			Length: 1, SHA256: fmt.Sprintf("%064x", index+1),
		}
	}
	broad.children = make(map[string]ChildArtifact, len(service.children)+1)
	for pathValue, child := range service.children {
		broad.children[pathValue] = child
	}
	broad.children[postingChild.Path] = postingChild
	if _, err := broad.Search(context.Background(), snapshot.ID, "spread"); !errors.Is(err, ErrQueryTooBroad) || !strings.Contains(err.Error(), "record segments") {
		t.Fatalf("decoded segment error = %v", err)
	}
}

func TestSearchDeadlineIsTypedAndDoesNotDisableService(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	deadline := *service
	deadline.deadline = time.Nanosecond
	query := string(snapshot.Directory.Documents[0].Operations[0].DetailID)
	if _, err := deadline.Search(context.Background(), snapshot.ID, query); !errors.Is(err, ErrSearchDeadline) {
		t.Fatalf("deadline error = %v", err)
	}
	result, err := service.Search(context.Background(), snapshot.ID, query)
	if err != nil || len(result.Results) == 0 {
		t.Fatalf("service did not recover after transient deadline: %#v, %v", result, err)
	}
}

func TestSearchRejectsCorruptChildAndInvalidExactCoordinate(t *testing.T) {
	t.Parallel()

	service, snapshot := compiledSearchFixture(t)
	query := string(snapshot.Directory.Documents[0].Operations[0].DetailID)
	normalized, err := normalizeSearchExact(query)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(normalized))
	prefix := hex.EncodeToString(digest[:1])[:1]
	bucketIndex := sort.Search(len(service.directory.ExactBuckets), func(index int) bool { return service.directory.ExactBuckets[index].Prefix >= prefix })
	if bucketIndex == len(service.directory.ExactBuckets) || service.directory.ExactBuckets[bucketIndex].Prefix != prefix {
		t.Fatal("exact bucket fixture is missing")
	}

	corrupt := *service
	corrupt.children = make(map[string]ChildArtifact, len(service.children))
	for pathValue, child := range service.children {
		corrupt.children[pathValue] = child
	}
	reference := service.directory.ExactBuckets[bucketIndex]
	child := corrupt.children[reference.Path]
	child.Bytes = append([]byte(nil), child.Bytes...)
	child.Bytes[len(child.Bytes)-1] ^= 1
	corrupt.children[reference.Path] = child
	if _, err := corrupt.Search(context.Background(), snapshot.ID, query); err == nil || !strings.Contains(err.Error(), "digest differs") {
		t.Fatalf("corrupt child error = %v", err)
	}

	invalid := *service
	segmentBytes, err := json.Marshal(SearchExactSegmentV1{SchemaVersion: 1, SearchVersion: searchVersion, Entries: []SearchExactEntryV1{{
		Key: normalized, Matches: []SearchExactMatchV1{{Record: math.MaxUint32, Priority: 1}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	invalidChild, err := contentAddressedSearchChild("exact", "search-exact", segmentBytes)
	if err != nil {
		t.Fatal(err)
	}
	invalid.directory.ExactBuckets = []SearchExactBucketReferenceV1{{
		Prefix: prefix, SearchSegmentReferenceV1: searchSegmentReference(invalidChild, 1, 1),
	}}
	invalid.children = make(map[string]ChildArtifact, len(service.children)+1)
	for pathValue, child := range service.children {
		invalid.children[pathValue] = child
	}
	invalid.children[invalidChild.Path] = invalidChild
	if _, err := invalid.Search(context.Background(), snapshot.ID, query); err == nil || !strings.Contains(err.Error(), "record ordinal") {
		t.Fatalf("invalid exact coordinate error = %v", err)
	}
}

func TestSearchSnippetNormalizesWhitespaceAndStaysWithinScalarCap(t *testing.T) {
	t.Parallel()

	snippet := searchSnippet("  alpha\n\tbeta  " + strings.Repeat("界", maxSearchSnippetScalars+10))
	if strings.ContainsAny(snippet, "\n\t") || strings.Contains(snippet, "  ") {
		t.Fatalf("snippet whitespace was not normalized: %q", snippet)
	}
	if len([]rune(snippet)) > maxSearchSnippetScalars || len([]rune(snippet)) > 256 {
		t.Fatalf("snippet scalar count = %d", len([]rune(snippet)))
	}
}

func compiledSearchFixture(t *testing.T) (*SearchService, CompiledSnapshot) {
	t.Helper()
	candidate, index := compilerFixture()
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewSearchService(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return service, snapshot
}

func equalSearchResults(left, right []SearchRecordV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].DetailID != right[index].DetailID {
			return false
		}
	}
	return true
}
