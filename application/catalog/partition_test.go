package catalog

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestPartitionDocumentMapsStableDetailsAndSchemaNodes(t *testing.T) {
	t.Parallel()

	operationID := testDetailID(1)
	schemaID := testDetailID(2)
	directory := DocumentDirectoryV1{
		Key:        "pets-v1",
		Operations: []OperationDirectoryV1{{DetailID: operationID, Method: "GET", Path: "/pets"}},
		Schemas:    []SchemaDirectoryV1{{DetailID: schemaID, Name: "Pet"}},
	}
	document := projection.Document{
		OperationDetails: []projection.OperationDetail{{Method: "GET", Path: "/pets", Summary: "List pets"}},
		SchemaDetails:    []projection.SchemaDetail{{Heading: "Pet", SchemaRef: 0}},
		SchemaNodes:      []projection.SchemaNode{{Ordinal: 0, ID: "node-pet", Name: "Pet", Type: "object"}},
	}
	artifacts, err := PartitionDocument("pets-v1", document, directory, DefaultPartitionLimits(DefaultBounds()))
	if err != nil {
		t.Fatal(err)
	}
	if artifacts.Directory.Operations[0].DetailChild == "" || artifacts.Directory.Schemas[0].DetailChild == "" {
		t.Fatalf("detail children were not mapped: %#v", artifacts.Directory)
	}
	if len(artifacts.Directory.SchemaNodeShards) != 1 || artifacts.Directory.SchemaNodeShards[0].FirstOrdinal != 0 || artifacts.Directory.SchemaNodeShards[0].LastOrdinal != 0 {
		t.Fatalf("schema node shards = %#v", artifacts.Directory.SchemaNodeShards)
	}
	for _, child := range artifacts.Children {
		if child.Kind == "detail" && !strings.Contains(string(child.Bytes), string(operationID)) && !strings.Contains(string(child.Bytes), string(schemaID)) {
			t.Fatalf("detail child lacks stable detail ID: %s", child.Bytes)
		}
	}
}

func TestPartitionDetailShardsUseLowerRecordAndByteBounds(t *testing.T) {
	t.Parallel()

	document, directory := detailPartitionFixture(257)
	limits := DefaultPartitionLimits(DefaultBounds())
	limits.DetailBytes = 16 << 20
	artifacts, err := PartitionDocument("large-v1", document, directory, limits)
	if err != nil {
		t.Fatal(err)
	}
	if got := countChildren(artifacts.Children, "detail"); got != 2 {
		t.Fatalf("detail shard count = %d, want 2", got)
	}

	oneDocument, oneDirectory := detailPartitionFixture(1)
	wide := limits
	wide.DetailRecords = 256
	wide.DetailBytes = 16 << 20
	one, err := PartitionDocument("one-v1", oneDocument, oneDirectory, wide)
	if err != nil {
		t.Fatal(err)
	}
	encodedLength := detailChild(one.Children).Length
	exact := wide
	exact.DetailBytes = encodedLength
	if _, err := PartitionDocument("one-v1", oneDocument, oneDirectory, exact); err != nil {
		t.Fatalf("exact detail byte boundary: %v", err)
	}
	exact.DetailBytes--
	if _, err := PartitionDocument("one-v1", oneDocument, oneDirectory, exact); err == nil {
		t.Fatal("detail shard one byte over limit was accepted")
	}
}

func TestPartitionSchemaNodesPreservesCyclesAndRecordBound(t *testing.T) {
	t.Parallel()

	nodes := make([]projection.SchemaNode, 513)
	for index := range nodes {
		nodes[index] = projection.SchemaNode{Ordinal: uint32(index), ID: fmt.Sprintf("node-%d", index), Name: fmt.Sprintf("Node%d", index)}
	}
	nodes[0].Items = []projection.SchemaNodeItem{{Ordinal: 0, ID: "to-one", SchemaRef: 1}}
	nodes[1].Items = []projection.SchemaNodeItem{{Ordinal: 0, ID: "to-zero", SchemaRef: 0}}
	limits := DefaultPartitionLimits(DefaultBounds())
	limits.SchemaNodeBytes = 16 << 20
	artifacts, err := PartitionDocument("schema-v1", projection.Document{SchemaNodes: nodes}, DocumentDirectoryV1{Key: "schema-v1"}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if got := countChildren(artifacts.Children, "schema-node"); got != 2 {
		t.Fatalf("schema-node shard count = %d, want 2", got)
	}
	var first SchemaNodeShardV1
	if err := json.Unmarshal(schemaNodeChild(artifacts.Children, 0).Bytes, &first); err != nil {
		t.Fatal(err)
	}
	if first.Nodes[0].Items[0].SchemaRef != 1 || first.Nodes[1].Items[0].SchemaRef != 0 {
		t.Fatalf("cycle refs changed: %#v / %#v", first.Nodes[0].Items, first.Nodes[1].Items)
	}

	again, err := PartitionDocument("schema-v1", projection.Document{SchemaNodes: nodes}, DocumentDirectoryV1{Key: "schema-v1"}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(artifacts, again) {
		t.Fatal("partition output changed across identical runs")
	}
}

func detailPartitionFixture(count int) (projection.Document, DocumentDirectoryV1) {
	document := projection.Document{OperationDetails: make([]projection.OperationDetail, count)}
	directory := DocumentDirectoryV1{Key: "large-v1", Operations: make([]OperationDirectoryV1, count)}
	for index := 0; index < count; index++ {
		literalPath := fmt.Sprintf("/items/%d", index)
		document.OperationDetails[index] = projection.OperationDetail{Method: "GET", Path: literalPath, Summary: strings.Repeat("x", 32)}
		directory.Operations[index] = OperationDirectoryV1{DetailID: testDetailID(index + 1), Method: "GET", Path: literalPath}
	}
	return document, directory
}

func testDetailID(value int) domain.DetailID {
	return domain.DetailID(fmt.Sprintf("detail-sha256-%064x", value))
}

func countChildren(children []ChildArtifact, kind string) int {
	count := 0
	for _, child := range children {
		if child.Kind == kind {
			count++
		}
	}
	return count
}

func detailChild(children []ChildArtifact) ChildArtifact {
	for _, child := range children {
		if child.Kind == "detail" {
			return child
		}
	}
	return ChildArtifact{}
}

func schemaNodeChild(children []ChildArtifact, firstOrdinal uint32) ChildArtifact {
	for _, child := range children {
		if child.Kind != "schema-node" {
			continue
		}
		var shard SchemaNodeShardV1
		if err := json.Unmarshal(child.Bytes, &shard); err == nil && shard.FirstOrdinal == firstOrdinal {
			return child
		}
	}
	return ChildArtifact{}
}
