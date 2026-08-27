package projectionjson

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestGoldenV2EmptyProjection(t *testing.T) {
	assertGolden(t, "v2-empty", emptyFixture())
}

func TestGoldenV2OperationProjection(t *testing.T) {
	assertGolden(t, "v2-operation", operationFixture())
}

func TestGoldenV2FullProjection(t *testing.T) {
	assertGolden(t, "v2-full", fullFixture())
}

func TestFullFixtureManifest(t *testing.T) {
	document := mustBuild(t, fullFixture())
	counts := map[string]int{
		"operations": len(document.Operations), "schemas": len(document.Schemas),
		"schemaNodes": len(document.SchemaNodes),
		"search":      len(document.Search), "routes": len(document.PublicRoutes),
		"servers": len(document.Overview.Servers), "sidebar": len(document.SidebarSections),
	}
	want := map[string]int{"operations": 2, "schemas": 2, "schemaNodes": 6, "search": 3, "routes": 4, "servers": 2, "sidebar": 3}
	for key, count := range want {
		if counts[key] != count {
			t.Errorf("manifest %s count = %d, want %d", key, counts[key], count)
		}
	}
	readme := string(mustReadFixture(t, "README.md"))
	for _, required := range []string{"operation-create-pet", "operation-list-pets", "schema-error", "schema-pet", "__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__", "__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__"} {
		if !strings.Contains(readme, required) {
			t.Errorf("manifest lacks %q", required)
		}
	}
}

func assertGolden(t *testing.T, name string, input domain.SpecIndex) {
	t.Helper()
	got := mustMarshal(t, mustBuild(t, input))
	if os.Getenv("MANJA_UPDATE_PROJECTION_GOLDEN") == "1" {
		if err := os.WriteFile(fixturePath(name+".candidate.json"), got, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixturePath(name+".candidate.sha256"), []byte(Digest(got)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	promoteReviewedGolden(t, name, got)
	want := mustReadFixture(t, name+".json")
	if len(want) == 0 || want[len(want)-1] == '\n' {
		t.Fatalf("%s has final newline", name)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s bytes differ\ngot:  %s\nwant: %s", name, got, want)
	}
	digest := strings.TrimSpace(string(mustReadFixture(t, name+".sha256")))
	if len(digest) != 64 || Digest(got) != digest {
		t.Fatalf("%s digest mismatch", name)
	}
}

func promoteReviewedGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	if os.Getenv("MANJA_ACCEPT_REVIEWED_PROJECTION_GOLDEN") != "1" {
		return
	}
	acceptedJSON := fixturePath(name + ".json")
	acceptedDigest := fixturePath(name + ".sha256")
	if _, err := os.Stat(acceptedJSON); !os.IsNotExist(err) {
		t.Fatalf("refusing to overwrite accepted golden %s", name)
	}
	if _, err := os.Stat(acceptedDigest); !os.IsNotExist(err) {
		t.Fatalf("refusing to overwrite accepted digest %s", name)
	}
	candidateJSON := fixturePath(name + ".candidate.json")
	candidateDigest := fixturePath(name + ".candidate.sha256")
	candidate := mustReadPath(t, candidateJSON)
	digest := strings.TrimSpace(string(mustReadPath(t, candidateDigest)))
	if !bytes.Equal(candidate, got) || len(candidate) == 0 || candidate[len(candidate)-1] == '\n' || Digest(candidate) != digest {
		t.Fatalf("reviewed candidate %s no longer matches current canonical output", name)
	}
	if err := os.Rename(candidateDigest, acceptedDigest); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(candidateJSON, acceptedJSON); err != nil {
		_ = os.Rename(acceptedDigest, candidateDigest)
		t.Fatal(err)
	}
}

func emptyFixture() domain.SpecIndex {
	return domain.SpecIndex{
		ProjectID: "payments", RevisionID: "rev-0001", Title: "Payments <API>\u2028", Version: "1.2.3",
		Operations: []domain.Operation{}, Schemas: []domain.Schema{}, Search: []domain.SearchDocument{}, PublicRoutes: []domain.PublicRoute{},
		Overview: domain.SpecOverview{Servers: []domain.SpecServer{}},
	}
}

func operationFixture() domain.SpecIndex {
	return domain.SpecIndex{
		ProjectID: "pets", RevisionID: "rev-0002", Title: "Petstore", Version: "1.0.0",
		Overview:     domain.SpecOverview{Servers: []domain.SpecServer{}},
		Operations:   []domain.Operation{{ID: "listPets", Anchor: "operation-list/pets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}}},
		Schemas:      []domain.Schema{},
		Search:       []domain.SearchDocument{{ID: "op-list", Title: "List pets", Href: "#operation-list/pets", Kind: "Operation", Method: "GET", Path: "/pets", Section: "Pets", Keywords: []string{"pets"}}},
		PublicRoutes: []domain.PublicRoute{{Path: "/", Title: "Petstore"}, {Path: "/?selected=operation-list%2Fpets#operation-list/pets", Title: "List pets"}},
	}
}

func fullFixture() domain.SpecIndex {
	input := domain.SpecIndex{
		ProjectID: "payments", RevisionID: "rev-0001", Title: "Payments <API>\u2028", Version: "2026-07",
		Branding: domain.DocsBranding{DisplayName: "Payments", Logo: domain.DocsBrandingLogo{Src: "/logo.svg", Alt: "Payments", HomeURL: "/"}, Favicon: "/favicon.svg"},
		Overview: domain.SpecOverview{
			Description: "Payments\nAPI", TermsOfService: "/terms",
			Contact: domain.SpecContact{Name: "API team", URL: "/contact", Email: "api@example.com"},
			License: domain.SpecLicense{Name: "MIT", URL: "/license", Identifier: "MIT"},
			Servers: []domain.SpecServer{{URL: "https://api.example", Variables: []domain.SpecServerVariable{{Name: "region", Default: "us", Enum: []string{"us", "eu"}}}}, {URL: "https://sandbox.example"}},
		},
		SpecDownload:    domain.SpecDownload{Filename: "openapi.json", JSON: []byte("__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__")},
		ExampleSpecJSON: "__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__",
		Operations: []domain.Operation{
			{ID: "createPet", Anchor: "operation-create-pet", Method: "POST", Path: "/pets", Summary: "Create pet", Tags: []string{"Pets", "Admin", "Pets"}, Parameters: []domain.OperationParameter{{Name: "trace", In: "header", Example: "true"}, {Name: "dryRun", In: "query", Schema: domain.SchemaSummary{Default: "1e+03", Example: "{\"looks\":\"json\"}"}}}, RequestBody: &domain.OperationRequestBody{Required: true, MediaTypes: []domain.OperationMediaType{{ContentType: "application/json", Schema: domain.SchemaSummary{JSON: "{\"z\":1e+03,\"a\":1e-3}"}, Example: "", ExampleProvided: true}}}, Responses: []domain.OperationResponse{{Status: "500"}, {Status: "201"}}, Security: []domain.OperationSecurity{{Name: "oauth", Scopes: []string{"write", "read"}}}, Snippets: []domain.RequestSnippet{{Label: "cURL", Language: "shell", Code: "curl"}, {Label: "JavaScript", Language: "javascript", Code: "fetch()"}}},
			{ID: "listPets", Anchor: "operation-list-pets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
		},
		Schemas: []domain.Schema{
			{Name: "Pet", Description: "A pet", Summary: domain.SchemaSummary{Type: "object", JSON: "{\"script\":\"<script>\",\"n\":-0}", Properties: []domain.SchemaProperty{{Name: "id", Required: true, Schema: domain.SchemaSummary{Type: "string"}}}}, Example: domain.SchemaExample{JSON: "{\"shape\":1.0}", Example: "__EXPLICIT_PET__", Provided: true}},
			{Name: "Error", Summary: domain.SchemaSummary{Type: "object", Items: &domain.SchemaSummary{Type: "string"}}},
		},
		Search:       []domain.SearchDocument{{ID: "schema-pet", Kind: "Schema", Href: "#schema-pet"}, {ID: "operation-list", Kind: "Operation", Href: "#operation-list-pets"}, {ID: "overview", Kind: "Overview", Href: "#overview"}},
		PublicRoutes: []domain.PublicRoute{{Path: "/", Title: "Payments"}, {Path: "/?selected=operation-create-pet#operation-create-pet", Title: "Create pet"}, {Path: "/?selected=operation-list-pets#operation-list-pets", Title: "List pets"}, {Path: "/?selected=schema-pet#schema-pet", Title: "Pet"}},
	}
	return input
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	return mustReadPath(t, fixturePath(name))
}

func mustReadPath(t *testing.T, path string) []byte {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "application", "projection", "testdata", name)
}

func marshalUnchecked(t *testing.T, document projection.Document) []byte {
	t.Helper()
	bytes, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

func fixtureDigest(input []byte) string {
	hash := sha256.Sum256(input)
	return hex.EncodeToString(hash[:])
}

var _ = context.Background
