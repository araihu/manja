# Shipped Artifact Matrix

Date: 2026-08-11

Status: packaging and licensing gates are **BLOCKED** by
[`provenance.md`](provenance.md). This file records current artifact contents
and required future placement of license evidence. It does not assert that any
artifact is cleared for Apache-2.0 distribution.

## Classification Rules

- A dependency is `shipped` only when its code, assets, derived data, or
  required notices are present in the inspected final artifact.
- A dependency used only to compile, generate, lint, or test is `build-only`
  or `test-only`; presence in `go.mod` or an acquisition manifest is not enough
  to call it redistributed.
- Generated JavaScript and renderer snapshots are classified from actual
  inputs and final bytes, not from manifest labels.
- Release inspection must operate on built archives/images/site artifacts, not
  only on the source tree.

## Current Artifact Matrix

| Artifact | Current build and inspected contents | Shipped scope | Required future evidence placement | Current inspection command/result |
| --- | --- | --- | --- | --- |
| Source tarball | No release packager exists. `git archive` contains 688 paths: tracked first-party source/docs/config, generated Go/JavaScript, 31 Muamba acquisition archives and their retained licenses, 65 locked Kubernetes OpenAPI files and upstream license, copied fixtures/assets, and three unresolved logo-concept PNGs. It excludes ignored `api/dist` and transient build state. No root project license, notice, third-party notice, or SBOM exists. | Complete tracked tree, including copied/generated material and retained third-party acquisition bytes. No root `vendor/` tree is tracked, but `internal/webassets/vendor/` is shipped in a raw source archive. | Archive root: verified project `LICENSE`, accurate `NOTICE`, reviewed `THIRD_PARTY_NOTICES.md`, and deterministic source inventory/SBOM if policy requires it. Preserve required upstream license files and exclude unresolved generated concepts unless cleared. | `git archive --format=tar HEAD \| tar -tf -` reports 688 paths. After a packager exists, unpack immutable bytes and reconcile every notice record to actual files. |
| `manja` binary archive | No binary-archive script or released binary layout exists. Current Docker build creates a build-time `cmd/manja` compiler and a separate runtime-only `cmd/manja-runtime` binary; only the runtime binary enters the image. | Cannot be called a shipped archive yet. A future runtime archive would include Manja plus templ, Goshtoso, Chroma, regexp2, `x/text`, YAML, runtime static assets, and precompiled renderer data if it mirrors the image. Build-time parser/compiler modules and test tools must stay excluded unless actual archive bytes prove otherwise. | Archive root: `bin/manja`, project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, Go/browser/renderer-data SBOMs, runtime static tree, renderer config/allowlist, and all notices required by compiled catalogs. | Build with the release script once one exists; unpack, run `go version -m bin/manja`, enumerate files, verify static/renderer digests, and compare SBOM/notices to final bytes. |
| OCI image `ghcr.io/araihu/manja` | Current Dockerfile builds with `golang:1.26.5-alpine`, compiles Kubernetes/GitHub/Stripe inputs, and copies only `manja-runtime` into `alpine:3.24`. Local `manja:provenance` arm64 inspection: 35,242,561-byte image, 9 layers, 17 final APK packages, 416 renderer-data files in three snapshots, renderer config/allowlist, and complete `internal/web/static`. Final stage has no `git`. | Runtime Go closure; Alpine base and `ca-certificates`; compiled Kubernetes/GitHub/Stripe renderer data; CSS/JS/SVG/PNG assets; both generated browser bundles. The blanket static copy also ships `request_composer_browser_test.go` and `schema_example_browser_test.go` as source files. Parser/compiler modules, Go toolchain, and integration/test dependencies remain build-only unless copied elsewhere. | `/usr/share/licenses/manja/`: verified project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and OCI/Go/browser/renderer-data SBOMs. Preserve required Kubernetes, GitHub, Stripe, Simple Icons, and other attributions. Current image contains none of these paths. | `docker build --pull=false -t manja:provenance .`; `docker history --no-trunc`; `docker image inspect`; run `/bin/sh` to enumerate APK and application files. Local image ID: `sha256:dd2c84189b78ec4d84635667f7e97fafe74c4401e3f97e2be23c7178b2d50c09`. |
| Public `site` artifact | Separate module `github.com/araihu/manja/site` builds `site/cmd/server`; `site/internal/site/assets.go` embeds `static/*`. No site archive/image/release packager exists. Direct `GOWORK=off go test ./... -count=1` currently stops before tests because `site/go.mod` needs two indirect dependency updates. CI instead copies the mod/sum to temporary modfiles, tidies those, and tests the candidate root graph; independent baseline review established that this path can pass, but this document claims no exact OC-01 run receipt for it. | If distributed, site server production closure plus embedded CSS, JavaScript, SVG assets, and product copy. Root test/tool dependencies are not automatically part of the site. The remote campaign script is fetched at runtime and is not embedded in the site binary. | Beside site binary/archive: project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, site-specific Go/static SBOM, and a validated social preview asset. If deployed as an image, mirror OCI placement. | Current direct command fails with `go: updates to go.mod needed; to update it: go mod tidy`. Keep this visible until a separately authorized dependency checkpoint fixes it. Use the exact temporary-modfile CI command for candidate-graph verification without changing committed site files. |

## Dependency And Artifact Boundaries

### Runtime Go code in the current OCI binary

`GOWORK=off go list -deps -tags=manja_runtime ./cmd/manja-runtime` resolves 264
packages. Non-standard module bodies are:

- `github.com/a-h/templ v0.3.1020`;
- `github.com/alecthomas/chroma/v2 v2.24.1`;
- `github.com/araihu/goshtoso v0.1.8`;
- `github.com/dlclark/regexp2 v1.12.0`;
- `golang.org/x/text v0.40.0`;
- `gopkg.in/yaml.v3 v3.0.1`;
- first-party `github.com/araihu/manja` packages.

### Browser and static bytes in the current OCI image

- `schema-example.js`: `openapi-sampler` plus Manja hydrator;
- `request-composer.js`: `@readme/httpsnippet`, `buffer`, selected
  `highlight.js` modules, their transitive bundle inputs, and Manja hydrator;
- exact archive/license/input membership in
  [`browser-bundles.md`](browser-bundles.md);
- Manja/catalog CSS and JavaScript, SVG marks, favicons, and 1280x640 Manja
  and Kubernetes social-preview PNGs;
- two browser-test Go source files copied by the static-directory rule.

### Compiled renderer data in the current OCI image

- Kubernetes: 65 OpenAPI documents locked to upstream commit
  `a818af18fe29d999d6741234c8cd72709ef2f424`;
- GitHub: tracked `github-v3-rest.json`, without immutable upstream byte pin;
- Stripe: `openapi/spec3.json` fetched at commit
  `d70de345383dd818a0ce831f4e20d375c5a90cec`;
- 416 final renderer-data files totaling about 67.6 MiB in the inspected image.

### Build-only or test-only unless final bytes prove otherwise

- `cmd/manja` parser/compiler dependency closure;
- `oapi-codegen` and templ CLIs;
- esbuild as generator rather than its generated output;
- Playwright and browser payload;
- Testcontainers, Forgejo module, and their container/client closure;
- Go toolchain and Docker build-stage packages.

Current OCI inspection shows no test dependency binaries, but it does ship the
two static-tree browser-test source files named above.

## Social Metadata Artifact Gate

Renderer/catalog initial HTML passes the current route-specific metadata tests
for title, description, canonical URL, `og:url`, Open Graph type/title/
description/site/image/type/width/height/alt, and explicit X Card tags. The
asset handler test proves the Kubernetes preview is HTTP `image/png`, 48,705
bytes, and 1280x640; `file` confirms the Manja preview is also 1280x640 PNG.
Both images are copied into the OCI image with matching SHA-256 values.

The public product site is not social-ready at this snapshot. `/` and `/docs`
render route-specific title and description in initial HTML, but emit no
canonical URL, `og:url`, other Open Graph tags, explicit X Card tags, or social
preview image. `site/internal/site/static` contains no preview image. This is a
release artifact gap, not authorization to edit the site or its styling in
OC-01.

## Required Gate Before Distribution

Provenance must first become `PASS`. Then Task 8 must generate deterministic
CycloneDX inventories from actual source/archive bytes, runtime binary, browser
bundles, compiled renderer data, site artifact, and final OCI filesystem.
Tests must unpack or export every final artifact and fail on missing notices,
unknown licenses, stale records, absent social metadata/assets, or test/build
packages incorrectly classified as shipped.
