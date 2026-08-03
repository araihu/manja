package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
)

type catalogChildReader interface {
	ReadChild(context.Context, catalog.RuntimeSnapshot, string) ([]byte, catalog.ChildIdentityV1, error)
}

type CatalogHandler struct {
	runtime  *catalog.Runtime
	children catalogChildReader
}

func NewCatalogHandler(runtime *catalog.Runtime, children catalogChildReader) http.Handler {
	return &CatalogHandler{runtime: runtime, children: children}
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
		handler.serveOverview(response, request, snapshot, mount)
	case relative == "catalog.json":
		handler.redirectStable(response, request, snapshot, mount, "catalog.json")
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

func (handler *CatalogHandler) serveOverview(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	var body strings.Builder
	body.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>")
	body.WriteString(html.EscapeString(snapshot.Directory.Title))
	body.WriteString("</title></head><body><main><h1>")
	body.WriteString(html.EscapeString(snapshot.Directory.Title))
	body.WriteString("</h1><p>")
	body.WriteString(fmt.Sprintf("%d OpenAPI documents", len(snapshot.Directory.Documents)))
	body.WriteString("</p><ul>")
	for _, document := range snapshot.Directory.Documents {
		href, _ := catalogURL(mount, document.Key)
		body.WriteString("<li><a href=\"")
		body.WriteString(html.EscapeString(href + "/"))
		body.WriteString("\">")
		body.WriteString(html.EscapeString(document.Title))
		body.WriteString("</a></li>")
	}
	body.WriteString("</ul></main></body></html>")
	writeCatalogRepresentation(response, request, []byte(body.String()), "text/html; charset=utf-8")
}

func (handler *CatalogHandler) serveDocument(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, key string) {
	var selected catalog.OperationDirectoryV1
	foundDocument := false
	foundSelected := request.URL.Query().Get("selected") == ""
	var document catalog.DocumentDirectoryV1
	for _, candidate := range snapshot.Directory.Documents {
		if candidate.Key == key {
			document = candidate
			foundDocument = true
			break
		}
	}
	if !foundDocument {
		http.NotFound(response, request)
		return
	}
	for _, operation := range document.Operations {
		if string(operation.DetailID) == request.URL.Query().Get("selected") {
			selected = operation
			foundSelected = true
			break
		}
	}
	if !foundSelected {
		for _, schema := range document.Schemas {
			if string(schema.DetailID) == request.URL.Query().Get("selected") {
				foundSelected = true
				break
			}
		}
	}
	if !foundSelected {
		http.NotFound(response, request)
		return
	}
	var body strings.Builder
	body.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>")
	body.WriteString(html.EscapeString(document.Title))
	body.WriteString("</title></head><body><main><h1>")
	body.WriteString(html.EscapeString(document.Title))
	body.WriteString("</h1><p>")
	body.WriteString(fmt.Sprintf("%d operations · %d schemas", len(document.Operations), len(document.Schemas)))
	body.WriteString("</p>")
	if selected.DetailID != "" {
		body.WriteString("<section id=\"")
		body.WriteString(html.EscapeString(string(selected.DetailID)))
		body.WriteString("\"><h2>")
		body.WriteString(html.EscapeString(selected.Title))
		body.WriteString("</h2><code>")
		body.WriteString(html.EscapeString(selected.Method + " " + selected.Path))
		body.WriteString("</code></section>")
	}
	body.WriteString("</main></body></html>")
	writeCatalogRepresentation(response, request, []byte(body.String()), "text/html; charset=utf-8")
}

func writeCatalogRepresentation(response http.ResponseWriter, request *http.Request, body []byte, contentType string) {
	digest := sha256.Sum256(body)
	etag := `"sha256-` + hex.EncodeToString(digest[:]) + `"`
	response.Header().Set("Cache-Control", "private, no-cache")
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("ETag", etag)
	response.Header().Set("Vary", "HX-Request, HX-Boosted, Accept-Encoding")
	response.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'")
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
