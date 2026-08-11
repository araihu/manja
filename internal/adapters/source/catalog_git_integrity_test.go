package source

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testGitSourceReceipt struct {
	SchemaVersion   uint32                    `json:"schemaVersion"`
	CatalogID       string                    `json:"catalogId"`
	CloneRepository string                    `json:"cloneRepository"`
	ProvenanceURL   string                    `json:"provenanceUrl"`
	ObjectFormat    string                    `json:"objectFormat"`
	SourceRoot      string                    `json:"sourceRoot"`
	CommitObjectID  string                    `json:"commitObjectId"`
	TreeObjectID    string                    `json:"treeObjectId"`
	Artifacts       []testGitArtifactEvidence `json:"artifacts"`
	License         testGitLicenseEvidence    `json:"license"`
}

type testGitArtifactEvidence struct {
	Path        string `json:"path"`
	Mode        string `json:"mode"`
	Size        int64  `json:"size"`
	GitObjectID string `json:"gitObjectId"`
	SHA256      string `json:"sha256"`
}

type testGitLicenseEvidence struct {
	Name           string `json:"name"`
	SPDX           string `json:"spdx"`
	UpstreamPath   string `json:"upstreamPath"`
	TrackedLocally bool   `json:"trackedLocally"`
	Size           int64  `json:"size"`
	GitBlobSHA     string `json:"gitBlobSha"`
	SHA256         string `json:"sha256"`
}

func TestLoadGitSourceProvenanceReceiptRejectsInvalidExpectations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		check  string
		mutate func(*testGitSourceReceipt)
	}{
		{name: "schema version", check: "receipt-schema", mutate: func(got *testGitSourceReceipt) { got.SchemaVersion = 1 }},
		{name: "catalog ID", check: "catalog", mutate: func(got *testGitSourceReceipt) { got.CatalogID = "Payments" }},
		{name: "clone repository", check: "repository", mutate: func(got *testGitSourceReceipt) { got.CloneRepository = "" }},
		{name: "provenance URL", check: "provenance-url", mutate: func(got *testGitSourceReceipt) { got.ProvenanceURL = "" }},
		{name: "object format", check: "object-format", mutate: func(got *testGitSourceReceipt) { got.ObjectFormat = "sha512" }},
		{name: "source root absolute", check: "root", mutate: func(got *testGitSourceReceipt) { got.SourceRoot = "/specs" }},
		{name: "source root escape", check: "root", mutate: func(got *testGitSourceReceipt) { got.SourceRoot = "../specs" }},
		{name: "commit uppercase", check: "commit", mutate: func(got *testGitSourceReceipt) { got.CommitObjectID = strings.Repeat("A", 40) }},
		{name: "tree length", check: "tree", mutate: func(got *testGitSourceReceipt) { got.TreeObjectID = strings.Repeat("b", 39) }},
		{name: "missing artifacts", check: "coverage-missing", mutate: func(got *testGitSourceReceipt) { got.Artifacts = nil }},
		{name: "duplicate artifact", check: "path", mutate: func(got *testGitSourceReceipt) { got.Artifacts = append(got.Artifacts, got.Artifacts[0]) }},
		{name: "unsorted artifacts", check: "path", mutate: func(got *testGitSourceReceipt) {
			got.Artifacts = append([]testGitArtifactEvidence{{Path: "z.json", Mode: "100644", Size: 1, GitObjectID: strings.Repeat("1", 40), SHA256: strings.Repeat("2", 64)}}, got.Artifacts...)
		}},
		{name: "artifact absolute", check: "path", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Path = "/openapi.json" }},
		{name: "artifact escape", check: "path", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Path = "../openapi.json" }},
		{name: "artifact backslash", check: "path", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Path = `specs\openapi.json` }},
		{name: "artifact NUL", check: "path", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Path = "specs/\x00openapi.json" }},
		{name: "artifact mode", check: "mode", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Mode = "120000" }},
		{name: "artifact zero size", check: "size", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Size = 0 }},
		{name: "artifact oversized", check: "size", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].Size = maxCatalogSourceFileBytes + 1 }},
		{name: "artifact object uppercase", check: "git-object-id", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].GitObjectID = strings.Repeat("C", 40) }},
		{name: "artifact SHA-256 length", check: "sha256", mutate: func(got *testGitSourceReceipt) { got.Artifacts[0].SHA256 = strings.Repeat("d", 63) }},
		{name: "license path", check: "license-path", mutate: func(got *testGitSourceReceipt) { got.License.UpstreamPath = "../LICENSE" }},
		{name: "license size", check: "license-size", mutate: func(got *testGitSourceReceipt) { got.License.Size = 0 }},
		{name: "license object format", check: "license-git-object-id", mutate: func(got *testGitSourceReceipt) { got.License.GitBlobSHA = strings.Repeat("e", 64) }},
		{name: "license SHA-256", check: "license-sha256", mutate: func(got *testGitSourceReceipt) { got.License.SHA256 = strings.Repeat("F", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			receipt := validTestGitSourceReceipt("sha1")
			test.mutate(&receipt)
			filename := writeTestGitSourceReceipt(t, receipt)
			_, err := loadGitSourceProvenanceReceipt(filename)
			var integrityErr *CatalogIntegrityError
			if !errors.As(err, &integrityErr) || integrityErr.Check != test.check || !errors.Is(err, ErrCatalogIntegrity) {
				t.Fatalf("invalid receipt error = %#v, want integrity check %q", err, test.check)
			}
		})
	}
}

func validTestGitSourceReceipt(objectFormat string) testGitSourceReceipt {
	objectLength := 40
	if objectFormat == "sha256" {
		objectLength = 64
	}
	return testGitSourceReceipt{
		SchemaVersion:   2,
		CatalogID:       "payments",
		CloneRepository: "/repositories/payments.git",
		ProvenanceURL:   "https://example.test/payments",
		ObjectFormat:    objectFormat,
		SourceRoot:      ".",
		CommitObjectID:  strings.Repeat("a", objectLength),
		TreeObjectID:    strings.Repeat("b", objectLength),
		Artifacts: []testGitArtifactEvidence{{
			Path: "openapi.json", Mode: "100644", Size: 3,
			GitObjectID: strings.Repeat("c", objectLength), SHA256: strings.Repeat("d", 64),
		}},
		License: testGitLicenseEvidence{
			Name: "MIT", SPDX: "MIT", UpstreamPath: "LICENSE", Size: 3,
			GitBlobSHA: strings.Repeat("e", objectLength), SHA256: strings.Repeat("f", 64),
		},
	}
}

func writeTestGitSourceReceipt(t *testing.T, receipt testGitSourceReceipt) string {
	t.Helper()
	contents, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(filename, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}
