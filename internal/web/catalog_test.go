package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
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
				{http.MethodGet, base + "core-v1/?selected=detail-sha256-" + strings.Repeat("a", 64), http.StatusOK},
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
	directory := catalog.CatalogArtifactV1{
		SchemaVersion: 1, CatalogID: "kubernetes", Title: "Kubernetes", DefaultDocumentKey: "core-v1", SearchChild: "search/directory.json",
		Documents: []catalog.DocumentDirectoryV1{{
			Key: "core-v1", SourcePath: "api/openapi-spec/v3/api__v1_openapi.json", Title: "Kubernetes Core v1", APIVersion: "v1", SourceChild: "sources/core-v1.json",
			Operations: []catalog.OperationDirectoryV1{{DetailID: detailID, OperationID: "listCoreV1Pod", Method: "GET", Path: "/api/v1/pods", Title: "List Pods", Href: "core-v1/?selected=" + string(detailID) + "#" + string(detailID), DetailChild: "details/core.json"}},
		}},
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
	children := memoryCatalogChildren{"catalog.json": catalogBytes, "sources/core-v1.json": sourceBytes, "details/core.json": detailBytes}
	manifestChildren := make([]catalog.ChildIdentityV1, 0, len(children))
	for path, data := range children {
		digest := sha256.Sum256(data)
		kind := "source"
		if path == "catalog.json" {
			kind = "catalog"
		} else if strings.HasPrefix(path, "details/") {
			kind = "detail"
		}
		manifestChildren = append(manifestChildren, catalog.ChildIdentityV1{Path: path, Kind: kind, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:])})
	}
	snapshot := catalog.RuntimeSnapshot{
		ID: "snapshot-sha256-" + catalog.SnapshotID(strings.Repeat("b", 64)), Location: "/memory",
		Directory: directory, Search: catalog.SearchDirectoryV1{SchemaVersion: 1, SearchVersion: 1},
		Manifest: catalog.ManifestV1{SchemaVersion: 1, Children: manifestChildren},
	}
	runtime := catalog.NewRuntime(1)
	if _, err := runtime.ActivateMount(mount, "", 1, snapshot); err != nil {
		t.Fatal(err)
	}
	return NewCatalogHandler(runtime, children), snapshot
}
