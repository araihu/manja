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
	SchemaVersion int                             `json:"schemaVersion"`
	Repository    string                          `json:"repository"`
	CommitSHA     string                          `json:"commitSha"`
	CommitTreeSHA string                          `json:"commitTreeSha"`
	Artifact      githubFixtureArtifactProvenance `json:"artifact"`
	License       githubFixtureLicenseProvenance  `json:"license"`
}

type githubFixtureArtifactProvenance struct {
	LocalPath    string `json:"localPath"`
	UpstreamPath string `json:"upstreamPath"`
	Size         int    `json:"size"`
	GitBlobSHA   string `json:"gitBlobSha"`
	SHA256       string `json:"sha256"`
}

type githubFixtureLicenseProvenance struct {
	Name         string `json:"name"`
	SPDX         string `json:"spdx"`
	UpstreamPath string `json:"upstreamPath"`
	Size         int    `json:"size"`
	GitBlobSHA   string `json:"gitBlobSha"`
	SHA256       string `json:"sha256"`
}

var approvedGitHubFixtureProvenance = githubFixtureProvenance{
	SchemaVersion: 1,
	Repository:    "https://github.com/github/rest-api-description",
	CommitSHA:     "6948cb04f5304188569c4bb4ae2190c08e7cbdba",
	CommitTreeSHA: "6270ed1bd31a741adf3c7143c39d9bdc57d2fbc1",
	Artifact: githubFixtureArtifactProvenance{
		LocalPath:    "internal/adapters/openapi/testdata/github-v3-rest.json",
		UpstreamPath: "descriptions/ghes-3.0/ghes-3.0.json",
		Size:         3319366,
		GitBlobSHA:   "f0ddf34ad4398c319db0643e45a0908ca026b382",
		SHA256:       "dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977",
	},
	License: githubFixtureLicenseProvenance{
		Name:         "MIT License",
		SPDX:         "MIT",
		UpstreamPath: "LICENSE.md",
		Size:         1063,
		GitBlobSHA:   "b50625eb63949013cae604b1cadd42cfa1eaf825",
		SHA256:       "3243761cbac07e6d169a5a2f4e7c25cc544da85248e735df74c3672e055cc87b",
	},
}

func TestGitHubFixtureProvenanceRejectsControlledDrift(t *testing.T) {
	t.Parallel()

	approved := approvedGitHubFixtureProvenance
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(approved.Artifact.LocalPath)))
	if err != nil {
		t.Fatal(err)
	}
	sourceURL := approved.Repository + "/blob/" + approved.CommitSHA + "/" + approved.Artifact.UpstreamPath
	licenseURL := approved.Repository + "/blob/" + approved.CommitSHA + "/" + approved.License.UpstreamPath

	mutations := []struct {
		name   string
		mutate func(*githubFixtureProvenance)
	}{
		{name: "schema version", mutate: func(got *githubFixtureProvenance) { got.SchemaVersion++ }},
		{name: "repository", mutate: func(got *githubFixtureProvenance) { got.Repository += "-fork" }},
		{name: "commit SHA", mutate: func(got *githubFixtureProvenance) { got.CommitSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }},
		{name: "commit tree SHA", mutate: func(got *githubFixtureProvenance) { got.CommitTreeSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" }},
		{name: "artifact local path", mutate: func(got *githubFixtureProvenance) { got.Artifact.LocalPath += ".changed" }},
		{name: "artifact upstream path", mutate: func(got *githubFixtureProvenance) { got.Artifact.UpstreamPath += ".changed" }},
		{name: "artifact size", mutate: func(got *githubFixtureProvenance) { got.Artifact.Size++ }},
		{name: "artifact Git blob SHA", mutate: func(got *githubFixtureProvenance) {
			got.Artifact.GitBlobSHA = "cccccccccccccccccccccccccccccccccccccccc"
		}},
		{name: "artifact SHA-256", mutate: func(got *githubFixtureProvenance) {
			got.Artifact.SHA256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		}},
		{name: "license name", mutate: func(got *githubFixtureProvenance) { got.License.Name = "changed" }},
		{name: "license SPDX", mutate: func(got *githubFixtureProvenance) { got.License.SPDX = "changed" }},
		{name: "license upstream path", mutate: func(got *githubFixtureProvenance) { got.License.UpstreamPath = "COPYING" }},
		{name: "license size", mutate: func(got *githubFixtureProvenance) { got.License.Size++ }},
		{name: "license Git blob SHA", mutate: func(got *githubFixtureProvenance) {
			got.License.GitBlobSHA = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		}},
		{name: "license SHA-256", mutate: func(got *githubFixtureProvenance) {
			got.License.SHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := approved
			mutation.mutate(&changed)
			if err := validateGitHubFixtureProvenance(changed, fixture, sourceURL, approved.License.Name, licenseURL); err == nil {
				t.Fatal("controlled receipt drift was accepted")
			}
		})
	}

	t.Run("coordinated receipt and renderer URL drift", func(t *testing.T) {
		changed := approved
		changed.CommitSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		changed.Artifact.UpstreamPath = "descriptions/ghes-3.1/ghes-3.1.json"
		changedSourceURL := changed.Repository + "/blob/" + changed.CommitSHA + "/" + changed.Artifact.UpstreamPath
		changedLicenseURL := changed.Repository + "/blob/" + changed.CommitSHA + "/" + changed.License.UpstreamPath
		if err := validateGitHubFixtureProvenance(changed, fixture, changedSourceURL, changed.License.Name, changedLicenseURL); err == nil {
			t.Fatal("coordinated receipt and renderer URL drift was accepted")
		}
	})

	t.Run("coordinated fixture and receipt digest drift", func(t *testing.T) {
		changedFixture := append(append([]byte(nil), fixture...), '\n')
		changed := approved
		changed.Artifact.Size = len(changedFixture)
		changed.Artifact.GitBlobSHA = gitBlobSHA(changedFixture)
		changed.Artifact.SHA256 = sha256Hex(changedFixture)
		if err := validateGitHubFixtureProvenance(changed, changedFixture, sourceURL, approved.License.Name, licenseURL); err == nil {
			t.Fatal("coordinated fixture and receipt digest drift was accepted")
		}
	})
}

func TestGitHubFixtureMatchesImmutableUpstreamReceipt(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "..")
	approved := approvedGitHubFixtureProvenance
	receiptPath := filepath.Join(root, "internal", "adapters", "openapi", "testdata", "github-v3-rest.provenance.json")
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt githubFixtureProvenance
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}

	fixture, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(approved.Artifact.LocalPath)))
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRenderer(filepath.Join(root, "internal", "renderer", "testdata", "kubernetes", "renderer.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := loaded.RuntimeConfig()
	sourceURL := ""
	for _, source := range runtime.Organization.Sources {
		if source.Location == approved.Artifact.LocalPath {
			sourceURL = source.URL
			break
		}
	}
	licenseName := ""
	licenseURL := ""
	for _, catalog := range runtime.Catalogs {
		if catalog.ID == "github" {
			licenseName = catalog.License.Name
			licenseURL = catalog.License.URL
			break
		}
	}
	if err := validateGitHubFixtureProvenance(receipt, fixture, sourceURL, licenseName, licenseURL); err != nil {
		t.Fatal(err)
	}
}

func validateGitHubFixtureProvenance(receipt githubFixtureProvenance, fixture []byte, sourceURL, licenseName, licenseURL string) error {
	approved := approvedGitHubFixtureProvenance
	if receipt.SchemaVersion != approved.SchemaVersion {
		return fmt.Errorf("receipt schema version = %d, want %d", receipt.SchemaVersion, approved.SchemaVersion)
	}
	if receipt.Repository != approved.Repository {
		return fmt.Errorf("receipt repository = %q, want %q", receipt.Repository, approved.Repository)
	}
	if receipt.CommitSHA != approved.CommitSHA {
		return fmt.Errorf("receipt commit SHA = %q, want %q", receipt.CommitSHA, approved.CommitSHA)
	}
	if receipt.CommitTreeSHA != approved.CommitTreeSHA {
		return fmt.Errorf("receipt commit tree SHA = %q, want %q", receipt.CommitTreeSHA, approved.CommitTreeSHA)
	}
	if receipt.Artifact.LocalPath != approved.Artifact.LocalPath {
		return fmt.Errorf("receipt artifact local path = %q, want %q", receipt.Artifact.LocalPath, approved.Artifact.LocalPath)
	}
	if receipt.Artifact.UpstreamPath != approved.Artifact.UpstreamPath {
		return fmt.Errorf("receipt artifact upstream path = %q, want %q", receipt.Artifact.UpstreamPath, approved.Artifact.UpstreamPath)
	}
	if receipt.Artifact.Size != approved.Artifact.Size {
		return fmt.Errorf("receipt artifact size = %d, want %d", receipt.Artifact.Size, approved.Artifact.Size)
	}
	if receipt.Artifact.GitBlobSHA != approved.Artifact.GitBlobSHA {
		return fmt.Errorf("receipt artifact Git blob SHA = %q, want %q", receipt.Artifact.GitBlobSHA, approved.Artifact.GitBlobSHA)
	}
	if receipt.Artifact.SHA256 != approved.Artifact.SHA256 {
		return fmt.Errorf("receipt artifact SHA-256 = %q, want %q", receipt.Artifact.SHA256, approved.Artifact.SHA256)
	}
	if receipt.License.Name != approved.License.Name {
		return fmt.Errorf("receipt license name = %q, want %q", receipt.License.Name, approved.License.Name)
	}
	if receipt.License.SPDX != approved.License.SPDX {
		return fmt.Errorf("receipt license SPDX = %q, want %q", receipt.License.SPDX, approved.License.SPDX)
	}
	if receipt.License.UpstreamPath != approved.License.UpstreamPath {
		return fmt.Errorf("receipt license upstream path = %q, want %q", receipt.License.UpstreamPath, approved.License.UpstreamPath)
	}
	if receipt.License.Size != approved.License.Size {
		return fmt.Errorf("receipt license size = %d, want %d", receipt.License.Size, approved.License.Size)
	}
	if receipt.License.GitBlobSHA != approved.License.GitBlobSHA {
		return fmt.Errorf("receipt license Git blob SHA = %q, want %q", receipt.License.GitBlobSHA, approved.License.GitBlobSHA)
	}
	if receipt.License.SHA256 != approved.License.SHA256 {
		return fmt.Errorf("receipt license SHA-256 = %q, want %q", receipt.License.SHA256, approved.License.SHA256)
	}

	if len(fixture) != approved.Artifact.Size {
		return fmt.Errorf("fixture size = %d, want %d", len(fixture), approved.Artifact.Size)
	}
	if got := sha256Hex(fixture); got != approved.Artifact.SHA256 {
		return fmt.Errorf("fixture SHA-256 = %s, want %s", got, approved.Artifact.SHA256)
	}
	if got := gitBlobSHA(fixture); got != approved.Artifact.GitBlobSHA {
		return fmt.Errorf("fixture Git blob = %s, want %s", got, approved.Artifact.GitBlobSHA)
	}

	wantSourceURL := approved.Repository + "/blob/" + approved.CommitSHA + "/" + approved.Artifact.UpstreamPath
	if sourceURL != wantSourceURL {
		return fmt.Errorf("GitHub source URL = %q, want %q", sourceURL, wantSourceURL)
	}
	wantLicenseURL := approved.Repository + "/blob/" + approved.CommitSHA + "/" + approved.License.UpstreamPath
	if licenseName != approved.License.Name || licenseURL != wantLicenseURL {
		return fmt.Errorf("GitHub license = %q at %q, want %q at %q", licenseName, licenseURL, approved.License.Name, wantLicenseURL)
	}
	return nil
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
