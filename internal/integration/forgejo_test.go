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

func TestForgejoContainerStarts(t *testing.T) {
	ctx := context.Background()
	c, err := forgejo.Run(ctx, "codeberg.org/forgejo/forgejo:11")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate(ctx)
}

func TestGitSourceFetchesSpecFromPublicForgejoRepository(t *testing.T) {
	ctx := context.Background()
	c, err := forgejo.Run(ctx, "codeberg.org/forgejo/forgejo:11")
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repoName := "openapi-docs"
	createForgejoRepo(t, ctx, baseURL, c.AdminUsername(), c.AdminPassword(), repoName, false)
	repoURL := forgejoRepoURL(baseURL, c.AdminUsername(), repoName)
	pushSpecToForgejo(t, baseURL, c.AdminUsername(), c.AdminPassword(), repoName, "docs/openapi.yaml")

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
	c, err := forgejo.Run(ctx, "codeberg.org/forgejo/forgejo:11")
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repoName := "private-openapi-docs"
	createForgejoRepo(t, ctx, baseURL, c.AdminUsername(), c.AdminPassword(), repoName, true)
	repoURL := forgejoRepoURL(baseURL, c.AdminUsername(), repoName)
	pushSpecToForgejo(t, baseURL, c.AdminUsername(), c.AdminPassword(), repoName, "docs/openapi.yaml")

	src := sourceadapter.Git{
		Repo:     repoURL,
		Ref:      "main",
		Path:     "docs/openapi.yaml",
		Username: c.AdminUsername(),
		Token:    c.AdminPassword(),
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
	c, err := forgejo.Run(ctx, "codeberg.org/forgejo/forgejo:11")
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, publicKey := generateSSHKeyPair(t)
	addForgejoSSHKey(t, ctx, baseURL, c.AdminUsername(), c.AdminPassword(), publicKey)

	repoName := "ssh-openapi-docs"
	createForgejoRepo(t, ctx, baseURL, c.AdminUsername(), c.AdminPassword(), repoName, true)
	pushSpecToForgejo(t, baseURL, c.AdminUsername(), c.AdminPassword(), repoName, "docs/openapi.yaml")

	sshEndpoint, err := c.SSHConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repoURL := forgejoSSHRepoURL(sshEndpoint, c.AdminUsername(), repoName)
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
