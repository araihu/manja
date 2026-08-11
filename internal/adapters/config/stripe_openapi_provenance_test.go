package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stripeOpenAPIProvenance struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Repository    string                        `json:"repository"`
	CommitSHA     string                        `json:"commitSha"`
	CommitTreeSHA string                        `json:"commitTreeSha"`
	Artifact      stripeOpenAPIArtifactEvidence `json:"artifact"`
	License       stripeOpenAPILicenseEvidence  `json:"license"`
}

type stripeOpenAPIArtifactEvidence struct {
	UpstreamPath   string `json:"upstreamPath"`
	TrackedLocally bool   `json:"trackedLocally"`
	Size           int    `json:"size"`
	GitBlobSHA     string `json:"gitBlobSha"`
	SHA256         string `json:"sha256"`
}

type stripeOpenAPILicenseEvidence struct {
	Name           string `json:"name"`
	SPDX           string `json:"spdx"`
	UpstreamPath   string `json:"upstreamPath"`
	TrackedLocally bool   `json:"trackedLocally"`
	Size           int    `json:"size"`
	GitBlobSHA     string `json:"gitBlobSha"`
	SHA256         string `json:"sha256"`
}

type stripeRendererBuildSource struct {
	OrganizationLocation string
	OrganizationURL      string
	Kind                 string
	Repository           string
	Ref                  string
	IncludePattern       string
	UpstreamPath         string
	DocumentKey          string
	LicenseName          string
	LicenseURL           string
	DockerConfigPath     string
}

var approvedStripeOpenAPIProvenance = stripeOpenAPIProvenance{
	SchemaVersion: 1,
	Repository:    "https://github.com/stripe/openapi",
	CommitSHA:     "d70de345383dd818a0ce831f4e20d375c5a90cec",
	CommitTreeSHA: "a7e155600c10dcfab91a94070b0e954419255862",
	Artifact: stripeOpenAPIArtifactEvidence{
		UpstreamPath:   "openapi/spec3.json",
		TrackedLocally: false,
		Size:           3840021,
		GitBlobSHA:     "058edc82a247c71f05b94dfa6b9cef0a794a1358",
		SHA256:         "8b608cba7129d121f12358a7092574e176833fe8cb4c9fcead178c71c545f870",
	},
	License: stripeOpenAPILicenseEvidence{
		Name:           "The MIT License",
		SPDX:           "MIT",
		UpstreamPath:   "LICENSE",
		TrackedLocally: false,
		Size:           1095,
		GitBlobSHA:     "edf2d132d8bb95146e05585c3a782d059298b46b",
		SHA256:         "8c1ce883f4eee7b531e0b7872dbfc72d410ced87dfff9501305de05ca8d203e5",
	},
}

var approvedStripeRendererBuildSource = stripeRendererBuildSource{
	OrganizationLocation: "github.com/stripe/openapi",
	OrganizationURL:      "https://github.com/stripe/openapi/blob/d70de345383dd818a0ce831f4e20d375c5a90cec/openapi/spec3.json",
	Kind:                 "git",
	Repository:           "https://github.com/stripe/openapi.git",
	Ref:                  "d70de345383dd818a0ce831f4e20d375c5a90cec",
	IncludePattern:       "openapi/spec3.json",
	UpstreamPath:         "openapi/spec3.json",
	DocumentKey:          "stripe-v1",
	LicenseName:          "MIT License",
	LicenseURL:           "https://github.com/stripe/openapi/blob/d70de345383dd818a0ce831f4e20d375c5a90cec/LICENSE",
	DockerConfigPath:     "/src/internal/renderer/testdata/kubernetes/renderer.yaml",
}

func TestStripeOpenAPIProvenanceMatchesRendererBuildSource(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "..")
	receiptBytes, err := os.ReadFile(filepath.Join(root, "internal", "renderer", "testdata", "kubernetes", "stripe-openapi.provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt stripeOpenAPIProvenance
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	buildSource := loadCommittedStripeRendererBuildSource(t, root)
	if err := validateStripeOpenAPIProvenance(receipt, buildSource); err != nil {
		t.Fatal(err)
	}
}

func TestStripeOpenAPIProvenanceRejectsControlledDrift(t *testing.T) {
	t.Parallel()

	approvedReceipt := approvedStripeOpenAPIProvenance
	approvedSource := approvedStripeRendererBuildSource
	mutations := []struct {
		name   string
		mutate func(*stripeOpenAPIProvenance)
	}{
		{name: "schema version", mutate: func(got *stripeOpenAPIProvenance) { got.SchemaVersion++ }},
		{name: "repository", mutate: func(got *stripeOpenAPIProvenance) { got.Repository += "-fork" }},
		{name: "commit SHA", mutate: func(got *stripeOpenAPIProvenance) { got.CommitSHA = strings.Repeat("a", 40) }},
		{name: "commit tree SHA", mutate: func(got *stripeOpenAPIProvenance) { got.CommitTreeSHA = strings.Repeat("b", 40) }},
		{name: "artifact upstream path", mutate: func(got *stripeOpenAPIProvenance) { got.Artifact.UpstreamPath += ".changed" }},
		{name: "artifact tracked locally", mutate: func(got *stripeOpenAPIProvenance) { got.Artifact.TrackedLocally = true }},
		{name: "artifact size", mutate: func(got *stripeOpenAPIProvenance) { got.Artifact.Size++ }},
		{name: "artifact Git blob SHA", mutate: func(got *stripeOpenAPIProvenance) { got.Artifact.GitBlobSHA = strings.Repeat("c", 40) }},
		{name: "artifact SHA-256", mutate: func(got *stripeOpenAPIProvenance) { got.Artifact.SHA256 = strings.Repeat("d", 64) }},
		{name: "license name", mutate: func(got *stripeOpenAPIProvenance) { got.License.Name = "changed" }},
		{name: "license SPDX", mutate: func(got *stripeOpenAPIProvenance) { got.License.SPDX = "changed" }},
		{name: "license upstream path", mutate: func(got *stripeOpenAPIProvenance) { got.License.UpstreamPath = "COPYING" }},
		{name: "license tracked locally", mutate: func(got *stripeOpenAPIProvenance) { got.License.TrackedLocally = true }},
		{name: "license size", mutate: func(got *stripeOpenAPIProvenance) { got.License.Size++ }},
		{name: "license Git blob SHA", mutate: func(got *stripeOpenAPIProvenance) { got.License.GitBlobSHA = strings.Repeat("e", 40) }},
		{name: "license SHA-256", mutate: func(got *stripeOpenAPIProvenance) { got.License.SHA256 = strings.Repeat("f", 64) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := approvedReceipt
			mutation.mutate(&changed)
			if err := validateStripeOpenAPIProvenance(changed, approvedSource); err == nil {
				t.Fatal("controlled receipt drift was accepted")
			}
		})
	}

	t.Run("coordinated receipt and renderer source drift", func(t *testing.T) {
		changedReceipt := approvedReceipt
		changedReceipt.CommitSHA = strings.Repeat("a", 40)
		changedReceipt.Artifact.UpstreamPath = "openapi/spec4.json"
		changedSource := approvedSource
		changedSource.Ref = changedReceipt.CommitSHA
		changedSource.UpstreamPath = changedReceipt.Artifact.UpstreamPath
		changedSource.OrganizationURL = changedReceipt.Repository + "/blob/" + changedReceipt.CommitSHA + "/" + changedReceipt.Artifact.UpstreamPath
		changedSource.LicenseURL = changedReceipt.Repository + "/blob/" + changedReceipt.CommitSHA + "/" + changedReceipt.License.UpstreamPath
		if err := validateStripeOpenAPIProvenance(changedReceipt, changedSource); err == nil {
			t.Fatal("coordinated receipt and renderer source drift was accepted")
		}
	})

	t.Run("coordinated untracked byte evidence drift", func(t *testing.T) {
		changed := approvedReceipt
		changed.Artifact.Size++
		changed.Artifact.GitBlobSHA = strings.Repeat("c", 40)
		changed.Artifact.SHA256 = strings.Repeat("d", 64)
		changed.License.Size++
		changed.License.GitBlobSHA = strings.Repeat("e", 40)
		changed.License.SHA256 = strings.Repeat("f", 64)
		if err := validateStripeOpenAPIProvenance(changed, approvedSource); err == nil {
			t.Fatal("coordinated untracked byte evidence drift was accepted")
		}
	})
}

func loadCommittedStripeRendererBuildSource(t *testing.T, root string) stripeRendererBuildSource {
	t.Helper()

	configPath := filepath.Join(root, "internal", "renderer", "testdata", "kubernetes", "renderer.yaml")
	loaded, err := LoadRenderer(configPath)
	if err != nil {
		t.Fatal(err)
	}
	source := stripeRendererBuildSource{}
	for _, organizationSource := range loaded.Organization.Sources {
		if organizationSource.Location == approvedStripeRendererBuildSource.OrganizationLocation {
			source.OrganizationLocation = organizationSource.Location
			source.OrganizationURL = organizationSource.URL
			break
		}
	}
	for _, catalog := range loaded.Catalogs {
		if catalog.ID != "stripe" {
			continue
		}
		source.Kind = catalog.Source.Kind
		source.Repository = catalog.Source.Repository
		source.Ref = catalog.Source.Ref
		if len(catalog.Source.Include) == 1 {
			source.IncludePattern = catalog.Source.Include[0]
		}
		if len(catalog.Source.Documents) == 1 {
			source.UpstreamPath = catalog.Source.Documents[0].Path
			source.DocumentKey = catalog.Source.Documents[0].Key
		}
		source.LicenseName = catalog.License.Name
		source.LicenseURL = catalog.License.URL
		break
	}
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dockerfile), "-renderer-config "+approvedStripeRendererBuildSource.DockerConfigPath) {
		source.DockerConfigPath = approvedStripeRendererBuildSource.DockerConfigPath
	}
	return source
}

func validateStripeOpenAPIProvenance(receipt stripeOpenAPIProvenance, source stripeRendererBuildSource) error {
	approved := approvedStripeOpenAPIProvenance
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
	if receipt.Artifact.UpstreamPath != approved.Artifact.UpstreamPath {
		return fmt.Errorf("receipt artifact upstream path = %q, want %q", receipt.Artifact.UpstreamPath, approved.Artifact.UpstreamPath)
	}
	if receipt.Artifact.TrackedLocally != approved.Artifact.TrackedLocally {
		return fmt.Errorf("receipt artifact tracked locally = %t, want %t", receipt.Artifact.TrackedLocally, approved.Artifact.TrackedLocally)
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
	if receipt.License.TrackedLocally != approved.License.TrackedLocally {
		return fmt.Errorf("receipt license tracked locally = %t, want %t", receipt.License.TrackedLocally, approved.License.TrackedLocally)
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
	if source != approvedStripeRendererBuildSource {
		return fmt.Errorf("Stripe renderer build source = %#v, want %#v", source, approvedStripeRendererBuildSource)
	}
	return nil
}
