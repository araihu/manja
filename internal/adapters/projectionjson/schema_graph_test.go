package projectionjson

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestUnmarshalRejectsOutOfRangeSchemaRefs(t *testing.T) {
	document := mustBuild(t, fullFixture())
	document.SchemaDetails[0].SchemaRef = projection.SchemaRef(len(document.SchemaNodes))
	assertGraphDecodeFailure(t, document, "schema_ref")
}

func TestUnmarshalRejectsSchemaNodeDigestMismatch(t *testing.T) {
	document := mustBuild(t, fullFixture())
	document.SchemaNodes[0].Description += " changed"
	assertGraphDecodeFailure(t, document, "hash_mismatch")
}

func TestUnmarshalRejectsDuplicateAndOrphanSchemaNodes(t *testing.T) {
	t.Run("duplicate", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		document.SchemaNodes = append(document.SchemaNodes, document.SchemaNodes[0])
		document.SchemaNodes[len(document.SchemaNodes)-1].Ordinal = uint32(len(document.SchemaNodes) - 1)
		assertGraphDecodeFailure(t, document, "duplicate")
	})

	t.Run("unequal collision", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		collision := document.SchemaNodes[0]
		collision.Ordinal = uint32(len(document.SchemaNodes))
		collision.Description += " changed"
		document.SchemaNodes = append(document.SchemaNodes, collision)
		assertGraphDecodeFailure(t, document, "hash_collision")
	})

	t.Run("unsorted", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		document.SchemaNodes[0], document.SchemaNodes[1] = document.SchemaNodes[1], document.SchemaNodes[0]
		document.SchemaNodes[0].Ordinal = 0
		document.SchemaNodes[1].Ordinal = 1
		assertGraphDecodeFailure(t, document, "unsorted")
	})

	t.Run("ordinal mismatch", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		document.SchemaNodes[0].Ordinal = 1
		assertGraphDecodeFailure(t, document, "ordinal_mismatch")
	})

	t.Run("duplicate property", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		for index := range document.SchemaNodes {
			if len(document.SchemaNodes[index].Properties) == 0 {
				continue
			}
			property := document.SchemaNodes[index].Properties[0]
			property.Ordinal = uint32(len(document.SchemaNodes[index].Properties))
			document.SchemaNodes[index].Properties = append(document.SchemaNodes[index].Properties, property)
			assertGraphDecodeFailure(t, document, "duplicate_property")
			return
		}
		t.Fatal("fixture has no schema property")
	})

	t.Run("items cardinality", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		for index := range document.SchemaNodes {
			if len(document.SchemaNodes[index].Items) == 0 {
				continue
			}
			item := document.SchemaNodes[index].Items[0]
			item.Ordinal = 1
			document.SchemaNodes[index].Items = append(document.SchemaNodes[index].Items, item)
			assertGraphDecodeFailure(t, document, "invalid_cardinality")
			return
		}
		t.Fatal("fixture has no schema items")
	})

	t.Run("orphan", func(t *testing.T) {
		input := emptyFixture()
		input.Operations = []domain.Operation{{
			Anchor: "operation-orphan", Method: "GET", Path: "/orphan",
			Parameters: []domain.OperationParameter{
				{Name: "text", In: "query", Schema: domain.SchemaSummary{Type: "string"}},
				{Name: "count", In: "query", Schema: domain.SchemaSummary{Type: "integer"}},
			},
		}}
		document := mustBuild(t, input)
		document.OperationDetails[0].Parameters[1].SchemaRef = document.OperationDetails[0].Parameters[0].SchemaRef
		assertGraphDecodeFailure(t, document, "orphan")
	})
}

func TestUnmarshalRejectsSchemaCycles(t *testing.T) {
	input := emptyFixture()
	input.Operations = []domain.Operation{{
		Anchor: "operation-cycle", Method: "GET", Path: "/cycle",
		Parameters: []domain.OperationParameter{{Name: "value", In: "query", Schema: domain.SchemaSummary{Type: "array", Items: &domain.SchemaSummary{Type: "string"}}}},
	}}
	document := mustBuild(t, input)
	rootRef := document.OperationDetails[0].Parameters[0].SchemaRef
	document.SchemaNodes[rootRef].Items[0].SchemaRef = rootRef
	assertGraphDecodeFailure(t, document, "cycle")
}

func TestUnmarshalEnforcesExpandedSchemaDepthAndNodeBudget(t *testing.T) {
	t.Run("depth 64 accepted", func(t *testing.T) {
		input := emptyFixture()
		input.Schemas = []domain.Schema{{Name: "Deep", Summary: nestedDomainSchema(64)}}
		document := mustBuild(t, input)
		encoded := mustMarshal(t, document)
		if _, err := Unmarshal(encoded); err != nil {
			t.Fatalf("depth 64 rejected: %v", err)
		}
	})

	t.Run("depth 65 rejected", func(t *testing.T) {
		document := fakeDepthDocument(t, 65)
		assertGraphDecodeFailure(t, document, "depth")
	})

	t.Run("unique node budget", func(t *testing.T) {
		document := mustBuild(t, emptyFixture())
		document.SchemaNodes = make([]projection.SchemaNode, maxSchemaGraphNodes+1)
		if err := validateSchemaGraph(document); err == nil || !strings.Contains(err.Error(), "node_budget") {
			t.Fatalf("unique-node budget error = %v", err)
		}
	})

	t.Run("expanded occurrence budget", func(t *testing.T) {
		input := emptyFixture()
		input.Operations = []domain.Operation{{
			Anchor: "operation-expanded", Method: "GET", Path: "/expanded",
			Parameters: []domain.OperationParameter{{Name: "root", In: "query", Schema: domain.SchemaSummary{Type: "string"}}},
		}}
		document := mustBuild(t, input)
		root := document.OperationDetails[0].Parameters[0]
		parameters := make([]projection.Parameter, maxExpandedSchemaOccurrences+1)
		for index := range parameters {
			parameter := root
			parameter.Ordinal = uint32(index)
			parameter.ID = fmt.Sprintf("parameter-%06d", index)
			parameters[index] = parameter
		}
		document.OperationDetails[0].Parameters = parameters
		encoded := marshalUnchecked(t, document)
		if len(encoded) > maxProjectionBytes {
			t.Fatalf("expanded-budget fixture is %d bytes, exceeds decoder preflight", len(encoded))
		}
		decoded, err := Unmarshal(encoded)
		if err == nil || decoded.FormatVersion != 0 || !strings.Contains(err.Error(), "expanded_budget") {
			t.Fatalf("expanded budget decode = %#v, %v", decoded, err)
		}
	})

	t.Run("edge budget", func(t *testing.T) {
		document := mustBuild(t, emptyFixture())
		document.SchemaNodes = []projection.SchemaNode{{
			Ordinal: 0, ID: "schema-node-" + strings.Repeat("0", 64),
			Properties: make([]projection.SchemaNodeProperty, maxSchemaGraphEdges+1), Items: []projection.SchemaNodeItem{},
		}}
		if err := validateSchemaGraph(document); err == nil || !strings.Contains(err.Error(), "edge_budget") {
			t.Fatalf("edge budget error = %v", err)
		}
	})
}

func TestUnmarshalRejectsSupersededV1(t *testing.T) {
	versionTwo := mustMarshal(t, mustBuild(t, emptyFixture()))
	versionOne := bytes.Replace(versionTwo, []byte(`"formatVersion":2`), []byte(`"formatVersion":1`), 1)
	document, err := Unmarshal(versionOne)
	if err == nil || document.FormatVersion != 0 || !strings.Contains(err.Error(), "formatVersion") {
		t.Fatalf("version 1 decode = %#v, %v", document, err)
	}
}

func assertGraphDecodeFailure(t *testing.T, document projection.Document, class string) {
	t.Helper()
	decoded, err := Unmarshal(marshalUnchecked(t, document))
	if err == nil || decoded.FormatVersion != 0 || !strings.Contains(err.Error(), class) {
		t.Fatalf("decode = %#v, %v; want %s rejection", decoded, err, class)
	}
}

func nestedDomainSchema(depth int) domain.SchemaSummary {
	current := domain.SchemaSummary{Type: "string"}
	for index := 1; index < depth; index++ {
		child := current
		current = domain.SchemaSummary{Type: "array", Items: &child}
	}
	return current
}

func fakeDepthDocument(t *testing.T, depth int) projection.Document {
	t.Helper()
	document := mustBuild(t, emptyFixture())
	document.SchemaNodes = make([]projection.SchemaNode, depth)
	for index := range document.SchemaNodes {
		node := projection.SchemaNode{
			Ordinal: uint32(index), ID: fmt.Sprintf("schema-node-%064x", index),
			Enum: []string{}, Constraints: []projection.SchemaConstraint{},
			Properties: []projection.SchemaNodeProperty{}, Items: []projection.SchemaNodeItem{},
		}
		if index+1 < depth {
			node.Items = append(node.Items, projection.SchemaNodeItem{Ordinal: 0, ID: "items", SchemaRef: projection.SchemaRef(index + 1)})
		}
		document.SchemaNodes[index] = node
	}
	document.OperationDetails = []projection.OperationDetail{{
		Ordinal: 0, ID: "operation-depth", Anchor: "operation-depth", Href: "?selected=operation-depth#operation-depth",
		HeadingID: "operation-depth", HeadingLevel: 3,
		Tags: []projection.TextRecord{}, Parameters: []projection.Parameter{{
			Ordinal: 0, ID: "parameter-depth", SchemaRef: 0, Examples: []projection.Example{},
		}},
		RequestBody: projection.RequestBody{MediaTypes: []projection.MediaType{}},
		Responses:   []projection.Response{}, Security: []projection.SecurityRequirement{}, CodeSamples: []projection.CodeSample{},
	}}
	return document
}
