# Manja Goshtoso Brand Theme Design

## Goal

Create a Manja-branded Goshtoso theme that lives in the Manja repo, uses the
approved Branded Workbench direction, and becomes the default theme for Manja
public docs while keeping the built-in Goshtoso theme options available.

## Scope

In scope:

- Add a local `data-theme="manja"` token override in Manja's own public docs
  stylesheet.
- Make `manja` the default theme used by the public docs shell when no stored
  theme exists.
- Keep the existing theme picker visible and include both `Manja` and the
  built-in Goshtoso themes.
- Preserve the existing single-icon dark-mode toggle and user dark-mode
  preference behavior.
- Update focused tests for the default theme, theme picker options, and local
  theme token CSS.
- Regenerate templ output after editing `.templ` files.

Out of scope:

- Changing Goshtoso itself or updating the Goshtoso dependency.
- Removing or redesigning the existing theme picker.
- Adding a new public docs layout, hero, dashboard, or try-it surface.
- Replacing the standalone product site palette.

## Visual Direction

Use option A, Branded Workbench:

- Warm off-white light surfaces based on the existing product-site tokens:
  `#f7f4ec`, `#fffdf8`, and `#eeece2`.
- Deep charcoal text and dark surfaces from the approved logo assets:
  `#101920`, `#0b1116`, `#101513`, and `#151b1a`.
- Vivid teal-green primary accent from the assets: `#18d6a7`, with a darker
  accessible light-mode action tone for text links and selected navigation.
- Restrained product UI usage: teal is for focus, selected state, active
  navigation, links, and true action emphasis, not decorative page washes.
- Radius stays compact and workbench-like by setting `--radius-radius` to
  `var(--radius-lg)`.

## Architecture

Manja already loads Goshtoso's compiled CSS through `@head.Dependencies()` and
then loads `/manja-assets/manja.css`. The theme should therefore live in
`internal/web/static/manja.css` as a local override:

```css
@layer base {
  [data-theme=manja] {
    --font-body: 'Inter', sans-serif;
    --font-title: 'Poppins', sans-serif;
    --color-surface: #f7f4ec;
    --color-surface-alt: #fffdf8;
    --color-on-surface: #101920;
    --color-on-surface-strong: #0b1116;
    --color-primary: #0d8f73;
    --color-on-primary: #fffdf8;
    --color-secondary: #18d6a7;
    --color-on-secondary: #07120f;
    --color-outline: rgba(16, 25, 32, 0.16);
    --color-outline-strong: rgba(16, 25, 32, 0.28);
    --color-surface-dark: #101513;
    --color-surface-dark-alt: #151b1a;
    --color-on-surface-dark: #f7f4ec;
    --color-on-surface-dark-strong: #fffdf8;
    --color-primary-dark: #68f0c8;
    --color-on-primary-dark: #07120f;
    --color-secondary-dark: #18d6a7;
    --color-on-secondary-dark: #07120f;
    --color-outline-dark: rgba(247, 244, 236, 0.14);
    --color-outline-dark-strong: rgba(247, 244, 236, 0.26);
    --radius-radius: var(--radius-lg);
  }
}
```

This keeps the theme local to Manja without requiring a Goshtoso release. It
also keeps all existing Goshtoso semantic classes, such as `bg-surface`,
`text-on-surface`, `border-outline`, and `text-primary`, working unchanged.

## Components And Behavior

### Layout Theme State

`internal/web/templates/layout.templ` should use a single default constant or
helper value, `manja`, in all theme initialization paths:

- The server-rendered `html` attribute should start as `data-theme="manja"`.
- Alpine state should initialize with `localStorage.getItem('theme') || 'manja'`.
- The early anti-flash script should apply
  `localStorage.getItem('theme') || 'manja'`.

Stored user preferences still win. If a reader has previously selected
`goshtoso`, the page should continue rendering with `data-theme="goshtoso"`.

### Theme Picker

`internal/web/templates/public.templ` should add `Manja` as the first theme
option and mark it selected:

```go
{Value: "manja", Label: "Manja", Selected: true}
```

The existing `Goshtoso`, `Minimal`, `Modern`, `Arctic`, and other built-in
options stay in the same picker. The picker remains visible at the current
breakpoints and continues to use the existing Goshtoso select component.

### Dark Mode

The existing single-icon dark-mode toggle stays unchanged. The new `manja`
theme provides dark token values so the current `dark:` utility classes keep
working when the user toggles dark mode or when system dark preference is used.

## Error Handling And Edge Cases

- Invalid or unknown stored theme values are not introduced by this change. The
  existing behavior simply applies the stored string to `data-theme`; this spec
  does not add validation because the current picker only writes known option
  values.
- If `/manja-assets/manja.css` fails to load, the page falls back to Goshtoso's
  compiled default token set. That is acceptable because the local asset server
  already tests static asset delivery.
- The theme picker dropdown must keep rendering outside the header clipping
  boundary. The existing `.manja-docs-header { overflow: visible; }` rule must
  remain in place.

## Testing

Use test-first implementation.

- Update `internal/web/public_test.go` so rendering public docs proves:
  - the static markup uses `data-theme="manja"`;
  - Alpine and the early script default to `manja`;
  - the theme picker includes `value:'manja'`;
  - `Manja` is selected;
  - `value:'goshtoso'` and the other built-in Goshtoso options are still
    present.
- Add or update a static CSS assertion proving `internal/web/static/manja.css`
  defines `[data-theme=manja]`, the teal primary token, the warm surface token,
  and dark theme tokens.
- Keep the existing E2E dropdown clipping regression valid; it should still
  click `#manja-theme-trigger` and verify the listbox is not clipped.
- Run `go run github.com/a-h/templ/cmd/templ generate` after `.templ` edits.
- Run focused tests first, then `go test ./...`.

## Approval

The approved direction is option A, Branded Workbench. It keeps Manja as the
default branded theme while preserving the built-in Goshtoso themes in the
picker.
