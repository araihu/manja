//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/selfhosted"
)

func TestKubernetesCatalog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	fixture := filepath.Join("..", "renderer", "testdata", "kubernetes")
	handler, receipts, err := selfhosted.NewRenderer(ctx, selfhosted.RendererOptions{
		ConfigPath: filepath.Join(fixture, "renderer.yaml"),
		DataDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].CatalogID != "kubernetes" || receipts[0].Mount != "/" || receipts[0].SnapshotID == "" {
		t.Fatalf("activation receipts = %#v", receipts)
	}

	overview := catalogRequest(t, handler, http.MethodGet, "/")
	if overview.Code != http.StatusOK || overview.Body.Len() > 512<<10 || strings.Count(overview.Body.String(), "<") > 2500 {
		t.Fatalf("overview = %d bytes=%d tags=%d", overview.Code, overview.Body.Len(), strings.Count(overview.Body.String(), "<"))
	}
	for _, want := range []string{`data-catalog-search-shortcut="true"`, `aria-label="Open API sections"`, `id="darkModeToggleBtn"`} {
		if !strings.Contains(overview.Body.String(), want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if strings.Contains(overview.Body.String(), `id="manja-theme-trigger"`) {
		t.Fatal("overview contains removed theme selector")
	}

	stable := catalogRequest(t, handler, http.MethodGet, "/catalog.json")
	if stable.Code != http.StatusTemporaryRedirect || stable.Header().Get("Location") == "" {
		t.Fatalf("stable catalog = %d location=%q", stable.Code, stable.Header().Get("Location"))
	}
	exact := catalogRequest(t, handler, http.MethodGet, stable.Header().Get("Location"))
	if exact.Code != http.StatusOK || exact.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("exact catalog = %d headers=%v", exact.Code, exact.Header())
	}
	directory, err := catalogjson.DecodeCatalog(exact.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	operationCount, schemaCount := 0, 0
	visible := make(map[domain.DetailID]string, 3028)
	maxDocumentBytes := 0
	for _, document := range directory.Documents {
		documentPage := catalogRequest(t, handler, http.MethodGet, "/documents/"+url.PathEscape(document.Key)+"/")
		if documentPage.Code != http.StatusOK || documentPage.Body.Len() > 512<<10 || !strings.Contains(documentPage.Body.String(), `data-catalog-document="`+document.Key+`"`) {
			t.Fatalf("document %q = %d bytes=%d", document.Key, documentPage.Code, documentPage.Body.Len())
		}
		if documentPage.Body.Len() > maxDocumentBytes {
			maxDocumentBytes = documentPage.Body.Len()
		}
		operationCount += len(document.Operations)
		schemaCount += len(document.Schemas)
		for _, operation := range document.Operations {
			assertCatalogDetailDirectory(t, document.Key, operation.DetailID, operation.Href, visible)
		}
		for _, schema := range document.Schemas {
			assertCatalogDetailDirectory(t, document.Key, schema.DetailID, schema.Href, visible)
		}
		if len(document.Operations) > 0 {
			assertVisibleCatalogDetail(t, handler, document.Operations[0].Href, document.Operations[0].DetailID, "operation")
		}
		if len(document.Schemas) > 0 {
			assertVisibleCatalogDetail(t, handler, document.Schemas[0].Href, document.Schemas[0].DetailID, "schema")
		}
	}
	if len(directory.Documents) != 65 || operationCount != 1202 || schemaCount != 1826 || len(visible) != operationCount+schemaCount {
		t.Fatalf("catalog totals = documents:%d operations:%d schemas:%d visible:%d", len(directory.Documents), operationCount, schemaCount, len(visible))
	}

	maxSearchBytes := 0
	for detailID := range visible {
		response := catalogRequest(t, handler, http.MethodGet, "/search.json?q="+url.QueryEscape(string(detailID)))
		if response.Code != http.StatusOK || response.Body.Len() > 64<<10 {
			t.Fatalf("exact search %q = %d bytes=%d", detailID, response.Code, response.Body.Len())
		}
		if response.Body.Len() > maxSearchBytes {
			maxSearchBytes = response.Body.Len()
		}
		var result struct {
			CatalogID  string             `json:"catalogId"`
			SnapshotID catalog.SnapshotID `json:"snapshotId"`
			Version    uint32             `json:"searchVersion"`
			Results    []struct {
				DetailID domain.DetailID `json:"detailId"`
				Href     string          `json:"href"`
			} `json:"results"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.CatalogID != "kubernetes" || string(result.SnapshotID) != receipts[0].SnapshotID || result.Version != 1 || len(result.Results) == 0 {
			t.Fatalf("exact search %q identity/results = %#v", detailID, result)
		}
		if expected, exists := visible[result.Results[0].DetailID]; !exists || result.Results[0].Href != "/"+expected {
			t.Fatalf("exact search %q returned non-visible target %#v", detailID, result.Results[0])
		}
	}

	for _, searchCase := range []struct {
		query       string
		documentKey string
		title       string
	}{
		{query: "listCoreV1NamespacedPod", documentKey: "core-v1", title: "Pod"},
		{query: "/api/v1/namespaces/{namespace}/pods", documentKey: "core-v1", title: "Pod"},
		{query: "GET /api/v1/namespaces/{namespace}/pods", documentKey: "core-v1", title: "Pod"},
		{query: "PodSpec", title: "PodSpec"},
		{query: "Deployment", documentKey: "apps-v1", title: "Deployment"},
		{query: "apps v1 deployment", documentKey: "apps-v1", title: "Deployment"},
		{query: "readAppsV1NamespacedDeployment", documentKey: "apps-v1", title: "Deployment"},
	} {
		assertKubernetesSearchResult(t, handler, searchCase.query, searchCase.documentKey, searchCase.title)
	}

	for _, route := range []string{"/manage", "/manage/specs", "/api/specs"} {
		if response := catalogRequest(t, handler, http.MethodGet, route); response.Code != http.StatusNotFound {
			t.Errorf("renderer-only route %q = %d, want 404", route, response.Code)
		}
	}
	t.Logf("Kubernetes catalog receipt: snapshot=%s documents=65 operations=1202 schemas=1826 max-document-html=%d max-exact-search-json=%d", receipts[0].SnapshotID, maxDocumentBytes, maxSearchBytes)
}

func assertCatalogDetailDirectory(t *testing.T, documentKey string, detailID domain.DetailID, href string, visible map[domain.DetailID]string) {
	t.Helper()
	want := "documents/" + documentKey + "/?selected=" + string(detailID) + "#" + string(detailID)
	if href != want {
		t.Fatalf("detail %q href = %q, want %q", detailID, href, want)
	}
	if _, exists := visible[detailID]; exists {
		t.Fatalf("detail ID %q is duplicated", detailID)
	}
	visible[detailID] = href
}

func assertVisibleCatalogDetail(t *testing.T, handler http.Handler, href string, detailID domain.DetailID, kind string) {
	t.Helper()
	path := "/" + strings.Split(href, "#")[0]
	response := catalogRequest(t, handler, http.MethodGet, path)
	if response.Code != http.StatusOK || response.Body.Len() > 512<<10 || !strings.Contains(response.Body.String(), `id="`+string(detailID)+`"`) || !strings.Contains(response.Body.String(), `data-catalog-detail="`+kind+`"`) {
		t.Fatalf("%s detail %q = %d bytes=%d", kind, detailID, response.Code, response.Body.Len())
	}
}

func assertKubernetesSearchResult(t *testing.T, handler http.Handler, query, documentKey, title string) {
	t.Helper()
	response := catalogRequest(t, handler, http.MethodGet, "/search.json?q="+url.QueryEscape(query))
	if response.Code != http.StatusOK {
		t.Fatalf("search %q = %d %q", query, response.Code, response.Body.String())
	}
	var payload struct {
		Results []struct {
			DocumentKey string `json:"documentKey"`
			Title       string `json:"title"`
		} `json:"results"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, result := range payload.Results {
		if (documentKey == "" || result.DocumentKey == documentKey) && strings.Contains(strings.ToLower(result.Title), strings.ToLower(title)) {
			return
		}
	}
	t.Fatalf("search %q did not find document=%q title~=%q in %#v", query, documentKey, title, payload.Results)
}

func catalogRequest(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, target, nil))
	return response
}
