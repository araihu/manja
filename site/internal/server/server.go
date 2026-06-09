package server

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/araihu/manja/internal/app"
	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/site/internal/site"
)

type Options struct {
	SpecPath  string
	DataDir   string
	StaticDir string
	Branding  core.DocsBranding
}

// New returns the standalone Manja product site handler.
func New() http.Handler {
	return NewWithOptions(context.Background(), Options{})
}

func NewWithOptions(ctx context.Context, opts Options) http.Handler {
	mux := http.NewServeMux()
	demo := mountDemo("/demo", demoHandler(ctx, opts))
	mux.Handle("/static/", cacheForever(http.FileServer(http.FS(site.StaticFiles))))
	mux.Handle("/demo", demo)
	mux.Handle("/demo/", demo)
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

func demoHandler(ctx context.Context, opts Options) http.Handler {
	handler, err := app.NewWithOptions(ctx, app.Options{
		SpecPath:  opts.specPath(),
		DataDir:   opts.dataDir(),
		StaticDir: opts.staticDir(),
		Branding:  opts.Branding,
	})
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
	}
	return handler
}

func mountDemo(prefix string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
			http.NotFound(w, r)
			return
		}

		rewritten := r.Clone(r.Context())
		rewritten.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		if rewritten.URL.Path == "" {
			rewritten.URL.Path = "/"
		}

		recorder := &prefixingResponseWriter{ResponseWriter: w, prefix: prefix}
		next.ServeHTTP(recorder, rewritten)
		recorder.flush()
	})
}

func (o Options) specPath() string {
	if o.SpecPath != "" {
		return o.SpecPath
	}
	return firstExistingPath(
		"internal/adapters/openapi/testdata/github-v3-rest.json",
		"../internal/adapters/openapi/testdata/github-v3-rest.json",
		"/app/manja/internal/adapters/openapi/testdata/github-v3-rest.json",
	)
}

func (o Options) dataDir() string {
	if o.DataDir != "" {
		return o.DataDir
	}
	if isDir("/data/manja") {
		return "/data/manja"
	}
	return ".manja/data"
}

func (o Options) staticDir() string {
	if o.StaticDir != "" {
		return o.StaticDir
	}
	return firstExistingPath(
		"internal/web/static",
		"../internal/web/static",
		"/app/manja/internal/web/static",
	)
}

func firstExistingPath(paths ...string) string {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return paths[0]
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

type prefixingResponseWriter struct {
	http.ResponseWriter
	body        bytes.Buffer
	prefix      string
	status      int
	wroteHeader bool
}

func (w *prefixingResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.status = statusCode
	w.wroteHeader = true
}

func (w *prefixingResponseWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *prefixingResponseWriter) flush() {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	body := w.body.Bytes()
	contentType := w.Header().Get("Content-Type")
	switch {
	case strings.Contains(contentType, "text/html"):
		body = prefixHTMLPaths(body, w.prefix)
		w.Header().Del("Content-Length")
	case strings.Contains(contentType, "application/json"):
		body = prefixJSONPaths(body, w.prefix)
		w.Header().Del("Content-Length")
	}
	w.ResponseWriter.WriteHeader(status)
	_, _ = w.ResponseWriter.Write(body)
}

func prefixHTMLPaths(body []byte, prefix string) []byte {
	return []byte(strings.ReplaceAll(string(body), `="/`, `="`+prefix+`/`))
}

func prefixJSONPaths(body []byte, prefix string) []byte {
	return []byte(strings.ReplaceAll(string(body), `"href":"/`, `"href":"`+prefix+`/`))
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
