package web

import (
	"log/slog"
	"net/http"

	"github.com/araihu/goshtoso/assets"

	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web/templates"
)

func NewPublicServer(idx core.SpecIndex) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.Dir("internal/web/static"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := templates.PublicDocs(idx).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "render public docs", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})
	return mux
}
