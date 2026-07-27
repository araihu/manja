package projection

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/araihu/manja/domain"
)

func (Builder) Build(ctx context.Context, index domain.SpecIndex) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	validationCopy := index
	validationCopy.SpecDownload.JSON = nil
	validationCopy.ExampleSpecJSON = ""
	if err := domain.ValidateCanonicalIdentity("projection project ID", validationCopy.ProjectID, false); err != nil {
		return Document{}, projectionFailure("projectId", "invalid_identity")
	}
	if err := domain.ValidateCanonicalIdentity("projection revision ID", validationCopy.RevisionID, false); err != nil {
		return Document{}, projectionFailure("revisionId", "invalid_identity")
	}
	if err := domain.ValidateSpecIndex(validationCopy); err != nil {
		return Document{}, projectionFailure("source", "invalid_source")
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	state := buildState{
		ctx: ctx,
		targets: map[string]string{
			"main-content":       "fixed",
			"overview":           "overview",
			"overview-heading":   "fixed",
			"operations-heading": "fixed",
			"schemas-heading":    "fixed",
			"schemas":            "fixed",
		},
		aliases: make(map[string]string),
	}
	document, err := state.build(index)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Document{}, err
		}
		return Document{}, err
	}
	return document, nil
}

type buildState struct {
	ctx     context.Context
	targets map[string]string
	aliases map[string]string
}

type indexedOperation struct {
	source domain.Operation
	anchor string
}

type indexedSchema struct {
	source domain.Schema
	anchor string
}

func (s *buildState) build(index domain.SpecIndex) (Document, error) {
	overview, err := s.buildOverview(index)
	if err != nil {
		return Document{}, err
	}
	operations, err := s.indexOperations(index.Operations)
	if err != nil {
		return Document{}, err
	}
	schemas, err := s.indexSchemas(index.Schemas)
	if err != nil {
		return Document{}, err
	}
	operationDirectories, operationDetails, sections, err := s.buildOperations(operations)
	if err != nil {
		return Document{}, err
	}
	schemaDirectories, schemaDetails, schemaSection, err := s.buildSchemas(schemas)
	if err != nil {
		return Document{}, err
	}
	if schemaSection != nil {
		sections = append(sections, *schemaSection)
	}
	for index := range sections {
		sections[index].Ordinal = uint32(index)
	}
	search, err := s.buildSearch(index.Search)
	if err != nil {
		return Document{}, err
	}
	routes, err := s.buildPublicRoutes(index.PublicRoutes)
	if err != nil {
		return Document{}, err
	}
	return Document{
		FormatVersion: 1, ProjectID: index.ProjectID, RevisionID: index.RevisionID,
		Title: index.Title, APIVersion: index.Version,
		Branding: Branding{
			DisplayName: index.Branding.DisplayName, LogoSrc: index.Branding.Logo.Src,
			LogoAlt: index.Branding.Logo.Alt, LogoHomeHref: index.Branding.Logo.HomeURL,
			FaviconHref: index.Branding.Favicon,
		},
		Overview:              overview,
		MainLandmark:          Landmark{ID: "main-content", Role: "main"},
		OperationGroupHeading: Heading{ID: "operations-heading", Text: "Operations", Level: 2},
		SchemaGroupHeading:    Heading{ID: "schemas-heading", Text: "Schemas", Level: 2},
		SidebarSections:       sections, Operations: operationDirectories, OperationDetails: operationDetails,
		Schemas: schemaDirectories, SchemaDetails: schemaDetails, Search: search, PublicRoutes: routes,
	}, nil
}

func (s *buildState) checkpoint() error {
	return s.ctx.Err()
}

func (s *buildState) claimTarget(id, kind string) error {
	if _, exists := s.targets[id]; exists {
		return projectionFailure("navigation.id", "duplicate_record")
	}
	s.targets[id] = kind
	return nil
}

func (s *buildState) buildOverview(index domain.SpecIndex) (Overview, error) {
	servers := make([]Server, 0, len(index.Overview.Servers))
	serverIDs := make(map[string]struct{})
	for sourceIndex, source := range index.Overview.Servers {
		if err := s.checkpoint(); err != nil {
			return Overview{}, err
		}
		server := Server{Ordinal: uint32(sourceIndex), ID: recordID("server", source.URL), URL: source.URL, Description: source.Description, Variables: []ServerVariable{}}
		if _, duplicate := serverIDs[server.ID]; duplicate {
			return Overview{}, projectionFailure("overview.servers", "duplicate_record")
		}
		serverIDs[server.ID] = struct{}{}
		variableIDs := make(map[string]struct{})
		for variableIndex, variable := range source.Variables {
			if err := s.checkpoint(); err != nil {
				return Overview{}, err
			}
			built := ServerVariable{
				Ordinal: uint32(variableIndex), ID: recordID("server-variable", server.ID, variable.Name),
				Name: variable.Name, Default: variable.Default, Description: variable.Description,
				Enum: textRecords("server-enum", variable.Enum, false, false),
			}
			if _, duplicate := variableIDs[built.ID]; duplicate {
				return Overview{}, projectionFailure("overview.servers.variables", "duplicate_record")
			}
			variableIDs[built.ID] = struct{}{}
			server.Variables = append(server.Variables, built)
		}
		servers = append(servers, server)
	}
	return Overview{
		Anchor: "overview", Href: selectedHref("overview"), HeadingID: "overview-heading",
		Heading: index.Title, HeadingLevel: 2, Description: index.Overview.Description,
		TermsOfService: index.Overview.TermsOfService, SpecDownloadFilename: index.SpecDownload.Filename,
		Contact: Contact{Name: index.Overview.Contact.Name, URL: index.Overview.Contact.URL, Email: index.Overview.Contact.Email},
		License: License{Name: index.Overview.License.Name, URL: index.Overview.License.URL, Identifier: index.Overview.License.Identifier},
		Servers: servers,
	}, nil
}

func (s *buildState) indexOperations(source []domain.Operation) ([]indexedOperation, error) {
	operations := make([]indexedOperation, 0, len(source))
	for _, operation := range source {
		if err := s.checkpoint(); err != nil {
			return nil, err
		}
		anchor, err := operationAnchor(operation.ID, operation.Anchor, operation.Method, operation.Path)
		if err != nil {
			return nil, err
		}
		if err := s.claimTarget(anchor, "operation"); err != nil {
			return nil, err
		}
		operations = append(operations, indexedOperation{source: operation, anchor: anchor})
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].anchor < operations[j].anchor })
	return operations, nil
}

func (s *buildState) indexSchemas(source []domain.Schema) ([]indexedSchema, error) {
	schemas := make([]indexedSchema, 0, len(source))
	for _, schema := range source {
		if err := s.checkpoint(); err != nil {
			return nil, err
		}
		anchor := "schema-" + slugASCII(schema.Name)
		if anchor == "schema-" {
			return nil, projectionFailure("schemas.anchor", "invalid_source")
		}
		if err := s.claimTarget(anchor, "schema"); err != nil {
			return nil, err
		}
		legacy := "schema-" + strings.ToLower(schema.Name)
		if existing, exists := s.aliases[legacy]; exists && existing != anchor {
			return nil, projectionFailure("schemas.alias", "duplicate_record")
		}
		if kind, exists := s.targets[legacy]; exists && legacy != anchor && kind != "schema" {
			return nil, projectionFailure("schemas.alias", "duplicate_record")
		}
		s.aliases[legacy] = anchor
		schemas = append(schemas, indexedSchema{source: schema, anchor: anchor})
	}
	sort.Slice(schemas, func(i, j int) bool { return schemas[i].anchor < schemas[j].anchor })
	return schemas, nil
}

func (s *buildState) buildOperations(source []indexedOperation) ([]OperationDirectory, []OperationDetail, []SidebarSection, error) {
	directories := make([]OperationDirectory, 0, len(source))
	details := make([]OperationDetail, 0, len(source))
	sectionByID := make(map[string]*SidebarSection)
	for operationIndex, indexed := range source {
		if err := s.checkpoint(); err != nil {
			return nil, nil, nil, err
		}
		operation := indexed.source
		tags := deduplicateStrings(operation.Tags, true)
		memberships := tags
		if len(memberships) == 0 {
			memberships = []string{"Untagged"}
		}
		sections := make([]TextRecord, 0, len(memberships))
		for membershipIndex, tag := range memberships {
			sectionID := recordID("operation-tag", tag)
			section, exists := sectionByID[sectionID]
			if !exists {
				if err := s.claimTarget(sectionID, "operation-tag"); err != nil {
					return nil, nil, nil, err
				}
				section = &SidebarSection{ID: sectionID, Kind: "operation-tag", Title: tag, Href: "?section=" + url.QueryEscape(sectionID), Items: []SidebarItem{}}
				sectionByID[sectionID] = section
			}
			section.Items = append(section.Items, SidebarItem{
				Ordinal: uint32(operationIndex), ID: indexed.anchor, Anchor: indexed.anchor,
				Href: selectedHref(indexed.anchor), Label: operationTitle(operation), Method: operation.Method,
			})
			sections = append(sections, TextRecord{Ordinal: uint32(membershipIndex), ID: recordID("operation-section", sectionID), Value: sectionID})
		}
		directory := OperationDirectory{
			Ordinal: uint32(operationIndex), ID: indexed.anchor, Anchor: indexed.anchor,
			Href: selectedHref(indexed.anchor), Method: operation.Method, Path: operation.Path,
			Title: operationTitle(operation), Deprecated: operation.Deprecated, Sections: sections,
		}
		detail, err := s.buildOperationDetail(uint32(operationIndex), indexed)
		if err != nil {
			return nil, nil, nil, err
		}
		directories = append(directories, directory)
		details = append(details, detail)
	}
	sections := make([]SidebarSection, 0, len(sectionByID))
	for _, section := range sectionByID {
		sections = append(sections, *section)
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].ID < sections[j].ID })
	return directories, details, sections, nil
}

func (s *buildState) buildOperationDetail(ordinal uint32, indexed indexedOperation) (OperationDetail, error) {
	operation := indexed.source
	parameters := make([]Parameter, 0, len(operation.Parameters))
	parameterIDs := make(map[string]struct{})
	for sourceIndex, parameter := range operation.Parameters {
		if err := s.checkpoint(); err != nil {
			return OperationDetail{}, err
		}
		schema, err := s.buildWireSchema(parameter.Schema)
		if err != nil {
			return OperationDetail{}, err
		}
		id := recordID("parameter", strings.ToLower(parameter.In), parameter.Name)
		if _, duplicate := parameterIDs[id]; duplicate {
			return OperationDetail{}, projectionFailure("operations.parameters", "duplicate_record")
		}
		parameterIDs[id] = struct{}{}
		examples := []Example{}
		if parameter.Example != "" {
			examples = append(examples, Example{Ordinal: 0, ID: "primary", Text: parameter.Example, Provided: true})
		}
		parameters = append(parameters, Parameter{Ordinal: uint32(sourceIndex), ID: id, Name: parameter.Name, In: parameter.In, Required: parameter.Required, Description: parameter.Description, Schema: schema, Examples: examples})
	}
	requestBody := RequestBody{MediaTypes: []MediaType{}}
	hasRequestBody := operation.RequestBody != nil
	if operation.RequestBody != nil {
		requestBody.Description = operation.RequestBody.Description
		requestBody.Required = operation.RequestBody.Required
		media, err := s.buildMediaTypes(operation.RequestBody.MediaTypes)
		if err != nil {
			return OperationDetail{}, err
		}
		requestBody.MediaTypes = media
	}
	responses := make([]Response, 0, len(operation.Responses))
	responseIDs := make(map[string]struct{})
	for sourceIndex, response := range operation.Responses {
		media, err := s.buildMediaTypes(response.MediaTypes)
		if err != nil {
			return OperationDetail{}, err
		}
		if _, duplicate := responseIDs[response.Status]; duplicate {
			return OperationDetail{}, projectionFailure("operations.responses", "duplicate_record")
		}
		responseIDs[response.Status] = struct{}{}
		responses = append(responses, Response{Ordinal: uint32(sourceIndex), ID: response.Status, Status: response.Status, Description: response.Description, MediaTypes: media})
	}
	security := make([]SecurityRequirement, 0, len(operation.Security))
	securityIDs := make(map[string]struct{})
	for sourceIndex, requirement := range operation.Security {
		if _, duplicate := securityIDs[requirement.Name]; duplicate {
			return OperationDetail{}, projectionFailure("operations.security", "duplicate_record")
		}
		securityIDs[requirement.Name] = struct{}{}
		security = append(security, SecurityRequirement{Ordinal: uint32(sourceIndex), ID: requirement.Name, Name: requirement.Name, Scopes: textRecords("scope", requirement.Scopes, false, true)})
	}
	codeSamples := make([]CodeSample, 0, len(operation.Snippets))
	codeIDs := make(map[string]struct{})
	for sourceIndex, snippet := range operation.Snippets {
		id := recordID("code-sample", snippet.Language, snippet.Label)
		if _, duplicate := codeIDs[id]; duplicate {
			return OperationDetail{}, projectionFailure("operations.codeSamples", "duplicate_record")
		}
		codeIDs[id] = struct{}{}
		codeSamples = append(codeSamples, CodeSample{Ordinal: uint32(sourceIndex), ID: id, Label: snippet.Label, Language: snippet.Language, Code: snippet.Code})
	}
	return OperationDetail{
		Ordinal: ordinal, ID: indexed.anchor, Anchor: indexed.anchor, Href: selectedHref(indexed.anchor),
		HeadingID: indexed.anchor, Heading: operationTitle(operation), HeadingLevel: 3,
		Method: operation.Method, Path: operation.Path, Summary: operation.Summary,
		Description: operation.Description, Deprecated: operation.Deprecated,
		Tags: textRecords("tag", operation.Tags, true, true), Parameters: parameters,
		HasRequestBody: hasRequestBody, RequestBody: requestBody, Responses: responses,
		Security: security, CodeSamples: codeSamples,
	}, nil
}

func (s *buildState) buildMediaTypes(source []domain.OperationMediaType) ([]MediaType, error) {
	result := make([]MediaType, 0, len(source))
	ids := make(map[string]struct{})
	for sourceIndex, media := range source {
		if err := s.checkpoint(); err != nil {
			return nil, err
		}
		if _, duplicate := ids[media.ContentType]; duplicate {
			return nil, projectionFailure("operations.mediaTypes", "duplicate_record")
		}
		ids[media.ContentType] = struct{}{}
		schema, err := s.buildWireSchema(media.Schema)
		if err != nil {
			return nil, err
		}
		examples := []Example{}
		if media.ExampleProvided {
			examples = append(examples, Example{Ordinal: 0, ID: "primary", Text: media.Example, Provided: true})
		}
		result = append(result, MediaType{Ordinal: uint32(sourceIndex), ID: media.ContentType, ContentType: media.ContentType, Schema: schema, Examples: examples})
	}
	return result, nil
}

func (s *buildState) buildWireSchema(source domain.SchemaSummary) (WireSchema, error) {
	if err := s.checkpoint(); err != nil {
		return WireSchema{}, err
	}
	canonicalJSON := ""
	var err error
	if source.JSON != "" {
		canonicalJSON, err = canonicalEmbeddedJSON(source.JSON)
		if err != nil {
			return WireSchema{}, err
		}
	}
	properties := make([]SchemaProperty, 0, len(source.Properties))
	propertyIDs := make(map[string]struct{})
	for sourceIndex, property := range source.Properties {
		if _, duplicate := propertyIDs[property.Name]; duplicate {
			return WireSchema{}, projectionFailure("schema.properties", "duplicate_record")
		}
		propertyIDs[property.Name] = struct{}{}
		schema, err := s.buildWireSchema(property.Schema)
		if err != nil {
			return WireSchema{}, err
		}
		properties = append(properties, SchemaProperty{Ordinal: uint32(sourceIndex), ID: property.Name, Name: property.Name, Required: property.Required, Description: property.Description, Schema: schema})
	}
	items := []SchemaItem{}
	if source.Items != nil {
		item, err := s.buildWireSchema(*source.Items)
		if err != nil {
			return WireSchema{}, err
		}
		items = append(items, SchemaItem{Ordinal: 0, ID: "items", Schema: item})
	}
	return WireSchema{
		Name: source.Name, Type: source.Type, Format: source.Format, Description: source.Description,
		DefaultValue: source.Default, ExampleText: source.Example, JSON: canonicalJSON,
		Properties: properties, Items: items,
	}, nil
}

func (s *buildState) buildSchemas(source []indexedSchema) ([]SchemaDirectory, []SchemaDetail, *SidebarSection, error) {
	directories := make([]SchemaDirectory, 0, len(source))
	details := make([]SchemaDetail, 0, len(source))
	items := make([]SidebarItem, 0, len(source))
	for sourceIndex, indexed := range source {
		if err := s.checkpoint(); err != nil {
			return nil, nil, nil, err
		}
		schema, err := s.buildWireSchema(indexed.source.Summary)
		if err != nil {
			return nil, nil, nil, err
		}
		exampleSchemaJSON := ""
		if indexed.source.Example.JSON != "" {
			exampleSchemaJSON, err = canonicalEmbeddedJSON(indexed.source.Example.JSON)
			if err != nil {
				return nil, nil, nil, err
			}
		}
		examples := []Example{}
		if indexed.source.Example.Provided {
			examples = append(examples, Example{Ordinal: 0, ID: "primary", Text: indexed.source.Example.Example, Provided: true})
		}
		ordinal := uint32(sourceIndex)
		href := selectedHref(indexed.anchor)
		directories = append(directories, SchemaDirectory{Ordinal: ordinal, ID: indexed.anchor, Anchor: indexed.anchor, Href: href, Name: indexed.source.Name, Title: indexed.source.Name, Description: indexed.source.Description})
		details = append(details, SchemaDetail{Ordinal: ordinal, ID: indexed.anchor, Anchor: indexed.anchor, Href: href, HeadingID: indexed.anchor, Heading: indexed.source.Name, HeadingLevel: 3, Description: indexed.source.Description, Schema: schema, ExampleSchemaJSON: exampleSchemaJSON, Examples: examples})
		items = append(items, SidebarItem{Ordinal: ordinal, ID: indexed.anchor, Anchor: indexed.anchor, Href: href, Label: indexed.source.Name})
	}
	if len(source) == 0 {
		return directories, details, nil, nil
	}
	section := &SidebarSection{ID: "schemas", Kind: "schemas", Title: "Schemas", Href: "?section=schemas", Items: items}
	return directories, details, section, nil
}

func (s *buildState) resolveTarget(source string) (string, string, bool) {
	if canonical, exists := s.aliases[source]; exists {
		source = canonical
	}
	kind, exists := s.targets[source]
	if !exists || kind == "fixed" || kind == "operation-tag" {
		return "", "", false
	}
	return source, kind, true
}

func (s *buildState) buildSearch(source []domain.SearchDocument) ([]SearchRecord, error) {
	sorted := append([]domain.SearchDocument(nil), source...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	result := make([]SearchRecord, 0, len(sorted))
	ids := make(map[string]struct{})
	for sourceIndex, document := range sorted {
		if err := s.checkpoint(); err != nil {
			return nil, err
		}
		if _, duplicate := ids[document.ID]; duplicate {
			return nil, projectionFailure("search.id", "duplicate_record")
		}
		ids[document.ID] = struct{}{}
		target, err := parseSelectedReference(document.Href)
		if err != nil {
			return nil, err
		}
		anchor, targetKind, ok := s.resolveTarget(target)
		kind := strings.ToLower(strings.TrimSpace(document.Kind))
		if !ok || kind != targetKind || kind != "overview" && kind != "operation" && kind != "schema" {
			return nil, projectionFailure("search.href", "invalid_source")
		}
		resultID := recordID("search-result", document.ID)
		if err := s.claimTarget(resultID, "search-result"); err != nil {
			return nil, err
		}
		result = append(result, SearchRecord{
			Ordinal: uint32(sourceIndex), ID: document.ID, ResultID: resultID,
			Title: document.Title, Description: document.Description, Href: selectedHref(anchor),
			Kind: kind, Method: document.Method, Path: document.Path, Section: document.Section,
			Keywords: textRecords("keyword", document.Keywords, true, true),
		})
	}
	return result, nil
}

func (s *buildState) buildPublicRoutes(source []domain.PublicRoute) ([]PublicRoute, error) {
	sorted := append([]domain.PublicRoute(nil), source...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Path == sorted[j].Path {
			return sorted[i].Title < sorted[j].Title
		}
		return sorted[i].Path < sorted[j].Path
	})
	result := make([]PublicRoute, 0, len(sorted))
	keys := make(map[string]struct{})
	for sourceIndex, route := range sorted {
		canonical, err := canonicalPublicRoute(route.Path, func(value string) (string, bool) {
			anchor, _, ok := s.resolveTarget(value)
			return anchor, ok
		})
		if err != nil {
			return nil, err
		}
		key := canonical + "\x00" + route.Title
		if _, duplicate := keys[key]; duplicate {
			return nil, projectionFailure("publicRoutes", "duplicate_record")
		}
		keys[key] = struct{}{}
		result = append(result, PublicRoute{Ordinal: uint32(sourceIndex), Path: canonical, Title: route.Title, Description: route.Description})
	}
	return result, nil
}

func operationTitle(operation domain.Operation) string {
	if operation.Summary != "" {
		return operation.Summary
	}
	if operation.ID != "" {
		return operation.ID
	}
	return strings.TrimSpace(strings.ToUpper(operation.Method) + " " + operation.Path)
}

func deduplicateStrings(source []string, dropBlank bool) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(source))
	for _, value := range source {
		if dropBlank && strings.TrimSpace(value) == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func textRecords(kind string, source []string, dropBlank, deduplicate bool) []TextRecord {
	values := source
	if deduplicate {
		values = deduplicateStrings(source, dropBlank)
	}
	result := make([]TextRecord, 0, len(values))
	for sourceIndex, value := range values {
		if dropBlank && strings.TrimSpace(value) == "" {
			continue
		}
		result = append(result, TextRecord{Ordinal: uint32(len(result)), ID: recordID(kind, value), Value: value})
		_ = sourceIndex
	}
	return result
}

func projectionFailure(path, class string) error {
	message := fmt.Sprintf("projection[%s]: %s", path, class)
	if len(message) > 256 {
		message = "projection[source]: invalid_source"
	}
	return errors.New(message)
}
