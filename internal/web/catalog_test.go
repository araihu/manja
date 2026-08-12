package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	localrender "github.com/araihu/manja/internal/localdocs/render"
	"github.com/araihu/manja/internal/web/templates"
)

func TestCatalogRouteMatrixForRootAndNestedMounts(t *testing.T) {
	t.Parallel()

	for _, mount := range []string{"/", "/kubernetes"} {
		t.Run(mount, func(t *testing.T) {
			handler, snapshot := catalogHandlerFixture(t, mount)
			base := mount
			if base != "/" {
				base += "/"
			}
			for _, test := range []struct {
				method string
				path   string
				status int
			}{
				{http.MethodGet, base, http.StatusOK},
				{http.MethodHead, base, http.StatusOK},
				{http.MethodGet, base + "core-v1/", http.StatusOK},
				{http.MethodGet, base + "documents/core-v1/", http.StatusOK},
				{http.MethodGet, base + "core-v1/?selected=detail-sha256-" + strings.Repeat("a", 64), http.StatusOK},
				{http.MethodGet, base + "search", http.StatusOK},
				{http.MethodGet, base + "search?q=listCoreV1Pod", http.StatusOK},
				{http.MethodGet, base + "search.json?q=listCoreV1Pod", http.StatusOK},
				{http.MethodGet, base + "missing", http.StatusNotFound},
				{http.MethodGet, base + "missing/", http.StatusNotFound},
				{http.MethodPost, base, http.StatusMethodNotAllowed},
			} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
				if response.Code != test.status {
					t.Errorf("%s %s = %d, want %d; body=%s", test.method, test.path, response.Code, test.status, response.Body.String())
				}
				if test.status == http.StatusOK && response.Header().Get("Cache-Control") != "private, no-cache" {
					t.Errorf("%s cache = %q", test.path, response.Header().Get("Cache-Control"))
				}
				if mount == "/" && test.method == http.MethodGet && test.path == base {
					body := response.Body.String()
					if !strings.Contains(body, `data-catalog-overview="true"`) || strings.Contains(body, `id="catalog-organization-navigation"`) {
						t.Errorf("root-mounted catalog response did not remain a catalog overview: %q", body)
					}
				}
				if mount == "/" && strings.Contains(test.path, "documents/core-v1") && strings.Contains(response.Body.String(), `id="catalog-organization-navigation"`) {
					t.Errorf("document response replaced operation sidebar with organization navigation")
				}
			}
			if mount != "/" {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, strings.TrimSuffix(base, "/"), nil))
				if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != base {
					t.Fatalf("mount redirect = %d %q", response.Code, response.Header().Get("Location"))
				}
			}
			_ = snapshot
		})
	}
}

func TestOrganizationRootCatalogCardNavigatesToCatalogOverview(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))
	if root.Code != http.StatusOK {
		t.Fatalf("organization root = %d body=%q", root.Code, root.Body.String())
	}
	if !strings.Contains(root.Body.String(), `id="catalog-organization-navigation"`) || !strings.Contains(root.Body.String(), `data-catalog-organization-item="catalog-kubernetes"`) || !strings.Contains(root.Body.String(), `href="/kubernetes/"`) {
		t.Fatalf("organization root card missing catalog overview target: %q", root.Body.String())
	}
	if strings.Contains(root.Body.String(), `id="organization-catalogs-heading"`) || strings.Contains(root.Body.String(), `id="organization-specs-heading"`) {
		t.Fatal("organization root main content duplicated sidebar catalogs or specs")
	}

	catalogRoot := httptest.NewRecorder()
	handler.ServeHTTP(catalogRoot, httptest.NewRequest(http.MethodGet, "/kubernetes/", nil))
	if catalogRoot.Code != http.StatusOK || !strings.Contains(catalogRoot.Body.String(), `data-catalog-overview="true"`) {
		t.Fatalf("catalog overview = %d body=%q", catalogRoot.Code, catalogRoot.Body.String())
	}
	if strings.Contains(catalogRoot.Body.String(), `id="catalog-readme-heading"`) || strings.Contains(catalogRoot.Body.String(), `id="catalog-license-heading"`) {
		t.Fatal("catalog overview rendered undeclared README or license")
	}
	globalSearch := httptest.NewRecorder()
	handler.ServeHTTP(globalSearch, httptest.NewRequest(http.MethodGet, "/search.json?q=listCoreV1Pod", nil))
	if globalSearch.Code != http.StatusOK || !strings.Contains(globalSearch.Body.String(), `"catalogId":"global"`) || !strings.Contains(globalSearch.Body.String(), `/kubernetes/documents/core-v1/?selected=`) {
		t.Fatalf("organization root global search = %d body=%q", globalSearch.Code, globalSearch.Body.String())
	}
}

func TestCatalogOverviewDocumentTableSortsThroughHTMXFragment(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	request := httptest.NewRequest(http.MethodGet, "/kubernetes/?table_id=catalog-documents-table&order_by=operations&order_dir=desc", nil)
	request.Header.Set("HX-Request", "true")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("sorted table = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, `data-catalog-overview="true"`) {
		t.Fatal("sorted table response rendered the full catalog page")
	}
	core := strings.Index(body, `data-search-text="core-v1 v1"`)
	apps := strings.Index(body, `data-search-text="apps-v1 v1"`)
	if core < 0 || apps < 0 || core > apps {
		t.Fatalf("operations descending order = core %d apps %d", core, apps)
	}
	for _, want := range []string{`id="catalog-documents-table-thead"`, `hx-swap-oob="innerHTML"`, `data-table-sort-by="operations"`, `data-table-sort-dir="desc"`, `table_id=catalog-documents-table`, `order_by=name`} {
		if !strings.Contains(body, want) {
			t.Errorf("sorted table response missing %q", want)
		}
	}
}

func TestCatalogSelectedMainTargetReturnsOnlyMainFragment(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	detailID := "detail-sha256-" + strings.Repeat("a", 64)
	request := httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/?selected="+detailID, nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "catalog-main-content")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("catalog main fragment = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		`id="catalog-main-content"`,
		`data-catalog-main-content="true"`,
		`data-document-title="List Pods · Manja"`,
		`data-catalog-detail="operation"`,
		`hx-target="#catalog-main-content"`,
		`hx-select="#catalog-main-content"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog main fragment missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{
		`<!doctype html>`,
		`<html`,
		`id="main-content"`,
		`id="catalog-navigation"`,
		`id="catalog-sidebar-groups"`,
		`/manja-assets/request-composer.js`,
		`hx-target="#main-content"`,
		`hx-select="#main-content"`,
	} {
		if strings.Contains(body, reject) {
			t.Errorf("catalog main fragment retained shell marker %q:\n%s", reject, body)
		}
	}
	for _, vary := range []string{"HX-Request", "HX-Boosted", "HX-Target", "HX-History-Restore-Request", "Accept-Encoding"} {
		if !strings.Contains(response.Header().Get("Vary"), vary) {
			t.Errorf("catalog fragment Vary = %q, missing %q", response.Header().Get("Vary"), vary)
		}
	}
}

func TestCatalogSidebarTargetReturnsOnlySidebarFragment(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	request := httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/?group=group-sidebar", nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "catalog-sidebar-groups")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("catalog sidebar fragment = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `id="catalog-sidebar-groups"`) {
		t.Fatalf("catalog sidebar fragment missing target:\n%s", body)
	}
	for _, reject := range []string{`<!doctype html>`, `<html`, `id="main-content"`, `id="catalog-navigation"`, `id="catalog-main-content"`, `/manja-assets/request-composer.js`} {
		if strings.Contains(body, reject) {
			t.Errorf("catalog sidebar fragment retained shell marker %q:\n%s", reject, body)
		}
	}
}

func TestCatalogSchemaNodeTargetReturnsOnlySchemaNodeFragment(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	schemaID := "detail-sha256-" + strings.Repeat("c", 64)
	request := httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/?selected="+schemaID+"&node=1", nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "schema-node-panel")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("catalog schema node fragment = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`id="schema-node-panel"`, `ObjectMeta`, `data-catalog-schema-node-focus="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("catalog schema node fragment missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{`<!doctype html>`, `<html`, `id="main-content"`, `id="catalog-navigation"`, `id="catalog-main-content"`, `id="catalog-sidebar-groups"`} {
		if strings.Contains(body, reject) {
			t.Errorf("catalog schema node fragment retained shell marker %q:\n%s", reject, body)
		}
	}
}

func TestLocalSchemaNodeRendererMatchesSSRFragmentBytes(t *testing.T) {
	t.Parallel()

	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	catalogHandler := handler.(*CatalogHandler)
	document := snapshot.Directory.Documents[1]
	schemaID := document.Schemas[0].DetailID
	detail, err := catalogHandler.loadCatalogDetail(context.Background(), snapshot, document.Schemas[0].DetailChild, schemaID)
	if err != nil {
		t.Fatal(err)
	}
	node, shard, err := catalogHandler.loadCatalogSchemaNode(context.Background(), snapshot, document, 1)
	if err != nil {
		t.Fatal(err)
	}
	references := []projection.SchemaNode{shard.Nodes[2]}
	fragment, err := localrender.PrepareSchemaNode(detail, node, references, "/kubernetes/documents/core-v1/")
	if err != nil {
		t.Fatal(err)
	}
	localBody, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/?selected="+string(schemaID)+"&node=1", nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "schema-node-panel")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("SSR schema-node fragment = %d body=%q", response.Code, response.Body.String())
	}
	if !bytes.Equal(localBody, response.Body.Bytes()) {
		t.Fatalf("local and SSR schema-node fragments differ:\nlocal=%s\nSSR=%s", localBody, response.Body.Bytes())
	}
}

func TestCatalogMainFragmentRequiresDirectNonRestoreRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{name: "missing target", headers: map[string]string{"HX-Request": "true"}},
		{name: "unknown target", headers: map[string]string{"HX-Request": "true", "HX-Target": "unknown-target"}},
		{name: "boosted", headers: map[string]string{"HX-Request": "true", "HX-Target": "catalog-main-content", "HX-Boosted": "true"}},
		{name: "history restore", headers: map[string]string{"HX-Request": "true", "HX-Target": "catalog-main-content", "HX-History-Restore-Request": "true"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, _ := catalogHandlerFixture(t, "/kubernetes")
			request := httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/", nil)
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("catalog page = %d body=%q", response.Code, response.Body.String())
			}
			body := response.Body.String()
			for _, want := range []string{`<!doctype html>`, `id="main-content"`, `id="catalog-navigation"`, `id="catalog-main-content"`} {
				if !strings.Contains(body, want) {
					t.Errorf("catalog fallback missing %q", want)
				}
			}
		})
	}
}

func TestCatalogOverviewDocumentTableRejectsInvalidSort(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/kubernetes/?table_id=catalog-documents-table&order_by=source&order_dir=asc", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort = %d, want 400", response.Code)
	}
}

func TestOrganizationRootRendersOnlyOptedInMetadata(t *testing.T) {
	t.Parallel()

	organization := OrganizationPresentation{
		Title: "Example APIs", Readme: "Public API documentation.",
		License: OrganizationLicensePresentation{Name: "Apache-2.0", URL: "https://example.test/license"},
		Sources: []OrganizationSourcePresentation{{Name: "Definitions", Kind: "git", Location: "github.com/example/apis", URL: "https://github.com/example/apis"}},
		SEO:     CatalogPresentation{Description: "Example API documentation.", CanonicalBase: "https://example.test", SocialImage: "https://example.test/social.png", SocialImageMIMEType: "image/png", SocialImageAlt: "Example APIs"},
	}
	handler, _ := catalogHandlerFixtureWithOrganization(t, "/kubernetes", nil, organization)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://example.test/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("organization root = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{"Example APIs", "Public API documentation.", "Apache-2.0", "Definitions", "github.com/example/apis", `<meta name="description" content="Example API documentation.">`, `<link rel="canonical" href="https://example.test/">`} {
		if !strings.Contains(body, want) {
			t.Errorf("organization root missing %q", want)
		}
	}
}

func TestCatalogSearchRendersGlobalResultsAndBoundsFailures(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/kubernetes/search?q=listCoreV1Pod", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "List Pods") || !strings.Contains(response.Body.String(), `/kubernetes/documents/core-v1/?selected=`) {
		t.Fatalf("search response = %d %q", response.Code, response.Body.String())
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/kubernetes/search?q="+strings.Repeat("x", 257), nil))
	if invalid.Code != http.StatusBadRequest || invalid.Body.Len() > 1024 {
		t.Fatalf("invalid search = %d bytes=%d", invalid.Code, invalid.Body.Len())
	}
}

func TestCatalogSearchJSONReturnsVersionedGlobalResults(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	requestURL := "/kubernetes/search.json?q=listCoreV1Pod"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestURL, nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("search JSON = %d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if response.Body.Len() > 64<<10 {
		t.Fatalf("search JSON = %d bytes, want at most 64 KiB", response.Body.Len())
	}
	var payload catalogSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.CatalogID != "global" || payload.SnapshotID != "" || payload.Version != 1 || payload.Query != "listcorev1pod" {
		t.Fatalf("search JSON identity = %+v", payload)
	}
	if len(payload.Results) != 1 || payload.Results[0].Title != "List Pods" || payload.Results[0].Section != "Kubernetes" || !strings.HasPrefix(payload.Results[0].Href, "/kubernetes/documents/core-v1/?selected=") {
		t.Fatalf("search JSON results = %+v", payload.Results)
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, requestURL, nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("ETag") != response.Header().Get("ETag") || head.Header().Get("Content-Length") != response.Header().Get("Content-Length") {
		t.Fatalf("search JSON HEAD = %d bytes=%d headers=%v", head.Code, head.Body.Len(), head.Header())
	}
}

func TestCatalogExactDetailSearchDoesNotDependOnSearchChildren(t *testing.T) {
	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	catalogHandler := handler.(*CatalogHandler)
	searchChildReads := 0
	catalogHandler.children = deadlineSearchCatalogChildren{
		fallback: catalogHandler.children,
		reads:    &searchChildReads,
	}

	detailID := snapshot.Directory.Documents[1].Operations[0].DetailID
	exact := httptest.NewRecorder()
	handler.ServeHTTP(exact, httptest.NewRequest(http.MethodGet, "/kubernetes/search.json?q="+string(detailID), nil))
	if exact.Code != http.StatusOK || !strings.Contains(exact.Body.String(), `"detailId":"`+string(detailID)+`"`) {
		t.Fatalf("exact detail search = %d body=%q", exact.Code, exact.Body.String())
	}
	if searchChildReads != 0 {
		t.Fatalf("exact detail search read %d search children, want directory-only lookup", searchChildReads)
	}

	nonExact := httptest.NewRecorder()
	handler.ServeHTTP(nonExact, httptest.NewRequest(http.MethodGet, "/kubernetes/search.json?q=listCoreV1Pod", nil))
	if nonExact.Code != http.StatusServiceUnavailable || nonExact.Header().Get("Retry-After") != "1" {
		t.Fatalf("non-exact search with unavailable children = %d retry=%q body=%q", nonExact.Code, nonExact.Header().Get("Retry-After"), nonExact.Body.String())
	}
	if searchChildReads == 0 {
		t.Fatal("non-exact search did not exercise unavailable search children")
	}
}

func TestCatalogExactDetailSearchUsesCanonicalValidation(t *testing.T) {
	detailID := "detail-sha256-" + strings.Repeat("a", 64)
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantDetail bool
	}{
		{
			name:       "lowercase exact",
			query:      detailID,
			wantStatus: http.StatusOK,
			wantDetail: true,
		},
		{
			name:       "uppercase normalized exact",
			query:      strings.ToUpper(detailID),
			wantStatus: http.StatusOK,
			wantDetail: true,
		},
		{
			name:       "over byte limit after trim",
			query:      strings.Repeat(" ", 257-len(detailID)) + detailID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "control wrapped",
			query:      "\n" + detailID + "\n",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "NFKC equivalent",
			query:      fullWidthASCII(detailID),
			wantStatus: http.StatusOK,
			wantDetail: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, _ := catalogHandlerFixture(t, "/kubernetes")
			catalogHandler := handler.(*CatalogHandler)
			searchChildReads := 0
			catalogHandler.children = deadlineSearchCatalogChildren{
				fallback: catalogHandler.children,
				reads:    &searchChildReads,
			}

			response := httptest.NewRecorder()
			target := "/kubernetes/search.json?q=" + url.QueryEscape(test.query)
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			if response.Code != test.wantStatus {
				t.Fatalf("canonical exact search = %d body=%q, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
			if test.wantDetail && !strings.Contains(response.Body.String(), `"detailId":"`+detailID+`"`) {
				t.Fatalf("canonical exact search body=%q, want detail %q", response.Body.String(), detailID)
			}
			if searchChildReads != 0 {
				t.Fatalf("canonical exact search read %d search children, want none", searchChildReads)
			}
		})
	}
}

func TestCatalogMalformedDetailQueriesSkipExactDirectoryTraversal(t *testing.T) {
	validDetailID := "detail-sha256-" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name  string
		query string
	}{
		{name: "wrong prefix", query: "details-sha256-" + strings.Repeat("a", 64)},
		{name: "wrong length", query: "detail-sha256-" + strings.Repeat("a", 63)},
		{name: "non-hex", query: "detail-sha256-" + strings.Repeat("a", 63) + "g"},
		{name: "suffix", query: validDetailID + "-extra"},
		{name: "ordinary text", query: "pod"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
			catalogHandler := handler.(*CatalogHandler)

			decoy := snapshot
			decoy.ID = "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("d", 64))
			decoy.Directory.Documents[1].Operations[0].DetailID = domain.DetailID(test.query)
			search, err := catalog.BuildSearchArtifacts(decoy.Directory, catalog.DefaultBounds())
			if err != nil {
				t.Fatal(err)
			}
			decoy.Search = search.Directory
			if _, err := catalogHandler.runtime.ActivateMount("/kubernetes", snapshot.ID, 1, decoy); err != nil {
				t.Fatal(err)
			}

			searchChildReads := 0
			catalogHandler.children = deadlineSearchCatalogChildren{
				fallback: catalogHandler.children,
				reads:    &searchChildReads,
			}
			response := httptest.NewRecorder()
			target := "/kubernetes/search.json?q=" + url.QueryEscape(test.query)
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Retry-After") != "1" {
				t.Fatalf("non-detail query = %d retry=%q body=%q, want bounded search 503", response.Code, response.Header().Get("Retry-After"), response.Body.String())
			}
			if searchChildReads == 0 {
				t.Fatal("non-detail query did not reach bounded search children")
			}
		})
	}
}

func TestCatalogExactDetailSearchCollectsAndRanksAcrossMounts(t *testing.T) {
	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	catalogHandler := handler.(*CatalogHandler)
	second := snapshot
	second.ID = "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("d", 64))
	second.Directory.CatalogID = "other"
	second.Directory.Title = "Other"
	if _, err := catalogHandler.runtime.ActivateMount("/other", "", 1, second); err != nil {
		t.Fatal(err)
	}

	detailID := snapshot.Directory.Documents[1].Operations[0].DetailID
	response := httptest.NewRecorder()
	target := "/kubernetes/search.json?q=" + url.QueryEscape(string(detailID))
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("cross-catalog exact search = %d body=%q", response.Code, response.Body.String())
	}
	var payload catalogSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 2 || payload.Results[0].DetailID != detailID || payload.Results[1].DetailID != detailID {
		t.Fatalf("cross-catalog exact results = %#v, want both detail matches", payload.Results)
	}
	if payload.Results[0].Section != "Kubernetes" || !strings.HasPrefix(payload.Results[0].Href, "/kubernetes/") || payload.Results[1].Section != "Other" || !strings.HasPrefix(payload.Results[1].Href, "/other/") {
		t.Fatalf("cross-catalog exact ranking = %#v, want context mount first", payload.Results)
	}
}

func TestGlobalSearchRankingUsesKindAndPageContext(t *testing.T) {
	t.Parallel()

	candidates := []globalSearchCandidate{
		{record: catalog.SearchRecordV1{DetailID: "schema-other", Kind: "schema", Title: "Shared schema"}, mount: "/other", localRank: 0},
		{record: catalog.SearchRecordV1{DetailID: "operation-other", Kind: "operation", Title: "Other operation", DocumentKey: "other-v1"}, mount: "/other", localRank: 0},
		{record: catalog.SearchRecordV1{DetailID: "schema-current", Kind: "schema", Title: "Current schema", DocumentKey: "current-v1"}, mount: "/current", localRank: 0},
		{record: catalog.SearchRecordV1{DetailID: "document-current", Kind: "document", Title: "current-v1", DocumentKey: "current-v1"}, mount: "/current", localRank: 1},
		{record: catalog.SearchRecordV1{DetailID: "operation-current", Kind: "operation", Title: "Current operation", DocumentKey: "current-v1"}, mount: "/current", localRank: 2},
	}

	rootResults := append([]globalSearchCandidate(nil), candidates...)
	rankGlobalSearchCandidates(rootResults, "", "")
	if rootResults[0].record.Kind != "operation" || rootResults[1].record.Kind != "operation" {
		t.Fatalf("workspace ranking = %+v, want operations first", rootResults)
	}

	pageResults := append([]globalSearchCandidate(nil), candidates...)
	rankGlobalSearchCandidates(pageResults, "/current", "current-v1")
	if pageResults[0].record.DetailID != "operation-current" {
		t.Fatalf("spec-page ranking first = %+v, want current operation", pageResults[0])
	}
	currentSchemaIndex := -1
	otherSchemaIndex := -1
	for index, candidate := range pageResults {
		if candidate.record.DetailID == "schema-current" {
			currentSchemaIndex = index
		}
		if candidate.record.DetailID == "schema-other" {
			otherSchemaIndex = index
		}
	}
	if currentSchemaIndex < 0 || otherSchemaIndex < 0 || currentSchemaIndex >= otherSchemaIndex {
		t.Fatalf("spec-page schema context ranking = %+v", pageResults)
	}

	exactResults := []globalSearchCandidate{
		{record: catalog.SearchRecordV1{DetailID: "schema-exact", Kind: "schema", Title: "Exact schema"}, exactID: true},
		{record: catalog.SearchRecordV1{DetailID: "operation-related", Kind: "operation", Title: "Related operation"}},
	}
	rankGlobalSearchCandidates(exactResults, "", "")
	if exactResults[0].record.DetailID != "schema-exact" {
		t.Fatalf("exact detail ranking = %+v, want exact detail first", exactResults)
	}
}

func TestCatalogSearchErrorsHaveStableHTTPClasses(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		err        error
		status     int
		retryAfter string
	}{
		{err: catalog.ErrInvalidQuery, status: http.StatusBadRequest},
		{err: catalog.ErrQueryTooBroad, status: http.StatusUnprocessableEntity},
		{err: catalog.ErrSearchDeadline, status: http.StatusServiceUnavailable, retryAfter: "1"},
	} {
		response := httptest.NewRecorder()
		writeCatalogSearchError(response, test.err)
		if response.Code != test.status || response.Header().Get("Retry-After") != test.retryAfter || response.Body.Len() > 1024 {
			t.Errorf("search error %v = %d retry=%q bytes=%d", test.err, response.Code, response.Header().Get("Retry-After"), response.Body.Len())
		}
	}
}

func TestCatalogDocumentComboboxFiltersDistinctKeysAndEmitsCanonicalSelection(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/_manja/catalog/document-combobox/options?catalog-mount=%2Fkubernetes&q=core", nil))
	if options.Code != http.StatusOK || options.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("combobox options = %d headers=%v body=%q", options.Code, options.Header(), options.Body.String())
	}
	if !strings.Contains(options.Body.String(), ">core-v1</span>") || strings.Contains(options.Body.String(), ">Kubernetes</span>") {
		t.Fatalf("combobox options do not expose distinct document keys: %q", options.Body.String())
	}

	toggle := httptest.NewRecorder()
	body := strings.NewReader("catalog-mount=%2Fkubernetes&value=%2Fkubernetes%2Fdocuments%2Fcore-v1%2F")
	request := httptest.NewRequest(http.MethodPost, "/_manja/catalog/document-combobox/toggle", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(toggle, request)
	if toggle.Code != http.StatusOK || !strings.Contains(toggle.Header().Get("HX-Trigger"), `"values":["/kubernetes/documents/core-v1/"]`) {
		t.Fatalf("combobox toggle = %d trigger=%q body=%q", toggle.Code, toggle.Header().Get("HX-Trigger"), toggle.Body.String())
	}

	overLimit := httptest.NewRecorder()
	handler.ServeHTTP(overLimit, httptest.NewRequest(http.MethodGet, "/_manja/catalog/document-combobox/options?catalog-mount=%2Fkubernetes&q="+strings.Repeat("x", 257), nil))
	if overLimit.Code != http.StatusBadRequest || overLimit.Body.Len() > 1024 {
		t.Fatalf("over-limit combobox query = %d bytes=%d", overLimit.Code, overLimit.Body.Len())
	}
}

func TestCatalogSchemaLoadsOneProgressiveNodeWithNoJSFallback(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	schemaID := "detail-sha256-" + strings.Repeat("c", 64)
	rootURL := "/kubernetes/documents/core-v1/?selected=" + schemaID
	root := httptest.NewRecorder()
	handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, rootURL, nil))
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), `id="schema-node-panel"`) || !strings.Contains(root.Body.String(), "metadata") || !strings.Contains(root.Body.String(), "object") {
		t.Fatalf("schema root = %d %q", root.Code, root.Body.String())
	}
	childURL := rootURL + "&node=1#schema-node-panel"
	child := httptest.NewRecorder()
	handler.ServeHTTP(child, httptest.NewRequest(http.MethodGet, strings.Split(childURL, "#")[0], nil))
	if child.Code != http.StatusOK || !strings.Contains(child.Body.String(), "ObjectMeta") || !strings.Contains(child.Body.String(), "resourceVersion") {
		t.Fatalf("schema child = %d %q", child.Code, child.Body.String())
	}
	escapedChildURL := strings.ReplaceAll(childURL, "&", "&amp;")
	if !strings.Contains(root.Body.String(), `hx-select="#schema-node-panel"`) || !strings.Contains(root.Body.String(), `href="`+escapedChildURL+`"`) {
		t.Fatal("progressive schema link lacks HTMX enhancement or normal navigation fallback")
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, rootURL+"&node=99", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing schema node = %d, want 404", missing.Code)
	}
}

func TestCatalogOperationRouteReusesRichEndpointProjection(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	detailID := "detail-sha256-" + strings.Repeat("a", 64)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/?selected="+detailID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("operation detail = %d body=%q", response.Code, response.Body.String())
	}
	for _, want := range []string{
		`class="manja-endpoint-shell-layout"`,
		"Query Parameters",
		`aria-label="Request body"`,
		`aria-label="Request body schema for application/json schema tree"`,
		"metadata",
		"Request body JSON",
		"Request Sample: Shell / cURL",
		`class="manja-endpoint-responses-section grid gap-5"`,
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("rich operation response missing %q", want)
		}
	}
}

func TestCatalogSelectedOperationPreparesRequestBodyMediaSummary(t *testing.T) {
	t.Parallel()

	for _, mount := range []string{"/", "/kubernetes"} {
		mount := mount
		t.Run(mount, func(t *testing.T) {
			t.Parallel()
			handler, snapshot := catalogHandlerFixture(t, mount)
			detailID := "detail-sha256-" + strings.Repeat("a", 64)
			data, err := handler.(*CatalogHandler).catalogPageData(
				context.Background(), snapshot, mount, "core-v1", detailID, "", "", "",
			)
			if err != nil {
				t.Fatal(err)
			}
			if data.OperationRequestBodyMedia == nil {
				t.Fatal("selected operation did not prepare request-body media summary")
			}
			body, err := data.OperationRequestBodyMedia.MediaBytes(context.Background(), 0)
			if err != nil {
				t.Fatal(err)
			}
			documentHref := "/documents/core-v1/"
			if mount != "/" {
				documentHref = mount + documentHref
			}
			for _, want := range []string{
				"application/json",
				`href="` + documentHref + `?selected=detail-sha256-` + strings.Repeat("c", 64),
				`hx-target="#catalog-main-content"`,
				">Pod object<",
			} {
				if !strings.Contains(string(body), want) {
					t.Errorf("prepared catalog request-body media summary missing %q in %s", want, body)
				}
			}
		})
	}
}

func TestCatalogOperationWithoutRequestBodyKeepsMediaFragmentAbsent(t *testing.T) {
	t.Parallel()

	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("f", 64))
	detailBytes, err := catalogjson.EncodeDetailShard(catalog.DetailShardV1{SchemaVersion: 1, DocumentKey: "plain", Records: []catalog.DetailRecordV1{{
		ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
			ID: string(detailID), Anchor: string(detailID), Href: "documents/plain/?selected=" + string(detailID) + "#" + string(detailID),
			HeadingID: string(detailID), Heading: "Ping", HeadingLevel: 2, Method: "GET", Path: "/ping",
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(detailBytes)
	snapshot := catalog.RuntimeSnapshot{
		ID: "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("a", 64)), Location: "/memory",
		Directory: catalog.CatalogArtifactV1{
			SchemaVersion: 1, CatalogID: "plain", Title: "Plain", SearchChild: "search/directory.json",
			Documents: []catalog.DocumentDirectoryV1{{
				Key: "plain", Title: "Plain", SourceChild: "sources/plain.json",
				Operations: []catalog.OperationDirectoryV1{{DetailID: detailID, OperationID: "ping", Method: "GET", Path: "/ping", Title: "Ping", DetailChild: "details/plain.json"}},
			}},
		},
		Manifest: catalog.ManifestV1{SchemaVersion: 1, Children: []catalog.ChildIdentityV1{
			{Path: "search/directory.json", Kind: "search-directory", Length: 2, SHA256: strings.Repeat("e", 64)},
			{Path: "details/plain.json", Kind: "detail", Length: uint64(len(detailBytes)), SHA256: hex.EncodeToString(digest[:])},
		}},
	}
	handler := &CatalogHandler{children: memoryCatalogChildren{"details/plain.json": detailBytes}, details: catalog.NewDetailCache()}
	data, err := handler.catalogPageData(context.Background(), snapshot, "/", "plain", string(detailID), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if data.OperationRequestBodyMedia != nil {
		t.Fatal("operation without request body prepared a media fragment")
	}
}

func TestCatalogInitialHTMLIncludesCompleteRouteSocialMetadata(t *testing.T) {
	t.Parallel()

	presentation := map[string]CatalogPresentation{"/kubernetes": {
		Description: "Browse Kubernetes APIs.", CanonicalBase: "https://docs.example.test/kubernetes",
		SocialImage: "https://docs.example.test/manja-assets/kubernetes-social.png", SocialImageMIMEType: "image/png", SocialImageAlt: "Kubernetes API reference rendered by Manja",
	}}
	handler, _ := catalogHandlerFixtureWithPresentation(t, "/kubernetes", presentation)
	detailID := "detail-sha256-" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name        string
		requestURL  string
		title       string
		description string
		canonical   string
	}{
		{
			name: "catalog root", requestURL: "/kubernetes/", title: "Kubernetes · Manja",
			description: "Browse Kubernetes APIs.", canonical: "https://docs.example.test/kubernetes/",
		},
		{
			name: "document", requestURL: "/kubernetes/documents/core-v1/", title: "core-v1 · Manja",
			description: "OpenAPI operations and schemas for core-v1.", canonical: "https://docs.example.test/kubernetes/documents/core-v1/",
		},
		{
			name: "operation", requestURL: "/kubernetes/documents/core-v1/?selected=" + detailID + "&group=ignored#ignored", title: "List Pods · Manja",
			description: "Lists Pods.", canonical: "https://docs.example.test/kubernetes/documents/core-v1/?selected=" + detailID,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.requestURL, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("route = %d body=%q", response.Code, response.Body.String())
			}
			body := response.Body.String()
			for _, want := range []string{
				`<title>` + test.title + `</title>`,
				`<meta name="description" content="` + test.description + `">`,
				`<link rel="canonical" href="` + test.canonical + `">`,
				`<meta property="og:url" content="` + test.canonical + `">`,
				`<meta property="og:type" content="website">`,
				`<meta property="og:title" content="` + test.title + `">`,
				`<meta property="og:description" content="` + test.description + `">`,
				`<meta property="og:site_name" content="Manja">`,
				`<meta property="og:image" content="https://docs.example.test/manja-assets/kubernetes-social.png">`,
				`<meta property="og:image:type" content="image/png">`,
				`<meta property="og:image:width" content="1280">`,
				`<meta property="og:image:height" content="640">`,
				`<meta property="og:image:alt" content="Kubernetes API reference rendered by Manja">`,
				`<meta name="twitter:card" content="summary_large_image">`,
				`<meta name="twitter:title" content="` + test.title + `">`,
				`<meta name="twitter:description" content="` + test.description + `">`,
				`<meta name="twitter:image" content="https://docs.example.test/manja-assets/kubernetes-social.png">`,
				`<meta name="twitter:image:alt" content="Kubernetes API reference rendered by Manja">`,
			} {
				if count := strings.Count(body, want); count != 1 {
					t.Errorf("metadata %q count = %d, want 1", want, count)
				}
			}
		})
	}
}

func TestOrganizationRootMetadataUsesOriginOnly(t *testing.T) {
	t.Parallel()

	presentation := map[string]CatalogPresentation{
		"/catalogs/alpha": {CanonicalBase: "https://docs.example.test/catalogs/alpha", SocialImage: "https://docs.example.test/catalogs/alpha/catalog.png", SocialImageMIMEType: "image/png"},
	}
	handler, _ := catalogHandlerFixtureWithPresentation(t, "/catalogs/alpha", presentation)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("organization root = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		`<link rel="canonical" href="https://docs.example.test/">`,
		`<meta property="og:url" content="https://docs.example.test/">`,
		`<meta property="og:image" content="https://docs.example.test/manja-assets/manja-social.png">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("organization metadata missing %q", want)
		}
	}
	if strings.Contains(body, "https://docs.example.test/catalogs/alpha/catalog.png") || strings.Contains(body, "https://docs.example.test/catalogs/alpha/") {
		t.Errorf("organization metadata leaked catalog-qualified presentation: %q", body)
	}
}

func TestOrganizationRootMetadataOmitsRelativePresentation(t *testing.T) {
	t.Parallel()

	presentation := map[string]CatalogPresentation{"/catalogs/alpha": {CanonicalBase: "/docs/alpha"}}
	handler, _ := catalogHandlerFixtureWithPresentation(t, "/catalogs/alpha", presentation)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("organization root = %d body=%q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, `rel="canonical"`) || strings.Contains(body, `property="og:image"`) {
		t.Errorf("organization metadata emitted relative presentation: %q", body)
	}
}

func TestLayoutMetadataModeEmitsSiteAndTypeWithoutImageMetadata(t *testing.T) {
	t.Parallel()

	var rendered bytes.Buffer
	err := templates.LayoutWithBrandingMetadata(templates.PageMetadata{
		Title: "Plain docs", Description: "No social image.", CanonicalURL: "https://docs.example.test/plain", SocialImageMIMEType: "image/jpeg",
	}, domain.DocsBranding{}, false, false).Render(context.Background(), &rendered)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:site_name" content="Manja">`,
	} {
		if count := strings.Count(rendered.String(), want); count != 1 {
			t.Errorf("metadata %q count = %d, want 1", want, count)
		}
	}
	if strings.Contains(rendered.String(), `property="og:image:type"`) {
		t.Error("image MIME metadata emitted without an image")
	}
}

func TestCatalogSocialImageMIMETypeUsesPresentationValue(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		imageURL string
		mimeType string
	}{
		{name: "PNG", imageURL: "https://docs.example.test/social.PNG", mimeType: "image/png"},
		{name: "JPEG short", imageURL: "https://docs.example.test/social.jpg", mimeType: "image/jpeg"},
		{name: "JPEG long", imageURL: "https://docs.example.test/social.jpeg", mimeType: "image/jpeg"},
		{name: "WebP", imageURL: "https://docs.example.test/social.webp", mimeType: "image/webp"},
		{name: "unsupported", imageURL: "https://docs.example.test/social.svg"},
	} {
		t.Run(test.name, func(t *testing.T) {
			presentation := map[string]CatalogPresentation{"/kubernetes": {
				Description: "Browse Kubernetes APIs.", CanonicalBase: "https://docs.example.test/kubernetes",
				SocialImage: test.imageURL, SocialImageMIMEType: test.mimeType, SocialImageAlt: "Preview",
			}}
			handler, _ := catalogHandlerFixtureWithPresentation(t, "/kubernetes", presentation)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/kubernetes/", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("route = %d body=%q", response.Code, response.Body.String())
			}
			if test.mimeType == "" {
				if strings.Contains(response.Body.String(), `property="og:image:type"`) {
					t.Error("unsupported image URL emitted MIME metadata")
				}
			} else if count := strings.Count(response.Body.String(), `<meta property="og:image:type" content="`+test.mimeType+`">`); count != 1 {
				t.Errorf("image MIME %q count = %d, want 1", test.mimeType, count)
			}
		})
	}
}

func TestCatalogRenderRejectsResponsesOverHardByteLimit(t *testing.T) {
	t.Parallel()

	data := templates.CatalogPageData{
		Mount:      "/",
		SnapshotID: catalog.SnapshotID("snapshot-sha256-" + strings.Repeat("a", 64)),
		Directory:  catalog.CatalogArtifactV1{Title: strings.Repeat("oversized", maxCatalogPageBytes)},
	}
	response := httptest.NewRecorder()
	(&CatalogHandler{}).renderCatalogPage(response, httptest.NewRequest(http.MethodGet, "/", nil), data)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("oversized render status = %d, want 500", response.Code)
	}
	if response.Body.Len() > 1024 {
		t.Fatalf("oversized render error body = %d bytes, want bounded", response.Body.Len())
	}
}

func TestCatalogMaxOperationGroupRejectsSelectedPageBeyondByteBound(t *testing.T) {
	t.Parallel()

	const operationCount = 20_000
	operations := make([]catalog.OperationDirectoryV1, operationCount)
	for index := range operations {
		detailID := domain.DetailID("detail-sha256-" + fmt.Sprintf("%064x", index+1))
		operations[index] = catalog.OperationDirectoryV1{
			DetailID: detailID, OperationID: fmt.Sprintf("operation%d", index+1), Method: "GET",
			Path: fmt.Sprintf("/items/%d", index+1), Title: fmt.Sprintf("Operation %d", index+1),
			DetailChild: "details/max.json", Tags: []string{"All operations"},
		}
	}
	selected := operations[len(operations)-1]
	detailBytes, err := catalogjson.EncodeDetailShard(catalog.DetailShardV1{SchemaVersion: 1, DocumentKey: "max", Records: []catalog.DetailRecordV1{{
		ID: selected.DetailID, Kind: "operation", Operation: &projection.OperationDetail{
			ID: string(selected.DetailID), Anchor: string(selected.DetailID), Href: "documents/max/?selected=" + string(selected.DetailID) + "#" + string(selected.DetailID),
			HeadingID: string(selected.DetailID), Heading: selected.Title, HeadingLevel: 2,
			Method: selected.Method, Path: selected.Path,
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	detailDigest := sha256.Sum256(detailBytes)
	snapshot := catalog.RuntimeSnapshot{
		ID: "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("f", 64)), Location: "/memory",
		Directory: catalog.CatalogArtifactV1{
			SchemaVersion: 1, CatalogID: "max", Title: "Maximum catalog", SearchChild: "search/directory.json",
			Documents: []catalog.DocumentDirectoryV1{{Key: "max", Title: "Maximum", SourceChild: "sources/max.json", Operations: operations}},
		},
		Manifest: catalog.ManifestV1{SchemaVersion: 1, Children: []catalog.ChildIdentityV1{
			{Path: "search/directory.json", Kind: "search-directory", Length: 2, SHA256: strings.Repeat("e", 64)},
			{Path: "details/max.json", Kind: "detail", Length: uint64(len(detailBytes)), SHA256: hex.EncodeToString(detailDigest[:])},
		}},
	}
	handler := &CatalogHandler{children: memoryCatalogChildren{"details/max.json": detailBytes}, details: catalog.NewDetailCache()}
	data, err := handler.catalogPageData(context.Background(), snapshot, "/", "max", string(selected.DetailID), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.renderCatalogPage(response, httptest.NewRequest(http.MethodGet, "/documents/max/?selected="+string(selected.DetailID), nil), data)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("max-bound selected operation = %d, want 500", response.Code)
	}
	if response.Body.Len() > 1024 {
		t.Fatalf("max-bound selected operation error = %d bytes, want bounded", response.Body.Len())
	}
	if !strings.Contains(response.Body.String(), "catalog representation exceeds byte limit") {
		t.Fatalf("max-bound selected operation error = %q", response.Body.String())
	}
}

func TestCatalogSidebarPageWindowRejectsHugePageWithoutOverflow(t *testing.T) {
	t.Parallel()

	start, end, ok := catalogSidebarPageWindow(1, 92_233_720_368_547_760)
	if ok || start != 0 || end != 0 {
		t.Fatalf("huge page window = (%d, %d, %t), want (0, 0, false)", start, end, ok)
	}
}

func TestCatalogHugeGroupPagesReturnBoundedResponseWithoutPanic(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	for _, test := range []struct {
		name    string
		groupID string
	}{
		{name: "operations", groupID: catalogGroupID("operations-Untagged")},
		{name: "schemas", groupID: catalogGroupID("schemas")},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestURL := "/kubernetes/documents/core-v1/?group=" + test.groupID + "&page=92233720368547760"
			response := httptest.NewRecorder()
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestURL, nil))
			}()
			if recovered != nil {
				t.Fatalf("huge %s group page panicked: %v", test.name, recovered)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("huge %s group page = %d, want 200", test.name, response.Code)
			}
			if response.Body.Len() >= maxCatalogPageBytes {
				t.Fatalf("huge %s group page = %d bytes, want below the page limit", test.name, response.Body.Len())
			}
		})
	}
}

func TestCatalogDownloadAndCacheContracts(t *testing.T) {
	t.Parallel()

	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	base := "/kubernetes/"
	for stable, exact := range map[string]string{
		base + "openapi/core-v1.json": base + "snapshots/" + string(snapshot.ID) + "/openapi/core-v1.json",
		base + "catalog.json":         base + "snapshots/" + string(snapshot.ID) + "/catalog.json",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, stable, nil))
		if response.Code != http.StatusTemporaryRedirect || response.Header().Get("Location") != exact || response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("stable %s = %d location=%q cache=%q", stable, response.Code, response.Header().Get("Location"), response.Header().Get("Cache-Control"))
		}
	}
	for _, path := range []string{
		base + "snapshots/" + string(snapshot.ID) + "/openapi/core-v1.json",
		base + "snapshots/" + string(snapshot.ID) + "/catalog.json",
		base + "snapshots/" + string(snapshot.ID) + "/search-data/search/directory.json",
	} {
		get := httptest.NewRecorder()
		handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, path, nil))
		if get.Code != http.StatusOK || get.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || get.Header().Get("Content-Encoding") != "" || !strings.HasPrefix(get.Header().Get("ETag"), `"sha256-`) {
			t.Errorf("exact %s = %d headers=%v", path, get.Code, get.Header())
		}
		head := httptest.NewRecorder()
		handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, path, nil))
		if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != get.Header().Get("Content-Length") || head.Header().Get("ETag") != get.Header().Get("ETag") {
			t.Errorf("HEAD %s = %d bytes=%d headers=%v", path, head.Code, head.Body.Len(), head.Header())
		}
	}
	for _, path := range []string{
		base + "snapshots/" + string(snapshot.ID) + "/search-data/details/core.json",
		base + "snapshots/" + string(snapshot.ID) + "/search-data/../catalog.json",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound && response.Code != http.StatusBadRequest {
			t.Errorf("non-search child %s = %d, want 404/400", path, response.Code)
		}
	}
}

func TestCatalogProjectionTransportServesDeclaredImmutableShards(t *testing.T) {
	t.Parallel()

	for _, mount := range []string{"/", "/kubernetes"} {
		t.Run(mount, func(t *testing.T) {
			handler, snapshot := catalogHandlerFixture(t, mount)
			base := mount
			if base != "/" {
				base += "/"
			}
			paths := []string{
				snapshot.Directory.Documents[1].Operations[0].DetailChild,
				snapshot.Directory.Documents[1].Schemas[0].DetailChild,
				snapshot.Directory.Documents[1].SchemaNodeShards[0].Path,
			}
			for _, childPath := range paths {
				identity, ok := catalogChildIdentity(snapshot.Manifest, childPath)
				if !ok {
					t.Fatalf("fixture child %q is undeclared", childPath)
				}
				requestPath := base + "snapshots/" + string(snapshot.ID) + "/projection-data/" + childPath
				get := httptest.NewRecorder()
				handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, requestPath, nil))
				if get.Code != http.StatusOK {
					t.Fatalf("GET %s = %d body=%q, want 200", requestPath, get.Code, get.Body.String())
				}
				digest := sha256.Sum256(get.Body.Bytes())
				if get.Body.Len() != int(identity.Length) || hex.EncodeToString(digest[:]) != identity.SHA256 {
					t.Errorf("GET %s payload = %d bytes sha256=%x, want %d bytes sha256=%s", requestPath, get.Body.Len(), digest, identity.Length, identity.SHA256)
				}
				if get.Header().Get("Content-Type") != "application/json" || get.Header().Get("Content-Length") != fmt.Sprintf("%d", identity.Length) || get.Header().Get("ETag") != `"sha256-`+identity.SHA256+`"` || get.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || get.Header().Get("Content-Encoding") != "" {
					t.Errorf("GET %s headers=%v", requestPath, get.Header())
				}

				head := httptest.NewRecorder()
				handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, requestPath, nil))
				if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != get.Header().Get("Content-Length") || head.Header().Get("ETag") != get.Header().Get("ETag") {
					t.Errorf("HEAD %s = %d bytes=%d headers=%v", requestPath, head.Code, head.Body.Len(), head.Header())
				}

				conditionalRequest := httptest.NewRequest(http.MethodGet, requestPath, nil)
				conditionalRequest.Header.Set("If-None-Match", get.Header().Get("ETag"))
				notModified := httptest.NewRecorder()
				handler.ServeHTTP(notModified, conditionalRequest)
				if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 || notModified.Header().Get("ETag") != get.Header().Get("ETag") {
					t.Errorf("conditional GET %s = %d bytes=%d headers=%v", requestPath, notModified.Code, notModified.Body.Len(), notModified.Header())
				}
			}
		})
	}
}

func TestCatalogProjectionManifestTransportServesCanonicalSnapshotInventory(t *testing.T) {
	for _, mount := range []string{"/", "/kubernetes"} {
		t.Run(mount, func(t *testing.T) {
			baseHandler, snapshot := catalogHandlerFixture(t, mount)
			base := baseHandler.(*CatalogHandler)
			snapshot = catalogEnhancementSnapshot(t, snapshot)
			runtime := catalog.NewRuntime(1)
			if _, err := runtime.ActivateMount(mount, "", 1, snapshot); err != nil {
				t.Fatal(err)
			}
			handler := NewCatalogHandler(runtime, base.children)
			publicationBase := mount
			if publicationBase != "/" {
				publicationBase += "/"
			}
			requestPath := publicationBase + "snapshots/" + string(snapshot.ID) + "/manifest.json"
			wantBytes, err := json.Marshal(snapshot.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			wantDigest := sha256.Sum256(wantBytes)
			wantETag := `"sha256-` + hex.EncodeToString(wantDigest[:]) + `"`

			get := httptest.NewRecorder()
			handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if get.Code != http.StatusOK || !bytes.Equal(get.Body.Bytes(), wantBytes) {
				t.Fatalf("GET %s = %d body=%q, want canonical manifest", requestPath, get.Code, get.Body.String())
			}
			if get.Header().Get("Content-Type") != "application/json" || get.Header().Get("Content-Length") != fmt.Sprintf("%d", len(wantBytes)) || get.Header().Get("ETag") != wantETag || get.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || get.Header().Get("Content-Encoding") != "" {
				t.Errorf("GET %s headers=%v", requestPath, get.Header())
			}

			head := httptest.NewRecorder()
			handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, requestPath, nil))
			if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != get.Header().Get("Content-Length") || head.Header().Get("ETag") != wantETag {
				t.Errorf("HEAD %s = %d bytes=%d headers=%v", requestPath, head.Code, head.Body.Len(), head.Header())
			}

			conditional := httptest.NewRequest(http.MethodGet, requestPath, nil)
			conditional.Header.Set("If-None-Match", wantETag)
			notModified := httptest.NewRecorder()
			handler.ServeHTTP(notModified, conditional)
			if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 || notModified.Header().Get("ETag") != wantETag {
				t.Errorf("conditional GET %s = %d bytes=%d headers=%v", requestPath, notModified.Code, notModified.Body.Len(), notModified.Header())
			}

			post := httptest.NewRecorder()
			handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, requestPath, nil))
			if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != "GET, HEAD" {
				t.Errorf("POST %s = %d allow=%q", requestPath, post.Code, post.Header().Get("Allow"))
			}
		})
	}
}

func TestCatalogProjectionManifestTransportFailsClosed(t *testing.T) {
	baseHandler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	base := baseHandler.(*CatalogHandler)
	snapshot = catalogEnhancementSnapshot(t, snapshot)
	requestBase := "/kubernetes/snapshots/" + string(snapshot.ID)

	for _, requestPath := range []string{
		"/kubernetes/snapshots/snapshot-sha256-" + strings.Repeat("f", 64) + "/manifest.json",
		requestBase + "/../manifest.json",
		requestBase + "/%2e%2e/manifest.json",
		requestBase + "/projection-data/../manifest.json",
	} {
		response := httptest.NewRecorder()
		baseHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if response.Code != http.StatusNotFound && response.Code != http.StatusBadRequest {
			t.Errorf("unsafe manifest path %q = %d, want 404/400", requestPath, response.Code)
		}
	}

	corrupt := snapshot
	corrupt.Manifest.Identity.RevisionID = "revision-corrupt"
	runtime := catalog.NewRuntime(1)
	if _, err := runtime.ActivateMount("/kubernetes", "", 1, corrupt); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewCatalogHandler(runtime, base.children).ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestBase+"/manifest.json", nil))
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "" || response.Header().Get("ETag") != "" {
		t.Errorf("corrupt manifest = %d headers=%v body=%q, want fail-closed 503", response.Code, response.Header(), response.Body.String())
	}
}

func TestCatalogProjectionTransportRejectsUnsafeOrNonProjectionChildren(t *testing.T) {
	t.Parallel()

	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	base := "/kubernetes/snapshots/" + string(snapshot.ID) + "/projection-data/"
	for _, childPath := range []string{
		"catalog.json",
		"sources/core-v1.json",
		"search/directory.json",
		"manifest.json",
		"unknown.json",
		"details/../catalog.json",
		"details//core.json",
		`details\\core.json`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, base+childPath, nil))
		if response.Code != http.StatusNotFound && response.Code != http.StatusBadRequest {
			t.Errorf("projection child %q = %d, want 404/400", childPath, response.Code)
		}
	}
	for _, requestPath := range []string{
		"/kubernetes/snapshots/snapshot-sha256-" + strings.Repeat("c", 64) + "/projection-data/details/core.json",
		base + "details%2fcore.json",
		base + "details%5ccore.json",
		base + "details/%2e%2e/catalog.json",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if response.Code != http.StatusNotFound && response.Code != http.StatusBadRequest {
			t.Errorf("projection path %q = %d, want 404/400", requestPath, response.Code)
		}
	}

	wrongKindSnapshot := snapshot
	wrongKindSnapshot.Manifest.Children = append([]catalog.ChildIdentityV1(nil), snapshot.Manifest.Children...)
	for index := range wrongKindSnapshot.Manifest.Children {
		if wrongKindSnapshot.Manifest.Children[index].Path == "details/core.json" {
			wrongKindSnapshot.Manifest.Children[index].Kind = "schema-node"
		}
	}
	runtime := catalog.NewRuntime(1)
	if _, err := runtime.ActivateMount("/kubernetes", "", 1, wrongKindSnapshot); err != nil {
		t.Fatal(err)
	}
	wrongKindHandler := NewCatalogHandler(runtime, handler.(*CatalogHandler).children)
	response := httptest.NewRecorder()
	wrongKindHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, base+"details/core.json", nil))
	if response.Code != http.StatusNotFound {
		t.Errorf("detail path declared as schema-node = %d, want 404", response.Code)
	}

	validPath := base + snapshot.Directory.Documents[1].Operations[0].DetailChild
	response = httptest.NewRecorder()
	wrongMethod := httptest.NewRequest(http.MethodPost, validPath, nil)
	handler.ServeHTTP(response, wrongMethod)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST projection child = %d allow=%q, want 405 GET, HEAD", response.Code, response.Header().Get("Allow"))
	}
}

func TestCatalogProjectionTransportFailsClosedOnChangedOrUnreadableChild(t *testing.T) {
	t.Parallel()

	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	childPath := snapshot.Directory.Documents[1].Operations[0].DetailChild
	identity, ok := catalogChildIdentity(snapshot.Manifest, childPath)
	if !ok {
		t.Fatalf("fixture child %q is undeclared", childPath)
	}
	requestPath := "/kubernetes/snapshots/" + string(snapshot.ID) + "/projection-data/" + childPath
	baseHandler := handler.(*CatalogHandler)
	changedIdentity := identity
	changedIdentity.SHA256 = strings.Repeat("f", 64)
	for _, test := range []struct {
		name     string
		override projectionTransportCatalogChildren
	}{
		{name: "same-length changed bytes", override: projectionTransportCatalogChildren{fallback: baseHandler.children, path: childPath, data: bytes.Repeat([]byte("x"), int(identity.Length))}},
		{name: "changed loaded identity", override: projectionTransportCatalogChildren{fallback: baseHandler.children, path: childPath, data: bytes.Repeat([]byte("x"), int(identity.Length)), identity: &changedIdentity}},
		{name: "read error", override: projectionTransportCatalogChildren{fallback: baseHandler.children, path: childPath, err: io.ErrUnexpectedEOF}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewCatalogHandler(baseHandler.runtime, test.override).ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "" || response.Header().Get("ETag") != "" {
				t.Errorf("%s = %d headers=%v body=%q, want fail-closed 503", test.name, response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestCatalogProjectionTransportIsNotActivatedByInitialHTML(t *testing.T) {
	t.Parallel()

	handler, _ := catalogHandlerFixture(t, "/kubernetes")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/kubernetes/documents/core-v1/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("initial HTML = %d body=%q", response.Code, response.Body.String())
	}
	digest := sha256.Sum256(response.Body.Bytes())
	if got := hex.EncodeToString(digest[:]); got != "891bf8623eae9f5d342b7aa01d85e7fb6f39ced1ca3319a2a46e8e5999469078" || response.Body.Len() != 53468 {
		t.Errorf("initial HTML = sha256 %s, %d bytes; want accepted OC-01M9 bytes", got, response.Body.Len())
	}
	for _, forbidden := range []string{"projection-data", "serviceWorker", "manja:local-ready", "MANJA_LOCAL_DOCS"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("initial HTML activated OC-04 runtime marker %q", forbidden)
		}
	}
}

func TestCatalogAssetsServeClientSearchRouter(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewCatalogAssetsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/catalog-search.js", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("catalog search asset = %d, want 200", response.Code)
	}
	for _, contract := range []string{"crypto.subtle.digest", "Browser index", "Server fallback", "manja.catalog.recent.v1", "escapeHTML", "search-highlight", "highlight", "usesCommandShortcut", "Command K", "Ctrl K"} {
		if !strings.Contains(response.Body.String(), contract) {
			t.Errorf("catalog search asset missing %q", contract)
		}
	}
}

func TestCatalogAssetsServeValidatedSocialPreview(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewCatalogAssetsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/kubernetes-social.png", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("social preview = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
	if size := response.Body.Len(); size != 48_705 {
		t.Fatalf("social preview size = %d, want 48705", size)
	}
	config, err := png.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
	if err != nil || config.Width != 1280 || config.Height != 640 {
		t.Fatalf("social preview dimensions = %dx%d err=%v, want 1280x640", config.Width, config.Height, err)
	}
}

func TestCatalogMountAwareURLRejectsEscapes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		mount    string
		segments []string
		want     string
		wantErr  bool
	}{
		{mount: "/", want: "/"},
		{mount: "/kubernetes", want: "/kubernetes/"},
		{mount: "/kubernetes", segments: []string{"core-v1"}, want: "/kubernetes/core-v1"},
		{mount: "/kubernetes", segments: []string{".."}, wantErr: true},
		{mount: "kubernetes", segments: []string{"core-v1"}, wantErr: true},
	} {
		got, err := catalogURL(test.mount, test.segments...)
		if (err != nil) != test.wantErr || got != test.want {
			t.Errorf("catalogURL(%q, %q) = %q, %v", test.mount, test.segments, got, err)
		}
	}
}

type memoryCatalogChildren map[string][]byte

func (children memoryCatalogChildren) ReadChild(_ context.Context, snapshot catalog.RuntimeSnapshot, path string) ([]byte, catalog.ChildIdentityV1, error) {
	data, ok := children[path]
	if !ok {
		return nil, catalog.ChildIdentityV1{}, io.ErrUnexpectedEOF
	}
	for _, child := range snapshot.Manifest.Children {
		if child.Path == path {
			return append([]byte(nil), data...), child, nil
		}
	}
	return nil, catalog.ChildIdentityV1{}, io.ErrUnexpectedEOF
}

type projectionTransportCatalogChildren struct {
	fallback catalogChildReader
	path     string
	data     []byte
	identity *catalog.ChildIdentityV1
	err      error
}

func (children projectionTransportCatalogChildren) ReadChild(ctx context.Context, snapshot catalog.RuntimeSnapshot, childPath string) ([]byte, catalog.ChildIdentityV1, error) {
	if childPath != children.path {
		return children.fallback.ReadChild(ctx, snapshot, childPath)
	}
	if children.err != nil {
		return nil, catalog.ChildIdentityV1{}, children.err
	}
	identity, ok := catalogChildIdentity(snapshot.Manifest, childPath)
	if !ok {
		return nil, catalog.ChildIdentityV1{}, io.ErrUnexpectedEOF
	}
	if children.identity != nil {
		identity = *children.identity
	}
	return append([]byte(nil), children.data...), identity, nil
}

type deadlineSearchCatalogChildren struct {
	fallback catalogChildReader
	reads    *int
}

func fullWidthASCII(value string) string {
	var result strings.Builder
	for _, character := range value {
		if character >= '!' && character <= '~' {
			result.WriteRune(character + 0xfee0)
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}

func (children deadlineSearchCatalogChildren) ReadChild(ctx context.Context, snapshot catalog.RuntimeSnapshot, path string) ([]byte, catalog.ChildIdentityV1, error) {
	if strings.HasPrefix(path, "search/") {
		*children.reads = *children.reads + 1
		<-ctx.Done()
		return nil, catalog.ChildIdentityV1{}, ctx.Err()
	}
	return children.fallback.ReadChild(ctx, snapshot, path)
}

func catalogHandlerFixture(t *testing.T, mount string) (http.Handler, catalog.RuntimeSnapshot) {
	return catalogHandlerFixtureWithPresentation(t, mount, nil)
}

func catalogHandlerFixtureWithPresentation(t *testing.T, mount string, presentation map[string]CatalogPresentation) (http.Handler, catalog.RuntimeSnapshot) {
	return catalogHandlerFixtureWithOrganization(t, mount, presentation, OrganizationPresentation{})
}

func catalogHandlerFixtureWithOrganization(t *testing.T, mount string, presentation map[string]CatalogPresentation, organization OrganizationPresentation) (http.Handler, catalog.RuntimeSnapshot) {
	t.Helper()
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	schemaID := domain.DetailID("detail-sha256-" + strings.Repeat("c", 64))
	nodeBytes, err := catalogjson.EncodeSchemaNodeShard(catalog.SchemaNodeShardV1{
		SchemaVersion: 1, DocumentKey: "core-v1", FirstOrdinal: 0,
		Nodes: []projection.SchemaNode{
			{Ordinal: 0, ID: "node-root", Name: "Pod", Type: "object", Properties: []projection.SchemaNodeProperty{{Ordinal: 0, ID: "property-metadata", Name: "metadata", Required: true, Description: "Object metadata.", SchemaRef: 1}}},
			{Ordinal: 1, ID: "node-metadata", Name: "ObjectMeta", Type: "object", Properties: []projection.SchemaNodeProperty{{Ordinal: 0, ID: "property-resource-version", Name: "resourceVersion", SchemaRef: 2}}},
			{Ordinal: 2, ID: "node-string", Name: "string", Type: "string"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	nodeDigest := sha256.Sum256(nodeBytes)
	nodePath := "schema-nodes/core-v1/" + hex.EncodeToString(nodeDigest[:]) + ".json"
	directory := catalog.CatalogArtifactV1{
		SchemaVersion: 1, CatalogID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1", SearchChild: "search/directory.json",
		Documents: []catalog.DocumentDirectoryV1{{
			Key: "apps-v1", SourcePath: "api/openapi-spec/v3/apis__apps__v1_openapi.json", Title: "Kubernetes Core v1", APIVersion: "v1", SourceChild: "sources/apps-v1.json",
		}, {
			Key: "core-v1", SourcePath: "api/openapi-spec/v3/api__v1_openapi.json", Title: "Kubernetes Core v1", APIVersion: "v1", SourceChild: "sources/core-v1.json",
			Operations:       []catalog.OperationDirectoryV1{{DetailID: detailID, OperationID: "listCoreV1Pod", Method: "GET", Path: "/api/v1/pods", Title: "List Pods", Href: "core-v1/?selected=" + string(detailID) + "#" + string(detailID), DetailChild: "details/core.json"}},
			Schemas:          []catalog.SchemaDirectoryV1{{DetailID: schemaID, Name: "Pod", Description: "Pod schema.", Href: "core-v1/?selected=" + string(schemaID) + "#" + string(schemaID), DetailChild: "details/schema.json", CanonicalSHA256: strings.Repeat("d", 64), ProjectionSHA256: strings.Repeat("e", 64)}},
			SchemaNodeShards: []catalog.ShardReferenceV1{{Path: nodePath, FirstOrdinal: 0, LastOrdinal: 2, Records: 3, Length: uint64(len(nodeBytes)), SHA256: hex.EncodeToString(nodeDigest[:])}},
		}},
	}
	search, err := catalog.BuildSearchArtifacts(directory, catalog.DefaultBounds())
	if err != nil {
		t.Fatal(err)
	}
	catalogBytes, err := catalogjson.EncodeCatalog(directory)
	if err != nil {
		t.Fatal(err)
	}
	sourceBytes := []byte(`{"openapi":"3.0.3","info":{"title":"Kubernetes Core v1","version":"v1"},"paths":{}}`)
	detailBytes, err := catalogjson.EncodeDetailShard(catalog.DetailShardV1{SchemaVersion: 1, DocumentKey: "core-v1", Records: []catalog.DetailRecordV1{{
		ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
			ID: string(detailID), Anchor: string(detailID), Href: "documents/core-v1/?selected=" + string(detailID) + "#" + string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2, Method: "GET", Path: "/api/v1/pods", Summary: "List Pods", Description: "Lists Pods.",
			Parameters:     []projection.Parameter{{ID: catalogParameterProjectionID("query", "watch"), Name: "watch", In: "query", Description: "Watch for changes.", SchemaRef: 2}},
			HasRequestBody: true,
			RequestBody:    projection.RequestBody{Required: true, MediaTypes: []projection.MediaType{{ID: "application/json", ContentType: "application/json", SchemaRef: 0, Examples: []projection.Example{{ID: "primary", Text: `{\"kind\":\"Pod\"}`, Provided: true}}}}},
			Responses:      []projection.Response{{ID: "200", Status: "200", Description: "OK", MediaTypes: []projection.MediaType{{ID: "application/json", ContentType: "application/json", SchemaRef: 0}}}},
			Security:       []projection.SecurityRequirement{{ID: "BearerToken", Name: "BearerToken"}},
			CodeSamples:    []projection.CodeSample{{ID: "curl", Label: "cURL", Language: "shell", Code: "curl --request GET /api/v1/pods"}},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	schemaBytes, err := catalogjson.EncodeDetailShard(catalog.DetailShardV1{SchemaVersion: 1, DocumentKey: "core-v1", Records: []catalog.DetailRecordV1{{
		ID: schemaID, Kind: "schema", Schema: &projection.SchemaDetail{ID: string(schemaID), Anchor: string(schemaID), Href: "documents/core-v1/?selected=" + string(schemaID) + "#" + string(schemaID), HeadingID: string(schemaID), Heading: "Pod", HeadingLevel: 2, Description: "Pod schema.", SchemaRef: 0},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	children := memoryCatalogChildren{"catalog.json": catalogBytes, "sources/core-v1.json": sourceBytes, "details/core.json": detailBytes, "details/schema.json": schemaBytes, nodePath: nodeBytes}
	for _, child := range search.Children {
		children[child.Path] = child.Bytes
	}
	manifestChildren := make([]catalog.ChildIdentityV1, 0, len(children))
	for path, data := range children {
		digest := sha256.Sum256(data)
		kind := "source"
		if path == "catalog.json" {
			kind = "catalog"
		} else if strings.HasPrefix(path, "details/") {
			kind = "detail"
		} else if strings.HasPrefix(path, "schema-nodes/") {
			kind = "schema-node"
		} else {
			for _, child := range search.Children {
				if child.Path == path {
					kind = child.Kind
					break
				}
			}
		}
		manifestChildren = append(manifestChildren, catalog.ChildIdentityV1{Path: path, Kind: kind, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:])})
	}
	snapshot := catalog.RuntimeSnapshot{
		ID: "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("b", 64)), Location: "/memory",
		Directory: directory, Search: search.Directory,
		Manifest: catalog.ManifestV1{SchemaVersion: 1, Children: manifestChildren},
	}
	runtime := catalog.NewRuntime(1)
	if _, err := runtime.ActivateMount(mount, "", 1, snapshot); err != nil {
		t.Fatal(err)
	}
	return NewCatalogHandlerWithOrganization(runtime, children, presentation, organization), snapshot
}

func catalogParameterProjectionID(location, name string) string {
	hash := sha256.New()
	hash.Write([]byte("parameter"))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range []string{strings.ToLower(location), name} {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		hash.Write(length[:])
		hash.Write([]byte(value))
	}
	return "parameter-" + hex.EncodeToString(hash.Sum(nil))
}
