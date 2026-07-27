package projectionjson

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/araihu/manja/application/projection"
)

func validateRecordSemantics(document projection.Document) error {
	if err := validateOverviewSemantics(document); err != nil {
		return err
	}
	targets := map[string]string{
		"main-content":       "fixed",
		"overview":           "overview",
		"overview-heading":   "fixed",
		"operations-heading": "fixed",
		"schemas-heading":    "fixed",
		"schemas":            "fixed",
	}
	operations, err := validateOperationSemantics(document, targets)
	if err != nil {
		return err
	}
	schemas, err := validateSchemaSemantics(document, targets)
	if err != nil {
		return err
	}
	if err := validateSidebarSemantics(document, operations, schemas, targets); err != nil {
		return err
	}
	if err := validateSearchSemantics(document, targets); err != nil {
		return err
	}
	return validatePublicRouteSemantics(document, targets)
}

func validateOverviewSemantics(document projection.Document) error {
	if document.Overview.Heading != document.Title {
		return codecFailure("overview", "non_canonical")
	}
	serverIDs := make(map[string]struct{}, len(document.Overview.Servers))
	for serverIndex, server := range document.Overview.Servers {
		if server.Ordinal != uint32(serverIndex) || server.ID != semanticRecordID("server", server.URL) {
			return codecFailure("overview.servers", "non_canonical")
		}
		if _, duplicate := serverIDs[server.ID]; duplicate {
			return codecFailure("overview.servers", "duplicate_record")
		}
		serverIDs[server.ID] = struct{}{}
		variableIDs := make(map[string]struct{}, len(server.Variables))
		for variableIndex, variable := range server.Variables {
			if variable.Ordinal != uint32(variableIndex) || !validIdentity(variable.Name) ||
				variable.ID != semanticRecordID("server-variable", server.ID, variable.Name) {
				return codecFailure("overview.servers.variables", "non_canonical")
			}
			if _, duplicate := variableIDs[variable.ID]; duplicate {
				return codecFailure("overview.servers.variables", "duplicate_record")
			}
			variableIDs[variable.ID] = struct{}{}
			if err := validateTextRecords(variable.Enum, "server-enum", false, false, "overview.servers.variables.enum"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOperationSemantics(document projection.Document, targets map[string]string) (map[string]projection.OperationDirectory, error) {
	if len(document.Operations) != len(document.OperationDetails) {
		return nil, codecFailure("operations", "invalid_source")
	}
	operations := make(map[string]projection.OperationDirectory, len(document.Operations))
	previousAnchor := ""
	for index := range document.Operations {
		directory := document.Operations[index]
		detail := document.OperationDetails[index]
		if directory.Ordinal != uint32(index) || detail.Ordinal != uint32(index) ||
			!validProjectionAnchor(directory.Anchor) || index > 0 && directory.Anchor <= previousAnchor ||
			directory.ID != directory.Anchor || directory.Href != selectedHref(directory.Anchor) ||
			!validIdentity(directory.Method) || !validIdentity(directory.Path) ||
			detail.ID != directory.Anchor || detail.Anchor != directory.Anchor || detail.Href != directory.Href ||
			detail.HeadingID != directory.Anchor || detail.HeadingLevel != 3 ||
			detail.Heading != directory.Title || detail.Method != directory.Method || detail.Path != directory.Path ||
			detail.Deprecated != directory.Deprecated || detail.Summary != "" && detail.Heading != detail.Summary {
			return nil, codecFailure("operations", "non_canonical")
		}
		if _, duplicate := targets[directory.Anchor]; duplicate {
			return nil, codecFailure("operations", "duplicate_record")
		}
		targets[directory.Anchor] = "operation"
		operations[directory.Anchor] = directory
		previousAnchor = directory.Anchor
		if err := validateOperationNestedSemantics(detail); err != nil {
			return nil, err
		}
	}
	return operations, nil
}

func validateOperationNestedSemantics(detail projection.OperationDetail) error {
	if err := validateTextRecords(detail.Tags, "tag", true, true, "operationDetails.tags"); err != nil {
		return err
	}
	parameterIDs := make(map[string]struct{}, len(detail.Parameters))
	for index, parameter := range detail.Parameters {
		expectedID := semanticRecordID("parameter", strings.ToLower(parameter.In), parameter.Name)
		if parameter.Ordinal != uint32(index) || !validIdentity(parameter.Name) || !validIdentity(parameter.In) || parameter.ID != expectedID {
			return codecFailure("operationDetails.parameters", "non_canonical")
		}
		if _, duplicate := parameterIDs[parameter.ID]; duplicate {
			return codecFailure("operationDetails.parameters", "duplicate_record")
		}
		parameterIDs[parameter.ID] = struct{}{}
		if err := validateExamples(parameter.Examples, false, "operationDetails.parameters.examples"); err != nil {
			return err
		}
	}
	if !detail.HasRequestBody &&
		(detail.RequestBody.Description != "" || detail.RequestBody.Required || len(detail.RequestBody.MediaTypes) != 0) {
		return codecFailure("operationDetails.requestBody", "non_canonical")
	}
	if err := validateMediaTypes(detail.RequestBody.MediaTypes, "operationDetails.requestBody.mediaTypes"); err != nil {
		return err
	}
	responseIDs := make(map[string]struct{}, len(detail.Responses))
	for index, response := range detail.Responses {
		if response.Ordinal != uint32(index) || !validIdentity(response.Status) || response.ID != response.Status {
			return codecFailure("operationDetails.responses", "non_canonical")
		}
		if _, duplicate := responseIDs[response.ID]; duplicate {
			return codecFailure("operationDetails.responses", "duplicate_record")
		}
		responseIDs[response.ID] = struct{}{}
		if err := validateMediaTypes(response.MediaTypes, "operationDetails.responses.mediaTypes"); err != nil {
			return err
		}
	}
	securityIDs := make(map[string]struct{}, len(detail.Security))
	for index, requirement := range detail.Security {
		if requirement.Ordinal != uint32(index) || !validIdentity(requirement.Name) || requirement.ID != requirement.Name {
			return codecFailure("operationDetails.security", "non_canonical")
		}
		if _, duplicate := securityIDs[requirement.ID]; duplicate {
			return codecFailure("operationDetails.security", "duplicate_record")
		}
		securityIDs[requirement.ID] = struct{}{}
		if err := validateTextRecords(requirement.Scopes, "scope", false, true, "operationDetails.security.scopes"); err != nil {
			return err
		}
		for _, scope := range requirement.Scopes {
			if !validIdentity(scope.Value) {
				return codecFailure("operationDetails.security.scopes", "non_canonical")
			}
		}
	}
	codeSampleIDs := make(map[string]struct{}, len(detail.CodeSamples))
	for index, sample := range detail.CodeSamples {
		if sample.Ordinal != uint32(index) || !validOptionalIdentity(sample.Language) ||
			sample.ID != semanticRecordID("code-sample", sample.Language, sample.Label) {
			return codecFailure("operationDetails.codeSamples", "non_canonical")
		}
		if _, duplicate := codeSampleIDs[sample.ID]; duplicate {
			return codecFailure("operationDetails.codeSamples", "duplicate_record")
		}
		codeSampleIDs[sample.ID] = struct{}{}
	}
	return nil
}

func validateMediaTypes(mediaTypes []projection.MediaType, recordPath string) error {
	ids := make(map[string]struct{}, len(mediaTypes))
	for index, media := range mediaTypes {
		if media.Ordinal != uint32(index) || !validIdentity(media.ContentType) || media.ID != media.ContentType {
			return codecFailure(recordPath, "non_canonical")
		}
		if _, duplicate := ids[media.ID]; duplicate {
			return codecFailure(recordPath, "duplicate_record")
		}
		ids[media.ID] = struct{}{}
		if err := validateExamples(media.Examples, true, recordPath+".examples"); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaSemantics(document projection.Document, targets map[string]string) (map[string]projection.SchemaDirectory, error) {
	if len(document.Schemas) != len(document.SchemaDetails) {
		return nil, codecFailure("schemas", "invalid_source")
	}
	schemas := make(map[string]projection.SchemaDirectory, len(document.Schemas))
	previousAnchor := ""
	for index := range document.Schemas {
		directory := document.Schemas[index]
		detail := document.SchemaDetails[index]
		expectedAnchor := "schema-" + semanticSlugASCII(directory.Name)
		if directory.Ordinal != uint32(index) || detail.Ordinal != uint32(index) ||
			!validIdentity(directory.Name) || expectedAnchor == "schema-" || directory.Anchor != expectedAnchor ||
			index > 0 && directory.Anchor <= previousAnchor ||
			directory.ID != directory.Anchor || directory.Href != selectedHref(directory.Anchor) ||
			directory.Name != directory.Title ||
			detail.ID != directory.Anchor || detail.Anchor != directory.Anchor || detail.Href != directory.Href ||
			detail.HeadingID != directory.Anchor || detail.Heading != directory.Title || detail.HeadingLevel != 3 ||
			detail.Description != directory.Description {
			return nil, codecFailure("schemas", "non_canonical")
		}
		if _, duplicate := targets[directory.Anchor]; duplicate {
			return nil, codecFailure("schemas", "duplicate_record")
		}
		targets[directory.Anchor] = "schema"
		schemas[directory.Anchor] = directory
		previousAnchor = directory.Anchor
		if err := validateEmbeddedString(detail.ExampleSchemaJSON); err != nil {
			return nil, err
		}
		if err := validateExamples(detail.Examples, true, "schemaDetails.examples"); err != nil {
			return nil, err
		}
	}
	return schemas, nil
}

func validateSidebarSemantics(
	document projection.Document,
	operations map[string]projection.OperationDirectory,
	schemas map[string]projection.SchemaDirectory,
	targets map[string]string,
) error {
	operationSectionCount := len(document.SidebarSections)
	if len(schemas) > 0 {
		if operationSectionCount == 0 {
			return codecFailure("sidebarSections", "invalid_source")
		}
		operationSectionCount--
	}
	sections := make(map[string]projection.SidebarSection, operationSectionCount)
	memberships := make(map[string]map[uint32]struct{}, operationSectionCount)
	previousID := ""
	for index, section := range document.SidebarSections {
		if section.Ordinal != uint32(index) {
			return codecFailure("sidebarSections", "non_canonical")
		}
		if index >= operationSectionCount {
			if err := validateSchemaSidebarSection(section, document.Schemas); err != nil {
				return err
			}
			continue
		}
		if section.Kind != "operation-tag" || section.ID != semanticRecordID("operation-tag", section.Title) ||
			section.Href != "?section="+url.QueryEscape(section.ID) || index > 0 && section.ID <= previousID {
			return codecFailure("sidebarSections", "non_canonical")
		}
		if _, duplicate := targets[section.ID]; duplicate {
			return codecFailure("sidebarSections", "duplicate_record")
		}
		targets[section.ID] = "operation-tag"
		sections[section.ID] = section
		memberships[section.ID] = make(map[uint32]struct{}, len(section.Items))
		previousID = section.ID
		previousOperation := uint32(0)
		for itemIndex, item := range section.Items {
			if int(item.Ordinal) >= len(document.Operations) || itemIndex > 0 && item.Ordinal <= previousOperation {
				return codecFailure("sidebarSections.items", "non_canonical")
			}
			operation, exists := operations[item.Anchor]
			if !exists || operation.Ordinal != item.Ordinal ||
				item.ID != operation.ID || item.Anchor != operation.Anchor || item.Href != operation.Href ||
				item.Label != operation.Title || item.Method != operation.Method {
				return codecFailure("sidebarSections.items", "non_canonical")
			}
			if _, duplicate := memberships[section.ID][item.Ordinal]; duplicate {
				return codecFailure("sidebarSections.items", "duplicate_record")
			}
			memberships[section.ID][item.Ordinal] = struct{}{}
			previousOperation = item.Ordinal
		}
		if len(section.Items) == 0 {
			return codecFailure("sidebarSections.items", "invalid_source")
		}
	}
	if len(schemas) == 0 && len(document.SidebarSections) != operationSectionCount {
		return codecFailure("sidebarSections", "invalid_source")
	}
	for operationIndex, operation := range document.Operations {
		detail := document.OperationDetails[operationIndex]
		expectedTitles := make([]string, 0, len(detail.Tags))
		for _, tag := range detail.Tags {
			expectedTitles = append(expectedTitles, tag.Value)
		}
		if len(expectedTitles) == 0 {
			expectedTitles = append(expectedTitles, "Untagged")
		}
		if len(operation.Sections) != len(expectedTitles) {
			return codecFailure("operations.sections", "invalid_source")
		}
		for sectionIndex, title := range expectedTitles {
			sectionID := semanticRecordID("operation-tag", title)
			record := operation.Sections[sectionIndex]
			if record.Ordinal != uint32(sectionIndex) || record.Value != sectionID ||
				record.ID != semanticRecordID("operation-section", sectionID) {
				return codecFailure("operations.sections", "non_canonical")
			}
			section, exists := sections[sectionID]
			if !exists || section.Title != title {
				return codecFailure("operations.sections", "invalid_source")
			}
			if _, exists := memberships[sectionID][uint32(operationIndex)]; !exists {
				return codecFailure("operations.sections", "invalid_source")
			}
		}
	}
	for sectionID, members := range memberships {
		for operationIndex := range members {
			found := false
			for _, membership := range document.Operations[operationIndex].Sections {
				if membership.Value == sectionID {
					found = true
					break
				}
			}
			if !found {
				return codecFailure("sidebarSections.items", "invalid_source")
			}
		}
	}
	return nil
}

func validateSchemaSidebarSection(section projection.SidebarSection, schemas []projection.SchemaDirectory) error {
	if section.ID != "schemas" || section.Kind != "schemas" || section.Title != "Schemas" ||
		section.Href != "?section=schemas" || len(section.Items) != len(schemas) {
		return codecFailure("sidebarSections.schemas", "non_canonical")
	}
	for index, item := range section.Items {
		schema := schemas[index]
		if item.Ordinal != uint32(index) || item.ID != schema.ID || item.Anchor != schema.Anchor ||
			item.Href != schema.Href || item.Label != schema.Title || item.Method != "" {
			return codecFailure("sidebarSections.schemas.items", "non_canonical")
		}
	}
	return nil
}

func validateSearchSemantics(document projection.Document, targets map[string]string) error {
	previousID := ""
	resultIDs := make(map[string]struct{}, len(document.Search))
	for index, record := range document.Search {
		if record.Ordinal != uint32(index) || !validIdentity(record.ID) || index > 0 && record.ID <= previousID ||
			record.ResultID != semanticRecordID("search-result", record.ID) ||
			!validOptionalIdentity(record.Method) || !validOptionalIdentity(record.Path) || !validOptionalIdentity(record.Section) {
			return codecFailure("search", "non_canonical")
		}
		if _, duplicate := resultIDs[record.ResultID]; duplicate {
			return codecFailure("search", "duplicate_record")
		}
		if _, duplicate := targets[record.ResultID]; duplicate {
			return codecFailure("search", "duplicate_record")
		}
		resultIDs[record.ResultID] = struct{}{}
		targets[record.ResultID] = "search-result"
		target, err := parseSelectedHref(record.Href)
		if err != nil || record.Kind != "overview" && record.Kind != "operation" && record.Kind != "schema" ||
			targets[target] != record.Kind {
			return codecFailure("search", "invalid_source")
		}
		if err := validateTextRecords(record.Keywords, "keyword", true, true, "search.keywords"); err != nil {
			return err
		}
		previousID = record.ID
	}
	return nil
}

func validatePublicRouteSemantics(document projection.Document, targets map[string]string) error {
	previousKey := ""
	for index, route := range document.PublicRoutes {
		key := route.Path + "\x00" + route.Title
		if route.Ordinal != uint32(index) || index > 0 && key <= previousKey ||
			!validCanonicalPublicRoute(route.Path, targets) {
			return codecFailure("publicRoutes", "non_canonical")
		}
		previousKey = key
	}
	return nil
}

func validCanonicalPublicRoute(value string, targets map[string]string) bool {
	if value == "/" {
		return true
	}
	reference, err := url.Parse(value)
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil ||
		strings.Contains(value, "\\") || strings.TrimSpace(value) != value ||
		reference.Path == "" || !strings.HasPrefix(reference.Path, "/") || path.Clean(reference.Path) != reference.Path ||
		reference.Fragment == "" {
		return false
	}
	values, ok := reference.Query()["selected"]
	if !ok || len(reference.Query()) != 1 || len(values) != 1 || values[0] != reference.Fragment {
		return false
	}
	kind := targets[reference.Fragment]
	if kind != "overview" && kind != "operation" && kind != "schema" {
		return false
	}
	return value == reference.Path+selectedHref(reference.Fragment)
}

func validateExamples(examples []projection.Example, allowEmpty bool, recordPath string) error {
	if len(examples) > 1 {
		return codecFailure(recordPath, "non_canonical")
	}
	if len(examples) == 0 {
		return nil
	}
	example := examples[0]
	if example.Ordinal != 0 || example.ID != "primary" || !example.Provided || !allowEmpty && example.Text == "" {
		return codecFailure(recordPath, "non_canonical")
	}
	return nil
}

func validateTextRecords(records []projection.TextRecord, kind string, rejectBlank, unique bool, recordPath string) error {
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		if record.Ordinal != uint32(index) || record.ID != semanticRecordID(kind, record.Value) ||
			rejectBlank && strings.TrimSpace(record.Value) == "" {
			return codecFailure(recordPath, "non_canonical")
		}
		if unique {
			if _, duplicate := seen[record.Value]; duplicate {
				return codecFailure(recordPath, "duplicate_record")
			}
			seen[record.Value] = struct{}{}
		}
	}
	return nil
}

func validOptionalIdentity(value string) bool {
	return value == "" || validIdentity(value)
}

func validProjectionAnchor(value string) bool {
	for index, r := range value {
		if r > unicode.MaxASCII {
			return false
		}
		if index == 0 {
			if !semanticASCIILowerOrDigit(byte(r)) {
				return false
			}
			continue
		}
		if !semanticASCIILowerOrDigit(byte(r)) && !strings.ContainsRune("._~/-", r) {
			return false
		}
	}
	return value != ""
}

func semanticASCIILowerOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func semanticSlugASCII(value string) string {
	var result strings.Builder
	dash := false
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			result.WriteByte(byte(r + ('a' - 'A')))
			dash = false
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			result.WriteByte(byte(r))
			dash = false
		default:
			if result.Len() != 0 {
				dash = true
			}
		}
		if dash {
			current := result.String()
			if !strings.HasSuffix(current, "-") {
				result.WriteByte('-')
			}
			dash = false
		}
	}
	return strings.Trim(result.String(), "-")
}

func semanticRecordID(kind string, parts ...string) string {
	hash := sha256.New()
	hash.Write([]byte(kind))
	hash.Write([]byte{0})
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(part))))
		hash.Write(length[:])
		hash.Write([]byte(part))
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil))
}
