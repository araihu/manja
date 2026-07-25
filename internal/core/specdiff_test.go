package core

import "testing"

func TestDiffSpecIndexesFlagsBreakingAndAdditiveContractChanges(t *testing.T) {
	baseline := SpecIndex{
		RevisionID: "rev-live",
		Operations: []Operation{
			{
				Method: "GET",
				Path:   "/customers",
				Responses: []OperationResponse{
					{Status: "200"},
				},
			},
			{
				Method: "GET",
				Path:   "/payments",
				Parameters: []OperationParameter{
					{Name: "expand", In: "query"},
				},
				Responses: []OperationResponse{
					{Status: "200"},
					{Status: "404"},
				},
			},
		},
		Schemas: []Schema{{Name: "Customer"}, {Name: "Payment"}},
	}
	candidate := SpecIndex{
		RevisionID: "rev-candidate",
		Operations: []Operation{
			{
				Method: "GET",
				Path:   "/payments",
				Parameters: []OperationParameter{
					{Name: "expand", In: "query", Required: true},
					{Name: "version", In: "query", Required: true},
				},
				Responses: []OperationResponse{
					{Status: "200"},
					{Status: "202"},
				},
			},
			{
				Method: "POST",
				Path:   "/payments",
			},
		},
		Schemas: []Schema{{Name: "Payment"}, {Name: "Refund"}},
	}

	diff := DiffSpecIndexes(baseline, candidate)

	if diff.BaselineRevisionID != "rev-live" || diff.CandidateRevisionID != "rev-candidate" {
		t.Fatalf("revision ids = %#v", diff)
	}
	for _, want := range []string{
		"Removed endpoint|GET /customers",
		"Parameter became required|GET /payments expand (query)",
		"Required parameter added|GET /payments version (query)",
		"Response status removed|GET /payments 404",
		"Removed schema|Customer",
	} {
		if !specChangesContain(diff.BreakingChanges, want) {
			t.Fatalf("breaking changes missing %q: %#v", want, diff.BreakingChanges)
		}
	}
	for _, want := range []string{
		"Added endpoint|POST /payments",
		"Response status added|GET /payments 202",
		"Added schema|Refund",
	} {
		if !specChangesContain(diff.AdditiveChanges, want) {
			t.Fatalf("additive changes missing %q: %#v", want, diff.AdditiveChanges)
		}
	}
}

func TestDiffSpecIndexesTreatsMatchingContractAsSafe(t *testing.T) {
	idx := SpecIndex{
		RevisionID: "rev",
		Operations: []Operation{{
			Method:    "GET",
			Path:      "/payments",
			Responses: []OperationResponse{{Status: "200"}},
		}},
		Schemas: []Schema{{Name: "Payment"}},
	}

	diff := DiffSpecIndexes(idx, idx)

	if len(diff.BreakingChanges) != 0 || len(diff.AdditiveChanges) != 0 {
		t.Fatalf("diff should be empty: %#v", diff)
	}
}

func specChangesContain(changes []SpecChange, want string) bool {
	for _, change := range changes {
		if change.Kind+"|"+change.Subject == want {
			return true
		}
	}
	return false
}
