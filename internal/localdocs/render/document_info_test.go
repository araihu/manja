package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
)

func TestPreparedCatalogDocumentInfoRendersCopiedMetadata(t *testing.T) {
	document := catalog.DocumentDirectoryV1{Key: "core-v1", Overview: projection.Overview{
		Contact:        projection.Contact{Name: "API <Support>", URL: "https://example.test/support?a=1&b=2", Email: "support@example.test"},
		License:        projection.License{Name: "Apache 2.0", URL: "https://example.test/license", Identifier: "Apache-2.0"},
		TermsOfService: "https://example.test/terms",
	}}
	fragment, err := PrepareCatalogDocumentInfo(document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`aria-label="OpenAPI information"`,
		`>Contact</dt>`,
		`API &lt;Support&gt;`,
		`href="https://example.test/support?a=1&amp;b=2"`,
		`href="mailto:support@example.test"`,
		`>License</dt>`,
		`>Apache 2.0</a>`,
		`>Apache-2.0</code>`,
		`>Terms of service</dt>`,
		`>View terms</a>`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("prepared document info missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte("<Support>")) {
		t.Fatal("prepared document info leaked unescaped contact name")
	}
}

func TestPrepareCatalogDocumentInfoFailsClosedOnInvalidTextOrOutput(t *testing.T) {
	base := catalog.DocumentDirectoryV1{Key: "core-v1", Overview: projection.Overview{
		Contact:        projection.Contact{Name: "Support"},
		License:        projection.License{Name: "Apache 2.0"},
		TermsOfService: "https://example.test/terms",
	}}
	tests := []struct {
		name   string
		mutate func(*catalog.DocumentDirectoryV1)
	}{
		{name: "invalid document key", mutate: func(document *catalog.DocumentDirectoryV1) { document.Key = "../core-v1" }},
		{name: "invalid contact name", mutate: func(document *catalog.DocumentDirectoryV1) { document.Overview.Contact.Name = string([]byte{0xff}) }},
		{name: "invalid contact URL", mutate: func(document *catalog.DocumentDirectoryV1) { document.Overview.Contact.URL = string([]byte{0xff}) }},
		{name: "invalid contact email", mutate: func(document *catalog.DocumentDirectoryV1) { document.Overview.Contact.Email = string([]byte{0xff}) }},
		{name: "invalid license name", mutate: func(document *catalog.DocumentDirectoryV1) { document.Overview.License.Name = string([]byte{0xff}) }},
		{name: "invalid license URL", mutate: func(document *catalog.DocumentDirectoryV1) { document.Overview.License.URL = string([]byte{0xff}) }},
		{name: "invalid license identifier", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.Overview.License.Identifier = string([]byte{0xff})
		}},
		{name: "invalid terms", mutate: func(document *catalog.DocumentDirectoryV1) { document.Overview.TermsOfService = string([]byte{0xff}) }},
		{name: "oversized output", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.Overview.Contact.Name = strings.Repeat("x", maximumHTMLFragmentBytes)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := base
			test.mutate(&document)
			fragment, err := PrepareCatalogDocumentInfo(document)
			if err == nil || !reflect.DeepEqual(fragment, CatalogDocumentInfoFragment{}) {
				t.Fatalf("invalid document info accepted: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedCatalogDocumentInfoCopiesInputsAndBytes(t *testing.T) {
	document := catalog.DocumentDirectoryV1{Key: "core-v1", Overview: projection.Overview{
		Contact:        projection.Contact{Name: "Support", URL: "https://example.test/support"},
		License:        projection.License{Name: "Apache 2.0"},
		TermsOfService: "https://example.test/terms",
	}}
	fragment, err := PrepareCatalogDocumentInfo(document)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document.Overview.Contact.Name = "changed"
	document.Overview.Contact.URL = "/changed"
	document.Overview.License.Name = "changed"
	document.Overview.TermsOfService = "/changed"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared document info changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered document info bytes alias fragment state")
	}
}

func TestPreparedCatalogDocumentInfoPreservesWhitespacePresenceRules(t *testing.T) {
	document := catalog.DocumentDirectoryV1{Key: "core-v1", Overview: projection.Overview{
		Contact:        projection.Contact{Name: " ", URL: "https://example.test/support"},
		License:        projection.License{Identifier: " "},
		TermsOfService: " ",
	}}
	fragment, err := PrepareCatalogDocumentInfo(document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`>Contact</dt>`)) || bytes.Contains(body, []byte(`>License</dt>`)) || bytes.Contains(body, []byte(`>Terms of service</dt>`)) {
		t.Fatalf("whitespace-only presence rules changed: %s", body)
	}
}

func TestZeroCatalogDocumentInfoFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (CatalogDocumentInfoFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero document info fragment = bytes=%d err=%v", len(body), err)
	}
}
