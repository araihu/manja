package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	canonical, err := json.Marshal(struct {
		Operations []ContractOperation `json:"operations"`
		Schemas    []string            `json:"schemas"`
	}{
		Operations: snapshot.Operations,
		Schemas:    snapshot.Schemas,
	})
	if err != nil {
		panic("marshal normalized contract snapshot: " + err.Error())
	}
	snapshot.ContractDigest = sha256Hex(canonical)
	return snapshot
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
