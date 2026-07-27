package projectionjson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/araihu/manja/application/projection"
)

const (
	maxOperationBytes  = 256 * 1024
	maxSchemaBytes     = 512 * 1024
	maxProjectionBytes = 16 * 1024 * 1024
)

func Marshal(document projection.Document) ([]byte, error) {
	if err := validateDocument(document); err != nil {
		return nil, err
	}
	for index := range document.OperationDetails {
		encoded, err := json.Marshal(document.OperationDetails[index])
		if err != nil {
			return nil, codecFailure("operationDetails", "invalid_source")
		}
		if len(encoded) > maxOperationBytes {
			return nil, codecFailure("operationDetails", "record_too_large")
		}
	}
	for index := range document.SchemaDetails {
		encoded, err := json.Marshal(document.SchemaDetails[index])
		if err != nil {
			return nil, codecFailure("schemaDetails", "invalid_source")
		}
		if len(encoded) > maxSchemaBytes {
			return nil, codecFailure("schemaDetails", "record_too_large")
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, codecFailure("document", "invalid_source")
	}
	if len(encoded) > maxProjectionBytes {
		return nil, codecFailure("document", "projection_too_large")
	}
	return encoded, nil
}

func Unmarshal(input []byte) (projection.Document, error) {
	if len(input) > maxProjectionBytes {
		return projection.Document{}, codecFailure("document", "projection_too_large")
	}
	if !utf8.Valid(input) {
		return projection.Document{}, codecFailure("document", "invalid_utf8")
	}
	if err := validateJSONTokens(input, 256, canonicalWireNumber); err != nil {
		return projection.Document{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var document projection.Document
	if err := decoder.Decode(&document); err != nil {
		return projection.Document{}, codecFailure("document", "unknown_field")
	}
	if err := requireDecoderEOF(decoder); err != nil {
		return projection.Document{}, codecFailure("document", "non_canonical")
	}
	reencoded, err := Marshal(document)
	if err != nil {
		return projection.Document{}, err
	}
	if !bytes.Equal(reencoded, input) {
		return projection.Document{}, codecFailure("document", "non_canonical")
	}
	return document, nil
}

func Digest(input []byte) string {
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

func requireDecoderEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func codecFailure(path, class string) error {
	message := fmt.Sprintf("projectionjson[%s]: %s", path, class)
	if len(message) > 256 || !utf8.ValidString(message) {
		message = "projectionjson[document]: invalid_source"
	}
	return errors.New(message)
}
