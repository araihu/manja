package web

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/araihu/manja/application/catalog"
)

func (handler *CatalogHandler) redirectStable(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string, segments ...string) {
	targetSegments := append([]string{"snapshots", string(snapshot.ID)}, segments...)
	target, err := catalogURL(mount, targetSegments...)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Location", target)
	response.WriteHeader(http.StatusTemporaryRedirect)
}

func (handler *CatalogHandler) serveStableSource(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, fileName string) {
	key, ok := sourceDocumentKey(snapshot.Directory, fileName)
	if !ok {
		http.NotFound(response, request)
		return
	}
	handler.redirectStable(response, request, snapshot, mount, "openapi", key+path.Ext(fileName))
}

func (handler *CatalogHandler) serveSnapshotResource(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, relative string) {
	parts := strings.Split(relative, "/")
	if len(parts) == 3 && parts[0] == "snapshots" && parts[1] == string(snapshot.ID) && parts[2] == "catalog.json" {
		handler.serveExactChild(response, request, snapshot, "catalog.json", "catalog.json")
		return
	}
	if len(parts) == 4 && parts[0] == "snapshots" && parts[1] == string(snapshot.ID) && parts[2] == "openapi" {
		key, ok := sourceDocumentKey(snapshot.Directory, parts[3])
		if !ok {
			http.NotFound(response, request)
			return
		}
		for _, document := range snapshot.Directory.Documents {
			if document.Key == key {
				handler.serveExactChild(response, request, snapshot, document.SourceChild, parts[3])
				return
			}
		}
	}
	http.NotFound(response, request)
}

func (handler *CatalogHandler) serveExactChild(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, childPath, fileName string) {
	data, identity, err := handler.children.ReadChild(request.Context(), snapshot, childPath)
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	contentType := "application/json"
	if strings.HasSuffix(strings.ToLower(fileName), ".yaml") || strings.HasSuffix(strings.ToLower(fileName), ".yml") {
		contentType = "application/yaml"
	}
	etag := `"sha256-` + identity.SHA256 + `"`
	response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(fileName, `"`, "")))
	response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	response.Header().Set("ETag", etag)
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(data)
	}
}

func sourceDocumentKey(directory catalog.CatalogArtifactV1, fileName string) (string, bool) {
	extension := path.Ext(fileName)
	if extension != ".json" && extension != ".yaml" && extension != ".yml" {
		return "", false
	}
	key := strings.TrimSuffix(fileName, extension)
	for _, document := range directory.Documents {
		if document.Key == key && path.Ext(document.SourceChild) == extension {
			return key, true
		}
	}
	return "", false
}
