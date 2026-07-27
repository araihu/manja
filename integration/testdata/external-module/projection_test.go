package extension_test

import (
	"context"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestProjectionPublicContractIsUsableByUnrelatedModule(t *testing.T) {
	document, err := (projection.Builder{}).Build(context.Background(), domain.SpecIndex{
		ProjectID:    "payments",
		RevisionID:   "rev-0001",
		Title:        "Payments API",
		Operations:   []domain.Operation{},
		Schemas:      []domain.Schema{},
		Search:       []domain.SearchDocument{},
		PublicRoutes: []domain.PublicRoute{},
	})
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}
	if document.FormatVersion != 2 {
		t.Fatalf("format version = %d, want 2", document.FormatVersion)
	}
	if document.SchemaNodes == nil || len(document.SchemaNodes) != 0 {
		t.Fatalf("schema nodes = %#v, want non-nil empty", document.SchemaNodes)
	}
	if document.ProjectID != "payments" || document.RevisionID != "rev-0001" {
		t.Fatalf("identity = %q/%q, want payments/rev-0001", document.ProjectID, document.RevisionID)
	}
	if document.Overview.Anchor != "overview" || document.Overview.Href != "?selected=overview#overview" {
		t.Fatalf("overview navigation = %q %q", document.Overview.Anchor, document.Overview.Href)
	}
	if document.MainLandmark.ID != "main-content" || document.MainLandmark.Role != "main" {
		t.Fatalf("main landmark = %#v", document.MainLandmark)
	}
}
