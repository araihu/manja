package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

func (activation Activation) SelectSchemaNode(pathValue, documentKey string, ordinal uint32, data []byte) (projection.SchemaNode, error) {
	artifact, exists := activation.artifact(pathValue, "schema-node")
	if !exists || !verifiedArtifactBytes(artifact, data) {
		return projection.SchemaNode{}, errors.New("local docs schema-node shard is not admitted")
	}
	if domain.ValidateCatalogDocumentKey(documentKey) != nil {
		return projection.SchemaNode{}, errors.New("local docs schema-node document key is invalid")
	}
	shard, err := catalogjson.DecodeSchemaNodeShard(data)
	if err != nil || shard.DocumentKey != documentKey || ordinal < shard.FirstOrdinal {
		return projection.SchemaNode{}, errors.New("local docs schema-node shard is invalid")
	}
	index := uint64(ordinal - shard.FirstOrdinal)
	if index >= uint64(len(shard.Nodes)) || shard.Nodes[index].Ordinal != ordinal {
		return projection.SchemaNode{}, errors.New("local docs schema node is missing")
	}
	return shard.Nodes[index], nil
}

func verifiedArtifactBytes(artifact ProjectionArtifact, data []byte) bool {
	if uint64(len(data)) != artifact.Length {
		return false
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == artifact.SHA256
}
