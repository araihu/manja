package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const detailIdentityDomain = "manja.renderer.detail.v1"

type DetailID string

type detailHasher func([]byte) [32]byte

func NewOperationDetailID(catalogID, documentKey, method, literalPath string) (DetailID, error) {
	identity, _, err := newOperationDetailIdentity(catalogID, documentKey, method, literalPath, "", nil, sha256.Sum256)
	return identity, err
}

func NewOperationDetailIDWithRequestTarget(catalogID, documentKey, method, literalPath, requestTarget string, fixed []FixedQueryParameter) (DetailID, error) {
	identity, _, err := newOperationDetailIdentity(catalogID, documentKey, method, literalPath, requestTarget, fixed, sha256.Sum256)
	return identity, err
}

func NewSchemaDetailID(catalogID, documentKey, literalName string) (DetailID, error) {
	identity, _, err := newSchemaDetailIdentity(catalogID, documentKey, literalName, sha256.Sum256)
	return identity, err
}

func newOperationDetailIdentity(
	catalogID, documentKey, method, literalPath, requestTarget string,
	fixed []FixedQueryParameter,
	hasher detailHasher,
) (DetailID, []byte, error) {
	if err := validateCatalogKey("catalog id", catalogID); err != nil {
		return "", nil, err
	}
	if err := validateCatalogKey("document key", documentKey); err != nil {
		return "", nil, err
	}
	method = strings.ToUpper(method)
	if method == "" {
		return "", nil, fmt.Errorf("operation method is required")
	}
	for _, character := range method {
		if character < 'A' || character > 'Z' {
			return "", nil, fmt.Errorf("operation method is invalid")
		}
	}
	if err := ValidateOperationRequestTarget(literalPath, requestTarget, fixed); err != nil {
		return "", nil, err
	}
	identityTarget := literalPath
	if requestTarget != "" {
		identityTarget = requestTarget
	}
	preimage := detailPreimage(detailIdentityDomain, catalogID, documentKey, "operation", method, identityTarget)
	return detailIDFromPreimage(preimage, hasher), preimage, nil
}

func newSchemaDetailIdentity(
	catalogID, documentKey, literalName string,
	hasher detailHasher,
) (DetailID, []byte, error) {
	if err := validateCatalogKey("catalog id", catalogID); err != nil {
		return "", nil, err
	}
	if err := validateCatalogKey("document key", documentKey); err != nil {
		return "", nil, err
	}
	if err := ValidateCanonicalIdentity("schema name", literalName, false); err != nil {
		return "", nil, err
	}
	preimage := detailPreimage(detailIdentityDomain, catalogID, documentKey, "schema", literalName)
	return detailIDFromPreimage(preimage, hasher), preimage, nil
}

func detailPreimage(fields ...string) []byte {
	size := 0
	for _, field := range fields {
		size += 4 + len(field)
	}
	result := make([]byte, 0, size)
	for _, field := range fields {
		result = binary.BigEndian.AppendUint32(result, uint32(len(field)))
		result = append(result, field...)
	}
	return result
}

func detailIDFromPreimage(preimage []byte, hasher detailHasher) DetailID {
	digest := hasher(preimage)
	return DetailID("detail-sha256-" + hex.EncodeToString(digest[:]))
}
