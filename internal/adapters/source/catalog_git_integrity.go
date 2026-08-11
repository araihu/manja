package source

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	return receipt, nil
}

func catalogIntegrityError(check, path string, err error) error {
	return &CatalogIntegrityError{Check: check, Path: path, Err: err}
}
