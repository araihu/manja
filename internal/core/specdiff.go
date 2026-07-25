package core

import (
	"fmt"
	"sort"
	"strings"
)

const (
	SpecChangeAdditive = "additive"
	SpecChangeBreaking = "breaking"
)

type SpecDiff struct {
	BaselineRevisionID  string
	CandidateRevisionID string
	BreakingChanges     []SpecChange
	AdditiveChanges     []SpecChange
}

type SpecChange struct {
	Severity    string
	Kind        string
	Subject     string
	Description string
}

func DiffSpecIndexes(baseline, candidate SpecIndex) SpecDiff {
	diff := SpecDiff{
		BaselineRevisionID:  baseline.RevisionID,
		CandidateRevisionID: candidate.RevisionID,
	}

	baselineOperations := operationsByContractKey(baseline.Operations)
	candidateOperations := operationsByContractKey(candidate.Operations)
	for _, key := range sortedOperationKeys(baselineOperations) {
		base := baselineOperations[key]
		next, ok := candidateOperations[key]
		if !ok {
			diff.BreakingChanges = append(diff.BreakingChanges, SpecChange{
				Severity:    SpecChangeBreaking,
				Kind:        "Removed endpoint",
				Subject:     operationSubject(base),
				Description: "Endpoint exists in production docs but is missing from the candidate.",
			})
			continue
		}
		diff.BreakingChanges = append(diff.BreakingChanges, operationBreakingChanges(base, next)...)
		diff.AdditiveChanges = append(diff.AdditiveChanges, operationAdditiveChanges(base, next)...)
	}
	for _, key := range sortedOperationKeys(candidateOperations) {
		next := candidateOperations[key]
		if _, ok := baselineOperations[key]; ok {
			continue
		}
		diff.AdditiveChanges = append(diff.AdditiveChanges, SpecChange{
			Severity:    SpecChangeAdditive,
			Kind:        "Added endpoint",
			Subject:     operationSubject(next),
			Description: "Candidate adds a new endpoint without changing the production route.",
		})
	}

	baselineSchemas := schemasByName(baseline.Schemas)
	candidateSchemas := schemasByName(candidate.Schemas)
	for _, name := range sortedSchemaNames(baselineSchemas) {
		if _, ok := candidateSchemas[name]; ok {
			continue
		}
		diff.BreakingChanges = append(diff.BreakingChanges, SpecChange{
			Severity:    SpecChangeBreaking,
			Kind:        "Removed schema",
			Subject:     baselineSchemas[name].Name,
			Description: "Schema exists in production docs but is missing from the candidate.",
		})
	}
	for _, name := range sortedSchemaNames(candidateSchemas) {
		if _, ok := baselineSchemas[name]; ok {
			continue
		}
		diff.AdditiveChanges = append(diff.AdditiveChanges, SpecChange{
			Severity:    SpecChangeAdditive,
			Kind:        "Added schema",
			Subject:     candidateSchemas[name].Name,
			Description: "Candidate adds a new reusable schema.",
		})
	}

	return diff
}

func operationBreakingChanges(baseline, candidate Operation) []SpecChange {
	changes := []SpecChange{}

	baselineParameters := parametersByContractKey(baseline.Parameters)
	for _, param := range candidate.Parameters {
		if !param.Required {
			continue
		}
		key := parameterContractKey(param)
		base, ok := baselineParameters[key]
		switch {
		case !ok:
			changes = append(changes, SpecChange{
				Severity:    SpecChangeBreaking,
				Kind:        "Required parameter added",
				Subject:     fmt.Sprintf("%s %s", operationSubject(candidate), parameterSubject(param)),
				Description: "Candidate requires a request parameter that production clients did not need.",
			})
		case !base.Required:
			changes = append(changes, SpecChange{
				Severity:    SpecChangeBreaking,
				Kind:        "Parameter became required",
				Subject:     fmt.Sprintf("%s %s", operationSubject(candidate), parameterSubject(param)),
				Description: "Candidate makes an existing optional request parameter required.",
			})
		}
	}

	if candidate.RequestBody != nil && candidate.RequestBody.Required {
		if baseline.RequestBody == nil || !baseline.RequestBody.Required {
			changes = append(changes, SpecChange{
				Severity:    SpecChangeBreaking,
				Kind:        "Request body became required",
				Subject:     operationSubject(candidate),
				Description: "Candidate requires a request body where production did not.",
			})
		}
	}

	candidateResponses := responseStatuses(candidate.Responses)
	for _, status := range sortedResponseStatuses(baseline.Responses) {
		if _, ok := candidateResponses[status]; ok {
			continue
		}
		changes = append(changes, SpecChange{
			Severity:    SpecChangeBreaking,
			Kind:        "Response status removed",
			Subject:     fmt.Sprintf("%s %s", operationSubject(baseline), status),
			Description: "Response status exists in production docs but is missing from the candidate.",
		})
	}

	return changes
}

func operationAdditiveChanges(baseline, candidate Operation) []SpecChange {
	changes := []SpecChange{}
	baselineResponses := responseStatuses(baseline.Responses)
	for _, status := range sortedResponseStatuses(candidate.Responses) {
		if _, ok := baselineResponses[status]; ok {
			continue
		}
		changes = append(changes, SpecChange{
			Severity:    SpecChangeAdditive,
			Kind:        "Response status added",
			Subject:     fmt.Sprintf("%s %s", operationSubject(candidate), status),
			Description: "Candidate documents a response status that production docs do not list.",
		})
	}
	return changes
}

func operationsByContractKey(operations []Operation) map[string]Operation {
	byKey := make(map[string]Operation, len(operations))
	for _, operation := range operations {
		byKey[operationContractKey(operation)] = operation
	}
	return byKey
}

func operationContractKey(operation Operation) string {
	return strings.ToUpper(strings.TrimSpace(operation.Method)) + " " + strings.TrimSpace(operation.Path)
}

func sortedOperationKeys(operations map[string]Operation) []string {
	keys := make([]string, 0, len(operations))
	for key := range operations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func operationSubject(operation Operation) string {
	return strings.TrimSpace(strings.ToUpper(operation.Method) + " " + operation.Path)
}

func parametersByContractKey(parameters []OperationParameter) map[string]OperationParameter {
	byKey := make(map[string]OperationParameter, len(parameters))
	for _, parameter := range parameters {
		byKey[parameterContractKey(parameter)] = parameter
	}
	return byKey
}

func parameterContractKey(parameter OperationParameter) string {
	return strings.ToLower(strings.TrimSpace(parameter.In)) + ":" + strings.TrimSpace(parameter.Name)
}

func parameterSubject(parameter OperationParameter) string {
	location := strings.TrimSpace(parameter.In)
	if location == "" {
		return strings.TrimSpace(parameter.Name)
	}
	return strings.TrimSpace(parameter.Name) + " (" + location + ")"
}

func responseStatuses(responses []OperationResponse) map[string]bool {
	statuses := make(map[string]bool, len(responses))
	for _, response := range responses {
		status := strings.TrimSpace(response.Status)
		if status != "" {
			statuses[status] = true
		}
	}
	return statuses
}

func sortedResponseStatuses(responses []OperationResponse) []string {
	statuses := responseStatuses(responses)
	keys := make([]string, 0, len(statuses))
	for status := range statuses {
		keys = append(keys, status)
	}
	sort.Strings(keys)
	return keys
}

func schemasByName(schemas []Schema) map[string]Schema {
	byName := make(map[string]Schema, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if name != "" {
			byName[name] = schema
		}
	}
	return byName
}

func sortedSchemaNames(schemas map[string]Schema) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
