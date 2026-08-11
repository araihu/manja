# Manja Open Core Product Goal

Updated: 2026-08-11

## Objective

Deliver Manja as a complete Open Core, self-hosted OpenAPI renderer and
publisher. Coordinate three independent Codex tasks: this product-management
task, one implementation task, and one read-only review task.

This file is the repository-visible recovery summary. External control-plane
YAML remains the lifecycle and ownership authority.

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
- Worktree: `/private/tmp/manja-opencore-product`
- Base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Owns scope, priorities, checkpoint acceptance, integration order, PR state,
  CodeQL/CodeRabbit triage, and durable recovery updates.

### Developer

- Task: `019fef17-2fad-73d2-b004-d3706d36ea82`
- Worktree: `/Users/guilhermecastro/.codex/worktrees/e1bf/manja`
- Base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Branch: `codex/oc01-provenance-baseline`
- Owns only bounded Open Core implementation checkpoints assigned here.
- Must commit each coherent checkpoint with a meaningful message.
- Must not implement deferred SaaS behavior or edit the active Arai Hû theme
  rollout.

### Independent reviewer

- Task: `019fef17-2faa-7620-b95c-ba6dc0343094`
- Read-only worktree: `/Users/guilhermecastro/.codex/worktrees/2106/manja`
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

- Live `origin/main`: `39d65ade21c080ee2102f53da5ed741f000d6dd7`.
- Main CI and CodeQL passed for that identity.
- No open Manja PR or issue existed when this control plane started.
- Public domain/application/port packages, adapter contract tests, external
  module proof, architecture gates, and self-hosted composition already exist.
- `docs/legal/provenance.md` remains `BLOCKED`; root `LICENSE`, `NOTICE`,
  `THIRD_PARTY_NOTICES.md`, deterministic SBOM generation, and verified release
  packaging remain absent.
- Independent developer and reviewer baseline audits agree that the July legal
  inventory is stale: current history, Muamba/browser inputs, Kubernetes
  renderer artifacts, social preview images, Go/Alpine versions, and OCI
  contents require fresh evidence.
- Active Arai Hû Modern-theme rollout belongs to task
  `019fef01-b65f-7980-a360-83e48f8a6345`; this control plane must avoid its
  files and refs.
- Historical Manja worktrees and branches remain preserved. Existence does not
  authorize reuse or cleanup.

## Checkpoint Queue

### OC-01: Current Open Core provenance and artifact baseline

Status: developer active; reviewer preparing independent baseline.

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

Before merge:

- head SHA and tree match reviewed candidate;
- worktree status includes staged, unstaged, and untracked state;
- relevant root, site, architecture, generation, and artifact gates pass;
- CodeQL check exists and succeeds;
- CodeRabbit review/check exists or its absence is explicitly recorded;
- every actionable finding is fixed and rereviewed;
- final PR scope contains Open Core work only.

## Next Action

Wait for the developer's first coherent OC-01 commit. Freeze its identity and
send only that candidate to the independent reviewer. Integrate promptly after
an identity-bound acceptance.
