package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

const detailIdentityDomain = "manja.renderer.detail.v1"

type DetailID string

type detailHasher func([]byte) [32]byte

func NewOperationDetailID(catalogID, documentKey, method, literalPath string) (DetailID, error) {
	identity, _, err := newOperationDetailIdentity(catalogID, documentKey, method, literalPath, sha256.Sum256)
	return identity, err
}

func NewSchemaDetailID(catalogID, documentKey, literalName string) (DetailID, error) {
	identity, _, err := newSchemaDetailIdentity(catalogID, documentKey, literalName, sha256.Sum256)
	return identity, err
}

func newOperationDetailIdentity(
	catalogID, documentKey, method, literalPath string,
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
	if err := ValidateCanonicalIdentity("operation path", literalPath, false); err != nil {
		return "", nil, err
	}
	cleanInput := literalPath
	if literalPath != "/" && strings.HasSuffix(literalPath, "/") {
		cleanInput = strings.TrimSuffix(literalPath, "/")
	}
	if !strings.HasPrefix(literalPath, "/") || path.Clean(cleanInput) != cleanInput || strings.ContainsAny(literalPath, " ?#") {
		return "", nil, fmt.Errorf("operation path is invalid")
	}
	preimage := detailPreimage(detailIdentityDomain, catalogID, documentKey, "operation", method, literalPath)
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
