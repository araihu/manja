package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web/templates"
)

type pageMarkdownDocument struct {
	Filename string
	Body     string
}

func publicPageMarkdown(index domain.SpecIndex, selectedID string) (pageMarkdownDocument, bool) {
	selectedID = strings.TrimPrefix(strings.TrimSpace(selectedID), "#")
	for _, operation := range index.Operations {
		if publicOperationAnchor(operation) == selectedID {
			return operationPageMarkdown(index.Title, operation), true
		}
	}
	for _, schema := range index.Schemas {
		if "schema-"+strings.ToLower(schema.Name) == selectedID {
			return schemaPageMarkdown(index.Title, schema), true
		}
	}
	return pageMarkdownDocument{}, false
}

func selectedPageMarkdownHref(pagePath, selectedID string) string {
	query := url.Values{"format": []string{"markdown"}}
	if selectedID = strings.TrimSpace(selectedID); selectedID != "" {
		query.Set("selected", selectedID)
	}
	return pagePath + "?" + query.Encode()
}

func catalogPageMarkdown(data templates.CatalogPageData) (pageMarkdownDocument, bool) {
	title := data.Directory.Title
	if data.Document != nil && strings.TrimSpace(data.Document.Title) != "" {
		title = data.Document.Title
	}
	if data.OperationView != nil {
		return operationPageMarkdown(title, *data.OperationView), true
	}
	if data.SchemaView != nil {
		return schemaPageMarkdown(title, *data.SchemaView), true
	}
	return pageMarkdownDocument{}, false
}

func publicLLMsText(index domain.SpecIndex) pageMarkdownDocument {
	var output strings.Builder
	title := firstNonEmpty(index.Title, "OpenAPI documentation")
	fmt.Fprintf(&output, "# %s\n\n", markdownHeading(title))
	if description := strings.TrimSpace(index.Overview.Description); description != "" {
		fmt.Fprintf(&output, "> %s\n\n", markdownInline(description))
	}
	output.WriteString("- [Documentation overview](/)\n")
	if len(index.SpecDownload.JSON) > 0 {
		output.WriteString("- [OpenAPI JSON](/openapi.json)\n")
	}

	hrefs := publicRouteHrefsByAnchor(index.PublicRoutes)
	if len(index.Operations) > 0 {
		output.WriteString("\n## Operations\n\n")
		for _, operation := range index.Operations {
			anchor := publicOperationAnchor(operation)
			href := hrefs[anchor]
			if href == "" {
				href = selectedDocsSearchHref("#" + anchor)
			}
			label := firstNonEmpty(operation.Title, operation.Summary, operation.ID, strings.TrimSpace(strings.ToUpper(operation.Method)+" "+operation.Path))
			fmt.Fprintf(&output, "- [%s](%s) — `%s`\n", markdownInline(label), href, markdownInline(strings.TrimSpace(strings.ToUpper(operation.Method)+" "+operation.Path)))
		}
	}
	if len(index.Schemas) > 0 {
		output.WriteString("\n## Schemas\n\n")
		for _, schema := range index.Schemas {
			anchor := "schema-" + strings.ToLower(schema.Name)
			href := hrefs[anchor]
			if href == "" {
				href = selectedDocsSearchHref("#" + anchor)
			}
			fmt.Fprintf(&output, "- [%s](%s)\n", markdownInline(schema.Name), href)
		}
	}
	return pageMarkdownDocument{Filename: "llms.txt", Body: finishMarkdown(output.String())}
}

func catalogLLMsText(directory catalog.CatalogArtifactV1, mount string) pageMarkdownDocument {
	var output strings.Builder
	title := firstNonEmpty(directory.Title, "OpenAPI catalog")
	fmt.Fprintf(&output, "# %s\n\n", markdownHeading(title))
	output.WriteString("> OpenAPI catalog published with Manja.\n\n")
	overviewHref, _ := catalogURL(mount)
	catalogJSONHref, _ := catalogURL(mount, "catalog.json")
	fmt.Fprintf(&output, "- [Catalog overview](%s)\n", overviewHref)
	fmt.Fprintf(&output, "- [Catalog JSON](%s)\n", catalogJSONHref)
	if len(directory.Documents) > 0 {
		output.WriteString("\n## API documents\n\n")
		for _, document := range directory.Documents {
			href, _ := catalogURL(mount, "documents", document.Key)
			label := catalogDocumentLabel(document)
			if strings.TrimSpace(document.APIVersion) != "" && !strings.EqualFold(document.APIVersion, "unversioned") {
				label += " (" + document.APIVersion + ")"
			}
			fmt.Fprintf(
				&output, "- [%s](%s/) — %d %s, %d %s\n",
				markdownInline(label), href,
				len(document.Operations), pluralLabel(len(document.Operations), "operation", "operations"),
				len(document.Schemas), pluralLabel(len(document.Schemas), "schema", "schemas"),
			)
		}
	}
	return pageMarkdownDocument{Filename: "llms.txt", Body: finishMarkdown(output.String())}
}

func pluralLabel(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func operationPageMarkdown(specTitle string, operation domain.Operation) pageMarkdownDocument {
	var output strings.Builder
	title := firstNonEmpty(operation.Title, operation.Summary, operation.ID, strings.TrimSpace(operation.Method+" "+operation.Path))
	fmt.Fprintf(&output, "# %s\n\n", markdownHeading(title))
	if strings.TrimSpace(specTitle) != "" {
		fmt.Fprintf(&output, "From **%s**.\n\n", markdownInline(specTitle))
	}
	fmt.Fprintf(&output, "%s\n\n", markdownCode(strings.TrimSpace(strings.ToUpper(operation.Method)+" "+operation.Path)))
	if operation.Deprecated {
		output.WriteString("**Deprecated.**\n\n")
	}
	writeMarkdownProse(&output, firstNonEmpty(operation.Description, operation.Summary))

	if len(operation.Parameters) > 0 {
		output.WriteString("## Parameters\n\n")
		for _, parameter := range operation.Parameters {
			fmt.Fprintf(&output, "### %s\n\n", markdownCode(parameter.Name))
			writeMarkdownField(&output, "Location", parameter.In)
			writeMarkdownField(&output, "Required", strconv.FormatBool(parameter.Required))
			if parameter.Example != "" {
				writeMarkdownCodeField(&output, "Example", parameter.Example)
			}
			writeSchemaMetadata(&output, parameter.Schema)
			writeMarkdownProse(&output, firstNonEmpty(parameter.Description, parameter.Schema.Description))
		}
	}

	if operation.RequestBody != nil {
		output.WriteString("## Request body\n\n")
		writeMarkdownField(&output, "Required", strconv.FormatBool(operation.RequestBody.Required))
		writeMarkdownProse(&output, operation.RequestBody.Description)
		writeMediaTypesMarkdown(&output, operation.RequestBody.MediaTypes)
	}

	if len(operation.Security) > 0 {
		output.WriteString("## Security\n\n")
		for _, security := range operation.Security {
			fmt.Fprintf(&output, "### %s\n\n", markdownCode(security.Name))
			writeMarkdownCodeField(&output, "Type", security.Definition.Type)
			if parameterName := operationSecurityParameterName(security); parameterName != "" {
				writeMarkdownCodeField(&output, securityParameterLocationLabel(security.Definition.In), parameterName)
			}
			writeMarkdownCodeField(&output, "Scheme", security.Definition.Scheme)
			writeMarkdownCodeField(&output, "Bearer format", security.Definition.BearerFormat)
			if len(security.Scopes) > 0 {
				output.WriteString("**Scopes:** ")
				writeInlineCodeList(&output, security.Scopes)
				output.WriteString("\n\n")
			}
			writeMarkdownProse(&output, operationSecurityInstruction(security))
			writeMarkdownProse(&output, security.Definition.Description)
		}
	}

	if len(operation.Responses) > 0 {
		output.WriteString("## Responses\n\n")
		for _, response := range operation.Responses {
			fmt.Fprintf(&output, "### %s\n\n", markdownCode(response.Status))
			writeMarkdownProse(&output, response.Description)
			writeResponseHeadersMarkdown(&output, response.Headers)
			writeMediaTypesMarkdown(&output, response.MediaTypes)
		}
	}

	if len(operation.Snippets) > 0 {
		output.WriteString("## Request examples\n\n")
		for _, snippet := range operation.Snippets {
			fmt.Fprintf(&output, "### %s\n\n", markdownHeading(snippet.Label))
			writeMarkdownFence(&output, snippet.Language, snippet.Code)
		}
	}

	return pageMarkdownDocument{Filename: markdownFilename(title), Body: finishMarkdown(output.String())}
}

func securityParameterLocationLabel(location string) string {
	location = strings.TrimSpace(strings.ToLower(location))
	switch location {
	case "header", "":
		return "Request header"
	case "query":
		return "Query parameter"
	case "cookie":
		return "Cookie"
	}
	return strings.ToUpper(location[:1]) + location[1:] + " parameter"
}

func operationSecurityParameterName(security domain.OperationSecurity) string {
	if strings.TrimSpace(security.Definition.ParameterName) != "" {
		return security.Definition.ParameterName
	}
	typeName := strings.ToLower(strings.TrimSpace(security.Definition.Type))
	if typeName == "http" || typeName == "oauth2" || typeName == "openidconnect" {
		return "Authorization"
	}
	return ""
}

func operationSecurityInstruction(security domain.OperationSecurity) string {
	definition := security.Definition
	typeName := strings.ToLower(strings.TrimSpace(definition.Type))
	scheme := strings.ToLower(strings.TrimSpace(definition.Scheme))
	parameter := firstNonEmpty(operationSecurityParameterName(security), "Authorization")
	location := strings.ToLower(strings.TrimSpace(definition.In))
	switch {
	case typeName == "http" && scheme == "bearer":
		return "Send the token in the `Authorization` request header using the `Bearer` scheme."
	case typeName == "http" && scheme == "basic":
		return "Send credentials in the `Authorization` request header using the `Basic` scheme."
	case typeName == "oauth2" || typeName == "openidconnect":
		return "Send the access token in the `Authorization` request header using the `Bearer` scheme."
	case typeName == "apikey" && location == "header":
		return "Send the API key in the `" + parameter + "` request header."
	case typeName == "apikey" && location != "":
		return "Send the API key in the `" + parameter + "` " + location + " parameter."
	}
	return ""
}

func schemaPageMarkdown(specTitle string, schema domain.Schema) pageMarkdownDocument {
	var output strings.Builder
	title := firstNonEmpty(schema.Name, schema.Summary.Name, "Schema")
	fmt.Fprintf(&output, "# %s\n\n", markdownHeading(title))
	if strings.TrimSpace(specTitle) != "" {
		fmt.Fprintf(&output, "From **%s**.\n\n", markdownInline(specTitle))
	}
	writeMarkdownProse(&output, firstNonEmpty(schema.Description, schema.Summary.Description))
	writeSchemaMetadata(&output, schema.Summary)

	if len(schema.Summary.Properties) > 0 {
		output.WriteString("## Properties\n\n")
		for _, property := range schema.Summary.Properties {
			fmt.Fprintf(&output, "### %s\n\n", markdownCode(property.Name))
			writeMarkdownField(&output, "Required", strconv.FormatBool(property.Required))
			writeSchemaMetadata(&output, property.Schema)
			writeMarkdownProse(&output, firstNonEmpty(property.Description, property.Schema.Description))
		}
	}
	if schema.Summary.JSON != "" {
		output.WriteString("## JSON Schema\n\n")
		writeMarkdownFence(&output, "json", schema.Summary.JSON)
	}
	if schema.Example.Example != "" {
		output.WriteString("## Example\n\n")
		writeMarkdownFence(&output, "json", schema.Example.Example)
	}
	return pageMarkdownDocument{Filename: markdownFilename(title), Body: finishMarkdown(output.String())}
}

func writeResponseHeadersMarkdown(output *strings.Builder, headers []domain.OperationResponseHeader) {
	if len(headers) == 0 {
		return
	}
	output.WriteString("#### Headers\n\n")
	for _, header := range headers {
		fmt.Fprintf(output, "##### %s\n\n", markdownCode(header.Name))
		writeSchemaMetadata(output, header.Schema)
		if header.Example != "" && header.Example != header.Schema.Example {
			writeMarkdownCodeField(output, "Example", header.Example)
			output.WriteString("\n")
		}
		writeMarkdownProse(output, firstNonEmpty(header.Description, header.Schema.Description))
	}
}

func writeMediaTypesMarkdown(output *strings.Builder, mediaTypes []domain.OperationMediaType) {
	for _, media := range mediaTypes {
		fmt.Fprintf(output, "### %s\n\n", markdownCode(media.ContentType))
		writeSchemaMetadata(output, media.Schema)
		writeMarkdownProse(output, media.Schema.Description)
		if media.Example != "" {
			writeMarkdownFence(output, "json", media.Example)
		}
		if media.Schema.JSON != "" {
			writeMarkdownFence(output, "json", media.Schema.JSON)
		}
	}
}

func writeSchemaMetadata(output *strings.Builder, schema domain.SchemaSummary) {
	writeMarkdownCodeField(output, "Type", schema.Type)
	writeMarkdownCodeField(output, "Format", schema.Format)
	if schema.Nullable {
		writeMarkdownField(output, "Nullable", "true")
	}
	if schema.Deprecated {
		writeMarkdownField(output, "Deprecated", "true")
	}
	writeMarkdownCodeField(output, "Default", schema.Default)
	writeMarkdownCodeField(output, "Example", schema.Example)
	if len(schema.Enum) > 0 {
		output.WriteString("- Allowed values: ")
		writeInlineCodeList(output, schema.Enum)
		output.WriteString("\n")
	}
	for _, constraint := range schema.Constraints {
		writeMarkdownCodeField(output, constraint.Name, constraint.Value)
	}
	if schema.Type != "" || schema.Format != "" || schema.Nullable || schema.Deprecated || schema.Default != "" || schema.Example != "" || len(schema.Enum) > 0 || len(schema.Constraints) > 0 {
		output.WriteString("\n")
	}
}

func writeMarkdownField(output *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(output, "- %s: %s\n", markdownInline(label), markdownInline(value))
}

func writeMarkdownCodeField(output *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(output, "- %s: %s\n", markdownInline(label), markdownCode(value))
}

func writeMarkdownProse(output *strings.Builder, value string) {
	value = markdownSourceText(value)
	if value == "" {
		return
	}
	output.WriteString(value)
	output.WriteString("\n\n")
}

func writeInlineCodeList(output *strings.Builder, values []string) {
	for index, value := range values {
		if index > 0 {
			output.WriteString(", ")
		}
		output.WriteString(markdownCode(value))
	}
}

func writeMarkdownFence(output *strings.Builder, language, value string) {
	value = markdownSourceText(value)
	if value == "" {
		return
	}
	fence := strings.Repeat("`", max(3, longestRun(value, '`')+1))
	fmt.Fprintf(output, "%s%s\n%s\n%s\n\n", fence, markdownFenceLanguage(language), value, fence)
}

func markdownFenceLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func markdownCode(value string) string {
	value = strings.TrimSpace(markdownSourceText(value))
	fence := strings.Repeat("`", max(1, longestRun(value, '`')+1))
	padding := ""
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") || strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		padding = " "
	}
	return fence + padding + value + padding + fence
}

func markdownInline(value string) string {
	value = markdownSourceText(value)
	replacer := strings.NewReplacer(
		"\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]",
		"<", "\\<", ">", "\\>", "|", "\\|", "#", "\\#",
	)
	return replacer.Replace(value)
}

func markdownHeading(value string) string {
	return strings.ReplaceAll(markdownInline(value), "\n", " ")
}

func markdownSourceText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' || !unicode.IsControl(character) {
			return character
		}
		return -1
	}, value)
	return strings.TrimSpace(value)
}

func longestRun(value string, target rune) int {
	longest, current := 0, 0
	for _, character := range value {
		if character == target {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

func finishMarkdown(value string) string {
	return strings.TrimRight(value, "\n") + "\n"
}

func markdownFilename(value string) string {
	value = strings.ToLower(markdownSourceText(value))
	var result strings.Builder
	separator := false
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(character)
			separator = false
		} else {
			separator = true
		}
	}
	name := strings.Trim(result.String(), "-")
	if name == "" {
		name = "api-page"
	}
	return name + ".md"
}

func publicOperationAnchor(operation domain.Operation) string {
	if strings.TrimSpace(operation.Anchor) != "" {
		return operation.Anchor
	}
	if strings.TrimSpace(operation.ID) != "" {
		return "operation-" + operation.ID
	}
	fragment := pageAnchorFragment(operation.Method + " " + operation.Path)
	if fragment == "" {
		return "operation"
	}
	return "operation-" + fragment
}

func pageAnchorFragment(value string) string {
	var result strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			result.WriteRune(character)
			separator = false
		default:
			if result.Len() > 0 && !separator {
				result.WriteByte('-')
				separator = true
			}
		}
	}
	return strings.Trim(result.String(), "-")
}

func writePageMarkdown(response http.ResponseWriter, request *http.Request, document pageMarkdownDocument) {
	body := []byte(document.Body)
	digest := sha256.Sum256(body)
	etag := `"sha256-` + hex.EncodeToString(digest[:]) + `"`
	response.Header().Set("Cache-Control", "private, no-cache")
	response.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	response.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": document.Filename}))
	response.Header().Set("ETag", etag)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	response.Header().Set("Content-Length", strconv.Itoa(len(body)))
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(body)
	}
}
