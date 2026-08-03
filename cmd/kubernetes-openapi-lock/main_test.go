package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const pinnedKubernetesCommit = "a818af18fe29d999d6741234c8cd72709ef2f424"

func TestDocumentKeyMapsEverySupportedUpstreamShape(t *testing.T) {
	t.Parallel()

	for filename, want := range map[string]string{
		".well-known__openid-configuration_openapi.json":      "well-known-openid-configuration",
		"api__v1_openapi.json":                                "core-v1",
		"api_openapi.json":                                    "core-discovery",
		"apis__apps__v1_openapi.json":                         "apps-v1",
		"apis__apps_openapi.json":                             "apps-discovery",
		"apis__flowcontrol.apiserver.k8s.io__v1_openapi.json": "flowcontrol-apiserver-v1",
		"apis__rbac.authorization.k8s.io__v1_openapi.json":    "rbac-authorization-v1",
		"apis_openapi.json":                                   "apis-discovery",
		"logs_openapi.json":                                   "logs",
		"openid__v1__jwks_openapi.json":                       "openid-v1-jwks",
		"version_openapi.json":                                "version",
	} {
		t.Run(filename, func(t *testing.T) {
			t.Parallel()
			got, err := documentKey(filename)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("documentKey(%q) = %q, want %q", filename, got, want)
			}
		})
	}

	for _, filename := range []string{"README.md", "apis__apps__v1.json", "apis____v1_openapi.json"} {
		if _, err := documentKey(filename); err == nil {
			t.Fatalf("unsupported filename %q was accepted", filename)
		}
	}
}

func TestResponseSizeCeilingUsesSmallestDeclaredBoundary(t *testing.T) {
	t.Parallel()

	for size, want := range map[int64]string{
		1:             "4KiB",
		4 << 10:       "4KiB",
		(4 << 10) + 1: "16KiB",
		16 << 10:      "16KiB",
		64 << 10:      "64KiB",
		256 << 10:     "256KiB",
		1 << 20:       "1MiB",
		(1 << 20) + 1: "4MiB",
		4 << 20:       "4MiB",
	} {
		if got, err := responseSizeCeiling(size); err != nil || got != want {
			t.Fatalf("responseSizeCeiling(%d) = %q, %v; want %q", size, got, err, want)
		}
	}
	for _, size := range []int64{0, -1, (4 << 20) + 1} {
		if _, err := responseSizeCeiling(size); err == nil {
			t.Fatalf("responseSizeCeiling(%d) succeeded", size)
		}
	}
}

func TestParseInventoryRejectsMalformedOrNonfileRows(t *testing.T) {
	t.Parallel()

	valid := "api__v1_openapi.json\tfile\t2\t9e26dfeeb6e641a33dae4961196235bdb965b21b\n"
	entries, err := parseInventory(strings.NewReader(valid))
	if err != nil || len(entries) != 1 || entries[0].Name != "api__v1_openapi.json" {
		t.Fatalf("parseInventory = %#v, %v", entries, err)
	}
	for name, input := range map[string]string{
		"columns": "api__v1_openapi.json\tfile\t2\n",
		"type":    "api__v1_openapi.json\tdir\t2\t9e26dfeeb6e641a33dae4961196235bdb965b21b\n",
		"size":    "api__v1_openapi.json\tfile\tinvalid\t9e26dfeeb6e641a33dae4961196235bdb965b21b\n",
		"blob":    "api__v1_openapi.json\tfile\t2\tnot-a-blob\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseInventory(strings.NewReader(input)); err == nil {
				t.Fatal("malformed inventory was accepted")
			}
		})
	}
}

func TestBuildSourceCatalogRequiresExactUnique65DocumentInventory(t *testing.T) {
	t.Parallel()

	entries := fakeInventory(65)
	catalog, err := buildSourceCatalog(pinnedKubernetesCommit, entries, inventoryEntry{
		Name: "LICENSE", Type: "file", Size: 11358, GitBlobSHA: "d645695673349e3947e8e5ae42332d0ac3164cd7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Documents) != 65 || catalog.CommitSHA != pinnedKubernetesCommit {
		t.Fatalf("catalog = %#v", catalog)
	}

	mutations := []struct {
		name    string
		entries []inventoryEntry
	}{
		{name: "removed", entries: append([]inventoryEntry(nil), entries[:64]...)},
		{name: "added", entries: append(append([]inventoryEntry(nil), entries...), inventoryEntry{Name: "apis__extra.k8s.io__v1_openapi.json", Type: "file", Size: 2, GitBlobSHA: strings.Repeat("a", 40)})},
		{name: "duplicate", entries: append(append([]inventoryEntry(nil), entries[:64]...), entries[0])},
		{name: "unknown shape", entries: append(append([]inventoryEntry(nil), entries[:64]...), inventoryEntry{Name: "unknown.json", Type: "file", Size: 2, GitBlobSHA: strings.Repeat("a", 40)})},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()
			if _, err := buildSourceCatalog(pinnedKubernetesCommit, mutation.entries, catalog.License.inventory()); err == nil {
				t.Fatal("invalid inventory was accepted")
			}
		})
	}
	if _, err := buildSourceCatalog("master", entries, catalog.License.inventory()); err == nil {
		t.Fatal("mutable commit was accepted")
	}
}

func TestGitBlobVerificationDetectsChangedBytes(t *testing.T) {
	t.Parallel()

	const blob = "9e26dfeeb6e641a33dae4961196235bdb965b21b"
	if err := verifyGitBlob(blob, []byte("{}")); err != nil {
		t.Fatalf("unchanged Git blob: %v", err)
	}
	if err := verifyGitBlob(blob, []byte("{ }")); err == nil {
		t.Fatal("changed Git blob was accepted")
	}
}

func TestCommittedCatalogSourceAndMuambaResourceCoverPinnedInventory(t *testing.T) {
	root := repositoryRoot(t)
	catalog, catalogBytes, err := loadSourceCatalog(filepath.Join(root, "internal/renderer/testdata/kubernetes/catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	if catalog.SchemaVersion != 1 || catalog.KeyAlgorithm != "kubernetes-openapi-v3-key-v1" || catalog.CommitSHA != pinnedKubernetesCommit {
		t.Fatalf("catalog authority = %#v", catalog)
	}
	if len(catalog.Documents) != 65 {
		t.Fatalf("document count = %d, want 65", len(catalog.Documents))
	}
	if got := sumSourceBytes(catalog.Documents); got != 12625155 {
		t.Fatalf("source bytes = %d, want 12625155", got)
	}
	if !sort.SliceIsSorted(catalog.Documents, func(i, j int) bool { return catalog.Documents[i].UpstreamPath < catalog.Documents[j].UpstreamPath }) {
		t.Fatal("catalog documents are not sorted by upstream path")
	}
	if encoded, err := encodeSourceCatalog(catalog); err != nil || !bytes.Equal(encoded, catalogBytes) {
		t.Fatalf("catalog bytes are not canonical: %v", err)
	}

	resource := readMuambaResource(t, filepath.Join(root, "muamba.yaml"))
	if resource.Version != pinnedKubernetesCommit || len(resource.Downloads) != 66 {
		t.Fatalf("Muamba resource version/downloads = %q/%d", resource.Version, len(resource.Downloads))
	}
	for _, document := range catalog.Documents {
		download, exists := resource.Downloads[document.Key]
		if !exists {
			t.Fatalf("Muamba download missing for %s", document.Key)
		}
		wantURL := "https://raw.githubusercontent.com/kubernetes/kubernetes/" + pinnedKubernetesCommit + "/" + document.UpstreamPath
		wantPath := "internal/renderer/testdata/kubernetes/specs/" + filepath.Base(document.UpstreamPath)
		wantMax, err := responseSizeCeiling(document.Size)
		if err != nil {
			t.Fatal(err)
		}
		if download.URL != wantURL || download.Path != wantPath || download.MaxSize != wantMax || !strings.HasPrefix(download.Integrity, "sha384-") {
			t.Fatalf("Muamba download %s = %#v", document.Key, download)
		}
	}
	if resource.Downloads["license"].Path != "internal/renderer/testdata/kubernetes/LICENSE" || !strings.HasPrefix(resource.Downloads["license"].Integrity, "sha384-") {
		t.Fatalf("Muamba license = %#v", resource.Downloads["license"])
	}
}

func TestGeneratedMuambaResourcePreservesMatchingIntegrityLocks(t *testing.T) {
	root := repositoryRoot(t)
	catalog, _, err := loadSourceCatalog(filepath.Join(root, "internal/renderer/testdata/kubernetes/catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "muamba.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	regenerated, err := replaceGeneratedMuambaResource(manifest, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(regenerated, manifest) {
		t.Fatal("unchanged generated resource lost reviewed Muamba integrity locks")
	}
}

func TestCommittedReceiptMatchesEveryLockedByteAndPinnedCounts(t *testing.T) {
	root := repositoryRoot(t)
	catalog, _, err := loadSourceCatalog(filepath.Join(root, "internal/renderer/testdata/kubernetes/catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := buildReceipt(
		catalog,
		filepath.Join(root, "internal/renderer/testdata/kubernetes/specs"),
		filepath.Join(root, "internal/renderer/testdata/kubernetes/LICENSE"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.DocumentCount != 65 || receipt.TotalBytes != 12625155 || receipt.OperationCount != 1202 || receipt.DocumentSchemaCount != 1826 || receipt.UniqueSchemaCount != 862 {
		t.Fatalf("receipt totals = documents:%d bytes:%d operations:%d schemas:%d unique:%d",
			receipt.DocumentCount, receipt.TotalBytes, receipt.OperationCount, receipt.DocumentSchemaCount, receipt.UniqueSchemaCount)
	}
	encoded, err := encodeReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join(root, "internal/renderer/testdata/kubernetes/receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, committed) {
		t.Fatal("committed receipt differs from locked-byte regeneration")
	}
}

func TestReceiptGenerationIsByteIdenticalAcrossDirectories(t *testing.T) {
	root := repositoryRoot(t)
	catalog, _, err := loadSourceCatalog(filepath.Join(root, "internal/renderer/testdata/kubernetes/catalog-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	first := copyLockedCatalog(t, root, catalog)
	second := copyLockedCatalog(t, root, catalog)
	firstReceipt, err := buildReceipt(catalog, filepath.Join(first, "specs"), filepath.Join(first, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	secondReceipt, err := buildReceipt(catalog, filepath.Join(second, "specs"), filepath.Join(second, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := encodeReceipt(firstReceipt)
	secondBytes, _ := encodeReceipt(secondReceipt)
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("receipt depends on absolute directory")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run code = %d, want 2", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("stdout/stderr = %q / %q", stdout.String(), stderr.String())
	}
}

func fakeInventory(count int) []inventoryEntry {
	result := make([]inventoryEntry, count)
	for index := range result {
		result[index] = inventoryEntry{
			Name: fmt.Sprintf("apis__group%02d.k8s.io__v1_openapi.json", index),
			Type: "file", Size: 2, GitBlobSHA: strings.Repeat("a", 40),
		}
	}
	return result
}

func sumSourceBytes(documents []sourceDocument) int64 {
	var total int64
	for _, document := range documents {
		total += document.Size
	}
	return total
}

type parsedMuamba struct {
	Resources map[string]parsedMuambaResource `yaml:"resources"`
}

type parsedMuambaResource struct {
	Version   string                          `yaml:"version"`
	Downloads map[string]parsedMuambaDownload `yaml:"downloads"`
}

type parsedMuambaDownload struct {
	URL       string `yaml:"url"`
	Path      string `yaml:"path"`
	MaxSize   string `yaml:"max_size"`
	Integrity string `yaml:"integrity"`
}

func readMuambaResource(t *testing.T, filename string) parsedMuambaResource {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var manifest parsedMuamba
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	resource, exists := manifest.Resources["kubernetes-openapi-v3"]
	if !exists {
		t.Fatal("Muamba Kubernetes resource missing")
	}
	return resource
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func copyLockedCatalog(t *testing.T, root string, catalog sourceCatalog) string {
	t.Helper()
	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(target, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, document := range catalog.Documents {
		data, err := os.ReadFile(filepath.Join(root, "internal/renderer/testdata/kubernetes/specs", filepath.Base(document.UpstreamPath)))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "specs", filepath.Base(document.UpstreamPath)), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	license, err := os.ReadFile(filepath.Join(root, "internal/renderer/testdata/kubernetes/LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "LICENSE"), license, 0o600); err != nil {
		t.Fatal(err)
	}
	return target
}

func decodeReceipt(t *testing.T, data []byte) receipt {
	t.Helper()
	var value receipt
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
