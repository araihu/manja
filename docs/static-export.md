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
required runtime and catalog artifacts, descriptors, and internal links.

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
- serve `.wasm` as `application/wasm`;
- allow `sw.js` to control the configured base path;
- serve the generated files without rewriting them to Manja or another API.

All JavaScript, Service Worker, Wasm, snapshot, search, projection, and OpenAPI
source bytes are included under the configured base path. Direct document
loads and reloads therefore work on a generic static server, while unseen
operation and schema navigation is rendered locally from the exported data.
