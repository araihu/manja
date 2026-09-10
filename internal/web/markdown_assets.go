package web

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	markdownadapter "github.com/araihu/manja/internal/adapters/markdown"
	localrender "github.com/araihu/manja/internal/localdocs/render"
	"github.com/araihu/margo"
)

var markdownAssetNames = []string{"document.css"}

func withOperationMarkdown(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, operationMarkdownRequest(r))
	})
}

func operationMarkdownRequest(r *http.Request) *http.Request {
	return r.WithContext(localrender.WithDescriptionRenderer(r.Context(), markdownadapter.DescriptionComponent))
}

// A closed asset set keeps Margo's core fragment dependencies same-origin and
// exportable without exposing its site/server packages or a second UI shell.
func markdownAssetsHandler() http.Handler {
	assets := make(map[string]margo.AssetRef, len(markdownAssetNames))
	for _, name := range markdownAssetNames {
		asset, err := margo.EmbeddedAsset(name)
		if err != nil {
			panic(err)
		}
		assets["/manja-assets/margo/"+name] = asset
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset, ok := assets[r.URL.Path]
		if !ok || !validPublicAssetRequest(r) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", asset.MediaType)
		w.Header().Set("ETag", `"`+asset.SHA256+`"`)
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		http.ServeContent(w, r, strings.TrimPrefix(r.URL.Path, "/manja-assets/margo/"), time.Time{}, bytes.NewReader(asset.Content))
	})
}
