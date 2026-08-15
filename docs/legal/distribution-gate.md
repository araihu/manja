# Self-hosted distribution evidence gate

Status: **BLOCKED**.

This document describes the bounded engineering seam in
`internal/distribution` and `cmd/distribution-gate`. It is an evidence
validator, not a legal opinion or a clearance decision. It does not create or
claim a project `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, SBOM, release
archive, binary package, or OCI image. No such release file is invented while
the repository provenance gate remains blocked.

## Input and output

The gate consumes one JSON evidence object supplied by the caller. It binds
the object to a full lowercase Git commit and tree SHA-1, and records separate
provenance and rights-holder authority receipts. A `PASS` authority receipt
must carry an immutable reference and a SHA-256 or SHA-384 digest. A
`BLOCKED` authority receipt remains blocked even when every mechanical hash
check succeeds; material or holder attribution supplied before that point is
reported as `legal.materials.before_clearance`.

For each dependency, the evidence records ecosystem, immutable version,
license identifier, scope (`shipped`, `build-only`, or `test-only`), source,
and a reproducible digest. For each produced artifact, it records the exact
source identity and digest, a complete/fresh/digest-bound inspection receipt,
a complete CycloneDX-JSON or SPDX-JSON SBOM receipt, and a recursive regular
file inventory with sizes and digests. Artifact dependency names must resolve
to exactly one dependency evidence record; build-only and test-only records
cannot enter a produced artifact.

`LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES.md` are required in every
runtime artifact only after both authority receipts are `PASS`. Their file
paths, regular-file type, positive size, and immutable digest are checked;
the validator never writes or synthesizes their contents. The same required
paths must be present in the artifact inventory.

OCI evidence additionally requires an explicit complete-coverage assertion
and one immutable digest for every published OS/architecture manifest. An
incomplete platform list is a blocker; a single local platform does not prove
multi-platform coverage.

## Exclusion gates

Every artifact inventory fails closed on missing or duplicate paths,
absolute/backslash/NUL/parent-traversal/non-canonical paths, links,
directories, special files, non-positive sizes, or invalid digests. Runtime
artifacts (`binary-archive`, `oci`, and `site`) reject the known browser-test
sources:

- `internal/web/static/request_composer_browser_test.go`
- `internal/web/static/schema_example_browser_test.go`

Raw source archives are intentionally not subject to that source exclusion,
but they still require complete digest-bound inspection, dependency evidence,
SBOM evidence, and legal-file evidence after authority clearance. This is the
current narrow policy, not a statement that any source or generated asset is
lawfully redistributable.

## Commands

Both commands write only to stdout; neither creates a release file or mutates
the worktree:

```bash
GOWORK=off go run ./cmd/distribution-gate canonical -input /path/to/evidence.json
GOWORK=off go run ./cmd/distribution-gate check -input /path/to/evidence.json
```

`canonical` emits stable JSON bytes with sorted dependency, artifact, file,
platform, and artifact-dependency arrays. `check` emits a raw deterministic
receipt and exits `0` for `PASS`, `1` for `BLOCKED`, and `2` for malformed
input or I/O failure. Unknown JSON fields and trailing JSON values are
rejected. A `BLOCKED` result is expected until independently reviewed
provenance and rights-holder evidence exists; it must not be converted into a
license claim by CI or packaging code.

The seam deliberately does not inspect a live image, build an archive, run an
image entrypoint, generate an SBOM, or infer a dependency graph from
`go.mod`. A future producer may supply those receipts only after a separately
authorized packaging checkpoint has real immutable bytes to inspect. Muamba
remains the source-acquisition lock for existing browser inputs; its strict
verification and generated-output checks are independent gates.

## Verification

The package and command tests exercise blocked authority, pre-clearance legal
claims, unknown licenses, mutable versions, build/test leakage, excluded
runtime sources, unsafe paths, non-regular files, incomplete extraction roots,
missing or incomplete SBOMs, incomplete OCI coverage, canonical byte
stability, and strict JSON decoding. The complete evidence fixture in those
tests is synthetic and is not a production clearance receipt.
