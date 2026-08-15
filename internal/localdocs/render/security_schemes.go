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

const maximumCatalogDocumentSecuritySchemes = 4096

var errInvalidCatalogDocumentSecuritySchemesFragment = errors.New("local docs catalog document security-schemes fragment is invalid")

// CatalogDocumentSecuritySchemesFragment owns copied, read-only security
// scheme metadata for a catalog document overview. It contains no parser,
// source, network, or request activation behavior.
type CatalogDocumentSecuritySchemesFragment struct {
	data  catalogDocumentSecuritySchemesData
	valid bool
}

type catalogDocumentSecuritySchemesData struct {
	Schemes []catalogDocumentSecuritySchemeData
}

type catalogDocumentSecuritySchemeData struct {
	Name             string
	Anchor           string
	Type             string
	Description      string
	ParameterName    string
	In               string
	Scheme           string
	BearerFormat     string
	OpenIDConnectURL string
}

// PrepareCatalogDocumentSecuritySchemes copies the admitted catalog security
// scheme projection and verifies its bounded identity/text inventory before
// rendering. It performs no source parsing.
func PrepareCatalogDocumentSecuritySchemes(document catalog.DocumentDirectoryV1) (CatalogDocumentSecuritySchemesFragment, error) {
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("document key")
	}
	if len(document.SecuritySchemes) > maximumCatalogDocumentSecuritySchemes {
		return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("security scheme inventory")
	}
	data := catalogDocumentSecuritySchemesData{Schemes: make([]catalogDocumentSecuritySchemeData, 0, len(document.SecuritySchemes))}
	seenNames := make(map[string]struct{}, len(document.SecuritySchemes))
	seenAnchors := make(map[string]struct{}, len(document.SecuritySchemes))
	for _, scheme := range document.SecuritySchemes {
		if domain.ValidateCanonicalIdentity("security scheme name", scheme.Name, false) != nil ||
			domain.ValidateCanonicalIdentity("security scheme anchor", scheme.Anchor, false) != nil {
			return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("security scheme identity")
		}
		if _, duplicate := seenNames[scheme.Name]; duplicate {
			return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("duplicate security scheme name")
		}
		if _, duplicate := seenAnchors[scheme.Anchor]; duplicate {
			return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("duplicate security scheme anchor")
		}
		seenNames[scheme.Name] = struct{}{}
		seenAnchors[scheme.Anchor] = struct{}{}
		for _, value := range []string{
			scheme.Type, scheme.Description, scheme.ParameterName, scheme.In,
			scheme.Scheme, scheme.BearerFormat, scheme.OpenIDConnectURL,
		} {
			if !utf8.ValidString(value) {
				return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("security scheme text")
			}
		}
		data.Schemes = append(data.Schemes, catalogDocumentSecuritySchemeData{
			Name: scheme.Name, Anchor: scheme.Anchor, Type: scheme.Type, Description: scheme.Description,
			ParameterName: scheme.ParameterName, In: scheme.In, Scheme: scheme.Scheme,
			BearerFormat: scheme.BearerFormat, OpenIDConnectURL: scheme.OpenIDConnectURL,
		})
	}
	fragment := CatalogDocumentSecuritySchemesFragment{data: data, valid: true}
	var output boundedBuffer
	if err := catalogDocumentSecuritySchemes(fragment.data).Render(context.Background(), &output); err != nil {
		return CatalogDocumentSecuritySchemesFragment{}, invalidCatalogDocumentSecuritySchemesField("rendered bytes")
	}
	return fragment, nil
}

// CatalogDocumentSecuritySchemes renders copied security scheme metadata.
func CatalogDocumentSecuritySchemes(fragment CatalogDocumentSecuritySchemesFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogDocumentSecuritySchemesFragment
		}
		var output boundedBuffer
		if err := catalogDocumentSecuritySchemes(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment CatalogDocumentSecuritySchemesFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogDocumentSecuritySchemes(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidCatalogDocumentSecuritySchemesField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidCatalogDocumentSecuritySchemesFragment, strings.TrimSpace(name))
}
