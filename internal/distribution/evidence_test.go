package distribution

import (
	"bytes"
	"encoding/json"
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
		Provenance: AuthorityEvidence{
			Status:    StatusPass,
			Reference: "docs/legal/provenance.md#evidence",
			Digest:    "sha256:" + strings.Repeat("3", 64),
		},
		RightsHolder: AuthorityEvidence{
			Status:    StatusPass,
			Reference: "docs/legal/rights-holder-confirmation.md",
			Digest:    "sha256:" + strings.Repeat("4", 64),
		},
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
				Source:    "https://example.com/runtime",
				Digest:    "sha256:" + strings.Repeat("5", 64),
			},
			{
				Ecosystem: "go",
				Name:      "example.com/tool",
				Version:   "v2.3.4",
				License:   "Apache-2.0",
				Scope:     ScopeBuildOnly,
				Source:    "https://example.com/tool",
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
					Digest: "sha256:" + strings.Repeat("a", 64), Complete: true,
				},
				Dependencies: []string{"example.com/runtime"},
				Files: []FileEvidence{
					validFile("LICENSE", 11),
					validFile("NOTICE", 7),
					validFile("THIRD_PARTY_NOTICES.md", 23),
					{Path: "sbom/manja-runtime.cdx.json", Type: "regular", Size: 31, Digest: "sha256:" + strings.Repeat("a", 64)},
				},
			},
		},
	}
}

func validFile(path string, size int64) FileEvidence {
	return FileEvidence{Path: path, Type: "regular", Size: size, Digest: "sha256:" + strings.Repeat("8", 64)}
}
