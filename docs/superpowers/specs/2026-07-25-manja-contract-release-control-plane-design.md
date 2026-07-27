# Manja Contract Release Control Plane Design

Date: 2026-07-25
Status: Approved product direction

## Summary

Manja is a self-hosted API contract release control plane.

It connects to the source that owns an OpenAPI contract, turns source changes
into immutable and reviewable contract revisions, lets authenticated users
preview any healthy revision, and publishes stable public documentation through
independent release tracks.

Public documentation remains a core Manja output, but it is not the whole
product. The product decision is whether a contract revision is compatible,
reviewed, and ready to advance a public release line.

The design supports both sides of that decision:

- Before merge, a portable CLI and provider integrations review proposed
  contract changes.
- After merge, the self-hosted control plane discovers revisions, exposes
  authenticated previews, and advances public release tracks according to
  explicit policy.

The first product posture is self-hosted and single-installation. One Manja
installation may manage many API contracts, but v1 does not introduce
organizations, tenants, billing, or hosted-service parity. The CLI remains
fully useful without a running Manja server.

## Current Foundation

The current code already provides useful parts of this direction:

- OpenAPI loading from file and Git sources
- Git branch and tag discovery
- immutable revision metadata and stored spec blobs
- a first contract-diff implementation
- explicit publication metadata and public routes
- a read-only, search-first public renderer
- a server-rendered management workbench
- filesystem-backed storage behind ports
- last-known-good behavior in the source/sync design

Those pieces remain valuable. The main product change is the organizing model:
the current spec/revision publication workflow becomes a contract, candidate,
review, and release-track workflow.

### Reconciled implementation checkpoint

This design was reconciled on 2026-07-27 against exact `origin/main`
`23293e9c96cf0a05d5ce5d261fcf5c069b7b741e`.

That checkpoint already contains:

- the public `domain`, `application`, `application/port`, and `contracttest`
  extension surface;
- generation-checked filesystem `UnitOfWork` persistence for revisions,
  canonical reviews, sync evidence, release tracks, publications, audit, and
  outbox records;
- deterministic release authorization and canonical snapshot validation;
- exact Goshtoso v0.0.13 consumption across root, `site/`, and the external
  consumer fixture;
- the Goshtoso AppShell-based public docs and management workbench, including
  one scroll owner, server-authored selected identity, HTMX recovery, native
  forms/links, and current Publish/Diff/Route/Sync/History/Details behavior.

The remaining release-track slice extends those contracts. It does not recreate
the open-core boundary, rerun a Goshtoso migration, or replace the integrated
application structure.

## Product Goals

Manja should let an API team:

1. Run the same contract review locally, in any CI system, or through a provider
   integration.
2. See two separate views of a pull request:
   - the change introduced by the pull request
   - the cumulative impact relative to the currently public release
3. Preview any healthy branch, tag, commit, or pull-request revision after
   authentication without making it public.
4. Operate concurrent public release lines such as `v1`, `v2`, and `nightly`
   for one logical API contract.
5. Choose explicit promotion or automatic following independently for each
   release track.
6. Apply portable repository policy plus stricter server policy without hidden
   precedence.
7. Keep public documentation on an immutable last-known-good revision whenever
   source, parsing, review, policy, or provider workflows fail.

## Product Non-Goals

This product cycle does not include:

- SaaS multi-tenancy, organizations, billing, or hosted/self-hosted parity
- an OpenAPI authoring or visual editing workflow
- a server-side Try It proxy or upstream API request execution
- GitHub-specific objects in core domain or analysis packages
- an automatic public release path that bypasses track policy
- AsyncAPI, GraphQL, or a generic artifact-release abstraction
- multiple active sources contributing to one contract
- speculative management REST APIs unrelated to CLI or provider integration

OpenAPI files remain source-controlled truth. Manja reviews and releases them;
it does not become a parallel authoring system.

## Domain Model

### API Contract

`APIContract` is the stable top-level product identity. It represents a logical
API such as Payments API independently of its branches, tags, revisions, or
public versions.

One installation may manage many contracts. A contract has one active
`ContractSource` in this product cycle.

### Contract Source

`ContractSource` describes where Manja obtains the contract:

- source kind, initially file or Git
- repository or filesystem identity
- OpenAPI spec path
- credential reference when required
- provider adapter configuration when available

Credentials remain references to secret material. They are never returned after
configuration.

### Change Candidate

`ChangeCandidate` describes source context that may produce a revision.

Candidate kinds include:

- pull request or merge request
- branch head
- tag
- explicit commit
- singleton file revision

A pull request is provider-enriched change context, not a Git ref kind.
`merged`, `closed`, and `open` are pull-request lifecycle states, not source ref
types. Provider context may add a number, title, author, base ref, head ref,
state, and external URL. Core release behavior does not depend on a GitHub or
GitLab object.

### Contract Revision

`ContractRevision` is an immutable snapshot suitable for analysis, preview, and
release. It contains or references:

- contract and source identity
- source ref and commit identity when available
- raw spec blob identity and digest
- normalized parsed contract identity
- render/search index identity
- author/message metadata when available
- creation and sync provenance

Candidate state may move. A revision does not.

### Contract Review

`ContractReview` records deterministic evidence about a candidate revision:

- pull-request delta findings
- release-impact findings
- repository policy identity
- server policy identity when connected
- effective policy and provenance
- applied exceptions
- verdict
- engine and report schema versions
- input revision identities and digests
- provider or CI provenance when supplied
- policy evaluation time

Findings have stable identifiers derived from semantic rule and contract
location, not display text.

### Authenticated Preview

A preview renders a healthy immutable revision for an authenticated user. It is
a view over a revision, not a publication entity.

Preview routes:

- authenticate the request before contract or revision lookup
- require authorization scoped to the requested contract and revision
- never imply public release
- are excluded from public search and sitemap generation
- emit `X-Robots-Tag: noindex, nofollow, noarchive`, matching HTML metadata,
  and `Cache-Control: private, no-store`
- never enter the public publication projection or a future offline-publication
  descriptor/cache
- may display manager-safe diagnostics when a candidate cannot render

Missing or invalid credentials return `401` with the configured authentication
challenge before lookup. A valid identity without the requested scope returns
the same `404` shape as an unknown revision so the preview surface does not
confirm private contract or revision existence. Preview authorization never
returns public release state and never persists or logs raw credentials.

### Release Track

`ReleaseTrack` represents one independently managed public line such as `v1`,
`v2`, or `nightly`.

A track owns:

- contract identity
- bound source-ref selector
- public path or hostname
- repository policy profile selection
- server-added compatibility rules
- advancement mode
- current immutable last-known-good revision
- candidate revision when one is awaiting promotion
- review and promotion history
- authoritative public visibility state and its generation
- durable withdrawal/deletion tombstone state for any route that was public

Advancement modes are:

- `pinned`: a manager or separately authorized automation explicitly promotes
  an accepted revision.
- `following`: a healthy accepted revision from the bound ref may advance the
  track automatically.

Both modes serve an immutable last-known-good revision. Following a ref never
means rendering arbitrary mutable source state directly.

Public visibility states are provider-neutral domain values:

- `private`: never eligible for anonymous routing; indistinguishable from an
  unknown public route
- `public`: eligible only when `CurrentRevisionID` names a stored, valid,
  immutable revision
- `withdrawn`: durable tombstone for a formerly public route
- `deleted`: durable tombstone retained after logical track deletion

Changing a public track to private is a withdrawal, not erasure. The route and
tombstone remain durable across restart. Physical track removal must not remove
that authority. Reauthorization is a separate authenticated transition that
names a new or explicitly reauthorized immutable revision, advances the
visibility generation, records actor/audit evidence, and then permits public
serving again. A source ref becoming healthy cannot reauthorize a route.

### Domain Invariants

- A ref is not a release.
- A change candidate is not an immutable revision.
- A spec file is not the top-level workflow object.
- A public route resolves a release track, then its last-known-good revision.
- A preview route resolves an immutable revision directly.
- Every release track belongs to exactly one API contract.
- Every track binds one source-ref selector.
- A source ref may feed more than one track only through explicit
  configuration.
- Provider-specific metadata enriches candidates and reviews but never defines
  release semantics.
- Public state advances only after source, parse, analysis, policy, and
  persistence gates succeed.
- Public visibility is checked before revision lookup, parser work, response
  body writes, cache headers, or cache writes.
- Withdrawal and deletion are durable state transitions, not best-effort cache
  operations.
- An unknown commit outcome is recovered by reloading durable generation and
  deterministic operation identity before any idempotent replay.

## Architecture

Manja keeps ports-first, hexagonal boundaries.

### Open Core And Public Extension Boundary

The canonical public repository and Go module remain
`github.com/araihu/manja`. The public repository is the complete, useful
self-hosted product rather than a reduced community edition. Its target license
is Apache License 2.0, subject to the provenance and copyright-authority gate
below. The repository must not describe itself as Apache-licensed until that
gate passes and the required license materials are present.

A future proprietary `github.com/araihu/manja-cloud` repository may import
Manja as an ordinary Go module and provide hosted authentication,
tenant-scoped persistence, authorization, billing, managed queues, provider
installations, custom domains, managed secrets, and a separate cloud
composition root. This design does not create that repository or implement any
hosted product behavior.

The public extension surface is intentional:

```text
github.com/araihu/manja/domain
github.com/araihu/manja/application
github.com/araihu/manja/application/port
github.com/araihu/manja/contracttest
```

- `domain` owns provider-neutral entities, value objects, validation,
  comparison, policy, and pure state transitions. It does not import HTTP,
  SQL, filesystem, Git, Goshtoso, generated transport types, or adapters.
- `application` owns reusable commands, queries, orchestration, and public
  application errors. Constructors receive ports; they do not select or
  construct infrastructure.
- `application/port` owns stable infrastructure contracts for operational
  transactions, blobs, sources, parsing, secrets, identity and authorization,
  clocks, identifiers, caches, and asynchronous work where needed.
- `contracttest` provides public conformance suites for replaceable adapters.
  It does not provision environments or require self-hosted credentials.

Self-hosted implementations and transport wiring may remain under
`internal/adapters`, `internal/web`, and `cmd/manja`. `cmd/manja`, or a narrow
self-hosted internal composition package called only from it, selects concrete
adapters. Reusable application behavior must not remain trapped under
`internal/core` or `internal/app`, because an unrelated Go module cannot import
those packages. The `site` module is not proof of external compatibility: its
module path remains below `github.com/araihu/manja` and therefore shares Go's
internal-package visibility boundary.

All public application and port operations accept `context.Context` first and
propagate the incoming context unchanged. A future cloud adapter may read an
already validated tenant scope from context, but context is not authorization
and the public domain does not gain speculative `tenant_id`,
`organization_id`, billing, subscription, or entitlement fields.

Consistency-sensitive work must be expressible through a coarse
context-propagating `UnitOfWork`. When revision creation, snapshot metadata,
review evidence, sync records, release-track mutation, publication, audit
events, and outbox work form one invariant, the public port must allow them to
commit atomically. Blob and cache writes may sit outside that transaction only
when ordering, idempotency, orphan handling, and recovery are documented.

Raw Git tokens and SSH private keys are self-hosted composition inputs, not
reusable application options. Public application code receives opaque secret
references and resolves them through an injected port when needed. The secret
contract must support filesystem, Vault/OpenBao, External Secrets Operator, or
cloud secret-manager adapters without changing domain types.

Do not automatically export `internal/web`. If an external consumer proves a
need to reuse rendering, promote only stable view models and rendering behavior
to a small public presentation package. Self-hosted routes, generated handlers,
sessions, and authentication middleware remain internal. The canonical `api/`
description is a public compatibility contract; generated Go transport types
remain implementation details unless deliberately promoted. Public Go,
OpenAPI, and CLI breaking changes follow semantic-versioning discipline.

The governing extension test for every new reusable capability is:

> Can `github.com/araihu/manja-cloud` import this capability and inject
> tenant-aware proprietary infrastructure without modifying or forking
> Manja's public domain and application logic?

If not, revise the boundary without implementing the SaaS product.

### Licensing And Redistribution Gate

Apache-2.0 publication is gated by evidence, not metadata alone:

1. Confirm the real copyright owner and their authority to license all existing
   code and documentation.
2. Audit copied fixtures, generated sources, logos, fonts, JavaScript bundles,
   and other redistributed assets for provenance and license obligations.
3. Only after that audit, add the unmodified Apache License 2.0 text as root
   `LICENSE` and an accurate root `NOTICE` with the real holder, year range, and
   required attributions only.
4. Maintain `THIRD_PARTY_NOTICES.md`, or a generated equivalent release
   inventory, for shipped dependencies and assets. Test-only dependencies must
   not be described as redistributed production code.
5. Include `LICENSE`, `NOTICE`, and applicable third-party notices in source
   archives, release binaries, OCI images, and bundled site/frontend artifacts.
6. Add automated Go and npm dependency-license checks plus reproducible SBOM
   generation for shipped artifacts.
7. Add Apache-2.0 repository/package metadata only after the files and audit are
   complete. Establish DCO or an equally clear inbound-contribution policy
   before accepting external contributions; evaluate a CLA only if future dual
   licensing is a real goal.

The public `site` module may remain with the self-hosted product under the same
public license. Future proprietary cloud marketing, signup, account, and
billing pages belong outside this repository. No proprietary cloud code,
credentials, or commercial-license exceptions belong in Manja.

Checkpoint status on 2026-07-25: **BLOCKED**. Git authorship does not establish
whether the individual author or an employer owns the existing work, and copied,
generated, bundled, image, and container inputs still require redistribution
evidence. See `docs/legal/provenance.md` and
`docs/legal/shipped-artifacts.md`. Until that record becomes `PASS`, the
repository must not add Apache-2.0 license files, notices, badges, or package
metadata. Safe public package-boundary work may continue independently.

### Shared Contract Analysis Core

The CLI and server use the same deterministic core:

1. `SnapshotBuilder` parses OpenAPI and produces a normalized immutable contract
   snapshot.
2. `DiffEngine` compares snapshots and emits typed findings with stable
   identifiers.
3. `PolicyEvaluator` composes repository and server policy, applies valid
   same-layer exceptions, and produces a verdict.
4. `ReviewReporter` produces the canonical versioned review document and
   derived human-readable views.

The same snapshots, engine version, effective policy, and policy evaluation
time must produce the same findings and verdict offline and connected.

The core accepts provider-neutral input identities and change context. It does
not import GitHub Actions payloads, GitHub pull-request types, or other provider
SDK models.

### Portable CLI

The generic CLI is the first integration surface:

```text
manja check
```

It supports:

- local paths
- checked-out Git refs or explicitly resolved files
- explicit target, candidate, and release baselines
- repository policy configuration
- offline execution with no server dependency
- optional connected baseline/policy retrieval
- optional review-evidence upload

Canonical output is versioned JSON. Text and Markdown are deterministic derived
formats. SARIF or additional provider formats may be adapters later; they are
not required by the first subproject.

Exit codes are stable:

- `0`: effective policy passed
- `1`: analysis completed and effective policy failed
- `2`: execution, configuration, input, parse, or evidence error

An ad hoc local check may omit the release baseline and records that comparison
as unavailable. A configured pull-request gate that requires dual-baseline
review treats a missing release baseline as exit code `2`, not as a successful
partial review.

### Self-Hosted Server

The Manja server:

- stores contracts, sources, candidates, revisions, reviews, and tracks
- discovers source refs through provider-neutral source adapters
- accepts webhook, polling, manual, and uploaded-CI triggers
- runs the shared analysis and policy core
- exposes authenticated previews
- advances pinned and following tracks
- serves public docs from track state
- records sync, review, exception, and promotion activity

Startup is recovery-first. It opens and validates persisted operational state,
materializes public resolvers from immutable stored revision, spec-blob, and
parsed-index artifacts, and only then attempts mutable source discovery or
sync. Successful candidate parsing persists a deterministic, versioned index
artifact bound to the same revision and spec digest before release can select
it. A source, candidate-parse, review, policy, or provider failure after
persisted state loads cannot prevent serving the stored last-known-good bytes
and index. A corrupt or missing referenced artifact fails closed for that track
and never falls back to the startup candidate or another track.

Filesystem-backed adapters remain the initial implementation. Domain and
application ports preserve an evolution path to SQLite or another store without
putting persistence behavior in the core.

### Provider Adapters

Provider integrations are thin adapters.

The GitHub Action:

- resolves event-specific base and head context
- invokes the generic CLI
- maps the canonical report to a GitHub check or comment
- optionally uploads review evidence to Manja

GitLab CI, another hosted CI system, or a local script can invoke the same CLI
without reproducing analysis logic.

Webhook adapters normalize provider events into candidate and sync context.
Polling and webhooks remain complementary eventually consistent triggers.

### Narrow Connected API

The first concrete automation need justifies a narrow management API:

- read an authorized track baseline and effective server policy
- upload review evidence

CI credentials use separate scopes:

- baseline/policy read
- evidence write

Those scopes do not authorize release promotion, track configuration, source
configuration, or credential management.

No broader management REST surface is introduced until another real integration
requires it.

## Review Flows

### Pull-Request Review

The pull-request flow is:

```text
provider/local CI resolves target and head
-> CLI builds immutable snapshots
-> compare target to head
-> compare current public release to head
-> compose repository and connected track policy
-> apply valid exceptions
-> emit verdict and canonical report
-> publish provider presentation
-> optionally upload authenticated evidence
```

The two comparisons remain separate:

1. **Pull-request delta** answers what this pull request introduces.
2. **Release impact** answers what users of the currently public release would
   experience if the candidate became public.

This prevents unreleased branch changes from being falsely attributed to the
current pull request while still exposing their cumulative release impact.

Offline mode accepts both baselines explicitly. Connected mode may resolve the
public baseline and stricter track policy from Manja.

### Post-Merge Release

The post-merge flow is:

```text
webhook, polling, or manual sync
-> discover candidate refs
-> fetch and parse selected spec
-> store immutable revision
-> expose authenticated preview
-> evaluate mapped release-track policy
-> pinned track waits or following track advances
-> atomically update last-known-good revision
-> existing renderer serves public track
```

A tag is never automatically public merely because it exists. A tag, branch, or
commit becomes public only when a track selects or follows an accepted revision.

## Policy And Configuration

### Repository Policy

A versioned `.manja.yaml` is the portable policy source. It may define:

- contract identifiers
- OpenAPI spec paths
- named compatibility policy profiles
- rule severity and enablement
- repository-owned exceptions
- whether dual-baseline evidence is required for a profile

It does not contain:

- credentials
- public routes or hostnames
- authentication settings
- deployment settings
- CI tokens

A release track selects a named repository profile.

### Server Policy

The server owns deployment and release state:

- track route/hostname
- ref binding
- pinned or following mode
- selected repository profile
- additional track rules
- server-owned exceptions

Policy composition is monotonic. Server rules may add checks or increase
severity. They cannot remove repository checks, lower repository severity, or
silently ignore repository requirements.

Every report includes the effective rules and the source layer of each rule.

### Exceptions

An exception includes:

- stable finding or rule identifier
- contract scope
- optional track scope
- reason
- author or authenticated actor
- creation time
- expiry
- source layer

Repository exceptions can waive repository rules and are reviewed through Git.
Server exceptions can waive only server-added rules and require an
authenticated manager. An exception cannot waive a stricter rule owned by a
different layer.

Expired, malformed, out-of-scope, or unknown-rule exceptions do not apply and
are reported explicitly.

## Management Information Architecture

Management navigation is organized by API contract.

The contract workspace contains:

- **Overview**: source health, open reviews, revision freshness, and track
  summary
- **Candidates**: open pull requests, mutable branch heads, and immutable tags
  in separate groups
- **Release tracks**: public route, bound ref, advancement mode, current
  revision, candidate revision, and policy state
- **Reviews**: provider checks and uploaded/local review evidence
- **Revisions**: immutable revision history and preview entry points
- **Settings**: contract source and contract-level configuration

Merged and closed pull requests move into review/history views. `Merged` is
never presented as a ref kind.

Action ownership is explicit:

| Action | Owner |
| --- | --- |
| Preview | candidate/revision |
| Diff | review |
| Promote | release track |
| Public route | release track |
| Sync | contract source |
| Compatibility policy | repository profile plus release track |

The current Publish, Diff, Route, Sync, History, and Details peer-tab structure
is not extended. It is replaced as the corresponding domain entities become
available.

The management UI remains server-rendered HTML and uses Goshtoso primitives.

All management mutations, including sync, track configuration, promotion,
publication, withdrawal, deletion, and reauthorization, require an
authenticated manager and action/contract-scoped authorization. Browser
mutations also require either a session-bound CSRF token carried in a dedicated
request header or a configured strict policy that requires both an exact
trusted `Origin` and `Sec-Fetch-Site: same-origin`. Missing,
malformed, opaque, scheme/host/port mismatched, same-site, cross-site, or
`none` evidence is rejected before reading the body.

The executable order is authentication; method/content-type validation;
CSRF/origin validation; syntactic target extraction; scoped authorization;
then lookup, remaining form parsing, mutation-slot/idempotency handling, and
effects. New endpoints place canonical project/track IDs in the route path.
During legacy `/manage/sync` and `/manage/publication` migration only, target
IDs may be extracted after browser protection from one strictly bounded,
retained body buffer by decoding only allowlisted target keys. Missing,
duplicate, conflicting, malformed, or oversized targets fail closed.
Canonicalizing an ID proves syntax only: it performs no target lookup and
reveals no existence. Authorization denial therefore occurs before contract,
track, revision, source, cache, mutation-slot, or persistence access.

Forwarded origin data is accepted only through an explicitly trusted proxy
adapter. Idempotency tokens prevent replay but are not authentication or CSRF
protection. Self-hosted CLI credentials and source secrets terminate at
`internal/selfhosted` or internal adapters and never enter public domain
types, HTML, logs, or stored release records.

## Routing And Rendering

Every public reader entry point, including root and nested document loads,
hostname routes, selected-detail and HTMX requests, `search.json`,
`openapi.json`, and track-local `sitemap.xml`, resolves:

```text
normalized hostname + longest segment-safe public path
-> route binding
-> ReleaseTrack
-> authoritative visibility state
-> immutable CurrentRevisionID
-> contract-scoped stored revision metadata
-> content-addressed blob
-> content-addressed parsed-index artifact for those exact bytes
-> renderer
```

No matched track may reuse a renderer or `SpecIndex` constructed from a startup
candidate, another track, or mutable source state. Shared first-party and
Goshtoso assets may be mounted ahead of dynamic docs; all publication-scoped
reader resources use the resolution chain above.

Authenticated preview routes resolve:

```text
contract + immutable revision -> authenticated preview -> renderer
```

The existing read-only, search-first renderer remains the presentation engine
for both. Public route indexes continue to drive search and sitemap behavior.
Preview routes do not enter the public route index.

Concurrent tracks may render different revisions of one logical API. Each track
owns its canonical URL and SEO state. A failure on one track does not change
another track.

Controlled public responses use `X-Manja-Publication-State`:

- `public`: normal public response. A missing selected anchor may still be
  `404`, but it remains a response from a known public publication.
- `revoked`: `410 Gone` for a withdrawn formerly public route.
- `deleted`: `410 Gone` for a deleted formerly public route.
- `disabled`: `503 Service Unavailable` only for an explicit deployment-level
  public-docs disable/kill switch.

A `private` track and an unknown route both return the same `404` without a
publication-state header. When a formerly public track becomes private, the
public route becomes the generic `revoked` tombstone; it does not reveal the
private successor. Anonymous public routing never returns `401` or `403`.
`401` is reserved for an authentication challenge such as preview/management
auth; `403` is reserved for an authenticated management identity that lacks the
requested manager action. Authenticated preview scope denial remains `404`.

The state decision and response policy are established before body or public
cache writes. Non-public states use `Cache-Control: no-store`. Withdrawal and
deletion synchronously persist the tombstone, then best-effort purge only the
matching public/search/sitemap/future-offline cache keys. Purge failure is
reported but cannot roll back or bypass the durable tombstone. A later explicit
reauthorization performs fresh immutable revision validation and never
resurrects cached tombstoned bytes.

## Resilience And Error Handling

Review and release decisions fail closed:

- source-fetch failure cannot produce an accepted revision
- parse failure cannot produce a compatible verdict
- diff or policy evaluation failure uses exit code `2`
- invalid uploaded evidence is rejected
- missing required dual-baseline evidence is an execution error
- persistence failure prevents track advancement
- authentication or authorization failure prevents lookup and mutation
- CSRF/origin failure prevents form parsing and mutation
- withdrawal/delete persistence failure leaves the prior authoritative state
  unchanged; cache purge alone never changes visibility

Public output remains non-destructive:

- following tracks advance only after a healthy accepted revision
- pinned tracks require explicit promotion
- every public track retains its last-known-good revision on failure
- failed candidates and revisions remain visible to authenticated managers
- public readers never receive private source, credential, or error details
- restart serves persisted immutable last-known-good bytes without requiring
  mutable source availability
- an `ErrCommitOutcomeUnknown` result triggers reload and deterministic
  idempotency/generation comparison; callers never guess whether to advance
- a matched route with missing/corrupt immutable evidence returns a bounded
  unavailable response and never falls back to a startup renderer

Provider outages may make connected context stale. They do not prevent local
offline review.

## Idempotency And Evidence

Webhook, polling, manual-sync, and CI-upload replays are idempotent.

Revision identity uses stable source, commit/ref, spec digest, and contract
context. Review identity includes input revision identities, engine version,
report schema version, effective policy identity, and policy evaluation time.

Uploaded evidence includes:

- contract and candidate identity
- input revision identities and digests
- engine and report schema versions
- repository and server policy identities when present
- effective-policy digest
- findings, exceptions, and verdict
- policy evaluation time
- CI/provider provenance
- upload actor and timestamp

The v1 trust boundary is authenticated upload plus input and schema validation.
Cryptographic attestations may be added later without changing the canonical
report model, but are not required by this product cycle.

Uploaded evidence never creates or advances a release by itself. The server
must associate it with an existing or fetched revision and apply the mapped
track workflow.

## Security

- Source and provider credentials are stored through `SecretStore`.
- Management, preview, review, policy, and exception routes require
  authentication.
- Every management mutation authenticates first and enforces its configured
  CSRF-header or strict Origin/Sec-Fetch protection before body reads. Scoped
  authorization follows syntactic target extraction and precedes lookup,
  remaining form parsing, mutation-slot acquisition, and effects.
- CI tokens are scoped to baseline/policy read and evidence upload.
- Promotion uses separate manager or automation authority.
- Uploaded evidence cannot make the server fetch an arbitrary user-provided URL.
- Public routes reveal only public contract and freshness metadata.
- Preview responses are no-index and absent from public sitemap/search payloads.
- Preview authentication precedes contract/revision lookup; scope denial and
  unknown private identity share one `404` response.
- Private tracks are indistinguishable from unknown anonymous routes. Formerly
  public routes expose only generic durable `revoked`/`deleted` tombstone state.
- Public visibility is decided before render, body write, cache header, or cache
  write. Cache purge is scoped and best-effort after durable state commits.
- Provider links and report content are treated as untrusted input when rendered.
- Logs and reports never contain source credentials or private keys.

## Migration From Current State

The migration preserves existing public output before changing management
semantics.

For each current managed spec:

1. Create or map one `APIContract`.
2. Map its configured file or Git input to the active `ContractSource`.
3. Preserve stored immutable revisions and spec blobs.
4. Convert each public publication path into a pinned release track.
5. Point that track at the same published revision.
6. Derive a stable track identifier from the existing public path; use
   `default` only when the publication path has no usable segment.
7. Preserve current renderer and route behavior through the new track lookup.

Private legacy publications migrate to private tracks and do not enter the
anonymous route index. A legacy public path that is withdrawn during migration
retains a durable tombstone. Migration and reconciliation are deterministic,
generation-checked, idempotent, and recover unknown commit outcomes by reload;
they never reset a stored current revision from deployment configuration.

Legacy tracks start pinned. Operators may bind a source ref and change a track
to following only after migration.

Current management routes may redirect into the mapped contract workspace while
the UI migration is in progress. No migration step may unpublish working docs or
silently change their canonical path.

## Delivery Decomposition

This umbrella design is deliberately not one implementation plan. It creates
four product subprojects, each with its own spec and plan, plus one mandatory
architecture-and-licensing checkpoint before further server capability is
added.

### Architecture Checkpoint: Open Core Extension Surface And Licensing

This checkpoint precedes Subproject 2 and all later server slices. It does not
create a cloud product. Its scope is:

- provenance and copyright-authority inventory
- external-module and architecture compatibility tests
- promotion of reusable domain, application, and port packages out of
  `internal`
- an injected self-hosted composition root
- a coarse operational `UnitOfWork` and explicit blob consistency model
- public adapter conformance suites
- context and opaque-secret boundaries
- public Go/OpenAPI/CLI compatibility policy
- `LICENSE`, `NOTICE`, third-party notices, license/SBOM gates, and artifact
  packaging only after the provenance gate passes

The detailed plan is
`docs/superpowers/plans/2026-07-25-open-core-extension-surface-and-licensing.md`.
Subproject 2 must use the promoted public packages and ports; it must not add
new reusable release behavior to `internal/core` or `internal/app`.

### Integrated Dependency Checkpoint: Goshtoso v0.0.13

The exact Goshtoso v0.0.13 consumer checkpoint is already integrated at the
reconciled implementation base. Root, `site/`, and every discovered nested or
external consumer resolve exactly `github.com/araihu/goshtoso v0.0.13` with Go
1.26.5 or newer and no Goshtoso replacement. Release-track implementation keeps
the recursive architecture gate green; it does not rerun historical dependency
migrations or change the dependency.

Goshtoso's later public runtime-manifest/library-identity work is separate. It
exists in later Goshtoso source history but was not present in any tag newer
than v0.0.13 at this reconciliation checkpoint. Release tracks and previews do
not need that API and must not use a replacement or pseudo-version for it. The
hybrid Wasm/offline-publication slice must wait for a released exact tag that
contains the public runtime manifest, then perform its own isolated consumer
checkpoint. That future tag is a hybrid prerequisite, not a release-track
blocker.

### UI Checkpoint: Goshtoso Application Structure

After the dependency checkpoint and before Subproject 2, refactor the public
documentation and management workbench around Goshtoso's App Shell, Operations
List, Detail Workspace, and Multi-step Workflow contracts. Preserve the
server-rendered/static boundary, native navigation, HTMX enhancement, public
read-only behavior, and existing domain semantics.

The checkpoint establishes one shell and primary scroll owner, one synchronized
selected identity, explicit loading/empty/error/success recovery, native form
and link semantics, deliberate HTMX non-2xx handling, authoritative Back/refresh
behavior, one reachable mobile drawer, semantic CSS boundaries, and the full
390/1440 by Goshtoso/Minimal by light/dark acceptance matrix. It does not add
release tracks, authenticated previews, provider flows, a Try It console, or an
upstream proxy.

The approved design and implementation plan are
`docs/superpowers/specs/2026-07-27-manja-goshtoso-application-structure-design.md`
and
`docs/superpowers/plans/2026-07-27-manja-goshtoso-application-structure.md`.

### Subproject 1: Contract Review Core And Offline CLI

Scope:

- immutable normalized snapshots
- stable typed findings
- dual-baseline report model
- versioned report schema
- repository policy profiles and exceptions
- deterministic policy evaluation
- `manja check`
- JSON, text, and Markdown outputs
- stable exit codes

This is the first implementation-plan scope.

### Subproject 2: Release Tracks And Authenticated Previews

Scope:

- release-track and review persistence
- ref-to-track mapping
- pinned and following state transitions
- last-known-good public routing
- authenticated revision previews
- legacy publication migration

### Subproject 3: Connected Review And GitHub Integration

Scope:

- authorized baseline/policy read API
- evidence-upload API
- scoped CI tokens
- connected CLI client
- GitHub Action wrapper
- GitHub check/comment presentation
- provider candidate metadata

### Subproject 4: Management Information Architecture Migration

Scope:

- contract workspace
- candidate grouping
- track and promotion views
- review and revision history
- source settings
- activity history
- migration away from the current per-spec publication tabs

Subprojects 3 and 4 depend on the core model. Connected review also depends on
release-track baselines. The Open Core checkpoint is a prerequisite for
Subprojects 2, 3, and 4. The Goshtoso v0.0.13 consumer migration and application
structure checkpoint follow it and precede Subproject 2 web work so dependency
and UI-structure changes stay isolated from release-track behavior. No
subproject should be expanded to implement the remaining roadmap or a hosted
SaaS product prematurely.

## Testing Strategy

### Core And CLI

- golden fixtures for every supported finding kind
- deterministic report and stable-finding-ID tests
- dual-baseline separation tests
- policy monotonicity tests
- same-layer and cross-layer exception tests
- expired and malformed exception tests
- CLI black-box tests for inputs, formats, and exit codes
- versioned JSON report schema compatibility tests
- offline and connected parity tests using identical resolved inputs

### Server And Storage

- pinned and following state-transition tests
- idempotent sync, webhook, and evidence replay tests
- last-known-good preservation across every failure stage
- restart recovery tests for stored revisions, reviews, and track state
- atomic track-advancement tests
- preview authentication and no-index tests
- authentication-before-lookup and preview scope non-disclosure tests
- manager authentication/authorization, CSRF-header, and exact
  Origin/Sec-Fetch ordering tests for sync, track configuration, promotion,
  publication, withdrawal, deletion, and reauthorization
- public/private/withdrawn/deleted transition, tombstone restart,
  reauthorization, and scoped purge-failure tests
- exact `401`/`403`/`404`/`410` response/header tests
- scoped-token authorization tests
- legacy publication migration tests
- public route, search, sitemap, and renderer regressions
- root/nested/hostname/HTMX/search/download/sitemap tests proving each request
  uses the resolved track's exact immutable revision and never a startup index

### Public Extension And Licensing

- an unrelated `example.com/manja-extension` fixture run with `GOWORK=off`
- import tests for `domain`, `application`, `application/port`, and
  `contracttest`
- dependency-direction tests forbidding public imports of `internal`, adapters,
  generated HTTP types, and infrastructure drivers
- constructor tests proving all reusable infrastructure is injected
- context-spy tests proving incoming contexts reach every called port unchanged
- public-domain scans forbidding tenant, organization, billing, subscription,
  and entitlement concepts
- transaction contract tests proving consistency-sensitive release invariants
  are expressible through `UnitOfWork`
- conformance tests for self-hosted and in-memory port implementations
- license/provenance inventory checks for Go, npm, generated, copied, and static
  assets
- release-archive, binary, OCI, and site-artifact checks for required notices
- Go and npm license policy plus SBOM generation gates

### Provider Adapters And UI

- provider event to normalized context contract tests
- GitHub Action fixture workflows
- check/comment rendering tests from canonical reports
- Forgejo integration tests for provider-style Git behavior when applicable
- Dex integration tests for authenticated management and preview behavior
- end-to-end candidate, review, preview, promotion, and following-track flows
- accessible navigation and visible-target route tests
- exact Goshtoso v0.0.13 resolution with no `replace` in root, `site/`, and
  every nested consumer module
- mechanical removed-API scans and drift-free templ regeneration
- direct-load and HTMX smoke tests for affected public/management pages
- light/dark checks for Manja, Goshtoso, Minimal, and other advertised themes
- console/network checks plus disabled/loading/error interaction states

## Acceptance Criteria

The product direction is realized when:

1. A developer gets the same verdict locally and in CI for the same snapshots,
   engine version, effective policy, and policy evaluation time.
2. A pull-request report displays its own delta separately from cumulative
   release impact.
3. Authenticated users can preview any healthy discovered ref without
   publishing it.
4. One API contract can serve concurrent public `v1` and `v2` tracks with
   independent routes, policies, and advancement modes.
5. A pinned track never advances without explicit promotion.
6. A following track never advances past an unhealthy or policy-rejected
   revision.
7. Public documentation remains on its last-known-good revision through source,
   parser, analysis, policy, storage, provider, and restart failures.
8. GitHub integration invokes the same portable CLI used locally and in other
   CI systems.
9. Management clearly distinguishes pull requests, branches, tags, revisions,
   previews, reviews, and release tracks.
10. An unrelated Go module can import the public domain, application, port, and
    contract-test packages, inject in-memory adapters, and execute review and
    sync use cases with `GOWORK=off`.
11. Public application packages construct no adapters, preserve incoming
    contexts, expose transaction boundaries capable of protecting release
    invariants, and contain no speculative SaaS domain concepts.
12. Manja claims Apache-2.0 licensing only after authority and provenance are
    confirmed, and every shipped artifact carries the required license and
    notice material with machine-verifiable dependency and SBOM evidence.
13. Every public reader entry point resolves hostname/path to one release track,
    then its immutable current revision and stored bytes; no matched route can
    reuse a different startup renderer.
14. Private routes do not disclose existence, while formerly public withdrawn
    or deleted routes retain durable generic tombstones, return exact state and
    status, purge scoped caches best-effort, and require explicit
    reauthorization.
15. Preview and management authentication precede protected lookup/effects;
    management mutations also enforce scoped authorization and strict
    same-origin browser protection.
