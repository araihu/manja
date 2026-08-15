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
func PrepareCatalogOverviewMetrics(directory catalog.CatalogArtifactV1) (CatalogOverviewMetricsFragment, error) {
	if domain.ValidateCatalogID(directory.CatalogID) != nil || !utf8.ValidString(directory.Title) {
		return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("catalog identity")
	}
	if len(directory.Documents) == 0 || len(directory.Documents) > maximumCatalogOverviewMetricsDocuments {
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
		if uint64(len(document.Operations)) > maximumCatalogOverviewMetricsOperations-data.Operations {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("operation inventory")
		}
		if uint64(len(document.Schemas)) > maximumCatalogOverviewMetricsSchemas-data.Schemas {
			return CatalogOverviewMetricsFragment{}, invalidCatalogOverviewMetricsField("schema inventory")
		}
		data.Operations += uint64(len(document.Operations))
		data.Schemas += uint64(len(document.Schemas))
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
