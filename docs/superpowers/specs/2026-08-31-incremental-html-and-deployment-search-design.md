# Incremental HTML and Deployment Search Design

**Status:** Approved for phased implementation

**Date:** 2026-08-31

**Compatibility baseline:** Manja v0.1.9 (`fc1cd1ac9f1cc568d75839961967e3464ae8bdad`)

## Context

Manja's v0.1.9 pipeline can publish the real VMware vSphere and Fortinet
corpora, but compilation, search construction, activation, SSR, and static
export still retain or copy representations proportional to the complete
catalog. The Fortinet export proved the consequence: 1,045 documents and
38,812 operations completed only after more than eleven minutes and at roughly
4 GiB maximum RSS. Late failures discard most of that work.

The durable build must instead be incremental, content-addressed, resumable,
and bounded. It produces two primary outputs from one canonical build plan:

1. deterministic HTML pages and portable resource fragments; and
2. a deployment-wide, client-side full-text fuzzy search index.

This design supersedes the shell-only, whole-snapshot rendering and
per-catalog-search assumptions in the 2026-08-24 static-export design for the
new v2 build path. Existing v1 artifacts remain readable during migration.
Current public URLs, v0.1.9 navigation behavior, and verified offline behavior
are compatibility requirements rather than implementation constraints.

## Goals

- Keep peak memory bounded by explicit per-worker and per-shard budgets instead
  of the number of completed documents or resources.
- Reuse unchanged normalized spec projections, HTML artifacts, and search runs
  across interrupted and repeated builds.
- Give every page, fragment, search shard, and sidecar a deterministic path and
  verifiable exact-byte identity.
- Render one minimal path, operation, schema, or example fragment at a time and
  compose nested resources lazily with HTMX.
- Keep the sidebar useful on first render while bounding its initial and live
  DOM with configurable, viewport-driven chunks.
- Make Ctrl/Cmd+K search deployment-wide from the home page, catalog pages, and
  spec pages, with contextual ranking but no implicit scope filter.
- Preserve root and configured-subpath publication, no-JavaScript canonical
  navigation, offline use after readiness, sanitization, and atomic activation.
- Emit phase timing, cache, byte, and budget evidence sufficient to demonstrate
  the design on both one-large-spec and many-spec workloads.

## Non-goals

- No API request proxy, try-it console, authoring UI, or browser-side OpenAPI
  parser.
- No unbounded in-browser index or complete deployment corpus transferred when
  search opens.
- No service-side search dependency for a completed static deployment.
- No requirement to use WebAssembly. A bounded query kernel may move to Wasm
  only after profiling proves that JavaScript is insufficient.
- No silent reinterpretation of v1 snapshot, projection, or search bytes.
- No replacement of content verification with modification times.
- No solution based only on larger resource constants or a larger build host.

## Terms and topology

- A **spec** is one source OpenAPI document with a stable spec key.
- A **declared catalog** is explicitly configured by the user.
- The **default catalog** is Manja's implicit catalog with logical key
  `default`. That key is reserved for the implicit catalog so topology cannot
  be ambiguous.
- A **resource** is a path, operation, schema, or example within a spec.
- A **page** is a complete public HTML document at a canonical route.
- A **fragment** is the smallest portable HTML representation of one resource
  or one navigation chunk.
- An **occurrence** is a resource as reached through one catalog/spec route. A
  spec referenced by multiple declared catalogs may therefore have multiple
  occurrences while reusing the same source and semantic projection.

The topology resolver runs before either output is planned and is authoritative
for HTML and search:

```text
effective catalogs =
    every user-declared catalog, including empty catalogs
    + default, only when one or more specs are not referenced by any declared catalog
```

Consequences:

- A spec referenced by at least one declared catalog is not placed in
  `default`.
- Every unreferenced spec is placed in `default`.
- An empty `default` produces no catalog page, navigation item, manifest entry,
  or search record.
- Every declared catalog is rendered and receives a catalog navigation search
  record even when it contains no specs.
- A declared empty catalog naturally has no spec or resource records.
- Adding the first unreferenced spec materializes `default`; assigning the last
  such spec to a declared catalog removes `default` in the next atomic
  deployment.
- Membership and route occurrences are kept separate from reusable per-spec
  semantic projections. Changing membership need not parse or tokenize the
  source again.

## Architecture

```text
source inventory + catalog declarations
                  |
                  v
      canonical deployment build plan
       (topology, digests, profiles)
                  |
          bounded per-spec workers
                  |
       immutable projection/run cache
          /                       \
         v                         v
 HTML page/fragment pipeline   per-spec search runs
         |                         |
 deterministic HTML +         spill/sort/k-way merge
 adjacent sidecars                 |
          \                       /
           v                     v
      deployment manifest written and verified last
                         |
                atomic publication pointer
```

The build never edits the active publication in place. It resolves cache hits
from immutable prior artifacts or a content-addressed cache, writes misses into
a staging generation, verifies the complete generation, and activates only the
final manifest/pointer. A failed or cancelled build leaves the previous
generation active and preserves completed cache entries for resume.

## Canonical build plan and bounded compilation

The initial plan contains only bounded metadata:

- effective catalog topology and deterministic ordering;
- source identity, byte length, SHA-256, format, and stable spec key;
- normalization identity and options;
- parser, Goshtoso, Manja, compiler, projection, fragment, analyzer, search,
  and partition-format identities;
- deployment base and output-affecting build options;
- explicit worker, decoded-byte, render-byte, spill, shard, and cache budgets.

Timestamps, filesystem paths, Go map iteration order, process IDs, and worker
completion order never enter canonical identities.

Each worker reads and normalizes one spec, parses/projects it, emits immutable
children and a small descriptor, and releases source and parsed representations
before accepting another spec. A per-spec projection cache key is derived from
the source digest, normalization identity, parser/compiler/projection
identities, compatibility profile, and every option that changes semantic
output. Unsupported cycles fail with spec/schema-scoped diagnostics; legal
references are represented by IDs rather than recursively expanded.

An unchanged projection is reused without parsing. A changed spec can still
reuse unchanged resource HTML after its new canonical resource payloads are
compared. Work queues have bounded concurrency and backpressure. A boolean
`ResourceLimits` switch is not an algorithm selector; build and interactive
paths acquire explicit policies and structured budget diagnostics.

## HTML output

### Deterministic layout

Canonical public pages keep the current route contract, including human titles
and stable spec keys in URLs. Internal artifact paths use validated catalog and
spec keys plus a resource key derived from the resource kind and a
domain-separated, length-framed canonical logical identity. Raw OpenAPI path
text is never used directly as a filesystem name.

The precise prefix is versioned, but the v2 shape is:

```text
<catalog-mount>/
  index.html
  documents/<spec-key>/
    index.html
    _manja/fragments/
      paths/<resource-key>.html
      paths/<resource-key>.html.meta.json
      operations/<resource-key>.html
      operations/<resource-key>.html.meta.json
      schemas/<resource-key>.html
      schemas/<resource-key>.html.meta.json
      examples/<resource-key>.html
      examples/<resource-key>.html.meta.json
      sidebar/operations/<chunk-ordinal>.html
      sidebar/operations/<chunk-ordinal>.html.meta.json
```

The publication manifest lists the exact path, kind, length, and SHA-256 of
every file. The adjacent sidecar is part of the fragment contract, not the
publication authority by itself.

### Two-digest fragment contract

Each fragment has two different hashes:

1. The **build key** determines whether rendering is necessary. It is computed
   before templ execution from the canonical render payload and its complete
   output-affecting fingerprint.
2. The **content digest** is SHA-256 of the exact emitted HTML bytes. It proves
   the artifact that will be served.

Rendering HTML merely to decide whether rendering can be skipped defeats the
incremental objective. A candidate cache hit therefore proceeds as follows:

1. derive the current build key from the in-memory canonical resource payload;
2. decode the adjacent sidecar strictly and compare its build key;
3. stream the existing HTML to verify declared length and content digest;
4. reuse it only if every check succeeds; otherwise render a replacement.

The sidecar is canonical JSON with at least:

```json
{
  "schemaVersion": 1,
  "fragmentFormat": "manja-html-fragment-v2",
  "kind": "operation",
  "resourceId": "operation-sha256-...",
  "buildKey": "...",
  "length": 4821,
  "sha256": "..."
}
```

Unknown fields, duplicate fields, a trailing JSON value, identity mismatch,
invalid hex, unexpected kind, wrong length, or changed bytes make the candidate
a miss. Rendering writes through a bounded hashing writer, so the builder does
not retain a second HTML copy. The HTML and sidecar are created in staging and
become visible together only through generation activation.

### Renderer/build fingerprint

The build key covers the exact canonical render payload plus a scoped
renderer/build fingerprint. The fingerprint includes:

- Manja semantic version and immutable build/commit identity;
- fragment wire-format and canonical-payload format versions;
- normalizer, parser, compiler, and projection identities, including Goshtoso
  module/version identity;
- the exact template-source or generated-template digest used by the renderer;
- sanitizer, Markdown, URL, ID, and escaping policy identities;
- relevant CSS, JavaScript, icon, and other UI asset digests;
- output-affecting flags, locale, theme contract, and deployment base when URLs
  in the fragment depend on it;
- sidebar chunk policy only for sidebar artifacts and pages that physically
  compose the first chunk.

This intentionally rebuilds fragments when Goshtoso, Manja, CSS, or another
upstream rendering input changes, even if a particular final HTML value would
happen to remain byte-identical. Fingerprints are scoped so a sidebar chunk-size
change does not invalidate operation, schema, path, example, or search
artifacts.

The canonical resource payload contains every value directly rendered and the
identity of every transitive value summarized by that fragment. A reference
rendered only as a stable link contributes its logical target identity; a
referenced schema summarized inline contributes the summary payload too.

### Portable fragment contract

A resource fragment:

- represents exactly one logical path, operation, schema, or example;
- omits the document shell, `<head>`, global navigation, global styles, and
  inline executable code;
- uses IDs namespaced by catalog occurrence, spec, resource kind, and resource
  identity so independently loaded fragments cannot collide;
- contains escaped/sanitized display text, never trusted projected HTML;
- gives every enhanced link a canonical `href` usable without HTMX;
- references nested resources by deterministic fragment URL rather than
  recursively embedding their complete representations;
- can be inserted into its declared target without knowing which page issued
  the request; and
- stays within the configured fragment byte budget.

An operation selection swaps its operation fragment into the main content
target. Schema, example, and other nested fragments are fetched only when their
control is expanded, selected, or enters the relevant viewport. Inserting one
operation must not eagerly fetch its entire referenced schema closure.

Direct canonical URLs remain usable without JavaScript and reload to visible
selected content. A page may compose a verified fragment during static build or
use the current canonical selection contract; HTMX is progressive enhancement,
not the public URL authority. Search records always link to canonical page URLs,
with fragment URLs carried only as enhancement metadata.

### Sidebar chunks

Operation navigation is the one resource list generated ahead of interaction.
It is partitioned after applying the canonical visible/source ordering into
ordered chunks of at most `sidebarChunkSize` entries. The parameter is a
positive integer and defaults to 12.

The initial sidebar composition exposes the first chunk. Every non-final chunk
ends with exactly one one-shot HTMX intersection sentinel that appends the next
deterministic chunk. Intersection is relative to the sidebar scroll container,
not the browser viewport. If the container has enough visible height, the newly
appended sentinel remains visible and chained requests load multiple chunks
until the next sentinel is out of view or the final chunk is reached.

Each chunk can be requested at most once, and only one request for a given next
chunk may be active. The continuation preserves keyboard order, selected state,
focus visibility, group state, history, and nested scroll behavior established
in v0.1.9. Worker completion order cannot affect chunk membership. A chunk-size
change invalidates only sidebar artifacts and any page that embeds its first
chunk, never resource fragments or search runs.

## Deployment-wide client search

### Interaction and scope

One Ctrl/Cmd+K component is available everywhere:

| Location | Scope | Contextual ranking |
| --- | --- | --- |
| Home/catalog list | All effective catalogs and specs | None |
| Catalog page | All effective catalogs and specs | Modest current-catalog boost |
| Spec page | All effective catalogs and specs | Strong current-spec boost, then modest current-catalog boost |

Context affects ranking only. It never becomes an implicit filter, and an exact
identity/title match elsewhere must outrank a weak fuzzy description match in
the current context. The index includes only content visible in that static
deployment or authorization/publication boundary.

Opening search with an empty query does not enumerate resources or fetch
posting shards. It may show local recent selections and query examples. Results
are relevance-ranked rather than grouped and always identify resource kind,
catalog title, spec title, and HTTP method where applicable.

### Records

The deployment index has five record kinds:

- catalog navigation;
- spec navigation;
- path;
- operation; and
- schema.

Catalog and spec navigation records let a user search for a destination by
name. Catalog/spec titles are metadata on resource records for labeling,
filtering, and context boosts, but are not duplicated into every resource's
full-text field; a catalog-name query must not return every resource in that
catalog.

One resource occurrence receives one stable record ID derived from catalog key,
spec key, resource kind, and canonical logical resource key. Identical schemas
in different specs or catalogs are not collapsed because they have different
URLs and context. The reusable semantic search run remains per spec; topology
expansion adds the lightweight catalog occurrence and route metadata during
deployment finalization.

Searchable fields and baseline weights are:

| Kind | Higher weight | Normal weight |
| --- | --- | --- |
| Catalog | Title/name | Description, if configured |
| Spec | Title/name, version | Description, if present |
| Path | Raw path, summary | Path description |
| Operation | Title, operation ID, method + path | Operation description |
| Schema | Full and short schema name | Schema description |

Examples receive fragments but are not part of the initial search corpus.

### Matching, ranking, and highlighting

The analyzer is versioned and shared by build and client. It applies Unicode
normalization and API-aware tokenization for camelCase, snake_case, HTTP
methods, slashes, braces, dots, and hyphens. The final query term supports
prefix matching for search-as-you-type.

Fuzzy matching is bounded:

- very short terms use exact/prefix matching only;
- medium terms allow edit distance one; and
- long terms allow edit distance two.

Match-quality tiers precede contextual boosts:

1. exact operation ID, schema name, complete path, catalog name, or spec name;
2. exact title/name token or method-plus-path;
3. prefix match;
4. fuzzy token match; and
5. description-only match.

Field weights, then current-spec/current-catalog boosts, then a stable record ID
tie-breaker apply within comparable quality. The exact numeric scoring contract
is versioned and covered by golden relevance cases.

Results contain original display text plus validated scalar/byte ranges for the
matched field. The component escapes text and creates `<mark>` elements from
those ranges; index artifacts never provide trusted HTML. A fuzzy match
highlights the actual matched token. Description snippets are centered on the
best match and remain bounded.

### Static shard architecture

The deployment search index is separate from HTML and from any Wasm binary:

```text
_manja/search/v2/
  directory.json
  vocabulary/<partition>...
  postings/<partition>...
  records/<partition>...
```

The exact encoding may use a compact FST/Levenshtein vocabulary or an equivalent
bounded structure, but the wire contract requires query-routed shards, exact
length/SHA-256 metadata, deterministic ordering, and independent code/data
caching. The browser must not instantiate or download the complete deployment
index to perform a query.

The first implementation uses a Web Worker and JavaScript query core. Pagefind
is the delivery-model reference, MiniSearch the relevance/API reference, and
docfind the compact fuzzy-traversal reference; none is adopted as the complete
engine because none supplies incremental verified static sharding, fuzzy
matching, contextual boosting, and highlighting together. Wasm is permitted
only for the bounded vocabulary/scoring kernel after the same wire format and a
representative benchmark show material benefit.

The client query sequence is:

1. normalize and tokenize the bounded query;
2. fetch and verify only routed vocabulary shards;
3. fetch and verify posting shards for candidate terms;
4. rank bounded candidates with page context;
5. fetch record shards only for top candidates; and
6. return canonical URLs, escaped display fields, and highlight ranges.

Decoded shard caches have explicit byte/count limits and deterministic eviction.
The worker deduplicates in-flight requests, rejects redirects and wrong-origin
URLs, validates length and SHA-256 before promotion, and can reuse verified
offline entries by digest.

### Incremental search build

Each unchanged spec reuses an immutable sorted semantic search run keyed by:

```text
projection digest
+ analyzer/tokenizer version
+ search record format
+ field extraction policy
```

Workers emit bounded sorted runs rather than global maps. When memory reaches
the spill budget, records are sorted canonically and written to disk. A k-way
merge creates deterministic vocabulary, posting, and result-record partitions.
Stable record IDs replace build-order ordinals.

Each final partition's build key is derived from its partition policy and the
ordered input-run digests that route to it. A topology or one-spec change
therefore regenerates only affected occurrence records and partitions. Every
partition is encoded and verified before its directory entry is added; the
search directory is canonical and written last. Reordering worker completion
must produce byte-identical runs, shards, directories, and deployment
manifests.

## Integrity, publication, and runtime

Every source, projection, HTML artifact, sidecar, search run, shard, directory,
and manifest has a content identity. Decoders are strict about version, unknown
or duplicate fields, bounds, canonical order, path ownership, declared length,
SHA-256, and trailing bytes.

The primary static exporter consumes finalized build artifacts directly. It
does not capture one HTTP response per route or regenerate a fragment through a
request-sized helper. HTTP capture remains a parity/smoke adapter. Publication
uses a sibling staging generation and a manifest/pointer swap; a manifest never
references files outside its generation.

The runtime activates an immutable snapshot/deployment handle rather than deep
copying catalog structures. Request admission and browser preparation retain
small descriptors and bounded decoded-shard caches. SSR/API paths window large
lists and keep untrusted interactive request limits even when static build
budgets are larger.

Build logs and receipts report, per phase and when applicable per spec/shard:

- input, output, and spill bytes;
- cache hit/miss/corruption counts;
- parse, projection, fragment, search, verify, and publish durations;
- worker concurrency, queue/backpressure time, and peak live bytes;
- process RSS/heap observations; and
- structured failures with phase, catalog, spec/resource/shard, observed value,
  limit, and remediation.

Logs never include source secrets or unbounded source/description bodies.

## Compatibility and migration

- Preserve current public catalog/spec routes, canonical selection URLs,
  anchors, document keys, human titles, source download links, and base-path
  behavior.
- Preserve v0.1.9 search selection, history, nested scroll, focus, group state,
  selected styling, warm/offline cache, and exact-byte verification contracts.
- Existing v1 snapshots and search artifacts keep their current reader. New v2
  directories use explicit media/schema versions and are never decoded as v1.
- Rollout uses dual read/single write: read active v1 or v2, write v2 only after
  compatibility and scale gates pass, then remove v1 writing in a separately
  reviewed release. Removing the v1 reader requires explicit support-policy
  approval.
- If JavaScript, HTMX, Service Worker, search, or Wasm fails, canonical page
  navigation and already rendered content remain usable. No-JavaScript
  navigation is not expected to provide client-side fuzzy search.
- CSS and renderer changes invalidate HTML through the renderer fingerprint;
  analyzer changes invalidate search runs; neither silently changes old bytes.

## Acceptance gates

### Build and cache

- A clean Fortinet build/export and a clean vSphere build/export succeed within
  declared budgets. Peak live build memory stays bounded as completed spec
  count grows.
- Killing and restarting after completed specs resumes without reparsing or
  rerendering verified unchanged work.
- Repeating an unchanged build records projection, fragment, and search-run
  hits and emits a byte-identical deployment manifest.
- Changing one operation description rebuilds only its affected projection
  outputs, fragment dependency closure, semantic run, and routed search shards.
- Changing Manja, Goshtoso, a relevant template, sanitizer, CSS, or JS digest
  invalidates the intended HTML scope. Changing the analyzer invalidates search
  without reparsing unchanged sources. Changing sidebar chunk size invalidates
  only sidebar/page-composition scope.
- Corrupting an HTML file, sidecar, run, or shard is detected and repaired or
  fails closed; it is never accepted from matching metadata alone.

### Determinism and integrity

- Multiple randomized worker-completion orders produce identical artifact
  paths, bytes, sidecars, directories, and final manifests.
- Every final tree verifies exact files, paths, kinds, lengths, and SHA-256 at
  root and a non-root deployment base.
- A failed build cannot expose a sidecar paired with old HTML, a new directory
  with old shards, or a partially materialized/removed `default` catalog.
- All decoders enforce canonical ordering and resource budgets with fuzz tests.

### HTML and interaction

- Each resource fragment renders independently, has no shell/global executable
  content, has collision-free IDs, and keeps canonical fallback links.
- Selecting an operation fetches only its fragment; expanding a nested schema or
  example fetches only the required additional fragments.
- Sidebar chunks never exceed the configured positive size (default 12), append
  once in canonical order, and chain while the sentinel remains in the sidebar
  viewport. DOM and transfer grow with viewed chunks, not total operations.
- Direct links, reload, back/forward, focus, selected state, nested scroll, and
  offline-ready behavior match the v0.1.9 compatibility floor.

### Search and topology

- Home search reaches all effective catalogs; catalog and spec pages search the
  same deployment corpus with the specified contextual boosts.
- Exact cross-context results outrank weak context-local fuzzy results; catalog
  and spec title queries return navigation records without flooding resource
  matches.
- Typo, prefix, API-tokenization, stable tie-break, and highlight-range goldens
  pass across supported Unicode input.
- Cold and warm queries fetch/decode only routed bounded shards and stay within
  declared transfer, heap, and latency budgets.
- Declared empty catalogs render and are searchable. Empty implicit `default`
  is absent. Unreferenced specs materialize `default`, and topology changes are
  atomic in HTML and search.

## Staged delivery

Delivery follows the matching implementation plan. The required order is:

1. freeze compatibility evidence and add build observability;
2. introduce effective topology, versioned build plans, and artifact storage;
3. make per-spec projection resumable and bounded;
4. emit cached resource fragments and direct page output;
5. add chunked HTMX sidebar composition;
6. emit and merge deployment-wide search v2 runs/shards;
7. activate the client worker, ranking, and highlighting;
8. switch primary export/runtime activation to immutable direct artifacts; and
9. run migration, determinism, offline, and real-corpus acceptance gates.

Every phase is independently mergeable behind a compatibility path. No phase
may remove the previous reader or production path merely because the next phase
is planned.
