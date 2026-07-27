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

Resolution: version-1 projection metadata owns the future canonical anchor,
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

Resolution: the future GitHub fixture test is a blocking measurement gate with
an explicitly unknown pre-implementation outcome. A `record_too_large` or
`projection_too_large` result stops delivery for a reviewed DTO-reduction
amendment; it does not authorize raising a bound, omitting required fields ad
hoc, or bypassing the codec. The POC's raw size remains risk evidence, not a
projected-size claim.

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
