# Manja Goshtoso Application Structure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor Manja's public documentation and management workbench into Goshtoso-native task-oriented application structures while preserving server rendering, static/public boundaries, HTMX navigation, current domain behavior, and last-known-good public output.

**Architecture:** Start from an `origin/main` containing exact predecessor `62ff13b78e45c3fe215770bd5656ad117a5019a4`. Replace hand-built fixed frames with separate public and management `appshell.AppShell` compositions, then migrate identity, list/detail, state, focus, and recovery markup onto `PageHeader`, `Toolbar`, `Panel`, `EmptyState`, and `Skeleton`. Keep handlers authoritative and native-first; JavaScript only synchronizes state that cannot be returned in the same fragment and is locked by browser tests.

**Tech Stack:** Go 1.26.5, templ, Goshtoso v0.0.13, HTMX 2.x, Alpine.js 3.x, Playwright-Go, semantic Manja CSS.

## Global Constraints

- Use a dedicated worktree and branch from current `origin/main`; verify the predecessor is an ancestor before editing.
- Keep Goshtoso exactly `v0.0.13` in root, `site/`, and every nested consumer module; no `replace` or pseudo-version.
- Public docs stay read-only, search-first, server-rendered, and compatible with static/public output.
- Management stays server-rendered and native-first with HTMX enhancement.
- No release-track, authenticated-preview, review, promotion, provider, licensing, Try It, or upstream-proxy behavior.
- Use public Goshtoso APIs only. Record every source dive, missing API, CSS escape hatch, generation slowdown, and workaround.
- Keep one selected identity across URL, detail, focus, selected styling, and ARIA.
- Keep one primary scroll region and one mobile navigation trigger per surface.
- Expected HTMX error/recovery fragments must visibly swap; real transport failure must be visible.
- Generated `*_templ.go` files are regenerated, never hand-edited.
- `site/` gates run with `GOWORK=off`.
- Assurance mode is `progressive_assurance / feature_delivery`. Minimum gates block integration; proof-expanding repeats require a repository quality-debt receipt before deferral.

---

## File Structure

**Create:**

- `internal/web/e2e/application_structure_test.go` — cross-surface shell, selection, mobile geometry, focus, error, and accessibility browser contracts.
- `internal/web/templates/application_structure_test.go` — rendered component/landmark/ID contracts without browser startup.
- `docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md` — source dives and workarounds.
- `internal/web/testdata/ui/README.md` only if deterministic state fixtures need documentation; prefer existing Go fixtures first.

**Modify:**

- `internal/web/templates/layout.templ` — theme/runtime helpers and scoped HTMX settle/error policy; no shell markup.
- `internal/web/templates/layout_templ.go` — generated.
- `internal/web/templates/public.templ` — `AppShell`, public header/sidebar/content composition, PageHeader/Panel/EmptyState/Skeleton states.
- `internal/web/templates/public_templ.go` — generated.
- `internal/web/templates/management.templ` — `AppShell`, Operations List, Detail Workspace, state/action semantics.
- `internal/web/templates/management_templ.go` — generated.
- `internal/web/public.go` and `internal/web/public_test.go` — explicit full-document/history and state models only when rendering requires handler data.
- `internal/web/management.go` and `internal/web/management_test.go` — explicit state/recovery responses, canonical URL, and loading/action contracts only; no domain expansion.
- `internal/web/e2e/public_docs_test.go` — public direct/HTMX/history identity and focus.
- `internal/web/e2e/management_tabs_test.go` — management navigation, selected identity, current tabs compatibility.
- `internal/web/e2e/goshtoso_visual_matrix_test.go` — extend reusable matrix helpers to new state routes rather than duplicate infrastructure.
- `internal/web/static/manja.css` — application-owned docs/tree/example layout and semantic escape hatches only.
- `docs/superpowers/specs/2026-07-27-manja-management-state-action-ledger.md` — mark each tested row with exact test evidence.

**Do not modify:**

- public domain/application APIs;
- storage or source adapters;
- release-track plan implementation files;
- API YAML/generated API types;
- Goshtoso module source.

### Task 1: Freeze Predecessor, Baseline, And Snag Ledger

**Files:**

- Create: `docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md`
- Test: `architecture/goshtoso_dependency_test.go`

**Interfaces:**

- Consumes: `origin/main`, predecessor SHA `62ff13b78e45c3fe215770bd5656ad117a5019a4`, existing recursive Goshtoso dependency gate.
- Produces: clean implementation branch, baseline command manifest, initial component/source inventory, durable snag format.

- [ ] **Step 1: Verify the implementation base**

```bash
git fetch origin
test "$(git merge-base origin/main 62ff13b78e45c3fe215770bd5656ad117a5019a4)" = \
  62ff13b78e45c3fe215770bd5656ad117a5019a4
git status --short --branch
git diff --check
```

Expected: predecessor is an ancestor of `origin/main`; worktree clean. Stop if not integrated.

- [ ] **Step 2: Record baseline dependency and surface inventory**

```bash
go list -m github.com/araihu/goshtoso
(cd site && GOWORK=off go list -m github.com/araihu/goshtoso)
(cd integration/testdata/external-module && GOWORK=off go list -m github.com/araihu/goshtoso)
rg -n 'appshell\.|pageheader\.|toolbar\.|panel\.|emptystate\.|skeleton\.|sidebar\.|hx-|x-data' internal/web/templates internal/web/e2e
```

Expected: all consumers resolve v0.0.13; inventory captures current manual shell and component use.

- [ ] **Step 3: Run baseline gates**

```bash
npm ci
go test ./... -count=1
(cd site && GOWORK=off go test ./... -count=1)
(cd integration/testdata/external-module && GOWORK=off go test ./... -count=1)
```

Expected: PASS; record existing npm audit and API lint baselines separately.

- [ ] **Step 4: Create snag ledger**

Use this exact entry form:

```markdown
## YYYY-MM-DD — <surface / component>

- Desired contract:
- Public source consulted:
- Source dive or missing API:
- Workaround or no-match decision:
- Risk:
- Upstream feedback candidate:
```

- [ ] **Step 5: Commit baseline ledger**

```bash
git add docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md
git commit -m "docs(ui): freeze Goshtoso refactor baseline"
```

### Task 2: Lock Shell And Landmark Contracts Before Markup Changes

**Files:**

- Create: `internal/web/templates/application_structure_test.go`
- Modify: `internal/web/public_test.go`
- Modify: `internal/web/management_test.go`

**Interfaces:**

- Consumes: rendered `PublicDocsWithOptions`, `ManagementOverview`, `ManagementSpecsPage`, and `ManagementSpecPage`.
- Produces: helper assertions for one skip link, one header landmark, one focusable main region, one primary scroll owner, one mobile trigger, and stable workspace IDs.

- [ ] **Step 1: Write failing rendered-document tests**

Add tests named:

```go
func TestPublicDocsUsesOneGoshtosoApplicationShell(t *testing.T)
func TestManagementUsesOneGoshtosoApplicationShell(t *testing.T)
func TestApplicationShellsExposeOneMobileNavigationTrigger(t *testing.T)
func TestApplicationShellsExposeOnePrimaryScrollRegion(t *testing.T)
```

Parse rendered HTML with `golang.org/x/net/html`. Assert structural semantics,
not Goshtoso-owned private class lists:

```go
assertCount(t, doc, `a[href="#main-content"]`, 1)
assertCount(t, doc, `header`, 1)
assertCount(t, doc, `main#main-content[tabindex="-1"]`, 1)
assertCount(t, doc, `button[aria-label="Open API sections"]`, 1)
assertCount(t, doc, `button[aria-label="Open management sections"]`, 1)
assertCount(t, doc, `[data-manja-primary-scroll="true"]`, 1)
```

- [ ] **Step 2: Run tests and verify RED**

```bash
go test ./internal/web/templates ./internal/web \
  -run 'Test(PublicDocs|Management|ApplicationShells).*ApplicationShell|TestApplicationShells' -count=1
```

Expected: FAIL because manual frames do not expose AppShell structure/markers.

- [ ] **Step 3: Add identity contract tests**

```go
func TestManagementSelectedIdentityMatchesURLDetailAndNavigation(t *testing.T)
func TestPublicSelectedIdentityMatchesContentAndNavigation(t *testing.T)
```

Assert `data-selected-contract`, `data-selected-doc`, canonical href, content
heading, and one `aria-current="page"` agree. Do not assert implementation classes.

- [ ] **Step 4: Run focused RED tests**

```bash
go test ./internal/web -run 'Test(Management|Public)SelectedIdentity' -count=1
```

- [ ] **Step 5: Commit tests only**

```bash
git add internal/web/templates/application_structure_test.go internal/web/public_test.go internal/web/management_test.go
git commit -m "test(web): define application shell contracts"
```

### Task 3: Replace Manual Frames With Goshtoso AppShell

**Files:**

- Modify: `internal/web/templates/layout.templ`
- Modify: `internal/web/templates/public.templ`
- Modify: `internal/web/templates/management.templ`
- Generate: `internal/web/templates/layout_templ.go`
- Generate: `internal/web/templates/public_templ.go`
- Generate: `internal/web/templates/management_templ.go`

**Interfaces:**

- Consumes: `appshell.AppShell(appshell.Config)`, `sidebar.Sidebar`, `sidebar.Overlay`, existing theme/mode/search helpers.
- Produces: `publicApplicationShell(...)` and `managementApplicationShell(...)`; each owns one AppShell and provides children to its main region.

- [ ] **Step 1: Import AppShell and create public shell composition**

Use the public API, preserving existing route-specific model ownership:

```templ
@appshell.AppShell(appshell.Config{
    Header: publicApplicationHeader(idx, selected, opts),
    Sidebar: publicDocsSidebar(idx, selected, opts),
    MainID: "main-content",
    SidebarAttrs: templpkg.Attributes{"aria-label": "API sections"},
    MainAttrs: templpkg.Attributes{
        "data-manja-primary-scroll": "true",
        "data-selected-doc": selected.Anchor,
    },
}) {
    @PublicDocsFragmentContent(idx, selectedID, opts)
}
```

Set `DisableSkipLink: true` in nested Sidebar configs. Keep SearchModal outside
the main scroll region. Keep exactly one `sidebar.Overlay` trigger in header.

- [ ] **Step 2: Create management shell composition**

```templ
@appshell.AppShell(appshell.Config{
    Header: managementApplicationHeader(model, active),
    Sidebar: managementSidebar(model, active, ""),
    MainID: "main-content",
    SidebarAttrs: templpkg.Attributes{"aria-label": "Management sections"},
    MainAttrs: templpkg.Attributes{
        "data-manja-primary-scroll": "true",
        "data-selected-contract": model.SelectedSpecID,
    },
}) {
    { children... }
}
```

Do not add a nested `<header>`. Preserve the existing `#management-main-content`
fragment target inside main.

- [ ] **Step 3: Keep mobile drawer viewport-owned**

For both surfaces:

```go
sidebar.OverlayConfig{
    PanelPositionClass:    "fixed top-16 bottom-0 left-0",
    BackdropPositionClass: "fixed top-16 bottom-0 left-0 right-0",
    PanelWidthClass:       "w-72 max-w-[85vw]",
}
```

Identify each trigger through its public `TriggerLabel` accessible name. Do not
add a private-DOM marker or copy component internals merely to simplify tests.

- [ ] **Step 4: Regenerate templates**

```bash
go run github.com/a-h/templ/cmd/templ generate
```

- [ ] **Step 5: Run shell tests**

```bash
go test ./internal/web/templates ./internal/web \
  -run 'Test(PublicDocs|Management|ApplicationShells)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit shell migration**

```bash
git add internal/web/templates/layout.templ internal/web/templates/layout_templ.go \
  internal/web/templates/public.templ internal/web/templates/public_templ.go \
  internal/web/templates/management.templ internal/web/templates/management_templ.go \
  docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md
git commit -m "refactor(web): adopt Goshtoso application shells"
```

### Task 4: Convert Public Docs To A Detail Workspace

**Files:**

- Modify: `internal/web/templates/public.templ`
- Modify: `internal/web/public_test.go`
- Modify: `internal/web/e2e/public_docs_test.go`
- Generate: `internal/web/templates/public_templ.go`

**Interfaces:**

- Consumes: selected `selectedDocsItem`, `PageHeader`, `Breadcrumbs`, `Panel`, `EmptyState`, `Skeleton`.
- Produces: `publicDocsPageHeader`, explicit public loading/empty/error components, synchronized public navigation identity.

- [ ] **Step 1: Add failing semantic tests**

```go
func TestPublicDocsSelectedContentUsesPageHeader(t *testing.T)
func TestPublicDocsUnknownSelectionUsesInShellEmptyState(t *testing.T)
func TestPublicDocsLoadingStateMatchesFinalWorkspaceShape(t *testing.T)
```

Assert one `h1`, visible selected operation/schema identity, in-shell recovery
link, `aria-busy` loading region, and stable main dimensions. No class-list snapshots.

- [ ] **Step 2: Verify RED**

```bash
go test ./internal/web -run 'TestPublicDocs(SelectedContent|UnknownSelection|LoadingState)' -count=1
```

- [ ] **Step 3: Implement PageHeader and neutral Panel composition**

Use `PageHeader` for overview/operation/schema identity. Keep breadcrumbs and
source/version freshness adjacent. Use `Panel` only for neutral regions already
implemented as generic bordered containers. Do not wrap every endpoint section
in Panel or Card.

- [ ] **Step 4: Implement public state components**

Add deterministic template functions:

```go
templ publicDocsLoading(selected selectedDocsItem)
templ publicDocsEmpty(idx core.SpecIndex)
templ publicDocsNotFound(idx core.SpecIndex, requested string)
```

Use `Skeleton` with an accessible label and `EmptyState` with a native link to
the contract overview. Keep search and theme context.

- [ ] **Step 5: Lock direct/HTMX/history identity**

Extend browser tests to assert after direct load, sidebar activation, Back, and
Forward:

```text
location pathname/hash == selected href
data-selected-doc == content identity
one sidebar link aria-current=page
document.activeElement == explicit settled target
document.title names selected content
```

- [ ] **Step 6: Regenerate and run focused tests**

```bash
go run github.com/a-h/templ/cmd/templ generate
go test ./internal/web -run 'TestPublicDocs' -count=1
go test ./internal/web/e2e -run 'TestPublicDocs' -count=1
```

- [ ] **Step 7: Commit public workspace**

```bash
git add internal/web/templates/public.templ internal/web/templates/public_templ.go \
  internal/web/public_test.go internal/web/e2e/public_docs_test.go
git commit -m "refactor(web): structure public docs workspace"
```

### Task 5: Convert Management List And Detail To Task Patterns

**Files:**

- Modify: `internal/web/templates/management.templ`
- Modify: `internal/web/management.go`
- Modify: `internal/web/management_test.go`
- Modify: `internal/web/e2e/management_tabs_test.go`
- Generate: `internal/web/templates/management_templ.go`

**Interfaces:**

- Consumes: current `ManagementOverviewModel`, `ManagedSpecModel`, Table row link/action contract.
- Produces: `managementOverviewHeader`, `managementSpecsToolbar`, `managementSpecHeader`, neutral detail panels, server-authored selected identity.

- [ ] **Step 1: Write Operations List RED tests**

```go
func TestManagementSpecsUsesPageHeaderToolbarAndTable(t *testing.T)
func TestManagementSpecsEmptyAndFilteredEmptyDiffer(t *testing.T)
func TestManagementTableRowsKeepNativeLinkAndActionsSeparate(t *testing.T)
```

Assert native anchors, accessible toolbar name, result count live region,
distinct EmptyState copy/actions, and no row-level click handler when actions exist.

- [ ] **Step 2: Write Detail Workspace RED tests**

```go
func TestManagementSpecUsesPageHeaderAndDominantWorkspace(t *testing.T)
func TestManagementUnknownSpecKeepsApplicationShell(t *testing.T)
func TestManagementSelectedIdentityIsServerAuthored(t *testing.T)
```

- [ ] **Step 3: Verify RED**

```bash
go test ./internal/web -run 'TestManagement(Specs|TableRows|SpecUses|UnknownSpec|SelectedIdentity)' -count=1
```

- [ ] **Step 4: Implement list pattern**

Use `PageHeader` for “Managed contracts/specs”, `Toolbar` for search/filter
controls, and Table for comparable rows. Keep the selected destination a native
link. Render `aria-live="polite"` result count outside the table. Use
`EmptyState` for zero managed specs and separate filtered-empty recovery.

- [ ] **Step 5: Implement detail pattern**

Use `PageHeader` for title/version/status/actions. Preserve current publication,
sync, diff, settings, history, and details behavior; migrate generic wrappers to
`Panel`. Keep metadata subordinate. Do not add future Candidates/Tracks/Reviews
navigation in this slice.

- [ ] **Step 6: Remove optimistic sidebar/table selection**

Prefer server-rendered selected navigation in the same `#management-main-content`
response. If sidebar remains outside the target, one settle handler may update
URL-matched state only after success. It must set/remove ARIA, visible selected
state, and focus together. Delete duplicated class maps copied from component
internals.

- [ ] **Step 7: Regenerate and run focused tests**

```bash
go run github.com/a-h/templ/cmd/templ generate
go test ./internal/web -run 'TestManagement' -count=1
go test ./internal/web/e2e -run 'TestManagement' -count=1
```

- [ ] **Step 8: Commit management patterns**

```bash
git add internal/web/templates/management.templ internal/web/templates/management_templ.go \
  internal/web/management.go internal/web/management_test.go \
  internal/web/e2e/management_tabs_test.go
git commit -m "refactor(web): structure management workbench"
```

### Task 6: Make Loading, Error, Focus, And History Executable

**Files:**

- Modify: `internal/web/templates/layout.templ`
- Modify: `internal/web/management.go`
- Modify: `internal/web/management_test.go`
- Create: `internal/web/e2e/application_structure_test.go`
- Modify: `docs/superpowers/specs/2026-07-27-manja-management-state-action-ledger.md`
- Generate: `internal/web/templates/layout_templ.go`

**Interfaces:**

- Consumes: `#main-content`, `#management-main-content`, response marker `[data-autofocus]`, current sync/publication forms.
- Produces: scoped `htmx:afterSettle`, explicit expected-error swap policy, real transport failure recovery, held-request loading proof, PRG/freshness tests.

- [ ] **Step 1: Write handler tests for every ledger row**

Name tests by state/action, not by implementation helper:

```go
func TestManagementSyncRejectsUnavailableCandidateWithoutEffect(t *testing.T)
func TestManagementPublicationFailureRetainsSelectedContractAndValues(t *testing.T)
func TestManagementUnknownRouteReturnsInShellRecovery(t *testing.T)
func TestManagementRepeatedSubmissionDoesNotDuplicateEffect(t *testing.T)
```

Use fake action/store counters. Assert exact effect count and rendered context.

- [ ] **Step 2: Verify RED where current responses are bare or lose context**

```bash
go test ./internal/web -run 'TestManagement(SyncRejects|PublicationFailure|UnknownRoute|RepeatedSubmission)' -count=1
```

- [ ] **Step 3: Implement explicit error swap contract**

Choose one scoped policy and document it in code:

```go
// Expected application validation/recovery renders HTML with HTTP 200 and
// X-Manja-Application-Status. Unexpected 5xx remains a transport/server error.
```

Or install a target-scoped `htmx:beforeSwap` listener keyed by an explicit
response header. Never enable all 4xx/5xx swaps globally.

- [ ] **Step 4: Implement selective settle focus**

```js
document.addEventListener('htmx:afterSettle', function (event) {
  var target = event.detail && event.detail.target;
  if (!target || !target.matches('#main-content, #management-main-content')) return;
  var focus = target.querySelector('[data-autofocus="true"]');
  if (focus) focus.focus();
});
```

Navigation responses mark a heading/main target. Search/filter responses do not.

- [ ] **Step 5: Add held-request and transport browser tests**

Intercept the real management mutation request. Assert:

```text
button loading text visible
submitter disabled while pending
second activation produces no second request
success/error resolves to authoritative selected workspace
aborted request renders visible retry and restores usable control
```

- [ ] **Step 6: Add Back/refresh freshness tests**

After a successful native or HTMX mutation, exercise Back, Forward, and refresh.
Compare rendered revision/publication state to a fresh server read. Add
`Cache-Control: no-store` only to management task documents whose browser cache
cannot be trusted.

- [ ] **Step 7: Update ledger evidence and run focused tests**

```bash
go run github.com/a-h/templ/cmd/templ generate
go test ./internal/web -run 'TestManagement' -count=1
go test ./internal/web/e2e -run 'Test(ApplicationStructure|Management)' -count=1
```

- [ ] **Step 8: Commit state contracts**

```bash
git add internal/web/templates/layout.templ internal/web/templates/layout_templ.go \
  internal/web/management.go internal/web/management_test.go \
  internal/web/e2e/application_structure_test.go \
  docs/superpowers/specs/2026-07-27-manja-management-state-action-ledger.md
git commit -m "fix(web): make management recovery authoritative"
```

### Task 7: Lock Responsive, Theme, Keyboard, And Accessibility Acceptance

**Files:**

- Modify: `internal/web/e2e/application_structure_test.go`
- Modify: `internal/web/e2e/goshtoso_visual_matrix_test.go`
- Modify: `internal/web/static/manja.css`
- Modify: `docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md`

**Interfaces:**

- Consumes: reusable Playwright server/fixture helpers and theme/mode controls.
- Produces: saved evidence for each changed archetype and state, selector-existence checks, zero-P1 accessibility result.

- [ ] **Step 1: Add eight-cell matrix table**

Test public detail, management list, management detail, unknown route, empty,
error, loading, and success at:

```go
[]struct{ Width int; Theme, Mode string }{
  {390, "goshtoso", "light"}, {390, "goshtoso", "dark"},
  {390, "minimal", "light"},  {390, "minimal", "dark"},
  {1440, "goshtoso", "light"}, {1440, "goshtoso", "dark"},
  {1440, "minimal", "light"},  {1440, "minimal", "dark"},
}
```

Use 844 px mobile height and 900 px desktop height.

- [ ] **Step 2: Add geometry assertions**

At 390 px assert document width fits viewport. Open the only mobile trigger and
assert trigger at least 44x44, panel/backdrop positive, panel top at or below
header bottom, first/last controls reachable, Escape closes and restores focus.

At 1440 px assert one vertical scroll owner and stable sidebar/main geometry.

- [ ] **Step 3: Add keyboard and selection journey**

Keyboard only: focus skip link, activate main, open/close drawer, invoke search,
navigate results, select management contract, traverse current tabs/forms, hold
mutation, complete recovery. Assert active element after every settle.

- [ ] **Step 4: Add console/network/page-error classifier**

Normal mode permits no resource failures. Forced dependency fallback permits
only exact intercepted versioned Goshtoso primary URLs/status 503, using the
existing URL-keyed multiset validator. First-party resource failure, arbitrary
console error, page error, and unhandled rejection remain blocking.

- [ ] **Step 5: Run accessibility and CSS boundary checks**

Run axe or existing equivalent and fail P1 findings. For every new application
utility, inspect `/manja-assets/manja.css` or `/assets/styles.css`; if absent,
add semantic application CSS and record the escape hatch.

- [ ] **Step 6: Run matrix and save artifacts**

```bash
go test ./internal/web/e2e \
  -run 'Test(ApplicationStructure|GoshtosoAffectedSurfaceVisualMatrix)' \
  -count=1 -timeout 15m
```

Expected: all matrix cells and state journeys PASS; screenshots stored in the
test artifact directory or report path.

- [ ] **Step 7: Commit acceptance coverage**

```bash
git add internal/web/e2e/application_structure_test.go \
  internal/web/e2e/goshtoso_visual_matrix_test.go internal/web/static/manja.css \
  docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md
git commit -m "test(web): lock application structure acceptance"
```

### Task 8: Run Combined Gates And Freeze The Integration Candidate

**Files:**

- Modify: `docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md`
- Regenerate: all affected `*_templ.go`, API bundle only as an untracked validation artifact, example bundles.

**Interfaces:**

- Consumes: complete implementation branch.
- Produces: clean exact head, completion envelope, classified deferred assurance with receipts.

- [ ] **Step 1: Run generation twice and compare manifests**

```bash
go run github.com/a-h/templ/cmd/templ generate
npm run examples:build
npm run api:bundle
find internal/web/templates internal/web/static -type f \
  \( -name '*_templ.go' -o -name 'schema-example.js' -o -name 'request-composer.js' \) \
  -print0 | sort -z | xargs -0 shasum -a 256 > /tmp/manja-ui-pass1.sha256

go run github.com/a-h/templ/cmd/templ generate
npm run examples:build
npm run api:bundle
find internal/web/templates internal/web/static -type f \
  \( -name '*_templ.go' -o -name 'schema-example.js' -o -name 'request-composer.js' \) \
  -print0 | sort -z | xargs -0 shasum -a 256 > /tmp/manja-ui-pass2.sha256
diff -u /tmp/manja-ui-pass1.sha256 /tmp/manja-ui-pass2.sha256
```

- [ ] **Step 2: Run minimum module gates**

```bash
go test ./... -count=1
go test -race ./internal/web/... ./architecture -count=1
go vet ./...
go build ./...
go mod tidy -diff

(cd site && GOWORK=off go test ./... -count=1)
(cd site && GOWORK=off go vet ./...)
(cd site && GOWORK=off go build ./...)
(cd site && GOWORK=off go mod tidy -diff)

(cd integration/testdata/external-module && GOWORK=off go test ./... -count=1)
(cd integration/testdata/external-module && GOWORK=off go vet ./...)
(cd integration/testdata/external-module && GOWORK=off go build ./...)
(cd integration/testdata/external-module && GOWORK=off go mod tidy -diff)
```

- [ ] **Step 3: Run API, examples, and exact dependency gates**

```bash
npm run api:lint
npm run examples:test
go test ./architecture -run Goshtoso -count=1
git diff --check
```

Expected: API lint has only the frozen three-warning baseline; examples 14/14;
all consumers exact v0.0.13.

- [ ] **Step 4: Stress-test Air if source/generation paths changed**

```bash
npm run dev
```

Wait for initial settle. Make one byte-identical `.templ` rewrite, confirm one
settled rebuild, 15 seconds idle without a loop, and HTTP 200 for product site
and Air proxy. Stop the process tree afterward.

- [ ] **Step 5: Classify assurance**

Delivery blockers remain blocking. Repeated stress, full-repository race beyond
changed paths, optional platform/provider acceptance, or an extra review wave
may defer only after a GitHub issue or `.control-plane/quality-debt.json` receipt
records exact SHA, skipped gate, risk, paths, green evidence, acceptance,
trigger, and owner.

- [ ] **Step 6: Finalize ledger and commit**

```bash
git add docs/superpowers/snags/2026-07-27-manja-goshtoso-application-structure.md
git commit -m "docs(ui): close application structure evidence"
git status --short --branch
git log --oneline origin/main..HEAD
```

Expected: clean worktree, exact local branch head, no push/merge/deploy/release.

## Final Review Gate

Use one primary independent review over the exact frozen range. If it finds a
delivery blocker, perform one correction wave and one scoped re-review. After
that, keep actual current-path defects blocking and track proof expansion as
quality debt. Reviewer must inspect:

- literal owned-path diff and generated-source provenance;
- AppShell landmark and scroll ownership;
- selected identity across direct/HTMX/Back/Forward;
- native link/form semantics and exact side-effect counts;
- expected-error and real-transport recovery;
- mobile drawer geometry and Escape/focus;
- full appearance/state matrix, console, overflow, keyboard, and accessibility;
- exact Goshtoso v0.0.13 resolution and CSS boundary;
- complete snag ledger and absence of downstream roadmap scope.
