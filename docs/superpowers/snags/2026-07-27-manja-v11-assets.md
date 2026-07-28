# Manja v11 adaptive identity assets

## Scope

Approved source: task-owned checkout of `https://github.com/araihu/assets` at
`a8a9647a6e803586c556859eb20f95ef9fcb20a1`.

Established consumers now use exact `concepts/v11` bytes:

- `internal/web/static/favicon.svg` and `site/internal/site/static/favicon.svg`
  use `manja-icon-background.svg`.
- `internal/web/static/manja-mark.svg` and `site/internal/site/static/manja-mark.svg`
  use `manja-icon-transparent.svg`.
- `site/internal/site/static/manja-logo.svg` uses `manja-logo-transparent.svg`.

The product-site navigation now references its canonical full logo directly.
The renderer retains its existing mark-only consumer. No redesigned mark,
geometry edit, or competing logo surface was introduced.

## Adaptive theme contract

Every installed v11 SVG preserves `v11-surface`, `v11-ink`, and `v11-signal`
semantic fills, including their `prefers-color-scheme: dark` fallback. The site
already applies `color-scheme: dark` on the root `.dark` class, matching the
Goshtoso dark-mode convention and allowing embedded SVGs to select their dark
palette with the site toggle.

## Goshtoso snag checkpoint

No Goshtoso component, helper, generated templ output, or dependency behavior
blocked this change. The only source dive was confirming that the standalone
product site has its own root `.dark`/`color-scheme` contract; it aligned with
the requested Goshtoso behavior, so no upstream API or workaround was needed.
