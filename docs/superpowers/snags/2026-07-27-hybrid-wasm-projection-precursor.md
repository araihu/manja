# Hybrid Wasm Projection-Only Precursor Snags

## Public Application Service Versus Internal Serialization

**Date:** 2026-07-27

The approved hybrid design calls the projection builder a provider-neutral
application service while also stating that serialization and browser-runtime
adapters stay internal. Manja's current architecture compounds that distinction:
reusable orchestration belongs in public `application`, but the open-core plan
warns against exporting presentation behavior without a stable need.

Resolution: the precursor design places the deterministic builder and concrete
DTO in public `application/projection`, with `domain.SpecIndex` as its only
product input. It places strict canonical JSON and SHA-256 in
`internal/adapters/projectionjson`. An unrelated module proves the public DTO
boundary; an independent Node fixture proves the wire without receiving an
exported Go codec. This keeps HTTP, browser, storage, and rendering details
internal while honoring the approved application-service decision.

Consequence: exported DTO fields become a semantic-versioned Go contract. If
reviewers reject that compatibility cost, implementation must stop for a design
amendment; moving everything under `internal` is not an unrecorded workaround.

## Canonical Navigation Logic Is Duplicated

**Date:** 2026-07-27

Source inspection found anchor/href fallback logic in three places:

- `internal/adapters/openapi/kin.go` on frozen main;
- `internal/web/templates/public.templ` on frozen main;
- `internal/pwa/router.go` in the preserved POC.

Operation fallback behavior is not identical in every path, and schema fallback
currently lowercases a raw name rather than producing a route-safe slug. The POC
also carries local `/api/spec/*` routing that the precursor cannot adopt.
This is observable on frozen main: schema `author_association` produces legacy
search/public targets `schema-author_association`, while the new ASCII slug is
`schema-author-association`.

Resolution: version-2 projection metadata owns the future canonical anchor,
base-relative selected href, heading, and landmark values. The precursor does
not modify the current server renderer. Later server/Wasm parity work must
consume projection values and remove duplicate derivation under its own tests.
Supplied `Operation.Anchor` remains authoritative; fallback operation/schema
anchors use one documented ASCII slug helper. POC values remain compatibility
fixtures, not exact-port code. At the input boundary only, the builder registers
the frozen parser's exact lowercase schema alias and resolves it uniquely to the
canonical slug before rebuilding output hrefs/routes. No general fuzzy aliasing
or old route output survives.

## Raw POC Snapshot Violates the Approved Projection Contract

**Date:** 2026-07-27

The POC engine serializes the complete `domain.SpecIndex` with `json.Marshal`.
That includes `SpecDownload.JSON` and `ExampleSpecJSON`, depends on an OpenAPI
parser in the Wasm path, carries arbitrary embedded JSON strings and excluded
raw bytes alongside otherwise concrete structs/slices/pointers, and decodes
with permissive `json.Unmarshal`. Its measured GitHub fixture snapshot is
25,844,624 bytes, already above the approved 16 MiB projection limit.

Resolution: no POC engine/codec line is ported. New tests begin from a direct
`domain.SpecIndex` fixture and require explicit DTOs, sentinel exclusion,
pre-`Marshal` UTF-8 validation, stable slices/ordinals, strict duplicate and
unknown-field rejection, canonical byte equality, exact bounds, and exact-byte
SHA-256. The GitHub fixture remains scale evidence only.

## Display Examples Have Lost Source Type Information

**Date:** 2026-07-27

`domain.SpecIndex` stores parameter, media-type, schema-summary, and explicit
schema examples as strings. A string such as `true`, `1e+03`, or
`{"looks":"json"}` no longer says whether the OpenAPI source was a string,
Boolean, number, or object. `domain.SchemaExample` also has two separate
payloads: `JSON` is schema input, while `Example` is the explicit example text.
Treating every example string as JSON would invent types and could overwrite
one schema payload with the other.

Resolution: the precursor preserves all display examples/defaults as exact
UTF-8 text and never infers their type. Only fields explicitly named `JSON`
(`SchemaSummary.JSON` and `SchemaExample.JSON`) enter strict embedded-JSON
canonicalization. `SchemaDetail.exampleSchemaJSON` and its primary text example
remain separate, with a paired-sentinel test. A later interactive renderer that
needs typed example values requires a reviewed domain-contract extension; this
precursor cannot manufacture that metadata. The parent design's canonical
decimal rule is enforceable here only for explicitly typed embedded-JSON number
tokens; applying it to ambiguous display strings would silently change valid
string examples.

## Preserved Generated Templates Are Stale

**Date:** 2026-07-27

The POC modified `internal/web/templates/public.templ` and regenerated
`public_templ.go` across several parity/update commits. The POC branch diverged
at `6216bcb2d9a50083b130daa37cf381c6e2b4b601` and is now 22 commits behind the
frozen main used by this design. Current main contains later Goshtoso and
application-structure work. The POC snag record also notes large generated
source-line expansion from small template changes.

Resolution: reject all POC `.templ` and generated `_templ.go` code for the
precursor. No template is edited or generated by this docs-only task. Later UI
work starts from then-current source `.templ` and regenerates; it never
hand-merges generated output.

## GitHub Fixture Projection Size Is Not Yet Proven

**Date:** 2026-07-27

The preserved POC measured the same GitHub fixture's full `SpecIndex` JSON at
25,844,624 bytes. Removing `SpecDownload.JSON` should save several MiB, but the
precursor also adds explicit IDs, ordinals, directories/details, and canonical
metadata while retaining bounded schema-detail JSON. Source inspection cannot
prove that the resulting projection will fit the approved 16 MiB total bound.

Resolution: the GitHub fixture test is a blocking measurement gate. Its first
version-1 run stopped delivery and triggered the independently reviewed
format-version-2 global-schema-interning amendment recorded below. The unchanged
bounds still forbid raising a limit, omitting required fields ad hoc, or
bypassing the codec. The POC's raw size remains risk evidence, not a projected
size claim.

### Implementation measurement

**Date:** 2026-07-27

The frozen-base implementation reached the blocking scale gate. Two fresh runs
both reported `operationDetails[570]: record_too_large bytes=282995
limit=262144`. Bounded diagnostic measurement then established this literal
version-1 evidence:

- complete document: 25,026,875 bytes;
- operation detail at canonical index 570: 282,995 bytes;
- dominant response within that record: 261,580 bytes;
- dominant schema within that response: 233,624 bytes;
- dominant example within that schema: 27,784 bytes;
- schema roots across operation parameters/media and schema details: 3,453;
- unique flat schema nodes: 2,236.

The owning record was identified only by bounded canonical index; its source
content was not logged. The inclusive 262,144-byte shallow-operation,
524,288-byte shallow-schema-detail, and 16,777,216-byte document limits remain
unchanged.

Independent read-only review returned GO for a version-2 global schema-node
interning amendment and STOP for implementation until the exact design and plan
are parent-accepted. The amended shape adds `Document.schemaNodes`, uses
`SchemaRef uint32` for every root/child edge, content-addresses nodes from a
domain-separated length-framed semantic preimage with raw child digests, and
strictly rejects invalid graph structure. Every schema display and embedded-JSON
value remains present; no omission or limit increase is authorized.

The parent's conservative diagnostic estimate is 8,067,080 total version-2
bytes, maximum shallow operation 35,906 bytes, and maximum node 30,657 bytes.
That estimate is design evidence only, not final acceptance. Delivery resumes
only after the canonical version-2 scale proof passes twice with identical
digest while reporting total, maximum operation, maximum schema detail, maximum
node, root count, and unique-node count.

Outcome: this turn commits only the version-2 design/plan/snag checkpoint and
stops for parent acceptance. The pre-existing uncommitted Task 5 files preserve
the Wasm/scale attempt unchanged. No production/test file is edited, staged,
committed, discarded, or cleaned.

### External consumer tidy closure

**Date:** 2026-07-27

Fresh Task 6 repository verification found one dependency-receipt snag. The
unrelated external module imported `application/projection`; Go's tidy package
loading then included `application/projection` tests, whose approved GitHub
scale proof imports `internal/adapters/openapi`. The focused external consumer
test and vet passed, but `GOWORK=off go mod tidy -diff` requested checksums for
the parser's already-root-declared transitive modules.

Resolution: the parent independently reproduced and narrowly authorized the
exact mechanical `integration/testdata/external-module/go.sum` update. It adds
14 checksum lines: module and `go.mod` hashes for kin-openapi v0.140.0,
jsonpointer v0.22.5, swag/jsonname v0.25.5, oasdiff/yaml v0.1.0,
oasdiff/yaml3 v0.0.13, jsonschema/v6 v6.0.2, and x/text v0.34.0. No `go.mod`,
version, replace directive, production dependency, test placement, or build tag
changes. The scale test remains literal and the external tidy-diff gate remains
enabled.

## No Repository Markdown Link Checker

**Date:** 2026-07-27

Repository scripts and `package.json` expose API, generation, examples, CSS,
and dev-server gates but no Markdown link/path validator. This required a
docs-only workaround rather than adding tooling outside scope.

Resolution: validate local Markdown targets with a read-only script during this
task, then run `git diff --check` and an exact owned-path audit. No new package,
module, JavaScript utility, or CI change is added.

## Goshtoso Checkpoint

**Date:** 2026-07-27

No current Goshtoso component/helper/source inspection was needed because this
precursor contains no UI or assets. The preserved POC's Goshtoso asset extraction
and banner-wrapper snags remain evidence for later runtime/UI nodes; neither is
reclassified or solved here.
