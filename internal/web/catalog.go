package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/web/templates"
)

type catalogChildReader interface {
	ReadChild(context.Context, catalog.RuntimeSnapshot, string) ([]byte, catalog.ChildIdentityV1, error)
}

type CatalogHandler struct {
	runtime  *catalog.Runtime
	children catalogChildReader
	details  *catalog.ByteCache
	search   *catalog.ByteCache
}

func NewCatalogHandler(runtime *catalog.Runtime, children catalogChildReader) http.Handler {
	return &CatalogHandler{runtime: runtime, children: children, details: catalog.NewDetailCache(), search: catalog.NewSearchCache()}
}

func (handler *CatalogHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.runtime == nil || handler.children == nil {
		http.Error(response, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	mount, exactMount := handler.matchMount(request.URL.Path)
	if mount == "" {
		http.NotFound(response, request)
		return
	}
	if exactMount && mount != "/" {
		http.Redirect(response, request, mount+"/", http.StatusPermanentRedirect)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if hasEncodedSeparator(request.URL.EscapedPath()) {
		http.Error(response, "invalid path", http.StatusBadRequest)
		return
	}
	relative := strings.TrimPrefix(request.URL.Path, mount)
	if mount == "/" {
		relative = strings.TrimPrefix(request.URL.Path, "/")
	} else {
		relative = strings.TrimPrefix(relative, "/")
	}
	requestedSnapshot := snapshotIDFromCatalogRoute(relative)
	var admission *catalog.Admission
	var err error
	if requestedSnapshot == "" {
		admission, err = handler.runtime.Admit(mount)
	} else {
		admission, err = handler.runtime.AdmitSnapshot(mount, requestedSnapshot)
	}
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer admission.Release()
	handler.serveAdmitted(response, request, admission.Snapshot, mount, relative)
}

func (handler *CatalogHandler) matchMount(requestPath string) (string, bool) {
	mounts := handler.runtime.MountNames()
	sort.Slice(mounts, func(left, right int) bool { return len(mounts[left]) > len(mounts[right]) })
	for _, mount := range mounts {
		if mount == "/" {
			return mount, requestPath == "/"
		}
		if requestPath == mount {
			return mount, true
		}
		if strings.HasPrefix(requestPath, mount+"/") {
			return mount, false
		}
	}
	return "", false
}

func (handler *CatalogHandler) serveAdmitted(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, relative string) {
	switch {
	case relative == "":
		if document := request.URL.Query().Get("document"); document != "" {
			target, err := catalogURL(mount, "documents", document)
			if err != nil || !catalogDocumentExists(snapshot.Directory, document) {
				http.NotFound(response, request)
				return
			}
			http.Redirect(response, request, target+"/", http.StatusSeeOther)
			return
		}
		handler.serveOverview(response, request, snapshot, mount)
	case relative == "catalog.json":
		handler.redirectStable(response, request, snapshot, mount, "catalog.json")
	case relative == "search":
		handler.serveSearch(response, request, snapshot, mount)
	case strings.HasPrefix(relative, "documents/"):
		documentPath := strings.TrimPrefix(relative, "documents/")
		key := strings.TrimSuffix(documentPath, "/")
		if key == "" || strings.Contains(key, "/") {
			http.NotFound(response, request)
			return
		}
		handler.serveDocument(response, request, snapshot, mount, key)
	case strings.HasPrefix(relative, "openapi/"):
		handler.serveStableSource(response, request, snapshot, mount, strings.TrimPrefix(relative, "openapi/"))
	case strings.HasPrefix(relative, "snapshots/"):
		handler.serveSnapshotResource(response, request, snapshot, mount, relative)
	case strings.HasSuffix(relative, "/") && !strings.Contains(strings.TrimSuffix(relative, "/"), "/"):
		handler.serveDocument(response, request, snapshot, mount, strings.TrimSuffix(relative, "/"))
	case !strings.Contains(relative, "/"):
		target, err := catalogURL(mount, relative)
		if err != nil {
			http.NotFound(response, request)
			return
		}
		http.Redirect(response, request, target+"/", http.StatusPermanentRedirect)
	default:
		http.NotFound(response, request)
	}
}

func (handler *CatalogHandler) serveSearch(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	data, err := handler.catalogPageData(request.Context(), snapshot, mount, "", "", "")
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	query := request.URL.Query().Get("q")
	data.Search = &templates.CatalogSearchData{Query: query}
	if query != "" {
		service, err := catalog.NewRuntimeSearchService(snapshot, handler.search, func(ctx context.Context, childPath string) ([]byte, catalog.ChildIdentityV1, error) {
			data, identity, err := handler.children.ReadChild(ctx, snapshot, childPath)
			return data, identity, err
		})
		if err == nil {
			var result catalog.SearchResult
			result, err = service.Search(request.Context(), snapshot.ID, query)
			if err == nil {
				data.Search.Query = result.Query
				data.Search.PostingsScanned = result.PostingsScanned
				data.Search.SegmentsDecoded = result.SegmentsDecoded
				data.Search.BytesDecoded = result.BytesDecoded
				for _, record := range result.Results {
					href, hrefErr := catalogSearchHref(mount, record.Href)
					if hrefErr != nil {
						err = hrefErr
						break
					}
					data.Search.Results = append(data.Search.Results, templates.CatalogSearchResultData{Record: record, Href: href})
				}
			}
		}
		if err != nil {
			switch {
			case errors.Is(err, catalog.ErrInvalidQuery), errors.Is(err, catalog.ErrQueryTooBroad):
				http.Error(response, "invalid or overly broad search query", http.StatusBadRequest)
			case errors.Is(err, catalog.ErrSearchDeadline):
				response.Header().Set("Retry-After", "1")
				http.Error(response, "search temporarily unavailable", http.StatusServiceUnavailable)
			default:
				http.Error(response, "search temporarily unavailable", http.StatusServiceUnavailable)
			}
			return
		}
	}
	handler.renderCatalogPage(response, request, data)
}

func (handler *CatalogHandler) serveOverview(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	data, err := handler.catalogPageData(request.Context(), snapshot, mount, "", "", "")
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	handler.renderCatalogPage(response, request, data)
}

func (handler *CatalogHandler) serveDocument(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, key string) {
	data, err := handler.catalogPageData(request.Context(), snapshot, mount, key, request.URL.Query().Get("selected"), request.URL.Query().Get("group"))
	if err != nil {
		if errors.Is(err, errCatalogPageNotFound) {
			http.NotFound(response, request)
			return
		}
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	handler.renderCatalogPage(response, request, data)
}

func (handler *CatalogHandler) renderCatalogPage(response http.ResponseWriter, request *http.Request, data templates.CatalogPageData) {
	var body bytes.Buffer
	if err := templates.CatalogPage(data).Render(request.Context(), &body); err != nil {
		http.Error(response, "render catalog", http.StatusInternalServerError)
		return
	}
	writeCatalogRepresentation(response, request, body.Bytes(), "text/html; charset=utf-8")
}

func writeCatalogRepresentation(response http.ResponseWriter, request *http.Request, body []byte, contentType string) {
	digest := sha256.Sum256(body)
	etag := `"sha256-` + hex.EncodeToString(digest[:]) + `"`
	response.Header().Set("Cache-Control", "private, no-cache")
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("ETag", etag)
	response.Header().Set("Vary", "HX-Request, HX-Boosted, Accept-Encoding")
	response.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; object-src 'none'; base-uri 'none'")
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	response.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(body)
	}
}

func catalogURL(mount string, segments ...string) (string, error) {
	if mount != "/" && (mount == "" || !strings.HasPrefix(mount, "/") || strings.HasSuffix(mount, "/") || strings.ContainsAny(mount, `\?#%`) || strings.Contains(mount, "//")) {
		return "", fmt.Errorf("invalid catalog mount %q", mount)
	}
	base := mount
	if base == "/" {
		base = ""
	}
	if len(segments) == 0 {
		return base + "/", nil
	}
	encoded := make([]string, len(segments))
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\?#`) {
			return "", fmt.Errorf("invalid catalog URL segment %q", segment)
		}
		encoded[index] = url.PathEscape(segment)
	}
	return base + "/" + strings.Join(encoded, "/"), nil
}

func snapshotIDFromCatalogRoute(relative string) catalog.SnapshotID {
	parts := strings.Split(relative, "/")
	if len(parts) >= 2 && parts[0] == "snapshots" {
		return catalog.SnapshotID(parts[1])
	}
	return ""
}

func hasEncodedSeparator(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c")
}
