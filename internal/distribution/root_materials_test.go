package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryRootLegalMaterialsArePresentButAttributionUnbound(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	evidence, err := LoadRepositoryLegalEvidence(root)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Holder != "" || evidence.YearRange != "" {
		t.Fatalf("repository materials invented attribution: %#v", evidence)
	}
}

func TestLoadRepositoryLegalEvidenceBindsRootBytesWithoutInventingAttribution(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "LICENSE", "MIT License\n")
	writeTestFile(t, root, "NOTICE", "No holder asserted.\n")
	writeTestFile(t, root, "THIRD_PARTY_NOTICES.md", "Upstream notices.\n")

	evidence, err := LoadRepositoryLegalEvidence(root)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Holder != "" || evidence.YearRange != "" {
		t.Fatalf("evidence invented attribution: %#v", evidence)
	}
	for _, file := range []FileEvidence{evidence.License, evidence.Notice, evidence.ThirdParty} {
		if file.Type != "regular" || file.Mode != 0o644 || file.Size <= 0 || !strings.HasPrefix(file.Digest, "sha256:") {
			t.Fatalf("file evidence = %#v, want regular non-empty sha256-bound file", file)
		}
	}
	if evidence.License.Path != "LICENSE" || evidence.Notice.Path != "NOTICE" || evidence.ThirdParty.Path != "THIRD_PARTY_NOTICES.md" {
		t.Fatalf("evidence paths = %#v, want canonical root paths", evidence)
	}
}

func TestLoadRepositoryLegalEvidenceRejectsMissingOrUnsafeRootMaterial(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "LICENSE", "MIT License\n")
	writeTestFile(t, root, "NOTICE", "Notice\n")

	if _, err := LoadRepositoryLegalEvidence(root); err == nil {
		t.Fatal("LoadRepositoryLegalEvidence accepted missing THIRD_PARTY_NOTICES.md")
	}

	thirdPartyPath := filepath.Join(root, "THIRD_PARTY_NOTICES.md")
	if err := os.Symlink(filepath.Join(root, "NOTICE"), thirdPartyPath); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRepositoryLegalEvidence(root)
	var inventoryError *InventoryError
	if !errors.As(err, &inventoryError) || inventoryError.Code != "artifact.file.link" {
		t.Fatalf("error = %v, want artifact.file.link", err)
	}
}
