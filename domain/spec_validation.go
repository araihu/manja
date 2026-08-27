package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// These bounds keep validation of untrusted parser output deterministic and
// stack-safe. The aggregate node budget is calibrated above the supported
// GitHub REST fixture's 73,034 expanded summary nodes while retaining a hard
// cap against adversarially wide documents. Pointer-shared DAG subtrees are
// memoized and charged once; independently expanded value nodes are charged.
const (
	maxSpecSchemaSummaryDepth = 64
	maxSpecSchemaSummaryNodes = 100_000
)

type ValidationOptions struct {
	ResourceLimits bool
}

// ValidateContractRevision verifies all provider-neutral immutable revision
// evidence before an adapter may clone or persist it. Display metadata is
// required only to be valid UTF-8; whitespace and newlines remain unchanged.
func ValidateContractRevision(revision ContractRevision) error {
	if err := validateUTF8Strings("contract revision", revision); err != nil {
		return err
	}
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "revision id", value: revision.ID},
		{name: "revision contract id", value: revision.ContractID, allowEmpty: true},
		{name: "revision source id", value: revision.SourceID, allowEmpty: true},
		{name: "revision blob key", value: revision.SpecBlobKey, allowEmpty: true},
		{name: "revision ref", value: revision.Ref, allowEmpty: true},
		{name: "revision commit sha", value: revision.CommitSHA, allowEmpty: true},
	} {
		if err := ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return err
		}
	}
	for _, digest := range []struct {
		name  string
		value string
	}{
		{name: "revision spec digest", value: revision.SpecDigest},
		{name: "revision contract digest", value: revision.ContractDigest},
	} {
		if digest.value != "" && !isLowerSHA256(digest.value) {
			return fmt.Errorf("%s must be lowercase SHA-256", digest.name)
		}
	}
	if revision.ReviewSnapshot != nil {
		if err := ValidateContractSnapshot(*revision.ReviewSnapshot); err != nil {
			return fmt.Errorf("revision review snapshot: %w", err)
		}
	}
	return nil
}

// ValidateSyncRecord verifies immutable sync evidence without treating its
// human-readable error summary as an identity.
func ValidateSyncRecord(record SyncRecord) error {
	if err := validateUTF8Strings("sync record", record); err != nil {
		return err
	}
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "sync id", value: record.ID},
		{name: "sync project id", value: record.ProjectID, allowEmpty: true},
		{name: "sync source id", value: record.SourceID, allowEmpty: true},
		{name: "sync revision id", value: record.RevisionID, allowEmpty: true},
		{name: "sync trigger", value: record.Trigger, allowEmpty: true},
		{name: "sync ref", value: record.Ref, allowEmpty: true},
		{name: "sync commit sha", value: record.CommitSHA, allowEmpty: true},
		{name: "sync spec path", value: record.SpecPath, allowEmpty: true},
		{name: "sync result", value: record.Result, allowEmpty: true},
	} {
		if err := ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return err
		}
	}
	return nil
}

// ValidateSpecIndex validates the parser's raw output before normalization can
// erase identity padding/control differences. All display text must round-trip
// as UTF-8, while compatibility and navigation surface identities are
// canonical.
func ValidateSpecIndex(index SpecIndex) error {
	return ValidateSpecIndexWithOptions(index, ValidationOptions{ResourceLimits: true})
}

func ValidateSpecIndexWithOptions(index SpecIndex, options ValidationOptions) error {
	if err := validateSpecIndexSchemaSummaries(index, options.ResourceLimits); err != nil {
		return err
	}
	if err := validateUTF8Strings("spec index", index); err != nil {
		return err
	}
	if len(index.SpecDownload.JSON) != 0 && !utf8.Valid(index.SpecDownload.JSON) {
		return fmt.Errorf("spec index download JSON must contain valid UTF-8")
	}
	for _, identity := range []struct {
		name       string
		value      string
		allowEmpty bool
	}{
		{name: "spec index project id", value: index.ProjectID, allowEmpty: true},
		{name: "spec index revision id", value: index.RevisionID, allowEmpty: true},
		{name: "spec download filename", value: index.SpecDownload.Filename, allowEmpty: true},
	} {
		if err := ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
			return err
		}
	}
	for serverIndex, server := range index.Overview.Servers {
		for variableIndex, variable := range server.Variables {
			if err := ValidateCanonicalIdentity(
				fmt.Sprintf("spec server %d variable %d name", serverIndex, variableIndex),
				variable.Name,
				false,
			); err != nil {
				return err
			}
		}
	}
	for operationIndex, operation := range index.Operations {
		prefix := fmt.Sprintf("spec operation %d", operationIndex)
		for _, identity := range []struct {
			name       string
			value      string
			allowEmpty bool
		}{
			{name: prefix + " id", value: operation.ID, allowEmpty: true},
			{name: prefix + " anchor", value: operation.Anchor, allowEmpty: true},
			{name: prefix + " method", value: operation.Method},
			{name: prefix + " path", value: operation.Path},
		} {
			if err := ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
				return err
			}
		}
		for parameterIndex, parameter := range operation.Parameters {
			parameterPrefix := fmt.Sprintf("%s parameter %d", prefix, parameterIndex)
			if err := ValidateCanonicalIdentity(parameterPrefix+" name", parameter.Name, false); err != nil {
				return err
			}
			if err := ValidateCanonicalIdentity(parameterPrefix+" location", parameter.In, false); err != nil {
				return err
			}
		}
		if err := validateOperationFacets(prefix, operation.Facets); err != nil {
			return err
		}
		if operation.RequestBody != nil {
			for mediaIndex, media := range operation.RequestBody.MediaTypes {
				if err := validateOperationMediaTypeIdentities(
					fmt.Sprintf("%s request media %d", prefix, mediaIndex),
					media,
				); err != nil {
					return err
				}
			}
		}
		for responseIndex, response := range operation.Responses {
			responsePrefix := fmt.Sprintf("%s response %d", prefix, responseIndex)
			if err := ValidateCanonicalIdentity(responsePrefix+" status", response.Status, false); err != nil {
				return err
			}
			for mediaIndex, media := range response.MediaTypes {
				if err := validateOperationMediaTypeIdentities(
					fmt.Sprintf("%s media %d", responsePrefix, mediaIndex),
					media,
				); err != nil {
					return err
				}
			}
		}
		for securityIndex, security := range operation.Security {
			securityPrefix := fmt.Sprintf("%s security %d", prefix, securityIndex)
			if err := ValidateCanonicalIdentity(securityPrefix+" name", security.Name, false); err != nil {
				return err
			}
			for scopeIndex, scope := range security.Scopes {
				if err := ValidateCanonicalIdentity(
					fmt.Sprintf("%s scope %d", securityPrefix, scopeIndex),
					scope,
					false,
				); err != nil {
					return err
				}
			}
		}
		for snippetIndex, snippet := range operation.Snippets {
			if err := ValidateCanonicalIdentity(
				fmt.Sprintf("%s snippet %d language", prefix, snippetIndex),
				snippet.Language,
				true,
			); err != nil {
				return err
			}
		}
	}
	for schemaIndex, schema := range index.Schemas {
		prefix := fmt.Sprintf("spec schema %d", schemaIndex)
		if err := ValidateCanonicalIdentity(prefix+" name", schema.Name, false); err != nil {
			return err
		}
	}
	for searchIndex, document := range index.Search {
		prefix := fmt.Sprintf("spec search document %d", searchIndex)
		for _, identity := range []struct {
			name       string
			value      string
			allowEmpty bool
		}{
			{name: prefix + " id", value: document.ID},
			{name: prefix + " href", value: document.Href},
			{name: prefix + " kind", value: document.Kind},
			{name: prefix + " method", value: document.Method, allowEmpty: true},
			{name: prefix + " path", value: document.Path, allowEmpty: true},
			{name: prefix + " section", value: document.Section, allowEmpty: true},
		} {
			if err := ValidateCanonicalIdentity(identity.name, identity.value, identity.allowEmpty); err != nil {
				return err
			}
		}
	}
	for routeIndex, route := range index.PublicRoutes {
		if err := ValidateCanonicalIdentity(
			fmt.Sprintf("spec public route %d path", routeIndex),
			route.Path,
			false,
		); err != nil {
			return err
		}
	}
	if err := validateSpecIndexSurfaceUniqueness(index); err != nil {
		return err
	}
	return nil
}

func validateOperationFacets(prefix string, facets []Facet) error {
	for index, facet := range facets {
		if err := ValidateCanonicalIdentity(fmt.Sprintf("%s facet %d name", prefix, index), facet.Name, false); err != nil {
			return err
		}
		if err := ValidateCanonicalIdentity(fmt.Sprintf("%s facet %d value", prefix, index), facet.Value, false); err != nil {
			return err
		}
		if index == 0 {
			continue
		}
		previous := facets[index-1]
		if previous.Name > facet.Name || previous.Name == facet.Name && previous.Value >= facet.Value {
			return fmt.Errorf("%s facets must be sorted and unique", prefix)
		}
	}
	return nil
}

func validateOperationMediaTypeIdentities(prefix string, media OperationMediaType) error {
	return ValidateCanonicalIdentity(prefix+" content type", media.ContentType, false)
}

type specSchemaSummaryValidator struct {
	active         map[*SchemaSummary]struct{}
	memoHeight     map[*SchemaSummary]int
	nodes          int
	resourceLimits bool
}

func validateSpecIndexSchemaSummaries(index SpecIndex, resourceLimits bool) error {
	validator := specSchemaSummaryValidator{
		active:         make(map[*SchemaSummary]struct{}),
		memoHeight:     make(map[*SchemaSummary]int),
		resourceLimits: resourceLimits,
	}
	for operationIndex, operation := range index.Operations {
		prefix := fmt.Sprintf("spec operation %d", operationIndex)
		for parameterIndex, parameter := range operation.Parameters {
			if err := validator.validateRoot(
				fmt.Sprintf("%s parameter %d schema", prefix, parameterIndex),
				parameter.Schema,
			); err != nil {
				return err
			}
		}
		if operation.RequestBody != nil {
			for mediaIndex, media := range operation.RequestBody.MediaTypes {
				if err := validator.validateRoot(
					fmt.Sprintf("%s request media %d schema", prefix, mediaIndex),
					media.Schema,
				); err != nil {
					return err
				}
			}
		}
		for responseIndex, response := range operation.Responses {
			for mediaIndex, media := range response.MediaTypes {
				if err := validator.validateRoot(
					fmt.Sprintf("%s response %d media %d schema", prefix, responseIndex, mediaIndex),
					media.Schema,
				); err != nil {
					return err
				}
			}
		}
	}
	for schemaIndex, schema := range index.Schemas {
		if err := validator.validateRoot(
			fmt.Sprintf("spec schema %d summary", schemaIndex),
			schema.Summary,
		); err != nil {
			return err
		}
	}
	return nil
}

func (v *specSchemaSummaryValidator) validateRoot(prefix string, schema SchemaSummary) error {
	_, err := v.validateValue(prefix, schema, 1)
	return err
}

func (v *specSchemaSummaryValidator) validateValue(
	prefix string,
	schema SchemaSummary,
	depth int,
) (int, error) {
	if depth > maxSpecSchemaSummaryDepth {
		return 0, fmt.Errorf(
			"%s exceeds maximum schema summary depth %d",
			prefix,
			maxSpecSchemaSummaryDepth,
		)
	}
	v.nodes++
	if v.resourceLimits && v.nodes > maxSpecSchemaSummaryNodes {
		return 0, fmt.Errorf(
			"%s exceeds maximum schema summary nodes %d",
			prefix,
			maxSpecSchemaSummaryNodes,
		)
	}
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: prefix + " name", value: schema.Name},
		{name: prefix + " type", value: schema.Type},
		{name: prefix + " format", value: schema.Format},
	} {
		if err := ValidateCanonicalIdentity(identity.name, identity.value, true); err != nil {
			return 0, err
		}
	}
	height := 1
	for propertyIndex, property := range schema.Properties {
		propertyPrefix := fmt.Sprintf("%s property %d", prefix, propertyIndex)
		if err := ValidateCanonicalIdentity(propertyPrefix+" name", property.Name, false); err != nil {
			return 0, err
		}
		propertyHeight, err := v.validateValue(propertyPrefix+" schema", property.Schema, depth+1)
		if err != nil {
			return 0, err
		}
		if 1+propertyHeight > height {
			height = 1 + propertyHeight
		}
	}
	if schema.Items != nil {
		itemsHeight, err := v.validatePointer(prefix+" items", schema.Items, depth+1)
		if err != nil {
			return 0, err
		}
		if 1+itemsHeight > height {
			height = 1 + itemsHeight
		}
	}
	return height, nil
}

func (v *specSchemaSummaryValidator) validatePointer(
	prefix string,
	schema *SchemaSummary,
	depth int,
) (int, error) {
	if _, ok := v.active[schema]; ok {
		return 0, fmt.Errorf("%s contains a schema summary cycle", prefix)
	}
	if height, ok := v.memoHeight[schema]; ok {
		if depth+height-1 > maxSpecSchemaSummaryDepth {
			return 0, fmt.Errorf(
				"%s exceeds maximum schema summary depth %d",
				prefix,
				maxSpecSchemaSummaryDepth,
			)
		}
		return height, nil
	}
	v.active[schema] = struct{}{}
	height, err := v.validateValue(prefix, *schema, depth)
	delete(v.active, schema)
	if err != nil {
		return 0, err
	}
	v.memoHeight[schema] = height
	return height, nil
}

func validateSpecIndexSurfaceUniqueness(index SpecIndex) error {
	type operationKey struct {
		method string
		path   string
	}
	operations := make(map[operationKey]struct{}, len(index.Operations))
	for operationIndex, operation := range index.Operations {
		key := operationKey{
			method: canonicalUpperSurfaceText(operation.Method),
			path:   strings.TrimSpace(operation.Path),
		}
		if _, ok := operations[key]; ok {
			return fmt.Errorf(
				"spec operation %d duplicates canonical operation %s %s",
				operationIndex,
				key.method,
				key.path,
			)
		}
		operations[key] = struct{}{}

		type parameterKey struct {
			name     string
			location string
		}
		parameters := make(map[parameterKey]struct{}, len(operation.Parameters))
		for parameterIndex, parameter := range operation.Parameters {
			key := parameterKey{
				name:     strings.TrimSpace(parameter.Name),
				location: canonicalLowerSurfaceText(parameter.In),
			}
			if _, ok := parameters[key]; ok {
				return fmt.Errorf(
					"spec operation %d parameter %d duplicates canonical parameter %s in %s",
					operationIndex,
					parameterIndex,
					key.name,
					key.location,
				)
			}
			parameters[key] = struct{}{}
		}

		statuses := make(map[string]struct{}, len(operation.Responses))
		for responseIndex, response := range operation.Responses {
			status := strings.TrimSpace(response.Status)
			if _, ok := statuses[status]; ok {
				return fmt.Errorf(
					"spec operation %d response %d duplicates status %s",
					operationIndex,
					responseIndex,
					status,
				)
			}
			statuses[status] = struct{}{}
		}
	}

	schemas := make(map[string]struct{}, len(index.Schemas))
	for schemaIndex, schema := range index.Schemas {
		name := strings.TrimSpace(schema.Name)
		if _, ok := schemas[name]; ok {
			return fmt.Errorf("spec schema %d duplicates schema %s", schemaIndex, name)
		}
		schemas[name] = struct{}{}
	}
	return nil
}
