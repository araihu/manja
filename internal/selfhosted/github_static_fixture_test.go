package selfhosted

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/adapters/catalogjson"
)

// Default browser fixtures are local and exportable offline. An explicit
// preview still lets maintainers exercise a complete GitHub catalog.
func staticGitHubPreview(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping static GitHub browser acceptance in short mode")
	}
	if output := os.Getenv("MANJA_GITHUB_MARKDOWN_PREVIEW"); output != "" {
		return output
	}
	source, err := filepath.Abs("testdata/github-static")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	config := fmt.Sprintf(`version: 1
dataDir: data
catalogs:
  - id: github
    mount: /catalogs/github
    title: GitHub
    defaultDocument: github-v3-rest
    profile: strict-v1
    source:
      kind: files
      root: %q
      include: [github-v3-rest.json]
`, source)
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/"}); err != nil {
		t.Fatal(err)
	}
	return output
}

// Detail IDs are content identities. Resolve them from the exported directory
// by the upstream operationId, so a fixture update cannot silently break a URL.
func staticGitHubOperation(t *testing.T, output, operationID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(output, exportManifestPath))
	if err != nil {
		t.Fatal(err)
	}
	var manifest exportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Catalogs {
		if entry.CatalogID != "github" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(output, strings.Trim(entry.Mount, "/"), "snapshots", entry.SnapshotID, "catalog.json"))
		if err != nil {
			t.Fatal(err)
		}
		directory, err := catalogjson.DecodeCatalogWithResourceLimits(data, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, document := range directory.Documents {
			if document.Key != "github-v3-rest" {
				continue
			}
			for _, operation := range document.Operations {
				if operation.OperationID == operationID {
					return string(operation.DetailID)
				}
			}
		}
	}
	t.Fatalf("GitHub static fixture is missing operationId %q", operationID)
	return ""
}

func TestStaticGitHubFixtureIsCompleteAndClosed(t *testing.T) {
	root := filepath.Join("testdata", "github-static")
	receiptBytes, err := os.ReadFile(filepath.Join(root, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		CommitSHA  string   `json:"commitSha"`
		Operations []string `json:"operations"`
		Fixture    struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"fixture"`
		License struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"license"`
	}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.CommitSHA != "320307e220815d528224ca891cf8a9e3c0243678" || len(receipt.Operations) != 7 {
		t.Fatal("unexpected upstream fixture selection")
	}
	for _, entry := range []struct{ path, digest string }{{receipt.Fixture.Path, receipt.Fixture.SHA256}, {receipt.License.Path, receipt.License.SHA256}} {
		data, err := os.ReadFile(filepath.Join(root, entry.path))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != entry.digest {
			t.Fatalf("%s differs from provenance", entry.path)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, receipt.Fixture.Path))
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	var visit func(any)
	visit = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if raw, ok := node["$ref"].(string); ok {
				if !strings.HasPrefix(raw, "#/") {
					t.Fatalf("nonlocal fixture reference %q", raw)
				}
				var target any = fixture
				for _, key := range strings.Split(strings.TrimPrefix(raw, "#/"), "/") {
					object, ok := target.(map[string]any)
					if !ok {
						t.Fatalf("unresolved fixture reference %q", raw)
					}
					target, ok = object[strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")]
					if !ok {
						t.Fatalf("unresolved fixture reference %q", raw)
					}
				}
			}
			for _, child := range node {
				visit(child)
			}
		case []any:
			for _, child := range node {
				visit(child)
			}
		}
	}
	visit(fixture)
	selected := map[string]bool{}
	for _, item := range fixture["paths"].(map[string]any) {
		for _, value := range item.(map[string]any) {
			if operation, ok := value.(map[string]any); ok {
				if id, ok := operation["operationId"].(string); ok {
					selected[id] = true
				}
			}
		}
	}
	if len(selected) != len(receipt.Operations) {
		t.Fatal("unexpected fixture operations")
	}
	for _, id := range receipt.Operations {
		if !selected[id] {
			t.Fatalf("missing fixture operation %q", id)
		}
	}
}
