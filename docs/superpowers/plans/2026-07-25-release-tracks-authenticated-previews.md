# Release Tracks And Authenticated Previews Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace mutable publication lookup with persistent release tracks that serve immutable last-known-good revisions, advance through deterministic pinned/following transitions, survive restarts and failed syncs, and expose authenticated no-index previews for stored revisions.

**Architecture:** Add provider-neutral release-track and stored-review types plus pure transition functions in `internal/core`; keep filesystem persistence behind narrow release/revision ports and use generation-checked atomic JSON replacement for track state. Application services load immutable revision artifacts, evaluate release-impact reviews with the shared analysis/policy core, migrate legacy publications, and resolve both public track docs and authenticated revision previews through callbacks consumed by `internal/web`. Repository `.manja.yaml` remains portable policy; a separate deployment-owned release config contains paths, hostnames, ref bindings, modes, and server-added policy.

**Tech Stack:** Go 1.26.1, standard-library `crypto/sha256`, `crypto/subtle`, `encoding/json`, `net/http`, `sync`, and filesystem primitives; existing kin-openapi parser, templ v0.3.1020, Goshtoso v0.0.10, and YAML v3.0.1.

## Global Constraints

- Work only in `/tmp/manja-release-tracks-previews` on `codex/release-tracks-previews`, created from `origin/main` at `58fb3ddbb2ee47d20d5daa5c13acfbf7b6c9fa85`.
- Treat the current `Project.ID` as the contract identity at compatibility boundaries; new release types use `ContractID` and do not introduce provider-specific objects.
- A ref is never public state. Public routing resolves `hostname/path -> ReleaseTrack -> CurrentRevisionID -> stored immutable blob -> parser/index`.
- Pinned tracks only change `CurrentRevisionID` through explicit `PromoteRelease`; following tracks only advance after a passing persisted release-impact review.
- Every failed fetch, parse, snapshot, policy, review-save, or generation-checked track-save leaves `CurrentRevisionID` unchanged.
- Repository policy remains in `.manja.yaml`. Public routes, hostnames, track modes, ref selectors, and server-added rules live only in deployment-owned release config.
- Preview routes are `/preview/{contractID}/{revisionID}` plus revision-local subroutes. Authentication runs before revision lookup, every response is `noindex`, and preview routes never enter public track lookup or sitemap generation.
- Ship a narrow replaceable request-authentication boundary and a self-hosted HTTP Basic adapter whose password is read from a file. Do not place credentials in URLs, YAML, logs, HTML, or persisted track/review documents.
- OIDC/Dex lifecycle, connected baseline/evidence APIs, CI tokens, provider actions, management information-architecture migration, and new promotion REST endpoints remain outside this subproject.
- Keep the existing management pages and legacy publication POST contract working while migration compatibility is present; do not extend the old peer-tab information architecture.
- Preserve the read-only renderer. Do not add Try It, OpenAPI authoring, or upstream request proxying.
- Never hand-edit `internal/web/templates/*_templ.go`; regenerate after `.templ` edits.
- Keep canonical review schema `manja.review/v1`; the new server-side release review uses the same report model with one `release_impact` comparison.
- Record any Goshtoso component/helper/docs/generated-templ source dive as a snag before the review gate.

## File Structure

- `internal/core/release.go`: release-track, stored-review, and history types; validation; deterministic IDs; pinned/following transitions.
- `internal/core/release_test.go`: domain invariants, replay idempotency, generation behavior, and last-known-good preservation.
- `internal/core/review.go`: release-impact-only evaluation using the existing canonical report and policy projection.
- `internal/core/spec.go`: immutable revision artifact metadata needed after restart.
- `internal/core/sync.go`: populate immutable revision metadata before persistence.
- `internal/core/ports.go`: narrow contract-revision and release-state persistence ports alongside the legacy store.
- `internal/adapters/store/fs.go`: contract-scoped revision lookup, atomic release/review storage, compare-and-swap updates, deterministic listing, and legacy publication listing.
- `internal/adapters/config/server.go`: strict deployment-owned release-track and preview-auth configuration.
- `internal/adapters/config/testdata/server.yaml`: two-track server fixture using `v1` and `v2`.
- `internal/adapters/auth/basic.go`: constant-time HTTP Basic authenticator backed by an injected password or password file.
- `internal/app/revisiondocs.go`: load a stored revision, blob, parser index, and contract snapshot after restart.
- `internal/app/release.go`: migrate legacy publications, reconcile configured track definitions, evaluate mapped sync results, persist reviews, and apply track transitions.
- `internal/app/app.go`: recovery-first startup, release sync wiring, dynamic docs resolvers, and compatibility management state.
- `internal/web/docs.go`: public-track request matching, preview path parsing/authentication, path rewriting, and response policy.
- `internal/web/public.go`: renderer base-path and indexability options.
- `internal/web/templates/layout.templ`: optional robots metadata.
- `internal/web/templates/public.templ`: base-path-aware home, search, download, HTMX, and public-route links.
- `cmd/manja/main.go`: optional repository policy, server release config, and preview password-file flags.
- `README.md`: release config, migration, authentication, routes, and failure behavior.

---

### Task 1: Canonical Release Reviews And Pure Track Transitions

**Files:**
- Create: `internal/core/release.go`
- Create: `internal/core/release_test.go`
- Modify: `internal/core/review.go`
- Modify: `internal/core/review_test.go`

**Interfaces:**
- Consumes: `ContractSnapshot`, `EffectivePolicy`, `EvaluateFindings`, `ReviewReport`, and `CanonicalReviewJSON`.
- Produces: `EvaluateReleaseReview`, `StoredReview`, `ReleaseTrack`, `ConsiderReleaseReview`, `PromoteRelease`, and deterministic review/event IDs.

- [ ] **Step 1: Write failing release-impact review tests**

Add tests requiring a single `release_impact` comparison and canonical stability:

```go
func TestEvaluateReleaseReviewProducesCanonicalReleaseImpact(t *testing.T) {
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	baseline := NewContractSnapshot("payments", "rev-v1", []byte("v1"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	candidate := NewContractSnapshot("payments", "rev-v2", []byte("v2"), SpecIndex{})
	policy, err := MergePolicy(PolicyLayer{Name: "stable", Source: PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}

	report, err := EvaluateReleaseReview(ReleaseReviewRequest{
		ContractID: "payments", Baseline: baseline, Candidate: candidate,
		Policy: policy, EvaluatedAt: at, EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Comparisons) != 1 || report.Comparisons[0].Kind != ComparisonReleaseImpact {
		t.Fatalf("comparisons = %#v", report.Comparisons)
	}
	first, _ := CanonicalReviewJSON(report)
	second, _ := CanonicalReviewJSON(report)
	if !bytes.Equal(first, second) {
		t.Fatalf("canonical reports differ:\n%s\n%s", first, second)
	}
}
```

Also test contract mismatch, zero evaluation time, blank engine version, and a
missing immutable baseline. Release-track advancement never invents an empty
baseline; a new track must be explicitly bootstrapped by migration or a future
authorized workflow.

- [ ] **Step 2: Run the release-review tests and verify RED**

Run:

```bash
go test ./internal/core -run TestEvaluateReleaseReview -count=1
```

Expected: compile failure because `ReleaseReviewRequest` and
`EvaluateReleaseReview` do not exist.

- [ ] **Step 3: Implement release-impact evaluation by reusing canonical helpers**

Add:

```go
type ReleaseReviewRequest struct {
	ContractID    string
	Baseline      ContractSnapshot
	Candidate     ContractSnapshot
	Policy        EffectivePolicy
	EvaluatedAt   time.Time
	EngineVersion string
}

func EvaluateReleaseReview(request ReleaseReviewRequest) (ReviewReport, error)
```

Validate and clone both snapshots with the same helpers as `EvaluateReview`,
normalize policy with `normalizeEffectivePolicy`, produce exactly one
`evaluateComparison(ComparisonReleaseImpact, ...)`, and set the overall verdict
from that comparison. Extract shared report construction only after both old
and new review tests are green.

- [ ] **Step 4: Write failing track transition tests**

Use these public shapes in the tests:

```go
type AdvancementMode string

const (
	AdvancementPinned    AdvancementMode = "pinned"
	AdvancementFollowing AdvancementMode = "following"
)

type StoredReview struct {
	ID                  string       `json:"id"`
	ContractID          string       `json:"contractId"`
	TrackID             string       `json:"trackId"`
	SourceRef           string       `json:"sourceRef"`
	BaselineRevisionID  string       `json:"baselineRevisionId"`
	CandidateRevisionID string       `json:"candidateRevisionId"`
	Report              ReviewReport `json:"report"`
	CreatedAt           time.Time    `json:"createdAt"`
}

type ReleaseEvent struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	RevisionID     string    `json:"revisionId"`
	ReviewID       string    `json:"reviewId,omitempty"`
	ActorID        string    `json:"actorId,omitempty"`
	PreviousID     string    `json:"previousRevisionId,omitempty"`
	OccurredAt     time.Time `json:"occurredAt"`
	StateGeneration uint64   `json:"stateGeneration"`
}

type ReleaseTrack struct {
	ID                    string          `json:"id"`
	ContractID            string          `json:"contractId"`
	SourceID              string          `json:"sourceId"`
	RefSelector           string          `json:"refSelector,omitempty"`
	PublicPath            string          `json:"publicPath"`
	Hostname              string          `json:"hostname,omitempty"`
	PolicyProfile         string          `json:"policyProfile"`
	ServerPolicy          PolicyLayer     `json:"serverPolicy"`
	Mode                  AdvancementMode `json:"mode"`
	CurrentRevisionID     string          `json:"currentRevisionId,omitempty"`
	CandidateRevisionID   string          `json:"candidateRevisionId,omitempty"`
	CandidateReviewID     string          `json:"candidateReviewId,omitempty"`
	Generation            uint64          `json:"generation"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
	History               []ReleaseEvent  `json:"history"`
}
```

Cover:

- a passing review on a pinned track sets candidate only
- explicit promotion advances current and clears candidate
- a passing review on a following track advances current immediately
- a failing review, ref mismatch, contract mismatch, or stale baseline leaves
  current unchanged
- replaying the same review produces `changed == false`
- event IDs and stored-review IDs are deterministic
- mode, path, hostname, policy source, timestamps, and IDs are validated

- [ ] **Step 5: Run transition tests and verify RED**

Run:

```bash
go test ./internal/core -run 'Test(ConsiderReleaseReview|PromoteRelease|NewStoredReview|ValidateReleaseTrack)' -count=1
```

Expected: compile failure because release types and functions do not exist.

- [ ] **Step 6: Implement minimal provider-neutral release state**

Implement:

```go
func NewStoredReview(track ReleaseTrack, sourceRef string, report ReviewReport, createdAt time.Time) (StoredReview, error)
func ValidateReleaseTrack(track ReleaseTrack) error
func ConsiderReleaseReview(track ReleaseTrack, review StoredReview) (next ReleaseTrack, changed bool, err error)
func PromoteRelease(track ReleaseTrack, review StoredReview, actorID string, occurredAt time.Time) (next ReleaseTrack, changed bool, err error)
```

Hash the canonical report plus NUL-delimited contract, track, baseline,
candidate, and source-ref identities for `StoredReview.ID`. Hash the event kind,
track identity, next generation, revision, review, actor, and UTC timestamp for
`ReleaseEvent.ID`. Clone slices/maps before returning; do not mutate caller
state. `ConsiderReleaseReview` accepts only `VerdictPass`; pinned mode records
the accepted candidate, following mode atomically projects it as current.
`PromoteRelease` accepts only the currently recorded candidate and review.

- [ ] **Step 7: Run all core release/review tests**

Run:

```bash
go test ./internal/core -run 'Release|Review|Policy|ContractSnapshot' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit core release behavior**

```bash
git add internal/core/release.go internal/core/release_test.go internal/core/review.go internal/core/review_test.go
git commit -m "feat(core): model deterministic release tracks"
```

---

### Task 2: Immutable Revision Artifact Metadata

**Files:**
- Modify: `internal/core/spec.go`
- Modify: `internal/core/sync.go`
- Modify: `internal/core/sync_test.go`
- Modify: `internal/core/ports.go`
- Modify: `internal/adapters/store/fs.go`
- Modify: `internal/adapters/store/fs_test.go`

**Interfaces:**
- Produces: restart-safe revision artifact metadata and
  `ContractRevisionStore.ContractRevision`.
- Preserves: legacy `Store.Revision(ctx, revisionID)` while migration is active.

- [ ] **Step 1: Write failing sync metadata and immutability tests**

Require successful sync to persist:

```go
type Revision struct {
	ID          string
	ContractID  string
	SourceID    string
	Ref         string
	CommitSHA   string
	Version     string
	AuthorName  string
	AuthorEmail string
	Message     string
	SpecPath    string
	SpecFormat  string
	SpecDigest  string
	BlobKey     string
	CreatedAt   time.Time
}
```

Assert `SpecDigest` is lowercase SHA-256 of raw bytes, `BlobKey` equals
`SpecBlobKey`, and `CreatedAt` uses the injected `Syncer.Now`. Add filesystem
tests that:

- save and load the same revision under
  `revisions/{contractID}/{revisionID}.json`
- replay an identical save successfully
- reject the same contract/revision identity with changed immutable fields
- allow the same revision ID in two contracts
- read the old flat revision file through the legacy method

- [ ] **Step 2: Run targeted tests and verify RED**

Run:

```bash
go test ./internal/core ./internal/adapters/store -run 'Revision|Sync.*Metadata' -count=1
```

Expected: assertions or compile failure because contract/artifact fields and
contract-scoped lookup do not exist.

- [ ] **Step 3: Add the narrow contract-revision port**

Add without expanding every existing store fake:

```go
type ContractRevisionStore interface {
	SaveRevision(context.Context, Revision) error
	ContractRevision(context.Context, string, string) (Revision, error)
}
```

Keep the existing `Store` interface for current sync/management compatibility.

- [ ] **Step 4: Populate immutable metadata before blob/revision persistence**

After parse succeeds and before `Blobs.Put`, normalize `Revision.ContractID`,
`SourceID`, `SpecPath`, `SpecFormat`, `SpecDigest`, `BlobKey`, and `CreatedAt`.
Use the same prepared revision in the blob key, revision file, sync record, and
returned `SyncResult`. Do not persist a revision if blob storage fails.

- [ ] **Step 5: Implement contract-scoped immutable filesystem storage**

Use `revisions/{contractID}/{revisionID}.json` for revisions carrying a contract
ID. Under the store mutex, read any existing record and compare every immutable
field; return a descriptive conflict instead of overwriting different content.
Write identical replays as no-ops. Keep legacy flat lookup only for migration.

- [ ] **Step 6: Run sync/store and full core tests**

Run:

```bash
go test ./internal/core ./internal/adapters/store -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit immutable revision metadata**

```bash
git add internal/core/spec.go internal/core/sync.go internal/core/sync_test.go internal/core/ports.go internal/adapters/store/fs.go internal/adapters/store/fs_test.go
git commit -m "feat(store): persist immutable revision artifacts"
```

---

### Task 3: Atomic Release And Review Persistence

**Files:**
- Modify: `internal/core/ports.go`
- Modify: `internal/adapters/store/fs.go`
- Modify: `internal/adapters/store/fs_test.go`

**Interfaces:**
- Produces: `ReleaseStore`, atomic compare-and-swap track updates, deterministic
  listing, public request lookup, and persisted canonical reviews.

- [ ] **Step 1: Write failing persistence and recovery tests**

Add tests that create `v1` and `v2` tracks plus reviews, reopen a new
`FileStore` at the same root, and require byte-equivalent state. Cover:

- deterministic `ReleaseTracks(contractID)` ordering by track ID
- lookup by `(contractID, trackID)` and review ID
- longest segment-boundary public-path match
- hostname matching after lowercase and request-port removal
- duplicate hostname/path rejection across tracks
- generation-checked update succeeds once and rejects a stale replay
- a failed stale update leaves the on-disk current revision unchanged
- malformed/truncated JSON returns an error and never becomes public
- review replay with identical bytes is a no-op; conflicting bytes are rejected

- [ ] **Step 2: Run store tests and verify RED**

Run:

```bash
go test ./internal/adapters/store -run 'ReleaseTrack|StoredReview|PublicRelease' -count=1
```

Expected: compile failure because release persistence does not exist.

- [ ] **Step 3: Add focused release ports**

Add:

```go
type ReleaseStore interface {
	SaveReleaseTrack(context.Context, ReleaseTrack) error
	ReleaseTrack(context.Context, string, string) (ReleaseTrack, error)
	ReleaseTracks(context.Context, string) ([]ReleaseTrack, error)
	PublicReleaseTrack(context.Context, string, string) (ReleaseTrack, error)
	CompareAndSwapReleaseTrack(context.Context, uint64, ReleaseTrack) error
	SaveStoredReview(context.Context, StoredReview) error
	StoredReview(context.Context, string, string) (StoredReview, error)
}

type LegacyPublicationStore interface {
	SavePublication(context.Context, Publication) error
	Publications(context.Context) ([]Publication, error)
}

type LegacyRevisionStore interface {
	ContractRevisionStore
	Revision(context.Context, string) (Revision, error)
}
```

`PublicReleaseTrack` accepts request hostname and path; it returns only tracks
with a non-empty current revision and an exact hostname plus longest safe path
prefix match.

- [ ] **Step 4: Replace direct JSON writes with atomic replacement**

Implement `writeJSONAtomic`:

1. marshal complete JSON plus one newline
2. create a temp file in the destination directory
3. chmod `0600`, write, `Sync`, and close
4. rename over the destination
5. remove the temp file on every error path

Guard release/review reads and writes with a `sync.RWMutex` on `FileStore`.
Perform generation read/check/write while holding the write lock. Do not expose
temp files through list methods.

- [ ] **Step 5: Implement deterministic paths and route collision checks**

Store:

```text
release-tracks/{contractID}/{trackID}.json
reviews/{contractID}/{reviewID}.json
```

Validate IDs with the existing safe-ID rules. Normalize public paths with
`path.Clean`, require a leading slash, preserve `/`, lowercase hostnames, and
strip request ports with `net.SplitHostPort`. Reject path-prefix matches that do
not end on a segment boundary.

- [ ] **Step 6: Run store tests including restart and stale-generation cases**

Run:

```bash
go test ./internal/adapters/store -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit release persistence**

```bash
git add internal/core/ports.go internal/adapters/store/fs.go internal/adapters/store/fs_test.go
git commit -m "feat(store): persist atomic release state"
```

---

### Task 4: Deployment Release Configuration And Legacy Migration

**Files:**
- Create: `internal/adapters/config/server.go`
- Create: `internal/adapters/config/server_test.go`
- Create: `internal/adapters/config/testdata/server.yaml`
- Create: `internal/app/migration.go`
- Create: `internal/app/migration_test.go`

**Interfaces:**
- Consumes: repository `ContractConfig.PolicyLayer`, legacy publications,
  contract revisions, and `ReleaseStore`.
- Produces: strict deployment track definitions, idempotent config
  reconciliation, and publication-to-pinned-track migration.

- [ ] **Step 1: Write failing strict server-config tests**

Use this fixture shape:

```yaml
version: 1
tracks:
  - id: v1
    contract: payments
    source: payments-git
    ref: refs/heads/v1
    path: /payments/v1
    mode: pinned
    policy: stable
    serverPolicy:
      rules:
        schema.removed: fail
  - id: v2
    contract: payments
    source: payments-git
    ref: refs/heads/v2
    path: /payments/v2
    mode: following
    policy: next
```

Require `KnownFields(true)`, one YAML document, version 1, unique
contract/track IDs, unique hostname/path routes, safe IDs/paths, valid modes,
nonblank repository policy names, and server rules that convert to
`PolicySourceServer`. Reject preview credentials in YAML.

- [ ] **Step 2: Run config tests and verify RED**

Run:

```bash
go test ./internal/adapters/config -run 'Server|ReleaseTrack' -count=1
```

Expected: compile failure because server config types do not exist.

- [ ] **Step 3: Implement deployment-owned server configuration**

Expose:

```go
type ServerFile struct {
	Version int                 `yaml:"version"`
	Tracks  []ServerTrackConfig `yaml:"tracks"`
}

type ServerTrackConfig struct {
	ID            string             `yaml:"id"`
	ContractID    string             `yaml:"contract"`
	SourceID      string             `yaml:"source"`
	RefSelector   string             `yaml:"ref"`
	PublicPath    string             `yaml:"path"`
	Hostname      string             `yaml:"hostname"`
	Mode          string             `yaml:"mode"`
	PolicyProfile string             `yaml:"policy"`
	ServerPolicy ServerPolicyConfig `yaml:"serverPolicy"`
}

func LoadServer(path string) (ServerFile, error)
func (f ServerFile) ReleaseTracks(now time.Time) ([]core.ReleaseTrack, error)
```

Keep repository profiles out of this file; store only the selected profile
name and server strengthening layer.

- [ ] **Step 4: Write failing legacy migration tests**

Cover:

- `/payments/v1` becomes pinned track `v1`
- `/` or an unusable final segment becomes `default`
- two colliding derived IDs gain a stable digest suffix
- path, hostname, project/contract, and current revision are preserved exactly
- private publications are not migrated
- old flat revision metadata is copied into contract-scoped storage without
  moving or deleting the blob
- rerunning migration is a no-op
- a pre-existing conflicting track fails closed without changing either record
- a one-time implicit root publication may be seeded only when no public
  publication and no release track exists

- [ ] **Step 5: Run migration tests and verify RED**

Run:

```bash
go test ./internal/app -run 'Migration|LegacyPublication' -count=1
```

Expected: compile failure because the migrator does not exist.

- [ ] **Step 6: Implement idempotent migration and configuration reconciliation**

Add:

```go
type ReleaseMigrator struct {
	Legacy   core.LegacyPublicationStore
	Revisions core.LegacyRevisionStore
	Releases core.ReleaseStore
	Now      func() time.Time
}

func (m ReleaseMigrator) Migrate(ctx context.Context) error
func (m ReleaseMigrator) SeedImplicitRoot(ctx context.Context, contractID, sourceID, revisionID string) error

type TrackReconciler struct {
	Releases core.ReleaseStore
	Now      func() time.Time
}

func (r TrackReconciler) Reconcile(ctx context.Context, definitions []core.ReleaseTrack) error
```

Reconciliation may update route, hostname, ref selector, mode, policy profile,
and server policy while preserving current/candidate revisions, generation, and
history. Use compare-and-swap; never reset public state from configuration.

- [ ] **Step 7: Run config, migration, and store tests**

Run:

```bash
go test ./internal/adapters/config ./internal/adapters/store ./internal/app -run 'Server|Release|Migration|Publication' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit configuration and migration**

```bash
git add internal/adapters/config/server.go internal/adapters/config/server_test.go internal/adapters/config/testdata/server.yaml internal/app/migration.go internal/app/migration_test.go
git commit -m "feat(release): migrate publications to tracks"
```

---

### Task 5: Restart-Safe Revision Loading And Release Coordination

**Files:**
- Create: `internal/app/revisiondocs.go`
- Create: `internal/app/revisiondocs_test.go`
- Create: `internal/app/release.go`
- Create: `internal/app/release_test.go`
- Modify: `internal/core/ports.go`

**Interfaces:**
- Consumes: `ContractRevisionStore`, `BlobStore`, `Parser`,
  `ContractSnapshotBuilder`, repository policy, and `ReleaseStore`.
- Produces: `RevisionDocsService.Load`, `ReleaseService.ApplySyncResult`,
  `ReleaseService.Promote`, and read-only public/preview resolvers.

- [ ] **Step 1: Write failing revision loader tests**

Require:

```go
type RevisionDocs struct {
	Revision core.Revision
	File     core.SpecFile
	Index    core.SpecIndex
	Snapshot core.ContractSnapshot
}

type RevisionDocsService struct {
	Revisions core.ContractRevisionStore
	Blobs     core.BlobStore
	Parser    core.Parser
	Snapshots core.ContractSnapshotBuilder
}

func (s RevisionDocsService) Load(context.Context, string, string) (RevisionDocs, error)
```

Test exact call order and identity propagation. Add corrupt/missing blob, parse
failure, mismatched contract, and context-cancellation cases. Confirm loading a
stored revision does not fetch a source or mutate release state.

- [ ] **Step 2: Run loader tests and verify RED**

Run:

```bash
go test ./internal/app -run TestRevisionDocsService -count=1
```

Expected: compile failure because the service does not exist.

- [ ] **Step 3: Implement immutable artifact loading**

Read contract-scoped revision metadata, fetch exactly `Revision.BlobKey`, parse
with its stored spec path/format, set index contract/revision identities, and
build the snapshot from the same bytes. Wrap errors by stage:
`load revision`, `load revision blob`, `parse revision`, and
`build revision snapshot`.

- [ ] **Step 4: Write failing release coordinator tests**

Define:

```go
type ReleaseService struct {
	Releases     core.ReleaseStore
	Docs         RevisionDocsService
	Snapshots    core.ContractSnapshotBuilder
	RepoPolicies func(contractID, profile string) (core.PolicyLayer, error)
	Now          func() time.Time
	EngineVersion string
}

func (s ReleaseService) ApplySyncResult(context.Context, core.SyncResult) error
func (s ReleaseService) Promote(context.Context, string, string, string, string) error
```

Tests must prove:

- only tracks whose contract/source/ref selector match the synced immutable
  revision are considered
- current baseline is loaded from storage, never from mutable source state
- repository and server policy merge monotonically
- review is saved before track compare-and-swap
- pinned pass records candidate only
- following pass advances current
- failed policy saves review evidence but preserves current
- parse/snapshot/policy/review-save/CAS failures preserve current
- repeating the same sync produces no duplicate event or generation
- two concurrent generation attempts allow one winner and force the loser to
  reload/re-evaluate once; a second stale result returns a conflict
- `Promote` loads the stored accepted review and delegates to core transition

- [ ] **Step 5: Run coordinator tests and verify RED**

Run:

```bash
go test ./internal/app -run 'TestReleaseService' -count=1
```

Expected: compile failure because the coordinator does not exist.

- [ ] **Step 6: Implement review-first, generation-checked coordination**

For each matching track:

1. load the current immutable baseline
2. build/reuse the candidate snapshot from `SyncResult`
3. merge selected repository policy with the track server layer
4. call `EvaluateReleaseReview`
5. call `NewStoredReview`
6. persist the review
7. compute the pure transition
8. compare-and-swap the whole track document

Sort tracks by contract/ID before processing. Continue independent tracks after
a policy rejection, but return joined infrastructure errors after all tracks
have been attempted. Never let failure on `v2` change `v1`.

- [ ] **Step 7: Run application, core, and store tests**

Run:

```bash
go test ./internal/app ./internal/core ./internal/adapters/store -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit revision loading and release coordination**

```bash
git add internal/app/revisiondocs.go internal/app/revisiondocs_test.go internal/app/release.go internal/app/release_test.go internal/core/ports.go
git commit -m "feat(release): coordinate reviewed track advancement"
```

---

### Task 6: Dynamic Last-Known-Good Public Routing

**Files:**
- Create: `internal/web/docs.go`
- Create: `internal/web/docs_test.go`
- Modify: `internal/web/public.go`
- Modify: `internal/web/public_test.go`
- Modify: `internal/web/server.go`
- Modify: `internal/web/seo_test.go`
- Modify: `internal/web/templates/layout.templ`
- Modify: `internal/web/templates/public.templ`
- Regenerate: `internal/web/templates/layout_templ.go`
- Regenerate: `internal/web/templates/public_templ.go`

**Interfaces:**
- Consumes: read-only resolver callbacks returning a track plus immutable
  `SpecIndex`.
- Produces: base-path-aware renderer, public track router, and preview renderer
  policy shared by Task 7.

- [ ] **Step 1: Read the templ escaping gotchas before editing templates**

Read:

```bash
cat .agents/skills/templ/reference/gotchas.md
```

Do not change existing Alpine or HTMX JavaScript expressions unless a failing
test requires it.

- [ ] **Step 2: Write failing base-path renderer tests**

Extend `PublicOptions` and `templates.PublicDocsOptions` with:

```go
BasePath string
NoIndex bool
```

Render at `/payments/v1` and require:

- home and generated public-route links stay below `/payments/v1`
- search loads `/payments/v1/search.json`
- download uses `/payments/v1/openapi.json`
- HTMX fragment links keep the same base
- public static assets remain shared at `/assets/` and `/manja-assets/`
- `NoIndex=true` emits `<meta name="robots" content="noindex,nofollow,noarchive">`
  and an `X-Robots-Tag` header
- sitemap is absent when `NoIndex=true`

- [ ] **Step 3: Run renderer tests and verify RED**

Run:

```bash
go test ./internal/web -run 'BasePath|NoIndex|Sitemap' -count=1
```

Expected: assertion or compile failure because renderer options are not
base-path/indexability aware.

- [ ] **Step 4: Parameterize renderer URLs without duplicating templates**

Add a `docsURL(basePath, localPath string) string` helper and use it for home,
search, download, selected route, and branding defaults. Add a small layout
options type so only preview pages emit robots metadata. Keep all user/source
URLs passed through `templ.URL`; do not use `templ.SafeURL`.

- [ ] **Step 5: Regenerate templ output and rerun renderer tests**

Run:

```bash
go run github.com/a-h/templ/cmd/templ generate
go test ./internal/web -run 'BasePath|NoIndex|Sitemap|Public' -count=1
```

Expected: PASS with generated changes only in matching `_templ.go` files.

- [ ] **Step 6: Write failing dynamic public-route tests**

Define web-level callbacks:

```go
type ResolvedTrackDocs struct {
	Track core.ReleaseTrack
	Index core.SpecIndex
}

type PublicDocsResolver func(context.Context, string, string) (ResolvedTrackDocs, error)
type PreviewDocsResolver func(context.Context, string, string) (core.SpecIndex, error)
```

Test two tracks of one contract serving different titles/versions and search
payloads. Verify request path stripping for:

```text
/payments/v1
/payments/v1?selected=operation
/payments/v1/search.json
/payments/v1/openapi.json
```

Also verify unknown/empty-current tracks return 404, a resolver failure returns
503 without error detail, the longest safe route wins, and `v1` failure does not
change or contaminate `v2`.

- [ ] **Step 7: Run router tests and verify RED**

Run:

```bash
go test ./internal/web -run 'TrackDocs|DynamicPublic' -count=1
```

Expected: compile failure because dynamic docs routing does not exist.

- [ ] **Step 8: Implement dynamic request resolution**

Resolve track state on every request (the filesystem adapter may cache later),
build a renderer for the resolved immutable index, clone the request, strip only
the matched base path, and preserve query/fragment semantics. Mount shared
assets before dynamic docs. Do not fall back to the startup candidate index when
a track exists but cannot load.

- [ ] **Step 9: Run web and SEO regressions**

Run:

```bash
go test ./internal/web ./internal/web/e2e -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit dynamic public routing**

```bash
git add internal/web/docs.go internal/web/docs_test.go internal/web/public.go internal/web/public_test.go internal/web/server.go internal/web/seo_test.go internal/web/templates/layout.templ internal/web/templates/layout_templ.go internal/web/templates/public.templ internal/web/templates/public_templ.go
git commit -m "feat(web): serve docs from release tracks"
```

---

### Task 7: Authenticated Revision Previews

**Files:**
- Create: `internal/adapters/auth/basic.go`
- Create: `internal/adapters/auth/basic_test.go`
- Modify: `internal/web/docs.go`
- Modify: `internal/web/docs_test.go`
- Modify: `internal/web/server.go`

**Interfaces:**
- Produces: constant-time Basic authentication and authenticated,
  contract-scoped preview routing.

- [ ] **Step 1: Write failing authenticator tests**

Expose:

```go
var ErrUnauthenticated = errors.New("unauthenticated")

type BasicAuthenticator struct {
	Username   string
	Password   []byte
	ContractIDs []string
}

func LoadBasicAuthenticator(username, passwordFile string, contractIDs []string) (BasicAuthenticator, error)
func (a BasicAuthenticator) Authenticate(*http.Request) (core.Actor, error)
```

Test missing/malformed headers, wrong username, wrong password, empty secret,
cancelled request, and successful actor scoping. Require constant-time digest
comparison in implementation; tests should assert behavior, not timing.

- [ ] **Step 2: Run auth tests and verify RED**

Run:

```bash
go test ./internal/adapters/auth -count=1
```

Expected: package or compile failure.

- [ ] **Step 3: Implement the Basic adapter**

Read the password once at startup, trim one trailing CRLF/LF only, reject empty
credentials, copy secret bytes, compare SHA-256 digests with
`subtle.ConstantTimeCompare`, and return a `core.Actor` whose project/contract
scope is a sorted copy. Never include supplied credentials in errors.

- [ ] **Step 4: Write failing preview route tests**

Add:

```go
type PreviewAuthenticator func(*http.Request) (core.Actor, error)
```

Test:

- authentication executes before contract/revision lookup
- missing/bad credentials return 401 plus
  `WWW-Authenticate: Basic realm="Manja preview"`
- an authenticated actor lacking the contract scope receives 404
- `/preview/payments/rev-2` and revision-local search/download routes render
  the immutable preview index
- every preview response has `X-Robots-Tag:
  noindex, nofollow, noarchive`
- `/preview/.../sitemap.xml` is 404
- preview URLs never appear in the public track sitemap or public
  `/search.json`
- HTMX fragments remain inside the authenticated preview prefix

- [ ] **Step 5: Run preview tests and verify RED**

Run:

```bash
go test ./internal/web -run 'Preview|Authentication|NoIndex' -count=1
```

Expected: compile/assertion failure because preview routing/authentication is
not wired.

- [ ] **Step 6: Implement auth-first preview routing**

Parse exactly two escaped path segments after `/preview/`, reject empty,
slash-containing, dot, and dot-dot identities, authenticate before invoking
`PreviewDocsResolver`, check `Actor.ProjectIDs`, and delegate to the
base-path-aware renderer with `NoIndex=true`. Do not log resolver errors or
render manager diagnostics to anonymous callers.

- [ ] **Step 7: Run auth and web suites**

Run:

```bash
go test ./internal/adapters/auth ./internal/web ./internal/web/e2e -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit authenticated previews**

```bash
git add internal/adapters/auth/basic.go internal/adapters/auth/basic_test.go internal/web/docs.go internal/web/docs_test.go internal/web/server.go
git commit -m "feat(preview): authenticate immutable revision docs"
```

---

### Task 8: Recovery-First Server Wiring And End-To-End Verification

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `cmd/manja/main.go`
- Modify: `cmd/manja/main_test.go`
- Modify: `README.md`
- Create: `internal/web/e2e/release_tracks_test.go`

**Interfaces:**
- Consumes: server/repository config, migrator, track reconciler,
  `ReleaseService`, `RevisionDocsService`, Basic authenticator, and web
  resolver callbacks.
- Produces: self-hosted startup/restart behavior and documented flags:
  `--config`, `--release-config`, `--contract`, `--preview-user`, and
  `--preview-password-file`.

- [ ] **Step 1: Write failing CLI configuration tests**

Require:

```text
manja
  --config .manja.yaml
  --release-config /etc/manja/release.yaml
  --contract payments
  --preview-user local-admin
  --preview-password-file /run/secrets/manja-preview-password
```

`--preview-user` and `--preview-password-file` are an all-or-nothing pair.
Release config requires repository config because tracks select repository
policy profiles. Existing server flags and `manja check --config` behavior stay
unchanged.

- [ ] **Step 2: Run command tests and verify RED**

Run:

```bash
go test ./cmd/manja -run 'ServerConfig|PreviewAuth|RunServer|RunCheck' -count=1
```

Expected: assertions fail because new flags are not parsed.

- [ ] **Step 3: Write failing recovery-first application tests**

Use a persisted track/current revision plus a source that fails and require
`NewWithOptions` to return a handler that still serves last-known-good public
docs. Add cases for:

- no stored state plus source failure returns an error
- stored state plus corrupt/missing current blob returns an error
- successful first sync with no publications/tracks seeds one pinned `/`
  compatibility track
- legacy migration happens before route resolution
- configured track definitions preserve stored current state
- successful mapped sync invokes release coordination
- Git sources sync each unique non-empty configured track ref in sorted order;
  one ref failure does not prevent independent refs from retaining or advancing
  their own tracks
- file sources perform one singleton sync and match the stored file ref
- release coordination failure does not prevent existing public docs from
  serving, but is recorded in management sync state
- restart constructs indexes from stored revision metadata, not the latest
  source ref
- previews are not mounted unless authentication is configured

- [ ] **Step 4: Run application tests and verify RED**

Run:

```bash
go test ./internal/app -run 'Recovery|ReleaseTrack|Preview|LastKnownGood' -count=1
```

Expected: assertions fail because startup remains source-first and uses one
startup index.

- [ ] **Step 5: Refactor startup in recovery-first order**

Wire startup in this order:

1. open filesystem store
2. migrate legacy publications
3. load repository/server configs and reconcile definitions
4. validate every stored current track can load immutable docs
5. configure public/preview resolver callbacks
6. attempt one file sync or each sorted unique configured Git ref
7. on first successful legacy-compatible startup, seed pinned `/` only if no
   release/public state exists
8. apply mapped release reviews/transitions
9. construct management compatibility model from the latest healthy candidate
   while public handlers remain store-backed

If a source/release attempt fails after step 4, retain the handler and expose
only manager-safe summary state; never swap a public track to the failed
candidate.

- [ ] **Step 6: Add end-to-end concurrent-track and preview tests**

Persist two revisions and two tracks, start the real HTTP handler, and assert:

- `/payments/v1` renders revision v1
- `/payments/v2` renders revision v2
- each track returns its own search/download data
- authenticated `/preview/payments/{revision}` can render either revision
- anonymous preview is 401
- preview HTML/search routes are noindex and absent from public sitemap/search
- after restart with source failure, both public routes still return the same
  content
- a failed following candidate leaves its track on the prior revision
- a passing following candidate changes only its mapped track

- [ ] **Step 7: Run targeted end-to-end tests**

Run:

```bash
go test ./internal/app ./internal/web ./internal/web/e2e ./cmd/manja -count=1
```

Expected: PASS.

- [ ] **Step 8: Document self-hosted release operation**

Document:

- portable repository policy vs deployment release config
- legacy publication migration and pinned default behavior
- pinned vs following semantics
- Basic preview credential file, required TLS/reverse-proxy protection, and
  replaceable auth boundary
- public and preview route examples
- last-known-good behavior on source/parser/policy/store failure
- OIDC/Dex and connected review APIs as later roadmap slices
- no Try It or upstream proxy

- [ ] **Step 9: Run complete project gates**

Run:

```bash
npm ci
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
go test ./...
(cd site && GOWORK=off go test ./...)
git diff --check
```

Expected: all commands exit 0; API lint may report only the same three
pre-existing warnings.

- [ ] **Step 10: Run restart and deterministic-state smoke checks**

Using a temporary data directory:

1. start Manja with the committed repository/server fixtures and preview
   password file
2. request both public tracks and one authenticated preview
3. stop Manja
4. make the configured source unavailable
5. restart against the same data directory
6. require both public responses to remain byte-equivalent
7. verify persisted track/review JSON contains no preview password

Expected: public requests remain HTTP 200; anonymous preview is 401;
authenticated preview is HTTP 200 and noindex.

- [ ] **Step 11: Run the Goshtoso snag checkpoint**

Ask:

```text
Did Goshtoso components, helpers, docs, examples, generated templ behavior, or
release/dependency workflow slow this task down or force source-diving?
```

If yes, record exact component/symbol/source/workaround before review. If no,
state explicitly that only existing Manja template helpers were changed.

- [ ] **Step 12: Commit wiring and docs**

```bash
git add internal/app/app.go internal/app/app_test.go cmd/manja/main.go cmd/manja/main_test.go README.md internal/web/e2e/release_tracks_test.go
git commit -m "feat(release): wire tracks and authenticated previews"
```

---

## Final Review Gate

Before requesting review:

```bash
git status --short
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
git diff --check origin/main...HEAD
go test ./...
(cd site && GOWORK=off go test ./...)
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
git status --short
```

Then review requirements line by line:

- provider-neutral core and ports-first persistence
- immutable revision loading after restart
- independent `v1`/`v2` current state
- pinned and following transition rules
- review-first and generation-checked advancement
- last-known-good preservation at every failure stage
- authenticated no-index preview isolation
- legacy publication migration without path loss
- no connected API/provider/management-IA/Try-It scope
- no Goshtoso snag left unrecorded

Stop at this gate with the branch/worktree, commit range, fresh command output,
and review findings. Do not merge, push, or clean the worktree without explicit
direction.
