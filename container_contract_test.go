package manja_test

import (
	"os"
	"strings"
	"testing"
)

func TestContainerImageContract(t *testing.T) {
	dockerfile := readFile(t, "Dockerfile")
	assertContains(t, dockerfile, "FROM golang:1.26.1-alpine AS build")
	assertContains(t, dockerfile, "FROM alpine:")
	assertContains(t, dockerfile, "ARG MANJA_VERSION=dev")
	assertContains(t, dockerfile, "CGO_ENABLED=0 GOOS=linux go build")
	assertContains(t, dockerfile, `-X main.version=${MANJA_VERSION}`)
	assertContains(t, dockerfile, "apk add --no-cache ca-certificates git")
	assertContains(t, dockerfile, "internal/web/static")
	assertContains(t, dockerfile, "internal/adapters/openapi/testdata/github-v3-rest.json")
}

func TestContainerPublishWorkflowContract(t *testing.T) {
	workflow := readFile(t, ".github/workflows/ci.yml")
	assertContains(t, workflow, "tags:")
	assertContains(t, workflow, "'v*.*.*'")
	assertContains(t, workflow, "permissions:")
	assertContains(t, workflow, "packages: write")
	assertContains(t, workflow, "registry: ghcr.io")
	assertContains(t, workflow, "images: ghcr.io/${{ github.repository }}")
	assertContains(t, workflow, "latest=auto")
	assertContains(t, workflow, "type=raw,value=main,enable=${{ github.ref == 'refs/heads/main' }}")
	assertContains(t, workflow, "type=sha,format=long,prefix=,enable=${{ github.ref == 'refs/heads/main' }}")
	assertContains(t, workflow, "type=semver,pattern={{version}}")
	assertContains(t, workflow, "type=semver,pattern={{major}}.{{minor}}")
	assertContains(t, workflow, "type=semver,pattern={{major}}")
	assertContains(t, workflow, "build-args: |")
	assertContains(t, workflow, "MANJA_VERSION=${{ github.ref_type == 'tag' && github.ref_name || github.sha }}")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q", needle)
	}
}
