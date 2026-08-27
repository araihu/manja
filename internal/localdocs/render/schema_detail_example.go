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

var errInvalidSchemaDetailExampleFragment = errors.New("local docs schema-detail example fragment is invalid")

// SchemaDetailExampleFragment owns the immutable schema example bytes shown
// below a prepared schema header. It receives already-prepared projection
// content and never reparses source JSON.
type SchemaDetailExampleFragment struct {
	data    schemaDetailExampleData
	binding schemaDetailPreparationBinding
	valid   bool
}

type schemaDetailExampleData struct {
	JSON string
}

// PrepareSchemaDetailExample binds the example to one verified schema detail,
// prepared schema, and document identity. The copied string remains stable
// after caller mutation.
func PrepareSchemaDetailExample(detail catalog.DetailRecordV1, schema domain.Schema, document catalog.DocumentDirectoryV1) (SchemaDetailExampleFragment, error) {
	if detail.Kind != "schema" || detail.Schema == nil || detail.Operation != nil || !validDetailID(detail.ID) {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("schema detail")
	}
	projected := detail.Schema
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || strings.TrimSpace(projected.Heading) == "" {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("schema detail identity")
	}
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("document key")
	}
	wantHref := "documents/" + document.Key + "/?selected=" + id + "#" + id
	if projected.Href != wantHref {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("schema detail href")
	}
	if schema.Name != projected.Heading || schema.Description != projected.Description {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("prepared schema")
	}
	if projected.ExampleSchemaJSON == "" || schema.Example.JSON != projected.ExampleSchemaJSON {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("prepared example")
	}
	if !utf8.ValidString(projected.ExampleSchemaJSON) || len(projected.ExampleSchemaJSON) > maximumHTMLFragmentBytes {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("example bytes")
	}
	binding, err := bindSchemaDetailPreparation(detail, document.Key)
	if err != nil {
		return SchemaDetailExampleFragment{}, invalidSchemaDetailExampleField("preparation context")
	}
	return SchemaDetailExampleFragment{data: schemaDetailExampleData{JSON: projected.ExampleSchemaJSON}, binding: binding, valid: true}, nil
}

// SchemaDetailExample renders the admitted schema example code block.
func SchemaDetailExample(fragment SchemaDetailExampleFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidSchemaDetailExampleFragment
		}
		var output boundedBuffer
		if err := schemaDetailExample(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment SchemaDetailExampleFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := SchemaDetailExample(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidSchemaDetailExampleField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidSchemaDetailExampleFragment, strings.TrimSpace(name))
}
