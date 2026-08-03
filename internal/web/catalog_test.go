package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{ID: string(detailID), Anchor: string(detailID), Href: "?selected=" + string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2, Method: "GET", Path: "/api/v1/pods", Summary: "List Pods"},
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
	return NewCatalogHandler(runtime, children), snapshot
}
