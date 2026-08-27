package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	core "github.com/araihu/manja/domain"
	app "github.com/araihu/manja/internal/selfhosted"
	"github.com/araihu/manja/renderer"
)

func TestBuildCommandWritesDeterministicSnapshotReceipt(t *testing.T) {
	originalBuild := buildRenderer
	t.Cleanup(func() { buildRenderer = originalBuild })
	buildRenderer = func(context.Context, app.RendererOptions) ([]renderer.ActivationReceipt, error) {
		return []renderer.ActivationReceipt{{CatalogID: "kubernetes", Mount: "/", RevisionID: "files-sha256-a", SnapshotID: "snapshot-sha256-b"}}, nil
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"build", "-renderer-config", "renderer.yaml", "-data-dir", "/out/catalog"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("build code = %d stderr=%q", code, stderr.String())
	}
	want := "{\"schemaVersion\":1,\"catalogs\":[{\"catalogId\":\"kubernetes\",\"mount\":\"/\",\"revisionId\":\"files-sha256-a\",\"snapshotId\":\"snapshot-sha256-b\"}]}\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q, want %q and empty", stdout.String(), stderr.String(), want)
	}
}

func TestBuildCommandRequiresExplicitRendererInputs(t *testing.T) {
	for _, args := range [][]string{{"build"}, {"build", "-renderer-config", "renderer.yaml"}, {"build", "-data-dir", "/out/catalog"}} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%v) code = %d, want 2; stderr=%q", args, code, stderr.String())
		}
	}
}

func TestRunServerPreservesExistingFlagsAndStartsServer(t *testing.T) {
	originalServe := serve
	t.Cleanup(func() { serve = originalServe })

	var got cliConfig
	serve = func(_ context.Context, cfg cliConfig) error {
		got = cfg
		return nil
	}
	args := []string{
		"-addr", ":9090",
		"-source", app.SourceKindGit,
		"-spec", "docs/openapi.yaml",
		"-git-repo", "https://example.test/acme/payments.git",
		"-git-ref", "release/v2",
		"-data-dir", "/tmp/manja-data",
		"-public-origin", "https://docs.example.test",
		"-endpoint-sidebar-label", string(app.EndpointSidebarLabelPath),
		"-brand-name", "Acme",
		"-brand-logo", "https://example.test/logo.svg",
		"-brand-logo-alt", "Acme logo",
		"-brand-home-url", "https://example.test",
		"-brand-favicon", "https://example.test/favicon.ico",
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%q", code, stderr.String())
	}
	want := cliConfig{
		Addr: ":9090",
		Options: app.Options{
			SourceKind:           app.SourceKindGit,
			SpecPath:             "docs/openapi.yaml",
			GitRepo:              "https://example.test/acme/payments.git",
			GitRef:               "release/v2",
			DataDir:              "/tmp/manja-data",
			PublicOrigin:         "https://docs.example.test",
			EndpointSidebarLabel: app.EndpointSidebarLabelPath,
			Branding: core.DocsBranding{
				DisplayName: "Acme",
				Logo: core.DocsBrandingLogo{
					Src:     "https://example.test/logo.svg",
					Alt:     "Acme logo",
					HomeURL: "https://example.test",
				},
				Favicon: "https://example.test/favicon.ico",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serve config = %#v, want %#v", got, want)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q, want empty", stdout.String(), stderr.String())
	}
}

func TestRunServerPreservesFailureExitCode(t *testing.T) {
	originalServe := serve
	t.Cleanup(func() { serve = originalServe })
	serve = func(context.Context, cliConfig) error {
		return errors.New("listen failed")
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, &stdout, &stderr); code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "listen failed") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDefaultSpecPathUsesGitHubRESTFixture(t *testing.T) {
	want := "internal/adapters/openapi/testdata/github-v3-rest.json"
	if got := defaultSpecPath(); got != want {
		t.Fatalf("defaultSpecPath() = %q, want %q", got, want)
	}
}

func TestOptionsFromArgsBuildsGitSourceOptions(t *testing.T) {
	opts, err := optionsFromArgs([]string{
		"-source", "git",
		"-git-repo", "https://example.test/acme/payments.git",
		"-git-ref", "release/v2",
		"-spec", "docs/openapi.yaml",
		"-data-dir", "/tmp/manja-data",
	})
	if err != nil {
		t.Fatal(err)
	}

	if opts.SourceKind != "git" || opts.GitRepo != "https://example.test/acme/payments.git" || opts.GitRef != "release/v2" {
		t.Fatalf("git source opts = %#v", opts)
	}
	if opts.SpecPath != "docs/openapi.yaml" || opts.DataDir != "/tmp/manja-data" {
		t.Fatalf("path opts = %#v", opts)
	}
}

func TestConfigFromArgsSelectsRendererOnlyConfig(t *testing.T) {
	cfg, err := configFromArgs([]string{
		"-renderer-config", "configs/renderer.yaml",
		"-data-dir", "/tmp/manja-renderer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RendererConfig != "configs/renderer.yaml" || cfg.Options.DataDir != "/tmp/manja-renderer" {
		t.Fatalf("renderer config = %#v", cfg)
	}
}
