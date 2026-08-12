//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/forgejo"
)

const forgejoImage = "codeberg.org/forgejo/forgejo:11@sha256:946243edbab116d5bb78b73ea68af6f3d69229ba1b1ed958dd82c3481167f3e0"

func TestForgejoContainerStarts(t *testing.T) {
	ctx := context.Background()
	fixture := acquireForgejo(t, ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fixture.baseURL+"/api/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Forgejo health status = %d", resp.StatusCode)
	}
}

func TestGitSourceFetchesSpecFromPublicForgejoRepository(t *testing.T) {
	ctx := context.Background()
	fixture := acquireForgejo(t, ctx)
	baseURL := fixture.baseURL
	repoName := "openapi-docs"
	createForgejoRepo(t, ctx, baseURL, fixture.username, fixture.password, repoName, false)
	repoURL := forgejoRepoURL(baseURL, fixture.username, repoName)
	pushSpecToForgejo(t, baseURL, fixture.username, fixture.password, repoName, "docs/openapi.yaml")

	src := sourceadapter.Git{Repo: repoURL, Ref: "main", Path: "docs/openapi.yaml"}
	spec, rev, err := src.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if spec.SourceID != repoURL {
		t.Fatalf("source id = %q, want %q", spec.SourceID, repoURL)
	}
	if spec.Path != "docs/openapi.yaml" {
		t.Fatalf("spec path = %q", spec.Path)
	}
	if spec.Format != "yaml" {
		t.Fatalf("spec format = %q", spec.Format)
	}
	if !strings.Contains(string(spec.Bytes), "Forgejo API") {
		t.Fatalf("spec bytes = %q", spec.Bytes)
	}
	if rev.SourceID != repoURL || rev.Ref != "main" || rev.CommitSHA == "" || rev.ID == "" {
		t.Fatalf("revision = %#v", rev)
	}
}

func TestGitSourceFetchesSpecFromPrivateForgejoRepositoryWithHTTPSCredentials(t *testing.T) {
	ctx := context.Background()
	fixture := acquireForgejo(t, ctx)
	baseURL := fixture.baseURL
	repoName := "private-openapi-docs"
	createForgejoRepo(t, ctx, baseURL, fixture.username, fixture.password, repoName, true)
	repoURL := forgejoRepoURL(baseURL, fixture.username, repoName)
	pushSpecToForgejo(t, baseURL, fixture.username, fixture.password, repoName, "docs/openapi.yaml")

	src := sourceadapter.Git{
		Repo:     repoURL,
		Ref:      "main",
		Path:     "docs/openapi.yaml",
		Username: fixture.username,
		Token:    fixture.password,
	}
	spec, rev, err := src.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(spec.Bytes), "Forgejo API") {
		t.Fatalf("spec bytes = %q", spec.Bytes)
	}
	if rev.SourceID != repoURL || rev.Ref != "main" || rev.CommitSHA == "" || rev.ID == "" {
		t.Fatalf("revision = %#v", rev)
	}
}

func TestGitSourceFetchesSpecFromPrivateForgejoRepositoryWithSSHCredentials(t *testing.T) {
	ctx := context.Background()
	fixture := acquireForgejo(t, ctx)
	baseURL := fixture.baseURL
	privateKey, publicKey := generateSSHKeyPair(t)
	addForgejoSSHKey(t, ctx, baseURL, fixture.username, fixture.password, publicKey)

	repoName := "ssh-openapi-docs"
	createForgejoRepo(t, ctx, baseURL, fixture.username, fixture.password, repoName, true)
	pushSpecToForgejo(t, baseURL, fixture.username, fixture.password, repoName, "docs/openapi.yaml")

	repoURL := forgejoSSHRepoURL(fixture.sshEndpoint, fixture.username, repoName)
	src := sourceadapter.Git{
		Repo:          repoURL,
		Ref:           "main",
		Path:          "docs/openapi.yaml",
		SSHPrivateKey: privateKey,
	}
	spec, rev, err := src.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(spec.Bytes), "Forgejo API") {
		t.Fatalf("spec bytes = %q", spec.Bytes)
	}
	if rev.SourceID != repoURL || rev.Ref != "main" || rev.CommitSHA == "" || rev.ID == "" {
		t.Fatalf("revision = %#v", rev)
	}
}

type forgejoFixture struct {
	baseURL     string
	sshEndpoint string
	username    string
	password    string
}

func acquireForgejo(t *testing.T, ctx context.Context) forgejoFixture {
	t.Helper()
	const (
		httpURLEnv  = "MANJA_FORGEJO_HTTP_URL"
		sshEnv      = "MANJA_FORGEJO_SSH_ENDPOINT"
		usernameEnv = "MANJA_FORGEJO_ADMIN_USERNAME"
		passwordEnv = "MANJA_FORGEJO_ADMIN_PASSWORD"
	)
	values := []string{
		os.Getenv(httpURLEnv),
		os.Getenv(sshEnv),
		os.Getenv(usernameEnv),
		os.Getenv(passwordEnv),
	}
	configured := 0
	for _, value := range values {
		if value != "" {
			configured++
		}
	}
	if configured != 0 {
		if configured != len(values) {
			t.Fatalf("%s, %s, %s, and %s must be set together", httpURLEnv, sshEnv, usernameEnv, passwordEnv)
		}
		return forgejoFixture{
			baseURL: values[0], sshEndpoint: values[1], username: values[2], password: values[3],
		}
	}

	container, err := forgejo.Run(ctx, forgejoImage)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sshEndpoint, err := container.SSHConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return forgejoFixture{
		baseURL:     baseURL,
		sshEndpoint: sshEndpoint,
		username:    container.AdminUsername(),
		password:    container.AdminPassword(),
	}
}

func createForgejoRepo(t *testing.T, ctx context.Context, baseURL, username, password, name string, private bool) {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"name":%q,"private":%t}`, name, private))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/user/repos", body)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var payload any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		t.Fatalf("create repo status = %d, body = %#v", resp.StatusCode, payload)
	}
}

func addForgejoSSHKey(t *testing.T, ctx context.Context, baseURL, username, password, publicKey string) {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"title":"manja-test-key","key":%q}`, publicKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/user/keys", body)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var payload any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		t.Fatalf("create ssh key status = %d, body = %#v", resp.StatusCode, payload)
	}
}

func pushSpecToForgejo(t *testing.T, baseURL, username, password, repoName, specPath string) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "manja@example.test")
	git(t, repo, "config", "user.name", "Manja Test")
	path := filepath.Join(repo, filepath.FromSlash(specPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := "openapi: 3.1.0\ninfo:\n  title: Forgejo API\n  version: v1\npaths: {}\n"
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", specPath)
	git(t, repo, "commit", "-m", "add spec")
	git(t, repo, "remote", "add", "origin", authenticatedForgejoRepoURL(baseURL, username, password, repoName))
	git(t, repo, "push", "-u", "origin", "main")
}

func forgejoRepoURL(baseURL, username, repoName string) string {
	return strings.TrimRight(baseURL, "/") + "/" + username + "/" + repoName + ".git"
}

func forgejoSSHRepoURL(endpoint, username, repoName string) string {
	return "ssh://git@" + endpoint + "/" + username + "/" + repoName + ".git"
}

func authenticatedForgejoRepoURL(baseURL, username, password, repoName string) string {
	parsed, err := url.Parse(forgejoRepoURL(baseURL, username, repoName))
	if err != nil {
		panic(err)
	}
	parsed.User = url.UserPassword(username, password)
	return parsed.String()
}

func generateSSHKeyPair(t *testing.T) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "id_ed25519")
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen failed: %v\n%s", err, out)
	}
	privateKey, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	return string(privateKey), strings.TrimSpace(string(publicKey))
}

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
