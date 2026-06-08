package web

import (
	"encoding/json"
	"encoding/xml"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
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

const openAPIJSONDownloadPath = "/openapi.json"
const searchJSONPath = "/search.json"

type searchJSONItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Href        string   `json:"href,omitempty"`
	Section     string   `json:"section,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
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

func selectedDocsSearchHref(href string) string {
	anchor, ok := strings.CutPrefix(strings.TrimSpace(href), "#")
	if !ok {
		return href
	}
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return "/"
	}
	return "/?selected=" + url.QueryEscape(anchor) + "#" + anchor
}

func searchJSONItems(docs []core.SearchDocument) []searchJSONItem {
	items := make([]searchJSONItem, 0, len(docs))
	for _, doc := range docs {
		items = append(items, searchJSONItem{
			ID:          "search-" + doc.ID,
			Title:       doc.Title,
			Description: doc.Description,
			Href:        selectedDocsSearchHref(doc.Href),
			Section:     doc.Section,
			Keywords:    doc.Keywords,
		})
	}
	return items
}

func NewPublicServer(idx core.SpecIndex) http.Handler {
	return NewPublicServerWithOptions(idx, PublicOptions{})
}

func NewPublicServerWithOptions(idx core.SpecIndex, opts PublicOptions) http.Handler {
	opts = opts.withDefaults()
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.Dir("internal/web/static"))))
	mux.HandleFunc(searchJSONPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != searchJSONPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method == http.MethodHead {
			return
		}
		if err := json.NewEncoder(w).Encode(searchJSONItems(idx.Search)); err != nil {
			slog.ErrorContext(r.Context(), "render public search index", "error", err)
		}
	})
	mux.HandleFunc(openAPIJSONDownloadPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != openAPIJSONDownloadPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if len(idx.SpecDownload.JSON) == 0 {
			http.NotFound(w, r)
			return
		}
		filename := strings.TrimSpace(idx.SpecDownload.Filename)
		if filename == "" {
			filename = "openapi.json"
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(idx.SpecDownload.JSON)
	})
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		selected := r.URL.Query().Get("selected")
		renderOpts := templates.PublicDocsOptions{
			EndpointSidebarLabel: opts.EndpointSidebarLabel,
		}
		component := templates.PublicDocsWithOptions(idx, selected, renderOpts)
		if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" {
			component = templates.PublicDocsFragmentWithOptions(idx, selected, renderOpts)
		}
		if err := component.Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "render public docs", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})
	return mux
}
