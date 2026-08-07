package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if len(loaded.Catalogs) != 1 || loaded.Catalogs[0].DefaultDocumentKey != "core-v1" || len(loaded.Catalogs[0].Source.Documents) != 65 {
		t.Fatalf("Kubernetes renderer config = %#v", loaded.Catalogs)
	}
	if loaded.RuntimeConfig().Catalogs[0].SEO.CanonicalBase != "https://manja.araihu.com/catalogs/kubernetes" {
		t.Fatalf("Kubernetes canonical base = %q", loaded.RuntimeConfig().Catalogs[0].SEO.CanonicalBase)
	}
	if loaded.RuntimeConfig().Organization.Title != "Manja" || len(loaded.RuntimeConfig().Organization.Sources) != 2 {
		t.Fatalf("Kubernetes organization config = %#v", loaded.RuntimeConfig().Organization)
	}
	candidate, err := loaded.Sources()[0].Load(context.Background())
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
