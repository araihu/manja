# Manja

Manja is a hosted OpenAPI renderer and publisher built with
[Goshtoso](https://github.com/araihu/goshtoso).

The first vertical slice renders read-only OpenAPI docs from a spec file and
provides Ctrl+K search across indexed operations and schemas.

## Development

```bash
go test ./...
go tool muamba verify --strict
go run ./cmd/webassets check
scripts/redocly bundle api/openapi.yaml -o api/dist/openapi.yaml
scripts/redocly lint api/openapi.yaml
go run ./cmd/manja -data-dir .manja/data
```

Open <http://localhost:8080>.

### Resource limits

Manja does not enforce its conservative catalog source, compilation, startup,
snapshot, storage, catalog-count, or HTML rendering budgets by default. This
lets operators compile and render unusually large catalog sets on hosts sized
for that work.

Set `MANJA_RESOURCE_LIMITS=true` on both `manja` and `manja-runtime` to restore
the bounded policy. Structural and trust-boundary checks remain enabled in both
modes: OpenAPI validity, canonical identities, safe paths, artifact integrity,
request-input limits, and local-doc browser download protections are not
resource-sizing policy.

An unbounded build or render can exhaust the host's memory or disk and may be
terminated by the operating system. See the
[generated environment reference](docs/environment.md) for all environment
variables.

### Portable CI with Dagger

CI toolchains and the same gates are available locally through Dagger v0.21.8.
The host needs only the exact Dagger CLI and a compatible container runtime:

```bash
dagger call verify --source=. --trust-domain=local
dagger call integration --source=. --trust-domain=local
dagger call image --source=. --version=dev
```

`verify` covers generated drift, Muamba, Redocly, oapi-codegen, templ, root
tests, Playwright, and the standalone `site/` module with `GOWORK=off`.
`integration` provisions a pinned Forgejo service with native Dagger service
bindings. It needs no Docker daemon, privileged service, or host runtime socket.
Mutable Go, npm, Muamba, and browser caches are partitioned by the runner trust
boundary. Admitted pull requests run on `hostinger-vps-pr`; protected main,
tag, publication, deployment, and Assets jobs run on
`hostinger-vps-trusted`. Host configuration isolates the two labels' Dagger
Engines, sockets, data roots, and ACLs.

Fork and same-repository PR domains both resolve to the stable `pr` cache
namespace inside PR Engines. They cannot read or populate `main`, `release`,
or `assets` volumes. GitHub-hosted fallback PRs use the same logical namespace
on an ephemeral Engine. Persistent volumes contain only Go module/build data,
npm downloads, Muamba's verified download cache, and Playwright browser tools;
source, generated outputs, publication artifacts, application state, metadata
files, and secrets remain outside them.

`publish-image`, `dispatch-fly`, and `update-araihu-assets` are uncached
effect/freshness functions. They require strict JSON `File` inputs, typed
secrets, and a unique nonce. Local callers can use
`local-$(uuidgen | tr '[:upper:]' '[:lower:]')` and
should invoke these functions only when intending the documented GHCR,
GitHub, or repository update effect.

For live development, run:

```bash
go run ./cmd/dev
```

Air watches Go, templ, API description, and static asset sources. On rebuild it
regenerates templ output, rebuilds the browser assets with the pinned Go
toolchain, and restarts Manja.

Generated templ Go files, static CSS bundles, and the schema example JS bundle
are excluded from the watcher so build outputs do not trigger rebuild loops.

The dev launcher chooses available app, proxy, and product site ports per
worktree, then prints the URLs before starting Air and the site server. Open the
printed product site URL for landing/docs work. Open the Air reload proxy URL,
not the app URL, when you want the standalone renderer with reload. To pin ports
explicitly, run:

```bash
go run ./cmd/dev --app-port 8080 --proxy-port 7331 --site-port 8180
```

Use `go run ./cmd/dev --print-ports` to inspect the selected ports without
starting the servers. Extra flags are passed through to Manja:

```bash
go run ./cmd/dev -- -spec internal/adapters/openapi/testdata/petstore.yaml
```

## Contract review

Manja can compare a candidate OpenAPI contract with target and release
baselines entirely offline. Add a repository-owned `.manja.yaml`:

```yaml
version: 1
contracts:
  payments:
    spec: docs/openapi.yaml
    defaultPolicy: stable
    policies:
      stable:
        requireReleaseBaseline: true
        rules:
          operation.removed: fail
          schema.removed: fail
```

Build the binary first so the shell receives Manja's exact `0`, `1`, or `2`
status:

```bash
mkdir -p ./bin
go build -o ./bin/manja ./cmd/manja
```

Review local files:

```bash
./bin/manja check \
  --config .manja.yaml \
  --contract payments \
  --target-file ./baselines/target.yaml \
  --candidate-file ./docs/openapi.yaml \
  --release-file ./baselines/release.yaml \
  --format text
```

Or load the configured spec path from refs in a local Git checkout:

```bash
./bin/manja check \
  --config .manja.yaml \
  --contract payments \
  --repo . \
  --target-ref origin/main \
  --candidate-ref HEAD \
  --release-ref v1.0.0 \
  --format json
```

Exit code `0` means policy passed, `1` means analysis completed with a policy
failure, and `2` means configuration, input, parsing, or execution failed.
GitHub Actions and connected Manja review are later subprojects; these examples
work locally and in any CI environment with the repository checked out.

## Docker Image

Manja images are published to GitHub Container Registry as
`ghcr.io/araihu/manja`. The `main` tag follows the latest successful build from
`main`; release tags also publish semver tags such as `1`, `1.2`, and `1.2.3`.

Run the image with its bundled GitHub REST API fixture:

```bash
docker run --rm -p 8080:8080 ghcr.io/araihu/manja:main
```

Open <http://localhost:8080>. The container stores local publication metadata in
`/var/lib/manja`; mount it when you want state to survive container restarts:

```bash
docker run --rm \
  -p 8080:8080 \
  -v manja-data:/var/lib/manja \
  ghcr.io/araihu/manja:main
```

To render your own OpenAPI document, mount the file and pass the same Manja
flags you would use with the local binary:

```bash
docker run --rm \
  -p 8080:8080 \
  -v "$PWD/openapi.yaml:/spec/openapi.yaml:ro" \
  -v manja-data:/var/lib/manja \
  ghcr.io/araihu/manja:main \
  -addr :8080 \
  -spec /spec/openapi.yaml \
  -data-dir /var/lib/manja
```
