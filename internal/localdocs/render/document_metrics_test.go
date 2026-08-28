package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
)

func TestPreparedCatalogDocumentMetricsRendersLegacyMetricMarkup(t *testing.T) {
	document := catalog.DocumentDirectoryV1{
		Key:        "core-v1",
		Operations: make([]catalog.OperationDirectoryV1, 2),
		Schemas:    make([]catalog.SchemaDirectoryV1, 1),
	}
	fragment, err := PrepareCatalogDocumentMetrics(document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := `<div class="grid gap-4 sm:grid-cols-2"><div class="rounded-radius border border-outline bg-surface p-5 dark:border-outline-dark dark:bg-surface-dark"><p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Operations</p><p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">2</p></div><div class="rounded-radius border border-outline bg-surface p-5 dark:border-outline-dark dark:bg-surface-dark"><p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Schemas</p><p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">1</p></div></div>`
	if string(body) != want {
		t.Fatalf("prepared catalog metrics drifted from legacy bytes:\nwant=%s\n got=%s", want, body)
	}
	for _, want := range []string{
		`<div class="grid gap-4 sm:grid-cols-2">`,
		`<p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Operations</p>`,
		`<p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">2</p>`,
		`<p class="text-sm font-medium text-on-surface-muted dark:text-on-surface-dark-muted">Schemas</p>`,
		`<p class="mt-2 break-words font-title text-2xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">1</p>`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("prepared catalog metrics missing %q:\n%s", want, body)
		}
	}
	if strings.Count(string(body), `class="grid gap-4 sm:grid-cols-2"`) != 1 {
		t.Fatalf("prepared catalog metrics rendered unexpected wrappers: %s", body)
	}
}

func TestPrepareCatalogDocumentMetricsFailsClosedAndCopiesCounts(t *testing.T) {
	base := catalog.DocumentDirectoryV1{
		Key:        "core-v1",
		Operations: make([]catalog.OperationDirectoryV1, 2),
		Schemas:    make([]catalog.SchemaDirectoryV1, 1),
	}
	for _, test := range []struct {
		name   string
		mutate func(*catalog.DocumentDirectoryV1)
	}{
		{name: "invalid document key", mutate: func(document *catalog.DocumentDirectoryV1) { document.Key = "../core-v1" }},
		{name: "oversized operation inventory", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.Operations = make([]catalog.OperationDirectoryV1, maximumCatalogDocumentMetricsCount+1)
		}},
		{name: "oversized schema inventory", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.Schemas = make([]catalog.SchemaDirectoryV1, maximumCatalogDocumentMetricsCount+1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := base
			test.mutate(&document)
			fragment, err := PrepareCatalogDocumentMetrics(document)
			if err == nil || !reflect.DeepEqual(fragment, CatalogDocumentMetricsFragment{}) {
				t.Fatalf("invalid catalog metrics accepted: fragment=%#v err=%v", fragment, err)
			}
		})
	}

	document := base
	fragment, err := PrepareCatalogDocumentMetrics(document)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document.Operations = append(document.Operations, catalog.OperationDirectoryV1{})
	document.Schemas = append(document.Schemas, catalog.SchemaDirectoryV1{}, catalog.SchemaDirectoryV1{})
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared catalog metrics changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered catalog metrics bytes alias fragment state")
	}
}

func TestPreparedCatalogDocumentMetricsAcceptsMaximumInventory(t *testing.T) {
	fragment, err := PrepareCatalogDocumentMetrics(catalog.DocumentDirectoryV1{
		Key:        "core-v1",
		Operations: make([]catalog.OperationDirectoryV1, maximumCatalogDocumentMetricsCount),
		Schemas:    make([]catalog.SchemaDirectoryV1, maximumCatalogDocumentMetricsCount),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fragment.Bytes(context.Background()); err != nil {
		t.Fatalf("bounded metrics render failed: %v", err)
	}
}

func TestPrepareCatalogDocumentMetricsWithoutResourceLimitsAcceptsLargeInventory(t *testing.T) {
	fragment, err := PrepareCatalogDocumentMetricsWithResourceLimits(catalog.DocumentDirectoryV1{
		Key:        "fortios-v1",
		Operations: make([]catalog.OperationDirectoryV1, 38_812),
		Schemas:    make([]catalog.SchemaDirectoryV1, 1_677),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{">38812</p>", ">1677</p>"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("unbounded catalog document metrics missing %q: %s", want, body)
		}
	}
}

func TestZeroCatalogDocumentMetricsFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (CatalogDocumentMetricsFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero catalog metrics fragment = bytes=%d err=%v", len(body), err)
	}
}
