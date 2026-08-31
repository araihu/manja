package searchv2

import (
	"bytes"
	"strings"
	"testing"
)

func TestSemanticRunEncodingIsDeterministicAcrossInputOrder(t *testing.T) {
	identity, err := NewBuildIdentity("manja-v2.0.0+abc123", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	records := semanticSpecRecords(t, "pets")
	forward, err := NewSemanticRun("pets", identity, records)
	if err != nil {
		t.Fatal(err)
	}
	reverseInput := []SemanticRecord{records[2], records[1], records[0]}
	reverse, err := NewSemanticRun("pets", identity, reverseInput)
	if err != nil {
		t.Fatal(err)
	}
	forwardBytes, err := forward.MarshalCanonical()
	if err != nil {
		t.Fatal(err)
	}
	reverseBytes, err := reverse.MarshalCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(forwardBytes, reverseBytes) {
		t.Fatalf("canonical encoding changed with enumeration order:\n%s\n%s", forwardBytes, reverseBytes)
	}
	if forward.BuildKey != reverse.BuildKey {
		t.Fatalf("build keys differ: %q != %q", forward.BuildKey, reverse.BuildKey)
	}
	if got := []RecordKind{forward.Records[0].Kind, forward.Records[1].Kind, forward.Records[2].Kind}; got[0] != KindSpec || got[1] != KindOperation || got[2] != KindSchema {
		t.Fatalf("canonical kind ordering = %v", got)
	}
}

func TestSemanticRunBuildKeyCapturesProducerProjectionAndInputs(t *testing.T) {
	records := semanticSpecRecords(t, "pets")
	baseIdentity, _ := NewBuildIdentity("manja-v2", strings.Repeat("a", 64))
	producerIdentity, _ := NewBuildIdentity("manja-v3", strings.Repeat("a", 64))
	projectionIdentity, _ := NewBuildIdentity("manja-v2", strings.Repeat("b", 64))
	base, _ := NewSemanticRun("pets", baseIdentity, records)
	producer, _ := NewSemanticRun("pets", producerIdentity, records)
	projection, _ := NewSemanticRun("pets", projectionIdentity, records)

	changed := append([]SemanticRecord(nil), records...)
	changed[2].Description = "Changed schema description"
	inputs, err := NewSemanticRun("pets", baseIdentity, changed)
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]SemanticRun{
		"producer": producer, "projection": projection, "inputs": inputs,
	} {
		if candidate.BuildKey == base.BuildKey {
			t.Fatalf("%s change did not invalidate build key", name)
		}
	}
}

func TestSemanticRunRejectsTamperedRecordIdentity(t *testing.T) {
	identity, _ := NewBuildIdentity("manja-v2", strings.Repeat("a", 64))
	records := semanticSpecRecords(t, "pets")
	records[0].ID = SemanticRecordID("search-semantic-sha256-" + strings.Repeat("0", 64))
	if _, err := NewSemanticRun("pets", identity, records); err == nil {
		t.Fatal("expected tampered semantic record identity to fail")
	}
}

func semanticSpecRecords(t *testing.T, specKey string) []SemanticRecord {
	t.Helper()
	common := SemanticRecordInput{SpecKey: specKey, SpecTitle: "Pets"}
	spec := common
	spec.Kind, spec.LogicalKey, spec.Title = KindSpec, specKey, "Pets"
	operation := common
	operation.Kind, operation.LogicalKey, operation.Title = KindOperation, "GET /pets", "List pets"
	operation.Method, operation.Path = "GET", "/pets"
	schema := common
	schema.Kind, schema.LogicalKey, schema.Title = KindSchema, "#/components/schemas/Pet", "Pet"
	schema.Description = "A pet"
	return []SemanticRecord{
		mustSemanticRecord(t, schema),
		mustSemanticRecord(t, spec),
		mustSemanticRecord(t, operation),
	}
}

func mustSemanticRecord(t *testing.T, input SemanticRecordInput) SemanticRecord {
	t.Helper()
	record, err := NewSemanticRecord(input)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
