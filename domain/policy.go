package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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

type FindingDecision struct {
	Finding   SpecChange `json:"finding"`
	Level     RuleLevel  `json:"level"`
	Source    string     `json:"source"`
	LayerName string     `json:"layerName"`
	Excepted  bool       `json:"excepted"`
}

type PolicyExceptionDisposition struct {
	LayerName   string          `json:"layerName"`
	Disposition string          `json:"disposition"`
	Exception   PolicyException `json:"exception"`
}

type PolicyResult struct {
	Verdict               string                       `json:"verdict"`
	Decisions             []FindingDecision            `json:"decisions"`
	AppliedExceptions     []PolicyException            `json:"appliedExceptions,omitempty"`
	ExceptionDispositions []PolicyExceptionDisposition `json:"exceptionDispositions,omitempty"`
}

const (
	VerdictPass = "pass"
	VerdictFail = "fail"

	ExceptionDispositionApplied       = "applied"
	ExceptionDispositionExpired       = "expired"
	ExceptionDispositionNotApplicable = "not_applicable"
)

type supportedRule struct {
	ID           string
	DefaultLevel RuleLevel
}

var supportedRuleRegistry = []supportedRule{
	{ID: RuleOperationRemoved, DefaultLevel: RuleLevelFail},
	{ID: RuleOperationAdded, DefaultLevel: RuleLevelAllow},
	{ID: RuleRequiredParameterAdded, DefaultLevel: RuleLevelFail},
	{ID: RuleParameterBecameRequired, DefaultLevel: RuleLevelFail},
	{ID: RuleRequestBodyBecameRequired, DefaultLevel: RuleLevelFail},
	{ID: RuleResponseStatusRemoved, DefaultLevel: RuleLevelFail},
	{ID: RuleResponseStatusAdded, DefaultLevel: RuleLevelAllow},
	{ID: RuleSchemaRemoved, DefaultLevel: RuleLevelFail},
	{ID: RuleSchemaAdded, DefaultLevel: RuleLevelAllow},
}

// MergePolicy composes one repository policy with zero or more server policies.
// Server policy is monotonic: it can retain or increase repository severity,
// but cannot weaken it.
func MergePolicy(layers ...PolicyLayer) (EffectivePolicy, error) {
	if len(layers) == 0 {
		return EffectivePolicy{}, fmt.Errorf("policy requires one repository layer")
	}
	if layers[0].Source != PolicySourceRepository {
		return EffectivePolicy{}, fmt.Errorf("first policy layer must be repository")
	}

	repository, err := normalizedRepositoryLayer(layers[0])
	if err != nil {
		return EffectivePolicy{}, err
	}
	effective := EffectivePolicy{
		Layers:                 []PolicyLayer{repository},
		RequireReleaseBaseline: repository.RequireReleaseBaseline,
	}

	for index, layer := range layers[1:] {
		if layer.Source != PolicySourceServer {
			return EffectivePolicy{}, fmt.Errorf("policy layer %d must be server", index+1)
		}
		if err := validateLayer(layer); err != nil {
			return EffectivePolicy{}, fmt.Errorf("policy layer %q: %w", layer.Name, err)
		}
		for ruleID, level := range layer.Rules {
			if levelRank(level) < levelRank(repository.Rules[ruleID]) {
				return EffectivePolicy{}, fmt.Errorf("server rule %q cannot weaken repository level %q", ruleID, repository.Rules[ruleID])
			}
		}

		effective.Layers = append(effective.Layers, clonePolicyLayer(layer))
		effective.RequireReleaseBaseline = effective.RequireReleaseBaseline || layer.RequireReleaseBaseline
	}
	return effective, nil
}

// EvaluateFindings evaluates each repository and explicit server rule
// contribution independently at the supplied deterministic evaluation time.
func EvaluateFindings(policy EffectivePolicy, findings []SpecChange, evaluatedAt time.Time) PolicyResult {
	result := PolicyResult{Verdict: VerdictPass}
	seenExceptions := map[policyExceptionReference]bool{}
	dispositionIndexes := map[policyExceptionReference]int{}

	for layerIndex, layer := range policy.Layers {
		for exceptionIndex, exception := range layer.Exceptions {
			disposition := ExceptionDispositionNotApplicable
			if !evaluatedAt.Before(exception.ExpiresAt) {
				disposition = ExceptionDispositionExpired
			}
			reference := policyExceptionReference{layer: layerIndex, exception: exceptionIndex}
			dispositionIndexes[reference] = len(result.ExceptionDispositions)
			result.ExceptionDispositions = append(result.ExceptionDispositions, PolicyExceptionDisposition{
				LayerName:   layer.Name,
				Disposition: disposition,
				Exception:   clonePolicyException(exception),
			})
		}
	}

	for _, finding := range findings {
		for layerIndex, layer := range policy.Layers {
			level, contributes := contributionLevel(layer, finding)
			if !contributes {
				continue
			}

			matching := matchingExceptions(layer, finding, evaluatedAt)
			decision := FindingDecision{
				Finding:   finding,
				Level:     level,
				Source:    layer.Source,
				LayerName: layer.Name,
				Excepted:  len(matching) > 0,
			}
			result.Decisions = append(result.Decisions, decision)
			for _, exceptionIndex := range matching {
				reference := policyExceptionReference{layer: layerIndex, exception: exceptionIndex}
				result.ExceptionDispositions[dispositionIndexes[reference]].Disposition = ExceptionDispositionApplied
				if seenExceptions[reference] {
					continue
				}
				seenExceptions[reference] = true
				result.AppliedExceptions = append(result.AppliedExceptions, clonePolicyException(layer.Exceptions[exceptionIndex]))
			}
			if level == RuleLevelFail && !decision.Excepted {
				result.Verdict = VerdictFail
			}
		}
	}
	return result
}

func normalizedRepositoryLayer(layer PolicyLayer) (PolicyLayer, error) {
	if err := validateLayer(layer); err != nil {
		return PolicyLayer{}, fmt.Errorf("repository policy layer %q: %w", layer.Name, err)
	}

	normalized := clonePolicyLayer(layer)
	normalized.Rules = repositoryDefaultRules()
	for ruleID, level := range layer.Rules {
		normalized.Rules[ruleID] = level
	}
	return normalized, nil
}

func validateLayer(layer PolicyLayer) error {
	if strings.TrimSpace(layer.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if layer.Source != PolicySourceRepository && layer.Source != PolicySourceServer {
		return fmt.Errorf("source %q is invalid", layer.Source)
	}
	for _, ruleID := range sortedRuleIDs(layer.Rules) {
		if strings.TrimSpace(ruleID) == "" {
			return fmt.Errorf("rule id is required")
		}
		if !isSupportedRuleID(ruleID) {
			return fmt.Errorf("unknown rule %q", ruleID)
		}
		if !validRuleLevel(layer.Rules[ruleID]) {
			return fmt.Errorf("rule %q has invalid level %q", ruleID, layer.Rules[ruleID])
		}
	}
	for index, exception := range layer.Exceptions {
		if err := validatePolicyException(exception, layer.Source); err != nil {
			return fmt.Errorf("exception %d: %w", index, err)
		}
	}
	return nil
}

func validatePolicyException(exception PolicyException, layerSource string) error {
	hasFindingID := strings.TrimSpace(exception.FindingID) != ""
	hasRuleID := strings.TrimSpace(exception.RuleID) != ""
	if hasFindingID == hasRuleID {
		return fmt.Errorf("exactly one finding id or rule id is required")
	}
	if hasFindingID && !isLowerSHA256(exception.FindingID) {
		return fmt.Errorf("finding id must be exactly 64 lowercase hexadecimal characters")
	}
	if hasRuleID && !isSupportedRuleID(exception.RuleID) {
		return fmt.Errorf("unknown rule %q", exception.RuleID)
	}
	if strings.TrimSpace(exception.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	if strings.TrimSpace(exception.Author) == "" {
		return fmt.Errorf("author is required")
	}
	if exception.ExpiresAt.IsZero() {
		return fmt.Errorf("expiry is required")
	}
	if exception.Source != layerSource {
		return fmt.Errorf("source %q does not match layer source %q", exception.Source, layerSource)
	}
	return nil
}

func repositoryDefaultRules() map[string]RuleLevel {
	rules := make(map[string]RuleLevel, len(supportedRuleRegistry))
	for _, rule := range supportedRuleRegistry {
		rules[rule.ID] = rule.DefaultLevel
	}
	return rules
}

func isSupportedRuleID(ruleID string) bool {
	for _, rule := range supportedRuleRegistry {
		if rule.ID == ruleID {
			return true
		}
	}
	return false
}

func contributionLevel(layer PolicyLayer, finding SpecChange) (RuleLevel, bool) {
	level, configured := layer.Rules[finding.RuleID]
	if layer.Source == PolicySourceRepository {
		if configured {
			return level, true
		}
		return defaultLevelForFinding(finding), true
	}
	return level, configured
}

func defaultLevelForFinding(finding SpecChange) RuleLevel {
	if finding.Severity == SpecChangeBreaking {
		return RuleLevelFail
	}
	return RuleLevelAllow
}

type policyExceptionReference struct {
	layer     int
	exception int
}

func matchingExceptions(layer PolicyLayer, finding SpecChange, evaluatedAt time.Time) []int {
	matching := make([]int, 0, 1)
	for index, exception := range layer.Exceptions {
		if exception.Source != layer.Source || !validPolicyException(exception, layer.Source) {
			continue
		}
		if !evaluatedAt.Before(exception.ExpiresAt) {
			continue
		}
		matchesFinding := exception.FindingID != "" && exception.FindingID == finding.ID
		matchesRule := exception.RuleID != "" && exception.RuleID == finding.RuleID
		if matchesFinding || matchesRule {
			matching = append(matching, index)
		}
	}
	return matching
}

func validPolicyException(exception PolicyException, layerSource string) bool {
	return validatePolicyException(exception, layerSource) == nil
}

func validRuleLevel(level RuleLevel) bool {
	return level == RuleLevelAllow || level == RuleLevelWarn || level == RuleLevelFail
}

func levelRank(level RuleLevel) int {
	switch level {
	case RuleLevelAllow:
		return 0
	case RuleLevelWarn:
		return 1
	case RuleLevelFail:
		return 2
	default:
		return -1
	}
}

func clonePolicyLayer(layer PolicyLayer) PolicyLayer {
	cloned := layer
	cloned.Rules = make(map[string]RuleLevel, len(layer.Rules))
	for ruleID, level := range layer.Rules {
		cloned.Rules[ruleID] = level
	}
	cloned.Exceptions = make([]PolicyException, len(layer.Exceptions))
	for index, exception := range layer.Exceptions {
		cloned.Exceptions[index] = clonePolicyException(exception)
	}
	return cloned
}

func clonePolicyException(exception PolicyException) PolicyException {
	cloned := exception
	cloned.ExpiresAt = exception.ExpiresAt.UTC()
	return cloned
}

func sortedRuleIDs(rules map[string]RuleLevel) []string {
	ruleIDs := make([]string, 0, len(rules))
	for ruleID := range rules {
		ruleIDs = append(ruleIDs, ruleID)
	}
	sort.Strings(ruleIDs)
	return ruleIDs
}
