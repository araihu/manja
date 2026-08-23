package catalogjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/araihu/manja/application/catalog"
)

const (
	maxCatalogBytes       = 4 << 20
	maxDetailBytes        = 2 << 20
	maxSchemaNodeBytes    = 2 << 20
	maxManifestBytes      = 4 << 20
	maxSearchBytes        = 4 << 20
	maxSearchSegmentBytes = 256 << 10
)

func EncodeCatalog(value catalog.CatalogArtifactV1) ([]byte, error) {
	return encodeCanonical(value, maxCatalogBytes, validateCatalog)
}

func DecodeCatalog(data []byte) (catalog.CatalogArtifactV1, error) {
	return decodeCanonical(data, maxCatalogBytes, validateCatalog)
}

func DecodeCatalogWithResourceLimits(data []byte, resourceLimits bool) (catalog.CatalogArtifactV1, error) {
	return decodeCanonical(data, optionalLimit(maxCatalogBytes, resourceLimits), validateCatalog)
}

func EncodeDetailShard(value catalog.DetailShardV1) ([]byte, error) {
	return encodeCanonical(value, maxDetailBytes, validateDetailShard)
}

func DecodeDetailShard(data []byte) (catalog.DetailShardV1, error) {
	return decodeCanonical(data, maxDetailBytes, validateDetailShard)
}

func EncodeSchemaNodeShard(value catalog.SchemaNodeShardV1) ([]byte, error) {
	return encodeCanonical(value, maxSchemaNodeBytes, validateSchemaNodeShard)
}

func DecodeSchemaNodeShard(data []byte) (catalog.SchemaNodeShardV1, error) {
	return decodeCanonical(data, maxSchemaNodeBytes, validateSchemaNodeShard)
}

func EncodeSearchDirectory(value catalog.SearchDirectoryV1) ([]byte, error) {
	return encodeCanonical(value, maxSearchBytes, validateSearchDirectory)
}

func DecodeSearchDirectory(data []byte) (catalog.SearchDirectoryV1, error) {
	return decodeCanonical(data, maxSearchBytes, validateSearchDirectory)
}

func DecodeSearchDirectoryWithResourceLimits(data []byte, resourceLimits bool) (catalog.SearchDirectoryV1, error) {
	return decodeCanonical(data, optionalLimit(maxSearchBytes, resourceLimits), validateSearchDirectory)
}

func EncodeSearchExactSegment(value catalog.SearchExactSegmentV1) ([]byte, error) {
	return encodeCanonical(value, maxSearchSegmentBytes, validateSearchExactSegment)
}

func DecodeSearchExactSegment(data []byte) (catalog.SearchExactSegmentV1, error) {
	return decodeCanonical(data, maxSearchSegmentBytes, validateSearchExactSegment)
}

func EncodeSearchPostingSegment(value catalog.SearchPostingSegmentV1) ([]byte, error) {
	return encodeCanonical(value, maxSearchSegmentBytes, validateSearchPostingSegment)
}

func DecodeSearchPostingSegment(data []byte) (catalog.SearchPostingSegmentV1, error) {
	return decodeCanonical(data, maxSearchSegmentBytes, validateSearchPostingSegment)
}

func EncodeSearchRecordSegment(value catalog.SearchRecordSegmentV1) ([]byte, error) {
	return encodeCanonical(value, maxSearchSegmentBytes, validateSearchRecordSegment)
}

func DecodeSearchRecordSegment(data []byte) (catalog.SearchRecordSegmentV1, error) {
	return decodeCanonical(data, maxSearchSegmentBytes, validateSearchRecordSegment)
}

func EncodeManifest(value catalog.ManifestV1) ([]byte, error) {
	return encodeCanonical(value, maxManifestBytes, validateManifest)
}

func DecodeManifest(data []byte) (catalog.ManifestV1, error) {
	return decodeCanonical(data, maxManifestBytes, validateManifest)
}

func DecodeManifestWithResourceLimits(data []byte, resourceLimits bool) (catalog.ManifestV1, error) {
	return decodeCanonical(data, optionalLimit(maxManifestBytes, resourceLimits), validateManifest)
}

func optionalLimit(limit int, resourceLimits bool) int {
	if resourceLimits {
		return limit
	}
	return 0
}

func encodeCanonical[T any](value T, limit int, validate func(T) error) ([]byte, error) {
	if err := validate(value); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("catalogjson: encode: %w", err)
	}
	if limit > 0 && len(data) > limit {
		return nil, fmt.Errorf("catalogjson: encoded bytes %d exceed %d", len(data), limit)
	}
	return data, nil
}

func decodeCanonical[T any](data []byte, limit int, validate func(T) error) (T, error) {
	var zero T
	if limit > 0 && len(data) > limit {
		return zero, fmt.Errorf("catalogjson: input bytes %d exceed %d", len(data), limit)
	}
	if !utf8.Valid(data) {
		return zero, fmt.Errorf("catalogjson: invalid UTF-8")
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var value T
	if err := decoder.Decode(&value); err != nil {
		return zero, fmt.Errorf("catalogjson: decode: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return zero, err
	}
	if err := validate(value); err != nil {
		return zero, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return zero, fmt.Errorf("catalogjson: re-encode: %w", err)
	}
	if !bytes.Equal(canonical, data) {
		return zero, fmt.Errorf("catalogjson: non-canonical bytes")
	}
	return value, nil
}
