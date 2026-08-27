# Hybrid Wasm Projection-Only Precursor Design

**Status:** Format-version-2 amendment checkpoint; implementation stopped for
parent acceptance

**Date:** 2026-07-27

**Implementation base:** `bdc210025f6c3c564f161d05c16da1a8b8dc006d`

**Parent design:**
[Hybrid Wasm Public Docs Design](./2026-07-27-hybrid-wasm-public-docs-design.md)

## Decision

Implement the parent design's projection-only exception as an unused,
provider-neutral `application/projection` builder plus an internal strict JSON
codec at `internal/adapters/projectionjson`. The only product input is
`domain.SpecIndex`. This precursor ends at deterministic version-2 bytes and
their SHA-256 digest. It exposes no HTTP route, UI, asset, Wasm command, browser
runtime, storage, cache, publication policy, release track, or rollout behavior.

The unpublished recursive version-1 candidate failed its required scale gate
before integration. It is abandoned. Canonical documents use
`formatVersion: 2`, and the strict decoder explicitly rejects version 1. There
is no parallel version-1 decoder unless a real consumer of the unpublished
bytes is discovered. Such discovery is a stop condition requiring a reviewed
compatibility amendment.

```text
domain.SpecIndex
        |
        v
application/projection             deterministic builder + public DTO
        |
        v
internal/adapters/projectionjson    strict canonical bytes + SHA-256
        |
        +-- later internal endpoint/Wasm/runtime consumers
```

## Package and Dependency Boundary

The public API remains deliberately small:

```go
type Builder struct{}

func (Builder) Build(context.Context, domain.SpecIndex) (Document, error)
```

`application/projection` imports only the standard library and `domain`.
`internal/adapters/projectionjson` imports only the standard library and
`application/projection`; `domain` may appear only transitively. Neither package
may import the OpenAPI parser, `kin-openapi`, `internal/web`, Goshtoso, source or
storage adapters, `net/http`, `syscall/js`, generated API code, or a Wasm
command. An unrelated Go module must be able to build and inspect the public DTO
with `GOWORK=off`; the internal codec is not a public storage/runtime API.

The builder copies its input, clears `SpecDownload.JSON` and `ExampleSpecJSON`
only in the validation copy, validates `ProjectID` and `RevisionID` with
`domain.ValidateCanonicalIdentity(..., false)`, and runs
`domain.ValidateSpecIndex` on that sanitized copy. Excluded bytes cannot affect
success, failure, DTO, canonical bytes, or digest. `SpecDownload.Filename`
remains overview display metadata. No parser, source, publication, actor,
credential, or request value enters the builder.

## Version-2 Wire Contract

All objects are concrete structs. Collections are slices, initialized as `[]`
and never `null`. The wire contains no map, `any`, `interface{}`,
`json.RawMessage`, float, pointer-shaped optional schema, or `omitempty` field.
Every record keeps the existing deterministic key, explicit `uint32` ordinal,
canonical sorting, anchor, href, heading, landmark, example, and embedded-JSON
semantics unless this amendment explicitly changes its schema representation.

`Document` adds `schemaNodes []SchemaNode` immediately after `schemaDetails`.
`Parameter`, `MediaType`, and `SchemaDetail` replace recursive `schema` with
`schemaRef SchemaRef`. Version 2 removes `WireSchema`, `SchemaProperty`, and
`SchemaItem`.

```go
type SchemaRef uint32

type SchemaNode struct {
    Ordinal      uint32               `json:"ordinal"`
    ID           string               `json:"id"`
    Name         string               `json:"name"`
    Type         string               `json:"type"`
    Format       string               `json:"format"`
    Description  string               `json:"description"`
    DefaultValue string               `json:"defaultValue"`
    ExampleText  string               `json:"exampleText"`
    JSON         string               `json:"json"`
    Properties   []SchemaNodeProperty `json:"properties"`
    Items        []SchemaNodeItem     `json:"items"`
}

type SchemaNodeProperty struct {
    Ordinal     uint32    `json:"ordinal"`
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Required    bool      `json:"required"`
    Description string    `json:"description"`
    SchemaRef   SchemaRef `json:"schemaRef"`
}

type SchemaNodeItem struct {
    Ordinal   uint32    `json:"ordinal"`
    ID        string    `json:"id"`
    SchemaRef SchemaRef `json:"schemaRef"`
}
```

Property and item edges retain their current semantic fields and replace only
the recursive child with `schemaRef`. `items` has cardinality zero or one; the
one item retains ordinal `0` and ID `items`. Every schema name, type, format,
description, default value, example text, canonical embedded-JSON string,
property description, and explicit example remains represented. Global
interning authorizes no omission, truncation, inference, or semantic merge.

Only `SchemaSummary.JSON` and `Schema.Example.JSON` are embedded JSON.
`SchemaSummary.JSON` becomes `SchemaNode.json`; `Schema.Example.JSON` remains
`SchemaDetail.exampleSchemaJSON`. They use the existing duplicate-key,
single-value, depth, number, UTF-8, and canonical-emission rules. Defaults and
examples remain exact display text, including `true`, `1e+03`, and
`{"looks":"json"}`. A provided empty example remains present.

## Schema-Node Identity and Interning

A node ID is:

```text
schema-node- + lowercaseHex(SHA256(semanticPreimage))
```

The semantic preimage starts with the exact bytes
`manja.projection.schema-node.v2\x00`. It then contains the semantic fields in
wire order: `name`, `type`, `format`, `description`, `defaultValue`,
`exampleText`, `json`, `properties`, and `items`. The node ID, global node
ordinal, final slice index, and final numeric references are excluded.

Framing is exact:

- a string is `uint64be(len(UTF-8 bytes))` followed by those bytes;
- a Boolean is one byte, `0` or `1`;
- an ordinal is `uint32be`;
- a slice is `uint64be(count)` followed by its elements;
- a property contributes ordinal, ID, name, required, description, then its
  child's raw 32-byte digest;
- an item contributes ordinal, ID, then its child's raw 32-byte digest.

The builder canonicalizes embedded JSON and walks bottom-up. Children are fully
identified before their parent. It interns by digest: equal digest and equal
preimage deduplicates; equal digest and unequal preimage fails closed with
`hash_collision`. It then sorts node IDs lexicographically, assigns dense
ordinals equal to slice indexes, and only afterward converts all root and child
digests to `SchemaRef uint32` values. Production uses fixed SHA-256. A collision
hasher may be injected only through an internal test helper; it is absent from
the exported API and production path.

## Determinism and Canonical JSON

Existing top-level canonical ordering, nested source ordinals, stable record
keys, navigation metadata, legacy schema-target translation, and caller-input
immutability remain unchanged. Equivalent top-level permutations and different
schema traversal orders produce identical node order, bytes, and digest.
Changing a retained semantic value or nested presentation order changes the
semantic preimage and document digest.

`projectionjson.Marshal` validates the DTO and calls `encoding/json.Marshal`
once for the top-level document. Output is compact standard Go JSON, uses
standard HTML/U+2028/U+2029 escaping, contains no literal CR/LF, initializes all
slices, spells integers as unsigned base-10 without leading zero, and has no
final newline.

`projectionjson.Digest` hashes the full, uncompressed canonical version-2
document and returns 64 lowercase hexadecimal characters. It never hashes a
struct, raw OpenAPI, compressed bytes, ETag spelling, or decoded browser value.

## Strict Iterative Decode

The codec rejects input larger than 16,777,216 bytes before allocation-heavy
decode, rejects invalid UTF-8, performs a token pass with `UseNumber` to reject
duplicate keys and invalid number spellings, decodes with
`DisallowUnknownFields`, validates the complete DTO/graph, then re-marshals and
requires byte equality. The graph validation is iterative and completes before
any recursive consumer can dereference it.

The whole document is rejected for:

- a missing, bad, or out-of-range root or child reference;
- duplicate node IDs, unsorted IDs, or an ordinal unequal to its slice index;
- a node ID whose digest does not match its semantic body;
- an equal digest associated with unequal semantic preimages;
- an orphan node unreachable from all parameter, media-type, and schema-detail
  roots;
- a self-cycle or multi-node cycle;
- dependency depth greater than 64;
- more than 100,000 unique nodes;
- more than 100,000 expanded occurrences across all roots;
- more than 100,000 property/item edges;
- duplicate properties;
- `items` cardinality other than zero or one;
- any `null` slice;
- a `uint32` token that is negative, decimal, exponential, zero-prefixed, or
  above `uint32` range;
- unknown or duplicate fields, trailing values, version 1 or any unsupported
  version, noncanonical whitespace/escapes/field order, or a final newline.

Expanded occurrences count each visit from each root. Counting only unique
nodes would permit a small DAG with exponential expansion. Cycle detection
precedes occurrence/depth traversal. Every error returns the zero document, is
valid UTF-8 and at most 256 bytes, names only an enumerated bounded path/class,
and never discloses identity, schema, example, unknown-key, or malformed-byte
content. No graph is partially repaired or accepted.

## Inclusive Bounds and Scale Gate

| Subject | Maximum canonical encoded bytes |
| --- | ---: |
| shallow `OperationDetail` | 262,144 |
| shallow `SchemaDetail` | 524,288 |
| one `SchemaNode` | 524,288 |
| complete `Document` | 16,777,216 |

Shallow operation and schema-detail accounting includes their fields and root
references, not the transitive schema bodies. A globally interned node is
counted once as a node record and once in the complete document. A later
renderer fragment limit owns visual-expansion protection. Raising a bound or
omitting a value is not an accepted response to failure.

The GitHub fixture scale proof runs twice. Each run records total bytes, maximum
shallow operation bytes, maximum shallow schema-detail bytes, maximum node
bytes, schema-root count, unique-node count, and digest. Both runs must have an
identical digest and all limits must pass.

The rejected version-1 evidence is literal:

- document: 25,026,875 bytes;
- `operationDetails[570]`: 282,995 bytes;
- dominant response: 261,580 bytes;
- dominant schema: 233,624 bytes;
- dominant example: 27,784 bytes;
- schema roots: 3,453;
- unique flat schema nodes: 2,236.

The parent's conservative version-2 estimate is 8,067,080 total bytes, maximum
shallow operation 35,906 bytes, and maximum node 30,657 bytes. This is design
evidence only. It is not final acceptance and cannot replace the twice-run
canonical version-2 proof.

## Goldens and Required RED

The version-1 golden bytes are deleted/replaced. Their hashes remain historical
ledger evidence only:

- `v1-empty`: `8267e1a8a597a6561409e81492b06c24b44b6cbd12875fc90985295c5765889d`;
- `v1-operation`: `6609c4e78e6556c8a178e500aeff8da85801ce30aaa784129c85b2c4e63cdc41`;
- `v1-full`: `5007d945b1c0440ce02cf5662f26777cde3e2b95b0521ffeba2c676d05cea440`.

The accepted oracles are independently reviewed, no-final-newline `v2-empty`,
`v2-operation`, and `v2-full` bytes/digests. Production output may create only
candidate files for review; it cannot approve or overwrite a golden.

Before version-2 production changes, record literal failure from these exact
tests:

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

The minimum GREEN commands are exactly:

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

The scale test then passes twice while logging the required metrics and
identical digest. Full repository, nested-site, external-module, Node,
architecture, vet, build, generation-drift, tidy-diff, exact-owned-path, and
clean-status gates remain mandatory as specified by the implementation plan.

## POC Disposition and Deferred Work

The preserved POC remains read-only at
`ec4b4eda5667947370a52e4a1fa9d2aff89a7aa9`. No commit is cherry-picked. Only
concepts are selectively reconsidered after a new failing test: exact-byte
digest/mismatch intent, anchor fixtures, malformed/corruption cases, and
measurement evidence. Raw `SpecIndex` serialization, parser-in-Wasm, router,
templates/generated templ, assets, shell, JavaScript runtime, Service Worker,
CacheStorage, IndexedDB, reload/update flows, browser harness, and PWA build
wiring remain rejected or deferred.

This precursor still ships unused. HTTP transport, content headers, server and
Wasm rendering, Goshtoso UI, search/sidebar fragments, 2 MiB rendered-fragment
enforcement, packaged Wasm/runtime assets, SSR fallback, Service Worker,
offline storage, freshness, corruption recovery, release tracks,
authenticated previews, visibility, rollout, deployment, and performance
acceptance remain later Integration DAG work.

## Acceptance Criteria

- Parent accepts this exact version-2 amendment before code/test mutation.
- Builder input remains only validated `domain.SpecIndex`; excluded raw bytes
  cannot affect result or digest.
- All schema display and embedded-JSON values survive global interning.
- Node identity, ordering, deduplication, collision failure, references, and
  iterative graph rejection match this contract literally.
- Independently reviewed version-2 bytes/digests, strict malformed/bound corpus,
  unrelated Go and Node consumers, and `js/wasm` compilation pass.
- The twice-run GitHub proof meets unchanged record/document limits and reports
  identical digest plus all required metrics.
- No endpoint, UI, asset, runtime, storage, release, or rollout behavior is
  implemented or claimed.
