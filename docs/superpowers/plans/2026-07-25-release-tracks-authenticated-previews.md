# Release Tracks And Authenticated Previews Implementation Plan

> **Implementation note:** Execute this plan task by task with test-driven development. This document was reconciled on 2026-07-27 against integrated Manja commit `23293e9c96cf0a05d5ce5d261fcf5c069b7b741e`; implementation must begin from the then-current `origin/main` in a fresh task worktree and must re-inspect every named API before editing.

**Goal:** Deliver durable release tracks, immutable last-known-good public documentation, authenticated revision previews, and manager-protected release mutations without exposing credentials or adding an upstream request proxy.

**Architecture:** Extend the existing provider-neutral `domain` and `application` contracts. Resolve every public request from hostname and mounted path to one `ReleaseTrack`, then to its immutable `CurrentRevisionID`, persisted spec/index artifacts, and Goshtoso renderer. Keep authorization, source credentials, filesystem details, and HTTP security in ports or internal composition.

**Integrated consumer checkpoint:** Go 1.26.5, templ v0.3.1020, YAML v3.0.1, and exact Goshtoso v0.0.13 in the root, `site/`, and `integration/testdata/external-module/` consumers. `architecture/goshtoso_dependency_test.go` is the recursive exact-version and no-replacement guard. The later unpublished Goshtoso runtime-manifest capability belongs only to the hybrid Wasm plan; it is not a release-track blocker unless implementation introduces that manifest contract.

## Authority And Global Constraints

- Read `AGENTS.md`, the companion design, the current application-structure design and plan, and the hybrid Wasm design before implementation.
- Reinspect the literal current signatures in `domain`, `application`, `application/port`, `internal/adapters/store`, `internal/web`, and `internal/selfhosted`. Do not copy signatures from this plan if current code has moved.
- Create a fresh dedicated worktree from the then-current `origin/main` under the repository's required worktree policy. Record the implementation base and keep unrelated changes out; this reconciliation does not prescribe a reusable path, branch, or historical base.
- Keep core provider-neutral, ports-first, deterministic, idempotent, generation/CAS safe, and recoverable after `port.ErrCommitOutcomeUnknown`.
- Write content-addressed blobs before transactional metadata. Publish revision, release, visibility/tombstone authority, audit, and outbox changes through one coarse `port.UnitOfWork`.
- Never let a source, parse, review, policy, store, cache, or restart failure replace or erase the prior immutable last-known-good public revision.
- Startup serves persisted immutable last-known-good bytes without requiring mutable source access. A startup sync may propose a candidate only after the persisted public surface is ready.
- Never silently serve the startup `SpecIndex` for a request that resolved to a different revision.
- Authenticate previews before contract, track, or revision lookup. Authorize the requested scope; return non-disclosing failures; mark previews `noindex`; exclude them from public publication, sitemap, search, offline, and Wasm projections.
- Require manager authentication and authorization for sync, track configuration, promotion, publication, withdrawal, deletion, and reauthorization. Browser mutations also require the exact same-origin policy in this plan.
- Raw preview credentials, manager credentials, Git tokens, and SSH keys terminate in internal adapters or `internal/selfhosted` composition. Public domain/application values carry principals, scopes, and opaque secret references only.
- Do not persist or log raw credentials, bearer values, session secrets, or CSRF tokens.
- Preserve the current Goshtoso `head.Dependencies`, AppShell, operation, schema, search, and management structures. Extend their route/base-path inputs; do not replace the integrated UI.
- Do not add a Try It console or any server-side upstream API proxy.
- Every future assurance deferral requires a GitHub issue receipt. Minimum implementation gates written here are planned acceptance gates, not debt and not deferrable by labeling them future work.
- Record every Goshtoso source dive, API gap, workaround, generated-template friction, or dependency friction in a task-specific snag ledger.

## Current Integrated Contracts To Extend

At the reconciliation checkpoint:

- `domain.ReleaseTrack` contains `ID`, `ContractID`, `BoundRef`, `Mode`, `Generation`, `CurrentRevisionID`, `CandidateRevisionID`, and `LastDecision`.
- `domain.ReleaseAuthorization` already binds a track, persisted `ContractReview`, `SyncRecord`, baseline/candidate revisions, source/ref, route, and policy digest.
- `application.ReleaseService.Coordinate` already reloads that persisted evidence and immutable revisions, validates their bindings, applies `ConsiderReleaseDecision`, and writes through `port.UnitOfWork`.
- `application.RevisionService.LoadSpec` already verifies content-addressed blob integrity.
- `port.OperationalStore` is the transactional write surface; `RevisionReader`, `ReleaseEvidenceReader`, `PublicationReader`, and `ReleaseAuthorizationWriter` are narrow read/write ports.
- `internal/adapters/store.FileStore` already implements manifest-based atomic commits, generation conflicts, immutable revisions/evidence, and unknown-outcome reporting.
- `internal/web/server.go` currently resolves a legacy `PublicationByPath` and rewrites it to one startup public renderer. That compatibility handler is the defect this plan removes.
- `internal/selfhosted.NewServer` currently performs source-first startup and constructs one startup `SpecIndex`. Recovery-first startup must invert that dependency.
- `internal/web` already renders the current AppShell public UI, `/search.json`, `/openapi.json`, `/sitemap.xml`, HTMX-selected content, assets, and management workbench routes.
- Existing management POST replay tokens are idempotency controls, not authentication or browser mutation protection.

## Authoritative Public HTTP Contract

For every public root, nested documentation path, hostname-mounted route, HTMX selection, search document, OpenAPI download, and sitemap request:

1. Canonicalize the externally trusted hostname and path.
2. Resolve exactly one route binding to its `ReleaseTrack` identity.
3. Load that track and decide its durable visibility before any revision, blob, index, renderer, body, or shared-cache work.
4. If public, snapshot the track generation and immutable `CurrentRevisionID`.
5. Load that exact stored `ContractRevision`, verify its content-addressed spec and parsed-index artifacts plus parser/codec binding, and render the decoded index with the track's base path.
6. Revalidate generation/visibility before publishing cache entries or response bytes when concurrent mutation can intervene.

The controlled response matrix is exact:

| Effective state | Anonymous response | `X-Manja-Publication-State` |
| --- | --- | --- |
| public and route exists | normal public status | `public` |
| public and docs anchor/resource is unknown | `404` | `public` |
| private or route never existed | `404` | absent |
| withdrawn durable tombstone | `410` | `revoked` |
| deleted durable tombstone | `410` | `deleted` |
| administratively disabled | `503` | `disabled` |

Anonymous public routes never use `401` or `403`. An unauthenticated preview or management challenge is `401` with the adapter-selected challenge. An authenticated principal without the requested preview scope gets indistinguishable `404`; an authenticated manager without the requested action gets `403`. All protected-route denials use `Cache-Control: no-store`.

Withdrawal and deletion must persist their durable authority before attempting a best-effort purge scoped to the affected route, track, revision, visibility generation, and host. Purge failure cannot restore visibility. Republishing a tombstoned route requires an explicit reauthorization command that revalidates manager authority, policy/review evidence, current immutable revision, route ownership, and an expected generation; changing source configuration or restarting is insufficient.

## Browser Mutation Policy

All management mutation endpoints use this exact order:

1. authenticate the manager;
2. validate the method and allowed content type;
3. before reading the request body, require either a valid session-bound CSRF token from a dedicated request header or, in strict Origin/Sec-Fetch mode, canonical same-origin `Origin` and `Sec-Fetch-Site: same-origin`;
4. obtain and syntactically canonicalize the project/track target without lookup or effect, preferably from IDs in a scoped route path;
5. authorize the concrete project/track/action using only those canonical IDs;
6. only after authorization, look up target existence, parse remaining fields, acquire a mutation slot, or execute the command.

Header-only protection rejects missing, malformed, opaque, cross-origin, scheme-mismatched, host-mismatched, or port-mismatched `Origin`, and rejects missing or non-`same-origin` `Sec-Fetch-Site`. Reverse-proxy normalization must use a trusted-proxy adapter and must not trust arbitrary forwarding headers. Tests cover POSTs to sync, track configuration, promotion, publication, withdrawal, deletion, and reauthorization for accepted same-origin requests and every rejection above.

New mutation routes place the authorization target in the path, such as
`/manage/spec/{specID}/sync` or a contract/track-scoped equivalent. During
migration only, legacy `/manage/sync` and `/manage/publication` may extract
the target after browser protection by reading one strictly bounded buffered
form body and decoding only the allowlisted target keys. The buffer is retained
for the authorized full parse; it is never reparsed from the network. Missing,
duplicate, conflicting, malformed, or oversized target input fails closed
without lookup.

Target extraction and canonicalization validate syntax only. They neither
confirm existence nor access contract, track, revision, source, cache, mutation
slot, or persistence state. Responses reveal no existence until authorization
succeeds. CSRF values are never accepted from the body because protection must
complete before body parsing.

## Planned Implementation Files

- Modify: `domain/release.go`, `domain/publication.go`
- Test: `domain/release_test.go`, `domain/publication_test.go`
- Modify: `application/release.go`, `application/revision.go`, `application/publication.go`
- Create or modify: focused `application/*_test.go` and `application/port/operational.go`
- Modify: `internal/adapters/store/fs.go`, `internal/adapters/store/fs_test.go`
- Modify: `internal/web/server.go`, `internal/web/public.go`, `internal/web/management.go`, focused web tests, and current source `.templ` files only when the UI requires it
- Modify: `internal/selfhosted/server.go`, `internal/selfhosted/server_test.go`, `cmd/manja/main.go` only for concrete composition
- Regenerate tracked `*_templ.go` only after source template edits; never hand-edit it
- Modify: `api/*.yaml` and regenerate `internal/web/api.gen.go` only if a real external API integration requires the management mutation surface
- Update: task-specific snag ledger and operator/public documentation required by the implemented contracts

### Task 1: Put Authoritative Visibility And Tombstones On Release Tracks

**Files:**
- Modify: `domain/release.go`
- Modify: `domain/release_test.go`
- Modify: `domain/publication.go`
- Create: `domain/publication_test.go`

**Purpose:** Make visibility, durable tombstones, reauthorization, and immutable current-revision selection explicit on the release track while evolving legacy publication records into route bindings.

**Step 1: Write failing release-track visibility tests**

Extend the existing `ReleaseTrack` tests with these cases:

- a new never-public track may be `private` with no current revision or tombstone;
- a public track requires a canonical non-empty `CurrentRevisionID`;
- making a formerly public track private is a withdrawal and persists a `revoked` tombstone before the private successor is visible to management;
- explicit withdrawal creates the same immutable tombstone bound to contract, track, route identity, current revision, prior generation, actor, and timestamp;
- deletion creates durable `deleted` evidence and retains the route identity;
- exact command replay is a no-op;
- stale expected generation returns a typed generation conflict;
- generation exhaustion fails closed;
- a different command at the same idempotency key fails closed;
- a tombstoned track cannot become public through a normal visibility toggle;
- explicit reauthorization may return a tombstoned track to public only with matching expected generations and complete reauthorization evidence;
- rejected or pinned release decisions preserve prior `CurrentRevisionID` and visibility;
- following acceptance changes current only to the persisted authorized candidate;
- pinned promotion requires the recorded accepted candidate;
- malformed identities, states, timestamps, or tombstone bindings fail validation;
- clone helpers isolate pointer-backed decision and tombstone evidence.

The stored track vocabulary is exactly `private`, `public`, `withdrawn`, and `deleted`. Map `withdrawn` to the HTTP header value `revoked` only at the web boundary. `disabled` is an effective deployment-level kill-switch response, not a persisted substitute for track visibility.

Run:

```bash
go test ./domain -run 'Test(ReleaseTrack|ReleaseVisibility|ReleaseTombstone|ReleaseReauthorization|ConsiderReleaseDecision|PromoteReleaseRevision)' -count=1
```

Expected: compile failures for the visibility/tombstone model and new pure transitions.

**Step 2: Add the smallest deterministic track extension**

Extend the current `domain.ReleaseTrack`; do not replace `ContractReview`, `ReleaseAuthorization`, `ReleaseEvidence`, `LastDecision`, `ConsiderReleaseDecision`, or `PromoteReleaseRevision`. The implementation may refine names after literal inspection, but the record must carry the equivalent of:

```go
type ReleaseVisibility string

const (
    ReleaseVisibilityPrivate   ReleaseVisibility = "private"
    ReleaseVisibilityPublic    ReleaseVisibility = "public"
    ReleaseVisibilityWithdrawn ReleaseVisibility = "withdrawn"
    ReleaseVisibilityDeleted   ReleaseVisibility = "deleted"
)

type ReleaseTrack struct {
    // existing identity, mode, generation, revision, and decision fields
    Visibility           ReleaseVisibility
    VisibilityGeneration uint64
    Tombstone            *ReleaseTombstone
}
```

A tombstone records the last publicly authorized immutable revision and canonical route identity. It is authority, not a cache hint. Provide validators, deep-clone helpers, and one generation-checked pure visibility transition. It must not load credentials, stores, clocks, policies, or caches.

Use canonical serialization and a deterministic command/idempotency digest. Do not use map iteration, local time, random IDs, or adapter-specific values in equality.

**Step 3: Turn publication into the route binding**

Evolve the current `domain.Publication` into the canonical hostname/base-path binding to a `TrackID`. Preserve `RevisionID` and `Public` only as decode inputs for Task 4 migration; new route writes must not use them as revision or visibility authority.

Add route-binding tests for:

- canonical contract, track, host, and segment-safe base path;
- optional explicit route generation for CAS-safe reconfiguration;
- no raw credentials or source values;
- deterministic equality and cloning;
- legacy decode followed by one explicit migration;
- validation against a track with matching contract/track identity.

A route record must not copy `CurrentRevisionID`, visibility, or tombstone state from the track.

**Step 4: Implement pure cross-invariants**

Add helpers that validate a route binding against a `ReleaseTrack`. A public track must have a non-empty current revision. A tombstone must bind to the recorded last public revision and route while leaving current/candidate history intact.

Do not add host lookup or cache concerns to release-decision functions. Task 5 coordinates release and visibility transitions atomically through the existing `UnitOfWork`.

**Step 5: Run focused and package tests**

```bash
go test ./domain -run 'Test(Publication|PublicRoute|ReleaseTrack|ReleaseVisibility|ReleaseTombstone|ReleaseReauthorization|ConsiderReleaseDecision|PromoteReleaseRevision|ValidateReleaseAuthorization)' -count=1
go test ./domain -count=1
```

Expected: PASS.
---

### Task 2: Make Immutable Revisions Independently Renderable

**Files:**
- Modify: `domain/spec.go`
- Modify: `application/revision.go`
- Modify: `application/revision_test.go`
- Create or modify: a narrow deterministic `application/port` index-artifact codec contract
- Modify: `application/sync.go`
- Modify: `application/sync_test.go`

**Purpose:** Ensure a committed revision contains every non-secret locator needed to load its exact public or preview index from persisted immutable spec and parsed-index bytes after restart.

**Step 1: Write failing metadata-validation tests**

Extend revision tests for:

- canonical contract/revision/source identities;
- required content-addressed `SpecBlobKey`, matching `SpecDigest`, and existing review snapshot invariants;
- required content-addressed parsed-index artifact key/digest plus an explicit codec/parser identity;
- persisted canonical spec path and format sufficient to rebuild `domain.SpecFile`;
- immutable metadata replay as a no-op;
- a conflicting record under the same contract/revision identity failing closed;
- no source URL, token, username, private key, bearer value, or raw credential entering revision metadata.

Use small `SpecPath`, `SpecFormat`, and index-artifact metadata extensions to `ContractRevision` unless current inspection shows equivalent persisted fields. Do not infer them from mutable deployment options during restart.

Run:

```bash
go test ./domain ./application -run 'Test(ValidateContractRevision|RevisionService|Sync.*Revision)' -count=1
```

Expected: failure because restart-render metadata and the exact loader do not exist yet.

**Step 2: Write failing exact-revision loader tests**

Introduce a provider-neutral application service that accepts `RevisionReader`, `BlobStore`, and a deterministic index-artifact decoder, then exposes the equivalent of:

```go
LoadIndex(ctx context.Context, contractID, revisionID string) (domain.SpecIndex, error)
```

Test this order and these failures:

1. validate canonical contract/revision input;
2. call `ContractRevision(ctx, contractID, revisionID)`;
3. verify the loaded record matches both requested identities;
4. call the existing content-integrity path used by `RevisionService.LoadSpec`;
5. load and content-verify the persisted parsed-index artifact referenced by that same revision;
6. strictly decode the declared artifact format/codec identity with bounds and no unknown/trailing data;
7. verify `SpecIndex.ProjectID == contractID` and `SpecIndex.RevisionID == revisionID`;
8. verify the index artifact is bound to the stored spec blob/digest and parser identity before returning.

Test missing revision, malformed metadata, missing spec or index blob, either digest mismatch, unsupported codec/parser identity, strict-decode failure, and index identity mismatch. In every case the service returns an integrity/not-found error and never substitutes a startup index, current source result, candidate, another revision, or a reparsed mutable-source result.

**Step 3: Implement the minimal loader**

Reuse or compose `RevisionService.LoadSpec`; do not duplicate content-address verification in HTTP handlers. The parser stays behind the existing `port.Parser` on the candidate/sync path. Encode its successful `domain.SpecIndex` through a deterministic, versioned artifact codec and store those immutable bytes content-addressed before committing revision metadata.

Any in-memory index cache is an adapter concern keyed by at least contract ID, revision ID, spec blob key, index artifact key, parser/codec identity, and visibility generation. Cache misses reload the same verified persisted index artifact. Corrupt or unsupported artifacts fail closed for that track; they never trigger source access or another-revision fallback.

The public/preview loader has neither source fetcher nor parser. That absence is the restart/LKG invariant and ensures later candidate parse failure cannot disturb the committed index.

**Step 4: Persist render locators during sync**

Update `application.SyncService` so a successful source fetch and parse records canonical path/format with the immutable revision before its `UnitOfWork` commit. Preserve the current order:

1. fetch candidate bytes;
2. parse and build review snapshot without changing public authority;
3. deterministically encode the parsed index and bind it to the contract, revision, spec digest, and parser/codec identity;
4. content-address and persist immutable spec and index bytes;
5. atomically save revision/sync/index evidence;
6. return a candidate/index to management.

Test source failure, parse failure, blob failure, metadata commit failure, and `ErrCommitOutcomeUnknown`. None may mutate a release track or route binding. On unknown outcome, reload by stable IDs before deciding whether an idempotent retry is needed.

**Step 5: Run focused tests**

```bash
go test ./application -run 'Test(RevisionService|RevisionArtifact|Sync)' -count=1
go test ./application/port ./domain -count=1
```

Expected: PASS.
---

### Task 3: Persist Host Routes, Tracks, Visibility, And Tombstones Atomically

**Files:**
- Modify: `application/port/operational.go`
- Modify: `application/port/port_test.go`
- Modify: `internal/adapters/store/fs.go`
- Modify: `internal/adapters/store/fs_test.go`
- Modify: `application/publication.go`
- Modify: `application/publication_test.go`

**Purpose:** Make hostname/path routing and visibility durable, indexed, generation-safe, and recoverable under unknown commit outcomes.

**Step 1: Write failing port contract tests**

Add the narrow reads needed by public resolution without exposing `FileStore`:

```go
type ReleaseTrackReader interface {
    ReleaseTrack(context.Context, string, string) (domain.ReleaseTrack, error)
}

type PublicationReader interface {
    PublicRoute(context.Context, string, string) (domain.Publication, error)
}
```

Names may follow current conventions, but the arguments are canonical hostname and request path, and the result is a route binding to one track, not a renderer or caller-selected revision. The trusted application resolver then loads authoritative visibility from that track; anonymous disclosure policy belongs above the store.

Change route-binding writes to require an expected route generation, either by evolving `OperationalStore.SavePublication` or by an equivalently coarse CAS method. Track visibility/tombstone changes continue to use `SaveReleaseTrack` with its expected generation in the same transaction. Update compile-time port fakes first.

Run:

```bash
go test ./application/port -count=1
```

Expected: compile failures in every adapter/fake that must adopt the CAS and route-reader contract.

**Step 2: Write failing filesystem tests**

Cover:

- route identity is the canonical tuple of hostname and mounted base path;
- hostnames are case-normalized, ports handled deliberately, and invalid/ambiguous host inputs rejected before lookup;
- path matching selects the longest canonical mounted prefix on a segment boundary;
- `/docs` does not match `/docs-other`;
- one route deterministically resolves exactly one contract/track;
- duplicate or overlapping bindings that would be ambiguous fail closed;
- root, nested content, HTMX, search, download, and sitemap paths all resolve through the same route binding;
- route bindings plus private, public, withdrawn, and deleted track states survive reopen;
- the deployment-level disabled overlay does not rewrite persisted track visibility;
- withdrawn/deleted tombstone evidence survives reopen even after cache purge failure;
- exact replay is a no-op and does not increment generation;
- stale expected generation returns `port.ErrGenerationConflict`;
- a pre-publication commit failure exposes only the prior complete manifest;
- a post-publication durability failure returns `port.ErrCommitOutcomeUnknown` and reopen exposes either the prior or next complete manifest, never a mixture;
- an unknown-outcome retry reloads state and converges without duplicate audit/outbox records;
- concurrent readers observe complete old or complete new route/track/visibility generations;
- stored records never contain credentials or request headers.

Run:

```bash
go test ./internal/adapters/store -run 'Test(FileStore.*(PublicRoute|Publication|ReleaseTrack|Tombstone|Generation|CommitOutcome))' -count=1
```

Expected: failures for host/path indexes, publication CAS, and durable tombstones.

**Step 3: Implement one manifest-owned route index**

Extend the existing operational manifest and immutable record layout. The route index must point to a route binding, which names one release track by canonical host/base-path identity. It must not point directly to visibility, a revision, a `SpecIndex`, mutable source ref, or startup renderer.

Continue using `FileStore.Within` for coarse atomicity. Validate all staged revision, authorization, review, sync, track, publication/tombstone, audit, and outbox references before the manifest rename. Preserve the existing rule that post-rename uncertainty wraps `ErrCommitOutcomeUnknown`.

When a track visibility mutation requires purge, commit the track tombstone and deterministic scoped purge intent in the same transaction. Dispatch is best effort after authority commits. A failed dispatch remains replayable and cannot roll state back to public.

**Step 4: Replace the legacy public resolver contract**

Refactor `application.PublicResolver` from `PublicationByPath(path)` into a trusted resolution that returns a route binding plus its `ReleaseTrack`. It must:

- accept already-normalized host and path value objects or validate both itself;
- load the route binding, then the identified track;
- classify track visibility as never-existed/private, tombstoned, or public and apply the separate deployment-disabled overlay;
- for public, snapshot track `Generation`, `VisibilityGeneration`, and `CurrentRevisionID`;
- validate route/track contract and track identities;
- never use legacy `Publication.RevisionID` as public revision authority;
- expose no raw credentials or adapter types.

Add resolver tests proving no revision/blob/index read occurs for private, withdrawn, deleted, or disabled routes.

**Step 5: Run focused and adapter tests**

```bash
go test ./application -run 'TestPublicResolver' -count=1
go test ./internal/adapters/store -run 'Test(FileStore.*(PublicRoute|Publication|ReleaseTrack|Tombstone|Generation|CommitOutcome))' -count=1
go test ./application/port ./application ./internal/adapters/store -count=1
```

Expected: PASS.
---

### Task 4: Add Deterministic Deployment Configuration And Legacy Migration

**Files:**
- Modify: `internal/selfhosted/server.go`
- Modify: `internal/selfhosted/server_test.go`
- Modify: `cmd/manja/main.go`
- Modify: `cmd/manja/main_test.go`
- Modify: `internal/adapters/store/fs.go`
- Modify: `internal/adapters/store/fs_test.go`

**Purpose:** Configure track routes and opaque auth references at composition while upgrading the existing single-publication state exactly once.

**Step 1: Write failing configuration tests**

Extend `internal/selfhosted.Options` with the smallest concrete configuration for:

- stable track ID;
- release mode and bound ref;
- canonical public hostname and mounted path;
- initial desired visibility;
- preview authentication adapter selection and opaque secret reference;
- manager authentication/authorization adapter selection and opaque secret reference;
- browser mutation protection mode;
- trusted proxy policy, disabled by default.

Test duplicate track IDs, duplicate or ambiguous host/path routes, malformed paths/hosts, unsupported modes/states, missing opaque secret references when auth is enabled, and any configuration that attempts to embed a raw token/key in public domain or application values.

Do not move `GitToken`, `GitSSHPrivateKey`, or future session-signing material out of `internal/selfhosted`/internal adapters. CLI flags and environment variables may feed those concrete options, but logs and errors must name only the option or secret reference, never its value.

Run:

```bash
go test ./internal/selfhosted ./cmd/manja -run 'Test.*(ReleaseTrack|Route|PreviewAuth|ManagerAuth|MutationProtection|Secret)' -count=1
```

Expected: compile failures for new configuration and validation.

**Step 2: Implement validated composition types**

Keep deployment configuration out of `domain`. Convert validated concrete options into domain/application commands at the composition boundary. Sort tracks and route bindings by canonical identity before creating deterministic commands or diagnostics.

The normal release-track slice remains on the integrated Goshtoso dependency. Do not add the unpublished runtime-manifest API, pseudo-versions, module replacements, or hybrid artifact configuration here.

**Step 3: Write failing migration tests**

Seed real pre-migration `FileStore` fixtures representing the current integrated states:

- a public legacy `Publication{ProjectID, RevisionID, Public: true, Path, Hostname}`;
- a private legacy publication;
- no publication;
- corrupt/ambiguous legacy records;
- an already migrated operational manifest;
- a crash before and an unknown outcome after the migration manifest rename.

Assert:

- public legacy state creates one configured track with `CurrentRevisionID` equal to the legacy immutable revision and explicit public authority;
- private legacy state creates private authority without a tombstone;
- absent legacy state remains non-disclosing and does not synthesize a tombstone;
- migration never selects the latest source revision or candidate;
- migration reads only the legacy immutable spec blob, parses it once with the integrated parser, and persists the deterministic index artifact required by Task 2 before changing schema authority;
- migrated route records use track ID plus host/base path; legacy `RevisionID` stops being routing authority;
- exact rerun is a no-op;
- configuration mismatch with already-migrated durable authority fails closed with operator guidance;
- unknown outcome is resolved by reopening and validating the migration marker and records;
- corrupt/ambiguous evidence aborts without partial publication;
- source fetcher is never called by migration; parser failure leaves the prior manifest and public state untouched.

Run:

```bash
go test ./internal/adapters/store ./internal/selfhosted -run 'Test.*(LegacyPublication|ReleaseMigration|MigrationCommitOutcome)' -count=1
```

Expected: failures for the explicit schema migration.

**Step 4: Implement one idempotent operational migration**

Use the existing manifest schema marker and `UnitOfWork`; do not create loose marker files. Validate the referenced immutable revision/blob and persist its content-addressed index artifact before making a formerly public record public under a track. Persist revision artifact metadata, route binding, track visibility authority, audit event, outbox message, and schema marker atomically.

A pre-commit failure leaves the old manifest readable. A post-rename unknown result is reconciled by reopen. Never delete legacy bytes during this slice; a later garbage collector may remove unreachable data only after proving no committed manifest references it.

**Step 5: Run focused tests**

```bash
go test ./internal/adapters/store -run 'Test.*(LegacyPublication|ReleaseMigration|MigrationCommitOutcome)' -count=1
go test ./internal/selfhosted ./cmd/manja -run 'Test.*(ReleaseTrack|Route|Auth|MutationProtection|Secret|Migration)' -count=1
```

Expected: PASS.
---

### Task 5: Coordinate Review-Bound Releases And Visibility Commands

**Files:**
- Modify: `application/release.go`
- Modify: `application/release_test.go`
- Create or modify: focused `application/publication_command*.go` and tests
- Modify: `application/port/operational.go` only for the minimal narrow readers required
- Modify: `internal/adapters/store/fs_test.go` for integration of the coarse transaction

**Purpose:** Evolve the current persisted-evidence release service and add explicit manager-authorized publication lifecycle commands without accepting caller-forged evidence.

**Step 1: Lock the current release trust boundary with failing tests**

Start from `application.ReleaseService.Coordinate`, `ReleaseDependencies`, `ReleaseEvidenceReader`, `RevisionReader`, and `UnitOfWork`. Do not introduce `StoredReview` or a second review model.

Refine `ReleaseCommand` so untrusted callers select stable IDs and intent, not authoritative `ContractReview` or `SyncRecord` bodies. Test that coordination reloads:

- `ReleaseAuthorization` selected by contract, track, and review identity;
- its persisted `ContractReview` and `SyncRecord`;
- baseline and candidate `ContractRevision` records and their canonical snapshots;
- the current generation of the identified `ReleaseTrack`.

Keep all current binding checks: contract, track, source, bound ref, route, policy digest, commit SHA, review digest/verdict/time, exception expiry, baseline, candidate, and actor identity. Caller disagreement or missing evidence fails closed before `UnitOfWork`.

Run:

```bash
go test ./application -run 'TestReleaseService' -count=1
```

Expected: focused failures for the ID-only command and any newly exposed gap, while existing evidence-integrity cases remain green after fixture updates.

**Step 2: Write failing coordination-state tests**

Cover following and pinned modes:

- accepted following decision atomically changes `CurrentRevisionID`, clears candidate, appends audit/outbox, and retains visibility state;
- failed review or rejected authorization records candidate/decision as current code specifies but preserves the prior current revision and public LKG;
- pinned acceptance records only the candidate;
- explicit promotion reloads the accepted decision/evidence and generation, then changes current;
- release/promotion never writes a caller-selected `Publication.RevisionID`;
- public cache purge intent is scoped to host/path/contract/track/old revision/new revision/track generation;
- source, parse, review, policy, blob, evidence, or commit failure leaves prior public state readable;
- exact replay is a no-op with stable generation and event IDs;
- stale generation fails with `ErrGenerationConflict`;
- `ErrCommitOutcomeUnknown` is returned unchanged, then reload by stable track ID proves old or new complete state; retry converges idempotently;
- two concurrent promotions cannot both win.

The service must not publish response bytes or invalidate caches before the authoritative commit.

**Step 3: Write failing publication lifecycle service tests**

Add application commands for track configuration, make-public/private, withdrawal, deletion, and explicit reauthorization. Keep the administrative disabled response as validated deployment composition, not a release-track visibility transition. Each command contains:

- canonical project, track, hostname, and base path identities;
- expected track/publication generations;
- stable idempotency key;
- authenticated manager principal/actor identity and already-resolved action scope, or a provider-neutral authorization value whose verifier is a port;
- trusted command time;
- for reauthorization, the persisted evidence identity that authorizes the current immutable revision.

Test:

- public authorization requires a non-empty current revision and valid persisted release evidence;
- a never-public private route resolves anonymously as non-disclosing not-found;
- making a formerly public route private commits the same durable `revoked` tombstone as withdrawal, so its old public URL returns `410` without revealing the private successor;
- withdrawal/deletion commits a durable tombstone and deterministic purge intent before any purge call;
- purge failure leaves `410` authority intact;
- reconfiguration, sync, source changes, or restart cannot clear a tombstone;
- reauthorization revalidates manager scope, route ownership, current revision, policy/review evidence, and expected generations;
- a route claimed by another track fails closed;
- replay and unknown outcomes converge;
- credentials never enter commands, events, errors, or stored records.

**Step 4: Implement coarse, deterministic orchestration**

Reuse the existing `UnitOfWork` and typed application errors. Validate outside the transaction only when the evidence is immutable; reload generation-sensitive track/publication state inside it. Create deterministic audit, outbox, and purge-intent IDs from canonical command/evidence bytes.

For withdrawal/deletion, commit tombstone authority and purge intent together. Invoke a cache-purge port only after the transaction reports a confirmed commit. If outcome is unknown, reload first; dispatch the idempotent purge only when the durable intent exists.

For release or promotion of a public track, the route stays bound to the track. Readers discover the new revision through `CurrentRevisionID`; no route-record revision rewrite is allowed.

**Step 5: Run application and store integration tests**

```bash
go test ./application -run 'Test(ReleaseService|Promotion|PublicationLifecycle|Reauthorization|Withdrawal|Deletion)' -count=1
go test ./internal/adapters/store -run 'Test.*(ReleaseTrack|Publication|Tombstone|PurgeIntent|CommitOutcome)' -count=1
go test ./application ./application/port ./internal/adapters/store -count=1
```

Expected: PASS.
---

### Task 6: Route Every Public Surface Through The Exact Current Revision

**Files:**
- Modify: `internal/web/server.go`
- Create: `internal/web/server_test.go`
- Modify: `internal/web/public.go`
- Modify: `internal/web/public_test.go`
- Modify: current `internal/web/templates/*.templ` files only for base-path/noindex inputs
- Regenerate: matching `internal/web/templates/*_templ.go`
- Modify: focused `internal/web/e2e/*_test.go`

**Purpose:** Remove the startup-renderer rewrite and make the integrated AppShell render a request-resolved immutable revision on every public entry point.

**Step 1: Write failing top-level routing tests**

Replace tests for `publishedDocsPathHandler` with a dynamic public handler whose dependencies are provider-neutral resolver and immutable index loader ports/services. Cover this exact call order:

1. normalize trusted hostname and request path;
2. resolve route visibility;
3. load the bound track and stop on private, tombstoned, or disabled effective state before revision/blob/index reads;
4. for public state, snapshot track ID, track generation, visibility generation, and `CurrentRevisionID`;
5. load the persisted index/artifact for that exact contract/revision;
6. verify index identities;
7. render to a private buffer;
8. re-read or CAS-validate route/track generations;
9. only then set shared-cache headers and write body bytes.

Use spies that panic on forbidden reads. Prove the old startup `SpecIndex` is never used after a different revision resolves. The production self-hosted route must not fall back to `NewPublicServer(startupIndex)` when a resolver dependency is missing; composition failure is safer.

Run:

```bash
go test ./internal/web -run 'Test(DynamicPublic|PublicRouteResolution|PublishedDocs)' -count=1
```

Expected: failures because `server.go` still rewrites an accepted path to `/` on the single startup renderer.

**Step 2: Lock the response matrix before implementation**

Add black-box tests for body, status, cache headers, and `X-Manja-Publication-State`:

- public existing root/nested/resource: normal status and `public`;
- public unknown docs path/anchor: `404` and `public`;
- private and never-existed route: observably indistinguishable `404`, no state header, no body/cache write from the private revision;
- withdrawn: `410`, `revoked`;
- deleted: `410`, `deleted`;
- disabled: `503`, `disabled`;
- anonymous public requests never return `401` or `403`;
- GET and HEAD have matching status/headers with no HEAD body;
- unsupported methods fail before render;
- concurrent withdrawal/deletion between render and response revalidation discards the buffer and returns the new non-public state;
- generation change to a new public revision retries resolution within a strict bound or returns a non-cacheable service error, never stale bytes labeled as new.

Non-public/error bodies must be generic and must not disclose contract, track, revision, title, path ownership, or existence history beyond the approved `410` state.

**Step 3: Make the existing public renderer base-path aware**

Extend `PublicOptions` and `templates.PublicDocsOptions` with request-scoped mounted base path and robots/noindex state. Preserve the integrated Goshtoso `head.Dependencies`, AppShell, sidebar, operation/schema views, semantic tokens, and search behavior.

Refactor URL helpers so all generated links stay under the resolved base path:

- root and selected HTMX content;
- operation/schema anchors and `?selected=` navigation;
- `search.json` result hrefs;
- `openapi.json` download;
- `sitemap.xml` locations;
- Goshtoso and Manja asset mounts where the existing head contract requires absolute paths.

Keep search result DOM IDs distinct from content anchors. Shared versioned asset handlers may remain outside release resolution, but no docs body, index, search data, download, or sitemap may bypass it.

**Step 4: Test every entry point against two revisions**

Create two deliberately different persisted indexes and two tracks/routes. For root-mounted, nested-mounted, and hostname-selected routes, assert:

- HTML title/version/content comes only from the resolved `CurrentRevisionID`;
- nested operation/schema navigation stays within the mount;
- HTMX fragments use the same revision and state header;
- search JSON contains only that revision and base-prefixed hrefs;
- OpenAPI download bytes/filename come only from that revision;
- sitemap includes only public routes for that revision and correct host/base paths;
- promotion flips every surface together;
- a candidate that is not current never appears;
- private/withdrawn/deleted tracks are excluded from any aggregate public search or sitemap;
- no Try It controls or upstream request endpoint appears.

**Step 5: Add safe cache semantics**

Cache only immutable index/render artifacts after identity validation. Key them by canonical host, base path, contract, track, track generation, visibility generation, revision ID, artifact digest, renderer identity, and representation/fragment selector.

Private, preview, tombstone, disabled, authorization error, and failed revalidation responses are `Cache-Control: no-store`. Public immutable content may use scoped validators, but must not be stored before the final state/generation check. Purge is defense in depth; durable visibility checks remain authoritative on every request.

**Step 6: Implement and run web tests**

Delete the compatibility behavior that accepts a route then rewrites it onto a different startup index. Keep the static `NewPublicServer(idx)` helper only for isolated renderer tests/demo use if current callers require it; production `NewServerWithOptions` must take the dynamic dependencies explicitly.

```bash
go run github.com/a-h/templ/cmd/templ generate
go test ./internal/web -run 'Test(DynamicPublic|PublicRoute|PublicServer|Search|OpenAPI|Sitemap|HTMX|PublicationState)' -count=1
go test ./internal/web/e2e -run 'Test.*(Public|Release|Route|Search|Sitemap)' -count=1
```

Expected: PASS and no generated template drift.
---

### Task 7: Add Authenticated, Scope-Bound Revision Previews

**Files:**
- Create or modify: focused `application/preview*.go` and tests
- Create or modify: focused `application/port` principal/authorization contracts and tests
- Create or modify: `internal/web/preview.go`, `internal/web/preview_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/public.go` only to reuse the base-path-aware renderer
- Modify: `internal/selfhosted/server.go` only for internal adapter composition
- Modify: `internal/selfhosted/server_test.go`

**Purpose:** Let an authenticated, authorized principal inspect one persisted immutable revision without making it a public release.

**Step 1: Write failing authentication-order tests**

Add a preview route with syntactically canonical identifiers, for example `/preview/{contractID}/{revisionID}/...`. Use spies to assert:

1. credential extraction and authentication run first;
2. unauthenticated requests return `401` with the configured challenge;
3. only after successful authentication may scope authorization inspect the requested canonical identifier values;
4. only after successful scope authorization may revision/blob/index lookup occur;
5. scope denial returns the same generic `404` as an authorized lookup of a missing revision;
6. malformed or credential-bearing query URLs fail without store access.

No contract, track, release, publication, revision, blob, or index lookup may occur before authentication. Authentication errors must not echo credentials.

Run:

```bash
go test ./internal/web -run 'TestPreview.*(Authentication|Authorization|LookupOrder|NonDisclosure)' -count=1
```

Expected: failure because no preview boundary exists.

**Step 2: Define provider-neutral principal and scope contracts**

Keep HTTP credential parsing in an internal web/self-hosted adapter. Pass only a validated principal and explicit scopes to application code. A preview grant must bind at least principal, contract, revision or approved revision set, action `preview:read`, expiry where applicable, and authentication method identity.

Do not place `http.Request`, bearer strings, cookies, raw token digests, Git credentials, or concrete identity-provider SDK types in `domain` or reusable `application`.

If the self-hosted adapter accepts static bearer credentials, store only a one-way configured digest, compare fixed-size digests with `crypto/subtle`, reject query-string credentials, and never log presented/configured values. Production session/OIDC adapters remain internal and must provide the same principal contract.

**Step 3: Reuse the exact immutable revision loader**

After authentication and scope authorization, call the Task 2 loader for the requested contract/revision. Preview must not:

- read or mutate `ReleaseTrack.CurrentRevisionID`;
- create/update `Publication`;
- create public route bindings, search aggregation, sitemap entries, offline bundles, or hybrid publication projections;
- update release review/policy evidence;
- proxy requests to the documented upstream API.

The preview may render HTML, HTMX fragments, and a preview-scoped download/search response only if the same authenticated request scope covers them. Preview sitemap and offline endpoints return non-disclosing `404`.

**Step 4: Write failing browser/cache/robots tests**

For full HTML, HTMX, search, and download preview responses assert:

- `Cache-Control: no-store, private`;
- `X-Robots-Tag: noindex, nofollow, noarchive`;
- an equivalent `<meta name="robots" ...>` in full HTML;
- a credential-aware `Vary` header where relevant;
- no `X-Manja-Publication-State: public`;
- every link stays under the authenticated preview base path;
- promoted or public status of the same revision does not change preview authorization;
- credentials never appear in response bodies, locations, URLs, logs, metrics labels, persisted records, snapshots, or test failure strings.

Test GET/HEAD and HTMX consistently. Unsupported methods must not invoke lookup.

**Step 5: Implement the preview handler and redaction tests**

Reuse the integrated AppShell with request-scoped base path and noindex options. Keep auth middleware outside the public resolver so a future refactor cannot accidentally look up existence first.

Install a recording `slog.Handler` in tests and search every attribute/message for the presented credential and configured secret. Add store snapshot inspection to the same test.

**Step 6: Run preview and public-isolation tests**

```bash
go test ./application -run 'TestPreview' -count=1
go test ./internal/web -run 'TestPreview' -count=1
go test ./internal/selfhosted -run 'Test.*Preview' -count=1
go test ./internal/web -run 'Test.*(PublicSearch|Sitemap|Offline|Publication).*Preview' -count=1
```

Expected: PASS.
---

### Task 8: Protect Every Management Mutation

**Files:**
- Modify: `internal/web/management.go`
- Modify: `internal/web/management_test.go`
- Modify: current management `.templ` sources when CSRF fields are needed
- Regenerate: matching `*_templ.go`
- Modify: `internal/web/server.go`
- Modify: `internal/selfhosted/server.go`
- Modify: `internal/selfhosted/server_test.go`
- Modify: `api/*.yaml` and regenerate `internal/web/api.gen.go` only if an external management API is actually exposed

**Purpose:** Require authenticated manager authority and browser mutation protection before sync, track, promotion, or publication state changes.

**Step 1: Inventory and lock every mutation route**

Start with current `POST /manage/sync` and `POST /manage/publication`. Add the planned track configuration, promotion, make-private/public, withdrawal, deletion, and reauthorization routes to one test table. If an API route invokes the same commands, include it too.

For every route assert the processing order:

1. manager authentication;
2. method and allowed content-type validation;
3. header-carried CSRF or strict same-origin validation before any body read;
4. extraction and syntactic canonicalization of project/track target IDs from the scoped route path, or the bounded legacy target-only buffer;
5. project/track/action authorization using those canonical IDs;
6. target lookup, remaining body/form parse, idempotency and mutation-slot handling;
7. application command/effects;
8. reload committed state for the response.

Authentication denial may inspect only authentication inputs and must perform
no mutation method/content-type, browser-protection, target, or body processing.
Method/content-type or CSRF/origin denial must perform no body read or target
processing. Authorization denial may observe only canonical target ID values;
spies must prove it performs no target-existence lookup, remaining form parse,
source/network call, mutation-slot acquisition, command, persistence, or
response-derived cache write.

Add scoped routes with project or track IDs in their path. For the current
legacy POSTs, test a migration-only extractor that runs after browser
protection, retains one bounded request-body buffer, decodes only allowlisted
`spec_id`/`track_id` target keys, and rejects absent, duplicate, conflicting,
malformed, or oversized values before authorization. After authorization, the
handler may parse the retained body for `ref`, `revision_id`, `path`,
`visibility`, `publish`, and idempotency values. A conflicting target
repeated in the authorized full form fails closed.

Syntactic target parsing is not a lookup and must not reveal whether the target
exists. Unknown canonical IDs and known IDs outside the principal's scope
produce the same generic authorization response until authorization succeeds.

The test table also proves unauthenticated requests stop at authentication even
when method, content type, browser headers, path target, or body are invalid;
authenticated invalid methods return `405`; invalid content types or browser
protection do not read the body; and the legacy target extractor accepts the
configured byte limit exactly but rejects one byte over it.

Missing or invalid manager authentication is exactly `401` with the configured challenge. An authenticated principal lacking the action is exactly `403`. Both use `Cache-Control: no-store` and a generic body.

**Step 2: Preserve idempotency without treating it as security**

Keep the existing management mutation request token/payload fingerprint as an idempotency mechanism. Tests must prove it cannot substitute for authentication, authorization, or CSRF/origin validation, and it is checked only after those gates.

Replace in-memory publication/sync mutation as authority. Handlers call Task 5 application services and render state reloaded from persistence. HTMX and full-page requests use the same command and security path.

**Step 3: Implement and test CSRF-header mode**

CSRF-header mode uses a cryptographically random token bound to the authenticated session, delivered only in protected management HTML, submitted through a dedicated request header, and validated in constant time before reading the body. Form-field CSRF tokens are forbidden because they would require body parsing before browser protection. Cover:

- valid token;
- missing token;
- malformed token;
- token from another session/principal;
- expired/rotated token;
- token replay after session invalidation;
- HTMX/header transport and absence of fallback to a form-field token;
- no token in URLs, logs, metrics, durable state, or error bodies.

Session/cookie creation and secret material stay in an internal adapter. Use `Secure`, `HttpOnly`, and an appropriate `SameSite` policy; CSRF remains required even with `SameSite`.

**Step 4: Implement and test strict Origin/Sec-Fetch mode**

Require both:

- a canonical `Origin` exactly equal to the effective management origin, including scheme, normalized host, and port;
- `Sec-Fetch-Site: same-origin`.

Table-test rejection of missing Origin, multiple Origin values, malformed/opaque/null Origin, userinfo, cross-origin scheme/host/port, missing `Sec-Fetch-Site`, and values `none`, `same-site`, or `cross-site`. Reject before body parsing.

Derive the effective origin from the direct request by default. Honor forwarding headers only through an explicitly configured trusted-proxy adapter that validates the immediate peer and canonicalizes a single value. Test spoofed forwarding headers from untrusted peers.

Self-hosted composition defaults to this strict Origin/Sec-Fetch mode so the
current native management forms remain executable without a body-carried CSRF
token. A deployment may select CSRF-header mode only when every mutation client
sets the dedicated header; it must not silently fall back to a form-field token.

**Step 5: Add action-specific authorization tests**

Use distinct permissions for:

- `sync`;
- `track:configure`;
- `release:promote`;
- `publication:publish`;
- `publication:make-private`;
- `publication:withdraw`;
- `publication:delete`;
- `publication:reauthorize`.

A manager authorized for one project/track/action cannot mutate another. Scope denial is `403` without target-existence details. Reauthorization requires its own permission and cannot be inferred from publish permission.

**Step 6: Add credential/log/store hygiene tests**

Use recording log, metrics, response, redirect, and store adapters. Search them for manager credentials, preview credentials, cookies, CSRF tokens, raw secret references, Git tokens, and SSH keys. Assert only opaque principal/secret-reference identifiers cross the internal composition boundary.

**Step 7: Run management security tests**

```bash
go run github.com/a-h/templ/cmd/templ generate
go test ./internal/web -run 'TestManagement.*(Authentication|Authorization|CSRF|Origin|Mutation|Sync|Track|Promotion|Publication|Withdrawal|Deletion|Reauthorization)' -count=1
go test ./internal/selfhosted -run 'Test.*(Manager|CSRF|Origin|Secret)' -count=1
```

If API YAML changed, also run:

```bash
npm run api:bundle
npm run api:lint
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
  -generate types,strict-server \
  -package web \
  -o internal/web/api.gen.go \
  api/dist/openapi.yaml
go test ./internal/web -run 'TestAPI.*Management' -count=1
```

Expected: PASS and generated output matches source.
---

### Task 9: Make Startup Recovery-First And Prove The Complete Slice

**Files:**
- Modify: `internal/selfhosted/server.go`
- Modify: `internal/selfhosted/server_test.go`
- Modify: `cmd/manja/main.go`
- Modify: `cmd/manja/main_test.go`
- Modify: focused `internal/web/e2e/*_test.go`
- Modify: operator/public docs and the task-specific snag ledger

**Purpose:** Serve durable immutable last-known-good documentation before touching mutable sources, then verify the complete release, visibility, auth, and restart contract.

**Step 1: Write failing startup-order tests**

Instrument every composition dependency and assert this order:

1. validate deployment configuration and resolve opaque secret references inside internal adapters;
2. open the operational store and reconcile any unknown prior manifest outcome;
3. run the idempotent legacy schema migration;
4. load route bindings and their release tracks;
5. validate every public/tombstoned track reference and make each public current revision's persisted immutable render artifact available;
6. construct the dynamic public, preview, and manager-protected handlers;
7. report readiness;
8. only after readiness, attempt configured source discovery/sync and candidate review.

Source fetch, Git ref discovery, candidate parse, review, or policy evaluation must not be prerequisites for serving an already committed LKG. A deployment with no prior valid public revision may fail readiness with an operator-safe error; it must not silently publish a new source result.

Run:

```bash
go test ./internal/selfhosted -run 'Test.*(StartupOrder|RecoveryFirst|LastKnownGood|Readiness)' -count=1
```

Expected: failure because current `NewServer` calls `syncSource` before it can construct the handler.

**Step 2: Write restart and failure-matrix tests**

Persist revision A as public LKG, then introduce candidate B. Restart or operate under each failure:

- source unavailable or credentials rejected;
- ref discovery failure;
- candidate parse/index failure;
- review or policy failure;
- blob write failure;
- metadata/store failure before commit;
- `ErrCommitOutcomeUnknown` after atomic publication;
- cache read/write/purge failure;
- corrupt candidate artifact;
- process restart during promotion, withdrawal, deletion, and reauthorization.

Assert A remains the only public body until a fully authorized B transition commits. After confirmed promotion, every public surface serves B. After withdrawal/deletion, restart returns the durable `410` state even if all caches still contain A. Private remains non-disclosing `404`; disabled remains `503`.

Parser failure during a later candidate sync must not affect A. Startup must load A's persisted immutable render artifact without invoking the source fetcher or requiring the mutable candidate parser path.

**Step 3: Reconcile unknown commit outcomes**

At every application command boundary, handle `port.ErrCommitOutcomeUnknown` by reopening/reloading stable record IDs and comparing the full expected generation/evidence state. Return a confirmed success only when the intended complete state is durable; return retryable uncertainty otherwise. Automatic retries are allowed only for deterministic idempotent commands and must not duplicate audit, outbox, or purge intents.

Test both possible reopen results after injected post-rename failure. Never infer success from an in-memory object.

**Step 4: Compose concrete security adapters**

Wire preview and manager authentication, authorization, CSRF/origin policy, secret resolution, trusted-proxy policy, and cache purge under `internal/selfhosted`. Keep reusable core free of concrete providers and raw secrets.

Startup diagnostics may name a missing secret reference or adapter kind but never its value. Add a final recursive scan of logs, operational manifests, cached artifacts, and rendered output for test credentials.

**Step 5: Add end-to-end release/visibility scenarios**

Using the real filesystem store, OpenAPI parser, current AppShell renderer, and `httptest`:

1. seed public revision A and start with source offline;
2. verify root, nested route, hostname route, HTMX, search, download, and sitemap all serve A;
3. sync/review candidate B and prove no public surface changes;
4. promote/accept B and prove all surfaces switch together;
5. preview a non-current revision only with valid scoped auth and verify noindex/no-store/public isolation;
6. deny preview before lookup for absent credentials and deny wrong scope without existence disclosure;
7. deny every management mutation without manager auth and browser protection;
8. withdraw, inject purge failure, and prove immediate plus restarted `410 revoked`;
9. prove config/source changes do not restore it;
10. reauthorize with valid evidence/expected generation and prove public service returns;
11. delete and prove durable `410 deleted`;
12. assert the HTML has no Try It console and the server exposes no upstream proxy path.

**Step 6: Run full relevant gates**

```bash
npm ci
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
go test ./...
GOWORK=off go test ./architecture -count=1
(cd site && GOWORK=off go test ./...)
(cd integration/testdata/external-module && GOWORK=off go test ./...)
```

Run container-backed integration tests only if implementation actually changes Forgejo, Dex, or another container-backed source/auth adapter:

```bash
go test -tags=integration ./internal/integration -v
```

Expected: PASS. Confirm root, `site/`, and `integration/testdata/external-module/` still resolve exact Goshtoso v0.0.13 with no replacement. Record every source dive or friction item in the snag ledger.

---

## Final Review Gate

### Requirements-To-Task Traceability

| Required contract | Planned coverage |
| --- | --- |
| Exact integrated Goshtoso v0.0.13 consumer checkpoint across root/site/external fixture | Authority, Task 9 |
| Later unpublished runtime manifest is hybrid-only, not a release blocker | Authority, Task 4 |
| Current public AppShell UI is extended rather than replaced | Tasks 6, 7 |
| Host/path resolves track then immutable current revision then stored artifact | Tasks 2, 3, 6 |
| Root, nested, hostname, HTMX, search, download, sitemap use same revision | Tasks 6, 9 |
| Exact visibility header/status and non-disclosure matrix | Authority, Tasks 1, 6 |
| Durable withdrawal/deletion tombstone, scoped purge, explicit reauthorization | Tasks 1, 3, 5, 9 |
| LKG survives source/parse/review/policy/store/cache/restart failure | Tasks 2, 5, 6, 9 |
| Startup serves persisted immutable LKG without source availability | Tasks 2, 4, 9 |
| Preview authenticates before lookup, authorizes scope, noindex/no-store, stays non-public | Task 7, Task 9 |
| Manager auth/authz and browser mutation protection for every mutation | Task 8, Task 9 |
| Raw credentials terminate at internal composition and never persist/log | Authority, Tasks 4, 7, 8, 9 |
| Provider-neutral, ports-first, deterministic, idempotent, CAS/unknown-outcome safe | Tasks 1, 3, 5, 9 |
| No Try It console and no upstream proxy | Authority, Tasks 6, 7, 9 |
| Goshtoso source dives/API gaps/workarounds become snags | Authority, Task 9 |

### Delivery Blockers Versus Assurance

The following are delivery blockers and cannot be deferred:

- any public entry point can bypass visibility/track/current-revision resolution;
- private existence can be inferred from status, headers, body, lookup order, search, sitemap, or cache;
- tombstone authority is not durable before purge;
- LKG startup depends on mutable source or candidate parsing;
- preview lookup precedes authentication or scope authorization;
- any management mutation lacks manager authorization or the configured browser mutation protection;
- credentials cross the internal boundary, persist, or appear in logs/output;
- CAS, idempotency, or unknown-outcome recovery tests fail;
- dependency guard, generation, unit, Markdown/API generation, or relevant integration gates fail.

Deferrable assurance is limited to additional evidence beyond the minimum gates, such as a broader browser/device matrix or an extra provider integration not used by this slice. Deferral requires a linked GitHub issue receipt with owner, scope, risk, and acceptance gate. Without that receipt, classify the item as unresolved and stop.

### Commands Before Requesting Implementation Review

Run from the dedicated implementation worktree:

```bash
git status --short
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
git diff --check origin/main...HEAD
npm ci
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
go test ./...
GOWORK=off go test ./architecture -count=1
(cd site && GOWORK=off go test ./...)
(cd integration/testdata/external-module && GOWORK=off go test ./...)
git status --short
```

Also inspect literal module resolution and replacements:

```bash
go list -m all | rg 'github.com/araihu/goshtoso'
(cd site && GOWORK=off go list -m all | rg 'github.com/araihu/goshtoso')
(cd integration/testdata/external-module && GOWORK=off go list -m all | rg 'github.com/araihu/goshtoso')
go list -m -json all
(cd site && GOWORK=off go list -m -json all)
(cd integration/testdata/external-module && GOWORK=off go list -m -json all)
```

Review the traceability table line by line against fresh code and test evidence. Audit exact changed paths, generated drift, local Markdown links, balanced fences, and the snag ledger. Record every delivery blocker and every deferrable item with its issue receipt.

Stop at this gate with the branch/worktree, commit range, fresh command output,
and review findings. Do not merge, push, or clean the worktree without explicit
direction.
