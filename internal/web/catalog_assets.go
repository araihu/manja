package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/araihu/goshtoso/assets"
)

//go:embed static/*
var catalogStaticFiles embed.FS

func NewCatalogAssetsHandler() http.Handler {
	static, err := fs.Sub(catalogStaticFiles, "static")
	if err != nil {
		panic(err)
	}
	allowlist, err := embeddedPublicAssetAllowlist(static)
	if err != nil {
		panic(err)
	}
	goshtosoAllowlist, err := embeddedPublicAssetAllowlist(assets.FS())
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.FS(static))))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !validPublicAssetRequest(request) || (!strings.HasPrefix(request.URL.Path, "/assets/") && !strings.HasPrefix(request.URL.Path, "/manja-assets/")) {
			http.NotFound(response, request)
			return
		}
		switch {
		case strings.HasPrefix(request.URL.Path, "/assets/") && !goshtosoAllowlist[strings.TrimPrefix(request.URL.Path, "/assets/")]:
			http.NotFound(response, request)
			return
		case strings.HasPrefix(request.URL.Path, "/manja-assets/") && !allowlist[strings.TrimPrefix(request.URL.Path, "/manja-assets/")]:
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

// CatalogAssetPaths returns the exact regular files exposed by the catalog
// asset handler. Static export uses this instead of widening the asset surface.
func CatalogAssetPaths() []string {
	static, err := fs.Sub(catalogStaticFiles, "static")
	if err != nil {
		panic(err)
	}
	manja, err := embeddedPublicAssetAllowlist(static)
	if err != nil {
		panic(err)
	}
	goshtoso, err := embeddedPublicAssetAllowlist(assets.FS())
	if err != nil {
		panic(err)
	}
	paths := make([]string, 0, len(manja)+len(goshtoso))
	for name := range manja {
		paths = append(paths, "/manja-assets/"+name)
	}
	for name := range goshtoso {
		paths = append(paths, "/assets/"+name)
	}
	sort.Strings(paths)
	return paths
}

// embeddedPublicAssetAllowlist contains only regular files embedded in an
// asset tree. Keeping directories out prevents FileServer redirects or
// listings from widening the public asset surface.
func embeddedPublicAssetAllowlist(static fs.FS) (map[string]bool, error) {
	allowlist := make(map[string]bool)
	err := fs.WalkDir(static, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fs.ErrInvalid
		}
		allowlist[name] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return allowlist, nil
}

// validPublicAssetRequest keeps asset reads same-origin and pathname-only.
// Service Workers use the same exact GET pathname contract; HEAD remains
// available for ordinary HTTP probes without permitting mutation methods.
func validPublicAssetRequest(request *http.Request) bool {
	if request == nil || request.URL == nil || request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	url := request.URL
	if url.Scheme != "" || url.Host != "" || url.Opaque != "" || url.RawQuery != "" || url.ForceQuery || url.Fragment != "" {
		return false
	}
	if url.RawPath != "" && url.RawPath != url.Path {
		return false
	}
	if url.Path == "" || path.Clean(url.Path) != url.Path || strings.ContainsAny(url.Path, `\\%?#`) {
		return false
	}
	for _, character := range url.Path {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}
