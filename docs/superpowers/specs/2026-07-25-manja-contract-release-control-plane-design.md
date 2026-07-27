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

- require authentication
- never imply public release
- are excluded from public search and sitemap generation
- emit no-index directives
- may display manager-safe diagnostics when a candidate cannot render

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

Advancement modes are:

- `pinned`: a manager or separately authorized automation explicitly promotes
  an accepted revision.
- `following`: a healthy accepted revision from the bound ref may advance the
  track automatically.

Both modes serve an immutable last-known-good revision. Following a ref never
means rendering arbitrary mutable source state directly.

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

## Routing And Rendering

Public documentation routes resolve:

```text
public path or hostname -> release track -> last-known-good revision -> renderer
```

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

## Resilience And Error Handling

Review and release decisions fail closed:

- source-fetch failure cannot produce an accepted revision
- parse failure cannot produce a compatible verdict
- diff or policy evaluation failure uses exit code `2`
- invalid uploaded evidence is rejected
- missing required dual-baseline evidence is an execution error
- persistence failure prevents track advancement

Public output remains non-destructive:

- following tracks advance only after a healthy accepted revision
- pinned tracks require explicit promotion
- every public track retains its last-known-good revision on failure
- failed candidates and revisions remain visible to authenticated managers
- public readers never receive private source, credential, or error details

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
- CI tokens are scoped to baseline/policy read and evidence upload.
- Promotion uses separate manager or automation authority.
- Uploaded evidence cannot make the server fetch an arbitrary user-provided URL.
- Public routes reveal only public contract and freshness metadata.
- Preview responses are no-index and absent from public sitemap/search payloads.
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

### Dependency Checkpoint: Goshtoso v0.0.13 Consumer Migration

After the Open Core checkpoint and before Subproject 2 changes web templates,
upgrade every Manja consumer module to exactly Goshtoso v0.0.13 using the
tagged changelog, migration guide, component model, head-dependency contract,
and Go reference. This is a
separate consumer migration, not part of the public domain/application
boundary and not a dependency-only bump.

The checkpoint includes the Go 1.26.5 minimum, a complete inventory of Manja's
component usage, migration from removed `Variant`/`Style` and config APIs,
composition in place of removed renderer internals, templ regeneration,
mechanical old-API scans, exact-version checks for nested consumer modules, and
direct/HTMX/light/dark/theme browser smoke coverage. It uses the default
CDN-first `head.Dependencies()` contract with exact embedded fallback and
records every Goshtoso source dive as a snag. The detailed plans are
`docs/superpowers/plans/2026-07-25-goshtoso-v0.0.12-consumer-migration.md` and
`docs/superpowers/plans/2026-07-26-goshtoso-v0.0.13-followup.md`.

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
- scoped-token authorization tests
- legacy publication migration tests
- public route, search, sitemap, and renderer regressions

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
- exact Goshtoso v0.0.12 resolution in root and `site/` modules
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
