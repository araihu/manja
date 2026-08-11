package source

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func TestCatalogGitIntegrityAdmitsExactSHA1AndSHA256Fixtures(t *testing.T) {
	for _, objectFormat := range []string{"sha1", "sha256"} {
		t.Run(objectFormat, func(t *testing.T) {
			fixture := newTestGitIntegrityFixture(t, objectFormat)
			candidate, err := fixture.source.Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(candidate.Documents) != 1 || string(candidate.Documents[0].Bytes) != string(fixture.documentBytes) {
				t.Fatalf("candidate documents = %#v", candidate.Documents)
			}
		})
	}
}

func TestCatalogGitIntegrityAdmitsRemoteSHA1AndSHA256Fixtures(t *testing.T) {
	for _, objectFormat := range []string{"sha1", "sha256"} {
		t.Run(objectFormat, func(t *testing.T) {
			fixture := newTestGitIntegrityFixture(t, objectFormat)
			remote := (&url.URL{Scheme: "file", Path: fixture.repository}).String()
			receipt := cloneTestGitSourceReceipt(fixture.receipt)
			receipt.CloneRepository = remote
			fixture.source.Repository = remote
			fixture.source.IntegrityReceiptPath = writeTestGitSourceReceipt(t, receipt)
			candidate, err := fixture.source.Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(candidate.Documents) != 1 || string(candidate.Documents[0].Bytes) != string(fixture.documentBytes) {
				t.Fatalf("candidate documents = %#v", candidate.Documents)
			}
		})
	}
}

func TestCatalogGitIntegrityRejectsRepositoryAndObjectDrift(t *testing.T) {
	tests := []struct {
		name   string
		check  string
		mutate func(*testGitSourceReceipt, *GitCatalogSource)
	}{
		{name: "repository exact string", check: "repository", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) { receipt.CloneRepository += "/." }},
		{name: "catalog reuse", check: "catalog", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) { receipt.CatalogID = "other" }},
		{name: "source root", check: "root", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) { receipt.SourceRoot = "specs" }},
		{name: "repository object format", check: "object-format", mutate: func(receipt *testGitSourceReceipt, source *GitCatalogSource) {
			receipt.ObjectFormat = "sha256"
			receipt.CommitObjectID = strings.Repeat("1", 64)
			receipt.TreeObjectID = strings.Repeat("2", 64)
			receipt.Artifacts[0].GitObjectID = strings.Repeat("3", 64)
			receipt.License.GitBlobSHA = strings.Repeat("4", 64)
			source.Ref = receipt.CommitObjectID
		}},
		{name: "configured ref", check: "ref", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) {
			receipt.CommitObjectID = strings.Repeat("1", len(receipt.CommitObjectID))
		}},
		{name: "commit tree", check: "tree", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) {
			receipt.TreeObjectID = strings.Repeat("2", len(receipt.TreeObjectID))
		}},
		{name: "artifact mode", check: "mode", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) { receipt.Artifacts[0].Mode = "100755" }},
		{name: "artifact size", check: "size", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) { receipt.Artifacts[0].Size++ }},
		{name: "artifact Git object", check: "git-object-id", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) {
			receipt.Artifacts[0].GitObjectID = strings.Repeat("3", len(receipt.Artifacts[0].GitObjectID))
		}},
		{name: "artifact raw SHA-256", check: "sha256", mutate: func(receipt *testGitSourceReceipt, _ *GitCatalogSource) {
			receipt.Artifacts[0].SHA256 = strings.Repeat("4", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTestGitIntegrityFixture(t, "sha1")
			receipt := cloneTestGitSourceReceipt(fixture.receipt)
			source := fixture.source
			test.mutate(&receipt, &source)
			source.IntegrityReceiptPath = writeTestGitSourceReceipt(t, receipt)
			_, err := source.Load(t.Context())
			assertCatalogIntegrityCheck(t, err, test.check)
		})
	}
}

func TestCatalogGitIntegrityRequiresExactCapturedCoverage(t *testing.T) {
	documentWithSupport := []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{},"components":{"schemas":{"Payment":{"$ref":"support.json#/Payment"}}}}`)
	supportBytes := []byte(`{"Payment":{"type":"object"}}`)

	t.Run("captured document and recursive support file", func(t *testing.T) {
		fixture := newTestGitIntegrityFixtureWithFiles(t, "sha1", documentWithSupport, map[string][]byte{
			"support.json": supportBytes,
		}, []string{"openapi.json", "support.json"})
		candidate, err := fixture.source.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(candidate.SupportFiles) != 1 || candidate.SupportFiles[0].SourcePath != "support.json" || string(candidate.SupportFiles[0].Bytes) != string(supportBytes) {
			t.Fatalf("support files = %#v", candidate.SupportFiles)
		}
	})

	t.Run("missing recursive support file evidence", func(t *testing.T) {
		fixture := newTestGitIntegrityFixtureWithFiles(t, "sha1", documentWithSupport, map[string][]byte{
			"support.json": supportBytes,
		}, []string{"openapi.json"})
		_, err := fixture.source.Load(t.Context())
		assertCatalogIntegrityCheck(t, err, "coverage-missing")
	})

	t.Run("unused repository file evidence", func(t *testing.T) {
		fixture := newTestGitIntegrityFixtureWithFiles(t, "sha1", nil, map[string][]byte{
			"unused.json": []byte(`{"unused":true}`),
		}, []string{"openapi.json", "unused.json"})
		_, err := fixture.source.Load(t.Context())
		assertCatalogIntegrityCheck(t, err, "coverage-unused")
	})
}

func TestCatalogGitIntegrityHashesTheSingleReturnedBlobRead(t *testing.T) {
	fixture := newTestGitIntegrityFixture(t, "sha1")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shimDirectory := t.TempDir()
	invocationLog := filepath.Join(t.TempDir(), "git.log")
	shim := filepath.Join(shimDirectory, "git")
	shimSource := `#!/bin/sh
printf '%s\n' "$*" >> "$MANJA_GIT_INVOCATIONS"
if [ "$1" = "-C" ] && [ "$3" = "cat-file" ] && [ "$4" = "blob" ] && [ "$5" = "$MANJA_SUBSTITUTE_OBJECT" ]; then
  printf '%s' "$MANJA_SUBSTITUTE_BYTES"
  exit 0
fi
exec "$MANJA_REAL_GIT" "$@"
`
	if err := os.WriteFile(shim, []byte(shimSource), 0o755); err != nil {
		t.Fatal(err)
	}
	substitute := strings.Replace(string(fixture.documentBytes), "Payments", "Tampered", 1)
	if len(substitute) != len(fixture.documentBytes) {
		t.Fatal("controlled substitution changed blob length")
	}
	t.Setenv("MANJA_GIT_INVOCATIONS", invocationLog)
	t.Setenv("MANJA_REAL_GIT", realGit)
	t.Setenv("MANJA_SUBSTITUTE_OBJECT", fixture.receipt.Artifacts[0].GitObjectID)
	t.Setenv("MANJA_SUBSTITUTE_BYTES", substitute)
	t.Setenv("PATH", shimDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err = fixture.source.Load(t.Context())
	assertCatalogIntegrityCheck(t, err, "git-object-id")
	logBytes, readErr := os.ReadFile(invocationLog)
	if readErr != nil {
		t.Fatal(readErr)
	}
	needle := "cat-file blob " + fixture.receipt.Artifacts[0].GitObjectID
	reads := 0
	for _, line := range strings.Split(string(logBytes), "\n") {
		if strings.Contains(line, needle) {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("blob reads = %d, want exactly 1; invocations:\n%s", reads, logBytes)
	}
}

func TestCatalogGitIntegrityPreservesOperationalErrorClassification(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		fixture := newTestGitIntegrityFixture(t, "sha1")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := fixture.source.Load(ctx)
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrCatalogIntegrity) {
			t.Fatalf("canceled load error = %#v", err)
		}
	})

	t.Run("Git process failure", func(t *testing.T) {
		fixture := newTestGitIntegrityFixture(t, "sha1")
		missing := filepath.Join(t.TempDir(), "missing.git")
		receipt := cloneTestGitSourceReceipt(fixture.receipt)
		receipt.CloneRepository = missing
		fixture.source.Repository = missing
		fixture.source.IntegrityReceiptPath = writeTestGitSourceReceipt(t, receipt)
		_, err := fixture.source.Load(t.Context())
		if err == nil || errors.Is(err, ErrCatalogIntegrity) {
			t.Fatalf("Git process error = %#v", err)
		}
	})
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

type testGitIntegrityFixture struct {
	repository    string
	receipt       testGitSourceReceipt
	source        GitCatalogSource
	documentBytes []byte
}

func newTestGitIntegrityFixture(t *testing.T, objectFormat string) testGitIntegrityFixture {
	t.Helper()
	documentBytes := []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{}}`)
	return newTestGitIntegrityFixtureWithFiles(t, objectFormat, documentBytes, nil, []string{"openapi.json"})
}

func newTestGitIntegrityFixtureWithFiles(t *testing.T, objectFormat string, documentBytes []byte, extraFiles map[string][]byte, artifactPaths []string) testGitIntegrityFixture {
	t.Helper()
	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q", "--object-format="+objectFormat)
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	if documentBytes == nil {
		documentBytes = []byte(`{"openapi":"3.0.3","info":{"title":"Payments","version":"v1"},"paths":{}}`)
	}
	licenseBytes := []byte("MIT\n")
	writeCatalogFile(t, repository, "openapi.json", string(documentBytes))
	writeCatalogFile(t, repository, "LICENSE", string(licenseBytes))
	files := map[string][]byte{"openapi.json": documentBytes, "LICENSE": licenseBytes}
	for filename, contents := range extraFiles {
		writeCatalogFile(t, repository, filename, string(contents))
		files[filename] = contents
	}
	runGitTestCommand(t, repository, "add", ".")
	runGitTestCommand(t, repository, "commit", "-qm", "fixture")
	commit := runGitTestCommand(t, repository, "rev-parse", "HEAD")
	tree := runGitTestCommand(t, repository, "rev-parse", "HEAD^{tree}")
	licenseObject := runGitTestCommand(t, repository, "rev-parse", "HEAD:LICENSE")
	artifacts := make([]testGitArtifactEvidence, 0, len(artifactPaths))
	for _, artifactPath := range artifactPaths {
		contents, exists := files[artifactPath]
		if !exists {
			t.Fatalf("fixture artifact %q has no file bytes", artifactPath)
		}
		artifacts = append(artifacts, testGitArtifactEvidence{
			Path: artifactPath, Mode: "100644", Size: int64(len(contents)),
			GitObjectID: runGitTestCommand(t, repository, "rev-parse", "HEAD:"+artifactPath),
			SHA256:      fmt.Sprintf("%x", sha256.Sum256(contents)),
		})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	receipt := testGitSourceReceipt{
		SchemaVersion:   2,
		CatalogID:       "catalog",
		CloneRepository: repository,
		ProvenanceURL:   "https://example.test/payments",
		ObjectFormat:    objectFormat,
		SourceRoot:      ".",
		CommitObjectID:  commit,
		TreeObjectID:    tree,
		Artifacts:       artifacts,
		License: testGitLicenseEvidence{
			Name: "MIT", SPDX: "MIT", UpstreamPath: "LICENSE", Size: int64(len(licenseBytes)), GitBlobSHA: licenseObject,
			SHA256: fmt.Sprintf("%x", sha256.Sum256(licenseBytes)),
		},
	}
	receiptPath := writeTestGitSourceReceipt(t, receipt)
	return testGitIntegrityFixture{
		repository: repository, receipt: receipt, documentBytes: documentBytes,
		source: GitCatalogSource{
			Repository: repository, Ref: commit, Manifest: testCatalogManifest("strict-v1", "openapi.json"),
			IntegrityReceiptPath: receiptPath,
		},
	}
}

func cloneTestGitSourceReceipt(receipt testGitSourceReceipt) testGitSourceReceipt {
	receipt.Artifacts = append([]testGitArtifactEvidence(nil), receipt.Artifacts...)
	return receipt
}

func assertCatalogIntegrityCheck(t *testing.T, err error, check string) {
	t.Helper()
	var integrityErr *CatalogIntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.Check != check || !errors.Is(err, ErrCatalogIntegrity) {
		t.Fatalf("integrity error = %#v, want check %q", err, check)
	}
}
