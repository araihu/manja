package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"sort"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
)

func (activation Activation) SelectDetail(pathValue, documentKey string, detailID domain.DetailID, data []byte) (catalog.DetailRecordV1, error) {
	artifact, exists := activation.artifact(pathValue, "detail")
	if !exists || !verifiedArtifactBytes(artifact, data) {
		return catalog.DetailRecordV1{}, errors.New("local docs detail shard is not admitted")
	}
	if domain.ValidateCatalogDocumentKey(documentKey) != nil {
		return catalog.DetailRecordV1{}, errors.New("local docs detail document key is invalid")
	}
	shard, err := catalogjson.DecodeDetailShard(data)
	if err != nil || shard.DocumentKey != documentKey {
		return catalog.DetailRecordV1{}, errors.New("local docs detail shard is invalid")
	}
	index := sort.Search(len(shard.Records), func(index int) bool {
		return shard.Records[index].ID >= detailID
	})
	if index == len(shard.Records) || shard.Records[index].ID != detailID {
		return catalog.DetailRecordV1{}, errors.New("local docs detail is missing")
	}
	return shard.Records[index], nil
}

// PreparedSchemaNodeShard holds one completely verified, canonical shard. Its
// selected nodes are copied so callers cannot mutate subsequent selections.
type PreparedSchemaNodeShard struct {
	firstOrdinal uint32
	nodes        []projection.SchemaNode
	valid        bool
}

func (activation Activation) PrepareSchemaNodeShard(pathValue, documentKey string, data []byte) (PreparedSchemaNodeShard, error) {
	artifact, exists := activation.artifact(pathValue, "schema-node")
	if !exists || !verifiedArtifactBytes(artifact, data) {
		return PreparedSchemaNodeShard{}, errors.New("local docs schema-node shard is not admitted")
	}
	if domain.ValidateCatalogDocumentKey(documentKey) != nil {
		return PreparedSchemaNodeShard{}, errors.New("local docs schema-node document key is invalid")
	}
	shard, err := catalogjson.DecodeSchemaNodeShard(data)
	if err != nil || shard.DocumentKey != documentKey {
		return PreparedSchemaNodeShard{}, errors.New("local docs schema-node shard is invalid")
	}
	return PreparedSchemaNodeShard{firstOrdinal: shard.FirstOrdinal, nodes: shard.Nodes, valid: true}, nil
}

func (shard PreparedSchemaNodeShard) Select(ordinal uint32) (projection.SchemaNode, error) {
	if !shard.valid || ordinal < shard.firstOrdinal {
		return projection.SchemaNode{}, errors.New("local docs schema-node shard is invalid")
	}
	index := uint64(ordinal - shard.firstOrdinal)
	if index >= uint64(len(shard.nodes)) || shard.nodes[index].Ordinal != ordinal {
		return projection.SchemaNode{}, errors.New("local docs schema node is missing")
	}
	node := shard.nodes[index]
	node.Enum = slices.Clone(node.Enum)
	node.Constraints = slices.Clone(node.Constraints)
	node.Properties = slices.Clone(node.Properties)
	node.Items = slices.Clone(node.Items)
	return node, nil
}

func (activation Activation) SelectSchemaNode(pathValue, documentKey string, ordinal uint32, data []byte) (projection.SchemaNode, error) {
	shard, err := activation.PrepareSchemaNodeShard(pathValue, documentKey, data)
	if err != nil {
		return projection.SchemaNode{}, err
	}
	return shard.Select(ordinal)
}

func verifiedArtifactBytes(artifact ProjectionArtifact, data []byte) bool {
	if uint64(len(data)) != artifact.Length {
		return false
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == artifact.SHA256
}
