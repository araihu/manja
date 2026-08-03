package catalog

import "fmt"

type Bounds struct {
	Catalogs               uint64 `json:"catalogs"`
	Documents              uint64 `json:"documents"`
	Operations             uint64 `json:"operations"`
	Schemas                uint64 `json:"schemas"`
	SourceBytes            uint64 `json:"sourceBytes"`
	KubernetesSourceBytes  uint64 `json:"kubernetesSourceBytes"`
	SourceDocumentBytes    uint64 `json:"sourceDocumentBytes"`
	DirectoryBytes         uint64 `json:"directoryBytes"`
	SearchBytes            uint64 `json:"searchBytes"`
	ProjectionBytes        uint64 `json:"projectionBytes"`
	SnapshotBytes          uint64 `json:"snapshotBytes"`
	Children               uint64 `json:"children"`
	DetailShardRecords     uint64 `json:"detailShardRecords"`
	DetailShardBytes       uint64 `json:"detailShardBytes"`
	SchemaNodeShardRecords uint64 `json:"schemaNodeShardRecords"`
	SchemaNodeShardBytes   uint64 `json:"schemaNodeShardBytes"`
	PostingSegmentBytes    uint64 `json:"postingSegmentBytes"`
	StartupCatalogBytes    uint64 `json:"startupCatalogBytes"`
	StartupProcessBytes    uint64 `json:"startupProcessBytes"`
	StoredBytes            uint64 `json:"storedBytes"`
	StagingBytes           uint64 `json:"stagingBytes"`
}

type BudgetUsage struct {
	Catalogs               uint64
	Documents              uint64
	Operations             uint64
	Schemas                uint64
	SourceBytes            uint64
	Kubernetes             bool
	SourceDocumentBytes    uint64
	DirectoryBytes         uint64
	SearchBytes            uint64
	ProjectionBytes        uint64
	SnapshotBytes          uint64
	Children               uint64
	DetailShardRecords     uint64
	DetailShardBytes       uint64
	SchemaNodeShardRecords uint64
	SchemaNodeShardBytes   uint64
	PostingSegmentBytes    uint64
	StartupCatalogBytes    uint64
	StartupProcessBytes    uint64
	StoredBytes            uint64
	StagingBytes           uint64
}

func DefaultBounds() Bounds {
	return Bounds{
		Catalogs: 8, Documents: 256, Operations: 20_000, Schemas: 20_000,
		SourceBytes: 64 << 20, KubernetesSourceBytes: 16 << 20, SourceDocumentBytes: 8 << 20,
		DirectoryBytes: 4 << 20, SearchBytes: 4 << 20, ProjectionBytes: 32 << 20, SnapshotBytes: 64 << 20,
		Children: 1024, DetailShardRecords: 256, DetailShardBytes: 2 << 20,
		SchemaNodeShardRecords: 512, SchemaNodeShardBytes: 2 << 20, PostingSegmentBytes: 256 << 10,
		StartupCatalogBytes: 8 << 20, StartupProcessBytes: 64 << 20,
		StoredBytes: 512 << 20, StagingBytes: 128 << 20,
	}
}

func (bounds Bounds) Validate(usage BudgetUsage) error {
	checks := []struct {
		name        string
		used, limit uint64
	}{
		{name: "catalogs", used: usage.Catalogs, limit: bounds.Catalogs},
		{name: "documents", used: usage.Documents, limit: bounds.Documents},
		{name: "operations", used: usage.Operations, limit: bounds.Operations},
		{name: "schemas", used: usage.Schemas, limit: bounds.Schemas},
		{name: "source document", used: usage.SourceDocumentBytes, limit: bounds.SourceDocumentBytes},
		{name: "directory", used: usage.DirectoryBytes, limit: bounds.DirectoryBytes},
		{name: "search", used: usage.SearchBytes, limit: bounds.SearchBytes},
		{name: "projection", used: usage.ProjectionBytes, limit: bounds.ProjectionBytes},
		{name: "snapshot", used: usage.SnapshotBytes, limit: bounds.SnapshotBytes},
		{name: "children", used: usage.Children, limit: bounds.Children},
		{name: "detail records", used: usage.DetailShardRecords, limit: bounds.DetailShardRecords},
		{name: "detail bytes", used: usage.DetailShardBytes, limit: bounds.DetailShardBytes},
		{name: "schema node records", used: usage.SchemaNodeShardRecords, limit: bounds.SchemaNodeShardRecords},
		{name: "schema node bytes", used: usage.SchemaNodeShardBytes, limit: bounds.SchemaNodeShardBytes},
		{name: "posting segment", used: usage.PostingSegmentBytes, limit: bounds.PostingSegmentBytes},
		{name: "startup catalog", used: usage.StartupCatalogBytes, limit: bounds.StartupCatalogBytes},
		{name: "startup process", used: usage.StartupProcessBytes, limit: bounds.StartupProcessBytes},
		{name: "stored", used: usage.StoredBytes, limit: bounds.StoredBytes},
		{name: "staging", used: usage.StagingBytes, limit: bounds.StagingBytes},
	}
	if usage.Kubernetes {
		checks = append(checks, struct {
			name        string
			used, limit uint64
		}{name: "Kubernetes source", used: usage.SourceBytes, limit: bounds.KubernetesSourceBytes})
	} else {
		checks = append(checks, struct {
			name        string
			used, limit uint64
		}{name: "source", used: usage.SourceBytes, limit: bounds.SourceBytes})
	}
	for _, check := range checks {
		if check.limit == 0 {
			return fmt.Errorf("%s budget is zero", check.name)
		}
		if check.used > check.limit {
			return fmt.Errorf("%s usage %d exceeds %d", check.name, check.used, check.limit)
		}
	}
	return nil
}
