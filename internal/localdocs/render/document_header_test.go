package render

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
)

func TestPreparedCatalogDocumentHeaderRendersCopiedIdentity(t *testing.T) {
	document, documentHref, downloadHref := catalogDocumentHeaderFixture()
	fragment, err := PrepareCatalogDocumentHeader(document, documentHref, downloadHref)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fragment.Bytes(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<header data-catalog-document-header class="mb-8 grid min-w-0 gap-4 border-b border-outline pb-8 dark:border-outline-dark">`,
		`<p class="mb-2 text-sm font-semibold uppercase tracking-wide text-primary dark:text-primary-dark">OpenAPI document</p>`,
		`<h1 tabindex="-1" data-manja-settled-focus="true" class="manja-schema-title min-w-0 break-words font-title text-3xl font-bold text-on-surface-strong sm:text-4xl dark:text-on-surface-dark-strong" title="core-v1">core-v1</h1>`,
		`href="/kubernetes/openapi/core-v1.json"`,
		`>Download source</span>`,
		`This &lt;document&gt; description.`,
	} {
		if !bytes.Contains(rendered, []byte(want)) {
			t.Errorf("prepared document header missing %q:\n%s", want, rendered)
		}
	}
	if bytes.Contains(rendered, []byte(`v1`)) && !bytes.Contains(rendered, []byte(`>v1</span>`)) {
		t.Fatal("prepared document header rendered version in unexpected form")
	}
}

func TestPreparedCatalogDocumentHeaderHidesUnversionedVersion(t *testing.T) {
	document, documentHref, downloadHref := catalogDocumentHeaderFixture()
	document.APIVersion = "unversioned"
	fragment, err := PrepareCatalogDocumentHeader(document, documentHref, downloadHref)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fragment.Bytes(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rendered, []byte(`>unversioned</span>`)) {
		t.Fatal("prepared document header rendered unversioned badge")
	}
}

func TestPrepareCatalogDocumentHeaderFailsClosedOnInvalidBinding(t *testing.T) {
	document, documentHref, downloadHref := catalogDocumentHeaderFixture()
	tests := []struct {
		name   string
		mutate func(*catalog.DocumentDirectoryV1, *string, *string)
	}{
		{name: "invalid document key", mutate: func(value *catalog.DocumentDirectoryV1, _, _ *string) { value.Key = "../core-v1" }},
		{name: "wrong document href", mutate: func(_ *catalog.DocumentDirectoryV1, value, _ *string) { *value = "/kubernetes/documents/other-v1/" }},
		{name: "invalid document href", mutate: func(_ *catalog.DocumentDirectoryV1, value, _ *string) {
			*value = "/kubernetes/documents/core-v1/?selected=x"
		}},
		{name: "wrong download document", mutate: func(_ *catalog.DocumentDirectoryV1, _, value *string) { *value = "/kubernetes/openapi/other-v1.json" }},
		{name: "download query", mutate: func(_ *catalog.DocumentDirectoryV1, _, value *string) { *value += "?download=1" }},
		{name: "missing source child extension", mutate: func(value *catalog.DocumentDirectoryV1, _, _ *string) { value.SourceChild = "sources/core-v1" }},
		{name: "invalid description", mutate: func(value *catalog.DocumentDirectoryV1, _, _ *string) {
			value.Overview.Description = string([]byte{0xff})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, href, download := document, documentHref, downloadHref
			test.mutate(&value, &href, &download)
			fragment, err := PrepareCatalogDocumentHeader(value, href, download)
			if err == nil || (fragment != CatalogDocumentHeaderFragment{}) {
				t.Fatalf("invalid document header accepted: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedCatalogDocumentHeaderCopiesInputsAndBytes(t *testing.T) {
	document, documentHref, downloadHref := catalogDocumentHeaderFixture()
	fragment, err := PrepareCatalogDocumentHeader(document, documentHref, downloadHref)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	document.Key = "changed-v1"
	document.APIVersion = "changed"
	document.Overview.Description = "changed"
	downloadHref = "/changed"
	got, err := fragment.Bytes(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared document header changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered document header bytes alias fragment state")
	}
}

func TestPreparedCatalogDocumentHeaderRejectsOversizedOutput(t *testing.T) {
	document, documentHref, downloadHref := catalogDocumentHeaderFixture()
	document.Overview.Description = strings.Repeat("x", maximumHTMLFragmentBytes)
	fragment, err := PrepareCatalogDocumentHeader(document, documentHref, downloadHref)
	if err == nil || fragment != (CatalogDocumentHeaderFragment{}) {
		t.Fatalf("oversized document header accepted: fragment=%#v err=%v", fragment, err)
	}
}

func catalogDocumentHeaderFixture() (catalog.DocumentDirectoryV1, string, string) {
	return catalog.DocumentDirectoryV1{
		Key: "core-v1", Title: "Kubernetes Core v1", APIVersion: "v1",
		SourcePath: "api/openapi.json", SourceChild: "sources/core-v1.json",
		Overview: projection.Overview{Description: "This <document> description."},
	}, "/kubernetes/documents/core-v1/", "/kubernetes/openapi/core-v1.json"
}
