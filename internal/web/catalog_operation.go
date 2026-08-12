package web

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

const (
	catalogOperationSchemaDepth = 4
	catalogOperationSchemaNodes = 256
)

type catalogOperationSchemaResolver struct {
	handler  *CatalogHandler
	ctx      context.Context
	snapshot catalog.RuntimeSnapshot
	document catalog.DocumentDirectoryV1
	active   map[projection.SchemaRef]bool
	selected map[projection.SchemaRef]projection.SchemaNode
	loaded   int
}

func (handler *CatalogHandler) catalogOperationView(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	document catalog.DocumentDirectoryV1,
	detail projection.OperationDetail,
) (*domain.Operation, []projection.SchemaNode, []projection.SchemaNode, error) {
	resolver := catalogOperationSchemaResolver{
		handler: handler, ctx: ctx, snapshot: snapshot, document: document,
		active: make(map[projection.SchemaRef]bool), selected: make(map[projection.SchemaRef]projection.SchemaNode),
	}
	operationID := string(detail.ID)
	for _, directoryOperation := range document.Operations {
		if string(directoryOperation.DetailID) == string(detail.ID) {
			if strings.TrimSpace(directoryOperation.OperationID) != "" {
				operationID = directoryOperation.OperationID
			}
			break
		}
	}
	operation := &domain.Operation{
		ID: operationID, Anchor: detail.Anchor, Title: detail.Heading, Method: detail.Method, Path: detail.Path,
		Summary: detail.Summary, Description: detail.Description, Deprecated: detail.Deprecated,
		Tags: textRecordValues(detail.Tags),
	}
	operation.Parameters = make([]domain.OperationParameter, 0, len(detail.Parameters))
	for _, parameter := range detail.Parameters {
		schema, err := resolver.schema(parameter.SchemaRef, 0)
		if err != nil {
			return nil, nil, nil, err
		}
		operation.Parameters = append(operation.Parameters, domain.OperationParameter{
			Name: parameter.Name, In: parameter.In, Required: parameter.Required,
			Description: parameter.Description, Schema: schema, Example: firstProjectionExample(parameter.Examples),
		})
	}
	parameterNodes := resolver.selectedNodes()
	var requestBodyNodes []projection.SchemaNode
	if detail.HasRequestBody {
		mediaTypes, err := resolver.mediaTypes(detail.RequestBody.MediaTypes)
		if err != nil {
			return nil, nil, nil, err
		}
		requestBodyNodes, err = resolver.selectedMediaLabelNodes(detail.RequestBody.MediaTypes, mediaTypes)
		if err != nil {
			return nil, nil, nil, err
		}
		operation.RequestBody = &domain.OperationRequestBody{
			Description: detail.RequestBody.Description,
			Required:    detail.RequestBody.Required,
			MediaTypes:  mediaTypes,
		}
	}
	operation.Responses = make([]domain.OperationResponse, 0, len(detail.Responses))
	for _, response := range detail.Responses {
		headers := make([]domain.OperationResponseHeader, 0, len(response.Headers))
		for _, header := range response.Headers {
			schema, err := resolver.schema(header.SchemaRef, 0)
			if err != nil {
				return nil, nil, nil, err
			}
			headers = append(headers, domain.OperationResponseHeader{
				Name: header.Name, Description: header.Description,
				Schema: schema, Example: firstProjectionExample(header.Examples),
			})
		}
		mediaTypes, err := resolver.mediaTypes(response.MediaTypes)
		if err != nil {
			return nil, nil, nil, err
		}
		operation.Responses = append(operation.Responses, domain.OperationResponse{
			Status: response.Status, Description: response.Description, Headers: headers, MediaTypes: mediaTypes,
		})
	}
	operation.Security = make([]domain.OperationSecurity, 0, len(detail.Security))
	for _, security := range detail.Security {
		operation.Security = append(operation.Security, domain.OperationSecurity{
			Name: security.Name, Scopes: textRecordValues(security.Scopes),
			Definition: domain.SecurityScheme{
				Name: security.Definition.Name, Type: security.Definition.Type,
				Description: security.Definition.Description, ParameterName: security.Definition.ParameterName,
				In: security.Definition.In, Scheme: security.Definition.Scheme,
				BearerFormat: security.Definition.BearerFormat, OpenIDConnectURL: security.Definition.OpenIDConnectURL,
			},
		})
	}
	operation.Snippets = make([]domain.RequestSnippet, 0, len(detail.CodeSamples))
	for _, sample := range detail.CodeSamples {
		operation.Snippets = append(operation.Snippets, domain.RequestSnippet{
			Label: sample.Label, Language: sample.Language, Code: sample.Code,
		})
	}
	if len(operation.Snippets) == 0 && strings.TrimSpace(operation.Method) != "" && strings.TrimSpace(operation.Path) != "" {
		operation.Snippets = []domain.RequestSnippet{{
			Label: "cURL", Language: "shell", Code: catalogOperationCurl(*operation),
		}}
	}
	return operation, parameterNodes, requestBodyNodes, nil
}

func (handler *CatalogHandler) catalogSchemaView(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	document catalog.DocumentDirectoryV1,
	detail projection.SchemaDetail,
) (*domain.Schema, error) {
	resolver := catalogOperationSchemaResolver{
		handler: handler, ctx: ctx, snapshot: snapshot, document: document,
		active: make(map[projection.SchemaRef]bool),
	}
	summary, err := resolver.schema(detail.SchemaRef, 0)
	if err != nil {
		return nil, err
	}
	example, provided := projectionExample(detail.Examples)
	return &domain.Schema{
		Name: detail.Heading, Description: detail.Description, Summary: summary,
		Example: domain.SchemaExample{JSON: detail.ExampleSchemaJSON, Example: example, Provided: provided},
	}, nil
}

func catalogOperationCurl(operation domain.Operation) string {
	lines := []string{fmt.Sprintf("curl --request %s \\", strings.ToUpper(operation.Method))}
	if operation.RequestBody != nil && len(operation.RequestBody.MediaTypes) > 0 {
		media := operation.RequestBody.MediaTypes[0]
		if strings.TrimSpace(media.ContentType) != "" {
			lines = append(lines, "  --header "+shellSingleQuote("content-type: "+media.ContentType)+" \\")
		}
		if strings.TrimSpace(media.Example) != "" {
			lines = append(lines, "  --data "+shellSingleQuote(media.Example)+" \\")
		}
	}
	lines = append(lines, "  --url "+shellSingleQuote(operation.Path))
	return strings.Join(lines, "\n")
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func (resolver *catalogOperationSchemaResolver) mediaTypes(source []projection.MediaType) ([]domain.OperationMediaType, error) {
	result := make([]domain.OperationMediaType, 0, len(source))
	for _, media := range source {
		schema, err := resolver.schema(media.SchemaRef, 0)
		if err != nil {
			return nil, err
		}
		example, provided := projectionExample(media.Examples)
		result = append(result, domain.OperationMediaType{
			ContentType: media.ContentType, Schema: schema,
			Example: example, ExampleProvided: provided,
		})
	}
	return result, nil
}

func (resolver *catalogOperationSchemaResolver) schema(ref projection.SchemaRef, depth int) (domain.SchemaSummary, error) {
	node, _, err := resolver.handler.loadCatalogSchemaNode(resolver.ctx, resolver.snapshot, resolver.document, uint32(ref))
	if err != nil {
		return domain.SchemaSummary{}, err
	}
	if resolver.selected != nil {
		resolver.selected[ref] = cloneProjectionSchemaNode(node)
	}
	summary := domain.SchemaSummary{
		Name: node.Name, Type: node.Type, Format: node.Format, Description: node.Description,
		Default: node.DefaultValue, Example: node.ExampleText,
		Enum: append([]string(nil), node.Enum...), Constraints: domainSchemaConstraints(node.Constraints),
		Nullable: node.Nullable, Deprecated: node.Deprecated, JSON: node.JSON,
	}
	if depth >= catalogOperationSchemaDepth || resolver.loaded >= catalogOperationSchemaNodes || resolver.active[ref] {
		return summary, nil
	}
	resolver.active[ref] = true
	resolver.loaded++
	defer delete(resolver.active, ref)
	for _, property := range node.Properties {
		if resolver.loaded >= catalogOperationSchemaNodes {
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
		summary.Properties = append(summary.Properties, domain.SchemaProperty{
			Name: property.Name, Required: property.Required, Description: description, Schema: child,
		})
	}
	if len(node.Items) > 0 && resolver.loaded < catalogOperationSchemaNodes {
		items, err := resolver.schema(node.Items[0].SchemaRef, depth+1)
		if err != nil {
			return domain.SchemaSummary{}, err
		}
		summary.Items = &items
	}
	return summary, nil
}

func (resolver *catalogOperationSchemaResolver) selectedNodes() []projection.SchemaNode {
	result := make([]projection.SchemaNode, 0, len(resolver.selected))
	for _, node := range resolver.selected {
		result = append(result, cloneProjectionSchemaNode(node))
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Ordinal < result[right].Ordinal })
	return result
}

func (resolver *catalogOperationSchemaResolver) selectedMediaLabelNodes(mediaTypes []projection.MediaType, prepared []domain.OperationMediaType) ([]projection.SchemaNode, error) {
	if len(mediaTypes) != len(prepared) {
		return nil, fmt.Errorf("request-body media inventory changed while selecting schema nodes")
	}
	selected := make(map[projection.SchemaRef]projection.SchemaNode, len(mediaTypes))
	for index, media := range mediaTypes {
		if err := resolver.selectMediaLabelNodes(selected, media.SchemaRef, prepared[index].Schema, 0); err != nil {
			return nil, err
		}
	}
	result := make([]projection.SchemaNode, 0, len(selected))
	for _, node := range selected {
		result = append(result, node)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Ordinal < result[right].Ordinal })
	return result, nil
}

func (resolver *catalogOperationSchemaResolver) selectMediaLabelNodes(selected map[projection.SchemaRef]projection.SchemaNode, ref projection.SchemaRef, schema domain.SchemaSummary, depth int) error {
	node, exists := resolver.selected[ref]
	if !exists {
		return fmt.Errorf("request-body media schema node %d was not selected", ref)
	}
	selected[ref] = cloneProjectionSchemaNode(node)
	if schema.Items == nil {
		return nil
	}
	if depth >= catalogOperationSchemaDepth || len(node.Items) != 1 {
		return fmt.Errorf("request-body media schema node %d has inconsistent items", ref)
	}
	return resolver.selectMediaLabelNodes(selected, node.Items[0].SchemaRef, *schema.Items, depth+1)
}

func cloneProjectionSchemaNode(node projection.SchemaNode) projection.SchemaNode {
	clone := node
	clone.Enum = append([]string(nil), node.Enum...)
	clone.Constraints = append([]projection.SchemaConstraint(nil), node.Constraints...)
	clone.Properties = append([]projection.SchemaNodeProperty(nil), node.Properties...)
	clone.Items = append([]projection.SchemaNodeItem(nil), node.Items...)
	return clone
}

func domainSchemaConstraints(constraints []projection.SchemaConstraint) []domain.SchemaConstraint {
	result := make([]domain.SchemaConstraint, 0, len(constraints))
	for _, constraint := range constraints {
		result = append(result, domain.SchemaConstraint{Name: constraint.Name, Value: constraint.Value})
	}
	return result
}

func textRecordValues(records []projection.TextRecord) []string {
	values := make([]string, 0, len(records))
	for _, record := range records {
		values = append(values, record.Value)
	}
	return values
}

func firstProjectionExample(examples []projection.Example) string {
	value, _ := projectionExample(examples)
	return value
}

func projectionExample(examples []projection.Example) (string, bool) {
	if len(examples) == 0 {
		return "", false
	}
	return examples[0].Text, examples[0].Provided
}
