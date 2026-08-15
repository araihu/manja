package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationResponseDetailsFragment = errors.New("local docs operation response-details fragment is invalid")

// OperationResponseDetailsFragment holds admitted SSR-equivalent response descriptions and headers,
// including header example fallbacks and canonical schema hrefs. Response media, body examples, and
// body schema trees remain outside this fragment.
type OperationResponseDetailsFragment struct {
	responses   []operationResponseDetailData
	schemaLinks map[string]string
	binding     operationPreparationBinding
	valid       bool
}

type operationResponseDetailData struct {
	Description string
	Headers     []operationResponseHeaderData
}

type operationResponseHeaderData struct {
	ID          string
	Name        string
	Location    string
	Required    bool
	Description string
	Example     string
	Schema      domain.SchemaSummary
}

func PrepareOperationResponseDetails(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	nodes []projection.SchemaNode,
	documentHref string,
	schemaLinks map[string]string,
) (OperationResponseDetailsFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationResponseDetailsFragment{}, invalidOperationResponseDetailsField("operation")
	}
	if detail.Operation.ID != string(detail.ID) || detail.Operation.Anchor != string(detail.ID) || operation.Anchor != detail.Operation.Anchor || !validDocumentHref(documentHref) {
		return OperationResponseDetailsFragment{}, invalidOperationResponseDetailsField("operation identity")
	}
	projected := detail.Operation.Responses
	prepared := operation.Responses
	if len(projected) != len(prepared) {
		return OperationResponseDetailsFragment{}, invalidOperationResponseDetailsField("response inventory")
	}

	headerCount := 0
	for _, response := range projected {
		headerCount += len(response.Headers)
	}
	resolver, err := newOperationResponseDetailsSchemaResolver(nodes, headerCount)
	if err != nil {
		return OperationResponseDetailsFragment{}, err
	}
	links := cloneResponseDetailSchemaLinks(schemaLinks)
	fragment := OperationResponseDetailsFragment{
		responses: make([]operationResponseDetailData, 0, len(projected)),
		valid:     true,
	}
	responseIDs := make(map[string]struct{}, len(projected))
	for responseIndex, response := range projected {
		preparedResponse := prepared[responseIndex]
		if err := validateOperationResponseDetail(responseIndex, response, preparedResponse, responseIDs); err != nil {
			return OperationResponseDetailsFragment{}, err
		}
		data := operationResponseDetailData{Description: preparedResponse.Description, Headers: make([]operationResponseHeaderData, 0, len(response.Headers))}
		headerIDs := make(map[string]struct{}, len(response.Headers))
		for headerIndex, header := range response.Headers {
			preparedHeader := preparedResponse.Headers[headerIndex]
			schema, err := resolver.schema(header.SchemaRef, 0)
			if err != nil {
				return OperationResponseDetailsFragment{}, err
			}
			if err := validateOperationResponseHeaderDetail(headerIndex, header, preparedHeader, schema, headerIDs); err != nil {
				return OperationResponseDetailsFragment{}, err
			}
			if err := admitOperationResponseDetailSchemaLinks(preparedHeader.Schema, documentHref, links, &fragment.schemaLinks); err != nil {
				return OperationResponseDetailsFragment{}, err
			}
			data.Headers = append(data.Headers, operationResponseHeaderData{
				ID: header.ID, Name: preparedHeader.Name, Location: "header", Required: false,
				Description: preparedHeader.Description, Example: preparedHeader.Example,
				Schema: cloneResponseDetailSchema(preparedHeader.Schema),
			})
		}
		fragment.responses = append(fragment.responses, data)
	}
	if len(resolver.used) != len(resolver.nodes) {
		return OperationResponseDetailsFragment{}, invalidOperationResponseDetailsField("schema-node inventory")
	}
	fragment.binding, err = bindOperationPreparation(detail, operation, documentHref, schemaLinks)
	if err != nil {
		return OperationResponseDetailsFragment{}, invalidOperationResponseDetailsField("preparation context")
	}
	return fragment, nil
}

func validateOperationResponseDetail(index int, response projection.Response, prepared domain.OperationResponse, seen map[string]struct{}) error {
	_, duplicate := seen[response.ID]
	if response.Ordinal != uint32(index) || response.ID != response.Status || duplicate || domain.ValidateCanonicalIdentity("response status", response.Status, false) != nil || !utf8.ValidString(response.Description) || response.Status != prepared.Status || response.Description != prepared.Description || len(response.Headers) != len(prepared.Headers) {
		return invalidOperationResponseDetailsField("response identity")
	}
	seen[response.ID] = struct{}{}
	return nil
}

func validateOperationResponseHeaderDetail(index int, header projection.ResponseHeader, prepared domain.OperationResponseHeader, schema domain.SchemaSummary, seen map[string]struct{}) error {
	_, duplicate := seen[header.ID]
	if header.Ordinal != uint32(index) || header.ID != operationResponseHeaderID(header.Name) || duplicate {
		return invalidOperationResponseDetailsField("response header identity")
	}
	if domain.ValidateCanonicalIdentity("response header name", header.Name, false) != nil || !utf8.ValidString(header.Description) {
		return invalidOperationResponseDetailsField("response header fields")
	}
	if header.Name != prepared.Name || header.Description != prepared.Description {
		return invalidOperationResponseDetailsField("prepared response header")
	}
	if !operationResponseHeaderExampleMatches(header.Examples, prepared.Example) {
		return invalidOperationResponseDetailsField("response header example")
	}
	if !responseDetailSchemaEqual(schema, prepared.Schema) {
		return invalidOperationResponseDetailsField("response header schema")
	}
	seen[header.ID] = struct{}{}
	return nil
}

func responseDetailSchemaEqual(left, right domain.SchemaSummary) bool {
	if left.Name != right.Name || left.Type != right.Type || left.Format != right.Format || left.Description != right.Description || left.Default != right.Default || left.Example != right.Example || left.Nullable != right.Nullable || left.Deprecated != right.Deprecated || left.JSON != right.JSON || !slices.Equal(left.Enum, right.Enum) || len(left.Constraints) != len(right.Constraints) || len(left.Properties) != len(right.Properties) {
		return false
	}
	for index := range left.Constraints {
		if left.Constraints[index] != right.Constraints[index] {
			return false
		}
	}
	for index := range left.Properties {
		leftProperty, rightProperty := left.Properties[index], right.Properties[index]
		if leftProperty.Name != rightProperty.Name || leftProperty.Required != rightProperty.Required || leftProperty.Description != rightProperty.Description || !responseDetailSchemaEqual(leftProperty.Schema, rightProperty.Schema) {
			return false
		}
	}
	if left.Items == nil || right.Items == nil {
		return left.Items == nil && right.Items == nil
	}
	return responseDetailSchemaEqual(*left.Items, *right.Items)
}

func operationResponseHeaderID(name string) string {
	hash := sha256.New()
	hash.Write([]byte("response-header"))
	hash.Write([]byte{0})
	var length [8]byte
	value := strings.ToLower(name)
	binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
	hash.Write(length[:])
	hash.Write([]byte(value))
	return "response-header-" + hex.EncodeToString(hash.Sum(nil))
}

func operationResponseHeaderExampleMatches(examples []projection.Example, value string) bool {
	if len(examples) == 0 {
		return value == ""
	}
	if len(examples) != 1 {
		return false
	}
	example := examples[0]
	return example.Ordinal == 0 && example.ID == "primary" && example.Provided && utf8.ValidString(example.Text) && example.Text == value
}

type operationResponseDetailsSchemaResolver struct {
	nodes  map[projection.SchemaRef]projection.SchemaNode
	used   map[projection.SchemaRef]struct{}
	active map[projection.SchemaRef]bool
	loaded int
}

func newOperationResponseDetailsSchemaResolver(nodes []projection.SchemaNode, headerCount int) (*operationResponseDetailsSchemaResolver, error) {
	if headerCount < 0 || len(nodes) > maximumParameterSchemaNodes+headerCount {
		return nil, invalidOperationResponseDetailsField("schema-node inventory")
	}
	resolver := &operationResponseDetailsSchemaResolver{nodes: make(map[projection.SchemaRef]projection.SchemaNode, len(nodes)), used: make(map[projection.SchemaRef]struct{}, len(nodes)), active: make(map[projection.SchemaRef]bool)}
	ids := make(map[string]struct{}, len(nodes))
	var previous uint32
	for index, node := range nodes {
		ref := projection.SchemaRef(node.Ordinal)
		_, duplicateRef := resolver.nodes[ref]
		_, duplicateID := ids[node.ID]
		if (index > 0 && node.Ordinal <= previous) || duplicateRef || duplicateID || !validOperationParameterSchemaNode(node) {
			return nil, invalidOperationResponseDetailsField("schema-node inventory")
		}
		resolver.nodes[ref] = cloneSchemaNode(node)
		ids[node.ID] = struct{}{}
		previous = node.Ordinal
	}
	return resolver, nil
}

func (resolver *operationResponseDetailsSchemaResolver) schema(ref projection.SchemaRef, depth int) (domain.SchemaSummary, error) {
	node, exists := resolver.nodes[ref]
	if !exists {
		return domain.SchemaSummary{}, invalidOperationResponseDetailsField("missing schema node")
	}
	resolver.used[ref] = struct{}{}
	summary := domain.SchemaSummary{Name: node.Name, Type: node.Type, Format: node.Format, Description: node.Description, Default: node.DefaultValue, Example: node.ExampleText, Enum: append([]string(nil), node.Enum...), Constraints: responseDetailSchemaConstraints(node.Constraints), Nullable: node.Nullable, Deprecated: node.Deprecated, JSON: node.JSON}
	if depth >= maximumParameterSchemaDepth || resolver.loaded >= maximumParameterSchemaNodes || resolver.active[ref] {
		return summary, nil
	}
	resolver.active[ref] = true
	resolver.loaded++
	defer delete(resolver.active, ref)
	for _, property := range node.Properties {
		if resolver.loaded >= maximumParameterSchemaNodes {
			break
		}
		child, err := resolver.schema(property.SchemaRef, depth+1)
		if err != nil {
			return domain.SchemaSummary{}, err
		}
		description := property.Description
		if strings.TrimSpace(description) == "" {
			description = child.Description
		}
		summary.Properties = append(summary.Properties, domain.SchemaProperty{Name: property.Name, Required: property.Required, Description: description, Schema: child})
	}
	if len(node.Items) > 0 && resolver.loaded < maximumParameterSchemaNodes {
		if len(node.Items) != 1 {
			return domain.SchemaSummary{}, invalidOperationResponseDetailsField("schema-node items")
		}
		item, err := resolver.schema(node.Items[0].SchemaRef, depth+1)
		if err != nil {
			return domain.SchemaSummary{}, err
		}
		summary.Items = &item
	}
	return summary, nil
}

func responseDetailSchemaConstraints(source []projection.SchemaConstraint) []domain.SchemaConstraint {
	if source == nil {
		return nil
	}
	result := make([]domain.SchemaConstraint, 0, len(source))
	for _, constraint := range source {
		result = append(result, domain.SchemaConstraint{Name: constraint.Name, Value: constraint.Value})
	}
	return result
}

func cloneResponseDetailSchemaLinks(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for name, href := range source {
		result[name] = href
	}
	return result
}

func admitOperationResponseDetailSchemaLinks(schema domain.SchemaSummary, documentHref string, links map[string]string, admitted *map[string]string) error {
	if responseDetailSchemaIsNamedPrimitiveEnumAlias(schema) {
		name := strings.TrimSpace(schema.Name)
		href := links[name]
		if href != strings.TrimSpace(href) || (href != "" && !validResponseSchemaHref(documentHref, href)) {
			return invalidOperationResponseDetailsField("schema href")
		}
		if href != "" {
			if *admitted == nil {
				*admitted = make(map[string]string)
			}
			(*admitted)[name] = href
		}
	}
	for _, property := range schema.Properties {
		if err := admitOperationResponseDetailSchemaLinks(property.Schema, documentHref, links, admitted); err != nil {
			return err
		}
	}
	if schema.Items != nil {
		return admitOperationResponseDetailSchemaLinks(*schema.Items, documentHref, links, admitted)
	}
	return nil
}

func responseDetailSchemaIsNamedPrimitiveEnumAlias(schema domain.SchemaSummary) bool {
	return strings.TrimSpace(schema.Name) != "" && parameterSchemaIsPrimitive(schema) && len(schema.Enum) > 0
}

func cloneResponseDetailSchema(source domain.SchemaSummary) domain.SchemaSummary {
	result := source
	result.Enum = append([]string(nil), source.Enum...)
	result.Constraints = append([]domain.SchemaConstraint(nil), source.Constraints...)
	result.Properties = make([]domain.SchemaProperty, 0, len(source.Properties))
	for _, property := range source.Properties {
		property.Schema = cloneResponseDetailSchema(property.Schema)
		result.Properties = append(result.Properties, property)
	}
	if source.Items != nil {
		item := cloneResponseDetailSchema(*source.Items)
		result.Items = &item
	}
	return result
}

func OperationResponseDetails(fragment OperationResponseDetailsFragment, responseIndex int, scope string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid || responseIndex < 0 || responseIndex >= len(fragment.responses) {
			return errInvalidOperationResponseDetailsFragment
		}
		var output boundedBuffer
		if err := operationResponseDetails(fragment.responses[responseIndex], scope, cloneResponseDetailSchemaLinks(fragment.schemaLinks)).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationResponseDetailsFragment) ResponseBytes(ctx context.Context, responseIndex int, scope string) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationResponseDetails(fragment, responseIndex, scope).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationResponseDetailsField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationResponseDetailsFragment, name)
}

var _ io.Writer = (*boundedBuffer)(nil)
