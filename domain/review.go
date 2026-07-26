package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
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

type EffectivePolicyProjection struct {
	RequireReleaseBaseline bool                    `json:"requireReleaseBaseline"`
	Layers                 []PolicyLayerProjection `json:"layers"`
}

type PolicyLayerProjection struct {
	Name                   string                 `json:"name"`
	Source                 string                 `json:"source"`
	RequireReleaseBaseline bool                   `json:"requireReleaseBaseline"`
	Rules                  []PolicyRuleProjection `json:"rules"`
	Exceptions             []PolicyException      `json:"exceptions"`
}

type PolicyRuleProjection struct {
	RuleID string    `json:"ruleId"`
	Level  RuleLevel `json:"level"`
}

// ReviewReport is the map-free canonical output of an offline contract
// compatibility review.
type ReviewReport struct {
	SchemaVersion   string                    `json:"schemaVersion"`
	EngineVersion   string                    `json:"engineVersion"`
	ContractID      string                    `json:"contractId"`
	EvaluatedAt     time.Time                 `json:"evaluatedAt"`
	EffectivePolicy EffectivePolicyProjection `json:"effectivePolicy"`
	PolicyDigest    string                    `json:"policyDigest"`
	Verdict         string                    `json:"verdict"`
	Comparisons     []ComparisonReport        `json:"comparisons"`
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

// ValidateReleaseReviewReport preserves the public reference-level validation
// contract used by offline and external callers. Release authorization must use
// ValidateReleaseReviewReportAgainstSnapshots with independently persisted
// snapshots.
func ValidateReleaseReviewReport(report ReviewReport, contractID string, baseline, candidate SnapshotRef) error {
	_, _, err := validateReleaseReviewReport(report, contractID, baseline, candidate)
	return err
}

// ValidateReleaseReviewReportAgainstSnapshots reconstructs canonical release
// evidence from independently persisted baseline and candidate snapshots.
func ValidateReleaseReviewReportAgainstSnapshots(report ReviewReport, contractID string, baseline, candidate ContractSnapshot) error {
	validatedBaseline, err := validateAndCloneContractSnapshot(baseline)
	if err != nil {
		return fmt.Errorf("validate persisted review baseline: %w", err)
	}
	validatedCandidate, err := validateAndCloneContractSnapshot(candidate)
	if err != nil {
		return fmt.Errorf("validate persisted review candidate: %w", err)
	}
	if validatedBaseline.ContractID != contractID {
		return fmt.Errorf("persisted review baseline contract id %q does not match release contract id %q", validatedBaseline.ContractID, contractID)
	}
	if validatedCandidate.ContractID != contractID {
		return fmt.Errorf("persisted review candidate contract id %q does not match release contract id %q", validatedCandidate.ContractID, contractID)
	}
	canonicalPolicy, comparison, err := validateReleaseReviewReport(
		report,
		contractID,
		snapshotRef(validatedBaseline),
		snapshotRef(validatedCandidate),
	)
	if err != nil {
		return err
	}
	expectedComparison := evaluateComparison(
		ComparisonReleaseImpact,
		validatedBaseline,
		validatedCandidate,
		canonicalPolicy,
		report.EvaluatedAt,
	)
	if !reflect.DeepEqual(comparison, expectedComparison) {
		return fmt.Errorf("release review result does not match canonical evaluation")
	}
	if report.Verdict != expectedComparison.Policy.Verdict {
		return fmt.Errorf("review verdict %q does not match canonical comparison verdict %q", report.Verdict, expectedComparison.Policy.Verdict)
	}
	return nil
}

func validateReleaseReviewReport(report ReviewReport, contractID string, baseline, candidate SnapshotRef) (EffectivePolicy, ComparisonReport, error) {
	if report.SchemaVersion != ReviewSchemaVersion {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("unsupported review schema version %q", report.SchemaVersion)
	}
	if report.ContractID != contractID {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review contract id %q does not match release contract id %q", report.ContractID, contractID)
	}
	if strings.TrimSpace(report.EngineVersion) == "" {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review engine version is required")
	}
	if report.EvaluatedAt.IsZero() {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review evaluation time is required")
	}
	if report.EvaluatedAt.Location() != time.UTC {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review evaluation time must be UTC")
	}
	if !isLowerSHA256(report.PolicyDigest) {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review policy digest must be lowercase SHA-256")
	}
	canonicalPolicy, canonicalPolicyDigest, err := validateCanonicalPolicyProjection(report.EffectivePolicy)
	if err != nil {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review effective policy is invalid: %w", err)
	}
	if report.PolicyDigest != canonicalPolicyDigest {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review policy digest %q does not match effective policy digest %q", report.PolicyDigest, canonicalPolicyDigest)
	}
	if len(report.Comparisons) != 1 {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("release review must contain exactly one comparison")
	}
	comparison := report.Comparisons[0]
	if comparison.Kind != ComparisonReleaseImpact {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("release review comparison kind %q is not %q", comparison.Kind, ComparisonReleaseImpact)
	}
	if err := validateExpectedSnapshotRef("baseline", comparison.Baseline, baseline); err != nil {
		return EffectivePolicy{}, ComparisonReport{}, err
	}
	if err := validateExpectedSnapshotRef("candidate", comparison.Candidate, candidate); err != nil {
		return EffectivePolicy{}, ComparisonReport{}, err
	}
	if comparison.Policy.Verdict != VerdictPass && comparison.Policy.Verdict != VerdictFail {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("unsupported comparison verdict %q", comparison.Policy.Verdict)
	}
	if report.Verdict != comparison.Policy.Verdict {
		return EffectivePolicy{}, ComparisonReport{}, fmt.Errorf("review verdict %q does not match comparison verdict %q", report.Verdict, comparison.Policy.Verdict)
	}
	return canonicalPolicy, comparison, nil
}

func validateCanonicalPolicyProjection(projection EffectivePolicyProjection) (EffectivePolicy, string, error) {
	policy := EffectivePolicy{RequireReleaseBaseline: projection.RequireReleaseBaseline}
	for layerIndex, projectedLayer := range projection.Layers {
		layer := PolicyLayer{
			Name:                   projectedLayer.Name,
			Source:                 projectedLayer.Source,
			RequireReleaseBaseline: projectedLayer.RequireReleaseBaseline,
			Rules:                  make(map[string]RuleLevel, len(projectedLayer.Rules)),
			Exceptions:             append([]PolicyException(nil), projectedLayer.Exceptions...),
		}
		for _, rule := range projectedLayer.Rules {
			if _, exists := layer.Rules[rule.RuleID]; exists {
				return EffectivePolicy{}, "", fmt.Errorf("policy layer %d contains duplicate rule %q", layerIndex, rule.RuleID)
			}
			layer.Rules[rule.RuleID] = rule.Level
		}
		policy.Layers = append(policy.Layers, layer)
	}

	normalized, err := normalizeEffectivePolicy(policy)
	if err != nil {
		return EffectivePolicy{}, "", err
	}
	canonicalPolicy, canonicalProjection, digest, err := canonicalReviewPolicy(normalized)
	if err != nil {
		return EffectivePolicy{}, "", err
	}
	if !reflect.DeepEqual(projection, canonicalProjection) {
		return EffectivePolicy{}, "", fmt.Errorf("effective policy projection is not canonical")
	}
	return canonicalPolicy, digest, nil
}

func validateExpectedSnapshotRef(role string, actual, expected SnapshotRef) error {
	if err := validateSnapshotRef(role, actual); err != nil {
		return err
	}
	if err := validateSnapshotRef("expected "+role, expected); err != nil {
		return err
	}
	if actual.RevisionID != expected.RevisionID {
		return fmt.Errorf("review %s revision id %q does not match expected revision %q", role, actual.RevisionID, expected.RevisionID)
	}
	if actual.SpecDigest != expected.SpecDigest {
		return fmt.Errorf("review %s spec digest %q does not match expected digest %q", role, actual.SpecDigest, expected.SpecDigest)
	}
	if actual.ContractDigest != expected.ContractDigest {
		return fmt.Errorf("review %s contract digest %q does not match expected digest %q", role, actual.ContractDigest, expected.ContractDigest)
	}
	return nil
}

func validateSnapshotRef(role string, ref SnapshotRef) error {
	if strings.TrimSpace(ref.RevisionID) == "" {
		return fmt.Errorf("review %s revision id is required", role)
	}
	if !isLowerSHA256(ref.SpecDigest) {
		return fmt.Errorf("review %s spec digest must be lowercase SHA-256", role)
	}
	if !isLowerSHA256(ref.ContractDigest) {
		return fmt.Errorf("review %s contract digest must be lowercase SHA-256", role)
	}
	return nil
}

// EvaluateReview produces a canonical dual-baseline report. The pull-request
// comparison is always first; a supplied release baseline follows it.
func EvaluateReview(request ReviewRequest) (ReviewReport, error) {
	if err := validateReviewRequest(request); err != nil {
		return ReviewReport{}, err
	}

	target, err := validateAndCloneContractSnapshot(request.Target)
	if err != nil {
		return ReviewReport{}, fmt.Errorf("validate target snapshot: %w", err)
	}
	candidate, err := validateAndCloneContractSnapshot(request.Candidate)
	if err != nil {
		return ReviewReport{}, fmt.Errorf("validate candidate snapshot: %w", err)
	}
	var release *ContractSnapshot
	if request.Release != nil {
		validated, err := validateAndCloneContractSnapshot(*request.Release)
		if err != nil {
			return ReviewReport{}, fmt.Errorf("validate release snapshot: %w", err)
		}
		release = &validated
	}

	effectivePolicy, err := normalizeEffectivePolicy(request.Policy)
	if err != nil {
		return ReviewReport{}, fmt.Errorf("validate effective policy: %w", err)
	}
	if effectivePolicy.RequireReleaseBaseline && release == nil {
		return ReviewReport{}, fmt.Errorf("release baseline is required by policy")
	}

	policy, policyProjection, policyDigest, err := canonicalReviewPolicy(effectivePolicy)
	if err != nil {
		return ReviewReport{}, fmt.Errorf("canonicalize policy: %w", err)
	}

	evaluatedAt := request.EvaluatedAt.UTC()
	report := ReviewReport{
		SchemaVersion:   ReviewSchemaVersion,
		EngineVersion:   request.EngineVersion,
		ContractID:      request.ContractID,
		EvaluatedAt:     evaluatedAt,
		EffectivePolicy: policyProjection,
		PolicyDigest:    policyDigest,
		Verdict:         VerdictPass,
		Comparisons: []ComparisonReport{
			evaluateComparison(ComparisonPullRequest, target, candidate, policy, evaluatedAt),
		},
	}
	if release != nil {
		report.Comparisons = append(report.Comparisons,
			evaluateComparison(ComparisonReleaseImpact, *release, candidate, policy, evaluatedAt),
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
	return nil
}

func normalizeEffectivePolicy(policy EffectivePolicy) (EffectivePolicy, error) {
	normalized, err := MergePolicy(policy.Layers...)
	if err != nil {
		return EffectivePolicy{}, err
	}
	if normalized.RequireReleaseBaseline != policy.RequireReleaseBaseline {
		return EffectivePolicy{}, fmt.Errorf(
			"release baseline aggregate %t does not match policy layers %t",
			policy.RequireReleaseBaseline,
			normalized.RequireReleaseBaseline,
		)
	}
	return normalized, nil
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
		if left.LayerName != right.LayerName {
			return left.LayerName < right.LayerName
		}
		if left.Level != right.Level {
			return left.Level < right.Level
		}
		return !left.Excepted && right.Excepted
	})
	sort.Slice(result.AppliedExceptions, func(i, j int) bool {
		return comparePolicyExceptions(result.AppliedExceptions[i], result.AppliedExceptions[j]) < 0
	})
	sort.Slice(result.ExceptionDispositions, func(i, j int) bool {
		left, right := result.ExceptionDispositions[i], result.ExceptionDispositions[j]
		if left.Exception.Source != right.Exception.Source {
			return left.Exception.Source < right.Exception.Source
		}
		if left.LayerName != right.LayerName {
			return left.LayerName < right.LayerName
		}
		if comparison := comparePolicyExceptions(left.Exception, right.Exception); comparison != 0 {
			return comparison < 0
		}
		return left.Disposition < right.Disposition
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

type sortablePolicyLayer struct {
	layer      PolicyLayer
	projection PolicyLayerProjection
	key        string
}

func canonicalReviewPolicy(policy EffectivePolicy) (EffectivePolicy, EffectivePolicyProjection, string, error) {
	sortedLayers := make([]sortablePolicyLayer, 0, len(policy.Layers))
	for _, layer := range policy.Layers {
		cloned := clonePolicyLayer(layer)
		sort.Slice(cloned.Exceptions, func(i, j int) bool {
			return comparePolicyExceptions(cloned.Exceptions[i], cloned.Exceptions[j]) < 0
		})
		projection := projectPolicyLayer(cloned)
		encoded, err := json.Marshal(projection)
		if err != nil {
			return EffectivePolicy{}, EffectivePolicyProjection{}, "", err
		}
		sortedLayers = append(sortedLayers, sortablePolicyLayer{
			layer:      cloned,
			projection: projection,
			key:        string(encoded),
		})
	}
	sort.Slice(sortedLayers, func(i, j int) bool {
		leftRank := policySourceRank(sortedLayers[i].layer.Source)
		rightRank := policySourceRank(sortedLayers[j].layer.Source)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if sortedLayers[i].layer.Name != sortedLayers[j].layer.Name {
			return sortedLayers[i].layer.Name < sortedLayers[j].layer.Name
		}
		return sortedLayers[i].key < sortedLayers[j].key
	})

	normalized := EffectivePolicy{RequireReleaseBaseline: policy.RequireReleaseBaseline}
	projection := EffectivePolicyProjection{RequireReleaseBaseline: normalized.RequireReleaseBaseline}
	for _, sorted := range sortedLayers {
		normalized.Layers = append(normalized.Layers, sorted.layer)
		projection.Layers = append(projection.Layers, sorted.projection)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return EffectivePolicy{}, EffectivePolicyProjection{}, "", err
	}
	return normalized, projection, sha256Hex(encoded), nil
}

func projectPolicyLayer(layer PolicyLayer) PolicyLayerProjection {
	projection := PolicyLayerProjection{
		Name:                   layer.Name,
		Source:                 layer.Source,
		RequireReleaseBaseline: layer.RequireReleaseBaseline,
		Exceptions:             append([]PolicyException(nil), layer.Exceptions...),
	}
	for _, ruleID := range sortedRuleIDs(layer.Rules) {
		projection.Rules = append(projection.Rules, PolicyRuleProjection{
			RuleID: ruleID,
			Level:  layer.Rules[ruleID],
		})
	}
	sort.Slice(projection.Exceptions, func(i, j int) bool {
		return comparePolicyExceptions(projection.Exceptions[i], projection.Exceptions[j]) < 0
	})
	return projection
}

func policySourceRank(source string) int {
	if source == PolicySourceRepository {
		return 0
	}
	return 1
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
