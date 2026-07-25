package reviewinput

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/core"
)

const yamlSpec = "openapi: 3.1.0\ninfo:\n  title: Review API\n  version: v1\npaths: {}\n"

func TestLoaderLoadsFileInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.yaml")
	if err := os.WriteFile(path, []byte(yamlSpec), 0o600); err != nil {
		t.Fatal(err)
	}

	file, rev, err := (Loader{}).Load(context.Background(), "docs/openapi.yaml",
		core.ReviewInputLocator{File: path})
	if err != nil {
		t.Fatal(err)
	}
	if file.Path != path || file.Format != "yaml" || string(file.Bytes) != yamlSpec {
		t.Fatalf("file = %#v", file)
	}
	const wantRevisionID = "file-e5e06e44894610be2661039008de62ad987b49ab75e82563669dc88ec0fee9b5"
	if rev.ID != wantRevisionID || rev.Ref != "file" || rev.CommitSHA != "" {
		t.Fatalf("revision = %#v", rev)
	}
}

func TestLoaderLoadsGitRefInput(t *testing.T) {
	repo, commit := initReviewInputGitRepo(t)

	file, rev, err := (Loader{RepoDir: repo}).Load(context.Background(), "docs/openapi.yaml",
		core.ReviewInputLocator{GitRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if file.Path != "docs/openapi.yaml" || file.Format != "yaml" || string(file.Bytes) != yamlSpec {
		t.Fatalf("file = %#v", file)
	}
	if rev.ID != commit || rev.Ref != "HEAD" || rev.CommitSHA != commit {
		t.Fatalf("revision = %#v, want commit %q", rev, commit)
	}
}

func TestLoaderInfersJSONFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, _, err := (Loader{}).Load(context.Background(), "ignored.yaml",
		core.ReviewInputLocator{File: path})
	if err != nil {
		t.Fatal(err)
	}
	if file.Format != "json" {
		t.Fatalf("format = %q", file.Format)
	}
}

func TestLoaderRejectsInvalidInputs(t *testing.T) {
	repo, _ := initReviewInputGitRepo(t)
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	tests := []struct {
		name      string
		specPath  string
		locator   core.ReviewInputLocator
		wantError string
	}{
		{
			name:      "both file and Git ref",
			specPath:  "docs/openapi.yaml",
			locator:   core.ReviewInputLocator{File: missing, GitRef: "HEAD"},
			wantError: "exactly one",
		},
		{
			name:      "neither file nor Git ref",
			specPath:  "docs/openapi.yaml",
			locator:   core.ReviewInputLocator{},
			wantError: "exactly one",
		},
		{
			name:      "absolute Git spec path",
			specPath:  "/docs/openapi.yaml",
			locator:   core.ReviewInputLocator{GitRef: "HEAD"},
			wantError: "unsafe git spec path",
		},
		{
			name:      "parent-traversing Git spec path",
			specPath:  "docs/../../openapi.yaml",
			locator:   core.ReviewInputLocator{GitRef: "HEAD"},
			wantError: "unsafe git spec path",
		},
		{
			name:      "option-like Git ref",
			specPath:  "docs/openapi.yaml",
			locator:   core.ReviewInputLocator{GitRef: "--help"},
			wantError: "unsafe git ref",
		},
		{
			name:      "unknown Git ref",
			specPath:  "docs/openapi.yaml",
			locator:   core.ReviewInputLocator{GitRef: "missing-ref"},
			wantError: "resolve git ref",
		},
		{
			name:      "missing file",
			specPath:  "docs/openapi.yaml",
			locator:   core.ReviewInputLocator{File: missing},
			wantError: "read review input file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := (Loader{RepoDir: repo}).Load(context.Background(), tt.specPath, tt.locator)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}

func initReviewInputGitRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	gitReviewInput(t, repo, "init")
	gitReviewInput(t, repo, "config", "user.email", "manja@example.test")
	gitReviewInput(t, repo, "config", "user.name", "Manja Test")
	path := filepath.Join(repo, "docs", "openapi.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(yamlSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	gitReviewInput(t, repo, "add", "docs/openapi.yaml")
	gitReviewInput(t, repo, "commit", "-m", "add review spec")
	return repo, gitReviewInput(t, repo, "rev-parse", "HEAD")
}

func gitReviewInput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
