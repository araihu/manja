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

const (
	maximumCatalogOverviewMetricsDocuments  = 256
	maximumCatalogOverviewMetricsOperations = 20_000
	maximumCatalogOverviewMetricsSchemas    = 20_000
)

var errInvalidCatalogOverviewMetricsFragment = errors.New("local docs catalog overview metrics fragment is invalid")

// CatalogOverviewMetricsFragment owns copied catalog overview counts. It
// contains no source, network, parser, or request activation behavior.
type CatalogOverviewMetricsFragment struct {
	data  catalogOverviewMetricsData
	valid bool
}

type catalogOverviewMetricsData struct {
	Documents  uint64
	Operations uint64
	Schemas    uint64
}

// PrepareCatalogOverviewMetrics copies the bounded catalog inventory counts
// consumed by the shared overview component. It performs no source parsing.
//
// Keep this strict wrapper for callers that render request-sized responses.
// Static export and other explicitly unbounded renderers should call
// PrepareCatalogOverviewMetricsWithResourceLimits with resourceLimits=false.
func PrepareCatalogOverviewMetrics(directory catalog.CatalogArtifactV1) (CatalogOverviewMetricsFragment, error) {
	return PrepareCatalogOverviewMetricsWithResourceLimits(directory, true)
}

// PrepareCatalogOverviewMetricsWithResourceLimits copies catalog inventory
// counts while applying request resource limits only when resourceLimits is
// true. The unbounded mode is used by static export, whose output is already
// admitted and written as a complete artifact. Identity and integer-overflow
// validation remain enabled in both modes.
func PrepareCatalogOverviewMetricsWithResourceLimits(directory catalog.CatalogArtifactV1, resourceLimits bool) (CatalogOverviewMetricsFragment, error) {
	if domain.ValidateCatalogID(directory.CatalogID) != nil || !utf8.ValidString(directory.Title) {
		return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("catalog identity")
	}
	if len(directory.Documents) == 0 || (resourceLimits && len(directory.Documents) > maximumCatalogOverviewMetricsDocuments) {
		return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("document inventory")
	}
	seenKeys := make(map[string]struct{}, len(directory.Documents))
	data := catalogOverviewMetricsData{Documents: uint64(len(directory.Documents))}
	for _, document := range directory.Documents {
		if domain.ValidateCatalogDocumentKey(document.Key) != nil || !utf8.ValidString(document.Key) {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("document identity")
		}
		if _, exists := seenKeys[document.Key]; exists {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("duplicate document identity")
		}
		seenKeys[document.Key] = struct{}{}
		operations := uint64(len(document.Operations))
		schemas := uint64(len(document.Schemas))
		if resourceLimits && operations > maximumCatalogOverviewMetricsOperations-data.Operations {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("operation inventory")
		}
		if resourceLimits && schemas > maximumCatalogOverviewMetricsSchemas-data.Schemas {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("schema inventory")
		}
		if operations > ^uint64(0)-data.Operations {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("operation inventory overflow")
		}
		if schemas > ^uint64(0)-data.Schemas {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("schema inventory overflow")
		}
		data.Operations += operations
		data.Schemas += schemas
	}
	fragment := CatalogOverviewMetricsFragment{data: data, valid: true}
	var output boundedBuffer
	if err := catalogOverviewMetrics(fragment.data).Render(context.Background(), &output); err != nil {
		return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("rendered bytes")
	}
	return fragment, nil
}

// CatalogOverviewMetrics renders copied catalog inventory counts.
func CatalogOverviewMetrics(fragment CatalogOverviewMetricsFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogOverviewMetricsFragment
		}
		var output boundedBuffer
		if err := catalogOverviewMetrics(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment CatalogOverviewMetricsFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogOverviewMetrics(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidCatalogOverviewMetricsField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidCatalogOverviewMetricsFragment, strings.TrimSpace(name))
}
