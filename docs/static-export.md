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

The output path must be absent or an empty directory. Export builds in a
sibling staging directory, verifies it, then renames it into place. It does not
replace a non-empty output directory.

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
