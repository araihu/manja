# Provenance And Licensing Authority Gate

Date: 2026-08-15

Result: **BLOCKED**

## Current exact-base revalidation

The current authority/provenance revalidation is recorded in
[`authority-revalidation-2026-08-15.md`](authority-revalidation-2026-08-15.md).
It began from the exact clean freeze requested for this checkpoint:

- ref: `refs/heads/codex/opencore-reconciler-next`;
- commit: `4fda300572506d12439a9427a4feb6976b0c4204`;
- tree: `15d215844b92e0c883a14a516a643832c5cac627`;
- parent: `8035a9faefa9b8ed45032375dd63d7cfd635cd1f`;
- `origin/main` and merge-base: `507c5ea9fcdc8cee670a023dbb82f348ba2ed763`.

The receipt found no independently verifiable first-party assignment or
trademark permission. The gate therefore remains **BLOCKED**. Adding the
receipt changes the candidate tree and requires a fresh identity-bound freeze.

This inventory is an engineering release gate, not a legal conclusion. Manja
must not claim Apache-2.0, publish a root `LICENSE` or `NOTICE`, or add Apache
package metadata until every blocking item below is resolved with evidence.

## Audited Base Snapshot And Evidence

The history counts and command receipts below were collected from a clean
checkout of this exact audited base:

- audited base commit: `39d65ade21c080ee2102f53da5ed741f000d6dd7`;
- audited base tree: `64cee6ab67060d1d8c4734fc5f54f6dbe6d272f6`;
- history: 280 commits from 2026-06-06 through 2026-08-10;
- toolchain: Go 1.26.5.

The candidate commit and tree for a review of this document are supplied by the
immutable external review packet and control plane. This file cannot embed its
own candidate commit or tree without changing that identity and creating a
self-reference.

The audited-base evidence commands included:

```bash
AUDITED_BASE=39d65ade21c080ee2102f53da5ed741f000d6dd7
test "$(git rev-parse HEAD)" = "$AUDITED_BASE"
git status --short --branch --untracked-files=all
git shortlog -sne "$AUDITED_BASE"
git log --reverse --format='%ad %H %an <%ae>' --date=short "$AUDITED_BASE"
rg -n 'Copyright|SPDX-License-Identifier|Licensed under|generated|DO NOT EDIT' .
go list -m all
GOWORK=off go list -deps -tags=manja_runtime ./cmd/manja-runtime
go tool muamba verify --strict
go run ./cmd/webassets check
go test ./cmd/kubernetes-openapi-lock -count=1
git archive --format=tar "$AUDITED_BASE" | tar -tf -
docker build --pull=false -t manja:provenance .
docker history --no-trunc manja:provenance
docker image inspect manja:provenance
docker run --rm --entrypoint /bin/sh manja:provenance -c 'apk info -vv'
```

`git shortlog -sne "$AUDITED_BASE"` reports 275 commits by
`Guilherme de Castro <guilherme.castro@totvs.com.br>`, three by Dependabot,
and two by the AraiHu asset-distribution bot. Git authorship and bot identity
show who recorded changes; neither establishes copyright ownership, employment
rights, assignment, or authority to license the resulting repository.

No tracked root `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, copyright
assignment, employer waiver, CLA, or repository-wide licensing-authority
record exists. The root has no npm manifest or lockfile; `npm ls --all --json`
returns an empty object. Browser inputs are instead acquired and locked by
Muamba. The root Go acquisition graph contains 147 modules, but only final
artifact inspection determines redistribution scope.

## First-Party Authority

| Body of work | Repository evidence | Authority result | Required decision |
| --- | --- | --- | --- |
| Go, JavaScript, templ, YAML, CSS, documentation, configuration, generated inputs, and product copy | Audited-base history is predominantly the maintainer identity above, with automated dependency and asset updates. The maintainer address is on the `totvs.com.br` domain. | **BLOCKED**. Commit authorship does not decide whether the individual, an employer, another entity, or several parties own the work. | The rights holder must provide durable written evidence naming the owner, confirming authority over the complete first-party work, and resolving employment or assignment rights. |
| Manja marks, logos, and favicons | [`rights-holder-confirmation.md`](rights-holder-confirmation.md) is an explicit **BLOCKED** maintainer draft, not an executed assignment or independent rights-holder receipt. | **BLOCKED**. No first-party visual-asset ownership, reproduction, licensing, redistribution, or trademark permission is cleared. | Obtain an immutable written instrument naming the holder, exact visual scope, employment/assignment status, and the permissions claimed. |
| Proposed `NOTICE` holder and year range | History proves activity in 2026, but no evidence names the holder for the complete first-party body of work. | **BLOCKED**. No holder or year range may be placed in `NOTICE`. | Rights holder must approve the exact holder and evidence-supported year range after the complete authority question is resolved. |

Resolution owner, not a copyright conclusion: Guilherme de Castro, repository
maintainer. Resolution must attach or link evidence; it must not replace
`BLOCKED` with an assumption.

## Copied, Generated, Browser, And Static Material

| Item | Audited-base evidence and shipped disposition | Result |
| --- | --- | --- |
| Kubernetes OpenAPI v3 catalog | `catalog-source.json` pins 65 upstream files and the upstream `LICENSE` to Kubernetes commit `a818af18fe29d999d6741234c8cd72709ef2f424`, including every Git blob SHA. `receipt.json` adds SHA-256 values and counts. Muamba verification and `go test ./cmd/kubernetes-openapi-lock -count=1` pass. The OCI build compiles these inputs into renderer snapshot data. | Mechanical source-byte provenance is resolved. Redistribution remains **BLOCKED** because the final image contains derived snapshot data but not the retained Kubernetes license or project notice set. Task 8 must place and verify required notices after the project authority gate passes. |
| `internal/adapters/openapi/testdata/github-v3-rest.json` | `github-v3-rest.provenance.json` binds the 3,319,366 tracked bytes (SHA-256 `dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977`) to `github/rest-api-description` commit `6948cb04f5304188569c4bb4ae2190c08e7cbdba`, tree `6270ed1bd31a741adf3c7143c39d9bdc57d2fbc1`, path `descriptions/ghes-3.0/ghes-3.0.json`, and Git blob `f0ddf34ad4398c319db0643e45a0908ca026b382`. The same receipt records that revision's MIT `LICENSE.md` as Git blob `b50625eb63949013cae604b1cadd42cfa1eaf825` and SHA-256 `3243761cbac07e6d169a5a2f4e7c25cc544da85248e735df74c3672e055cc87b`; renderer source and license links use that immutable commit. The Docker build compiles the file into a shipped renderer snapshot even though the source JSON is not copied into the final image. | Mechanical source-byte and same-revision license identification are resolved. Redistribution remains **BLOCKED** pending reviewed attribution/notice placement in the final artifact and the independent first-party authority gate. |
| Stripe OpenAPI input | The Docker build passes the checked renderer configuration to `manja build`; its Git catalog source fetches `https://github.com/stripe/openapi.git` at commit `d70de345383dd818a0ce831f4e20d375c5a90cec` and compiles `openapi/spec3.json`. `stripe-openapi.provenance.json` binds that 3,840,021-byte path to commit tree `a7e155600c10dcfab91a94070b0e954419255862`, Git blob `058edc82a247c71f05b94dfa6b9cef0a794a1358`, and SHA-256 `8b608cba7129d121f12358a7092574e176833fe8cb4c9fcead178c71c545f870`; it separately records the same commit's 1,095-byte `The MIT License` at `LICENSE`, Git blob `edf2d132d8bb95146e05585c3a782d059298b46b`, and SHA-256 `8c1ce883f4eee7b531e0b7872dbfc72d410ced87dfff9501305de05ca8d203e5`. Renderer source and license URLs use the immutable commit. The runtime-only `stripe-openapi.integrity.json` contains no license claim and fixes the exact clone string, SHA-1 object format, commit, tree, source root, regular-file mode, size, Git blob, and raw-byte SHA-256. `renderer.yaml` selects that adjacent receipt. During `manja build`, the Git source verifies the repository metadata and exact captured inventory before blob admission, then recomputes the Git object ID and SHA-256 over the same single-read byte slice returned to the catalog candidate. Ordinary offline tests do not fetch Stripe. | Mechanical source-byte admission and same-revision license identification are resolved to the recorded scope. Redistribution remains **BLOCKED** pending reviewed attribution/notice placement in final artifacts, the independent first-party authority gate, and Task 8. |
| Other OpenAPI/config fixtures | Manja-specific fixtures and review/config inputs are tracked in the first-party history. | **BLOCKED** with the unresolved first-party authority item unless a fixture separately identifies an upstream source. |
| `internal/web/api.gen.go` | Header and current `go.mod` tool graph both identify `oapi-codegen` v2.8.0. Bundling the tracked split `api/` sources reproduces `api/dist/openapi.yaml` at SHA-256 `a11d2b149c7dc0d9f088b39113f0ff0129e051d672eb803a7112d2b996e32176`; the canonical pinned generator command reproduces `internal/web/api.gen.go` at SHA-256 `7a9f31c2b80feca1c8297bf29d12f5f4017c40d1bf27fd37ad39b09b4c0fb5a9`. CI now runs that generation before `git diff --exit-code`. | Mechanical generator-version and exact-byte drift gap is resolved for this checkpoint. First-party authority and broader dependency-license clearance remain **BLOCKED** independently. |
| `internal/web/templates/*_templ.go` | Headers identify templ-generated output; `.templ` inputs are tracked. | Generated source is attributable to tracked inputs. Final disposition remains **BLOCKED** with first-party authority and dependency-license classification. |
| Browser bundles | `schema-example.js` and `request-composer.js` are generated by `cmd/webassets`. Muamba verifies 31 exact npm-registry archives and retained license bytes. [`browser-bundles.md`](browser-bundles.md) records bundle membership from esbuild metafiles, SPDX labels, hashes, sources, licenses, and included files. `go run ./cmd/webassets check` passes at the audited base. | Mechanical browser provenance is verified at the audited base. Broader release clearance remains blocked by first-party authority and final-artifact notice/SBOM work. |
| GitHub and Stripe catalog marks | `simple-icons.provenance.json` binds both adapted marks to official Simple Icons version `16.28.0`, commit `fc91ef03ec113d06627b2d47c1f9644ca202b6f9`, and tree `4c01339d8cafffdd7a6a59837b2fc0bbc5ad6e92`. At that revision, `icons/github.svg` is 822 bytes, Git blob `538ec5bf2a9a5724899daf728577cd0b8beaae90`, SHA-256 `3bf8cceead820aec50d4ee825a3fd02c5a1cd6665cc9cf4cbf3d9c8861a204bb`; `icons/stripe.svg` is 588 bytes, Git blob `8ebadf74d367a5a9bd7deb45a53f1844fc08a095`, SHA-256 `130c6d957b8977f5eda2928267b9df531ca038a400a801765d263801bb1bd870`. Their respective 712- and 478-character `d` values match the tracked marks exactly, with SHA-256 `d82e21f6c9bfbfd889fed4b8d8604121be1d364ef75b7fe42cc9c0b8737ae529` and `e3b2e90079bcdca94a620b4a871c4ad4f39a448b23b25066340531ce3d701d71`. Local adaptation adds the accessible title, 64-by-64 circular wrapper/background, white fill, and exact path transform `translate(14 14) scale(1.5)`; whole local SVG size, Git blob, and SHA-256 are fixed in the receipt and recomputed offline. The same revision's 6,569-byte `LICENSE.md` identifies `CC0 1.0 Universal` / SPDX `CC0-1.0`, Git blob `70d4a7b6740c5d9b594ff2fc27d3ea7e89413185`, SHA-256 `9046848b63a5c92bff14e4accca80bd987e0623b74adf9226ce5198d312b79d5`. | Mechanical upstream path-byte, local adaptation, and same-revision CC0 identification are resolved. This is not trademark permission or final redistribution clearance: CC0 expressly leaves trademark rights unaffected, and GitHub/Stripe brand disposition plus reviewed notice/attribution placement in shipped artifacts remain **BLOCKED**. |
| `manja-social.svg` and `manja-social.png` | `manja-social.provenance.json` binds both files to introduction commit `b2df3a5f0d67c6a04539f96c804b404a5236c1d4` and tree `12b20134b059f2fce38041597f021a36ecd7f61a`. It records the 2,198-byte SVG as Git blob `5cb2fda632e511e4eeccec6412858f6f630bc6c9` and SHA-256 `002b05823a870f28ff28d12fe0b793cee979418435bb4ff4c4a634affc7b2fe2`, and the 21,500-byte 1280x640 RGB PNG as Git blob `9260d190361cceeef611f3a2178f14c613b0f533` and SHA-256 `7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd`. Two observed runs of `LC_ALL=C.UTF-8 rsvg-convert --format=png --width=1280 --height=640 internal/web/static/manja-social.svg` produced the exact tracked PNG under the recorded macOS 26.5.2 arm64, librsvg 2.62.1/library, and system Arial font hashes. The offline test fixes every receipt field independently, recomputes both tracked blobs/hashes and image properties, rejects controlled coordinated drift, and checks the asset handler response. It does not rerun the renderer in CI. Both assets ship in the inspected OCI static tree; no candidate image was rebuilt here, and the adjacent receipt will enter a later image through the unchanged blanket static copy. | The narrow source/output relationship has exact observed evidence, but this is not portable/general reproducibility: the exact host renderer and proprietary system-font bytes are not durably or lawfully pinned for CI/release acquisition. That environment gate and first-party authority over the complete social composition remain **BLOCKED**; no font bytes are redistributed by this receipt. |
| `kubernetes-social.png` | PNG is 1280x640, 48,705 bytes, and SHA-256 `a7cf0baba81cf79fdbe8a0487bd30ed1b6a34dc816ec8345a50591a99a2db423`. No editable source, generator receipt, generation terms, or Kubernetes visual-asset attribution accompanies it. It ships in the OCI static tree. | **BLOCKED** pending source/generation provenance and an explicit redistribution/trademark disposition from the responsible rights holder. |
| Three PNG concepts under `docs/brand/logo-concepts/2026-06-08/` | Commit history and the adjacent README describe generated concepts and retain prompts, but do not name the generator, account, model, generation terms, or redistribution decision. They enter a raw source archive but not the OCI image. | **BLOCKED**. Exclude from a distributed source archive or document generator provenance and rights before distribution. |
| First-party CSS and JavaScript | Tracked source has no external copyright header. No font binaries or tracked screenshots were found. | **BLOCKED** only by first-party authority, except for generated browser/vendor inputs listed separately. |

## Go And Container Inputs

`GOWORK=off go list -deps -tags=manja_runtime ./cmd/manja-runtime` shows the
runtime binary contains Manja plus six external module bodies: templ, Goshtoso,
Chroma, regexp2, `golang.org/x/text`, and `gopkg.in/yaml.v3`. Parser/compiler
modules used by `manja build`, Playwright, Testcontainers, Forgejo, code
generators, and their transitive graphs are not in that runtime closure merely
because they occur in `go.mod`.

A local `manja:provenance` image built successfully from the audited-base
Dockerfile. This prior observation used the audited command
`docker build --pull=false -t manja:provenance .`; it did not pass an explicit
`--platform` argument. Inspection identified the resulting image as
`linux/arm64`, 35,242,561 bytes, using:

- build base `golang:1.26.5-alpine` at OCI index digest
  `sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2`;
- runtime base `alpine:3.24` at OCI index digest
  `sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`;
- 17 final APK packages, including Alpine base packages and
  `ca-certificates`; no final-stage `git`;
- `/usr/local/bin/manja`, 416 renderer-data files across Kubernetes, GitHub,
  and Stripe snapshots, renderer configuration/allowlist, and the complete
  `internal/web/static` tree.

A future reproduction must make the platform explicit and validate the
mutable base tags before building. The expected values for this audited
receipt can be checked with:

```bash
set -euo pipefail

resolve_tag_index_digest() {
  descriptor=$(docker buildx imagetools inspect "$1" \
    --format '{{json .Manifest}}')
  test "$(jq -r '.mediaType' <<<"$descriptor")" = \
    'application/vnd.oci.image.index.v1+json'
  jq -er '.digest' <<<"$descriptor"
}

resolve_linux_arm64_manifest_digest() {
  docker buildx imagetools inspect --raw "${1}@${2}" |
    jq -er '[.manifests[] |
      select(.platform.os == "linux" and .platform.architecture == "arm64") |
      .digest] |
      if length == 1 then .[0] else error("expected one linux/arm64 manifest") end'
}

resolve_config_digest() {
  docker buildx imagetools inspect --raw "${1}@${2}" | jq -er '.config.digest'
}

GOLANG_IMAGE=docker.io/library/golang:1.26.5-alpine
ALPINE_IMAGE=docker.io/library/alpine:3.24
GOLANG_INDEX=$(resolve_tag_index_digest "$GOLANG_IMAGE")
ALPINE_INDEX=$(resolve_tag_index_digest "$ALPINE_IMAGE")

test "$GOLANG_INDEX" = \
  'sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2'
test "$ALPINE_INDEX" = \
  'sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b'

GOLANG_MANIFEST=$(resolve_linux_arm64_manifest_digest \
  "$GOLANG_IMAGE" "$GOLANG_INDEX")
ALPINE_MANIFEST=$(resolve_linux_arm64_manifest_digest \
  "$ALPINE_IMAGE" "$ALPINE_INDEX")

test "$GOLANG_MANIFEST" = \
  'sha256:787328cefd7937073af18fc4b3a725f47e011ffdde9c2908239a25cae6b2f02b'
test "$ALPINE_MANIFEST" = \
  'sha256:e7a1a92a5bfeee40966aea60f0796b0e7917cc35591542701834f03a68fa3d18'

test "$(resolve_config_digest "$GOLANG_IMAGE" "$GOLANG_MANIFEST")" = \
  'sha256:766c0063a18bd23eff1d68216dd04832370d5f356af68c8b7683923c4c279f5f'
test "$(resolve_config_digest "$ALPINE_IMAGE" "$ALPINE_MANIFEST")" = \
  'sha256:1991bd789d7184290c3cce84fd6af068b8b745e9bddf178661ce7f5ecf68135c'
docker build --platform=linux/arm64 --pull=true -t manja:provenance .
test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' \
  manja:provenance)" = 'linux/arm64'
```

This is a future reproduction command, not a claim about the command used for
the prior observed image. Because both `FROM` references remain mutable tags,
Task 8 packaging remains **BLOCKED** until the Dockerfile digest-pins them or
the release gate compares their resolved OCI index digests with reviewed
expected values immediately before every build, verifies the selected platform
manifest and config digests, and verifies the built platform.

The read-only resolver probe at this checkpoint confirmed that the live tags
still resolve to the prior observed OCI indexes. Those indexes select:

- `golang:1.26.5-alpine`: `linux/arm64` manifest
  `sha256:787328cefd7937073af18fc4b3a725f47e011ffdde9c2908239a25cae6b2f02b`,
  config
  `sha256:766c0063a18bd23eff1d68216dd04832370d5f356af68c8b7683923c4c279f5f`;
- `alpine:3.24`: `linux/arm64` manifest
  `sha256:e7a1a92a5bfeee40966aea60f0796b0e7917cc35591542701834f03a68fa3d18`,
  config
  `sha256:1991bd789d7184290c3cce84fd6af068b8b745e9bddf178661ce7f5ecf68135c`.

The index, `linux/arm64` manifest, config, and existing final-image platform
assertions pass. No replacement image was built for this correction
checkpoint. Mutable tags remain a prospective Task 8 risk because a future tag
update could change the index unless the Dockerfile is digest-pinned or the
release gate performs the index comparison immediately before each build.

The final image contains no project `LICENSE`, `NOTICE`, third-party notices,
SBOM, or retained Kubernetes/Stripe/GitHub license path. It also copies the two
browser-test `.go` source files found under `internal/web/static`; those files
are shipped bytes even though their Go dependencies are not part of the
runtime binary.

## Gate Decision

The gate remains **BLOCKED** for independent reasons:

1. repository-wide first-party ownership and licensing authority are not
   established;
2. Simple Icons CC0 source evidence does not resolve GitHub/Stripe trademark or
   brand-use disposition; portable Manja social-preview regeneration remains
   blocked on exact renderer/font acquisition, while Kubernetes social-preview
   source, generation, and trademark provenance remain incomplete;
3. audited-base source/OCI artifacts do not carry a complete project license,
   notices, or SBOM set.

Mechanical source/license identification for the GitHub REST fixture and
Stripe OpenAPI input, exact generated-API reproducibility, Simple Icons mark
adaptation evidence, and the observed Manja SVG-to-PNG source/output
relationship are resolved only to the degrees recorded above. Those mechanical
results do not clear the independent blockers.

Accordingly, this checkpoint creates no root `LICENSE`, `NOTICE`,
`THIRD_PARTY_NOTICES.md`, Apache badge/metadata, SBOM, or production release
artifact. The mechanical archive/evidence seam exists in
`internal/distribution`, but it is not a production release definition and
does not clear Task 8. Production distribution and OCI inspection remain
stopped until this document is changed to `PASS` from concrete rights-holder
and redistribution evidence.
