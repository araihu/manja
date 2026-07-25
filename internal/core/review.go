package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const ReviewSchemaVersion = "manja.review/v1"

const (
	ComparisonPullRequest   = "pull_request_delta"
	ComparisonReleaseImpact = "release_impact"
)

// SnapshotRef identifies an immutable contract snapshot without embedding its
// indexed contract surface in a review report.
type SnapshotRef struct {
	RevisionID     string `json:"revisionId"`
	SpecDigest     string `json:"specDigest"`
	ContractDigest string `json:"contractDigest"`
}

// ComparisonReport contains one policy-evaluated change set against a named
// baseline.
type ComparisonReport struct {
	Kind      string       `json:"kind"`
	Baseline  SnapshotRef  `json:"baseline"`
	Candidate SnapshotRef  `json:"candidate"`
	Findings  []SpecChange `json:"findings"`
	Policy    PolicyResult `json:"policy"`
}

// ReviewReport is the map-free canonical output of an offline contract
// compatibility review.
type ReviewReport struct {
	SchemaVersion string             `json:"schemaVersion"`
	EngineVersion string             `json:"engineVersion"`
	ContractID    string             `json:"contractId"`
	EvaluatedAt   time.Time          `json:"evaluatedAt"`
	PolicyDigest  string             `json:"policyDigest"`
	Verdict       string             `json:"verdict"`
	Comparisons   []ComparisonReport `json:"comparisons"`
}

// ReviewRequest supplies the target baseline, candidate, and optional release
// baseline for a deterministic compatibility review.
type ReviewRequest struct {
	ContractID    string
	Target        ContractSnapshot
	Candidate     ContractSnapshot
	Release       *ContractSnapshot
	Policy        EffectivePolicy
	EvaluatedAt   time.Time
	EngineVersion string
}

// EvaluateReview produces a canonical dual-baseline report. The pull-request
// comparison is always first; a supplied release baseline follows it.
func EvaluateReview(request ReviewRequest) (ReviewReport, error) {
	if err := validateReviewRequest(request); err != nil {
		return ReviewReport{}, err
	}

	policy, policyDigest, err := canonicalReviewPolicy(request.Policy)
	if err != nil {
		return ReviewReport{}, fmt.Errorf("canonicalize policy: %w", err)
	}

	evaluatedAt := request.EvaluatedAt.UTC()
	report := ReviewReport{
		SchemaVersion: ReviewSchemaVersion,
		EngineVersion: request.EngineVersion,
		ContractID:    request.ContractID,
		EvaluatedAt:   evaluatedAt,
		PolicyDigest:  policyDigest,
		Verdict:       VerdictPass,
		Comparisons: []ComparisonReport{
			evaluateComparison(ComparisonPullRequest, request.Target, request.Candidate, policy, evaluatedAt),
		},
	}
	if request.Release != nil {
		report.Comparisons = append(report.Comparisons,
			evaluateComparison(ComparisonReleaseImpact, *request.Release, request.Candidate, policy, evaluatedAt),
		)
	}
	for _, comparison := range report.Comparisons {
		if comparison.Policy.Verdict == VerdictFail {
			report.Verdict = VerdictFail
			break
		}
	}
	return report, nil
}

func validateReviewRequest(request ReviewRequest) error {
	if request.Target.ContractID != request.ContractID {
		return fmt.Errorf("target contract id %q does not match review contract id %q", request.Target.ContractID, request.ContractID)
	}
	if request.Candidate.ContractID != request.ContractID {
		return fmt.Errorf("candidate contract id %q does not match review contract id %q", request.Candidate.ContractID, request.ContractID)
	}
	if request.Release != nil && request.Release.ContractID != request.ContractID {
		return fmt.Errorf("release contract id %q does not match review contract id %q", request.Release.ContractID, request.ContractID)
	}
	if request.EvaluatedAt.IsZero() {
		return fmt.Errorf("evaluation time is required")
	}
	if strings.TrimSpace(request.EngineVersion) == "" {
		return fmt.Errorf("engine version is required")
	}
	if request.Policy.RequireReleaseBaseline && request.Release == nil {
		return fmt.Errorf("release baseline is required by policy")
	}
	return nil
}

func evaluateComparison(kind string, baseline, candidate ContractSnapshot, policy EffectivePolicy, evaluatedAt time.Time) ComparisonReport {
	diff := DiffContractSnapshots(baseline, candidate)
	findings := append([]SpecChange(nil), diff.BreakingChanges...)
	findings = append(findings, diff.AdditiveChanges...)
	sortSpecChanges(findings)

	result := EvaluateFindings(policy, findings, evaluatedAt)
	sortPolicyResult(&result)
	return ComparisonReport{
		Kind:      kind,
		Baseline:  snapshotRef(baseline),
		Candidate: snapshotRef(candidate),
		Findings:  findings,
		Policy:    result,
	}
}

func snapshotRef(snapshot ContractSnapshot) SnapshotRef {
	return SnapshotRef{
		RevisionID:     snapshot.RevisionID,
		SpecDigest:     snapshot.SpecDigest,
		ContractDigest: snapshot.ContractDigest,
	}
}

func sortPolicyResult(result *PolicyResult) {
	sort.Slice(result.Decisions, func(i, j int) bool {
		left, right := result.Decisions[i], result.Decisions[j]
		if comparison := compareSpecChanges(left.Finding, right.Finding); comparison != 0 {
			return comparison < 0
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Level != right.Level {
			return left.Level < right.Level
		}
		return !left.Excepted && right.Excepted
	})
	sort.Slice(result.AppliedExceptions, func(i, j int) bool {
		return comparePolicyExceptions(result.AppliedExceptions[i], result.AppliedExceptions[j]) < 0
	})
}

func compareSpecChanges(left, right SpecChange) int {
	for _, values := range [][2]string{
		{left.RuleID, right.RuleID},
		{left.Subject, right.Subject},
		{left.ID, right.ID},
		{left.Severity, right.Severity},
		{left.Kind, right.Kind},
		{left.Description, right.Description},
	} {
		if values[0] < values[1] {
			return -1
		}
		if values[0] > values[1] {
			return 1
		}
	}
	return 0
}

// CanonicalReviewJSON returns compact standard JSON for a report. ReviewReport
// deliberately contains no maps, so its sorted slices preserve byte stability.
func CanonicalReviewJSON(report ReviewReport) ([]byte, error) {
	return json.Marshal(report)
}

type canonicalPolicy struct {
	RequireReleaseBaseline bool                   `json:"requireReleaseBaseline"`
	Layers                 []canonicalPolicyLayer `json:"layers"`
}

type canonicalPolicyLayer struct {
	Name                   string                `json:"name"`
	Source                 string                `json:"source"`
	RequireReleaseBaseline bool                  `json:"requireReleaseBaseline"`
	Rules                  []canonicalPolicyRule `json:"rules"`
	Exceptions             []PolicyException     `json:"exceptions"`
}

type canonicalPolicyRule struct {
	ID    string    `json:"id"`
	Level RuleLevel `json:"level"`
}

type sortablePolicyLayer struct {
	layer PolicyLayer
	key   string
}

func canonicalReviewPolicy(policy EffectivePolicy) (EffectivePolicy, string, error) {
	sortedLayers := make([]sortablePolicyLayer, 0, len(policy.Layers))
	for _, layer := range policy.Layers {
		cloned := clonePolicyLayer(layer)
		sort.Slice(cloned.Exceptions, func(i, j int) bool {
			return comparePolicyExceptions(cloned.Exceptions[i], cloned.Exceptions[j]) < 0
		})
		canonicalLayer := canonicalizePolicyLayer(cloned)
		encoded, err := json.Marshal(canonicalLayer)
		if err != nil {
			return EffectivePolicy{}, "", err
		}
		sortedLayers = append(sortedLayers, sortablePolicyLayer{layer: cloned, key: string(encoded)})
	}
	sort.Slice(sortedLayers, func(i, j int) bool {
		return sortedLayers[i].key < sortedLayers[j].key
	})

	normalized := EffectivePolicy{RequireReleaseBaseline: policy.RequireReleaseBaseline}
	projection := canonicalPolicy{RequireReleaseBaseline: normalized.RequireReleaseBaseline}
	for _, sorted := range sortedLayers {
		normalized.Layers = append(normalized.Layers, sorted.layer)
		projection.Layers = append(projection.Layers, canonicalizePolicyLayer(sorted.layer))
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return EffectivePolicy{}, "", err
	}
	return normalized, sha256Hex(encoded), nil
}

func canonicalizePolicyLayer(layer PolicyLayer) canonicalPolicyLayer {
	canonical := canonicalPolicyLayer{
		Name:                   layer.Name,
		Source:                 layer.Source,
		RequireReleaseBaseline: layer.RequireReleaseBaseline,
		Exceptions:             append([]PolicyException(nil), layer.Exceptions...),
	}
	for _, ruleID := range sortedRuleIDs(layer.Rules) {
		canonical.Rules = append(canonical.Rules, canonicalPolicyRule{ID: ruleID, Level: layer.Rules[ruleID]})
	}
	sort.Slice(canonical.Exceptions, func(i, j int) bool {
		return comparePolicyExceptions(canonical.Exceptions[i], canonical.Exceptions[j]) < 0
	})
	return canonical
}

func comparePolicyExceptions(left, right PolicyException) int {
	for _, values := range [][2]string{
		{left.FindingID, right.FindingID},
		{left.RuleID, right.RuleID},
		{left.Reason, right.Reason},
		{left.Author, right.Author},
		{left.ExpiresAt.UTC().Format(time.RFC3339Nano), right.ExpiresAt.UTC().Format(time.RFC3339Nano)},
		{left.Source, right.Source},
	} {
		if values[0] < values[1] {
			return -1
		}
		if values[0] > values[1] {
			return 1
		}
	}
	return 0
}
