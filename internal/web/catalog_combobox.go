package web

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/araihu/goshtoso/components/combobox"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/web/templates"
)

const (
	maxCatalogDocumentComboboxQueryBytes   = 256
	maxCatalogDocumentComboboxRequestBytes = 4 << 10
)

type catalogDocumentComboboxContextKey struct{}

type catalogDocumentComboboxContext struct {
	mount    string
	snapshot catalog.RuntimeSnapshot
}

var catalogDocumentComboboxHandler = combobox.Handler(
	templates.CatalogDocumentComboboxConfig(),
	catalogDocumentComboboxOptions,
)

func IsCatalogComponentPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, templates.CatalogDocumentComboboxPathPrefix)
}

func (handler *CatalogHandler) serveCatalogDocumentCombobox(response http.ResponseWriter, request *http.Request) {
	if len(request.URL.RawQuery) > maxCatalogDocumentComboboxRequestBytes {
		http.Error(response, "invalid combobox request", http.StatusBadRequest)
		return
	}
	if request.Method == http.MethodPost {
		request.Body = http.MaxBytesReader(response, request.Body, maxCatalogDocumentComboboxRequestBytes)
	}
	if err := request.ParseForm(); err != nil {
		http.Error(response, "invalid combobox request", http.StatusBadRequest)
		return
	}
	query := request.Form.Get("q")
	if len(query) > maxCatalogDocumentComboboxQueryBytes || !utf8.ValidString(query) {
		http.Error(response, "invalid combobox query", http.StatusBadRequest)
		return
	}
	mount := request.Form.Get(templates.CatalogDocumentComboboxMountFieldName)
	admission, err := handler.runtime.Admit(mount)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer admission.Release()

	response.Header().Set("Cache-Control", "private, no-store")
	response.Header().Set("Vary", "HX-Request, Accept-Encoding")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	ctx := context.WithValue(request.Context(), catalogDocumentComboboxContextKey{}, catalogDocumentComboboxContext{
		mount: mount, snapshot: admission.Snapshot,
	})
	catalogDocumentComboboxHandler.ServeHTTP(response, request.WithContext(ctx))
}

func catalogDocumentComboboxOptions(ctx context.Context, search string, _ map[string]string) ([]combobox.Option, error) {
	value, ok := ctx.Value(catalogDocumentComboboxContextKey{}).(catalogDocumentComboboxContext)
	if !ok {
		return nil, catalog.ErrMountUnavailable
	}
	search = strings.ToLower(strings.TrimSpace(search))
	options := make([]combobox.Option, 0, len(value.snapshot.Directory.Documents))
	for _, document := range value.snapshot.Directory.Documents {
		label := catalogDocumentLabel(document)
		if search != "" && !strings.Contains(strings.ToLower(label), search) && !strings.Contains(strings.ToLower(document.SourcePath), search) {
			continue
		}
		href, err := catalogURL(value.mount, "documents", document.Key)
		if err != nil {
			return nil, err
		}
		options = append(options, combobox.Option{Value: href + "/", Label: label})
	}
	return options, nil
}
