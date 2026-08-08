package projection

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"sort"

	"github.com/araihu/manja/domain"
)

const maxSchemaGraphNodes = 100_000
const maxSchemaGraphEdges = 100_000

var schemaNodeDomain = []byte("manja.projection.schema-node.v3\x00")

type schemaHashFunc func([]byte) [32]byte

type schemaNodeDraft struct {
	digest   [32]byte
	preimage []byte
	node     SchemaNode
}

type schemaGraphBuilder struct {
	hasher   schemaHashFunc
	drafts   []schemaNodeDraft
	byDigest map[[32]byte]SchemaRef
	edges    int
}

func newSchemaGraphBuilder(hasher schemaHashFunc) *schemaGraphBuilder {
	return &schemaGraphBuilder{
		hasher:   hasher,
		drafts:   []schemaNodeDraft{},
		byDigest: make(map[[32]byte]SchemaRef),
	}
}

func (s *buildState) internSchema(source domain.SchemaSummary) (SchemaRef, error) {
	if err := s.checkpoint(); err != nil {
		return 0, err
	}
	canonicalJSON := ""
	var err error
	if source.JSON != "" {
		canonicalJSON, err = canonicalEmbeddedJSON(source.JSON)
		if err != nil {
			return 0, err
		}
	}

	properties := make([]SchemaNodeProperty, 0, len(source.Properties))
	propertyIDs := make(map[string]struct{}, len(source.Properties))
	for sourceIndex, property := range source.Properties {
		if _, duplicate := propertyIDs[property.Name]; duplicate {
			return 0, projectionFailure("schema.properties", "duplicate_record")
		}
		propertyIDs[property.Name] = struct{}{}
		childRef, err := s.internSchema(property.Schema)
		if err != nil {
			return 0, err
		}
		properties = append(properties, SchemaNodeProperty{
			Ordinal: uint32(sourceIndex), ID: property.Name, Name: property.Name,
			Required: property.Required, Description: property.Description, SchemaRef: childRef,
		})
	}

	items := []SchemaNodeItem{}
	if source.Items != nil {
		childRef, err := s.internSchema(*source.Items)
		if err != nil {
			return 0, err
		}
		items = append(items, SchemaNodeItem{Ordinal: 0, ID: "items", SchemaRef: childRef})
	}

	constraints := make([]SchemaConstraint, 0, len(source.Constraints))
	for _, constraint := range source.Constraints {
		constraints = append(constraints, SchemaConstraint{Name: constraint.Name, Value: constraint.Value})
	}
	node := SchemaNode{
		Name: source.Name, Type: source.Type, Format: source.Format, Description: source.Description,
		DefaultValue: source.Default, ExampleText: source.Example,
		Enum: append([]string{}, source.Enum...), Constraints: constraints,
		Nullable: source.Nullable, Deprecated: source.Deprecated, JSON: canonicalJSON,
		Properties: properties, Items: items,
	}
	preimage := s.schemaNodePreimage(node)
	digest := s.schemaGraph.hasher(preimage)
	if existingRef, exists := s.schemaGraph.byDigest[digest]; exists {
		existing := s.schemaGraph.drafts[existingRef]
		if !bytes.Equal(existing.preimage, preimage) {
			return 0, projectionFailure("schemaNodes", "hash_collision")
		}
		return existingRef, nil
	}
	if len(s.schemaGraph.drafts) >= maxSchemaGraphNodes {
		return 0, projectionFailure("schemaNodes", "node_budget")
	}
	if s.schemaGraph.edges+len(properties)+len(items) > maxSchemaGraphEdges {
		return 0, projectionFailure("schemaNodes", "edge_budget")
	}
	ref := SchemaRef(len(s.schemaGraph.drafts))
	node.ID = "schema-node-" + hex.EncodeToString(digest[:])
	s.schemaGraph.drafts = append(s.schemaGraph.drafts, schemaNodeDraft{
		digest: digest, preimage: append([]byte(nil), preimage...), node: node,
	})
	s.schemaGraph.byDigest[digest] = ref
	s.schemaGraph.edges += len(properties) + len(items)
	return ref, nil
}

func (s *buildState) schemaNodePreimage(node SchemaNode) []byte {
	preimage := append([]byte(nil), schemaNodeDomain...)
	for _, value := range []string{
		node.Name, node.Type, node.Format, node.Description,
		node.DefaultValue, node.ExampleText, node.JSON,
	} {
		preimage = appendSchemaString(preimage, value)
	}
	preimage = appendSchemaStrings(preimage, node.Enum)
	preimage = appendSchemaUint64(preimage, uint64(len(node.Constraints)))
	for _, constraint := range node.Constraints {
		preimage = appendSchemaString(preimage, constraint.Name)
		preimage = appendSchemaString(preimage, constraint.Value)
	}
	preimage = appendSchemaBool(preimage, node.Nullable)
	preimage = appendSchemaBool(preimage, node.Deprecated)
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
		preimage = append(preimage, s.schemaGraph.drafts[property.SchemaRef].digest[:]...)
	}
	preimage = appendSchemaUint64(preimage, uint64(len(node.Items)))
	for _, item := range node.Items {
		preimage = appendSchemaUint32(preimage, item.Ordinal)
		preimage = appendSchemaString(preimage, item.ID)
		preimage = append(preimage, s.schemaGraph.drafts[item.SchemaRef].digest[:]...)
	}
	return preimage
}

func appendSchemaString(target []byte, value string) []byte {
	target = appendSchemaUint64(target, uint64(len([]byte(value))))
	return append(target, value...)
}

func appendSchemaStrings(target []byte, values []string) []byte {
	target = appendSchemaUint64(target, uint64(len(values)))
	for _, value := range values {
		target = appendSchemaString(target, value)
	}
	return target
}

func appendSchemaBool(target []byte, value bool) []byte {
	if value {
		return append(target, 1)
	}
	return append(target, 0)
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

func (s *buildState) finalizeSchemaGraph(document *Document) error {
	order := make([]int, len(s.schemaGraph.drafts))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(i, j int) bool {
		return s.schemaGraph.drafts[order[i]].node.ID < s.schemaGraph.drafts[order[j]].node.ID
	})
	remap := make([]SchemaRef, len(order))
	for ordinal, oldIndex := range order {
		remap[oldIndex] = SchemaRef(ordinal)
	}
	nodes := make([]SchemaNode, len(order))
	for ordinal, oldIndex := range order {
		node := s.schemaGraph.drafts[oldIndex].node
		node.Ordinal = uint32(ordinal)
		for index := range node.Properties {
			node.Properties[index].SchemaRef = remap[node.Properties[index].SchemaRef]
		}
		for index := range node.Items {
			node.Items[index].SchemaRef = remap[node.Items[index].SchemaRef]
		}
		nodes[ordinal] = node
	}
	document.SchemaNodes = nodes
	for operationIndex := range document.OperationDetails {
		operation := &document.OperationDetails[operationIndex]
		for index := range operation.Parameters {
			operation.Parameters[index].SchemaRef = remap[operation.Parameters[index].SchemaRef]
		}
		for index := range operation.RequestBody.MediaTypes {
			operation.RequestBody.MediaTypes[index].SchemaRef = remap[operation.RequestBody.MediaTypes[index].SchemaRef]
		}
		for responseIndex := range operation.Responses {
			for headerIndex := range operation.Responses[responseIndex].Headers {
				header := &operation.Responses[responseIndex].Headers[headerIndex]
				header.SchemaRef = remap[header.SchemaRef]
			}
			for mediaIndex := range operation.Responses[responseIndex].MediaTypes {
				media := &operation.Responses[responseIndex].MediaTypes[mediaIndex]
				media.SchemaRef = remap[media.SchemaRef]
			}
		}
	}
	for index := range document.SchemaDetails {
		document.SchemaDetails[index].SchemaRef = remap[document.SchemaDetails[index].SchemaRef]
	}
	return nil
}
