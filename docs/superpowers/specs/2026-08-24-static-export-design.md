# Static Local-Docs Export Design

**Status:** Reviewed and approved for implementation planning

**Date:** 2026-08-24

**Issue:** [#114](https://github.com/araihu/manja/issues/114)

## Context

Manja v0.1.2 builds durable catalog snapshots and conditionally serves the
local-docs enhancement for public, anonymous catalogs. The same page can
activate the Service Worker, storage layer, projection data, and Wasm admission
boundary. That runtime still depends on Manja for its first shell and for HTML
or HTMX states the browser has not already cached.

Generic static hosts need a complete directory instead. The directory must
contain every input needed by the local renderer, work below either `/`
or a configured project subpath, support direct document URLs and reloads, and
prove that no link or runtime request depends on a Manja process.

## Decisions

- Add `manja export`; do not change `manja build` semantics.
- Export every configured catalog. `localDocs.public`, `localDocs.anonymous`,
  and `localDocs.publicationKey` do not select or exclude export content.
- Treat invocation as the explicit disclosure boundary. The operator owns the
  output and decides where it is published.
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

- No visibility filtering, catalog selection flags, or access-control policy.
  Those can be designed later if a concrete need appears.
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

Creation mode requires all four flags. `--base-path` must be `/` or a canonical
absolute path ending in `/`; it rejects backslashes, dot segments, duplicate
slashes, percent escapes, queries, fragments, whitespace, and control
characters. Verification mode is dispatched separately as
`manja export verify --output <directory>` and requires only `--output`.

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
  "manifest": "_manja/export.json"
}
```

`publicationKey` is the catalog ID in static exports. This creates a unique,
stable cache namespace without consulting visibility configuration. Renderer
configuration still passes its existing validation before export.

## Catalog Selection and Disclosure Boundary

Renderer configuration is decoded and validated once. Every configured catalog
then follows the normal source loading, parsing, snapshot building, and artifact
capture path. Organization navigation and presentation retain the complete
configured catalog set.

The export descriptor derives its cache namespace from the validated catalog ID
and carries export authority separately from server-side local-docs visibility.
No provider-neutral `domain`, `application`, or public `renderer` type acquires
CLI visibility policy.

**Security boundary:** export deliberately materializes every configured
catalog, including catalogs otherwise served privately or with no local-docs
enhancement. Publishing the output makes that data available to the static
host's audience. The command does not infer or preserve Manja authentication.

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
    llms.txt
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

The exporter obtains rendered bytes through the active renderer HTTP handler
instead of decoding private catalog-store layout. Snapshot manifests remain the
authority for immutable child inventory, lengths, kinds, and SHA-256 digests.
Every exported child must match that manifest before it is written.

`sw.js` is placed at the published root so a generic host permits Service Worker
scope over the complete `--base-path` without requiring a
`Service-Worker-Allowed` response header. Other local-docs files remain exact
embedded product assets under `manja-assets/local-docs/`.

All static HTML routes end in `/` and map to directory `index.html` files. The
export descriptor therefore uses `<publication-base>_manja/offline-shell/`,
including the trailing slash. Static descriptor validation admits that form;
hybrid descriptors retain the existing extensionless offline-shell route. This
avoids a redirect during the worker's exact, redirect-rejecting shell fetch.

HTML route files are shell entry points, not pre-rendered copies of every detail
state. Existing canonical query URLs remain valid:

```text
<base><mount>/documents/<document>/?selected=<anchor>#<anchor>
```

A static host ignores the query when selecting `index.html`; the local renderer
reads it and materializes the selected operation or schema. Reload therefore
works without one generated file per anchor.

The exporter writes each catalog's `llms.txt`. It rewrites stable
`catalog.json` and OpenAPI download links to their immutable exported snapshot
files, avoiding redirects and duplicate bytes. Static pages omit the
server-generated Page Markdown action because `?format=markdown` cannot return a
different representation from a generic static host. Server and hybrid pages
keep that action unchanged.

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

Static shell HTML contains an export descriptor with the export-manifest path,
catalog ID, snapshot identity, and deployment base. The descriptor contains no
export-manifest digest: the manifest hashes the HTML file, so putting its digest
back into that HTML would create a cycle. The browser checks that the manifest's
catalog and snapshot identities equal the descriptor before enabling static
routing. Server local-docs visibility flags are not part of this descriptor.

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

- navigation receives the cached catalog shell for its exported publication;
- immutable snapshot and asset requests receive verified cached bytes;
- the page renderer resolves URL state from the complete local generation;
- unknown, cross-origin, non-GET, and out-of-base requests pass through.

## Export Manifest and Verifier

`_manja/export.json` is canonical compact JSON containing:

- schema version and base path;
- exported catalog, publication, revision, and snapshot identities;
- every relative output path, byte length, media type, and SHA-256 digest;
- shell routes and descriptor identities.

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
  files for each exported catalog;
- descriptor/snapshot/publication identity agreement;
- parsed internal HTML links and asset references resolve within the tree after
  removing the declared base-path prefix;
- no exported page references `/manage`, `/api`, SSR fragment endpoints,
  dynamic `search.json`, or another unclassified runtime-only route;
- worker scope and every internal URL stay within the declared deployment base.

Verification does not contact sources or rebuild snapshots. It proves the exact
directory that CI will publish.

## Failure and Security Boundaries

- Invocation authorizes export of every configured catalog; visibility is not a
  selection input.
- Static export descriptors use catalog IDs as cache namespaces and cannot
  inherit server authentication or credentials.
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

1. CLI parsing, all-catalog selection, and catalog-ID cache namespaces;
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
