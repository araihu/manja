package searchv2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/araihu/manja/domain"
)

// BuildIdentity contains every version boundary that makes an otherwise
// unchanged per-spec semantic run unsafe to reuse.
type BuildIdentity struct {
	RunFormat            string `json:"runFormat"`
	Analyzer             string `json:"analyzer"`
	Producer             string `json:"producer"`
	SpecProjectionSHA256 string `json:"specProjectionSha256"`
}

// NewBuildIdentity binds a run to the producer identity and upstream canonical
// spec projection. Producer should be a release or commit identity.
func NewBuildIdentity(producer, projectionSHA256 string) (BuildIdentity, error) {
	identity := BuildIdentity{
		RunFormat: RunFormatVersion, Analyzer: AnalyzerVersion,
		Producer: producer, SpecProjectionSHA256: projectionSHA256,
	}
	if err := identity.validate(); err != nil {
		return BuildIdentity{}, err
	}
	return identity, nil
}

// SemanticRun is an immutable, canonically ordered cache unit emitted once per
// spec. It is intentionally independent of catalog membership and public URLs;
// topology expansion adds those lightweight occurrence details afterward.
type SemanticRun struct {
	SchemaVersion uint32           `json:"schemaVersion"`
	Identity      BuildIdentity    `json:"identity"`
	SpecKey       string           `json:"specKey"`
	InputSHA256   string           `json:"inputSha256"`
	BuildKey      string           `json:"buildKey"`
	Records       []SemanticRecord `json:"records"`
}

// NewSemanticRun validates and copies records, sorts them canonically, and
// derives an input digest plus cache build key. Catalog assignment is absent
// from both the run bytes and build key, so topology-only changes reuse it.
func NewSemanticRun(specKey string, identity BuildIdentity, records []SemanticRecord) (SemanticRun, error) {
	if err := domain.ValidateCatalogDocumentKey(specKey); err != nil {
		return SemanticRun{}, err
	}
	if err := identity.validate(); err != nil {
		return SemanticRun{}, err
	}

	canonical := append([]SemanticRecord(nil), records...)
	var specTitle string
	specRecordCount := 0
	for index := range canonical {
		if err := validateCanonicalSemanticRecord(canonical[index]); err != nil {
			return SemanticRun{}, fmt.Errorf("semantic search run record %d: %w", index, err)
		}
		if canonical[index].SpecKey != specKey {
			return SemanticRun{}, fmt.Errorf("semantic search run record %d is outside spec scope", index)
		}
		if specTitle == "" {
			specTitle = canonical[index].SpecTitle
		} else if canonical[index].SpecTitle != specTitle {
			return SemanticRun{}, fmt.Errorf("semantic search run record %d has inconsistent spec title", index)
		}
		if canonical[index].Kind == KindSpec {
			specRecordCount++
		}
	}
	if specRecordCount != 1 {
		return SemanticRun{}, fmt.Errorf("semantic search run must contain exactly one spec navigation record")
	}
	sortSemanticRecords(canonical)
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].ID == canonical[index].ID {
			return SemanticRun{}, fmt.Errorf("semantic search record id %q is duplicated", canonical[index].ID)
		}
	}

	inputBytes, err := json.Marshal(canonical)
	if err != nil {
		return SemanticRun{}, fmt.Errorf("encode canonical semantic search inputs: %w", err)
	}
	inputDigest := sha256.Sum256(inputBytes)
	inputSHA256 := hex.EncodeToString(inputDigest[:])
	buildDigest := sha256.Sum256(framedStrings(
		identity.RunFormat, identity.Analyzer, identity.Producer,
		identity.SpecProjectionSHA256, specKey, inputSHA256,
	))
	return SemanticRun{
		SchemaVersion: SchemaVersion, Identity: identity, SpecKey: specKey,
		InputSHA256: inputSHA256, BuildKey: hex.EncodeToString(buildDigest[:]),
		Records: canonical,
	}, nil
}

// MarshalCanonical returns the deterministic JSON used for persistence.
func (run SemanticRun) MarshalCanonical() ([]byte, error) {
	if err := run.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(run)
}

// SHA256 returns the digest of the exact canonical semantic-run bytes.
func (run SemanticRun) SHA256() (string, error) {
	encoded, err := run.MarshalCanonical()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (identity BuildIdentity) validate() error {
	if identity.RunFormat != RunFormatVersion {
		return fmt.Errorf("search run format %q is unsupported", identity.RunFormat)
	}
	if identity.Analyzer != AnalyzerVersion {
		return fmt.Errorf("search analyzer %q is unsupported", identity.Analyzer)
	}
	if err := domain.ValidateCanonicalIdentity("search producer identity", identity.Producer, false); err != nil {
		return err
	}
	if !isLowerSHA256(identity.SpecProjectionSHA256) {
		return fmt.Errorf("search projection digest must be lowercase SHA-256")
	}
	return nil
}

func (run SemanticRun) validate() error {
	if run.SchemaVersion != SchemaVersion {
		return fmt.Errorf("semantic search run schema version %d is unsupported", run.SchemaVersion)
	}
	rebuilt, err := NewSemanticRun(run.SpecKey, run.Identity, run.Records)
	if err != nil {
		return err
	}
	if run.InputSHA256 != rebuilt.InputSHA256 || run.BuildKey != rebuilt.BuildKey {
		return fmt.Errorf("semantic search run digest metadata does not match canonical inputs")
	}
	for index := range run.Records {
		if run.Records[index].ID != rebuilt.Records[index].ID {
			return fmt.Errorf("semantic search run records are not canonically ordered")
		}
	}
	return nil
}

func validateCanonicalSemanticRecord(record SemanticRecord) error {
	rebuilt, err := NewSemanticRecord(SemanticRecordInput{
		Kind: record.Kind, SpecKey: record.SpecKey, SpecTitle: record.SpecTitle,
		LogicalKey: record.LogicalKey, Title: record.Title, Description: record.Description,
		OperationID: record.OperationID, Method: record.Method, Path: record.Path,
		RequestTarget: record.RequestTarget, FixedQuery: record.FixedQuery,
	})
	if err != nil {
		return err
	}
	if record.ID != rebuilt.ID {
		return fmt.Errorf("semantic search record id %q does not match logical identity", record.ID)
	}
	if record.Method != rebuilt.Method {
		return fmt.Errorf("semantic search operation method is not canonical")
	}
	return nil
}

func validateCanonicalRecord(record Record) error {
	rebuilt, err := NewRecord(RecordInput{
		Kind:       record.Kind,
		CatalogKey: record.CatalogKey, CatalogTitle: record.CatalogTitle,
		SpecKey: record.SpecKey, SpecTitle: record.SpecTitle,
		LogicalKey: record.LogicalKey, Title: record.Title, Description: record.Description,
		OperationID: record.OperationID, Method: record.Method, Path: record.Path,
		RequestTarget: record.RequestTarget, FixedQuery: record.FixedQuery,
		PageHref: record.PageHref, FragmentHref: record.FragmentHref,
	})
	if err != nil {
		return err
	}
	if record.ID != rebuilt.ID {
		return fmt.Errorf("search record id %q does not match logical identity", record.ID)
	}
	if record.Method != rebuilt.Method {
		return fmt.Errorf("search operation method is not canonical")
	}
	return nil
}

func sortSemanticRecords(records []SemanticRecord) {
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if kindOrder(left.Kind) != kindOrder(right.Kind) {
			return kindOrder(left.Kind) < kindOrder(right.Kind)
		}
		if left.LogicalKey != right.LogicalKey {
			return left.LogicalKey < right.LogicalKey
		}
		return left.ID < right.ID
	})
}

func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if left.CatalogKey != right.CatalogKey {
			return left.CatalogKey < right.CatalogKey
		}
		if left.SpecKey != right.SpecKey {
			return left.SpecKey < right.SpecKey
		}
		if kindOrder(left.Kind) != kindOrder(right.Kind) {
			return kindOrder(left.Kind) < kindOrder(right.Kind)
		}
		if left.LogicalKey != right.LogicalKey {
			return left.LogicalKey < right.LogicalKey
		}
		return left.ID < right.ID
	})
}

func kindOrder(kind RecordKind) uint8 {
	switch kind {
	case KindCatalog:
		return 0
	case KindSpec:
		return 1
	case KindPath:
		return 2
	case KindOperation:
		return 3
	case KindSchema:
		return 4
	default:
		return 255
	}
}

func isLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
