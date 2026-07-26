package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
	if report.Verdict != VerdictFail {
		t.Fatalf("verdict = %q", report.Verdict)
	}
}

func TestEvaluateReviewRequiresReleaseBaselineWhenPolicyRequiresIt(t *testing.T) {
	policy, err := MergePolicy(PolicyLayer{
		Name: "stable", Source: PolicySourceRepository, RequireReleaseBaseline: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = EvaluateReview(reviewRequestForTest(policy))
	if err == nil {
		t.Fatal("EvaluateReview() error = nil")
	}
}

func TestEvaluateReviewRejectsInvalidRequestMetadata(t *testing.T) {
	policy, err := MergePolicy(PolicyLayer{Name: "stable", Source: PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*ReviewRequest){
		"mismatched candidate contract": func(request *ReviewRequest) {
			request.Candidate.ContractID = "orders"
		},
		"mismatched release contract": func(request *ReviewRequest) {
			release := request.Target
			release.ContractID = "orders"
			request.Release = &release
		},
		"zero evaluation time": func(request *ReviewRequest) {
			request.EvaluatedAt = time.Time{}
		},
		"blank engine version": func(request *ReviewRequest) {
			request.EngineVersion = " \t"
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := reviewRequestForTest(policy)
			mutate(&request)
			if _, err := EvaluateReview(request); err == nil {
				t.Fatal("EvaluateReview() error = nil")
			}
		})
	}
}

func TestEvaluateReviewRejectsMalformedEffectivePolicy(t *testing.T) {
	tests := []struct {
		name        string
		policy      EffectivePolicy
		withRelease bool
		want        string
	}{
		{
			name: "server-only policy",
			policy: EffectivePolicy{Layers: []PolicyLayer{{
				Name: "public-v1", Source: PolicySourceServer,
				Rules: map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
			}}},
			want: "first policy layer must be repository",
		},
		{
			name: "invalid level",
			policy: EffectivePolicy{Layers: []PolicyLayer{{
				Name: "stable", Source: PolicySourceRepository,
				Rules: map[string]RuleLevel{RuleOperationRemoved: "block"},
			}}},
			want: "invalid level",
		},
		{
			name: "false aggregate omits layer requirement",
			policy: EffectivePolicy{Layers: []PolicyLayer{{
				Name: "stable", Source: PolicySourceRepository, RequireReleaseBaseline: true,
			}}},
			want: "release baseline aggregate",
		},
		{
			name: "true aggregate invents layer requirement",
			policy: EffectivePolicy{
				Layers: []PolicyLayer{{
					Name: "stable", Source: PolicySourceRepository,
				}},
				RequireReleaseBaseline: true,
			},
			withRelease: true,
			want:        "release baseline aggregate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := reviewRequestForTest(tt.policy)
			if tt.withRelease {
				release := request.Target
				request.Release = &release
			}
			_, err := EvaluateReview(request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("EvaluateReview error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestEvaluateReviewRejectsMalformedContractSnapshots(t *testing.T) {
	policy, err := MergePolicy(PolicyLayer{Name: "stable", Source: PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}
	valid := NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{
		Operations: []Operation{
			{Method: "GET", Path: "/payments"},
			{Method: "POST", Path: "/payments"},
		},
		Schemas: []Schema{{Name: "Error"}, {Name: "Payment"}},
	})

	tests := []struct {
		name   string
		mutate func(*ContractSnapshot)
		want   string
	}{
		{
			name: "missing revision identity",
			mutate: func(snapshot *ContractSnapshot) {
				snapshot.RevisionID = " "
			},
			want: "revision id",
		},
		{
			name: "malformed spec digest",
			mutate: func(snapshot *ContractSnapshot) {
				snapshot.SpecDigest = strings.Repeat("A", 64)
			},
			want: "spec digest",
		},
		{
			name: "stale contract digest",
			mutate: func(snapshot *ContractSnapshot) {
				snapshot.ContractDigest = strings.Repeat("0", 64)
			},
			want: "contract digest",
		},
		{
			name: "non-normalized surface",
			mutate: func(snapshot *ContractSnapshot) {
				snapshot.Operations[0].Method = "get"
			},
			want: "normalized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := valid
			target.Operations = append([]ContractOperation(nil), valid.Operations...)
			target.Schemas = append([]string(nil), valid.Schemas...)
			tt.mutate(&target)
			request := ReviewRequest{
				ContractID:    "payments",
				Target:        target,
				Candidate:     valid,
				Policy:        policy,
				EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
				EngineVersion: "test",
			}
			_, err := EvaluateReview(request)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("EvaluateReview error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCanonicalReviewJSONIsStable(t *testing.T) {
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	policy, err := MergePolicy(
		PolicyLayer{
			Name: "stable", Source: PolicySourceRepository,
			Rules: map[string]RuleLevel{RuleOperationAdded: RuleLevelWarn},
		},
		PolicyLayer{
			Name: "public", Source: PolicySourceServer,
			Rules: map[string]RuleLevel{RuleSchemaAdded: RuleLevelWarn},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	report, err := EvaluateReview(ReviewRequest{
		ContractID: "payments",
		Target:     NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{}),
		Candidate: NewContractSnapshot("payments", "head", []byte("head"), SpecIndex{
			Operations: []Operation{{Method: "POST", Path: "/payments"}},
			Schemas:    []Schema{{Name: "Payment"}},
		}),
		Policy: policy, EvaluatedAt: at, EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("canonical JSON differs:\n%s\n%s", first, second)
	}
	if report.PolicyDigest == "" {
		t.Fatal("PolicyDigest is empty")
	}
}

func TestEvaluateReviewReportsEffectivePolicyAndLayerProvenance(t *testing.T) {
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	policy, err := MergePolicy(
		PolicyLayer{Name: "stable", Source: PolicySourceRepository},
		PolicyLayer{
			Name: "public-v1", Source: PolicySourceServer,
			Rules: map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateReview(ReviewRequest{
		ContractID: "payments",
		Target: NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{
			Operations: []Operation{{Method: "GET", Path: "/payments"}},
		}),
		Candidate:     NewContractSnapshot("payments", "head", []byte("head"), SpecIndex{}),
		Policy:        policy,
		EvaluatedAt:   at,
		EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.EffectivePolicy.Layers) != 2 {
		t.Fatalf("effective policy = %#v", report.EffectivePolicy)
	}
	repository := report.EffectivePolicy.Layers[0]
	server := report.EffectivePolicy.Layers[1]
	if repository.Name != "stable" || repository.Source != PolicySourceRepository || len(repository.Rules) != len(supportedRuleRegistry) {
		t.Fatalf("repository policy evidence = %#v", repository)
	}
	if server.Name != "public-v1" || server.Source != PolicySourceServer ||
		len(server.Rules) != 1 || server.Rules[0].RuleID != RuleOperationRemoved || server.Rules[0].Level != RuleLevelFail {
		t.Fatalf("server policy evidence = %#v", server)
	}
	decisionLayers := map[string]string{}
	for _, decision := range report.Comparisons[0].Policy.Decisions {
		decisionLayers[decision.Source] = decision.LayerName
	}
	if decisionLayers[PolicySourceRepository] != "stable" || decisionLayers[PolicySourceServer] != "public-v1" {
		t.Fatalf("decision layer provenance = %#v", decisionLayers)
	}
	projectionJSON, err := json.Marshal(report.EffectivePolicy)
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256Hex(projectionJSON); report.PolicyDigest != want {
		t.Fatalf("policy digest = %q, want digest of embedded projection %q", report.PolicyDigest, want)
	}
}

func TestEvaluateReviewReportsExceptionDispositions(t *testing.T) {
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	appliedID := stableFindingID(RuleOperationRemoved, "GET /payments")
	unmatchedID := strings.Repeat("f", 64)
	policy, err := MergePolicy(PolicyLayer{
		Name: "stable", Source: PolicySourceRepository,
		Exceptions: []PolicyException{
			{
				FindingID: appliedID, Reason: "applied", Author: "api-team",
				ExpiresAt: at.Add(time.Hour), Source: PolicySourceRepository,
			},
			{
				RuleID: RuleOperationRemoved, Reason: "expired", Author: "api-team",
				ExpiresAt: at, Source: PolicySourceRepository,
			},
			{
				FindingID: unmatchedID, Reason: "unmatched", Author: "api-team",
				ExpiresAt: at.Add(time.Hour), Source: PolicySourceRepository,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateReview(ReviewRequest{
		ContractID: "payments",
		Target: NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{
			Operations: []Operation{{Method: "GET", Path: "/payments"}},
		}),
		Candidate:     NewContractSnapshot("payments", "head", []byte("head"), SpecIndex{}),
		Policy:        policy,
		EvaluatedAt:   at,
		EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := report.Comparisons[0].Policy
	got := map[string]PolicyExceptionDisposition{}
	for _, disposition := range result.ExceptionDispositions {
		got[disposition.Exception.Reason] = disposition
	}
	for reason, want := range map[string]string{
		"applied":   ExceptionDispositionApplied,
		"expired":   ExceptionDispositionExpired,
		"unmatched": ExceptionDispositionNotApplicable,
	} {
		disposition, ok := got[reason]
		if !ok || disposition.Disposition != want || disposition.LayerName != "stable" {
			t.Fatalf("disposition %q = %#v, all = %#v", reason, disposition, result.ExceptionDispositions)
		}
	}
	if len(result.AppliedExceptions) != 1 || result.AppliedExceptions[0].Reason != "applied" {
		t.Fatalf("applied exceptions = %#v", result.AppliedExceptions)
	}
}

func TestEvaluateReviewCanonicalizesEquivalentExceptionExpiryOffsets(t *testing.T) {
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	expiryUTC := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	expiryOffset := time.Date(2026, 7, 25, 21, 0, 0, 0, time.FixedZone("minus-three", -3*60*60))
	buildPolicy := func(expiry time.Time) EffectivePolicy {
		t.Helper()
		policy, err := MergePolicy(PolicyLayer{
			Name: "stable", Source: PolicySourceRepository,
			Exceptions: []PolicyException{{
				RuleID: RuleSchemaAdded, Reason: "migration", Author: "api-team",
				ExpiresAt: expiry, Source: PolicySourceRepository,
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return policy
	}
	buildReport := func(policy EffectivePolicy) ReviewReport {
		t.Helper()
		snapshot := NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{})
		report, err := EvaluateReview(ReviewRequest{
			ContractID: "payments", Target: snapshot, Candidate: snapshot,
			Policy: policy, EvaluatedAt: at, EngineVersion: "test",
		})
		if err != nil {
			t.Fatal(err)
		}
		return report
	}

	first := buildReport(buildPolicy(expiryUTC))
	second := buildReport(buildPolicy(expiryOffset))
	if first.PolicyDigest != second.PolicyDigest {
		t.Fatalf("policy digests differ: %q != %q", first.PolicyDigest, second.PolicyDigest)
	}
	firstJSON, err := CanonicalReviewJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := CanonicalReviewJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical JSON differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestValidateReleaseReviewReportRejectsNonCanonicalEvidence(t *testing.T) {
	report, baseline, candidate := canonicalReleaseReportForTest(t)

	tests := []struct {
		name   string
		mutate func(*ReviewReport)
		want   string
	}{
		{
			name: "zero evaluation time",
			mutate: func(report *ReviewReport) {
				report.EvaluatedAt = time.Time{}
			},
			want: "evaluation time",
		},
		{
			name: "absent repository policy",
			mutate: func(report *ReviewReport) {
				report.EffectivePolicy = EffectivePolicyProjection{}
			},
			want: "repository",
		},
		{
			name: "invalid repository policy",
			mutate: func(report *ReviewReport) {
				report.EffectivePolicy.Layers[0].Source = PolicySourceServer
			},
			want: "repository",
		},
		{
			name: "policy digest mismatch",
			mutate: func(report *ReviewReport) {
				report.PolicyDigest = strings.Repeat("f", 64)
			},
			want: "policy digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := report
			mutated.EffectivePolicy.Layers = append([]PolicyLayerProjection(nil), report.EffectivePolicy.Layers...)
			tt.mutate(&mutated)
			err := ValidateReleaseReviewReport(mutated, "payments", baseline, candidate)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateReleaseReviewReport error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateReleaseReviewReportAcceptsCanonicalEvidence(t *testing.T) {
	report, baseline, candidate := canonicalReleaseReportForTest(t)
	if err := ValidateReleaseReviewReport(report, "payments", baseline, candidate); err != nil {
		t.Fatalf("ValidateReleaseReviewReport canonical evidence: %v", err)
	}
}

func TestValidateReleaseReviewReportRejectsRewrittenCanonicalResult(t *testing.T) {
	report, baseline, candidate := canonicalReleaseReportWithFindingForTest(t, true)
	if len(report.Comparisons[0].Findings) != 1 ||
		len(report.Comparisons[0].Policy.Decisions) != 1 ||
		len(report.Comparisons[0].Policy.AppliedExceptions) != 1 ||
		len(report.Comparisons[0].Policy.ExceptionDispositions) != 1 {
		t.Fatalf("test fixture lacks complete canonical result: %#v", report.Comparisons[0])
	}
	tests := []struct {
		name   string
		mutate func(*ReviewReport)
	}{
		{
			name: "rewritten finding",
			mutate: func(report *ReviewReport) {
				report.Comparisons[0].Findings[0].Description = "forged description"
			},
		},
		{
			name: "removed finding",
			mutate: func(report *ReviewReport) {
				report.Comparisons[0].Findings = nil
			},
		},
		{
			name: "rewritten decision",
			mutate: func(report *ReviewReport) {
				report.Comparisons[0].Policy.Decisions[0].Excepted = false
			},
		},
		{
			name: "rewritten applied exception",
			mutate: func(report *ReviewReport) {
				report.Comparisons[0].Policy.AppliedExceptions[0].Reason = "forged reason"
			},
		},
		{
			name: "rewritten exception disposition",
			mutate: func(report *ReviewReport) {
				report.Comparisons[0].Policy.ExceptionDispositions[0].Disposition = ExceptionDispositionExpired
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := cloneReviewReportForTest(t, report)
			tt.mutate(&mutated)
			if err := ValidateReleaseReviewReport(
				mutated,
				"payments",
				baseline,
				candidate,
			); err == nil {
				t.Fatal("rewritten canonical review result was accepted")
			}
		})
	}
}

func TestValidateReleaseReviewReportRejectsForgedPassWithValidEvidence(t *testing.T) {
	report, baseline, candidate := canonicalReleaseReportWithFindingForTest(t, false)
	if report.Verdict != VerdictFail {
		t.Fatalf("canonical fixture verdict = %q, want fail", report.Verdict)
	}
	report.Comparisons[0].Findings = nil
	report.Comparisons[0].Policy = PolicyResult{Verdict: VerdictPass}
	report.Verdict = VerdictPass

	if err := ValidateReleaseReviewReport(
		report,
		"payments",
		baseline,
		candidate,
	); err == nil {
		t.Fatal("forged passing result with valid snapshot and policy evidence was accepted")
	}
}

func canonicalReleaseReportWithFindingForTest(t *testing.T, exceptFinding bool) (ReviewReport, ContractSnapshot, ContractSnapshot) {
	t.Helper()
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	baseline := NewContractSnapshot("payments", "revision-good", []byte("baseline"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	candidate := NewContractSnapshot("payments", "revision-next", []byte("candidate"), SpecIndex{})
	layer := PolicyLayer{Name: "stable", Source: PolicySourceRepository}
	if exceptFinding {
		layer.Exceptions = []PolicyException{{
			RuleID: RuleOperationRemoved, Reason: "planned migration", Author: "api-team",
			ExpiresAt: at.Add(time.Hour), Source: PolicySourceRepository,
		}}
	}
	policy, err := MergePolicy(layer)
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateReview(ReviewRequest{
		ContractID: "payments", Target: baseline, Candidate: candidate,
		Release: &baseline, Policy: policy, EvaluatedAt: at, EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	report.Comparisons = []ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	return report, baseline, candidate
}

func cloneReviewReportForTest(t *testing.T, report ReviewReport) ReviewReport {
	t.Helper()
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var cloned ReviewReport
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func canonicalReleaseReportForTest(t *testing.T) (ReviewReport, ContractSnapshot, ContractSnapshot) {
	t.Helper()
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	baselineSnapshot := NewContractSnapshot("payments", "revision-good", []byte("baseline"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	candidateSnapshot := NewContractSnapshot("payments", "revision-next", []byte("candidate"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	policy, err := MergePolicy(PolicyLayer{Name: "stable", Source: PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateReview(ReviewRequest{
		ContractID: "payments", Target: baselineSnapshot, Candidate: candidateSnapshot,
		Release: &baselineSnapshot, Policy: policy, EvaluatedAt: at, EngineVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	report.Comparisons = []ComparisonReport{report.Comparisons[1]}
	report.Verdict = report.Comparisons[0].Policy.Verdict
	return report, baselineSnapshot, candidateSnapshot
}

func reviewRequestForTest(policy EffectivePolicy) ReviewRequest {
	target := NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	return ReviewRequest{
		ContractID:    "payments",
		Target:        target,
		Candidate:     target,
		Policy:        policy,
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		EngineVersion: "test",
	}
}
