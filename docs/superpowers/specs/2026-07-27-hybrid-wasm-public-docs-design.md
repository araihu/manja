# Hybrid Wasm Public Docs Design

**Status:** Approved architecture, pending written-spec review

**Date:** 2026-07-27

## Context

Manja currently serves public OpenAPI documentation as complete server-rendered
HTML and returns `#main-content` fragments for HTMX navigation. The experimental
Wasm PWA proved offline routing, IndexedDB recovery, conditional freshness, and
canonical UI parity, but it must not be integrated unchanged.

The GitHub REST fixture exposed the important trade-offs:

- cold boot: about 4.03 seconds in the PWA versus 0.25 seconds in SSR;
- warm reload: about 197 milliseconds versus 173 milliseconds;
- operation navigation: about 219 milliseconds versus 272 milliseconds;
- DOM size: 5,919 nodes versus 5,900 nodes;
- PWA build: 27.89 MB, including a 23.76 MB Go Wasm module;
- full SSR HTML: 1.95 MB, dominated by eager navigation and interaction data.

The production design therefore keeps SSR as the immediate, canonical surface
and activates a smaller local fragment renderer in the background. It reuses the
current Manja/Goshtoso workspace; it does not create a second documentation UI.

## Goals

- Keep first render, SEO, copied URLs, no-JavaScript access, and server HTMX
  fragments fully functional without Service Worker or Wasm support.
- Enable the local renderer by default for eligible public publications without
  delaying or hiding SSR content.
- Make all documentation, sidebar sections, and global search usable offline
  after background activation completes.
- Remove OpenAPI parsing and `kin-openapi` from the browser runtime path.
- Reduce initial HTML and live DOM by lazily materializing sidebar sections and
  detail-only interaction data.
- Cache multiple public publications safely: the three most recently used,
  with two validated revisions per publication.
- Activate server-published revisions automatically after validation and keep
  the previous revision for rollback.
- Provide a deployment kill switch that restores pure SSR/HTMX behavior without
  changing publication data.

## Non-goals

- No alternative docs page, visual redesign, marketing surface, or try-it proxy.
- No browser caching of private publications in the first release.
- No client-side OpenAPI parsing on the normal reader path.
- No fixed polling loop for freshness.
- No Service Worker interception of management, API, private-publication, or
  unrelated application routes.
- No TinyGo migration in this slice. Artifact reduction comes first from a
  smaller dependency graph and compressed delivery; TinyGo remains a later
  compatibility investigation.

## Chosen Architecture

### Progressive enhancement

The server remains authoritative and renders the selected documentation page
immediately. The page is usable before any local runtime work starts.

Eligible HTML includes a small, non-secret enhancement descriptor containing:

- a stable public-publication cache key;
- the immutable revision identifier and projection digest;
- same-origin projection, sidebar-section, spec-download, worker, and Wasm URLs;
- the canonical publication base path;
- a projection format version.

The browser enhancer registers the product Service Worker after the document is
interactive. Registration, projection download, cache writes, Wasm download,
and hydration run in the background. Until the worker reports the exact
publication revision ready, existing HTMX requests continue to the server.

Once ready, only allowlisted HTMX fragment requests for that public publication
are routed to Wasm. If routing, hydration, storage, or worker control fails, the
request falls through to the original server URL. Local failure never replaces
usable SSR content with a boot skeleton or error-only page.

### Server-generated projection

The server builds a versioned, deterministic projection from the already parsed
`domain.SpecIndex`. The browser does not receive the parser as part of Wasm.

Projection version 1 contains:

- publication identity, immutable revision identity, and branding;
- overview data and canonical public routes;
- compact operation and schema directories used by sidebar sections;
- operation and schema detail records keyed by canonical anchor;
- the complete compact search index;
- detail-scoped request, response, schema-example, and code-sample inputs.

The projection excludes the original spec download and avoids embedding the
complete OpenAPI JSON into every detail. Original JSON/YAML remains a separate
same-origin resource. It is cached after local navigation is ready so download
support does not delay enhancement activation.

Canonical projection bytes are hashed with SHA-256. The response uses an ETag
whose opaque value is the lowercase hexadecimal digest and content type
`application/vnd.manja.publication+json; version=1`. Projection responses are
limited to 16 MiB, individual generated fragments to 2 MiB, and original spec
downloads to 64 MiB for offline caching. The client verifies format version,
declared identity, response bounds, byte digest, and all record keys before
committing storage or activating Wasm.

The projection builder is a provider-neutral application service. HTTP response
details stay in `internal/web`; serialization and browser-runtime adapters stay
internal. Public `domain` types do not acquire CacheStorage, Service Worker, or
Wasm concerns.

### Main-product packaging

Wasm and enhancement assets ship with the main Manja server and its reproducible
build. There is no standalone `pwa/index.html`, copied `openapi.json`, or separate
documentation server.

Static assets live under a Manja-owned asset prefix. The worker may have root
scope to cover multiple publication paths, but its fetch handler uses exact
same-origin route allowlists from validated descriptors. All other requests pass
through untouched. The worker response sets `Service-Worker-Allowed: /`; no
other asset response may widen its scope.

The Wasm command imports the projection model and canonical public-docs renderer
only. It must not import source adapters, the OpenAPI parser, management wiring,
filesystem stores, or network clients.

### HTTP routes

Each canonical public-publication base exposes these reader resources. A route
join helper maps root publication `/` without producing a double slash:

- `GET <base>/_manja/projection.json` — projection bytes;
- `GET <base>/_manja/sidebar/<section-id>` — one sidebar section;
- `GET <base>/_manja/offline-shell` — canonical cached workspace shell;
- `GET <base>/search.json` — existing server search fallback;
- `GET <base>/openapi.json` — existing original spec download;
- `GET <base>?selected=<anchor>` — existing full page or HTMX detail fragment.

Product runtime assets use `/manja-assets/local-docs/`, including `sw.js`,
`manja.wasm`, its Go runtime, and the small page enhancer. Internal reader
resources do not become public management API contracts.

Normal selected URLs remain canonical and refreshable:

```text
<publication-base>?selected=<anchor>#<anchor>
```

Sidebar group headers are ordinary links to
`<publication-base>?section=<section-id>`. HTMX enhances those links with the
internal sidebar-fragment route, but a no-JavaScript request renders the same
expanded group in the full SSR page.

Server and local renderers accept the same selected anchor and produce the same
`#main-content` contract. HTMX history therefore remains valid with or without
the local runtime.

### Lazy sidebar and search

Full-page SSR renders:

- overview;
- operation/schema group headers and counts;
- the selected item's ancestor group and its items;
- any server-requested expanded section.

Collapsed groups do not contribute thousands of links to the DOM. Expanding a
group requests one sidebar-section fragment. The same URL returns server HTML
before local activation and Wasm HTML afterward. Every item remains a canonical
link, so history, refresh, and no-JavaScript fallback remain valid.

The complete compact search index is present in the projection but not rendered
as hidden result elements. Ctrl/Cmd+K queries the active local index when ready
and the existing server search resource otherwise. Search result IDs remain
distinct from visible content anchors, and every href resolves to a visible
selected section.

Mobile overlay and desktop sidebar use the same section data contract. A section
may appear in only the active presentation container; the implementation must
not duplicate the full menu in hidden desktop/mobile DOM.

## Storage and Lifecycle

### Storage ownership

CacheStorage owns immutable product assets, the validated worker/Wasm response,
the most recent offline shell for each eligible publication, and original spec
downloads cached after activation.

IndexedDB owns projection bytes and metadata. Records are keyed by public
publication cache key and revision digest. Each publication retains active and
previous generations. Global LRU eviction retains the three most recently used
publications.

Storage transitions are journaled and crash-consistent:

1. validate candidate bytes and digest;
2. write the candidate generation;
3. prepare it in Wasm without changing the active router;
4. atomically rotate active to previous and candidate to active;
5. activate the prepared router;
6. update the recovery pointer and LRU metadata;
7. garbage-collect unreachable generations only after a valid commit.

If runtime activation fails after metadata rotation, storage and runtime roll
back to the previous validated generation. If IndexedDB is corrupt, recreate it
once from validated cached projection bytes. Repeated recreation or reload loops
are forbidden.

### Freshness

Revalidation occurs on initial enhancement, browser `online`, and return to a
visible document after a bounded stale interval. All triggers share one in-flight
operation. Requests use the active projection ETag.

- `304`: retain the active generation and refresh observation time.
- Valid changed projection: prepare, commit, and activate automatically.
- Invalid or failed candidate: keep current active generation and server
  fallback; never replace last-known-good docs.
- Offline: use active projection, then previous projection if active is corrupt.

The reader does not ask for a second confirmation. Publication approval already
occurred in Manja's release control plane.

### Offline navigation

Online document requests remain network-first SSR. The worker stores one
canonical offline shell per eligible publication. When document navigation is
offline, it serves that same Manja workspace shell and then resolves the current
URL's selected anchor through the active local projection.

The page may claim “offline ready” only after the shell, projection, runtime,
required static assets, complete search index, and current publication detail
records are validated locally. Original spec download readiness is reported
separately and does not block documentation navigation.

## Eligibility and Configuration

Enhancement is enabled by default only when all conditions hold:

- publication is public and visible to anonymous readers;
- projection identity and revision digest are available;
- request is for the public documentation surface;
- deployment kill switch is not set.

Eligibility is supplied by the publication resolver/composition boundary. It is
never inferred from `SpecIndex` alone, because index data does not prove current
anonymous visibility.

Private or identity-bound publications stay pure SSR/HTMX. Adding them later
requires encrypted-at-rest browser storage, user partitioning, revocation, and
logout deletion tests.

The kill switch is owned by self-hosted composition as `MANJA_LOCAL_DOCS=off`
and is exposed to `internal/web` as a narrow disabled boolean. It removes the
enhancement descriptor and worker registration from rendered HTML. It does not
delete server publication data or change routes.

## Failure and Security Boundaries

- Same-origin GET only; exact publication route allowlist; bounded response size.
- Non-HTMX document and management requests never enter the Wasm router.
- Projection content is untrusted data and is escaped by canonical templ
  components. No `templ.Raw` may contain projection/user content.
- Projection identity, format, ETag, and SHA-256 digest must agree before use.
- Cache keys derive from validated canonical identities, never raw request paths.
- A worker update cannot delete caches for unknown Manja versions or unrelated
  applications.
- Storage quota failure evicts least-recent public publications, then falls back
  to server behavior without damaging active SSR navigation.
- Diagnostics expose bounded codes and timing classes, not raw specs, paths,
  credentials, or browser storage contents.

## Performance Gates

The GitHub REST fixture remains the reference large specification. Product
integration must meet these gates on the project Chromium harness:

- SSR selected content is visible without waiting for Service Worker/Wasm.
- SSR first-content P95 does not regress by more than 10% from the pre-change
  baseline.
- Full selected-operation HTML is at most 250 KB uncompressed.
- Ready selected-operation DOM is at most 2,000 elements at desktop width.
- Raw Wasm is at most 10 MB; compressed transfer is at most 4 MB.
- Background cold activation P95 is at most 1.5 seconds and never blocks input.
- Worker-restart hydration P95 is at most 500 milliseconds.
- Local operation navigation P95 is no slower than server HTMX navigation.
- No task attributable to activation blocks the main thread for over 50 ms.

Artifact, HTML, and DOM limits are hard CI gates. Runtime timing is recorded over
multiple samples with generous regression thresholds to avoid single-sample
flakiness; a material regression blocks rollout even when functional tests pass.

## Testing Strategy

### Go unit and contract tests

- Deterministic projection bytes, digest, version, identity, and size bounds.
- Projection excludes full spec-download bytes and parser-only data.
- Root and nested publication route construction cannot escape publication base.
- Server and Wasm renderers produce equivalent canonical anchors, hrefs,
  headings, and fragment landmarks.
- Default-on eligibility, kill switch, public/private boundary, and pure SSR
  fallback.
- Lazy sidebar section rendering and selected ancestor behavior.

### JavaScript worker/storage tests

- Exact interception allowlist and unconditional pass-through for management,
  private, cross-origin, non-GET, and ordinary document requests.
- Multipublication active/previous storage, LRU three-publication eviction, quota
  recovery, corruption rebuild, rollback, and concurrency serialization.
- Conditional revalidation and automatic valid-revision activation.
- Worker control recovery without reload loops.
- Server fallback for every runtime, storage, digest, and routing failure.

### Browser tests

- SSR content remains usable while worker support is absent, disabled, slow, or
  failed.
- Current UI, theme, mobile overlay, accordions, request examples, canonical
  links, and Ctrl/Cmd+K remain unchanged.
- Sidebar DOM is lazy and does not duplicate complete mobile/desktop menus.
- Three public publications work online and offline; a fourth evicts the oldest.
- New revision activates automatically; corrupt active rolls back to previous.
- Full reload and pasted deep links work online and after offline readiness.
- Private publication and `/manage` produce no persistent docs cache entries.
- Browser console has no errors or HTMX OOB/attribute parsing warnings.

### Benchmark evidence

Every performance run records fixture digest, browser version, sample count,
cold activation, worker restart, reload, local/server navigation, HTML bytes,
projection bytes, raw/compressed Wasm bytes, DOM count, and long tasks.

## Rollout and Reversal

Implementation proceeds in independently reversible slices:

1. deterministic projection and server endpoints, unused by the page;
2. lazy sidebar and client search on normal SSR/HTMX;
3. product-packaged reduced Wasm renderer and parity tests;
4. background Service Worker activation with server fallback;
5. multipublication offline storage and automatic freshness;
6. default-on eligibility for public publications.

Each slice keeps server rendering deployable. The kill switch reverses client
activation immediately. Removing the descriptor returns readers to pure SSR;
stored public cache data becomes unreachable and is deleted by a later bounded
worker cleanup, not during request handling.

## POC Reuse Policy

The experimental branch is not merged or cherry-picked wholesale. Implementation
ports only reviewed behavior, test fixtures, and bounded components after a new
failing test on the integration branch.

Reusable concepts include digest validation, two-generation recovery, serialized
freshness transitions, canonical renderer parity, offline browser scenarios, and
Service Worker controller recovery.

Discarded concepts include the standalone PWA page, global loading skeleton,
copied `openapi.json`, browser OpenAPI parser, eager full sidebar, user-confirmed
revision activation, and 503-only behavior when the local engine is unavailable.

## Acceptance Criteria

- Public docs render and navigate through SSR with JavaScript disabled.
- Eligible public docs enhance automatically without delaying first content.
- Failed/disabled local runtime transparently uses server HTMX.
- After offline-ready, all docs, lazy sidebar sections, and global search work
  across reload and Service Worker restart.
- Three recent public publications and two revisions each recover correctly;
  private publications are never persisted.
- Published revision changes activate automatically and preserve rollback.
- Current Manja/Goshtoso UI and canonical URLs remain unchanged.
- Functional, security, corruption, parity, and performance gates pass.
