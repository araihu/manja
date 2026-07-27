package projection

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/araihu/manja/domain"
)

func TestBuilderBuildsVersionOneDocument(t *testing.T) {
	input := fullBuilderFixture()
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if document.FormatVersion != 1 || document.ProjectID != "payments" || document.RevisionID != "rev-0001" {
		t.Fatalf("identity/version = %d %q %q", document.FormatVersion, document.ProjectID, document.RevisionID)
	}
	if document.Branding.DisplayName != "Payments" || document.Overview.SpecDownloadFilename != "openapi.json" {
		t.Fatalf("metadata = %#v %#v", document.Branding, document.Overview)
	}
	assertDocumentSlicesNonNil(t, document)
}

func TestBuilderDoesNotMutateSpecIndex(t *testing.T) {
	input := fullBuilderFixture()
	want := deepCopySpecIndex(t, input)
	if _, err := (Builder{}).Build(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, want) {
		t.Fatalf("Build mutated input\ngot:  %#v\nwant: %#v", input, want)
	}
}

func TestBuilderCanonicalNavigationMetadata(t *testing.T) {
	input := minimalIndex()
	input.Title = "Petstore"
	input.Operations = []domain.Operation{
		{ID: "listPets", Anchor: "operation-list/pets", Method: "get", Path: "/pets", Summary: "List pets"},
		{Method: "post", Path: "/pets/{pet-id}"},
	}
	input.Schemas = []domain.Schema{{Name: "Pet <Profile>"}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if document.Overview.Anchor != "overview" || document.Overview.Href != "?selected=overview#overview" || document.Overview.HeadingID != "overview-heading" || document.Overview.HeadingLevel != 2 {
		t.Fatalf("overview = %#v", document.Overview)
	}
	if document.MainLandmark != (Landmark{ID: "main-content", Role: "main"}) {
		t.Fatalf("landmark = %#v", document.MainLandmark)
	}
	if got := document.Operations[0]; got.Anchor != "operation-list/pets" || got.Href != "?selected=operation-list%2Fpets#operation-list/pets" {
		t.Fatalf("supplied operation = %#v", got)
	}
	if got := document.Operations[1]; got.Anchor != "operation-post-pets-pet-id" || got.Title != "POST /pets/{pet-id}" {
		t.Fatalf("fallback operation = %#v", got)
	}
	if got := document.Schemas[0]; got.Anchor != "schema-pet-profile" || got.Href != "?selected=schema-pet-profile#schema-pet-profile" {
		t.Fatalf("schema = %#v", got)
	}
}

func TestBuilderTranslatesLegacySchemaTargets(t *testing.T) {
	input := minimalIndex()
	input.Schemas = []domain.Schema{{Name: "author_association"}}
	input.Search = []domain.SearchDocument{{ID: "schema-author", Kind: "Schema", Href: "#schema-author_association"}}
	input.PublicRoutes = []domain.PublicRoute{{Path: "/?selected=schema-author_association#schema-author_association", Title: "Author association"}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantHref := "?selected=schema-author-association#schema-author-association"
	if document.Search[0].Href != wantHref {
		t.Fatalf("search href = %q, want %q", document.Search[0].Href, wantHref)
	}
	if document.PublicRoutes[0].Path != "/"+wantHref {
		t.Fatalf("route = %q, want %q", document.PublicRoutes[0].Path, "/"+wantHref)
	}

	input.Schemas = append(input.Schemas, domain.Schema{Name: "author-association"})
	if _, err := (Builder{}).Build(context.Background(), input); err == nil {
		t.Fatal("ambiguous legacy alias accepted")
	}
}

func TestBuilderRejectsDuplicateRecordKeys(t *testing.T) {
	tests := map[string]domain.SpecIndex{
		"operation anchor": func() domain.SpecIndex {
			idx := minimalIndex()
			idx.Operations = []domain.Operation{{Anchor: "same", Method: "GET", Path: "/a"}, {Anchor: "same", Method: "POST", Path: "/b"}}
			return idx
		}(),
		"schema anchor": func() domain.SpecIndex {
			idx := minimalIndex()
			idx.Schemas = []domain.Schema{{Name: "Pet Profile"}, {Name: "Pet-Profile"}}
			return idx
		}(),
		"parameter": func() domain.SpecIndex {
			idx := minimalIndex()
			idx.Operations = []domain.Operation{{Anchor: "op", Method: "GET", Path: "/", Parameters: []domain.OperationParameter{{Name: "id", In: "query"}, {Name: "id", In: "QUERY"}}}}
			return idx
		}(),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if document, err := (Builder{}).Build(context.Background(), input); err == nil || !reflect.ValueOf(document).IsZero() {
				t.Fatalf("Build = %#v, %v; want zero document and error", document, err)
			}
		})
	}
}

func TestBuilderPreservesOrdinalsBeforeSorting(t *testing.T) {
	input := minimalIndex()
	input.Overview.Servers = []domain.SpecServer{
		{URL: "https://z.example", Variables: []domain.SpecServerVariable{{Name: "z"}, {Name: "a"}}},
		{URL: "https://a.example"},
	}
	input.Operations = []domain.Operation{{
		Anchor: "op", Method: "GET", Path: "/",
		Parameters: []domain.OperationParameter{{Name: "z", In: "query"}, {Name: "a", In: "query"}},
		Responses:  []domain.OperationResponse{{Status: "500"}, {Status: "200"}},
	}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if document.Overview.Servers[0].Ordinal != 0 || document.Overview.Servers[1].Ordinal != 1 {
		t.Fatalf("server ordinals = %#v", document.Overview.Servers)
	}
	if got := document.OperationDetails[0].Parameters; got[0].Name != "z" || got[0].Ordinal != 0 || got[1].Name != "a" || got[1].Ordinal != 1 {
		t.Fatalf("parameter ordinals = %#v", got)
	}
	if got := document.OperationDetails[0].Responses; got[0].Status != "500" || got[0].Ordinal != 0 || got[1].Status != "200" || got[1].Ordinal != 1 {
		t.Fatalf("response ordinals = %#v", got)
	}
}

func TestBuilderHonorsCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if document, err := (Builder{}).Build(canceled, minimalIndex()); !errors.Is(err, context.Canceled) || !reflect.ValueOf(document).IsZero() {
		t.Fatalf("pre-canceled Build = %#v, %v", document, err)
	}

	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Unix(0, 0))
	cancelDeadline()
	if document, err := (Builder{}).Build(deadline, minimalIndex()); !errors.Is(err, context.DeadlineExceeded) || !reflect.ValueOf(document).IsZero() {
		t.Fatalf("expired-deadline Build = %#v, %v", document, err)
	}

	input := minimalIndex()
	for i := 0; i < 20; i++ {
		suffix := string(rune('a' + i))
		input.Operations = append(input.Operations, domain.Operation{Anchor: "operation-" + suffix, Method: "GET", Path: "/" + suffix})
	}
	ctx := &countingContext{Context: context.Background(), remaining: 5}
	if document, err := (Builder{}).Build(ctx, input); !errors.Is(err, context.Canceled) || !reflect.ValueOf(document).IsZero() {
		t.Fatalf("during-build cancellation = %#v, %v", document, err)
	}
}

func TestBuilderPreservesDisplayExampleText(t *testing.T) {
	input := minimalIndex()
	input.Operations = []domain.Operation{{
		Anchor: "examples", Method: "POST", Path: "/examples",
		Parameters:  []domain.OperationParameter{{Name: "flag", In: "query", Example: "true", Schema: domain.SchemaSummary{Default: "1e+03", Example: "{\"looks\":\"json\"}"}}},
		RequestBody: &domain.OperationRequestBody{MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Example: "", ExampleProvided: true}}},
	}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	parameter := document.OperationDetails[0].Parameters[0]
	if parameter.Examples[0].Text != "true" || parameter.Schema.DefaultValue != "1e+03" || parameter.Schema.ExampleText != "{\"looks\":\"json\"}" {
		t.Fatalf("display text changed: %#v", parameter)
	}
	if examples := document.OperationDetails[0].RequestBody.MediaTypes[0].Examples; len(examples) != 1 || examples[0].Text != "" || !examples[0].Provided {
		t.Fatalf("provided empty example = %#v", examples)
	}
}

func TestBuilderSeparatesSchemaJSONAndExample(t *testing.T) {
	input := minimalIndex()
	input.Schemas = []domain.Schema{{
		Name: "Pair", Summary: domain.SchemaSummary{JSON: "{\"schema\":1e+03}"},
		Example: domain.SchemaExample{JSON: "{\"shape\":1.0}", Example: "__EXPLICIT_TEXT__", Provided: true},
	}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	detail := document.SchemaDetails[0]
	if detail.Schema.JSON != "{\"schema\":1000}" || detail.ExampleSchemaJSON != "{\"shape\":1}" || detail.Examples[0].Text != "__EXPLICIT_TEXT__" {
		t.Fatalf("paired schema payloads = %#v", detail)
	}
}

func TestBuilderValidatesProjectionIdentityWithDomainContract(t *testing.T) {
	values := []string{"", " valid", "valid ", "bad\x00id", "bad\nid", string([]byte{0xff}), "valid", "internal space", "café"}
	for _, field := range []string{"projectId", "revisionId"} {
		for _, value := range values {
			t.Run(field+"/"+strconvSafe(value), func(t *testing.T) {
				input := minimalIndex()
				if field == "projectId" {
					input.ProjectID = value
				} else {
					input.RevisionID = value
				}
				domainErr := domain.ValidateCanonicalIdentity("contract", value, false)
				document, err := (Builder{}).Build(context.Background(), input)
				if (err != nil) != (domainErr != nil) {
					t.Fatalf("Build error = %v; domain error = %v", err, domainErr)
				}
				if err != nil && !reflect.ValueOf(document).IsZero() {
					t.Fatalf("rejected identity returned partial document: %#v", document)
				}
			})
		}
	}
}

func TestBuilderErrorsDoNotDiscloseSourceContent(t *testing.T) {
	sentinel := "__PRIVATE_SOURCE_SENTINEL__"
	input := minimalIndex()
	input.ProjectID = sentinel + "\x00"
	assertBoundedError(t, input, sentinel)
	input = minimalIndex()
	input.Schemas = []domain.Schema{{Name: "Schema", Example: domain.SchemaExample{JSON: "{\"" + sentinel + "\":1,\"" + sentinel + "\":2}"}}}
	assertBoundedError(t, input, sentinel)
}

func TestBuilderErrorsAreBounded(t *testing.T) {
	input := minimalIndex()
	input.Operations = []domain.Operation{{Anchor: strings.Repeat("x", 400) + " ", Method: "GET", Path: "/"}}
	document, err := (Builder{}).Build(context.Background(), input)
	if err == nil || !reflect.ValueOf(document).IsZero() {
		t.Fatalf("Build = %#v, %v", document, err)
	}
	if len(err.Error()) > 256 || !utf8.ValidString(err.Error()) {
		t.Fatalf("error length/UTF-8 = %d/%v", len(err.Error()), utf8.ValidString(err.Error()))
	}
}

type countingContext struct {
	context.Context
	remaining int
}

func (c *countingContext) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func minimalIndex() domain.SpecIndex {
	return domain.SpecIndex{
		ProjectID: "payments", RevisionID: "rev-0001", Title: "Payments", Version: "1.0.0",
		Operations: []domain.Operation{}, Schemas: []domain.Schema{}, Search: []domain.SearchDocument{}, PublicRoutes: []domain.PublicRoute{},
		Overview: domain.SpecOverview{Servers: []domain.SpecServer{}},
	}
}

func fullBuilderFixture() domain.SpecIndex {
	input := minimalIndex()
	input.Branding = domain.DocsBranding{DisplayName: "Payments", Logo: domain.DocsBrandingLogo{Src: "/logo.svg", Alt: "Payments", HomeURL: "/"}, Favicon: "/favicon.svg"}
	input.Overview = domain.SpecOverview{
		Description: "Payments docs", TermsOfService: "/terms",
		Contact: domain.SpecContact{Name: "API", URL: "/contact", Email: "api@example.com"},
		License: domain.SpecLicense{Name: "MIT", URL: "/license", Identifier: "MIT"},
		Servers: []domain.SpecServer{},
	}
	input.SpecDownload = domain.SpecDownload{Filename: "openapi.json", JSON: []byte("__RAW__")}
	return input
}

func assertDocumentSlicesNonNil(t *testing.T, document Document) {
	t.Helper()
	values := []any{document.Overview.Servers, document.SidebarSections, document.Operations, document.OperationDetails, document.Schemas, document.SchemaDetails, document.Search, document.PublicRoutes}
	for index, value := range values {
		if reflect.ValueOf(value).IsNil() {
			t.Errorf("top-level slice %d is nil", index)
		}
	}
}

func assertBoundedError(t *testing.T, input domain.SpecIndex, sentinel string) {
	t.Helper()
	document, err := (Builder{}).Build(context.Background(), input)
	if err == nil || !reflect.ValueOf(document).IsZero() {
		t.Fatalf("Build = %#v, %v", document, err)
	}
	if strings.Contains(err.Error(), sentinel) || len(err.Error()) > 256 || !utf8.ValidString(err.Error()) {
		t.Fatalf("unsafe error %q", err)
	}
}

func deepCopySpecIndex(t *testing.T, input domain.SpecIndex) domain.SpecIndex {
	t.Helper()
	// Fixture contains no pointer-shared or cyclic schema values; copying every
	// source branch manually would duplicate the production mapping under test.
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var copied domain.SpecIndex
	if err := json.Unmarshal(encoded, &copied); err != nil {
		t.Fatal(err)
	}
	return copied
}

func strconvSafe(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == utf8.RuneError {
			return '_'
		}
		return r
	}, value)
}
