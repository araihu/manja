package core

import (
	"bytes"
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
