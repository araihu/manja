package source

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCatalogGitSourcePinsEveryReadToResolvedCommit(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	writeCatalogFile(t, repository, "specs/openapi.json", `{"revision":"first"}`)
	runGitTestCommand(t, repository, "add", "specs/openapi.json")
	runGitTestCommand(t, repository, "commit", "-qm", "first")
	first := runGitTestCommand(t, repository, "rev-parse", "HEAD")

	source := GitCatalogSource{
		Repository: repository, Ref: "HEAD", Root: "specs", Manifest: testCatalogManifest("strict-v1", "*.json"),
	}
	source.afterResolve = func(commit string) {
		if commit != first {
			t.Fatalf("resolved commit = %q, want %q", commit, first)
		}
		writeCatalogFile(t, repository, "specs/openapi.json", `{"revision":"later"}`)
		runGitTestCommand(t, repository, "add", "specs/openapi.json")
		runGitTestCommand(t, repository, "commit", "-qm", "later")
	}
	candidate, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Revision.CommitSHA != first || string(candidate.Documents[0].Bytes) != `{"revision":"first"}` {
		t.Fatalf("candidate revision/bytes = %q / %q", candidate.Revision.CommitSHA, candidate.Documents[0].Bytes)
	}
	if strings.TrimSpace(runGitTestCommand(t, repository, "status", "--porcelain")) != "" {
		t.Fatal("Git catalog acquisition dirtied the repository")
	}
}

func runGitTestCommand(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestCatalogGitSourceUsesRootRelativeSlashPaths(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	writeCatalogFile(t, repository, "nested/specs/openapi.yaml", "openapi: 3.0.0\n")
	runGitTestCommand(t, repository, "add", ".")
	runGitTestCommand(t, repository, "commit", "-qm", "fixture")

	candidate, err := (GitCatalogSource{
		Repository: repository, Ref: "HEAD", Root: filepath.ToSlash("nested/specs"),
		Manifest: testCatalogManifest("strict-v1", "*.yaml"),
	}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Documents) != 1 || candidate.Documents[0].SourcePath != "openapi.yaml" || candidate.Documents[0].Key != "openapi" {
		t.Fatalf("documents = %#v", candidate.Documents)
	}
}

func TestCatalogGitSourceRecursesBelowRepositoryRoot(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	writeCatalogFile(t, repository, "nested/specs/groups/apps/openapi.yaml", "openapi: 3.0.0\ninfo:\n  title: Nested\n  version: v1\npaths: {}\n")
	runGitTestCommand(t, repository, "add", ".")
	runGitTestCommand(t, repository, "commit", "-qm", "nested fixture")

	candidate, err := (GitCatalogSource{
		Repository: repository, Ref: "HEAD",
		Manifest: testCatalogManifest("strict-v1", "nested/*/*/*/*.yaml"),
	}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Documents) != 1 || candidate.Documents[0].SourcePath != "nested/specs/groups/apps/openapi.yaml" {
		t.Fatalf("nested documents = %#v", candidate.Documents)
	}
}

func TestCatalogGitSourceRejectsSymlinkEntries(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	if err := os.Symlink("../../outside.json", filepath.Join(repository, "openapi.json")); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repository, "add", "openapi.json")
	runGitTestCommand(t, repository, "commit", "-qm", "symlink")
	if _, err := (GitCatalogSource{
		Repository: repository, Ref: "HEAD", Manifest: testCatalogManifest("strict-v1", "*.json"),
	}).Load(context.Background()); err == nil {
		t.Fatal("Git symlink entry was accepted")
	}
}

func TestCatalogGitInventoryRejectsTreeOverEntryBound(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	writeCatalogFile(t, repository, "openapi.json", `{"openapi":"3.0.3","info":{"title":"Bounded","version":"v1"},"paths":{}}`)
	for index := 0; index < maxCatalogInventoryEntries; index++ {
		writeCatalogFile(t, repository, fmt.Sprintf("extra/%04d.txt", index), "x")
	}
	runGitTestCommand(t, repository, "add", ".")
	runGitTestCommand(t, repository, "commit", "-qm", "large tree")

	_, err := (GitCatalogSource{
		Repository: repository, Ref: "HEAD", Manifest: testCatalogManifest("strict-v1", "*.json"),
	}).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "inventory") {
		t.Fatalf("large Git tree error = %v", err)
	}
}

func TestGitCommandOutputIsBounded(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	command := exec.Command("git", "-C", repository, "hash-object", "-w", "--stdin")
	command.Stdin = strings.NewReader(strings.Repeat("x", 4096))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	objectID := strings.TrimSpace(string(output))
	_, err = gitOutputBytesLimit(context.Background(), repository, 1024, "cat-file", "blob", objectID)
	if !errors.Is(err, errGitOutputLimit) {
		t.Fatalf("bounded Git output error = %v, want %v", err, errGitOutputLimit)
	}
}

func TestCatalogGitSourceRejectsOversizedBlobBeforeReadingIt(t *testing.T) {
	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	runGitTestCommand(t, repository, "config", "user.name", "Test")
	runGitTestCommand(t, repository, "config", "user.email", "test@example.com")
	oversized := filepath.Join(repository, "openapi.json")
	file, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxCatalogSourceFileBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repository, "add", "openapi.json")
	runGitTestCommand(t, repository, "commit", "-qm", "oversized blob")
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

	_, err = (GitCatalogSource{
		Repository: repository, Ref: "HEAD", Manifest: testCatalogManifest("strict-v1", "*.json"),
	}).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeds 8388608 bytes") {
		t.Fatalf("oversized Git blob error = %v", err)
	}
	invocations, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(invocations), "cat-file -s") || strings.Contains(string(invocations), "cat-file blob") {
		t.Fatalf("oversized Git blob invocations = %q", invocations)
	}
}

func TestGitCatalogRemoteAcquisitionIsShallowAndPartial(t *testing.T) {
	t.Parallel()

	worktree := t.TempDir()
	runGitTestCommand(t, worktree, "init", "-q", "-b", "main")
	runGitTestCommand(t, worktree, "config", "user.name", "Test")
	runGitTestCommand(t, worktree, "config", "user.email", "test@example.com")
	for revision := 0; revision < 4; revision++ {
		writeCatalogFile(t, worktree, "history.txt", strings.Repeat(string(rune('a'+revision)), 1<<20))
		writeCatalogFile(t, worktree, "openapi.json", fmt.Sprintf(`{"openapi":"3.0.3","info":{"title":"Remote","version":"v%d"},"paths":{}}`, revision))
		runGitTestCommand(t, worktree, "add", ".")
		runGitTestCommand(t, worktree, "commit", "-qm", fmt.Sprintf("revision %d", revision))
	}
	bare := filepath.Join(t.TempDir(), "catalog.git")
	runGitTestCommand(t, worktree, "clone", "--bare", ".", bare)
	runGitTestCommand(t, bare, "config", "uploadpack.allowFilter", "true")
	remote := (&url.URL{Scheme: "file", Path: bare}).String()

	repository, resolvedRef, cleanup, err := gitCatalogRepository(context.Background(), remote, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if resolvedRef != "FETCH_HEAD" {
		t.Fatalf("resolved ref = %q, want FETCH_HEAD", resolvedRef)
	}
	if shallow := runGitTestCommand(t, repository, "rev-parse", "--is-shallow-repository"); shallow != "true" {
		t.Fatalf("remote acquisition shallow = %q, want true", shallow)
	}
	if commits := runGitTestCommand(t, repository, "rev-list", "--count", resolvedRef); commits != "1" {
		t.Fatalf("remote acquisition commits = %q, want 1", commits)
	}
	objects := runGitTestCommand(t, repository, "rev-list", "--objects", "--missing=print", resolvedRef)
	if !strings.Contains(objects, "?") {
		t.Fatalf("remote acquisition materialized every blob:\n%s", objects)
	}
	exceeded, err := directoryExceeds(repository, maxGitRepositoryBytes)
	if err != nil || exceeded {
		t.Fatalf("remote acquisition disk budget exceeded=%v error=%v", exceeded, err)
	}
}

func TestGitCommandAbortsWhenRepositoryDiskBudgetIsExceeded(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runGitTestCommand(t, repository, "init", "-q")
	oversized := filepath.Join(repository, "oversized.pack")
	file, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(int64(maxGitRepositoryBytes + 1)); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "daemon", "--verbose", "--export-all", "--base-path="+repository, "--listen=127.0.0.1", "--port=0", repository)
	err = runGitCommand(ctx, command, repository)
	if !errors.Is(err, errGitRepositoryLimit) {
		t.Fatalf("repository disk error = %v, want %v", err, errGitRepositoryLimit)
	}
}
