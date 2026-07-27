# Hybrid Wasm Public Docs Design

**Status:** Approved design; implementation plan deferred by Integration DAG

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

## Integration DAG and Base Authority

This design-only branch starts at `origin/main` commit
`6216bcb2d9a50083b130daa37cf381c6e2b4b601`. That base is sufficient to review
the architecture, but it is not authorized as the implementation base for the
complete hybrid feature.

Writing the implementation plan or product code must wait until the control
plane verifies all of these prerequisites on then-current `origin/main`:

1. Goshtoso `v0.0.13` and its public runtime/fallback asset contract;
2. the application-structure public-docs UI checkpoint;
3. release-tracks and authenticated-previews integration, including the
   authoritative public/private publication resolver used for eligibility.

After those commits land, create a fresh implementation worktree and branch from
that exact `origin/main`. Carry this reviewed spec forward as documentation, but
do not rebase, merge, or build the complete feature on stale `6216bcb`. The
implementation plan must record the new base SHA and literal prerequisite SHAs
before its first task.

An independent projection-only precursor is permitted earlier only through its
own spec, plan, worktree, and review. It may depend solely on `domain.SpecIndex`
and deterministic serialization; it cannot touch Goshtoso assets, publication
eligibility, lazy UI, Service Worker, offline storage, or default-on rollout.
This design does not currently split out that precursor.

The product dependency order is therefore: prerequisite integrations, fresh
implementation base, projection contract, lazy SSR/HTMX UI, integrated runtime
assets, Wasm renderer, Service Worker/storage, then default-on eligibility.

## Chosen Architecture

### Progressive enhancement

The server remains authoritative and renders the selected documentation page
immediately. The page is usable before any local runtime work starts.

Eligible HTML includes a small, non-secret enhancement descriptor containing:

- a stable public publication cache key;
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

The wire model uses explicit DTO structs and slices only. It has no map-valued
fields and no generic JSON objects. Records use this stable order before
encoding:

- sidebar sections by kind, then canonical section ID;
- operations and operation details by canonical anchor;
- schemas and schema details by canonical anchor;
- search records by stable search ID;
- public routes by canonical path, then title;
- nested parameters, responses, media types, properties, examples, and code
  samples by explicit ordinal followed by their canonical ID.

When source order affects presentation, the builder assigns the ordinal before
sorting and transmits it as a non-negative integer. Decimal OpenAPI values use
canonical decimal strings; the wire model has no floating-point fields. Bounded
example/config JSON is validated separately and encoded as a JSON string, never
inserted as an arbitrary object. Operation-scoped inputs are limited to 256 KiB
per record and schema-scoped inputs to 512 KiB per record.

Canonical encoding is Go `encoding/json.Marshal` over the versioned DTO:

- struct declaration fixes object field order;
- input must already be valid UTF-8; replacement of invalid bytes is forbidden;
- standard Go JSON escaping applies, including HTML-sensitive and U+2028/U+2029
  escapes;
- integers use base-10 JSON number syntax with no leading zero;
- output is compact and has no trailing newline or insignificant whitespace;
- a strict token pass rejects duplicate object keys at every depth, unknown DTO
  fields, trailing values, invalid UTF-8, and out-of-range integers before any
  DTO decode or cache write.

Canonical projection bytes are hashed with SHA-256. The response uses an ETag
whose opaque value is the lowercase hexadecimal digest and content type
`application/vnd.manja.publication+json; version=1`. Projection responses are
limited to 16 MiB, individual generated fragments to 2 MiB, and original spec
downloads to 64 MiB for offline caching. The digest covers the exact uncompressed
HTTP entity bytes before content coding; browser verification uses the decoded
response bytes returned by Fetch. The client verifies format version, declared
identity, response bounds, duplicate-key rules, byte digest, and all record keys
before committing storage or activating Wasm.

`SpecDownload.JSON` and the complete `ExampleSpecJSON` are explicitly excluded.
The builder extracts only the bounded detail-scoped inputs named above. A test
with sentinel bytes in both excluded fields must prove they do not occur in the
projection or affect its digest.

The projection builder is a provider-neutral application service. HTTP response
details stay in `internal/web`; serialization and browser-runtime adapters stay
internal. Public `domain` types do not acquire CacheStorage, Service Worker, or
Wasm concerns.

### Main-product packaging

Wasm and enhancement assets ship with the main Manja server and its reproducible
build. There is no standalone `pwa/index.html`, copied `openapi.json`, or separate
documentation server.

Static assets live under a Manja-owned asset prefix. The worker has root scope
to cover multiple publication paths, but its fetch handler uses exact
same-origin route allowlists from validated descriptors. All other requests pass
through untouched. The worker response sets `Service-Worker-Allowed: /`; no
other asset response may widen its scope.

The Wasm command imports the projection model and canonical public-docs renderer
only. It must not import source adapters, the OpenAPI parser, management wiring,
filesystem stores, or network clients.

### Offline runtime dependency contract

Offline readiness is bound to the integrated Goshtoso `v0.0.13` dependency
contract, not to paths copied from the Go module cache. Manja may consume only
Goshtoso's public `assets.Handler()` and public dependency/URL exports. If
`v0.0.13` does not publicly expose enough information to build a same-version
fallback manifest, implementation stops and records a Goshtoso snag; it must not
import an `internal` package or scrape private generated files.

The enhancement descriptor identifies the exact Goshtoso version and ordered
embedded fallback manifest used by the rendered page. The offline shell preserves
this document execution order:

1. Goshtoso compiled CSS;
2. Alpine collapse plugin;
3. Alpine focus plugin;
4. Alpine Mask plugin;
5. Alpine core;
6. HTMX;
7. combobox navigation;
8. Manja CSS, schema-example, request-composer, and page enhancer assets.

The worker runtime additionally caches the matching Go runtime before loading
the Manja Wasm module. Every Goshtoso fallback response must come from the same
version's `assets.Handler()`. A successful CDN response may improve online load,
but CDN availability, DNS, or cache state is never an offline-readiness input.

The offline shell body and its security headers are cached atomically. If the
integrated layout uses a CSP nonce, every cached inline nonce must match the
cached `Content-Security-Policy` header; the worker serves both unchanged and
never rewrites shell HTML. New enhancement code uses external scripts; local
routing does not add inline executable content or weaken the integrated CSP.

Offline-ready is withheld until all fallback responses above, the offline shell,
Go runtime, enhancer, and Wasm validate. Browser acceptance blocks every CDN
request, proves fallback activation, then disconnects all network and repeats
reload, search, sidebar expansion, and detail navigation.

### HTTP routes

Each canonical public publication base exposes these reader resources. A route
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

Mobile overlay and desktop sidebar use the same section data contract. A
materialized section exists in exactly one presentation container selected for
the current viewport; only compact group headers may be duplicated. Hidden
desktop/mobile DOM never contains a second complete menu.

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
visible document when the last authoritative observation is older than five
minutes. All triggers share one in-flight operation. Requests use the active
projection ETag.

- `304`: retain the active generation and refresh observation time.
- Valid changed projection: prepare, commit, and activate automatically.
- Invalid or failed candidate: keep current active generation and server
  fallback; never replace last-known-good docs.
- Offline: use active projection, then previous projection if active is corrupt.

The reader does not ask for a second confirmation. Publication approval already
occurred in Manja's release control plane.

### Visibility withdrawal and deletion

These online observations authoritatively disable a known cached publication:

- `401`, `403`, `404`, or `410` from its projection, offline-shell, or original
  spec resource;
- `401`, `403`, or `410` from its canonical document route;
- `404` from its canonical document route when the response identifies the
  publication as missing;
- a successful canonical document response carrying
  `X-Manja-Publication-State: private`, `revoked`, `deleted`, or `disabled`;
- removal of the enhancement descriptor paired with that state header.

A selected-detail `404` without the publication-state header means only an
unknown anchor and does not revoke the publication cache. Updated Manja servers
always emit `X-Manja-Publication-State` on controlled public-document responses,
including SSR pages that no longer contain an enhancement descriptor.

On the first authoritative withdrawal observation, the worker must:

1. disable local routing for the publication in memory before awaiting I/O;
2. abort in-flight preparation/revalidation and pass the authoritative server
   response through;
3. persist a disabled tombstone so cached bytes cannot be served even if deletion
   fails or the browser immediately goes offline;
4. reject every new projection, spec, shell, fragment, or private-successor write;
5. notify controlled clients to keep pure SSR/HTMX behavior;
6. best-effort delete active, previous, candidate, offline-shell, spec-download,
   recovery-pointer, and LRU records for that publication only.

Re-enablement requires a later authoritative public descriptor with a new or
explicitly reauthorized immutable revision. It performs a fresh projection
validation; it never resurrects tombstoned bytes.

This design does not claim cryptographic recall. Bytes legitimately delivered
while a publication was public may have been copied outside Manja, and a browser
that never reconnects can keep reading its already active public cache until
eviction or deletion. After any authoritative online withdrawal observation,
the persisted tombstone prevents further local disclosure. Product v1 accepts
this boundary and does not impose a short offline lease that would weaken the
offline guarantee.

### Offline navigation

Online document requests remain network-first SSR. The worker stores one
canonical offline shell per eligible publication. When document navigation is
offline, it serves that same Manja workspace shell and then resolves the current
URL's selected anchor through the active local projection.

The page must not claim “offline ready” until the shell, projection, runtime,
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

One committed Playwright-Go harness,
`internal/web/e2e/local_docs_performance_test.go`, measures baseline and candidate
with the same Go/Node/Chromium toolchains and runner. It writes the machine-
readable receipt
`docs/superpowers/evidence/local-docs-performance.json`. A run without its receipt
or with an uncommitted harness is not release evidence.

The frozen historical baseline is
`6216bcb2d9a50083b130daa37cf381c6e2b4b601`. The implementation receipt also
records the fresh post-prerequisite base SHA required by the integration DAG and
runs that base immediately before the candidate on the same machine. Candidate
SSR regression is evaluated against this immediate base; the historical result
remains visible for drift. Both SHAs and every prerequisite SHA are literal
receipt fields.

The reference fixture is
`internal/adapters/openapi/testdata/github-v3-rest.json`, SHA-256
`dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977`.
The exact benchmark URL is:

```text
/?selected=operation-enterprise-admin-delete-global-webhook#operation-enterprise-admin-delete-global-webhook
```

The harness uses a production-built Manja binary, loopback HTTP without Air,
Chromium headless at 1440x900 CSS pixels, device scale factor 1, default CPU,
no network throttling, and reduced-motion media. OS, CPU model, core count,
memory, Go version, Node version, Chromium version, and Manja build SHAs are
recorded.

For each baseline/candidate metric, the harness discards three warm-up samples
and records 30 samples. P95 uses nearest rank: sort ascending and select the
one-based element `ceil(0.95 * n)`. Mean, median, P95, minimum, maximum, and raw
sample values are stored. Baseline and candidate groups alternate order to limit
thermal/order bias.

Cold samples use a new browser context and origin per sample, with no Service
Worker registration, CacheStorage, IndexedDB, HTTP cache, localStorage, or
sessionStorage carried from another sample. Warm navigation samples begin only
after one publication is local-ready; three complete unrecorded round trips
between `delete-global-webhook` and `get-global-webhook` precede 30 alternating
recorded navigations.

“First content” is elapsed navigation-start to the exact selected anchor
`#operation-enterprise-admin-delete-global-webhook` becoming visible in the SSR
document. It must occur without waiting for `manja:local-ready`. Full HTML size
is the decoded, uncompressed byte length of the complete HTTP 200 document body
for the exact URL, not an HTMX fragment. DOM count occurs after selected content
is visible and, for the candidate, after `manja:local-ready`, before opening
search, expanding another sidebar section, or changing viewport.

Cold activation is measured from `manja:activation-start` to
`manja:local-ready` in a cold context. Worker-restart hydration first establishes
offline-ready, terminates the active worker through the harness, reloads the
exact URL with retained origin storage, then measures the same marks until the
replacement worker reports the same publication digest ready.

The harness installs a `PerformanceObserver` before navigation. It records long
tasks overlapping the interval from `manja:activation-start` through one second
after `manja:local-ready`; unsupported Long Tasks API is a failed receipt, not a
zero. No overlapping entry may exceed 50 milliseconds.

Wasm size gates use the raw `manja.wasm` and a reproducible precompressed
`manja.wasm.br` generated with Brotli quality 11 and window 22. The HTTP test
sends `Accept-Encoding: br`, requires `Content-Encoding: br`, and records encoded
and decoded sizes. Gzip may also ship but is not the 4 MB gate measurement.

Product integration must meet all gates:

- SSR first-content P95 with default enhancement does not regress by more than
  10% from the immediate post-prerequisite base.
- Complete decoded selected-operation HTML is at most 250 KiB.
- Selected-operation DOM is at most 2,000 elements at the defined count point.
- Raw Wasm is at most 10 MiB; Brotli artifact is at most 4 MiB.
- Background cold activation P95 is at most 1.5 seconds and never blocks input.
- Worker-restart hydration P95 is at most 500 milliseconds.
- Local operation navigation P95 is no slower than server HTMX navigation.
- The defined activation window contains no long task over 50 milliseconds.

Artifact, HTML, DOM, sample protocol, and receipt-schema checks are hard CI
gates. Runtime comparisons run on one runner job against its freshly built base;
a material regression blocks rollout even when functional tests pass.

## Testing Strategy

### Go unit and contract tests

- Canonical projection bytes stay identical across repeated builds and shuffled
  equivalent input; strict decoding rejects duplicate keys, unknown fields,
  invalid UTF-8, trailing values, floats, and out-of-range integers.
- Deterministic digest, version, identity, stable ordering, per-record bounds,
  and total response bounds.
- Sentinel bytes prove `SpecDownload.JSON`, full `ExampleSpecJSON`, and
  parser-only data neither appear in projection bytes nor affect the digest.
- Root and nested publication route construction cannot escape publication base.
- Server and Wasm renderers produce equivalent canonical anchors, hrefs,
  headings, and fragment landmarks.
- Default-on eligibility, kill switch, public/private boundary, and pure SSR
  fallback.
- Lazy sidebar section rendering and selected ancestor behavior.
- Enhancement manifest uses only public Goshtoso `v0.0.13` exports, preserves
  dependency order, and every embedded fallback resolves through
  `assets.Handler()` with the declared version.
- Offline-shell cached body, CSP header, and any nonce form one valid immutable
  response pair.

### JavaScript worker/storage tests

- Exact interception allowlist and unconditional pass-through for management,
  private, cross-origin, non-GET, and ordinary document requests.
- Multipublication active/previous storage, LRU three-publication eviction, quota
  recovery, corruption rebuild, rollback, and concurrency serialization.
- Conditional revalidation and automatic valid-revision activation.
- `401`, `403`, `404`, `410`, disabled descriptor, public-to-private successor,
  and deleted publication disable routing before I/O, persist tombstones, reject
  new writes, and perform publication-scoped best-effort purge.
- Purge failure cannot bypass a tombstone or affect another publication.
- Offline-ready rejects a missing, wrong-version, non-embedded, or failed
  Goshtoso fallback and rejects shell body/header CSP mismatch.
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
- Public-to-private transition and deleted publication stop local routing on the
  first authoritative online observation, never cache a private successor, and
  remain disabled offline even when physical purge is forced to fail.
- Full reload and pasted deep links work online and after offline readiness.
- Private publication and `/manage` produce no persistent docs cache entries.
- Forced CDN failure still reaches offline-ready from exact embedded Goshtoso
  fallbacks; after full network disconnection, reload, search, sidebar expansion,
  request interactions, and detail navigation pass with valid CSP.
- Browser console has no errors or HTMX OOB/attribute parsing warnings.

### Benchmark evidence

Every performance run records fixture digest, browser version, sample count,
cold activation, worker restart, reload, local/server navigation, HTML bytes,
projection bytes, raw/compressed Wasm bytes, DOM count, and long tasks.

## Rollout and Reversal

No slice starts until the Integration DAG prerequisites are verified and a fresh
implementation worktree records its exact base. Implementation then proceeds in
independently reversible slices:

1. deterministic projection and server endpoints, unused by the page;
2. lazy sidebar and client search on normal SSR/HTMX;
3. product-packaged reduced Wasm renderer and parity tests;
4. background Service Worker activation with server fallback;
5. multipublication offline storage and automatic freshness;
6. default-on eligibility for public publications.

Each slice keeps server rendering deployable. The kill switch makes projection
and offline-shell resources return authoritative disabled state and removes the
descriptor from new SSR responses. A connected worker disables local routing on
its next document response, `online` event, or five-minute visibility
revalidation; an offline worker retains the same unavoidable disclosure boundary
as a withdrawn public publication. The worker tombstones before best-effort
publication cleanup. Unknown or unrelated caches are never deleted.

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
- Authoritative public-to-private, disabled, revoked, and deleted observations
  tombstone the publication before purge and stop local disclosure thereafter;
  acceptance acknowledges that previously public bytes cannot be recalled from
  a browser that never reconnects.
- Published revision changes activate automatically and preserve rollback.
- Current Manja/Goshtoso UI and canonical URLs remain unchanged.
- Functional, security, corruption, parity, and performance gates pass.
