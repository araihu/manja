package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/araihu/manja/internal/core"
)

type Parser struct{}

func (Parser) Parse(ctx context.Context, file core.SpecFile, rev core.Revision) (core.SpecIndex, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromDataWithPath(file.Bytes, &url.URL{Path: file.Path})
	if err != nil {
		return core.SpecIndex{}, err
	}
	if err := doc.Validate(ctx, openapi3.DisableExamplesValidation()); err != nil {
		return core.SpecIndex{}, err
	}
	download, err := specDownload(doc, file.Path)
	if err != nil {
		return core.SpecIndex{}, err
	}

	idx := core.SpecIndex{
		RevisionID:      rev.ID,
		Title:           doc.Info.Title,
		Version:         doc.Info.Version,
		Overview:        overview(doc.Info, doc.Servers),
		SpecDownload:    download,
		ExampleSpecJSON: exampleSpecJSON(doc),
	}

	serverURL := firstServerURL(doc.Servers)
	inferFragments := operationCount(doc) <= 100
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			operation := core.Operation{
				ID:          op.OperationID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Summary:     op.Summary,
				Description: op.Description,
				Tags:        append([]string(nil), op.Tags...),
				Deprecated:  op.Deprecated,
			}
			operation.Anchor = operationAnchor(operation)
			operation.Parameters = operationParameters(item.Parameters, op.Parameters)
			operation.RequestBody = operationRequestBody(op.RequestBody, inferFragments)
			operation.Responses = operationResponses(op.Responses, inferFragments)
			operation.Security = operationSecurity(doc.Security, op.Security)
			operation.Snippets = operationSnippets(operation, firstNonEmpty(firstOperationServerURL(op.Servers), serverURL), inferFragments)
			idx.Operations = append(idx.Operations, operation)
		}
	}
	sort.Slice(idx.Operations, func(i, j int) bool {
		if idx.Operations[i].Path == idx.Operations[j].Path {
			return idx.Operations[i].Method < idx.Operations[j].Method
		}
		return idx.Operations[i].Path < idx.Operations[j].Path
	})

	if doc.Components != nil {
		for name, schema := range doc.Components.Schemas {
			summary := schemaSummary(schema)
			if summary.Name == "" {
				summary.Name = name
			}
			idx.Schemas = append(idx.Schemas, core.Schema{
				Name:        name,
				Description: summary.Description,
				Summary:     summary,
				Example:     schemaExample(schema),
			})
		}
	}
	sort.Slice(idx.Schemas, func(i, j int) bool { return idx.Schemas[i].Name < idx.Schemas[j].Name })

	idx.Search = buildSearch(idx)
	idx.PublicRoutes = buildPublicRoutes(idx)
	return idx, nil
}

func operationCount(doc *openapi3.T) int {
	if doc == nil || doc.Paths == nil {
		return 0
	}
	count := 0
	for _, item := range doc.Paths.Map() {
		count += len(item.Operations())
	}
	return count
}

func specDownload(doc *openapi3.T, path string) (core.SpecDownload, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return core.SpecDownload{}, fmt.Errorf("marshal OpenAPI JSON download: %w", err)
	}
	return core.SpecDownload{
		JSON:     data,
		Filename: jsonSpecFilename(path),
	}, nil
}

func jsonSpecFilename(path string) string {
	name := filepath.Base(strings.TrimSpace(path))
	if name == "" || name == "." || name == "/" {
		return "openapi.json"
	}
	ext := filepath.Ext(name)
	if ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "openapi.json"
	}
	return name + ".json"
}

func overview(info *openapi3.Info, servers openapi3.Servers) core.SpecOverview {
	var result core.SpecOverview
	if info != nil {
		result.Description = info.Description
		result.TermsOfService = info.TermsOfService
		if info.Contact != nil {
			result.Contact = core.SpecContact{
				Name:  info.Contact.Name,
				URL:   info.Contact.URL,
				Email: info.Contact.Email,
			}
		}
		if info.License != nil {
			result.License = core.SpecLicense{
				Name:       info.License.Name,
				URL:        info.License.URL,
				Identifier: info.License.Identifier,
			}
		}
	}
	result.Servers = overviewServers(servers)
	return result
}

func overviewServers(servers openapi3.Servers) []core.SpecServer {
	result := make([]core.SpecServer, 0, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		result = append(result, core.SpecServer{
			URL:         server.URL,
			Description: server.Description,
			Variables:   overviewServerVariables(server.Variables),
		})
	}
	return result
}

func overviewServerVariables(variables openapi3.ServerVariables) []core.SpecServerVariable {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]core.SpecServerVariable, 0, len(names))
	for _, name := range names {
		variable := variables[name]
		if variable == nil {
			continue
		}
		result = append(result, core.SpecServerVariable{
			Name:        name,
			Default:     variable.Default,
			Description: variable.Description,
			Enum:        append([]string(nil), variable.Enum...),
		})
	}
	return result
}

func operationParameters(groups ...openapi3.Parameters) []core.OperationParameter {
	var parameters []core.OperationParameter
	for _, group := range groups {
		for _, ref := range group {
			if ref == nil || ref.Value == nil {
				continue
			}
			parameter := ref.Value
			parameters = append(parameters, core.OperationParameter{
				Name:        parameter.Name,
				In:          parameter.In,
				Required:    parameter.Required,
				Description: parameter.Description,
				Schema:      schemaSummary(parameter.Schema),
				Example:     exampleString(parameter.Example),
			})
		}
	}
	return parameters
}

func operationRequestBody(ref *openapi3.RequestBodyRef, inferExamples bool) *core.OperationRequestBody {
	if ref == nil || ref.Value == nil {
		return nil
	}
	body := ref.Value
	return &core.OperationRequestBody{
		Description: body.Description,
		Required:    body.Required,
		MediaTypes:  operationMediaTypes(body.Content, inferExamples),
	}
}

func operationResponses(responses *openapi3.Responses, inferExamples bool) []core.OperationResponse {
	if responses == nil {
		return nil
	}
	statuses := responses.Keys()
	sort.Slice(statuses, func(i, j int) bool {
		return responseStatusLess(statuses[i], statuses[j])
	})
	indexed := make([]core.OperationResponse, 0, len(statuses))
	for _, status := range statuses {
		ref := responses.Value(status)
		if ref == nil || ref.Value == nil {
			continue
		}
		response := ref.Value
		description := ""
		if response.Description != nil {
			description = *response.Description
		}
		indexed = append(indexed, core.OperationResponse{
			Status:      status,
			Description: description,
			MediaTypes:  operationMediaTypes(response.Content, inferExamples),
		})
	}
	return indexed
}

func responseStatusLess(left, right string) bool {
	leftDefault := strings.EqualFold(left, "default")
	rightDefault := strings.EqualFold(right, "default")
	if leftDefault || rightDefault {
		return rightDefault
	}
	return left < right
}

func operationMediaTypes(content openapi3.Content, inferExamples bool) []core.OperationMediaType {
	if len(content) == 0 {
		return nil
	}
	contentTypes := make([]string, 0, len(content))
	for contentType := range content {
		contentTypes = append(contentTypes, contentType)
	}
	sort.Strings(contentTypes)
	mediaTypes := make([]core.OperationMediaType, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		media := content[contentType]
		if media == nil {
			continue
		}
		summary := schemaSummary(media.Schema)
		example, exampleProvided := mediaExample(media, inferExamples)
		mediaTypes = append(mediaTypes, core.OperationMediaType{
			ContentType:     contentType,
			Schema:          summary,
			Example:         example,
			ExampleProvided: exampleProvided,
		})
	}
	return mediaTypes
}

func mediaExample(media *openapi3.MediaType, inferExamples bool) (string, bool) {
	if media == nil {
		return "", false
	}
	if example := exampleString(media.Example); example != "" {
		return example, true
	}
	for _, key := range sortedExampleKeys(media.Examples) {
		ref := media.Examples[key]
		if ref != nil && ref.Value != nil {
			if example := exampleString(ref.Value.Value); example != "" {
				return example, true
			}
		}
	}
	if media.Schema != nil && media.Schema.Value != nil {
		if example, provided := schemaProvidedExample(media.Schema.Value); provided {
			return example, true
		}
		if inferExamples {
			if example := sampleWithOpenAPISampler(media.Schema.Value); example != "" {
				return example, false
			}
		}
		if example := exampleString(simpleSample(media.Schema.Value)); example != "" {
			return example, false
		}
	}
	return "", false
}

func sortedExampleKeys(examples openapi3.Examples) []string {
	keys := make([]string, 0, len(examples))
	for key := range examples {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func operationSecurity(root openapi3.SecurityRequirements, operation *openapi3.SecurityRequirements) []core.OperationSecurity {
	requirements := root
	if operation != nil {
		requirements = *operation
	}
	if len(requirements) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var security []core.OperationSecurity
	for _, requirement := range requirements {
		names := make([]string, 0, len(requirement))
		for name := range requirement {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if seen[name] {
				continue
			}
			seen[name] = true
			security = append(security, core.OperationSecurity{
				Name:   name,
				Scopes: append([]string(nil), requirement[name]...),
			})
		}
	}
	return security
}

func operationSnippets(operation core.Operation, serverURL string, inferFragments bool) []core.RequestSnippet {
	if serverURL == "" {
		return nil
	}
	request := map[string]any{
		"method": strings.ToUpper(operation.Method),
		"url":    strings.TrimRight(serverURL, "/") + operation.Path,
	}
	if operation.RequestBody != nil && len(operation.RequestBody.MediaTypes) > 0 {
		media := operation.RequestBody.MediaTypes[0]
		request["headers"] = []map[string]string{{"name": "content-type", "value": media.ContentType}}
		if media.Example != "" {
			request["postData"] = map[string]string{"mimeType": media.ContentType, "text": media.Example}
		}
	}
	code := ""
	if inferFragments && operation.RequestBody != nil {
		code = requestSnippetWithHTTPSnippet(request)
	}
	if code == "" {
		code = fallbackCurlSnippet(request)
	}
	if code == "" {
		return nil
	}
	return []core.RequestSnippet{{
		Label:    "cURL",
		Language: "shell",
		Code:     code,
	}}
}

func schemaSummary(ref *openapi3.SchemaRef) core.SchemaSummary {
	return schemaSummaryDepth(ref, 0)
}

func schemaSummaryDepth(ref *openapi3.SchemaRef, depth int) core.SchemaSummary {
	if ref == nil {
		return core.SchemaSummary{}
	}
	summary := core.SchemaSummary{Name: refName(ref.Ref)}
	if ref.Value == nil || depth > 4 {
		return summary
	}
	schema := ref.Value
	if strings.TrimSpace(schema.Title) != "" {
		summary.Name = schema.Title
	}
	summary.Type = schemaType(schema)
	summary.Format = schema.Format
	summary.Description = schema.Description
	summary.Example = exampleString(schema.Example)
	if depth == 0 {
		summary.JSON = schemaJSON(schema)
	}
	if schema.Items != nil {
		items := schemaSummaryDepth(schema.Items, depth+1)
		summary.Items = &items
	}
	required := requiredProperties(schema.Required)
	propertyNames := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)
	for _, name := range propertyNames {
		propertySummary := schemaSummaryDepth(schema.Properties[name], depth+1)
		summary.Properties = append(summary.Properties, core.SchemaProperty{
			Name:        name,
			Required:    required[name],
			Schema:      propertySummary,
			Description: propertySummary.Description,
		})
	}
	return summary
}

func refName(ref string) string {
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func schemaType(schema *openapi3.Schema) string {
	if schema == nil || schema.Type == nil {
		return ""
	}
	types := schema.Type.Slice()
	if len(types) == 0 {
		return ""
	}
	return strings.Join(types, " | ")
}

func requiredProperties(names []string) map[string]bool {
	required := make(map[string]bool, len(names))
	for _, name := range names {
		required[name] = true
	}
	return required
}

func exampleString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		data, err := json.MarshalIndent(typed, "", "  ")
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func schemaExample(ref *openapi3.SchemaRef) core.SchemaExample {
	if ref == nil || ref.Value == nil {
		return core.SchemaExample{}
	}
	example, provided := schemaProvidedExample(ref.Value)
	return core.SchemaExample{
		JSON:     schemaJSON(ref.Value),
		Example:  example,
		Provided: provided,
	}
}

func schemaProvidedExample(schema *openapi3.Schema) (string, bool) {
	if schema == nil {
		return "", false
	}
	if example := exampleString(schema.Example); example != "" {
		return example, true
	}
	for _, candidate := range schema.Examples {
		if example := exampleString(candidate); example != "" {
			return example, true
		}
	}
	return "", false
}

func schemaJSON(schema *openapi3.Schema) string {
	if schema == nil {
		return ""
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return string(data)
}

func exampleSpecJSON(doc *openapi3.T) string {
	if doc == nil || doc.Components == nil || len(doc.Components.Schemas) == 0 {
		return ""
	}
	data, err := json.Marshal(map[string]any{
		"components": map[string]any{
			"schemas": doc.Components.Schemas,
		},
	})
	if err != nil {
		return ""
	}
	return string(data)
}

func sampleWithOpenAPISampler(schema *openapi3.Schema) string {
	if !canInferSample(schema, 0, 0) {
		return ""
	}
	return runFragmentsScript(map[string]any{
		"mode":   "sample",
		"schema": schema,
	})
}

func canInferSample(schema *openapi3.Schema, depth int, nodes int) bool {
	if schema == nil || depth > 2 || nodes > 6 {
		return false
	}
	if len(schema.OneOf) > 0 || len(schema.AnyOf) > 0 || len(schema.AllOf) > 0 {
		return false
	}
	if schema.Type == nil {
		return false
	}
	if schema.Type.Includes("array") {
		return schema.Items != nil && schema.Items.Value != nil && canInferSample(schema.Items.Value, depth+1, nodes+1)
	}
	if !schema.Type.Includes("object") {
		return true
	}
	if len(schema.Properties) > 6 {
		return false
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ref := schema.Properties[name]
		if ref == nil || ref.Value == nil {
			continue
		}
		nodes++
		if !canInferSample(ref.Value, depth+1, nodes) {
			return false
		}
	}
	return true
}

func requestSnippetWithHTTPSnippet(request map[string]any) string {
	return runFragmentsScript(map[string]any{
		"mode":    "snippet",
		"request": request,
	})
}

func runFragmentsScript(payload map[string]any) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	cmd := exec.Command("node", filepath.Join(filepath.Dir(filename), "fragments.mjs"))
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func simpleSample(schema *openapi3.Schema) any {
	if schema == nil {
		return nil
	}
	switch {
	case schema.Type != nil && schema.Type.Includes("object"):
		object := map[string]any{}
		names := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if ref := schema.Properties[name]; ref != nil && ref.Value != nil {
				object[name] = simpleSample(ref.Value)
			}
		}
		return object
	case schema.Type != nil && schema.Type.Includes("array"):
		if schema.Items != nil && schema.Items.Value != nil {
			return []any{simpleSample(schema.Items.Value)}
		}
		return []any{}
	case schema.Type != nil && schema.Type.Includes("boolean"):
		return true
	case schema.Type != nil && (schema.Type.Includes("number") || schema.Type.Includes("integer")):
		return 1
	default:
		return "string"
	}
}

func fallbackCurlSnippet(request map[string]any) string {
	method, _ := request["method"].(string)
	rawURL, _ := request["url"].(string)
	if method == "" || rawURL == "" {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "curl --request %s \\\n  --url %s", method, rawURL)
	if headers, ok := request["headers"].([]map[string]string); ok {
		for _, header := range headers {
			fmt.Fprintf(&builder, " \\\n  --header '%s: %s'", header["name"], header["value"])
		}
	}
	if postData, ok := request["postData"].(map[string]string); ok && postData["text"] != "" {
		fmt.Fprintf(&builder, " \\\n  --data '%s'", strings.ReplaceAll(postData["text"], "'", "'\\''"))
	}
	return builder.String()
}

func firstServerURL(servers openapi3.Servers) string {
	if len(servers) == 0 || servers[0] == nil {
		return ""
	}
	return servers[0].URL
}

func firstOperationServerURL(servers *openapi3.Servers) string {
	if servers == nil {
		return ""
	}
	return firstServerURL(*servers)
}

func buildSearch(idx core.SpecIndex) []core.SearchDocument {
	docs := make([]core.SearchDocument, 0, len(idx.Operations)+len(idx.Schemas)+1)
	for _, op := range idx.Operations {
		anchor := operationAnchor(op)
		title := fmt.Sprintf("%s %s", op.Method, op.Path)
		docs = append(docs, core.SearchDocument{
			ID:          anchor,
			Title:       title,
			Description: firstNonEmpty(op.Summary, op.Description),
			Href:        "#" + anchor,
			Kind:        "Operation",
			Section:     strings.Join(op.Tags, ", "),
			Keywords:    []string{op.ID, op.Method, op.Path, strings.Join(op.Tags, " ")},
		})
	}
	for _, schema := range idx.Schemas {
		docs = append(docs, core.SearchDocument{
			ID:          "schema-" + schema.Name,
			Title:       schema.Name,
			Description: schema.Description,
			Href:        "#schema-" + strings.ToLower(schema.Name),
			Kind:        "Schema",
			Section:     "Schemas",
			Keywords:    []string{"schema", schema.Name},
		})
	}
	docs = append(docs, core.SearchDocument{
		ID:          "overview",
		Title:       firstNonEmpty(idx.Title, "Overview"),
		Description: firstNonEmpty(idx.Overview.Description, "API overview"),
		Href:        "#overview",
		Kind:        "Overview",
		Section:     "Overview",
		Keywords:    []string{"overview", idx.Title},
	})
	return docs
}

func buildPublicRoutes(idx core.SpecIndex) []core.PublicRoute {
	routes := []core.PublicRoute{{Path: "/", Title: idx.Title, Description: "API overview"}}
	for _, op := range idx.Operations {
		anchor := operationAnchor(op)
		routes = append(routes, core.PublicRoute{
			Path:        "#" + anchor,
			Title:       op.Method + " " + op.Path,
			Description: firstNonEmpty(op.Summary, op.Description),
		})
	}
	return routes
}

func operationAnchor(op core.Operation) string {
	if op.Anchor != "" {
		return op.Anchor
	}
	fragment := anchorFragment(op.ID)
	if fragment == "" {
		fragment = anchorFragment(op.Method + " " + op.Path)
	}
	return "operation-" + fragment
}

func anchorFragment(value string) string {
	var builder strings.Builder
	lastWasSeparator := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastWasSeparator = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastWasSeparator = false
		default:
			if builder.Len() > 0 && !lastWasSeparator {
				builder.WriteByte('-')
				lastWasSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
