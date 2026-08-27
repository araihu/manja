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
	media   []operationRequestBodyMediaData
	binding operationPreparationBinding
	valid   bool
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
	resolver, err := newOperationRequestBodyMediaSchemaResolver(nodes, len(projected.MediaTypes))
	if err != nil {
		return OperationRequestBodyMediaFragment{}, err
	}
	fragment := OperationRequestBodyMediaFragment{media: make([]operationRequestBodyMediaData, 0, len(projected.MediaTypes)), valid: true}
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
		schema := prepared.MediaTypes[index].Schema
		if err := resolver.validate(media.SchemaRef, schema, 0); err != nil {
			return OperationRequestBodyMediaFragment{}, err
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
	if len(resolver.used) != len(resolver.nodes) {
		return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("schema-node inventory")
	}
	fragment.binding, err = bindOperationPreparation(detail, operation, documentHref, schemaLinks)
	if err != nil {
		return OperationRequestBodyMediaFragment{}, invalidOperationRequestBodyMediaField("preparation context")
	}
	return fragment, nil
}

type operationRequestBodyMediaSchemaResolver struct {
	nodes  map[projection.SchemaRef]projection.SchemaNode
	used   map[projection.SchemaRef]struct{}
	active map[projection.SchemaRef]bool
}

func newOperationRequestBodyMediaSchemaResolver(nodes []projection.SchemaNode, mediaCount int) (*operationRequestBodyMediaSchemaResolver, error) {
	if mediaCount < 0 || len(nodes) > maximumParameterSchemaNodes+mediaCount {
		return nil, invalidOperationRequestBodyMediaField("schema-node inventory")
	}
	resolver := &operationRequestBodyMediaSchemaResolver{
		nodes:  make(map[projection.SchemaRef]projection.SchemaNode, len(nodes)),
		used:   make(map[projection.SchemaRef]struct{}, len(nodes)),
		active: make(map[projection.SchemaRef]bool),
	}
	ids := make(map[string]struct{}, len(nodes))
	var previous uint32
	for index, node := range nodes {
		ref := projection.SchemaRef(node.Ordinal)
		_, duplicateRef := resolver.nodes[ref]
		_, duplicateID := ids[node.ID]
		if (index > 0 && node.Ordinal <= previous) || duplicateRef || duplicateID || !validOperationParameterSchemaNode(node) {
			return nil, invalidOperationRequestBodyMediaField("schema-node inventory")
		}
		resolver.nodes[ref] = cloneSchemaNode(node)
		ids[node.ID] = struct{}{}
		previous = node.Ordinal
	}
	return resolver, nil
}

func (resolver *operationRequestBodyMediaSchemaResolver) validate(ref projection.SchemaRef, schema domain.SchemaSummary, depth int) error {
	node, exists := resolver.nodes[ref]
	if !exists {
		return invalidOperationRequestBodyMediaField("missing schema node")
	}
	resolver.used[ref] = struct{}{}
	if node.Name != schema.Name || node.Type != schema.Type || node.Format != schema.Format || !slices.Equal(node.Enum, schema.Enum) {
		return invalidOperationRequestBodyMediaField("schema summary")
	}
	if depth >= maximumParameterSchemaDepth || resolver.active[ref] {
		if schema.Items != nil {
			return invalidOperationRequestBodyMediaField("schema summary")
		}
		return nil
	}
	if (len(node.Items) == 1) != (schema.Items != nil) {
		return invalidOperationRequestBodyMediaField("schema summary")
	}
	if schema.Items == nil {
		return nil
	}
	resolver.active[ref] = true
	defer delete(resolver.active, ref)
	return resolver.validate(node.Items[0].SchemaRef, *schema.Items, depth+1)
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
