package core

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
