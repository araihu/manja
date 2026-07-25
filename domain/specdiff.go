package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	SpecChangeAdditive = "additive"
	SpecChangeBreaking = "breaking"

	RuleOperationRemoved          = "operation.removed"
	RuleOperationAdded            = "operation.added"
	RuleRequiredParameterAdded    = "request.parameter.required_added"
	RuleParameterBecameRequired   = "request.parameter.became_required"
	RuleRequestBodyBecameRequired = "request.body.became_required"
	RuleResponseStatusRemoved     = "response.status.removed"
	RuleResponseStatusAdded       = "response.status.added"
	RuleSchemaRemoved             = "schema.removed"
	RuleSchemaAdded               = "schema.added"
)

type SpecDiff struct {
	BaselineRevisionID  string
	CandidateRevisionID string
	BreakingChanges     []SpecChange
	AdditiveChanges     []SpecChange
}

type SpecChange struct {
	ID          string `json:"id"`
	RuleID      string `json:"ruleId"`
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
}

// DiffSpecIndexes preserves the management-facing diff API while delegating
// comparison to normalized snapshots.
func DiffSpecIndexes(baseline, candidate SpecIndex) SpecDiff {
	return DiffContractSnapshots(
		NewContractSnapshot("", baseline.RevisionID, nil, baseline),
		NewContractSnapshot("", candidate.RevisionID, nil, candidate),
	)
}

// DiffContractSnapshots compares normalized contract surfaces and emits stable
// findings suitable for storage, automation, and repeatable CLI output.
func DiffContractSnapshots(baseline, candidate ContractSnapshot) SpecDiff {
	diff := SpecDiff{
		BaselineRevisionID:  baseline.RevisionID,
		CandidateRevisionID: candidate.RevisionID,
	}

	baselineOperations := contractOperationsByKey(baseline.Operations)
	candidateOperations := contractOperationsByKey(candidate.Operations)
	for _, key := range sortedContractOperationKeys(baselineOperations) {
		base := baselineOperations[key]
		next, ok := candidateOperations[key]
		if !ok {
			diff.BreakingChanges = append(diff.BreakingChanges, newSpecChange(
				RuleOperationRemoved,
				SpecChangeBreaking,
				"Removed endpoint",
				contractOperationSubject(base),
				"Endpoint exists in production docs but is missing from the candidate.",
			))
			continue
		}
		diff.BreakingChanges = append(diff.BreakingChanges, contractOperationBreakingChanges(base, next)...)
		diff.AdditiveChanges = append(diff.AdditiveChanges, contractOperationAdditiveChanges(base, next)...)
	}
	for _, key := range sortedContractOperationKeys(candidateOperations) {
		next := candidateOperations[key]
		if _, ok := baselineOperations[key]; ok {
			continue
		}
		diff.AdditiveChanges = append(diff.AdditiveChanges, newSpecChange(
			RuleOperationAdded,
			SpecChangeAdditive,
			"Added endpoint",
			contractOperationSubject(next),
			"Candidate adds a new endpoint without changing the production route.",
		))
	}

	baselineSchemas := contractSchemasByName(baseline.Schemas)
	candidateSchemas := contractSchemasByName(candidate.Schemas)
	for _, name := range sortedContractSchemaNames(baselineSchemas) {
		if _, ok := candidateSchemas[name]; ok {
			continue
		}
		diff.BreakingChanges = append(diff.BreakingChanges, newSpecChange(
			RuleSchemaRemoved,
			SpecChangeBreaking,
			"Removed schema",
			name,
			"Schema exists in production docs but is missing from the candidate.",
		))
	}
	for _, name := range sortedContractSchemaNames(candidateSchemas) {
		if _, ok := baselineSchemas[name]; ok {
			continue
		}
		diff.AdditiveChanges = append(diff.AdditiveChanges, newSpecChange(
			RuleSchemaAdded,
			SpecChangeAdditive,
			"Added schema",
			name,
			"Candidate adds a new reusable schema.",
		))
	}

	sortSpecChanges(diff.BreakingChanges)
	sortSpecChanges(diff.AdditiveChanges)
	return diff
}

func stableFindingID(ruleID, subject string) string {
	digest := sha256.Sum256([]byte(ruleID + "\x00" + normalizedFindingSubject(subject)))
	return hex.EncodeToString(digest[:])
}

func normalizedFindingSubject(subject string) string {
	return strings.Join(strings.Fields(subject), " ")
}

func newSpecChange(ruleID, severity, kind, subject, description string) SpecChange {
	subject = normalizedFindingSubject(subject)
	return SpecChange{
		ID:          stableFindingID(ruleID, subject),
		RuleID:      ruleID,
		Severity:    severity,
		Kind:        kind,
		Subject:     subject,
		Description: description,
	}
}

func contractOperationBreakingChanges(baseline, candidate ContractOperation) []SpecChange {
	changes := []SpecChange{}

	baselineParameters := contractParametersByKey(baseline.Parameters)
	for _, param := range candidate.Parameters {
		if !param.Required {
			continue
		}
		key := contractParameterKey(param)
		base, ok := baselineParameters[key]
		switch {
		case !ok:
			changes = append(changes, newSpecChange(
				RuleRequiredParameterAdded,
				SpecChangeBreaking,
				"Required parameter added",
				fmt.Sprintf("%s %s", contractOperationSubject(candidate), contractParameterSubject(param)),
				"Candidate requires a request parameter that production clients did not need.",
			))
		case !base.Required:
			changes = append(changes, newSpecChange(
				RuleParameterBecameRequired,
				SpecChangeBreaking,
				"Parameter became required",
				fmt.Sprintf("%s %s", contractOperationSubject(candidate), contractParameterSubject(param)),
				"Candidate makes an existing optional request parameter required.",
			))
		}
	}

	if candidate.RequestBodyRequired && !baseline.RequestBodyRequired {
		changes = append(changes, newSpecChange(
			RuleRequestBodyBecameRequired,
			SpecChangeBreaking,
			"Request body became required",
			contractOperationSubject(candidate),
			"Candidate requires a request body where production did not.",
		))
	}

	candidateResponses := contractResponseStatuses(candidate.ResponseStatuses)
	for _, status := range baseline.ResponseStatuses {
		if candidateResponses[status] {
			continue
		}
		changes = append(changes, newSpecChange(
			RuleResponseStatusRemoved,
			SpecChangeBreaking,
			"Response status removed",
			fmt.Sprintf("%s %s", contractOperationSubject(baseline), status),
			"Response status exists in production docs but is missing from the candidate.",
		))
	}

	return changes
}

func contractOperationAdditiveChanges(baseline, candidate ContractOperation) []SpecChange {
	changes := []SpecChange{}
	baselineResponses := contractResponseStatuses(baseline.ResponseStatuses)
	for _, status := range candidate.ResponseStatuses {
		if baselineResponses[status] {
			continue
		}
		changes = append(changes, newSpecChange(
			RuleResponseStatusAdded,
			SpecChangeAdditive,
			"Response status added",
			fmt.Sprintf("%s %s", contractOperationSubject(candidate), status),
			"Candidate documents a response status that production docs do not list.",
		))
	}
	return changes
}

func contractOperationsByKey(operations []ContractOperation) map[string]ContractOperation {
	byKey := make(map[string]ContractOperation, len(operations))
	for _, operation := range operations {
		byKey[contractOperationKey(operation)] = operation
	}
	return byKey
}

func contractOperationKey(operation ContractOperation) string {
	return operation.Method + " " + operation.Path
}

func sortedContractOperationKeys(operations map[string]ContractOperation) []string {
	keys := make([]string, 0, len(operations))
	for key := range operations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contractOperationSubject(operation ContractOperation) string {
	return operation.Method + " " + operation.Path
}

func contractParametersByKey(parameters []ContractParameter) map[string]ContractParameter {
	byKey := make(map[string]ContractParameter, len(parameters))
	for _, parameter := range parameters {
		byKey[contractParameterKey(parameter)] = parameter
	}
	return byKey
}

func contractParameterKey(parameter ContractParameter) string {
	return parameter.In + ":" + parameter.Name
}

func contractParameterSubject(parameter ContractParameter) string {
	if parameter.In == "" {
		return parameter.Name
	}
	return parameter.Name + " (" + parameter.In + ")"
}

func contractResponseStatuses(statuses []string) map[string]bool {
	set := make(map[string]bool, len(statuses))
	for _, status := range statuses {
		set[status] = true
	}
	return set
}

func contractSchemasByName(schemas []string) map[string]bool {
	byName := make(map[string]bool, len(schemas))
	for _, name := range schemas {
		byName[name] = true
	}
	return byName
}

func sortedContractSchemaNames(schemas map[string]bool) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortSpecChanges(changes []SpecChange) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].RuleID != changes[j].RuleID {
			return changes[i].RuleID < changes[j].RuleID
		}
		if changes[i].Subject != changes[j].Subject {
			return changes[i].Subject < changes[j].Subject
		}
		return changes[i].ID < changes[j].ID
	})
}
