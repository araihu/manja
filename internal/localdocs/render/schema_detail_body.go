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

var errInvalidSchemaDetailBodyFragment = errors.New("local docs schema-detail body fragment is invalid")

// SchemaDetailBodyFragment owns the prepared schema-node and optional root
// example panels below a schema-detail header. Child fragments are copied at
// admission, so rendering never reads mutable projection or domain values.
type SchemaDetailBodyFragment struct {
	data    schemaDetailBodyData
	binding schemaDetailPreparationBinding
	valid   bool
}

type schemaDetailBodyData struct {
	Node    *SchemaNodeFragment
	Example *SchemaDetailExampleFragment
}

// PrepareSchemaDetailBody binds the schema-detail body to one immutable
// detail/document context and already-prepared child fragments. Header,
// actions, provenance, and endpoint composition remain outside this seam.
func PrepareSchemaDetailBody(
	detail catalog.DetailRecordV1,
	schema domain.Schema,
	document catalog.DocumentDirectoryV1,
	node *SchemaNodeFragment,
	example *SchemaDetailExampleFragment,
) (SchemaDetailBodyFragment, error) {
	if detail.Kind != "schema" || detail.Schema == nil || detail.Operation != nil || !validDetailID(detail.ID) {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("schema detail")
	}
	projected := detail.Schema
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || strings.TrimSpace(projected.Heading) == "" {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("schema detail identity")
	}
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("document key")
	}
	wantHref := "documents/" + document.Key + "/?selected=" + id + "#" + id
	if projected.Href != wantHref {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("schema detail href")
	}
	if schema.Name != projected.Heading || schema.Description != projected.Description {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("prepared schema")
	}
	if schema.Example.JSON != projected.ExampleSchemaJSON {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("prepared example")
	}
	binding, err := bindSchemaDetailPreparation(detail, document.Key)
	if err != nil {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("preparation context")
	}
	if node == nil || !node.valid || node.binding != binding || node.data.RootHeading != projected.Heading || !validSchemaNodeData(node.data) {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("schema node context")
	}
	if projected.ExampleSchemaJSON == "" {
		if example != nil {
			return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("unexpected example")
		}
	} else {
		if example == nil || !example.valid || example.binding != binding || example.data.JSON != projected.ExampleSchemaJSON || !utf8.ValidString(example.data.JSON) {
			return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("schema example context")
		}
	}
	data := schemaDetailBodyData{Node: cloneSchemaNodeFragment(node)}
	if example != nil {
		clone := cloneSchemaDetailExampleFragment(*example)
		data.Example = &clone
	}
	fragment := SchemaDetailBodyFragment{data: data, binding: binding, valid: true}
	var output boundedBuffer
	if err := schemaDetailBody(fragment.data).Render(context.Background(), &output); err != nil {
		return SchemaDetailBodyFragment{}, invalidSchemaDetailBodyField("rendered bytes")
	}
	return fragment, nil
}

func validSchemaNodeData(data schemaNodeData) bool {
	values := []string{data.Name, data.RootHeading, data.Type, data.Format, data.Description, data.DefaultValue, data.ExampleText}
	for _, value := range values {
		if !utf8.ValidString(value) {
			return false
		}
	}
	for _, value := range data.EnumValues {
		if !utf8.ValidString(value) {
			return false
		}
	}
	for _, constraint := range data.Constraints {
		if !utf8.ValidString(constraint.Name) || !utf8.ValidString(constraint.Value) {
			return false
		}
	}
	for _, edge := range data.Edges {
		for _, value := range []string{edge.Name, edge.Type, edge.Reference, edge.Description, edge.DefaultValue, edge.ExampleText, edge.ReferenceHref} {
			if !utf8.ValidString(value) {
				return false
			}
		}
		for _, value := range edge.EnumValues {
			if !utf8.ValidString(value) {
				return false
			}
		}
		for _, constraint := range edge.Constraints {
			if !utf8.ValidString(constraint.Name) || !utf8.ValidString(constraint.Value) {
				return false
			}
		}
	}
	return true
}

func cloneSchemaNodeFragment(source *SchemaNodeFragment) *SchemaNodeFragment {
	if source == nil {
		return nil
	}
	clone := *source
	clone.data.EnumValues = append([]string(nil), source.data.EnumValues...)
	clone.data.Constraints = append([]schemaConstraintData(nil), source.data.Constraints...)
	clone.data.Edges = make([]schemaEdgeData, len(source.data.Edges))
	for index, edge := range source.data.Edges {
		clone.data.Edges[index] = edge
		clone.data.Edges[index].EnumValues = append([]string(nil), edge.EnumValues...)
		clone.data.Edges[index].Constraints = append([]schemaConstraintData(nil), edge.Constraints...)
	}
	return &clone
}

func cloneSchemaDetailExampleFragment(source SchemaDetailExampleFragment) SchemaDetailExampleFragment {
	clone := source
	return clone
}

func schemaDetailBodyNode(fragment SchemaNodeFragment) templ.Component {
	// Legacy catalog SSR leaves one separator after a node-only schema body.
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if err := SchemaNode(fragment).Render(ctx, writer); err != nil {
			return err
		}
		_, err := io.WriteString(writer, " ")
		return err
	})
}

// SchemaDetailBody renders the admitted schema-detail body.
func SchemaDetailBody(fragment SchemaDetailBodyFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidSchemaDetailBodyFragment
		}
		var output boundedBuffer
		if err := schemaDetailBody(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment SchemaDetailBodyFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := SchemaDetailBody(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidSchemaDetailBodyField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidSchemaDetailBodyFragment, strings.TrimSpace(name))
}
