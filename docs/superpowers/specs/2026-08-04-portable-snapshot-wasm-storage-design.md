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
  new ports without weakening crash recovery, active/previous fallback, CAS, or
  garbage-collection protections;
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
format. This spec must be updated to the merged `origin/main` base before an
implementation plan is approved.

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
- every logical child path, kind, uncompressed length, and SHA-256 digest.

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

The canonical manifest is itself an immutable artifact. For each child it
declares:

- logical path within the snapshot;
- semantic kind and media type;
- exact uncompressed length and SHA-256 digest;
- whether the child is required for base activation, search, selected rendering,
  source download, or full-offline readiness;
- format version required to decode it.

Logical paths are presentation and routing names only. Storage adapters key
physical objects by content digest. A manifest may reference one object from
multiple logical paths without duplicating bytes.

Canonical JSON follows the existing strict codec rules: sorted slices, fixed
struct field order, valid UTF-8, no duplicate keys, no unknown fields, no trailing
values, compact encoding, no timestamps, and no map iteration in identity bytes.

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
    Kind        ArtifactKind
    MediaType   string
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

### Activation repository

Mutable route authority is separate from artifact bytes:

```go
type ActivationKey struct {
    Namespace string // opaque composition-owned scope
    Mount     string
}

type ActivationState struct {
    Generation uint64
    Active     SnapshotID
    Previous   SnapshotID
}

type ActivationRepository interface {
    Get(context.Context, ActivationKey) (ActivationState, error)
    CompareAndSwap(
        context.Context,
        ActivationKey,
        ActivationState, // exact expected state
        ActivationState, // desired state
    ) error
}
```

Rules:

- only fully preflighted snapshots may become active;
- same-snapshot activation is an idempotent no-op preserving distinct previous;
- a changed activation shifts active to previous exactly once;
- CAS mismatch never overwrites newer authority;
- deterministic corruption does not erase active/previous state;
- transient failures remain retryable and do not permanently disable a snapshot;
- a successful call is durable according to adapter contract;
- namespace is opaque to open core. Manja Cloud may derive it from tenant and
  environment identity without adding tenant types to `domain`.

Filesystem uses the existing journal and atomic replace behind this interface.
Browser uses one IndexedDB transaction. Future cloud uses a relational transaction
or conditional database update. S3 object metadata is not activation authority.

### References and garbage collection

Serving correctness cannot depend on immediate deletion. Artifact objects are
immutable and may be shared across snapshots.

Garbage collection is a separate application service over a repository-specific
enumeration capability. It computes roots from active, previous, admitted
candidate, retained static-export, and explicit lease records. It deletes only
objects unreachable for a configured grace period. Failure leaks bounded storage;
it cannot break active snapshots.

The next slice retains current filesystem budgets and browser LRU budgets. Cloud
retention, legal hold, billing, and cross-tenant deduplication policy remain
Manja Cloud concerns.

## Adapter Mapping

| Mode | Artifact store | Snapshot registry | Activation repository |
|---|---|---|---|
| Local native | filesystem objects | filesystem manifests | durable journal and route table |
| Static export | output directory | exported manifest | none |
| Browser preview | CacheStorage | IndexedDB manifest metadata | IndexedDB transaction |
| Future Manja Cloud | S3-compatible objects | database row plus manifest object | PostgreSQL/DynamoDB CAS |

`io/fs` may remain a read-only adapter for embedded assets. It is insufficient
for streaming verified writes, conditional activation, multipart transfer,
garbage collection, or browser transactions.

## Native Commands

### `manja build`

```text
manja build \
  --renderer-config renderer.yaml \
  --snapshot-store ./manja-snapshots
```

Loads configured file/Git sources, compiles all catalogs, writes complete
snapshots, preflights them, and atomically updates the local activation repository.
No HTTP listener starts. Output includes bounded machine-readable receipts.

### `manja serve`

```text
manja serve \
  --renderer-config renderer.yaml \
  --snapshot-store ./manja-snapshots \
  --refresh=never
```

Recovers active/previous snapshots and serves them without loading source bytes
or constructing compiler/parser instances. `--refresh=never` is the live-demo
and static runtime contract. Other deployments may use `--refresh=startup` or an
explicit future sync command.

If no recoverable active snapshot exists, startup fails readiness. It never
silently compiles when refresh is disabled.

### `manja export`

```text
manja export \
  --snapshot-store ./manja-snapshots \
  --catalog kubernetes \
  --output ./public
```

Materializes canonical index/detail URLs, source downloads, search artifacts,
assets, metadata, and an offline manifest. Output contains no mutable activation
state. Two exports of the same snapshot and presentation runtime are byte-identical.

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
8. The application reports whether visited-only or complete offline readiness is
   available.

There is no manual “compile then upload” step.

### Input policy

- File upload is always local and never sent to Manja servers.
- URL import uses browser Fetch with `credentials: omit` and requires a
  CORS-readable response.
- Redirect count, final scheme, bytes, decompression, documents, references,
  depth, diagnostics, and compile time are bounded.
- Only `https:` URLs are accepted outside localhost development.
- Cross-origin references obey the same CORS and credential policy.
- The open-core demo does not proxy blocked URLs. Manja Cloud may later offer an
  authenticated, audited import service under its own policy.
- Cancellation terminates the compiler worker and discards its candidate
  activation. Previously active local snapshots remain available.

### Browser compilation profiles

Browser defaults are intentionally lower than native publisher limits. Initial
v1 gates:

- 16 MiB aggregate decoded source bytes;
- 8 MiB per document;
- 64 documents;
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

## Wasm Module Boundaries

### `compiler.wasm`

Contains parser, compatibility validation, compiler, partitioner, and search-
index builder. It imports no DOM, Service Worker, filesystem, Git, management,
or HTTP server package. JavaScript supplies source bytes and a browser artifact
writer bridge.

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
  "href": "/documents/core-v1/?selected=...#...",
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
time, and cancellation remain bounded.

Native server fallback calls the same Go query package. Given the same snapshot,
query, and search-contract identity, browser and server return identical ordered
IDs, scores, and match ranges.

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
- IndexedDB owns snapshot manifests, active/previous/candidate pointers, LRU,
  readiness, and corruption tombstones.
- `localStorage` owns none of the snapshot or activation state.

### Readiness levels

- **Online only:** no validated local snapshot.
- **Visited offline:** shell, runtime, search, directory, and visited selected
  shards are cached.
- **Complete offline:** every manifest child marked `fullOfflineRequired` is
  verified locally. Every operation/schema route and search result can open with
  network blocked.

Complete offline is explicit for published large catalogs. Local preview may
default to complete readiness when its admitted snapshot is below 32 MiB.

### Atomic browser activation

1. write immutable candidate artifacts into a versioned cache;
2. write and validate candidate manifest metadata;
3. instantiate matching renderer/search modules and perform a smoke query/render;
4. one IndexedDB transaction rotates active to previous and candidate to active;
5. notify controlled pages of the new active snapshot;
6. delete unreachable cache generations only after commit and grace period.

Crash before step 4 leaves active unchanged. Crash after step 4 recovers the new
active pointer. Active corruption falls back to previous once and records a
tombstone. A transient timeout does not permanently poison valid bytes.

### Service Worker boundary

The Service Worker intercepts only exact same-origin Manja reader routes and
asset prefixes declared by a validated descriptor. Management, APIs, arbitrary
fetches, cross-origin requests, and upstream API traffic always pass through.

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
2. upload immutable manifest;
3. preflight candidate through cloud reader;
4. database transaction/CAS publishes active snapshot;
5. renderer nodes observe the new generation;
6. asynchronous GC handles unreachable objects after grace.

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
- active/previous preservation;
- no reliance on filesystem path, rename, lock file, or process-local mutex.

## Failure and Recovery Matrix

| Failure | Required outcome |
|---|---|
| Compiler worker crashes or is canceled | Candidate abandoned; active snapshot unchanged; compiler memory released |
| Artifact write is short, long, or wrong digest | Candidate rejected before manifest commit |
| Manifest write fails | Objects remain unreachable; no activation |
| Preflight finds missing/corrupt required child | Candidate rejected; active/previous remain serveable |
| Activation CAS loses race | Newer authority preserved; candidate may remain reusable |
| Browser quota exceeded | Candidate rejected with actionable estimate; active cache preserved |
| IndexedDB transaction aborts | Active pointer unchanged |
| Active local snapshot corrupt | Promote verified previous once; tombstone corrupt active |
| Search Wasm unavailable | Server fallback online; bounded unavailable state offline |
| Renderer Wasm unavailable offline | Cached full page if present, otherwise explicit offline unavailable response |
| Service Worker update fails | Existing active worker/snapshot continues |
| Native source unavailable on restart | Recovered active snapshot serves; refresh diagnostic observable |
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

- native and Wasm compilers emit the same snapshot ID, manifest bytes, child
  logical identities, search artifacts, and projection bytes;
- native and Wasm search return identical ordered results, scores, and highlight
  ranges for exact, token, fuzzy, typo, Unicode, empty, and maximum-bound queries;
- native and Wasm renderers expose equivalent semantic selected content, anchors,
  links, headings, response/request structures, and accessible names. HTML bytes
  need not match when presentation runtime identity differs.

### Storage adapter suite

Every adapter must pass reusable tests for:

- idempotent immutable writes and conflicting-byte rejection;
- short/long/digest-corrupt stream rejection;
- cancellation during read/write;
- bounded concurrent distinct writes;
- manifest-last visibility;
- active/previous CAS transitions including `v1, v2, v2`;
- concurrent writers and stale expected state;
- restart recovery;
- deterministic corruption versus transient failure;
- GC roots and grace behavior where enumeration is supported.

An in-memory object-store test adapter must model no directories, no rename, and
separate artifact/activation failures. Filesystem-only tests are insufficient
proof of cloud portability.

### Browser acceptance

Automated Chromium tests must prove:

- upload and CORS URL compilation without server upload;
- compiler worker cancellation and memory release;
- exact native/Wasm snapshot and search parity fixtures;
- Ctrl/Cmd+K, sidebar click, recent visits, highlights, keyboard, focus, and
  accessibility-tree behavior;
- online SSR fallback before local readiness;
- visited and complete offline modes with all network blocked;
- offline reload into an unvisited operation/schema in complete mode;
- crash/reload at each browser activation transition;
- quota denial, storage denial, corruption, stale module, and version mismatch;
- 390 px, 768 px, and desktop responsive rendering without primary overflow;
- no management/API/upstream route interception;
- no browser console or page errors.

### Native/static acceptance

- locked Kubernetes native build remains 65 documents, 1,202 operations, 1,826
  schemas, and 3,028 exact-searchable targets;
- runtime recovery from a precompiled snapshot performs no source read, Git
  command, OpenAPI parse, or compiler call;
- source files may be absent from runtime image;
- static export works from the precompiled snapshot only;
- two clean exports are byte-identical;
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
