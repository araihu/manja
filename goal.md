# Manja Open Core Product Goal

Updated: 2026-08-11

## Objective

Deliver Manja as a complete Open Core, self-hosted OpenAPI renderer and
publisher. Coordinate three independent Codex tasks: this product-management
task, one implementation task, and one read-only review task.

This file is the repository-visible recovery summary. External control-plane
YAML remains the lifecycle and ownership authority. Machine-specific worktree
paths stay in that external ledger rather than this committed record.

## Product Boundary

Open Core includes:

- self-hosted renderer and catalog;
- offline `manja check` workflows;
- public `domain`, `application`, `application/port`, and `contracttest` APIs;
- self-hosted source, storage, rendering, and composition adapters;
- provenance, licensing, notices, SBOMs, reproducible packages, and operator
  documentation;
- hybrid SSR/Wasm public documentation, Service Worker, offline storage,
  recovery, rollback, performance gates, and kill switch.

Deferred hosted SaaS scope includes:

- hosted accounts, tenants, billing, entitlements, or marketplaces;
- hosted authentication and multi-user management;
- connected GitHub review as a hosted service;
- hosted release promotion, authenticated preview, and publication control.

Provider-neutral seams may support later hosted products. Open Core work must
not implement hosted product behavior speculatively.

## Lanes

### Product manager and integration coordinator

- Task: `019fef00-0495-7841-b442-031451ebb185`
- Branch: `coord/opencore-product`
- Worktree label: product-manager integration worktree
- Base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Owns scope, priorities, checkpoint acceptance, integration order, PR state,
  CodeQL/CodeRabbit triage, and durable recovery updates.

### Developer

- Task: `019fef17-2fad-73d2-b004-d3706d36ea82`
- Worktree label: dedicated Open Core developer worktree
- Initial lane base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Current checkpoint base: `811b34b311c29dd60a70d6c88b0c0ec155ffbf12`
- Current checkpoint base tree: `d947f03358248aeb51c851ac0d9da93045f7a047`
- Branch: `codex/oc01-final-merge-gate-cleanliness`
- Owns only bounded Open Core implementation checkpoints assigned here.
- Must commit each coherent checkpoint with a meaningful message.
- Must not implement deferred SaaS behavior or edit the active Arai Hû theme
  rollout.

### Independent reviewer

- Task: `019fef17-2faa-7620-b95c-ba6dc0343094`
- Worktree label: independent read-only review worktree
- Base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Read-only unless the product manager explicitly assigns a separate correction
  checkpoint.
- Reviews exact base/head identities, behavior, tests, scope, provenance, and
  Open Core boundary independently from developer conclusions.

## Operating Rules

1. Branch every unit from fetched, verified `origin/main` in an isolated
   worktree. Preserve the shared primary checkout and unrelated worktrees.
2. Developer commits coherent checkpoints often. Product manager integrates
   accepted checkpoints often; do not accumulate one massive merge block.
3. Freeze base, head, tree, status, and test evidence for every review.
4. Reviewer verdict binds one exact candidate identity. Any correction requires
   a fresh review of the new identity.
5. Push, PR, merge, release, publication, deployment, and cleanup remain
   separate lifecycle actions. Perform only actions explicitly in this goal or
   later authorized by the user.
6. PR must be checked for CodeQL and CodeRabbit presence. Investigate findings
   against current behavior. Fix findings that materially apply; record why
   false positives or irrelevant suggestions do not justify code changes.
7. Record Goshtoso source lookup, generated templ friction, component gaps, or
   dependency friction in the applicable snag ledger.
8. Keep this file current after every accepted checkpoint, lane replacement,
   PR transition, or important blocker.

## Current Evidence

- Audited OC-01 and PR #94 base:
  `39d65ade21c080ee2102f53da5ed741f000d6dd7`.
- Main CI and CodeQL passed for that audited-base identity.
- At the read-only `2026-08-11T11:03:29Z` observation, live `origin/main` is
  `c0216011b52c72677a05396d13a7552f23eca6f2`. It advanced through merged active
  theme PR #95 at `2026-08-11T08:25:35Z`. That remote advancement is excluded
  active-theme work; this lane does not absorb, rebase onto, or edit its bytes.
- No open Manja PR or issue existed when this control plane started.
- Public domain/application/port packages, adapter contract tests, external
  module proof, architecture gates, and self-hosted composition already exist.
- `docs/legal/provenance.md` remains `BLOCKED`; root `LICENSE`, `NOTICE`,
  `THIRD_PARTY_NOTICES.md`, deterministic SBOM generation, and verified release
  packaging remain absent.
- OC-01 refreshed the stale July legal inventory with current Muamba/browser,
  Kubernetes renderer, social preview, Go/Alpine, source-archive, site, and OCI
  evidence. Provenance remains `BLOCKED`, and licensing/package-generation Task
  8 remains stopped.
- Simple Icons provenance checkpoint `7eb7d58c8bb936d2ca3813b90f91884a2f9fdb29`
  / tree `f234bf311c2a6a03e3cde4bf126a07c6d1e30182` received independent
  `ACCEPT`, then was fast-forward integrated and pushed by the product manager.
- CodeRabbit review `4904235223`, run
  `be132353-6da0-4b99-a5ab-5b9785ed2126`, completed substantively at exact
  `7eb7d58c8bb936d2ca3813b90f91884a2f9fdb29`. Discussion `3756311030` is
  independently `VALID` and material. The first correction candidate
  `051ef67bfb7dd49c7052fe7ee743fd1a88fad1ab`, tree
  `abf63a77c74228e019a1e39dc4016b156ab05389`, was rejected because same-`RUN`
  tails and Go flag aliases could bypass its exact-flag scan. Child
  `d5ede512ebb784c7c695948616fb209ac182db1e`, tree
  `c5e43f4336690bea451088cb023f654bd572aeb6`, replaced flag scanning with a
  strict canonical full-command token comparison and is integrated and pushed
  on the product-manager branch.
- OC-01M5's exact observed Manja social-rendering checkpoint and goal correction
  remain in that history at `dd1a9e5d41422cb400d99a407500562c70ab21a0`, tree
  `7ac46daea42ab9215231475f215becbf539de9fc`.
- Final-head CodeRabbit review `4904696726`, run
  `05b3efe6-5170-4f07-bb67-59f5121f6772`, reviewed exact
  `d5ede512ebb784c7c695948616fb209ac182db1e` and produced a material rejection.
  Independent classification confirms the complete browser-test-source artifact
  scan as material. Strict receipt decoding, corrected module prose, stable
  worktree labels, effective-build-command failure wording, and retained
  Dockerfile parse diagnostics are valid minor corrections. Candidate
  `2bd9114ec7c2a6d034c66a56692b3141da2a769a`, tree
  `b9b1fae0b0875938de84f55668a205478c947410`, implemented those findings and
  was integrated and pushed.
- CodeRabbit review `4905151932`, run
  `fde28eb6-45b5-4def-945e-4efe262617dd`, thread
  `PRRT_kwDOSzGXLc6YMCp7`, comment `PRRC_kwDOSzGXLc7f8HHp`, reviewed exact
  `2bd9114ec7c2a6d034c66a56692b3141da2a769a`. Independent verdict:
  `VALID` and material. A caller-selected clean subdirectory could pass while
  prohibited source remained elsewhere, so that exact parent is rejected for
  merge. This bounded docs correction removes the false host-archive success
  claim and leaves the host archive gate blocked under Task 8. Exact correction
  `e3d6bb977c096dc13933068369f46d3cf8decd3c`, tree
  `d867bfd01430196aad29e97b67636564958c1b17`, received independent `ACCEPT`
  with no findings, then was fast-forward integrated and pushed unchanged.
- Exact-head CodeRabbit invocation
  `84e96e2d-bec2-45ef-9ef6-925155c9f783` at `e3d6bb977c096dc13933068369f46d3cf8decd3c`
  was rate-limited and produced no substantive review object. A successful
  status context is not review evidence, so the CodeRabbit merge gate remains
  open. Goal-ledger correction `52c7598c4ea1f0b5f0f5e27e363320866b49f789`,
  tree `cef5aa1ce775095d1e7cd8d156c9f9249f8b2901`, received acceptance, then was
  fast-forward integrated and pushed unchanged.
- CodeRabbit review `4905658559`, run
  `d5161f3c-d4aa-41e6-af99-3f76ba07eeb0`, invocation
  `f2f1fbc0-de97-4c29-9fe0-365880fb74cc`, reviewed exact
  `52c7598c4ea1f0b5f0f5e27e363320866b49f789` substantively. Independent verdict:
  `VALID` and material. The inline digest/publication facet and outside-diff
  no-execution facet are one OCI inspection trust-boundary finding, not two.
  Candidate `59f6df7de70f7b41d3d53c911307192c5c2fe7ef`, tree
  `6ea9a541e6ddfbf9b8b6cf11a72e54a1c312c171`, marked OCI distribution
  inspection blocked and recorded prospective fail-closed Task-8 invariants,
  but was rejected because it falsely said no published release artifact
  existed. Child `a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`, tree
  `d580a55a59972032e578a9f38e5287341517ed71`, corrected that claim, received
  acceptance, then was fast-forward integrated and pushed unchanged.
- CI runs `31485230010` at `52c7598c4ea1f0b5f0f5e27e363320866b49f789`
  and `31487572347` at `a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`
  reproduce one narrow failure. `TestKubernetesCatalog` exhaustively checks all
  3,028 published detail IDs through global exact search; changing late IDs
  returned the bounded 31-byte temporary-unavailability 503 after both the
  initial request and its one documented retry. Activation, public and private
  Forgejo paths, the retry-helper test, root tests, and CodeQL passed in both
  runs. The test is not racing renderer readiness: its catalog, directory, and
  detail checks have already succeeded before the exhaustive search loop.
- Candidate `1866412c67650a413f6e2340f728a278ef8b685c`, tree
  `b1622cdf413945ab98c2b03a4ad313d2f217faa0`, resolved the production ordering
  defect by looking up published detail IDs in the admitted immutable catalog
  directory before deadline-bound search-child loading, but independent review
  rejected it. Its early path bypassed canonical `SearchService` validation:
  over-limit and control-wrapped exact IDs returned 200, while an
  NFKC-equivalent exact ID remained deadline-bound and could return 503.
- Correction `f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a`, tree
  `7c6a60757fbad01f8fc5d62db65952af06dda98d`, canonicalizes and validates the
  caller query exactly once, then passes the same opaque canonical value to
  both directory lookup and subsequent search. Controlled RED reproduced all
  three rejected cases;
  GREEN returns 400 without child reads for invalid queries, resolves the NFKC
  exact query directory-only, and preserves non-exact persistent 503 plus
  `Retry-After: 1`. It received independent acceptance, then was fast-forward
  integrated and pushed unchanged.
- CodeRabbit review `4906397511`, run
  `03263440-726a-4661-bd9e-b2d52654deb9`, reviewed exact
  `f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a` and was submitted at
  `2026-08-11T12:51:50Z`. Independent classification marks its final-head-loop
  wording `DUPLICATE`: existing external identity, review, CI, CodeQL, and
  substantive CodeRabbit gates already fail closed, so no recursive candidate
  identity is added. Its unconditional exact-directory preflight finding is
  `VALID` and material: ordinary global queries compare against all 4,872
  current demo details before the existing child-search deadline.
- Exact-query traversal correction
  `4578206bec2d28d2ca51e9b25f6823078c56e333`, tree
  `6c5077ed6e570fa92c2d9e5149a9a2eaf363e7bb`, keeps canonical validation first,
  then permits exact directory traversal only for canonical
  `detail-sha256-` plus 64-lowercase-hex queries. Controlled decoy receipts
  prove wrong-prefix, wrong-length, non-hex, suffixed, and ordinary queries
  enter bounded `SearchService` instead. Exact lowercase,
  uppercase-normalized, and NFKC-equivalent IDs remain directory-only and
  collect across catalogs. It received independent `ACCEPT` with no findings,
  then was fast-forward integrated and pushed unchanged.
- The first automatic CodeRabbit status at `2026-08-11T13:20:54Z` for exact
  `4578206bec2d28d2ca51e9b25f6823078c56e333` was rate-limited and insufficient.
  After exact-head revalidation, the product manager retriggered review at
  `2026-08-11T13:52:18Z` with [request comment
  `5254090894`](https://github.com/araihu/manja/pull/94#issuecomment-5254090894),
  invocation `aaedd82e-3e68-47f6-b28c-2fd7da1f4ad7`. The [bot reply
  `5254092288`](https://github.com/araihu/manja/pull/94#issuecomment-5254092288)
  was updated to `Review finished` at `2026-08-11T13:55:39Z`; the exact-head
  CodeRabbit context moved from `Review in progress` to successful
  `Review completed`.
- The [CodeRabbit standing
  summary](https://github.com/araihu/manja/pull/94#issuecomment-5249207465) was
  updated at `2026-08-11T13:55:36Z`. Run
  `048fe19f-1f5f-466e-844c-9f1f6b23cdce` reviewed exact range
  `f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a...4578206bec2d28d2ca51e9b25f6823078c56e333`,
  selected exactly `goal.md`, `internal/web/catalog_search.go`, and
  `internal/web/catalog_test.go`, and generated no actionable comments. It
  processed `internal/web/catalog_test.go` and disclosed that
  `internal/web/catalog_search.go` and `goal.md` were skipped as similar to
  earlier changes; independent exact-byte review covers those skipped files.
- The independent final-gate audit chose `ACCEPT` option A: this successful
  exact-head incremental review/check is substantive and has zero findings.
  The absence of a new pull-review object does not block this immutable
  identity because the retrigger, invocation, status transition, run, exact
  range, selected/skipped files, and standing-summary result form the stronger
  evidence chain. The generic docstring warning is non-material: the change
  adds only an unexported helper and conventional Go `Test*` functions, with no
  undocumented exported production API.
- The final PR gate is satisfied only for immutable
  `4578206bec2d28d2ca51e9b25f6823078c56e333`, tree
  `6c5077ed6e570fa92c2d9e5149a9a2eaf363e7bb`. Any child or other head movement
  restarts exact-head independent review, CI, CodeQL, and substantive
  CodeRabbit gates. This goal-only ledger child adds no product decision; its
  moving commit and tree identity remain external to avoid recursive
  self-reference.
- Goal-only ledger checkpoint
  `811b34b311c29dd60a70d6c88b0c0ec155ffbf12`, tree
  `d947f03358248aeb51c851ac0d9da93045f7a047`, is the current pushed PR head.
  CodeRabbit review
  [`4907614755`](https://github.com/araihu/manja/pull/94#pullrequestreview-4907614755),
  run `8fda1015-1268-4df3-af8e-afc302d9b7d2`, reviewed exact range
  `4578206bec2d28d2ca51e9b25f6823078c56e333...811b34b311c29dd60a70d6c88b0c0ec155ffbf12`
  after [request
  `5254855031`](https://github.com/araihu/manja/pull/94#issuecomment-5254855031),
  invocation `a837d3db-c790-4712-9e36-804a1d2af553`, and [finished reply
  `5254857274`](https://github.com/araihu/manja/pull/94#issuecomment-5254857274).
  It selected and processed the only changed file, `goal.md`. Independent
  classification is `REJECT` for merge only because the durable merge gate did
  not explicitly require a clean worktree.
- Discussion
  [`3759102440`](https://github.com/araihu/manja/pull/94#discussion_r3759102440)
  is a false positive and non-material: the referenced lane-base entries are
  historical lineage. Operating Rule 1 still requires every new unit from
  current `origin/main`; this correction does not rebase, absorb active-theme
  bytes, or change that scope rule. Discussion
  [`3759102445`](https://github.com/araihu/manja/pull/94#discussion_r3759102445)
  is duplicate, superseded, and non-material: the current exact-811 review
  selected and processed `goal.md`, while the prior exact-457 skip remains
  truthfully disclosed. Discussion
  [`3759102451`](https://github.com/araihu/manja/pull/94#discussion_r3759102451)
  is duplicate and non-material: the prior immutable-identity `ACCEPT` remains
  applicable after unchanged fast-forward integration and push; fresh review is
  already required only when bytes or identity move.
- The outside-diff clean-worktree finding is `VALID` and material. This
  correction makes an empty staged, unstaged, and untracked state an explicit
  merge condition. The generic 0% docstring warning is a false positive and
  non-material because the reviewed delta is Markdown-only. This correction's
  moving head and tree stay external to avoid recursive self-reference.
- Full root tests, strict Muamba verification, architecture, unrelated external
  module, generation, browser, API, and templ gates passed for the OC-01
  evidence. Local OCI inventory observations supplied baseline evidence only;
  they are not a digest-bound distribution gate, which remains blocked under
  Task 8. Direct standalone `site` testing still fails because its committed
  module files need indirect dependency updates; OC-01 did not widen scope to
  change them.
- Renderer/catalog initial HTML and preview-image checks pass the social-ready
  metadata gate. The public product site lacks canonical, Open Graph, explicit
  X Card, and preview-image metadata; this is a recorded Open Core artifact gap,
  not an OC-01 page change.
- Active Arai Hû Modern-theme rollout belongs to task
  `019fef01-b65f-7980-a360-83e48f8a6345`; this control plane must avoid its
  files and refs.
- Historical Manja worktrees and branches remain preserved. Existence does not
  authorize reuse or cleanup.

## Checkpoint Queue

### OC-01: Current Open Core provenance and artifact baseline

Status: accepted baseline and mechanical checkpoints, OC-01M5, strict Stripe
Dockerfile-binding correction, final-head corrections, root-gate truth,
goal-ledger corrections, and the OCI inspection trust-boundary correction are
integrated/pushed through exact
`811b34b311c29dd60a70d6c88b0c0ec155ffbf12`, tree
`d947f03358248aeb51c851ac0d9da93045f7a047`. The exact-query traversal
correction received independent `ACCEPT`, was fast-forward integrated and
pushed unchanged, and satisfied its immutable exact-head PR gate. The subsequent
goal-only ledger checkpoint is the current pushed PR head; independent review
rejects it for merge only because its durable gate did not explicitly require a
clean worktree. This bounded correction child addresses that single finding and
requires fresh exact-identity independent review.

Accepted source identity:

- base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`;
- rejected candidate preserved: `bcb8bcf455d380f4c35787fc3671f512c917ca7d`;
- accepted corrected head: `f4a9f48080902e0f6e390589e9f8662b525131f1`;
- accepted corrected tree: `9324262e34a2db5596300aa3f74cf77eaabada28`;
- independent verdict: `ACCEPT`, no findings;
- PM integration commits: `f5b2f6d` and `17f7d2e`.

Reviewed PR-transition identity:

- parent commit: `98b6218e4f78236e68708ef4332975ee5292badc`;
- parent tree: `e9ec53bd10e5255db449e7eb1bed9a14075ab760`;
- independent verdict: `ACCEPT`, no findings.

Current integrated/pushed product-manager checkpoint:

- head: `811b34b311c29dd60a70d6c88b0c0ec155ffbf12`;
- tree: `d947f03358248aeb51c851ac0d9da93045f7a047`;
- disposition: current pushed PR head; independently rejected for merge only
  because the durable merge gate did not explicitly require an empty worktree;
  preserved as the parent of this bounded correction child.

The final moving candidate head and tree are bound by the immutable external
review packet and control plane. A commit cannot embed its own final commit and
tree identity without changing that identity recursively.

Goal: reconcile the approved Open Core plan with current `origin/main` and
produce current, behavior-backed provenance and shipped-artifact evidence
without making an Apache-2.0 claim while authority remains blocked.

Required work:

- inspect current first-party, copied, generated, browser, Go, site, and OCI
  inputs;
- update stale evidence in `docs/legal/provenance.md` and
  `docs/legal/shipped-artifacts.md` only when current commands prove it;
- resolve mechanical provenance gaps where repository evidence is sufficient;
- identify authority decisions that require the rights holder rather than
  inventing them;
- verify current architecture, external-module, root, and site gates;
- keep licensing/package-generation Task 8 stopped while provenance is
  `BLOCKED`.

Acceptance:

- isolated clean worktree from current `origin/main`;
- narrow meaningful commits, each with command receipts;
- no SaaS or theme changes;
- exact independent reviewer verdict for every candidate checkpoint;
- accepted commits integrated into the PM branch promptly.

### OC-02: License, notice, SBOM, and reproducible package gate

Status: blocked on OC-01 provenance `PASS` and explicit rights-holder evidence.

### OC-03: Self-hosted installation and operator lifecycle

Status: queued after packaging boundary is current.

### OC-04: Hybrid SSR/Wasm and offline runtime

Status: Open Core; queued for plan reconciliation after self-hosted authority and
packaging checkpoints. Existing projection-v2 work is groundwork, not proof of
the complete runtime.

## PR Gate

Current PR: [#94](https://github.com/araihu/manja/pull/94),
`coord/opencore-product` into `main`.

Prior immutable snapshot at `2026-08-11T14:04:43Z`: PR #94 was open at pushed head
`4578206bec2d28d2ca51e9b25f6823078c56e333`. CodeQL run
[`31495654112`](https://github.com/araihu/manja/actions/runs/31495654112)
succeeded for `actions`, `go`, and `javascript-typescript`; its summary reports
no new alerts. CI run
[`31495658551`](https://github.com/araihu/manja/actions/runs/31495658551)
succeeded: `integration` completed in 13m41s and `test` completed in 22m54s;
build/publish and deploy were skipped as expected for the PR.

The first exact-head automatic CodeRabbit status at `2026-08-11T13:20:54Z`
was rate-limited and insufficient. The product-manager retrigger at
`2026-08-11T13:52:18Z`, after exact-head revalidation, completed successfully at
`2026-08-11T13:55:39Z`. Standing-summary run
`048fe19f-1f5f-466e-844c-9f1f6b23cdce` covers exact range
`f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a...4578206bec2d28d2ca51e9b25f6823078c56e333`
and reports no actionable comments. Independent final-gate audit `ACCEPT`
option A treats that incremental exact-head result as substantive despite no
new pull-review object. Therefore the final PR gate is satisfied for
`4578206bec2d28d2ca51e9b25f6823078c56e333`, tree
`6c5077ed6e570fa92c2d9e5149a9a2eaf363e7bb`, and no other identity. The PR
remains unmerged. No release, deployment, or cleanup is authorized.

Current snapshot at `2026-08-11T15:00:19Z`: PR #94 remains open at exact pushed
head `811b34b311c29dd60a70d6c88b0c0ec155ffbf12`. CodeRabbit review
`4907614755`, run `8fda1015-1268-4df3-af8e-afc302d9b7d2`, selected and
processed `goal.md`. Independent classification rejects that immutable head for
merge only because the gate did not require a clean worktree. The three inline
discussions are non-material under the classifications recorded above; the
outside-diff clean-worktree finding is valid and corrected by this child. The
PR remains unmerged, and no release, deployment, or cleanup is authorized.

Before merge:

- head SHA and tree match reviewed candidate;
- worktree is clean: `git status --porcelain=v1 --untracked-files=all` emits no
  output, so staged, unstaged, and untracked state are all empty;
- relevant root, site, architecture, generation, and artifact gates pass;
- CodeQL check exists and succeeds;
- CodeRabbit is present and completes a substantive successful review/check
  for the final moving candidate; absence, a rate-limited no-review, or a
  presence-only status blocks merge;
- every actionable finding is fixed and rereviewed;
- final PR scope contains Open Core work only.

## Next Action

Freeze this bounded correction child's moving commit and tree in the immutable
external reviewer packet and control plane, then obtain fresh exact-identity
independent review. If the accepted child is integrated and pushed, its head
movement restarts exact-head CI, CodeQL, and substantive CodeRabbit gates; all
must succeed at that same identity before any merge decision. Do not merge.
The PR remains open, provenance remains `BLOCKED`, and
licensing/package-generation Task 8 remains stopped. Deferred SaaS behavior
and the active Arai Hû theme remain excluded; OC-04 remains Open Core. No
release, deployment, or cleanup is authorized.
