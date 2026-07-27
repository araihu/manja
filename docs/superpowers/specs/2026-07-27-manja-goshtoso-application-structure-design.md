# Manja Goshtoso Application Structure Design

Date: 2026-07-27
Status: Approved checkpoint derived from the accepted Goshtoso agent-quality handoff

## Summary

Refactor Manja's public documentation and management workbench around Goshtoso's
application-level contracts. This is a structural UI migration, not a theme
rewrite. Server-rendered routes, static public output, HTMX navigation, current
domain behavior, and the read-only public renderer remain intact.

The slice lands after the Goshtoso v0.0.13 migration and before release tracks
and authenticated previews. It removes duplicated page scaffolding, establishes
one application shell and scroll owner per surface, and makes selection,
navigation, loading, recovery, and focus behavior explicit enough for later
release-track workflows to extend without rebuilding the frame.

## Goals

1. Use `appshell.AppShell` as the structural frame for public and management
   surfaces.
2. Use `pageheader.PageHeader`, `toolbar.Toolbar`, `panel.Panel`,
   `emptystate.EmptyState`, and `skeleton.Skeleton` where their semantics match.
3. Keep public docs read-only, search-first, server-rendered, and suitable for
   static/public delivery.
4. Keep management navigation server-rendered with native links and HTMX
   fragment enhancement.
5. Make one selected contract/spec identity authoritative across URL, sidebar or
   table state, detail content, visual selection, focus, and ARIA state.
6. Model loading, empty, filtered-empty, error, partial, stale, success, and
   unknown-route recovery states before adding release-track actions.
7. Preserve task and context across HTMX errors, Back/Forward, refresh, and
   interrupted requests.
8. Validate the full 390/1440, Goshtoso/Minimal, light/dark matrix with keyboard,
   console, overflow, and accessibility evidence.
9. Record every Goshtoso source dive, missing API, CSS escape hatch, generation
   slowdown, or workaround in a durable snag ledger.

## Non-Goals

- No release-track, preview-authentication, review, promotion, or policy domain
  behavior.
- No public Try It console and no upstream request proxy.
- No conversion to a client-side SPA.
- No new theme or product identity.
- No speculative dashboard metrics, card gallery, gradients, glass effects, or
  decorative hero content.
- No custom replacement for Goshtoso primitives that already match the task.
- No direct edits to generated `*_templ.go` files.

## Frozen Inputs

- Predecessor candidate:
  `62ff13b78e45c3fe215770bd5656ad117a5019a4`.
- Goshtoso dependency: exactly `github.com/araihu/goshtoso v0.0.13`.
- Goshtoso guidance checkpoint: merge `196b3ff517bcc9d6644caf7c429764d7daef15e9`
  plus the published v0.0.13 head-dependency contract.
- Existing public and management routes and handler semantics.
- Approved contract-release control-plane design.

Implementation starts only from an `origin/main` that contains the predecessor.
The old `codex/release-tracks-previews` plan worktree is not an implementation
base and must not be reused as a writer.

## Surface Briefs

### Public Documentation

```text
Primary user and task: API consumer locating an operation or schema and reading exact contract documentation
Usage scene and constraints: desktop or mobile browser; direct links and keyboard search; public and cacheable where existing policy permits
Product register: product documentation
Archetype: App Shell plus Detail Workspace
Information priority: selected operation/schema identity > contract content > source/version freshness > navigation context
Navigation model: native document links enhanced by HTMX fragments; full document on direct load/history restore
Consequential states: loading, missing route, unavailable content, stale freshness metadata, success
Existing identity: Manja branding and Goshtoso semantic tokens
Density: compact navigation, standard reading surface
Motion: restrained; only navigation/state change
Visual direction: precise documentation workbench, stable columns, restrained dividers, no decorative cards
Chosen primitives: AppShell, Sidebar/Overlay, Search, PageHeader, Breadcrumbs, Panel, Table, Badge, EmptyState, Skeleton
```

### Management Workbench

```text
Primary user and task: contract maintainer selecting a managed API, checking source/publication state, and executing a safe sync or publication action
Usage scene and constraints: desktop-first operational work; mobile remains fully usable; keyboard and HTMX navigation
Product register: product
Archetype: App Shell plus Operations List plus Detail Workspace
Information priority: selected contract identity > actionable state > source/publication evidence > history and secondary facts
Navigation model: native links enhanced by one server-rendered HTMX workspace swap
Consequential states: loading, empty, filtered-empty, unknown record, stale, partial, validation error, transport error, success
Existing identity: Manja branding and Goshtoso semantic tokens
Density: compact
Motion: restrained; no ornamental animation
Visual direction: one dense contract list and one dominant workspace, subordinate metadata rail, semantic state color only
Chosen primitives: AppShell, Sidebar/Overlay, PageHeader, Toolbar, Table, Panel, Badge, Button, Link, EmptyState, Skeleton
Invariant ledger: docs/superpowers/specs/2026-07-27-manja-management-state-action-ledger.md
```

## Structural Design

### Shell Ownership

Each rendered document contains:

1. one skip link;
2. one Goshtoso `AppShell`;
3. one persistent top region inside AppShell's header landmark;
4. one desktop sidebar and one mobile overlay representation of the same
   navigation model;
5. exactly one primary scroll region;
6. one `main` target with a stable focusable ID;
7. overlays, search, and transient feedback outside the scroll-clipped content.

Do not nest a `<header>` inside `AppShell.Header`. Disable the Sidebar skip link
because AppShell owns it. The mobile overlay panel remains viewport-owned with
`fixed top-16 bottom-0`, adjusted only if the actual header height changes.

Public and management shells share small theme, mode, focus, and HTMX helpers,
but keep their navigation/view models separate. Avoid a generic shell builder
that hides route semantics.

### Page Structure

Public detail pages use `PageHeader` for the selected operation, schema, or
overview identity. Existing endpoint/schema content remains in its current
semantic sections. `Panel` replaces only neutral hand-built containers; tables,
accordions, code blocks, and request examples retain their specialized
components.

Management list routes use `PageHeader` plus `Toolbar` above the contract table.
The selected-contract route uses `PageHeader` plus a dominant detail workspace
and subordinate facts/history regions built from `Panel`. Existing peer tabs may
remain during this slice only where they still represent current behavior; the
refactor must not invent the future release-track IA. It must remove duplicate
header and surface scaffolding so the later IA migration can replace content
without replacing the shell.

### Selection Integrity

For management contract navigation, these representations must always name the
same contract after direct load, HTMX navigation, Back, and Forward:

- canonical URL;
- server-rendered detail key;
- active/focused target after settle;
- selected sidebar or table link styling;
- `aria-current="page"` or `aria-selected="true"`.

Prefer returning the collection and detail workspace from the same server
rendered fragment. If the collection remains outside the swap, update all
representations in one small documented handler and lock them with browser
tests. No optimistic selected style before a successful response.

Public documentation keeps the same rule for selected operation/schema:
canonical URL, content anchor, sidebar current state, document title, and focus
must agree.

## HTMX And Native Semantics

- Every GET destination remains a real `<a href>`; button appearance uses
  `link.WithAppearance(link.AppearanceButton)` where needed.
- Mutations remain real forms and buttons.
- Routine navigation swaps one named workspace and pushes the canonical URL.
- Full direct loads and `HX-History-Restore-Request` return complete documents.
- Public pages retain `hx-history="false"` while expanded Alpine DOM snapshots
  are unsafe to restore.
- Expected validation/recovery fragments must visibly swap. Use a deliberate
  2xx fragment contract with an application-status header or a scoped
  `htmx:beforeSwap` policy; never rely on HTMX swapping arbitrary non-2xx by
  default.
- `htmx:afterSettle` moves focus only when the response marks an explicit
  target. Search and filter requests retain focus and caret.
- A real transport failure renders visible in-shell retry UI and cannot leave a
  perpetual loading state.
- Form-owned HTMX requests use `button.WithLoadingText` and
  `hx-disabled-elt="find button[type='submit']"` where a mutation is present.
- Native POST success uses Post/Redirect/Get when the resulting state is a
  durable document. Back/refresh must compare against authoritative server
  state; sensitive management task pages use `Cache-Control: no-store` or an
  equivalent explicit refresh policy.

## State And Action Contract

The companion management ledger enumerates every current sync/publication
action from available, stale, partial, failed, and terminal states. Every row
records allowed/denied, HTTP and swap result, retained selected identity and
input, focus/destination, and effect count.

This UI slice does not change domain transition rules. It proves current actions
remain server-authoritative, visible during loading, retryable after transport
failure where safe, and deduplicated. Future release-track promotion extends the
ledger instead of replacing it.

## CSS Boundary

- Use Goshtoso component markup and guaranteed recipe utilities first.
- Use `var(--color-*)` and semantic utilities for application-owned CSS.
- Status copy uses contrast-safe `text-*-text` and dark counterparts; filled
  actions use component tones.
- Keep `/manja-assets/manja.css` for genuinely Manja-specific documentation
  layout, markdown, schema trees, and request examples.
- Do not add arbitrary utilities unless verified in delivered CSS or emitted by
  an explicit Manja Tailwind build.
- No broad shadows on structural panels. Borders express structure; elevation
  expresses real overlay layers only.
- Every form control boundary, selected state, focus ring, and status label must
  remain readable in both themes and color modes.

## Responsive And Accessibility Contracts

At 390 px:

- exactly one primary mobile navigation trigger per surface;
- trigger target at least 44 by 44 CSS pixels;
- open drawer intersects viewport with positive dimensions, begins below the
  header, closes on Escape, and restores focus and truthful `aria-expanded`;
- no document horizontal overflow; specialized tables/code blocks may own one
  local horizontal scroller;
- detail rails follow main content;
- actions and long labels remain visible and do not cover focused content.

At 1440 px:

- sidebar remains fixed-width;
- one main region owns vertical scrolling;
- dominant work/content region stays visually primary;
- secondary metadata does not become an equal card column.

Every route has one `h1`. Skip-link focus is immediately visible. Keyboard
traversal, direct load, HTMX, Back, Forward, refresh, unknown route, loading,
empty, error, and success paths are covered. Axe or equivalent must report no
P1 issue.

## Acceptance Matrix

Every changed responsive composition is inspected in all eight cells:

- 390 Goshtoso light/dark;
- 390 Minimal light/dark;
- 1440 Goshtoso light/dark;
- 1440 Minimal light/dark.

Required states: loading, empty, filtered-empty where relevant, error, partial
or stale where available, unknown route, and success. Required evidence:

- saved screenshots;
- bounding boxes and overflow measurements;
- selected identity assertions;
- keyboard/focus trace;
- console and page-error cleanliness;
- accessibility scan;
- held-request loading/deduplication proof;
- real transport failure and recovery;
- CSS selector existence for every new application utility.

## Snag Contract

Record snags in
`docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md`.
Each entry includes date, surface, desired public API, lookup/source opened,
workaround or no-match decision, risk, and upstream feedback candidate.

## Delivery Position

This checkpoint follows the exact v0.0.13 migration and precedes release tracks
and authenticated previews. Release-track implementation must start from the
integrated UI checkpoint, rerun its existing plan against current public
packages, and preserve the shell, state, focus, and error contracts here.
