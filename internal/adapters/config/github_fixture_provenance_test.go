package config

import (
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type githubFixtureProvenance struct {
	SchemaVersion int    `json:"schemaVersion"`
	Repository    string `json:"repository"`
	CommitSHA     string `json:"commitSha"`
	CommitTreeSHA string `json:"commitTreeSha"`
	Artifact      struct {
		LocalPath    string `json:"localPath"`
		UpstreamPath string `json:"upstreamPath"`
		Size         int    `json:"size"`
		GitBlobSHA   string `json:"gitBlobSha"`
		SHA256       string `json:"sha256"`
	} `json:"artifact"`
	License struct {
		Name         string `json:"name"`
		SPDX         string `json:"spdx"`
		UpstreamPath string `json:"upstreamPath"`
		Size         int    `json:"size"`
		GitBlobSHA   string `json:"gitBlobSha"`
		SHA256       string `json:"sha256"`
	} `json:"license"`
}

func TestGitHubFixtureMatchesImmutableUpstreamReceipt(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "..")
	receiptPath := filepath.Join(root, "internal", "adapters", "openapi", "testdata", "github-v3-rest.provenance.json")
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt githubFixtureProvenance
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.Repository != "https://github.com/github/rest-api-description" {
		t.Fatalf("receipt authority = %#v", receipt)
	}
	if len(receipt.CommitSHA) != 40 || len(receipt.CommitTreeSHA) != 40 || receipt.Artifact.UpstreamPath == "" || receipt.License.UpstreamPath == "" {
		t.Fatalf("receipt has incomplete immutable identity: %#v", receipt)
	}

	fixture, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(receipt.Artifact.LocalPath)))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture) != receipt.Artifact.Size {
		t.Fatalf("fixture size = %d, want %d", len(fixture), receipt.Artifact.Size)
	}
	if got := sha256Hex(fixture); got != receipt.Artifact.SHA256 {
		t.Fatalf("fixture SHA-256 = %s, want %s", got, receipt.Artifact.SHA256)
	}
	if got := gitBlobSHA(fixture); got != receipt.Artifact.GitBlobSHA {
		t.Fatalf("fixture Git blob = %s, want %s", got, receipt.Artifact.GitBlobSHA)
	}

	loaded, err := LoadRenderer(filepath.Join(root, "internal", "renderer", "testdata", "kubernetes", "renderer.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := loaded.RuntimeConfig()
	wantSourceURL := receipt.Repository + "/blob/" + receipt.CommitSHA + "/" + receipt.Artifact.UpstreamPath
	wantLicenseURL := receipt.Repository + "/blob/" + receipt.CommitSHA + "/" + receipt.License.UpstreamPath
	sourceURL := ""
	for _, source := range runtime.Organization.Sources {
		if source.Location == receipt.Artifact.LocalPath {
			sourceURL = source.URL
			break
		}
	}
	if sourceURL != wantSourceURL {
		t.Fatalf("GitHub source URL = %q, want %q", sourceURL, wantSourceURL)
	}
	for _, catalog := range runtime.Catalogs {
		if catalog.ID == "github" {
			if catalog.License.Name != receipt.License.Name || catalog.License.URL != wantLicenseURL {
				t.Fatalf("GitHub license = %#v, want %q at %q", catalog.License, receipt.License.Name, wantLicenseURL)
			}
			return
		}
	}
	t.Fatal("GitHub catalog is missing")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gitBlobSHA(data []byte) string {
	hash := sha1.New() // #nosec G401 -- Git blob identity is defined as SHA-1.
	_, _ = fmt.Fprintf(hash, "blob %d\x00", len(data))
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
