package distribution

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestEvaluatePreservesBlockedAuthorityAndDoesNotClearLegalGate(t *testing.T) {
	evidence := Evidence{
		SchemaVersion: 1,
		Subject: SubjectEvidence{
			CommitSHA: strings.Repeat("a", 40),
			TreeSHA:   strings.Repeat("b", 40),
		},
		Provenance:   AuthorityEvidence{Status: StatusBlocked, Reference: "docs/legal/provenance.md"},
		RightsHolder: AuthorityEvidence{Status: StatusBlocked, Reference: "docs/legal/provenance.md"},
	}

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, StatusBlocked)
	}
	if !result.HasCode("authority.provenance.blocked") {
		t.Fatalf("result lacks provenance blocker: %#v", result.Findings)
	}
	if !result.HasCode("authority.rights_holder.blocked") {
		t.Fatalf("result lacks rights-holder blocker: %#v", result.Findings)
	}
	if result.Status == StatusPass {
		t.Fatal("blocked authority was cleared")
	}
}

func TestEvaluateRejectsLegalClaimBeforeAuthorityEvidence(t *testing.T) {
	evidence := validEvidence()
	evidence.Provenance.Status = StatusBlocked
	evidence.RightsHolder.Status = StatusBlocked
	evidence.Legal = LegalEvidence{
		Holder:    "Unverified Holder",
		YearRange: "2026",
		License:   validFile("LICENSE", 11),
		Notice:    validFile("NOTICE", 7),
	}

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("legal.materials.before_clearance") {
		t.Fatalf("result = %#v, want blocked pre-clearance finding", result)
	}
}

func TestEvaluateRejectsUnverifiedAuthorityAndMutableDependency(t *testing.T) {
	evidence := validEvidence()
	evidence.Provenance = AuthorityEvidence{
		Status:    StatusPass,
		Reference: "https://example.com/provenance/latest",
		Digest:    "sha256:" + strings.Repeat("f", 64),
		Receipt:   []byte("invented receipt"),
	}
	evidence.Dependencies[0].Version = "^1"
	evidence.Dependencies[0].Source = "https://example.com/runtime/latest"

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("authority.provenance.reference_invalid") || !result.HasCode("authority.provenance.receipt_digest_mismatch") || !result.HasCode("dependency.version.invalid") || !result.HasCode("dependency.source.invalid") {
		t.Fatalf("result = %#v, want unverified-authority and mutable-dependency blockers", result)
	}
}

func TestResolveAuthorityBindsReferencedReceiptBytes(t *testing.T) {
	root := t.TempDir()
	input := gitAuthorityEvidence(t, root, "docs/legal/provenance-receipt.txt", "provenance receipt")
	input.resolved = false

	resolved, err := ResolveAuthority(root, input)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.resolved || !bytes.Equal(resolved.Receipt, input.Receipt) {
		t.Fatalf("resolved authority = %#v", resolved)
	}
}

func TestResolveAuthorityRejectsNonGitRootAndFakeCommit(t *testing.T) {
	nonGitRoot := t.TempDir()
	writeTestFile(t, nonGitRoot, "docs/legal/provenance-receipt.txt", "provenance receipt")
	if _, err := ResolveAuthority(nonGitRoot, testProvenanceEvidence()); err == nil {
		t.Fatal("ResolveAuthority accepted a receipt from a non-Git directory")
	}

	root := t.TempDir()
	input := gitAuthorityEvidence(t, root, "docs/legal/provenance-receipt.txt", "provenance receipt")
	blob := input.Reference[strings.LastIndex(input.Reference, "#blob=")+len("#blob="):]
	input.Reference = "git:example.com/manja@" + strings.Repeat("f", 40) + ":docs/legal/provenance-receipt.txt#blob=" + blob
	if _, err := ResolveAuthority(root, input); err == nil {
		t.Fatal("ResolveAuthority accepted a fabricated commit")
	}
}

func TestResolveAuthorityRejectsWorkingTreeBytesOutsideGitObject(t *testing.T) {
	root := t.TempDir()
	input := gitAuthorityEvidence(t, root, "docs/legal/provenance-receipt.txt", "provenance receipt")
	writeTestFile(t, root, "docs/legal/provenance-receipt.txt", "drifted receipt")
	if _, err := ResolveAuthority(root, input); err == nil {
		t.Fatal("ResolveAuthority accepted working-tree bytes different from the referenced Git object")
	}
}

func TestSerializedAuthorityCannotSelfAssertPass(t *testing.T) {
	encoded, err := MarshalCanonical(validEvidence())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrict(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	result := Evaluate(decoded, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("authority.provenance.unresolved") || !result.HasCode("authority.rights_holder.unresolved") {
		t.Fatalf("result = %#v, want serialized authority to remain blocked", result)
	}
}

func TestEvaluateRejectsUnknownDependencyAndTestOnlyLeak(t *testing.T) {
	evidence := validEvidence()
	evidence.Dependencies = append(evidence.Dependencies,
		DependencyEvidence{
			Ecosystem: "go",
			Name:      "example.com/unknown",
			Version:   "v1.2.3",
			Scope:     ScopeShipped,
			Source:    "https://example.com/unknown",
			Digest:    "sha256:" + strings.Repeat("1", 64),
		},
		DependencyEvidence{
			Ecosystem: "go",
			Name:      "example.com/test-only",
			Version:   "v1.2.3",
			License:   "MIT",
			Scope:     ScopeTestOnly,
			Source:    "https://example.com/test-only",
			Digest:    "sha256:" + strings.Repeat("2", 64),
		},
	)
	evidence.Artifacts[0].Dependencies = append(evidence.Artifacts[0].Dependencies, "example.com/test-only")

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, StatusBlocked)
	}
	if !result.HasCode("dependency.license.missing") {
		t.Fatalf("result lacks unknown-license blocker: %#v", result.Findings)
	}
	if !result.HasCode("dependency.test_only_shipped") {
		t.Fatalf("result lacks test-only leak blocker: %#v", result.Findings)
	}
}

func TestEvaluateRejectsExcludedRuntimeSourcesAndUnsafePaths(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Files = append(evidence.Artifacts[0].Files,
		validFile("internal/web/static/request_composer_browser_test.go", 10),
		validFile("nested/../escape", 10),
	)

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, StatusBlocked)
	}
	if !result.HasCode("artifact.excluded_source") {
		t.Fatalf("result lacks excluded-source blocker: %#v", result.Findings)
	}
	if !result.HasCode("artifact.path.unsafe") {
		t.Fatalf("result lacks unsafe-path blocker: %#v", result.Findings)
	}
}

func TestEvaluateRejectsUntrustedRootAndNonRegularEntries(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Inspection = InspectionEvidence{Complete: false, FreshRoot: true}
	evidence.Artifacts[0].Files[0].Type = "symlink"

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, StatusBlocked)
	}
	if !result.HasCode("artifact.inspection.incomplete") {
		t.Fatalf("result lacks complete-root blocker: %#v", result.Findings)
	}
	if !result.HasCode("artifact.file.type_invalid") {
		t.Fatalf("result lacks non-regular-entry blocker: %#v", result.Findings)
	}
}

func TestEvaluateRejectsIncompleteOCIPlatformInventory(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Kind = ArtifactOCI
	evidence.Artifacts[0].Platforms = []PlatformEvidence{{
		OS: "linux", Architecture: "amd64", Digest: "sha256:" + strings.Repeat("9", 64),
	}}
	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.oci.coverage_incomplete") {
		t.Fatalf("incomplete OCI coverage status = %q, findings = %#v", result.Status, result.Findings)
	}

	evidence.Artifacts[0].PlatformCoverageComplete = true
	evidence.Artifacts[0].Platforms = nil
	result = Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.oci.platforms_missing") {
		t.Fatalf("missing OCI platform inventory result = %#v", result)
	}
}

func TestEvaluateRejectsIncompleteSBOM(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].SBOM.Complete = false

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.sbom.incomplete") {
		t.Fatalf("result = %#v, want incomplete SBOM blocker", result)
	}
}

func TestEvaluateRejectsMissingFileMode(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Files[0].Mode = 0

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.file.mode_invalid") {
		t.Fatalf("result = %#v, want missing-mode blocker", result)
	}
}

func TestEvaluateRejectsSBOMDigestDifferentFromInventory(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Files[3].Digest = "sha256:" + strings.Repeat("b", 64)

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.sbom.bytes_mismatch") {
		t.Fatalf("result = %#v, want SBOM byte-mismatch blocker", result)
	}
}

func TestEvaluateRejectsLegalDigestDifferentFromInventory(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Files[0].Digest = "sha256:" + strings.Repeat("b", 64)

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.legal_file.bytes_mismatch") {
		t.Fatalf("result = %#v, want legal byte-mismatch blocker", result)
	}
}

func TestEvaluateRejectsNoncanonicalDigestLengths(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Digest = "sha256:" + strings.Repeat("a", 96)

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.digest.invalid") {
		t.Fatalf("result = %#v, want exact digest-length blocker", result)
	}
}

func TestEvaluateRejectsAmbiguousDependencyIdentity(t *testing.T) {
	evidence := validEvidence()
	evidence.Dependencies = append(evidence.Dependencies, DependencyEvidence{
		Ecosystem: "npm",
		Name:      "example.com/runtime",
		Version:   "1.2.3",
		License:   "MIT",
		Scope:     ScopeShipped,
		Source:    "https://example.com/runtime-npm",
		Digest:    "sha256:" + strings.Repeat("b", 64),
	})

	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("dependency.identity.ambiguous") || !result.HasCode("artifact.dependency.ambiguous") {
		t.Fatalf("result = %#v, want ambiguous dependency blocker", result)
	}
}

func TestCanonicalEvidenceSortsAndIsByteStable(t *testing.T) {
	first := validEvidence()
	first.Artifacts[0].Platforms = []PlatformEvidence{
		{OS: "windows", Architecture: "amd64", Digest: "sha256:" + strings.Repeat("9", 64)},
		{OS: "linux", Architecture: "amd64", Digest: "sha256:" + strings.Repeat("a", 64)},
	}
	second := validEvidence()
	second.Dependencies = append([]DependencyEvidence(nil), first.Dependencies...)
	second.Dependencies[0], second.Dependencies[1] = second.Dependencies[1], second.Dependencies[0]
	second.Artifacts[0].Files = append([]FileEvidence(nil), first.Artifacts[0].Files...)
	second.Artifacts[0].Files[0], second.Artifacts[0].Files[1] = second.Artifacts[0].Files[1], second.Artifacts[0].Files[0]
	second.Artifacts[0].Platforms = append([]PlatformEvidence(nil), first.Artifacts[0].Platforms...)
	second.Artifacts[0].Platforms[0], second.Artifacts[0].Platforms[1] = second.Artifacts[0].Platforms[1], second.Artifacts[0].Platforms[0]

	firstBytes, err := MarshalCanonical(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalCanonical(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("canonical bytes differ:\n%s\n---\n%s", firstBytes, secondBytes)
	}
	if !bytes.HasSuffix(firstBytes, []byte("\n")) {
		t.Fatal("canonical evidence lacks trailing newline")
	}
	var decoded Evidence
	decoder := json.NewDecoder(bytes.NewReader(firstBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("canonical evidence contains multiple JSON values")
	}
}

func TestCanonicalEvidenceUsesTotalOrderingForEqualPrimaryKeys(t *testing.T) {
	first := validEvidence()
	second := validEvidence()

	fileA := validFile("same.txt", 1)
	fileB := validFile("same.txt", 2)
	fileB.Digest = "sha256:" + strings.Repeat("9", 64)
	first.Artifacts[0].Files = []FileEvidence{fileA, fileB}
	second.Artifacts[0].Files = []FileEvidence{fileB, fileA}

	dependencyA := first.Dependencies[0]
	dependencyA.Version = "v0.0.1"
	dependencyB := first.Dependencies[0]
	dependencyB.Version = "v9.9.9"
	first.Dependencies = []DependencyEvidence{dependencyA, dependencyB}
	second.Dependencies = []DependencyEvidence{dependencyB, dependencyA}

	artifactA := first.Artifacts[0]
	artifactA.Source = "git:a"
	artifactB := first.Artifacts[0]
	artifactB.Source = "git:b"
	first.Artifacts = []ArtifactEvidence{artifactA, artifactB}
	second.Artifacts = []ArtifactEvidence{artifactB, artifactA}

	firstBytes, err := MarshalCanonical(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalCanonical(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("canonical bytes differ for equal primary keys:\n%s\n---\n%s", firstBytes, secondBytes)
	}
}

func TestEvaluatePassesCompleteEvidenceAndAllRuntimeArtifacts(t *testing.T) {
	evidence := validEvidence()
	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusPass {
		t.Fatalf("status = %q, findings = %#v", result.Status, result.Findings)
	}
}

func TestEvaluateRejectsMutableArtifactSource(t *testing.T) {
	evidence := validEvidence()
	evidence.Artifacts[0].Source = "git:test"
	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("artifact.source.invalid") {
		t.Fatalf("result = %#v, want mutable artifact source blocker", result)
	}
}

func validEvidence() Evidence {
	return Evidence{
		SchemaVersion: 1,
		Subject: SubjectEvidence{
			CommitSHA: strings.Repeat("a", 40),
			TreeSHA:   strings.Repeat("b", 40),
		},
		Provenance:   testProvenanceEvidence(),
		RightsHolder: testRightsHolderEvidence(),
		Legal: LegalEvidence{
			Holder:     "Verified Holder",
			YearRange:  "2026",
			License:    validFile("LICENSE", 11),
			Notice:     validFile("NOTICE", 7),
			ThirdParty: validFile("THIRD_PARTY_NOTICES.md", 23),
		},
		Dependencies: []DependencyEvidence{
			{
				Ecosystem: "go",
				Name:      "example.com/runtime",
				Version:   "v1.2.3",
				License:   "MIT",
				Scope:     ScopeShipped,
				Source:    "https://example.com/runtime@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Digest:    "sha256:" + strings.Repeat("5", 64),
			},
			{
				Ecosystem: "go",
				Name:      "example.com/tool",
				Version:   "v2.3.4",
				License:   "Apache-2.0",
				Scope:     ScopeBuildOnly,
				Source:    "https://example.com/tool@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Digest:    "sha256:" + strings.Repeat("6", 64),
			},
		},
		Artifacts: []ArtifactEvidence{
			{
				Name:       "manja-runtime",
				Kind:       ArtifactBinary,
				Source:     "git:example.com/manja@c20241437b6309b5ce73d8ab30f14e3be9812552",
				Digest:     "sha256:" + strings.Repeat("7", 64),
				Inspection: InspectionEvidence{Complete: true, FreshRoot: true, DigestBound: true},
				SBOM: SBOMEvidence{
					Format: "CycloneDX-JSON", Source: "sbom/manja-runtime.cdx.json",
					Digest: "sha256:" + strings.Repeat("a", 64), Complete: true,
				},
				Dependencies: []string{"example.com/runtime"},
				Files: []FileEvidence{
					validFile("LICENSE", 11),
					validFile("NOTICE", 7),
					validFile("THIRD_PARTY_NOTICES.md", 23),
					{Path: "sbom/manja-runtime.cdx.json", Type: "regular", Size: 31, Mode: 0o644, Digest: "sha256:" + strings.Repeat("a", 64)},
				},
			},
		},
	}
}

func gitAuthorityEvidence(t *testing.T, root, receiptPath, content string) AuthorityEvidence {
	t.Helper()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		runGitTest(t, root, args...)
	}
	writeTestFile(t, root, receiptPath, content)
	runGitTest(t, root, "add", receiptPath)
	runGitTest(t, root, "commit", "-m", "receipt")
	commit := strings.TrimSpace(string(runGitTest(t, root, "rev-parse", "HEAD")))
	blob := strings.TrimSpace(string(runGitTest(t, root, "rev-parse", "HEAD:"+receiptPath)))
	return AuthorityEvidence{
		Status:    StatusPass,
		Reference: "git:example.com/manja@" + commit + ":" + receiptPath + "#blob=" + blob,
		Digest:    sha256Digest([]byte(content)),
		Receipt:   []byte(content),
	}
}

func runGitTest(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return output
}

func testProvenanceEvidence() AuthorityEvidence {
	return AuthorityEvidence{
		Status:    StatusPass,
		Reference: "git:example.com/manja@" + strings.Repeat("a", 40) + ":docs/legal/provenance-receipt.txt#blob=a4f33c88285d57140c931a5342b9cb72a1583f25",
		Digest:    "sha256:354f669fd34cc43c5b029902722e79350ee1eed8d197ebc98e3a4d3bf53aaf17",
		Receipt:   []byte("provenance receipt"),
		resolved:  true,
	}
}

func testRightsHolderEvidence() AuthorityEvidence {
	return AuthorityEvidence{
		Status:    StatusPass,
		Reference: "git:example.com/manja@" + strings.Repeat("b", 40) + ":docs/legal/rights-holder-receipt.txt#blob=00ace30a8b1db2d8afe5a51333d9137650e10e8a",
		Digest:    "sha256:0690f2cc4e8e452e6d42a4b43db96dc9816f567573b7dcefb3aebd989daf305d",
		Receipt:   []byte("rights receipt"),
		resolved:  true,
	}
}

func validFile(path string, size int64) FileEvidence {
	return FileEvidence{Path: path, Type: "regular", Size: size, Mode: 0o644, Digest: "sha256:" + strings.Repeat("8", 64)}
}
