package openapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-openapi/jsonpointer"

	"github.com/araihu/manja/domain"
)

const defaultDiagnosticClass = "schema_validation"

type rawSchemaDefault struct {
	tokens      []string
	value       any
	schema      map[string]any
	reference   string
	jsonPointer string
}

func BuildKubernetesDefaultAllowlist(ctx context.Context, documents []domain.CatalogDocument) ([]byte, error) {
	documents = append([]domain.CatalogDocument(nil), documents...)
	sort.Slice(documents, func(i, j int) bool {
		if documents[i].SourcePath == documents[j].SourcePath {
			return documents[i].Key < documents[j].Key
		}
		return documents[i].SourcePath < documents[j].SourcePath
	})
	allowlist := defaultAllowlist{SchemaVersion: 1, Diagnostics: []defaultDiagnostic{}}
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if document.Format != domain.CatalogFormatJSON {
			return nil, fmt.Errorf("Kubernetes default allowlist requires JSON document %q", document.SourcePath)
		}
		if err := validateNoDuplicateJSONKeys(document.Bytes); err != nil {
			return nil, fmt.Errorf("catalog document %q: %w", document.Key, err)
		}
		doc, err := loadSpec(domain.SpecFile{Path: document.SourcePath, Format: string(document.Format), Bytes: document.Bytes})
		if err != nil {
			return nil, fmt.Errorf("catalog document %q: %w", document.Key, err)
		}
		diagnostics, err := observeDefaultDiagnostics(document.SourcePath, document.Bytes, doc)
		if err != nil {
			return nil, fmt.Errorf("catalog document %q defaults: %w", document.Key, err)
		}
		allowlist.Diagnostics = append(allowlist.Diagnostics, diagnostics...)
	}
	sortDefaultDiagnostics(allowlist.Diagnostics)
	data, err := json.MarshalIndent(allowlist, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Kubernetes default allowlist: %w", err)
	}
	return append(data, '\n'), nil
}

func observeDefaultDiagnostics(documentPath string, data []byte, doc *openapi3.T) ([]defaultDiagnostic, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode defaults source: %w", err)
	}
	if err := requireCatalogJSONEOF(decoder); err != nil {
		return nil, err
	}
	var defaults []rawSchemaDefault
	collectRawSchemaDefaults(root, nil, &defaults)
	diagnostics := make([]defaultDiagnostic, 0)
	for _, candidate := range defaults {
		schema, isSchema, err := resolvedSchemaAtTokens(doc, candidate.tokens)
		if err != nil {
			return nil, fmt.Errorf("resolve schema at %s: %w", candidate.jsonPointer, err)
		}
		if !isSchema {
			continue
		}
		if err := schema.VisitJSON(candidate.value); err == nil {
			continue
		}
		valueBytes, err := json.Marshal(candidate.value)
		if err != nil {
			return nil, fmt.Errorf("encode default at %s: %w", candidate.jsonPointer, err)
		}
		schemaBytes, err := json.Marshal(candidate.schema)
		if err != nil {
			return nil, fmt.Errorf("encode schema at %s: %w", candidate.jsonPointer, err)
		}
		diagnostics = append(diagnostics, defaultDiagnostic{
			DocumentPath:         documentPath,
			JSONPointer:          candidate.jsonPointer + "/default",
			Class:                defaultDiagnosticClass,
			OffendingValueSHA256: sha256String(valueBytes),
			SchemaSHA256:         sha256String(schemaBytes),
			Reference:            candidate.reference,
		})
	}
	sortDefaultDiagnostics(diagnostics)
	return diagnostics, nil
}

func collectRawSchemaDefaults(value any, tokens []string, result *[]rawSchemaDefault) {
	switch value := value.(type) {
	case map[string]any:
		if defaultValue, exists := value["default"]; exists {
			reference, _ := value["$ref"].(string)
			copiedTokens := append([]string(nil), tokens...)
			*result = append(*result, rawSchemaDefault{
				tokens: copiedTokens, value: defaultValue, schema: value, reference: reference,
				jsonPointer: encodeJSONPointer(copiedTokens),
			})
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			collectRawSchemaDefaults(value[key], append(tokens, key), result)
		}
	case []any:
		for index, item := range value {
			collectRawSchemaDefaults(item, append(tokens, fmt.Sprint(index)), result)
		}
	}
}

func resolvedSchemaAtTokens(doc *openapi3.T, tokens []string) (*openapi3.Schema, bool, error) {
	var current any = doc
	for _, token := range tokens {
		next, _, err := jsonpointer.GetForToken(current, token)
		if err != nil {
			return nil, false, err
		}
		current = next
	}
	switch value := current.(type) {
	case *openapi3.Schema:
		return value, true, nil
	case openapi3.Schema:
		return &value, true, nil
	default:
		return nil, false, nil
	}
}

func encodeJSONPointer(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	encoded := make([]string, len(tokens))
	for index, token := range tokens {
		token = strings.ReplaceAll(token, "~", "~0")
		encoded[index] = strings.ReplaceAll(token, "/", "~1")
	}
	return "/" + strings.Join(encoded, "/")
}

func sha256String(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortDefaultDiagnostics(diagnostics []defaultDiagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.DocumentPath != right.DocumentPath {
			return left.DocumentPath < right.DocumentPath
		}
		if left.JSONPointer != right.JSONPointer {
			return left.JSONPointer < right.JSONPointer
		}
		if left.Class != right.Class {
			return left.Class < right.Class
		}
		if left.OffendingValueSHA256 != right.OffendingValueSHA256 {
			return left.OffendingValueSHA256 < right.OffendingValueSHA256
		}
		if left.SchemaSHA256 != right.SchemaSHA256 {
			return left.SchemaSHA256 < right.SchemaSHA256
		}
		return left.Reference < right.Reference
	})
}
