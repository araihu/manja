package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/contracttest"
	"github.com/araihu/manja/domain"
)

func TestGitSourcePublicContract(t *testing.T) {
	contracttest.SourceFetcher(t, func(t testing.TB) contracttest.SourceFixture {
		repo := initGitRepo(t)
		const specPath = "docs/openapi.yaml"
		data := []byte("openapi: 3.1.0\ninfo:\n  title: Contract\n  version: v1\npaths: {}\n")
		writeGitFile(t, repo, specPath, string(data))
		commit := gitTestOutput(t, repo, "rev-parse", "HEAD")
		digest := sha256.Sum256([]byte(commit + ":" + specPath))
		return contracttest.SourceFixture{
			Fetcher:  Git{Repo: repo, Ref: "HEAD", Path: specPath},
			WantSpec: domain.SpecFile{SourceID: repo, Path: specPath, Format: "yaml", Bytes: data},
			WantRevision: domain.ContractRevision{
				ID: "git-" + hex.EncodeToString(digest[:])[:16], SourceID: repo, Ref: "HEAD", CommitSHA: commit,
				AuthorName: "Manja Test", AuthorEmail: "manja@example.test", Message: "add spec",
			},
			WantCandidates: []domain.RevisionCandidate{
				{SourceID: repo, Ref: "main", Kind: "branch", CommitSHA: commit, AuthorName: "Manja Test", AuthorEmail: "manja@example.test", Message: "add spec"},
			},
		}
	})
}

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
	if rev.AuthorName != "Manja Test" || rev.AuthorEmail != "manja@example.test" || rev.Message != "add spec" {
		t.Fatalf("revision author metadata = %#v", rev)
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

func TestGitSourceDoesNotLeakHTTPSTokenInCloneErrors(t *testing.T) {
	src := Git{
		Repo:     "http://127.0.0.1:1/manja/missing.git",
		Ref:      "main",
		Path:     "openapi.yaml",
		Username: "manja",
		Token:    "super-secret-token",
	}

	_, _, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected clone error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestGitSourceFetchesSpecFromLocalBareRepositoryRefs(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Branch API\n  version: v1\npaths: {}\n")
	branchCommit := gitTestOutput(t, worktree, "rev-parse", "HEAD")
	git(t, worktree, "tag", "v1")

	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Commit API\n  version: v2\npaths: {}\n")
	headCommit := gitTestOutput(t, worktree, "rev-parse", "HEAD")

	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)

	tests := []struct {
		name       string
		ref        string
		wantTitle  string
		wantCommit string
	}{
		{name: "branch", ref: "main", wantTitle: "Commit API", wantCommit: headCommit},
		{name: "tag", ref: "v1", wantTitle: "Branch API", wantCommit: branchCommit},
		{name: "commit", ref: branchCommit, wantTitle: "Branch API", wantCommit: branchCommit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := Git{Repo: bare, Ref: tt.ref, Path: "docs/openapi.yaml"}
			spec, rev, err := src.Fetch(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(spec.Bytes), tt.wantTitle) {
				t.Fatalf("spec bytes = %q", spec.Bytes)
			}
			if rev.Ref != tt.ref || rev.CommitSHA != tt.wantCommit || rev.ID == "" {
				t.Fatalf("revision = %#v, want commit %q", rev, tt.wantCommit)
			}
		})
	}
}

func TestGitSourceDiscoversBranchAndTagRefs(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Main API\n  version: v1\npaths: {}\n")
	mainCommit := gitTestOutput(t, worktree, "rev-parse", "HEAD")
	git(t, worktree, "tag", "v1.0.0")
	git(t, worktree, "tag", "-a", "v1.0.1", "-m", "release v1.0.1")
	git(t, worktree, "checkout", "-b", "release/v2")
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Release API\n  version: v2\npaths: {}\n")
	releaseCommit := gitTestOutput(t, worktree, "rev-parse", "HEAD")

	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)

	src := Git{Repo: bare, Path: "docs/openapi.yaml"}
	candidates, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		kind   string
		commit string
	}{
		"main":       {kind: "branch", commit: mainCommit},
		"release/v2": {kind: "branch", commit: releaseCommit},
		"v1.0.0":     {kind: "tag", commit: mainCommit},
		"v1.0.1":     {kind: "tag", commit: mainCommit},
	}
	if len(candidates) != len(want) {
		t.Fatalf("candidates = %#v, want %d", candidates, len(want))
	}
	for _, candidate := range candidates {
		expected, ok := want[candidate.Ref]
		if !ok {
			t.Fatalf("unexpected candidate %#v", candidate)
		}
		if candidate.SourceID != bare || candidate.Kind != expected.kind || candidate.CommitSHA != expected.commit {
			t.Fatalf("candidate = %#v, want kind %q commit %q source %q", candidate, expected.kind, expected.commit, bare)
		}
		if candidate.AuthorName != "Manja Test" || candidate.AuthorEmail != "manja@example.test" || candidate.Message == "" {
			t.Fatalf("candidate author metadata = %#v", candidate)
		}
	}
}

func TestGitSourceDiscoversRemoteBranchesWithoutRemoteName(t *testing.T) {
	repo := initGitRepo(t)
	writeGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Main API\n  version: v1\npaths: {}\n")
	commit := gitTestOutput(t, repo, "rev-parse", "HEAD")
	git(t, repo, "update-ref", "refs/remotes/upstream/feature/remote", commit)

	src := Git{Repo: repo, Path: "docs/openapi.yaml"}
	candidates, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, candidate := range candidates {
		if candidate.Ref == "upstream/feature/remote" {
			t.Fatalf("remote branch should be normalized without remote name: %#v", candidates)
		}
		if candidate.Ref == "feature/remote" && candidate.Kind == "branch" && candidate.CommitSHA == commit {
			return
		}
	}
	t.Fatalf("missing normalized feature/remote branch in candidates %#v", candidates)
}

func TestGitSourceDiscoversRemoteBranchesFromClonedRepository(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Main API\n  version: v1\npaths: {}\n")
	mainCommit := gitTestOutput(t, worktree, "rev-parse", "HEAD")
	git(t, worktree, "checkout", "-b", "feature/ref-discovery")
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Feature API\n  version: v2\npaths: {}\n")
	featureCommit := gitTestOutput(t, worktree, "rev-parse", "HEAD")

	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)

	src := Git{Repo: "file://" + bare, Path: "docs/openapi.yaml"}
	candidates, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	counts := map[string]int{}
	for _, candidate := range candidates {
		if candidate.Kind == "branch" {
			got[candidate.Ref] = candidate.CommitSHA
			counts[candidate.Ref]++
		}
	}
	if got["main"] != mainCommit || got["feature/ref-discovery"] != featureCommit {
		t.Fatalf("branch candidates = %#v, want main %q and feature/ref-discovery %q", got, mainCommit, featureCommit)
	}
	for ref, count := range counts {
		if count != 1 {
			t.Fatalf("branch %q appeared %d times in candidates %#v", ref, count, candidates)
		}
	}
	if _, found := got["origin/HEAD"]; found {
		t.Fatalf("branch candidates should not include origin/HEAD: %#v", got)
	}
}

func initGitRepo(t testing.TB) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "manja@example.test")
	git(t, repo, "config", "user.name", "Manja Test")
	return repo
}

func writeGitFile(t testing.TB, repo, name, body string) {
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

func git(t testing.TB, repo string, args ...string) {
	t.Helper()
	_ = gitTestOutput(t, repo, args...)
}

func gitTestOutput(t testing.TB, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
