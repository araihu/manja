package openapi

import (
	"context"
	"os"
	"testing"

	"github.com/araihu/manja/internal/core"
)

func TestSnapshotBuilderBuildsNormalizedOpenAPIContract(t *testing.T) {
	data, err := os.ReadFile("testdata/review/candidate.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := (SnapshotBuilder{}).Build(context.Background(), "payments",
		core.SpecFile{Path: "candidate.yaml", Format: "yaml", Bytes: data},
		core.Revision{ID: "candidate"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContractID != "payments" || got.RevisionID != "candidate" {
		t.Fatalf("snapshot = %#v", got)
	}
	if got.SpecDigest == "" || got.ContractDigest == "" || len(got.Operations) != 1 {
		t.Fatalf("snapshot = %#v", got)
	}
}
