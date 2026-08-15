package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/araihu/goshtoso/assets"
)

//go:embed static/* static/local-docs/*
var catalogStaticFiles embed.FS

func NewCatalogAssetsHandler() http.Handler {
	static, err := fs.Sub(catalogStaticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.FS(static))))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/assets/") && !strings.HasPrefix(request.URL.Path, "/manja-assets/") {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		if request.URL.Path == "/manja-assets/local-docs/sw.js" {
			response.Header().Set("Service-Worker-Allowed", "/")
		}
		mux.ServeHTTP(response, request)
	})
}
