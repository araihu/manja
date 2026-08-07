package web

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/araihu/goshtoso/assets"

	"github.com/araihu/manja/application/port"
	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web/templates"
)

type EndpointSidebarLabelMode = templates.EndpointSidebarLabelMode

const (
	EndpointSidebarLabelAuto = templates.EndpointSidebarLabelAuto
	EndpointSidebarLabelPath = templates.EndpointSidebarLabelPath
)

type PublicOptions struct {
	EndpointSidebarLabel EndpointSidebarLabelMode
	MarkdownRenderer     port.MarkdownRenderer
	StaticDir            string
	Branding             core.DocsBranding
}

const openAPIJSONDownloadPath = "/openapi.json"
const searchJSONPath = "/search.json"

type searchJSONItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Href        string   `json:"href,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Method      string   `json:"method,omitempty"`
	Path        string   `json:"path,omitempty"`
	Section     string   `json:"section,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

func (opts PublicOptions) withDefaults() PublicOptions {
	if opts.StaticDir == "" {
		opts.StaticDir = "internal/web/static"
	}
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

func publicRouteHrefsByAnchor(routes []core.PublicRoute) map[string]string {
	hrefs := make(map[string]string, len(routes))
	for _, route := range routes {
		_, anchor, ok := strings.Cut(strings.TrimSpace(route.Path), "#")
		anchor = strings.TrimSpace(anchor)
		if !ok || anchor == "" {
			continue
		}
		hrefs[anchor] = route.Path
	}
	return hrefs
}

func searchDocumentAnchor(href string) (string, bool) {
	anchor, ok := strings.CutPrefix(strings.TrimSpace(href), "#")
	anchor = strings.TrimSpace(anchor)
	return anchor, ok && anchor != ""
}

func publicRouteSearchHref(href string, routeHrefs map[string]string) string {
	if anchor, ok := searchDocumentAnchor(href); ok {
		if routeHref, found := routeHrefs[anchor]; found {
			return routeHref
		}
	}
	return selectedDocsSearchHref(href)
}

func searchJSONItems(ctx context.Context, docs []core.SearchDocument, publicRoutes []core.PublicRoute, renderer port.MarkdownRenderer) ([]searchJSONItem, error) {
	routeHrefs := publicRouteHrefsByAnchor(publicRoutes)
	items := make([]searchJSONItem, 0, len(docs))
	for _, doc := range docs {
		description, err := markdownPlainText(ctx, renderer, doc.Description)
		if err != nil {
			return nil, err
		}
		items = append(items, searchJSONItem{
			ID:          "search-" + doc.ID,
			Title:       doc.Title,
			Description: description,
			Href:        publicRouteSearchHref(doc.Href, routeHrefs),
			Kind:        doc.Kind,
			Method:      doc.Method,
			Path:        doc.Path,
			Section:     doc.Section,
			Keywords:    doc.Keywords,
		})
	}
	return items, nil
}

func markdownPlainText(ctx context.Context, renderer port.MarkdownRenderer, value string) (string, error) {
	if renderer == nil || strings.TrimSpace(value) == "" {
		return value, nil
	}
	result, err := renderer.Render(ctx, value)
	if err != nil {
		return "", err
	}
	return result.Plain, nil
}

func NewPublicServer(idx core.SpecIndex) http.Handler {
	return NewPublicServerWithOptions(idx, PublicOptions{})
}

func NewPublicServerWithOptions(idx core.SpecIndex, opts PublicOptions) http.Handler {
	opts = opts.withDefaults()
	manageDefaultLogo := strings.TrimSpace(idx.Branding.Logo.Src) == "" && strings.TrimSpace(opts.Branding.Logo.Src) == ""
	manageDefaultFavicon := strings.TrimSpace(idx.Branding.Favicon) == "" && strings.TrimSpace(opts.Branding.Favicon) == ""
	idx.Branding = docsBranding(idx.Branding, opts.Branding)
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.Dir(opts.StaticDir))))
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
		items, err := searchJSONItems(r.Context(), idx.Search, idx.PublicRoutes, opts.MarkdownRenderer)
		if err != nil {
			slog.ErrorContext(r.Context(), "render public search markdown", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(items); err != nil {
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
	mux.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llms.txt" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		writePageMarkdown(w, r, publicLLMsText(idx))
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
		selected := r.URL.Query().Get("selected")
		if r.URL.Query().Get("format") == "markdown" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			document, ok := publicPageMarkdown(idx, selected)
			if !ok {
				http.NotFound(w, r)
				return
			}
			writePageMarkdown(w, r, document)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderOpts := templates.PublicDocsOptions{
			EndpointSidebarLabel: opts.EndpointSidebarLabel,
			MarkdownRenderer:     opts.MarkdownRenderer,
			ManageDefaultLogo:    manageDefaultLogo,
			ManageDefaultFavicon: manageDefaultFavicon,
		}
		component := templates.PublicDocsWithOptions(idx, selected, renderOpts)
		if r.Header.Get("HX-Request") == "true" &&
			r.Header.Get("HX-Boosted") != "true" &&
			r.Header.Get("HX-History-Restore-Request") != "true" {
			component = templates.PublicDocsFragmentWithOptions(idx, selected, renderOpts)
		}
		if err := component.Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "render public docs", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})
	return mux
}

func docsBranding(specBranding core.DocsBranding, optionBranding core.DocsBranding) core.DocsBranding {
	branding := core.DocsBranding{
		DisplayName: "Manja",
		Logo: core.DocsBrandingLogo{
			Src:     "/manja-assets/manja-mark.svg",
			HomeURL: "/",
		},
		Favicon: "/manja-assets/favicon.svg",
	}
	branding = mergeDocsBranding(branding, specBranding)
	branding = mergeDocsBranding(branding, optionBranding)
	if strings.TrimSpace(branding.Logo.HomeURL) == "" {
		branding.Logo.HomeURL = "/"
	}
	return branding
}

func mergeDocsBranding(base core.DocsBranding, override core.DocsBranding) core.DocsBranding {
	if strings.TrimSpace(override.DisplayName) != "" {
		base.DisplayName = strings.TrimSpace(override.DisplayName)
	}
	if strings.TrimSpace(override.Logo.Src) != "" {
		base.Logo.Src = strings.TrimSpace(override.Logo.Src)
	}
	if strings.TrimSpace(override.Logo.Alt) != "" {
		base.Logo.Alt = strings.TrimSpace(override.Logo.Alt)
	}
	if strings.TrimSpace(override.Logo.HomeURL) != "" {
		base.Logo.HomeURL = strings.TrimSpace(override.Logo.HomeURL)
	}
	if strings.TrimSpace(override.Favicon) != "" {
		base.Favicon = strings.TrimSpace(override.Favicon)
	}
	return base
}
