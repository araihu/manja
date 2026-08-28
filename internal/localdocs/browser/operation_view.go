package browser

import (
	"fmt"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

// Keep the static operation projection bounded to the same shape used by the
// server-side catalog renderer. A large document must not be able to turn one
// route change into an unbounded recursive schema walk.
const (
	browserOperationSchemaDepth = 4
	browserOperationSchemaNodes = 256
)

type browserOperationSchemaResolver struct {
	browser  *Browser
	document catalog.DocumentDirectoryV1
	active   map[projection.SchemaRef]bool
	selected map[projection.SchemaRef]projection.SchemaNode
	loaded   int
}

// browserOperationView prepares the complete operation model and the exact
// schema-node subsets consumed by the local render fragments. It deliberately
// mirrors the dynamic catalog view: the static path must expose parameters,
// auth, request bodies, responses, examples and schema trees rather than only
// the operation heading.
func (browser *Browser) browserOperationView(
	document catalog.DocumentDirectoryV1,
	detail projection.OperationDetail,
) (*domain.Operation, []projection.SchemaNode, []projection.SchemaNode, []projection.SchemaNode, []projection.SchemaNode, []projection.SchemaNode, error) {
	resolver := browserOperationSchemaResolver{
		browser: browser, document: document,
		active:   make(map[projection.SchemaRef]bool),
		selected: make(map[projection.SchemaRef]projection.SchemaNode),
	}
	operationID := string(detail.ID)
	for _, directoryOperation := range document.Operations {
		if directoryOperation.DetailID == domain.DetailID(detail.ID) {
			if strings.TrimSpace(directoryOperation.OperationID) != "" {
				operationID = directoryOperation.OperationID
			}
			break
		}
	}
	operation := &domain.Operation{
		ID: operationID, Anchor: detail.Anchor, Title: detail.Heading, Method: detail.Method, Path: detail.Path,
		Summary: detail.Summary, Description: detail.Description, Deprecated: detail.Deprecated,
		Tags: browserTextValues(detail.Tags),
	}
	operation.Parameters = make([]domain.OperationParameter, 0, len(detail.Parameters))
	for _, parameter := range detail.Parameters {
		schema, err := resolver.schema(parameter.SchemaRef, 0)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		operation.Parameters = append(operation.Parameters, domain.OperationParameter{
			Name: parameter.Name, In: parameter.In, Required: parameter.Required,
			Description: parameter.Description, Schema: schema, Example: browserFirstProjectionExample(parameter.Examples),
		})
	}
	parameterNodes := resolver.selectedNodes()
	var requestBodyNodes []projection.SchemaNode
	if detail.HasRequestBody {
		mediaTypes, err := resolver.mediaTypes(detail.RequestBody.MediaTypes)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		requestBodyNodes, err = resolver.selectedMediaLabelNodes(detail.RequestBody.MediaTypes, mediaTypes)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		operation.RequestBody = &domain.OperationRequestBody{
			Description: detail.RequestBody.Description,
			Required:    detail.RequestBody.Required,
			MediaTypes:  mediaTypes,
		}
	}
	operation.Responses = make([]domain.OperationResponse, 0, len(detail.Responses))
	responseMediaTypes := make([]projection.MediaType, 0)
	preparedResponseMediaTypes := make([]domain.OperationMediaType, 0)
	for _, response := range detail.Responses {
		headers := make([]domain.OperationResponseHeader, 0, len(response.Headers))
		for _, header := range response.Headers {
			schema, err := resolver.schema(header.SchemaRef, 0)
			if err != nil {
				return nil, nil, nil, nil, nil, nil, err
			}
			headers = append(headers, domain.OperationResponseHeader{
				Name: header.Name, Description: header.Description,
				Schema: schema, Example: browserFirstProjectionExample(header.Examples),
			})
		}
		mediaTypes, err := resolver.mediaTypes(response.MediaTypes)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		operation.Responses = append(operation.Responses, domain.OperationResponse{
			Status: response.Status, Description: response.Description, Headers: headers, MediaTypes: mediaTypes,
		})
		responseMediaTypes = append(responseMediaTypes, response.MediaTypes...)
		preparedResponseMediaTypes = append(preparedResponseMediaTypes, mediaTypes...)
	}
	responseMediaNodes, err := resolver.selectedMediaLabelNodes(responseMediaTypes, preparedResponseMediaTypes)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	responseDetailNodes, err := resolver.selectedResponseDetailNodes(detail.Responses, operation.Responses)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	schemaTreeNodes, err := resolver.selectedOperationSchemaTreeNodes(detail, *operation)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	operation.Security = make([]domain.OperationSecurity, 0, len(detail.Security))
	for _, security := range detail.Security {
		operation.Security = append(operation.Security, domain.OperationSecurity{
			Name: security.Name, Scopes: browserTextValues(security.Scopes),
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
		operation.Snippets = append(operation.Snippets, domain.RequestSnippet{Label: sample.Label, Language: sample.Language, Code: sample.Code})
	}
	if len(operation.Snippets) == 0 && strings.TrimSpace(operation.Method) != "" && strings.TrimSpace(operation.Path) != "" {
		operation.Snippets = []domain.RequestSnippet{{
			Label: "cURL", Language: "shell", Code: browserOperationCurl(*operation),
		}}
	}
	return operation, parameterNodes, requestBodyNodes, responseMediaNodes, responseDetailNodes, schemaTreeNodes, nil
}

func (resolver *browserOperationSchemaResolver) mediaTypes(source []projection.MediaType) ([]domain.OperationMediaType, error) {
	result := make([]domain.OperationMediaType, 0, len(source))
	for _, media := range source {
		schema, err := resolver.schema(media.SchemaRef, 0)
		if err != nil {
			return nil, err
		}
		example, provided := browserProjectionExample(media.Examples)
		result = append(result, domain.OperationMediaType{ContentType: media.ContentType, Schema: schema, Example: example, ExampleProvided: provided})
	}
	return result, nil
}

func (resolver *browserOperationSchemaResolver) schema(ref projection.SchemaRef, depth int) (domain.SchemaSummary, error) {
	node, err := resolver.browser.schemaNode(resolver.document, uint32(ref))
	if err != nil {
		return domain.SchemaSummary{}, err
	}
	if resolver.selected != nil {
		resolver.selected[ref] = browserCloneProjectionSchemaNode(node)
	}
	summary := domain.SchemaSummary{
		Name: node.Name, Type: node.Type, Format: node.Format, Description: node.Description,
		Default: node.DefaultValue, Example: node.ExampleText,
		Enum: append([]string(nil), node.Enum...), Constraints: browserDomainSchemaConstraints(node.Constraints),
		Nullable: node.Nullable, Deprecated: node.Deprecated, JSON: node.JSON,
	}
	if depth >= browserOperationSchemaDepth || resolver.loaded >= browserOperationSchemaNodes || resolver.active[ref] {
		return summary, nil
	}
	resolver.active[ref] = true
	resolver.loaded++
	defer delete(resolver.active, ref)
	for _, property := range node.Properties {
		if resolver.loaded >= browserOperationSchemaNodes {
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
	if len(node.Items) > 0 && resolver.loaded < browserOperationSchemaNodes {
		items, err := resolver.schema(node.Items[0].SchemaRef, depth+1)
		if err != nil {
			return domain.SchemaSummary{}, err
		}
		summary.Items = &items
	}
	return summary, nil
}

func (resolver *browserOperationSchemaResolver) selectedNodes() []projection.SchemaNode {
	result := make([]projection.SchemaNode, 0, len(resolver.selected))
	for _, node := range resolver.selected {
		result = append(result, browserCloneProjectionSchemaNode(node))
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Ordinal < result[right].Ordinal })
	return result
}

func (resolver *browserOperationSchemaResolver) selectedMediaLabelNodes(mediaTypes []projection.MediaType, prepared []domain.OperationMediaType) ([]projection.SchemaNode, error) {
	if len(mediaTypes) != len(prepared) {
		return nil, fmt.Errorf("media inventory changed while selecting schema nodes")
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

func (resolver *browserOperationSchemaResolver) selectedResponseDetailNodes(responses []projection.Response, prepared []domain.OperationResponse) ([]projection.SchemaNode, error) {
	if len(responses) != len(prepared) {
		return nil, fmt.Errorf("response inventory changed while selecting schema nodes")
	}
	selected := make(map[projection.SchemaRef]projection.SchemaNode)
	for responseIndex, response := range responses {
		if len(response.Headers) != len(prepared[responseIndex].Headers) {
			return nil, fmt.Errorf("response header inventory changed while selecting schema nodes")
		}
		for headerIndex, header := range response.Headers {
			if err := resolver.selectResponseDetailNodes(selected, header.SchemaRef, prepared[responseIndex].Headers[headerIndex].Schema, 0); err != nil {
				return nil, err
			}
		}
	}
	result := make([]projection.SchemaNode, 0, len(selected))
	for _, node := range selected {
		result = append(result, node)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Ordinal < result[right].Ordinal })
	return result, nil
}

func (resolver *browserOperationSchemaResolver) selectedOperationSchemaTreeNodes(detail projection.OperationDetail, operation domain.Operation) ([]projection.SchemaNode, error) {
	selected := make(map[projection.SchemaRef]projection.SchemaNode)
	if detail.HasRequestBody {
		if operation.RequestBody == nil || len(detail.RequestBody.MediaTypes) != len(operation.RequestBody.MediaTypes) {
			return nil, fmt.Errorf("request schema-tree inventory changed")
		}
		for index, media := range detail.RequestBody.MediaTypes {
			if err := resolver.selectOperationSchemaTreeNodes(selected, make(map[projection.SchemaRef]bool), media.SchemaRef, operation.RequestBody.MediaTypes[index].Schema, 0); err != nil {
				return nil, err
			}
		}
	} else if operation.RequestBody != nil {
		return nil, fmt.Errorf("request schema-tree inventory changed")
	}
	if len(detail.Responses) != len(operation.Responses) {
		return nil, fmt.Errorf("response schema-tree inventory changed")
	}
	for responseIndex, response := range detail.Responses {
		if len(response.MediaTypes) != len(operation.Responses[responseIndex].MediaTypes) {
			return nil, fmt.Errorf("response schema-tree media inventory changed")
		}
		for mediaIndex, media := range response.MediaTypes {
			if err := resolver.selectOperationSchemaTreeNodes(selected, make(map[projection.SchemaRef]bool), media.SchemaRef, operation.Responses[responseIndex].MediaTypes[mediaIndex].Schema, 0); err != nil {
				return nil, err
			}
		}
	}
	result := make([]projection.SchemaNode, 0, len(selected))
	for _, node := range selected {
		result = append(result, browserCloneProjectionSchemaNode(node))
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Ordinal < result[right].Ordinal })
	return result, nil
}

func (resolver *browserOperationSchemaResolver) selectOperationSchemaTreeNodes(selected map[projection.SchemaRef]projection.SchemaNode, active map[projection.SchemaRef]bool, ref projection.SchemaRef, schema domain.SchemaSummary, depth int) error {
	node, exists := resolver.selected[ref]
	if !exists {
		return fmt.Errorf("operation schema-tree node %d was not selected", ref)
	}
	selected[ref] = browserCloneProjectionSchemaNode(node)
	if depth >= browserOperationSchemaDepth || active[ref] {
		return nil
	}
	if len(node.Properties) != len(schema.Properties) || (len(node.Items) == 1) != (schema.Items != nil) {
		return fmt.Errorf("operation schema-tree node %d has inconsistent edges", ref)
	}
	active[ref] = true
	defer delete(active, ref)
	for index, property := range node.Properties {
		if err := resolver.selectOperationSchemaTreeNodes(selected, active, property.SchemaRef, schema.Properties[index].Schema, depth+1); err != nil {
			return err
		}
	}
	if schema.Items != nil {
		return resolver.selectOperationSchemaTreeNodes(selected, active, node.Items[0].SchemaRef, *schema.Items, depth+1)
	}
	return nil
}

func (resolver *browserOperationSchemaResolver) selectResponseDetailNodes(selected map[projection.SchemaRef]projection.SchemaNode, ref projection.SchemaRef, schema domain.SchemaSummary, depth int) error {
	node, exists := resolver.selected[ref]
	if !exists {
		return fmt.Errorf("response header schema node %d was not selected", ref)
	}
	selected[ref] = browserCloneProjectionSchemaNode(node)
	if depth >= browserOperationSchemaDepth {
		return nil
	}
	if (len(node.Items) == 1) != (schema.Items != nil) {
		return fmt.Errorf("response header schema node %d has inconsistent items", ref)
	}
	if schema.Items != nil {
		if err := resolver.selectResponseDetailNodes(selected, node.Items[0].SchemaRef, *schema.Items, depth+1); err != nil {
			return err
		}
	}
	if len(node.Properties) != len(schema.Properties) {
		return fmt.Errorf("response header schema node %d has inconsistent properties", ref)
	}
	for index, property := range node.Properties {
		if err := resolver.selectResponseDetailNodes(selected, property.SchemaRef, schema.Properties[index].Schema, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (resolver *browserOperationSchemaResolver) selectMediaLabelNodes(selected map[projection.SchemaRef]projection.SchemaNode, ref projection.SchemaRef, schema domain.SchemaSummary, depth int) error {
	node, exists := resolver.selected[ref]
	if !exists {
		return fmt.Errorf("media schema node %d was not selected", ref)
	}
	selected[ref] = browserCloneProjectionSchemaNode(node)
	if schema.Items == nil {
		return nil
	}
	if depth >= browserOperationSchemaDepth || len(node.Items) != 1 {
		return fmt.Errorf("media schema node %d has inconsistent items", ref)
	}
	return resolver.selectMediaLabelNodes(selected, node.Items[0].SchemaRef, *schema.Items, depth+1)
}

func browserCloneProjectionSchemaNode(node projection.SchemaNode) projection.SchemaNode {
	clone := node
	clone.Enum = append([]string(nil), node.Enum...)
	clone.Constraints = append([]projection.SchemaConstraint(nil), node.Constraints...)
	clone.Properties = append([]projection.SchemaNodeProperty(nil), node.Properties...)
	clone.Items = append([]projection.SchemaNodeItem(nil), node.Items...)
	return clone
}

func browserDomainSchemaConstraints(constraints []projection.SchemaConstraint) []domain.SchemaConstraint {
	result := make([]domain.SchemaConstraint, 0, len(constraints))
	for _, constraint := range constraints {
		result = append(result, domain.SchemaConstraint{Name: constraint.Name, Value: constraint.Value})
	}
	return result
}

func browserFirstProjectionExample(examples []projection.Example) string {
	value, _ := browserProjectionExample(examples)
	return value
}

func browserProjectionExample(examples []projection.Example) (string, bool) {
	if len(examples) == 0 {
		return "", false
	}
	return examples[0].Text, examples[0].Provided
}

func browserOperationCurl(operation domain.Operation) string {
	lines := []string{fmt.Sprintf("curl --request %s \\", strings.ToUpper(operation.Method))}
	if operation.RequestBody != nil && len(operation.RequestBody.MediaTypes) > 0 {
		media := operation.RequestBody.MediaTypes[0]
		if strings.TrimSpace(media.ContentType) != "" {
			lines = append(lines, "  --header "+browserShellSingleQuote("content-type: "+media.ContentType)+" \\")
		}
		if strings.TrimSpace(media.Example) != "" {
			lines = append(lines, "  --data "+browserShellSingleQuote(media.Example)+" \\")
		}
	}
	lines = append(lines, "  --url "+browserShellSingleQuote(operation.Path))
	return strings.Join(lines, "\n")
}

func browserShellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
