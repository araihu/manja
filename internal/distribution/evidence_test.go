package distribution

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	root, input := gitAuthorityEvidence(t, "docs/legal/provenance-receipt.txt", []byte("provenance receipt"), "https://example.com/manja.git")

	resolved, err := ResolveAuthority(root, input)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.resolved || !bytes.Equal(resolved.Receipt, input.Receipt) {
		t.Fatalf("resolved authority = %#v", resolved)
	}
}

func TestResolveAuthorityRejectsNonGitWrongRepoAndMissingCommitPath(t *testing.T) {
	root, input := gitAuthorityEvidence(t, "docs/legal/provenance-receipt.txt", []byte("provenance receipt"), "https://example.com/manja.git")
	cases := map[string]func(*AuthorityEvidence, string){
		"non-git-root": func(value *AuthorityEvidence, _ string) {},
		"wrong-repository": func(value *AuthorityEvidence, _ string) {
			value.Reference = strings.Replace(value.Reference, "example.com/manja", "example.com/other", 1)
		},
		"missing-commit": func(value *AuthorityEvidence, commit string) {
			value.Reference = strings.Replace(value.Reference, commit, strings.Repeat("0", 40), 1)
		},
		"missing-path": func(value *AuthorityEvidence, _ string) {
			value.Reference = strings.Replace(value.Reference, "docs/legal/provenance-receipt.txt", "docs/legal/missing.txt", 1)
		},
	}
	commit := strings.Split(strings.Split(input.Reference, "@")[1], ":")[0]
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := input
			candidate.resolved = false
			mutate(&candidate, commit)
			candidateRoot := root
			if name == "non-git-root" {
				candidateRoot = t.TempDir()
			}
			if _, err := ResolveAuthority(candidateRoot, candidate); err == nil {
				t.Fatal("ResolveAuthority accepted unverifiable authority")
			}
		})
	}
}

func TestResolveDependencyLicenseBindsImmutableGitBytesAndMode(t *testing.T) {
	root := t.TempDir()
	content := []byte("MIT License\n")
	writeTestFile(t, root, "LICENSE", string(content))
	if err := os.Chmod(filepath.Join(root, "LICENSE"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "init")
	runGitTest(t, root, "config", "user.email", "test@example.com")
	runGitTest(t, root, "config", "user.name", "Test")
	runGitTest(t, root, "remote", "add", "origin", "https://example.com/licenses.git")
	runGitTest(t, root, "add", "LICENSE")
	runGitTest(t, root, "commit", "-m", "license")
	commit := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD^{tree}"))
	blob := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD:LICENSE"))
	dependency := DependencyEvidence{
		Ecosystem: "go", Name: "example.com/runtime", Version: "v1.2.3", License: "MIT", Scope: ScopeShipped,
		Source: "https://example.com/runtime@" + strings.Repeat("a", 40), Digest: "sha256:" + strings.Repeat("1", 64),
		LicenseReceipt: LicenseReceipt{
			Reference: "git:example.com/licenses@" + commit + ":LICENSE#tree=" + tree + "&blob=" + blob,
			Tree:      tree,
			Size:      int64(len(content)), Mode: 0o644, Digest: sha256Digest(content),
		},
	}
	resolved, err := ResolveDependencyLicense(root, dependency)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.LicenseReceipt.resolved || !bytes.Equal(resolved.LicenseReceipt.Receipt, content) {
		t.Fatalf("resolved dependency license = %#v", resolved)
	}
}

func TestEvaluateRejectsRightsReceiptThatDoesNotAuthorizeLegalClaim(t *testing.T) {
	evidence := validEvidence()
	receipt := []byte("rights receipt\n" +
		"copyright-holder: Other Holder\n" +
		"copyright-year-range: 2026\n" +
		"redistribution: verified for test fixture\n" +
		"trademark: verified for test fixture\n")
	evidence.RightsHolder.Receipt = receipt
	evidence.RightsHolder.Size = int64(len(receipt))
	evidence.RightsHolder.Digest = sha256Digest(receipt)
	parts := strings.Split(evidence.RightsHolder.Reference, "&blob=")
	evidence.RightsHolder.Reference = parts[0] + "&blob=" + gitBlobSHA1(receipt)
	result := Evaluate(evidence, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("authority.rights_holder.claim_mismatch") {
		t.Fatalf("result = %#v, want rights-holder claim blocker", result)
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

func TestSerializedDependencyLicenseCannotSelfAssertPass(t *testing.T) {
	encoded, err := MarshalCanonical(validEvidence())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrict(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	result := Evaluate(decoded, DefaultPolicy())
	if result.Status != StatusBlocked || !result.HasCode("dependency.license.unresolved") {
		t.Fatalf("result = %#v, want serialized license receipt to remain blocked", result)
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
				Ecosystem:      "go",
				Name:           "example.com/runtime",
				Version:        "v1.2.3",
				License:        "MIT",
				Scope:          ScopeShipped,
				Source:         "https://example.com/runtime@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Digest:         "sha256:" + strings.Repeat("5", 64),
				LicenseReceipt: testLicenseReceipt(),
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
				Source:     "git:c20241437b6309b5ce73d8ab30f14e3be9812552",
				Digest:     "sha256:" + strings.Repeat("7", 64),
				Inspection: InspectionEvidence{Complete: true, FreshRoot: true, DigestBound: true},
				SBOM: SBOMEvidence{
					Format: "CycloneDX-JSON", Source: "sbom/manja-runtime.cdx.json",
					Size: 31, Mode: 0o644, Digest: "sha256:" + strings.Repeat("a", 64), Complete: true,
				},
				Dependencies:                []string{"example.com/runtime"},
				DependencyInventoryComplete: true,
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

func testProvenanceEvidence() AuthorityEvidence {
	receipt := []byte("provenance receipt\n" +
		"copyright-holder: Verified Holder\n" +
		"copyright-year-range: 2026\n" +
		"redistribution: verified for test fixture\n" +
		"trademark: verified for test fixture\n")
	tree := strings.Repeat("b", 40)
	return AuthorityEvidence{
		Status: StatusPass,
		Reference: "git:example.com/manja@" + strings.Repeat("a", 40) +
			":docs/legal/provenance-receipt.txt#tree=" + tree + "&blob=" + gitBlobSHA1(receipt),
		Tree: tree, Size: int64(len(receipt)), Mode: 0o644,
		Digest: sha256Digest(receipt), Receipt: receipt,
		Claims: AuthorityClaims{
			CopyrightHolder: "Verified Holder", CopyrightYearRange: "2026",
			Redistribution: "verified for test fixture", Trademark: "verified for test fixture",
		},
		resolved: true,
	}
}

func testRightsHolderEvidence() AuthorityEvidence {
	receipt := []byte("rights receipt\n" +
		"copyright-holder: Verified Holder\n" +
		"copyright-year-range: 2026\n" +
		"redistribution: verified for test fixture\n" +
		"trademark: verified for test fixture\n")
	tree := strings.Repeat("c", 40)
	return AuthorityEvidence{
		Status: StatusPass,
		Reference: "git:example.com/manja@" + strings.Repeat("b", 40) +
			":docs/legal/rights-holder-receipt.txt#tree=" + tree + "&blob=" + gitBlobSHA1(receipt),
		Tree: tree, Size: int64(len(receipt)), Mode: 0o644,
		Digest: sha256Digest(receipt), Receipt: receipt,
		Claims: AuthorityClaims{
			CopyrightHolder: "Verified Holder", CopyrightYearRange: "2026",
			Redistribution: "verified for test fixture", Trademark: "verified for test fixture",
		},
		resolved: true,
	}
}

func testLicenseReceipt() LicenseReceipt {
	receipt := []byte("SPDX-License-Identifier: MIT\nMIT License\nPermission is hereby granted\n")
	tree := strings.Repeat("d", 40)
	return LicenseReceipt{
		Reference: "git:example.com/licenses@" + strings.Repeat("c", 40) + ":LICENSE#tree=" + tree + "&blob=" + gitBlobSHA1(receipt),
		Tree:      tree, Size: int64(len(receipt)), Mode: 0o644,
		Digest: sha256Digest(receipt), Receipt: receipt, resolved: true,
	}
}

func gitAuthorityEvidence(t *testing.T, relative string, content []byte, remote string) (string, AuthorityEvidence) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, relative, string(content))
	runGitTest(t, root, "init")
	runGitTest(t, root, "config", "user.email", "test@example.com")
	runGitTest(t, root, "config", "user.name", "Test")
	runGitTest(t, root, "remote", "add", "origin", remote)
	runGitTest(t, root, "add", ".")
	runGitTest(t, root, "commit", "-m", "receipt")
	commit := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD^{tree}"))
	blob := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD:"+relative))
	return root, AuthorityEvidence{
		Status:    StatusPass,
		Reference: "git:example.com/manja@" + commit + ":" + relative + "#tree=" + tree + "&blob=" + blob,
		Tree:      tree, Size: int64(len(content)), Mode: 0o644,
		Digest: sha256Digest(content), Receipt: append([]byte(nil), content...),
	}
}

func runGitTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := gitCommand(root, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func validFile(path string, size int64) FileEvidence {
	return FileEvidence{Path: path, Type: "regular", Size: size, Mode: 0o644, Digest: "sha256:" + strings.Repeat("8", 64)}
}

func TestSPDXParserRequiresKnownExceptionsForWITH(t *testing.T) {
	if !validSPDXExpression("GPL-2.0-only WITH Classpath-exception-2.0") {
		t.Fatal("valid SPDX exception expression was rejected")
	}
	if validSPDXExpression("MIT WITH made-up-exception") {
		t.Fatal("unknown SPDX exception was accepted")
	}
	if validSPDXExpression("MIT OR") {
		t.Fatal("incomplete SPDX expression was accepted")
	}
}

func TestLicenseReceiptDoesNotMapEveryLicenseToGenericLicenseText(t *testing.T) {
	findings := validateLicenseReceipt("example.com/runtime", "Unlicense", testLicenseReceipt())
	if !hasFinding(findings, "dependency.license.claim_mismatch") {
		t.Fatalf("findings = %#v, want a license/text mismatch", findings)
	}
}
