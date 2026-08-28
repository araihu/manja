package render

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestPreparedCatalogDocumentTableRendersBoundedRowsAndSortState(t *testing.T) {
	fragment, err := PrepareCatalogDocumentTable("/kubernetes", "operations", "desc", []CatalogDocumentTableEntry{
		{
			Key: "core-v1", Label: "core-v1", Version: "v1", Operations: 2, Schemas: 1,
			SearchText: "core-v1 v1", Href: "/kubernetes/documents/core-v1/", AvatarSrc: "/logos/core.svg",
		},
		{
			Key: "apps-v1", Label: "apps-v1", Version: "v1", Operations: 1, Schemas: 3,
			SearchText: "apps-v1 v1", Href: "/kubernetes/documents/apps-v1/", AvatarSrc: "/logos/apps.svg",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-search-text="core-v1 v1"`,
		`data-search-text="apps-v1 v1"`,
		`data-table-sort-by="operations"`,
		`data-table-sort-dir="desc"`,
		`hx-swap-oob="innerHTML"`,
		`table_id=catalog-documents-table`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("prepared catalog document table missing %q:\n%s", want, body)
		}
	}
}

func TestPreparedCatalogDocumentTableRendersHumanDocumentTitle(t *testing.T) {
	label := "Virtual Infrastructure JSON API"
	fragment, err := PrepareCatalogDocumentTable("/vmware", "", "", []CatalogDocumentTableEntry{{
		Key: "openapi", Label: label, Version: "8.0.3", SearchText: CatalogDocumentTableSearchText("openapi", label, "8.0.3"),
		Href: "/vmware/documents/openapi/", Operations: 1902, Schemas: 8141,
	}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.PageBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(">Virtual Infrastructure JSON API</span>")) {
		t.Fatalf("document table omitted human title: %s", body)
	}
	if bytes.Contains(body, []byte(">openapi</span>")) {
		t.Fatalf("document table exposed internal document key: %s", body)
	}
}

func TestPrepareCatalogDocumentTableFailsClosedAndCopiesRows(t *testing.T) {
	base := []CatalogDocumentTableEntry{{
		Key: "core-v1", Label: "core-v1", Version: "v1", Operations: 2, Schemas: 1,
		SearchText: "core-v1 v1", Href: "/documents/core-v1/", AvatarSrc: "/logos/core.svg",
	}}
	for _, test := range []struct {
		name   string
		mutate func(string, string, string, []CatalogDocumentTableEntry) (string, string, string, []CatalogDocumentTableEntry)
	}{
		{name: "invalid endpoint", mutate: func(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry) (string, string, string, []CatalogDocumentTableEntry) {
			return "https://evil.test/", sortBy, sortDir, entries
		}},
		{name: "invalid href", mutate: func(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry) (string, string, string, []CatalogDocumentTableEntry) {
			entries[0].Href = "javascript:alert(1)"
			return mount, sortBy, sortDir, entries
		}},
		{name: "search text drift", mutate: func(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry) (string, string, string, []CatalogDocumentTableEntry) {
			entries[0].SearchText = "changed"
			return mount, sortBy, sortDir, entries
		}},
		{name: "invalid sort", mutate: func(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry) (string, string, string, []CatalogDocumentTableEntry) {
			return mount, "source", "asc", entries
		}},
		{name: "invalid utf8", mutate: func(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry) (string, string, string, []CatalogDocumentTableEntry) {
			entries[0].Label = string([]byte{0xff})
			return mount, sortBy, sortDir, entries
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := append([]CatalogDocumentTableEntry(nil), base...)
			mount, sortBy, sortDir, entries := test.mutate("/", "", "", entries)
			fragment, err := PrepareCatalogDocumentTable(mount, sortBy, sortDir, entries)
			if err == nil || !reflect.DeepEqual(fragment, CatalogDocumentTableFragment{}) {
				t.Fatalf("invalid catalog document table accepted: fragment=%#v err=%v", fragment, err)
			}
		})
	}

	fragment, err := PrepareCatalogDocumentTable("/", "", "", base)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	base[0].Label = "changed"
	base[0].SearchText = "changed"
	base[0].Operations = 99
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared catalog document table changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered catalog document table bytes alias fragment state")
	}
}

func TestZeroCatalogDocumentTableFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (CatalogDocumentTableFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero catalog document table fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestPrepareCatalogDocumentTableRejectsOversizedRowsAndOutput(t *testing.T) {
	entries := make([]CatalogDocumentTableEntry, maximumCatalogDocumentTableRows+1)
	if fragment, err := PrepareCatalogDocumentTable("/", "", "", entries); err == nil || !reflect.DeepEqual(fragment, CatalogDocumentTableFragment{}) {
		t.Fatalf("oversized catalog document table accepted: fragment=%#v err=%v", fragment, err)
	}
	entries = []CatalogDocumentTableEntry{{
		Key: "core-v1", Label: strings.Repeat("x", maximumHTMLFragmentBytes), Version: "v1",
		SearchText: "core-v1 v1", Href: "/documents/core-v1/",
	}}
	if fragment, err := PrepareCatalogDocumentTable("/", "", "", entries); err == nil || !reflect.DeepEqual(fragment, CatalogDocumentTableFragment{}) {
		t.Fatalf("oversized catalog document table output accepted: fragment=%#v err=%v", fragment, err)
	}
}

func TestPrepareCatalogDocumentTableWithoutResourceLimitsAcceptsLargeInventory(t *testing.T) {
	const documentCount = 1045
	entries := make([]CatalogDocumentTableEntry, documentCount)
	for index := range entries {
		key := "fortios-" + strconv.Itoa(index) + "-v1"
		label := "FortiOS " + strconv.Itoa(index)
		entries[index] = CatalogDocumentTableEntry{
			Key: key, Label: label, Version: "v1", Operations: index, Schemas: index + 1,
			SearchText: CatalogDocumentTableSearchText(key, label, "v1"),
			Href:       "/fortinet/documents/" + key + "/",
		}
	}
	fragment, err := PrepareCatalogDocumentTableWithResourceLimits("/fortinet", "", "", entries, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.PageBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("FortiOS 1044")) {
		t.Fatalf("unbounded catalog document table omitted final row: %s", body)
	}
}
