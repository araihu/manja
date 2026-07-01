package main

import "testing"

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
