package server

import (
	"net/http"
	"strings"

	"github.com/araihu/manja/site/internal/site"
)

// New returns the standalone Manja product site handler.
func New() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", cacheForever(http.FileServer(http.FS(site.StaticFiles))))
	mux.HandleFunc("/", page)

	return securityHeaders(mux)
}

func page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var key site.PageKey
	switch strings.TrimRight(r.URL.Path, "/") {
	case "":
		key = site.Home
	case "/demo":
		key = site.Demo
	case "/docs":
		key = site.Docs
	default:
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := site.Render(w, key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func cacheForever(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
