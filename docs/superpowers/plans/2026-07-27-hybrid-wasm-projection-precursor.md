# Hybrid Wasm Projection-Only Precursor Implementation Plan

> **Authority:** Execute only after this plan and its design are reviewed and
> merged. This plan authorizes projection-only code on a new frozen base. It
> does not authorize any full hybrid runtime node.

**Goal:** Implement an unused deterministic projection from
`domain.SpecIndex`, plus its internal strict JSON codec and proof suite, without
adding product routes, UI, assets, browser runtime, storage, or rollout behavior.

**Design:**
[Hybrid Wasm Projection-Only Precursor Design](../specs/2026-07-27-hybrid-wasm-projection-precursor-design.md)

**Parent design:**
[Hybrid Wasm Public Docs Design](../specs/2026-07-27-hybrid-wasm-public-docs-design.md)

**Execution model:** Strict TDD. Each GREEN follows the literal RED in the same
task. Do not port a POC file before its new test fails on the frozen
implementation base. Do not cherry-pick any POC commit.

## Authoritative Post-Task-5 Format-Version-2 Amendment

**Gate:** STOP all production and test mutation until the parent accepts this
exact design-and-plan checkpoint. Tasks 0-4 below are preserved as historical
receipts for the unpublished version-1 attempt; their recursive DTO/golden
instructions are superseded and must not be rerun. The existing uncommitted Task
5 evidence files stay untouched until acceptance.

The pre-integration version-1 candidate is abandoned. The continuation replaces
it with `formatVersion: 2` and explicitly rejects version 1; no parallel
version-1 decoder is added unless a real consumer is discovered. If one is
discovered, stop for a reviewed compatibility plan.

### Amended DTO and Algorithm

- Add `Document.schemaNodes []SchemaNode` immediately after `schemaDetails`.
- Define `SchemaRef uint32` on `Parameter`, `MediaType`, `SchemaDetail`, and
  schema-node property/item edges.
- Remove recursive `WireSchema`, `SchemaProperty`, and `SchemaItem` from the
  version-2 contract.
- Define `SchemaNode` wire fields in this exact order: `ordinal`, `id`, `name`,
  `type`, `format`, `description`, `defaultValue`, `exampleText`, `json`,
  `properties`, `items`.
- Property edges retain `ordinal`, `id`, `name`, `required`, `description`, then
  add `schemaRef`; item edges retain `ordinal`, `id`, then add `schemaRef`.
- Preserve every existing schema display and embedded-JSON value; omit nothing.

A node ID is `schema-node-` plus lowercase SHA-256 of a domain-separated,
length-framed semantic preimage. Its exact prefix is
`manja.projection.schema-node.v2\x00`. Strings use uint64-big-endian byte length
plus UTF-8 bytes; Booleans use one byte; ordinals use uint32 big-endian; slices
use uint64-big-endian count plus elements. Child edges use the raw 32-byte child
digest. Global node ordinal/index, final numeric refs, and the derived node ID
are excluded from the preimage.

Build bottom-up: canonicalize embedded JSON, identify children before parents,
intern by digest/preimage, deduplicate only equal digest plus equal preimage,
fail unequal preimages sharing a digest with `hash_collision`, sort IDs
lexicographically, assign `ordinal == index`, then convert internal/root digests
to `uint32` references. SHA-256 is fixed in production; collision injection is
available only through an internal test helper.

The decoder validates iteratively and rejects the whole document for bad or
out-of-range references, duplicate/unsorted IDs or ordinal mismatch,
digest/body mismatch, unequal collision, orphan, cycle, depth greater than 64,
more than 100,000 unique nodes, more than 100,000 expanded occurrences across
all roots, more than 100,000 edges, duplicate properties, `items` cardinality
other than zero or one, `null` slices, invalid `uint32` spelling/range,
unknown/duplicate/trailing JSON, or noncanonical bytes. It returns a zero
document and bounded non-disclosing error. Cycle detection precedes iterative
depth/expanded-occurrence traversal.

The unchanged inclusive limits are 262,144 bytes per shallow
`OperationDetail`, 524,288 bytes per shallow `SchemaDetail`, and 16,777,216
bytes per document. Add 524,288 bytes per `SchemaNode`. A globally interned node
is counted once. Visual expansion remains protected by a later renderer fragment
limit. The digest remains over the full uncompressed canonical version-2
document. The `js/wasm` build remains standard-library plus `domain` only, an
invalid graph rejects the entire document, and SSR fallback remains later work.

### Step A: Add Literal Version-2 RED

Before changing production code, add and run these exact tests:

```text
TestBuilderInternsSchemasAcrossAllRoots
TestBuilderSchemaNodeIdentityUsesChildDigest
TestBuilderSchemaNodeOrderIgnoresTraversalOrder
TestBuilderSchemaHashCollisionFailsClosed
TestBuilderPreservesAllInternedSchemaContent
TestUnmarshalRejectsOutOfRangeSchemaRefs
TestUnmarshalRejectsSchemaNodeDigestMismatch
TestUnmarshalRejectsDuplicateAndOrphanSchemaNodes
TestUnmarshalRejectsSchemaCycles
TestUnmarshalEnforcesExpandedSchemaDepthAndNodeBudget
TestMarshalEnforcesInternedRecordNodeAndDocumentBounds
TestUnmarshalRejectsSupersededV1
TestGoldenV2EmptyProjection
TestGoldenV2OperationProjection
TestGoldenV2FullProjection
```

Run the smallest owning-package commands with `-count=1` and record literal
compile/assertion failures caused by the still-recursive version-1 model. The
GitHub scale RED retains the literal version-1 failure at operation index 570,
282,995 bytes versus the 262,144-byte limit. Do not accept a RED caused only by
test syntax, fixture lookup, or an unrelated failure.

### Step B: Minimal Version-2 GREEN

Implement only the DTO, bottom-up canonicalizer/interner, reference conversion,
strict iterative graph validation, and bound accounting required by the RED.
Delete/replace the version-1 goldens with independently reviewed
`v2-empty`, `v2-operation`, and `v2-full` bytes/digests. Keep version-1 hashes
only as historical ledger evidence. Candidate generation must not overwrite an
accepted oracle, and every accepted fixture must prove exact bytes, digest, and
no final newline.

Run these exact minimum GREEN commands:

```bash
GOWORK=off go test ./application/projection \
  -run 'Intern|SchemaNode|HashCollision|GitHubFixture' -count=2

GOWORK=off go test ./internal/adapters/projectionjson \
  -run 'SchemaRef|SchemaNode|Cycle|Depth|Bound|GoldenV2|SupersededV1' -count=1

GOWORK=off go test ./application/projection ./internal/adapters/projectionjson -count=1

GOWORK=off go test ./architecture -run Projection -count=1

(cd integration/testdata/external-module &&
  GOWORK=off go test ./... -run Projection -count=1)

node --test integration/testdata/projection-consumer/projection.test.mjs

GOOS=js GOARCH=wasm GOWORK=off \
  go build -trimpath ./application/projection
```

### Step C: Twice-Run Scale Acceptance

Run the GitHub fixture proof twice. Each run records:

- complete canonical document bytes;
- maximum shallow operation bytes;
- maximum shallow schema-detail bytes;
- maximum schema-node bytes;
- schema-root count;
- unique-node count;
- full document digest.

Both runs must report an identical digest and remain within every unchanged
limit. Then run the plan's full repository, nested-site, external Go, Node,
architecture, vet, build, generation-drift, tidy-diff, exact-owned-path, and
clean-status gates. Any remaining oversize record/document, nondeterminism,
graph-validation gap, excluded data, boundary violation, or `js/wasm` compile
failure blocks delivery; never raise a limit or omit a value.

### Measurement Ledger

The rejected version-1 measurement is exact: document 25,026,875 bytes;
`operationDetails[570]` 282,995 bytes; dominant response 261,580 bytes, schema
233,624 bytes, example 27,784 bytes; 3,453 roots; 2,236 unique nodes. The parent
conservative estimate for version 2 is 8,067,080 total bytes, maximum shallow
operation 35,906 bytes, and maximum node 30,657 bytes. These estimates are
design evidence only and are not final acceptance.

## Frozen Authority and Placeholders

Docs authoring state and known reviewed prerequisites:

```text
DOCS_AUTHORING_BASE_SHA=6b5e7a2cbb604d1da0f1cee9da29d6c1f5fbe3c5
HYBRID_DESIGN_SHA=8267668e848f938659ae715c782c0d8d50cec947
DOMAIN_SPECINDEX_SHA=48d2b9c65f2670c928c73ca673b898c49b4e8207
POC_EVIDENCE_SHA=ec4b4eda5667947370a52e4a1fa9d2aff89a7aa9
POC_MERGE_BASE_SHA=6216bcb2d9a50083b130daa37cf381c6e2b4b601
```

During docs authoring, the control plane reported disjoint canonical-SVG work
integrated at `origin/main=539f6504e2f72e3a7fa470fa600dbfa0e3179966`.
This plan intentionally remains authored on `DOCS_AUTHORING_BASE_SHA`; the
parent reconciles the docs commit. The observed SHA is historical context, not
an implementation-base default.

Fill these placeholders with literal 40-character SHAs before Task 1:

```text
IMPLEMENTATION_BASE_SHA=bdc210025f6c3c564f161d05c16da1a8b8dc006d
PRECURSOR_DESIGN_SHA=bdc210025f6c3c564f161d05c16da1a8b8dc006d
PRECURSOR_PLAN_SHA=bdc210025f6c3c564f161d05c16da1a8b8dc006d
```

Implementation receipt:

```text
WORKTREE=/Users/guilhermecastro/.codex/worktrees/e17a/manja
BRANCH=codex/hybrid-projection-precursor
HEAD=bdc210025f6c3c564f161d05c16da1a8b8dc006d
ORIGIN_MAIN=bdc210025f6c3c564f161d05c16da1a8b8dc006d
MERGE_BASE=bdc210025f6c3c564f161d05c16da1a8b8dc006d
DOMAIN_CONTRACT_DIFF=clean against 48d2b9c65f2670c928c73ca673b898c49b4e8207
HYBRID_DESIGN_ANCESTOR=8267668e848f938659ae715c782c0d8d50cec947
DOMAIN_SPECINDEX_ANCESTOR=48d2b9c65f2670c928c73ca673b898c49b4e8207
POC_EVIDENCE_HEAD=ec4b4eda5667947370a52e4a1fa9d2aff89a7aa9
```

The Codex harness had already created the dedicated linked worktree at the
exact fetched base, so Task 0 created the named branch in that existing
worktree rather than nesting a second worktree. The preserved POC was inspected
read-only and remains unmodified.

No task may start while a placeholder remains. `IMPLEMENTATION_BASE_SHA` must
be the exact fresh `origin/main` observed after Task 0's fetch, even when it is
newer than both the docs authoring base and the later observation above. It must
contain the exact known prerequisite commits and the reviewed precursor
artifacts. If `domain.SpecIndex`, its validation, or
`domain.ValidateCanonicalIdentity` changed after `DOMAIN_SPECINDEX_SHA`, stop
and reconcile the design; do not silently adapt the wire contract.

The complete hybrid feature still waits for the parent Integration DAG. This
precursor does not require release-track/authenticated-preview/Goshtoso/UI
prerequisites because it consumes only `domain.SpecIndex` and deterministic
serialization.

## Authorized Future Paths

Only these implementation paths are authorized by this plan:

```text
application/projection/
internal/adapters/projectionjson/
architecture/projection_boundary_test.go
architecture/projection_wasm_build_test.go
integration/testdata/external-module/projection_test.go
integration/testdata/external-module/go.sum
integration/testdata/projection-consumer/
```

The implementation may update this plan's verification evidence and the
precursor snag file. Any `.templ`, generated `_templ.go`, existing public-docs
handler/template, JavaScript, CSS, API YAML, Goshtoso dependency, module file,
Wasm command, PWA file, Service Worker, or product asset edit is out of scope and
requires a later plan.

## Task 0: Freeze the New Implementation Base

**Files:**

- Modify: `docs/superpowers/plans/2026-07-27-hybrid-wasm-projection-precursor.md`

1. Fetch and create a fresh task worktree from exact `origin/main`:

   ```bash
   git fetch origin
   git rev-parse origin/main
   git worktree add -b codex/hybrid-projection-precursor /tmp/manja-hybrid-projection origin/main
   cd /tmp/manja-hybrid-projection
   git status --short --branch
   git rev-parse HEAD
   git merge-base HEAD origin/main
   ```

   Expected: clean branch; all three SHA commands equal the literal value copied
   to `IMPLEMENTATION_BASE_SHA`. Do not reuse, reset, or rebase this docs-authoring
   branch, and do not assume the earlier observed `539f650...` is still current.

2. Verify prerequisites and fill placeholders in the execution copy of this
   plan:

   ```bash
   git merge-base --is-ancestor 8267668e848f938659ae715c782c0d8d50cec947 HEAD
   git merge-base --is-ancestor 48d2b9c65f2670c928c73ca673b898c49b4e8207 HEAD
   git log -1 --format=%H -- docs/superpowers/specs/2026-07-27-hybrid-wasm-projection-precursor-design.md
   git log -1 --format=%H -- docs/superpowers/plans/2026-07-27-hybrid-wasm-projection-precursor.md
   ! sed -n '1,70p' docs/superpowers/plans/2026-07-27-hybrid-wasm-projection-precursor.md | rg -n '<[^>]+SHA|IMPLEMENTATION_BASE_SHA=<|PRECURSOR_.*=<|TBD|TODO'
   ```

   Expected: both ancestor checks exit 0; both doc logs return 40 hex
   characters; final `rg` finds no unresolved placeholder or task ambiguity.

3. Compare current source contracts literally:

   ```bash
   git diff --exit-code 48d2b9c65f2670c928c73ca673b898c49b4e8207..HEAD -- domain/spec.go domain/spec_validation.go domain/identity.go architecture/public_boundary_test.go architecture/context_boundary_test.go
   ```

   Expected: empty. Any diff blocks Task 1 pending reviewed reconciliation.

4. Install locked tooling only after the gate passes:

   ```bash
   npm ci
   ```

5. Record the three literal SHAs and the gate output in this plan, then commit
   that execution-only evidence before any RED:

   ```bash
   git add docs/superpowers/plans/2026-07-27-hybrid-wasm-projection-precursor.md
   git commit -m "docs(projection): freeze precursor implementation base"
   ```

**Commit:** `docs(projection): freeze precursor implementation base`

**Reversal:** Revert the evidence commit if the implementation base is
abandoned. If the gate blocks before the commit, remove only the new,
still-clean task worktree/branch when the control plane authorizes cleanup.
Never touch the preserved POC.

## Historical Task 1 Receipt: RED Architecture and Consumer Contracts

**Files:**

- Create: `architecture/projection_boundary_test.go`
- Create: `integration/testdata/external-module/projection_test.go`
- Create: `integration/testdata/projection-consumer/projection.test.mjs`

### Step 1.1: Write the dependency RED

`TestProjectionBuilderDependencyDirection` and
`TestProjectionCodecDependencyDirection` must inspect direct imports and also
invoke `go list -deps -json` for forbidden transitive packages. Require:

- `application/projection` directly imports only `domain` and standard library;
- `internal/adapters/projectionjson` directly imports only
  `application/projection` and standard library; transitive `domain` is allowed;
- neither dependency graph contains `kin-openapi`, Goshtoso, `internal/web`,
  `internal/adapters/openapi`, source/store packages, generated API code,
  `syscall/js`, or HTTP routing.

`TestProjectionPublicAPIShape` parses Go AST and requires exactly the documented
`Builder.Build(context.Context, domain.SpecIndex)` product-input shape and the
documented exported DTO/type/field allowlist. It rejects runtime/network
concepts such as HTTP handlers, endpoint URLs, network allowlists, cache,
worker, publication, eligibility, release, storage, and raw-spec bytes while
explicitly allowing presentation-only `PublicRoutes`, `PublicRoute`, `href`,
and `path` fields required by the wire model. It must not use an ambiguous
substring rejection for `route`.

### Step 1.2: Write the unrelated-module RED

The external test imports:

```go
github.com/araihu/manja/application/projection
github.com/araihu/manja/domain
```

It builds the empty-slice fixture and asserts version, identity, overview
anchor/href, and `main-content` landmark. It must not import any `internal`
package or claim access to the codec.

The Node test reads `v1-empty.json` and `v1-operation.json`, asserts their exact
872/2,780 byte counts and no final newline, computes SHA-256 with `node:crypto`,
compares both literal digest files, parses the documented identities,
landmarks, operation anchor/href, IDs, ordinals, and route, and confirms
excluded sentinels are absent. It has no npm dependency and does not execute
browser APIs.

### Step 1.3: Run literal RED commands

```bash
GOWORK=off go test ./architecture -run Projection -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -run Projection -count=1)
node --test integration/testdata/projection-consumer/projection.test.mjs
```

Expected RED:

- architecture and external-module commands fail because
  `github.com/araihu/manja/application/projection` does not exist;
- Node fails with `ENOENT` for `v1-empty.json`.

Unexpected compile, module, or test failures are fixed in tests before moving
on; do not weaken the expected contract.

### Step 1.4: Commit RED contracts

```bash
gofmt -w architecture/projection_boundary_test.go integration/testdata/external-module/projection_test.go
git diff --check
git add architecture/projection_boundary_test.go integration/testdata/external-module/projection_test.go integration/testdata/projection-consumer/projection.test.mjs
git commit -m "test(projection): require deterministic public boundary"
```

**Reversal:** Revert this one test commit. No product path exists yet.

## Historical Task 2 Receipt: Unpublished Version-1 DTO

**Files:**

- Create: `application/projection/doc.go`
- Create: `application/projection/model.go`
- Create: `application/projection/build.go`
- Create: `application/projection/anchors.go`
- Create: `application/projection/embedded_json.go`
- Create: `application/projection/build_test.go`
- Create: `application/projection/anchors_test.go`
- Create: `application/projection/embedded_json_test.go`
- Create: `application/projection/exclusion_test.go`
- Create: `application/projection/determinism_test.go`
- Create: `application/projection/testdata/README.md`

### Step 2.1: Write DTO/anchor RED tests

Tests must construct `domain.SpecIndex` directly and assert:

- `FormatVersion == 1`, required identities, no caller-input mutation;
- every empty collection is non-nil;
- supplied operation anchor preservation;
- supplied-anchor slash acceptance and `%`, `#`, space, Unicode, backslash,
  reserved-fixed-ID, operation-versus-schema, section-ID, and generated
  `search-result-...` collision rejection;
- fallback operation ID/method-path ASCII slug;
- schema slug, overview metadata, relative hrefs, heading IDs/levels, and
  `main-content` landmark;
- hash-stable operation-tag section IDs;
- distinct hash-stable `search-result-` IDs and resolvable hrefs;
- schema `author_association` translates frozen-parser search href
  `#schema-author_association` and selected public route to canonical
  `schema-author-association`; ambiguous/non-exact aliases fail;
- duplicate/empty anchors and unresolved search targets fail;
- public routes accept exact `/` and `/?selected=x#x` forms and reject
  query/fragment conflicts, absolute/scheme-relative URLs, traversal/unclean
  paths, backslashes, extra query keys, and missing query/fragment halves;
- top-level ordinals follow canonical order; operation-tag sidebar items and
  schema sidebar items follow canonical operation/schema order;
  `OperationDirectory.sections` follows that operation's deduplicated tag order;
  other nested ordinals capture retained source position before nested sorting;
- exact duplicate scalar tags/scopes/keywords are collapsed at first
  occurrence; blank tags/keywords are dropped, blank scopes fail existing
  domain validation, and retained ordinals compact;
- canceled context before and during a multi-record build returns
  `context.Canceled` and the zero `Document`.

The during-build case uses a test-only counting `context.Context` whose `Err()`
returns nil for exactly N builder checkpoints and then `context.Canceled`; it
does not race a goroutine or wall clock. Pre-canceled and expired-deadline
contexts cover exact standard-error propagation.

Use exact subtest names:

```text
TestBuilderBuildsVersionOneDocument
TestBuilderDoesNotMutateSpecIndex
TestBuilderCanonicalNavigationMetadata
TestBuilderTranslatesLegacySchemaTargets
TestBuilderRejectsDuplicateRecordKeys
TestBuilderPreservesOrdinalsBeforeSorting
TestBuilderHonorsCancellation
TestBuilderExcludesRawSpecSentinels
TestBuilderTopLevelPermutationDeterminism
TestBuilderPreservesDisplayExampleText
TestBuilderSeparatesSchemaJSONAndExample
TestBuilderValidatesProjectionIdentityWithDomainContract
TestBuilderErrorsDoNotDiscloseSourceContent
TestBuilderErrorsAreBounded
```

`TestBuilderExcludesRawSpecSentinels` builds otherwise equal documents with the
two exact sentinels and invalid-UTF-8 excluded variants, then compares complete
DTO values. It separately proves `SpecDownload.Filename` is included.
`TestBuilderTopLevelPermutationDeterminism` uses seeds `0..999`, compares DTOs,
and deep-compares the input after each build. Canonical byte/digest assertions
for both behaviors belong to Task 3's external `projectionjson` tests, avoiding
an application-to-internal import cycle.

The two builder error tests feed identity/example sentinels and a depth-64
schema path that makes frozen domain validation verbose. Aside from exact
`context.Canceled`/`context.DeadlineExceeded`, require a coarse enumerated
path/class, no sentinel or original domain error text, valid UTF-8, at most 256
bytes, and a zero `Document`.

`TestBuilderValidatesProjectionIdentityWithDomainContract` runs the same table
for `ProjectID` and `RevisionID`. Rejected cases are empty, invalid UTF-8,
leading whitespace, trailing whitespace, NUL, and newline/control input.
Accepted cases include ASCII, internal whitespace, and non-ASCII non-control
text. For every case, success/failure must match a direct
`domain.ValidateCanonicalIdentity(fieldName, value, false)` call. Every rejected
case returns zero `Document`; errors stay bounded and omit the rejected value.

### Step 2.2: Run RED

```bash
GOWORK=off go test ./application/projection -run 'TestBuilder|TestAnchor' -count=1
```

Expected: package/identifier compile failure before implementation.

### Step 2.3: Minimal GREEN model and builder

Implement only the design's concrete DTOs and builder:

- copy the index and zero `SpecDownload.JSON`/`ExampleSpecJSON` only in that
  validation copy;
- call
  `domain.ValidateCanonicalIdentity("projection project ID", validationCopy.ProjectID, false)`
  and
  `domain.ValidateCanonicalIdentity("projection revision ID", validationCopy.RevisionID, false)`
  exactly; translate failures to bounded non-disclosing `invalid_identity`
  errors;
- call `domain.ValidateSpecIndex` on the sanitized validation copy and translate
  failure to a bounded non-disclosing `invalid_source` error;
- copy every source value;
- assign checked top-level `uint32` ordinals after canonical sorting and nested
  presentation ordinals before nested sorting;
- calculate exact anchors, hrefs, headings, landmarks, section IDs, and record
  IDs;
- register only the exact frozen-parser legacy schema aliases, require unique
  resolution, and rebuild search/public-route outputs with canonical anchors;
- sort only by the documented keys;
- initialize every slice;
- check `ctx.Err()` before work and between top-level/nested record loops.
- translate domain/builder validation failures to bounded, non-wrapping
  enumerated errors; preserve exact context cancellation/deadline errors.

Do not add JSON tags with `omitempty`, a parser, renderer, HTTP type, adapter,
storage type, or publication field.

### Step 2.4: RED/GREEN embedded JSON tests

Before implementing `embedded_json.go`, add tests for valid canonical object,
shuffled object keys, HTML/U+2028/U+2029 escaping, duplicate keys, trailing
values, depth 64/65, valid scalar, invalid plain text rejection, and input
immutability. Add exact embedded-number cases `1e+03` to `1000`, `1.0` to `1`,
`-0` to `0`, and `1e-3` to `0.001`, plus an exponent whose bounded expansion is
rejected before allocation. Separate builder tests prove parameter/media/schema
display examples `true`, `1e+03`, and `{"looks":"json"}` remain exact text and
that a provided empty media/schema example remains present. Set
`Schema.Example.JSON` and `.Example` to different sentinels and require
`exampleSchemaJSON` and primary example text to remain distinct.

```bash
GOWORK=off go test ./application/projection -run 'EmbeddedJSON|PreservesDisplayExampleText|SeparatesSchemaJSONAndExample' -count=1
```

Expected RED: missing canonicalizer. Minimal GREEN uses `UseNumber`, explicit
duplicate/depth checks, a sorted-key recursive emitter with custom decimal
number normalization, and `json.Marshal` string escaping. It stores only a
string in the wire DTO.

### Step 2.5: Focused GREEN and refactor

```bash
gofmt -w application/projection/*.go
GOWORK=off go test ./application/projection -count=1
GOWORK=off go test ./architecture -run 'Projection(Builder|Public)' -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -run Projection -count=1)
git diff --check
```

Expected: builder/public architecture and external-module tests pass. The codec
architecture test and Node consumer remain RED because the codec/goldens do not
exist; do not run or claim the full `-run Projection` gate yet.

### Step 2.6: Commit DTO slice

```bash
git add application/projection architecture/projection_boundary_test.go integration/testdata/external-module/projection_test.go
git commit -m "feat(projection): define deterministic projection model"
```

**Reversal:** Revert this commit, then the Task 1 RED commit if abandoning the
precursor. No runtime consumer exists.

## Historical Task 3 Receipt: Unpublished Version-1 Codec and Goldens

**Files:**

- Create: `internal/adapters/projectionjson/codec.go`
- Create: `internal/adapters/projectionjson/strict.go`
- Create: `internal/adapters/projectionjson/validate.go`
- Create: `internal/adapters/projectionjson/codec_test.go`
- Create: `internal/adapters/projectionjson/strict_test.go`
- Create: `internal/adapters/projectionjson/golden_test.go`
- Create: `internal/adapters/projectionjson/exclusion_test.go`
- Create: `internal/adapters/projectionjson/determinism_test.go`
- Create: `internal/adapters/projectionjson/bounds_test.go`
- Create: `application/projection/testdata/v1-empty.json`
- Create: `application/projection/testdata/v1-empty.sha256`
- Create: `application/projection/testdata/v1-operation.json`
- Create: `application/projection/testdata/v1-operation.sha256`
- Create: `application/projection/testdata/v1-full.json`
- Create: `application/projection/testdata/v1-full.sha256`
- Modify: `application/projection/testdata/README.md`

### Step 3.1: Write canonical codec RED

Exact tests:

```text
TestMarshalUsesCanonicalGoJSON
TestMarshalRejectsInvalidUTF8BeforeEncoding
TestUnmarshalRejectsDuplicateKeysAtEveryDepth
TestUnmarshalRejectsUnknownFieldsAtEveryLayer
TestUnmarshalRejectsTrailingValues
TestUnmarshalRejectsNonCanonicalBytes
TestUnmarshalEnforcesWireSchemaDepthAndNodeBounds
TestDigestHashesExactBytes
TestGoldenEmptyProjection
TestGoldenOperationProjection
TestGoldenFullProjection
TestFullFixtureManifest
TestProjectionExcludedFieldsDoNotAffectBytesOrDigest
TestProjectionTopLevelPermutationsKeepBytesAndDigest
TestMarshalEnforcesInclusiveBounds
TestErrorsDoNotDiscloseProjectionContent
TestErrorsAreBounded
```

Generate table cases for every DTO layer listed in the design. Cases must cover
invalid UTF-8, top-level array/scalar/null, unknown field, duplicate field,
negative ordinal, float ordinal, exponent ordinal, uint32 overflow, `null`
slice, reordered field, extra space, alternate `<` escape, and final newline.
Construct wire schemas iteratively and require depth 64/node 100,000 acceptance
and depth 65/node 100,001 rejection without panic. Inject one sentinel into an
unknown key, identity, example, and malformed byte sequence; every error must
omit it, use an enumerated path/class, and be at most 256 UTF-8 bytes.

Build the exact design sentinels and all 1,000 seeded top-level permutations
again through the codec; require complete bytes/digest equality. Binary-search
filler lengths and freeze exact record/document cases at 262,143/262,144/
262,145, 524,287/524,288/524,289, and 16,777,215/16,777,216/16,777,217 bytes.
The below/equal cases pass, above cases return the documented bounded class,
and a record-valid document may still fail the total limit. Decoder preflight
uses the actual input slice length; there is no fictitious declared length.

### Step 3.2: Run RED

```bash
GOWORK=off go test ./internal/adapters/projectionjson -count=1
```

Expected: package/identifier compile failure.

### Step 3.3: Minimal GREEN token pass and codec

Implement:

- preflight 16 MiB and UTF-8 checks;
- `json.Decoder.UseNumber` token stack with per-object key sets;
- one top-level object and EOF requirement;
- unsigned canonical integer token validation;
- `DisallowUnknownFields` concrete decode;
- recursive DTO validation, non-nil slices, sort/key/relationship checks;
- iterative wire-schema depth/node validation before recursive processing;
- inclusive operation/schema/total byte limits and bounded non-disclosing
  errors;
- `json.Marshal` and exact input/re-encoded byte comparison;
- lowercase SHA-256 of exact canonical bytes.

No error includes raw projection/example data, unknown key text, identity
values, or malformed input bytes.

### Step 3.4: Materialize exact vectors and review one candidate golden

Create `v1-empty` and `v1-operation` from the two literal design code blocks
with `apply_patch`, without their Markdown line terminators. Do not generate
these independent oracles from production code. The full-fixture helper may
write only `v1-full.candidate.json` and `v1-full.candidate.sha256` when the
explicit environment variable is set; it never overwrites an accepted golden:

```bash
MANJA_UPDATE_PROJECTION_GOLDEN=1 GOWORK=off go test ./internal/adapters/projectionjson -run TestGoldenFullProjection -count=1
wc -c application/projection/testdata/v1-empty.json application/projection/testdata/v1-operation.json application/projection/testdata/v1-full.candidate.json
tail -c 1 application/projection/testdata/v1-empty.json | od -An -t x1
tail -c 1 application/projection/testdata/v1-operation.json | od -An -t x1
tail -c 1 application/projection/testdata/v1-full.candidate.json | od -An -t x1
shasum -a 256 application/projection/testdata/v1-empty.json application/projection/testdata/v1-operation.json application/projection/testdata/v1-full.candidate.json
cat application/projection/testdata/v1-empty.sha256 application/projection/testdata/v1-operation.sha256 application/projection/testdata/v1-full.candidate.sha256
```

Expected:

- `v1-empty.json` is exactly 872 bytes;
- `v1-operation.json` is exactly 2,780 bytes;
- each `tail` shows the closing-brace byte `7d`, not `0a`;
- computed digests equal their lowercase digest files;
- `v1-empty.sha256` is
  `8267e1a8a597a6561409e81492b06c24b44b6cbd12875fc90985295c5765889d`;
- `v1-operation.sha256` is
  `6609c4e78e6556c8a178e500aeff8da85801ce30aaa784129c85b2c4e63cdc41`.

Before generation, write the independently declared full-fixture manifest to
`application/projection/testdata/README.md`; enumerate every source value,
output record count, ID, ordinal, canonical example, and exclusion, and make
`TestFullFixtureManifest` assert that literal table. Inspect the complete full
candidate against it. Apply the accepted candidate as
`v1-full.json`/`.sha256` in a reviewed patch, then delete candidate files. The
full fixture is coverage breadth; it cannot redefine either literal oracle.
Run without update authority:

```bash
gofmt -w internal/adapters/projectionjson/*.go
GOWORK=off go test ./internal/adapters/projectionjson -count=1
GOWORK=off go test ./architecture -run Projection -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -run Projection -count=1)
node --test integration/testdata/projection-consumer/projection.test.mjs
git diff --check
```

Expected: PASS; no command modifies files.

### Step 3.5: Commit codec and reviewed vectors

```bash
git add internal/adapters/projectionjson application/projection/testdata integration/testdata/projection-consumer/projection.test.mjs
git commit -m "feat(projection): add canonical version one wire codec"
```

**Reversal:** Revert this commit. Task 2 model remains unused and has no wire
consumer.

## Historical Task 4 Receipt: Property and Fuzz Assurance

**Files:**

- Create: `application/projection/fuzz_test.go`
- Create: `internal/adapters/projectionjson/fuzz_test.go`
- Modify only after a saved RED regression: the narrow Task 2/3 production file

Tasks 2 and 3 already put exclusions, 1,000 permutations, exact bounds, and
strict malformed cases RED before their behavior. Task 4 is verification-only
unless fuzzing discovers a new counterexample; it must not fabricate a late RED
for already implemented behavior.

### Step 4.1: Add fuzz targets and seeds

Add the three exact fuzz targets from the design. Seed byte goldens A-C, Vector
D's sentinel variants, and every strict malformed table case. Constrain derived
slice counts/string lengths before building so the harness tests behavior
rather than exhausting memory.

`application/projection/fuzz_test.go` uses `package projection` only for
`FuzzEmbeddedJSON`, which needs the unexported canonicalizer.
`internal/adapters/projectionjson/fuzz_test.go` owns
`FuzzBuildDeterminism` and `FuzzUnmarshalCanonicalProjection`, imports the
public builder, and can compare canonical bytes/digests without creating an
application-to-internal import cycle.

### Step 4.2: Compile and run seeds

```bash
GOWORK=off go test ./application/projection ./internal/adapters/projectionjson -run 'Fuzz|Determinism|Permutation|Bound|Excluded' -count=1
```

Expected: PASS. This is an assurance addition, not a new production GREEN.

### Step 4.3: Fuzz smoke

Seed all golden and malformed cases, then run bounded fuzz smoke tests:

```bash
GOWORK=off go test ./application/projection -run '^$' -fuzz FuzzEmbeddedJSON -fuzztime=10s
GOWORK=off go test ./internal/adapters/projectionjson -run '^$' -fuzz FuzzBuildDeterminism -fuzztime=10s
GOWORK=off go test ./internal/adapters/projectionjson -run '^$' -fuzz FuzzUnmarshalCanonicalProjection -fuzztime=10s
```

Expected: no panic; any accepted decode re-marshals to identical bytes. Store a
minimal regression seed in source if fuzzing finds a failure; do not check in a
machine-specific fuzz cache.

### Step 4.4: Conditional regression RED

Only when a counterexample exists, first copy its minimal bytes into a named
table case and run the smallest owning package/test command with `-count=1`.
Record the literal failing command and mismatch/panic class in the commit
message body. Expected: RED for that saved case. Then make the narrowest
production change, rerun the saved case, the owning package, and all three fuzz
smokes. If no counterexample exists, no production file changes in Task 4.

### Step 4.5: Commit assurance slice

```bash
gofmt -w application/projection/*.go internal/adapters/projectionjson/*.go
GOWORK=off go test ./application/projection ./internal/adapters/projectionjson -count=1
git diff --check
git add application/projection internal/adapters/projectionjson
git commit -m "test(projection): add property and fuzz assurance"
```

**Reversal:** Revert the assurance commit. If it contains a conditional
regression fix, revert that fix only together with the saved regression test.

## Blocked Task 5 Receipt: Version-1 Wasm and Scale Attempt

**Files:**

- Create: `architecture/projection_wasm_build_test.go`
- Create: `application/projection/github_fixture_test.go`
- Create: `internal/adapters/projectionjson/benchmark_test.go`
- Modify: `architecture/projection_boundary_test.go` only if broader subpackage
  enumeration is required

Task 5 is compatibility verification. The Wasm test executes behavior already
implemented by Tasks 2/3 and should pass without a new GREEN. The large-fixture
size outcome is intentionally unknown until measured because POC evidence is
larger than the projection bound; it is a blocking acceptance gate, not a late
RED. Any implementation defect follows Task 4's saved-regression rule.

### Step 5.1: WebAssembly compile proof

The architecture test executes:

```text
GOOS=js GOARCH=wasm GOWORK=off go build -trimpath ./application/projection
```

It sets subprocess cwd with the architecture suite's existing
`repositoryRoot(t)` helper, requires exit 0, and prints bounded compiler stderr
on failure. Compiling the production package avoids pulling the scale test's
parser import into the Wasm proof. It does not add `cmd/manja-*`, `syscall/js`,
`wasm_exec.js`, or assets.

```bash
GOWORK=off go test ./architecture -run ProjectionWasm -count=1
```

Expected: PASS. If it fails, save the exact compiler diagnostic as a regression
and remove only the accidental platform-specific dependency.

### Step 5.2: GitHub scale test

The file declares `package projection_test` so test-only imports do not create a
cycle. It may import `internal/adapters/openapi` and
`internal/adapters/projectionjson` to parse and measure the fixed fixture, then
passes only the returned `domain.SpecIndex` to the builder. Construct the parser
revision as `domain.Revision{ID:"github-v3-rest-fixture"}` and, after parse,
set `idx.ProjectID = "github"`; current parser code does not fill `ProjectID`.
This is explicit fixture composition, never production identity inference. It
requires:

- fixture digest
  `dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977`;
- projection size at most 16 MiB;
- repeated build bytes/digest identical;
- set `idx.SpecDownload.JSON` and `idx.ExampleSpecJSON` to the exact Vector D
  sentinels before both builds and prove neither occurs in output;
- build and codec benchmark metrics logged, not promoted to parent performance
  acceptance.

Name the benchmark functions exactly `BenchmarkBuildGitHubFixture` in
`github_fixture_test.go`, plus `BenchmarkMarshalFullProjection` and
`BenchmarkUnmarshalFullProjection` in the codec benchmark file. Fixture parsing
and construction occurs before `b.ResetTimer`; the benchmark loop measures only
the named operation.

```bash
GOWORK=off go test ./application/projection -run GitHubFixture -count=2 -v
```

Expected: repeated DTO builds are equal and either (a) every detail meets its
256/512 KiB bound, the canonical projection is at most 16 MiB, repeated
bytes/digest are stable, and the test passes, or (b) both codec attempts return
the same `record_too_large` or `projection_too_large` class and implementation
stops for a reviewed DTO-reduction design amendment. Report only the bounded
enumerated owning record/path, never its content. Do not raise a limit, omit
required fields ad hoc, or call the full precursor complete after outcome (b).
The only added wiring is test-only; never add a production parser import.

Only after outcome (a), capture the deferrable benchmark receipt:

```bash
GOWORK=off go test ./application/projection ./internal/adapters/projectionjson -run '^$' -bench 'Build|Marshal|Unmarshal' -benchtime=5x
```

Expected: all three exact benchmark names run; the values are logged as
diagnostic evidence, not parent performance acceptance.

### Step 5.3: Commit compatibility proof

```bash
gofmt -w architecture/projection_wasm_build_test.go architecture/projection_boundary_test.go application/projection/github_fixture_test.go internal/adapters/projectionjson/benchmark_test.go
git diff --check
git add architecture/projection_wasm_build_test.go architecture/projection_boundary_test.go application/projection/github_fixture_test.go internal/adapters/projectionjson/benchmark_test.go
git commit -m "test(projection): prove wasm and scale compatibility"
```

Reach and commit Step 5.3 only after size outcome (a). Outcome (b) stops before
this proof commit and reports the design blocker.

**Reversal:** Revert this test commit. The package-only `go build` creates no
repository runtime artifact.

## Task 6: Fresh Verification and Scope Audit

**Files:** No intended source changes. Update snag/evidence docs only when fresh
inspection finds a real ambiguity or workaround.

### Focused gates

```bash
GOWORK=off go test ./application/projection ./internal/adapters/projectionjson -count=1
GOWORK=off go test ./architecture -run Projection -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -run Projection -count=1)
node --test integration/testdata/projection-consumer/projection.test.mjs
GOOS=js GOARCH=wasm GOWORK=off go build -trimpath ./application/projection
```

### Repository gates

```bash
GOWORK=off go test ./architecture -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -count=1)
go test ./...
(cd site && GOWORK=off go test ./...)
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
git diff --check
git diff --exit-code
```

`git diff --exit-code` mechanically proves generation produced no tracked drift
because Task 5 ended clean and this plan changes no template. Redocly's existing
warnings may remain, but new errors fail.

### Task 6 dependency receipt

Fresh verification exposed one mechanical checksum consequence of the approved
scale proof: external-module tidy loads tests for `application/projection`, and
`github_fixture_test.go` imports `internal/adapters/openapi`. Parent review
narrowly extended ownership to
`integration/testdata/external-module/go.sum` for the exact 14-line
`GOWORK=off go mod tidy` result. The receipt adds module and `go.mod` checksums
only for kin-openapi v0.140.0, jsonpointer v0.22.5, swag/jsonname v0.25.5,
oasdiff/yaml v0.1.0, oasdiff/yaml3 v0.0.13, jsonschema/v6 v6.0.2, and x/text
v0.34.0. Any `go.mod`, version, replacement, or additional checksum change
remains a delivery blocker.

### Focused semantic-canonicality correction receipt

Independent review of exact head
`3311b54d7802eefcc17c678c820add134d55ef8f` demonstrated that the shared
`Marshal`/`Unmarshal` validator accepted Builder-inconsistent nested records.
The correction adds a permanent 44-case table covering every non-graph
derived-record family. Each case requires `Marshal` rejection, `Unmarshal`
rejection, and a zero decoded document. The internal validator now enforces all
Builder-derived IDs, ordinals, ordering, navigation, presence flags, and
directory/detail/sidebar/search/route relationships. Existing schema-graph
tests continue to own node/ref/digest/cycle/orphan semantics.

Acceptance requires unchanged v2 golden bytes/digests, one unchanged GitHub
fixture measurement/digest, full projection/codec tests, focused codec race,
architecture Projection, external Go and dependency-free Node consumers,
js/wasm build, representative root/site tests, and root/site/external
vet/build/tidy-diff. No runtime, UI, route, asset, API, module, or dependency
change is authorized by this correction.

### Exact owned-path gate

```bash
git diff --name-only IMPLEMENTATION_BASE_SHA...HEAD | sort
git status --short
```

Replace `IMPLEMENTATION_BASE_SHA` with the literal recorded SHA. Every changed
path must be in Authorized Future Paths or this plan/spec/snag. Reject any
`.templ`, `_templ.go`, `.js` outside the independent consumer test, CSS, API
YAML, module, PWA, asset, public handler/template, or generated-code diff.

### Forbidden dependency/scope gates

```bash
! rg -n 'kin-openapi|goshtoso|internal/web|internal/adapters/openapi|syscall/js|ServiceWorker|CacheStorage|IndexedDB|MANJA_LOCAL_DOCS' application/projection internal/adapters/projectionjson --glob '*.go' --glob '!*_test.go'
! rg -n 'SpecFile|Publication|ReleaseTrack|SecretRef' application/projection internal/adapters/projectionjson --glob '*.go' --glob '!*_test.go'
! rg -n 'templ\.Raw|html/template|net/http' application/projection internal/adapters/projectionjson --glob '*.go' --glob '!*_test.go'
test "$(rg -n 'validationCopy\.SpecDownload\.JSON = nil' application/projection/build.go | wc -l | tr -d ' ')" = 1
test "$(rg -n 'validationCopy\.ExampleSpecJSON = ""' application/projection/build.go | wc -l | tr -d ' ')" = 1
test "$(rg -l 'internal/adapters/openapi' application/projection --glob '*.go')" = 'application/projection/github_fixture_test.go'
```

Expected: the first three production-only searches have no matches; the fourth
and fifth commands prove the only two production references are the audited
validation-copy zeroing assignments; the final command proves the only parser
import is the exact scale-test file.

### Golden drift gate

```bash
before="$(git status --porcelain=v1)"
GOWORK=off go test ./application/projection ./internal/adapters/projectionjson -count=1
after="$(git status --porcelain=v1)"
test "$before" = "$after"
shasum -a 256 application/projection/testdata/v2-empty.json application/projection/testdata/v2-operation.json application/projection/testdata/v2-full.json
```

Expected: tests do not rewrite goldens; hashes match checked-in files.

## Task 7: Final Commit and Handoff

1. Review commit series and diff:

   ```bash
   git log --oneline IMPLEMENTATION_BASE_SHA..HEAD
   git diff --stat IMPLEMENTATION_BASE_SHA...HEAD
   git diff --check IMPLEMENTATION_BASE_SHA...HEAD
   git status --short
   ```

2. If Task 6 required documentation-only evidence changes, commit them
   separately:

   ```bash
   git add docs/superpowers/specs/2026-07-27-hybrid-wasm-projection-precursor-design.md docs/superpowers/plans/2026-07-27-hybrid-wasm-projection-precursor.md docs/superpowers/snags/2026-07-27-hybrid-wasm-projection-precursor.md
   git commit -m "docs(projection): record precursor verification evidence"
   ```

3. Require final clean status. Do not push, open a PR, merge, deploy, tag,
   release, start a server, touch the POC, or begin a later DAG node.

4. Report literal base/head SHAs, commits, owned paths, focused/full gates,
   golden digests/sizes, scale measurement, POC zero-cherry-pick disposition,
   and all deferred scope. Recommended parent action must be one review action,
   not integration of the complete hybrid feature.

## Commit and Reversal Matrix

| Slice | Commit | Reversal |
| --- | --- | --- |
| Frozen base evidence | `docs(projection): freeze precursor implementation base` | Revert docs-only evidence if base is abandoned |
| RED boundaries | `test(projection): require deterministic public boundary` | Revert alone; no product code |
| Pure DTO/builder | `feat(projection): define deterministic projection model` | Revert DTO then RED boundary if abandoning |
| Canonical codec/goldens | `feat(projection): add canonical version one wire codec` | Revert codec/goldens; DTO stays unused |
| Property/fuzz assurance | `test(projection): add property and fuzz assurance` | Revert tests; pair any regression fix with its saved case |
| Version-2 amendment checkpoint | `docs(projection): amend precursor to schema graph v2` | Revert docs only; code remains stopped |
| Wasm/scale proof | `test(projection): prove wasm and scale compatibility` | Revert test-only proof; `go build` leaves no runtime artifact |
| Evidence update | `docs(projection): record precursor verification evidence` | Revert docs only |

No slice changes database schema, browser cache, publication data, endpoint
behavior, HTML, assets, or deployment configuration. Rollback needs no data
migration.

## Deferred Handoffs

After precursor review/merge, later Integration DAG nodes may consume it only
after their own frozen-base gates:

1. server projection endpoint and content headers;
2. lazy SSR/HTMX sidebar/search UI using projection canonical metadata;
3. product-packaged reduced Wasm renderer and fragment parity;
4. Service Worker routing and server fallback;
5. offline storage, freshness, corruption recovery, withdrawal, and LRU;
6. default-on public eligibility and kill switch;
7. complete functional/security/browser/performance receipt.

Release tracks, authenticated previews, publication resolver authority, and
Goshtoso runtime contracts remain prerequisites of those later nodes. This plan
must never be cited as their completion evidence.

## Definition of Done

- All placeholders replaced and frozen-base gate recorded.
- Commit series matches the matrix and only authorized paths changed.
- New RED evidence was observed before each GREEN.
- Golden bytes/digests, strict decode corpus, exclusions, bounds, determinism,
  fuzz smoke, dependency direction, unrelated consumers, `js/wasm` compile, and
  scale fixture pass.
- Full repository, nested-site, API, generation, diff, and clean-status gates
  pass without generated drift.
- POC remains unchanged at its preserved SHA; zero exact cherry-picks.
- Handoff states projection-only precursor is review-ready and explicitly does
  not claim the hybrid feature exists.
