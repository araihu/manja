package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/core"
)

func TestNewWithOptionsSyncsSpecBeforeServingPublicDocs(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	specPath := filepath.Join(dataDir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte("openapi: 3.1.0\ninfo:\n  title: Synced API\n  version: v1\npaths: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	handler, err := NewWithOptions(ctx, Options{
		ProjectID: "project1",
		SourceID:  "source1",
		SpecPath:  specPath,
		DataDir:   dataDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Synced API", "OpenAPI docs") {
		t.Fatalf("body = %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("management status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Management", "Synced API", "source1", "startup", "success") {
		t.Fatalf("management body = %s", body)
	}

	form := url.Values{
		"visibility": {"public"},
		"path":       {"/synced/v1"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publication status = %d", rec.Code)
	}

	revisions := entries(t, filepath.Join(dataDir, "revisions"))
	if len(revisions) != 1 {
		t.Fatalf("revisions = %#v", revisions)
	}
	var rev core.Revision
	readJSON(t, filepath.Join(dataDir, "revisions", revisions[0]), &rev)
	if rev.ID == "" || rev.SourceID != "source1" || rev.Ref != "file" {
		t.Fatalf("revision = %#v", rev)
	}

	var pub core.Publication
	readJSON(t, filepath.Join(dataDir, "publications", "project1-"+rev.ID+".json"), &pub)
	if pub.ProjectID != "project1" || pub.RevisionID != rev.ID || !pub.Public || pub.Path != "/synced/v1" {
		t.Fatalf("publication = %#v", pub)
	}

	blobPath := filepath.Join(dataDir, "blobs", "specs", rev.ID+".yaml")
	blob, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(blob) == "" || !containsAll(string(blob), "Synced API") {
		t.Fatalf("blob = %q", blob)
	}

	records := entries(t, filepath.Join(dataDir, "sync-history"))
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	var record core.SyncRecord
	readJSON(t, filepath.Join(dataDir, "sync-history", records[0]), &record)
	if record.Result != core.SyncResultSuccess || record.ProjectID != "project1" || record.SourceID != "source1" || record.RevisionID != rev.ID {
		t.Fatalf("sync record = %#v", record)
	}
}

func TestNewWithOptionsSyncsGitSourceBeforeServingPublicDocs(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	repo := initAppGitRepo(t)
	writeAppGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Git Synced API\n  version: v2\npaths: {}\n")
	commit := appGitOutput(t, repo, "rev-parse", "HEAD")

	handler, err := NewWithOptions(ctx, Options{
		ProjectID:  "project1",
		SourceKind: "git",
		GitRepo:    repo,
		GitRef:     "main",
		SpecPath:   "docs/openapi.yaml",
		DataDir:    dataDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Git Synced API", "OpenAPI docs") {
		t.Fatalf("body = %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("management status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Management", "Git Synced API", repo, "git", "docs/openapi.yaml", "main", commit, "success") {
		t.Fatalf("management body = %s", body)
	}

	revisions := entries(t, filepath.Join(dataDir, "revisions"))
	if len(revisions) != 1 {
		t.Fatalf("revisions = %#v", revisions)
	}
	var rev core.Revision
	readJSON(t, filepath.Join(dataDir, "revisions", revisions[0]), &rev)
	if rev.ID == "" || rev.SourceID != repo || rev.Ref != "main" || rev.CommitSHA != commit {
		t.Fatalf("revision = %#v, want source %q ref main commit %q", rev, repo, commit)
	}
}

func TestNewWithOptionsShowsGitDiscoveryErrorsInManagementState(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	installFailingDiscoveryGit(t)

	handler, err := NewWithOptions(ctx, Options{
		ProjectID:  "project1",
		SourceKind: "git",
		GitRepo:    "https://example.test/repo.git",
		GitRef:     "main",
		SpecPath:   "docs/openapi.yaml",
		DataDir:    dataDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public docs status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Discovery Fallback API", "OpenAPI docs") {
		t.Fatalf("public docs body = %s", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("management status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Discovery Fallback API", "failure", "discover refs failed") {
		t.Fatalf("management body = %s", body)
	}
}

func TestNewWithOptionsManagesGitRefDiscoveryAndSyncPublication(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	repo := initAppGitRepo(t)
	writeAppGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Main API\n  version: v1\npaths: {}\n")
	appGit(t, repo, "tag", "v1.0.0")
	appGit(t, repo, "checkout", "-b", "release/v2")
	writeAppGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Release API\n  version: v2\npaths: {}\n")
	releaseCommit := appGitOutput(t, repo, "rev-parse", "HEAD")
	appGit(t, repo, "checkout", "main")

	handler, err := NewWithOptions(ctx, Options{
		ProjectID:  "project1",
		SourceKind: "git",
		GitRepo:    repo,
		GitRef:     "main",
		SpecPath:   "docs/openapi.yaml",
		DataDir:    dataDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("management status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Main API", "Available refs", "release/v2", "v1.0.0") {
		t.Fatalf("management body = %s", body)
	}

	form := url.Values{
		"ref":     {"release/v2"},
		"publish": {"public"},
		"path":    {"/release/v2"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("sync status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if body := rec.Body.String(); !containsAll(body, "Release API", "release/v2", releaseCommit, "/release/v2", "Public") {
		t.Fatalf("updated management body = %s", body)
	}

	revisions := entries(t, filepath.Join(dataDir, "revisions"))
	if len(revisions) != 2 {
		t.Fatalf("revisions = %#v", revisions)
	}
	var releaseRevision core.Revision
	for _, name := range revisions {
		var rev core.Revision
		readJSON(t, filepath.Join(dataDir, "revisions", name), &rev)
		if rev.Ref == "release/v2" {
			releaseRevision = rev
		}
	}
	if releaseRevision.ID == "" || releaseRevision.CommitSHA != releaseCommit {
		t.Fatalf("release revision = %#v, want commit %q", releaseRevision, releaseCommit)
	}
	var pub core.Publication
	readJSON(t, filepath.Join(dataDir, "publications", "project1-"+releaseRevision.ID+".json"), &pub)
	if pub.ProjectID != "project1" || pub.RevisionID != releaseRevision.ID || !pub.Public || pub.Path != "/release/v2" {
		t.Fatalf("publication = %#v", pub)
	}
}

func TestNewWithOptionsRefreshesGitCandidatesAfterManualSync(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	repo := initAppGitRepo(t)
	writeAppGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Main API\n  version: v1\npaths: {}\n")

	handler, err := NewWithOptions(ctx, Options{
		ProjectID:  "project1",
		SourceKind: "git",
		GitRepo:    repo,
		GitRef:     "main",
		SpecPath:   "docs/openapi.yaml",
		DataDir:    dataDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	appGit(t, repo, "checkout", "-b", "release/v2")
	writeAppGitFile(t, repo, "docs/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Release API\n  version: v2\npaths: {}\n")
	appGit(t, repo, "checkout", "main")

	form := url.Values{
		"ref": {"main"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("sync status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if body := rec.Body.String(); !strings.Contains(body, "release/v2") {
		t.Fatalf("management body missing refreshed ref:\n%s", body)
	}
}

func entries(t *testing.T, path string) []string {
	t.Helper()
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(dirEntries))
	for _, entry := range dirEntries {
		names = append(names, entry.Name())
	}
	return names
}

func readJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}

func containsAll(body string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(body, want) {
			return false
		}
	}
	return true
}

func initAppGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	appGit(t, repo, "init", "-b", "main")
	appGit(t, repo, "config", "user.email", "manja@example.test")
	appGit(t, repo, "config", "user.name", "Manja Test")
	return repo
}

func writeAppGitFile(t *testing.T, repo, name, body string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	appGit(t, repo, "add", name)
	appGit(t, repo, "commit", "-m", "add spec")
}

func installFailingDiscoveryGit(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	script := `#!/bin/sh
set -eu
if [ "${1:-}" = "-C" ]; then
	shift 2
fi
case "${1:-}" in
	clone)
		mkdir -p "$5"
		;;
	rev-parse)
		printf '%s\n' "abc123abc123abc123abc123abc123abc123abcd"
		;;
	show)
		cat <<'EOF'
openapi: 3.1.0
info:
  title: Discovery Fallback API
  version: v1
paths: {}
EOF
		;;
	for-each-ref)
		printf '%s\n' "discover refs failed" >&2
		exit 2
		;;
	*)
		printf '%s\n' "unexpected git command: $*" >&2
		exit 2
		;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func appGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	_ = appGitOutput(t, repo, args...)
}

func appGitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
