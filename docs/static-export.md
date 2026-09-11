# Static export

`manja export` generates an API documentation site that runs on an ordinary
static HTTP host. Manja reads and compiles the OpenAPI sources at build time.
The host serves files; navigation, lazy fragment loading, and search run in the
browser. No Manja process, database, or search API is required at request time.

> **Disclosure boundary:** export includes every configured catalog regardless
> of catalog visibility. The command warns on stderr, but visibility never
> filters the output. Access control belongs at the static host. Export only
> sources you intend readers of that host to access, including source downloads.

## Configure, export, and preview

Use a Manja binary containing the incremental HTML exporter. The
[Pages examples](pages-deployment.md) pin a revision with this implementation.
When building from a Manja checkout, run `go build -o ./bin/manja ./cmd/manja`
and use `./bin/manja` in place of `manja` below.

For a documentation repository with `specs/petstore.yaml`, create
`renderer.yaml` in its root:

```yaml
version: 1
catalogs:
  - id: pets
    mount: /catalogs/pets
    title: Pet APIs
    defaultDocument: petstore
    profile: strict-v1
    source:
      kind: files
      root: ./specs
      include:
        - petstore.yaml
```

The source root is relative to the configuration file. Add catalogs to the
same list to build a deployment with multiple catalogs. Catalog mounts become
URL paths below the deployment base path.

```bash
manja export \
  --renderer-config ./renderer.yaml \
  --data-dir ./.manja/data \
  --output ./public \
  --base-path / \
  --sidebar-chunk-size 12 \
  --fragment-workers 4 \
  --progress

manja export verify --output ./public
python3 -m http.server 8080 --directory ./public
```

Open `http://localhost:8080/`. This preview serves generated files and does not
start the Manja documentation server. These preview instructions assume
`--base-path /`; a subpath export must be served at its matching URL prefix.

The export and verify commands write JSON receipts to stdout. Verification reads only the
export directory. It checks the canonical receipt, exact file set, hashes,
required browser and catalog artifacts, descriptors, internal links, and the
complete graph of prebuilt HTML fragments and integrity sidecars.
Store redirected command output outside `public`, since extra files invalidate
the export's file inventory.

## Progress reporting

Export is quiet apart from its catalog-visibility warning unless `--progress` is
requested. Progress events are written to stderr, so the JSON receipt on stdout
remains safe for callers to parse. Use `--progress` for plain log lines or
`--progress=json` for newline-delimited JSON events. Both modes flush each event
and work in TTY and non-interactive CI logs; no terminal control sequences are
used. JSON mode also omits the ordinary visibility warning from stderr so the
stream contains only progress events; the default invocation and human mode
retain that warning.

The event schema has `schemaVersion: 1` and these event values:

| Event | Meaning |
| --- | --- |
| `start` | Export reporter started; the initial phase is `load/normalize/compile`. |
| `progress` | A phase change or periodic snapshot while export is running. |
| `complete` | Export succeeded and the final counters are available. |
| `error` | Export failed; `error` contains the returned diagnostic. |

Every event includes `status`, `phase`, `elapsedMs`, catalog/document progress,
operation and schema counts where available, `files`, `bytes`, and worker
activity. Phases are `load/normalize/compile`, `materialize`, `render`,
`hash/manifest`, `verify`, and `complete`. A typical machine-readable event is:

```json
{"schemaVersion":1,"event":"progress","status":"running","phase":"render","elapsedMs":12000,"catalog":"pets","document":"petstore","catalogsCompleted":0,"catalogsTotal":1,"documentsCompleted":0,"documentsTotal":1,"operationsCompleted":24,"operationsTotal":81,"schemasCompleted":0,"schemasTotal":32,"fragmentsCompleted":24,"fragmentsTotal":113,"files":145,"bytes":823104,"activeWorkers":4,"peakWorkers":4}
```

The final `complete` or `error` event reports success or failure, elapsed time,
and the output file and byte totals. Without `--progress`, generated files,
hashes, ordering, and runtime behavior are unchanged.
The command exits `0` after a `complete` event, `1` after an export `error`,
and `2` for invalid command-line arguments before an export starts.

## What readers get

Page shells provide home, catalog, and document overviews. Selecting an
operation loads its generated HTML fragment into the document page. Schema
trees are separate fragments, loaded and hash-verified lazily when needed.
They can be reused by operation fragments within a document. The export also
includes standalone schema, example, and path fragments.

The document sidebar has Operations and Schemas tabs. Its default state opens
the first operation group; explicit URL state or a selected operation can open
other groups. Opening a group loads its operations on demand. Lists load in
chunks, defaulting to 12 entries. More chunks can load while the loading marker
remains visible, including several chunks on a tall viewport.

Ctrl+K/Cmd+K search is deployment-wide, including from home and catalog pages.
It uses exported spec-derived data, including operation/path/schema names and
descriptions, with fuzzy matching and highlighted results. On a document page,
matches in that spec receive a ranking boost; results from other catalogs
remain available. Search results navigate to document detail URLs. Search
indexes are fetched by the browser as needed, without a backend search service.

JavaScript is required for interactive navigation and search. Static exports load
verified HTML for operations, lazy trees, sidebar groups and deep schema-node links.
Projection JSON, WASM and its loader are not published. Static
hosting does not mean the entire site is downloaded on the first visit:
offline availability depends on which files have been cached and on browser
storage. An uncached fragment or search shard still requires network access.
Opening `index.html` through `file://` is not supported.

## Output layout

For the example catalog, the generated directory has this shape. Names shown
with `<...>` are deterministic resource or snapshot identifiers.

```text
public/
  index.html
  search/index.html
  sw.js
  _manja/
    export.json
    identity.json
    search/directory.json
  assets/
  manja-assets/
  catalogs/pets/
    index.html
    documents/petstore/
      index.html
      _manja/fragments/
        operations/<resource>.html
        operations/<resource>.html.meta.json
        schemas/...
        examples/...
        paths/...
        sidebar/...
    snapshots/<snapshot>/
      catalog.json
      openapi/...
      projection-data/...
      search-data/...
```

Deploy the complete directory. HTML alone is insufficient: sidecars are used
for fragment verification, and search data, manifests, and runtime assets are
part of the site. Keep build state from `--data-dir` outside the published
directory. Generated output is suitable for CI artifacts; it need not be
committed to your source repository.

Fragment URLs are determined by resource identity. A hash-shaped filename
does not guarantee immutable content across builds: the bytes for that resource
can change. The adjacent `.html.meta.json` records its build key and content
identity. Preserve generated URLs instead of constructing fragment filenames
from raw OpenAPI paths or display names.

## Incremental rebuilds

Run the same export command again with the existing `public` directory. The
output may be absent, empty, or an intact, verified Manja export. An unrelated
nonempty directory or damaged export is rejected; export does not silently
overwrite it.

Manja checks cached fragment build keys and verifies HTML bytes against their
sidecars before reuse. Keys include resource inputs, upstream source/compiler
identities, the deployment context, relevant build options, and the Manja
executable identity. The executable's digest also covers its embedded
templates, CSS, JavaScript, and Goshtoso dependency. Updating these inputs can
invalidate cached fragments even when the spec text is unchanged.

Invalidation is conservative: source/document identity changes can affect
more fragments than the individual edited resource. Incremental export saves
rendering work on cache hits; it still prepares catalogs, writes a staged
output tree, and verifies that complete tree. It is not an in-place update of
only changed files, and some fragment types are prepared before cache lookup.

The existing export remains in place while a sibling staging directory is
built and verified. Publication renames the previous output aside and renames
the staged tree into place, restoring the previous directory if that rename
fails. Successful publication removes the previous tree. Reserve disk for both
trees. These are two rename operations, not a zero-downtime hosting guarantee;
use the static provider's deployment mechanism to publish a complete revision.

For reuse between CI runs, restore the complete verified export to the same
output path before invoking Manja and save the successful replacement afterward.
Preserving only `--data-dir` does not preserve the HTML fragment cache. Keep
the Manja executable/toolchain and base path consistent to maximize hits.
Scope cached exports to the project, deployment prefix, and trusted build
workflow; export hashes verify consistency, not who supplied the cache.

Avoid immutable cache keys that forever restore only the first build: save a
new successful export per run and restore the latest compatible one. Do not
let simultaneous exporters write the same output directory. A missing cache
should produce a clean build; if a restored cache fails verification, retry
with a fresh output directory or invalidate that CI cache.

## Build controls

| Flag | Default | Purpose |
| --- | --- | --- |
| `--sidebar-chunk-size` | `12` | Maximum entries per sidebar chunk; larger values trade fewer fetches for larger chunks. |
| `--fragment-workers` | `4` | Concurrent detail fragment renderers; use 1–32, reducing it when build memory is constrained. |
| `--progress[=json]` | off | Emit periodic human-readable progress to stderr, or stable newline-delimited JSON with `=json`. |
| `--base-path` | Required | Final URL prefix, starting and ending with `/`. |

The renderer configuration, data directory, and output directory flags are
also required. Worker count is a concurrency control, not a total-memory cap.
Source compilation, catalog preparation, and output verification still need
resources. See [resource limits](../README.md#resource-limits) for
`MANJA_RESOURCE_LIMITS` and [environment variables](environment.md).

## Build memory and concurrency

Export uses four fragment workers by default; set `--fragment-workers` to a value
from 1 to 32 to override it. Workers share a decoded schema cache with a default
budget of 128 MiB. Set `--schema-cache-mib 256` for a larger cache, or
`--schema-cache-mib 0` to disable shared retention.

The cache budget accounts for estimated decoded storage and bookkeeping, rather
than limiting total process memory. Active rendering, source compilation, and
verification also consume memory. Loaded schema files remain hash-verified even
when their decoded contents are cached. The cache is used during export and does
not change the deployed site's browser cache or search behavior.

## URL prefixes

Use the final URL prefix, including its trailing slash:

```bash
manja export \
  --renderer-config ./renderer.yaml \
  --data-dir ./.manja/data \
  --output ./public \
  --base-path /group/project/
```

Mount `./public` at `/group/project/`. Do not copy it into a nested
`group/project` directory unless the host itself serves that directory as the
URL prefix.

Choose the prefix from the final public URL, not the local output directory:

| Public URL | `--base-path` |
| --- | --- |
| `https://docs.example.com/` | `/` |
| `https://example.github.io/api-docs/` | `/api-docs/` |
| `https://docs.example.com/group/project/` | `/group/project/` |

Re-export when the prefix changes. Preserve query strings and fragment
identifiers in document links, including `?selected=...#...` deep links.

## Static host requirements

The host must:

- serve directory requests from their `index.html`;
- serve JavaScript, JSON, and CSS with their correct MIME types;
- allow `sw.js` to control the configured base path;
- serve the generated files without rewriting them to Manja or another API.

Use HTTPS in production so service workers and browser cryptography are
available; localhost HTTP works for development. The root `sw.js` must be
accessible below the selected base path and allowed to control that scope.

Do not use an SPA catch-all that returns the home HTML for missing fragments
or JSON files. Disable host-side HTML/JS rewriting or minification after
export, as modified bytes can fail integrity checks. Normal HTTP content
encoding is compatible when decoded bytes are unchanged.

Allow revalidation for page shells, `sw.js`, export/search manifests, and
stable fragment URLs with their sidecars. Avoid a blanket long-lived immutable
cache policy for the entire tree. Publish a complete revision together so an
old sidecar is not paired with new HTML. Versioned snapshot paths can have a
separate cache policy.

For Pages CI workflows, see [Deploy to Pages](pages-deployment.md). Object
storage hosting also needs directory-index handling and HTTPS, typically
through its website/CDN configuration; merely uploading files to a raw object
endpoint does not supply all of these behaviors.

## Deployment checks and troubleshooting

Before uploading, run `manja export verify --output ./public`. After publishing,
open home, a catalog, a document, and a direct selected-operation URL. Reload
the deep link, expand a previously closed operation group, load a schema, and
search for a result in another catalog.

If sections or search fail, inspect the requested fragment, sidecar, or JSON
URL. It should return the expected file, not a host error page or SPA shell.
Check the deployment prefix, MIME types, secure context, and stale CDN/service
worker caches. Integrity failures often mean an incomplete deployment or
post-export file changes. Preserve the failed output for inspection and build
to a fresh directory if it no longer passes verification.

Measure both total file bytes and entry count when sizing deployments. Many
small fragments and sidecars consume more filesystem blocks than their payload
size; a compressed CI archive is smaller again. Provider quotas may measure
these differently. Check the hosting provider's current limits before
publishing large catalog collections.

## HTML-only publication

The bundle includes JavaScript, the Service Worker, prebuilt HTML and integrity
sidecars, search indexes, and OpenAPI source downloads. Operation navigation,
lazy schema trees, sidebar groups, and deep schema-node links load verified HTML.
Shared schema-node panels are emitted once per document and combined with the
selected schema's HTML in the browser. Visited content remains available offline.

Projection JSON shards, the WASM renderer, and its JavaScript loader are build or
browser-rendered deployment inputs; static exports do not publish them. Snapshot
manifests and catalog directories remain as provenance and verification metadata.
Their source-child inventory describes the compiled snapshot, while the export
receipt inventories the files actually published. Search retains its own index
files and does not depend on projection shards.

New export receipts declare `rendering: "html"`. Verification also accepts older
exports, which can be used as warm-build inputs; the build identity invalidates
HTML produced by a different executable.
