# Canonical Manja V10 identity assets

## Scope and independent-symbol rule

Canonical V10 SVG bytes replace only established Manja consumers:

- `internal/web/static/favicon.svg` <- `logos/manja-favicon.svg`
- `internal/web/static/manja-mark.svg` <- `logos/manja-mark.svg`
- `site/internal/site/static/favicon.svg` <- `logos/manja-favicon.svg`
- `site/internal/site/static/manja-logo.svg` <- `logos/manja-logo.svg`
- `site/internal/site/static/manja-mark.svg` <- `logos/manja-mark.svg`

Manja remains an independent product symbol. Only Arai Hû uses the cloud;
Manja receives no Arai Hû cloud-derived asset. `logos/manja-mark-reverse.svg`
was provenance-verified only: no existing runtime or template consumer requires
it, so no competing asset surface was created.

## Source proof

- Historical evidence only: V3 snag and Manja issue #56. Their V3 source and
  installed hashes are superseded and their stale primary-checkout SVG bytes
  were never read or used.
- Source: clean, task-owned checkout of `https://github.com/araihu/assets` at
  `ab01f1a0f592e4f1398173df04e4f8fc013cb21a`.
- Manifest: `brand/canonical-assets-v10.sha256`.
- `shasum -a 256 -c brand/canonical-assets-v10.sha256` passed for every
  manifest entry.
- Declared favicon checksum matches source:
  `468b6884c9ff98a3eea7fe081676453ca6132dac541f2e0fc4cd9a3e3224d749`.

## Canonical and installed checksums

| Source asset | SHA-256 |
| --- | --- |
| `logos/manja-logo.svg` | `69b9bc559e634cf3d25a515ecd42215e288775f522db7ee82e57558087471c2a` |
| `logos/manja-mark.svg` | `3069ca71a4ae2e2af647d86a07388bea92a32bb51bccedf9adc149216c55053d` |
| `logos/manja-favicon.svg` | `468b6884c9ff98a3eea7fe081676453ca6132dac541f2e0fc4cd9a3e3224d749` |
| `logos/manja-mark-reverse.svg` (uninstalled) | `0e78289efe263e309805c0a69172f92c47b73bdc7a05dfa6c81fbd2cfb79f678` |

| Consumer path | SHA-256 |
| --- | --- |
| `internal/web/static/favicon.svg` | `468b6884c9ff98a3eea7fe081676453ca6132dac541f2e0fc4cd9a3e3224d749` |
| `internal/web/static/manja-mark.svg` | `3069ca71a4ae2e2af647d86a07388bea92a32bb51bccedf9adc149216c55053d` |
| `site/internal/site/static/favicon.svg` | `468b6884c9ff98a3eea7fe081676453ca6132dac541f2e0fc4cd9a3e3224d749` |
| `site/internal/site/static/manja-logo.svg` | `69b9bc559e634cf3d25a515ecd42215e288775f522db7ee82e57558087471c2a` |
| `site/internal/site/static/manja-mark.svg` | `3069ca71a4ae2e2af647d86a07388bea92a32bb51bccedf9adc149216c55053d` |

`cmp -s` passed for every installed consumer against its source counterpart.

## Reference inventory

`rg -n --hidden -i 'manja[-_ ]?(logo|mark|favicon)|/manja-assets/(favicon|manja-mark)\\.svg|/static/(favicon|manja-logo|manja-mark)\\.svg'` found:

- Renderer defaults: `/manja-assets/favicon.svg` and
  `/manja-assets/manja-mark.svg` in `internal/web/public.go`; public tests
  assert both.
- Product/demo site: `/static/favicon.svg` and `/static/manja-mark.svg` in
  `site/internal/site/pages.go`; the site embeds its static files through
  `site/internal/site/assets.go` and server tests fetch the favicon.
- No runtime/template consumer references a Manja logo in renderer or a reverse
  mark in either product surface. The product-site logo remains an existing
  embedded consumer and is canonicalized without adding a new reference.

## Verification and baseline issue

Manja issue [#56](https://github.com/araihu/manja/issues/56) remains open as
the historical V3 quality-debt receipt. Parent control plane owns its mutation.
Proposed update: replace V3 candidate/integrated SHA evidence with this V10
candidate/integrated SHA and retain the same deferred visual/browser/cache/
performance acceptance criteria.

Verification passed:

- `npm ci`
- `GOWORK=off go test ./internal/web`
- `GOWORK=off go test ./internal/server` from `site/`
- root and `site/` `GOWORK=off go test ./...`
- `go run github.com/a-h/templ/cmd/templ generate`, then `git diff --exit-code`
  for generated output: zero generated drift
- `npm run dev` HTTP `200` smoke: renderer Air proxy `/`, product site `/`, and
  product demo `/demo/payments/v1/`
- HTTP-served SHA-256 checks for every established static consumer: renderer
  `/manja-assets/favicon.svg` and `/manja-assets/manja-mark.svg`; product site
  `/static/favicon.svg`, `/static/manja-logo.svg`, and
  `/static/manja-mark.svg`; and demo-mounted renderer favicon and mark. Every
  hash equals the installed V10 checksum table above.
- `git diff --check`; changed paths are only the five consumers above plus this
  provenance ledger.

## Goshtoso friction

None observed. This migration changes static canonical bytes only: no Goshtoso
component, theme, template, generated templ, CSS, route, or behavior changed.

## Deferred assurance

Deferred: full visual theme/browser/platform matrix, repeated reload/cache
stress, and rendering-performance sampling. Risk is low because established
static consumers retain their paths and every installed and HTTP-served byte is
checked against the V10 manifest source. Complete the deferred gates before
release certification including this candidate or descendants.
