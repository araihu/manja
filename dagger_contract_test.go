package manja_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDaggerConfigurationAndRuntimeContract(t *testing.T) {
	config := readFile(t, "dagger.json")
	for _, want := range []string{`"engineVersion": "v0.21.8"`, `"source": ".dagger"`, `"source": "typescript"`} {
		assertContains(t, config, want)
	}
	packageJSON := readFile(t, ".dagger/package.json")
	assertContains(t, packageJSON, `"@dagger.io/dagger": "./sdk"`)
	assertContains(t, packageJSON, `"typescript": "6.0.3"`)
	assertNotContains(t, packageJSON, `"devDependencies"`)
	packageLock := readFile(t, ".dagger/package-lock.json")
	assertContains(t, packageLock, `"@dagger.io/dagger": "./sdk"`)
	assertContains(t, packageLock, `"resolved": "sdk"`)
	trackedSDK, err := exec.Command("git", "ls-files", ".dagger/sdk/**").Output()
	if err != nil {
		t.Fatalf("list tracked generated SDK: %v", err)
	}
	if len(trackedSDK) != 0 {
		t.Fatalf("generated Dagger SDK must not be versioned:\n%s", trackedSDK)
	}
}

func TestDaggerModulePreservesPipelineBoundaries(t *testing.T) {
	module := readFile(t, ".dagger/src/index.ts")
	for _, function := range []string{"verify", "integration", "image", "publishImage", "dispatchFly", "updateAraihuAssets"} {
		if !strings.Contains(module, "  "+function+"(") && !strings.Contains(module, "  async "+function+"(") {
			t.Errorf("Dagger function %s missing", function)
		}
	}
	for _, want := range []string{
		`"golang:1.26.5-bookworm@sha256:`,
		`"node:22-bookworm@sha256:`,
		`"golang:1.26.5-alpine@sha256:`,
		`"docker:29.1.3-dind@sha256:`,
		`dag.cacheVolume(`,
		`manja-${partition}-go-mod-v1`,
		`manja-${partition}-go-build-v1`,
		`manja-${partition}-muamba-v1`,
		`manja-${partition}-npm-v1`,
		`manja-${partition}-playwright-${PLAYWRIGHT_VERSION}`,
		`/^(fork|internal|main|release|assets|local)$/`,
		`@func({ cache: "never" })`,
		`runNonce: string`,
		`Secret`,
		`File`,
		`insecureRootCapabilities`,
		`.withEnvVariable("GOWORK", "off")`,
		`"go", "test", "./...", "-count=1"`,
		`"go", "test", "-tags=integration", "./internal/integration", "-v", "-count=1"`,
		`"scripts/redocly", "bundle", "api/openapi.yaml", "-o", "api/dist/openapi.yaml"`,
		`"go", "tool", "muamba", "verify", "--strict"`,
		`"node", "--test", "--experimental-strip-types", "test/publication.test.ts"`,
		`"npm", "audit", "--package-lock-only", "--omit=dev", "--audit-level=high"`,
		`git ls-files '.dagger/sdk/**'`,
		`.dockerBuild({`,
		`.withRegistryAuth(`,
		`.publish(`,
		`https://api.github.com/repos/araihu/fly-deploy/dispatches`,
		`dag.http(releaseUrl`,
	} {
		assertContains(t, module, want)
	}
	for _, forbidden := range []string{"CodeRabbit", "coderabbit", "dagger/dagger-for-github", "@latest"} {
		assertNotContains(t, module, forbidden)
	}
}

func TestPublishImageCarriesStandardOCIMetadata(t *testing.T) {
	module := daggerFunction(t, readFile(t, ".dagger/src/index.ts"), "publishImage")
	assertContains(t, module, `"created", "ref_name", "ref_type", "registry_username", "source_repository"`)
	assertContains(t, module, `created must be a canonical UTC RFC3339 action timestamp`)
	for _, label := range []string{
		"created", "description", "licenses", "revision", "source", "title", "url", "version",
	} {
		assertContains(t, module, `.withLabel("org.opencontainers.image.`+label+`"`)
	}
	workflow := readFile(t, ".github/workflows/ci.yml")
	for _, want := range []string{
		`"created": created`,
		`datetime.now(timezone.utc).isoformat(timespec="seconds")`,
	} {
		assertContains(t, workflow, want)
	}
	assertNotContains(t, workflow, `git show`)
}

func TestDaggerEffectFunctionsAreFreshAndStrict(t *testing.T) {
	module := readFile(t, ".dagger/src/index.ts")
	for _, name := range []string{"publishImage", "dispatchFly", "updateAraihuAssets"} {
		body := daggerFunction(t, module, name)
		if !strings.HasPrefix(strings.TrimSpace(body), `@func({ cache: "never" })`) {
			t.Errorf("%s is not cache=never", name)
		}
		assertContains(t, body, "runNonce: string")
		assertContains(t, body, "validateRunNonce(runNonce)")
	}
	for _, name := range []string{"verify", "integration", "image"} {
		body := daggerFunction(t, module, name)
		assertNotContains(t, body, `cache: "never"`)
		assertNotContains(t, body, "runNonce: string")
	}
	assertContains(t, module, `metadata fields do not match the strict schema`)
	assertContains(t, module, `source repository must be araihu/manja`)
}

func TestPullRequestTrustDomainsNeverMountPersistentCaches(t *testing.T) {
	module := readFile(t, ".dagger/src/index.ts")
	for _, want := range []string{
		`return value === "fork" || value === "internal"`,
		`if (this.isUntrustedPartition(partition))`,
		`if (!this.isUntrustedPartition(partition))`,
	} {
		assertContains(t, module, want)
	}
	for _, workflow := range []string{
		readFile(t, ".github/workflows/ci.yml"),
		readFile(t, ".github/workflows/araihu-assets.yml"),
	} {
		assertNotContains(t, workflow, "dag.cacheVolume")
	}
}

func TestWorkflowAdaptersUseExactDaggerCLIAndDirectCalls(t *testing.T) {
	for _, test := range []struct {
		name   string
		setups int
		calls  int
	}{
		{"ci.yml", 4, 4},
		{"araihu-assets.yml", 1, 1},
	} {
		workflow := readFile(t, filepath.Join(".github", "workflows", test.name))
		assertNotContains(t, workflow, "dagger/dagger-for-github")
		assertNotContains(t, strings.ToLower(workflow), "coderabbit")
		if got := strings.Count(workflow, "run: bash .github/scripts/setup-dagger.sh"); got != test.setups {
			t.Errorf("%s setup gates = %d, want %d", test.name, got, test.setups)
		}
		if got := strings.Count(workflow, "dagger call "); got != test.calls {
			t.Errorf("%s Dagger calls = %d, want %d", test.name, got, test.calls)
		}
		if firstSetup, firstCall := strings.Index(workflow, "run: bash .github/scripts/setup-dagger.sh"), strings.Index(workflow, "dagger call "); firstSetup < 0 || firstCall < firstSetup {
			t.Errorf("%s calls Dagger before exact CLI gate", test.name)
		}
		for _, match := range regexp.MustCompile(`(?m)^\s*uses:\s*([^\s#]+)`).FindAllStringSubmatch(workflow, -1) {
			parts := strings.Split(match[1], "@")
			if len(parts) != 2 || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(parts[1]) {
				t.Errorf("%s action is not pinned to a full SHA: %s", test.name, match[1])
			}
		}
	}
}

func TestWorkflowAdaptersUseTypedExternalInputsAndTrustPartitions(t *testing.T) {
	ci := readFile(t, ".github/workflows/ci.yml")
	for _, want := range []string{
		"MANJA_TRUST_DOMAIN:",
		"persist-credentials: false",
		`test -z "$(git ls-files '.dagger/sdk/**')"`,
		"--trust-domain=\"$MANJA_TRUST_DOMAIN\"",
		"--metadata=\"$MANJA_METADATA\"",
		`test -n "$REGISTRY_TOKEN"`,
		"--registry-token=env://REGISTRY_TOKEN",
		`test -n "$FLY_DEPLOY_DISPATCH_TOKEN"`,
		"--token=env://FLY_DEPLOY_DISPATCH_TOKEN",
		"--run-nonce='${{ github.run_id }}-${{ github.run_attempt }}'",
	} {
		assertContains(t, ci, want)
	}
	assets := readFile(t, ".github/workflows/araihu-assets.yml")
	for _, want := range []string{
		"--metadata=\"$MANJA_ASSETS_METADATA\"",
		"--github-token=env://GITHUB_TOKEN",
		"--trust-domain=assets",
		"export --path=.",
		"peter-evans/create-pull-request@",
		"This workflow never auto-merges.",
	} {
		assertContains(t, assets, want)
	}
	for _, workflow := range []string{ci, assets} {
		assertNotContains(t, workflow, "actions/setup-go@")
		assertNotContains(t, workflow, "actions/setup-node@")
		assertNotContains(t, workflow, "docker/build-push-action@")
	}
}

func TestExactDaggerSetupScript(t *testing.T) {
	scriptPath := filepath.Join(".github", "scripts", "setup-dagger.sh")
	script := readFile(t, scriptPath)
	for _, want := range []string{
		`expected_version="v0.21.8"`,
		"dagger_v0.21.8_linux_amd64.tar.gz",
		"53e226c7da8fb75171e58c35759d736d961ce8b3a12db0baa7b7107954fccc5a",
		"dagger_v0.21.8_linux_arm64.tar.gz",
		"cd0df4885f2050082932b4abc5a6aad9a733f6aa4e7d8474740558517ffec4af",
		"sha256sum --check --strict",
		`RUNNER_ENVIRONMENT" != "self-hosted"`,
		`[[ "$resolved_version" != "$expected_version" ]]`,
	} {
		assertContains(t, script, want)
	}

	for _, test := range []struct {
		version string
		ok      bool
	}{{"v0.21.8", true}, {"v0.21.7", false}, {"v0.22.0", false}} {
		bin := t.TempDir()
		dagger := filepath.Join(bin, "dagger")
		if err := os.WriteFile(dagger, []byte("#!/bin/sh\nprintf 'dagger "+test.version+" (test) linux/amd64\\n'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("bash", scriptPath)
		command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "RUNNER_ENVIRONMENT=self-hosted")
		err := command.Run()
		if (err == nil) != test.ok {
			t.Errorf("self-hosted Dagger %s accepted=%t, want %t", test.version, err == nil, test.ok)
		}
	}
}

func daggerFunction(t *testing.T, module, name string) string {
	t.Helper()
	start := strings.Index(module, "\n  "+name+"(")
	if start < 0 {
		start = strings.Index(module, "\n  async "+name+"(")
	}
	if start < 0 {
		t.Fatalf("Dagger function %s missing", name)
	}
	annotation := strings.LastIndex(module[:start], "\n  @func")
	if annotation < 0 {
		t.Fatalf("Dagger function %s annotation missing", name)
	}
	next := strings.Index(module[start+1:], "\n  @func")
	if next < 0 {
		next = strings.Index(module[start+1:], "\n  private ")
	}
	if next < 0 {
		next = len(module) - start - 1
	}
	return module[annotation+1 : start+1+next]
}
