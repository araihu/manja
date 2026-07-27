package projectionjson

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/araihu/manja/application/projection"
)

func validateDocument(document projection.Document) error {
	if document.FormatVersion != 1 {
		return codecFailure("formatVersion", "invalid_source")
	}
	if !validIdentity(document.ProjectID) || !validIdentity(document.RevisionID) {
		return codecFailure("identity", "invalid_identity")
	}
	if err := validateValue(reflect.ValueOf(document)); err != nil {
		return err
	}
	if document.MainLandmark != (projection.Landmark{ID: "main-content", Role: "main"}) ||
		document.Overview.Anchor != "overview" || document.Overview.Href != "?selected=overview#overview" ||
		document.Overview.HeadingID != "overview-heading" || document.Overview.HeadingLevel != 2 ||
		document.OperationGroupHeading != (projection.Heading{ID: "operations-heading", Text: "Operations", Level: 2}) ||
		document.SchemaGroupHeading != (projection.Heading{ID: "schemas-heading", Text: "Schemas", Level: 2}) {
		return codecFailure("navigation", "invalid_source")
	}
	if err := validateTopLevelRecords(document); err != nil {
		return err
	}
	if err := validateWireSchemas(document); err != nil {
		return err
	}
	return nil
}

func validIdentity(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validateValue(value reflect.Value) error {
	switch value.Kind() {
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return codecFailure("string", "invalid_utf8")
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if err := validateValue(value.Field(index)); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return codecFailure("collection", "non_canonical")
		}
		for index := 0; index < value.Len(); index++ {
			if err := validateValue(value.Index(index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTopLevelRecords(document projection.Document) error {
	if len(document.Operations) != len(document.OperationDetails) || len(document.Schemas) != len(document.SchemaDetails) {
		return codecFailure("records", "invalid_source")
	}
	targets := map[string]string{"overview": "overview"}
	previous := ""
	for index := range document.Operations {
		directory := document.Operations[index]
		detail := document.OperationDetails[index]
		if directory.Ordinal != uint32(index) || detail.Ordinal != uint32(index) || directory.Anchor <= previous ||
			directory.ID != directory.Anchor || directory.Href != selectedHref(directory.Anchor) ||
			detail.ID != directory.Anchor || detail.Anchor != directory.Anchor || detail.Href != directory.Href ||
			detail.HeadingID != directory.Anchor || detail.HeadingLevel != 3 {
			return codecFailure("operations", "non_canonical")
		}
		if _, duplicate := targets[directory.Anchor]; duplicate {
			return codecFailure("operations", "duplicate_record")
		}
		targets[directory.Anchor] = "operation"
		previous = directory.Anchor
		if err := validateOperationNested(detail); err != nil {
			return err
		}
	}
	previous = ""
	for index := range document.Schemas {
		directory := document.Schemas[index]
		detail := document.SchemaDetails[index]
		if directory.Ordinal != uint32(index) || detail.Ordinal != uint32(index) || directory.Anchor <= previous ||
			directory.ID != directory.Anchor || directory.Href != selectedHref(directory.Anchor) ||
			detail.ID != directory.Anchor || detail.Anchor != directory.Anchor || detail.Href != directory.Href ||
			detail.HeadingID != directory.Anchor || detail.HeadingLevel != 3 {
			return codecFailure("schemas", "non_canonical")
		}
		if _, duplicate := targets[directory.Anchor]; duplicate {
			return codecFailure("schemas", "duplicate_record")
		}
		targets[directory.Anchor] = "schema"
		previous = directory.Anchor
		if err := validateEmbeddedString(detail.ExampleSchemaJSON); err != nil {
			return err
		}
	}
	previous = ""
	for index, record := range document.Search {
		if record.Ordinal != uint32(index) || record.ID <= previous || record.ResultID == record.ID {
			return codecFailure("search", "non_canonical")
		}
		target, err := parseSelectedHref(record.Href)
		if err != nil || targets[target] != record.Kind {
			return codecFailure("search", "invalid_source")
		}
		previous = record.ID
	}
	previousKey := ""
	for index, route := range document.PublicRoutes {
		key := route.Path + "\x00" + route.Title
		if route.Ordinal != uint32(index) || index > 0 && key <= previousKey {
			return codecFailure("publicRoutes", "non_canonical")
		}
		previousKey = key
	}
	for index, section := range document.SidebarSections {
		if section.Ordinal != uint32(index) || section.Items == nil {
			return codecFailure("sidebarSections", "non_canonical")
		}
	}
	return nil
}

func validateOperationNested(detail projection.OperationDetail) error {
	if detail.RequestBody.MediaTypes == nil || detail.Tags == nil || detail.Parameters == nil || detail.Responses == nil || detail.Security == nil || detail.CodeSamples == nil {
		return codecFailure("operationDetails", "non_canonical")
	}
	for index, parameter := range detail.Parameters {
		if parameter.Ordinal != uint32(index) || parameter.Examples == nil {
			return codecFailure("operationDetails.parameters", "non_canonical")
		}
	}
	for index, response := range detail.Responses {
		if response.Ordinal != uint32(index) || response.MediaTypes == nil {
			return codecFailure("operationDetails.responses", "non_canonical")
		}
	}
	return nil
}

func validateWireSchemas(document projection.Document) error {
	type entry struct {
		schema *projection.WireSchema
		depth  int
	}
	stack := []entry{}
	push := func(schema *projection.WireSchema) { stack = append(stack, entry{schema: schema, depth: 1}) }
	for index := range document.OperationDetails {
		detail := &document.OperationDetails[index]
		for parameterIndex := range detail.Parameters {
			push(&detail.Parameters[parameterIndex].Schema)
		}
		for mediaIndex := range detail.RequestBody.MediaTypes {
			push(&detail.RequestBody.MediaTypes[mediaIndex].Schema)
		}
		for responseIndex := range detail.Responses {
			for mediaIndex := range detail.Responses[responseIndex].MediaTypes {
				push(&detail.Responses[responseIndex].MediaTypes[mediaIndex].Schema)
			}
		}
	}
	for index := range document.SchemaDetails {
		push(&document.SchemaDetails[index].Schema)
	}
	nodes := 0
	for len(stack) != 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		nodes++
		if current.depth > 64 || nodes > 100_000 || len(current.schema.Items) > 1 {
			return codecFailure("wireSchema", "invalid_source")
		}
		if err := validateEmbeddedString(current.schema.JSON); err != nil {
			return err
		}
		for index := range current.schema.Properties {
			property := &current.schema.Properties[index]
			if property.Ordinal != uint32(index) || property.ID != property.Name {
				return codecFailure("wireSchema.properties", "non_canonical")
			}
			stack = append(stack, entry{schema: &property.Schema, depth: current.depth + 1})
		}
		for index := range current.schema.Items {
			item := &current.schema.Items[index]
			if item.Ordinal != 0 || item.ID != "items" {
				return codecFailure("wireSchema.items", "non_canonical")
			}
			stack = append(stack, entry{schema: &item.Schema, depth: current.depth + 1})
		}
	}
	return nil
}

func validateEmbeddedString(value string) error {
	if value == "" {
		return nil
	}
	bytesValue := []byte(value)
	if len(bytesValue) > maxSchemaBytes || !utf8.Valid(bytesValue) {
		return codecFailure("embeddedJSON", "invalid_utf8")
	}
	if err := validateJSONTokens(bytesValue, 64, canonicalEmbeddedNumber); err != nil {
		return codecFailure("embeddedJSON", "non_canonical")
	}
	decoder := json.NewDecoder(bytes.NewReader(bytesValue))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return codecFailure("embeddedJSON", "invalid_source")
	}
	encoded, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(encoded, bytesValue) {
		return codecFailure("embeddedJSON", "non_canonical")
	}
	return nil
}

func selectedHref(anchor string) string {
	return "?selected=" + url.QueryEscape(anchor) + "#" + anchor
}

func parseSelectedHref(value string) (string, error) {
	reference, err := url.Parse(value)
	if err != nil || reference.Path != "" || reference.Fragment == "" || reference.Query().Get("selected") != reference.Fragment || len(reference.Query()) != 1 {
		return "", codecFailure("href", "invalid_source")
	}
	return reference.Fragment, nil
}

var _ = sort.Strings
