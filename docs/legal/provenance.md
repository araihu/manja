# Provenance And Licensing Authority Gate

Date: 2026-08-11

Result: **BLOCKED**

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
| Manja marks, logos, and favicons | [`rights-holder-confirmation.md`](rights-holder-confirmation.md) names Guilherme de Castro as individual rights holder for those visual assets and scopes their MIT redistribution to `github.com/araihu/assets`. | Cleared only within that named visual-asset scope. It does not establish authority over Manja source, docs, product copy, social-preview compositions, or third-party material. | Keep this narrow confirmation; do not use it as repository-wide authority. |
| Proposed `NOTICE` holder and year range | History proves activity in 2026, but no evidence names the holder for the complete first-party body of work. | **BLOCKED**. No holder or year range may be placed in `NOTICE`. | Rights holder must approve the exact holder and evidence-supported year range after the complete authority question is resolved. |

Resolution owner, not a copyright conclusion: Guilherme de Castro, repository
maintainer. Resolution must attach or link evidence; it must not replace
`BLOCKED` with an assumption.

## Copied, Generated, Browser, And Static Material

| Item | Audited-base evidence and shipped disposition | Result |
| --- | --- | --- |
| Kubernetes OpenAPI v3 catalog | `catalog-source.json` pins 65 upstream files and the upstream `LICENSE` to Kubernetes commit `a818af18fe29d999d6741234c8cd72709ef2f424`, including every Git blob SHA. `receipt.json` adds SHA-256 values and counts. Muamba verification and `go test ./cmd/kubernetes-openapi-lock -count=1` pass. The OCI build compiles these inputs into renderer snapshot data. | Mechanical source-byte provenance is resolved. Redistribution remains **BLOCKED** because the final image contains derived snapshot data but not the retained Kubernetes license or project notice set. Task 8 must place and verify required notices after the project authority gate passes. |
| `internal/adapters/openapi/testdata/github-v3-rest.json` | The file identifies GitHub's REST API and declares MIT. Audited-base renderer configuration records the GitHub source repository and license URL. The exact upstream commit/blob for these bytes is still absent. The Docker build compiles the file into a shipped renderer snapshot even though the source JSON is not copied into the final image. | **BLOCKED** pending immutable upstream revision/blob evidence and reviewed attribution/notice disposition for the exact bytes. |
| Stripe OpenAPI input | The Docker build fetches `https://github.com/stripe/openapi.git` at commit `d70de345383dd818a0ce831f4e20d375c5a90cec`, compiles `openapi/spec3.json`, and ships only the derived snapshot. Renderer configuration links to the repository's MIT license on a mutable branch; no locked license bytes or hash are retained beside the pin. | Source revision is mechanically pinned. **BLOCKED** pending immutable license evidence for that revision and reviewed notice placement in final artifacts. |
| Other OpenAPI/config fixtures | Manja-specific fixtures and review/config inputs are tracked in the first-party history. | **BLOCKED** with the unresolved first-party authority item unless a fixture separately identifies an upstream source. |
| `internal/web/api.gen.go` | Header says `oapi-codegen` v2.7.1 generated it from the ignored `api/dist/openapi.yaml`; split `api/` sources are tracked. Audited-base `go.mod` pins `oapi-codegen` v2.8.0. | Input path is known, but generator version and audited-base module pin differ. Record as a reproducibility gap; do not claim pinned byte reproduction until regeneration/drift proof is reviewed. |
| `internal/web/templates/*_templ.go` | Headers identify templ-generated output; `.templ` inputs are tracked. | Generated source is attributable to tracked inputs. Final disposition remains **BLOCKED** with first-party authority and dependency-license classification. |
| Browser bundles | `schema-example.js` and `request-composer.js` are generated by `cmd/webassets`. Muamba verifies 31 exact npm-registry archives and retained license bytes. [`browser-bundles.md`](browser-bundles.md) records bundle membership from esbuild metafiles, SPDX labels, hashes, sources, licenses, and included files. `go run ./cmd/webassets check` passes at the audited base. | Mechanical browser provenance is verified at the audited base. Broader release clearance remains blocked by first-party authority and final-artifact notice/SBOM work. |
| GitHub and Stripe catalog marks | The SVGs state that their paths were adapted from Simple Icons under CC0-1.0. They do not record the exact Simple Icons version, upstream file hash, or source URL. The OCI image copies both SVGs. | **BLOCKED** pending immutable upstream evidence for the adapted paths and confirmation that the recorded CC0 source applies to those exact bytes. |
| `manja-social.svg` and `manja-social.png` | The tracked SVG is editable source; PNG is 1280x640 and SHA-256 `7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd`. No committed conversion command or distinct rights statement covers the complete social composition. Both ship in the OCI static tree. | **BLOCKED** with first-party authority; add reproducible SVG-to-PNG evidence before release packaging. |
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

- build base `golang:1.26.5-alpine` at image-config digest
  `sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2`;
- runtime base `alpine:3.24` at image-config digest
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

resolve_linux_arm64_manifest_digest() {
  docker buildx imagetools inspect --raw "$1" |
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
GOLANG_MANIFEST=$(resolve_linux_arm64_manifest_digest "$GOLANG_IMAGE")
ALPINE_MANIFEST=$(resolve_linux_arm64_manifest_digest "$ALPINE_IMAGE")

test "$(resolve_config_digest "$GOLANG_IMAGE" "$GOLANG_MANIFEST")" = \
  'sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2'
test "$(resolve_config_digest "$ALPINE_IMAGE" "$ALPINE_MANIFEST")" = \
  'sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b'
docker build --platform=linux/arm64 --pull=true -t manja:provenance .
test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' \
  manja:provenance)" = 'linux/arm64'
```

This is a future reproduction command, not a claim about the command used for
the prior observed image. Because both `FROM` references remain mutable tags,
Task 8 packaging remains **BLOCKED** until the Dockerfile digest-pins them or
the release gate compares their resolved platform digests with reviewed
expected values immediately before every build and verifies the built
platform.

The read-only resolver probe at this checkpoint selected these current tag
values:

- `golang:1.26.5-alpine`: `linux/arm64` manifest
  `sha256:787328cefd7937073af18fc4b3a725f47e011ffdde9c2908239a25cae6b2f02b`,
  config
  `sha256:766c0063a18bd23eff1d68216dd04832370d5f356af68c8b7683923c4c279f5f`;
- `alpine:3.24`: `linux/arm64` manifest
  `sha256:e7a1a92a5bfeee40966aea60f0796b0e7917cc35591542701834f03a68fa3d18`,
  config
  `sha256:1991bd789d7184290c3cce84fd6af068b8b745e9bddf178661ce7f5ecf68135c`.

Both config digests differ from the prior observed receipt, so the expected
value checks above currently stop before the build. No replacement image was
built for this correction checkpoint.

The final image contains no project `LICENSE`, `NOTICE`, third-party notices,
SBOM, or retained Kubernetes/Stripe/GitHub license path. It also copies the two
browser-test `.go` source files found under `internal/web/static`; those files
are shipped bytes even though their Go dependencies are not part of the
runtime binary.

## Gate Decision

The gate remains **BLOCKED** for independent reasons:

1. repository-wide first-party ownership and licensing authority are not
   established;
2. GitHub, Stripe-license, Simple Icons, and social-preview evidence remains
   incomplete;
3. generated API output is not proven byte-reproducible with the audited-base
   pinned generator; and
4. audited-base source/OCI artifacts do not carry a complete project license,
   notices, or SBOM set.

Accordingly, this checkpoint creates no root `LICENSE`, `NOTICE`,
`THIRD_PARTY_NOTICES.md`, Apache badge/metadata, SBOM, or release packager.
Task 8 of the Open Core plan remains stopped until this document is changed to
`PASS` from concrete rights-holder and redistribution evidence.
