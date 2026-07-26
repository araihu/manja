package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

// ContractSnapshot is the normalized contract surface used for deterministic
// compatibility comparison.
type ContractSnapshot struct {
	ContractID     string              `json:"contractId"`
	RevisionID     string              `json:"revisionId"`
	SpecDigest     string              `json:"specDigest"`
	ContractDigest string              `json:"contractDigest"`
	Operations     []ContractOperation `json:"operations"`
	Schemas        []string            `json:"schemas"`
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

// NewContractSnapshot extracts the compatibility-relevant parts of idx and
// normalizes them before calculating content digests.
func NewContractSnapshot(contractID, revisionID string, raw []byte, idx SpecIndex) ContractSnapshot {
	snapshot := ContractSnapshot{
		ContractID: contractID,
		RevisionID: revisionID,
		SpecDigest: sha256Hex(raw),
		Operations: normalizeContractOperations(idx.Operations),
		Schemas:    normalizeContractSchemas(idx.Schemas),
	}

	contractDigest, err := contractSurfaceDigest(snapshot.Operations, snapshot.Schemas)
	if err != nil {
		panic("marshal normalized contract snapshot: " + err.Error())
	}
	snapshot.ContractDigest = contractDigest
	return snapshot
}

func validateAndCloneContractSnapshot(snapshot ContractSnapshot) (ContractSnapshot, error) {
	if err := validateCanonicalIdentity("contract id", snapshot.ContractID, false); err != nil {
		return ContractSnapshot{}, err
	}
	if err := validateCanonicalIdentity("revision id", snapshot.RevisionID, false); err != nil {
		return ContractSnapshot{}, err
	}
	if !isLowerSHA256(snapshot.SpecDigest) {
		return ContractSnapshot{}, fmt.Errorf("spec digest must be lowercase SHA-256")
	}
	if !isLowerSHA256(snapshot.ContractDigest) {
		return ContractSnapshot{}, fmt.Errorf("contract digest must be lowercase SHA-256")
	}
	if err := validateContractSurfaceIdentities(snapshot); err != nil {
		return ContractSnapshot{}, err
	}
	if err := validateContractSurfaceUniqueness(snapshot); err != nil {
		return ContractSnapshot{}, err
	}

	normalizedOperations := normalizeSnapshotOperations(snapshot.Operations)
	normalizedSchemas := normalizeSnapshotSchemas(snapshot.Schemas)
	if !reflect.DeepEqual(snapshot.Operations, normalizedOperations) ||
		!reflect.DeepEqual(snapshot.Schemas, normalizedSchemas) {
		return ContractSnapshot{}, fmt.Errorf("contract snapshot surface must be normalized and sorted")
	}
	recomputedDigest, err := contractSurfaceDigest(normalizedOperations, normalizedSchemas)
	if err != nil {
		return ContractSnapshot{}, fmt.Errorf("recompute contract digest: %w", err)
	}
	if snapshot.ContractDigest != recomputedDigest {
		return ContractSnapshot{}, fmt.Errorf(
			"contract digest %q does not match normalized surface digest %q",
			snapshot.ContractDigest,
			recomputedDigest,
		)
	}

	cloned := snapshot
	cloned.Operations = normalizedOperations
	cloned.Schemas = normalizedSchemas
	return cloned, nil
}

func validateContractSurfaceIdentities(snapshot ContractSnapshot) error {
	for operationIndex, operation := range snapshot.Operations {
		prefix := fmt.Sprintf("contract operation %d", operationIndex)
		if err := validateCanonicalIdentity(prefix+" method", operation.Method, false); err != nil {
			return err
		}
		if err := validateCanonicalIdentity(prefix+" path", operation.Path, false); err != nil {
			return err
		}
		for parameterIndex, parameter := range operation.Parameters {
			parameterPrefix := fmt.Sprintf("%s parameter %d", prefix, parameterIndex)
			if err := validateCanonicalIdentity(parameterPrefix+" name", parameter.Name, false); err != nil {
				return err
			}
			if err := validateCanonicalIdentity(parameterPrefix+" location", parameter.In, false); err != nil {
				return err
			}
		}
		for statusIndex, status := range operation.ResponseStatuses {
			if err := validateCanonicalIdentity(fmt.Sprintf("%s response status %d", prefix, statusIndex), status, false); err != nil {
				return err
			}
		}
	}
	for schemaIndex, schema := range snapshot.Schemas {
		if err := validateCanonicalIdentity(fmt.Sprintf("contract schema %d name", schemaIndex), schema, false); err != nil {
			return err
		}
	}
	return nil
}

func validateContractSurfaceUniqueness(snapshot ContractSnapshot) error {
	type operationKey struct {
		method string
		path   string
	}
	operations := make(map[operationKey]struct{}, len(snapshot.Operations))
	for operationIndex, operation := range snapshot.Operations {
		key := operationKey{
			method: canonicalUpperSurfaceText(operation.Method),
			path:   strings.TrimSpace(operation.Path),
		}
		if _, ok := operations[key]; ok {
			return fmt.Errorf(
				"contract operation %d duplicates canonical operation %s %s",
				operationIndex,
				key.method,
				key.path,
			)
		}
		operations[key] = struct{}{}

		type parameterKey struct {
			name     string
			location string
		}
		parameters := make(map[parameterKey]struct{}, len(operation.Parameters))
		for parameterIndex, parameter := range operation.Parameters {
			key := parameterKey{
				name:     strings.TrimSpace(parameter.Name),
				location: canonicalLowerSurfaceText(parameter.In),
			}
			if _, ok := parameters[key]; ok {
				return fmt.Errorf(
					"contract operation %d parameter %d duplicates canonical parameter %s in %s",
					operationIndex,
					parameterIndex,
					key.name,
					key.location,
				)
			}
			parameters[key] = struct{}{}
		}

		statuses := make(map[string]struct{}, len(operation.ResponseStatuses))
		for statusIndex, status := range operation.ResponseStatuses {
			status = strings.TrimSpace(status)
			if _, ok := statuses[status]; ok {
				return fmt.Errorf(
					"contract operation %d response status %d duplicates status %s",
					operationIndex,
					statusIndex,
					status,
				)
			}
			statuses[status] = struct{}{}
		}
	}

	schemas := make(map[string]struct{}, len(snapshot.Schemas))
	for schemaIndex, schema := range snapshot.Schemas {
		schema = strings.TrimSpace(schema)
		if _, ok := schemas[schema]; ok {
			return fmt.Errorf("contract schema %d duplicates schema %s", schemaIndex, schema)
		}
		schemas[schema] = struct{}{}
	}
	return nil
}

// ValidateContractSnapshot verifies that snapshot is canonical and that its
// contract digest was derived from its normalized compatibility surface.
func ValidateContractSnapshot(snapshot ContractSnapshot) error {
	_, err := validateAndCloneContractSnapshot(snapshot)
	return err
}

func normalizeSnapshotOperations(operations []ContractOperation) []ContractOperation {
	normalized := make([]ContractOperation, 0, len(operations))
	for _, operation := range operations {
		cloned := ContractOperation{
			Method:              canonicalUpperSurfaceText(operation.Method),
			Path:                strings.TrimSpace(operation.Path),
			RequestBodyRequired: operation.RequestBodyRequired,
		}
		for _, parameter := range operation.Parameters {
			cloned.Parameters = append(cloned.Parameters, ContractParameter{
				Name:     strings.TrimSpace(parameter.Name),
				In:       canonicalLowerSurfaceText(parameter.In),
				Required: parameter.Required,
			})
		}
		sort.Slice(cloned.Parameters, func(i, j int) bool {
			if cloned.Parameters[i].In != cloned.Parameters[j].In {
				return cloned.Parameters[i].In < cloned.Parameters[j].In
			}
			return cloned.Parameters[i].Name < cloned.Parameters[j].Name
		})
		for _, status := range operation.ResponseStatuses {
			status = strings.TrimSpace(status)
			if status != "" {
				cloned.ResponseStatuses = append(cloned.ResponseStatuses, status)
			}
		}
		sort.Strings(cloned.ResponseStatuses)
		normalized = append(normalized, cloned)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Method != normalized[j].Method {
			return normalized[i].Method < normalized[j].Method
		}
		return normalized[i].Path < normalized[j].Path
	})
	return normalized
}

func normalizeSnapshotSchemas(schemas []string) []string {
	normalized := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		schema = strings.TrimSpace(schema)
		if schema != "" {
			normalized = append(normalized, schema)
		}
	}
	sort.Strings(normalized)
	return normalized
}

func contractSurfaceDigest(operations []ContractOperation, schemas []string) (string, error) {
	canonical, err := json.Marshal(struct {
		Operations []ContractOperation `json:"operations"`
		Schemas    []string            `json:"schemas"`
	}{
		Operations: operations,
		Schemas:    schemas,
	})
	if err != nil {
		return "", err
	}
	return sha256Hex(canonical), nil
}

func normalizeContractOperations(operations []Operation) []ContractOperation {
	normalized := make([]ContractOperation, 0, len(operations))
	for _, operation := range operations {
		normalizedOperation := ContractOperation{
			Method: canonicalUpperSurfaceText(operation.Method),
			Path:   strings.TrimSpace(operation.Path),
		}
		if operation.RequestBody != nil {
			normalizedOperation.RequestBodyRequired = operation.RequestBody.Required
		}
		for _, parameter := range operation.Parameters {
			normalizedOperation.Parameters = append(normalizedOperation.Parameters, ContractParameter{
				Name:     strings.TrimSpace(parameter.Name),
				In:       canonicalLowerSurfaceText(parameter.In),
				Required: parameter.Required,
			})
		}
		sort.Slice(normalizedOperation.Parameters, func(i, j int) bool {
			if normalizedOperation.Parameters[i].In != normalizedOperation.Parameters[j].In {
				return normalizedOperation.Parameters[i].In < normalizedOperation.Parameters[j].In
			}
			return normalizedOperation.Parameters[i].Name < normalizedOperation.Parameters[j].Name
		})

		for _, response := range operation.Responses {
			status := strings.TrimSpace(response.Status)
			if status != "" {
				normalizedOperation.ResponseStatuses = append(normalizedOperation.ResponseStatuses, status)
			}
		}
		sort.Strings(normalizedOperation.ResponseStatuses)
		normalized = append(normalized, normalizedOperation)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Method != normalized[j].Method {
			return normalized[i].Method < normalized[j].Method
		}
		return normalized[i].Path < normalized[j].Path
	})
	return normalized
}

func canonicalUpperSurfaceText(value string) string {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) {
		return value
	}
	return strings.ToUpper(value)
}

func canonicalLowerSurfaceText(value string) string {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) {
		return value
	}
	return strings.ToLower(value)
}

func normalizeContractSchemas(schemas []Schema) []string {
	normalized := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if name != "" {
			normalized = append(normalized, name)
		}
	}
	sort.Strings(normalized)
	return normalized
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func isLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
