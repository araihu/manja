package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/araihu/manja/application/port"
	"github.com/araihu/manja/domain"
)

type CatalogParser struct {
	kubernetesAllowlist defaultAllowlist
	hasAllowlist        bool
	resourceLimits      bool
}

type defaultAllowlist struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Diagnostics   []defaultDiagnostic `json:"diagnostics"`
}

type defaultDiagnostic struct {
	DocumentPath         string `json:"documentPath"`
	JSONPointer          string `json:"jsonPointer"`
	Class                string `json:"class"`
	OffendingValueSHA256 string `json:"offendingValueSha256"`
	SchemaSHA256         string `json:"schemaSha256"`
	Reference            string `json:"reference"`
}

func NewCatalogParser(kubernetesAllowlist []byte) (*CatalogParser, error) {
	return NewCatalogParserWithResourceLimits(kubernetesAllowlist, true)
}

func NewCatalogParserWithResourceLimits(kubernetesAllowlist []byte, resourceLimits bool) (*CatalogParser, error) {
	parser := &CatalogParser{resourceLimits: resourceLimits}
	if len(kubernetesAllowlist) == 0 {
		return parser, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(kubernetesAllowlist))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parser.kubernetesAllowlist); err != nil {
		return nil, fmt.Errorf("decode Kubernetes default allowlist: %w", err)
	}
	if err := requireCatalogJSONEOF(decoder); err != nil {
		return nil, err
	}
	if parser.kubernetesAllowlist.SchemaVersion != 1 {
		return nil, fmt.Errorf("Kubernetes default allowlist schema version 1 is required")
	}
	if err := validateDefaultAllowlist(parser.kubernetesAllowlist); err != nil {
		return nil, err
	}
	parser.hasAllowlist = true
	return parser, nil
}

func (parser *CatalogParser) Parse(ctx context.Context, candidate domain.CatalogCandidate) (domain.CatalogIndex, error) {
	if err := ctx.Err(); err != nil {
		return domain.CatalogIndex{}, err
	}
	validation := domain.ValidationOptions{ResourceLimits: parser.resourceLimits}
	if err := domain.ValidateCatalogCandidateWithOptions(candidate, validation); err != nil {
		return domain.CatalogIndex{}, err
	}
	if candidate.ProfileID == domain.CompatibilityProfileKubernetes && !parser.hasAllowlist {
		return domain.CatalogIndex{}, fmt.Errorf("Kubernetes profile requires an exact default allowlist")
	}
	if candidate.ProfileID == domain.CompatibilityProfileKubernetes && len(candidate.SupportFiles) != 0 {
		return domain.CatalogIndex{}, fmt.Errorf("Kubernetes profile v1 does not admit support files outside its exact default audit")
	}
	if candidate.ProfileID != domain.CompatibilityProfileStrict && candidate.ProfileID != domain.CompatibilityProfileKubernetes {
		return domain.CatalogIndex{}, fmt.Errorf("compatibility profile %q is unsupported", candidate.ProfileID)
	}

	documents := append([]domain.CatalogDocument(nil), candidate.Documents...)
	sort.Slice(documents, func(i, j int) bool { return documents[i].Key < documents[j].Key })
	index := domain.CatalogIndex{
		CatalogID: candidate.ID, RevisionID: candidate.Revision.ID, Title: candidate.Title,
		Branding: candidate.Branding, ProfileID: candidate.ProfileID,
		Documents: make([]domain.CatalogDocumentIndex, 0, len(documents)),
	}
	captured := make(map[string][]byte, len(documents)+len(candidate.SupportFiles))
	for _, document := range documents {
		captured[document.SourcePath] = append([]byte(nil), document.Bytes...)
	}
	for _, support := range candidate.SupportFiles {
		if strings.EqualFold(filepath.Ext(support.SourcePath), ".json") {
			if err := validateNoDuplicateJSONKeys(support.Bytes); err != nil {
				return domain.CatalogIndex{}, fmt.Errorf("catalog support file %q: %w", support.SourcePath, err)
			}
		}
		captured[support.SourcePath] = append([]byte(nil), support.Bytes...)
	}
	var observedDefaults []defaultDiagnostic
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return domain.CatalogIndex{}, err
		}
		if document.Format == domain.CatalogFormatJSON {
			if err := validateNoDuplicateJSONKeys(document.Bytes); err != nil {
				return domain.CatalogIndex{}, fmt.Errorf("catalog document %q: %w", document.Key, err)
			}
		}
		file := domain.SpecFile{Path: document.SourcePath, Format: string(document.Format), Bytes: document.Bytes}
		revision := domain.Revision{ID: candidate.Revision.ID, CommitSHA: candidate.Revision.CommitSHA}
		doc, loadErr := loadCapturedSpec(ctx, file, captured)
		if loadErr != nil {
			return domain.CatalogIndex{}, fmt.Errorf("catalog document %q: %w", document.Key, loadErr)
		}
		var validationOptions []openapi3.ValidationOption
		if candidate.ProfileID == domain.CompatibilityProfileKubernetes {
			diagnostics, observeErr := observeDefaultDiagnostics(document.SourcePath, document.Bytes, doc)
			if observeErr != nil {
				return domain.CatalogIndex{}, fmt.Errorf("catalog document %q defaults: %w", document.Key, observeErr)
			}
			observedDefaults = append(observedDefaults, diagnostics...)
			validationOptions = append(validationOptions, openapi3.DisableSchemaDefaultsValidation())
		}
		validationOptions = append(validationOptions, openapi3.DisableExamplesValidation())
		if validateErr := doc.Validate(ctx, validationOptions...); validateErr != nil {
			return domain.CatalogIndex{}, fmt.Errorf("catalog document %q: %w", document.Key, validateErr)
		}
		documentIndex, err := projectSpec(doc, file, revision)
		if err != nil {
			return domain.CatalogIndex{}, fmt.Errorf("catalog document %q: %w", document.Key, err)
		}
		documentIndex.SpecDownload.JSON = append([]byte(nil), document.Bytes...)
		documentIndex.SpecDownload.Filename = filepath.Base(document.SourcePath)
		index.Documents = append(index.Documents, domain.CatalogDocumentIndex{
			Key: document.Key, SourcePath: document.SourcePath, Index: documentIndex,
		})
	}
	if candidate.ProfileID == domain.CompatibilityProfileKubernetes {
		sortDefaultDiagnostics(observedDefaults)
		if !equalDefaultDiagnostics(observedDefaults, parser.kubernetesAllowlist.Diagnostics) {
			encoded, _ := json.Marshal(observedDefaults)
			return domain.CatalogIndex{}, fmt.Errorf("Kubernetes default diagnostics differ from exact allowlist: observed %s", encoded)
		}
	}
	if err := domain.ValidateCatalogIndexWithOptions(index, validation); err != nil {
		return domain.CatalogIndex{}, err
	}
	return index, nil
}

func validateDefaultAllowlist(allowlist defaultAllowlist) error {
	seen := make(map[string]struct{}, len(allowlist.Diagnostics))
	for index, diagnostic := range allowlist.Diagnostics {
		if strings.TrimSpace(diagnostic.DocumentPath) == "" || strings.TrimSpace(diagnostic.JSONPointer) == "" || diagnostic.Class != defaultDiagnosticClass ||
			!isLowerHexDigest(diagnostic.OffendingValueSHA256) || !isLowerHexDigest(diagnostic.SchemaSHA256) {
			return fmt.Errorf("Kubernetes default allowlist diagnostic %d is invalid", index)
		}
		key := diagnostic.DocumentPath + "\x00" + diagnostic.JSONPointer + "\x00" + diagnostic.Class
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("Kubernetes default allowlist diagnostic %d is duplicated", index)
		}
		seen[key] = struct{}{}
	}
	if len(allowlist.Diagnostics) > 1 {
		copyValue := append([]defaultDiagnostic(nil), allowlist.Diagnostics...)
		sortDefaultDiagnostics(copyValue)
		if !equalDefaultDiagnostics(copyValue, allowlist.Diagnostics) {
			return fmt.Errorf("Kubernetes default allowlist diagnostics must be sorted")
		}
	}
	return nil
}

func equalDefaultDiagnostics(left, right []defaultDiagnostic) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validateNoDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateCatalogJSONValue(decoder); err != nil {
		return err
	}
	if err := requireCatalogJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func validateCatalogJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("decode JSON object key: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("decode JSON object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := validateCatalogJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("decode JSON object")
		}
	case '[':
		for decoder.More() {
			if err := validateCatalogJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("decode JSON array")
		}
	default:
		return fmt.Errorf("decode JSON delimiter %q", delimiter)
	}
	return nil
}

func requireCatalogJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("trailing JSON value")
	}
	return fmt.Errorf("decode trailing JSON: %w", err)
}

var _ port.CatalogParser = (*CatalogParser)(nil)
