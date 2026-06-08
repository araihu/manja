package web

import (
	"encoding/xml"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/araihu/goshtoso/assets"

	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web/templates"
)

type EndpointSidebarLabelMode = templates.EndpointSidebarLabelMode

const (
	EndpointSidebarLabelAuto = templates.EndpointSidebarLabelAuto
	EndpointSidebarLabelPath = templates.EndpointSidebarLabelPath
)

type PublicOptions struct {
	EndpointSidebarLabel EndpointSidebarLabelMode
}

func (opts PublicOptions) withDefaults() PublicOptions {
	switch opts.EndpointSidebarLabel {
	case EndpointSidebarLabelPath:
		return opts
	default:
		opts.EndpointSidebarLabel = EndpointSidebarLabelAuto
		return opts
	}
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

func sitemapScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	hostOnly, _, err := net.SplitHostPort(host)
	if err != nil {
		hostOnly = host
	}
	if hostOnly == "localhost" || hostOnly == "127.0.0.1" {
		return "http"
	}
	return "https"
}

func sitemapLoc(r *http.Request, path string) (string, bool) {
	if path == "" || strings.HasPrefix(path, "#") || !strings.HasPrefix(path, "/") {
		return "", false
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return sitemapScheme(r) + "://" + host + path, true
}

func NewPublicServer(idx core.SpecIndex) http.Handler {
	return NewPublicServerWithOptions(idx, PublicOptions{})
}

func NewPublicServerWithOptions(idx core.SpecIndex, opts PublicOptions) http.Handler {
	opts = opts.withDefaults()
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.Dir("internal/web/static"))))
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sitemap.xml" {
			http.NotFound(w, r)
			return
		}
		urls := make([]sitemapURL, 0, len(idx.PublicRoutes))
		for _, route := range idx.PublicRoutes {
			loc, ok := sitemapLoc(r, route.Path)
			if !ok {
				continue
			}
			urls = append(urls, sitemapURL{Loc: loc})
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write([]byte(xml.Header))
		_ = xml.NewEncoder(w).Encode(sitemapURLSet{
			Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
			URLs:  urls,
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := templates.PublicDocsWithOptions(idx, r.URL.Query().Get("selected"), templates.PublicDocsOptions{
			EndpointSidebarLabel: opts.EndpointSidebarLabel,
		}).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "render public docs", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})
	return mux
}
