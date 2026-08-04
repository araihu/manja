package manja_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestContainerImageContract(t *testing.T) {
	dockerfile := readFile(t, "Dockerfile")
	assertContains(t, dockerfile, "FROM golang:"+moduleGoVersion(t)+"-alpine AS build")
	assertContains(t, dockerfile, "FROM alpine:")
	assertContains(t, dockerfile, "ARG MANJA_VERSION=dev")
	assertContains(t, dockerfile, "./cmd/manja-runtime")
	assertContains(t, dockerfile, "-tags=manja_runtime")
	assertContains(t, dockerfile, `-X main.version=${MANJA_VERSION}`)
	assertContains(t, dockerfile, "manja build")
	assertContains(t, dockerfile, "-renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml")
	assertContains(t, dockerfile, "-data-dir /out/renderer-data")
	assertContains(t, dockerfile, "COPY --from=build /out/renderer-data /app/renderer-data")
	assertContains(t, dockerfile, "internal/renderer/testdata/kubernetes/renderer.yaml")
	assertContains(t, dockerfile, "internal/renderer/testdata/kubernetes/default-allowlist.json")
	assertContains(t, dockerfile, "internal/web/static")
	assertContains(t, dockerfile, `CMD ["-addr", ":8080", "-renderer-config", "/app/renderer/renderer.yaml", "-data-dir", "/app/renderer-data"]`)
	assertNotContains(t, dockerfile, "internal/adapters/openapi/testdata/github-v3-rest.json")
	assertNotContains(t, dockerfile, `CMD ["-addr", ":8080", "-spec"`)
	finalStage := dockerfile[strings.LastIndex(dockerfile, "FROM alpine:"):]
	assertNotContains(t, finalStage, " git")
	assertNotContains(t, finalStage, "/out/manja ")
}

func TestRuntimeOnlyBuildExcludesSourceAndOpenAPICompilerPackages(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-tags=manja_runtime", "./cmd/manja-runtime")
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list runtime-only dependencies: %v: %s", err, output)
	}
	dependencies := string(output)
	for _, forbidden := range []string{
		"github.com/araihu/manja/internal/adapters/openapi",
		"github.com/araihu/manja/internal/adapters/source",
		"github.com/getkin/kin-openapi",
	} {
		assertNotContains(t, dependencies, forbidden)
	}
}

func moduleGoVersion(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(readFile(t, "go.mod"), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	t.Fatal("go.mod has no go directive")
	return ""
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

func assertNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("unexpected %q", needle)
	}
}
