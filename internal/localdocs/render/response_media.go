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

var errInvalidOperationResponseMediaFragment = errors.New("local docs operation response-media fragment is invalid")

// OperationResponseMediaFragment holds admitted SSR-equivalent response media labels.
// It intentionally excludes response descriptions, headers, examples, and schema trees.
type OperationResponseMediaFragment struct {
	media   [][]operationResponseMediaData
	binding operationPreparationBinding
	valid   bool
}

type operationResponseMediaData struct {
	ContentType string
	SchemaLabel string
	SchemaHref  string
}

func PrepareOperationResponseMedia(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	nodes []projection.SchemaNode,
	documentHref string,
	schemaLinks map[string]string,
) (OperationResponseMediaFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("operation")
	}
	if detail.Operation.ID != string(detail.ID) || detail.Operation.Anchor != string(detail.ID) || operation.Anchor != detail.Operation.Anchor || !validDocumentHref(documentHref) {
		return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("operation identity")
	}
	projected := detail.Operation.Responses
	prepared := operation.Responses
	if len(projected) != len(prepared) {
		return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("response inventory")
	}
	mediaCount := 0
	for _, response := range projected {
		mediaCount += len(response.MediaTypes)
	}
	resolver, err := newOperationResponseMediaSchemaResolver(nodes, mediaCount)
	if err != nil {
		return OperationResponseMediaFragment{}, err
	}
	fragment := OperationResponseMediaFragment{media: make([][]operationResponseMediaData, len(projected)), valid: true}
	responseIDs := make(map[string]struct{}, len(projected))
	for responseIndex, response := range projected {
		_, duplicate := responseIDs[response.ID]
		preparedResponse := prepared[responseIndex]
		if response.Ordinal != uint32(responseIndex) || response.ID != response.Status || duplicate || domain.ValidateCanonicalIdentity("response status", response.Status, false) != nil || response.Status != preparedResponse.Status || len(response.MediaTypes) != len(preparedResponse.MediaTypes) {
			return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("response identity")
		}
		responseIDs[response.ID] = struct{}{}
		fragment.media[responseIndex] = make([]operationResponseMediaData, 0, len(response.MediaTypes))
		mediaIDs := make(map[string]struct{}, len(response.MediaTypes))
		for mediaIndex, media := range response.MediaTypes {
			_, duplicate := mediaIDs[media.ID]
			preparedMedia := preparedResponse.MediaTypes[mediaIndex]
			if media.Ordinal != uint32(mediaIndex) || media.ID != media.ContentType || duplicate || domain.ValidateCanonicalIdentity("media content type", media.ContentType, false) != nil || media.ContentType != preparedMedia.ContentType {
				return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("media identity")
			}
			mediaIDs[media.ID] = struct{}{}
			if !operationResponseMediaExampleMatches(media.Examples, preparedMedia) {
				return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("media example")
			}
			schema := preparedMedia.Schema
			if err := resolver.validate(media.SchemaRef, schema, 0); err != nil {
				return OperationResponseMediaFragment{}, err
			}
			data := operationResponseMediaData{ContentType: media.ContentType, SchemaLabel: parameterSchemaInline(schema)}
			if responseSchemaIsLinkable(schema) {
				data.SchemaHref = schemaLinks[strings.TrimSpace(schema.Name)]
				if data.SchemaHref != strings.TrimSpace(data.SchemaHref) || (data.SchemaHref != "" && !validResponseSchemaHref(documentHref, data.SchemaHref)) {
					return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("schema href")
				}
			}
			fragment.media[responseIndex] = append(fragment.media[responseIndex], data)
		}
	}
	if len(resolver.used) != len(resolver.nodes) {
		return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("schema-node inventory")
	}
	fragment.binding, err = bindOperationPreparation(detail, operation, documentHref, schemaLinks)
	if err != nil {
		return OperationResponseMediaFragment{}, invalidOperationResponseMediaField("preparation context")
	}
	return fragment, nil
}

type operationResponseMediaSchemaResolver struct {
	nodes  map[projection.SchemaRef]projection.SchemaNode
	used   map[projection.SchemaRef]struct{}
	active map[projection.SchemaRef]bool
}

func newOperationResponseMediaSchemaResolver(nodes []projection.SchemaNode, mediaCount int) (*operationResponseMediaSchemaResolver, error) {
	if mediaCount < 0 || len(nodes) > maximumParameterSchemaNodes+mediaCount {
		return nil, invalidOperationResponseMediaField("schema-node inventory")
	}
	resolver := &operationResponseMediaSchemaResolver{nodes: make(map[projection.SchemaRef]projection.SchemaNode, len(nodes)), used: make(map[projection.SchemaRef]struct{}, len(nodes)), active: make(map[projection.SchemaRef]bool)}
	ids := make(map[string]struct{}, len(nodes))
	var previous uint32
	for index, node := range nodes {
		ref := projection.SchemaRef(node.Ordinal)
		_, duplicateRef := resolver.nodes[ref]
		_, duplicateID := ids[node.ID]
		if (index > 0 && node.Ordinal <= previous) || duplicateRef || duplicateID || !validOperationParameterSchemaNode(node) {
			return nil, invalidOperationResponseMediaField("schema-node inventory")
		}
		resolver.nodes[ref] = cloneSchemaNode(node)
		ids[node.ID] = struct{}{}
		previous = node.Ordinal
	}
	return resolver, nil
}

func (resolver *operationResponseMediaSchemaResolver) validate(ref projection.SchemaRef, schema domain.SchemaSummary, depth int) error {
	node, exists := resolver.nodes[ref]
	if !exists {
		return invalidOperationResponseMediaField("missing schema node")
	}
	resolver.used[ref] = struct{}{}
	if node.Name != schema.Name || node.Type != schema.Type || node.Format != schema.Format || !slices.Equal(node.Enum, schema.Enum) {
		return invalidOperationResponseMediaField("schema summary")
	}
	if depth >= maximumParameterSchemaDepth || resolver.active[ref] {
		if schema.Items != nil {
			return invalidOperationResponseMediaField("schema summary")
		}
		return nil
	}
	if (len(node.Items) == 1) != (schema.Items != nil) {
		return invalidOperationResponseMediaField("schema summary")
	}
	if schema.Items == nil {
		return nil
	}
	resolver.active[ref] = true
	defer delete(resolver.active, ref)
	return resolver.validate(node.Items[0].SchemaRef, *schema.Items, depth+1)
}

func operationResponseMediaExampleMatches(examples []projection.Example, media domain.OperationMediaType) bool {
	if len(examples) == 0 {
		return !media.ExampleProvided && media.Example == ""
	}
	if len(examples) != 1 {
		return false
	}
	example := examples[0]
	return example.Ordinal == 0 && example.ID == "primary" && example.Provided && utf8.ValidString(example.Text) && media.ExampleProvided && media.Example == example.Text
}

func responseSchemaIsLinkable(schema domain.SchemaSummary) bool {
	return strings.TrimSpace(schema.Name) != "" && (!parameterSchemaIsPrimitive(schema) || len(schema.Enum) > 0)
}

func validResponseSchemaHref(documentHref, href string) bool {
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

func OperationResponseMedia(fragment OperationResponseMediaFragment, responseIndex, mediaIndex int) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid || responseIndex < 0 || responseIndex >= len(fragment.media) || mediaIndex < 0 || mediaIndex >= len(fragment.media[responseIndex]) {
			return errInvalidOperationResponseMediaFragment
		}
		var output boundedBuffer
		if err := operationResponseMedia(fragment.media[responseIndex][mediaIndex]).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationResponseMediaFragment) MediaBytes(ctx context.Context, responseIndex, mediaIndex int) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationResponseMedia(fragment, responseIndex, mediaIndex).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationResponseMediaField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationResponseMediaFragment, name)
}
