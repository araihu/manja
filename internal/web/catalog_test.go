package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
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
					if !strings.Contains(body, `id="catalog-organization-navigation"`) || !strings.Contains(body, `data-catalog-organization-item="catalog-kubernetes"`) {
						t.Errorf("root response missing organization navigation: %q", body)
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

func TestCatalogSearchRendersMountAwareResultsAndBoundsFailures(t *testing.T) {
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

func TestCatalogSearchJSONReturnsVersionedMountAwareResults(t *testing.T) {
	t.Parallel()

	handler, snapshot := catalogHandlerFixture(t, "/kubernetes")
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
	if payload.CatalogID != "kubernetes" || payload.SnapshotID != snapshot.ID || payload.Version != 1 || payload.Query != "listcorev1pod" {
		t.Fatalf("search JSON identity = %+v", payload)
	}
	if len(payload.Results) != 1 || payload.Results[0].Title != "List Pods" || !strings.HasPrefix(payload.Results[0].Href, "/kubernetes/documents/core-v1/?selected=") {
		t.Fatalf("search JSON results = %+v", payload.Results)
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, requestURL, nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("ETag") != response.Header().Get("ETag") || head.Header().Get("Content-Length") != response.Header().Get("Content-Length") {
		t.Fatalf("search JSON HEAD = %d bytes=%d headers=%v", head.Code, head.Body.Len(), head.Header())
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
			name: "catalog root", requestURL: "/kubernetes/", title: "Kubernetes",
			description: "Browse Kubernetes APIs.", canonical: "https://docs.example.test/kubernetes/",
		},
		{
			name: "document", requestURL: "/kubernetes/documents/core-v1/", title: "Kubernetes Core v1 · Kubernetes",
			description: "OpenAPI operations and schemas for Kubernetes Core v1.", canonical: "https://docs.example.test/kubernetes/documents/core-v1/",
		},
		{
			name: "operation", requestURL: "/kubernetes/documents/core-v1/?selected=" + detailID + "&group=ignored#ignored", title: "List Pods · Kubernetes",
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

func TestCatalogMaxOperationGroupRendersSelectedPageWithinByteBound(t *testing.T) {
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
			ID: string(selected.DetailID), Anchor: string(selected.DetailID), Href: "?selected=" + string(selected.DetailID),
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
	if response.Code != http.StatusOK || response.Body.Len() >= maxCatalogPageBytes {
		t.Fatalf("max-bound selected operation = %d bytes=%d body=%q", response.Code, response.Body.Len(), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `id="`+string(selected.DetailID)+`"`) || !strings.Contains(response.Body.String(), selected.Title) {
		t.Fatal("max-bound selected operation target is not visible")
	}
	if strings.Count(response.Body.String(), `id="sidebar-detail-sha256-`) > catalogSidebarPageSize {
		t.Fatalf("max-bound sidebar materialized more than %d operation links", catalogSidebarPageSize)
	}
}

func TestCatalogSidebarPageWindowRejectsHugePageWithoutOverflow(t *testing.T) {
	t.Parallel()

	start, end, ok := catalogSidebarPageWindow(1, 92_233_720_368_547_760)
	if ok || start != 0 || end != 0 {
		t.Fatalf("huge page window = (%d, %d, %t), want (0, 0, false)", start, end, ok)
	}
}

func TestCatalogHugeGroupPagesReturnBoundedClientErrorWithoutPanic(t *testing.T) {
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
			if response.Code != http.StatusNotFound && response.Code != http.StatusBadRequest {
				t.Fatalf("huge %s group page = %d, want 404/400", test.name, response.Code)
			}
			if response.Body.Len() > 1024 {
				t.Fatalf("huge %s group page error = %d bytes, want bounded", test.name, response.Body.Len())
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

func TestCatalogAssetsServeClientSearchRouter(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewCatalogAssetsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/catalog-search.js", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("catalog search asset = %d, want 200", response.Code)
	}
	for _, contract := range []string{"crypto.subtle.digest", "Browser index", "Server fallback", "manja.catalog.recent.v1", "escapeHTML", "search-highlight", "highlight"} {
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

func catalogHandlerFixture(t *testing.T, mount string) (http.Handler, catalog.RuntimeSnapshot) {
	return catalogHandlerFixtureWithPresentation(t, mount, nil)
}

func catalogHandlerFixtureWithPresentation(t *testing.T, mount string, presentation map[string]CatalogPresentation) (http.Handler, catalog.RuntimeSnapshot) {
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
			ID: string(detailID), Anchor: string(detailID), Href: "?selected=" + string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2, Method: "GET", Path: "/api/v1/pods", Summary: "List Pods", Description: "Lists Pods.",
			Parameters:     []projection.Parameter{{ID: "query-watch", Name: "watch", In: "query", Description: "Watch for changes.", SchemaRef: 2}},
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
		ID: schemaID, Kind: "schema", Schema: &projection.SchemaDetail{ID: string(schemaID), Anchor: string(schemaID), Href: "?selected=" + string(schemaID), HeadingID: string(schemaID), Heading: "Pod", HeadingLevel: 2, Description: "Pod schema.", SchemaRef: 0},
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
	return NewCatalogHandlerWithPresentation(runtime, children, presentation), snapshot
}
