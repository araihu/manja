package domain

import (
	"strings"
	"testing"
	"time"
)

const policyTestFindingID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
			FindingID: policyTestFindingID,
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

func TestMergePolicyRejectsUnknownRuleIdentifiers(t *testing.T) {
	expiresAt := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		layer PolicyLayer
	}{
		{
			name: "configured rule",
			layer: PolicyLayer{
				Name: "stable", Source: PolicySourceRepository,
				Rules: map[string]RuleLevel{"operation.aded": RuleLevelFail},
			},
		},
		{
			name: "rule-scoped exception",
			layer: PolicyLayer{
				Name: "stable", Source: PolicySourceRepository,
				Exceptions: []PolicyException{{
					RuleID: "operation.aded", Reason: "typo", Author: "api-team",
					ExpiresAt: expiresAt, Source: PolicySourceRepository,
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MergePolicy(tt.layer)
			if err == nil || !strings.Contains(err.Error(), "unknown rule") {
				t.Fatalf("error = %v, want unknown rule", err)
			}
		})
	}
}

func TestMergePolicyRejectsMalformedFindingExceptionIDs(t *testing.T) {
	expiresAt := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	for name, findingID := range map[string]string{
		"truncated": "4f6c9c3f",
		"uppercase": strings.Repeat("A", 64),
		"non-hex":   strings.Repeat("a", 63) + "g",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := MergePolicy(PolicyLayer{
				Name: "stable", Source: PolicySourceRepository,
				Exceptions: []PolicyException{{
					FindingID: findingID, Reason: "migration", Author: "api-team",
					ExpiresAt: expiresAt, Source: PolicySourceRepository,
				}},
			})
			if err == nil || !strings.Contains(err.Error(), "finding id") {
				t.Fatalf("error = %v, want invalid finding id", err)
			}
		})
	}
}

func TestEvaluateFindingsAppliesOnlySameLayerUnexpiredException(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	finding := SpecChange{ID: policyTestFindingID, RuleID: RuleOperationRemoved, Severity: SpecChangeBreaking}
	repo := PolicyLayer{
		Name: "stable", Source: PolicySourceRepository,
		Rules: map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
		Exceptions: []PolicyException{{
			FindingID: policyTestFindingID, Reason: "v2 migration", Author: "api-team",
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
	finding := SpecChange{ID: policyTestFindingID, RuleID: RuleOperationRemoved, Severity: SpecChangeBreaking}
	effective, err := MergePolicy(
		PolicyLayer{Name: "stable", Source: PolicySourceRepository},
		PolicyLayer{
			Name: "public-v1", Source: PolicySourceServer,
			Rules: map[string]RuleLevel{RuleOperationRemoved: RuleLevelFail},
			Exceptions: []PolicyException{{
				FindingID: policyTestFindingID, Reason: "track migration", Author: "release-team",
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
	finding := SpecChange{ID: policyTestFindingID, RuleID: RuleOperationRemoved, Severity: SpecChangeBreaking}
	effective, err := MergePolicy(PolicyLayer{
		Name: "stable", Source: PolicySourceRepository,
		Exceptions: []PolicyException{{
			FindingID: policyTestFindingID, Reason: "expired migration", Author: "api-team",
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

func TestEvaluateFindingsDoesNotMatchEmptyFindingIDToRuleException(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	policy, err := MergePolicy(PolicyLayer{
		Name: "stable", Source: PolicySourceRepository,
		Exceptions: []PolicyException{{
			RuleID: RuleOperationRemoved, Reason: "operation migration", Author: "api-team",
			ExpiresAt: now.Add(time.Hour), Source: PolicySourceRepository,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFindings(policy, []SpecChange{{
		RuleID: RuleSchemaRemoved, Severity: SpecChangeBreaking, Subject: "Schema Customer",
	}}, now)
	if len(result.AppliedExceptions) != 0 {
		t.Fatalf("unrelated rule exception matched empty finding id: %#v", result.AppliedExceptions)
	}
}
