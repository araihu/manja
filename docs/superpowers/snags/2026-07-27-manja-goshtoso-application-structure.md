# Manja Goshtoso Application Structure Snag Ledger

Date: 2026-07-27
Scope: public documentation and management application-structure slice

Record every Goshtoso source dive, missing public API, CSS escape hatch,
generation slowdown, or workaround encountered during this slice. Public
consumer guidance and exported APIs are the authority; this ledger does not
authorize edits to Goshtoso or access to private/generated upstream output.

## 2026-07-27 — dependency-sensitive module gates

- Desired contract: resolve Manja and every nested consumer exactly as CI and a
  fresh external consumer resolve them.
- Public source consulted: `AGENTS.md`, the approved implementation plan, and
  the installed `using-goshtoso` consumer skill.
- Source dive or missing API: the primary checkout has an ignored, user-owned
  `go.work` using Go 1.26.1. It can silently override the tagged Goshtoso
  dependency and the required Go 1.26.5 floor.
- Workaround or no-match decision: do not edit or copy that workspace file.
  Use `GOWORK=off` for dependency-sensitive root, `site/`, and external-module
  gates as appropriate, and verify the resolved module plus Go version
  explicitly.
- Risk: a locally green build could consume the wrong Goshtoso checkout or Go
  toolchain and fail in CI or for external consumers.
- Upstream feedback candidate: none; this is a downstream workspace-isolation
  requirement.

## 2026-07-27 — immutable full dependency fallback manifest

- Desired contract: a public immutable ordered manifest for the full
  `head.Dependencies()` fallback stack, paired with an exact library-version
  API for the approved future hybrid design.
- Public source consulted: Goshtoso v0.0.13 `using-goshtoso` guidance,
  `components-reference.md`, and the frozen head-dependency fallback audit.
- Source dive or missing API: Goshtoso v0.0.13 does not expose the complete
  immutable ordered fallback manifest plus exact library-version API required
  by that later design.
- Workaround or no-match decision: treat this as a downstream blocker for the
  later hybrid design. Preserve the current public `head.Dependencies()`
  contract; do not scrape private or generated output and do not edit upstream
  Goshtoso in this UI slice.
- Risk: attempting the later hybrid design now would couple Manja to private
  implementation details and break versioned fallback integrity.
- Upstream feedback candidate: expose a documented immutable full dependency
  manifest and exact library-version API in a separately reviewed Goshtoso
  release.

## 2026-07-27 — public detail workspace composition examples

- Desired contract: compose selected public documentation identity, loading,
  empty, and neutral workspace regions from the documented `PageHeader`,
  `Panel`, `EmptyState`, and `Skeleton` APIs without relying on
  component-private classes.
- Public source consulted: `go doc` for the four Goshtoso v0.0.13 component
  `Config` types and the public application-pattern example at
  `examples/application-patterns/views.templ`.
- Source dive or missing API: read the public example implementation to confirm
  supported component composition after the component reference documented the
  fields but not the complete detail-workspace call shape.
- Workaround or no-match decision: use only exported configuration fields and
  Manja-owned semantic hooks; do not copy example-only CSS or inspect generated
  or private component output.
- Risk: low. The implementation remains on the documented public API, but the
  extra example lookup is consumer friction that should remain visible.
- Upstream feedback candidate: add a compact, copyable detail-workspace
  composition to the consumer component reference so downstream users do not
  need to inspect the example source.

## 2026-07-27 — HTMX loading indicator display boundary

- Desired contract: keep the selected public detail visible until navigation
  begins, then expose an accessible skeleton within the same stable workspace.
- Public source consulted: Goshtoso `Skeleton` configuration documentation and
  the existing Manja HTMX swap contract.
- Source dive or missing API: no missing Goshtoso API; the library does not own
  consumer-specific HTMX indicator display timing.
- Workaround or no-match decision: add a narrowly scoped Manja CSS rule for
  `#public-docs-loading` and the standard `htmx-request` state. All color and
  geometry continue to come from Goshtoso primitives and semantic tokens.
- Risk: the indicator depends on HTMX's documented request-state class, which
  is already part of the application navigation contract.
- Upstream feedback candidate: none; this is an application-owned integration
  boundary rather than a Goshtoso component gap.

## 2026-07-27 — sidebar selection after an out-of-shell HTMX swap

- Desired contract: URL, detail identity, focus destination, visual selection,
  and `aria-current` remain identical after direct load, HTMX navigation, Back,
  and Forward without copying Goshtoso's private classes into JavaScript.
- Public source consulted: Goshtoso Sidebar public item attributes and Manja's
  frozen selective-focus and HTMX history contracts.
- Source dive or missing API: the v0.0.13 Sidebar does not expose a public
  client-side selection-state controller for a sidebar that lives outside the
  consumer's swap target.
- Workaround or no-match decision: the fragment emits authoritative
  `data-selected-doc` and document-title values; one small handler copies only
  those semantic values, toggles `aria-current`, and focuses the explicit
  settled heading. Manja-owned CSS styles the semantic selection state after
  the post-Goshtoso stylesheet boundary.
- Risk: selection synchronization remains a small consumer integration surface,
  but it no longer snapshots or mutates Goshtoso implementation class lists.
- Upstream feedback candidate: document an attribute-driven selection update
  contract or provide a replaceable selection subregion for out-of-shell HTMX
  detail swaps.

## 2026-07-27 — hash navigation and the AppShell root scroll container

- Desired contract: exactly one primary scroll owner; direct hash navigation
  must scroll the main workspace without moving the persistent header or shell.
- Public source consulted: the exported AppShell `RootAttrs` hook and the frozen
  browser scroll-owner acceptance test.
- Source dive or missing API: browser evidence showed AppShell's default
  `overflow-hidden` root remained programmatically scrollable, allowing a direct
  hash to shift the entire shell upward by the 65px header height.
- Workaround or no-match decision: mark the consumer root through `RootAttrs`
  and override only that root to `overflow: clip` in post-Goshtoso Manja CSS.
  Main retains `overflow-y: auto` as the sole primary scroll owner.
- Risk: low and browser-locked. `clip` intentionally removes programmatic shell
  scrolling while retaining the AppShell's visual clipping boundary.
- Upstream feedback candidate: consider `overflow-clip` as the AppShell default
  when its contract promises a single scrollable main region.

## Task 4 RED to GREEN evidence

- RED command: `GOWORK=off go test ./internal/web -run
  'TestPublicDocs(SelectedContent|UnknownSelection|LoadingState)' -count=1`.
  Literal failures: `selected detail should expose PageHeader identity
  "operation-createPet"`; unknown selection rendered `operation-listPets`;
  `public loading state missing "data-public-docs-loading=\"true\""`.
- RED command: `GOWORK=off go test ./internal/web/e2e -run
  'TestPublicDocsScrollsMainContentInsideShell' -count=1`. Literal failure:
  `aside should keep the viewport-height navigation rail; want 768 got 703`,
  with `headerRectBottom:0`, `headerRectHeight:65`, and `mainRectTop:0`, proving
  the shell root had scrolled by the header height.
- GREEN commands: `GOWORK=off go test ./internal/web -run
  'TestPublicDocs' -count=1` and `GOWORK=off go test ./internal/web/e2e -run
  'TestPublicDocs' -count=1`; both passed.

## 2026-07-27 — management navigation and tab state after swaps

- Desired contract: server-authored management selection and tab ARIA state
  remain visually accurate without JavaScript copies of Goshtoso private class
  lists.
- Public source consulted: Goshtoso Sidebar item attributes, Table row-link and
  actions documentation, Tabs ARIA output, and the post-Goshtoso consumer CSS
  boundary.
- Source dive or missing API: no private source dive. The existing Manja
  workaround copied Sidebar and Tabs implementation classes into JavaScript.
- Workaround or no-match decision: delete those class maps, toggle only
  `aria-current`, `aria-selected`, `tabindex`, and panel visibility, and style
  those semantic states through narrowly scoped Manja CSS selectors.
- Risk: low and testable. Visual state now follows stable accessibility
  attributes; Alpine reinitialization remains limited to swapped controls.
- Upstream feedback candidate: document semantic CSS hooks for consumers that
  need to keep persistent navigation outside an HTMX swap target.

## 2026-07-27 — Table LinkBoost and fragment negotiation

- Desired contract: a safe native Table row link may use Goshtoso `LinkBoost`
  without blanking the management application.
- Public source consulted: the exported Table `Row.Link`, `Row.Actions`, and
  `LinkBoost` documentation plus Manja's existing public full-document HTMX
  negotiation rule.
- Source dive or missing API: no Goshtoso source dive. Browser RED showed the
  management handler returned a fragment to a `LinkBoost` request whose public
  output uses `hx-target="body"` and `hx-select="body"` without an `HX-Boosted`
  request header, leaving the browser with an empty body.
- Workaround or no-match decision: use documented `LinkFull` for the Table's
  safe native first-cell anchor and keep the persistent Sidebar as the targeted
  HTMX navigation path. The browser test proves both row safety and HTMX
  selected-identity behavior without ambiguous server negotiation.
- Risk: low. Table navigation incurs a full page load, while the first-class
  sidebar HTMX path retains context and history. Both paths remain native-link
  recoverable.
- Upstream feedback candidate: document that `LinkBoost` is body-targeted HTMX,
  not an `hx-boost` request, so server fragment negotiation can recognize it.

## Task 5 RED to GREEN evidence

- RED command: `GOWORK=off go test ./internal/web -run
  'TestManagement(Specs|TableRows|SpecUses|UnknownSpec|SelectedIdentity)'
  -count=1`. Literal failures included `management specs list missing
  "data-management-page-header=\"specs\""` and an unknown request rendering
  `Payments API` instead of preserving `unknown-api`.
- Browser RED command: `GOWORK=off go test ./internal/web/e2e -run
  'TestManagementListFiltersAndSelectedIdentity' -count=1`. Literal failure:
  `management detail did not settle`, with URL changed to
  `/manage/spec/payments-api` while `body:""`; the captured row link used
  `hx-select:"body"` and `hx-target:"body"`.
- GREEN commands: `GOWORK=off go test ./internal/web -run 'TestManagement'
  -count=1` and `GOWORK=off go test ./internal/web/e2e -run 'TestManagement'
  -count=1`; both passed, including filters, HTMX sidebar navigation, focus,
  selected identity, and Back.

## 2026-07-27 — transport recovery composition

- Desired contract: failed management mutations retain the current workspace,
  announce an honest retry state, and expose a 44px retry target without
  inventing a competing application component.
- Public source consulted: Goshtoso Alert and form-error public component
  source plus the documented semantic-token boundary.
- Source dive or missing API: the first Manja draft used non-existent
  `danger-soft` token utilities. The public Goshtoso form-error implementation
  confirmed the supported danger surface contract is `bg-danger/10` (and
  `bg-danger/15` in dark mode).
- Workaround or no-match decision: keep the transport lifecycle in Manja's
  HTMX integration script and use the documented semantic danger classes for
  the persistent recovery region. No Goshtoso source or theme was changed.
- Risk: the retry region depends on documented HTMX lifecycle events; unit and
  browser tests cover send/response failure, retained form values, and retry.
- Upstream feedback candidate: add a transport-recovery example to the
  application-pattern guidance alongside loading and empty states.

## 2026-07-27 — loading-copy test hook

- Desired contract: prove mutation buttons use Goshtoso's ancestor-form HTMX
  loading behavior without asserting generated private markup.
- Public source consulted: Goshtoso v0.0.13 `button.WithLoadingText` docs and
  its public component contract test.
- Source dive or missing API: the option docs describe the behavior but do not
  name the stable rendered test hook. The public test identifies
  `data-goshtoso-loading` as the contract marker.
- Workaround or no-match decision: assert the public marker and user-visible
  loading copy in Manja unit tests; do not inspect or copy generated component
  implementation.
- Risk: low. The assertion follows Goshtoso's own public contract test.
- Upstream feedback candidate: document `data-goshtoso-loading` next to
  `WithLoadingText` in the consumer component reference.

## Task 6 RED to GREEN evidence

- RED command: `GOWORK=off go test ./internal/web -run
  'TestManagement(SyncRejectsUnavailableCandidateWithoutEffect|PublicationFailureRetainsSelectedContractAndValues|UnknownRouteReturnsInShellRecovery|RepeatedSubmissionDoesNotDuplicateEffect)$'
  -count=1 -v`.
- Literal failures: `status = 400, want 200: ref is not available for this
  source`; `status = 400, want 200: persistence unavailable`; `unknown
  management route missing "<!doctype html>": 404 page not found`; and `sync
  effect count = 2, want 1`.
- GREEN command: the same focused command after implementing authoritative
  application-error fragments, outer-route delegation, and bounded
  per-contract idempotency slots; all four tests passed.
