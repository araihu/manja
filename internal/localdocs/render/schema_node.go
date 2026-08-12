package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

const (
	maximumSchemaNodeEdges   = 100
	maximumHTMLFragmentBytes = 2 << 20
)

var errInvalidSchemaNodeFragment = errors.New("local docs schema-node fragment is invalid")

type SchemaNodeFragment struct {
	data  schemaNodeData
	valid bool
}

type schemaNodeData struct {
	Ordinal      uint32
	Name         string
	RootHeading  string
	Type         string
	Format       string
	Description  string
	DefaultValue string
	ExampleText  string
	EnumValues   []string
	Constraints  []schemaConstraintData
	Nullable     bool
	Deprecated   bool
	Edges        []schemaEdgeData
	Truncated    bool
}

type schemaConstraintData struct {
	Name  string
	Value string
}

type schemaEdgeData struct {
	Name                 string
	Type                 string
	Reference            string
	Description          string
	DefaultValue         string
	ExampleText          string
	EnumValues           []string
	Constraints          []schemaConstraintData
	Nullable             bool
	Deprecated           bool
	Required             bool
	ReferenceHref        string
	ReferenceIsEnumAlias bool
}

func PrepareSchemaNode(detail catalog.DetailRecordV1, node projection.SchemaNode, references []projection.SchemaNode, documentHref string) (SchemaNodeFragment, error) {
	if !validDocumentHref(documentHref) {
		return SchemaNodeFragment{}, invalidField("document href")
	}
	if err := validateSchemaDetail(detail, documentHref); err != nil {
		return SchemaNodeFragment{}, err
	}
	if err := validateSchemaNodeIdentity(node); err != nil {
		return SchemaNodeFragment{}, err
	}

	required := sortedReferenceOrdinals(node)
	if index := sort.Search(len(required), func(index int) bool { return required[index] >= projection.SchemaRef(node.Ordinal) }); index < len(required) && required[index] == projection.SchemaRef(node.Ordinal) {
		required = append(required[:index:index], required[index+1:]...)
	}
	if len(references) != len(required) {
		return SchemaNodeFragment{}, invalidField("reference coverage")
	}
	known := make(map[projection.SchemaRef]projection.SchemaNode, len(references)+1)
	known[projection.SchemaRef(node.Ordinal)] = cloneSchemaNode(node)
	knownIDs := map[string]struct{}{node.ID: {}}
	for _, reference := range references {
		ordinal := projection.SchemaRef(reference.Ordinal)
		if err := validateSchemaNodeIdentity(reference); err != nil {
			return SchemaNodeFragment{}, err
		}
		if _, exists := known[ordinal]; exists {
			return SchemaNodeFragment{}, invalidField("duplicate reference ordinal")
		}
		if _, exists := knownIDs[reference.ID]; exists {
			return SchemaNodeFragment{}, invalidField("duplicate schema node id")
		}
		index := sort.Search(len(required), func(index int) bool { return required[index] >= ordinal })
		if index == len(required) || required[index] != ordinal {
			return SchemaNodeFragment{}, invalidField("extra reference")
		}
		known[ordinal] = cloneSchemaNode(reference)
		knownIDs[reference.ID] = struct{}{}
	}
	for _, ordinal := range required {
		if _, exists := known[ordinal]; !exists {
			return SchemaNodeFragment{}, invalidField("missing reference")
		}
	}

	data := schemaNodeData{
		Ordinal: node.Ordinal, Name: firstNonEmpty(node.Name, "Schema node "+strconv.FormatUint(uint64(node.Ordinal), 10)),
		RootHeading: detail.Schema.Heading,
		Type:        node.Type, Format: node.Format, Description: schemaText(node.Description),
		DefaultValue: schemaText(node.DefaultValue), ExampleText: schemaText(node.ExampleText),
		EnumValues: append([]string(nil), node.Enum...), Constraints: schemaConstraints(node.Constraints),
		Nullable: node.Nullable, Deprecated: node.Deprecated,
	}
	appendEdge := func(name, description string, required bool, ref projection.SchemaRef) error {
		if len(data.Edges) == maximumSchemaNodeEdges {
			data.Truncated = true
			return nil
		}
		target, exists := known[ref]
		if !exists {
			return errInvalidSchemaNodeFragment
		}
		reference := ""
		referenceHref := ""
		referenceIsEnumAlias := schemaNodeIsNamedPrimitiveEnumAlias(target)
		if schemaNodeReferenceNavigable(target) {
			reference = strings.TrimSpace(target.Name)
			referenceHref = schemaNodeHref(documentHref, detail.ID, uint32(ref))
		}
		data.Edges = append(data.Edges, schemaEdgeData{
			Name: name, Description: schemaText(description), Required: required,
			Type: schemaNodeShapeType(target), Reference: reference, ReferenceHref: referenceHref, ReferenceIsEnumAlias: referenceIsEnumAlias,
			DefaultValue: schemaText(target.DefaultValue), ExampleText: schemaText(target.ExampleText),
			EnumValues: append([]string(nil), target.Enum...), Constraints: schemaConstraints(target.Constraints),
			Nullable: target.Nullable, Deprecated: target.Deprecated,
		})
		return nil
	}
	for _, item := range node.Items {
		if err := appendEdge("items", "", false, item.SchemaRef); err != nil {
			return SchemaNodeFragment{}, err
		}
	}
	for _, property := range node.Properties {
		if err := appendEdge(property.Name, property.Description, property.Required, property.SchemaRef); err != nil {
			return SchemaNodeFragment{}, err
		}
	}
	return SchemaNodeFragment{data: data, valid: true}, nil
}

func validateSchemaDetail(detail catalog.DetailRecordV1, documentHref string) error {
	if detail.Kind != "schema" || detail.Schema == nil || detail.Operation != nil || !validDetailID(detail.ID) {
		return invalidField("schema detail")
	}
	id := string(detail.ID)
	if detail.Schema.ID != id || detail.Schema.Anchor != id || detail.Schema.HeadingID != id {
		return invalidField("schema detail identity")
	}
	documentIndex := strings.LastIndex(documentHref, "/documents/")
	if documentIndex < 0 {
		return invalidField("document href route")
	}
	wantHref := documentHref[documentIndex+1:] + "?selected=" + id + "#" + id
	if detail.Schema.Href != wantHref {
		return invalidField("schema detail href")
	}
	return nil
}

func validateSchemaNodeIdentity(node projection.SchemaNode) error {
	if domain.ValidateCanonicalIdentity("schema node id", node.ID, false) != nil {
		return invalidField("schema node id")
	}
	return nil
}

func validDetailID(value domain.DetailID) bool {
	const prefix = "detail-sha256-"
	text := string(value)
	if len(text) != len(prefix)+64 || !strings.HasPrefix(text, prefix) {
		return false
	}
	for _, character := range text[len(prefix):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func (fragment SchemaNodeFragment) Type() string { return fragment.data.Type }

func (fragment SchemaNodeFragment) Format() string { return fragment.data.Format }

func SchemaNode(fragment SchemaNodeFragment) templ.Component { return fragment }

func (fragment SchemaNodeFragment) Render(ctx context.Context, writer io.Writer) error {
	if !fragment.valid {
		return errInvalidSchemaNodeFragment
	}
	var output boundedBuffer
	if err := schemaNode(fragment.data).Render(ctx, &output); err != nil {
		return err
	}
	_, err := writer.Write(output.Bytes())
	return err
}

func (fragment SchemaNodeFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := fragment.Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

type boundedBuffer struct {
	bytes.Buffer
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	if len(data) > maximumHTMLFragmentBytes-buffer.Len() {
		return 0, errors.New("local docs HTML fragment exceeds byte limit")
	}
	return buffer.Buffer.Write(data)
}

var _ io.Writer = (*boundedBuffer)(nil)
var _ templ.Component = SchemaNodeFragment{}

func validDocumentHref(value string) bool {
	if value == "" || len(value) > 1024 || !strings.HasPrefix(value, "/") || !strings.HasSuffix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, `\`) || strings.ContainsAny(value, "%?#") || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host != "" || parsed.Scheme != "" {
		return false
	}
	trimmed := strings.TrimSuffix(value, "/")
	if trimmed == "" || path.Clean(trimmed) != trimmed || strings.Contains(trimmed, "//") {
		return false
	}
	documentIndex := strings.LastIndex(value, "/documents/")
	if documentIndex < 0 {
		return false
	}
	documentKey := strings.TrimSuffix(value[documentIndex+len("/documents/"):], "/")
	return !strings.Contains(documentKey, "/") && domain.ValidateCatalogDocumentKey(documentKey) == nil
}

func schemaNodeHref(documentHref string, detailID domain.DetailID, ordinal uint32) string {
	return documentHref + "?selected=" + url.QueryEscape(string(detailID)) + "&node=" + strconv.FormatUint(uint64(ordinal), 10) + "#schema-node-panel"
}

func schemaConstraints(source []projection.SchemaConstraint) []schemaConstraintData {
	result := make([]schemaConstraintData, 0, len(source))
	for _, constraint := range source {
		result = append(result, schemaConstraintData{Name: constraint.Name, Value: constraint.Value})
	}
	return result
}

func schemaNodeShapeType(node projection.SchemaNode) string {
	parts := make([]string, 0, 2)
	if node.Type != "" {
		parts = append(parts, node.Type)
	}
	if node.Format != "" {
		parts = append(parts, "("+node.Format+")")
	}
	return strings.Join(parts, " ")
}

func schemaNodeReferenceNavigable(node projection.SchemaNode) bool {
	if strings.TrimSpace(node.Name) == "" {
		return false
	}
	return schemaNodeIsNamedPrimitiveEnumAlias(node) || !schemaNodeIsPrimitiveType(node)
}

func schemaNodeIsPrimitiveType(node projection.SchemaNode) bool {
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "string", "number", "integer", "boolean", "null":
		return true
	}
	return false
}

func schemaNodeIsNamedPrimitiveEnumAlias(node projection.SchemaNode) bool {
	return strings.TrimSpace(node.Name) != "" && schemaNodeIsPrimitiveType(node) && len(node.Enum) > 0
}

func schemaText(value string) string {
	const maximumRunes = 512
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maximumRunes {
		return string(runes)
	}
	return string(runes[:maximumRunes]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cloneSchemaNode(node projection.SchemaNode) projection.SchemaNode {
	clone := node
	clone.Enum = append([]string(nil), node.Enum...)
	clone.Constraints = append([]projection.SchemaConstraint(nil), node.Constraints...)
	clone.Properties = append([]projection.SchemaNodeProperty(nil), node.Properties...)
	clone.Items = append([]projection.SchemaNodeItem(nil), node.Items...)
	return clone
}

func sortedReferenceOrdinals(node projection.SchemaNode) []projection.SchemaRef {
	set := make(map[projection.SchemaRef]struct{})
	for _, item := range node.Items {
		set[item.SchemaRef] = struct{}{}
	}
	for _, property := range node.Properties {
		set[property.SchemaRef] = struct{}{}
	}
	result := make([]projection.SchemaRef, 0, len(set))
	for ref := range set {
		result = append(result, ref)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func invalidField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidSchemaNodeFragment, name)
}
