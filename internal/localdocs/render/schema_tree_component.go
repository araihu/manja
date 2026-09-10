package render

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
	"github.com/araihu/goshtoso/components/schematree"
	"github.com/araihu/manja/domain"
)

// SchemaTreeFromSummary adapts the legacy server-rendered schema model to the
// same component used by verified offline fragments. No parsing or fetching is
// performed here; callers retain responsibility for bounded schema traversal.
func SchemaTreeFromSummary(id, caption string, schema domain.SchemaSummary, links map[string]string, target, selectTarget, swap string) templ.Component {
	tree := operationSchemaTreeData{ID: id, Caption: caption, Root: schemaTreeSummaryNode(schema, links), LinkTarget: target, LinkSelect: selectTarget, LinkSwap: swap}
	return schematree.SchemaTree(operationSharedSchemaTree(tree))
}

func schemaTreeSummaryNode(schema domain.SchemaSummary, links map[string]string) operationSchemaTreeNodeData {
	node := operationSchemaTreeNodeData{Name: schema.Name, SchemaName: schema.Name, Type: schema.Type, Format: schema.Format,
		Description: schema.Description, DefaultValue: schema.Default, ExampleText: schema.Example, EnumValues: schema.Enum,
		Nullable: schema.Nullable, Deprecated: schema.Deprecated, ReferenceHref: links[strings.TrimSpace(schema.Name)],
	}
	for _, c := range schema.Constraints {
		node.Constraints = append(node.Constraints, schemaConstraintData{Name: c.Name, Value: c.Value})
	}
	for _, p := range schema.Properties {
		child := schemaTreeSummaryNode(p.Schema, links)
		child.Name = operationSchemaDisplayName(p.Name, child.SchemaName)
		node.Properties = append(node.Properties, operationSchemaTreePropertyData{Name: child.Name, Required: p.Required, Schema: child})
	}
	if schema.Items != nil {
		child := schemaTreeSummaryNode(*schema.Items, links)
		child.Name = "items"
		node.Items = &child
	}
	node.Expandable = len(node.Properties) > 0 || node.Items != nil && node.Items.Expandable
	node.EnumAlias = operationSchemaTreeIsNamedPrimitiveEnumAlias(node)
	finishOperationSchemaTreeNode(&node)
	return node
}

// ResponseHeaderSchemaTree renders header schemas through the shared tree.
func ResponseHeaderSchemaTree(headers []domain.OperationResponseHeader, scope string, links map[string]string) templ.Component {
	prepared := make([]operationResponseHeaderData, 0, len(headers))
	for _, h := range headers {
		prepared = append(prepared, operationResponseHeaderData{Name: h.Name, Description: h.Description, Example: h.Example, Schema: h.Schema})
	}
	return schematree.SchemaTree(sharedResponseHeaderTree(prepared, scope, links))
}

func schemaDescriptionContent(description, scope string) templ.Component {
	if strings.TrimSpace(description) == "" {
		return nil
	}
	return Description(description, "schema-"+scope)
}

func operationSharedSchemaTree(tree operationSchemaTreeData) schematree.Config {
	cfg := schematree.Config{ID: tree.ID, AriaLabel: tree.Caption + " schema tree", RootAttrs: templ.Attributes{"data-manja-schema-tree": "true"}}
	if tree.Root.Expandable {
		cfg.Nodes = operationSharedSchemaChildren(tree.Root, tree.ID, tree)
	} else {
		node := tree.Root
		node.Name = operationSchemaTreeRootName(tree.Caption, node.SchemaName)
		cfg.Nodes = []schematree.Node{operationSharedSchemaNode(node, tree.ID, "", true, tree)}
	}
	return cfg
}

func operationSharedSchemaChildren(node operationSchemaTreeNodeData, scope string, tree operationSchemaTreeData) []schematree.Node {
	children := make([]schematree.Node, 0, len(node.Properties)+1)
	if node.Items != nil && node.Items.Expandable {
		children = append(children, operationSharedSchemaNode(*node.Items, scope, "", false, tree))
	}
	for _, property := range node.Properties {
		children = append(children, operationSharedSchemaNode(property.Schema, scope, schemaPropertyStateLabel(property.Required), false, tree))
	}
	return children
}

func operationSharedSchemaNode(node operationSchemaTreeNodeData, scope, state string, root bool, tree operationSchemaTreeData) schematree.Node {
	rowScope := scope + "-" + anchorFragment(node.Name)
	attrs := templ.Attributes{"data-schema-tree-row": node.Name}
	if root {
		attrs = templ.Attributes{"data-schema-tree-node": node.Name}
	}
	result := schematree.Node{
		Name: node.Name, Type: node.Inline, DescriptionContent: schemaDescriptionContent(node.Description, rowScope),
		Required: state == "required", Nullable: node.Nullable, Deprecated: node.Deprecated,
		RootAttrs: attrs, Constraints: sharedSchemaConstraints(node.DefaultValue, node.ExampleText, node.EnumValues, node.Constraints),
	}
	if node.EnumAlias {
		result.TypeContent = operationSchemaTreeEnumAliasType(node, rowScope, tree)
	}
	if node.Expandable {
		result.Children = operationSharedSchemaChildren(node, rowScope, tree)
	}
	return result
}

func sharedSchemaConstraints(defaultValue, example string, enums []string, constraints []schemaConstraintData) []schematree.Constraint {
	result := make([]schematree.Constraint, 0, len(constraints)+3)
	if defaultValue != "" {
		result = append(result, schematree.Constraint{Name: "Default", Value: defaultValue})
	}
	if example != "" {
		result = append(result, schematree.Constraint{Name: "Example", Value: example})
	}
	if len(enums) > 0 {
		result = append(result, schematree.Constraint{Name: "Allowed", Value: strings.Join(enums, ", ")})
	}
	for _, c := range constraints {
		result = append(result, schematree.Constraint{Name: c.Name, Value: c.Value})
	}
	return result
}

func sharedSchemaNodeEdges(node schemaNodeData) schematree.Config {
	cfg := schematree.Config{AriaLabel: node.Name + " properties"}
	for _, edge := range node.Edges {
		item := schematree.Node{
			Name: edge.Name, TypeContent: sharedSchemaEdgeType(node.Ordinal, edge),
			Required: edge.Required, Nullable: edge.Nullable, Deprecated: edge.Deprecated,
			Constraints: sharedSchemaConstraints(edge.DefaultValue, edge.ExampleText, edge.EnumValues, edge.Constraints),
			RootAttrs:   templ.Attributes{"data-catalog-schema-property": edge.Name},
		}
		if schemaEdgeHasDescription(edge) {
			item.DescriptionContent = schemaDescriptionContent(edge.Description, fmt.Sprintf("node-%d-property-%s", node.Ordinal, edge.Name))
		}
		cfg.Nodes = append(cfg.Nodes, item)
	}
	return cfg
}

func sharedResponseHeaderTree(headers []operationResponseHeaderData, scope string, links map[string]string) schematree.Config {
	cfg := schematree.Config{AriaLabel: "Response header schemas", RootAttrs: templ.Attributes{"data-manja-schema-tree": "true"}}
	for _, header := range headers {
		cfg.Nodes = append(cfg.Nodes, sharedResponseSchemaNode(header.Name, "", operationResponseDetailHeaderSummary(header), scope, links))
	}
	return cfg
}

func sharedResponseSchemaNode(name, state string, schema domain.SchemaSummary, scope string, links map[string]string) schematree.Node {
	name = operationResponseDetailSchemaTreeDisplayName(name, schema)
	scope += "-" + anchorFragment(name)
	constraints := make([]schemaConstraintData, 0, len(schema.Constraints))
	for _, c := range schema.Constraints {
		constraints = append(constraints, schemaConstraintData{Name: c.Name, Value: c.Value})
	}
	node := schematree.Node{
		Name: name, Type: parameterSchemaInline(schema), DescriptionContent: schemaDescriptionContent(schema.Description, scope),
		Required: state == "required", Nullable: schema.Nullable, Deprecated: schema.Deprecated,
		RootAttrs:   templ.Attributes{"data-schema-tree-row": name},
		Constraints: sharedSchemaConstraints(schema.Default, schema.Example, schema.Enum, constraints),
	}
	if responseDetailSchemaIsNamedPrimitiveEnumAlias(schema) {
		node.TypeContent = operationResponseDetailSchemaEnumAliasType(name, schema, scope, links)
	}
	if schema.Items != nil && operationResponseDetailSchemaTreeExpandable(*schema.Items) {
		node.Children = append(node.Children, sharedResponseSchemaNode("items", "", *schema.Items, scope, links))
	}
	for _, property := range schema.Properties {
		node.Children = append(node.Children, sharedResponseSchemaNode(property.Name, schemaPropertyStateLabel(property.Required), property.Schema, scope, links))
	}
	return node
}
