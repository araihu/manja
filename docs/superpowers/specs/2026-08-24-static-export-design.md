# Static Local-Docs Export Design

**Status:** Approved in chat; awaiting written-spec review

**Date:** 2026-08-24

**Issue:** [#114](https://github.com/araihu/manja/issues/114)

## Context

Manja v0.1.2 builds durable catalog snapshots and serves eligible public,
anonymous catalogs through the server-rendered catalog handler. The same page
can activate the local-docs Service Worker, storage layer, projection data, and
Wasm admission boundary. That runtime still depends on Manja for its first
shell and for HTML or HTMX states the browser has not already cached.

Generic static hosts need a complete directory instead. The directory must
contain every public input needed by the local renderer, work below either `/`
or a configured project subpath, support direct document URLs and reloads, and
prove that no link or runtime request depends on a Manja process.

## Decisions

- Add `manja export`; do not change `manja build` semantics.
- Export only catalogs explicitly configured with `localDocs.public: true`,
  `localDocs.anonymous: true`, and a valid publication key.
- Ignore ineligible catalogs before source loading, parsing, snapshot building,
  or artifact enumeration. They may appear only as bounded warnings in the
  export receipt.
- Reuse the current snapshot compiler, projection model, Goshtoso templates,
  local-docs render package, embedded assets, and validation boundaries.
- Complete browser-side rendering and search from validated projection data.
  Do not pre-render one HTML file per operation or schema.
- Treat `--base-path` as a URL prefix, not an output-directory prefix.
- Verify the completed artifact before publishing it into the requested output
  directory.
- Keep existing SSR and hybrid offline behavior unchanged. Static behavior is
  enabled only by an export descriptor.

## Goals

- Produce a directory served by an ordinary static HTTP server with no Manja
  process, reverse proxy, rewrite plugin, CDN dependency, or serverless code.
- Serve all runtime assets, projection data, search data, and downloadable
  OpenAPI sources from the same origin.
- Make catalog, document, operation, schema, sidebar, and search navigation work
  after local-docs reports offline readiness, including unseen detail states.
- Support root deployment and canonical subpaths such as `/group/project/`.
- Preserve deterministic snapshot identities and emit a machine-readable
  receipt for reproducible CI publication.
- Detect missing files, changed bytes, invalid base paths, broken internal
  links, and runtime-only requests before a successful export.

## Non-goals

- No export of private, authenticated, or partially configured catalogs.
- No browser OpenAPI parser, try-it proxy, management API, or authoring UI.
- No host-specific redirect, rewrite, or deployment configuration.
- No new visual design or parallel static-only template system.
- No incremental update protocol. A successful export is one complete immutable
  artifact tree.

## CLI Contract

```text
manja export \
  --renderer-config ./renderer.yaml \
  --data-dir ./data \
  --output ./public \
  --base-path /
```

All four flags are required. `--base-path` must be `/` or a canonical absolute
path ending in `/`; it rejects backslashes, dot segments, duplicate slashes,
percent escapes, queries, fragments, whitespace, and control characters.

The exporter writes into a sibling staging directory, verifies that tree, then
renames it into place. A non-empty output directory is rejected rather than
deleted or merged, preventing stale files and accidental data loss. An absent
or empty output directory is accepted. Cancellation or failure removes only the
exporter's staging directory.

The JSON stdout receipt has this shape:

```json
{
  "schemaVersion": 1,
  "basePath": "/project/",
  "catalogs": [
    {
      "catalogId": "fortios",
      "mount": "/fortios",
      "publicationKey": "fortios",
      "revisionId": "v7.6.3",
      "snapshotId": "snapshot-sha256-..."
    }
  ],
  "warnings": [
    {
      "code": "catalog_ineligible",
      "catalogId": "private-preview"
    }
  ],
  "manifest": "_manja/export.json"
}
```

Warnings contain a stable code and catalog ID only. They do not expose source
locations, credentials, parser errors, or source contents. When no catalogs are
eligible, export succeeds with no catalog payload, zero catalogs, the export
manifest, and warnings for skipped catalogs.

Malformed local-docs configuration remains a configuration error. A catalog
that sets only one authority flag or supplies an invalid publication key is not
silently downgraded to ineligible.

## Eligibility Boundary

Renderer configuration is decoded and validated once. The export composition
boundary partitions catalog configuration before constructing sources:

1. fully eligible public and anonymous catalogs enter an export-only renderer
   configuration;
2. wholly unconfigured local-docs catalogs become receipt warnings;
3. malformed partial local-docs configuration fails normal config validation.

Organization navigation and presentation are filtered to the eligible set.
Ineligible sources are never opened, so an unreachable private source cannot
fail or delay a public static export.

This filtering stays in `internal/selfhosted`. Provider-neutral `domain`,
`application`, and public `renderer` types do not acquire CLI or static-host
policy.

## Artifact Layout

The output root is the directory a host publishes at `--base-path`:

```text
public/
  index.html
  search/index.html
  sw.js
  manja-assets/...
  assets/...
  <catalog-mount>/
    index.html
    search/index.html
    documents/<document-key>/index.html
    _manja/offline-shell/index.html
    snapshots/<snapshot-id>/
      manifest.json
      catalog.json
      search-data/...
      projection-data/...
      openapi/...
  _manja/export.json
```

The exporter obtains public bytes through the active renderer HTTP handler
instead of decoding private catalog-store layout. Snapshot manifests remain the
authority for immutable child inventory, lengths, kinds, and SHA-256 digests.
Every exported child must match that manifest before it is written.

`sw.js` is placed at the published root so a generic host permits Service Worker
scope over the complete `--base-path` without requiring a
`Service-Worker-Allowed` response header. Other local-docs files remain exact
embedded product assets under `manja-assets/local-docs/`.

HTML route files are shell entry points, not pre-rendered copies of every detail
state. Existing canonical query URLs remain valid:

```text
<base><mount>/documents/<document>/?selected=<anchor>#<anchor>
```

A static host ignores the query when selecting `index.html`; the local renderer
reads it and materializes the selected operation or schema. Reload therefore
works without one generated file per anchor.

## Base-Path Model

`--base-path` prefixes every public URL in exported HTML, descriptors, search
records, worker registration, runtime asset lookup, snapshot routes, canonical
document links, and downloadable sources. Catalog mounts remain relative to
that deployment prefix.

For example, catalog mount `/fortios` with base path `/group/project/` becomes:

```text
/group/project/fortios/
/group/project/fortios/documents/v7.6.3/
/group/project/fortios/snapshots/<snapshot-id>/manifest.json
/group/project/manja-assets/local-docs/manja.wasm
/group/project/sw.js
```

The export rewriter operates on parsed HTML and typed descriptor/search data;
it does not use byte replacement. It rejects any absolute internal path that
cannot be classified and prefixed. External HTTPS links and fragment-only links
remain unchanged.

Server responses continue using their current root-based URLs and root-scoped
worker. Optional static descriptor fields carry deployment base, worker URL,
worker scope, and static mode. Their absence preserves the v0.1.2 hybrid
contract.

## Browser Runtime

### Activation

Static shell HTML contains the normal public eligibility descriptor plus an
export descriptor bound to the export manifest and deployment base. The browser
validates both before enabling static routing.

Local-docs activation remains fail closed:

1. validate descriptors and same-origin canonical URLs;
2. load exact `wasm_exec.js` and `manja.wasm` from the deployment base;
3. validate the snapshot manifest identity and child inventory;
4. fetch, bound, and hash every current search and projection child;
5. commit the complete generation to local storage;
6. prepare and activate Wasm rendering;
7. cache shell and product assets;
8. report offline readiness only after every navigation dependency is local.

Original OpenAPI source downloads are included in the export but remain a
separate readiness capability; documentation navigation does not require their
browser cache.

### Rendering and routing

The Wasm command expands its current `activate`, `allows`, and `resolve` surface
with bounded rendering operations backed by `internal/localdocs` selection and
`internal/localdocs/render` components. Projection bytes remain untrusted and
are admitted by digest and typed identity before rendering. Rendered user data
continues through templ escaping; no raw projected HTML is accepted.

The browser enhancer handles:

- initial URL selection after a direct load or offline shell fallback;
- operation and schema selection;
- sidebar group expansion;
- catalog and document search from complete local search shards;
- `popstate` and canonical history updates;
- HTMX-compatible main/sidebar fragment swaps when HTMX is present.

Static mode never requests server-only HTML or JSON search endpoints. Runtime
failure leaves the exported shell visible and reports a bounded error state; it
cannot fall back to an absent Manja server.

Hybrid mode keeps its current network-first SSR and HTMX fallback. New local
rendering functions may be shared, but static interception and complete-data
activation require the export descriptor.

### Service Worker

The exported root worker registers with `scope: <base-path>`. Asset and
publication allowlists derive from validated export descriptors, never from raw
request paths. It intercepts same-origin GET requests inside the deployment
base only and excludes management/API paths.

After offline readiness:

- navigation receives the cached catalog shell for its eligible publication;
- immutable snapshot and asset requests receive verified cached bytes;
- the page renderer resolves URL state from the complete local generation;
- unknown, cross-origin, non-GET, and out-of-base requests pass through.

## Export Manifest and Verifier

`_manja/export.json` is canonical compact JSON containing:

- schema version and base path;
- Manja/runtime version identity;
- eligible catalog, publication, revision, and snapshot identities;
- every relative output path, byte length, media type, and SHA-256 digest;
- shell routes and descriptor identities;
- warning codes for skipped catalogs.

The manifest excludes itself from its file entries. Entries are sorted by
relative path; exact-file verification compares those entries plus the manifest
path against the output tree.

Verification runs against the completed staging tree and is also exposed as:

```text
manja export verify --output ./public
```

The verifier checks:

- canonical manifest decoding with no duplicate or unknown fields;
- exact file-set equality, rejecting missing and undeclared files;
- every declared length and SHA-256 digest;
- required shell, worker, Wasm, runtime, manifest, search, projection, and source
  files for each eligible catalog;
- descriptor/snapshot/publication identity agreement;
- parsed internal HTML links and asset references resolve within the tree after
  removing the declared base-path prefix;
- no exported page references `/manage`, `/api`, SSR fragment endpoints,
  dynamic `search.json`, or another unclassified runtime-only route;
- worker scope and every internal URL stay within the declared deployment base.

Verification does not contact sources or rebuild snapshots. It proves the exact
directory that CI will publish.

## Failure and Security Boundaries

- Only explicit public and anonymous authority permits export.
- Export filtering happens before source construction for ineligible catalogs.
- Same-origin canonical paths only; no credentials, userinfo, query-bearing
  resources, traversal, symlinks, or device files enter the output.
- Every snapshot child and product asset is bounded and hashed before staging.
- Output publication is all-or-nothing; partial trees never replace the target.
- HTML and projection content retain current render-size and input bounds.
- Cancellation propagates through source loading, rendering, file writes, and
  verification.
- Diagnostics are bounded and never include source bodies or secrets.

## Compatibility

- `manja build`, server mode, server routes, hybrid descriptors, and current
  local-docs withdrawal behavior remain unchanged.
- Export requires JavaScript, Service Worker, IndexedDB, CacheStorage, Web
  Crypto, and WebAssembly for complete offline navigation. Initial static shell
  content remains readable when enhancement fails.
- Static hosting must serve `.wasm` as `application/wasm`. The verifier's generic
  server browser test enforces this contract.

## Test Strategy

TDD proceeds in independently runnable slices:

1. CLI parsing, eligibility filtering, warnings, and empty export;
2. canonical base-path and safe staging/output publication;
3. artifact capture, manifest creation, and exact-file verification;
4. root and subpath HTML/descriptor rewriting;
5. complete projection/search activation and Wasm fragment rendering;
6. static routing, direct reload, history, search, and Service Worker behavior;
7. generic-server browser acceptance with Manja stopped and network disabled.

Focused Go tests use small deterministic renderer fixtures. JavaScript tests
exercise routing/storage without a browser where possible. Browser acceptance
runs both `/` and `/group/project/`, proves no CDN or Manja request, opens a
previously unseen operation and schema after network disconnection, reloads a
direct URL, and uses search.

Final gates:

```bash
GOWORK=off go test ./... -count=1
(cd site && GOWORK=off go test ./... -count=1)
go run github.com/a-h/templ/cmd/templ generate
git diff --check
```

Run API generation/lint only if API YAML changes. No API change is planned.

## Rollout Boundary

This task may produce a reviewed branch and commit for issue #114. Merge, push,
release, image publication, deployment, and Pages upload remain separate user
authorities.
