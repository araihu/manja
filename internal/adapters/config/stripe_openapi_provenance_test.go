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

func TestStripeOpenAPIProvenanceRejectsDockerfileDecoys(t *testing.T) {
	t.Parallel()

	const approvedPath = "/src/internal/renderer/testdata/kubernetes/renderer.yaml"
	tests := []struct {
		name       string
		dockerfile string
		wantValid  bool
	}{
		{
			name: "baseline",
			dockerfile: `# syntax=docker/dockerfile:1

FROM golang:1.26.5-alpine AS build
RUN /out/manja build \
	-renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml \
	-data-dir /out/renderer-data \
	> /out/renderer-build-receipt.json
FROM alpine:3.24
`,
			wantValid: true,
		},
		{
			name: "approved path comment decoy with wrong effective path",
			dockerfile: `FROM golang:1.26.5-alpine AS build
# RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
RUN /out/manja build -renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "unrelated RUN decoy",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN echo -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
RUN /out/manja build -renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "CMD decoy",
			dockerfile: `FROM golang:1.26.5-alpine AS build
CMD echo -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
RUN /out/manja build -renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "ENTRYPOINT decoy",
			dockerfile: `FROM golang:1.26.5-alpine AS build
ENTRYPOINT echo -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
RUN /out/manja build -renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "approved then wrong duplicate flags",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "wrong then approved duplicate flags",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /wrong/renderer.yaml -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "two build commands",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /wrong/renderer.yaml
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "changed build alias",
			dockerfile: `FROM golang:1.26.5-alpine AS builder
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "duplicate build alias",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM golang:1.26.5-alpine AS build
FROM alpine:3.24
`,
		},
		{
			name: "ambiguous build alias",
			dockerfile: `FROM golang:1.26.5-alpine AS compiler AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "missing build alias",
			dockerfile: `FROM golang:1.26.5-alpine
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "build command only in another stage",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN echo compiler
FROM alpine:3.24
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
`,
		},
		{
			name: "quoted config path",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config "/src/internal/renderer/testdata/kubernetes/renderer.yaml"
FROM alpine:3.24
`,
		},
		{
			name: "quoted command",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN "/out/manja" build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "single-dash equals config form",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config=/src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "double-dash equals config form",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build --renderer-config=/src/internal/renderer/testdata/kubernetes/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "wrong config path",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "missing config path",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -data-dir /out/renderer-data
FROM alpine:3.24
`,
		},
		{
			name: "inline comment after canonical command",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json # decoy
FROM alpine:3.24
`,
		},
		{
			name: "semicolon tail after canonical command",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json ; echo decoy
FROM alpine:3.24
`,
		},
		{
			name: "approved plus wrong double-dash equals flag",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json --renderer-config=/wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "approved plus wrong double-dash flag",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json --renderer-config /wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "wrong double-dash flag then approved",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build --renderer-config /wrong/renderer.yaml -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json
FROM alpine:3.24
`,
		},
		{
			name: "terminator before approved flag",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -- -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json
FROM alpine:3.24
`,
		},
		{
			name: "positional before approved flag",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build positional -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json
FROM alpine:3.24
`,
		},
		{
			name: "chained second build",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /out/renderer-build-receipt.json && /out/manja build --renderer-config=/wrong/renderer.yaml
FROM alpine:3.24
`,
		},
		{
			name: "missing data dir",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml > /out/renderer-build-receipt.json
FROM alpine:3.24
`,
		},
		{
			name: "reordered canonical flags",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -data-dir /out/renderer-data -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml > /out/renderer-build-receipt.json
FROM alpine:3.24
`,
		},
		{
			name: "wrong data dir",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /wrong/renderer-data > /out/renderer-build-receipt.json
FROM alpine:3.24
`,
		},
		{
			name: "missing receipt redirection",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data
FROM alpine:3.24
`,
		},
		{
			name: "wrong receipt path",
			dockerfile: `FROM golang:1.26.5-alpine AS build
RUN /out/manja build -renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml -data-dir /out/renderer-data > /wrong/receipt.json
FROM alpine:3.24
`,
		},
	}

	committedConfigDir, err := filepath.Abs(filepath.Join("..", "..", "..", "internal", "renderer", "testdata", "kubernetes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			configDir := filepath.Join(root, "internal", "renderer", "testdata", "kubernetes")
			if err := os.MkdirAll(filepath.Dir(configDir), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(committedConfigDir, configDir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(test.dockerfile), 0o600); err != nil {
				t.Fatal(err)
			}

			source := loadCommittedStripeRendererBuildSource(t, root)
			err := validateStripeOpenAPIProvenance(approvedStripeOpenAPIProvenance, source)
			if test.wantValid && err != nil {
				t.Fatalf("valid Dockerfile rejected: %v", err)
			}
			if !test.wantValid && err == nil {
				t.Fatalf("unsafe Dockerfile accepted; approved path %q must bind the effective build command", approvedPath)
			}
		})
	}
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
	dockerConfigPath, dockerErr := strictStripeDockerConfigPath(dockerfile)
	if dockerErr != nil {
		t.Logf("Dockerfile binding rejected: %v", dockerErr)
	} else {
		source.DockerConfigPath = dockerConfigPath
	}
	return source
}

func strictStripeDockerConfigPath(dockerfile []byte) (string, error) {
	instructions, err := logicalDockerfileInstructions(dockerfile)
	if err != nil {
		return "", err
	}

	buildStageCount := 0
	buildCommandCount := 0
	configPath := ""
	inBuildStage := false
	for _, instruction := range instructions {
		fields := strings.Fields(instruction)
		if len(fields) == 0 {
			continue
		}
		keyword := strings.ToUpper(fields[0])
		arguments := strings.TrimSpace(instruction[len(fields[0]):])
		switch keyword {
		case "FROM":
			inBuildStage = false
			fromFields := strings.Fields(arguments)
			asIndex := -1
			for index, field := range fromFields {
				if !strings.EqualFold(field, "AS") {
					continue
				}
				if asIndex >= 0 {
					return "", fmt.Errorf("Dockerfile FROM instruction has ambiguous stage aliases")
				}
				asIndex = index
			}
			if asIndex >= 0 && (asIndex == 0 || asIndex != len(fromFields)-2) {
				return "", fmt.Errorf("Dockerfile FROM stage alias has unsupported syntax")
			}
			if asIndex >= 0 && fromFields[asIndex+1] == "build" {
				buildStageCount++
				inBuildStage = true
			}
		case "RUN":
			if !inBuildStage {
				continue
			}
			path, isBuildCommand, err := strictStripeBuildCommandConfigPath(arguments)
			if err != nil {
				return "", err
			}
			if isBuildCommand {
				buildCommandCount++
				configPath = path
			}
		}
	}

	if buildStageCount != 1 {
		return "", fmt.Errorf("Dockerfile build stage count = %d, want 1", buildStageCount)
	}
	if buildCommandCount != 1 {
		return "", fmt.Errorf("Dockerfile /out/manja build command count = %d, want 1", buildCommandCount)
	}
	return configPath, nil
}

func logicalDockerfileInstructions(dockerfile []byte) ([]string, error) {
	lines := strings.Split(strings.ReplaceAll(string(dockerfile), "\r\n", "\n"), "\n")
	instructions := make([]string, 0, len(lines))
	pending := ""
	continued := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		continued = strings.HasSuffix(trimmed, "\\")
		if continued {
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "\\"))
		}
		if pending == "" {
			pending = trimmed
		} else {
			pending += " " + trimmed
		}
		if !continued {
			instructions = append(instructions, pending)
			pending = ""
		}
	}
	if pending != "" || continued {
		return nil, fmt.Errorf("Dockerfile ends during a line continuation")
	}
	return instructions, nil
}

func strictStripeBuildCommandConfigPath(command string) (string, bool, error) {
	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "/out/manja" || fields[1] != "build" {
		return "", false, nil
	}
	want := []string{
		"/out/manja",
		"build",
		"-renderer-config",
		approvedStripeRendererBuildSource.DockerConfigPath,
		"-data-dir",
		"/out/renderer-data",
		">",
		"/out/renderer-build-receipt.json",
	}
	if len(fields) != len(want) {
		return "", true, fmt.Errorf("Dockerfile /out/manja build token count = %d, want %d", len(fields), len(want))
	}
	for index := range want {
		if fields[index] != want[index] {
			return "", true, fmt.Errorf("Dockerfile /out/manja build token %d = %q, want %q", index, fields[index], want[index])
		}
	}
	return approvedStripeRendererBuildSource.DockerConfigPath, true, nil
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
