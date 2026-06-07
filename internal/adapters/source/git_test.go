package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitSourceFetchesSpecAtRef(t *testing.T) {
	repo := initGitRepo(t)
	writeGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Git API\n  version: v1\npaths: {}\n")
	commit := gitTestOutput(t, repo, "rev-parse", "HEAD")

	src := Git{Repo: repo, Ref: "HEAD", Path: "docs/openapi.yaml"}
	spec, rev, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "docs/openapi.yaml" {
		t.Fatalf("spec path = %q", spec.Path)
	}
	if string(spec.Bytes) == "" || !strings.Contains(string(spec.Bytes), "Git API") {
		t.Fatalf("spec bytes = %q", spec.Bytes)
	}
	if spec.Format != "yaml" {
		t.Fatalf("spec format = %q", spec.Format)
	}
	if rev.Ref != "HEAD" || rev.CommitSHA != commit || rev.ID == "" {
		t.Fatalf("revision = %#v, commit %q", rev, commit)
	}
}

func TestGitSourceReportsMissingSpecAtRef(t *testing.T) {
	repo := initGitRepo(t)
	writeGitFile(t, repo, "openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Git API\n  version: v1\npaths: {}\n")

	src := Git{Repo: repo, Ref: "HEAD", Path: "missing.yaml"}
	_, _, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected missing spec error")
	}
	if !strings.Contains(err.Error(), "missing.yaml") {
		t.Fatalf("error = %v", err)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "manja@example.test")
	git(t, repo, "config", "user.name", "Manja Test")
	return repo
}

func writeGitFile(t *testing.T, repo, name, body string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", name)
	git(t, repo, "commit", "-m", "add spec")
}

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	_ = gitTestOutput(t, repo, args...)
}

func gitTestOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
