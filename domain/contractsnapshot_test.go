package domain

import "testing"

func TestNewContractSnapshotNormalizesOrderAndDigestsContent(t *testing.T) {
	left := SpecIndex{
		RevisionID: "target",
		Operations: []Operation{
			{Method: "POST", Path: "/payments", Responses: []OperationResponse{{Status: "201"}, {Status: "400"}}},
			{Method: "GET", Path: "/payments", Parameters: []OperationParameter{{Name: "limit", In: "query"}}},
		},
		Schemas: []Schema{{Name: "Payment"}, {Name: "Error"}},
	}
	right := SpecIndex{
		RevisionID: "target",
		Operations: []Operation{
			{Method: "GET", Path: "/payments", Parameters: []OperationParameter{{Name: "limit", In: "query"}}},
			{Method: "POST", Path: "/payments", Responses: []OperationResponse{{Status: "400"}, {Status: "201"}}},
		},
		Schemas: []Schema{{Name: "Error"}, {Name: "Payment"}},
	}

	a := NewContractSnapshot("payments", "target", []byte("raw-a"), left)
	b := NewContractSnapshot("payments", "target", []byte("raw-a"), right)

	if a.ContractDigest == "" || a.ContractDigest != b.ContractDigest {
		t.Fatalf("contract digests differ: %q != %q", a.ContractDigest, b.ContractDigest)
	}
	if a.SpecDigest == "" || a.SpecDigest != b.SpecDigest {
		t.Fatalf("spec digests differ: %q != %q", a.SpecDigest, b.SpecDigest)
	}
}

func TestValidateAndCloneContractSnapshotDeepCopiesSurface(t *testing.T) {
	original := NewContractSnapshot("payments", "target", []byte("target"), SpecIndex{
		Operations: []Operation{{
			Method: "GET",
			Path:   "/payments",
			Parameters: []OperationParameter{{
				Name: "limit", In: "query",
			}},
			Responses: []OperationResponse{{Status: "200"}},
		}},
		Schemas: []Schema{{Name: "Payment"}},
	})

	cloned, err := validateAndCloneContractSnapshot(original)
	if err != nil {
		t.Fatal(err)
	}
	original.Operations[0].Parameters[0].Name = "mutated"
	original.Operations[0].ResponseStatuses[0] = "599"
	original.Schemas[0] = "Mutated"

	if got := cloned.Operations[0].Parameters[0].Name; got != "limit" {
		t.Fatalf("cloned parameter name = %q", got)
	}
	if got := cloned.Operations[0].ResponseStatuses[0]; got != "200" {
		t.Fatalf("cloned response status = %q", got)
	}
	if got := cloned.Schemas[0]; got != "Payment" {
		t.Fatalf("cloned schema = %q", got)
	}
}
