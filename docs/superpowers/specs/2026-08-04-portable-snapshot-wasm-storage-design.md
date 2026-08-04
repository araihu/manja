# Portable Snapshot, Wasm, Offline, and Storage Design

**Status:** Draft for independent adversarial review

**Date:** 2026-08-04

**Release stage:** pre-v0.1.0

**Current milestone:** Define the next open-core slice that makes one immutable
Manja snapshot portable across native serving, static export, and a browser-only
local preview. Browser compilation, rendering, and search must work offline for
bounded inputs. Native publication must retain the existing Kubernetes-scale,
cross-catalog path. Storage contracts must admit a future S3-compatible artifact
adapter plus transactional cloud activation without importing Manja Cloud
management into the open core.

## Decision

Manja will have three first-class execution modes over one canonical snapshot
format:

1. **Local preview:** the browser accepts an uploaded OpenAPI source or a
   CORS-readable URL, compiles it in a dedicated Web Worker, stores a validated
   immutable snapshot locally, searches through a separate Wasm query engine,
   renders selected routes through a separate Wasm renderer, and works offline.
2. **Published renderer:** the native Go compiler loads one or many file/Git
   catalogs, produces immutable snapshots, activates them atomically, and serves
   bounded SSR plus client-first search. This remains the path for Kubernetes-
   scale catalogs and automated publishers.
3. **Static export:** the native CLI compiles the same snapshot and materializes
   canonical HTML, fragments, assets, source downloads, and search artifacts into
   a directory suitable for a CDN or object-storage upload.

The compiler, renderer, and search engine each have one provider-neutral Go
implementation. Native binaries and Wasm commands are build targets around those
packages, not parallel product implementations.

```text
                         domain.CatalogCandidate
                                   |
                         application/catalog
                           portable compiler
                                   |
                         immutable snapshot v1
                       /           |            \
              native server    static export   browser artifact store
                  SSR                 HTML         CacheStorage + IDB
                   |                   |                 |
             native search       CDN/static host    renderer.wasm
                   |                                     |
             HTTP fallback                            search.wasm
```

## Why This Replaces Neither SSR nor Native Compilation

A public demo that requires users to precompile a source and upload generated
artifacts is unacceptable. Local preview must begin with “drop a file” or “paste
a URL.” Browser compilation avoids donating arbitrary CPU and memory from a
public anonymous server while keeping private input on the device.

Browser compilation is not the default publisher architecture. Large multi-
document catalogs need bounded Git acquisition, compatibility profiles, durable
last-known-good snapshots, atomic activation, predictable startup, and server
fallback. Native compilation retains those capabilities.

SSR remains canonical online behavior. It provides initial HTML, SEO, copied
URLs, no-JavaScript access, and a fallback when Wasm, Service Worker, local
storage, or browser compilation is unavailable.

## Scope

### In scope for the next implementation program

- freeze and document the canonical snapshot and manifest format used by the
  accepted Kubernetes renderer;
- replace the compiler's retained `[]ChildArtifact.Bytes` publication boundary
  with a bounded streaming snapshot writer;
- introduce provider-neutral immutable-artifact and activation-repository ports;
- refactor the current filesystem store and journal coordinator to implement the
  new ports without weakening crash recovery, structural current/fallback,
  per-component history, CAS, or garbage-collection protections;
- add native `manja build` and `manja serve` commands for precompiled snapshots;
- add `manja export` for complete static output;
- compile the portable compiler to `compiler.wasm` and run it only inside a
  dedicated Web Worker;
- compile the query engine to a separate `search.wasm` Web Worker module;
- compile the shared selected-route presentation engine to `renderer.wasm`;
- add browser artifact and activation adapters using CacheStorage and IndexedDB;
- add an allowlisted Service Worker and offline application shell;
- support uploaded OpenAPI JSON/YAML and non-credentialed CORS-readable URLs;
- preserve server search fallback and exact browser/server ranking parity;
- publish format identities, resource bounds, progress, failure, cancellation,
  and recovery behavior;
- provide conformance suites reusable by filesystem, browser, static-export,
  and future cloud adapters.

### Out of scope

- Manja Cloud authentication, organizations, billing, tenancy UI, release
  management, audit UI, or hosted source credentials;
- an S3, R2, GCS, PostgreSQL, DynamoDB, or Manja Cloud adapter in this slice;
- anonymous server-side compilation for the public local-preview demo;
- server-side proxying of arbitrary URL imports;
- a live upstream “try it” request proxy;
- private-publication offline caching;
- browser Git clone or private repository credentials;
- collaborative editing or mutable snapshot contents;
- importing or reviving the historical `codex/wasm-pwa-poc` branch wholesale.

## Existing Baseline and Publication Prerequisite

The accepted renderer candidate at
`c1e4709592222e4d8eb09250dbf2e10f1130670b` establishes the baseline snapshot,
bounded search, durable filesystem activation, Kubernetes fixture, SSR, and
browser-search behavior. Its independent review identity is
`17cfc215da7f4d6c4244f07c892f5bf206f93c2ca3df32e580454031b0eb34df`.

Before this program begins, that candidate is published as the public live demo
with Kubernetes already compiled in the image build. Fly runtime machines recover
and serve the baked snapshot; they do not parse or compile Kubernetes sources.
That narrow deployment change may introduce the native build/recovery commands
earlier, but it does not silently establish the final storage ports or browser
format. This design branch is based directly on that accepted renderer commit.
The live-demo publication slice may merge it to `origin/main` first, but
implementation planning must re-check that the accepted commit remains an
ancestor of the then-current integration base and rerun the locked fixture
before code is assigned.

The existing local live-demo snapshot contains 65 documents, 1,202 operations,
1,826 schemas, and 3,028 exact-searchable visible targets. It is the required
large-catalog native acceptance fixture, not the default browser-compilation
fixture.

## Package Boundary

The portable dependency direction is:

```text
domain
  ^
application/catalog       compiler, search, runtime snapshot values
  ^
application/port          artifact, activation, source interfaces
  ^
internal/adapters         filesystem, browser bridge, static output, future cloud
  ^
internal/selfhosted       native composition
cmd/*                     native and Wasm entrypoints
```

Portable packages must not import:

- `os`, `path/filepath`, `database/sql`, AWS SDKs, or `syscall/js`;
- HTTP handlers, Fly configuration, Manja Cloud packages, or management models;
- browser storage, Service Worker, or DOM APIs;
- concrete filesystem, database, or object-store implementations.

Wasm entrypoints and JavaScript bridges remain internal composition adapters.
Public Go packages expose domain values and context-first ports only.

## Canonical Snapshot Contract

### Snapshot identity

`SnapshotID` remains the SHA-256 identity of canonical `SnapshotIdentityV1`.
That identity binds:

- every source byte digest, length, role, document key, and source path;
- catalog ID, title, branding, default document, and compatibility profile;
- exact compatibility allowlist bytes;
- parser module identity and checksum;
- compiler, projection, search, and partition format identities;
- effective compilation bounds;
- every logical child path, kind, media type, uncompressed length, and SHA-256
  digest;
- every child activation, search, rendering, source-download, and full-offline
  readiness role;
- every child decoder/format version and required compatibility identity.

Wall-clock time, machine paths, HTTP origin, storage bucket, object metadata,
compression, presentation runtime build, and activation state are excluded.

Snapshot identity is data/projection identity. Rendered HTML cache identity is
the composite:

```text
snapshot ID + renderer format + Goshtoso asset identity + route + representation
```

This prevents immutable snapshot URLs from falsely claiming that HTML produced
by a later renderer build is byte-identical.

### Manifest

The canonical manifest is itself an immutable artifact. Every behavior-bearing
field below is copied into `SnapshotIdentityV1` in the same canonical order; it
is never mutable metadata outside the identity. For each child it declares:

- logical path within the snapshot;
- semantic kind and media type;
- exact uncompressed length and SHA-256 digest;
- whether the child is required for base activation, search, selected rendering,
  source download, or full-offline readiness;
- format version required to decode it.

Logical paths are presentation and routing names only. Storage adapters key
physical objects by content digest. A manifest may reference one object from
multiple logical paths without duplicating bytes.

The manifest is the sole authority for logical kind, media type, readiness role,
and decoder version. Physical object metadata contains only key, byte length,
and digest. Therefore the same object bytes may be referenced as more than one
logical kind without depending on first-writer metadata. Snapshot decoding
recomputes identity from the complete manifest and rejects a mutation of any
behavior-bearing field even when child bytes did not change.

Canonical JSON follows the existing strict codec rules: sorted slices, fixed
struct field order, valid UTF-8, no duplicate keys, no unknown fields, no trailing
values, compact encoding, no timestamps, and no map iteration in identity bytes.

### Portable source seed and finalized envelope

The compiler input and output are intentionally different. Native and browser
adapters first construct `SourceSeedSetV1`, containing requested/defaultable
catalog fields plus bounded root bytes and logical paths. It contains no resolved
reference graph:

```go
type SourceSeedSet struct {
    FormatVersion   string
    CatalogID       string // optional until portable defaulting
    Title           string // optional until portable defaulting
    Profile         string
    DefaultDocument string
    Roots           []SeedSource // sorted by LogicalPath
}

type SeedSource struct {
    Descriptor SeedDescriptor
    Context    ResolutionContext // opaque and ephemeral
    Bytes      ByteStream        // one-pass root bytes
}

type ResolutionContext uint32 // nonzero per-compilation intern handle

type SeedDescriptor struct {
    LogicalPath string
    Role        SourceRole
    Length      uint64
    SHA256      [32]byte
}
```

Only `SeedDescriptor` fields are canonical. `Context` and `Bytes` are runtime
capabilities excluded from serialization and identity. `ByteStream` is a
context-bound single-use read/close stream: the compiler reserves its declared
root bytes before the first read, reads at most `Length + 1`, hashes while
reading, closes on success/error/cancellation, and rejects short, long, or digest-
mismatched input. It is never rewound. Reads after close return the shared typed
closed-stream error; a second compilation must request a new seed/stream from the
adapter. Root URL acquisition applies the same portable browser acquisition
policy before supplying this stream.

`ResolutionContext` has one permitted operation outside its adapter: exact
equality. For one compilation, an adapter must intern semantic acquisition
targets and assign one stable nonzero handle to each distinct `(target,
reference-base)` context. Repeated resolution of the same target/base returns
the same handle; different target/base contexts return different handles even
when bytes match. Handles are never recycled during that compilation, need not
have equal numeric values across adapters, and are never serialized, hashed,
logged, or exposed in diagnostics. An adapter unable to preserve this
equivalence returns `resolver_context_unavailable`. Duplicate root handles are
rejected as `duplicate_root_context`; equal handles with conflicting supplied
root descriptors are `source_changed`.

After parsing and resolution, the compiler emits canonical
`FinalizedSourceSetEnvelopeV1` with sorted resource occurrences and edges. Each
occurrence separates semantic reference-base context from physical byte identity:

```go
type ResolvedResource struct {
    InstanceID  SourceInstanceID
    LogicalPath string
    Role        SourceRole
    Format      SourceFormat // json or yaml
    MediaType   string       // canonical value derived only from Format
    Length      uint64
    SHA256      [32]byte
}

type ResolutionEdge struct {
    FromInstance SourceInstanceID
    FromPointer  string
    ToInstance   SourceInstanceID
    Kind         ResolutionEdgeKind // creation or reference
}
```

`SourceFormat` is a closed portable enum with values `json` and `yaml`.
Portable code detects it from admitted bytes, never from an extension or HTTP
header: strict JSON decoding is attempted first; only a strict JSON failure may
fall through to the named portable YAML decoder. Both reject duplicate keys,
unknown document encodings, trailing values/documents, and unsupported YAML
features. Detection derives the only canonical media values:
`application/json` for `json` and `application/yaml` for `yaml`. Acquisition
content type is only an ephemeral hint and cannot override byte detection or
enter identity.

Root and child IDs are SHA-256 over canonical JSON of these disjoint tagged
preimages; every field shown is serialized and identity-bearing:

```go
type RootSourceInstanceV1 struct {
    Kind        string // "root-v1"
    LogicalPath string
    Role        SourceRole
    Format      SourceFormat
    MediaType   string
    Length      uint64
    SHA256      [32]byte
}

type ChildSourceInstanceV1 struct {
    Kind           string // "child-v1"
    Parent         SourceInstanceID
    ParentPointer  string
    LogicalPath    string
    Role           SourceRole
    Format         SourceFormat
    MediaType      string
    Length         uint64
    SHA256         [32]byte
}
```

Each child preimage records its one identity-forming creation parent/pointer.
Portable v1 coalesces equal `ResolutionContext` handles within one compilation.
The first target committed in deterministic request order creates the semantic
occurrence; later equal-context targets emit `reference` edges to it without
opening, parsing, or charging its bytes/documents again. Unequal contexts always
create distinct semantic occurrences even when bytes match, so different
reference bases cannot collapse. Physical bytes additionally deduplicate by
digest.

Child logical path is compiler-derived, never adapter-derived:

```text
refs/<full-parent-instance-hex>/<full-sha256(canonical ChildLocatorV1)>
ChildLocatorV1 = { parentPointer, format }
```

`parentPointer` is normalized before hashing. The full digests keep the mapping
collision-free under the format's SHA-256 identity assumption; an exact path
collision with a different locator is corruption, never suffix-renamed. No
acquisition URL, reference string, credential, context handle, or response order
enters the path.

Traversal is deterministic. Distinct roots are registered in finalized
logical-path order, then occurrences are processed through a FIFO queue. Within
each occurrence, external references are identified by normalized JSON Pointer
order and assigned global monotonic request IDs. At most four identifications may
run concurrently, but results are committed strictly in request-ID order; later
completions remain bounded pending results. A new context is opened exactly once,
then its created occurrence is appended to the FIFO queue. Adapter latency cannot
change the creator, logical path, edge kind, occurrence order, or ID.

Seed/finalization rules are shared portable code:

- paths are NFC UTF-8, slash-separated, relative, dot-segment free, and unique
  under exact and Unicode-case-fold comparison;
- a directory upload preserves normalized relative paths; a group of loose files
  must have unique normalized names or is rejected rather than renamed;
- one root/default document is explicit when several documents are possible;
- if the user does not enter a catalog ID, it is the validated slug of root
  `info.title`, or `local-preview-` plus the first 12 hex characters of the root
  content digest when the title has no slug; collisions are rejected;
- title defaults to root `info.title`; profile defaults to the named portable v1
  profile; every explicit/defaulted value is stored in the finalized envelope;
- revision is `source-set-sha256-<digest>` over finalized metadata, occurrences,
  and resolution edges. It is computed only after graph completion and never
  contains a filesystem path, context handle, or URL;
- acquisition URL, authorization data, browser object URL, and local machine path
  are ephemeral adapter state and never enter the envelope, manifest, cache key,
  diagnostic, or receipt;
- ordering follows canonical instance/edge keys, never fetch completion order.
- `ResolutionEdgeKind` is a closed enum. Each non-root occurrence has exactly
  one `creation` edge whose parent/pointer equals the serialized child preimage
  and reconstructs that child ID. Roots have no creation edge. Every later equal-
  context target is `reference`. Decoding reconstructs the canonical root/FIFO/
  pointer traversal from the creation forest; a reference target must be the
  source itself or an occurrence already registered before that edge. Across
  both kinds, at most one edge may exist for each `(from, pointer)`. Decoder also
  verifies normalized pointer syntax, endpoints, canonical order, and no exact
  duplicate edge. It does not re-prove context equality. The child preimage is
  reconstructed from its `ResolvedResource` fields plus its sole creation edge;
- finalized-envelope decoding recomputes every root and child instance ID from
  its complete preimage, rejects unknown format/media/edge-kind values or a
  noncanonical format/media pair, verifies every edge endpoint, and rejects
  missing, duplicate, or forged creation bindings;
- envelope decoding proves canonical structure and identity integrity, not the
  semantic truth of later reference targets. Re-proving those targets requires
  source reacquisition and compilation; content-addressed snapshot consumers do
  neither during normal load;
- every unique context creator charges document count, aggregate decoded-source
  bytes, depth, diagnostics, and parse/compiler work once. Every resolution edge
  separately charges bounded fan-out work. Physical-store deduplication never
  weakens either admission class;
- canonical codec vectors cover JSON and YAML roots/children, the complete
  `SourceInstanceID` preimage, shuffled occurrences/edges, malformed media/format
  pairs, dangling edges, root self-reference, two-node cycles, non-root back
  edges, repeated DAG references, duplicate-pointer/different-target edges,
  reference edges to not-yet-created targets, and missing/duplicate/forged
  creation edges.

The UI may collect the explicit root, catalog ID, title, or profile, but it calls
the shared seed constructor. Shuffling inputs cannot change finalized bytes.
Token-bearing pasted URLs are ephemeral acquisition addresses; their query and
fragment never enter persisted identity or user-visible diagnostics.

### Streaming compiler output

The compiler must stop retaining the entire expanded source graph, compiled
projection, and encoded children at the publication boundary. It writes each
bounded child to a `SnapshotWriter` as soon as the child's canonical bytes and
identity are final. The writer may spool, upload, or cache the bytes.

The compiler retains only bounded working state required for cross-document
validation, semantic schema deduplication, the global directory, and global
search construction. Search builders may use bounded spill files or adapter-
provided scratch storage. Scratch bytes are never snapshot identity inputs.

Commit order:

1. validate and admit all sources;
2. write immutable children;
3. finish global directory and search artifacts;
4. write the immutable manifest last;
5. preflight all required artifacts through the reader contract;
6. return the snapshot reference;
7. activation occurs separately.

Failure before step 6 leaves only unreachable immutable objects. It never exposes
a partial snapshot. `Abort` releases scratch resources but cannot require remote
deletion for correctness.

## Storage Ports

### Immutable artifact store

The application contract expresses immutable objects, not a writable filesystem:

```go
type ArtifactDescriptor struct {
    Key         ArtifactKey // sha256:<lowercase hex>
    Length      uint64
    SHA256      [32]byte
}

type ArtifactStore interface {
    PutIfAbsent(context.Context, ArtifactDescriptor, io.Reader) error
    Open(context.Context, ArtifactKey) (io.ReadCloser, ArtifactMetadata, error)
    Stat(context.Context, ArtifactKey) (ArtifactMetadata, error)
}
```

Required semantics:

- `PutIfAbsent` reads at most `Length + 1`, hashes while streaming, rejects short,
  long, or digest-mismatched content, and never replaces different bytes;
- repeated identical writes are idempotent;
- a successful `Open` returns exact immutable bytes and verified metadata;
- not-found, corrupt, transient, canceled, and budget errors remain typed;
- adapters bound diagnostics and honor context cancellation;
- no interface exposes `os.File`, filesystem path, rename, directory, bucket,
  ETag, provider SDK type, or browser object;
- object listing and deletion are not required for serving or publication.

The v1 reader returns a stream. Optional range/CDN acceleration may use a
capability interface later; it cannot weaken base correctness.

### Snapshot registry

An immutable snapshot registry associates one validated manifest with its
snapshot ID and supports preflight without scanning storage:

```go
type SnapshotRegistry interface {
    PutManifestIfAbsent(context.Context, SnapshotManifest) error
    Manifest(context.Context, SnapshotID) (SnapshotManifest, error)
}
```

The manifest write is the candidate-complete marker. Existing manifests are
immutable. A conflicting manifest for the same ID is corruption.

### Route-set and runtime registries

Activation IDs are always dereferenceable through immutable registries:

```go
type RouteSetRegistry interface {
    PutIfAbsent(context.Context, RouteSetDescriptor) error
    Descriptor(context.Context, RouteSetID) (RouteSetDescriptor, error)
}

type RuntimeBundleRegistry interface {
    PutIfAbsent(context.Context, RuntimeBundleDescriptor) error
    Descriptor(context.Context, RuntimeBundleID) (RuntimeBundleDescriptor, error)
}
```

`PutIfAbsent` and `Descriptor` recompute canonical ID, reject unknown fields, and
validate descriptor structure without requiring referenced objects to be healthy.
Reference preflight is a separate application operation returning a bounded
per-mount/runtime health map; this lets recovery inspect a trusted mapping even
when one referenced snapshot is corrupt. Descriptor lifetime is at least the
lifetime of any current, pointer fallback, per-mount previous, candidate,
static-export, or lease reference; mutation-outcome expiry cannot delete it. Structural descriptor
corruption remains distinct from referenced-component corruption.

### Route-set activation repository

Mutable route authority is separate from artifact bytes and is atomic for the
whole namespace, never one mount at a time:

```go
type NamespaceKey struct {
    Namespace string // opaque composition-owned scope
}

type RouteSetDescriptor struct {
    ID      RouteSetID
    Mounts  []MountState // sorted, unique mount
    Runtime RuntimeState
}

type MountState struct {
    Mount        string
    Status       MountStatus // ready or unavailable
    Active       SnapshotID
    Previous     SnapshotID
    FailureCode  FailureCode // required only when unavailable
}

type RuntimeState struct {
    Mode        RuntimeMode   // native or wasm
    Status      RuntimeStatus // ready or unavailable
    Active      RuntimeBundleID
    Previous    RuntimeBundleID
    FailureCode FailureCode // required only when unavailable
}

type ActivationState struct {
    Generation uint64
    Current    RouteSetID
    Fallback   RouteSetID // structural fallback if Current cannot be decoded
}

type ActivationPointers struct {
    Current  RouteSetID
    Fallback RouteSetID
}

type ActivationMutation struct {
    ID                 MutationID // globally unique, stable across retries
    Kind               MutationKind // publish or recovery
    ExpectedGeneration uint64
    Desired            ActivationPointers // descriptors already registered
}

type MutationOutcome struct {
    MutationID MutationID
    Status     MutationStatus // committed or conflicted
    State      ActivationState
    Desired    ActivationPointers
}

type ActivationRepository interface {
    Get(context.Context, NamespaceKey) (ActivationState, error)
    ReplaceRouteSet(context.Context, NamespaceKey, ActivationMutation) (
        MutationOutcome, error,
    )
    Outcome(context.Context, NamespaceKey, MutationID) (MutationOutcome, error)
}
```

The repository durably records `(namespace, mutation ID, kind, expected
generation, desired current/fallback IDs, outcome)` in the same transaction that
advances the pointers. Replaying the exact mutation returns the exact recorded outcome. Reuse
of a mutation ID with different inputs is corruption. A caller receiving an
unknown commit result calls `Outcome`; it never invents a new mutation to retry.
Committed and conflicted outcomes remain queryable through the configured
reconciliation/receipt retention window, which must exceed every retry window.

Rules:

- desired current/fallback descriptors are loaded structurally from
  `RouteSetRegistry`; the current descriptor's referenced health map is evaluated
  before CAS;
- all mounts change as one generation or none do; a failure after staging the
  first mount cannot expose a mixed route set;
- a normal `publish` transition validates every ready active component. For each
  unchanged mount/runtime it preserves history; for each changed active value it
  shifts only that component's old active into its previous field exactly once.
  A changed current descriptor sets pointer fallback to the exact old current;
  an unchanged publish preserves the existing fallback;
- a `recovery` transition validates the exact desired per-component state but
  performs no implicit rotation. It may clear previous, promote previous while
  clearing corrupt active, substitute one mount/runtime, or mark only one mount
  unavailable. It may also promote/clear pointer fallback without retaining a
  corrupt current descriptor. It must reference tombstones/health evidence for
  every removed corrupt component;
- identical desired pointers are an idempotent no-op for either kind;
- generation mismatch never overwrites newer authority;
- no corrupt component retained by a recovery transition remains a GC root;
- transient failures remain retryable and do not permanently disable a route set;
- namespace is opaque to open core. Manja Cloud may derive it from tenant and
  environment identity without adding tenant types to `domain`.

Canonical mount-state invariants:

- `ready` requires a nonempty active snapshot, permits either no previous
  snapshot or one nonempty previous snapshot distinct from active, and requires
  an empty failure code;
- `unavailable` requires empty active and previous snapshot IDs and exactly one
  bounded typed failure code. It never retains the rejected snapshot as route
  authority or as a GC root;
- unknown status values, equal active/previous IDs, a failure code on `ready`,
  a missing failure code on `unavailable`, or any other field combination is
  invalid and cannot produce a `RouteSetID`;
- `FailureCode` is a closed RouteSetDescriptorV1 enum encoded as a lowercase
  ASCII snake-case token of at most 64 bytes. Mount states permit
  `snapshot_missing`, `snapshot_corrupt`, or `snapshot_incompatible`; Wasm
  runtime states permit `runtime_missing`, `runtime_corrupt`, or
  `runtime_incompatible`. A descriptor using the wrong component's code is
  invalid.

Canonical runtime-state invariants:

- `native + ready` has empty active/previous/failure fields and means the
  composition-pinned in-process renderer/search implementation;
- `wasm + ready` requires a nonempty compatible active bundle, permits one
  distinct compatible previous bundle, and has no failure code;
- `wasm + unavailable` requires empty active/previous and one bounded typed
  failure code. It is never interpreted as native rendering;
- `native + unavailable`, unknown enum values, equal active/previous IDs, or any
  field combination outside these cases is invalid.

Filesystem uses the existing journal, immutable descriptor files, and atomic
route-table replace behind these interfaces. Browser uses CacheStorage/IndexedDB
registries plus one IndexedDB activation transaction. Future cloud uses immutable
descriptor objects/rows plus a database conditional transaction and mutation-
outcome table. S3 object metadata is not activation authority.

### Runtime bundle descriptor

`RuntimeBundleID` is the digest of canonical `RuntimeBundleDescriptorV1`, which
binds exact renderer/search Wasm bytes, Go Wasm runtime, shell, Goshtoso/assets,
compatibility matrix, required supervisor protocol, and route-protocol identity.
Native in-process rendering records the native presentation identity in receipts
and uses canonical `native + ready` runtime state because deployment authority
already pins one executable.

Browser activation, rollback, recovery, and garbage collection always operate on
the current route-set descriptor and its per-mount/runtime active/previous fields
loaded through both registries. Each component keeps its exact fallback across
unrelated mount publication.

The executing Service Worker is a stable supervisor outside rotating runtime
bundles because browser install/controller state cannot join an IndexedDB
transaction. Supervisor protocol v1 can serve both active and previous
descriptors and a pending compatible candidate. A supervisor update uses the
browser's normal install/waiting/activate lifecycle and may claim clients only
after it proves compatibility with the currently active and previous protocol
requirements. Route-set CAS may select a candidate only when both the current
controller and any waiting successor declare compatibility. Thus old or new
supervisor bytes can route to the generation named in IndexedDB; neither is
treated as atomically rotated or rollbackable by that transaction.

### Component-scoped corruption recovery

The activation record is authority. `Current` and `Fallback` protect structural
descriptor recovery; each structurally valid current descriptor independently
protects every mount and runtime through its `Active` and `Previous` fields.
Recovery computes one exact desired descriptor plus exact current/fallback
pointers; it never relies on publish rotation:

- healthy current descriptor + corrupt/missing pointer fallback: preserve the
  current descriptor byte-for-byte and recovery-CAS `{Current: current,
  Fallback: empty}`;
- corrupt/missing current descriptor + healthy pointer fallback: recovery-CAS
  `{Current: fallback, Fallback: empty}`. The corrupt current never becomes a
  fallback or GC root;
- healthy current descriptor + one corrupt active snapshot for mount B:
  register a repaired descriptor preserving every other mount/runtime field.
  Set B active to its verified B previous and clear B previous, or set only B to
  unavailable when no verified B previous exists. A previous snapshot from mount
  A is never substituted for B;
- corrupt active runtime: register a repaired descriptor preserving every mount.
  Set runtime active to its verified compatible runtime previous and clear
  runtime previous. If none is compatible, set runtime to canonical
  `wasm + unavailable`, preserve every healthy mount/history, and fail local
  render/search closed with bounded degraded readiness;
- corruption reachable only from one component's previous field: register a
  repaired descriptor preserving its active and every unrelated history while
  clearing only that corrupt previous;
- after repairing a component within a healthy current descriptor, preserve a
  healthy pointer fallback only when it does not retain the same corrupt
  component; otherwise clear it;
- corrupt/unverifiable activation authority, or no reconstructable trusted route
  mapping, fails the namespace closed without deleting evidence.

Example: current mounts are `A(active=A2, previous=A1)` and
`B(active=B1, previous=B0)`. Corrupt B1 yields A2/A1 unchanged and either
B0/empty or B unavailable. It never rolls A back and never retains B1 in any
current/fallback descriptor.

When B has no verified B0, the only canonical repaired mount is
`B(status=unavailable, active=empty, previous=empty,
failure=snapshot_corrupt)`. The recovery fixture fixes the descriptor's exact
canonical bytes and recomputed `RouteSetID`; records a tombstone for B1; and
asserts that B1 is absent from current, pointer fallback, admitted-candidate,
lease, and enumerated GC roots. An encoding that retains B1 in either ID field
or uses a different failure/status combination is corruption, not an alternate
adapter policy.

Runtime example: `wasm + ready(active=R1, previous=R0)` with corrupt R1 becomes
`wasm + ready(active=R0, previous=empty)`. Without R0 it becomes
`wasm + unavailable(active=empty, previous=empty,
failure=runtime_corrupt)`. Neither state is confused with `native + ready`.

Recovery uses a deterministic mutation ID derived from the damaged generation,
health evidence, and exact desired descriptor/pointers. It registers the repaired
descriptor, performs at most one durable recovery CAS, records tombstones, and
never loops between generations. Concurrent publication winning the generation
CAS supersedes recovery; recovery reloads authority and does not apply stale
state. Native, browser, and cloud adapters implement the same state machine;
transient read/time failures do not trigger corruption recovery.

### References and garbage collection

Serving correctness cannot depend on immediate deletion. Artifact objects are
immutable and may be shared across snapshots.

Garbage collection is a separate application service over a repository-specific
enumeration capability. It computes roots from current/fallback descriptors,
every per-mount and runtime active/previous reference reachable from those
descriptors, admitted candidates, retained static exports, and explicit leases.
Quarantined descriptors/components removed by a committed recovery mutation are
not roots. It deletes only objects unreachable for a configured grace period. Failure may
consume remaining configured capacity; admission then fails before exceeding the
hard store cap. It cannot break active snapshots.

The next slice retains current filesystem budgets and the numeric browser budgets
below. Cloud retention, legal hold, billing, and cross-tenant deduplication policy
remain Manja Cloud concerns.

## Adapter Mapping

| Mode | Artifact store | Snapshot/route-set/runtime registries | Activation repository |
|---|---|---|---|
| Local native | filesystem objects | immutable filesystem descriptors | durable journal and route table |
| Static export | output directory | exported descriptors | none |
| Browser preview | CacheStorage | verified CacheStorage objects + IndexedDB descriptor index | IndexedDB transaction |
| Future Manja Cloud | S3-compatible objects | immutable objects plus database descriptor index | PostgreSQL/DynamoDB CAS |

`io/fs` may remain a read-only adapter for embedded assets. It is insufficient
for streaming verified writes, conditional activation, multipart transfer,
garbage collection, or browser transactions.

## Versioned Route Protocol

Route ownership is composition-supplied and source data can never choose an
arbitrary HTTP mount. `RouteProtocolV1` defines path-based canonical routes:

```text
<base>/catalogs/<mount>/
<base>/catalogs/<mount>/documents/<document>/
<base>/catalogs/<mount>/documents/<document>/details/<detail-id>/
<base>/catalogs/<mount>/search/
<base>/catalogs/<mount>/artifacts/<logical-path>
```

Published native composition supplies a normalized public base such as `/` or
`/docs/`. Static export maps each trailing-slash route to a distinct
`index.html`, so a plain directory server returns route-specific initial HTML
with JavaScript disabled. The current `?selected=<id>#<id>` form remains a
temporary native compatibility alias that redirects to the path canonical; it is
not a static canonical or immutable representation key.

Local preview owns only this disjoint, reserved scope:

```text
/__manja/local/v1/<local-activation-id>/catalogs/<mount>/...
```

`local-activation-id` is a random, browser-local opaque ID allocated by the
shell, not a catalog/source value. Local mounts are validated slugs within that
scope. The Service Worker allowlist is the exact local activation prefix plus
content-addressed Manja asset prefixes. It can never intercept `/api`, `/manage`,
published `/catalogs`, another activation ID, or an upstream request. Local
canonical metadata is `noindex` and uses the scoped route; published/static
canonical metadata uses the configured public origin and base.

History entries contain the full path canonical. Reload, back/forward, and an
unvisited complete-offline deep link resolve the route-set generation named by
the local activation ID. Removing a local activation first revokes its route-set
pointer, then its scope returns bounded 404; cleanup never reassigns that ID.

`RouteRepresentationID` binds route protocol, route, data snapshot, runtime
bundle/presentation identity, and normalized export configuration where
applicable. Mutable aliases and redirects are never immutable cache keys.

Snapshot directories and search records are route-neutral. The accepted v1
fields that currently persist `Href` migrate to this canonical token:

```go
type RouteTarget struct {
    DocumentKey string
    Kind        DetailKind
    DetailID    string
}
```

`RouteTarget` participates in snapshot/search identity; HTTP origin, base path,
mount, local activation ID, query strings, and absolute/relative href do not. A
shared `RouteComposer` validates the target and combines it with
`RouteProtocolV1` plus the composition-owned scope after ranking. Native SSR,
static export, browser presentation, and Service Worker all use that composer.
Search parity covers ordered targets, scores, and match ranges; href composition
has its own native/browser vector suite. One unchanged snapshot can therefore be
mounted at two published bases and one local scope without recompilation.

## Native Commands

### `manja build`

```text
manja build \
  --renderer-config renderer.yaml \
  --snapshot-store ./manja-snapshots
```

Loads configured file/Git sources, compiles all catalogs, writes complete
snapshots, preflights them, constructs one complete route set, and atomically
updates the namespace activation repository. No HTTP listener starts. Output
includes bounded machine-readable receipts and its idempotent mutation ID.

Route-set construction begins from the recovered current descriptor and produces
one per-configured-mount outcome:

- a successful refresh advances that mount to the new snapshot;
- source, parse, compile, or candidate-preflight failure carries forward the
  exact healthy active snapshot and its history for that mount and emits a bounded degraded
  receipt;
- a configured mount with neither a healthy recovered snapshot nor a valid new
  candidate aborts the whole mutation and leaves current/fallback unchanged;
- explicit config removal omits the old mount only after every remaining mount
  is admissible and emits a removal receipt;
- cancellation, namespace/global budget failure, corrupt current route-set
  authority, or failure to load any carried snapshot aborts the whole mutation;
- all per-mount receipts and the desired descriptor are deterministic inputs to
  one registry write and one namespace CAS.

Thus successful catalogs may advance while a failed catalog preserves its
last-known-good snapshot, but no request can observe a half-written route set.

### `manja serve`

```text
manja serve \
  --renderer-config renderer.yaml \
  --snapshot-store ./manja-snapshots \
  --refresh=never
```

Recovers current/fallback route descriptors and every per-mount/runtime history,
then serves without loading source bytes or constructing compiler/parser
instances. `--refresh=never` is the live-demo and static runtime contract. Other
deployments may use `--refresh=startup` or an explicit future sync command.

If no recoverable active snapshot exists, startup fails readiness. It never
silently compiles when refresh is disabled.

### `manja export`

```text
manja export \
  --snapshot-store ./manja-snapshots \
  --catalog kubernetes \
  --output ./public
```

Materializes canonical path-based index/detail URLs, source downloads, search
artifacts, assets, metadata, and an offline manifest. Output contains no mutable
activation state.

Export consumes canonical `ExportConfigurationV1`: normalized public origin,
base path, route protocol, canonical/social URL policy, social image identity,
offline mode, shell/runtime/asset identity, header policy, CSP, Service Worker
scope, and content-coding policy. Every byte-affecting option is present in that
configuration and in `RouteRepresentationID`; environment variables not copied
into it cannot affect output. Two clean exports of the same route set, runtime
bundle, and exact normalized configuration are byte-identical. Changing any
byte-affecting option changes representation identity and cannot reuse immutable
aliases.

## Browser Local Preview

### User journey

1. User opens the Manja live demo.
2. User drops one or more OpenAPI JSON/YAML files or pastes a URL.
3. Manja shows admitted input size, selected compatibility profile, and a
   cancelable compilation progress indicator.
4. `compiler.wasm` compiles in a dedicated Worker and writes a candidate snapshot
   to browser storage.
5. Browser preflights required artifacts and atomically activates the snapshot.
6. The worker terminates, releasing compiler/parser memory.
7. Normal Manja navigation opens the local catalog. `search.wasm` and
   `renderer.wasm` run in dedicated workers.
8. The application reports complete-offline readiness or rejects activation with
   an actionable storage estimate.

There is no manual “compile then upload” step.

### Input policy

- File upload is always local and never sent to Manja servers.
- URL import uses browser Fetch with `credentials: omit` and requires a
  CORS-readable response.
- Redirects are rejected in portable browser profile v1; scheme, bytes,
  decompression, documents, references, depth, diagnostics, and compile time are
  bounded.
- Only `https:` URLs are accepted outside localhost development.
- Cross-origin references obey the same CORS and credential policy.
- The open-core demo does not proxy blocked URLs. Manja Cloud may later offer an
  authenticated, audited import service under its own policy.
- Cancellation terminates the compiler worker and discards its candidate
  activation. Previously active local snapshots remain available.

### Source resolution protocol

OpenAPI parsing discovers references; adapters acquire bytes. The portable
compiler calls this context-first contract through a native adapter or a
JavaScript/Wasm bridge:

```go
type ResolveRequest struct {
    RequestID       uint64
    ReferrerInstance SourceInstanceID
    ReferrerContext ResolutionContext // opaque adapter handle
    ReferrerPointer string
    Reference       string
}

type ResolutionTarget struct {
    RequestID uint64
    Context   ResolutionContext
}

type OpenRequest struct {
    Context   ResolutionContext
    Remaining ResolutionBudget
}

type OpenResponse struct {
    Context       ResolutionContext
    Length        uint64
    SHA256        [32]byte
    MediaTypeHint string // ephemeral acquisition hint; never canonical or identity
    Bytes         ByteStream
}

type SourceResolver interface {
    Identify(context.Context, ResolveRequest) (ResolutionTarget, error)
    Open(context.Context, OpenRequest) (OpenResponse, error)
}
```

The compiler owns reference discovery, canonical edge construction, cycle and
depth detection, context-group deduplication, and budget reservation. The adapter
owns target interning and bounded byte acquisition. `Identify` resolves the
reference against private adapter context without network/body acquisition and
returns its stable per-compilation token. In deterministic commit order, the
compiler calls `Open` only for a token absent from its creator table. `Open` must
return the requested token, and its stream is consumed once; a repeated `Open`
for that token is a compiler error. Later equal tokens reuse the verified creator
occurrence. The browser bridge resolves relative network requests against the
private acquisition context. Portable browser profile v1 uses Fetch
with `redirect: "error"`: zero redirects are followed. This is deliberately
narrow because automatic Fetch hides hop count and manual cross-origin redirects
hide `Location`. The initial URL must be `https:` outside localhost, credentials
are omitted, opaque/redirected responses are rejected, and at most the reserved
decoded bytes are streamed. Native cross-target parity fixtures use the same
zero-redirect portable profile; a native publisher may expose a separately named
non-portable acquisition profile without claiming identical browser availability.
Browser Fetch cannot distinguish redirect rejection,
CORS denial, DNS/TLS failure, or other pre-response network rejection. Portable
profile v1 maps all of those browser-unobservable causes to the single bounded
`fetch_unavailable` class; the native portable adapter maps equivalent cases to
the same class. Only locally observable pre-fetch scheme/admission errors,
post-response size/format/digest errors, and cancellation remain distinct. Richer
native acquisition may expose a separately named non-portable taxonomy. The
adapter never returns or guesses acquisition URLs/causes in portable diagnostics.

Each new context reserves aggregate decoded bytes, document count, and depth
before `Open`. A body is hashed and length-checked while streaming; unused
reservation is released, but a crossing is rejected before another body is read.
Identification shares one cancellation scope and is bounded to four concurrent
requests; body opening/commit is serialized by request order. Total resolution
edges and edges per occurrence have separate hard limits. Native and browser
adapters receive shared token-equivalence conformance vectors and must produce
the identical finalized envelope, snapshot, and typed deterministic error for
the same admitted resolved graph. Fresh-token-for-same-target,
recycled/colliding-token, zero-token, or unavailable-interning adapters fail
conformance; an equal token with conflicting root/open descriptors fails
`source_changed`. A fixture with byte-identical `a/common.yaml` and
`b/common.yaml` under unequal contexts, whose relative `child.yaml` bytes differ,
must retain both occurrence graphs regardless of shuffled or concurrent
identification.

### Browser compilation profiles

Browser defaults are intentionally lower than native publisher limits. Initial
v1 gates:

- 16 MiB aggregate decoded source bytes;
- 8 MiB per document;
- 64 documents;
- 16,384 resolution edges total and 1,024 per document;
- 5,000 operations;
- 5,000 schemas;
- 64 MiB committed snapshot bytes;
- one compiler worker;
- one candidate compilation at a time;
- explicit 256 MiB estimated working-memory admission ceiling.

These are deterministic admission limits, not promises that every device has
enough memory. When `navigator.deviceMemory` exists and indicates a constrained
device, Manja may lower the local ceiling but must not raise the canonical v1
limits. Unknown capability uses conservative defaults.

The Kubernetes catalog is not required to compile in the browser milestone. The
native published demo remains available and can export an installable snapshot.

Local preview activation is complete-offline only in v1. Every admitted snapshot,
including a 32-64 MiB snapshot, must have every `fullOfflineRequired` child and
its compatible runtime bundle verified locally before pointer mutation. There is
no visited-only local activation because the terminated compiler is not an
artifact origin. If storage reservation cannot retain the complete generation,
the candidate is rejected before activation with the required/available byte
estimate. Visited-only readiness remains an opt-in cache state for published
server-backed catalogs.

## Wasm Module Boundaries

### `compiler.wasm`

Contains parser, compatibility validation, compiler, partitioner, search-index
builder, and the portable side of `SourceResolver`. It imports no DOM, Service
Worker, filesystem, Git, management, or HTTP client/server package. JavaScript
supplies the bounded resolver and browser artifact-writer bridges; it does not
rediscover or interpret OpenAPI references.

It runs once per candidate, reports bounded progress, and terminates after
success, cancellation, or failure. It is never retained as the search runtime.

### `search.wasm`

Contains only canonical normalization, exact lookup, token/trigram candidate
generation, fuzzy scoring, ranking, and matched-range calculation. It reads the
compact immutable search artifacts, not raw OpenAPI.

It returns typed data:

```json
{
  "id": "operation-createNamespacedPod",
  "target": {
    "documentKey": "core-v1",
    "kind": "operation",
    "detailId": "detail-sha256-..."
  },
  "score": 0.94,
  "matches": {
    "title": [[0, 6]],
    "description": [[18, 24]]
  }
}
```

Ranges use UTF-8 byte offsets into exact returned strings. JavaScript validates
boundaries, escapes text, and inserts `<mark>` elements; Wasm never returns
trusted HTML. Result count, decoded bytes, postings scanned, query bytes, tokens,
and candidate comparisons are bounded by deterministic counters in the search
contract.

Native server fallback calls the same Go query package. Given the same snapshot,
query, and search-contract identity, browser and server return identical ordered
IDs, route targets, scores, and match ranges. Presentation composes the owning
scope's href only after the query completes. Crossing a deterministic work budget
returns the same typed `search_work_budget` error on both targets and no partial
results.
Wall-clock deadline and user cancellation are external availability controls:
they return typed all-or-nothing `canceled` or `temporarily_unavailable`, never a
ranked prefix, and are excluded from result parity. Server fallback may differ
only in availability; every successful result remains identical.

### `renderer.wasm`

Contains selected-route projection decoding and the shared Goshtoso/templ
presentation components. It loads only the catalog directory and selected detail
or schema-node shards. It does not parse OpenAPI or load the complete snapshot.

It returns bounded `#main-content`, sidebar-section, and search-result fragments.
JavaScript installs output only into allowlisted targets. Full offline reload uses
the cached shell, then renders the selected canonical route from local artifacts.

Renderer output is keyed by the composite presentation identity, not only the
data snapshot ID.

## Search Contract

Search remains client-first for all eligible online and offline catalogs:

1. load the immutable search directory;
2. lazily load verified segments required by the query;
3. execute `search.wasm` in its Worker;
4. render safe result text and match highlights;
5. navigate to canonical visible detail anchors;
6. use native `/search.json` only when browser search is unavailable or fails
   before local readiness.

Ctrl/Cmd+K, visible sidebar search, recent visits, Escape, arrow keys, Enter,
combobox accessibility, focus restoration, and no-reload close behavior remain
current UI requirements.

The search directory declares both `searchFormat` and `rankingContract`.
Mismatched cached artifacts are never queried. Browser either downloads/builds a
compatible snapshot or uses server fallback.

## Offline Storage and Routing

### Storage ownership

- CacheStorage owns immutable artifact responses, product assets, offline shell,
  Wasm modules, and optional original source downloads.
- IndexedDB owns snapshot manifests, current/fallback route pointers,
  per-component history, candidate state, LRU, readiness, and corruption
  tombstones.
- `localStorage` owns none of the snapshot or activation state.

Browser v1 has one local activation namespace and these origin-wide hard caps:

- exactly one locally activated mount. Its source graph may contain many root,
  referenced, and support documents, but compilation produces one logical
  snapshot bounded by the portable profile. Starting another local preview
  first revokes the old activation pointer; multi-catalog local composition is
  deferred to a later profile with separately proven aggregate headroom;
- 432 MiB total Manja persistent bytes across CacheStorage and IndexedDB;
- 256 MiB unique immutable snapshot/source-object bytes;
- 128 MiB rotating runtime-bundle bytes, with at most 32 MiB and 256 physical
  entries per complete runtime bundle. That complete-bundle bound includes its
  renderer/search Wasm, Go Wasm runtime, shell, Goshtoso assets, and presentation
  assets, but excludes the stable supervisor;
- 16 MiB and 512 physical entries for the stable supervisor worker and its
  non-rotating bootstrap assets. These fixed bytes are installed and accounted
  before any candidate reservation;
- 32 MiB metadata, receipts, tombstones, and candidate metadata;
- 2,048 physical entries per complete snapshot generation and 8,192 entries for
  the four-generation snapshot pool;
- 1,024 physical entries for rotating runtime bundles, 1,024 for all metadata
  and candidate descriptors, and 16,384 physical entries origin-wide. The
  remaining 5,632 entries are available only to evictable visited caches and
  optional source downloads; candidate admission may reclaim all of them;
- at most two committed structural route descriptors (current and distinct
  pointer fallback), their bounded per-mount/runtime active and previous
  components, and one candidate descriptor;
- one candidate lease, renewed by its worker, with a 15-minute maximum lifetime.

Accounting uses verified encoded length for stored objects and charges shared
digests once. The manifest does not exist before streamed child writes, so
reservation cannot depend on it. Before compiler start, IndexedDB atomically
reserves simultaneously the portable profile's full 64 MiB and 2,048-entry
snapshot maximum, exact already-known missing runtime-bundle bytes and entries
(bounded by 32 MiB and 256 entries), one MiB candidate metadata, its bounded
metadata-entry count, and the already charged fixed supervisor allowance. Writes
debit that reservation; the final manifest's actual
lengths must fit it before commit, and unused capacity is released only after
commit/abort reconciliation. Admission never depends on browser quota failure.
The snapshot and runtime subcaps each hold three complete maximum-size committed
generations plus one complete candidate: `3 * 64 MiB + 64 MiB = 256 MiB` and
`3 * 32 MiB + 32 MiB = 128 MiB`. Three committed generations are the maximum
unique set reachable for that one mount from current active/previous plus
pointer-fallback active/previous after digest deduplication. Therefore a legal current/fallback
state always leaves one full candidate reservation inside each hard subcap; a
fourth committed generation cannot become protected before the same atomic CAS
makes the oldest generation unreachable. Metadata has its independent 32 MiB
cap, so candidate metadata never borrows artifact headroom. The simultaneous
byte proof is `256 + 128 + 16 + 32 = 432 MiB`. The simultaneous required-entry
proof is `4 * 2,048 + 4 * 256 + 512 + 1,024 = 10,752`, below the 16,384
origin-wide cap; optional entries are evicted before admission. A candidate is
not admitted when any byte or entry pool cannot reserve its complete maximum.
Current/fallback descriptors, their reachable per-mount/runtime active/previous
components, and an unexpired candidate lease are protected roots. Deterministic
eviction removes expired candidates first, then unreferenced runtimes, optional
original sources, and least-recently-used published visited caches. It never
evicts a protected root. If sufficient space
cannot be reserved after eviction, compilation does not start.

Startup and Service Worker activation reconcile both stores: expire abandoned
leases, delete unreferenced candidate caches after grace, recompute bounded
accounting, and fail closed on missing/extra protected bytes. Browser quota
revocation rejects new work while current/fallback and healthy component history
remain readable when the platform permits. Runtime upgrades reserve the new
bundle before download and retain the old bundle until the route-set CAS and
grace period complete.

### Readiness levels

- **Online only:** no validated local snapshot.
- **Visited offline:** shell, runtime, search, directory, and visited selected
  shards are cached.
- **Complete offline:** every manifest child marked `fullOfflineRequired` is
  verified locally. Every operation/schema route and search result can open with
  network blocked.

`wasm + unavailable` makes local readiness degraded and presentation unavailable
even while snapshots remain healthy. Online published pages may use native SSR
and server-search fallback; a local-only offline scope returns the bounded
runtime-unavailable response until an explicit compatible runtime publication
wins CAS. Recovery never silently downloads or selects a new bundle.

Complete offline is explicit for published large catalogs. Local preview is
always complete offline before activation in v1.

### Atomic browser activation

1. reserve aggregate storage and write immutable candidate artifacts into a
   versioned cache;
2. write and validate candidate manifests and the complete route-set descriptor;
3. reserve and verify the exact runtime-bundle descriptor;
4. prove the current and any waiting supervisor support the bundle's required
   protocol;
5. instantiate matching renderer/search modules and perform a smoke query/render;
6. verify every `fullOfflineRequired` child for local preview;
7. one IndexedDB transaction records the publish mutation outcome and selects
   the candidate current descriptor plus its exact pointer fallback; per-mount
   and runtime history was already validated in the candidate descriptor;
8. notify controlled pages of the new current generation;
9. delete unreachable cache generations only after commit and grace period.

Crash before step 7 leaves current unchanged. Crash after step 7 recovers the
new coherent descriptor and exact mutation outcome. Corruption applies the
component-scoped recovery matrix once and records tombstones; unrelated mounts
never roll back. A transient timeout does not permanently poison valid bytes.

### Service Worker boundary

The Service Worker intercepts only exact local-preview paths and content-addressed
asset prefixes declared by the validated route/runtime descriptor. Published
reader routes remain server/CDN-owned unless a separate published-offline package
is explicitly installed. Management, APIs, arbitrary fetches, cross-origin
requests, and upstream API traffic always pass through.

Unknown local routes receive a bounded offline 404. The worker never invents a
success response for an artifact absent from the active manifest.

## Static Export Contract

Static export materializes:

- initial HTML for catalog overview and every canonical document/detail route;
- route-specific title, description, canonical, Open Graph, and explicit X Card
  metadata;
- a versioned 1280×640 social image with absolute production URL configuration;
- immutable assets and Wasm modules when offline navigation is enabled;
- source downloads and license/provenance artifacts;
- complete search directory and segments;
- offline manifest and Service Worker configuration;
- `_headers`/equivalent reference policy for MIME, caching, CSP, and Service
  Worker scope.

Content-addressed artifacts use long-lived immutable caching. Rendered HTML and
search responses must revalidate unless their URL includes the complete composite
representation identity. Strong ETags hash exact wire bytes after content coding;
otherwise use a weak validator over canonical uncompressed bytes. Responses vary
on `Accept-Encoding` when content coding differs.

## Cloud Adapter Compatibility

Future Manja Cloud composition is:

```text
compiler worker
    |
S3-compatible ArtifactStore
    |
immutable manifest
    |
PostgreSQL/DynamoDB ActivationRepository CAS
    |
renderer nodes + CDN
```

Publication transaction:

1. upload immutable artifact objects;
2. upload immutable snapshot and runtime-bundle descriptors;
3. preflight candidates through the cloud reader;
4. construct, register, and preflight one complete route-set descriptor;
5. a database transaction records the idempotent mutation outcome and publishes
   the complete route set;
6. if the commit reply is unknown, reconcile by mutation ID before any retry;
7. renderer nodes observe the new generation;
8. asynchronous GC handles unreachable objects after grace.

No S3 directory rename or multi-object transaction is assumed. A failed database
commit leaves unreachable immutable objects, not a partially active release.
Production/release authority remains the database activation record, never an
object-store listing or mutable object tag.

Cloud adapter requirements carried by conformance tests now:

- streaming bodies and bounded memory;
- idempotent put-if-absent;
- exact digest/length verification;
- typed transient, missing, corrupt, canceled, and CAS errors;
- independent artifact and activation failure injection;
- concurrent writers from separate processes;
- atomic multi-catalog route replacement, exact mutation replay, unknown-outcome
  reconciliation, structural current/fallback preservation, and per-mount/runtime
  active/previous preservation;
- no reliance on filesystem path, rename, lock file, or process-local mutex.

## Failure and Recovery Matrix

| Failure | Required outcome |
|---|---|
| Compiler worker crashes or is canceled | Candidate abandoned; current/fallback unchanged; compiler memory released |
| Artifact write is short, long, or wrong digest | Candidate rejected before manifest commit |
| Manifest write fails | Objects remain unreachable; no activation |
| Pointer fallback descriptor missing or corrupt while current is healthy | Current continues; one recovery CAS clears only pointer fallback |
| Current descriptor corrupt with verified pointer fallback | One recovery CAS promotes fallback to current and clears corrupt authority |
| Active component corrupt with verified same-component previous | One recovery CAS substitutes only affected mount/runtime, clears damaged history, preserves unrelated mounts |
| Active mount corrupt with no verified same-mount previous | Affected mount becomes unavailable; repaired descriptor continues unaffected mounts with degraded readiness |
| Active Wasm runtime corrupt with no compatible previous | Repaired descriptor retains healthy mounts, encodes `wasm + unavailable`, and render/search fail closed without retaining corrupt runtime as a root |
| Activation authority corrupt/unverifiable | Namespace fails closed; evidence and objects remain untouched |
| Preflight finds missing/corrupt required child | Candidate rejected; current descriptor and all component histories remain serveable |
| Activation CAS loses race | Newer route-set authority preserved; candidate may remain reusable |
| Activation commits but reply is lost | Same mutation ID reconciles exact committed outcome; no duplicate rotation |
| Browser quota exceeded | Candidate rejected with actionable estimate; current cache preserved |
| IndexedDB transaction aborts | Current/fallback pointers unchanged |
| Current local component corrupt | Apply component-scoped recovery once; tombstone corrupt components |
| Search Wasm unavailable | Server fallback online; bounded unavailable state offline |
| Renderer Wasm unavailable offline | Cached full page if present, otherwise explicit offline unavailable response |
| Service Worker update fails | Existing controlling worker and current descriptor continue |
| Native source unavailable on restart | Recovered current descriptor serves; refresh diagnostic observable |
| Cloud artifact store transient failure | No pointer mutation; retry policy owned by caller |

## Security and Privacy Boundary

- Local uploaded bytes never leave the browser unless user later chooses a
  separately authenticated publish action.
- Browser URL fetch omits credentials and rejects non-HTTPS outside localhost.
- Wasm receives only admitted bytes and bounded descriptors.
- Snapshot decoders reject duplicate keys, unknown fields, path traversal,
  invalid UTF-8, digest mismatches, length mismatches, and version mismatch.
- Rendered Markdown and OpenAPI text remain escaped/sanitized under existing
  Manja/Goshtoso policy.
- Service Worker uses explicit route allowlists and cannot intercept upstream
  API requests.
- Local cache identity never contains credentials, tokens, private URLs, or raw
  authorization headers.
- Manja Cloud secrets terminate at cloud composition/source adapters; no public
  open-core storage port accepts raw secret strings.

## Performance and Resource Gates

Hard safety gates:

- native source, snapshot, staging, stored, cache, startup process, response,
  search, and concurrency bounds remain enforced;
- browser source/snapshot limits listed above;
- Wasm execution occurs off main thread;
- one compiler worker and bounded search/render workers;
- query cancellation and latest-query-wins behavior;
- deterministic search work budgets; wall-clock expiry returns no partial result;
- selected rendering loads no unrelated detail shards;
- browser page DOM and response ceilings remain current Kubernetes gates;
- CacheStorage and IndexedDB writes are streamed or chunked without a second
  complete snapshot copy.

Measurement gates, initially non-normative until baselines are recorded:

- compiler Wasm download and initialization;
- local compilation elapsed time and peak browser memory;
- cold and warm search latency;
- selected-route render latency;
- offline first reload;
- static export size;
- native startup RSS after precompiled recovery.

Implementation plan may promote numeric user-experience thresholds only after
recording desktop and constrained-device baselines. It may not relax hard byte,
record, concurrency, and cancellation gates without a spec amendment.

## Conformance and Acceptance

### Cross-target determinism

For identical canonical candidate bytes and compiler options:

- native and browser envelope construction produces identical canonical source
  sets for shuffled inputs, nested refs, redirects, and equivalent acquisition
  locations without persisting acquisition secrets;
- JSON/YAML format detection and canonical media derivation are byte-based and
  identical across targets; acquisition media hints cannot change identity;
- native and Wasm compilers emit the same snapshot ID, manifest bytes, child
  logical identities, search artifacts, and projection bytes;
- mutating any child kind, media type, readiness role, decoder version, or
  compatibility field without changing object bytes changes identity and causes
  old-ID validation to fail;
- native and Wasm search return identical ordered results, scores, and highlight
  ranges for exact, token, fuzzy, typo, Unicode, empty, and maximum-bound queries,
  or the identical deterministic work-budget error;
- native and Wasm renderers expose equivalent semantic selected content, anchors,
  links, headings, response/request structures, and accessible names. HTML bytes
  need not match when presentation runtime identity differs.

### Storage adapter suite

Every adapter must pass reusable tests for:

- idempotent immutable writes and conflicting-byte rejection;
- one physical digest referenced by multiple logical paths, kinds, and media
  types round-trips without physical-metadata conflict;
- route-set/runtime descriptors round-trip by recomputed ID; missing, corrupt,
  conflicting, and wrong-ID descriptors enter the component recovery matrix;
- mount codec vectors distinguish ready with/without a distinct previous and
  unavailable with empty IDs plus one allowed failure code. Empty active on
  ready, equal active/previous, any retained ID on unavailable, missing or
  extraneous failure codes, cross-component failure codes, unknown status, and
  unknown fields all fail before a `RouteSetID` is admitted;
- runtime codec vectors distinguish native ready, Wasm ready with/without
  previous, and Wasm unavailable; every other field combination fails decoding;
- short/long/digest-corrupt stream rejection;
- cancellation during read/write;
- bounded concurrent distinct writes;
- manifest-last visibility;
- publish transitions including `v1, v2, v2` preserve distinct per-component
  previous values and pointer fallback;
- concurrent writers and stale expected state;
- failure after staging one mount, commit-before-reply, exact mutation replay,
  mutation-ID conflict, and restart outcome reconciliation;
- restart after mutation outcomes are pruned recovers current/fallback
  descriptors, all per-component histories, serves every healthy mount, and
  enumerates exact GC roots using only ports;
- recovery matrices assert exact post-state for healthy current/corrupt pointer
  fallback; corrupt current/healthy pointer fallback; corrupt active or previous
  snapshot; corrupt runtime; and no healthy same-component fallback. The
  `A2/A1 + B1/B0` fixture must repair B to B0 or unavailable while preserving
  A2/A1, removing B1 from every GC root, and committing at most one recovery;
- the `A2/A1 + B1/no-B0` fixture must produce the one canonical unavailable B
  state above and the same exact descriptor bytes, `RouteSetID`, tombstone set,
  and GC-root set in filesystem, browser, and cloud test adapters, including
  after restart and mutation-outcome reconciliation;
- runtime recovery matrices assert exact state, readiness, route/search outcome,
  tombstones, GC roots, restart, and CAS-race behavior for native ready, Wasm
  ready, corrupt Wasm with compatible previous, and corrupt Wasm without
  previous;
- restart recovery;
- deterministic corruption versus transient failure;
- GC roots and grace behavior where enumeration is supported.

An in-memory object-store test adapter must model no directories, no rename, and
separate artifact/activation failures. Filesystem-only tests are insufficient
proof of cloud portability.

### Browser acceptance

Automated Chromium tests must prove:

- upload and CORS URL compilation without server upload;
- uploaded and CORS root streams compile solely through seed/resolver ports; short,
  long, digest mismatch, cancellation, read-after-close, and new-stream-for-retry
  behavior is identical across native and Wasm;
- shared source-envelope/resolver vectors for shuffled directories, duplicate and
  case-colliding names, nested relative and cross-origin refs, redirects, root
  self-reference, two-node cycles, non-root back edges, repeated DAG references,
  CORS, token-bearing root URLs, budget crossing, and cancellation;
- shared vectors reverse completion latency and shuffle roots for two references
  to one context, a diamond DAG with descendants, identical bytes under equal
  and unequal contexts, and DAG-plus-cycle graphs. Native and Wasm must emit the
  same occurrence count, logical paths, edge kinds, instance IDs, canonical
  envelope bytes, snapshot ID, and typed budget errors;
- a two-file fan-out fixture with 1,024 references and a diamond-with-descendants
  fixture open and parse each equal context once, charge bytes/documents once,
  stay within edge-work limits, and choose the same creator under reversed
  identification latency;
- fresh-token-for-same-target, token collision/reuse, zero token, unavailable
  interning, equal-token/conflicting-root-descriptor, and unequal-context/equal-
  bytes adapters produce the specified deterministic result or typed failure;
- byte-identical external parents under two acquisition contexts with different
  relative children remain distinct source instances under shuffled/concurrent
  native and browser acquisition, with no context/URL persisted;
- portable browser acquisition accepts zero-hop CORS and maps redirect rejection,
  CORS denial, and network/TLS failure to `fetch_unavailable` without guessing;
  equivalent native portable fixtures map the same cases to that class while
  observable scheme/size/format failures stay distinct;
- compiler worker cancellation and memory release;
- exact native/Wasm snapshot and search parity fixtures;
- Ctrl/Cmd+K, sidebar click, recent visits, highlights, keyboard, focus, and
  accessibility-tree behavior;
- online SSR fallback before local readiness;
- visited and complete offline modes with all network blocked;
- offline reload into an unvisited operation/schema in complete mode;
- a deliberate local/published mount collision cannot shadow the published URL;
  scoped local reload, history, and cleanup remain correct;
- one unchanged snapshot mounted at two published bases and one local activation
  keeps identical bytes/ID while every composed target resolves in its scope;
- crash/reload at Service Worker install, waiting, activation, IndexedDB commit,
  and controller change plus each old/new runtime-bundle transition serves a
  compatible generation or deterministic previous fallback;
- browser runtime corruption with a compatible previous bundle promotes that
  bundle exactly once; without one, restart preserves healthy snapshots,
  exposes `wasm + unavailable`, and returns bounded render/search unavailable
  behavior without retaining the corrupt bundle as a GC root;
- repeated compiles, worker crashes, candidate expiry, runtime upgrades, quota
  denial, storage denial, corruption, stale module, and version mismatch stay
  within aggregate storage caps while current/fallback descriptors and healthy
  per-component histories remain recoverable;
- one combined worst-case sequence publishes three maximum-byte/maximum-entry
  snapshots and three maximum-byte/maximum-entry runtime bundles, installs the
  maximum fixed supervisor/bootstrap set, and fills the allowed metadata set.
  It then reserves one simultaneous maximum snapshot plus runtime candidate.
  Every write/crash boundary and winning/losing CAS outcome proves exact
  post-state roots and GC sets. The fourth candidate reserves in full while
  generations 1-3 remain protected; its successful CAS roots only generations
  2-4, and no active generation is deleted or exceeds the
  432/256/128/16/32 MiB or 16,384-entry caps;
- a maximum-bound candidate reserves from profile/runtime limits before its first
  write, streams once without a manifest, reconciles final actual bytes, and
  releases unused capacity; insufficient capacity causes zero candidate writes;
- 390 px, 768 px, and desktop responsive rendering without primary overflow;
- no management/API/upstream route interception;
- no browser console or page errors.

### Native/static acceptance

- locked Kubernetes native build remains 65 documents, 1,202 operations, 1,826
  schemas, and 3,028 exact-searchable targets;
- mixed multi-catalog build advances successful mounts, carries a failed mount's
  healthy last-known-good snapshot with degraded receipt, and aborts when a
  failed configured mount has no healthy prior state;
- runtime recovery from a precompiled snapshot performs no source read, Git
  command, OpenAPI parse, or compiler call;
- source files may be absent from runtime image;
- static export works from the precompiled snapshot only;
- two clean exports with identical normalized configuration are byte-identical;
  changing each byte-affecting option changes representation identity;
- a plain directory server with JavaScript disabled returns distinct correct
  initial HTML for two selected detail canonical paths;
- representative root, operation, schema, search, source, asset, metadata, and
  offline routes return correct content and cache policy;
- social preview image returns HTTPS 200, correct MIME, 1280×640 dimensions, and
  remains below 1 MiB;
- non-E2E repository tests, integration fixture, race, vet, generation, Muamba,
  webassets, API bundle/lint, root/site builds, and relevant browser gates pass.

## Migration Sequence

1. Freeze current snapshot v1 behavior and add black-box fixture vectors.
2. Introduce artifact, registry, activation, and writer ports with an in-memory
   object-store conformance adapter.
3. Refactor filesystem publication/recovery behind the ports with no route or
   receipt change.
4. Stream compiler output and replace retained child-byte publication.
5. Add native build, recovery-only serve, and static export.
6. Split portable search package from native HTTP adapter; add native/Wasm parity.
7. Add portable selected-route renderer package and renderer Wasm parity.
8. Add compiler Wasm and browser artifact writer.
9. Add browser activation repository, shell, and Service Worker.
10. Add complete offline packaging, corruption recovery, and browser budgets.
11. Run Kubernetes native gates and bounded browser fixtures.
12. Ship local preview behind an explicit experimental flag, then make it the
    default live-demo import path after its acceptance matrix passes.

Each numbered step is independently commit-able and reversible. No step changes
Manja Cloud repositories or deploys a cloud adapter.

## Rejected Alternatives

### Require users to precompile before using the live demo

Rejected. It makes local preview a build-tool demonstration instead of a useful
renderer and prevents private/offline exploration.

### Send raw OpenAPI to JavaScript and implement a second compiler/search stack

Rejected. It duplicates normalization, ranking, highlighting, identity, bounds,
and compatibility behavior.

### Keep compiler Wasm resident for search

Rejected. Parser/compiler memory survives after compilation and couples search
startup to the largest dependency graph. `search.wasm` is separate.

### Treat S3 as a writable filesystem

Rejected. Object storage has no atomic directory rename and must not become
mutable route authority.

### Store all snapshot bytes in a relational database

Rejected as default. Large immutable objects belong in an artifact store; the
database owns small transactional activation and management metadata. An adapter
may use database blobs for a constrained deployment if it still passes ports and
bounds.

### Pre-render every page into the canonical data snapshot

Rejected. It binds presentation code and theme assets into data identity and can
multiply storage for large catalogs. Static export may materialize all pages
under the separate composite presentation identity.

### Reuse the historical Wasm/PWA proof branch

Rejected. It diverges from the accepted catalog renderer and deletes unrelated
current work. Only independently justified behavior and tests may be re-derived
against fresh `origin/main`.

## Definition of Done

This design is ready for implementation planning when:

- two fresh independent artifact-only reviewers accept one frozen Git identity;
- all findings are resolved in this file without transcript-based defenses;
- the design names one source of truth for compiler, renderer, search, snapshot,
  artifacts, and activation;
- filesystem, browser, static, and future cloud semantics are coherent without
  a fake filesystem abstraction;
- local preview requires no server compilation or manual prebuild;
- published and static modes require no browser compilation;
- complete offline navigation is defined for unvisited routes;
- resource, failure, corruption, cancellation, concurrency, identity, caching,
  and recovery contracts are testable;
- Manja Cloud management remains outside open-core scope;
- an implementation plan can split the migration sequence without changing the
  chosen architecture.
