package render

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
)

func TestPreparedCatalogOverviewMetricsRendersLegacyMetricMarkup(t *testing.T) {
	directory := catalog.CatalogArtifactV1{
		CatalogID: "kubernetes",
		Title:     "Kubernetes",
		Documents: []catalog.DocumentDirectoryV1{
			{Key: "core-v1", Operations: make([]catalog.OperationDirectoryV1, 2), Schemas: make([]catalog.SchemaDirectoryV1, 1)},
			{Key: "apps-v1", Operations: make([]catalog.OperationDirectoryV1, 1), Schemas: make([]catalog.SchemaDirectoryV1, 3)},
		},
	}
	fragment, err := PrepareCatalogOverviewMetrics(directory)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := `<div class="grid gap-4 sm:grid-cols-3"><div class="rounded-radius border border-outline bg-surface p-5 dark:border-outline-dark dark:bg-surface-dark"><p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Documents</p><p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">2</p></div><div class="rounded-radius border border-outline bg-surface p-5 dark:border-outline-dark dark:bg-surface-dark"><p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Operations</p><p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">3</p></div><div class="rounded-radius border border-outline bg-surface p-5 dark:border-outline-dark dark:bg-surface-dark"><p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Schemas</p><p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">4</p></div></div>`
	if string(body) != want {
		t.Fatalf("prepared catalog overview metrics drifted from legacy bytes:\nwant=%s\n got=%s", want, body)
	}
	if strings.Count(string(body), `class="grid gap-4 sm:grid-cols-3"`) != 1 {
		t.Fatalf("prepared catalog overview metrics rendered unexpected wrappers: %s", body)
	}
}

func TestPrepareCatalogOverviewMetricsFailsClosedAndCopiesCounts(t *testing.T) {
	base := catalog.CatalogArtifactV1{
		CatalogID: "kubernetes",
		Title:     "Kubernetes",
		Documents: []catalog.DocumentDirectoryV1{
			{Key: "core-v1", Operations: make([]catalog.OperationDirectoryV1, 2), Schemas: make([]catalog.SchemaDirectoryV1, 1)},
			{Key: "apps-v1", Operations: make([]catalog.OperationDirectoryV1, 1), Schemas: make([]catalog.SchemaDirectoryV1, 3)},
		},
	}
	for _, test := range []struct {
		name   string
		mutate func(*catalog.CatalogArtifactV1)
	}{
		{name: "invalid catalog id", mutate: func(directory *catalog.CatalogArtifactV1) { directory.CatalogID = "../kubernetes" }},
		{name: "invalid document key", mutate: func(directory *catalog.CatalogArtifactV1) { directory.Documents[0].Key = "../core-v1" }},
		{name: "duplicate document key", mutate: func(directory *catalog.CatalogArtifactV1) { directory.Documents[1].Key = directory.Documents[0].Key }},
		{name: "invalid catalog title", mutate: func(directory *catalog.CatalogArtifactV1) { directory.Title = string([]byte{0xff}) }},
		{name: "oversized document inventory", mutate: func(directory *catalog.CatalogArtifactV1) {
			directory.Documents = make([]catalog.DocumentDirectoryV1, maximumCatalogOverviewMetricsDocuments+1)
		}},
		{name: "oversized operation inventory", mutate: func(directory *catalog.CatalogArtifactV1) {
			directory.Documents[0].Operations = make([]catalog.OperationDirectoryV1, maximumCatalogOverviewMetricsOperations+1)
		}},
		{name: "oversized schema inventory", mutate: func(directory *catalog.CatalogArtifactV1) {
			directory.Documents[0].Schemas = make([]catalog.SchemaDirectoryV1, maximumCatalogOverviewMetricsSchemas+1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := cloneCatalogOverviewMetricsDirectory(base)
			test.mutate(&directory)
			fragment, err := PrepareCatalogOverviewMetrics(directory)
			if err == nil || !reflect.DeepEqual(fragment, CatalogOverviewMetricsFragment{}) {
				t.Fatalf("invalid catalog overview metrics accepted: fragment=%#v err=%v", fragment, err)
			}
		})
	}

	directory := cloneCatalogOverviewMetricsDirectory(base)
	fragment, err := PrepareCatalogOverviewMetrics(directory)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directory.CatalogID = "changed"
	directory.Title = "changed"
	directory.Documents[0].Key = "changed"
	directory.Documents[0].Operations = append(directory.Documents[0].Operations, catalog.OperationDirectoryV1{})
	directory.Documents[0].Schemas = append(directory.Documents[0].Schemas, catalog.SchemaDirectoryV1{})
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared catalog overview metrics changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered catalog overview metrics bytes alias fragment state")
	}
}

func TestPreparedCatalogOverviewMetricsAcceptsMaximumInventory(t *testing.T) {
	directory := catalog.CatalogArtifactV1{
		CatalogID: "kubernetes",
		Title:     "Kubernetes",
		Documents: make([]catalog.DocumentDirectoryV1, maximumCatalogOverviewMetricsDocuments),
	}
	for index := range directory.Documents {
		directory.Documents[index].Key = "doc-" + strconv.Itoa(index) + "-v1"
	}
	directory.Documents[0].Operations = make([]catalog.OperationDirectoryV1, maximumCatalogOverviewMetricsOperations)
	directory.Documents[0].Schemas = make([]catalog.SchemaDirectoryV1, maximumCatalogOverviewMetricsSchemas)
	fragment, err := PrepareCatalogOverviewMetrics(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fragment.Bytes(context.Background()); err != nil {
		t.Fatalf("bounded catalog overview metrics render failed: %v", err)
	}
}

func TestZeroCatalogOverviewMetricsFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (CatalogOverviewMetricsFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero catalog overview metrics fragment = bytes=%d err=%v", len(body), err)
	}
}

func cloneCatalogOverviewMetricsDirectory(source catalog.CatalogArtifactV1) catalog.CatalogArtifactV1 {
	clone := source
	clone.Documents = append([]catalog.DocumentDirectoryV1(nil), source.Documents...)
	for index := range clone.Documents {
		clone.Documents[index].Operations = append([]catalog.OperationDirectoryV1(nil), source.Documents[index].Operations...)
		clone.Documents[index].Schemas = append([]catalog.SchemaDirectoryV1(nil), source.Documents[index].Schemas...)
	}
	return clone
}
