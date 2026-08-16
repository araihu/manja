package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
)

func TestActivationSelectDetailRequiresAdmittedCanonicalShard(t *testing.T) {
	operationID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	schemaID := domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
	data := mustDetailShard(t, "core-v1", operationID, schemaID)
	activation := shardActivation(t, data, mustSchemaNodeShard(t, "core-v1", 7))

	operation, err := activation.SelectDetail("details/core.json", "core-v1", operationID, data)
	if err != nil {
		t.Fatal(err)
	}
	if operation.ID != operationID || operation.Kind != "operation" || operation.Operation == nil || operation.Schema != nil || operation.Operation.Heading != "List Pods" {
		t.Fatalf("operation = %#v", operation)
	}

	schema, err := activation.SelectDetail("details/core.json", "core-v1", schemaID, data)
	if err != nil {
		t.Fatal(err)
	}
	if schema.ID != schemaID || schema.Kind != "schema" || schema.Schema == nil || schema.Operation != nil || schema.Schema.Heading != "Pod" {
		t.Fatalf("schema = %#v", schema)
	}
}

func TestActivationSelectSchemaNodeRequiresAdmittedCanonicalShard(t *testing.T) {
	detailData := mustDetailShard(t, "core-v1", domain.DetailID("detail-sha256-"+strings.Repeat("a", 64)))
	nodeData := mustSchemaNodeShard(t, "core-v1", 7)
	activation := shardActivation(t, detailData, nodeData)

	node, err := activation.SelectSchemaNode("schema-nodes/core.json", "core-v1", 8, nodeData)
	if err != nil {
		t.Fatal(err)
	}
	if node.Ordinal != 8 || node.ID != "node-spec" || node.Name != "PodSpec" {
		t.Fatalf("node = %#v", node)
	}
}

func TestActivationSelectDetailFailsClosed(t *testing.T) {
	operationID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	missingID := domain.DetailID("detail-sha256-" + strings.Repeat("c", 64))
	valid := mustDetailShard(t, "core-v1", operationID)
	nodes := mustSchemaNodeShard(t, "core-v1", 7)
	activation := shardActivation(t, valid, nodes)

	tests := []struct {
		name       string
		activation Activation
		path       string
		document   string
		id         domain.DetailID
		data       []byte
	}{
		{name: "unknown path", activation: activation, path: "details/missing.json", document: "core-v1", id: operationID, data: valid},
		{name: "wrong kind", activation: activation, path: "schema-nodes/core.json", document: "core-v1", id: operationID, data: nodes},
		{name: "changed length", activation: activation, path: "details/core.json", document: "core-v1", id: operationID, data: append(append([]byte(nil), valid...), ' ')},
		{name: "same length substitution", activation: activation, path: "details/core.json", document: "core-v1", id: operationID, data: sameLengthMutation(valid)},
		{name: "invalid expected document", activation: activation, path: "details/core.json", document: "../core-v1", id: operationID, data: valid},
		{name: "wrong document", activation: shardActivation(t, mustDetailShard(t, "other", operationID), nodes), path: "details/core.json", document: "core-v1", id: operationID, data: mustDetailShard(t, "other", operationID)},
		{name: "missing detail", activation: activation, path: "details/core.json", document: "core-v1", id: missingID, data: valid},
	}
	for _, mutation := range detailJSONMutations(valid) {
		tests = append(tests, struct {
			name       string
			activation Activation
			path       string
			document   string
			id         domain.DetailID
			data       []byte
		}{
			name: mutation.name, activation: shardActivation(t, mutation.data, nodes),
			path: "details/core.json", document: "core-v1", id: operationID, data: mutation.data,
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.activation.SelectDetail(test.path, test.document, test.id, test.data)
			requireZeroDetail(t, got, err)
		})
	}
}

func TestActivationSelectSchemaNodeFailsClosed(t *testing.T) {
	operationID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	detailData := mustDetailShard(t, "core-v1", operationID)
	valid := mustSchemaNodeShard(t, "core-v1", 7)
	activation := shardActivation(t, detailData, valid)
	wrongDocument := mustSchemaNodeShard(t, "other", 7)

	tests := []struct {
		name       string
		activation Activation
		path       string
		document   string
		ordinal    uint32
		data       []byte
	}{
		{name: "unknown path", activation: activation, path: "schema-nodes/missing.json", document: "core-v1", ordinal: 7, data: valid},
		{name: "wrong kind", activation: activation, path: "details/core.json", document: "core-v1", ordinal: 7, data: detailData},
		{name: "changed length", activation: activation, path: "schema-nodes/core.json", document: "core-v1", ordinal: 7, data: append(append([]byte(nil), valid...), ' ')},
		{name: "same length substitution", activation: activation, path: "schema-nodes/core.json", document: "core-v1", ordinal: 7, data: sameLengthMutation(valid)},
		{name: "invalid expected document", activation: activation, path: "schema-nodes/core.json", document: "../core-v1", ordinal: 7, data: valid},
		{name: "wrong document", activation: shardActivation(t, detailData, wrongDocument), path: "schema-nodes/core.json", document: "core-v1", ordinal: 7, data: wrongDocument},
		{name: "ordinal below shard", activation: activation, path: "schema-nodes/core.json", document: "core-v1", ordinal: 6, data: valid},
		{name: "ordinal above shard", activation: activation, path: "schema-nodes/core.json", document: "core-v1", ordinal: 9, data: valid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.activation.SelectSchemaNode(test.path, test.document, test.ordinal, test.data)
			requireZeroNode(t, got, err)
		})
	}
}

func TestActivationShardSelectionDoesNotAliasInputOrState(t *testing.T) {
	operationID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	detailData := mustDetailShard(t, "core-v1", operationID)
	nodeData := mustSchemaNodeShard(t, "core-v1", 7)
	originalDetail := append([]byte(nil), detailData...)
	originalNodes := append([]byte(nil), nodeData...)
	activation := shardActivation(t, detailData, nodeData)
	wantInventory := activation.Inventory()

	detail, err := activation.SelectDetail("details/core.json", "core-v1", operationID, detailData)
	if err != nil {
		t.Fatal(err)
	}
	node, err := activation.SelectSchemaNode("schema-nodes/core.json", "core-v1", 7, nodeData)
	if err != nil {
		t.Fatal(err)
	}
	for index := range detailData {
		detailData[index] = 'x'
	}
	for index := range nodeData {
		nodeData[index] = 'x'
	}
	if detail.Operation == nil || detail.Operation.Heading != "List Pods" || node.Name != "Pod" {
		t.Fatalf("selected values alias input: detail=%#v node=%#v", detail, node)
	}
	detail.Operation.Heading = "changed"
	node.Name = "changed"
	freshDetail, err := activation.SelectDetail("details/core.json", "core-v1", operationID, originalDetail)
	if err != nil {
		t.Fatal(err)
	}
	freshNode, err := activation.SelectSchemaNode("schema-nodes/core.json", "core-v1", 7, originalNodes)
	if err != nil {
		t.Fatal(err)
	}
	if freshDetail.Operation.Heading != "List Pods" || freshNode.Name != "Pod" || !reflect.DeepEqual(activation.Inventory(), wantInventory) {
		t.Fatalf("selection mutated activation state: detail=%#v node=%#v inventory=%#v", freshDetail, freshNode, activation.Inventory())
	}
}

func mustDetailShard(t *testing.T, documentKey string, ids ...domain.DetailID) []byte {
	t.Helper()
	records := make([]catalog.DetailRecordV1, 0, len(ids))
	for index, id := range ids {
		if index == 0 {
			records = append(records, catalog.DetailRecordV1{
				ID: id, Kind: "operation", Operation: &projection.OperationDetail{
					ID: string(id), Anchor: string(id), Href: "?selected=" + string(id),
					HeadingID: string(id), Heading: "List Pods", HeadingLevel: 2,
					Method: "GET", Path: "/api/v1/pods",
				},
			})
			continue
		}
		records = append(records, catalog.DetailRecordV1{
			ID: id, Kind: "schema", Schema: &projection.SchemaDetail{
				ID: string(id), Anchor: string(id), Href: "?selected=" + string(id),
				HeadingID: string(id), Heading: "Pod", HeadingLevel: 2, SchemaRef: 7,
			},
		})
	}
	data, err := catalogjson.EncodeDetailShard(catalog.DetailShardV1{
		SchemaVersion: 1, DocumentKey: documentKey, Records: records,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustSchemaNodeShard(t *testing.T, documentKey string, first uint32) []byte {
	t.Helper()
	data, err := catalogjson.EncodeSchemaNodeShard(catalog.SchemaNodeShardV1{
		SchemaVersion: 1,
		DocumentKey:   documentKey,
		FirstOrdinal:  first,
		Nodes: []projection.SchemaNode{
			{Ordinal: first, ID: "node-pod", Name: "Pod", Type: "object"},
			{Ordinal: first + 1, ID: "node-spec", Name: "PodSpec", Type: "object"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func shardActivation(t *testing.T, detailData, nodeData []byte) Activation {
	t.Helper()
	children := []catalog.ChildIdentityV1{
		{Path: "catalog.json", Kind: "catalog", Length: 1, SHA256: strings.Repeat("1", 64)},
		shardIdentity("details/core.json", "detail", detailData),
		shardIdentity("schema-nodes/core.json", "schema-node", nodeData),
	}
	identity := catalog.SnapshotIdentityV1{
		SchemaVersion: 1,
		CatalogID:     "kubernetes",
		RevisionID:    "git-0123456789abcdef",
		Versions:      catalog.CompilerVersions{ProjectionFormat: "projection-v2"},
		Children:      append([]catalog.ChildIdentityV1(nil), children...),
	}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityBytes)
	digestHex := hex.EncodeToString(digest[:])
	snapshotID := "snapshot-sha256-" + digestHex
	manifestBytes, err := catalogjson.EncodeManifest(catalog.ManifestV1{
		SchemaVersion: 1,
		SnapshotID:    catalog.SnapshotID(snapshotID),
		Identity:      identity,
		Children:      append([]catalog.ChildIdentityV1(nil), children...),
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := DescriptorV1{
		SchemaVersion:         1,
		CatalogID:             "kubernetes",
		PublicationKey:        "public-kubernetes",
		Public:                true,
		Anonymous:             true,
		PublicationBase:       "/kubernetes/",
		SnapshotID:            snapshotID,
		RevisionID:            identity.RevisionID,
		ProjectionFormat:      "projection-v2",
		ProjectionDigest:      digestHex,
		ProjectionManifestURL: "/kubernetes/snapshots/" + snapshotID + "/manifest.json",
		CatalogURL:            "/kubernetes/snapshots/" + snapshotID + "/catalog.json",
		SearchDataBase:        "/kubernetes/snapshots/" + snapshotID + "/search-data/",
		ProjectionDataBase:    "/kubernetes/snapshots/" + snapshotID + "/projection-data/",
	}
	activation, err := Admit(descriptor, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	return activation
}

func shardIdentity(path, kind string, data []byte) catalog.ChildIdentityV1 {
	digest := sha256.Sum256(data)
	return catalog.ChildIdentityV1{
		Path: path, Kind: kind, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:]),
	}
}

type shardJSONMutation struct {
	name string
	data []byte
}

func detailJSONMutations(valid []byte) []shardJSONMutation {
	return []shardJSONMutation{
		{name: "duplicate key", data: append([]byte(`{"schemaVersion":1,`), valid[1:]...)},
		{name: "unknown field", data: append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"unknown":true}`)...)},
		{name: "trailing value", data: append(append([]byte(nil), valid...), []byte(`{}`)...)},
		{name: "noncanonical whitespace", data: append([]byte{' '}, valid...)},
	}
}

func sameLengthMutation(data []byte) []byte {
	changed := append([]byte(nil), data...)
	changed[len(changed)/2] ^= 1
	return changed
}

func requireZeroDetail(t *testing.T, got catalog.DetailRecordV1, err error) {
	t.Helper()
	if err == nil || !reflect.DeepEqual(got, catalog.DetailRecordV1{}) {
		t.Fatalf("detail = %#v err=%v, want zero plus error", got, err)
	}
}

func requireZeroNode(t *testing.T, got projection.SchemaNode, err error) {
	t.Helper()
	if err == nil || !reflect.DeepEqual(got, projection.SchemaNode{}) {
		t.Fatalf("node = %#v err=%v, want zero plus error", got, err)
	}
}
