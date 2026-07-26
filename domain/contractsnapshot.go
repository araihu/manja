package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
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
		ContractID: strings.TrimSpace(contractID),
		RevisionID: strings.TrimSpace(revisionID),
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
	if strings.TrimSpace(snapshot.ContractID) == "" {
		return ContractSnapshot{}, fmt.Errorf("contract id is required")
	}
	if snapshot.ContractID != strings.TrimSpace(snapshot.ContractID) {
		return ContractSnapshot{}, fmt.Errorf("contract id must be normalized")
	}
	if strings.TrimSpace(snapshot.RevisionID) == "" {
		return ContractSnapshot{}, fmt.Errorf("revision id is required")
	}
	if snapshot.RevisionID != strings.TrimSpace(snapshot.RevisionID) {
		return ContractSnapshot{}, fmt.Errorf("revision id must be normalized")
	}
	if !isLowerSHA256(snapshot.SpecDigest) {
		return ContractSnapshot{}, fmt.Errorf("spec digest must be lowercase SHA-256")
	}
	if !isLowerSHA256(snapshot.ContractDigest) {
		return ContractSnapshot{}, fmt.Errorf("contract digest must be lowercase SHA-256")
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
			Method:              strings.ToUpper(strings.TrimSpace(operation.Method)),
			Path:                strings.TrimSpace(operation.Path),
			RequestBodyRequired: operation.RequestBodyRequired,
		}
		for _, parameter := range operation.Parameters {
			cloned.Parameters = append(cloned.Parameters, ContractParameter{
				Name:     strings.TrimSpace(parameter.Name),
				In:       strings.ToLower(strings.TrimSpace(parameter.In)),
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
			Method: strings.ToUpper(strings.TrimSpace(operation.Method)),
			Path:   strings.TrimSpace(operation.Path),
		}
		if operation.RequestBody != nil {
			normalizedOperation.RequestBodyRequired = operation.RequestBody.Required
		}
		for _, parameter := range operation.Parameters {
			normalizedOperation.Parameters = append(normalizedOperation.Parameters, ContractParameter{
				Name:     strings.TrimSpace(parameter.Name),
				In:       strings.ToLower(strings.TrimSpace(parameter.In)),
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
