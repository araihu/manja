package selfhosted

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/araihu/manja/application/port"
	core "github.com/araihu/manja/domain"
	storeadapter "github.com/araihu/manja/internal/adapters/store"
	"github.com/araihu/manja/internal/web"
)

func TestNewServerRejectsInvalidPublicOriginBeforeSourceAccess(t *testing.T) {
	t.Parallel()

	options := Options{
		SpecPath:     filepath.Join(t.TempDir(), "missing.yaml"),
		PublicOrigin: "http://docs.example.test",
	}

	_, err := NewServer(context.Background(), options)
	if err == nil || !strings.Contains(err.Error(), "public origin") || strings.Contains(err.Error(), "missing.yaml") {
		t.Fatalf("NewServer invalid public origin error = %v, want pre-source validation", err)
	}
}

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
		"request_id": {"selfhosted-publication-token"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publication status = %d", rec.Code)
	}

	state := readOperationalState(t, dataDir)
	rev := onlyRevision(t, state)
	if rev.ID == "" || rev.SourceID != "source1" || rev.Ref != "file" {
		t.Fatalf("revision = %#v", rev)
	}

	pub := publicationForRevision(t, state, rev.ID)
	if pub.ProjectID != "project1" || pub.RevisionID != rev.ID || !pub.Public || pub.Path != "/synced/v1" {
		t.Fatalf("publication = %#v", pub)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/synced/v1", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("published docs status = %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "Synced API", "OpenAPI docs") {
		t.Fatalf("published docs body = %s", body)
	}

	blob, err := storeadapter.NewFileStore(dataDir).Get(ctx, port.BlobKey(rev.SpecBlobKey))
	if err != nil {
		t.Fatal(err)
	}
	if string(blob) == "" || !containsAll(string(blob), "Synced API") {
		t.Fatalf("blob = %q", blob)
	}

	record := onlySyncRecord(t, state)
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

	rev := onlyRevision(t, readOperationalState(t, dataDir))
	if rev.ID == "" || rev.SourceID != repo || rev.Ref != commit || rev.CommitSHA != commit {
		t.Fatalf("revision = %#v, want source %q resolved ref/commit %q", rev, repo, commit)
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
	writeAppGitFile(t, repo, "docs/openapi.yaml", `openapi: 3.1.0
info:
  title: Main API
  version: v1
paths:
  /customers:
    get:
      responses:
        "200":
          description: OK
components:
  schemas:
    Customer:
      type: object
`)
	appGit(t, repo, "tag", "v1.0.0")
	appGit(t, repo, "checkout", "-b", "release/v2")
	writeAppGitFile(t, repo, "docs/openapi.yaml", `openapi: 3.1.0
info:
  title: Release API
  version: v2
paths:
  /payments:
    get:
      responses:
        "200":
          description: OK
components:
  schemas:
    Payment:
      type: object
`)
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
	if body := rec.Body.String(); !containsAll(body, "Main API", "Available refs") || !selectConfigContainsValues(body, "release/v2", "v1.0.0") {
		t.Fatalf("management body = %s", body)
	}

	form := url.Values{
		"ref":        {"release/v2"},
		"publish":    {"public"},
		"path":       {"/release/v2"},
		"request_id": {"selfhosted-release-sync-token"},
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
	if body := rec.Body.String(); !containsAll(body, "Spec diff", "No contract breaks") {
		t.Fatalf("published candidate diff body = %s", body)
	}

	form = url.Values{
		"ref":        {"main"},
		"request_id": {"selfhosted-main-sync-token"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("sync back status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	handler.ServeHTTP(rec, req)
	if body := rec.Body.String(); !containsAll(body, "Main API", "/release/v2", "Removed endpoint", "GET /payments", "Added endpoint", "GET /customers", "Removed schema", "Payment", "Added schema", "Customer") {
		t.Fatalf("candidate diff body = %s", body)
	}

	state := readOperationalState(t, dataDir)
	if len(state.Revisions) != 2 {
		t.Fatalf("revisions = %#v", state.Revisions)
	}
	var releaseRevision core.Revision
	for _, rev := range state.Revisions {
		if rev.CommitSHA == releaseCommit {
			releaseRevision = rev
		}
	}
	if releaseRevision.ID == "" || releaseRevision.CommitSHA != releaseCommit {
		t.Fatalf("release revision = %#v, want commit %q", releaseRevision, releaseCommit)
	}
	pub := publicationForRevision(t, state, releaseRevision.ID)
	if pub.ProjectID != "project1" || pub.RevisionID != releaseRevision.ID || !pub.Public || pub.Path != "/release/v2" {
		t.Fatalf("publication = %#v", pub)
	}
}

func selectConfigContainsValues(body string, values ...string) bool {
	type option struct {
		Value string `json:"value"`
	}
	type config struct {
		Options []option `json:"options"`
	}
	found := make(map[string]bool, len(values))
	for _, match := range regexp.MustCompile(`data-select-config="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		decoded, err := base64.StdEncoding.DecodeString(html.UnescapeString(match[1]))
		if err != nil {
			continue
		}
		var current config
		if err := json.Unmarshal(decoded, &current); err != nil {
			continue
		}
		for _, item := range current.Options {
			found[item.Value] = true
		}
	}
	for _, value := range values {
		if !found[value] {
			return false
		}
	}
	return true
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
		"ref":        {"main"},
		"request_id": {"selfhosted-refresh-sync-token"},
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
	if body := rec.Body.String(); !selectConfigContainsValues(body, "release/v2") {
		t.Fatalf("management body missing refreshed ref:\n%s", body)
	}
}

func TestCandidateMemoryRetainsLatestSuccessfulDiscovery(t *testing.T) {
	startup := []core.RevisionCandidate{{Ref: "main"}}
	refreshed := []core.RevisionCandidate{{Ref: "main"}, {Ref: "release/v2"}}
	memory := newCandidateMemory(startup)
	if got := memory.resolve(refreshed, nil); len(got) != 2 {
		t.Fatalf("successful discovery = %#v", got)
	}
	got := memory.resolve(nil, errors.New("discovery unavailable"))
	if len(got) != 2 || got[1].Ref != "release/v2" {
		t.Fatalf("failure fallback = %#v, want latest successful discovery", got)
	}
}

func TestManagementPublishedIndexLoaderScopesSharedRevisionIDsByContract(t *testing.T) {
	ctx := context.Background()
	store := storeadapter.NewFileStore(t.TempDir())
	saveRevision := func(contractID, revisionID, title string) core.ContractRevision {
		t.Helper()
		spec := []byte("openapi: 3.1.0\ninfo:\n  title: " + title + "\n  version: v1\npaths: {}\n")
		key, err := store.Put(ctx, spec)
		if err != nil {
			t.Fatal(err)
		}
		revision := core.ContractRevision{
			ID: revisionID, ContractID: contractID, SourceID: contractID + "-git",
			Ref: "refs/heads/main", SpecBlobKey: string(key),
		}
		if err := store.SaveRevision(ctx, revision); err != nil {
			t.Fatal(err)
		}
		return revision
	}
	paymentsShared := saveRevision("payments", "shared", "Payments Published")
	ordersShared := saveRevision("orders", "shared", "Orders Published")
	paymentsNext := saveRevision("payments", "payments-next", "Payments Current")
	for _, publication := range []core.Publication{
		{ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments"},
		{ProjectID: "orders", RevisionID: "shared", Public: true, Path: "/orders"},
	} {
		if err := store.SavePublication(ctx, publication); err != nil {
			t.Fatal(err)
		}
	}

	paymentsLoader := managementPublishedIndexLoader(Options{
		ProjectID: "payments",
		SpecPath:  "openapi.yaml",
	}, store)
	t.Run("historical publication loads the owning contract revision", func(t *testing.T) {
		index, ok, err := paymentsLoader(ctx, web.ManagedSpec{
			Project:     core.Project{ID: "payments"},
			Source:      core.Source{ID: "payments-git", ProjectID: "payments", SpecPath: "openapi.yaml"},
			Revision:    paymentsNext,
			Index:       core.SpecIndex{Title: "Payments Current"},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || index.Title != "Payments Published" {
			t.Fatalf("historical published index = (%#v, %v)", index, ok)
		}
	})

	t.Run("same revision id from another contract cannot hit current fast path", func(t *testing.T) {
		index, ok, err := paymentsLoader(ctx, web.ManagedSpec{
			Project:     core.Project{ID: "payments"},
			Source:      core.Source{ID: "payments-git", ProjectID: "payments", SpecPath: "openapi.yaml"},
			Revision:    ordersShared,
			Index:       core.SpecIndex{Title: "Orders Leaked Current"},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || index.Title != "Payments Published" {
			t.Fatalf("cross-contract current fast path leaked: (%#v, %v)", index, ok)
		}
	})

	t.Run("owning contract current fast path remains available", func(t *testing.T) {
		current := core.SpecIndex{
			ProjectID: "payments", RevisionID: "shared",
			Title: "Payments Current Fast Path",
		}
		index, ok, err := paymentsLoader(ctx, web.ManagedSpec{
			Project:     core.Project{ID: "payments"},
			Revision:    paymentsShared,
			Index:       current,
			Publication: core.Publication{ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || index.Title != current.Title {
			t.Fatalf("owning current fast path = (%#v, %v)", index, ok)
		}
	})

	t.Run("mismatched current index loads contract-scoped persisted revision", func(t *testing.T) {
		index, ok, err := paymentsLoader(ctx, web.ManagedSpec{
			Project:  core.Project{ID: "payments"},
			Revision: paymentsShared,
			Index: core.SpecIndex{
				ProjectID: "orders", RevisionID: "shared",
				Title: "Orders Leaked Index",
			},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "shared", Public: true, Path: "/payments"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || index.Title != "Payments Published" {
			t.Fatalf("mismatched current index bypassed contract-scoped read: (%#v, %v)", index, ok)
		}
	})

	t.Run("other contract remains isolated", func(t *testing.T) {
		ordersLoader := managementPublishedIndexLoader(Options{
			ProjectID: "orders",
			SpecPath:  "openapi.yaml",
		}, store)
		index, ok, err := ordersLoader(ctx, web.ManagedSpec{
			Project:     core.Project{ID: "orders"},
			Source:      core.Source{ID: "orders-git", ProjectID: "orders", SpecPath: "openapi.yaml"},
			Revision:    core.ContractRevision{ID: "orders-next", ContractID: "orders"},
			Publication: core.Publication{ProjectID: "orders", RevisionID: "shared", Public: true, Path: "/orders"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || index.Title != "Orders Published" {
			t.Fatalf("orders published index = (%#v, %v)", index, ok)
		}
	})
}

type persistedOperationalState struct {
	Revisions    map[string]core.ContractRevision `json:"revisions"`
	SyncRecords  map[string]core.SyncRecord       `json:"syncRecords"`
	Publications map[string]core.Publication      `json:"publications"`
}

func readOperationalState(t *testing.T, dataDir string) persistedOperationalState {
	t.Helper()
	var state persistedOperationalState
	readJSON(t, filepath.Join(dataDir, "operational", "state.json"), &state)
	return state
}

func onlyRevision(t *testing.T, state persistedOperationalState) core.ContractRevision {
	t.Helper()
	if len(state.Revisions) != 1 {
		t.Fatalf("revisions = %#v", state.Revisions)
	}
	for _, revision := range state.Revisions {
		return revision
	}
	return core.ContractRevision{}
}

func onlySyncRecord(t *testing.T, state persistedOperationalState) core.SyncRecord {
	t.Helper()
	if len(state.SyncRecords) != 1 {
		t.Fatalf("sync records = %#v", state.SyncRecords)
	}
	for _, record := range state.SyncRecords {
		return record
	}
	return core.SyncRecord{}
}

func publicationForRevision(t *testing.T, state persistedOperationalState, revisionID string) core.Publication {
	t.Helper()
	for _, publication := range state.Publications {
		if publication.RevisionID == revisionID {
			return publication
		}
	}
	t.Fatalf("publication for revision %q not found in %#v", revisionID, state.Publications)
	return core.Publication{}
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
	init)
		mkdir -p "$3"
		;;
	fetch)
		:
		;;
	rev-parse)
		case "$*" in
			*:*) printf '%s\n' "def456def456def456def456def456def456def4" ;;
			*) printf '%s\n' "abc123abc123abc123abc123abc123abc123abcd" ;;
		esac
		;;
	cat-file)
		if [ "${2:-}" = "-s" ]; then
			printf '%s\n' "77"
			exit 0
		fi
		cat <<'EOF'
openapi: 3.1.0
info:
  title: Discovery Fallback API
  version: v1
paths: {}
EOF
		;;
	show)
		printf 'Manja Test\000manja@example.test\000fixture\n'
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
