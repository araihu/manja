package source

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- Git SHA-1 object IDs are required provenance identifiers, not security digests.
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/araihu/manja/domain"
)

const maxGitIntegrityReceiptBytes = 64 << 10

var ErrCatalogIntegrity = errors.New("Git catalog integrity")

type CatalogIntegrityError struct {
	Check string
	Path  string
	Err   error
}

func (err *CatalogIntegrityError) Error() string {
	if err.Path == "" {
		return fmt.Sprintf("Git catalog integrity %s: %v", err.Check, err.Err)
	}
	return fmt.Sprintf("Git catalog integrity %s for %q: %v", err.Check, err.Path, err.Err)
}

func (err *CatalogIntegrityError) Unwrap() error {
	return ErrCatalogIntegrity
}

type gitObjectFormat string

const (
	gitObjectFormatSHA1   gitObjectFormat = "sha1"
	gitObjectFormatSHA256 gitObjectFormat = "sha256"
)

type gitSourceProvenanceReceipt struct {
	SchemaVersion   uint32                `json:"schemaVersion"`
	CatalogID       string                `json:"catalogId"`
	CloneRepository string                `json:"cloneRepository"`
	ProvenanceURL   string                `json:"provenanceUrl"`
	ObjectFormat    gitObjectFormat       `json:"objectFormat"`
	SourceRoot      string                `json:"sourceRoot"`
	CommitObjectID  string                `json:"commitObjectId"`
	TreeObjectID    string                `json:"treeObjectId"`
	Artifacts       []gitArtifactEvidence `json:"artifacts"`
}

type gitArtifactEvidence struct {
	Path        string `json:"path"`
	Mode        string `json:"mode"`
	Size        int64  `json:"size"`
	GitObjectID string `json:"gitObjectId"`
	SHA256      string `json:"sha256"`
}

type gitCatalogIntegrity struct {
	receipt     gitSourceProvenanceReceipt
	expectation map[string]gitArtifactEvidence
	used        map[string]struct{}
}

func loadGitSourceProvenanceReceipt(rootDirectory, filename string) (gitSourceProvenanceReceipt, error) {
	input, err := openGitSourceProvenanceReceipt(rootDirectory, filename)
	if err != nil {
		return gitSourceProvenanceReceipt{}, err
	}
	defer input.Close()

	contents, err := io.ReadAll(io.LimitReader(input, maxGitIntegrityReceiptBytes+1))
	if err != nil {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-read", filename, err)
	}
	if len(contents) > maxGitIntegrityReceiptBytes {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-size", filename, fmt.Errorf("Git integrity receipt exceeds %d bytes", maxGitIntegrityReceiptBytes))
	}
	if err := validateGitIntegrityReceiptJSON(contents); err != nil {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-schema", filename, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var receipt gitSourceProvenanceReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-schema", filename, fmt.Errorf("decode Git integrity receipt: %w", err))
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-schema", filename, fmt.Errorf("Git integrity receipt must contain exactly one JSON value"))
		}
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-schema", filename, fmt.Errorf("decode Git integrity receipt: %w", err))
	}
	if err := validateGitSourceProvenanceReceipt(receipt); err != nil {
		return gitSourceProvenanceReceipt{}, err
	}
	return receipt, nil
}

func validateGitIntegrityReceiptJSON(contents []byte) error {
	artifactShape := &gitIntegrityReceiptJSONShape{objectFields: map[string]*gitIntegrityReceiptJSONShape{
		"path":        nil,
		"mode":        nil,
		"size":        nil,
		"gitObjectId": nil,
		"sha256":      nil,
	}}
	receiptShape := &gitIntegrityReceiptJSONShape{objectFields: map[string]*gitIntegrityReceiptJSONShape{
		"schemaVersion":   nil,
		"catalogId":       nil,
		"cloneRepository": nil,
		"provenanceUrl":   nil,
		"objectFormat":    nil,
		"sourceRoot":      nil,
		"commitObjectId":  nil,
		"treeObjectId":    nil,
		"artifacts":       {arrayItem: artifactShape},
	}}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := validateGitIntegrityReceiptJSONValue(decoder, receiptShape); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("Git integrity receipt must contain exactly one JSON value")
		}
		return fmt.Errorf("decode Git integrity receipt: %w", err)
	}
	return nil
}

type gitIntegrityReceiptJSONShape struct {
	objectFields map[string]*gitIntegrityReceiptJSONShape
	arrayItem    *gitIntegrityReceiptJSONShape
}

func validateGitIntegrityReceiptJSONValue(decoder *json.Decoder, shape *gitIntegrityReceiptJSONShape) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode Git integrity receipt: %w", err)
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
				return fmt.Errorf("decode Git integrity receipt object key: %w", err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("Git integrity receipt object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("Git integrity receipt contains duplicate key %q", key)
			}
			seen[key] = struct{}{}
			var childShape *gitIntegrityReceiptJSONShape
			if shape != nil && shape.objectFields != nil {
				var known bool
				childShape, known = shape.objectFields[key]
				if !known {
					return fmt.Errorf("decode Git integrity receipt: json: unknown field %q", key)
				}
			}
			if err := validateGitIntegrityReceiptJSONValue(decoder, childShape); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("decode Git integrity receipt object")
		}
	case '[':
		var itemShape *gitIntegrityReceiptJSONShape
		if shape != nil {
			itemShape = shape.arrayItem
		}
		for decoder.More() {
			if err := validateGitIntegrityReceiptJSONValue(decoder, itemShape); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("decode Git integrity receipt array")
		}
	default:
		return fmt.Errorf("decode Git integrity receipt delimiter %q", delimiter)
	}
	return nil
}

func openGitSourceProvenanceReceipt(rootDirectory, filename string) (*os.File, error) {
	if !filepath.IsAbs(rootDirectory) {
		return nil, catalogIntegrityError("receipt-root", rootDirectory, fmt.Errorf("receipt root must be absolute"))
	}
	if err := validateGitIntegrityReceiptPath(filename); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootDirectory)
	if err != nil {
		return nil, catalogIntegrityError("receipt-root", rootDirectory, err)
	}
	components := strings.Split(filename, "/")
	for _, component := range components[:len(components)-1] {
		info, statErr := root.Lstat(component)
		if statErr != nil {
			root.Close()
			return nil, catalogIntegrityError("receipt-file", filename, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			root.Close()
			return nil, catalogIntegrityError("receipt-file", filename, fmt.Errorf("receipt parent %q must be a non-symlink directory", component))
		}
		next, openErr := root.OpenRoot(component)
		if openErr != nil {
			root.Close()
			return nil, catalogIntegrityError("receipt-file", filename, openErr)
		}
		openedInfo, openedStatErr := next.Stat(".")
		if openedStatErr != nil || !os.SameFile(info, openedInfo) {
			next.Close()
			root.Close()
			if openedStatErr != nil {
				return nil, catalogIntegrityError("receipt-file", filename, openedStatErr)
			}
			return nil, catalogIntegrityError("receipt-file", filename, fmt.Errorf("receipt parent %q changed during open", component))
		}
		root.Close()
		root = next
	}
	defer root.Close()

	leaf := components[len(components)-1]
	leafInfo, err := root.Lstat(leaf)
	if err != nil {
		return nil, catalogIntegrityError("receipt-file", filename, err)
	}
	if leafInfo.Mode()&os.ModeSymlink != 0 || !leafInfo.Mode().IsRegular() {
		return nil, catalogIntegrityError("receipt-file", filename, fmt.Errorf("receipt leaf must be a non-symlink regular file"))
	}
	input, err := root.Open(leaf)
	if err != nil {
		return nil, catalogIntegrityError("receipt-file", filename, err)
	}
	openedInfo, err := input.Stat()
	if err != nil {
		input.Close()
		return nil, catalogIntegrityError("receipt-file", filename, err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(leafInfo, openedInfo) {
		input.Close()
		return nil, catalogIntegrityError("receipt-file", filename, fmt.Errorf("receipt leaf changed during open"))
	}
	return input, nil
}

func validateGitIntegrityReceiptPath(filename string) error {
	if filename == "" || strings.TrimSpace(filename) != filename || strings.HasPrefix(filename, "/") || strings.Contains(filename, `\`) || strings.ContainsRune(filename, 0) || hasGitWindowsDrivePrefix(filename) || filename == "." || path.Clean(filename) != filename || strings.HasPrefix(filename, "../") {
		return catalogIntegrityError("receipt-path", filename, fmt.Errorf("receipt path must be a canonical relative slash path"))
	}
	return nil
}

func hasGitWindowsDrivePrefix(filename string) bool {
	if len(filename) < 2 || filename[1] != ':' {
		return false
	}
	return filename[0] >= 'a' && filename[0] <= 'z' || filename[0] >= 'A' && filename[0] <= 'Z'
}

func validateGitSourceProvenanceReceipt(receipt gitSourceProvenanceReceipt) error {
	if receipt.SchemaVersion != 2 {
		return catalogIntegrityError("receipt-schema", "", fmt.Errorf("schema version = %d, want 2", receipt.SchemaVersion))
	}
	if err := domain.ValidateCatalogID(receipt.CatalogID); err != nil {
		return catalogIntegrityError("catalog", "", err)
	}
	if receipt.CloneRepository == "" || strings.TrimSpace(receipt.CloneRepository) != receipt.CloneRepository {
		return catalogIntegrityError("repository", "", fmt.Errorf("clone repository is required without surrounding whitespace"))
	}
	if receipt.ProvenanceURL == "" || strings.TrimSpace(receipt.ProvenanceURL) != receipt.ProvenanceURL {
		return catalogIntegrityError("provenance-url", "", fmt.Errorf("provenance URL is required without surrounding whitespace"))
	}
	objectLength := 0
	switch receipt.ObjectFormat {
	case gitObjectFormatSHA1:
		objectLength = 40
	case gitObjectFormatSHA256:
		objectLength = 64
	default:
		return catalogIntegrityError("object-format", "", fmt.Errorf("object format %q is unsupported", receipt.ObjectFormat))
	}
	if receipt.SourceRoot != "." {
		if err := validateSourcePath("Git integrity source root", receipt.SourceRoot); err != nil {
			return catalogIntegrityError("root", receipt.SourceRoot, err)
		}
	}
	if !isLowerHexLength(receipt.CommitObjectID, objectLength) {
		return catalogIntegrityError("commit", "", fmt.Errorf("commit object ID must be %d lowercase hexadecimal characters", objectLength))
	}
	if !isLowerHexLength(receipt.TreeObjectID, objectLength) {
		return catalogIntegrityError("tree", "", fmt.Errorf("tree object ID must be %d lowercase hexadecimal characters", objectLength))
	}
	if len(receipt.Artifacts) == 0 {
		return catalogIntegrityError("coverage-missing", "", fmt.Errorf("at least one artifact is required"))
	}
	if len(receipt.Artifacts) > maxCatalogInventoryEntries {
		return catalogIntegrityError("coverage-unused", "", fmt.Errorf("artifact count exceeds %d", maxCatalogInventoryEntries))
	}
	previousPath := ""
	for _, artifact := range receipt.Artifacts {
		if err := validateSourcePath("Git integrity artifact path", artifact.Path); err != nil {
			return catalogIntegrityError("path", artifact.Path, err)
		}
		if previousPath != "" && artifact.Path <= previousPath {
			return catalogIntegrityError("path", artifact.Path, fmt.Errorf("artifact paths must be strictly increasing and unique"))
		}
		previousPath = artifact.Path
		if artifact.Mode != "100644" && artifact.Mode != "100755" {
			return catalogIntegrityError("mode", artifact.Path, fmt.Errorf("mode %q is not a regular Git blob mode", artifact.Mode))
		}
		if artifact.Size <= 0 || artifact.Size > maxCatalogSourceFileBytes {
			return catalogIntegrityError("size", artifact.Path, fmt.Errorf("size %d is outside 1..%d", artifact.Size, maxCatalogSourceFileBytes))
		}
		if !isLowerHexLength(artifact.GitObjectID, objectLength) {
			return catalogIntegrityError("git-object-id", artifact.Path, fmt.Errorf("Git object ID must be %d lowercase hexadecimal characters", objectLength))
		}
		if !isLowerHexLength(artifact.SHA256, 64) {
			return catalogIntegrityError("sha256", artifact.Path, fmt.Errorf("raw-byte SHA-256 must be 64 lowercase hexadecimal characters"))
		}
	}
	return nil
}

func isLowerHexLength(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func newGitCatalogIntegrity(receipt gitSourceProvenanceReceipt, source GitCatalogSource, root, reference string) (*gitCatalogIntegrity, error) {
	if receipt.CloneRepository != source.Repository {
		return nil, catalogIntegrityError("repository", "", fmt.Errorf("clone repository = %q, want exact configured repository %q", receipt.CloneRepository, source.Repository))
	}
	if receipt.CatalogID != source.Manifest.ID {
		return nil, catalogIntegrityError("catalog", "", fmt.Errorf("catalog ID = %q, want %q", receipt.CatalogID, source.Manifest.ID))
	}
	if receipt.SourceRoot != root {
		return nil, catalogIntegrityError("root", "", fmt.Errorf("source root = %q, want %q", receipt.SourceRoot, root))
	}
	if receipt.CommitObjectID != reference {
		return nil, catalogIntegrityError("ref", "", fmt.Errorf("configured ref = %q, want exact receipt commit %q", reference, receipt.CommitObjectID))
	}
	expectation := make(map[string]gitArtifactEvidence, len(receipt.Artifacts))
	for _, artifact := range receipt.Artifacts {
		expectation[artifact.Path] = artifact
	}
	return &gitCatalogIntegrity{receipt: receipt, expectation: expectation, used: make(map[string]struct{}, len(expectation))}, nil
}

func (integrity *gitCatalogIntegrity) verifyObjectFormat(objectFormat string) error {
	if objectFormat != string(integrity.receipt.ObjectFormat) {
		return catalogIntegrityError("object-format", "", fmt.Errorf("repository object format = %q, want %q", objectFormat, integrity.receipt.ObjectFormat))
	}
	return nil
}

func (integrity *gitCatalogIntegrity) verifyRepository(commit, tree string) error {
	if commit != integrity.receipt.CommitObjectID {
		return catalogIntegrityError("commit", "", fmt.Errorf("resolved commit = %q, want %q", commit, integrity.receipt.CommitObjectID))
	}
	if tree != integrity.receipt.TreeObjectID {
		return catalogIntegrityError("tree", "", fmt.Errorf("resolved tree = %q, want %q", tree, integrity.receipt.TreeObjectID))
	}
	return nil
}

func (integrity *gitCatalogIntegrity) verifyMetadata(entry catalogInventoryEntry, size int64) (gitArtifactEvidence, error) {
	expected, exists := integrity.expectation[entry.path]
	if !exists {
		return gitArtifactEvidence{}, catalogIntegrityError("coverage-missing", entry.path, fmt.Errorf("captured path has no receipt artifact"))
	}
	if entry.mode != expected.Mode {
		return gitArtifactEvidence{}, catalogIntegrityError("mode", entry.path, fmt.Errorf("inventory mode = %q, want %q", entry.mode, expected.Mode))
	}
	if entry.objectID != expected.GitObjectID {
		return gitArtifactEvidence{}, catalogIntegrityError("git-object-id", entry.path, fmt.Errorf("inventory object ID = %q, want %q", entry.objectID, expected.GitObjectID))
	}
	if size >= 0 && size != expected.Size {
		return gitArtifactEvidence{}, catalogIntegrityError("size", entry.path, fmt.Errorf("object size = %d, want %d", size, expected.Size))
	}
	return expected, nil
}

func (integrity *gitCatalogIntegrity) verifyBytes(entry catalogInventoryEntry, data []byte) error {
	expected, err := integrity.verifyMetadata(entry, int64(len(data)))
	if err != nil {
		return err
	}
	objectID := gitBlobObjectID(integrity.receipt.ObjectFormat, data)
	if objectID != entry.objectID || objectID != expected.GitObjectID {
		return catalogIntegrityError("git-object-id", entry.path, fmt.Errorf("recomputed object ID = %q, inventory = %q, receipt = %q", objectID, entry.objectID, expected.GitObjectID))
	}
	rawDigest := fmt.Sprintf("%x", sha256.Sum256(data))
	if rawDigest != expected.SHA256 {
		return catalogIntegrityError("sha256", entry.path, fmt.Errorf("raw-byte SHA-256 = %q, want %q", rawDigest, expected.SHA256))
	}
	integrity.used[entry.path] = struct{}{}
	return nil
}

func (integrity *gitCatalogIntegrity) verifyComplete() error {
	for _, artifact := range integrity.receipt.Artifacts {
		if _, used := integrity.used[artifact.Path]; !used {
			return catalogIntegrityError("coverage-unused", artifact.Path, fmt.Errorf("receipt artifact was not captured"))
		}
	}
	return nil
}

func gitBlobObjectID(objectFormat gitObjectFormat, data []byte) string {
	header := []byte("blob " + strconv.Itoa(len(data)) + "\x00")
	if objectFormat == gitObjectFormatSHA1 {
		digest := sha1.New() // #nosec G401 -- Git SHA-1 object IDs are required provenance identifiers, not security digests.
		_, _ = digest.Write(header)
		_, _ = digest.Write(data)
		return fmt.Sprintf("%x", digest.Sum(nil))
	}
	digest := sha256.New()
	_, _ = digest.Write(header)
	_, _ = digest.Write(data)
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func catalogIntegrityError(check, path string, err error) error {
	return &CatalogIntegrityError{Check: check, Path: path, Err: err}
}
