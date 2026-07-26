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

func TestNewContractSnapshotPreservesIdentityForValidation(t *testing.T) {
	for _, test := range []struct {
		name       string
		contractID string
		revisionID string
	}{
		{name: "contract padding", contractID: " payments ", revisionID: "revision-1"},
		{name: "revision padding", contractID: "payments", revisionID: " revision-1 "},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := NewContractSnapshot(test.contractID, test.revisionID, []byte("{}"), SpecIndex{})
			if snapshot.ContractID != test.contractID || snapshot.RevisionID != test.revisionID {
				t.Fatalf("snapshot identities = %q/%q, want raw %q/%q", snapshot.ContractID, snapshot.RevisionID, test.contractID, test.revisionID)
			}
			if err := ValidateContractSnapshot(snapshot); err == nil {
				t.Fatal("ValidateContractSnapshot accepted a normalized caller identity")
			}
		})
	}
}

func TestValidateContractSnapshotRejectsNonCanonicalSurfaceIdentity(t *testing.T) {
	for _, test := range []struct {
		name  string
		index SpecIndex
	}{
		{
			name:  "operation method",
			index: SpecIndex{Operations: []Operation{{Method: "GET\x00shadow", Path: "/payments"}}},
		},
		{
			name:  "operation path",
			index: SpecIndex{Operations: []Operation{{Method: "GET", Path: "/payments-\xff"}}},
		},
		{
			name: "parameter name",
			index: SpecIndex{Operations: []Operation{{
				Method: "GET", Path: "/payments",
				Parameters: []OperationParameter{{Name: "account\x00shadow", In: "query"}},
			}}},
		},
		{
			name: "parameter location",
			index: SpecIndex{Operations: []Operation{{
				Method: "GET", Path: "/payments",
				Parameters: []OperationParameter{{Name: "account", In: "query-\xff"}},
			}}},
		},
		{
			name: "response status",
			index: SpecIndex{Operations: []Operation{{
				Method: "GET", Path: "/payments",
				Responses: []OperationResponse{{Status: "200\x00shadow"}},
			}}},
		},
		{name: "schema", index: SpecIndex{Schemas: []Schema{{Name: "Payment-\xff"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := NewContractSnapshot("payments", "revision-1", []byte("spec"), test.index)
			if err := ValidateContractSnapshot(snapshot); err == nil {
				t.Fatal("ValidateContractSnapshot accepted non-canonical contract surface identity")
			}
		})
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
