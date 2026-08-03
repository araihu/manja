package catalog

import (
	"strings"
	"testing"
)

func TestBoundsAcceptEveryLimitAndRejectOneOver(t *testing.T) {
	t.Parallel()

	bounds := DefaultBounds()
	tests := []struct {
		name  string
		limit uint64
		set   func(*BudgetUsage, uint64)
	}{
		{name: "catalogs", limit: bounds.Catalogs, set: func(value *BudgetUsage, count uint64) { value.Catalogs = count }},
		{name: "documents", limit: bounds.Documents, set: func(value *BudgetUsage, count uint64) { value.Documents = count }},
		{name: "operations", limit: bounds.Operations, set: func(value *BudgetUsage, count uint64) { value.Operations = count }},
		{name: "schemas", limit: bounds.Schemas, set: func(value *BudgetUsage, count uint64) { value.Schemas = count }},
		{name: "Kubernetes source", limit: bounds.KubernetesSourceBytes, set: func(value *BudgetUsage, count uint64) { value.SourceBytes = count; value.Kubernetes = true }},
		{name: "source document", limit: bounds.SourceDocumentBytes, set: func(value *BudgetUsage, count uint64) { value.SourceDocumentBytes = count }},
		{name: "directory", limit: bounds.DirectoryBytes, set: func(value *BudgetUsage, count uint64) { value.DirectoryBytes = count }},
		{name: "search", limit: bounds.SearchBytes, set: func(value *BudgetUsage, count uint64) { value.SearchBytes = count }},
		{name: "projection", limit: bounds.ProjectionBytes, set: func(value *BudgetUsage, count uint64) { value.ProjectionBytes = count }},
		{name: "snapshot", limit: bounds.SnapshotBytes, set: func(value *BudgetUsage, count uint64) { value.SnapshotBytes = count }},
		{name: "children", limit: bounds.Children, set: func(value *BudgetUsage, count uint64) { value.Children = count }},
		{name: "detail records", limit: bounds.DetailShardRecords, set: func(value *BudgetUsage, count uint64) { value.DetailShardRecords = count }},
		{name: "detail bytes", limit: bounds.DetailShardBytes, set: func(value *BudgetUsage, count uint64) { value.DetailShardBytes = count }},
		{name: "schema node records", limit: bounds.SchemaNodeShardRecords, set: func(value *BudgetUsage, count uint64) { value.SchemaNodeShardRecords = count }},
		{name: "schema node bytes", limit: bounds.SchemaNodeShardBytes, set: func(value *BudgetUsage, count uint64) { value.SchemaNodeShardBytes = count }},
		{name: "posting segment", limit: bounds.PostingSegmentBytes, set: func(value *BudgetUsage, count uint64) { value.PostingSegmentBytes = count }},
		{name: "startup catalog", limit: bounds.StartupCatalogBytes, set: func(value *BudgetUsage, count uint64) { value.StartupCatalogBytes = count }},
		{name: "startup process", limit: bounds.StartupProcessBytes, set: func(value *BudgetUsage, count uint64) { value.StartupProcessBytes = count }},
		{name: "stored", limit: bounds.StoredBytes, set: func(value *BudgetUsage, count uint64) { value.StoredBytes = count }},
		{name: "staging", limit: bounds.StagingBytes, set: func(value *BudgetUsage, count uint64) { value.StagingBytes = count }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage := BudgetUsage{}
			test.set(&usage, test.limit)
			if err := bounds.Validate(usage); err != nil {
				t.Fatalf("limit rejected: %v", err)
			}
			test.set(&usage, test.limit+1)
			if err := bounds.Validate(usage); err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("one over error = %v", err)
			}
		})
	}
}
