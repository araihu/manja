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

func TestLoaderRejectsAmbiguousGitRef(t *testing.T) {
	repo, commit := initReviewInputGitRepo(t)
	gitReviewInput(t, repo, "branch", "collision", commit)
	gitReviewInput(t, repo, "tag", "collision", commit)

	_, _, err := (Loader{RepoDir: repo}).Load(context.Background(), "docs/openapi.yaml",
		core.ReviewInputLocator{GitRef: "collision"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous git ref") {
		t.Fatalf("error = %v, want ambiguous git ref", err)
	}
}

func TestLoaderReadsGitSpecFromResolvedCommitWhenRefMoves(t *testing.T) {
	repo, resolvedCommit := initReviewInputGitRepo(t)
	const movedSpec = "openapi: 3.1.0\ninfo:\n  title: Moved Ref API\n  version: v2\npaths: {}\n"
	writeReviewInputGitSpec(t, repo, movedSpec, "move review spec")
	movedCommit := gitReviewInput(t, repo, "rev-parse", "HEAD")
	gitReviewInput(t, repo, "branch", "moving", resolvedCommit)
	installMovingRefGitWrapper(t, movedCommit)

	file, rev, err := (Loader{RepoDir: repo}).Load(context.Background(), "docs/openapi.yaml",
		core.ReviewInputLocator{GitRef: "moving"})
	if err != nil {
		t.Fatal(err)
	}
	if string(file.Bytes) != yamlSpec {
		t.Fatalf("spec bytes = %q, want bytes from resolved commit", file.Bytes)
	}
	if rev.ID != resolvedCommit || rev.CommitSHA != resolvedCommit || rev.Ref != "moving" {
		t.Fatalf("revision = %#v, want resolved commit %q and ref moving", rev, resolvedCommit)
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
			name:      "missing Git tree spec",
			specPath:  "docs/missing.yaml",
			locator:   core.ReviewInputLocator{GitRef: "HEAD"},
			wantError: "read git spec",
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
	writeReviewInputGitSpec(t, repo, yamlSpec, "add review spec")
	return repo, gitReviewInput(t, repo, "rev-parse", "HEAD")
}

func writeReviewInputGitSpec(t *testing.T, repo, body, message string) {
	t.Helper()
	path := filepath.Join(repo, "docs", "openapi.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	gitReviewInput(t, repo, "add", "docs/openapi.yaml")
	gitReviewInput(t, repo, "commit", "-m", message)
}

func installMovingRefGitWrapper(t *testing.T, movedCommit string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	wrapperDir := t.TempDir()
	wrapperPath := filepath.Join(wrapperDir, "git")
	const wrapper = `#!/bin/sh
if [ "$3" = "rev-parse" ]; then
  "$MANJA_TEST_REAL_GIT" "$@"
  status=$?
  if [ "$status" -eq 0 ]; then
    "$MANJA_TEST_REAL_GIT" -C "$2" update-ref refs/heads/moving "$MANJA_TEST_MOVED_COMMIT"
  fi
  exit "$status"
fi
exec "$MANJA_TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MANJA_TEST_REAL_GIT", realGit)
	t.Setenv("MANJA_TEST_MOVED_COMMIT", movedCommit)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
