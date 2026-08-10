# Catalog sidebar fragment navigation and visual restraint

## Scope

- Keep operation, schema, and schema-node navigation inside the persistent
  catalog shell through HTMX fragments.
- Preserve direct-link fallback URLs, history, selection, focus, and the
  existing responsive sidebar behavior.
- Keep repeating organization/spec avatars neutral; reserve the Arai Hû
  primary color for focus, selection, and actions.

## Goshtoso snag checkpoint

- `sidebar.Item.LinkAttrs` supplied the required HTMX extension point without
  copying Goshtoso markup. Manja still had to distinguish the main-content,
  sidebar, and schema-node fragment targets in its own handler.
- The operation badge fallback is consumer CSS. Schema items carried an empty
  `data-catalog-method`, so the fallback pseudo-element rendered an empty
  capsule. No Goshtoso API change was needed; Manja now requires a non-empty
  method before drawing that fallback.
- Matching the neutral document-table avatar required a small Goshtoso source
  dive: `table.ImageCell` uses the default avatar tone when no image is present.
  The organization sidebar now composes `avatar.ToneDefault` directly instead
  of repeating `avatar.TonePrimary` for every spec.
- Templ generation behaved as documented. No dependency replacement, copied
  component internals, or upstream compatibility shim was introduced.

## Evidence

- Focused Chromium coverage proves operation, schema, and schema-node clicks
  preserve the existing page context while updating their intended fragment,
  URL, selection, and focus state.
- The visual regression compares computed avatar colors against the active
  theme tokens at mobile and desktop sizes: spec avatars resolve to the neutral
  surface token and not the primary token.
- Direct links remain ordinary anchors, so navigation still works without
  HTMX. HTTP method badges remain singular, while schema items render none.
