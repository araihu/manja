# Open Core Extension Surface And Licensing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `github.com/araihu/manja` a complete self-hosted product with an
externally importable, provider-neutral domain and application surface, then
establish an evidence-backed Apache-2.0 licensing and redistribution posture
without implementing a hosted SaaS product.

**Architecture:** Promote pure behavior from `internal/core` to `domain`,
move reusable orchestration to `application`, and define infrastructure
contracts in `application/port`. Consistency-sensitive operational state is
changed through one injected `UnitOfWork`; immutable blobs stay content
addressed and outside that transaction under a documented write-first,
orphan-safe recovery model. Self-hosted adapters, HTTP, credentials, and
composition remain internal or under `cmd/manja`. Public conformance suites
live in `contracttest`, and a genuinely unrelated Go module proves the
extension surface with `GOWORK=off`.

**Tech Stack:** Go 1.26.1, standard-library context/testing/AST tooling,
filesystem-backed self-hosted adapters, npm lockfile metadata, CycloneDX JSON
SBOMs, existing GitHub Actions CI, Docker/OCI packaging, and semantic-versioned
public Go/OpenAPI/CLI contracts.

## Global Constraints

- Execute this plan in a fresh dedicated worktree and branch from the then
  current `origin/main`; do not implement it inside a release-tracks feature
  worktree.
- Preserve the module path `github.com/araihu/manja`.
- The public repository remains the complete single-installation self-hosted
  product. Do not create `manja-cloud`, cloud build tags, Go plugins,
  proprietary directories, or edition-specific compilation.
- Do not introduce tenants, organizations, billing, subscriptions,
  entitlements, hosted accounts, or provider marketplaces into the public
  domain.
- `domain` is pure and provider neutral. `application` depends only on
  `domain`, `application/port`, and the standard library.
- All public application and port methods accept `context.Context` first.
  Application services pass the received context to ports unchanged; never
  replace it with `context.Background()` or `context.TODO()`.
- Public application constructors receive every infrastructure dependency.
  They do not construct filesystem, Git, OpenAPI, Markdown, cache, HTTP, or
  secret implementations.
- Raw Git tokens and SSH private keys remain self-hosted composition inputs.
  Public types carry only opaque secret references.
- Protect revision, review, sync, release-track, publication, audit, and
  outbox invariants through one coarse operational `UnitOfWork`; do not
  fragment the port surface so atomicity becomes impossible.
- Keep immutable blob writes separate only under the specified content-addressed
  write-before-transaction model. A failed transaction may leave an unreachable
  blob that is safe to replay and eligible for garbage collection.
- Do not export all of `internal/web`. Promote presentation behavior only
  after an external consumer proves a stable need.
- Do not claim Apache-2.0 licensing, add license metadata, or invent a holder
  name/year until the authority and provenance audit passes.
- Keep the public docs read-only. Do not add Try It or an upstream proxy.
- Record any Goshtoso ambiguity or source dive as a snag before review.

## Target File Structure

- `domain/`: domain types, validation, deterministic review/policy behavior,
  snapshots, findings, publications, and pure release transitions.
- `application/`: review/check, discovery, sync, preview preparation,
  publication, and release coordination services.
- `application/port/`: operational unit of work, blobs, sources, parsing,
  secrets, identity/authz, clocks, IDs, cache, and outbox contracts.
- `contracttest/`: exported adapter conformance suites with no environment
  provisioning.
- `internal/adapters/`: self-hosted filesystem, Git/file source, OpenAPI,
  Markdown, cache, auth, and other infrastructure.
- `internal/selfhosted/`: optional self-hosted server composition called only
  by `cmd/manja`.
- `internal/web/`: self-hosted routing, generated transport, sessions, and
  HTTP presentation.
- `integration/testdata/external-module/`: unrelated import-compatibility
  module `example.com/manja-extension`.
- `architecture/`: executable dependency, context, and forbidden-domain
  boundary tests.
- `docs/legal/`: provenance and shipped-artifact evidence.
- `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`: created only after the
  audit gate passes.

---

### Task 1: Establish The Provenance And Compatibility Baseline

**Files:**
- Create: `docs/legal/provenance.md`
- Create: `docs/legal/shipped-artifacts.md`
- Create: `docs/legal/inbound-contributions.md`
- Modify: `docs/superpowers/specs/2026-07-25-manja-contract-release-control-plane-design.md`

**Interfaces:**
- Produces: an auditable decision about licensing authority, copied/generated
  material, shipped artifacts, and inbound contribution terms.

- [ ] **Step 1: Confirm the isolated base**

Run:

```bash
git fetch origin
git worktree add -b codex/open-core-extension-surface /tmp/manja-open-core origin/main
cd /tmp/manja-open-core
npm ci
git status --short --branch
git rev-parse HEAD
```

Expected: a clean worktree whose HEAD equals the fetched `origin/main`.

- [ ] **Step 2: Inventory authorship and redistribution inputs**

Record evidence for:

- commit authors and imported/copy-derived source history
- OpenAPI fixtures and generated `api.gen.go` output
- generated templ output and bundled JavaScript
- logos, fonts, icons, CSS, screenshots, and other static assets
- Go modules and npm packages separated into shipped, build-only, and test-only
- Docker base images and files copied into the final image
- source archives, binary archives, OCI images, and product-site artifacts

Use literal repository evidence:

```bash
git shortlog -sne --all
rg -n 'Copyright|SPDX-License-Identifier|Licensed under|generated|DO NOT EDIT' .
go list -m -json all
npm ls --all --json
docker history --no-trunc manja:provenance
```

Build the temporary image locally before the last command if no current image
exists. Do not infer that a dependency is shipped merely because it occurs in
`go.mod` or `package-lock.json`.

- [ ] **Step 3: Write the authority decision as a hard gate**

`docs/legal/provenance.md` must identify:

- the person or entity that owns or can license each first-party body of work
- the evidence used to establish that authority
- every unresolved copied/generated/asset item and its disposition
- the actual holder and year range proposed for `NOTICE`
- an explicit `PASS` or `BLOCKED` result

If the holder, authority, or a redistributed asset cannot be verified, mark the
gate `BLOCKED`, do not add `LICENSE`/metadata, and stop the licensing steps.
Package-boundary work may proceed, but Manja must not be described as
Apache-licensed.

- [ ] **Step 4: Document the artifact matrix**

`docs/legal/shipped-artifacts.md` must state for each artifact:

- how it is built
- which executable, frontend, and asset dependencies it redistributes
- where `LICENSE`, `NOTICE`, third-party notices, and SBOMs will be placed
- which test-only dependencies are intentionally absent
- the command that inspects the final artifact

Include source tarball, binary archive, OCI image, and the public `site`
artifact if it is distributed.

- [ ] **Step 5: Define inbound contribution terms**

`docs/legal/inbound-contributions.md` must select DCO or another explicit
Apache-2.0 inbound policy. It must not introduce a CLA or relicensing grant
unless dual licensing is an approved, current product requirement.

- [ ] **Step 6: Review and commit the evidence checkpoint**

Run:

```bash
git diff --check
rg -n 'TBD|TODO|unknown holder|assume ownership' docs/legal
```

Expected: no placeholder ownership claim. Any unresolved item is explicitly
marked `BLOCKED` with evidence and an owner for resolution.

```bash
git add docs/legal docs/superpowers/specs/2026-07-25-manja-contract-release-control-plane-design.md
git commit -m "docs(legal): establish open core provenance gate"
```

---

### Task 2: Add Failing External-Module And Architecture Gates

**Files:**
- Create: `integration/testdata/external-module/go.mod`
- Create: `integration/testdata/external-module/extension_test.go`
- Create: `architecture/public_boundary_test.go`
- Create: `architecture/context_boundary_test.go`
- Create: `architecture/domain_vocabulary_test.go`

**Interfaces:**
- Consumes: intended public package names.
- Produces: executable proof that an unrelated module can import Manja and that
  public dependency/context/domain boundaries remain enforceable.

- [ ] **Step 1: Write the unrelated module fixture**

Create:

```go
module example.com/manja-extension

go 1.26.1

require github.com/araihu/manja v0.0.0

replace github.com/araihu/manja => ../../..
```

`extension_test.go` must import:

```go
github.com/araihu/manja/application
github.com/araihu/manja/application/port
github.com/araihu/manja/contracttest
github.com/araihu/manja/domain
```

The test defines in-memory ports, constructs public review and sync services,
executes one use case, and invokes at least one public conformance suite. It
must not import any `internal` path.

- [ ] **Step 2: Run the fixture and verify RED**

Run:

```bash
cd integration/testdata/external-module
GOWORK=off go test ./... -count=1
```

Expected: compile failure because the public packages do not exist yet. Capture
the exact missing import errors in the task notes.

- [ ] **Step 3: Write dependency-direction tests**

`architecture/public_boundary_test.go` must enumerate imports with
`go list -deps -json` and fail when:

- `domain` imports anything outside the standard library
- `application` imports `internal`, `internal/adapters`, `internal/web`,
  generated API packages, Goshtoso, SQL, filesystem, Git, or HTTP drivers
- `application/port` imports infrastructure drivers
- `contracttest` imports self-hosted adapter construction

The test must also assert that `go list -m` returns
`github.com/araihu/manja`.

- [ ] **Step 4: Write context and vocabulary gates**

Use Go AST inspection to fail exported port methods and application service
operations that omit a first `context.Context` parameter; constructors and pure
configuration/value helpers are not operations. Add runtime context-spy
assertions in the external fixture. Scan `domain` identifiers and serialized field tags for
case-insensitive tenant, organization, billing, subscription, and entitlement
terms.

- [ ] **Step 5: Run architecture tests and verify RED**

Run:

```bash
GOWORK=off go test ./architecture -count=1
```

Expected: failure because the target packages and public contracts do not exist.

- [ ] **Step 6: Commit only the red tests**

```bash
git add integration/testdata/external-module architecture
git commit -m "test(architecture): require external Manja extension surface"
```

---

### Task 3: Promote The Pure Domain

**Files:**
- Create: `domain/project.go`
- Create: `domain/spec.go`
- Create: `domain/contractsnapshot.go`
- Create: `domain/specdiff.go`
- Create: `domain/policy.go`
- Create: `domain/review.go`
- Create: `domain/publication.go`
- Create: `domain/release.go`
- Move/modify: corresponding `internal/core/*_test.go` files into `domain/`
- Create temporarily: `internal/core/compat.go`

**Interfaces:**
- Produces: provider-neutral public domain types and pure behavior with no
  infrastructure imports.

- [ ] **Step 1: Move tests before implementations**

Change the existing pure-domain tests to `package domain` and run:

```bash
go test ./domain -count=1
```

Expected: compile failure until the public implementations move.

- [ ] **Step 2: Move pure entities and functions**

Move projects/contracts, sources and candidate data, immutable revisions,
snapshots, diff findings, policy, review documents, publications, and release
transitions into `domain`. Keep JSON/YAML compatibility and deterministic IDs
byte stable. Do not move:

- fetch, parser, store, blob, cache, clock, or ID interfaces
- sync/check orchestration
- HTTP view models or handlers
- source adapter configuration containing credentials

- [ ] **Step 3: Introduce a temporary compatibility bridge**

`internal/core/compat.go` may use type aliases and thin forwarding functions
so adapters can migrate in later tasks. It must be explicitly temporary and
must not define new behavior.

- [ ] **Step 4: Prove domain purity and parity**

Run:

```bash
go test ./domain ./internal/core -count=1
GOWORK=off go test ./architecture -run 'Domain|Module' -count=1
```

Expected: all migrated tests pass with unchanged golden review output and the
domain dependency test passes.

- [ ] **Step 5: Commit the public domain**

```bash
git add domain internal/core
git commit -m "refactor(domain): publish provider-neutral model"
```

---

### Task 4: Define Public Ports, Unit Of Work, Context, And Secrets

**Files:**
- Create: `application/port/operational.go`
- Create: `application/port/blob.go`
- Create: `application/port/source.go`
- Create: `application/port/parser.go`
- Create: `application/port/secret.go`
- Create: `application/port/support.go`
- Create: `application/port/port_test.go`
- Modify: `architecture/context_boundary_test.go`

**Interfaces:**
- Produces: stable infrastructure contracts that preserve atomic invariants and
  permit self-hosted or future tenant-aware implementations.

- [ ] **Step 1: Write failing port contract tests**

Require this transaction shape:

```go
type UnitOfWork interface {
    Within(
        context.Context,
        func(context.Context, OperationalStore) error,
    ) error
}

type OperationalStore interface {
    SaveRevision(context.Context, domain.ContractRevision) error
    SaveReview(context.Context, domain.ContractReview) error
    SaveSyncRecord(context.Context, domain.SyncRecord) error
    ReleaseTrack(context.Context, string, string) (domain.ReleaseTrack, error)
    SaveReleaseTrack(context.Context, uint64, domain.ReleaseTrack) error
    SavePublication(context.Context, domain.Publication) error
    AppendAuditEvent(context.Context, domain.AuditEvent) error
    Enqueue(context.Context, domain.OutboxMessage) error
}
```

The exact records may be grouped into focused read/write interfaces embedded by
`OperationalStore`, but one callback must be able to change all
consistency-sensitive records atomically. Do not expose independent production
constructors that make partial commits the only possible implementation.

- [ ] **Step 2: Specify blob consistency**

Define a content-addressed `BlobStore` with context-first `Put` and `Get`.
Contract tests must require:

- identical bytes produce an idempotent key/write
- a committed operational record never points to a missing blob
- application code writes the blob before entering `UnitOfWork`
- transaction failure leaves only an unreachable, replay-safe blob
- garbage collection cannot remove a blob referenced by committed state

Document this model in the interface comments and
`docs/legal/shipped-artifacts.md` only if blobs enter release artifacts.

- [ ] **Step 3: Define source, parser, and deterministic support ports**

Add context-first ports for source fetch/discovery, OpenAPI parsing/snapshot
building, clock, identifier generation, cache, actor resolution,
authorization, and asynchronous publication where current use cases need them.
Do not add speculative cloud-only ports.

- [ ] **Step 4: Define opaque secret resolution**

Use an opaque reference:

```go
type SecretRef struct {
    Name string
}

type SecretResolver interface {
    Resolve(context.Context, SecretRef) ([]byte, error)
}
```

The public source/application configuration carries `SecretRef`, never a raw
token, password, or SSH private key. Self-hosted CLI options may accept
credential file paths and construct the resolver/adapter outside
`application`.

- [ ] **Step 5: Prove incoming context identity**

Port fakes record the exact context object. Tests require the callback and every
port invocation to receive the same input context. Cancellation and deadlines
must be observable by the adapter.

- [ ] **Step 6: Run and commit the port boundary**

```bash
go test ./application/port ./architecture -count=1
git add application/port architecture/context_boundary_test.go
git commit -m "feat(application): publish transactional ports"
```

---

### Task 5: Move Reusable Use Cases Into Application

**Files:**
- Create: `application/check.go`
- Create: `application/check_test.go`
- Create: `application/sync.go`
- Create: `application/sync_test.go`
- Create: `application/revision.go`
- Create: `application/revision_test.go`
- Create: `application/release.go`
- Create: `application/release_test.go`
- Create: `application/errors.go`
- Modify: `cmd/manja/check.go`

**Interfaces:**
- Consumes: `domain` and injected `application/port` contracts.
- Produces: public deterministic review/check, sync, revision preparation, and
  release services with no adapter construction.

- [ ] **Step 1: Port the existing check tests and verify RED**

Move application-level cases from `internal/app/check_test.go` and sync
orchestration cases from `internal/core/sync_test.go`. Require constructors
that reject nil required ports and preserve canonical `manja.review/v1`
output.

```bash
go test ./application -run 'Check|Sync' -count=1
```

Expected: compile failure before the services exist.

- [ ] **Step 2: Implement injected check and sync services**

Move orchestration only. The application service:

1. accepts a context-first command
2. calls injected source/parser/snapshot/policy ports
3. writes content-addressed blobs before the operational transaction
4. commits revision, review, sync, audit, and outbox records in one
   `UnitOfWork` callback when the use case requires them
5. returns public typed errors that preserve causes

No constructor imports or selects self-hosted adapters.

- [ ] **Step 3: Add revision loading and release coordination**

Express release-track advancement through the same transaction callback:
persist the accepted review and sync evidence, verify expected generation, save
the new track/publication, append audit, and enqueue follow-up work atomically.
A failure leaves last-known-good public state unchanged. Independent tracks use
independent transactions in deterministic order so one rejected track does not
change another.

- [ ] **Step 4: Prove context, recovery, and deterministic behavior**

Cover:

- exact incoming context at every port
- canceled/deadline contexts
- missing/corrupt blobs
- repeated source event and review evidence replay
- transaction rollback at every write stage
- audit/outbox failure rollback
- stable error classification and deterministic ordering
- no source ref becoming public without policy and release transition

- [ ] **Step 5: Adapt the CLI to public application constructors**

`cmd/manja/check.go` composes local file/parser implementations and invokes
`application`; it must not call a second private review implementation.
Golden JSON/text/Markdown and exit-code tests remain unchanged.

- [ ] **Step 6: Run and commit application services**

```bash
go test ./application ./cmd/manja -count=1
GOWORK=off go test ./architecture -run 'Application|Context' -count=1
git add application cmd/manja/check.go cmd/manja/check_test.go
git commit -m "refactor(application): publish reusable use cases"
```

---

### Task 6: Separate Self-Hosted Composition And Remove Internal Reuse Barriers

**Files:**
- Create: `internal/selfhosted/server.go`
- Create: `internal/selfhosted/server_test.go`
- Modify: `cmd/manja/main.go`
- Modify: `cmd/manja/main_test.go`
- Modify: `internal/adapters/source/git.go`
- Modify: `internal/adapters/source/file.go`
- Modify: `internal/adapters/store/fs.go`
- Modify: `internal/web/server.go`
- Delete: `internal/app/app.go`
- Delete: `internal/app/app_test.go`
- Delete after imports migrate: `internal/core/compat.go`
- Delete or empty after migration: `internal/core/`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: public application constructors and self-hosted adapters.
- Produces: one self-hosted composition root with no reusable behavior trapped
  behind Go's `internal` rule.

- [ ] **Step 1: Write failing composition tests**

Require `internal/selfhosted.NewServer` to receive self-hosted options, create
concrete adapters, and inject them into public application constructors.
Require raw Git credentials to terminate at source/secret adapter construction.
No public option type may expose raw credential fields.

- [ ] **Step 2: Implement self-hosted wiring**

Move adapter selection, startup recovery, web handler construction, static
assets, and management compatibility wiring out of `internal/app`. Keep
`cmd/manja` as the executable composition entry point and
`internal/selfhosted` as an optional testable helper called only from the
command.

- [ ] **Step 3: Adapt filesystem storage to UnitOfWork**

Under one file-store lock:

1. load the complete operational state/generations needed by the callback
2. stage mutations in memory or a transaction directory
3. write and fsync complete temporary files
4. atomically publish the transaction manifest/state
5. recover or discard incomplete staging on restart

The implementation contract must cover crash points and prove no public track
can reference an uncommitted review/revision/publication. Blob writes follow the
separate content-addressed model from Task 4.

- [ ] **Step 4: Migrate handlers and adapters**

Self-hosted web handlers call application services. Adapters implement public
ports without leaking their concrete types into `application`. Generated API
types remain internal transport details.

- [ ] **Step 5: Remove compatibility aliases and old internal packages**

Update all imports to `domain`, `application`, or `application/port`,
then remove `internal/core/compat.go` and reusable `internal/app`. Update
`AGENTS.md` architecture guidance in the same commit so future work does not
reintroduce the old boundary.

- [ ] **Step 6: Turn the unrelated fixture GREEN**

Run:

```bash
(cd integration/testdata/external-module && GOWORK=off go test ./... -count=1)
GOWORK=off go test ./architecture -count=1
go test ./... -count=1
```

Expected: the unrelated module imports only public packages, injects in-memory
ports, and executes review and sync successfully.

- [ ] **Step 7: Commit the self-hosted separation**

```bash
git add AGENTS.md cmd/manja internal application domain integration architecture
git commit -m "refactor(selfhosted): isolate Manja composition root"
```

---

### Task 7: Publish Adapter Contract Tests

**Files:**
- Create: `contracttest/unitofwork.go`
- Create: `contracttest/blob.go`
- Create: `contracttest/source.go`
- Create: `contracttest/context.go`
- Create: `contracttest/doc.go`
- Create: `contracttest/contracttest_test.go`
- Modify: `internal/adapters/store/fs_test.go`
- Modify: `internal/adapters/source/file_test.go`
- Modify: `internal/adapters/source/git_test.go`
- Modify: `integration/testdata/external-module/extension_test.go`

**Interfaces:**
- Produces: exported, environment-neutral conformance suites for replaceable
  adapters.

- [ ] **Step 1: Write failing public suite self-tests**

Each suite accepts a factory and `testing.TB`; it never provisions databases,
containers, credentials, repositories, or cloud resources. Add deliberately
broken fake adapters and prove the suite detects:

- partial UnitOfWork commit
- changed/replaced context
- non-idempotent content-addressed blob writes
- nondeterministic source discovery ordering
- committed metadata pointing to a missing blob
- concurrent lost updates

- [ ] **Step 2: Implement focused conformance suites**

Export small entry points such as:

```go
func UnitOfWork(t *testing.T, factory UnitOfWorkFactory)
func BlobStore(t *testing.T, factory BlobStoreFactory)
func SourceFetcher(t *testing.T, factory SourceFactory)
```

Document lifecycle ownership, cleanup, concurrency, and required fixtures in
`contracttest/doc.go`.

- [ ] **Step 3: Run suites against self-hosted adapters**

Existing adapter tests invoke the public suites. Integration-only Git behavior
may supply a factory from the integration package, but `contracttest` itself
must remain free of Forgejo/Testcontainers imports.

- [ ] **Step 4: Run suites from the unrelated module**

The external fixture implements in-memory adapters and invokes at least
`UnitOfWork` and `BlobStore`. This proves the tests are usable outside
Manja's internal visibility tree.

- [ ] **Step 5: Run and commit contract tests**

```bash
go test ./contracttest ./internal/adapters/store ./internal/adapters/source -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -count=1)
git add contracttest internal/adapters integration/testdata/external-module
git commit -m "test(contract): publish adapter conformance suites"
```

---

### Task 8: Add Evidence-Backed Licensing, SBOM, And Artifact Gates

**Prerequisite:** `docs/legal/provenance.md` is explicitly `PASS` and names
the verified copyright holder/year range. If it is `BLOCKED`, do not perform
Steps 1-7; report the blocker at the review gate.

**Files:**
- Create: `LICENSE`
- Create: `NOTICE`
- Create: `THIRD_PARTY_NOTICES.md`
- Create: `scripts/check-license-policy.mjs`
- Create: `scripts/check-license-policy.test.mjs`
- Create: `scripts/generate-sbom.mjs`
- Create: `scripts/package-release.mjs`
- Create: `scripts/package-release.test.mjs`
- Modify: `package.json`
- Modify: `Dockerfile`
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`
- Modify: `docs/legal/shipped-artifacts.md`

**Interfaces:**
- Consumes: the passed provenance inventory and locked Go/npm dependency graph.
- Produces: accurate Apache-2.0 repository materials, third-party notices,
  CycloneDX SBOMs, and release artifacts that carry them.

- [ ] **Step 1: Add exact license and notice materials**

Copy the unmodified Apache License 2.0 text into root `LICENSE`. Generate
`NOTICE` only from the verified holder/year evidence and mandatory
attributions. Do not add dependency license summaries to `NOTICE` when they
belong in `THIRD_PARTY_NOTICES.md`.

- [ ] **Step 2: Write failing dependency classification tests**

`check-license-policy.mjs` must consume a reviewed allow/deny policy and emit
deterministically sorted records for:

- direct and transitive Go modules used in shipped binaries
- npm packages whose code/assets enter shipped frontend bundles
- build-only dependencies
- test-only dependencies, including Forgejo Testcontainers
- copied/generated/static assets from the provenance inventory

Tests must fail for missing/unknown license evidence, forbidden shipped
licenses, misclassified test-only packages, and stale third-party notices.
They must not label every `go.mod` entry as redistributed.

- [ ] **Step 3: Add reproducible SBOM generation**

Implement:

```bash
npm run licenses:check
npm run sbom:generate
```

Generate deterministic CycloneDX JSON for the Go binary, bundled npm assets,
and final OCI filesystem. Pin any external scanner/generator version in the
repository lock or CI configuration, record it in the SBOM metadata, and sort
or normalize volatile fields before drift comparison.

- [ ] **Step 4: Package license material with every artifact**

`scripts/package-release.mjs` must create a staging tree containing:

```text
bin/manja
LICENSE
NOTICE
THIRD_PARTY_NOTICES.md
sbom/manja-go.cdx.json
sbom/manja-npm.cdx.json
```

Add site-specific files/SBOM when the site is distributed. The Dockerfile must
copy the notice files and OCI SBOM/inventory to a documented location such as
`/usr/share/licenses/manja/`.

- [ ] **Step 5: Test final artifacts, not only source files**

Tests unpack the source and binary archives and inspect the built OCI image.
Require exact license/notice presence and verify that test-only dependencies do
not appear in shipped notice/SBOM scopes unless the final artifact actually
contains them.

- [ ] **Step 6: Add CI and metadata only after the gate is green**

CI runs license policy, SBOM drift, package inspection, root tests, and site
tests. Only now add Apache-2.0 badges/metadata and document semantic-versioning
expectations for public Go packages, `api/`, and the CLI.

- [ ] **Step 7: Run the licensing and artifact gate**

```bash
npm run licenses:check
npm run sbom:generate
npm run release:package
npm run release:test
git diff --check
```

Expected: all commands exit 0, generated inventories are deterministic, and
every packaged artifact carries the verified materials.

- [ ] **Step 8: Commit licensing and release evidence**

```bash
git add LICENSE NOTICE THIRD_PARTY_NOTICES.md README.md package.json Dockerfile .github/workflows/ci.yml scripts docs/legal
git commit -m "build(release): verify Apache licensing artifacts"
```

---

## Final Review Gate

Run fresh:

```bash
git status --short
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
git diff --check origin/main...HEAD
GOWORK=off go test ./architecture -count=1
(cd integration/testdata/external-module && GOWORK=off go test ./... -count=1)
go test ./...
(cd site && GOWORK=off go test ./...)
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
npm run licenses:check
npm run sbom:generate
npm run release:test
git status --short
```

Run integration tests when the storage/source migration touches Forgejo or
container-backed paths:

```bash
go test -tags=integration ./internal/integration -v
```

Review requirements line by line:

- public module path remains `github.com/araihu/manja`
- unrelated external module imports and executes public use cases
- `domain` and `application` do not import internal/infrastructure packages
- every application/port operation is context first and preserves context
- all reusable infrastructure is injected
- operational `UnitOfWork` can protect complete release invariants
- blob consistency, replay, orphan recovery, and garbage collection are explicit
- raw credentials terminate at self-hosted composition
- no SaaS nouns or behavior entered the public domain
- self-hosted Manja remains complete and read-only on public docs
- public Go/OpenAPI/CLI compatibility follows semantic versioning
- provenance gate is `PASS` before any Apache-2.0 claim
- notices and SBOMs describe shipped, not merely test/build, dependencies
- source, binary, OCI, and site artifacts carry required materials
- no Goshtoso snag is left unrecorded

Stop at this gate with the branch/worktree, commit range, fresh command output,
provenance result, artifact evidence, and review findings. Do not merge, push,
or clean the worktree without explicit direction.
