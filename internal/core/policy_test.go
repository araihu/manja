package core

import (
	"strings"
	"testing"
	"time"
)

func TestMergePolicyInitializesRepositoryDefaults(t *testing.T) {
	effective, err := MergePolicy(PolicyLayer{
		Name:   "stable",
		Source: PolicySourceRepository,
	})
	if err != nil {
		t.Fatal(err)
	}

	rules := effective.Layers[0].Rules
	if rules[RuleOperationRemoved] != RuleLevelFail {
		t.Fatalf("breaking default = %q", rules[RuleOperationRemoved])
	}
	if rules[RuleOperationAdded] != RuleLevelAllow {
		t.Fatalf("additive default = %q", rules[RuleOperationAdded])
	}
}

func TestMergePolicyRepositoryRuleOverridesDefault(t *testing.T) {
	effective, err := MergePolicy(PolicyLayer{
		Name:   "stable",
		Source: PolicySourceRepository,
		Rules: map[string]RuleLevel{
			RuleOperationRemoved: RuleLevelWarn,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := effective.Layers[0].Rules[RuleOperationRemoved]; got != RuleLevelWarn {
		t.Fatalf("repository override = %q", got)
	}
}

func TestMergePolicyAllowsStricterServerPolicy(t *testing.T) {
	repo := PolicyLayer{
		Name:   "stable",
		Source: PolicySourceRepository,
	}
	server := PolicyLayer{
		Name:   "public-v1",
		Source: PolicySourceServer,
		Rules: map[string]RuleLevel{
			RuleOperationAdded: RuleLevelWarn,
		},
	}

	effective, err := MergePolicy(repo, server)
	if err != nil {
		t.Fatal(err)
	}
	if got := effective.Layers[1].Rules[RuleOperationAdded]; got != RuleLevelWarn {
		t.Fatalf("server rule = %q", got)
	}
}

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

func TestMergePolicyORsReleaseBaselineRequirement(t *testing.T) {
	effective, err := MergePolicy(
		PolicyLayer{Name: "stable", Source: PolicySourceRepository},
		PolicyLayer{Name: "public-v1", Source: PolicySourceServer, RequireReleaseBaseline: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !effective.RequireReleaseBaseline {
		t.Fatal("RequireReleaseBaseline = false")
	}
}

func TestMergePolicyRejectsMalformedException(t *testing.T) {
	_, err := MergePolicy(PolicyLayer{
		Name:   "stable",
		Source: PolicySourceRepository,
		Exceptions: []PolicyException{{
			FindingID: "finding-1",
			RuleID:    RuleOperationRemoved,
			Reason:    "migration",
			Author:    "api-team",
			ExpiresAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
			Source:    PolicySourceRepository,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
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
	if len(result.Decisions) != 1 || !result.Decisions[0].Excepted {
		t.Fatalf("decisions = %#v", result.Decisions)
	}
}

func TestEvaluateFindingsKeepsRepositoryFailureWhenServerExceptionApplies(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	finding := SpecChange{ID: "finding-1", RuleID: RuleOperationRemoved, Severity: SpecChangeBreaking}
	effective, err := MergePolicy(
		PolicyLayer{Name: "stable", Source: PolicySourceRepository},
		PolicyLayer{
			Name: "public-v1", Source: PolicySourceServer,
			Rules: map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
			Exceptions: []PolicyException{{
				FindingID: "finding-1", Reason: "track migration", Author: "release-team",
				ExpiresAt: now.Add(time.Hour), Source: PolicySourceServer,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result := EvaluateFindings(effective, []SpecChange{finding}, now)
	if result.Verdict != VerdictFail {
		t.Fatalf("verdict = %q", result.Verdict)
	}
	if len(result.Decisions) != 2 || result.Decisions[0].Excepted || !result.Decisions[1].Excepted {
		t.Fatalf("decisions = %#v", result.Decisions)
	}
}

func TestEvaluateFindingsRejectsExpiredExceptionAtEvaluationTime(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	finding := SpecChange{ID: "finding-1", RuleID: RuleOperationRemoved, Severity: SpecChangeBreaking}
	effective, err := MergePolicy(PolicyLayer{
		Name: "stable", Source: PolicySourceRepository,
		Exceptions: []PolicyException{{
			FindingID: "finding-1", Reason: "expired migration", Author: "api-team",
			ExpiresAt: now, Source: PolicySourceRepository,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := EvaluateFindings(effective, []SpecChange{finding}, now)
	if result.Verdict != VerdictFail || len(result.AppliedExceptions) != 0 {
		t.Fatalf("result = %#v", result)
	}
}
