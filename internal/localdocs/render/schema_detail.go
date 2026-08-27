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
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var errInvalidSchemaDetailFragment = errors.New("local docs schema-detail fragment is invalid")

// SchemaDetailFragment owns one immutable schema-detail article assembled from
// the already-prepared header and body children. Actions and provenance stay
// caller-owned composition slots.
type SchemaDetailFragment struct {
	data    schemaDetailData
	binding schemaDetailPreparationBinding
	valid   bool
}

type schemaDetailData struct {
	Anchor string
	Header SchemaDetailHeaderFragment
	Body   SchemaDetailBodyFragment
}

// PrepareSchemaDetail binds the article wrapper to one schema detail, prepared
// schema, document, and matching header/body children. No source parsing or
// runtime state is retained.
func PrepareSchemaDetail(
	detail catalog.DetailRecordV1,
	schema domain.Schema,
	document catalog.DocumentDirectoryV1,
	header *SchemaDetailHeaderFragment,
	body *SchemaDetailBodyFragment,
) (SchemaDetailFragment, error) {
	if detail.Kind != "schema" || detail.Schema == nil || detail.Operation != nil || !validDetailID(detail.ID) {
		return SchemaDetailFragment{}, invalidSchemaDetailField("schema detail")
	}
	projected := detail.Schema
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || strings.TrimSpace(projected.Heading) == "" {
		return SchemaDetailFragment{}, invalidSchemaDetailField("schema detail identity")
	}
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return SchemaDetailFragment{}, invalidSchemaDetailField("document key")
	}
	wantHref := "documents/" + document.Key + "/?selected=" + id + "#" + id
	if projected.Href != wantHref {
		return SchemaDetailFragment{}, invalidSchemaDetailField("schema detail href")
	}
	if schema.Name != projected.Heading || schema.Description != projected.Description {
		return SchemaDetailFragment{}, invalidSchemaDetailField("prepared schema")
	}
	binding, err := bindSchemaDetailPreparation(detail, document.Key)
	if err != nil {
		return SchemaDetailFragment{}, invalidSchemaDetailField("preparation context")
	}
	if header == nil || !header.valid || header.binding != binding || !validSchemaDetailHeaderData(header.data, projected.Heading, projected.Description, document.APIVersion) {
		return SchemaDetailFragment{}, invalidSchemaDetailField("schema header context")
	}
	if body == nil || !body.valid || body.binding != binding || !validSchemaDetailBodyData(body.data, projected, binding) {
		return SchemaDetailFragment{}, invalidSchemaDetailField("schema body context")
	}
	if header.data.TypeLabel != strings.TrimSpace(body.data.Node.data.Type+" "+body.data.Node.data.Format) {
		return SchemaDetailFragment{}, invalidSchemaDetailField("schema type context")
	}

	data := schemaDetailData{
		Anchor: id,
		Header: cloneSchemaDetailHeaderFragment(*header),
		Body:   cloneSchemaDetailBodyFragment(*body),
	}
	fragment := SchemaDetailFragment{data: data, binding: binding, valid: true}
	var output boundedBuffer
	if err := schemaDetail(fragment.data, nil, nil).Render(context.Background(), &output); err != nil {
		return SchemaDetailFragment{}, invalidSchemaDetailField("rendered bytes")
	}
	return fragment, nil
}

func validSchemaDetailHeaderData(data schemaDetailHeaderData, heading, description, apiVersion string) bool {
	if !utf8.ValidString(data.Version) || !utf8.ValidString(data.TypeLabel) || !utf8.ValidString(data.Title) || !utf8.ValidString(data.Description) {
		return false
	}
	version := strings.TrimSpace(apiVersion)
	if strings.EqualFold(version, "unversioned") {
		version = ""
	}
	return data.Title == heading && data.Description == description && data.Version == version
}

func validSchemaDetailBodyData(data schemaDetailBodyData, projected *projection.SchemaDetail, binding schemaDetailPreparationBinding) bool {
	if data.Node == nil || !data.Node.valid || data.Node.binding != binding || data.Node.data.RootHeading != projected.Heading || !validSchemaNodeData(data.Node.data) {
		return false
	}
	if projected.ExampleSchemaJSON == "" {
		return data.Example == nil
	}
	return data.Example != nil && data.Example.valid && data.Example.binding == binding && data.Example.data.JSON == projected.ExampleSchemaJSON && utf8.ValidString(data.Example.data.JSON)
}

func cloneSchemaDetailHeaderFragment(source SchemaDetailHeaderFragment) SchemaDetailHeaderFragment {
	return source
}

func cloneSchemaDetailBodyFragment(source SchemaDetailBodyFragment) SchemaDetailBodyFragment {
	clone := source
	clone.data.Node = cloneSchemaNodeFragment(source.data.Node)
	if source.data.Example != nil {
		example := cloneSchemaDetailExampleFragment(*source.data.Example)
		clone.data.Example = &example
	}
	return clone
}

// SchemaDetail renders the admitted schema-detail article with explicit
// composition-owned action and provenance slots.
func SchemaDetail(fragment SchemaDetailFragment, actions, provenance templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidSchemaDetailFragment
		}
		var output boundedBuffer
		if err := schemaDetail(fragment.data, actions, provenance).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment SchemaDetailFragment) Bytes(ctx context.Context, actions, provenance templ.Component) ([]byte, error) {
	var output bytes.Buffer
	if err := SchemaDetail(fragment, actions, provenance).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidSchemaDetailField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidSchemaDetailFragment, strings.TrimSpace(name))
}
