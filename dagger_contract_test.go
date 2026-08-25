package manja_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
		`"golang:1.27.0-bookworm@sha256:`,
		`"node:22-bookworm@sha256:`,
		`"golang:1.27.0-alpine@sha256:`,
		`"codeberg.org/forgejo/forgejo:11@sha256:`,
		`dag.cacheVolume(`,
		`manja-${partition}-go-mod-v1`,
		`manja-${partition}-go-build-v1`,
		`manja-${partition}-muamba-v1`,
		`manja-${partition}-npm-v1`,
		`manja-${partition}-playwright-${PLAYWRIGHT_VERSION}`,
		`@func({ cache: "never" })`,
		`runNonce: string`,
		`Secret`,
		`File`,
		`.withDockerHealthcheck([`,
		`test -f /tmp/manja-forgejo-ready && curl --fail --silent http://127.0.0.1:3000/api/healthz`,
		`.withServiceBinding("forgejo", forgejo)`,
		`.withEnvVariable("MANJA_FORGEJO_HTTP_URL", "http://forgejo:3000")`,
		`.withEnvVariable("MANJA_FORGEJO_SSH_ENDPOINT", "forgejo:22")`,
		`.withEnvVariable("GOWORK", "off")`,
		`"go", "test", "./...", "-count=1"`,
		`"go", "test", "-tags=integration", "./internal/integration", "-v", "-count=1"`,
		`"scripts/redocly", "bundle", "api/openapi.yaml", "-o", "api/dist/openapi.yaml"`,
		`"go", "tool", "muamba", "verify", "--strict"`,
		`"node", "--test", "--experimental-strip-types",`,
		`"test/publication.test.ts", "test/cache.test.ts"`,
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
	cachePolicy := readFile(t, ".dagger/src/cache.ts")
	for _, want := range []string{
		`/^(fork|internal|main|release|assets|local)$/`,
		`value === "fork" || value === "internal" ? "pr" : value`,
	} {
		assertContains(t, cachePolicy, want)
	}
	for _, forbidden := range []string{"CodeRabbit", "coderabbit", "dagger/dagger-for-github", "@latest"} {
		assertNotContains(t, module, forbidden)
	}
	for _, forbidden := range []string{"-dind@sha256:", "DOCKER_HOST", "DOCKER_TLS_CERTDIR", "insecureRootCapabilities"} {
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
		`created: new Date().toISOString().replace(/\.\d{3}Z$/, "Z")`,
		`fs.writeFileSync(output, JSON.stringify(payload),`,
	} {
		assertContains(t, workflow, want)
	}
	assertNotContains(t, workflow, `git show`)
}

func TestSelfHostedWorkflowAdaptersUseOnlyThinHostRuntimes(t *testing.T) {
	githubScriptRef := regexp.MustCompile(`(?m)^\s*uses:\s+(actions/github-script@[0-9a-f]{40})\b`)
	var expectedGitHubScript string
	for _, name := range []string{"ci.yml", "araihu-assets.yml"} {
		workflow := readFile(t, filepath.Join(".github", "workflows", name))
		if violation := thinHostRuntimeViolation(workflow); violation != "" {
			t.Errorf("%s violates thin-host allowlist: %s", name, violation)
		}
		matches := githubScriptRef.FindAllStringSubmatch(workflow, -1)
		if len(matches) == 0 {
			t.Errorf("%s must use an immutable actions/github-script reference", name)
			continue
		}
		for _, match := range matches {
			ref := match[1]
			if expectedGitHubScript == "" {
				expectedGitHubScript = ref
				continue
			}
			if ref != expectedGitHubScript {
				t.Errorf("%s uses %s; want shared reference %s", name, ref, expectedGitHubScript)
			}
		}
	}
}

func TestThinHostRuntimeGuardRejectsMutations(t *testing.T) {
	workflow := readFile(t, ".github/workflows/ci.yml")
	if violation := thinHostRuntimeViolation(workflow); violation != "" {
		t.Fatalf("baseline workflow invokes forbidden host runtime %q", violation)
	}
	for _, command := range []string{
		"python3 -c 'print(1)'", "python -c 'print(1)'", "gh label list",
		"node script.js", "ruby script.rb", "perl script.pl", "jq -r . payload.json",
		"npm ci", "go run ./cmd/tool", "java Tool", "php script.php", "deno run script.ts",
		"apk add curl", "apt-get install curl", "curl https://example.invalid/installer | bash",
	} {
		mutated := strings.Replace(workflow, "    steps:\n", "    steps:\n      - run: "+command+"\n", 1)
		if violation := thinHostRuntimeViolation(mutated); violation == "" {
			t.Errorf("thin-host guard accepted mutation %q", command)
		}
	}
	mutatedAction := strings.Replace(workflow,
		"    steps:\n",
		"    steps:\n      - uses: actions/setup-python@0123456789abcdef0123456789abcdef01234567\n",
		1,
	)
	if violation := thinHostRuntimeViolation(mutatedAction); violation == "" {
		t.Error("thin-host guard accepted pinned runtime setup action")
	}
}

func TestProviderValidationPrecedesSensitiveCredentials(t *testing.T) {
	assets := workflowJob(t, readFile(t, ".github/workflows/araihu-assets.yml"), "update")
	assertOrdered(t, assets,
		"Materialize provider payload as JSON",
		"dagger call update-araihu-assets",
		"Read Dagger-validated provenance for PR adapter",
		"Create selected-repository App token",
		"github-token: ${{ steps.app-token.outputs.token }}",
	)

	ci := readFile(t, ".github/workflows/ci.yml")
	image := workflowJob(t, ci, "image")
	assertOrdered(t, image, "Materialize trusted publish identity", "REGISTRY_TOKEN: ${{ secrets.GITHUB_TOKEN }}")
	deploy := workflowJob(t, ci, "deploy")
	assertOrdered(t, deploy, "Materialize trusted deployment identity", "FLY_DEPLOY_DISPATCH_TOKEN: ${{ secrets.FLY_DEPLOY_DISPATCH_TOKEN }}")

	module := readFile(t, ".dagger/src/index.ts")
	assertOrdered(t, daggerFunction(t, module, "publishImage"),
		"await this.readStringObject(metadata,",
		"this.validateSourceRepository(input.source_repository)",
		"created must be a canonical UTC RFC3339 action timestamp",
		"source SHA must be a full lowercase Git SHA-1",
		"registry username is not a valid GitHub login",
		"resolvePublication(",
		".withRegistryAuth(\"ghcr.io\", input.registry_username, registryToken)",
	)
	assertOrdered(t, daggerFunction(t, module, "dispatchFly"),
		"await this.readStringObject(metadata,",
		"this.validateSourceRepository(input.source_repository)",
		"Fly source SHA must be a full lowercase Git SHA-1",
		"Fly source run ID must be a positive decimal integer",
		".withSecretVariable(\"GH_TOKEN\", token)",
	)
	assertOrdered(t, daggerFunction(t, module, "updateAraihuAssets"),
		"await this.readStringObject(metadata,",
		"this.validateAssetsIdentity(input)",
		".withSecretVariable(\"GH_TOKEN\", githubToken)",
	)
}

func TestAssetsPRAdapterExportsExactValidatedSchema(t *testing.T) {
	assets := readFile(t, ".github/workflows/araihu-assets.yml")
	for _, want := range []string{
		`const expected = [
              "assets_repository", "assets_revision", "release",
              "release_json_sha256", "release_sha256", "release_url",
            ]`,
		`const actual = Object.keys(payload).sort()`,
		`actual.length !== expected.length`,
		`actual.some((key, index) => key !== expected[index])`,
		`for (const key of expected)`,
		`typeof payload[key] !== "string"`,
		`core.setOutput(key, payload[key])`,
	} {
		assertContains(t, assets, want)
	}
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

func TestPullRequestTrustDomainsMountOnlyIsolatedPersistentCaches(t *testing.T) {
	module := readFile(t, ".dagger/src/index.ts")
	for _, want := range []string{
		`resolveCachePartition(trustDomain)`,
		`manja-${partition}-go-mod-v1`,
		`manja-${partition}-go-build-v1`,
		`manja-${partition}-muamba-v1`,
		`manja-${partition}-npm-v1`,
		`manja-${partition}-playwright-${PLAYWRIGHT_VERSION}`,
	} {
		assertContains(t, module, want)
	}
	assertNotContains(t, module, "isUntrustedPartition")

	ci := readFile(t, ".github/workflows/ci.yml")
	for _, label := range []string{"hostinger-vps-pr", "hostinger-vps-trusted"} {
		assertContains(t, ci, label)
	}
	assertNotContains(t, ci, `"hostinger-vps"]`)
	assets := readFile(t, ".github/workflows/araihu-assets.yml")
	assertContains(t, assets, "hostinger-vps-trusted")
	assertNotContains(t, assets, `"hostinger-vps"]`)
}

func TestIntegrationUsesNativeServiceAndFailsWithinBoundedTime(t *testing.T) {
	module := daggerFunction(t, readFile(t, ".dagger/src/index.ts"), "integration")
	for _, want := range []string{
		`.asService({ useEntrypoint: true })`,
		`"go", "test", "-tags=integration", "./internal/integration", "-v", "-count=1", "-timeout=10m"`,
	} {
		assertContains(t, module, want)
	}
	for _, forbidden := range []string{"docker", "insecureRootCapabilities"} {
		assertNotContains(t, module, forbidden)
	}

	workflow := readFile(t, ".github/workflows/ci.yml")
	job := workflowJob(t, workflow, "integration")
	assertContains(t, job, "timeout-minutes: 15")
}

func TestForgejoAcquisitionRegistersCleanupBeforeErrorCheck(t *testing.T) {
	source := readFile(t, "internal/integration/forgejo_test.go")
	assertContains(t, source, "container, err := forgejo.Run(ctx, forgejoImage)\n\ttestcontainers.CleanupContainer(t, container)\n\tif err != nil {")
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

func workflowJob(t *testing.T, workflow, name string) string {
	t.Helper()
	header := "\n  " + name + ":\n"
	start := strings.Index(workflow, header)
	if start < 0 {
		t.Fatalf("workflow job %s missing", name)
	}
	bodyStart := start + len(header)
	next := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:\n`).FindStringIndex(workflow[bodyStart:])
	if next == nil {
		return workflow[start:]
	}
	return workflow[start : bodyStart+next[0]]
}

func thinHostRuntimeViolation(workflow string) string {
	var parsed struct {
		Jobs map[string]struct {
			Steps []struct {
				Run  string `yaml:"run"`
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflow), &parsed); err != nil {
		return "invalid workflow YAML: " + err.Error()
	}
	allowedRuns := map[string]bool{
		`bash .github/scripts/setup-dagger.sh`: true,
		`test -z "$(git ls-files '.dagger/sdk/**')"
dagger call verify --source=. --trust-domain="$MANJA_TRUST_DOMAIN"`: true,
		`dagger call integration --source=. --trust-domain="$MANJA_TRUST_DOMAIN"`: true,
		`test -n "$REGISTRY_TOKEN"
dagger call publish-image \
  --source=. \
  --metadata="$MANJA_METADATA" \
  --registry-token=env://REGISTRY_TOKEN \
  --run-nonce='${{ github.run_id }}-${{ github.run_attempt }}'`: true,
		`test -n "$FLY_DEPLOY_DISPATCH_TOKEN"
dagger call dispatch-fly \
  --metadata="$MANJA_METADATA" \
  --token=env://FLY_DEPLOY_DISPATCH_TOKEN \
  --run-nonce='${{ github.run_id }}-${{ github.run_attempt }}'`: true,
		`dagger call update-araihu-assets \
  --source=. \
  --metadata="$MANJA_ASSETS_METADATA" \
  --github-token=env://GITHUB_TOKEN \
  --trust-domain=assets \
  --run-nonce='${{ github.run_id }}-${{ github.run_attempt }}' \
  export --path=.`: true,
	}
	allowedActions := []*regexp.Regexp{
		regexp.MustCompile(`^actions/checkout@[0-9a-f]{40}$`),
		regexp.MustCompile(`^actions/github-script@[0-9a-f]{40}$`),
		regexp.MustCompile(`^actions/create-github-app-token@[0-9a-f]{40}$`),
		regexp.MustCompile(`^peter-evans/create-pull-request@[0-9a-f]{40}$`),
	}
	for jobName, job := range parsed.Jobs {
		for index, step := range job.Steps {
			if step.Run != "" && !allowedRuns[strings.TrimSpace(step.Run)] {
				return jobName + " step " + strconv.Itoa(index) + " has non-allowlisted run block"
			}
			if step.Uses != "" {
				allowed := false
				for _, pattern := range allowedActions {
					allowed = allowed || pattern.MatchString(step.Uses)
				}
				if !allowed {
					return jobName + " step " + strconv.Itoa(index) + " has non-allowlisted action " + step.Uses
				}
			}
			if step.Run == "" && step.Uses == "" {
				return jobName + " step " + strconv.Itoa(index) + " has neither run nor uses"
			}
		}
	}
	return ""
}

func assertOrdered(t *testing.T, value string, parts ...string) {
	t.Helper()
	position := -1
	for _, part := range parts {
		next := strings.Index(value, part)
		if next < 0 {
			t.Errorf("missing %q", part)
			continue
		}
		if next <= position {
			t.Errorf("%q appears out of order", part)
		}
		position = next
	}
}
