package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationRequestBodyMediaFragment = errors.New("local docs operation request-body media fragment is invalid")

type OperationRequestBodyMediaFragment struct {
	media []operationRequestBodyMediaData
	valid bool
}

type operationRequestBodyMediaData struct {
	ContentType string
	SchemaLabel string
	SchemaHref  string
}

func PrepareOperationRequestBodyMedia(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	nodes []projection.SchemaNode,
	documentHref string,
	schemaLinks map[string]string,
) (OperationRequestBodyMediaFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) || !detail.Operation.HasRequestBody || operation.RequestBody == nil {
		return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("operation request body")
	}
	if detail.Operation.ID != string(detail.ID) || detail.Operation.Anchor != string(detail.ID) || operation.Anchor != detail.Operation.Anchor || !validDocumentHref(documentHref) {
		return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("operation identity")
	}
	projected := detail.Operation.RequestBody
	prepared := operation.RequestBody
	if !utf8.ValidString(projected.Description) || projected.Description != prepared.Description || projected.Required != prepared.Required || len(projected.MediaTypes) != len(prepared.MediaTypes) {
		return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("request body")
	}
	nodeByRef := make(map[projection.SchemaRef]projection.SchemaNode, len(nodes))
	nodeIDs := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		ref := projection.SchemaRef(node.Ordinal)
		_, duplicateRef := nodeByRef[ref]
		_, duplicateID := nodeIDs[node.ID]
		if duplicateRef || duplicateID || !validOperationParameterSchemaNode(node) {
			return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("schema-node inventory")
		}
		nodeByRef[ref] = cloneSchemaNode(node)
		nodeIDs[node.ID] = struct{}{}
	}
	fragment := OperationRequestBodyMediaFragment{media: make([]operationRequestBodyMediaData, 0, len(projected.MediaTypes)), valid: true}
	used := make(map[projection.SchemaRef]struct{}, len(projected.MediaTypes))
	mediaIDs := make(map[string]struct{}, len(projected.MediaTypes))
	for index, media := range projected.MediaTypes {
		_, duplicate := mediaIDs[media.ID]
		if media.Ordinal != uint32(index) || media.ID != media.ContentType || duplicate || domain.ValidateCanonicalIdentity("media content type", media.ContentType, false) != nil || media.ContentType != prepared.MediaTypes[index].ContentType {
			return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("media identity")
		}
		mediaIDs[media.ID] = struct{}{}
		if !operationRequestBodyMediaExampleMatches(media.Examples, prepared.MediaTypes[index]) {
			return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("media example")
		}
		node, exists := nodeByRef[media.SchemaRef]
		if !exists {
			return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("missing schema node")
		}
		used[media.SchemaRef] = struct{}{}
		schema := prepared.MediaTypes[index].Schema
		if node.Name != schema.Name || node.Type != schema.Type || node.Format != schema.Format || !slices.Equal(node.Enum, schema.Enum) {
			return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("schema summary")
		}
		data := operationRequestBodyMediaData{ContentType: media.ContentType, SchemaLabel: parameterSchemaInline(schema)}
		if requestBodySchemaIsLinkable(schema) {
			data.SchemaHref = schemaLinks[strings.TrimSpace(schema.Name)]
			if data.SchemaHref != strings.TrimSpace(data.SchemaHref) {
				return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("schema href")
			}
			if data.SchemaHref != "" && !validRequestBodySchemaHref(documentHref, data.SchemaHref) {
				return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("schema href")
			}
		}
		fragment.media = append(fragment.media, data)
	}
	if len(used) != len(nodeByRef) {
		return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("schema-node inventory")
	}
	return fragment, nil
}

func operationRequestBodyMediaExampleMatches(examples []projection.Example, media domain.OperationMediaType) bool {
	if len(examples) == 0 {
		return !media.ExampleProvided && media.Example == ""
	}
	if len(examples) != 1 {
		return false
	}
	example := examples[0]
	return example.Ordinal == 0 && example.ID == "primary" && example.Provided && utf8.ValidString(example.Text) && media.ExampleProvided && media.Example == example.Text
}

func requestBodySchemaIsLinkable(schema domain.SchemaSummary) bool {
	return strings.TrimSpace(schema.Name) != "" && (!parameterSchemaIsPrimitive(schema) || len(schema.Enum) > 0)
}

func validRequestBodySchemaHref(documentHref, href string) bool {
	parsed, err := url.Parse(href)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path != documentHref {
		return false
	}
	values := parsed.Query()
	if len(values) != 1 || len(values["selected"]) != 1 || !validDetailID(domain.DetailID(values.Get("selected"))) || parsed.Fragment != values.Get("selected") {
		return false
	}
	want := documentHref + "?selected=" + url.QueryEscape(values.Get("selected")) + "#" + url.PathEscape(values.Get("selected"))
	return href == want
}

func OperationRequestBodyMedia(fragment OperationRequestBodyMediaFragment, index int) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid || index < 0 || index >= len(fragment.media) {
			return errInvalidOperationRequestBodyMediaFragment
		}
		var output boundedBuffer
		if err := operationRequestBodyMedia(fragment.media[index]).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationRequestBodyMediaFragment) MediaBytes(ctx context.Context, index int) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationRequestBodyMedia(fragment, index).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationRequestBodyMediaField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationRequestBodyMediaFragment, name)
}
