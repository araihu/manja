package source

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	License         gitLicenseEvidence    `json:"license"`
}

type gitArtifactEvidence struct {
	Path        string `json:"path"`
	Mode        string `json:"mode"`
	Size        int64  `json:"size"`
	GitObjectID string `json:"gitObjectId"`
	SHA256      string `json:"sha256"`
}

type gitLicenseEvidence struct {
	Name           string `json:"name"`
	SPDX           string `json:"spdx"`
	UpstreamPath   string `json:"upstreamPath"`
	TrackedLocally bool   `json:"trackedLocally"`
	Size           int64  `json:"size"`
	GitBlobSHA     string `json:"gitBlobSha"`
	SHA256         string `json:"sha256"`
}

func loadGitSourceProvenanceReceipt(filename string) (gitSourceProvenanceReceipt, error) {
	input, err := os.Open(filename)
	if err != nil {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-open", filename, err)
	}
	defer input.Close()

	contents, err := io.ReadAll(io.LimitReader(input, maxGitIntegrityReceiptBytes+1))
	if err != nil {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-read", filename, err)
	}
	if len(contents) > maxGitIntegrityReceiptBytes {
		return gitSourceProvenanceReceipt{}, catalogIntegrityError("receipt-size", filename, fmt.Errorf("Git integrity receipt exceeds %d bytes", maxGitIntegrityReceiptBytes))
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
	if err := validateSourcePath("Git integrity license path", receipt.License.UpstreamPath); err != nil {
		return catalogIntegrityError("license-path", receipt.License.UpstreamPath, err)
	}
	if receipt.License.Size <= 0 || receipt.License.Size > maxCatalogSourceFileBytes {
		return catalogIntegrityError("license-size", receipt.License.UpstreamPath, fmt.Errorf("size %d is outside 1..%d", receipt.License.Size, maxCatalogSourceFileBytes))
	}
	if !isLowerHexLength(receipt.License.GitBlobSHA, objectLength) {
		return catalogIntegrityError("license-git-object-id", receipt.License.UpstreamPath, fmt.Errorf("Git object ID must be %d lowercase hexadecimal characters", objectLength))
	}
	if !isLowerHexLength(receipt.License.SHA256, 64) {
		return catalogIntegrityError("license-sha256", receipt.License.UpstreamPath, fmt.Errorf("raw-byte SHA-256 must be 64 lowercase hexadecimal characters"))
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

func catalogIntegrityError(check, path string, err error) error {
	return &CatalogIntegrityError{Check: check, Path: path, Err: err}
}
