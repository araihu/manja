package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

const (
	maximumParameterSchemaDepth = 4
	maximumParameterSchemaNodes = 256
)

var errInvalidOperationParametersFragment = errors.New("local docs operation-parameters fragment is invalid")

type OperationParametersFragment struct {
	groups []operationParameterGroupData
	valid  bool
}

type operationParameterGroupData struct {
	ID         string
	Title      string
	Parameters []operationParameterData
}

type operationParameterData struct {
	ID          string
	Name        string
	TypeLabel   string
	Required    bool
	Description string
	Example     string
}

func PrepareOperationParameters(detail catalog.DetailRecordV1, operation domain.Operation, nodes []projection.SchemaNode) (OperationParametersFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationParametersFragment{}, invalidOperationParametersField("operation detail")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || operation.Anchor != projected.Anchor {
		return OperationParametersFragment{}, invalidOperationParametersField("operation detail identity")
	}
	if len(projected.Parameters) != len(operation.Parameters) {
		return OperationParametersFragment{}, invalidOperationParametersField("parameter inventory")
	}

	resolver, err := newOperationParameterSchemaResolver(nodes, len(projected.Parameters))
	if err != nil {
		return OperationParametersFragment{}, err
	}
	for index, parameter := range projected.Parameters {
		if err := validateProjectedParameter(index, parameter); err != nil {
			return OperationParametersFragment{}, err
		}
		schema, err := resolver.schema(parameter.SchemaRef, 0)
		if err != nil {
			return OperationParametersFragment{}, err
		}
		prepared := domain.OperationParameter{
			Name: parameter.Name, In: parameter.In, Required: parameter.Required,
			Description: parameter.Description, Schema: schema, Example: projectedParameterExample(parameter.Examples),
		}
		if !reflect.DeepEqual(prepared, operation.Parameters[index]) {
			return OperationParametersFragment{}, invalidOperationParametersField("prepared parameter")
		}
	}
	if len(resolver.used) != len(resolver.nodes) {
		return OperationParametersFragment{}, invalidOperationParametersField("schema-node inventory")
	}

	fragment := OperationParametersFragment{valid: true}
	for _, group := range []struct {
		location string
		title    string
	}{{"path", "Path Parameters"}, {"query", "Query Parameters"}, {"header", "Header Parameters"}} {
		data := operationParameterGroupData{ID: operation.Anchor + "-" + anchorFragment(group.title), Title: group.title}
		for _, parameter := range operation.Parameters {
			if !strings.EqualFold(parameter.In, group.location) {
				continue
			}
			typeLabel := parameterSchemaInline(parameter.Schema)
			if typeLabel == "" {
				typeLabel = parameter.In
			}
			data.Parameters = append(data.Parameters, operationParameterData{
				ID:   operation.Anchor + "-" + anchorFragment(group.title) + "-" + anchorFragment(parameter.In+"-"+parameter.Name),
				Name: parameter.Name, TypeLabel: typeLabel, Required: parameter.Required,
				Description: parameter.Description, Example: parameter.Example,
			})
		}
		fragment.groups = append(fragment.groups, data)
	}
	return fragment, nil
}

func validateProjectedParameter(index int, parameter projection.Parameter) error {
	if parameter.Ordinal != uint32(index) || parameter.ID != operationParameterRecordID(parameter.In, parameter.Name) {
		return invalidOperationParametersField("parameter identity")
	}
	if domain.ValidateCanonicalIdentity("parameter name", parameter.Name, false) != nil || domain.ValidateCanonicalIdentity("parameter location", parameter.In, false) != nil || !utf8.ValidString(parameter.Description) {
		return invalidOperationParametersField("parameter fields")
	}
	if len(parameter.Examples) > 1 {
		return invalidOperationParametersField("parameter examples")
	}
	if len(parameter.Examples) == 1 {
		example := parameter.Examples[0]
		if example.Ordinal != 0 || example.ID != "primary" || !example.Provided || example.Text == "" || !utf8.ValidString(example.Text) {
			return invalidOperationParametersField("parameter example")
		}
	}
	return nil
}

func operationParameterRecordID(location, name string) string {
	hash := sha256.New()
	hash.Write([]byte("parameter"))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range []string{strings.ToLower(location), name} {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		hash.Write(length[:])
		hash.Write([]byte(value))
	}
	return "parameter-" + hex.EncodeToString(hash.Sum(nil))
}

func projectedParameterExample(examples []projection.Example) string {
	if len(examples) == 0 {
		return ""
	}
	return examples[0].Text
}

type operationParameterSchemaResolver struct {
	nodes  map[projection.SchemaRef]projection.SchemaNode
	used   map[projection.SchemaRef]struct{}
	active map[projection.SchemaRef]bool
	loaded int
}

func newOperationParameterSchemaResolver(nodes []projection.SchemaNode, parameterCount int) (*operationParameterSchemaResolver, error) {
	if parameterCount < 0 || len(nodes) > maximumParameterSchemaNodes+parameterCount {
		return nil, invalidOperationParametersField("schema-node inventory")
	}
	resolver := &operationParameterSchemaResolver{
		nodes: make(map[projection.SchemaRef]projection.SchemaNode, len(nodes)), used: make(map[projection.SchemaRef]struct{}, len(nodes)), active: make(map[projection.SchemaRef]bool),
	}
	ids := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		ref := projection.SchemaRef(node.Ordinal)
		if _, duplicate := resolver.nodes[ref]; duplicate {
			return nil, invalidOperationParametersField("duplicate schema-node ordinal")
		}
		if domain.ValidateCanonicalIdentity("schema node id", node.ID, false) != nil {
			return nil, invalidOperationParametersField("schema-node identity")
		}
		if _, duplicate := ids[node.ID]; duplicate {
			return nil, invalidOperationParametersField("duplicate schema-node id")
		}
		if !validOperationParameterSchemaNode(node) {
			return nil, invalidOperationParametersField("schema-node fields")
		}
		ids[node.ID] = struct{}{}
		resolver.nodes[ref] = cloneSchemaNode(node)
	}
	return resolver, nil
}

func validOperationParameterSchemaNode(node projection.SchemaNode) bool {
	if len(node.Items) > 1 {
		return false
	}
	for _, value := range []string{node.Name, node.Type, node.Format, node.Description, node.DefaultValue, node.ExampleText, node.JSON} {
		if !utf8.ValidString(value) {
			return false
		}
	}
	for _, value := range node.Enum {
		if !utf8.ValidString(value) {
			return false
		}
	}
	for _, constraint := range node.Constraints {
		if !utf8.ValidString(constraint.Name) || !utf8.ValidString(constraint.Value) || constraint.Name == "" {
			return false
		}
	}
	propertyIDs := make(map[string]struct{}, len(node.Properties))
	for index, property := range node.Properties {
		if property.Ordinal != uint32(index) || domain.ValidateCanonicalIdentity("schema property id", property.ID, false) != nil || domain.ValidateCanonicalIdentity("schema property name", property.Name, false) != nil || !utf8.ValidString(property.Description) {
			return false
		}
		if _, duplicate := propertyIDs[property.ID]; duplicate {
			return false
		}
		propertyIDs[property.ID] = struct{}{}
	}
	itemIDs := make(map[string]struct{}, len(node.Items))
	for index, item := range node.Items {
		if item.Ordinal != uint32(index) || domain.ValidateCanonicalIdentity("schema item id", item.ID, false) != nil {
			return false
		}
		if _, duplicate := itemIDs[item.ID]; duplicate {
			return false
		}
		itemIDs[item.ID] = struct{}{}
	}
	return true
}

func (resolver *operationParameterSchemaResolver) schema(ref projection.SchemaRef, depth int) (domain.SchemaSummary, error) {
	node, exists := resolver.nodes[ref]
	if !exists {
		return domain.SchemaSummary{}, invalidOperationParametersField("missing schema node")
	}
	resolver.used[ref] = struct{}{}
	summary := domain.SchemaSummary{
		Name: node.Name, Type: node.Type, Format: node.Format, Description: node.Description,
		Default: node.DefaultValue, Example: node.ExampleText, Enum: append([]string(nil), node.Enum...),
		Constraints: operationParameterSchemaConstraints(node.Constraints), Nullable: node.Nullable, Deprecated: node.Deprecated, JSON: node.JSON,
	}
	if depth >= maximumParameterSchemaDepth || resolver.loaded >= maximumParameterSchemaNodes || resolver.active[ref] {
		return summary, nil
	}
	resolver.active[ref] = true
	resolver.loaded++
	defer delete(resolver.active, ref)
	for _, property := range node.Properties {
		if resolver.loaded >= maximumParameterSchemaNodes {
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
	if len(node.Items) > 0 && resolver.loaded < maximumParameterSchemaNodes {
		if len(node.Items) != 1 {
			return domain.SchemaSummary{}, invalidOperationParametersField("schema-node items")
		}
		item, err := resolver.schema(node.Items[0].SchemaRef, depth+1)
		if err != nil {
			return domain.SchemaSummary{}, err
		}
		summary.Items = &item
	}
	return summary, nil
}

func operationParameterSchemaConstraints(source []projection.SchemaConstraint) []domain.SchemaConstraint {
	result := make([]domain.SchemaConstraint, 0, len(source))
	for _, constraint := range source {
		result = append(result, domain.SchemaConstraint{Name: constraint.Name, Value: constraint.Value})
	}
	return result
}

func parameterSchemaInline(schema domain.SchemaSummary) string {
	typeLabel := schema.Type
	if strings.Contains(schema.Type, "array") {
		typeLabel = "array"
		if schema.Items != nil {
			if item := parameterSchemaInline(*schema.Items); item != "" {
				typeLabel += "[" + item + "]"
			}
		}
	} else if schema.Format != "" {
		if typeLabel != "" {
			typeLabel += "<" + schema.Format + ">"
		} else {
			typeLabel = schema.Format
		}
	}
	if strings.TrimSpace(schema.Name) != "" && parameterSchemaIsPrimitive(schema) && len(schema.Enum) > 0 {
		return typeLabel + "<" + strings.TrimSpace(schema.Name) + ">"
	}
	parts := make([]string, 0, 2)
	if schema.Name != "" {
		parts = append(parts, schema.Name)
	}
	if typeLabel != "" {
		parts = append(parts, typeLabel)
	}
	return strings.Join(parts, " ")
}

func parameterSchemaIsPrimitive(schema domain.SchemaSummary) bool {
	switch strings.ToLower(strings.TrimSpace(schema.Type)) {
	case "string", "number", "integer", "boolean", "null":
		return true
	}
	return false
}

func OperationParameters(fragment OperationParametersFragment) templ.Component { return fragment }

func (fragment OperationParametersFragment) Render(ctx context.Context, writer io.Writer) error {
	if !fragment.valid {
		return errInvalidOperationParametersFragment
	}
	var output boundedBuffer
	for index, group := range fragment.groups {
		if index > 0 {
			if _, err := output.Write([]byte(" ")); err != nil {
				return err
			}
		}
		if len(group.Parameters) == 0 {
			continue
		}
		if err := operationParameterGroup(group).Render(ctx, &output); err != nil {
			return err
		}
	}
	_, err := writer.Write(output.Bytes())
	return err
}

func (fragment OperationParametersFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := fragment.Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationParametersField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationParametersFragment, name)
}

var _ templ.Component = OperationParametersFragment{}
