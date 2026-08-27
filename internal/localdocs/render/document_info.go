package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

var errInvalidCatalogDocumentInfoFragment = errors.New("local docs catalog document-info fragment is invalid")

// CatalogDocumentInfoFragment owns the copied OpenAPI contact, license, and
// terms metadata shown below a catalog document header. It renders metadata
// only; source parsing and document navigation remain outside this seam.
type CatalogDocumentInfoFragment struct {
	data  catalogDocumentInfoData
	valid bool
}

type catalogDocumentInfoData struct {
	ContactName  string
	ContactURL   string
	ContactEmail string

	LicenseName       string
	LicenseURL        string
	LicenseIdentifier string

	TermsOfService string
	HasContact     bool
	HasLicense     bool
	HasTerms       bool
	HasInfo        bool
}

// PrepareCatalogDocumentInfo copies every value consumed by the shared
// catalog document information component. It performs no source parsing and
// rejects invalid text before any component can retain or render it.
func PrepareCatalogDocumentInfo(document catalog.DocumentDirectoryV1) (CatalogDocumentInfoFragment, error) {
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return CatalogDocumentInfoFragment{}, invalidCatalogDocumentInfoField("document key")
	}
	values := []string{
		document.Overview.Contact.Name,
		document.Overview.Contact.URL,
		document.Overview.Contact.Email,
		document.Overview.License.Name,
		document.Overview.License.URL,
		document.Overview.License.Identifier,
		document.Overview.TermsOfService,
	}
	for _, value := range values {
		if !utf8.ValidString(value) {
			return CatalogDocumentInfoFragment{}, invalidCatalogDocumentInfoField("document text")
		}
	}

	contact := document.Overview.Contact
	license := document.Overview.License
	data := catalogDocumentInfoData{
		ContactName:       contact.Name,
		ContactURL:        contact.URL,
		ContactEmail:      contact.Email,
		LicenseName:       license.Name,
		LicenseURL:        license.URL,
		LicenseIdentifier: license.Identifier,
		TermsOfService:    document.Overview.TermsOfService,
		HasContact:        strings.TrimSpace(contact.Name) != "" || strings.TrimSpace(contact.URL) != "" || strings.TrimSpace(contact.Email) != "",
		HasLicense:        strings.TrimSpace(license.Name) != "" || strings.TrimSpace(license.URL) != "" || strings.TrimSpace(license.Identifier) != "",
		HasTerms:          document.Overview.TermsOfService != "",
	}
	data.HasInfo = data.HasContact || data.HasLicense || strings.TrimSpace(document.Overview.TermsOfService) != ""
	fragment := CatalogDocumentInfoFragment{data: data, valid: true}
	var output boundedBuffer
	if err := catalogDocumentInfo(fragment.data).Render(context.Background(), &output); err != nil {
		return CatalogDocumentInfoFragment{}, invalidCatalogDocumentInfoField("rendered bytes")
	}
	return fragment, nil
}

// CatalogDocumentInfo renders copied OpenAPI information metadata.
func CatalogDocumentInfo(fragment CatalogDocumentInfoFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogDocumentInfoFragment
		}
		var output boundedBuffer
		if err := catalogDocumentInfo(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment CatalogDocumentInfoFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogDocumentInfo(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidCatalogDocumentInfoField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidCatalogDocumentInfoFragment, strings.TrimSpace(name))
}
