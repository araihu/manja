# Shipped Artifact Matrix

Date: 2026-07-25

Status: packaging and licensing gates are **BLOCKED** by
`docs/legal/provenance.md`. This file records current artifact contents and the
required future placement of license evidence; it does not assert that any
artifact is presently cleared for Apache-2.0 distribution.

## Classification Rules

- A dependency is `shipped` only when its code, assets, or required notices are
  present in the inspected final artifact.
- A dependency used only to compile, generate, lint, or test is `build-only` or
  `test-only`; presence in `go.mod` or `package-lock.json` is not sufficient to
  call it redistributed.
- Generated JavaScript bundles are inspected by bundle content and source
  inputs, not by the package's `devDependency` label.
- Every release inspection operates on a built archive/image/site artifact, not
  only on the source tree.

## Current And Planned Artifacts

| Artifact | Build/current contents | Shipped dependency scope | Required license, notice, and SBOM placement | Final inspection |
| --- | --- | --- | --- | --- |
| Source tarball | No release packager exists. A raw `git archive` would include all tracked Go/templ/JS/YAML/docs, generated Go/JS, the GitHub fixture, and logo-concept PNGs. It excludes ignored `api/dist` and `node_modules`. | First-party source plus copied/generated/static material; dependency source is not included unless vendored (no `vendor/` is tracked). | Root of archive: verified `LICENSE`, accurate `NOTICE`, reviewed `THIRD_PARTY_NOTICES.md`, and source inventory/SBOM if the release policy requires it. | `git archive --format=tar HEAD | tar -tvf -`; after packaging, unpack into a temporary directory and compare the notice inventory to actual files. |
| `manja` binary archive | No binary-archive script exists. Current executable build is `CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' ./cmd/manja`. Static templ output and embedded Go dependencies become part of the executable; separately served web files must be packaged beside it for the current runtime layout. | Production closure from `go list -deps ./cmd/manja`, plus runtime static assets and browser bundles. Playwright, Testcontainers, oapi-codegen, Redocly, and templ's CLI are not shipped merely because they build/test/generate. | Archive root: `bin/manja`, verified `LICENSE`, accurate `NOTICE`, `THIRD_PARTY_NOTICES.md`, `sbom/manja-go.cdx.json`, and `sbom/manja-npm.cdx.json`; include the runtime static tree or change the binary to embed it before packaging. | Unpack archive; run `bin/manja -version` or equivalent; enumerate files; compare Go build info and browser bundle inventory with both SBOMs and notices. |
| OCI image `ghcr.io/araihu/manja` | `Dockerfile` builds from `golang:1.26.1-alpine`; the final `alpine:3.22` image installs `ca-certificates` and `git`, then copies `/usr/local/bin/manja`, `internal/web/static`, and `github-v3-rest.json`. Current CI publishes main/SHA/semver tags. | Binary production closure, bundled browser dependencies, final Alpine packages, copied static assets, and the GitHub fixture. Build-stage Go/npm/test dependencies are excluded unless copied into the final filesystem. | `/usr/share/licenses/manja/`: verified `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and OCI/Go/npm SBOMs. Base-image and installed-package evidence must be represented in the OCI inventory. | `docker history --no-trunc IMAGE`; `docker image inspect IMAGE`; create/export a container and enumerate the final filesystem and APK database. This checkpoint did not perform the inspection because no approved rootless Docker provider was configured. Colima Docker was reachable only through explicit host-socket settings used for integration verification; that does not complete or approve the licensing inspection. |
| Public `site` artifact | The separate `github.com/araihu/manja/site` module builds `site/cmd/server`. `site/internal/site/assets.go` embeds `static/*` into the binary. It imports the local Manja module and Goshtoso through the site module graph. No site release packager exists. | Site server production closure plus embedded CSS, theme JS, Manja SVG assets, and rendered product copy. Root-module test/tool dependencies are not automatically part of the site artifact. | Beside a site binary/archive: verified `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and a site-specific Go/static SBOM. If deployed as an image, mirror the OCI placement above. | `GOWORK=off go list -deps ./...` and `GOWORK=off go build ./cmd/server` from `site/`; inspect Go build info and embedded/static file responses from the built server. |

## Dependency Boundaries Observed At This Snapshot

### Browser code that is shipped

- `schema-example.js`: `openapi-sampler` distribution plus Manja's hydrator;
  the generated file contains Faker-related MIT notices.
- `request-composer.js`: esbuild output containing `@readme/httpsnippet`,
  `buffer`, selected `highlight.js` language modules, transitive dependencies,
  and Manja's hydrator.
- Manja CSS, SVGs, favicons, and small hydration scripts copied by the image or
  embedded by the site.

### Build-only or test-only unless final inspection proves otherwise

- `@redocly/cli`: OpenAPI bundle/lint tooling.
- `esbuild`: bundle generator, not itself assumed shipped merely because its
  output is shipped.
- `oapi-codegen` and templ CLI packages: code generators; generated output is
  shipped, the generator executables are not.
- Playwright and its browser payload: end-to-end test tooling.
- Testcontainers, Forgejo test module, and their container/client dependency
  closure: integration-test tooling.
- Go/Node toolchains and the Docker build-stage packages.

## Required Gate Before Distribution

After provenance becomes `PASS`, the release tooling must generate deterministic
CycloneDX inventories from the actual production binary, both browser bundles,
the site artifact, and the final OCI filesystem. Tests must unpack or export
each final artifact and fail on missing notice material, unknown licenses,
stale records, or test-only packages incorrectly classified as shipped.
