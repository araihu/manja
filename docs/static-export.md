# Static export

`manja export` builds the configured renderer, captures its active catalogs and
browser assets, and writes a directory that does not require Manja at request
time.

> **Disclosure boundary:** export includes every configured catalog regardless
> of catalog visibility. The command warns on stderr, but visibility never
> filters the output. Access control belongs at the static host.

## Create and verify

```bash
manja export \
  --renderer-config ./renderer.yaml \
  --data-dir ./data \
  --output ./public \
  --base-path /

manja export verify --output ./public
```

Both commands write a JSON receipt to stdout. Verification reads only the
export directory. It checks the canonical receipt, exact file set, hashes,
required browser and catalog artifacts, descriptors, internal links, and the
complete graph of prebuilt HTML fragments and their integrity sidecars.

The output path may be absent, empty, or an existing valid export that can be
reused for a warm build. Other non-empty directories are rejected. Export builds
in a sibling staging directory, verifies it, then publishes it into place.

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

## Project subpaths

Use the final URL prefix, including its trailing slash:

```bash
manja export \
  --renderer-config ./renderer.yaml \
  --data-dir ./data \
  --output ./public \
  --base-path /group/project/
```

Mount `./public` at `/group/project/`. Do not copy it into a nested
`group/project` directory unless the host itself serves that directory as the
URL prefix.

## Static host requirements

The host must:

- serve directory requests from their `index.html`;
- allow `sw.js` to control the configured base path;
- serve the generated files without rewriting them to Manja or another API.

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
