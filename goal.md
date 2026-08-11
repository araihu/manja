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
- Current checkpoint base: `a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`
- Current checkpoint base tree: `d580a55a59972032e578a9f38e5287341517ed71`
- Branch: `codex/oc01-kubernetes-search-ci`
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
- This bounded correction resolves the production ordering defect: an exact
  published detail ID is now resolved from the admitted immutable catalog
  directory before loading deadline-bound search children. Non-exact queries
  retain the existing search path and persistent 503 behavior. Local controlled
  RED/GREEN proof and the full 3,028-ID Kubernetes integration acceptance pass;
  the moving candidate commit and tree remain external pending independent
  review.
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
`a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`. The bounded Kubernetes exact-search
correction is active locally and awaits independent review.

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

- head: `a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`;
- tree: `d580a55a59972032e578a9f38e5287341517ed71`;
- disposition: accepted; fast-forward integrated and pushed unchanged; preserved
  as immutable parent for this Kubernetes exact-search correction.

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

Snapshot at `2026-08-11T12:07:13Z`: PR #94 remains open at pushed head
`a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`. CodeQL `actions`, `go`,
`javascript-typescript`, and its summary pass. CI run `31487572347` completed
with failure: root `test` passed, while `integration` failed only
`TestKubernetesCatalog` after 289.11s when exact search
`detail-sha256-b4fc061c5abcf63b6476bf0d438474dda10cd053cfc9e0954cc859942e4fa526`
returned HTTP 503, `bytes=31`. Forgejo start, public fetch, private HTTPS, and
private SSH tests passed, as did
`TestCatalogSearchRequestRetriesOneTemporaryDeadline`. Run `31485230010` at
`52c7598c4ea1f0b5f0f5e27e363320866b49f789` had the same sole integration
failure after 247.38s for exact ID
`detail-sha256-674e48dbf74f258a2cf294bdf69f20494d91cb67b2ad1b916f9decdadb26c3ee`.
No runtime bytes changed across those docs-only heads. The exact-search
correction remains local and requires an exact-head CI rerun and independent
review before any lifecycle action. Merge remains blocked.

Before merge:

- head SHA and tree match reviewed candidate;
- worktree status includes staged, unstaged, and untracked state;
- relevant root, site, architecture, generation, and artifact gates pass;
- CodeQL check exists and succeeds;
- CodeRabbit is present and completes a substantive successful review/check
  for the final moving candidate; absence, a rate-limited no-review, or a
  presence-only status blocks merge;
- every actionable finding is fixed and rereviewed;
- final PR scope contains Open Core work only.

## Next Action

Freeze this Kubernetes exact-search correction's moving commit/tree in the
immutable external reviewer packet and control plane and obtain fresh independent
review. Rerun CI at the correction head and require `test` and `integration`
success. Then request one exact-head substantive CodeRabbit review and fix only
independently validated material findings. Do not merge. A failed CI gate,
absent review, or rate-limited no-review keeps merge blocked.
Deferred SaaS behavior and the active Arai Hû theme remain excluded; OC-04
remains Open Core; licensing/package-generation Task 8 remains stopped.
