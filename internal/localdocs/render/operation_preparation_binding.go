package render

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

// operationPreparationBinding proves child fragments were admitted against one
// parent operation and one link authority. Each child validates its complete
// effective node inventory before binding; those inventories intentionally
// differ because media summaries consume request roots while trees consume the
// recursive request and response graph.
type operationPreparationBinding struct {
	parent  [sha256.Size]byte
	context [sha256.Size]byte
}

type operationPreparationParent struct {
	Detail    catalog.DetailRecordV1 `json:"detail"`
	Operation domain.Operation       `json:"operation"`
}

type operationPreparationContext struct {
	DocumentHref string            `json:"documentHref"`
	SchemaLinks  map[string]string `json:"schemaLinks"`
}

func bindOperationPreparation(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	documentHref string,
	schemaLinks map[string]string,
) (operationPreparationBinding, error) {
	parent := operationPreparationParent{Detail: detail, Operation: operation}
	parentBytes, err := json.Marshal(parent)
	if err != nil {
		return operationPreparationBinding{}, fmt.Errorf("bind operation parent: %w", err)
	}
	contextBytes, err := json.Marshal(operationPreparationContext{
		DocumentHref: documentHref,
		SchemaLinks:  schemaLinks,
	})
	if err != nil {
		return operationPreparationBinding{}, fmt.Errorf("bind operation context: %w", err)
	}
	return operationPreparationBinding{
		parent:  sha256.Sum256(parentBytes),
		context: sha256.Sum256(contextBytes),
	}, nil
}

func bindOperationPreparationParent(detail catalog.DetailRecordV1, operation domain.Operation) ([sha256.Size]byte, error) {
	bytes, err := json.Marshal(operationPreparationParent{Detail: detail, Operation: operation})
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("bind operation parent: %w", err)
	}
	return sha256.Sum256(bytes), nil
}
