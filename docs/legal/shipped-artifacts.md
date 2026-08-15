# Shipped Artifact Matrix

Date: 2026-08-15

Status: packaging and licensing gates are **BLOCKED** by
[`provenance.md`](provenance.md). This file records current artifact contents
and required future placement of license evidence. It does not assert that any
artifact is cleared for Apache-2.0 distribution.

The mechanical archive seam now exists in commit `e90f8da`
(`internal/distribution/packager.go`), with recursive root/archive inspection,
deterministic tar output, legal-byte/SBOM checks, and explicit OCI refusal.
This does not create a production release layout or establish authority; no
real source, binary, site, or digest-bound OCI artifact has been cleared at
this checkpoint.

## 2026-08-15 read-only artifact observations

- A diagnostic `git archive` of candidate `HEAD eccc468013b5b93eb48929db2071d6aa7b1a150f` contained 846 paths and had SHA-256 `64f2654015bfb71f339832d526d5a8f85d4a83d36d2256f25b6d099971ff9683`. It contained retained upstream/vendor license files but no root `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, or SBOM. It is not a cleared release archive.
- The only local Manja image observed was `manja:provenance`, image ID `sha256:dd2c84189b78ec4d84635667f7e97fafe74c4401e3f97e2be23c7178b2d50c09`, `linux/arm64`, 35,242,561 bytes. No local `ghcr.io/araihu/manja:main` image was available, and no digest-bound release image receipt exists.
- `site` has no release artifact; the direct `GOWORK=off go test ./... -count=1` check passed for `site/` at this candidate. That test result is not a site archive or redistribution clearance.

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
| Source tarball | No production release definition exists. The mechanical packager can inspect a complete source root and emit deterministic tar bytes only after authority clearance. The diagnostic candidate archive is recorded above; it contains retained upstream/vendor license files but no root project license, notice, third-party notice, or SBOM. The earlier audited-base archive at commit `39d65ade21c080ee2102f53da5ed741f000d6dd7` and tree `64cee6ab67060d1d8c4734fc5f54f6dbe6d272f6` contained 688 paths and remains a historical receipt. | Complete tracked tree, including copied/generated material and retained third-party acquisition bytes. No root `vendor/` tree is tracked, but `internal/webassets/vendor/` is shipped in a raw source archive. | Archive root: verified project `LICENSE`, accurate `NOTICE`, reviewed `THIRD_PARTY_NOTICES.md`, and deterministic source inventory/SBOM if policy requires it. Preserve required upstream license files and exclude unresolved generated concepts unless cleared. | Historical 688-path receipt plus current 846-path diagnostic archive are not release clearance. No candidate archive has passed the legal gate. |
| `manja` binary archive | No binary-archive release script, layout, or released bytes exist. `Pack` can only package a caller-supplied complete root after authority clearance. The Docker build creates a build-time `cmd/manja` compiler and a separate runtime-only `cmd/manja-runtime` binary; only the runtime binary enters the image. | Cannot be called a shipped archive yet. A future runtime archive would include Manja plus templ, Goshtoso, Chroma, regexp2, `x/text`, YAML, runtime static assets, and precompiled renderer data if it mirrors the image. Build-time parser/compiler modules and test tools must stay excluded unless actual archive bytes prove otherwise. | Archive root: `bin/manja`, project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, Go/browser/renderer-data SBOMs, runtime static tree, renderer config/allowlist, and all notices required by compiled catalogs. | No release script or candidate archive exists. When authorized, unpack the immutable output, run `go version -m bin/manja`, enumerate files, verify static/renderer digests, and compare SBOM/notices to final bytes. |
| OCI image `ghcr.io/araihu/manja` | Current Dockerfile builds with `golang:1.26.5-alpine`, compiles Kubernetes/GitHub/Stripe inputs, and copies only `manja-runtime` into `alpine:3.24`. Local `manja:provenance` inspection reported `linux/arm64`: 35,242,561-byte image, 9 layers, 17 final APK packages, 416 renderer-data files in three snapshots, renderer config/allowlist, and complete `internal/web/static`. Final stage has no `git`. | Runtime Go closure; Alpine base and `ca-certificates`; compiled Kubernetes/GitHub/Stripe renderer data; CSS/JS/SVG/PNG assets; both generated browser bundles. The blanket static copy also ships `request_composer_browser_test.go` and `schema_example_browser_test.go` as source files. Parser/compiler modules, Go toolchain, and integration/test dependencies remain build-only unless copied elsewhere. | `/usr/share/licenses/manja/`: verified project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, and OCI/Go/browser/renderer-data SBOMs. Preserve required Kubernetes, GitHub, Stripe, Simple Icons, and other attributions. Current image contains none of these paths. | Prior observed receipt: `docker build --pull=false -t manja:provenance .`; `docker history --no-trunc`; `docker image inspect`; run `/bin/sh` to enumerate APK and application files. The build command did not specify a platform; inspection reported `linux/arm64`. Local image ID: `sha256:dd2c84189b78ec4d84635667f7e97fafe74c4401e3f97e2be23c7178b2d50c09`. Future reproduction must use the explicit platform and base-digest checks in `provenance.md`. |
| Public `site` artifact | Separate module `github.com/araihu/manja/site` builds `site/cmd/server`; `site/internal/site/assets.go` embeds `static/*`. No site production archive/image exists; the mechanical packager can inspect a supplied site root but does not build or discover one. The direct `GOWORK=off go test ./... -count=1` check passed at this candidate, but no site artifact was built or inspected. | If distributed, site server production closure plus embedded CSS, JavaScript, SVG assets, and product copy. Root test/tool dependencies are not automatically part of the site. The remote campaign script is fetched at runtime and is not embedded in the site binary. | Beside site binary/archive: project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, site-specific Go/static SBOM, and a validated social preview asset. If deployed as an image, mirror OCI placement. | Test receipt: `GOWORK=off go test ./... -count=1` in `site/` exited 0. This does not replace a site release artifact, digest, inventory, or legal clearance. |

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

The current HEAD product-site handler emits route-specific title and
description, canonical URLs, `og:url`, the required Open Graph fields, explicit
X Card tags, and the tracked Manja preview URL in initial HTML for `/` and
`/docs`. The site handler also serves the Manja preview at
`/manja-assets/manja-social.png`; the focused tests verify its PNG bytes and
dimensions. These are current source/test observations, not proof of deployed
or lawfully cleared bytes. Portable preview regeneration, complete first-party
authority, and redistribution remain **BLOCKED**.

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

No Manja runtime/binary/site production archive, immutable release digest,
final layout, or authoritative archive manifest exists at this snapshot; CI
publishes only the OCI artifact. The internal packager is intentionally not a
release definition: it accepts a caller-supplied complete root and refuses
missing roots, selected subdirectories, drift, unknown files, incomplete SBOMs,
and uncleared legal material. Therefore no caller-supplied host directory can
currently prove released runtime/binary-package contents. The host archive gate
remains **BLOCKED** under Task 8 rather than reporting success from such a root.

The implemented Task-8 archive invariant is per archive: the packager receives
the immutable archive bytes and an independently trusted expected digest from
the release definition, verifies that digest, creates a fresh empty extraction
root, safely extracts the complete archive into that root itself, and
recursively scans that exact root. It rejects digest mismatch; unsafe or
incomplete extraction; path traversal or link escape; inventory or permission
errors; and any scan error or prohibited browser-test source. Each separately
produced runtime/binary archive must repeat the process in its own fresh root.
Regression coverage rejects substitution of a clean selected subdirectory while
prohibited bytes remain elsewhere. A sibling checksum file or marker inside the
extracted root is not independent authority. No production archive, release
digest, layout, or passing clearance receipt exists yet.

OCI distribution inspection is also **BLOCKED** until stopped Task 8 implements
digest-bound inspection and promotion. Existing CI-published OCI artifacts
remain uninspected and uncleared. No independently trusted release digest,
passing inspection receipt, or same-digest promotion proof is recorded.
Prior local inspection found the prohibited sources under
`/app/internal/web/static`, but that observation is not a digest-bound release
gate and cannot authorize publication.

Prospective Task-8 OCI inspection must fail closed under all these invariants:

- a trusted release definition supplies the exact image reference, matching
  `^ghcr\.io/araihu/manja@sha256:[0-9a-f]{64}$`; tags and unrelated
  caller-selected digests are rejected;
- every platform manifest published by that digest is inspected, rather than
  treating one local platform as multi-platform coverage;
- each inspection container is created with `docker create --network none` and
  is never started; image-supplied programs, shells, entrypoints, and commands
  are never executed;
- declared volumes are rejected unless their contents are separately and fully
  inspected, because `docker export` omits volume contents;
- `docker export` streams directly into a trusted host scanner without
  root-owned extraction. Export failure, archive parse failure, unsafe path or
  link, duplicate entry, special file, permission error, or incomplete scan all
  fail the gate;
- pipeline failure propagation and cleanup preserve the scanner result; neither
  a missing `pipefail` equivalent nor cleanup status may mask rejection;
- tag promotion and deployment consume successful inspection for the same exact
  digest, with no tag re-resolution or digest substitution between inspection
  and promotion.

Under those constraints, create/export inspection is sufficient only for this
flattened-filesystem filename exclusion. It does not cover declared volumes,
image metadata, SBOM provenance, or multi-platform publication by itself. A
scanner that reads saved OCI layers instead must implement layer-aware whiteout
and opaque-directory semantics before it can claim the equivalent final
filesystem view. These are prospective invariants, not implemented commands or
a current passing receipt.

This exclusion may be changed only if shipping those source files in a runtime
artifact is an intentional redistribution decision and an explicit notice/SBOM
policy review clears and inventories them. Their accidental presence under a
blanket static copy is not clearance. Task 8 remains blocked by the absent real
release artifacts, digest-bound OCI inspection, and other final packager gates;
the archive-owned extraction/scanning seam is implemented but has no production
receipt yet. OC-01 does not change the Dockerfile. The recorded current OCI
source presence remains a blocker until separately authorized packaging work
removes or explicitly clears it.
