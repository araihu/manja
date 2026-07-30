# Arai Hu fallback assets

`araihu-assets.json` pins one immutable `araihu/assets` release and owns the
allowlist used to refresh Manja's local no-JavaScript fallbacks. The updater
never reads the mutable `current` channel and never changes campaign runtime
configuration.

Manja has two independently embedded surfaces:

- `internal/web/static` serves renderer fallbacks under `/manja-assets/`;
- `site/internal/site/static` serves the product site and demo fallbacks.

Both surfaces receive the canonical Arai Hu theme, adaptive transparent Manja
mark, and web favicon. The product site also receives the adaptive transparent
Manja wordmark. These files are source assets embedded by Go; they are not templ
or JavaScript generator outputs.

Run a pinned release locally after separately verifying its archive:

```bash
go run ./cmd/araihu-assets-update \
  -root . \
  -release-dir /path/to/extracted/araihu-assets-v0.1.1
```

The updater verifies `release.json`, `catalog.json`, selected catalog roles,
file sizes and hashes, path confinement, and symlink absence. It stages and
syncs every replacement, commits fallbacks before `araihu-assets.json`, and
restores applied paths in reverse order if replacement fails.

`.github/workflows/araihu-assets.yml` accepts `araihu-assets-released` or a
manual dispatch containing exactly these six string fields:

- `assets_repository`
- `assets_revision`
- `release`
- `release_url`
- `release_sha256`
- `release_json_sha256`

The workflow authenticates the release tag against the dispatched revision,
verifies and extracts the archive once, proves the updater is idempotent, then
opens or updates a labeled version branch with a selected-repository GitHub App
token. It never auto-merges.
