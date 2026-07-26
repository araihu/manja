package domain

import "testing"

func TestPublicDiffAPIsReturnDeterministicValidationErrors(t *testing.T) {
	baselineIndex := SpecIndex{
		RevisionID: "baseline",
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	}
	candidateForward := SpecIndex{
		RevisionID: "candidate",
		Operations: []Operation{
			{Method: "GET", Path: "/payments"},
			{Method: "get", Path: "/payments", Description: "duplicate"},
		},
	}
	candidateReverse := candidateForward
	candidateReverse.Operations = append([]Operation(nil), candidateForward.Operations...)
	candidateReverse.Operations[0], candidateReverse.Operations[1] =
		candidateReverse.Operations[1], candidateReverse.Operations[0]

	baseline := NewContractSnapshot(
		"payments",
		baselineIndex.RevisionID,
		[]byte("baseline"),
		baselineIndex,
	)
	var firstIndexError, firstSnapshotError string
	for index, candidateIndex := range []SpecIndex{candidateForward, candidateReverse} {
		_, indexErr := DiffSpecIndexes(baselineIndex, candidateIndex)
		if indexErr == nil {
			t.Fatal("public index diff accepted a canonical duplicate")
		}
		candidate := NewContractSnapshot(
			"payments",
			candidateIndex.RevisionID,
			[]byte("candidate"),
			candidateIndex,
		)
		_, snapshotErr := DiffContractSnapshots(baseline, candidate)
		if snapshotErr == nil {
			t.Fatal("public snapshot diff accepted a canonical duplicate")
		}
		if index == 0 {
			firstIndexError = indexErr.Error()
			firstSnapshotError = snapshotErr.Error()
			continue
		}
		if indexErr.Error() != firstIndexError {
			t.Fatalf("reversed index duplicate error = %q, want %q", indexErr, firstIndexError)
		}
		if snapshotErr.Error() != firstSnapshotError {
			t.Fatalf("reversed snapshot duplicate error = %q, want %q", snapshotErr, firstSnapshotError)
		}
	}
}

func TestDiffContractSnapshotsUsesStableRuleAndFindingIDs(t *testing.T) {
	baseline := NewContractSnapshot("payments", "base", []byte("base"), SpecIndex{
		Operations: []Operation{{Method: "GET", Path: "/payments"}},
	})
	candidate := NewContractSnapshot("payments", "head", []byte("head"), SpecIndex{})

	first, err := DiffContractSnapshots(baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DiffContractSnapshots(baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}

	change := first.BreakingChanges[0]
	if change.RuleID != RuleOperationRemoved {
		t.Fatalf("rule = %q", change.RuleID)
	}
	if change.ID == "" || change.ID != second.BreakingChanges[0].ID {
		t.Fatalf("finding ids are not stable: %#v %#v", first, second)
	}
}

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

	diff, err := DiffSpecIndexes(baseline, candidate)
	if err != nil {
		t.Fatal(err)
	}

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

	diff, err := DiffSpecIndexes(idx, idx)
	if err != nil {
		t.Fatal(err)
	}

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
