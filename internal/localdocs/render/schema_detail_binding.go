package render

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/araihu/manja/application/catalog"
)

// schemaDetailPreparationBinding proves schema-detail children were prepared
// from one immutable detail and document key before composition.
type schemaDetailPreparationBinding [sha256.Size]byte

type schemaDetailPreparationInput struct {
	Detail      catalog.DetailRecordV1 `json:"detail"`
	DocumentKey string                 `json:"documentKey"`
}

func bindSchemaDetailPreparation(detail catalog.DetailRecordV1, documentKey string) (schemaDetailPreparationBinding, error) {
	input, err := json.Marshal(schemaDetailPreparationInput{Detail: detail, DocumentKey: documentKey})
	if err != nil {
		return schemaDetailPreparationBinding{}, fmt.Errorf("bind schema detail preparation: %w", err)
	}
	return schemaDetailPreparationBinding(sha256.Sum256(input)), nil
}

func schemaDocumentKeyFromHref(value string) string {
	const marker = "/documents/"
	index := strings.LastIndex(value, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimSuffix(value[index+len(marker):], "/")
}
