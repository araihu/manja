package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/adapters/catalogjson"
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
	if len(parts) == 3 && parts[0] == "snapshots" && parts[1] == string(snapshot.ID) && parts[2] == "manifest.json" {
		handler.serveProjectionManifest(response, request, snapshot)
		return
	}
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
	if len(parts) >= 5 && parts[0] == "snapshots" && parts[1] == string(snapshot.ID) && parts[2] == "search-data" {
		childPath := strings.Join(parts[3:], "/")
		identity, exists := catalogChildIdentity(snapshot.Manifest, childPath)
		if !exists || !strings.HasPrefix(childPath, "search/") || !strings.HasPrefix(identity.Kind, "search-") {
			http.NotFound(response, request)
			return
		}
		handler.serveExactSearchChild(response, request, snapshot, childPath)
		return
	}
	if len(parts) >= 5 && parts[0] == "snapshots" && parts[1] == string(snapshot.ID) && parts[2] == "projection-data" {
		childPath := strings.Join(parts[3:], "/")
		identity, exists := catalogChildIdentity(snapshot.Manifest, childPath)
		if !exists || !isProjectionChildPath(childPath, identity.Kind) {
			http.NotFound(response, request)
			return
		}
		handler.serveExactProjectionChild(response, request, snapshot, identity)
		return
	}
	http.NotFound(response, request)
}

func (handler *CatalogHandler) serveProjectionManifest(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot) {
	data, err := catalogjson.EncodeManifest(snapshot.Manifest)
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	digest := sha256.Sum256(data)
	etag := `"sha256-` + hex.EncodeToString(digest[:]) + `"`
	response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	response.Header().Set("Content-Type", "application/json")
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

func isProjectionChildPath(childPath, kind string) bool {
	if childPath == "" || path.Clean(childPath) != childPath || strings.Contains(childPath, `\`) {
		return false
	}
	switch kind {
	case "detail":
		return strings.HasPrefix(childPath, "details/") && len(childPath) > len("details/")
	case "schema-node":
		return strings.HasPrefix(childPath, "schema-nodes/") && len(childPath) > len("schema-nodes/")
	default:
		return false
	}
}

func (handler *CatalogHandler) serveExactProjectionChild(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, approved catalog.ChildIdentityV1) {
	data, loaded, err := handler.children.ReadChild(request.Context(), snapshot, approved.Path)
	if err != nil || loaded != approved || uint64(len(data)) != approved.Length {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != approved.SHA256 {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	etag := `"sha256-` + approved.SHA256 + `"`
	response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Content-Length", fmt.Sprintf("%d", approved.Length))
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

func (handler *CatalogHandler) serveExactSearchChild(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, childPath string) {
	data, identity, err := handler.children.ReadChild(request.Context(), snapshot, childPath)
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	etag := `"sha256-` + identity.SHA256 + `"`
	response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	response.Header().Set("Content-Type", "application/json")
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
