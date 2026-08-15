package distribution

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectRootRecursesAndRejectsExcludedNestedFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "nested/ok.txt", "ok")
	writeTestFile(t, root, "nested/internal/web/static/request_composer_browser_test.go", "package static")

	_, err := InspectRoot(root, RootOptions{ExcludedPaths: []string{"internal/web/static/request_composer_browser_test.go"}})
	if err == nil {
		t.Fatal("InspectRoot accepted an excluded file below a nested root")
	}
	var inventoryError *InventoryError
	if !errors.As(err, &inventoryError) || inventoryError.Code != "artifact.excluded_source" {
		t.Fatalf("error = %v, want recursive excluded-source error", err)
	}
}

func TestInspectRootRejectsNonexistentRootAsMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := InspectRoot(missing, RootOptions{})
	if err == nil {
		t.Fatal("InspectRoot accepted a nonexistent artifact root")
	}
	var inventoryError *InventoryError
	if !errors.As(err, &inventoryError) || inventoryError.Code != "artifact.root.missing" {
		t.Fatalf("error = %v, want missing-root error", err)
	}
}

func TestInspectRootRejectsLinksAndSpecialEntries(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "ok.txt", "ok")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink("ok.txt", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := InspectRoot(root, RootOptions{})
	if err == nil {
		t.Fatal("InspectRoot accepted a symbolic link")
	}
	var inventoryError *InventoryError
	if !errors.As(err, &inventoryError) || inventoryError.Code != "artifact.file.link" {
		t.Fatalf("error = %v, want link rejection", err)
	}
}

func TestInspectArchiveRequiresDigestAndScansCompleteExtraction(t *testing.T) {
	archive := makeTestTar(t, map[string]string{
		"nested/ok.txt": "ok",
	})
	digest := sha256Digest(archive)
	archivePath := filepath.Join(t.TempDir(), "artifact.tar")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectArchive(archivePath, ArchiveOptions{}); err == nil {
		t.Fatal("InspectArchive accepted an archive without independent digest")
	}
	inventory, err := InspectArchive(archivePath, ArchiveOptions{ExpectedDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Files) != 1 || inventory.Files[0].Path != "nested/ok.txt" {
		t.Fatalf("inventory = %#v, want complete nested file inventory", inventory)
	}
	if _, err := InspectArchive(archivePath, ArchiveOptions{ExpectedDigest: testSHA384Digest(archive)}); err != nil {
		t.Fatalf("InspectArchive rejected a valid SHA-384 digest: %v", err)
	}
	if _, err := InspectArchive(archivePath, ArchiveOptions{ExpectedDigest: strings.Replace(digest, "a", "b", 1)}); err == nil {
		t.Fatal("InspectArchive accepted a drifted archive digest")
	}
}

func TestCompareInventoryDetectsMissingExtraAndChangedFiles(t *testing.T) {
	expected := []FileEvidence{
		{Path: "changed.txt", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("1", 64)},
		{Path: "missing.txt", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("2", 64)},
	}
	actual := []FileEvidence{
		{Path: "changed.txt", Type: "regular", Size: 2, Digest: "sha256:" + strings.Repeat("3", 64)},
		{Path: "extra.txt", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("4", 64)},
	}
	findings := CompareInventory(expected, actual)
	for _, code := range []string{"artifact.drift.changed", "artifact.drift.missing", "artifact.drift.extra"} {
		found := false
		for _, finding := range findings {
			if finding.Code == code {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("findings = %#v, missing %s", findings, code)
		}
	}
}

func TestCompareInventoryRejectsDuplicateRenamedAndUnknownPaths(t *testing.T) {
	expected := []FileEvidence{
		{Path: "bin/manja", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("1", 64)},
		{Path: "bin/manja", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("1", 64)},
	}
	actual := []FileEvidence{
		{Path: "bin/renamed", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("2", 64)},
		{Path: "unknown.txt", Type: "regular", Size: 1, Digest: "sha256:" + strings.Repeat("3", 64)},
	}

	findings := CompareInventory(expected, actual)
	for _, code := range []string{"artifact.drift.duplicate", "artifact.drift.missing", "artifact.drift.extra"} {
		if !hasFinding(findings, code) {
			t.Fatalf("findings = %#v, missing %s", findings, code)
		}
	}
}

func TestGenerateSBOMRejectsUnknownLicenseAndIsByteStable(t *testing.T) {
	dependency := DependencyEvidence{
		Ecosystem: "go", Name: "example.com/runtime", Version: "v1.2.3",
		License: "", Scope: ScopeShipped, Source: "https://example.com/runtime",
		Digest: "sha256:" + strings.Repeat("1", 64),
	}
	if _, _, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency}); err == nil {
		t.Fatal("GenerateSBOM accepted missing license evidence")
	}
	dependency.License = "MIT"
	first, firstEvidence, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency})
	if err != nil {
		t.Fatal(err)
	}
	second, secondEvidence, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || firstEvidence != secondEvidence {
		t.Fatalf("SBOM generation drifted:\n%s\n---\n%s", first, second)
	}
	if bytes.Contains(first, []byte("timestamp")) || bytes.Contains(first, []byte("serialNumber")) {
		t.Fatalf("SBOM contains volatile metadata: %s", first)
	}
}

func TestGenerateNoticeManifestRejectsUnknownLicenseAndIsByteStable(t *testing.T) {
	dependencies := []DependencyEvidence{
		{Ecosystem: "npm", Name: "zeta", Version: "1.0.0", Scope: ScopeShipped, Source: "https://example.test/zeta", Digest: "sha256:" + strings.Repeat("2", 64)},
		{Ecosystem: "go", Name: "example.com/alpha", Version: "v1.0.0", License: "unknown", Scope: ScopeShipped, Source: "https://example.test/alpha", Digest: "sha256:" + strings.Repeat("1", 64)},
	}
	if _, _, err := GenerateNoticeManifest("manja", dependencies); err == nil {
		t.Fatal("GenerateNoticeManifest accepted unknown license evidence")
	}
	dependencies[0].License = "MIT"
	dependencies[1].License = "Apache-2.0"
	first, firstEvidence, err := GenerateNoticeManifest("manja", dependencies)
	if err != nil {
		t.Fatal(err)
	}
	second, secondEvidence, err := GenerateNoticeManifest("manja", dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || firstEvidence != secondEvidence {
		t.Fatalf("notice manifest generation drifted:\n%s\n---\n%s", first, second)
	}
	if bytes.Contains(first, []byte("timestamp")) || bytes.Contains(first, []byte("generatedAt")) {
		t.Fatalf("notice manifest contains volatile metadata: %s", first)
	}
	if !bytes.Contains(first, []byte(`"name": "example.com/alpha"`)) {
		t.Fatalf("notice manifest lacks dependency identity: %s", first)
	}
}

func TestGenerateSBOMRejectsUnrecognizedLicenseIdentifier(t *testing.T) {
	dependency := DependencyEvidence{
		Ecosystem: "go", Name: "example.com/runtime", Version: "v1.2.3",
		License: "not-a-license", Scope: ScopeShipped, Source: "https://example.com/runtime",
		Digest: "sha256:" + strings.Repeat("1", 64),
	}
	if _, _, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency}); err == nil {
		t.Fatal("GenerateSBOM accepted an unrecognized license identifier")
	}
}

func TestGenerateSBOMRejectsDuplicateComponentIdentity(t *testing.T) {
	dependency := DependencyEvidence{
		Ecosystem: "go", Name: "example.com/runtime", Version: "v1.2.3",
		License: "MIT", Scope: ScopeShipped, Source: "https://example.com/runtime",
		Digest: "sha256:" + strings.Repeat("1", 64),
	}
	duplicate := dependency
	duplicate.Digest = "sha256:" + strings.Repeat("2", 64)
	if _, _, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency, duplicate}); err == nil {
		t.Fatal("GenerateSBOM accepted duplicate component identity")
	}
}

func TestGenerateSBOMRejectsMissingOrMutableDependencyIdentity(t *testing.T) {
	dependency := DependencyEvidence{
		Ecosystem: "go", Name: "example.com/runtime", Version: "latest",
		License: "MIT", Scope: ScopeShipped, Digest: "sha256:" + strings.Repeat("1", 64),
	}
	if _, _, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency}); err == nil {
		t.Fatal("GenerateSBOM accepted mutable or missing dependency identity")
	}
	dependency.Version = "v1.2.3"
	if _, _, err := GenerateSBOM("manja", "dev", []DependencyEvidence{dependency}); err == nil {
		t.Fatal("GenerateSBOM accepted missing dependency source")
	}
}

func TestPackBlockedAuthorityNeverWritesReleaseArtifacts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	output := filepath.Join(t.TempDir(), "release")
	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusBlocked},
		RightsHolder: AuthorityEvidence{Status: StatusBlocked},
		Artifacts:    []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root}},
		OutputDir:    output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("authority.provenance.blocked") {
		t.Fatalf("result = %#v, want blocked authority", result.Result)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory exists after blocked pack: %v", err)
	}
}

func TestPackPassesSyntheticAuthorityAndPlacesNotices(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	legalRoot := t.TempDir()
	writeTestFile(t, legalRoot, "LICENSE", "license")
	writeTestFile(t, legalRoot, "NOTICE", "notice")
	writeTestFile(t, legalRoot, "THIRD_PARTY_NOTICES.md", "third party")
	legal := LegalEvidence{
		Holder: "Verified Holder", YearRange: "2026",
		License:    fileEvidenceFromPath(t, legalRoot, "LICENSE"),
		Notice:     fileEvidenceFromPath(t, legalRoot, "NOTICE"),
		ThirdParty: fileEvidenceFromPath(t, legalRoot, "THIRD_PARTY_NOTICES.md"),
	}
	rootInventory, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "release")
	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal, LegalRoot: legalRoot,
		Artifacts: []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root, RootDigest: rootInventory.Digest}},
		OutputDir: output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusPass || len(result.Outputs) != 1 {
		t.Fatalf("result = %#v, outputs = %#v", result.Result, result.Outputs)
	}
	archive, err := os.ReadFile(result.Outputs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archive, mustRead(t, result.Outputs[0].Path)) {
		t.Fatal("packaged archive read is not stable")
	}
	inspected, err := InspectArchive(result.Outputs[0].Path, ArchiveOptions{ExpectedDigest: result.Outputs[0].Digest})
	if err != nil {
		t.Fatal(err)
	}
	for _, pathValue := range []string{"LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md", "sbom/manja.cdx.json", "notices/manja.json"} {
		if !inventoryHas(inspected.Files, pathValue) {
			t.Fatalf("final archive lacks required placement %q: %#v", pathValue, inspected.Files)
		}
	}
}

func TestPackRejectsIncompleteSBOMAlreadyInArtifactRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	writeTestFile(t, root, "sbom/manja.cdx.json", `{"bomFormat":"CycloneDX"}`)
	legalRoot, legal := testLegalEvidence(t)
	rootInventory, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "release")

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal,
		LegalRoot:    legalRoot,
		Artifacts:    []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root, RootDigest: rootInventory.Digest}},
		OutputDir:    output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.sbom.incomplete") {
		t.Fatalf("result = %#v, want incomplete-SBOM blocker", result.Result)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory exists after incomplete SBOM: %v", err)
	}
}

func TestPackRejectsDriftedNoticeManifestAlreadyInArtifactRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	writeTestFile(t, root, "notices/manja.json", `{"format":"Manja-Notice-Manifest-JSON","schemaVersion":1,"artifact":"manja","dependencies":[]}`)
	legalRoot, legal := testLegalEvidence(t)
	rootInventory, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "release")

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal,
		LegalRoot:    legalRoot,
		Artifacts:    []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root, RootDigest: rootInventory.Digest}},
		OutputDir:    output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.notice_manifest.bytes_mismatch") {
		t.Fatalf("result = %#v, want drifted notice-manifest blocker", result.Result)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory exists after notice-manifest drift: %v", err)
	}
}

func TestPackValidatesLegalBytesBeforeCreatingOutput(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	legalRoot, legal := testLegalEvidence(t)
	legal.License.Digest = "sha256:" + strings.Repeat("f", 64)
	output := filepath.Join(t.TempDir(), "release")

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal,
		LegalRoot:    legalRoot,
		Artifacts:    []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root}},
		OutputDir:    output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("legal.file.digest_mismatch") {
		t.Fatalf("result = %#v, want legal digest blocker", result.Result)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory exists before legal clearance: %v", err)
	}
}

func TestPackDoesNotLeavePartialOutputsWhenLaterArtifactFails(t *testing.T) {
	rootOne := t.TempDir()
	writeTestFile(t, rootOne, "bin/manja", "first")
	rootTwo := t.TempDir()
	writeTestFile(t, rootTwo, "bin/manja", "second")
	legalRoot, legal := testLegalEvidence(t)
	output := filepath.Join(t.TempDir(), "release")

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal,
		LegalRoot:    legalRoot,
		Artifacts: []ArtifactRequest{
			{Name: "first", Kind: ArtifactBinary, Source: "git:test", Root: rootOne},
			{Name: "second", Kind: ArtifactBinary, Source: "git:test", Root: rootTwo, ExpectedDigest: "sha256:" + strings.Repeat("f", 64)},
		},
		OutputDir: output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.package.failed") {
		t.Fatalf("result = %#v, want blocked package failure", result.Result)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("outputs = %#v, want none after blocked package failure", result.Outputs)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory exists after partial package failure: %v", err)
	}
}

func TestPackDoesNotPackageOCIAsSyntheticTar(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "usr/local/bin/manja", "binary")
	legalRoot, legal := testLegalEvidence(t)
	rootInventory, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "release")

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal,
		LegalRoot:    legalRoot,
		Artifacts: []ArtifactRequest{{
			Name: "manja", Kind: ArtifactOCI, Source: "ghcr.io/araihu/manja:latest", Root: root,
			RootDigest: rootInventory.Digest, PlatformCoverageComplete: true,
			Platforms: []PlatformEvidence{{OS: "linux", Architecture: "amd64", Digest: "sha256:" + strings.Repeat("a", 64)}},
		}},
		OutputDir: output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.oci.real_artifact_missing") {
		t.Fatalf("result = %#v, want explicit OCI blocker", result.Result)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("outputs = %#v, want none for synthetic OCI packaging", result.Outputs)
	}
}

func TestValidateArtifactRootReportsActualDrift(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "before")
	expected, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "bin/manja", "after")
	writeTestFile(t, root, "extra.txt", "extra")
	artifact := ArtifactEvidence{Name: "manja", Kind: ArtifactBinary, Digest: expected.Digest, Files: expected.Files}
	_, findings := ValidateArtifactRoot(root, artifact, DefaultPolicy())
	if !hasFinding(findings, "artifact.drift.changed") || !hasFinding(findings, "artifact.drift.extra") || !hasFinding(findings, "artifact.root.digest_mismatch") {
		t.Fatalf("findings = %#v, want actual-root drift", findings)
	}
}

func writeTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	pathValue := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(pathValue), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathValue, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeTestTar(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for pathValue, content := range files {
		data := []byte(content)
		if err := writer.WriteHeader(&tar.Header{Name: pathValue, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func fileEvidenceFromPath(t *testing.T, root, relative string) FileEvidence {
	t.Helper()
	pathValue := filepath.Join(root, filepath.FromSlash(relative))
	data, err := os.ReadFile(pathValue)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return FileEvidence{Path: relative, Type: "regular", Size: int64(len(data)), Digest: "sha256:" + hex.EncodeToString(digest[:])}
}

func mustRead(t *testing.T, pathValue string) []byte {
	t.Helper()
	data, err := os.ReadFile(pathValue)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func inventoryHas(files []FileEvidence, pathValue string) bool {
	for _, file := range files {
		if file.Path == pathValue {
			return true
		}
	}
	return false
}

func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func testSHA384Digest(data []byte) string {
	digest := sha512.Sum384(data)
	return "sha384:" + hex.EncodeToString(digest[:])
}

func testLegalEvidence(t *testing.T) (string, LegalEvidence) {
	t.Helper()
	legalRoot := t.TempDir()
	writeTestFile(t, legalRoot, "LICENSE", "license")
	writeTestFile(t, legalRoot, "NOTICE", "notice")
	writeTestFile(t, legalRoot, "THIRD_PARTY_NOTICES.md", "third party")
	return legalRoot, LegalEvidence{
		Holder:     "Verified Holder",
		YearRange:  "2026",
		License:    fileEvidenceFromPath(t, legalRoot, "LICENSE"),
		Notice:     fileEvidenceFromPath(t, legalRoot, "NOTICE"),
		ThirdParty: fileEvidenceFromPath(t, legalRoot, "THIRD_PARTY_NOTICES.md"),
	}
}
