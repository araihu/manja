package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/domain"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
)

const validRendererConfig = `
version: 1
dataDir: .manja/renderer
organization:
  title: Example APIs
  readme: API documentation published by Example.
  license:
    name: Apache-2.0
    url: https://example.test/license
  sources:
    - name: Public definitions
      kind: git
      location: github.com/example/apis
      url: https://github.com/example/apis
  seo:
    description: Example API documentation.
    canonicalBase: https://example.test
    socialImage: https://example.test/social.png
    socialImageAlt: Example APIs
catalogs:
  - id: kubernetes
    mount: /kubernetes
    title: Kubernetes
    readme: Kubernetes API documentation.
    license:
      name: Apache-2.0
      url: https://example.test/kubernetes-license
    defaultDocument: core-v1
    profile: kubernetes-v3-v1
    compatibilityAllowlist: allowlists/kubernetes.json
    seo:
      description: Browse Kubernetes APIs.
      canonicalBase: https://docs.example.test/kubernetes
      socialImage: https://docs.example.test/social.png
      socialImageAlt: Kubernetes API reference
    source:
      kind: files
      root: internal/renderer/testdata/kubernetes/specs
      include:
        - "*_openapi.json"
      documents:
        - path: api__v1_openapi.json
          key: core-v1
  - id: payments
    mount: /payments
    title: Payments
    profile: strict-v1
    source:
      kind: git
      repository: https://example.invalid/payments.git
      ref: refs/heads/main
      include:
        - openapi.yaml
`

const validConfiguredGitIntegrityReceipt = `{
  "schemaVersion": 2,
  "catalogId": "payments",
  "cloneRepository": "/missing/payments.git",
  "provenanceUrl": "https://example.test/payments",
  "objectFormat": "sha1",
  "sourceRoot": ".",
  "commitObjectId": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "treeObjectId": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "artifacts": [{
    "path": "openapi.json",
    "mode": "100644",
    "size": 3,
    "gitObjectId": "cccccccccccccccccccccccccccccccccccccccc",
    "sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
  }]
}`

func TestLoadRendererBuildsRuntimeAndSourceConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "allowlists"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "allowlists", "kubernetes.json"), []byte(`{"schemaVersion":1,"diagnostics":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(filename, []byte(validRendererConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRenderer(filename)
	if err != nil {
		t.Fatal(err)
	}
	runtime := loaded.RuntimeConfig()
	if runtime.Version != 1 || runtime.DataDir != filepath.Join(root, ".manja/renderer") || len(runtime.Catalogs) != 2 {
		t.Fatalf("runtime config = %#v", runtime)
	}
	if runtime.Organization.Title != "Example APIs" || runtime.Organization.License.Name != "Apache-2.0" || len(runtime.Organization.Sources) != 1 || runtime.Organization.Sources[0].Location != "github.com/example/apis" {
		t.Fatalf("organization config = %#v", runtime.Organization)
	}
	if runtime.Catalogs[0].ID != "kubernetes" || runtime.Catalogs[0].ProfileID != domain.CompatibilityProfileKubernetes {
		t.Fatalf("Kubernetes runtime catalog = %#v", runtime.Catalogs[0])
	}
	if runtime.Catalogs[0].Readme != "Kubernetes API documentation." || runtime.Catalogs[0].License.Name != "Apache-2.0" {
		t.Fatalf("Kubernetes catalog metadata = %#v", runtime.Catalogs[0])
	}
	if runtime.Catalogs[0].SEO.CanonicalBase != "https://docs.example.test/kubernetes" || runtime.Catalogs[0].SEO.SocialImageAlt != "Kubernetes API reference" {
		t.Fatalf("Kubernetes SEO config = %#v", runtime.Catalogs[0].SEO)
	}
	if string(runtime.Catalogs[0].CompatibilityAllowlist) != `{"schemaVersion":1,"diagnostics":[]}` {
		t.Fatalf("Kubernetes allowlist = %q", runtime.Catalogs[0].CompatibilityAllowlist)
	}
	if loaded.Catalogs[0].Source.Root != "internal/renderer/testdata/kubernetes/specs" {
		t.Fatalf("file source = %#v", loaded.Catalogs[0].Source)
	}
	if len(loaded.Catalogs[0].Source.Documents) != 1 || loaded.Catalogs[0].Source.Documents[0].Key != "core-v1" {
		t.Fatalf("file source document mappings = %#v", loaded.Catalogs[0].Source.Documents)
	}
	if loaded.Catalogs[1].Source.Repository != "https://example.invalid/payments.git" || loaded.Catalogs[1].Source.Ref != "refs/heads/main" {
		t.Fatalf("Git source = %#v", loaded.Catalogs[1].Source)
	}
	sources := loaded.Sources()
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(sources))
	}
	fileSource, ok := sources[0].(sourceadapter.FileCatalogSource)
	if !ok || len(fileSource.Manifest.DocumentKeys) != 1 || fileSource.Manifest.DocumentKeys[0].SourcePath != "api__v1_openapi.json" || fileSource.Manifest.DocumentKeys[0].Key != "core-v1" {
		t.Fatalf("translated file source = %#v", sources[0])
	}
}

func TestLoadRendererRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	t.Parallel()

	for name, data := range map[string]string{
		"unknown field":      validRendererConfig + "unknown: rejected\n",
		"multiple documents": validRendererConfig + "---\nversion: 1\ncatalogs: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := LoadRenderer(writeRendererConfig(t, data)); err == nil {
				t.Fatal("invalid renderer YAML was accepted")
			}
		})
	}
}

func TestLoadRendererRejectsInvalidSourceConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "unknown kind", data: strings.Replace(validRendererConfig, "kind: files", "kind: network", 1)},
		{name: "file root", data: strings.Replace(validRendererConfig, "root: internal/renderer/testdata/kubernetes/specs", "root: \"\"", 1)},
		{name: "file include", data: strings.Replace(validRendererConfig, "      include:\n        - \"*_openapi.json\"", "      include: []", 1)},
		{name: "file repository", data: strings.Replace(validRendererConfig, "      root: internal/renderer/testdata/kubernetes/specs", "      root: internal/renderer/testdata/kubernetes/specs\n      repository: forbidden", 1)},
		{name: "git repository", data: strings.Replace(validRendererConfig, "repository: https://example.invalid/payments.git", "repository: \"\"", 1)},
		{name: "git ref", data: strings.Replace(validRendererConfig, "ref: refs/heads/main", "ref: \"\"", 1)},
		{name: "git credentials", data: strings.Replace(validRendererConfig, "https://example.invalid/payments.git", "https://user:password@example.invalid/payments.git", 1)},
		{name: "git root", data: strings.Replace(validRendererConfig, "      repository: https://example.invalid/payments.git", "      root: forbidden\n      repository: https://example.invalid/payments.git", 1)},
		{name: "runtime mount", data: strings.Replace(validRendererConfig, "mount: /payments", "mount: /kubernetes/payments", 1)},
		{name: "duplicate document path", data: strings.Replace(validRendererConfig, "          key: core-v1", "          key: core-v1\n        - path: api__v1_openapi.json\n          key: core-copy", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := LoadRenderer(writeRendererConfig(t, test.data)); err == nil {
				t.Fatal("invalid renderer source configuration was accepted")
			}
		})
	}
}

func TestLoadRendererAdmitsCanonicalGitIntegrityReceiptOnly(t *testing.T) {
	t.Parallel()

	withReceipt := strings.Replace(
		validRendererConfig,
		"      ref: refs/heads/main\n",
		"      ref: refs/heads/main\n      integrityReceipt: payments.provenance.json\n",
		1,
	)
	if _, err := LoadRenderer(writeRendererConfigWithAllowlist(t, withReceipt)); err != nil {
		t.Fatalf("canonical Git integrity receipt rejected: %v", err)
	}

	for name, data := range map[string]string{
		"file source": strings.Replace(
			validRendererConfig,
			"      root: internal/renderer/testdata/kubernetes/specs\n",
			"      root: internal/renderer/testdata/kubernetes/specs\n      integrityReceipt: kubernetes.provenance.json\n",
			1,
		),
		"empty":                  strings.Replace(withReceipt, "payments.provenance.json", `""`, 1),
		"whitespace only":        strings.Replace(withReceipt, "payments.provenance.json", `"   "`, 1),
		"surrounding whitespace": strings.Replace(withReceipt, "payments.provenance.json", `" payments.provenance.json "`, 1),
		"absolute":               strings.Replace(withReceipt, "payments.provenance.json", "/tmp/payments.provenance.json", 1),
		"Windows drive":          strings.Replace(withReceipt, "payments.provenance.json", `"C:/receipts/payments.provenance.json"`, 1),
		"UNC":                    strings.Replace(withReceipt, "payments.provenance.json", `"//server/receipts/payments.provenance.json"`, 1),
		"escape":                 strings.Replace(withReceipt, "payments.provenance.json", "../payments.provenance.json", 1),
		"backslash": strings.Replace(
			withReceipt,
			"payments.provenance.json",
			`receipts\payments.provenance.json`,
			1,
		),
		"non-canonical": strings.Replace(withReceipt, "payments.provenance.json", "receipts/../payments.provenance.json", 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := LoadRenderer(writeRendererConfigWithAllowlist(t, data)); err == nil {
				t.Fatal("invalid integrity receipt path was accepted")
			}
		})
	}
}

func TestConfiguredGitIntegrityReceiptIsRejectedBeforeRepositoryAcquisition(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		receipt string
		want    string
	}{
		"unknown field": {
			receipt: strings.Replace(validConfiguredGitIntegrityReceipt, "\n}", ",\n  \"unexpected\": true\n}", 1),
			want:    "decode Git integrity receipt",
		},
		"trailing value": {
			receipt: validConfiguredGitIntegrityReceipt + "\n{}\n",
			want:    "must contain exactly one JSON value",
		},
		"oversized": {
			receipt: strings.Repeat(" ", 65537),
			want:    "exceeds 65536 bytes",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			repository := filepath.Join(root, "missing.git")
			receipt := strings.Replace(test.receipt, "/missing/payments.git", repository, 1)
			if err := os.WriteFile(filepath.Join(root, "receipt.json"), []byte(receipt), 0o600); err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf(`
version: 1
catalogs:
  - id: payments
    mount: /
    title: Payments
    profile: strict-v1
    source:
      kind: git
      repository: %q
      ref: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      integrityReceipt: receipt.json
      include:
        - openapi.json
      documents:
        - path: openapi.json
          key: payments-v1
`, repository)
			configPath := filepath.Join(root, "renderer.yaml")
			if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadRenderer(configPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = loaded.Sources()[0].Load(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("configured integrity receipt error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestConfiguredGitIntegrityReceiptRequiresRegularNonSymlinkFileUnderRendererRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named-pipe fixture requires a Unix-like host")
	}

	tests := []struct {
		name       string
		receipt    string
		setup      func(*testing.T, string, string)
		wantUnsafe bool
	}{
		{name: "regular file", receipt: "receipt.json", setup: func(t *testing.T, root, _ string) {
			writeConfiguredGitIntegrityReceipt(t, filepath.Join(root, "receipt.json"), root)
		}},
		{name: "leaf symlink", receipt: "receipt.json", wantUnsafe: true, setup: func(t *testing.T, root, outside string) {
			target := filepath.Join(outside, "receipt.json")
			writeConfiguredGitIntegrityReceipt(t, target, root)
			if err := os.Symlink(target, filepath.Join(root, "receipt.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "parent symlink", receipt: "receipts/receipt.json", wantUnsafe: true, setup: func(t *testing.T, root, outside string) {
			writeConfiguredGitIntegrityReceipt(t, filepath.Join(outside, "receipt.json"), root)
			if err := os.Symlink(outside, filepath.Join(root, "receipts")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory", receipt: "receipt.json", wantUnsafe: true, setup: func(t *testing.T, root, _ string) {
			if err := os.Mkdir(filepath.Join(root, "receipt.json"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "FIFO", receipt: "receipt.json", wantUnsafe: true, setup: func(t *testing.T, root, _ string) {
			if output, err := exec.Command("mkfifo", filepath.Join(root, "receipt.json")).CombinedOutput(); err != nil {
				t.Fatalf("mkfifo: %v: %s", err, output)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			test.setup(t, root, outside)
			repository := filepath.Join(root, "missing.git")
			configPath := writeConfiguredGitIntegrityRenderer(t, root, repository, test.receipt)
			loaded, err := LoadRenderer(configPath)
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				_, loadErr := loaded.Sources()[0].Load(context.Background())
				result <- loadErr
			}()
			select {
			case err := <-result:
				if !test.wantUnsafe {
					if err == nil || errors.Is(err, sourceadapter.ErrCatalogIntegrity) {
						t.Fatalf("regular receipt error = %v, want later Git acquisition error", err)
					}
					return
				}
				var integrityErr *sourceadapter.CatalogIntegrityError
				if !errors.As(err, &integrityErr) || integrityErr.Check != "receipt-file" {
					t.Fatalf("unsafe receipt error = %#v, want receipt-file integrity error", err)
				}
			case <-time.After(time.Second):
				t.Fatal("unsafe receipt read blocked instead of failing closed")
			}
		})
	}
}

func writeConfiguredGitIntegrityReceipt(t *testing.T, filename, rendererRoot string) {
	t.Helper()
	repository := filepath.Join(rendererRoot, "missing.git")
	receipt := strings.Replace(validConfiguredGitIntegrityReceipt, "/missing/payments.git", repository, 1)
	if err := os.WriteFile(filename, []byte(receipt), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeConfiguredGitIntegrityRenderer(t *testing.T, root, repository, receipt string) string {
	t.Helper()
	config := fmt.Sprintf(`
version: 1
catalogs:
  - id: payments
    mount: /
    title: Payments
    profile: strict-v1
    source:
      kind: git
      repository: %q
      ref: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      integrityReceipt: %q
      include:
        - openapi.json
      documents:
        - path: openapi.json
          key: payments-v1
`, repository, receipt)
	filename := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(filename, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestCommittedRendererConfigLoads(t *testing.T) {
	t.Parallel()

	loaded, err := LoadRenderer("testdata/renderer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Catalogs) != 1 || loaded.Catalogs[0].ID != "payments" {
		t.Fatalf("committed renderer config = %#v", loaded)
	}
}

func TestCommittedKubernetesRendererConfigUsesAuthorityDocumentKeys(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("..", "..", "renderer", "testdata", "kubernetes", "renderer.yaml")
	loaded, err := LoadRenderer(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Catalogs) != 3 {
		t.Fatalf("Kubernetes renderer catalog count = %d, want 3", len(loaded.Catalogs))
	}
	catalogIndex := make(map[string]int, len(loaded.Catalogs))
	for index, catalog := range loaded.Catalogs {
		catalogIndex[catalog.ID] = index
	}
	for id, mount := range map[string]string{
		"kubernetes": "/catalogs/kubernetes",
		"github":     "/catalogs/github",
		"stripe":     "/catalogs/stripe",
	} {
		index, exists := catalogIndex[id]
		if !exists || loaded.Catalogs[index].Mount != mount {
			t.Fatalf("catalog %q mount = %#v, want %q", id, loaded.Catalogs[index], mount)
		}
		localDocs := loaded.RuntimeConfig().Catalogs[index].LocalDocs
		if !localDocs.Public || !localDocs.Anonymous || localDocs.PublicationKey != id {
			t.Fatalf("catalog %q local docs authority = %#v", id, localDocs)
		}
	}
	kubernetesIndex := catalogIndex["kubernetes"]
	kubernetes := loaded.Catalogs[kubernetesIndex]
	if kubernetes.DefaultDocumentKey != "core-v1" || len(kubernetes.Source.Documents) != 65 {
		t.Fatalf("Kubernetes renderer config = %#v", kubernetes)
	}
	runtime := loaded.RuntimeConfig()
	if runtime.Catalogs[kubernetesIndex].SEO.CanonicalBase != "https://manja.araihu.com/catalogs/kubernetes" {
		t.Fatalf("Kubernetes canonical base = %q", runtime.Catalogs[kubernetesIndex].SEO.CanonicalBase)
	}
	if runtime.Organization.Title != "Manja" || len(runtime.Organization.Sources) != 4 {
		t.Fatalf("Kubernetes organization config = %#v", runtime.Organization)
	}
	candidate, err := loaded.Sources()[kubernetesIndex].Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]struct{}, len(candidate.Documents))
	for _, document := range candidate.Documents {
		keys[document.Key] = struct{}{}
	}
	for _, key := range []string{"core-v1", "apps-v1", "storage-v1", "resource-v1"} {
		if _, exists := keys[key]; !exists {
			t.Errorf("Kubernetes renderer source missing authority key %q", key)
		}
	}
}

func writeRendererConfig(t *testing.T, data string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "renderer.yaml")
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func writeRendererConfigWithAllowlist(t *testing.T, data string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "allowlists"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "allowlists", "kubernetes.json"), []byte(`{"schemaVersion":1,"diagnostics":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}
