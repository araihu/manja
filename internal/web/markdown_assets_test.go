package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/araihu/manja/domain"
	localrender "github.com/araihu/manja/internal/localdocs/render"
	"github.com/araihu/margo"
	"strings"
)

func TestMargoStylesAreSharedLocalAssets(t *testing.T) {
	asset, err := margo.EmbeddedAsset("document.css")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewCatalogAssetsHandler()
	for _, path := range []string{"/manja-assets/margo/document.css", "/manja-assets/margo/", "/manja-assets/margo/document.css?x=1", "/manja-assets/margo/unknown.js"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if path == "/manja-assets/margo/document.css" {
			if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), asset.Content) {
				t.Fatalf("stylesheet %d", rec.Code)
			}
		} else if rec.Code != 404 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	count := 0
	for _, path := range CatalogAssetPaths() {
		if path == "/manja-assets/margo/document.css" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("stylesheet export entries: %d", count)
	}
}

func TestNestedSchemaDescriptionsUseMargoSlots(t *testing.T) {
	schema := domain.SchemaSummary{Name: "RunnerGroup", Type: "object", Properties: []domain.SchemaProperty{
		{Name: "visibility", Schema: domain.SchemaSummary{Type: "string", Description: "Select **visibility**: `all`, `selected`, or `private`."}},
		{Name: "settings", Schema: domain.SchemaSummary{Type: "object", Description: "Nested **settings**.", Properties: []domain.SchemaProperty{
			{Name: "enabled", Schema: domain.SchemaSummary{Type: "boolean", Description: "Set to `true`."}},
		}}},
	}}
	request := operationMarkdownRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	var output bytes.Buffer
	if err := localrender.SchemaTreeFromSummary("runner", "Request", schema, nil, "", "", "").Render(request.Context(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<strong>visibility</strong>", `<code title="all">all</code>`, "<strong>settings</strong>", `<code title="true">true</code>`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %s in %s", want, output.String())
		}
	}
}

func TestPathParametersUseSchemaTreeAndMarkdown(t *testing.T) {
	parameters := []domain.OperationParameter{{Name: "owner", In: "path", Required: true, Description: "The **owner** of the `repository`.", Example: "octocat", Schema: domain.SchemaSummary{Type: "string", Default: "default-owner", Enum: []string{"octocat", "hubot"}, Constraints: []domain.SchemaConstraint{{Name: "minLength", Value: "1"}}}}}
	request := operationMarkdownRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	var output bytes.Buffer
	if err := localrender.PathParameterTree("test-path", parameters).Render(request.Context(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="gs-schema-tree `, `id="test-path-path-owner"`, `data-manja-parameter-row`, `data-required="true"`, `<strong>owner</strong>`, `title="repository"`, `octocat`, `default-owner`, `minLength`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %s in %s", want, output.String())
		}
	}
}
