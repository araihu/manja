package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

var errInvalidCatalogDocumentHeaderFragment = errors.New("local docs catalog document-header fragment is invalid")

// CatalogDocumentHeaderFragment owns the immutable identity and description
// shown above a catalog document overview. Provenance stays a composition
// slot because it carries request-specific snapshot and revision values.
type CatalogDocumentHeaderFragment struct {
	data  catalogDocumentHeaderData
	valid bool
}

type catalogDocumentHeaderData struct {
	Title        string
	Version      string
	Description  string
	DownloadHref string
}

// PrepareCatalogDocumentHeader binds the catalog document header to one
// canonical document route and source download route. It copies every value
// consumed by the shared templ component and performs no source parsing.
func PrepareCatalogDocumentHeader(document catalog.DocumentDirectoryV1, documentHref, downloadHref string) (CatalogDocumentHeaderFragment, error) {
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return CatalogDocumentHeaderFragment{}, invalidCatalogDocumentHeaderField("document key")
	}
	if !validCatalogDocumentHeaderDocumentHref(documentHref, document.Key) {
		return CatalogDocumentHeaderFragment{}, invalidCatalogDocumentHeaderField("document href")
	}
	if !validCatalogDocumentHeaderDownloadHref(downloadHref, document) {
		return CatalogDocumentHeaderFragment{}, invalidCatalogDocumentHeaderField("download href")
	}
	if !utf8.ValidString(document.APIVersion) || !utf8.ValidString(document.Overview.Description) {
		return CatalogDocumentHeaderFragment{}, invalidCatalogDocumentHeaderField("document text")
	}
	version := strings.TrimSpace(document.APIVersion)
	if strings.EqualFold(version, "unversioned") {
		version = ""
	}
	title := catalogDocumentDisplayTitle(document)
	data := catalogDocumentHeaderData{
		Title:        title,
		Version:      version,
		Description:  document.Overview.Description,
		DownloadHref: downloadHref,
	}
	fragment := CatalogDocumentHeaderFragment{data: data, valid: true}
	var output boundedBuffer
	if err := catalogDocumentHeader(fragment.data, nil).Render(context.Background(), &output); err != nil {
		return CatalogDocumentHeaderFragment{}, invalidCatalogDocumentHeaderField("rendered bytes")
	}
	return fragment, nil
}

func catalogDocumentDisplayTitle(document catalog.DocumentDirectoryV1) string {
	if title := strings.TrimSpace(document.Title); title != "" {
		return title
	}
	if key := strings.TrimSpace(document.Key); key != "" {
		return key
	}
	return "Untitled document"
}

// CatalogDocumentHeader renders the admitted document header with an
// explicit composition-owned provenance slot.
func CatalogDocumentHeader(fragment CatalogDocumentHeaderFragment, provenance templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogDocumentHeaderFragment
		}
		var output boundedBuffer
		if err := catalogDocumentHeader(fragment.data, provenance).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment CatalogDocumentHeaderFragment) Bytes(ctx context.Context, provenance templ.Component) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogDocumentHeader(fragment, provenance).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func validCatalogDocumentHeaderDocumentHref(value, documentKey string) bool {
	return validDocumentHref(value) && strings.HasSuffix(value, "/documents/"+documentKey+"/")
}

func validCatalogDocumentHeaderDownloadHref(value string, document catalog.DocumentDirectoryV1) bool {
	if !utf8.ValidString(value) || len(value) > 1024 || value == "" || strings.HasSuffix(value, "/") ||
		strings.ContainsAny(value, `\\%?#`) || strings.Contains(value, "//") ||
		domain.ValidateCanonicalPublicPath("catalog document download href", value, false) != nil ||
		path.Clean(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	extension := path.Ext(document.SourceChild)
	if strings.Contains(value, "/openapi/") {
		if len(extension) < 2 || strings.ContainsAny(extension, `/\\%?#`) {
			return false
		}
		return path.Base(value) == document.Key+extension
	}
	return strings.Contains(value, "/documents/"+document.Key+"/")
}

func invalidCatalogDocumentHeaderField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidCatalogDocumentHeaderFragment, strings.TrimSpace(name))
}
