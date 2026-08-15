import {
  argument,
  Container,
  dag,
  Directory,
  File,
  func,
  object,
  Secret,
} from "@dagger.io/dagger"

import { assertOCIPublicationGate, resolvePublication } from "./publication.js"
import { resolveCachePartition } from "./cache.js"

const GO_IMAGE =
  "golang:1.26.5-bookworm@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd"
const NODE_IMAGE =
  "node:22-bookworm@sha256:0557ac14e0d45d02ed563067b82856ca5e7aa3437fa28d98d4350ea9c3d9494a"
const GO_BUILD_IMAGE =
  "golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2"
const FORGEJO_IMAGE =
  "codeberg.org/forgejo/forgejo:11@sha256:946243edbab116d5bb78b73ea68af6f3d69229ba1b1ed958dd82c3481167f3e0"
const ALPINE_IMAGE =
  "alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b"
const PLAYWRIGHT_VERSION = "v0.6100.0"
const OCI_DESCRIPTION = "Hosted OpenAPI renderer and publisher built with Goshtoso"
const OCI_LICENSES = ""
// OC-01 is still blocked: no first-party authority, redistribution, or
// trademark clearance receipt has been verified for the actual image bytes.
// Keep the publication boundary fail-closed until a reviewed legal gate
// supplies PASS for the exact image/platform digest set.
const OCI_LEGAL_GATE_STATUS = "BLOCKED"
const OCI_SOURCE = "https://github.com/araihu/manja"
const OCI_TITLE = "manja"
const OCI_URL = "https://github.com/araihu/manja"
const FORGEJO_ADMIN_USERNAME = "forgejo-admin"
const FORGEJO_ADMIN_PASSWORD = "forgejo-admin"
const FORGEJO_ENTRYPOINT = `#!/bin/sh
set -eu

/usr/bin/entrypoint &
service_pid=$!
cleanup() {
  kill "$service_pid" 2>/dev/null || true
  wait "$service_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

attempt=0
until curl --fail --silent http://127.0.0.1:3000/api/healthz >/dev/null; do
  if ! kill -0 "$service_pid" 2>/dev/null; then
    wait "$service_pid"
    exit $?
  fi
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    echo "Forgejo did not become ready within 60 seconds" >&2
    exit 1
  fi
  sleep 1
done

su-exec git forgejo admin user create \
  --username "${FORGEJO_ADMIN_USERNAME}" \
  --password "${FORGEJO_ADMIN_PASSWORD}" \
  --email admin@forgejo.local \
  --admin \
  --must-change-password=false
touch /tmp/manja-forgejo-ready

wait "$service_pid"
trap - EXIT INT TERM
`

const SOURCE_EXCLUDES = [
  ".cache",
  ".cache/**",
  ".dagger/node_modules",
  ".dagger/node_modules/**",
  ".dagger/sdk",
  ".dagger/sdk/**",
  ".dagger-inputs",
  ".dagger-inputs/**",
  ".git",
  ".git/**",
  ".manja",
  ".manja/**",
  "api/dist",
  "api/dist/**",
  "bin",
  "bin/**",
  "tmp",
  "tmp/**",
]

const ASSET_OUTPUTS = [
  "araihu-assets.json",
  "internal/web/static/araihu.css",
  "internal/web/static/favicon.svg",
  "internal/web/static/manja-mark.svg",
  "site/internal/site/static/araihu.css",
  "site/internal/site/static/favicon.svg",
  "site/internal/site/static/manja-logo.svg",
  "site/internal/site/static/manja-mark.svg",
] as const

@object()
export class Manja {
  /** Run root and standalone-site generation, dependency, API, and test gates. */
  @func()
  async verify(
    @argument({ defaultPath: ".", ignore: SOURCE_EXCLUDES }) source: Directory,
    trustDomain: string,
  ): Promise<string> {
    const project = this.browserProject(source, trustDomain)
      .withWorkdir("/work/.dagger")
      .withExec([
        "node", "--test", "--experimental-strip-types",
        "test/publication.test.ts", "test/cache.test.ts",
      ])
      .withExec(["npm", "audit", "--package-lock-only", "--omit=dev", "--audit-level=high"])
      .withWorkdir("/work")
      .withExec(["go", "mod", "tidy"])
      .withExec(["git", "diff", "--exit-code", "--", "go.mod", "go.sum"])
      .withExec(["go", "tool", "muamba", "sync", "--strict"])
      .withExec(["go", "tool", "muamba", "verify", "--strict"])
      .withExec([
        "go", "tool", "muamba", "generate-go", "--strict", "--check",
        "--dir", "internal/webassets", "--output", "muamba_gen.go",
      ])
      .withExec(["go", "run", "./cmd/webassets", "check"])
      .withExec(["scripts/redocly", "bundle", "api/openapi.yaml", "-o", "api/dist/openapi.yaml"])
      .withExec(["scripts/redocly", "lint", "api/openapi.yaml"])
      .withExec([
        "go", "run", "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen",
        "-generate", "types,strict-server", "-package", "web",
        "-o", "internal/web/api.gen.go", "api/dist/openapi.yaml",
      ])
      .withExec(["go", "run", "github.com/a-h/templ/cmd/templ", "generate"])
      .withExec(["git", "diff", "--exit-code"])
      .withExec([
        "go", "run", `github.com/mxschmitt/playwright-go/cmd/playwright@${PLAYWRIGHT_VERSION}`,
        "install", "--with-deps", "chromium",
      ])
      .withExec(["go", "test", "./...", "-count=1"])
    return project
      .withWorkdir("/work/site")
      .withExec(["go", "mod", "tidy", "-diff"])
      .withExec(["go", "test", "./...", "-count=1"])
      .stdout()
  }

  /** Run integration tests against an isolated native Forgejo service. */
  @func()
  async integration(
    @argument({ defaultPath: ".", ignore: SOURCE_EXCLUDES }) source: Directory,
    trustDomain: string,
  ): Promise<string> {
    const forgejo = dag.container()
      .from(FORGEJO_IMAGE)
      .withEnvVariable("FORGEJO__database__DB_TYPE", "sqlite3")
      .withEnvVariable("FORGEJO__security__INSTALL_LOCK", "true")
      .withEnvVariable("FORGEJO__server__ROOT_URL", "http://forgejo:3000/")
      .withEnvVariable("FORGEJO__server__SSH_DOMAIN", "forgejo")
      .withEnvVariable("FORGEJO__server__SSH_PORT", "22")
      .withNewFile("/usr/local/bin/manja-forgejo-entrypoint", FORGEJO_ENTRYPOINT, {
        permissions: 0o755,
      })
      .withEntrypoint(["/usr/local/bin/manja-forgejo-entrypoint"])
      .withoutDefaultArgs()
      .withExposedPort(3000)
      .withExposedPort(22)
      .withDockerHealthcheck([
        "/bin/sh", "-ec",
        "test -f /tmp/manja-forgejo-ready && curl --fail --silent http://127.0.0.1:3000/api/healthz >/dev/null",
      ], { interval: "1s", retries: 15, startPeriod: "60s", timeout: "3s" })
      .asService({ useEntrypoint: true })

    return this.project(source, trustDomain)
      .withServiceBinding("forgejo", forgejo)
      .withEnvVariable("MANJA_FORGEJO_HTTP_URL", "http://forgejo:3000")
      .withEnvVariable("MANJA_FORGEJO_SSH_ENDPOINT", "forgejo:22")
      .withEnvVariable("MANJA_FORGEJO_ADMIN_USERNAME", FORGEJO_ADMIN_USERNAME)
      .withEnvVariable("MANJA_FORGEJO_ADMIN_PASSWORD", FORGEJO_ADMIN_PASSWORD)
      .withExec([
        "go", "test", "-tags=integration", "./internal/integration", "-v", "-count=1", "-timeout=10m",
      ])
      .stdout()
  }

  /** Build the production OCI image without publishing it. */
  @func()
  async image(
    @argument({ defaultPath: ".", ignore: SOURCE_EXCLUDES }) source: Directory,
    version = "dev",
  ): Promise<Container> {
    this.validateVersion(version)
    return this.buildImage(source, version)
  }

  /** Build and publish the main or stable-tag image set to GHCR. */
  @func({ cache: "never" })
  async publishImage(
    @argument({ defaultPath: ".", ignore: SOURCE_EXCLUDES }) source: Directory,
    metadata: File,
    registryToken: Secret,
    runNonce: string,
  ): Promise<string> {
    this.validateRunNonce(runNonce)
    assertOCIPublicationGate(OCI_LEGAL_GATE_STATUS)
    const input = await this.readStringObject(metadata, [
      "created", "ref_name", "ref_type", "registry_username", "source_repository",
      "source_sha",
    ])
    this.validateSourceRepository(input.source_repository)
    const createdEpoch = Date.parse(input.created)
    if (
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(input.created) ||
      Number.isNaN(createdEpoch) ||
      new Date(createdEpoch).toISOString().replace(".000Z", "Z") !== input.created
    ) {
      throw new Error("created must be a canonical UTC RFC3339 action timestamp")
    }
    if (!/^[0-9a-f]{40}$/.test(input.source_sha)) {
      throw new Error("source SHA must be a full lowercase Git SHA-1")
    }
    if (
      input.registry_username.length > 100 ||
      !/^[A-Za-z0-9][A-Za-z0-9-]*(?:\[bot\])?$/.test(input.registry_username)
    ) {
      throw new Error("registry username is not a valid GitHub login")
    }

    const { buildVersion, ociVersion, tags } = resolvePublication(
      input.ref_type,
      input.ref_name,
      input.source_sha,
    )

    const image = (await this.buildImage(source, buildVersion))
      .withLabel("org.opencontainers.image.created", input.created)
      .withLabel("org.opencontainers.image.description", OCI_DESCRIPTION)
      .withLabel("org.opencontainers.image.licenses", OCI_LICENSES)
      .withLabel("org.opencontainers.image.revision", input.source_sha)
      .withLabel("org.opencontainers.image.source", OCI_SOURCE)
      .withLabel("org.opencontainers.image.title", OCI_TITLE)
      .withLabel("org.opencontainers.image.url", OCI_URL)
      .withLabel("org.opencontainers.image.version", ociVersion)
      .withEnvVariable("MANJA_RUN_NONCE", runNonce)
      .withExec(["true"])
      .withoutEnvVariable("MANJA_RUN_NONCE")
      .withRegistryAuth("ghcr.io", input.registry_username, registryToken)
    const references = await Promise.all(
      tags.map((tag) => image.publish(`ghcr.io/araihu/manja:${tag}`)),
    )
    return references.join("\n") + `\nrun=${runNonce}`
  }

  /** Dispatch one verified main identity to the central Fly deployment repo. */
  @func({ cache: "never" })
  async dispatchFly(
    metadata: File,
    token: Secret,
    runNonce: string,
  ): Promise<string> {
    this.validateRunNonce(runNonce)
    const input = await this.readStringObject(metadata, [
      "source_repository", "source_run_id", "source_sha",
    ])
    this.validateSourceRepository(input.source_repository)
    if (!/^[0-9a-f]{40}$/.test(input.source_sha)) {
      throw new Error("Fly source SHA must be a full lowercase Git SHA-1")
    }
    if (!/^[1-9][0-9]*$/.test(input.source_run_id)) {
      throw new Error("Fly source run ID must be a positive decimal integer")
    }
    const payload = JSON.stringify({
      event_type: "manja-main",
      client_payload: {
        manja_ref: input.source_sha,
        manja_sha: input.source_sha,
        manja_run_id: input.source_run_id,
        source_repository: "araihu/manja",
      },
    })

    return dag.container()
      .from(GO_IMAGE)
      .withExec(["apt-get", "update"])
      .withExec([
        "apt-get", "install", "-y", "--no-install-recommends", "ca-certificates", "curl",
      ])
      .withExec(["rm", "-rf", "/var/lib/apt/lists/*"])
      .withSecretVariable("GH_TOKEN", token)
      .withNewFile("/tmp/payload.json", payload)
      .withEnvVariable("MANJA_RUN_NONCE", runNonce)
      .withExec([
        "bash", "-euo", "pipefail", "-c",
        `curl --fail --silent --show-error --request POST \
  --header 'Accept: application/vnd.github+json' \
  --header "Authorization: Bearer $GH_TOKEN" \
  --header 'X-GitHub-Api-Version: 2022-11-28' \
  --data-binary @/tmp/payload.json \
  https://api.github.com/repos/araihu/fly-deploy/dispatches`,
      ])
      .withExec(["printf", "Fly dispatch accepted\n"])
      .stdout()
  }

  /** Verify an immutable Assets handoff and return only allowlisted updated files. */
  @func({ cache: "never" })
  async updateAraihuAssets(
    @argument({ defaultPath: ".", ignore: SOURCE_EXCLUDES }) source: Directory,
    metadata: File,
    githubToken: Secret,
    trustDomain: string,
    runNonce: string,
  ): Promise<Directory> {
    this.validateRunNonce(runNonce)
    const input = await this.readStringObject(metadata, [
      "assets_repository", "assets_revision", "release", "release_json_sha256",
      "release_sha256", "release_url",
    ])
    this.validateAssetsIdentity(input)
    const release = input.release
    const releaseUrl = input.release_url
    const archive = dag.http(releaseUrl, {
      checksum: `sha256:${input.release_sha256}`,
      name: `araihu-assets-${release}.tar.gz`,
    })
    const updaterArgs = [
      "-release-dir", "/tmp/araihu-assets-release",
      "-assets-repository", input.assets_repository,
      "-assets-revision", input.assets_revision,
      "-release", release,
      "-release-url", releaseUrl,
      "-release-sha256", input.release_sha256,
      "-release-json-sha256", input.release_json_sha256,
    ]

    const updated = this.project(source, trustDomain)
      .withSecretVariable("GH_TOKEN", githubToken)
      .withEnvVariable("ASSETS_REVISION", input.assets_revision)
      .withEnvVariable("RELEASE", release)
      .withEnvVariable("MANJA_RUN_NONCE", runNonce)
      .withExec([
        "bash", "-euo", "pipefail", "-c",
        `object=$(curl --fail --silent --show-error \
  --header "Authorization: Bearer $GH_TOKEN" \
  --header 'Accept: application/vnd.github+json' \
  "https://api.github.com/repos/araihu/assets/git/ref/tags/$RELEASE")
object_type=$(printf '%s' "$object" | jq -er '.object.type')
object_sha=$(printf '%s' "$object" | jq -er '.object.sha')
if test "$object_type" = tag; then
  object=$(curl --fail --silent --show-error \
    --header "Authorization: Bearer $GH_TOKEN" \
    --header 'Accept: application/vnd.github+json' \
    "https://api.github.com/repos/araihu/assets/git/tags/$object_sha")
  object_type=$(printf '%s' "$object" | jq -er '.object.type')
  object_sha=$(printf '%s' "$object" | jq -er '.object.sha')
fi
test "$object_type" = commit
test "$object_sha" = "$ASSETS_REVISION"`,
      ])
      .withFile("/tmp/araihu-assets-release.tar.gz", archive)
      .withExec([
        "bash", "-euo", "pipefail", "-c",
        `while IFS= read -r member; do
  case "$member" in
    /*|../*|*/../*|*/..) echo "unsafe archive member: $member" >&2; exit 1 ;;
  esac
done < <(tar --list --gzip --file /tmp/araihu-assets-release.tar.gz)
if tar --list --verbose --gzip --file /tmp/araihu-assets-release.tar.gz | grep --extended-regexp --quiet '^[lh]'; then
  echo 'release archive contains a link' >&2
  exit 1
fi
mkdir /tmp/araihu-assets-release
tar --extract --gzip --file /tmp/araihu-assets-release.tar.gz \
  --directory /tmp/araihu-assets-release --no-same-owner --no-same-permissions`,
      ])
      .withExec(["go", "run", "./cmd/araihu-assets-update", ...updaterArgs])
      .withExec(["git", "diff", "--binary"], { redirectStdout: "/tmp/first.diff" })
      .withExec(["go", "run", "./cmd/araihu-assets-update", ...updaterArgs])
      .withExec(["git", "diff", "--binary"], { redirectStdout: "/tmp/second.diff" })
      .withExec(["cmp", "/tmp/first.diff", "/tmp/second.diff"])
      .withExec([
        "go", "test", "./internal/araihuassets", "./cmd/araihu-assets-update", "-count=1",
      ])

    let output = dag.directory()
    for (const path of ASSET_OUTPUTS) {
      output = output.withFile(path, updated.file(`/work/${path}`))
    }
    return output
  }

  private async buildImage(source: Directory, version: string): Promise<Container> {
    const original = await source.file("Dockerfile").contents()
    const buildFrom = "FROM golang:1.26.5-alpine AS build"
    const runtimeFrom = "FROM alpine:3.24"
    if (
      original.split(buildFrom).length !== 2 ||
      original.split(runtimeFrom).length !== 2
    ) {
      throw new Error("Dockerfile base stages changed; refresh verified Dagger image pins")
    }
    const pinned = original
      .replace(buildFrom, `FROM ${GO_BUILD_IMAGE} AS build`)
      .replace(runtimeFrom, `FROM ${ALPINE_IMAGE}`)
    return source
      .withNewFile(".manja-dagger.Dockerfile", pinned)
      .dockerBuild({
        dockerfile: ".manja-dagger.Dockerfile",
        buildArgs: [{ name: "MANJA_VERSION", value: version }],
      })
  }

  private browserProject(source: Directory, trustDomain: string): Container {
    const partition = resolveCachePartition(trustDomain)
    const project = this.project(source, trustDomain)
      .withEnvVariable("PLAYWRIGHT_BROWSERS_PATH", "/ms-playwright")
    return project
      .withMountedCache(
        "/ms-playwright",
        dag.cacheVolume(`manja-${partition}-playwright-${PLAYWRIGHT_VERSION}`),
      )
  }

  private project(source: Directory, trustDomain: string): Container {
    const partition = resolveCachePartition(trustDomain)
    const goDistribution = dag.container().from(GO_IMAGE).directory("/usr/local/go")
    let project = dag.container()
      .from(NODE_IMAGE)
      .withExec(["apt-get", "update"])
      .withExec([
        "apt-get", "install", "-y", "--no-install-recommends",
        "bash", "ca-certificates", "curl", "git", "jq", "openssh-client",
      ])
      .withExec(["rm", "-rf", "/var/lib/apt/lists/*"])
      .withDirectory("/usr/local/go", goDistribution)
      .withDirectory("/work", source)
      .withWorkdir("/work")
      .withEnvVariable("GOWORK", "off")
      .withEnvVariable("GOMODCACHE", "/go/pkg/mod")
      .withEnvVariable("GOCACHE", "/root/.cache/go-build")
      .withEnvVariable("PATH", "/usr/local/go/bin:$PATH", { expand: true })
    project = project
      .withMountedCache("/go/pkg/mod", dag.cacheVolume(`manja-${partition}-go-mod-v1`))
      .withMountedCache(
        "/root/.cache/go-build",
        dag.cacheVolume(`manja-${partition}-go-build-v1`),
      )
      .withMountedCache(
        "/work/.cache/muamba",
        dag.cacheVolume(`manja-${partition}-muamba-v1`),
      )
      .withMountedCache("/root/.npm", dag.cacheVolume(`manja-${partition}-npm-v1`))
    return project
      .withExec([
        "bash", "-euo", "pipefail", "-c",
        "git init -q && git config user.name Dagger && git config user.email dagger@invalid && git add -A && git commit -qm source-baseline",
      ])
      .withExec([
        "bash", "-euo", "pipefail", "-c",
        "test -z \"$(git ls-files '.dagger/sdk/**')\"",
      ])
  }

  private validateRunNonce(value: string): void {
    if (!/^(?:[1-9][0-9]*-[1-9][0-9]*|local-[0-9a-f]{8}-[0-9a-f-]{27})$/.test(value)) {
      throw new Error("run nonce must be github.run_id-github.run_attempt or local-UUID")
    }
  }

  private validateSourceRepository(value: string): void {
    if (value !== "araihu/manja") {
      throw new Error("source repository must be araihu/manja")
    }
  }

  private validateVersion(value: string): void {
    if (
      value !== "dev" &&
      !/^[0-9a-f]{40}$/.test(value) &&
      !/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(value)
    ) {
      throw new Error("image version must be dev, a full Git SHA, or stable SemVer tag")
    }
  }

  private validateAssetsIdentity(input: Record<string, string>): void {
    if (input.assets_repository !== "araihu/assets") {
      throw new Error("assets repository must be araihu/assets")
    }
    if (!/^[0-9a-f]{40}$/.test(input.assets_revision)) {
      throw new Error("assets revision must be a full lowercase Git SHA-1")
    }
    if (!/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(input.release)) {
      throw new Error("assets release must be a stable SemVer tag")
    }
    if (
      !/^[0-9a-f]{64}$/.test(input.release_sha256) ||
      !/^[0-9a-f]{64}$/.test(input.release_json_sha256)
    ) {
      throw new Error("Assets SHA-256 values must be lowercase hexadecimal")
    }
    const expected = `https://github.com/araihu/assets/releases/download/${input.release}/araihu-assets-${input.release}.tar.gz`
    if (input.release_url !== expected) {
      throw new Error("release URL differs from immutable archive URL")
    }
  }

  private async readStringObject(
    file: File,
    expectedKeys: readonly string[],
  ): Promise<Record<string, string>> {
    let parsed: unknown
    try {
      parsed = JSON.parse(await file.contents())
    } catch {
      throw new Error("metadata is not valid JSON")
    }
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
      throw new Error("metadata must be a JSON object")
    }
    const record = parsed as Record<string, unknown>
    const actual = Object.keys(record).sort()
    const expected = [...expectedKeys].sort()
    if (
      actual.length !== expected.length ||
      actual.some((key, index) => key !== expected[index])
    ) {
      throw new Error("metadata fields do not match the strict schema")
    }
    for (const key of expected) {
      if (typeof record[key] !== "string") {
        throw new Error(`metadata field ${key} must be a string`)
      }
    }
    return record as Record<string, string>
  }
}
