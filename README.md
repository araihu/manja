# Manja

Manja is a hosted OpenAPI renderer and publisher built with
[Goshtoso](https://github.com/araihu/goshtoso).

The first vertical slice renders read-only OpenAPI docs from a spec file and
provides Ctrl+K search across indexed operations and schemas.

## Development

```bash
go test ./...
npm ci
npm run api:bundle
npm run api:lint
go run ./cmd/manja -data-dir .manja/data
```

Open <http://localhost:8080>.

For live development, run:

```bash
npm run dev
```

Air watches Go, templ, API description, and static asset sources. On rebuild it
regenerates templ output, rebuilds schema example assets, runs `css:build` when
that npm script exists, and restarts Manja.

Generated templ Go files, static CSS bundles, and the schema example JS bundle
are excluded from the watcher so build outputs do not trigger rebuild loops.

The dev launcher chooses available app, proxy, and product site ports per
worktree, then prints the URLs before starting Air and the site server. Open the
printed product site URL for landing/docs work. Open the Air reload proxy URL,
not the app URL, when you want the standalone renderer with reload. To pin ports
explicitly, run:

```bash
npm run dev -- --app-port 8080 --proxy-port 7331 --site-port 8180
```

Use `npm run dev -- --print-ports` to inspect the selected ports without
starting the servers. Extra flags are passed through to Manja:

```bash
npm run dev -- -spec internal/adapters/openapi/testdata/petstore.yaml
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

Review local files:

```bash
go run ./cmd/manja check \
  --config .manja.yaml \
  --contract payments \
  --target-file ./baselines/target.yaml \
  --candidate-file ./docs/openapi.yaml \
  --release-file ./baselines/release.yaml \
  --format text
```

Or load the configured spec path from refs in a local Git checkout:

```bash
go run ./cmd/manja check \
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
