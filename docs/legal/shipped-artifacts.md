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
| Source tarball | No release packager exists. The audited-base archive at commit `39d65ade21c080ee2102f53da5ed741f000d6dd7` and tree `64cee6ab67060d1d8c4734fc5f54f6dbe6d272f6` contains 688 paths: tracked first-party source/docs/config, generated Go/JavaScript, 31 Muamba acquisition archives and their retained licenses, 65 locked Kubernetes OpenAPI files and upstream license, copied fixtures/assets, and three unresolved logo-concept PNGs. It excludes ignored `api/dist` and transient build state. No root project license, notice, third-party notice, or SBOM exists. This is an immutable audited-base receipt, not a candidate-`HEAD` count. | Complete tracked tree, including copied/generated material and retained third-party acquisition bytes. No root `vendor/` tree is tracked, but `internal/webassets/vendor/` is shipped in a raw source archive. | Archive root: verified project `LICENSE`, accurate `NOTICE`, reviewed `THIRD_PARTY_NOTICES.md`, and deterministic source inventory/SBOM if policy requires it. Preserve required upstream license files and exclude unresolved generated concepts unless cleared. | `git archive --format=tar 39d65ade21c080ee2102f53da5ed741f000d6dd7 \| tar -tf -` reports 688 paths. After a packager exists, unpack immutable bytes and reconcile every notice record to actual files. |
| `manja` binary archive | No binary-archive script or released binary layout exists. Current Docker build creates a build-time `cmd/manja` compiler and a separate runtime-only `cmd/manja-runtime` binary; only the runtime binary enters the image. | Cannot be called a shipped archive yet. A future runtime archive would include Manja plus templ, Goshtoso, Chroma, regexp2, `x/text`, YAML, runtime static assets, and precompiled renderer data if it mirrors the image. Build-time parser/compiler modules and test tools must stay excluded unless actual archive bytes prove otherwise. | Archive root: `bin/manja`, project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, Go/browser/renderer-data SBOMs, runtime static tree, renderer config/allowlist, and all notices required by compiled catalogs. | Build with the release script once one exists; unpack, run `go version -m bin/manja`, enumerate files, verify static/renderer digests, and compare SBOM/notices to final bytes. |
| OCI image `ghcr.io/araihu/manja` | Current Dockerfile builds with `golang:1.26.5-alpine`, compiles Kubernetes/GitHub/Stripe inputs, and copies only `manja-runtime` into `alpine:3.24`. Local `manja:provenance` inspection reported `linux/arm64`: 35,242,561-byte image, 9 layers, 17 final APK packages, 416 renderer-data files in three snapshots, renderer config/allowlist, and complete `internal/web/static`. Final stage has no `git`. | Runtime Go closure; Alpine base and `ca-certificates`; compiled Kubernetes/GitHub/Stripe renderer data; CSS/JS/SVG/PNG assets; both generated browser bundles. The blanket static copy also ships `request_composer_browser_test.go` and `schema_example_browser_test.go` as source files. Parser/compiler modules, Go toolchain, and integration/test dependencies remain build-only unless copied elsewhere. | `/usr/share/licenses/manja/`: verified project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and OCI/Go/browser/renderer-data SBOMs. Preserve required Kubernetes, GitHub, Stripe, Simple Icons, and other attributions. Current image contains none of these paths. | Prior observed receipt: `docker build --pull=false -t manja:provenance .`; `docker history --no-trunc`; `docker image inspect`; run `/bin/sh` to enumerate APK and application files. The build command did not specify a platform; inspection reported `linux/arm64`. Local image ID: `sha256:dd2c84189b78ec4d84635667f7e97fafe74c4401e3f97e2be23c7178b2d50c09`. Future reproduction must use the explicit platform and base-digest checks in `provenance.md`. |
| Public `site` artifact | Separate module `github.com/araihu/manja/site` builds `site/cmd/server`; `site/internal/site/assets.go` embeds `static/*`. No site archive/image/release packager exists. Direct `GOWORK=off go test ./... -count=1` currently stops before tests because `site/go.mod` needs two indirect dependency updates. CI instead copies the mod/sum to temporary modfiles, tidies those, and tests the candidate root graph; independent baseline review established that this path can pass, but this document claims no exact OC-01 run receipt for it. | If distributed, site server production closure plus embedded CSS, JavaScript, SVG assets, and product copy. Root test/tool dependencies are not automatically part of the site. The remote campaign script is fetched at runtime and is not embedded in the site binary. | Beside site binary/archive: project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, site-specific Go/static SBOM, and a validated social preview asset. If deployed as an image, mirror OCI placement. | Current direct command fails with `go: updates to go.mod needed; to update it: go mod tidy`. Keep this visible until a separately authorized dependency checkpoint fixes it. Use the exact temporary-modfile CI command for candidate-graph verification without changing committed site files. |

## Dependency And Artifact Boundaries

### Runtime Go code in the current OCI binary

`GOWORK=off go list -deps -tags=manja_runtime ./cmd/manja-runtime` resolves 264
package paths, but that command does not emit module versions. The matching
module-aware receipt is:

```bash
GOWORK=off go list -deps -tags=manja_runtime \
  -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}' \
  ./cmd/manja-runtime |
  awk 'NF == 2' |
  LC_ALL=C sort -u
```

It directly reports exactly these six external module/version pairs:

- `github.com/a-h/templ v0.3.1020`;
- `github.com/alecthomas/chroma/v2 v2.24.1`;
- `github.com/araihu/goshtoso v0.1.8`;
- `github.com/dlclark/regexp2 v1.12.0`;
- `golang.org/x/text v0.40.0`;
- `gopkg.in/yaml.v3 v3.0.1`.

The 264-package-path query also includes 14 first-party Manja package paths.
They are deliberately absent from the external module/version receipt because
the `.Main` filter excludes the main module. Reproduce that count directly:

```bash
GOWORK=off go list -deps -tags=manja_runtime \
  -f '{{if .Module}}{{if .Module.Main}}{{.ImportPath}}{{end}}{{end}}' \
  ./cmd/manja-runtime |
  awk 'NF' |
  LC_ALL=C sort -u |
  wc -l
```

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
- GitHub: `github-v3-rest.provenance.json` binds the 3,319,366-byte tracked
  `github-v3-rest.json` to `github/rest-api-description` commit
  `6948cb04f5304188569c4bb4ae2190c08e7cbdba`, tree
  `6270ed1bd31a741adf3c7143c39d9bdc57d2fbc1`, Git blob
  `f0ddf34ad4398c319db0643e45a0908ca026b382`, and SHA-256
  `dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977`.
  The receipt also records the 1,063-byte same-revision MIT `LICENSE.md` Git blob
  `b50625eb63949013cae604b1cadd42cfa1eaf825` and SHA-256
  `3243761cbac07e6d169a5a2f4e7c25cc544da85248e735df74c3672e055cc87b`.
  Mechanical identification is resolved; compiled renderer data remains
  **BLOCKED** pending reviewed attribution/notice placement in final artifacts;
- Stripe: `openapi/spec3.json` fetched at commit
  `d70de345383dd818a0ce831f4e20d375c5a90cec`;
- 416 final renderer-data files totaling about 67.6 MiB in the inspected image.

### Build-only or test-only unless final bytes prove otherwise

- `cmd/manja` parser/compiler dependency closure;
- `oapi-codegen` and templ CLIs;
- esbuild as generator rather than its generated output;
- the receipt-only macOS/arm64 `rsvg-convert` 2.62.1 environment and system
  Arial fonts used to reproduce the tracked Manja social PNG exactly; those
  tool/font bytes are not tracked, run by CI, copied into the OCI image, or
  authorized for redistribution by this evidence;
- Playwright and browser payload;
- Testcontainers, Forgejo module, and their container/client closure;
- Go toolchain and Docker build-stage packages.

Current OCI inspection shows no test dependency binaries, but it does ship the
two static-tree browser-test source files named above.

## Social Metadata Artifact Gate

Renderer/catalog initial HTML passes the current route-specific metadata tests
for title, description, canonical URL, `og:url`, Open Graph type/title/
description/site/image/type/width/height/alt, and explicit X Card tags. The
asset handler tests prove the Kubernetes preview is HTTP `image/png`, 48,705
bytes, and 1280x640, and the Manja preview is HTTP `image/png`, 21,500 bytes,
1280x640, under 1 MiB, and byte-identical to the tracked artifact. The Manja
offline receipt test also fixes the SVG/PNG hashes and observed renderer/font
environment without invoking that renderer in CI. On 2026-08-11, the public
catalog routes emitted the required route-specific tags exactly once and the
absolute HTTPS Manja preview returned HTTP 200, `image/png`, the tracked 21,500
bytes, 1280x640 dimensions, and matching SHA-256. Both images are copied into
the OCI image with matching SHA-256 values.

This does not establish portable Manja preview regeneration: CI and the release
build do not have a durable lawful pin for the exact macOS renderer and system
Arial font bytes. That acquisition gate, the complete composition's first-party
authority, and the separate Kubernetes preview provenance remain blocked.

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
unknown licenses, stale provenance or artifact-specific inventory records, or
test/build packages incorrectly classified as shipped. Applicable web
artifacts containing route HTML must additionally fail when initial HTML lacks
route-specific title, description, canonical URL, `og:url`, required Open Graph
metadata, explicit X Card tags, or a validated social-preview image.

By default, the distribution gate must also fail if either browser-test source
is present anywhere in runtime/binary-package or OCI bytes. Raw source archives
are not subject to this exclusion.

No Manja runtime/binary archive, archive format, immutable archive digest,
packager, final layout, or authoritative archive manifest exists at this
snapshot; CI publishes only the OCI artifact. Therefore no caller-supplied host
directory can currently prove complete runtime/binary-package contents. A clean
selected subdirectory can hide prohibited files elsewhere. The host archive gate
remains **BLOCKED** under Task 8 rather than reporting success from such a root.

The prospective Task-8 invariant is per archive: the packager must receive the
immutable archive bytes and an independently trusted expected digest from the
release definition, verify that digest, create a fresh empty extraction root,
safely extract the complete archive into that root itself, and recursively scan
that exact root. It must reject digest mismatch; unsafe or incomplete extraction;
path traversal or link escape; inventory or permission errors; and any scan
error or prohibited browser-test source. Each separately produced runtime/binary
archive must repeat the process in its own fresh root. Regression coverage must
reject substitution of a clean selected subdirectory while prohibited bytes
remain elsewhere. A sibling checksum file or marker inside the extracted root
is not independent authority. This is a future invariant, not a current archive,
digest, command, layout, marker, or receipt.

The current OCI gate remains concrete. It runs read-only as root and scans the
complete image root filesystem rather than selected `/app` paths:

```bash
set -euo pipefail
: "${IMAGE:?set IMAGE}"

docker run --rm --read-only --user 0:0 --entrypoint /bin/sh "$IMAGE" -ec '
  if ! match=$(
    find / -xdev -type f \
      \( -name request_composer_browser_test.go \
         -o -name schema_example_browser_test.go \) \
      -print -quit
  ); then
    printf "browser-test source scan failed: /\n" >&2
    exit 1
  fi
  if [ -n "$match" ]; then
    printf "forbidden browser-test source: %s\n" "$match" >&2
    exit 1
  fi
'
```

This exclusion may be changed only if shipping those source files in a runtime
artifact is an intentional redistribution decision and an explicit notice/SBOM
policy review clears and inventories them. Their accidental presence under a
blanket static copy is not clearance. Task 8 remains blocked until archive-owned
extraction/scanning and the other final packager gates are implemented; OC-01
does not change the Dockerfile. The current OCI scan fails on the browser-test
sources already recorded under `/app/internal/web/static`; that failure remains
truthful until separately authorized packaging work removes or explicitly
clears them.
