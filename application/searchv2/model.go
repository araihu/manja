package searchv2

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	"github.com/araihu/manja/domain"
)

const (
	// SchemaVersion identifies the JSON object shapes emitted by this package.
	SchemaVersion uint32 = 2
	// RunFormatVersion identifies the canonical semantic-run wire contract.
	RunFormatVersion = "manja.search.semantic-run.v2"
	// AnalyzerVersion identifies tokenization and normalization semantics. A
	// future analyzer change must use a new value even when the wire shape stays
	// compatible, so cached runs cannot cross analyzer boundaries.
	AnalyzerVersion = "manja.search.analyzer.v2"
)

const (
	semanticIdentityDomain = "manja.search.semantic-record.v2"
	recordIdentityDomain   = "manja.search.occurrence-record.v2"
)

// RecordKind identifies a searchable deployment navigation or spec resource.
type RecordKind string

const (
	KindCatalog   RecordKind = "catalog"
	KindSpec      RecordKind = "spec"
	KindPath      RecordKind = "path"
	KindOperation RecordKind = "operation"
	KindSchema    RecordKind = "schema"
)

// SemanticRecordID identifies one resource within a spec independently of its
// deployment catalog membership.
type SemanticRecordID string

// RecordID identifies one deployment occurrence. The same spec resource gets
// a distinct ID in each catalog in which the spec is published.
type RecordID string

// SemanticRecordInput is the catalog-independent projection emitted once per
// spec. Catalog membership and public hrefs are deliberately added later.
//
// LogicalKey is kind-specific: the spec key for specs, a literal OpenAPI path
// for paths, METHOD + SP + literal path for operations, and the canonical
// component JSON pointer for schemas.
type SemanticRecordInput struct {
	Kind        RecordKind
	SpecKey     string
	SpecTitle   string
	LogicalKey  string
	Title       string
	Description string
	OperationID string
	Method      string
	Path        string
}

// SemanticRecord is canonical reusable search content for one spec resource.
// It contains neither catalog identity nor catalog-specific URLs.
type SemanticRecord struct {
	ID          SemanticRecordID `json:"id"`
	Kind        RecordKind       `json:"kind"`
	SpecKey     string           `json:"specKey"`
	SpecTitle   string           `json:"specTitle"`
	LogicalKey  string           `json:"logicalKey"`
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	OperationID string           `json:"operationId,omitempty"`
	Method      string           `json:"method,omitempty"`
	Path        string           `json:"path,omitempty"`
}

// RecordInput is one catalog-specific deployment occurrence. Catalog records
// are created directly; every other kind should normally be expanded from a
// SemanticRecord with ExpandSemanticRun.
type RecordInput struct {
	Kind         RecordKind
	CatalogKey   string
	CatalogTitle string
	SpecKey      string
	SpecTitle    string
	LogicalKey   string
	Title        string
	Description  string
	OperationID  string
	Method       string
	Path         string
	PageHref     string
	FragmentHref string
}

// Record is the canonical deployment-wide search input. It deliberately
// contains no provider-specific source locators and no trusted HTML.
type Record struct {
	ID           RecordID   `json:"id"`
	Kind         RecordKind `json:"kind"`
	CatalogKey   string     `json:"catalogKey"`
	CatalogTitle string     `json:"catalogTitle"`
	SpecKey      string     `json:"specKey,omitempty"`
	SpecTitle    string     `json:"specTitle,omitempty"`
	LogicalKey   string     `json:"logicalKey"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	OperationID  string     `json:"operationId,omitempty"`
	Method       string     `json:"method,omitempty"`
	Path         string     `json:"path,omitempty"`
	PageHref     string     `json:"pageHref"`
	FragmentHref string     `json:"fragmentHref,omitempty"`
}

// NewSemanticRecord validates and canonicalizes a reusable spec record.
func NewSemanticRecord(input SemanticRecordInput) (SemanticRecord, error) {
	input.Method = strings.ToUpper(input.Method)
	if err := validateSemanticRecordInput(input); err != nil {
		return SemanticRecord{}, err
	}
	id, err := NewSemanticRecordID(input.SpecKey, input.Kind, input.LogicalKey)
	if err != nil {
		return SemanticRecord{}, err
	}
	return SemanticRecord{
		ID: id, Kind: input.Kind, SpecKey: input.SpecKey, SpecTitle: input.SpecTitle,
		LogicalKey: input.LogicalKey, Title: input.Title, Description: input.Description,
		OperationID: input.OperationID, Method: input.Method, Path: input.Path,
	}, nil
}

// NewRecord validates and canonicalizes one deployment occurrence. Navigation
// records may omit FragmentHref; path, operation, and schema records may not.
func NewRecord(input RecordInput) (Record, error) {
	input.Method = strings.ToUpper(input.Method)
	if err := validateRecordInput(input); err != nil {
		return Record{}, err
	}
	id, err := NewRecordID(input.CatalogKey, input.SpecKey, input.Kind, input.LogicalKey)
	if err != nil {
		return Record{}, err
	}
	return Record{
		ID: id, Kind: input.Kind,
		CatalogKey: input.CatalogKey, CatalogTitle: input.CatalogTitle,
		SpecKey: input.SpecKey, SpecTitle: input.SpecTitle,
		LogicalKey: input.LogicalKey, Title: input.Title, Description: input.Description,
		OperationID: input.OperationID, Method: input.Method, Path: input.Path,
		PageHref: input.PageHref, FragmentHref: input.FragmentHref,
	}, nil
}

// NewSemanticRecordID derives an identity unaffected by catalog assignment.
func NewSemanticRecordID(specKey string, kind RecordKind, logicalKey string) (SemanticRecordID, error) {
	if err := domain.ValidateCatalogDocumentKey(specKey); err != nil {
		return "", err
	}
	if !kind.semantic() {
		return "", fmt.Errorf("semantic search record kind %q is unsupported", kind)
	}
	if err := domain.ValidateCanonicalIdentity("search logical key", logicalKey, false); err != nil {
		return "", err
	}
	digest := sha256.Sum256(framedStrings(semanticIdentityDomain, specKey, string(kind), logicalKey))
	return SemanticRecordID("search-semantic-sha256-" + hex.EncodeToString(digest[:])), nil
}

// NewRecordID derives a stable occurrence ID using length-framed fields.
func NewRecordID(catalogKey, specKey string, kind RecordKind, logicalKey string) (RecordID, error) {
	if err := domain.ValidateCatalogID(catalogKey); err != nil {
		return "", err
	}
	if kind != KindCatalog {
		if err := domain.ValidateCatalogDocumentKey(specKey); err != nil {
			return "", err
		}
	} else if specKey != "" {
		return "", fmt.Errorf("catalog search record must not contain a spec key")
	}
	if !kind.valid() {
		return "", fmt.Errorf("search record kind %q is unsupported", kind)
	}
	if err := domain.ValidateCanonicalIdentity("search logical key", logicalKey, false); err != nil {
		return "", err
	}
	digest := sha256.Sum256(framedStrings(recordIdentityDomain, catalogKey, specKey, string(kind), logicalKey))
	return RecordID("search-record-sha256-" + hex.EncodeToString(digest[:])), nil
}

func validateRecordInput(input RecordInput) error {
	if !input.Kind.valid() {
		return fmt.Errorf("search record kind %q is unsupported", input.Kind)
	}
	if err := domain.ValidateCatalogID(input.CatalogKey); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalIdentity("search catalog title", input.CatalogTitle, false); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalPublicPath("search page href", input.PageHref, false); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalPublicPath("search fragment href", input.FragmentHref, true); err != nil {
		return err
	}

	if input.Kind == KindCatalog {
		if input.SpecKey != "" || input.SpecTitle != "" {
			return fmt.Errorf("catalog search record must not contain spec metadata")
		}
		if input.LogicalKey != input.CatalogKey {
			return fmt.Errorf("catalog search logical key must equal catalog key")
		}
		if input.Title != input.CatalogTitle {
			return fmt.Errorf("catalog search title must equal catalog title")
		}
		if input.FragmentHref != "" {
			return fmt.Errorf("catalog search record must not contain a fragment href")
		}
		if input.Description != "" || hasOperationFields(input.OperationID, input.Method, input.Path) {
			return fmt.Errorf("catalog search record contains unsupported resource fields")
		}
		return nil
	}

	_, err := NewSemanticRecord(SemanticRecordInput{
		Kind: input.Kind, SpecKey: input.SpecKey, SpecTitle: input.SpecTitle,
		LogicalKey: input.LogicalKey, Title: input.Title, Description: input.Description,
		OperationID: input.OperationID, Method: input.Method, Path: input.Path,
	})
	if err != nil {
		return err
	}
	if input.Kind == KindSpec && input.FragmentHref != "" {
		return fmt.Errorf("spec search record must not contain a fragment href")
	}
	if input.Kind != KindSpec && input.FragmentHref == "" {
		return fmt.Errorf("%s search record fragment href is required", input.Kind)
	}
	return nil
}

func validateSemanticRecordInput(input SemanticRecordInput) error {
	if !input.Kind.semantic() {
		return fmt.Errorf("semantic search record kind %q is unsupported", input.Kind)
	}
	if err := domain.ValidateCatalogDocumentKey(input.SpecKey); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalIdentity("search spec title", input.SpecTitle, false); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalIdentity("search logical key", input.LogicalKey, false); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalIdentity("search title", input.Title, false); err != nil {
		return err
	}
	if err := domain.ValidateCanonicalIdentity("search description", input.Description, true); err != nil {
		return err
	}

	switch input.Kind {
	case KindSpec:
		if input.LogicalKey != input.SpecKey {
			return fmt.Errorf("spec search logical key must equal spec key")
		}
		if input.Title != input.SpecTitle {
			return fmt.Errorf("spec search title must equal spec title")
		}
		if input.Description != "" || hasOperationFields(input.OperationID, input.Method, input.Path) {
			return fmt.Errorf("spec search record contains unsupported resource fields")
		}
	case KindPath:
		if input.OperationID != "" || input.Method != "" {
			return fmt.Errorf("path search record must not contain operation fields")
		}
		if input.Path != input.LogicalKey {
			return fmt.Errorf("path search logical key must equal path")
		}
		if input.Title != input.Path {
			return fmt.Errorf("path search title must equal path")
		}
		return validateAPIPath(input.Path)
	case KindSchema:
		if hasOperationFields(input.OperationID, input.Method, input.Path) {
			return fmt.Errorf("schema search record must not contain operation fields")
		}
		if err := validateSchemaPointer(input.LogicalKey); err != nil {
			return err
		}
	case KindOperation:
		if err := validateMethod(input.Method); err != nil {
			return err
		}
		if input.Method+" "+input.Path != input.LogicalKey {
			return fmt.Errorf("operation search logical key must equal uppercase method plus path")
		}
		if err := validateAPIPath(input.Path); err != nil {
			return err
		}
		if err := domain.ValidateCanonicalIdentity("search operation id", input.OperationID, true); err != nil {
			return err
		}
	}
	return nil
}

func validateMethod(method string) error {
	if err := domain.ValidateCanonicalIdentity("search operation method", method, false); err != nil {
		return err
	}
	for _, character := range method {
		if character < 'A' || character > 'Z' {
			return fmt.Errorf("search operation method is invalid")
		}
	}
	return nil
}

func validateAPIPath(value string) error {
	if err := domain.ValidateCanonicalIdentity("search API path", value, false); err != nil {
		return err
	}
	cleanInput := value
	if value != "/" && strings.HasSuffix(value, "/") {
		cleanInput = strings.TrimSuffix(value, "/")
	}
	if !strings.HasPrefix(value, "/") || path.Clean(cleanInput) != cleanInput || strings.ContainsAny(value, " ?#") {
		return fmt.Errorf("search API path is invalid")
	}
	return nil
}

func validateSchemaPointer(value string) error {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return fmt.Errorf("search schema logical key must be a canonical component JSON pointer")
	}
	name := strings.TrimPrefix(value, prefix)
	for index := 0; index < len(name); index++ {
		switch name[index] {
		case '/':
			return fmt.Errorf("search schema logical key must address one component")
		case '~':
			if index+1 >= len(name) || (name[index+1] != '0' && name[index+1] != '1') {
				return fmt.Errorf("search schema logical key has an invalid JSON pointer escape")
			}
			index++
		}
	}
	return nil
}

func hasOperationFields(operationID, method, apiPath string) bool {
	return operationID != "" || method != "" || apiPath != ""
}

func (kind RecordKind) valid() bool {
	return kind == KindCatalog || kind.semantic()
}

func (kind RecordKind) semantic() bool {
	switch kind {
	case KindSpec, KindPath, KindOperation, KindSchema:
		return true
	default:
		return false
	}
}

func framedStrings(values ...string) []byte {
	length := 0
	for _, value := range values {
		length += 4 + len(value)
	}
	result := make([]byte, 0, length)
	for _, value := range values {
		result = binary.BigEndian.AppendUint32(result, uint32(len(value)))
		result = append(result, value...)
	}
	return result
}
