package projectionjson

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/araihu/manja/application/projection"
)

const (
	maxSchemaGraphNodes          = 100_000
	maxExpandedSchemaOccurrences = 100_000
	maxSchemaGraphEdges          = 100_000
)

var schemaNodeDomain = []byte("manja.projection.schema-node.v2\x00")

func validateDocument(document projection.Document) error {
	if document.FormatVersion != 2 {
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
	if err := validateSchemaGraph(document); err != nil {
		return err
	}
	if err := validateTopLevelRecords(document); err != nil {
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
	return validateRecordSemantics(document)
}

func validateSchemaGraph(document projection.Document) error {
	if len(document.SchemaNodes) > maxSchemaGraphNodes {
		return codecFailure("schemaNodes", "node_budget")
	}
	edges := 0
	for _, node := range document.SchemaNodes {
		edges += len(node.Properties) + len(node.Items)
		if edges > maxSchemaGraphEdges {
			return codecFailure("schemaNodes", "edge_budget")
		}
	}

	preimages := make([][]byte, len(document.SchemaNodes))
	seenIDs := make(map[string]int, len(document.SchemaNodes))
	previousID := ""
	for index := range document.SchemaNodes {
		node := &document.SchemaNodes[index]
		if node.Ordinal != uint32(index) {
			return codecFailure("schemaNodes", "ordinal_mismatch")
		}
		if _, err := schemaDigestFromID(node.ID); err != nil {
			return err
		}
		if len(node.Items) > 1 {
			return codecFailure("schemaNodes.items", "invalid_cardinality")
		}
		propertyIDs := make(map[string]struct{}, len(node.Properties))
		for propertyIndex, property := range node.Properties {
			if property.Ordinal != uint32(propertyIndex) || property.ID != property.Name {
				return codecFailure("schemaNodes.properties", "non_canonical")
			}
			if _, duplicate := propertyIDs[property.ID]; duplicate {
				return codecFailure("schemaNodes.properties", "duplicate_property")
			}
			propertyIDs[property.ID] = struct{}{}
			if int(property.SchemaRef) >= len(document.SchemaNodes) {
				return codecFailure("schemaNodes.properties.schemaRef", "schema_ref")
			}
		}
		for itemIndex, item := range node.Items {
			if itemIndex != 0 || item.Ordinal != 0 || item.ID != "items" {
				return codecFailure("schemaNodes.items", "non_canonical")
			}
			if int(item.SchemaRef) >= len(document.SchemaNodes) {
				return codecFailure("schemaNodes.items.schemaRef", "schema_ref")
			}
		}
		if err := validateEmbeddedString(node.JSON); err != nil {
			return err
		}
		preimage, err := schemaNodePreimage(document.SchemaNodes, *node)
		if err != nil {
			return err
		}
		preimages[index] = preimage
		if previousIndex, duplicate := seenIDs[node.ID]; duplicate {
			if !bytes.Equal(preimages[previousIndex], preimage) {
				return codecFailure("schemaNodes", "hash_collision")
			}
			return codecFailure("schemaNodes", "duplicate")
		}
		seenIDs[node.ID] = index
		if index > 0 && node.ID < previousID {
			return codecFailure("schemaNodes", "unsorted")
		}
		previousID = node.ID
	}

	roots, err := schemaRoots(document)
	if err != nil {
		return err
	}
	if err := rejectSchemaCycles(document.SchemaNodes); err != nil {
		return err
	}
	reachable := make([]bool, len(document.SchemaNodes))
	type occurrence struct {
		ref   projection.SchemaRef
		depth int
	}
	stack := make([]occurrence, 0, len(roots))
	for _, root := range roots {
		stack = append(stack, occurrence{ref: root, depth: 1})
	}
	expanded := 0
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		expanded++
		if expanded > maxExpandedSchemaOccurrences {
			return codecFailure("schemaNodes", "expanded_budget")
		}
		if current.depth > 64 {
			return codecFailure("schemaNodes", "depth")
		}
		reachable[current.ref] = true
		node := document.SchemaNodes[current.ref]
		for _, property := range node.Properties {
			stack = append(stack, occurrence{ref: property.SchemaRef, depth: current.depth + 1})
		}
		for _, item := range node.Items {
			stack = append(stack, occurrence{ref: item.SchemaRef, depth: current.depth + 1})
		}
	}
	for _, reached := range reachable {
		if !reached {
			return codecFailure("schemaNodes", "orphan")
		}
	}
	for index, node := range document.SchemaNodes {
		digest := sha256.Sum256(preimages[index])
		if node.ID != "schema-node-"+hex.EncodeToString(digest[:]) {
			return codecFailure("schemaNodes", "hash_mismatch")
		}
	}
	return nil
}

func schemaRoots(document projection.Document) ([]projection.SchemaRef, error) {
	roots := make([]projection.SchemaRef, 0)
	appendRoot := func(ref projection.SchemaRef) error {
		if int(ref) >= len(document.SchemaNodes) {
			return codecFailure("schemaRef", "schema_ref")
		}
		roots = append(roots, ref)
		return nil
	}
	for _, operation := range document.OperationDetails {
		for _, parameter := range operation.Parameters {
			if err := appendRoot(parameter.SchemaRef); err != nil {
				return nil, err
			}
		}
		for _, media := range operation.RequestBody.MediaTypes {
			if err := appendRoot(media.SchemaRef); err != nil {
				return nil, err
			}
		}
		for _, response := range operation.Responses {
			for _, media := range response.MediaTypes {
				if err := appendRoot(media.SchemaRef); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, detail := range document.SchemaDetails {
		if err := appendRoot(detail.SchemaRef); err != nil {
			return nil, err
		}
	}
	return roots, nil
}

func rejectSchemaCycles(nodes []projection.SchemaNode) error {
	type frame struct {
		ref      projection.SchemaRef
		children []projection.SchemaRef
		next     int
	}
	colors := make([]uint8, len(nodes))
	for start := range nodes {
		if colors[start] != 0 {
			continue
		}
		stack := []frame{{ref: projection.SchemaRef(start), children: schemaChildren(nodes[start])}}
		colors[start] = 1
		for len(stack) > 0 {
			current := &stack[len(stack)-1]
			if current.next == len(current.children) {
				colors[current.ref] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			child := current.children[current.next]
			current.next++
			switch colors[child] {
			case 1:
				return codecFailure("schemaNodes", "cycle")
			case 0:
				colors[child] = 1
				stack = append(stack, frame{ref: child, children: schemaChildren(nodes[child])})
			}
		}
	}
	return nil
}

func schemaChildren(node projection.SchemaNode) []projection.SchemaRef {
	children := make([]projection.SchemaRef, 0, len(node.Properties)+len(node.Items))
	for _, property := range node.Properties {
		children = append(children, property.SchemaRef)
	}
	for _, item := range node.Items {
		children = append(children, item.SchemaRef)
	}
	return children
}

func schemaNodePreimage(nodes []projection.SchemaNode, node projection.SchemaNode) ([]byte, error) {
	preimage := append([]byte(nil), schemaNodeDomain...)
	for _, value := range []string{node.Name, node.Type, node.Format, node.Description, node.DefaultValue, node.ExampleText, node.JSON} {
		preimage = appendSchemaString(preimage, value)
	}
	preimage = appendSchemaUint64(preimage, uint64(len(node.Properties)))
	for _, property := range node.Properties {
		preimage = appendSchemaUint32(preimage, property.Ordinal)
		preimage = appendSchemaString(preimage, property.ID)
		preimage = appendSchemaString(preimage, property.Name)
		if property.Required {
			preimage = append(preimage, 1)
		} else {
			preimage = append(preimage, 0)
		}
		preimage = appendSchemaString(preimage, property.Description)
		digest, err := schemaDigestFromID(nodes[property.SchemaRef].ID)
		if err != nil {
			return nil, err
		}
		preimage = append(preimage, digest[:]...)
	}
	preimage = appendSchemaUint64(preimage, uint64(len(node.Items)))
	for _, item := range node.Items {
		preimage = appendSchemaUint32(preimage, item.Ordinal)
		preimage = appendSchemaString(preimage, item.ID)
		digest, err := schemaDigestFromID(nodes[item.SchemaRef].ID)
		if err != nil {
			return nil, err
		}
		preimage = append(preimage, digest[:]...)
	}
	return preimage, nil
}

func schemaDigestFromID(id string) ([32]byte, error) {
	var digest [32]byte
	const prefix = "schema-node-"
	if !strings.HasPrefix(id, prefix) || len(id) != len(prefix)+64 || id[len(prefix):] != strings.ToLower(id[len(prefix):]) {
		return digest, codecFailure("schemaNodes.id", "non_canonical")
	}
	decoded, err := hex.DecodeString(id[len(prefix):])
	if err != nil || len(decoded) != len(digest) {
		return digest, codecFailure("schemaNodes.id", "non_canonical")
	}
	copy(digest[:], decoded)
	return digest, nil
}

func appendSchemaString(target []byte, value string) []byte {
	target = appendSchemaUint64(target, uint64(len([]byte(value))))
	return append(target, value...)
}

func appendSchemaUint32(target []byte, value uint32) []byte {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], value)
	return append(target, encoded[:]...)
}

func appendSchemaUint64(target []byte, value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return append(target, encoded[:]...)
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
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil ||
		reference.Path != "" || reference.Fragment == "" || strings.Contains(value, "\\") ||
		strings.TrimSpace(value) != value || reference.Query().Get("selected") != reference.Fragment ||
		len(reference.Query()) != 1 || value != selectedHref(reference.Fragment) {
		return "", codecFailure("href", "invalid_source")
	}
	return reference.Fragment, nil
}
