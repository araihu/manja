package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/distribution"
)

func TestRunCheckReturnsRawBlockedReceiptWithoutWritingArtifacts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inputPath := filepath.Join(root, "evidence.json")
	evidence := distribution.Evidence{
		SchemaVersion: 1,
		Subject: distribution.SubjectEvidence{
			CommitSHA: strings.Repeat("a", 40),
			TreeSHA:   strings.Repeat("b", 40),
		},
		Provenance:   distribution.AuthorityEvidence{Status: distribution.StatusBlocked, Reference: "docs/legal/provenance.md"},
		RightsHolder: distribution.AuthorityEvidence{Status: distribution.StatusBlocked, Reference: "docs/legal/provenance.md"},
	}
	input, err := distribution.MarshalCanonical(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "-input", inputPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var result distribution.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid raw receipt: %v\n%s", err, stdout.Bytes())
	}
	if result.Status != distribution.StatusBlocked || !result.HasCode("authority.provenance.blocked") {
		t.Fatalf("result = %#v", result)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "evidence.json" {
		t.Fatalf("command created release output: %#v", entries)
	}
}

func TestRunCanonicalProducesStableEvidenceBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inputPath := filepath.Join(root, "evidence.json")
	evidence := distribution.Evidence{
		SchemaVersion: 1,
		Subject: distribution.SubjectEvidence{
			CommitSHA: strings.Repeat("a", 40),
			TreeSHA:   strings.Repeat("b", 40),
		},
		Provenance:   distribution.AuthorityEvidence{Status: distribution.StatusBlocked},
		RightsHolder: distribution.AuthorityEvidence{Status: distribution.StatusBlocked},
		Dependencies: []distribution.DependencyEvidence{
			{Name: "z", Ecosystem: "go", Version: "v1", Scope: distribution.ScopeBuildOnly, License: "MIT", Source: "z", Digest: "sha256:" + strings.Repeat("1", 64)},
			{Name: "a", Ecosystem: "go", Version: "v1", Scope: distribution.ScopeTestOnly, License: "MIT", Source: "a", Digest: "sha256:" + strings.Repeat("2", 64)},
		},
	}
	input, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"canonical", "-input", inputPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	expected, err := distribution.MarshalCanonical(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), expected) {
		t.Fatalf("canonical output differs:\n%s\n---\n%s", stdout.Bytes(), expected)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunRejectsTrailingEvidenceValue(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inputPath := filepath.Join(root, "evidence.json")
	if err := os.WriteFile(inputPath, []byte(`{"schemaVersion":1} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"check", "-input", inputPath}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "multiple JSON values") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
