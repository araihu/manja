package projection_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	"github.com/araihu/manja/internal/adapters/projectionjson"
)

const (
	githubFixtureDigest = "dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977"
	downloadSentinel    = "__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__"
	exampleSentinel     = "__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__"
)

func TestGitHubFixtureProjectionFitsBounds(t *testing.T) {
	index := githubFixtureIndex(t)
	documentA, err := (projection.Builder{}).Build(context.Background(), index)
	if err != nil {
		t.Fatalf("build GitHub projection: %v", err)
	}
	maxOperation, maxOperationIndex, maxSchema, maxSchemaIndex, maxNode, maxNodeIndex := largestRecords(t, documentA)
	if maxOperation > 256*1024 {
		t.Fatalf("operationDetails[%d]: record_too_large bytes=%d limit=%d", maxOperationIndex, maxOperation, 256*1024)
	}
	if maxSchema > 512*1024 {
		t.Fatalf("schemaDetails[%d]: record_too_large bytes=%d limit=%d", maxSchemaIndex, maxSchema, 512*1024)
	}
	if maxNode > 512*1024 {
		t.Fatalf("schemaNodes[%d]: record_too_large bytes=%d limit=%d", maxNodeIndex, maxNode, 512*1024)
	}
	bytesA, err := projectionjson.Marshal(documentA)
	if err != nil {
		t.Fatalf("marshal GitHub projection: %v", err)
	}
	documentB, err := (projection.Builder{}).Build(context.Background(), index)
	if err != nil {
		t.Fatalf("repeat build GitHub projection: %v", err)
	}
	bytesB, err := projectionjson.Marshal(documentB)
	if err != nil {
		t.Fatalf("repeat marshal GitHub projection: %v", err)
	}
	if !bytes.Equal(bytesA, bytesB) || projectionjson.Digest(bytesA) != projectionjson.Digest(bytesB) {
		t.Fatal("repeated GitHub projection changed bytes or digest")
	}
	if len(bytesA) > 16*1024*1024 {
		t.Fatalf("GitHub projection size = %d, exceeds 16 MiB", len(bytesA))
	}
	for _, sentinel := range [][]byte{[]byte(downloadSentinel), []byte(exampleSentinel)} {
		if bytes.Contains(bytesA, sentinel) {
			t.Fatal("GitHub projection contains excluded sentinel")
		}
	}
	t.Logf("GitHub projection bytes=%d digest=%s operations=%d schemas=%d max_operation=%d max_schema=%d max_node=%d schema_roots=%d schema_nodes=%d", len(bytesA), projectionjson.Digest(bytesA), len(documentA.OperationDetails), len(documentA.SchemaDetails), maxOperation, maxSchema, maxNode, schemaRootCount(documentA), len(documentA.SchemaNodes))
}

func BenchmarkBuildGitHubFixture(b *testing.B) {
	index := githubFixtureIndex(b)
	b.ResetTimer()
	for range b.N {
		if _, err := (projection.Builder{}).Build(context.Background(), index); err != nil {
			b.Fatal(err)
		}
	}
}

func githubFixtureIndex(tb testing.TB) domain.SpecIndex {
	tb.Helper()
	raw, err := os.ReadFile("../../internal/adapters/openapi/testdata/github-v3-rest.json")
	if err != nil {
		tb.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != githubFixtureDigest {
		tb.Fatalf("GitHub fixture digest = %s, want %s", got, githubFixtureDigest)
	}
	index, err := (openapiadapter.Parser{}).Parse(
		context.Background(),
		domain.SpecFile{Path: "github-v3-rest.json", Format: "json", Bytes: raw},
		domain.Revision{ID: "github-v3-rest-fixture"},
	)
	if err != nil {
		tb.Fatalf("parse GitHub fixture: %v", err)
	}
	index.ProjectID = "github"
	index.SpecDownload.JSON = []byte(downloadSentinel)
	index.ExampleSpecJSON = exampleSentinel
	return index
}

func largestRecords(tb testing.TB, document projection.Document) (int, int, int, int, int, int) {
	tb.Helper()
	maxOperation := 0
	maxOperationIndex := -1
	for index, record := range document.OperationDetails {
		encoded, err := json.Marshal(record)
		if err != nil {
			tb.Fatal(err)
		}
		if len(encoded) > maxOperation {
			maxOperation = len(encoded)
			maxOperationIndex = index
		}
	}
	maxSchema := 0
	maxSchemaIndex := -1
	for index, record := range document.SchemaDetails {
		encoded, err := json.Marshal(record)
		if err != nil {
			tb.Fatal(err)
		}
		if len(encoded) > maxSchema {
			maxSchema = len(encoded)
			maxSchemaIndex = index
		}
	}
	maxNode := 0
	maxNodeIndex := -1
	for index, record := range document.SchemaNodes {
		encoded, err := json.Marshal(record)
		if err != nil {
			tb.Fatal(err)
		}
		if len(encoded) > maxNode {
			maxNode = len(encoded)
			maxNodeIndex = index
		}
	}
	return maxOperation, maxOperationIndex, maxSchema, maxSchemaIndex, maxNode, maxNodeIndex
}

func schemaRootCount(document projection.Document) int {
	count := len(document.SchemaDetails)
	for _, operation := range document.OperationDetails {
		count += len(operation.Parameters)
		count += len(operation.RequestBody.MediaTypes)
		for _, response := range operation.Responses {
			count += len(response.MediaTypes)
		}
	}
	return count
}
