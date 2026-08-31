# Incremental HTML and Deployment Search Implementation Plan

> **For agentic workers:** Execute each phase in its own worktree from the then-current `origin/main`. Use subagents only for bounded, independent tasks. Every phase must preserve a green compatibility path and end with focused tests, `git diff --check`, and one or more reviewable commits.

**Goal:** Replace Manja's whole-corpus build/export path with an incremental,
bounded pipeline that emits verified HTML pages/resource fragments and a
deployment-wide client search index.

**Architecture:** Resolve one effective deployment topology, compile and cache
one spec at a time, render one fragment at a time using a pre-render build key
and post-render content digest, externally merge reusable per-spec search runs,
and publish the fully verified generation atomically. Existing v1 readers and
routes remain available throughout migration.

**Design:**
[Incremental HTML and Deployment Search Design](../specs/2026-08-31-incremental-html-and-deployment-search-design.md)

**Baseline:** v0.1.9 (`fc1cd1ac9f1cc568d75839961967e3464ae8bdad`)

**Primary stack:** Go, templ/Goshtoso, HTMX, JavaScript Web Worker, Service
Worker cache, filesystem content-addressed storage, SHA-256, strict canonical
JSON, Playwright.

## Global constraints

- Start each phase from current `origin/main`; verify the predecessor phase is
  integrated before depending on it.
- Use test-first contract changes. Never hand-edit generated `*_templ.go` or
  generated browser bundles.
- Do not load the full catalog, all postings, or all rendered HTML into memory.
- Keep public routes, stable spec keys, human titles, v0.1.9 interaction
  behavior, root/subpath publication, and offline verification compatible.
- Do not remove the v1 reader or primary path until the final migration phase
  has passed all acceptance gates and has separate approval.
- Do not use `ResourceLimits=false` as the new build algorithm. Introduce
  explicit bounded policies and structured diagnostics.
- The active publication is immutable. All output mutation occurs in a staging
  generation or content-addressed cache.
- Exact artifact paths, wire fields, and bounds added by a phase must be locked
  by canonical-byte and malformed-input tests before a later phase consumes
  them.
- Keep full Fortinet/vSphere data out of ordinary unit tests. Check in reduced
  structural fixtures and run full corpora through documented opt-in gates.

## Phase 0: Freeze evidence and add observability

**Purpose:** Establish a measurable compatibility floor before changing wire
formats or algorithms.

**Likely areas:** `application/catalog`, `internal/adapters/catalogstore`,
`internal/selfhosted`, `internal/web`, benchmark fixtures, architecture tests.

- [ ] Record exact v0.1.9 golden identities for representative snapshot,
  projection, search, route, static root, and static subpath artifacts.
- [ ] Add a phase observer that reports inventory, normalize, parse,
  projection, search, serialization, verify, publish, activation, and export
  durations without changing output bytes.
- [ ] Add bounded counters for source/output/spill bytes, worker concurrency,
  cache outcome, queue/backpressure time, and structured budget failures.
- [ ] Add repeatable synthetic tiers for one-large-spec, many-spec, hot-term,
  unchanged-rebuild, and interrupted-build shapes. Add reduced VMware/Fortinet
  structural fixtures with provenance.
- [ ] Measure current clean and unchanged runs. Store commands and results in a
  snag/measurement ledger; do not commit vendor corpora or machine-specific
  absolute paths.
- [ ] Prove instrumentation disabled/enabled produces identical artifact bytes.

**Merge gate:** Existing `go test ./...`, static export verification, browser
compatibility tests, and canonical golden tests pass. The phase lands without a
wire-format or behavior change.

## Phase 1: Effective topology, build profile, and artifact contracts

**Purpose:** Give both outputs one deterministic plan and cache identity model.

**Likely areas:** renderer/config topology, a new application build package,
`application/port`, filesystem build-cache adapter, strict JSON codecs.

- [ ] Add topology tests before implementation for declared empty catalogs,
  unreferenced specs entering implicit `default`, referenced specs excluded from
  `default`, multiple catalog occurrences, removal of empty `default`, and
  canonical catalog/spec order.
- [ ] Reserve and validate the implicit `default` identity. Reject ambiguous
  topology rather than silently merging declarations.
- [ ] Introduce a small versioned `DeploymentPlan` containing effective
  topology, source digests, compiler/render/search identities, base path, and
  explicit budgets. Keep source bytes and parsed models out of it.
- [ ] Replace new-path uses of the resource-limit boolean with named build and
  interactive policies; retain compatibility adapters for existing callers.
- [ ] Define strict canonical codecs for artifact identity, build key, content
  identity, and adjacent HTML sidecar. Cover duplicate/unknown fields, bad
  lengths/hashes, trailing values, path traversal, symlinks, and non-regular
  files.
- [ ] Add a filesystem content-addressed cache/staging interface with atomic
  create, verified reuse, resumable namespace, and no active-tree mutation.
- [ ] Lock domain-separated, length-framed identities with golden vectors.

**Merge gate:** The existing compiler/exporter can consume an adapter derived
from `DeploymentPlan`; production still emits v1. Topology tests prove HTML and
search receive the exact same effective catalog set.

## Phase 2: Bounded, resumable per-spec projection

**Purpose:** Stop retaining source and projection children for the whole catalog
and make an unchanged spec a parse/projection cache hit.

**Likely areas:** `application/catalog/compiler.go`, projection builder and
partitioner, catalog-store publish/activation, new build orchestration and cache
ports.

- [ ] Extract one-spec compilation behind a context-first port. Its input is a
  source reference/digest and build profile; its output is a small descriptor
  plus immutable child references and usage—not a slice of all child bytes.
- [ ] Derive the projection key from source, normalization, parser/compiler,
  projection, compatibility-profile, and output-affecting option identities.
- [ ] Stream source/detail/schema children to staging CAS as they are produced;
  verify before recording a descriptor.
- [ ] Add a bounded worker pool with cancellation, backpressure, and deterministic
  descriptor collection independent of completion order.
- [ ] Add checkpoint/resume tests: kill after selected specs, restart, prove
  completed specs are not opened by parser/projector, and prove a corrupt cache
  entry is rebuilt or rejected.
- [ ] Replace recursively expanded legal schema references with existing
  versioned node/reference identities; keep cycle diagnostics resource-scoped.
- [ ] Assemble the catalog directory and manifest from sorted small descriptors,
  writing them last without first materializing every child.

**Merge gate:** v1-compatible output remains byte-identical for golden fixtures.
Repeated unchanged builds record projection hits. Randomized worker completion
does not alter output. Peak live bytes remain within the declared worker budget
in synthetic many-spec tests.

## Phase 3: Incremental HTML fragment renderer and direct pages

**Purpose:** Introduce the first primary output without rerendering verified
unchanged resources.

**Likely areas:** a new application HTML-build package, `internal/localdocs/render`,
`internal/web/templates`, `internal/selfhosted/export`, artifact codecs/tests.

- [ ] Define canonical render payload DTOs for path, operation, schema, and
  example fragments. Include rendered transitive summaries but only identities
  for link-only dependencies.
- [ ] Add renderer-fingerprint generation from Manja/build identity, Goshtoso,
  templates, sanitizer/Markdown/URL/ID policy, CSS/JS/assets, base path when
  relevant, and format versions. Prove map order, timestamps, and workspace path
  cannot affect it.
- [ ] Write RED tests for the two-digest algorithm: matching build key and exact
  HTML reuses; changed payload/upstream/CSS invalidates; changed bytes with a
  matching sidecar do not reuse; unrelated sidebar policy leaves resource
  fragments valid.
- [ ] Implement pre-render build-key calculation and strict sidecar read. Verify
  candidate HTML by streaming length/SHA-256. Render misses through a bounded
  hashing writer and atomically stage HTML plus canonical sidecar.
- [ ] Split current render preparation into one-resource components. Fragments
  must exclude shells/global scripts/styles, namespace IDs, escape all display
  text, and carry canonical `href` plus deterministic HTMX fragment metadata.
- [ ] Generate deterministic resource keys and paths without placing raw
  OpenAPI paths on disk. Add collision, traversal, Unicode, root/subpath, and
  cross-spec ID tests.
- [ ] Generate canonical home, catalog, and spec pages directly from finalized
  build data. Keep current direct-selection/no-JavaScript routes usable and
  retain HTTP capture as parity tests.
- [ ] Add fragment independence tests that parse each fragment in an isolated
  target and verify no duplicate/global IDs or inline executable content.

**Merge gate:** A feature-gated v2 export emits and verifies resource fragments,
sidecars, and pages directly. Unchanged second build performs no templ render.
Current v1 serving/export remains the default until later acceptance.

## Phase 4: HTMX resource composition and sidebar chunks

**Purpose:** Bound first response and live DOM while preserving the v0.1.9
navigation contract.

**Likely areas:** public templates, catalog handler/fragment routing,
`internal/localdocs/browser`, Manja CSS/JS sources, HTMX/browser tests.

- [ ] Add validated `sidebarChunkSize` build configuration with default 12 and
  positive/bounded validation. Include it only in sidebar/page-composition
  fingerprints.
- [ ] Partition operation navigation after canonical visible/source ordering.
  Emit deterministic zero-padded chunk ordinals and sidecars.
- [ ] Compose the first chunk into the initial sidebar state. Add one one-shot
  continuation sentinel to every non-final chunk, rooted at the sidebar scroll
  container and appending the next chunk.
- [ ] Deduplicate in-flight/repeated next-chunk requests. Allow chained loading
  while each new sentinel remains visible; stop out of view or at the final
  chunk. Do not add a separate manual load button to this contract.
- [ ] Wire operation selection to swap one operation fragment. Fetch schema and
  example fragments only on expansion/selection/relevant intersection, never
  the complete reference closure.
- [ ] Preserve canonical hrefs, history, back/forward, selected state, group
  state, nested scroll, settled focus, and keyboard order in SSR, static,
  offline-ready, HTMX, and no-HTMX cases.
- [ ] Add browser tests for empty, one, 12, 13, and multiple-viewport chunks;
  tall viewport chaining; slow/duplicate responses; deep links; and offline
  cached continuation.

**Merge gate:** Initial HTML and DOM are bounded by page/chunk budgets for the
large-operation fixture. Network assertions prove each interaction fetches only
the selected resource/chunk dependency set.

## Phase 5: Search v2 semantics and bounded engine proof

**Purpose:** Freeze relevance and wire requirements before committing to a
particular compact vocabulary implementation.

**Likely areas:** a new versioned search package, analyzer golden fixtures,
benchmark harness, browser-worker prototype outside active production path.

- [ ] Define stable occurrence IDs and strict DTOs for catalog, spec, path,
  operation, and schema records. Keep catalog/spec labels as metadata on resource
  records, not resource full-text fields.
- [ ] Implement one analyzer used by Go build tests and client conformance
  vectors: Unicode normalization plus camelCase, snake_case, method, slash,
  brace, dot, and hyphen tokenization.
- [ ] Lock fuzzy limits, prefix behavior, match-quality tiers, field weights,
  context boosts, exact-over-context precedence, stable tie-breaks, snippet
  selection, and highlight ranges with golden queries.
- [ ] Build an inactive bakeoff against the reduced and opt-in real corpora:
  current Manja, MiniSearch relevance baseline, Pagefind-style static routing,
  and a small FST/Levenshtein or equivalent vocabulary prototype.
- [ ] Measure index bytes, build peak RSS, cold/warm transfer, browser heap,
  query latency, typo recall/false positives, boost behavior, and highlight
  correctness. Record why the selected bounded structure meets the contract.
- [ ] Keep query data outside the executable. Choose JavaScript Worker first;
  authorize a Wasm kernel only if measured evidence justifies it without changing
  the data/wire contract.

**Merge gate:** Search v2 types/analyzer/scorer are versioned and inactive;
goldens pass identically in Go and JavaScript. The decision ledger identifies
the selected shard structure and fixed benchmark budgets.

## Phase 6: Incremental deployment-wide search builder

**Purpose:** Produce the second primary output without global in-memory maps.

**Likely areas:** `application/catalog/search_model.go`, new search-run/merge
packages, filesystem spill adapter, deployment manifest/export verifier.

- [ ] Emit one canonical semantic run per spec keyed by projection digest,
  analyzer version, record format, and field extraction policy. Do not collapse
  identical schemas across occurrences.
- [ ] Expand catalog occurrence IDs/routes as a lightweight topology stage, so a
  membership change reuses semantic tokenization.
- [ ] Spill canonically sorted exact/vocabulary/posting/record entries at the
  configured memory threshold. Implement bounded k-way merge with stable IDs,
  no global sequential ordinals, and canonical duplicate handling.
- [ ] Fix deterministic routing/partition policy before encoding. Compute each
  output partition key from the policy plus ordered contributing run digests;
  reuse partitions only after exact-byte verification.
- [ ] Emit separate vocabulary, posting, and result-record shards plus a strict
  deployment-level v2 directory written last. Centralize one canonical ordering
  routine for producer, decoder, verifier, and activation tests.
- [ ] Add incremental tests: unchanged deployment, one changed description, one
  added/removed resource, catalog membership change, materialized/removed
  `default`, declared empty catalog, changed analyzer, corrupt spill/run/shard,
  cancellation, and randomized worker/spill order.
- [ ] Add fuzz and property tests for partition routing, k-way merge, strict
  directories, stable tie-breaking, and maximum-byte boundaries.

**Merge gate:** Search artifacts and deployment manifest are byte-identical
across repeated/randomized builds. One-spec changes rewrite only affected runs
and routed partitions. Peak merge memory stays within spill+fan-in budgets.

## Phase 7: Deployment-wide client search interaction

**Purpose:** Expose one global Ctrl/Cmd+K experience at home, catalog, and spec
pages using verified bounded shards.

**Likely areas:** search component templates, source JavaScript and generated
webassets, Service Worker verified cache, static and SSR browser tests.

- [ ] Put the search trigger/component on the home/catalog-list initial state as
  well as catalog/spec pages. Empty query must not fetch posting/record shards
  or enumerate all resources.
- [ ] Pass current catalog/spec identity only as scoring context. Never convert
  page context into an implicit filter.
- [ ] Implement a Web Worker query flow that fetches routed vocabulary shards,
  bounded postings, and only top-candidate record shards. Deduplicate requests,
  reject redirects/wrong origins, verify length/SHA-256 before decode/promotion,
  and bound decoded caches.
- [ ] Render relevance-ranked results with kind, method, catalog, spec,
  canonical URL, centered snippet, and `<mark>` nodes constructed from validated
  ranges after text escaping. Never accept result HTML from an artifact.
- [ ] Add catalog/spec navigation results, including declared empty catalogs.
  Verify catalog-name queries do not flood results with every child resource.
- [ ] Enhance canonical result links with fragment swaps when the destination
  can be resolved locally; preserve full navigation/no-JavaScript fallback.
- [ ] Add browser tests for home global results, cross-catalog exact match,
  catalog/spec boosts, exact-over-weak-fuzzy precedence, keyboard interaction,
  history/focus, root/subpath URLs, corrupt shards, offline warm queries, cache
  eviction, and query cancellation/races.

**Merge gate:** Search remains within fixed clean/warm transfer, latency, and
heap budgets on reduced fixtures and the opt-in real corpora. Disabling the
enhancer leaves normal public navigation intact.

## Phase 8: Direct export, immutable activation, and v2 default path

**Purpose:** Remove route-by-route HTTP capture and deep snapshot copies from
the production data plane.

**Likely areas:** `internal/selfhosted/export.go`, catalog-store
publish/activation, `application/catalog/runtime.go`, local-docs activation and
Service Worker.

- [ ] Make the primary exporter consume the verified finalized generation and
  manifest-driven route list directly. Keep `captureHTTP` only for parity/smoke
  assertions and useful bounded diagnostics.
- [ ] Verify every page, fragment, sidecar, asset, search shard, directory, and
  source child before writing the deployment manifest. Reject unknown extras,
  missing entries, symlinks, devices, path escapes, and changed bytes.
- [ ] Atomically publish a sibling staging generation. Prove failures cannot
  expose mismatched HTML/sidecars, search directory/shards, or mixed catalog
  topology.
- [ ] Replace runtime deep clones with immutable snapshot/deployment handles and
  bounded decoded detail/shard caches. Keep last-known-good activation on source,
  parse, verify, or publish failure.
- [ ] Update offline admission to cache artifacts by verified digest, load only
  route/query dependencies, instantiate Wasm at most once when present, and
  avoid sending an accumulated child map through the bridge.
- [ ] Run route parity between direct output and HTTP smoke rendering for every
  canonical route/selection in fixtures.
- [ ] Change the feature gate/default only after all earlier acceptance gates
  pass; retain an explicit rollback to the v1 reader/path for the support window.

**Merge gate:** Static export performs no per-child HTTP capture in its primary
path. Server/static activation retains immutable handles and bounded caches.
`manja export verify` passes at `/` and a configured subpath.

## Phase 9: Migration and real-corpus acceptance

**Purpose:** Prove the architecture on the workloads that motivated it and
complete a reversible rollout.

- [ ] Run clean, unchanged, one-spec-changed, CSS/Goshtoso-changed,
  analyzer-changed, interrupted/resumed, and cache-corruption matrices on the
  vSphere and Fortinet corpora.
- [ ] Record wall time, peak RSS/heap, live-byte budget, spill bytes, output
  bytes/files, cache-hit ratios, rewritten partitions/fragments, and exact final
  manifest identities.
- [ ] Run determinism builds with randomized worker completion, spill size, and
  merge fan-in; output identities must remain identical.
- [ ] Run root/subpath export verify, all canonical/deep links, no-JavaScript,
  HTMX, online cold/warm, changed revision, offline reload, sidebar chaining,
  and deployment-wide search browser matrices.
- [ ] Confirm declared-empty/default topology transitions are atomic and search
  visibility equals rendered visibility.
- [ ] Compare all v0.1.9 compatibility goldens and document intentional v2-only
  byte/layout changes. No route, title, focus/history, selected-state, or
  verified-cache regression is accepted.
- [ ] Publish a migration receipt with v1-reader support duration and rollback
  procedure. Removing v1 writes/readers is a later, separately approved change.

**Release gate:** The real corpora pass every design acceptance gate; peak live
memory does not grow linearly with completed specs; unchanged/restarted builds
reuse verified work; client interactions transfer only required shards; and the
v1 compatibility/rollback path remains tested.

## Standard checks for every phase

Run the focused tests named by the phase, then the repository-relevant subset
of:

```bash
go tool -modfile=tools/go.mod muamba verify --strict
go tool -modfile=tools/go.mod muamba generate-go --strict --check --dir internal/webassets --output muamba_gen.go
go run ./cmd/webassets check
go run -modfile=tools/go.mod github.com/a-h/templ/cmd/templ generate
go test ./...
git diff --check
```

Run static browser tests for phases 4, 7, 8, and 9; integration tests when a
phase changes source/store behavior; root and nested consumer tests when public
types change. Regenerate files only through their documented generators and
commit source plus generated output together.
