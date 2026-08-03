package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
