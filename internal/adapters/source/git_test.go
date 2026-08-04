package source

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
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

func TestGitSourceRevisionIdentityCoalescesAliasesAndSeparatesCommitOrPath(t *testing.T) {
	repo := initGitRepo(t)
	writeGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Git API\n  version: v1\npaths: {}\n")
	writeGitFile(t, repo, "docs/other.yaml", "openapi: 3.1.0\ninfo:\n  title: Other API\n  version: v1\npaths: {}\n")
	firstCommit := gitTestOutput(t, repo, "rev-parse", "HEAD")
	git(t, repo, "tag", "v1")

	fetch := func(ref, path string) domain.ContractRevision {
		t.Helper()
		_, revision, err := (Git{Repo: repo, Ref: ref, Path: path}).Fetch(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return revision
	}
	head := fetch("HEAD", "docs/openapi.yaml")
	branch := fetch("main", "docs/openapi.yaml")
	tag := fetch("v1", "docs/openapi.yaml")
	otherPath := fetch("HEAD", "docs/other.yaml")
	if head.CommitSHA != firstCommit || head.ID != branch.ID || head.ID != tag.ID {
		t.Fatalf("same commit/path aliases produced different revisions: HEAD=%#v branch=%#v tag=%#v", head, branch, tag)
	}
	if otherPath.ID == head.ID {
		t.Fatalf("different spec paths reused revision id %q", head.ID)
	}

	writeGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Git API\n  version: v2\npaths: {}\n")
	next := fetch("main", "docs/openapi.yaml")
	if next.CommitSHA == firstCommit || next.ID == head.ID {
		t.Fatalf("different commit reused revision identity: first=%#v next=%#v", head, next)
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

func TestGitSourceRemoteFetchUsesBoundedExactRefAcquisition(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Remote API\n  version: v1\npaths: {}\n")
	writeGitFile(t, worktree, "unrelated.txt", strings.Repeat("x", 9<<20))
	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)
	git(t, bare, "config", "uploadpack.allowFilter", "true")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shimDirectory := t.TempDir()
	logPath := filepath.Join(shimDirectory, "invocations.log")
	shim := "#!/bin/sh\nprintf 'no-lazy=%s args=%s\\n' \"$GIT_NO_LAZY_FETCH\" \"$*\" >> \"$MANJA_GIT_INVOCATIONS\"\nexec \"$MANJA_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDirectory, "git"), []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MANJA_REAL_GIT", realGit)
	t.Setenv("MANJA_GIT_INVOCATIONS", logPath)
	t.Setenv("PATH", shimDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	spec, _, err := (Git{
		Repo: (&url.URL{Scheme: "file", Path: bare}).String(), Ref: "main", Path: "docs/openapi.yaml",
	}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec.Bytes), "Remote API") {
		t.Fatalf("remote spec = %q", spec.Bytes)
	}
	invocations, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(invocations)
	if strings.Contains(log, " clone ") || !strings.Contains(log, "fetch --quiet --depth=1 --filter=blob:limit=8388609 --no-tags") {
		t.Fatalf("remote fetch acquisition was not shallow, filtered, and exact-ref:\n%s", log)
	}
	for _, invocation := range strings.Split(log, "\n") {
		if strings.Contains(invocation, "cat-file") && !strings.HasPrefix(invocation, "no-lazy=1 ") {
			t.Fatalf("remote spec object access allowed lazy fetch: %q", invocation)
		}
	}
}

func TestGitSourceRemoteFetchRejectsOversizedSelectedBlobBeforeRead(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", strings.Repeat("x", (8<<20)+1))
	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)
	git(t, bare, "config", "uploadpack.allowFilter", "true")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shimDirectory := t.TempDir()
	logPath := filepath.Join(shimDirectory, "invocations.log")
	shim := "#!/bin/sh\nprintf 'no-lazy=%s args=%s\\n' \"$GIT_NO_LAZY_FETCH\" \"$*\" >> \"$MANJA_GIT_INVOCATIONS\"\nexec \"$MANJA_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDirectory, "git"), []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MANJA_REAL_GIT", realGit)
	t.Setenv("MANJA_GIT_INVOCATIONS", logPath)
	t.Setenv("PATH", shimDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	remote := (&url.URL{Scheme: "file", Path: bare}).String()
	_, _, err = (Git{Repo: remote, Ref: "main", Path: "docs/openapi.yaml"}).Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeds 8388608 bytes") {
		t.Fatalf("oversized remote spec error = %v", err)
	}
	invocations, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	log := string(invocations)
	if !strings.Contains(log, "no-lazy=1 args=-C") || !strings.Contains(log, "cat-file -s") || strings.Contains(log, "cat-file blob") {
		t.Fatalf("oversized selected blob access was not bounded and no-lazy:\n%s", log)
	}
}

func TestGitSourceRemoteDiscoveryUsesBoundedMetadataAcquisition(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Main API\n  version: v1\npaths: {}\n")
	git(t, worktree, "checkout", "-b", "feature/discovery")
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Feature API\n  version: v2\npaths: {}\n")
	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)
	git(t, bare, "config", "uploadpack.allowFilter", "true")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shimDirectory := t.TempDir()
	logPath := filepath.Join(shimDirectory, "invocations.log")
	shim := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MANJA_GIT_INVOCATIONS\"\nexec \"$MANJA_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDirectory, "git"), []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MANJA_REAL_GIT", realGit)
	t.Setenv("MANJA_GIT_INVOCATIONS", logPath)
	t.Setenv("PATH", shimDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	candidates, err := (Git{Repo: (&url.URL{Scheme: "file", Path: bare}).String(), Path: "docs/openapi.yaml"}).Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("remote candidates = %#v", candidates)
	}
	invocations, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(invocations)
	if strings.Contains(log, " clone ") || !strings.Contains(log, "fetch --quiet --depth=1 --filter=blob:none --no-tags") {
		t.Fatalf("remote discovery acquisition was not shallow and metadata-only:\n%s", log)
	}
}

func TestGitSourceRemoteAcquisitionRejectsRepositoryPastDiskLimit(t *testing.T) {
	worktree := initGitRepo(t)
	writeGitFile(t, worktree, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Bounded API\n  version: v1\npaths: {}\n")
	oversizedPath := filepath.Join(worktree, "incompressible.bin")
	file, err := os.OpenFile(oversizedPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(file, rand.Reader, int64(maxGitRepositoryBytes+(8<<20))); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	git(t, worktree, "add", "incompressible.bin")
	git(t, worktree, "commit", "-m", "oversized repository")
	bare := filepath.Join(t.TempDir(), "repo.git")
	git(t, worktree, "clone", "--bare", ".", bare)
	git(t, bare, "config", "uploadpack.allowFilter", "false")
	remote := (&url.URL{Scheme: "file", Path: bare}).String()

	for name, run := range map[string]func() error{
		"fetch": func() error {
			_, _, err := (Git{Repo: remote, Ref: "main", Path: "docs/openapi.yaml"}).Fetch(context.Background())
			return err
		},
		"discover": func() error {
			_, err := (Git{Repo: remote, Path: "docs/openapi.yaml"}).Discover(context.Background())
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			checkoutRoot := t.TempDir()
			t.Setenv("TMPDIR", checkoutRoot)
			err := run()
			if !errors.Is(err, errGitRepositoryLimit) {
				t.Fatalf("over-limit remote acquisition error = %v, want %v", err, errGitRepositoryLimit)
			}
			entries, readErr := os.ReadDir(checkoutRoot)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("failed acquisition retained checkout entries: %v", entries)
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
