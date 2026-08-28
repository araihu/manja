package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

const maximumCatalogDocumentMetricsCount = 20_000

var errInvalidCatalogDocumentMetricsFragment = errors.New("local docs catalog document metrics fragment is invalid")

// CatalogDocumentMetricsFragment owns the copied operation and schema counts
// shown in a catalog document overview. It contains no source, network,
// parser, or request activation behavior.
type CatalogDocumentMetricsFragment struct {
	data  catalogDocumentMetricsData
	valid bool
}

type catalogDocumentMetricsData struct {
	Operations int
	Schemas    int
}

// PrepareCatalogDocumentMetrics copies the bounded catalog-document counts
// consumed by the shared overview component. It performs no source parsing.
//
// Keep this strict wrapper for callers that render request-sized responses.
// Static export and other explicitly unbounded renderers should call
// PrepareCatalogDocumentMetricsWithResourceLimits with resourceLimits=false.
func PrepareCatalogDocumentMetrics(document catalog.DocumentDirectoryV1) (CatalogDocumentMetricsFragment, error) {
	return PrepareCatalogDocumentMetricsWithResourceLimits(document, true)
}

// PrepareCatalogDocumentMetricsWithResourceLimits copies document counts
// while applying request resource limits only when resourceLimits is true.
// Document identity and rendered-fragment limits remain enabled in both modes.
func PrepareCatalogDocumentMetricsWithResourceLimits(document catalog.DocumentDirectoryV1, resourceLimits bool) (CatalogDocumentMetricsFragment, error) {
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return CatalogDocumentMetricsFragment{}, invalidCatalogDocumentMetricsField("document key")
	}
	if resourceLimits && len(document.Operations) > maximumCatalogDocumentMetricsCount {
		return CatalogDocumentMetricsFragment{}, invalidCatalogDocumentMetricsField("operation inventory")
	}
	if resourceLimits && len(document.Schemas) > maximumCatalogDocumentMetricsCount {
		return CatalogDocumentMetricsFragment{}, invalidCatalogDocumentMetricsField("schema inventory")
	}
	fragment := CatalogDocumentMetricsFragment{
		data: catalogDocumentMetricsData{
			Operations: len(document.Operations),
			Schemas:    len(document.Schemas),
		},
		valid: true,
	}
	var output boundedBuffer
	if err := catalogDocumentMetrics(fragment.data).Render(context.Background(), &output); err != nil {
		return CatalogDocumentMetricsFragment{}, invalidCatalogDocumentMetricsField("rendered bytes")
	}
	return fragment, nil
}

// CatalogDocumentMetrics renders copied catalog-document counts.
func CatalogDocumentMetrics(fragment CatalogDocumentMetricsFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogDocumentMetricsFragment
		}
		var output boundedBuffer
		if err := catalogDocumentMetrics(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment CatalogDocumentMetricsFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogDocumentMetrics(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidCatalogDocumentMetricsField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidCatalogDocumentMetricsFragment, strings.TrimSpace(name))
}
