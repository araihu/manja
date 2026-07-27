# Release Tracks And Authenticated Previews Plan Reconciliation Snags

Date: 2026-07-27

Scope: docs-only reconciliation from exact `origin/main`
`23293e9c96cf0a05d5ce5d261fcf5c069b7b741e`.

Use this entry form for every Goshtoso source dive, public API gap, generated
templ issue, or workaround found while reconciling the design and plan:

```text
## YYYY-MM-DD — <surface / component>

- Desired contract:
- Public source consulted:
- Source dive or missing API:
- Workaround or no-match decision:
- Risk:
- Upstream feedback candidate:
```

## 2026-07-27 — exact v0.0.13 consumer checkpoint

- Desired contract: identify the dependency checkpoint already integrated by
  every Manja consumer before planning release-track web work.
- Public source consulted: root `go.mod`, `site/go.mod`, external-module
  fixture, `architecture/goshtoso_dependency_test.go`, Manja layout use of
  `head.Dependencies()`, and Manja public mount of `assets.Handler()`.
- Source dive or missing API: current root, site, and external consumer graphs
  are already guarded to resolve exactly Goshtoso `v0.0.13`, Go `1.26.5` or
  newer, and no Goshtoso replacement. No release-track task needs a dependency
  migration.
- Workaround or no-match decision: retain the exact v0.0.13 gate as a
  characterization and final integration gate; remove stale v0.0.12 migration
  instructions from the release-track plan.
- Risk: re-running the obsolete migration would mix dependency and release
  behavior and could regress the integrated application shell.
- Upstream feedback candidate: none.

## 2026-07-27 — later Goshtoso public runtime manifest

- Desired contract: distinguish release-track requirements from the later
  hybrid offline-runtime prerequisite.
- Public source consulted: Goshtoso `assets/embed.go`, public runtime-manifest
  commit `f38c330f23a9ebee808ac10c5a448fe03807a292`, its consumer guidance, local
  tag inventory, and Manja's approved hybrid Wasm design.
- Source dive or missing API: Goshtoso v0.0.13 tag
  `6e1b94a473d3e6903347c75955b126b980abde32` predates the public
  `assets.DefaultRuntimeManifest()` and `assets.GoshtosoVersion()` contract.
  The manifest work is merged in Goshtoso history, but no tag newer than
  v0.0.13 is present in the inspected tag inventory.
- Workaround or no-match decision: do not block release tracks or authenticated
  previews on this unpublished Goshtoso checkpoint. Record a separately tagged
  exact-version consumer upgrade as a hybrid Wasm prerequisite before offline
  runtime implementation.
- Risk: treating the untagged source commit as the current Manja dependency
  would require a replacement or pseudo-version and violate the exact released
  consumer checkpoint.
- Upstream feedback candidate: publish a Goshtoso release containing the public
  runtime manifest before Manja implements hybrid offline assets.

## 2026-07-27 — AppShell-aware publication base paths

- Desired contract: make every public document, search, download, HTMX, and
  sitemap URL publication-base-aware without replacing the integrated public
  application structure.
- Public source consulted: current `public.templ`, `layout.templ`, public
  handler, application-structure tests, and Goshtoso v0.0.13 AppShell, Sidebar,
  Search, PageHeader, and Panel usage already integrated in Manja.
- Source dive or missing API: current Manja helpers such as
  `selectedDocsHref`, branding home, search configuration, and download links
  assume `/`; Goshtoso does not own Manja publication routing.
- Workaround or no-match decision: extend Manja's existing public render
  options with a validated base path and response policy, then thread those
  options through current AppShell/sidebar/search helpers. Do not fork or
  replace Goshtoso components.
- Risk: a parallel renderer or partial URL rewrite could silently serve the
  startup revision, escape a track base, or desynchronize canonical URL,
  selected identity, focus, and ARIA state.
- Upstream feedback candidate: none; publication routing belongs to Manja.

## 2026-07-27 — external consumer fixture path

- Desired contract: keep the exact Goshtoso consumer gate executable for the
  root module, site module, and external consumer fixture.
- Public source consulted: `architecture/goshtoso_dependency_test.go`,
  `site/go.mod`, and `integration/testdata/external-module/go.mod`.
- Source dive or missing API: there is no top-level `external/` module at the
  reconciled base. The recursive architecture test discovers the external
  consumer at `integration/testdata/external-module/`.
- Workaround or no-match decision: use the literal fixture path in executable
  plan commands and retain the recursive architecture test as the authoritative
  discovery/version/no-replacement gate.
- Risk: a stale top-level path would make the final dependency command fail
  before it tested the intended consumer.
- Upstream feedback candidate: none; this is a Manja plan-path correction.

## 2026-07-27 — scoped management authorization target ordering

- Desired contract: authenticate first, reject unsafe browser mutations before
  body parsing, and authorize the concrete project/track/action before any
  existence lookup or effect.
- Public source consulted: current `internal/web/management.go`,
  `POST /manage/publication`, `POST /manage/sync`, and the reconciled
  management security contract.
- Source dive or missing API: both current handlers call `r.ParseForm()`
  before reading `spec_id`. Requiring scoped authorization before parsing any
  identifier is therefore circular because the authorization target currently
  exists only in the form body.
- Workaround or no-match decision: prefer new scoped routes whose canonical
  target IDs live in the path. During legacy endpoint migration only, perform
  authentication, method/content-type checks, and header-carried CSRF or strict
  Origin/Sec-Fetch validation first; then extract only allowlisted target keys
  from one strictly bounded retained body buffer, authorize those syntactic
  IDs, and defer lookup, remaining form parsing, mutation-slot handling, and
  effects until authorization succeeds.
- Risk: parsing an unbounded or full form before browser protection expands
  attacker-controlled work; authorizing before a target exists is
  unimplementable; looking up a target before scoped authorization leaks
  existence.
- Upstream feedback candidate: none; this is a Manja endpoint-contract
  migration.
