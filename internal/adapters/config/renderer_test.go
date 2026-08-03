package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/domain"
)

const validRendererConfig = `
version: 1
dataDir: .manja/renderer
catalogs:
  - id: kubernetes
    mount: /kubernetes
    title: Kubernetes
    defaultDocument: core-v1
    profile: kubernetes-v3-v1
    compatibilityAllowlist: allowlists/kubernetes.json
    source:
      kind: files
      root: internal/renderer/testdata/kubernetes/specs
      include:
        - "*_openapi.json"
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
	if runtime.Catalogs[0].ID != "kubernetes" || runtime.Catalogs[0].ProfileID != domain.CompatibilityProfileKubernetes {
		t.Fatalf("Kubernetes runtime catalog = %#v", runtime.Catalogs[0])
	}
	if string(runtime.Catalogs[0].CompatibilityAllowlist) != `{"schemaVersion":1,"diagnostics":[]}` {
		t.Fatalf("Kubernetes allowlist = %q", runtime.Catalogs[0].CompatibilityAllowlist)
	}
	if loaded.Catalogs[0].Source.Root != "internal/renderer/testdata/kubernetes/specs" {
		t.Fatalf("file source = %#v", loaded.Catalogs[0].Source)
	}
	if loaded.Catalogs[1].Source.Repository != "https://example.invalid/payments.git" || loaded.Catalogs[1].Source.Ref != "refs/heads/main" {
		t.Fatalf("Git source = %#v", loaded.Catalogs[1].Source)
	}
	sources := loaded.Sources()
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(sources))
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

func writeRendererConfig(t *testing.T, data string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "renderer.yaml")
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}
