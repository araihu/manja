package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationSchemaTreesFragment = errors.New("local docs operation schema-trees fragment is invalid")

// OperationSchemaTreesFragment holds admitted request/response schema-property trees.
// All values are copied from verified projection records and prepared operation data.
type OperationSchemaTreesFragment struct {
	request   []operationSchemaTreeData
	responses [][]operationSchemaTreeData
	binding   operationPreparationBinding
	valid     bool
}

type operationSchemaTreeData struct {
	ID         string
	Caption    string
	HasContent bool
	Root       operationSchemaTreeNodeData
	LinkTarget string
	LinkSelect string
	LinkSwap   string
}

type operationSchemaTreeNodeData struct {
	Name          string
	SchemaName    string
	Type          string
	Format        string
	Inline        string
	InlineType    string
	Description   string
	DefaultValue  string
	ExampleText   string
	EnumValues    []string
	Constraints   []schemaConstraintData
	Nullable      bool
	Deprecated    bool
	Properties    []operationSchemaTreePropertyData
	Items         *operationSchemaTreeNodeData
	Expandable    bool
	EnumAlias     bool
	ReferenceHref string
	Limited       bool
}

type operationSchemaTreePropertyData struct {
	Name     string
	Required bool
	Schema   operationSchemaTreeNodeData
}

// PrepareOperationSchemaTrees validates canonical nodes for every schema visited
// by the operation projection, including parameters and response headers.
func PrepareOperationSchemaTrees(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	nodes []projection.SchemaNode,
	documentHref string,
	schemaLinks map[string]string,
) (OperationSchemaTreesFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) || !validDocumentHref(documentHref) {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("operation")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || operation.Anchor != projected.Anchor {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("operation identity")
	}
	// Root schemas are still visited after the expansion budget is exhausted.
	rootCount := len(projected.Parameters) + len(projected.RequestBody.MediaTypes)
	for _, response := range projected.Responses {
		rootCount += len(response.Headers) + len(response.MediaTypes)
	}
	// Nodes retained at the depth boundary do not consume expansion budget.
	// Count them from the prepared summaries, stopping once the candidate
	// inventory is covered. Replay below still validates every edge and rejects
	// duplicate, unvisited, or inconsistent nodes.
	retainedCount := rootCount + operationSchemaTreeBoundaryNodes(operation, len(nodes))
	resolver, err := newOperationSchemaTreeResolver(nodes, documentHref, schemaLinks, retainedCount)
	if err != nil {
		return OperationSchemaTreesFragment{}, err
	}
	// Replay schema visits in projection order. The node budget is shared by
	// parameters, request bodies, response headers, and response bodies.
	if len(projected.Parameters) != len(operation.Parameters) {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("parameter inventory")
	}
	for index, parameter := range projected.Parameters {
		if _, err := resolver.prepare(parameter.SchemaRef, operation.Parameters[index].Schema, 0); err != nil {
			return OperationSchemaTreesFragment{}, err
		}
	}
	fragment := OperationSchemaTreesFragment{
		responses: make([][]operationSchemaTreeData, len(projected.Responses)),
		valid:     true,
	}
	if projected.HasRequestBody != (operation.RequestBody != nil) {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("request body")
	}
	if projected.HasRequestBody {
		if projected.RequestBody.Description != operation.RequestBody.Description || projected.RequestBody.Required != operation.RequestBody.Required || len(projected.RequestBody.MediaTypes) != len(operation.RequestBody.MediaTypes) {
			return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("request body")
		}
		fragment.request = make([]operationSchemaTreeData, 0, len(projected.RequestBody.MediaTypes))
		mediaIDs := make(map[string]struct{}, len(projected.RequestBody.MediaTypes))
		for index, media := range projected.RequestBody.MediaTypes {
			prepared := operation.RequestBody.MediaTypes[index]
			_, duplicate := mediaIDs[media.ID]
			if duplicate || !validOperationSchemaMedia(index, media, prepared) {
				return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("request media identity")
			}
			mediaIDs[media.ID] = struct{}{}
			root, err := resolver.prepare(media.SchemaRef, prepared.Schema, 0)
			if err != nil {
				return OperationSchemaTreesFragment{}, err
			}
			fragment.request = append(fragment.request, operationSchemaTreeData{
				ID: operationSchemaTreeMediaID(operation.Anchor+"-request-body", media.ContentType), Caption: "Request body schema for " + media.ContentType,
				HasContent: operationSchemaTreeHasContent(root), Root: root,
				LinkTarget: "#catalog-main-content", LinkSelect: "#catalog-main-content", LinkSwap: "outerHTML show:top showTarget:#main-content",
			})
		}
	}
	if len(projected.Responses) != len(operation.Responses) {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("response inventory")
	}
	responseIDs := make(map[string]struct{}, len(projected.Responses))
	for responseIndex, response := range projected.Responses {
		preparedResponse := operation.Responses[responseIndex]
		_, duplicate := responseIDs[response.ID]
		if response.Ordinal != uint32(responseIndex) || response.ID != response.Status || duplicate || domain.ValidateCanonicalIdentity("response status", response.Status, false) != nil || response.Status != preparedResponse.Status || len(response.MediaTypes) != len(preparedResponse.MediaTypes) {
			return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("response identity")
		}
		if len(response.Headers) != len(preparedResponse.Headers) {
			return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("response header inventory")
		}
		for index, header := range response.Headers {
			if _, err := resolver.prepare(header.SchemaRef, preparedResponse.Headers[index].Schema, 0); err != nil {
				return OperationSchemaTreesFragment{}, err
			}
		}
		responseIDs[response.ID] = struct{}{}
		fragment.responses[responseIndex] = make([]operationSchemaTreeData, 0, len(response.MediaTypes))
		mediaIDs := make(map[string]struct{}, len(response.MediaTypes))
		for mediaIndex, media := range response.MediaTypes {
			prepared := preparedResponse.MediaTypes[mediaIndex]
			_, duplicate := mediaIDs[media.ID]
			if duplicate || !validOperationSchemaMedia(mediaIndex, media, prepared) {
				return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("response media identity")
			}
			mediaIDs[media.ID] = struct{}{}
			root, err := resolver.prepare(media.SchemaRef, prepared.Schema, 0)
			if err != nil {
				return OperationSchemaTreesFragment{}, err
			}
			idPrefix := operation.Anchor + "-response-" + anchorFragment(response.Status)
			fragment.responses[responseIndex] = append(fragment.responses[responseIndex], operationSchemaTreeData{
				ID: operationSchemaTreeMediaID(idPrefix, media.ContentType), Caption: "Response body",
				HasContent: operationSchemaTreeHasContent(root), Root: root,
				LinkTarget: "#catalog-main-content", LinkSelect: "#catalog-main-content", LinkSwap: "outerHTML show:top showTarget:#main-content",
			})
		}
	}
	if len(resolver.used) != len(resolver.nodes) {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("schema-node inventory")
	}
	for _, tree := range fragment.request {
		if err := validateOperationSchemaTreeOutput(tree); err != nil {
			return OperationSchemaTreesFragment{}, err
		}
	}
	for _, response := range fragment.responses {
		for _, tree := range response {
			if err := validateOperationSchemaTreeOutput(tree); err != nil {
				return OperationSchemaTreesFragment{}, err
			}
		}
	}
	fragment.binding, err = bindOperationPreparation(detail, operation, documentHref, schemaLinks)
	if err != nil {
		return OperationSchemaTreesFragment{}, invalidOperationSchemaTreesField("preparation context")
	}
	return fragment, nil
}

func validateOperationSchemaTreeOutput(tree operationSchemaTreeData) error {
	var output boundedBuffer
	if err := operationSchemaProperties(tree).Render(context.Background(), &output); err != nil {
		return invalidOperationSchemaTreesField("rendered bytes")
	}
	return nil
}

func validOperationSchemaMedia(index int, projected projection.MediaType, prepared domain.OperationMediaType) bool {
	return projected.Ordinal == uint32(index) && projected.ID == projected.ContentType &&
		domain.ValidateCanonicalIdentity("media content type", projected.ContentType, false) == nil && projected.ContentType == prepared.ContentType
}

type operationSchemaTreeResolver struct {
	nodes        map[projection.SchemaRef]projection.SchemaNode
	used         map[projection.SchemaRef]struct{}
	active       map[projection.SchemaRef]bool
	loaded       int
	documentHref string
	schemaLinks  map[string]string
}

func operationSchemaTreeBoundaryNodes(operation domain.Operation, limit int) int {
	count := 0
	var visit func(domain.SchemaSummary, int)
	visit = func(schema domain.SchemaSummary, depth int) {
		if count >= limit {
			return
		}
		if depth == maximumParameterSchemaDepth {
			count++
			return
		}
		for _, property := range schema.Properties {
			visit(property.Schema, depth+1)
		}
		if schema.Items != nil {
			visit(*schema.Items, depth+1)
		}
	}
	for _, parameter := range operation.Parameters {
		visit(parameter.Schema, 0)
	}
	if operation.RequestBody != nil {
		for _, media := range operation.RequestBody.MediaTypes {
			visit(media.Schema, 0)
		}
	}
	for _, response := range operation.Responses {
		for _, header := range response.Headers {
			visit(header.Schema, 0)
		}
		for _, media := range response.MediaTypes {
			visit(media.Schema, 0)
		}
	}
	return count
}

func newOperationSchemaTreeResolver(nodes []projection.SchemaNode, documentHref string, schemaLinks map[string]string, retainedCount int) (*operationSchemaTreeResolver, error) {
	if retainedCount < 0 || len(nodes) > maximumParameterSchemaNodes+retainedCount {
		return nil, invalidOperationSchemaTreesField("schema-node inventory")
	}
	resolver := &operationSchemaTreeResolver{
		nodes: make(map[projection.SchemaRef]projection.SchemaNode, len(nodes)), used: make(map[projection.SchemaRef]struct{}, len(nodes)),
		active: make(map[projection.SchemaRef]bool), documentHref: documentHref, schemaLinks: make(map[string]string, len(schemaLinks)),
	}
	ids := make(map[string]struct{}, len(nodes))
	var previous uint32
	for index, node := range nodes {
		ref := projection.SchemaRef(node.Ordinal)
		_, duplicateRef := resolver.nodes[ref]
		_, duplicateID := ids[node.ID]
		if (index > 0 && node.Ordinal <= previous) || duplicateRef || duplicateID || domain.ValidateCanonicalIdentity("schema node id", node.ID, false) != nil || !validOperationParameterSchemaNode(node) {
			return nil, invalidOperationSchemaTreesField("schema-node inventory")
		}
		resolver.nodes[ref] = cloneSchemaNode(node)
		ids[node.ID] = struct{}{}
		previous = node.Ordinal
	}
	for name, href := range schemaLinks {
		resolver.schemaLinks[name] = href
	}
	return resolver, nil
}

func (resolver *operationSchemaTreeResolver) prepare(ref projection.SchemaRef, schema domain.SchemaSummary, depth int) (operationSchemaTreeNodeData, error) {
	node, exists := resolver.nodes[ref]
	if !exists {
		return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("missing schema node")
	}
	resolver.used[ref] = struct{}{}
	if !operationSchemaSummaryMatchesNode(schema, node) {
		return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("schema summary")
	}
	data := operationSchemaTreeNodeData{
		SchemaName: node.Name, Type: node.Type, Format: node.Format,
		Description: node.Description, DefaultValue: node.DefaultValue, ExampleText: node.ExampleText,
		EnumValues: append([]string(nil), node.Enum...), Constraints: schemaConstraints(node.Constraints),
		Nullable: node.Nullable, Deprecated: node.Deprecated,
	}
	data.EnumAlias = operationSchemaTreeIsNamedPrimitiveEnumAlias(data)
	if operationSchemaTreeIsLinkable(data) {
		data.ReferenceHref = resolver.schemaLinks[strings.TrimSpace(schema.Name)]
		if data.ReferenceHref != strings.TrimSpace(data.ReferenceHref) || (data.ReferenceHref != "" && !validRequestBodySchemaHref(resolver.documentHref, data.ReferenceHref)) {
			return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("schema href")
		}
	}
	if depth >= maximumParameterSchemaDepth || resolver.active[ref] || resolver.loaded >= maximumParameterSchemaNodes {
		if len(schema.Properties) != 0 || schema.Items != nil {
			return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("schema recursion")
		}
		data.Limited = len(node.Properties) > 0 || len(node.Items) > 0
		finishOperationSchemaTreeNode(&data)
		return data, nil
	}
	resolver.loaded++
	resolver.active[ref] = true
	defer delete(resolver.active, ref)
	data.Properties = make([]operationSchemaTreePropertyData, 0, len(node.Properties))
	for index, property := range node.Properties {
		if resolver.loaded >= maximumParameterSchemaNodes {
			break
		}
		if index >= len(schema.Properties) {
			return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("schema edges")
		}
		preparedProperty := schema.Properties[index]
		child, err := resolver.prepare(property.SchemaRef, preparedProperty.Schema, depth+1)
		if err != nil {
			return operationSchemaTreeNodeData{}, err
		}
		expectedDescription := property.Description
		if strings.TrimSpace(expectedDescription) == "" {
			expectedDescription = child.Description
		}
		if property.Name != preparedProperty.Name || property.Required != preparedProperty.Required || expectedDescription != preparedProperty.Description {
			return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("schema property")
		}
		data.Limited = data.Limited || child.Limited
		child.Name = operationSchemaDisplayName(property.Name, child.SchemaName)
		data.Properties = append(data.Properties, operationSchemaTreePropertyData{Name: child.Name, Required: property.Required, Schema: child})
	}
	if len(data.Properties) != len(schema.Properties) || ((len(node.Items) == 1 && resolver.loaded < maximumParameterSchemaNodes) != (schema.Items != nil)) {
		return operationSchemaTreeNodeData{}, invalidOperationSchemaTreesField("schema edges")
	}
	if schema.Items != nil {
		child, err := resolver.prepare(node.Items[0].SchemaRef, *schema.Items, depth+1)
		if err != nil {
			return operationSchemaTreeNodeData{}, err
		}
		data.Limited = data.Limited || child.Limited
		child.Name = operationSchemaDisplayName("items", child.SchemaName)
		data.Items = &child
	}
	data.Expandable = len(data.Properties) > 0 || data.Items != nil && data.Items.Expandable
	data.Limited = data.Limited || len(data.Properties) < len(node.Properties) || (len(node.Items) > 0 && data.Items == nil)
	finishOperationSchemaTreeNode(&data)
	return data, nil
}

func operationSchemaSummaryMatchesNode(schema domain.SchemaSummary, node projection.SchemaNode) bool {
	if schema.Name != node.Name || schema.Type != node.Type || schema.Format != node.Format || schema.Description != node.Description ||
		schema.Default != node.DefaultValue || schema.Example != node.ExampleText || schema.Nullable != node.Nullable || schema.Deprecated != node.Deprecated || schema.JSON != node.JSON ||
		!slices.Equal(schema.Enum, node.Enum) || len(schema.Constraints) != len(node.Constraints) {
		return false
	}
	for index, constraint := range node.Constraints {
		if schema.Constraints[index].Name != constraint.Name || schema.Constraints[index].Value != constraint.Value {
			return false
		}
	}
	return true
}

func operationSchemaTreeMediaID(idPrefix, contentType string) string {
	fragment := anchorFragment(contentType)
	if fragment == "" {
		fragment = "media"
	}
	return idPrefix + "-" + fragment + "-schema"
}

func operationSchemaDisplayName(name, schemaName string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if strings.TrimSpace(schemaName) != "" {
		return schemaName
	}
	return "schema"
}

func operationSchemaTreeHasContent(schema operationSchemaTreeNodeData) bool {
	return schema.Limited || schema.Inline != "" || strings.TrimSpace(schema.Description) != "" || strings.TrimSpace(schema.ExampleText) != "" || len(schema.Properties) > 0 || schema.Items != nil
}

func finishOperationSchemaTreeNode(node *operationSchemaTreeNodeData) {
	node.InlineType = operationSchemaTreeInlineType(*node)
	node.Inline = operationSchemaTreeInline(*node)
}

func operationSchemaTreeInline(schema operationSchemaTreeNodeData) string {
	typeLabel := operationSchemaTreeInlineType(schema)
	if operationSchemaTreeIsNamedPrimitiveEnumAlias(schema) {
		return typeLabel + "<" + strings.TrimSpace(schema.SchemaName) + ">"
	}
	parts := make([]string, 0, 2)
	if schema.SchemaName != "" {
		parts = append(parts, schema.SchemaName)
	}
	if typeLabel != "" {
		parts = append(parts, typeLabel)
	}
	return strings.Join(parts, " ")
}

func operationSchemaTreeInlineType(schema operationSchemaTreeNodeData) string {
	typeLabel := schema.Type
	if strings.Contains(schema.Type, "array") {
		typeLabel = "array"
		if schema.Items != nil {
			if item := operationSchemaTreeInline(*schema.Items); item != "" {
				typeLabel += "[" + item + "]"
			}
		}
	} else if schema.Format != "" {
		if typeLabel != "" {
			typeLabel += "<" + schema.Format + ">"
		} else {
			typeLabel = schema.Format
		}
	}
	return typeLabel
}

func operationSchemaTreeIsPrimitive(schema operationSchemaTreeNodeData) bool {
	switch strings.ToLower(strings.TrimSpace(schema.Type)) {
	case "string", "number", "integer", "boolean", "null":
		return true
	}
	return false
}

func operationSchemaTreeIsNamedPrimitiveEnumAlias(schema operationSchemaTreeNodeData) bool {
	return strings.TrimSpace(schema.SchemaName) != "" && operationSchemaTreeIsPrimitive(schema) && len(schema.EnumValues) > 0
}

func operationSchemaTreeIsLinkable(schema operationSchemaTreeNodeData) bool {
	return strings.TrimSpace(schema.SchemaName) != "" && (!operationSchemaTreeIsPrimitive(schema) || len(schema.EnumValues) > 0)
}

func OperationRequestBodySchemaTree(fragment OperationSchemaTreesFragment, index int) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid || index < 0 || index >= len(fragment.request) {
			return errInvalidOperationSchemaTreesFragment
		}
		var output boundedBuffer
		if err := operationSchemaProperties(fragment.request[index]).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func OperationResponseSchemaTree(fragment OperationSchemaTreesFragment, responseIndex, mediaIndex int) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid || responseIndex < 0 || responseIndex >= len(fragment.responses) || mediaIndex < 0 || mediaIndex >= len(fragment.responses[responseIndex]) {
			return errInvalidOperationSchemaTreesFragment
		}
		var output boundedBuffer
		if err := operationSchemaProperties(fragment.responses[responseIndex][mediaIndex]).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationSchemaTreesFragment) RequestBodyBytes(ctx context.Context, index int) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationRequestBodySchemaTree(fragment, index).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func (fragment OperationSchemaTreesFragment) ResponseBytes(ctx context.Context, responseIndex, mediaIndex int) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationResponseSchemaTree(fragment, responseIndex, mediaIndex).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationSchemaTreesField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationSchemaTreesFragment, name)
}
