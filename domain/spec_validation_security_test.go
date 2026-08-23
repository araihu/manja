package domain

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestValidateSpecIndexRejectsCanonicalSurfaceCollisionsInEitherOrder(t *testing.T) {
	tests := []struct {
		name    string
		index   SpecIndex
		reverse func(*SpecIndex)
	}{
		{
			name: "operation method and path",
			index: SpecIndex{Operations: []Operation{
				{Method: "GET", Path: "/payments"},
				{Method: "get", Path: "/payments"},
			}},
			reverse: func(index *SpecIndex) {
				index.Operations[0], index.Operations[1] = index.Operations[1], index.Operations[0]
			},
		},
		{
			name: "parameter name and canonical location",
			index: SpecIndex{Operations: []Operation{{
				Method: "GET", Path: "/payments",
				Parameters: []OperationParameter{
					{Name: "limit", In: "query"},
					{Name: "limit", In: "QUERY"},
				},
			}}},
			reverse: func(index *SpecIndex) {
				parameters := index.Operations[0].Parameters
				parameters[0], parameters[1] = parameters[1], parameters[0]
			},
		},
		{
			name: "response status",
			index: SpecIndex{Operations: []Operation{{
				Method: "GET", Path: "/payments",
				Responses: []OperationResponse{
					{Status: "200", Description: "first"},
					{Status: "200", Description: "second"},
				},
			}}},
			reverse: func(index *SpecIndex) {
				responses := index.Operations[0].Responses
				responses[0], responses[1] = responses[1], responses[0]
			},
		},
		{
			name: "schema exact name",
			index: SpecIndex{Schemas: []Schema{
				{Name: "Payment", Description: "first"},
				{Name: "Payment", Description: "second"},
			}},
			reverse: func(index *SpecIndex) {
				index.Schemas[0], index.Schemas[1] = index.Schemas[1], index.Schemas[0]
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, order := range []string{"forward", "reverse"} {
				t.Run(order, func(t *testing.T) {
					index := test.index
					index.Operations = append([]Operation(nil), test.index.Operations...)
					for operationIndex := range index.Operations {
						index.Operations[operationIndex].Parameters = append(
							[]OperationParameter(nil),
							test.index.Operations[operationIndex].Parameters...,
						)
						index.Operations[operationIndex].Responses = append(
							[]OperationResponse(nil),
							test.index.Operations[operationIndex].Responses...,
						)
					}
					index.Schemas = append([]Schema(nil), test.index.Schemas...)
					if order == "reverse" {
						test.reverse(&index)
					}
					if err := ValidateSpecIndex(index); err == nil {
						t.Error("ValidateSpecIndex accepted a canonical surface collision")
					}
					snapshot := NewContractSnapshot("payments", "revision-next", []byte("spec"), index)
					if err := ValidateContractSnapshot(snapshot); err == nil {
						t.Error("ValidateContractSnapshot accepted a canonical surface collision")
					}
				})
			}
		})
	}
}

func TestValidateSpecIndexKeepsSchemaNamesCaseSensitive(t *testing.T) {
	index := SpecIndex{Schemas: []Schema{{Name: "Payment"}, {Name: "payment"}}}
	if err := ValidateSpecIndex(index); err != nil {
		t.Fatalf("case-distinct schema names: %v", err)
	}
	if err := ValidateContractSnapshot(
		NewContractSnapshot("payments", "revision-next", []byte("spec"), index),
	); err != nil {
		t.Fatalf("case-distinct snapshot schema names: %v", err)
	}
}

func TestEvaluateReviewRejectsDuplicateCanonicalSnapshotSurface(t *testing.T) {
	policy, err := MergePolicy(PolicyLayer{Name: "stable", Source: PolicySourceRepository})
	if err != nil {
		t.Fatal(err)
	}
	baseline := NewContractSnapshot("payments", "revision-good", []byte("baseline"), SpecIndex{})
	candidate := NewContractSnapshot("payments", "revision-next", []byte("candidate"), SpecIndex{
		Operations: []Operation{
			{Method: "GET", Path: "/payments"},
			{Method: "get", Path: "/payments"},
		},
	})
	if _, err := EvaluateReview(ReviewRequest{
		ContractID: "payments", Target: baseline, Candidate: candidate,
		Policy: policy, EvaluatedAt: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		EngineVersion: "test",
	}); err == nil {
		t.Fatal("EvaluateReview accepted duplicate canonical candidate surface")
	}
}

func TestValidateSpecIndexRejectsSchemaSummaryCyclesWithoutHanging(t *testing.T) {
	if os.Getenv("MANJA_SCHEMA_CYCLE_HELPER") == "1" {
		summary := &SchemaSummary{Name: "Node"}
		summary.Items = summary
		if err := ValidateSpecIndex(SpecIndex{
			Schemas: []Schema{{Name: "Node", Summary: *summary}},
		}); err == nil {
			os.Exit(2)
		}
		return
	}

	// The subprocess should return immediately once validation is cycle-safe.
	// Five seconds leaves startup headroom for -race while still detecting the
	// former unbounded recursion deterministically.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestValidateSpecIndexRejectsSchemaSummaryCyclesWithoutHanging$",
	)
	command.Env = append(os.Environ(), "MANJA_SCHEMA_CYCLE_HELPER=1")
	if output, err := command.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			t.Fatalf("ValidateSpecIndex hung on a schema cycle: %v", ctx.Err())
		}
		t.Fatalf("schema cycle helper failed: %v\n%s", err, output)
	}
}

func TestValidateSpecIndexBoundsSchemaSummaryDepth(t *testing.T) {
	for _, test := range []struct {
		nodes     int
		wantError bool
	}{
		{nodes: 63},
		{nodes: 64},
		{nodes: 65, wantError: true},
	} {
		t.Run(fmt.Sprintf("%d_nodes", test.nodes), func(t *testing.T) {
			err := ValidateSpecIndex(SpecIndex{
				Schemas: []Schema{{Name: "Root", Summary: schemaSummaryChain(test.nodes)}},
			})
			if test.wantError && err == nil {
				t.Fatal("ValidateSpecIndex accepted an over-depth schema summary")
			}
			if !test.wantError && err != nil {
				t.Fatalf("ValidateSpecIndex rejected a bounded schema summary: %v", err)
			}
		})
	}
}

func TestValidateSpecIndexBoundsTotalSchemaSummaryNodes(t *testing.T) {
	validator := specSchemaSummaryValidator{
		active:         make(map[*SchemaSummary]struct{}),
		memoHeight:     make(map[*SchemaSummary]int),
		nodes:          maxSpecSchemaSummaryNodes - 1,
		resourceLimits: true,
	}
	if err := validator.validateRoot("boundary", SchemaSummary{}); err != nil {
		t.Fatalf("validator rejected aggregate node boundary: %v", err)
	}
	if validator.nodes != maxSpecSchemaSummaryNodes {
		t.Fatalf("aggregate nodes = %d, want %d", validator.nodes, maxSpecSchemaSummaryNodes)
	}
	if err := validator.validateRoot("over boundary", SchemaSummary{}); err == nil {
		t.Fatal("validator accepted an aggregate node beyond the hard budget")
	}
}

func TestValidateSpecIndexAllowsSharedAcyclicSchemaSubtrees(t *testing.T) {
	shared := &SchemaSummary{Name: "Shared", Type: "string"}
	index := SpecIndex{Schemas: []Schema{
		{Name: "First", Summary: SchemaSummary{Items: shared}},
		{Name: "Second", Summary: SchemaSummary{Items: shared}},
	}}
	if err := ValidateSpecIndex(index); err != nil {
		t.Fatalf("shared acyclic schema subtree: %v", err)
	}
}

func schemaSummaryChain(nodes int) SchemaSummary {
	if nodes <= 0 {
		return SchemaSummary{}
	}
	root := SchemaSummary{Name: "Node0"}
	current := &root
	for index := 1; index < nodes; index++ {
		current.Items = &SchemaSummary{Name: fmt.Sprintf("Node%d", index)}
		current = current.Items
	}
	return root
}
