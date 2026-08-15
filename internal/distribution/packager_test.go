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

func TestInspectRootIncludesExecutableModeInInventoryAndDigest(t *testing.T) {
	root := t.TempDir()
	pathValue := filepath.Join(root, "bin", "manja")
	writeTestFile(t, root, "bin/manja", "binary")
	if err := os.Chmod(pathValue, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != 1 || first.Files[0].Mode != 0o755 {
		t.Fatalf("inventory = %#v, want executable mode", first)
	}
	if err := os.Chmod(pathValue, 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := InspectRoot(root, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatalf("inventory digest ignored mode change: %s", first.Digest)
	}
	if !hasFinding(CompareInventory(first.Files, second.Files), "artifact.drift.changed") {
		t.Fatal("inventory comparison ignored mode change")
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
	drifted := "sha256:" + strings.Repeat("0", 64)
	if drifted == digest {
		drifted = "sha256:" + strings.Repeat("1", 64)
	}
	if _, err := InspectArchive(archivePath, ArchiveOptions{ExpectedDigest: drifted}); err == nil {
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

func TestPackReportsMissingOutputDirectoryAfterGates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	legalRoot, legal := testLegalEvidence(t)

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal, LegalRoot: legalRoot,
		Artifacts: []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root}},
	}, DefaultPolicy())
	if err == nil {
		t.Fatal("Pack accepted a missing output directory")
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.output.missing") {
		t.Fatalf("result = %#v, want explicit output-directory blocker", result.Result)
	}
}

func TestPackPassesSyntheticAuthorityAndPlacesNotices(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	if err := os.Chmod(filepath.Join(root, "bin", "manja"), 0o755); err != nil {
		t.Fatal(err)
	}
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
	packageRequest := PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal, LegalRoot: legalRoot,
		Artifacts: []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root, RootDigest: rootInventory.Digest}},
		OutputDir: output,
	}
	result, err := Pack(packageRequest, DefaultPolicy())
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
	secondOutput := filepath.Join(t.TempDir(), "release")
	packageRequest.OutputDir = secondOutput
	second, err := Pack(packageRequest, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	secondArchive, err := os.ReadFile(second.Outputs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outputs[0].Digest != second.Outputs[0].Digest || !bytes.Equal(archive, secondArchive) {
		t.Fatal("equivalent package inputs produced different archive bytes")
	}
	inspected, err := InspectArchive(result.Outputs[0].Path, ArchiveOptions{ExpectedDigest: result.Outputs[0].Digest})
	if err != nil {
		t.Fatal(err)
	}
	for _, pathValue := range []string{"LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md", "sbom/manja.cdx.json"} {
		if !inventoryHas(inspected.Files, pathValue) {
			t.Fatalf("final archive lacks required placement %q: %#v", pathValue, inspected.Files)
		}
	}
	if !inventoryHas(inspected.Files, "bin/manja") {
		t.Fatalf("final archive lacks executable: %#v", inspected.Files)
	}
	for _, file := range inspected.Files {
		if file.Path == "bin/manja" && file.Mode != 0o755 {
			t.Fatalf("final archive changed executable mode: %#v", file)
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

func TestPackDoesNotLeaveArtifactsWhenLaterPackageFails(t *testing.T) {
	firstRoot := t.TempDir()
	writeTestFile(t, firstRoot, "bin/first", "first")
	firstInventory, err := InspectRoot(firstRoot, RootOptions{})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot := t.TempDir()
	writeTestFile(t, secondRoot, "bin/second", "second")
	legalRoot, legal := testLegalEvidence(t)
	output := filepath.Join(t.TempDir(), "release")
	wrongDigest := "sha256:" + strings.Repeat("f", 64)

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal,
		LegalRoot:    legalRoot,
		Artifacts: []ArtifactRequest{
			{Name: "first", Kind: ArtifactBinary, Source: "git:first", Root: firstRoot},
			{Name: "second", Kind: ArtifactBinary, Source: "git:second", Root: secondRoot, ExpectedDigest: wrongDigest},
		},
		OutputDir: output,
	}, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.package.failed") {
		t.Fatalf("result = %#v, want package failure blocker", result.Result)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("outputs = %#v, want none after staged package failure", result.Outputs)
	}
	if len(result.Evidence.Artifacts) != 2 || result.Evidence.Artifacts[0].Digest != firstInventory.Digest {
		t.Fatalf("evidence = %#v, want pre-packaging artifact evidence", result.Evidence)
	}
	if result.Evidence.Artifacts[0].Inspection.FreshRoot || result.Evidence.Artifacts[0].Inspection.DigestBound {
		t.Fatalf("pre-packaging evidence claimed final extraction: %#v", result.Evidence.Artifacts[0].Inspection)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output directory exists after staged package failure: %v", err)
	}
}

func TestPublishOutputsRemovesOnlyNewEmptyDirectoryOnFailure(t *testing.T) {
	staged := t.TempDir()
	first := filepath.Join(staged, "first.tar")
	if err := os.WriteFile(first, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputRoot := t.TempDir()
	newOutput := filepath.Join(outputRoot, "new-release")
	if _, err := publishOutputs(newOutput, []PackagedArtifact{
		{Name: "first", Path: first},
		{Name: "second", Path: filepath.Join(staged, "missing.tar")},
	}); err == nil {
		t.Fatal("publishOutputs accepted a missing staged archive")
	}
	if _, err := os.Stat(newOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new output directory survived failed publication: %v", err)
	}

	existingOutput := filepath.Join(outputRoot, "existing-release")
	if err := os.Mkdir(existingOutput, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := publishOutputs(existingOutput, []PackagedArtifact{
		{Name: "first", Path: first},
		{Name: "second", Path: filepath.Join(staged, "missing.tar")},
	}); err == nil {
		t.Fatal("publishOutputs accepted a missing staged archive in an existing directory")
	}
	if _, err := os.Stat(existingOutput); err != nil {
		t.Fatalf("pre-existing output directory was removed: %v", err)
	}
}

func TestPackRejectsUnsafeSBOMPlacement(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/manja", "binary")
	legalRoot, legal := testLegalEvidence(t)
	policy := DefaultPolicy()
	policy.SBOMPlacement = map[ArtifactKind]string{ArtifactBinary: "../outside"}

	result, err := Pack(PackageRequest{
		Subject:      SubjectEvidence{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40)},
		Provenance:   AuthorityEvidence{Status: StatusPass, Reference: "provenance-receipt", Digest: "sha256:" + strings.Repeat("1", 64)},
		RightsHolder: AuthorityEvidence{Status: StatusPass, Reference: "rights-receipt", Digest: "sha256:" + strings.Repeat("2", 64)},
		Legal:        legal, LegalRoot: legalRoot,
		Artifacts: []ArtifactRequest{{Name: "manja", Kind: ArtifactBinary, Source: "git:test", Root: root}},
		OutputDir: filepath.Join(t.TempDir(), "release"),
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != StatusBlocked || !result.Result.HasCode("artifact.sbom.path_unsafe") {
		t.Fatalf("result = %#v, want unsafe-SBOM-path blocker", result.Result)
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
