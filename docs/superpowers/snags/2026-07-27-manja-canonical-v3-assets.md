# Canonical Manja v3 identity assets

## Scope

Canonical v3 SVG bytes replace only these established consumers:

- `internal/web/static/favicon.svg` <- `logos/manja-favicon.svg`
- `internal/web/static/manja-mark.svg` <- `logos/manja-mark.svg`
- `site/internal/site/static/favicon.svg` <- `logos/manja-favicon.svg`
- `site/internal/site/static/manja-logo.svg` <- `logos/manja-logo.svg`
- `site/internal/site/static/manja-mark.svg` <- `logos/manja-mark.svg`

`logos/manja-mark-reverse.svg` was provenance-verified only. No existing Manja
consumer requires it, so this change does not create a new static asset surface.

## Supersession and source proof

- Supersedes canonical source commit `81300f5` and its stale, uncommitted primary
  checkout SVGs. Those files were neither read nor changed.
- Source was a clean, task-owned checkout at
  `https://github.com/araihu/assets` commit
  `bffc2acfc9380eaf84473abfeaacbba625ac73d5`.
- `shasum -a 256 -c brand/canonical-assets-v3.sha256` passed for the complete
  canonical manifest in that checkout.

## Canonical manifest hashes

| Source asset | SHA-256 |
| --- | --- |
| `logos/manja-logo.svg` | `fed1f1cbc547b070b38528493cd9086816d7c66a6dd205f4a82b2dbd3a0bb760` |
| `logos/manja-mark.svg` | `1859f1b7782becff9cde39ef18d29d1c84896612f39c9f4c33b90474c582c0be` |
| `logos/manja-favicon.svg` | `250cf00695efd8dee2d5f5b3335fec5d8301ca2ff86f328ccaa236ec9cf699a8` |
| `logos/manja-mark-reverse.svg` | `ceba20ce0c14876989b64e3cc88bc10cca995c32c1d01f53b1fb4d77b020852a` |

## Installed byte proof

| Consumer path | SHA-256 |
| --- | --- |
| `internal/web/static/favicon.svg` | `250cf00695efd8dee2d5f5b3335fec5d8301ca2ff86f328ccaa236ec9cf699a8` |
| `internal/web/static/manja-mark.svg` | `1859f1b7782becff9cde39ef18d29d1c84896612f39c9f4c33b90474c582c0be` |
| `site/internal/site/static/favicon.svg` | `250cf00695efd8dee2d5f5b3335fec5d8301ca2ff86f328ccaa236ec9cf699a8` |
| `site/internal/site/static/manja-logo.svg` | `fed1f1cbc547b070b38528493cd9086816d7c66a6dd205f4a82b2dbd3a0bb760` |
| `site/internal/site/static/manja-mark.svg` | `1859f1b7782becff9cde39ef18d29d1c84896612f39c9f4c33b90474c582c0be` |

## Reference inventory and verification

`rg` confirmed public renderer references `/manja-assets/favicon.svg` and
`/manja-assets/manja-mark.svg`; site pages reference `/static/favicon.svg` and
`/static/manja-mark.svg`. Site static files are embedded by
`site/internal/site/assets.go`. No reverse-mark consumer or competing new logo
was added. `araihu.css` and generated files remain unchanged.

Verification passed:

- `GOWORK=off go test ./internal/web`
- `GOWORK=off go test ./internal/server` from `site/`
- `GOWORK=off go test ./...` from root and `site/`
- `go run github.com/a-h/templ/cmd/templ generate` with no generated drift
- `git diff --check`; no `araihu.css` diff; no generated-file diff
- `cmp -s` for all five installed files against their task-owned canonical source
- direct HTTP smoke: renderer and site roots returned `200`; every served
  favicon, mark, and logo hashed to the installed canonical SHA-256 above

## Tooling friction

- Mem0 semantic search quota was already exhausted. It did not affect source
  provenance, repository changes, or verification.
- CodeRabbit CLI `0.7.0` was authenticated but reported `Review failed: All
  files are ignored` for this SVG-and-Markdown-only diff. Manual review used
  exact source/manifest verification, five byte comparisons, reference
  inventory, generated-drift checks, full module tests, and HTTP-served hashes.

## Deferred assurance receipt

Authoritative receipt: [Manja issue #56](https://github.com/araihu/manja/issues/56).

- Exact candidate SHA: `6e70b1dbb8e12c1c0f223211157684c5ee0c0212`.
- Skipped gate: full visual theme/browser/platform matrix, repeated reload/cache
  stress, and asset-rendering performance sampling.
- Risk: low. This feature-delivery slice only replaces established static SVG
  bytes; provenance, served hashes, module tests, generation drift, and HTTP
  smoke are already green.
- Affected paths:
  - `internal/web/static/favicon.svg`
  - `internal/web/static/manja-mark.svg`
  - `site/internal/site/static/favicon.svg`
  - `site/internal/site/static/manja-logo.svg`
  - `site/internal/site/static/manja-mark.svg`
- Current green evidence: complete
  `araihu/assets@bffc2acfc9380eaf84473abfeaacbba625ac73d5` manifest
  verification; installed and HTTP-served SHA-256 bytes match canonical v3;
  root and `site/` `GOWORK=off go test ./...`; `templ generate` with zero drift;
  renderer and product-site HTTP `200` smoke.
- Acceptance: render every affected asset in supported release browser engines;
  pass 390px and 1440px Goshtoso/Minimal/Arai Hû visual checks where supported,
  in light and dark modes; pass repeated reload/cache validation without missing
  or stale assets; record objective visual evidence and material
  rendering/performance regressions.
- Trigger: before next Manja release certification including candidate
  `6e70b1d` or descendants.
- Owner: Manja release gate.
