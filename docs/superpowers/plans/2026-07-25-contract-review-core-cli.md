# Contract Review Core And Offline CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build deterministic dual-baseline OpenAPI contract review and expose it through an offline-first `manja check` command without changing public docs or management behavior.

**Architecture:** Extend the existing provider-neutral core with normalized contract snapshots, stable findings, layered policy evaluation, and a versioned review report. Keep the current `SpecDiff` API as a compatibility wrapper for management, use the existing kin-openapi parser to build snapshots, and add file/Git-ref input plus YAML policy adapters around an application-level check service. The server, release tracks, connected APIs, and GitHub Action remain outside this plan.

**Tech Stack:** Go 1.26.1, `github.com/getkin/kin-openapi` v0.140.0, `gopkg.in/yaml.v3` v3.0.1, standard-library `flag`, `os/exec`, `crypto/sha256`, and `encoding/json`.

## Global Constraints

- Work only in a dedicated branch and worktree created from current `origin/main`.
- Keep core analysis and policy provider-neutral; no GitHub or GitLab SDK types.
- Keep current `internal/web` management and public-renderer behavior passing unchanged.
- The same snapshots, engine version, effective policy, and policy evaluation time must produce byte-stable canonical JSON and the same verdict.
- Repository policy is the only configured layer in this subproject, but core policy composition must already reject a server layer that weakens repository rules.
- Canonical report schema is `manja.review/v1`.
- CLI exit codes are exactly `0` for pass, `1` for policy failure, and `2` for input/configuration/parse/execution failure.
- Do not implement release tracks, authenticated previews, connected APIs, provider actions, or management information-architecture changes.
- Do not add a server-side Try It proxy or upstream request execution.
- Preserve the read-only, search-first public documentation surface.

## File Structure

Create or change these focused units:

- `internal/core/contractsnapshot.go`: normalized immutable snapshot types, sorting, and digests.
- `internal/core/specdiff.go`: stable rule/finding IDs and snapshot diffing while retaining `DiffSpecIndexes`.
- `internal/core/policy.go`: rule levels, policy layers, monotonic composition, exceptions, and verdict evaluation.
- `internal/core/review.go`: two named comparisons and the canonical `manja.review/v1` report.
- `internal/core/ports.go`: snapshot-builder and review-input loader ports used by application orchestration.
- `internal/adapters/openapi/snapshot.go`: reuse the existing parser and convert a `SpecIndex` into a normalized snapshot.
- `internal/adapters/openapi/testdata/review/*.yaml`: small target, candidate, and release fixtures.
- `internal/adapters/config/manja.go`: strict `.manja.yaml` parsing and conversion to a repository policy layer.
- `internal/adapters/reviewinput/loader.go`: read a spec from a file or `git show` without shell interpolation.
- `internal/adapters/reviewformat/format.go`: JSON, text, and Markdown report writers.
- `internal/adapters/reviewformat/testdata/*.golden`: stable human-output fixtures.
- `internal/app/check.go`: provider-neutral check orchestration.
- `cmd/manja/check.go`: `manja check` flags, validation, and adapter wiring.
- `cmd/manja/main.go`: subcommand dispatch while preserving the existing server command.
- `README.md`: local and CI-neutral review usage.
- `go.mod`, `go.sum`: make `gopkg.in/yaml.v3` a direct dependency.

---

### Task 1: Normalized Snapshots And Stable Diff Findings

**Files:**
- Create: `internal/core/contractsnapshot.go`
- Create: `internal/core/contractsnapshot_test.go`
- Modify: `internal/core/specdiff.go`
- Modify: `internal/core/specdiff_test.go`

**Interfaces:**
- Produces: `ContractSnapshot`, `NewContractSnapshot`, `DiffContractSnapshots`, stable `SpecChange.ID`, and stable `SpecChange.RuleID`.
- Exposes: checked `DiffSpecIndexes(SpecIndex, SpecIndex) (SpecDiff, error)` for management and external callers so ambiguous compatibility surfaces fail closed.

- [ ] **Step 1: Write failing snapshot normalization and digest tests**

Add tests that construct semantically identical `SpecIndex` values in different
orders and require identical snapshot digests:

```go
func TestNewContractSnapshotNormalizesOrderAndDigestsContent(t *testing.T) {
	left := SpecIndex{
		RevisionID: "target",
		Operations: []Operation{
			{Method: "POST", Path: "/payments", Responses: []OperationResponse{{Status: "201"}, {Status: "400"}}},
			{Method: "GET", Path: "/payments", Parameters: []OperationParameter{{Name: "limit", In: "query"}}},
		},
		Schemas: []Schema{{Name: "Payment"}, {Name: "Error"}},
	}
	right := SpecIndex{
		RevisionID: "target",
		Operations: []Operation{
			{Method: "GET", Path: "/payments", Parameters: []OperationParameter{{Name: "limit", In: "query"}}},
			{Method: "POST", Path: "/payments", Responses: []OperationResponse{{Status: "400"}, {Status: "201"}}},
		},
		Schemas: []Schema{{Name: "Error"}, {Name: "Payment"}},
	}

	a := NewContractSnapshot("payments", "target", []byte("raw-a"), left)
	b := NewContractSnapshot("payments", "target", []byte("raw-a"), right)

	if a.ContractDigest == "" || a.ContractDigest != b.ContractDigest {
		t.Fatalf("contract digests differ: %q != %q", a.ContractDigest, b.ContractDigest)
	}
	if a.SpecDigest == "" || a.SpecDigest != b.SpecDigest {
		t.Fatalf("spec digests differ: %q != %q", a.SpecDigest, b.SpecDigest)
	}
}
```

- [ ] **Step 2: Run the snapshot test and verify it fails**

Run:

```bash
go test ./internal/core -run TestNewContractSnapshotNormalizesOrderAndDigestsContent -count=1
```

Expected: compile failure because `NewContractSnapshot` and snapshot fields do
not exist.

- [ ] **Step 3: Add normalized snapshot types and deterministic digests**

Implement these public core shapes:

```go
type ContractSnapshot struct {
	ContractID    string              `json:"contractId"`
	RevisionID    string              `json:"revisionId"`
	SpecDigest    string              `json:"specDigest"`
	ContractDigest string             `json:"contractDigest"`
	Operations    []ContractOperation `json:"operations"`
	Schemas       []string            `json:"schemas"`
}

type ContractOperation struct {
	Method              string              `json:"method"`
	Path                string              `json:"path"`
	Parameters          []ContractParameter `json:"parameters,omitempty"`
	RequestBodyRequired bool                `json:"requestBodyRequired,omitempty"`
	ResponseStatuses    []string            `json:"responseStatuses,omitempty"`
}

type ContractParameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required,omitempty"`
}

func NewContractSnapshot(contractID, revisionID string, raw []byte, idx SpecIndex) ContractSnapshot
```

Normalize method casing and whitespace, sort operations by method/path,
parameters by `in/name`, response statuses lexically, and schema names
lexically. Compute `SpecDigest` as lowercase SHA-256 hex of `raw`. Compute
`ContractDigest` as SHA-256 hex of canonical JSON containing the normalized
operations and schemas but excluding identity and digest fields.

- [ ] **Step 4: Run the core snapshot tests**

Run:

```bash
go test ./internal/core -run 'TestNewContractSnapshot' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing stable-finding tests**

Extend `specdiff_test.go` to assert exact rule IDs and stable finding IDs:

```go
func TestDiffContractSnapshotsUsesStableRuleAndFindingIDs(t *testing.T) {
	baseline := NewContractSnapshot("payments", "base", []byte("base"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	candidate := NewContractSnapshot("payments", "head", []byte("head"), SpecIndex{})

	first, err := DiffContractSnapshots(baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DiffContractSnapshots(baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}

	change := first.BreakingChanges[0]
	if change.RuleID != RuleOperationRemoved {
		t.Fatalf("rule = %q", change.RuleID)
	}
	if change.ID == "" || change.ID != second.BreakingChanges[0].ID {
		t.Fatalf("finding ids are not stable: %#v %#v", first, second)
	}
}
```

- [ ] **Step 6: Run the stable-finding test and verify it fails**

Run:

```bash
go test ./internal/core -run TestDiffContractSnapshotsUsesStableRuleAndFindingIDs -count=1
```

Expected: compile failure because the snapshot diff, rule constants, and fields
do not exist.

- [ ] **Step 7: Convert the current diff into a stable snapshot diff**

Add exact rule constants:

```go
const (
	RuleOperationRemoved             = "operation.removed"
	RuleOperationAdded               = "operation.added"
	RuleRequiredParameterAdded       = "request.parameter.required_added"
	RuleParameterBecameRequired      = "request.parameter.became_required"
	RuleRequestBodyBecameRequired    = "request.body.became_required"
	RuleResponseStatusRemoved        = "response.status.removed"
	RuleResponseStatusAdded          = "response.status.added"
	RuleSchemaRemoved                = "schema.removed"
	RuleSchemaAdded                  = "schema.added"
)
```

Extend `SpecChange` without removing current fields:

```go
type SpecChange struct {
	ID          string `json:"id"`
	RuleID      string `json:"ruleId"`
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
}
```

Implement:

```go
func DiffContractSnapshots(baseline, candidate ContractSnapshot) (SpecDiff, error)
func stableFindingID(ruleID, subject string) string
```

Use SHA-256 of `ruleID + "\x00" + normalized subject` for `ID`. Sort breaking
and additive results by `RuleID`, then `Subject`, then `ID`.

Retain:

```go
func DiffSpecIndexes(baseline, candidate SpecIndex) (SpecDiff, error)
```

Both public diff entry points validate their inputs before constructing maps, so
canonical duplicates produce deterministic errors rather than order-dependent
findings. Management callers surface that error as unavailable comparison
evidence.

- [ ] **Step 8: Run all core and management diff tests**

Run:

```bash
go test ./internal/core ./internal/web -run 'SpecDiff|Management' -count=1
```

Expected: PASS, including existing management behavior.

- [ ] **Step 9: Commit snapshot and diff work**

```bash
git add internal/core/contractsnapshot.go internal/core/contractsnapshot_test.go internal/core/specdiff.go internal/core/specdiff_test.go
git commit -m "feat(core): add deterministic contract snapshots"
```

---

### Task 2: Layered Policy And Audited Exceptions

**Files:**
- Create: `internal/core/policy.go`
- Create: `internal/core/policy_test.go`

**Interfaces:**
- Consumes: stable `SpecChange.RuleID`, `SpecChange.ID`, and additive/breaking classes from Task 1.
- Produces: `PolicyLayer`, `PolicyException`, `EffectivePolicy`, `MergePolicy`, and `EvaluateFindings`.

- [ ] **Step 1: Write failing policy composition tests**

Cover defaults, repository overrides, stricter server policy, weakening
rejection, same-layer exceptions, cross-layer isolation, and expiry:

```go
func TestMergePolicyRejectsServerWeakening(t *testing.T) {
	repo := PolicyLayer{
		Name:   "stable",
		Source: PolicySourceRepository,
		Rules:  map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
	}
	server := PolicyLayer{
		Name:   "public-v1",
		Source: PolicySourceServer,
		Rules:  map[string]RuleLevel{RuleOperationRemoved: RuleLevelWarn},
	}

	_, err := MergePolicy(repo, server)
	if err == nil || !strings.Contains(err.Error(), "cannot weaken") {
		t.Fatalf("error = %v", err)
	}
}

func TestEvaluateFindingsAppliesOnlySameLayerUnexpiredException(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	finding := SpecChange{ID: "finding-1", RuleID: RuleOperationRemoved, Severity: SpecChangeBreaking}
	repo := PolicyLayer{
		Name: "stable", Source: PolicySourceRepository,
		Rules: map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
		Exceptions: []PolicyException{{
			FindingID: "finding-1", Reason: "v2 migration", Author: "api-team",
			ExpiresAt: now.Add(time.Hour), Source: PolicySourceRepository,
		}},
	}

	effective, err := MergePolicy(repo)
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFindings(effective, []SpecChange{finding}, now)
	if result.Verdict != VerdictPass || len(result.AppliedExceptions) != 1 {
		t.Fatalf("result = %#v", result)
	}
}
```

- [ ] **Step 2: Run policy tests and verify they fail**

Run:

```bash
go test ./internal/core -run 'TestMergePolicy|TestEvaluateFindings' -count=1
```

Expected: compile failure because policy types do not exist.

- [ ] **Step 3: Implement policy levels, layers, and monotonic merge**

Use exact enums:

```go
type RuleLevel string

const (
	RuleLevelAllow RuleLevel = "allow"
	RuleLevelWarn  RuleLevel = "warn"
	RuleLevelFail  RuleLevel = "fail"
)

const (
	PolicySourceRepository = "repository"
	PolicySourceServer     = "server"
)

type PolicyException struct {
	FindingID string    `json:"findingId,omitempty"`
	RuleID    string    `json:"ruleId,omitempty"`
	Reason    string    `json:"reason"`
	Author    string    `json:"author"`
	ExpiresAt time.Time `json:"expiresAt"`
	Source    string    `json:"source"`
}

type PolicyLayer struct {
	Name                   string               `json:"name"`
	Source                 string               `json:"source"`
	RequireReleaseBaseline bool                 `json:"requireReleaseBaseline"`
	Rules                  map[string]RuleLevel `json:"rules"`
	Exceptions             []PolicyException    `json:"exceptions,omitempty"`
}

type EffectivePolicy struct {
	Layers                 []PolicyLayer `json:"layers"`
	RequireReleaseBaseline bool          `json:"requireReleaseBaseline"`
}

func MergePolicy(layers ...PolicyLayer) (EffectivePolicy, error)
```

`MergePolicy` must require exactly one repository layer first, allow zero or
more server layers afterward, validate all levels and exception fields, OR
`RequireReleaseBaseline`, and reject any server rule lower than the repository
level for the same rule. Initialize the repository layer with every rule
constant from Task 1 so the default levels participate in weakening checks even
when a YAML profile omits the rule.

Repository defaults are:

- every breaking finding: `fail`
- every additive finding: `allow`

An explicit repository rule replaces its default. A server rule may only keep
or increase the repository level.

- [ ] **Step 4: Implement finding evaluation and exception isolation**

Implement:

```go
type FindingDecision struct {
	Finding SpecChange `json:"finding"`
	Level   RuleLevel  `json:"level"`
	Source  string     `json:"source"`
	Excepted bool      `json:"excepted"`
}

type PolicyResult struct {
	Verdict           string            `json:"verdict"`
	Decisions         []FindingDecision `json:"decisions"`
	AppliedExceptions []PolicyException `json:"appliedExceptions,omitempty"`
}

const (
	VerdictPass = "pass"
	VerdictFail = "fail"
)

func EvaluateFindings(policy EffectivePolicy, findings []SpecChange, evaluatedAt time.Time) PolicyResult
```

An exception applies when exactly one of `FindingID` or `RuleID` matches, its
source equals the contribution it waives, required text fields are nonblank, and
`evaluatedAt.Before(ExpiresAt)` is true. Evaluate repository and server
contributions independently; an excepted server contribution cannot waive an
unexcepted repository failure and vice versa.

- [ ] **Step 5: Run policy tests**

Run:

```bash
go test ./internal/core -run 'Policy|EvaluateFindings' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit policy work**

```bash
git add internal/core/policy.go internal/core/policy_test.go
git commit -m "feat(core): add layered compatibility policy"
```

---

### Task 3: Dual-Baseline Canonical Review Report

**Files:**
- Create: `internal/core/review.go`
- Create: `internal/core/review_test.go`

**Interfaces:**
- Consumes: `ContractSnapshot`, `DiffContractSnapshots`, `EffectivePolicy`, and `EvaluateFindings`.
- Produces: `ReviewRequest`, `ReviewReport`, `EvaluateReview`, `CanonicalReviewJSON`, and `ReviewSchemaVersion`.

- [ ] **Step 1: Write failing dual-baseline report tests**

```go
func TestEvaluateReviewSeparatesPullRequestAndReleaseImpact(t *testing.T) {
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	target := NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	release := NewContractSnapshot("payments", "release", []byte("release"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}, {Method: "POST", Path: "/payments"}},
	})
	candidate := NewContractSnapshot("payments", "head", []byte("head"), SpecIndex{})
	policy, err := MergePolicy(PolicyLayer{Name: "stable", Source: PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}

	report, err := EvaluateReview(ReviewRequest{
		ContractID: "payments", Target: target, Candidate: candidate,
		Release: &release, Policy: policy, EvaluatedAt: at, EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReviewSchemaVersion || len(report.Comparisons) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Comparisons[0].Kind != ComparisonPullRequest ||
		report.Comparisons[1].Kind != ComparisonReleaseImpact {
		t.Fatalf("comparison order = %#v", report.Comparisons)
	}
}
```

Add a second test requiring an error when policy requires a release baseline and
`ReviewRequest.Release` is nil.

- [ ] **Step 2: Run review tests and verify they fail**

Run:

```bash
go test ./internal/core -run TestEvaluateReview -count=1
```

Expected: compile failure because review types do not exist.

- [ ] **Step 3: Implement the canonical report model**

Use these shapes:

```go
const ReviewSchemaVersion = "manja.review/v1"

const (
	ComparisonPullRequest  = "pull_request_delta"
	ComparisonReleaseImpact = "release_impact"
)

type SnapshotRef struct {
	RevisionID     string `json:"revisionId"`
	SpecDigest     string `json:"specDigest"`
	ContractDigest string `json:"contractDigest"`
}

type ComparisonReport struct {
	Kind      string       `json:"kind"`
	Baseline  SnapshotRef  `json:"baseline"`
	Candidate SnapshotRef  `json:"candidate"`
	Findings  []SpecChange `json:"findings"`
	Policy    PolicyResult `json:"policy"`
}

type ReviewReport struct {
	SchemaVersion string             `json:"schemaVersion"`
	EngineVersion string             `json:"engineVersion"`
	ContractID    string             `json:"contractId"`
	EvaluatedAt   time.Time          `json:"evaluatedAt"`
	PolicyDigest  string             `json:"policyDigest"`
	Verdict       string             `json:"verdict"`
	Comparisons   []ComparisonReport `json:"comparisons"`
}

type ReviewRequest struct {
	ContractID    string
	Target        ContractSnapshot
	Candidate     ContractSnapshot
	Release       *ContractSnapshot
	Policy        EffectivePolicy
	EvaluatedAt   time.Time
	EngineVersion string
}

func EvaluateReview(ReviewRequest) (ReviewReport, error)
```

`EvaluateReview` always emits pull-request delta first. It emits release impact
second when supplied. Overall verdict is `fail` when any comparison fails.
Reject mismatched contract IDs, zero evaluation time, blank engine version, and
missing required release baseline.

- [ ] **Step 4: Implement canonical JSON and digest stability**

Implement:

```go
func CanonicalReviewJSON(report ReviewReport) ([]byte, error)
```

Use `json.Marshal` without indentation or HTML escaping changes. Ensure all
slices are deterministically sorted before creating the report; do not place
maps in `ReviewReport`. Compute `PolicyDigest` from a separate canonical
slice-based policy projection sorted by layer and rule ID.

Add a test that calls `CanonicalReviewJSON` twice and compares bytes exactly.

- [ ] **Step 5: Run review and core tests**

Run:

```bash
go test ./internal/core -run 'Review|Policy|ContractSnapshot|SpecDiff' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit review-report work**

```bash
git add internal/core/review.go internal/core/review_test.go
git commit -m "feat(core): add dual-baseline review reports"
```

---

### Task 4: OpenAPI Snapshot Builder And Review Fixtures

**Files:**
- Create: `internal/adapters/openapi/snapshot.go`
- Create: `internal/adapters/openapi/snapshot_test.go`
- Create: `internal/adapters/openapi/testdata/review/release.yaml`
- Create: `internal/adapters/openapi/testdata/review/target.yaml`
- Create: `internal/adapters/openapi/testdata/review/candidate.yaml`
- Modify: `internal/core/ports.go`

**Interfaces:**
- Consumes: existing `openapi.Parser.Parse` and `core.NewContractSnapshot`.
- Produces: `core.ContractSnapshotBuilder` and `openapi.SnapshotBuilder.Build`.

- [ ] **Step 1: Add small review fixtures**

Use OpenAPI 3.1 fixtures with these exact contract differences:

- `release.yaml`: `GET /payments`, `POST /payments`, response `200`, schemas
  `Payment` and `Error`.
- `target.yaml`: `GET /payments`, response `200`, schemas `Payment` and `Error`.
- `candidate.yaml`: `GET /payments` with new required query parameter
  `account_id`, response `202` instead of `200`, schema `Payment`.

Keep each fixture under 60 lines and include valid `info.title` and
`info.version`.

- [ ] **Step 2: Write a failing snapshot-builder test**

```go
func TestSnapshotBuilderBuildsNormalizedOpenAPIContract(t *testing.T) {
	data, err := os.ReadFile("testdata/review/candidate.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := (SnapshotBuilder{}).Build(context.Background(), "payments",
		core.SpecFile{Path: "candidate.yaml", Format: "yaml", Bytes: data},
		core.Revision{ID: "candidate"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContractID != "payments" || got.RevisionID != "candidate" {
		t.Fatalf("snapshot = %#v", got)
	}
	if got.SpecDigest == "" || got.ContractDigest == "" || len(got.Operations) != 1 {
		t.Fatalf("snapshot = %#v", got)
	}
}
```

- [ ] **Step 3: Run the builder test and verify it fails**

Run:

```bash
go test ./internal/adapters/openapi -run TestSnapshotBuilderBuildsNormalizedOpenAPIContract -count=1
```

Expected: compile failure because `SnapshotBuilder` does not exist.

- [ ] **Step 4: Add the builder port and adapter**

Add to `internal/core/ports.go`:

```go
type ContractSnapshotBuilder interface {
	Build(context.Context, string, SpecFile, Revision) (ContractSnapshot, error)
}
```

Implement:

```go
type SnapshotBuilder struct {
	Parser Parser
}

func (b SnapshotBuilder) Build(
	ctx context.Context,
	contractID string,
	file core.SpecFile,
	rev core.Revision,
) (core.ContractSnapshot, error)
```

Default `Parser` to `openapi.Parser{}` when unset, parse once into `SpecIndex`,
and pass `file.Bytes` plus the index to `core.NewContractSnapshot`. Wrap errors
with `build contract snapshot`.

- [ ] **Step 5: Run OpenAPI and core tests**

Run:

```bash
go test ./internal/adapters/openapi ./internal/core -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit snapshot adapter and fixtures**

```bash
git add internal/core/ports.go internal/adapters/openapi/snapshot.go internal/adapters/openapi/snapshot_test.go internal/adapters/openapi/testdata/review
git commit -m "feat(openapi): build contract review snapshots"
```

---

### Task 5: Strict Repository Configuration

**Files:**
- Create: `internal/adapters/config/manja.go`
- Create: `internal/adapters/config/manja_test.go`
- Create: `internal/adapters/config/testdata/manja.yaml`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: `config.Load`, `config.File.Contract`, and `config.Contract.PolicyLayer`.
- Consumes: core policy types from Task 2.

- [ ] **Step 1: Write failing strict-config tests**

Use this valid fixture inline:

```go
const validConfig = `
version: 1
contracts:
  payments:
    spec: docs/openapi.yaml
    defaultPolicy: stable
    policies:
      stable:
        requireReleaseBaseline: true
        rules:
          operation.removed: fail
          schema.removed: warn
        exceptions:
          - finding: 4f6c9c3f
            reason: Existing v1 client migration
            author: api-platform
            expires: 2026-08-31T23:59:59Z
`
```

Write the same configuration to
`internal/adapters/config/testdata/manja.yaml`, using
`internal/adapters/openapi/testdata/review/candidate.yaml` as the `spec` value,
so Task 8 has a committed smoke-test configuration.

Tests must verify:

- the contract and `stable` profile load
- expiry parses as UTC RFC3339
- unknown fields fail because `KnownFields(true)` is active
- missing version, contract ID, spec, profile, reason, author, or expiry fails
- an exception with both `finding` and `rule` fails
- rule levels outside `allow`, `warn`, and `fail` fail

- [ ] **Step 2: Run config tests and verify they fail**

Run:

```bash
go test ./internal/adapters/config -count=1
```

Expected: package or compile failure because the config adapter does not exist.

- [ ] **Step 3: Implement strict YAML types and validation**

Expose:

```go
type File struct {
	Version   int                       `yaml:"version"`
	Contracts map[string]ContractConfig `yaml:"contracts"`
}

type ContractConfig struct {
	Spec          string                  `yaml:"spec"`
	DefaultPolicy string                  `yaml:"defaultPolicy"`
	Policies      map[string]PolicyConfig `yaml:"policies"`
}

type PolicyConfig struct {
	RequireReleaseBaseline bool              `yaml:"requireReleaseBaseline"`
	Rules                  map[string]string `yaml:"rules"`
	Exceptions             []ExceptionConfig `yaml:"exceptions"`
}

type ExceptionConfig struct {
	Finding string `yaml:"finding"`
	Rule    string `yaml:"rule"`
	Reason  string `yaml:"reason"`
	Author  string `yaml:"author"`
	Expires string `yaml:"expires"`
}

func Load(path string) (File, error)
func (f File) Contract(id string) (ContractConfig, error)
func (c ContractConfig) PolicyLayer(name string) (core.PolicyLayer, error)
```

Use `yaml.NewDecoder`, call `KnownFields(true)`, require EOF after the first
document, parse expiry with `time.RFC3339`, and set exception source to
`core.PolicySourceRepository`. `PolicyLayer("")` selects `DefaultPolicy`;
an explicit unknown profile and a missing default profile are validation
errors.

- [ ] **Step 4: Make YAML a direct dependency and run tests**

Run:

```bash
go mod tidy
go test ./internal/adapters/config ./internal/core -count=1
```

Expected: PASS, with `gopkg.in/yaml.v3 v3.0.1` in the direct `require` block.

- [ ] **Step 5: Commit repository config**

```bash
git add internal/adapters/config/manja.go internal/adapters/config/manja_test.go internal/adapters/config/testdata/manja.yaml go.mod go.sum
git commit -m "feat(config): load contract review policy"
```

---

### Task 6: File And Git-Ref Review Inputs

**Files:**
- Create: `internal/adapters/reviewinput/loader.go`
- Create: `internal/adapters/reviewinput/loader_test.go`
- Modify: `internal/core/ports.go`

**Interfaces:**
- Produces: `core.ReviewInputLocator`, `core.ReviewInputLoader`, and `reviewinput.Loader.Load`.
- Consumes: `core.SpecFile` and `core.Revision`.

- [ ] **Step 1: Write failing file and Git-ref loader tests**

Create a temporary Git repository in the test with `git init`, configure a local
test identity, commit `docs/openapi.yaml`, and record `HEAD`.

Test both forms:

```go
file, rev, err := (Loader{}).Load(ctx, "docs/openapi.yaml",
	core.ReviewInputLocator{File: "/tmp/candidate.yaml"})

file, rev, err = (Loader{RepoDir: repo}).Load(ctx, "docs/openapi.yaml",
	core.ReviewInputLocator{GitRef: "HEAD"})
```

Assert file bytes, path, format, revision ID, ref, and commit SHA. Add validation
tests for:

- both `File` and `GitRef` set
- neither set
- absolute or `..` Git spec paths
- a Git ref beginning with `-`
- unknown Git ref
- missing file

- [ ] **Step 2: Run loader tests and verify they fail**

Run:

```bash
go test ./internal/adapters/reviewinput -count=1
```

Expected: package or compile failure.

- [ ] **Step 3: Add the input port**

Add:

```go
type ReviewInputLocator struct {
	File   string
	GitRef string
}

type ReviewInputLoader interface {
	Load(context.Context, string, ReviewInputLocator) (SpecFile, Revision, error)
}
```

- [ ] **Step 4: Implement safe file and Git loading**

Implement:

```go
type Loader struct {
	RepoDir string
}

func (l Loader) Load(
	ctx context.Context,
	specPath string,
	locator core.ReviewInputLocator,
) (core.SpecFile, core.Revision, error)
```

For files, use `os.ReadFile`. For Git refs, reject absolute and parent-traversal
spec paths, then call:

```go
exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--verify", ref+"^{commit}")
exec.CommandContext(ctx, "git", "-C", repoDir, "show", ref+":"+filepath.ToSlash(specPath))
```

Never invoke a shell. Use the resolved commit SHA as revision ID and preserve the
user-provided ref in `Revision.Ref`. Reject Git refs beginning with `-`.

For a file locator, set `SpecFile.Path` to the file path, infer `yaml` or `json`
from its extension, set `Revision.Ref` to `file`, and set `Revision.ID` to
`file-` plus the lowercase SHA-256 hex digest of the bytes. For a Git locator,
set `SpecFile.Path` to the configured repo-relative spec path, infer format from
that path, and set both `Revision.ID` and `Revision.CommitSHA` to the resolved
commit SHA.

- [ ] **Step 5: Run loader tests**

Run:

```bash
go test ./internal/adapters/reviewinput -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit input loading**

```bash
git add internal/core/ports.go internal/adapters/reviewinput/loader.go internal/adapters/reviewinput/loader_test.go
git commit -m "feat(review): load file and git review inputs"
```

---

### Task 7: Deterministic JSON, Text, And Markdown Output

**Files:**
- Create: `internal/adapters/reviewformat/format.go`
- Create: `internal/adapters/reviewformat/format_test.go`
- Create: `internal/adapters/reviewformat/testdata/pass.txt.golden`
- Create: `internal/adapters/reviewformat/testdata/fail.txt.golden`
- Create: `internal/adapters/reviewformat/testdata/fail.md.golden`

**Interfaces:**
- Consumes: `core.ReviewReport` and `core.CanonicalReviewJSON`.
- Produces: `reviewformat.Write(io.Writer, string, core.ReviewReport) error`.

- [ ] **Step 1: Write failing formatter tests**

```go
func TestWriteJSONUsesCanonicalReviewJSON(t *testing.T) {
	report := core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID: "payments",
		EvaluatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest: "policy-digest",
		Verdict: core.VerdictPass,
		Comparisons: []core.ComparisonReport{{
			Kind: core.ComparisonPullRequest,
			Baseline: core.SnapshotRef{RevisionID: "target"},
			Candidate: core.SnapshotRef{RevisionID: "head"},
			Policy: core.PolicyResult{Verdict: core.VerdictPass},
		}},
	}
	var got bytes.Buffer
	if err := Write(&got, FormatJSON, report); err != nil {
		t.Fatal(err)
	}
	want, err := core.CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("json differs:\n%s\nwant:\n%s", got.Bytes(), want)
	}
}
```

Add table tests comparing text and Markdown output with golden files and a test
that rejects an unknown format.

- [ ] **Step 2: Run formatter tests and verify they fail**

Run:

```bash
go test ./internal/adapters/reviewformat -count=1
```

Expected: package or compile failure.

- [ ] **Step 3: Implement deterministic writers**

Use:

```go
const (
	FormatJSON     = "json"
	FormatText     = "text"
	FormatMarkdown = "markdown"
)

func Write(w io.Writer, format string, report core.ReviewReport) error
```

Text and Markdown must show, in this order:

1. contract, engine, evaluation time, and overall verdict
2. pull-request delta
3. release impact when present
4. each finding's level, rule ID, subject, and description
5. applied exception reason, author, and expiry
6. effective policy digest

Do not use maps or terminal color. End every format with one newline.

- [ ] **Step 4: Run formatter tests**

Run:

```bash
go test ./internal/adapters/reviewformat -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit report formatting**

```bash
git add internal/adapters/reviewformat
git commit -m "feat(review): format deterministic review reports"
```

---

### Task 8: Application Check Service And `manja check`

**Files:**
- Create: `internal/app/check.go`
- Create: `internal/app/check_test.go`
- Create: `cmd/manja/check.go`
- Create: `cmd/manja/check_test.go`
- Modify: `cmd/manja/main.go`
- Modify: `cmd/manja/main_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: config, input, snapshot, review, policy, and formatting interfaces from Tasks 1-7.
- Produces: `app.CheckService.Run`, CLI `run`, and the public `manja check` command.

- [ ] **Step 1: Write failing application-service tests**

Define:

```go
type CheckRequest struct {
	ContractID       string
	SpecPath         string
	Target           core.ReviewInputLocator
	Candidate        core.ReviewInputLocator
	Release          *core.ReviewInputLocator
	Policy           core.EffectivePolicy
	EvaluatedAt      time.Time
	EngineVersion    string
}

type CheckService struct {
	Inputs   core.ReviewInputLoader
	Snapshots core.ContractSnapshotBuilder
}

func (s CheckService) Run(context.Context, CheckRequest) (core.ReviewReport, error)
```

Use fakes to verify the service loads target, candidate, then optional release;
builds all snapshots with the same contract ID; and passes exact evaluation time
and engine version into `core.EvaluateReview`.

- [ ] **Step 2: Run service tests and verify they fail**

Run:

```bash
go test ./internal/app -run TestCheckService -count=1
```

Expected: compile failure because `CheckService` does not exist.

- [ ] **Step 3: Implement the application service**

Validate all dependencies and required request fields. Wrap errors with the input
role (`load target`, `load candidate`, `load release`) and return the core report
unchanged.

- [ ] **Step 4: Write failing command parsing and exit-code tests**

Refactor command execution behind:

```go
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int
```

Add tests for:

- no subcommand still parses and starts the existing server path through a fake
  `serve` function
- `check` requires `--config`, `--contract`, target locator, and candidate
  locator
- file and ref flags are mutually exclusive per role
- `--format` accepts only `json`, `text`, and `markdown`
- `--evaluated-at` requires RFC3339 when supplied
- passing review returns `0`
- policy failure returns `1`
- config/input/parse error returns `2` and writes a concise stderr message

The flag surface is:

```text
manja check
  --config .manja.yaml
  --contract payments
  --policy stable
  --repo .
  --target-file PATH | --target-ref REF
  --candidate-file PATH | --candidate-ref REF
  [--release-file PATH | --release-ref REF]
  [--format text|json|markdown]
  [--evaluated-at RFC3339]
```

- [ ] **Step 5: Run command tests and verify they fail**

Run:

```bash
go test ./cmd/manja -run 'TestRunCheck|TestCheckArgs|TestRunServer' -count=1
```

Expected: compile or assertion failure because dispatch does not exist.

- [ ] **Step 6: Implement `check` dispatch and preserve server behavior**

Move current server startup into:

```go
var serve = func(ctx context.Context, cfg cliConfig) error
func runServer(ctx context.Context, args []string) error
```

`run` dispatches to `runCheck` only when `args[0] == "check"`; otherwise it
passes the complete argument list to `runServer`. Keep all current server flags
and defaults unchanged.

Wire `runCheck` to:

1. load strict config
2. select the contract and use `--policy` when supplied or the contract's
   `defaultPolicy` otherwise
3. merge repository policy
4. build locators
5. default evaluation time to `time.Now().UTC()` only when not explicitly set
6. call `app.CheckService`
7. write the selected format
8. return `1` only for a completed failing report

Use:

```go
var version = "dev"
```

as the engine version for local builds. Leave release-time `-ldflags` wiring to
the existing release workflow; do not add a second version source.

- [ ] **Step 7: Run targeted CLI tests**

Run:

```bash
go test ./internal/app ./cmd/manja ./internal/adapters/config ./internal/adapters/reviewinput ./internal/adapters/reviewformat -count=1
```

Expected: PASS.

- [ ] **Step 8: Document local and CI-neutral usage**

Add a `Contract review` README section with:

```yaml
version: 1
contracts:
  payments:
    spec: docs/openapi.yaml
    defaultPolicy: stable
    policies:
      stable:
        requireReleaseBaseline: true
        rules:
          operation.removed: fail
          schema.removed: fail
```

Show one file-based command and one Git-ref command. Explain exit codes and state
that GitHub Actions and connected Manja review are later subprojects.

- [ ] **Step 9: Run the end-to-end smoke test**

Run:

```bash
go run ./cmd/manja check \
  --config internal/adapters/config/testdata/manja.yaml \
  --contract payments \
  --policy stable \
  --target-file internal/adapters/openapi/testdata/review/target.yaml \
  --candidate-file internal/adapters/openapi/testdata/review/candidate.yaml \
  --release-file internal/adapters/openapi/testdata/review/release.yaml \
  --format text \
  --evaluated-at 2026-07-25T12:00:00Z
```

Expected: a report with separate `pull_request_delta` and `release_impact`
sections and exit code `1`.

- [ ] **Step 10: Run all quality gates**

Run:

```bash
go test ./...
git diff --check
```

Expected: all tests PASS and no whitespace errors.

No API YAML or `.templ` source changes are planned. If implementation changes
either unexpectedly, stop and create a separate reviewed scope rather than
silently expanding this plan.

- [ ] **Step 11: Run the Goshtoso snag checkpoint**

Ask:

```text
Did Goshtoso components, helpers, docs, examples, generated templ behavior, or
release/dependency workflow slow this task down or force source-diving?
```

Record any yes answer before merge. This backend/CLI plan should not require a
Goshtoso change.

- [ ] **Step 12: Commit the complete command**

```bash
git add internal/app/check.go internal/app/check_test.go cmd/manja/check.go cmd/manja/check_test.go cmd/manja/main.go cmd/manja/main_test.go README.md internal/adapters/config/testdata/manja.yaml
git commit -m "feat(cli): add offline contract review command"
```

---

## Final Verification

After all task commits:

```bash
go test ./...
go run ./cmd/manja check \
  --config internal/adapters/config/testdata/manja.yaml \
  --contract payments \
  --policy stable \
  --target-file internal/adapters/openapi/testdata/review/target.yaml \
  --candidate-file internal/adapters/openapi/testdata/review/candidate.yaml \
  --release-file internal/adapters/openapi/testdata/review/release.yaml \
  --format json \
  --evaluated-at 2026-07-25T12:00:00Z > /tmp/manja-review.json
git diff --check
git status --short
```

Expected:

- all Go tests pass
- the CLI exits `1` because the fixture violates stable policy
- `/tmp/manja-review.json` is valid `manja.review/v1` JSON with two comparisons
- only intended implementation and generated dependency files are modified
- no release-track, connected API, provider-action, or management-UI work is
  present
