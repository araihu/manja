package architecture_test

import (
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type externalSourceProvenance struct {
	SchemaVersion int                `json:"schemaVersion"`
	Assets        assetsProvenance   `json:"assets"`
	Goshtoso      goshtosoProvenance `json:"goshtoso"`
}

type sourceRevision struct {
	Ref       string `json:"ref"`
	CommitSHA string `json:"commitSha"`
	TreeSHA   string `json:"treeSha"`
}

type legalFileEvidence struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	SPDX       string `json:"spdx,omitempty"`
	Size       int    `json:"size"`
	GitBlobSHA string `json:"gitBlobSha"`
	SHA256     string `json:"sha256"`
}

type assetsProvenance struct {
	Repository string                    `json:"repository"`
	OriginMain sourceRevision            `json:"originMain"`
	Release    assetsReleaseProvenance   `json:"release"`
	Legal      []legalFileEvidence       `json:"legal"`
	Consumers  []assetConsumerProvenance `json:"consumers"`
}

type assetsReleaseProvenance struct {
	Tag               string `json:"tag"`
	CommitSHA         string `json:"commitSha"`
	TreeSHA           string `json:"treeSha"`
	ArchiveURL        string `json:"archiveUrl"`
	ArchiveSHA256     string `json:"archiveSha256"`
	ReleaseJSONSHA256 string `json:"releaseJsonSha256"`
}

type assetConsumerProvenance struct {
	SourcePath string   `json:"sourcePath"`
	GitPath    string   `json:"gitPath"`
	LocalPaths []string `json:"localPaths"`
	Size       int      `json:"size"`
	GitBlobSHA string   `json:"gitBlobSha"`
	SHA256     string   `json:"sha256"`
}

type goshtosoProvenance struct {
	Repository   string            `json:"repository"`
	OriginMain   sourceRevision    `json:"originMain"`
	Module       string            `json:"module"`
	Version      string            `json:"version"`
	TagCommitSHA string            `json:"tagCommitSha"`
	TagTreeSHA   string            `json:"tagTreeSha"`
	License      legalFileEvidence `json:"license"`
}

var approvedExternalSourceProvenance = externalSourceProvenance{
	SchemaVersion: 1,
	Assets: assetsProvenance{
		Repository: "https://github.com/araihu/assets",
		OriginMain: sourceRevision{
			Ref:       "refs/remotes/origin/main",
			CommitSHA: "9a1fce17ad1a99892e81bf3b3b36e7ed48448b63",
			TreeSHA:   "2be5242f515b452052e514c8dd495a95791e5925",
		},
		Release: assetsReleaseProvenance{
			Tag:               "v0.2.1",
			CommitSHA:         "fdfb1c2aad8fa61779e7b8c6f208e52a6cf825ce",
			TreeSHA:           "e89dce3f6bbb129bf3dfd3f04d874cdb220bb4a0",
			ArchiveURL:        "https://github.com/araihu/assets/releases/download/v0.2.1/araihu-assets-v0.2.1.tar.gz",
			ArchiveSHA256:     "818a32246c040871c8f28bb085269b6b9f21c579b18dc4c3c1f20d70716eaf70",
			ReleaseJSONSHA256: "1e071ba6d88efa862b6166820bdc759c7edb917c8566ce7111358c5c3dc2714e",
		},
		Legal: []legalFileEvidence{
			{Path: "LICENSE", Kind: "license", SPDX: "Apache-2.0", Size: 11358, GitBlobSHA: "d645695673349e3947e8e5ae42332d0ac3164cd7", SHA256: "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30"},
			{Path: "NOTICE", Kind: "notice", Size: 633, GitBlobSHA: "61949028cb73348c69d488a844a36fb6253b46b7", SHA256: "8ab9628587c91891e424abdc1c16fdd8d9a89191e56fe496dadc979994bd6366"},
		},
		Consumers: []assetConsumerProvenance{
			{SourcePath: "themes/araihu.css", GitPath: "dist/themes/araihu.css", LocalPaths: []string{"internal/web/static/araihu.css", "site/internal/site/static/araihu.css"}, Size: 2203, GitBlobSHA: "3c54e1236bc3356574dfffb849132d0fe29aaa2c", SHA256: "9ec3f3187b736252b18f3aefef4737ba2025ef1c637611c3d0ecf58748043f1b"},
			{SourcePath: "platform/web/manja/favicon.svg", GitPath: "dist/platform/web/manja/favicon.svg", LocalPaths: []string{"internal/web/static/favicon.svg", "site/internal/site/static/favicon.svg"}, Size: 1839, GitBlobSHA: "d615108fde30afea6da1e4cc8266a9c37b2c8d44", SHA256: "d622096910fa1d2a4d5f64b8d75ade1b3b521f28915e91e70627c52649e9dc1e"},
			{SourcePath: "icons/brand/manja-icon-adaptive-transparent-optical.svg", GitPath: "dist/icons/brand/manja-icon-adaptive-transparent-optical.svg", LocalPaths: []string{"internal/web/static/manja-mark.svg", "site/internal/site/static/manja-mark.svg"}, Size: 1839, GitBlobSHA: "d615108fde30afea6da1e4cc8266a9c37b2c8d44", SHA256: "d622096910fa1d2a4d5f64b8d75ade1b3b521f28915e91e70627c52649e9dc1e"},
			{SourcePath: "brand/manja/logo/adaptive-transparent-optical.svg", GitPath: "dist/brand/manja/logo/adaptive-transparent-optical.svg", LocalPaths: []string{"site/internal/site/static/manja-logo.svg"}, Size: 3995, GitBlobSHA: "76426a562bea2836d0ca5e5fcabf2f99fbbb592f", SHA256: "309ff1be58cc19126a0e70e317c864a53c71ee664767e0f1b8e8a1995a780391"},
		},
	},
	Goshtoso: goshtosoProvenance{
		Repository:   "https://github.com/araihu/goshtoso",
		OriginMain:   sourceRevision{Ref: "refs/remotes/origin/main", CommitSHA: "78921015c2f3b46379ac30d3d9f80755bb860307", TreeSHA: "87d6cac8b9aa732b27976c32851d28fd430475af"},
		Module:       "github.com/araihu/goshtoso",
		Version:      "v0.1.13",
		TagCommitSHA: "19c86f1dbbcf5a85c55f2d9b3bfaac4fd5febea6",
		TagTreeSHA:   "49c9f5557ae5bb74a8f333af8958bbf495377af0",
		License:      legalFileEvidence{Path: "LICENSE", Kind: "license", SPDX: "MIT", Size: 1078, GitBlobSHA: "0a7743398ecbeacc05ed822e1f74023ee9b36842", SHA256: "cacf68ff9920c026f5de2ebf992333c1a243e45d81aaa5b4577e05b52c5a9584"},
	},
}

func TestExternalSourceProvenanceReceiptMatchesApprovedIdentity(t *testing.T) {
	t.Parallel()

	receipt := loadExternalSourceProvenance(t)
	if !reflect.DeepEqual(receipt, approvedExternalSourceProvenance) {
		t.Fatalf("external source provenance = %#v, want %#v", receipt, approvedExternalSourceProvenance)
	}
}

func TestExternalSourceProvenanceBindsPinnedAssetsManifest(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "araihu-assets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		AssetsRepository  string `json:"assetsRepository"`
		AssetsRevision    string `json:"assetsRevision"`
		Release           string `json:"release"`
		ReleaseURL        string `json:"releaseUrl"`
		ReleaseSHA256     string `json:"releaseSha256"`
		ReleaseJSONSHA256 string `json:"releaseJsonSha256"`
		Mappings          []struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	release := approvedExternalSourceProvenance.Assets.Release
	if manifest.AssetsRepository != "araihu/assets" || manifest.AssetsRevision != release.CommitSHA || manifest.Release != release.Tag || manifest.ReleaseURL != release.ArchiveURL || manifest.ReleaseSHA256 != release.ArchiveSHA256 || manifest.ReleaseJSONSHA256 != release.ReleaseJSONSHA256 {
		t.Fatalf("araihu-assets.json identity = %#v, want release %#v", manifest, release)
	}
	consumers := make(map[string]assetConsumerProvenance, len(approvedExternalSourceProvenance.Assets.Consumers))
	for _, consumer := range approvedExternalSourceProvenance.Assets.Consumers {
		consumers[consumer.SourcePath] = consumer
	}
	seen := make(map[string]bool)
	for _, mapping := range manifest.Mappings {
		consumer, ok := consumers[mapping.Source]
		if !ok || !contains(consumer.LocalPaths, mapping.Destination) {
			t.Fatalf("manifest mapping %q -> %q is absent from provenance receipt", mapping.Source, mapping.Destination)
		}
		seen[mapping.Source] = true
	}
	if len(seen) != len(consumers) {
		t.Fatalf("manifest covered %d source paths, want %d", len(seen), len(consumers))
	}
}

func TestExternalSourceProvenanceMatchesInstalledAssetBytes(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, consumer := range approvedExternalSourceProvenance.Assets.Consumers {
		for _, relative := range consumer.LocalPaths {
			contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatalf("read installed asset %s: %v", relative, err)
			}
			if len(contents) != consumer.Size {
				t.Errorf("%s size = %d, want %d", relative, len(contents), consumer.Size)
			}
			if got := sha256Hex(contents); got != consumer.SHA256 {
				t.Errorf("%s SHA-256 = %s, want %s", relative, got, consumer.SHA256)
			}
			if got := gitBlobSHA(contents); got != consumer.GitBlobSHA {
				t.Errorf("%s Git blob = %s, want %s", relative, got, consumer.GitBlobSHA)
			}
		}
	}
}

func TestExternalSourceProvenanceMatchesPinnedModuleLicenseBytes(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, evidence := range []struct {
		module  string
		version string
		file    legalFileEvidence
	}{
		{module: "github.com/araihu/assets", version: "v0.2.1", file: approvedExternalSourceProvenance.Assets.Legal[0]},
		{module: "github.com/araihu/assets", version: "v0.2.1", file: approvedExternalSourceProvenance.Assets.Legal[1]},
		{module: "github.com/araihu/goshtoso", version: "v0.1.13", file: approvedExternalSourceProvenance.Goshtoso.License},
	} {
		moduleRoot := goModuleRoot(t, root, evidence.module)
		if got := goModuleVersion(t, root, evidence.module); got != evidence.version {
			t.Fatalf("%s module version = %s, want %s", evidence.module, got, evidence.version)
		}
		contents, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(evidence.file.Path)))
		if err != nil {
			t.Fatalf("read %s %s: %v", evidence.module, evidence.file.Path, err)
		}
		if len(contents) != evidence.file.Size {
			t.Errorf("%s %s size = %d, want %d", evidence.module, evidence.file.Path, len(contents), evidence.file.Size)
		}
		if got := sha256Hex(contents); got != evidence.file.SHA256 {
			t.Errorf("%s %s SHA-256 = %s, want %s", evidence.module, evidence.file.Path, got, evidence.file.SHA256)
		}
		if got := gitBlobSHA(contents); got != evidence.file.GitBlobSHA {
			t.Errorf("%s %s Git blob = %s, want %s", evidence.module, evidence.file.Path, got, evidence.file.GitBlobSHA)
		}
	}
}

func loadExternalSourceProvenance(t *testing.T) externalSourceProvenance {
	t.Helper()
	root := repositoryRoot(t)
	file, err := os.Open(filepath.Join(root, "docs", "legal", "external-source-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var receipt externalSourceProvenance
	if err := decoder.Decode(&receipt); err != nil {
		t.Fatalf("decode external source provenance: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("external source provenance has trailing JSON: %v", err)
	}
	return receipt
}

func goModuleRoot(t *testing.T, root, module string) string {
	t.Helper()
	command := exec.Command("go", "list", "-mod=readonly", "-m", "-f", "{{.Dir}}", module)
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("resolve %s module root: %v", module, err)
	}
	return strings.TrimSpace(string(output))
}

func goModuleVersion(t *testing.T, root, module string) string {
	t.Helper()
	command := exec.Command("go", "list", "-mod=readonly", "-m", "-f", "{{.Version}}", module)
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("resolve %s module version: %v", module, err)
	}
	return strings.TrimSpace(string(output))
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sha256Hex(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func gitBlobSHA(contents []byte) string {
	digest := sha1.New() // #nosec G401 -- Git blob identity is defined as SHA-1.
	_, _ = fmt.Fprintf(digest, "blob %d\x00", len(contents))
	_, _ = digest.Write(contents)
	return hex.EncodeToString(digest.Sum(nil))
}
