package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
)

func TestPreparedCatalogDocumentSecuritySchemesRendersCopiedMetadata(t *testing.T) {
	document := catalog.DocumentDirectoryV1{
		Key: "core-v1",
		SecuritySchemes: []catalog.SecuritySchemeDirectoryV1{{
			Name:             "BearerToken",
			Anchor:           "security-scheme-bearer",
			Type:             "http",
			Description:      "Use a bearer token <read-only>.",
			ParameterName:    "Authorization",
			In:               "header",
			Scheme:           "bearer",
			BearerFormat:     "JWT",
			OpenIDConnectURL: "https://example.test/.well-known/openid-configuration?a=1&b=2",
		}},
	}
	fragment, err := PrepareCatalogDocumentSecuritySchemes(document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="document-security-schemes"`,
		`>Security Schemes</h2>`,
		`id="security-scheme-bearer"`,
		`>BearerToken</h3>`,
		`>http</span>`,
		`Use a bearer token &lt;read-only&gt;.`,
		`>Request header</dt>`,
		`>Authorization</dd>`,
		`>Scheme</dt>`,
		`>bearer</dd>`,
		`>Bearer format</dt>`,
		`>JWT</dd>`,
		`>OpenID Connect URL</dt>`,
		`https://example.test/.well-known/openid-configuration?a=1&amp;b=2`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("prepared catalog security schemes missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte(`<read-only>`)) {
		t.Fatal("prepared catalog security schemes leaked unescaped description")
	}
}

func TestPreparedCatalogDocumentSecuritySchemesOmitsEmptyInventory(t *testing.T) {
	fragment, err := PrepareCatalogDocumentSecuritySchemes(catalog.DocumentDirectoryV1{Key: "core-v1"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("empty catalog security scheme inventory rendered %d bytes: %s", len(body), body)
	}
}

func TestPrepareCatalogDocumentSecuritySchemesFailsClosedAndCopiesInputs(t *testing.T) {
	base := catalog.DocumentDirectoryV1{
		Key: "core-v1",
		SecuritySchemes: []catalog.SecuritySchemeDirectoryV1{{
			Name: "ApiKey", Anchor: "security-scheme-api-key", Type: "apiKey", ParameterName: "X-API-Key", In: "header",
		}},
	}
	tests := []struct {
		name   string
		mutate func(*catalog.DocumentDirectoryV1)
	}{
		{name: "invalid document key", mutate: func(document *catalog.DocumentDirectoryV1) { document.Key = "../core-v1" }},
		{name: "empty scheme name", mutate: func(document *catalog.DocumentDirectoryV1) { document.SecuritySchemes[0].Name = "" }},
		{name: "invalid scheme name", mutate: func(document *catalog.DocumentDirectoryV1) { document.SecuritySchemes[0].Name = string([]byte{0xff}) }},
		{name: "invalid scheme anchor", mutate: func(document *catalog.DocumentDirectoryV1) { document.SecuritySchemes[0].Anchor = "" }},
		{name: "invalid scheme description", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].Description = string([]byte{0xff})
		}},
		{name: "invalid scheme type", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].Type = string([]byte{0xff})
		}},
		{name: "invalid parameter name", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].ParameterName = string([]byte{0xff})
		}},
		{name: "invalid parameter location", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].In = string([]byte{0xff})
		}},
		{name: "invalid scheme", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].Scheme = string([]byte{0xff})
		}},
		{name: "invalid bearer format", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].BearerFormat = string([]byte{0xff})
		}},
		{name: "invalid openid url", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].OpenIDConnectURL = string([]byte{0xff})
		}},
		{name: "oversized output", mutate: func(document *catalog.DocumentDirectoryV1) {
			document.SecuritySchemes[0].Description = strings.Repeat("x", maximumHTMLFragmentBytes)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := base
			document.SecuritySchemes = append([]catalog.SecuritySchemeDirectoryV1(nil), base.SecuritySchemes...)
			test.mutate(&document)
			fragment, err := PrepareCatalogDocumentSecuritySchemes(document)
			if err == nil || !reflect.DeepEqual(fragment, CatalogDocumentSecuritySchemesFragment{}) {
				t.Fatalf("invalid catalog security schemes accepted: fragment=%#v err=%v", fragment, err)
			}
		})
	}

	document := base
	fragment, err := PrepareCatalogDocumentSecuritySchemes(document)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document.SecuritySchemes[0].Name = "changed"
	document.SecuritySchemes[0].Description = "changed"
	document.SecuritySchemes[0].ParameterName = "changed"
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared catalog security schemes changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered catalog security schemes bytes alias fragment state")
	}
}

func TestPreparedCatalogDocumentSecuritySchemesRejectsDuplicateIdentity(t *testing.T) {
	document := catalog.DocumentDirectoryV1{
		Key: "core-v1",
		SecuritySchemes: []catalog.SecuritySchemeDirectoryV1{
			{Name: "ApiKey", Anchor: "security-scheme-api-key", Type: "apiKey"},
			{Name: "ApiKey", Anchor: "security-scheme-api-key-2", Type: "apiKey"},
		},
	}
	fragment, err := PrepareCatalogDocumentSecuritySchemes(document)
	if err == nil || !reflect.DeepEqual(fragment, CatalogDocumentSecuritySchemesFragment{}) {
		t.Fatalf("duplicate catalog security schemes accepted: fragment=%#v err=%v", fragment, err)
	}
}

func TestPreparedCatalogDocumentSecuritySchemesRejectsOversizedInventory(t *testing.T) {
	schemes := make([]catalog.SecuritySchemeDirectoryV1, maximumCatalogDocumentSecuritySchemes+1)
	for index := range schemes {
		schemes[index] = catalog.SecuritySchemeDirectoryV1{
			Name:   "scheme-" + strings.Repeat("x", 4) + string(rune('a'+index%26)) + "-" + strings.Repeat("y", index%7),
			Anchor: "security-scheme-" + strings.Repeat("a", 16) + "-" + strings.Repeat("b", index%7),
		}
	}
	fragment, err := PrepareCatalogDocumentSecuritySchemes(catalog.DocumentDirectoryV1{Key: "core-v1", SecuritySchemes: schemes})
	if err == nil || !reflect.DeepEqual(fragment, CatalogDocumentSecuritySchemesFragment{}) {
		t.Fatalf("oversized catalog security scheme inventory accepted: fragment=%#v err=%v", fragment, err)
	}
}

func TestZeroCatalogDocumentSecuritySchemesFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (CatalogDocumentSecuritySchemesFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero catalog security schemes fragment = bytes=%d err=%v", len(body), err)
	}
}
