package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationRequestSectionFragment = errors.New("local docs operation-request section fragment is invalid")

// OperationRequestSectionFragment holds one admitted operation request
// landmark. Child fragments are copied before rendering, so this seam never
// reads mutable projection or domain values after preparation.
type OperationRequestSectionFragment struct {
	data    operationRequestSectionData
	binding operationPreparationBinding
	valid   bool
}

type operationRequestSectionData struct {
	Authorization OperationAuthorizationFragment
	Parameters    OperationParametersFragment
	Body          *OperationRequestBodyFragment
}

// PrepareOperationRequestSection binds the request landmark to one immutable
// operation parent and its already-prepared authorization, parameter, and
// request-body children. Endpoint shell, request composer, and response
// rendering remain outside this fragment.
func PrepareOperationRequestSection(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	authorization OperationAuthorizationFragment,
	parameters OperationParametersFragment,
	body *OperationRequestBodyFragment,
	documentHref string,
	schemaLinks map[string]string,
) (OperationRequestSectionFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("operation detail")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || operation.Anchor != id {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("operation identity")
	}
	if !authorization.valid || !parameters.valid {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("request child fragment")
	}
	if !validDocumentHref(documentHref) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("document href")
	}
	binding, err := bindOperationPreparation(detail, operation, documentHref, schemaLinks)
	if err != nil || authorization.binding.parent != binding.parent || parameters.binding.parent != binding.parent {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("preparation context")
	}

	wantBody := projected.HasRequestBody
	if wantBody != (operation.RequestBody != nil) || wantBody != (body != nil) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("request body declaration")
	}
	if len(projected.Security) != len(operation.Security) || len(projected.Parameters) != len(operation.Parameters) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("request inventory")
	}
	if len(projected.Security) == 0 && len(projected.Parameters) == 0 && !wantBody {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("empty request")
	}
	if body != nil && (!body.valid || body.binding != binding) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("request-body context")
	}
	if !validRequestSectionChildren(authorization, parameters) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("request child strings")
	}
	if body != nil && !validOperationRequestBodyData(body.data) {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("request-body strings")
	}

	fragment := OperationRequestSectionFragment{
		data: operationRequestSectionData{
			Authorization: cloneOperationAuthorizationFragment(authorization),
			Parameters:    cloneOperationParametersFragment(parameters),
		},
		binding: binding,
		valid:   true,
	}
	if body != nil {
		clone := cloneOperationRequestBodyFragment(*body)
		fragment.data.Body = &clone
	}
	var output boundedBuffer
	if err := operationRequestSection(fragment.data).Render(context.Background(), &output); err != nil {
		return OperationRequestSectionFragment{}, invalidOperationRequestSectionField("rendered bytes")
	}
	return fragment, nil
}

func validOperationRequestBodyData(data operationRequestBodyData) bool {
	if !utf8.ValidString(data.Description) {
		return false
	}
	for _, media := range data.Media {
		for _, value := range []string{media.ContentType, media.Summary.ContentType, media.Summary.SchemaLabel, media.Summary.SchemaHref} {
			if !utf8.ValidString(value) {
				return false
			}
		}
		if !validOperationSchemaTreeStrings(media.Tree) {
			return false
		}
	}
	return true
}

func validOperationSchemaTreeStrings(tree operationSchemaTreeData) bool {
	for _, value := range []string{tree.ID, tree.Caption, tree.Root.Name, tree.Root.SchemaName, tree.Root.Type, tree.Root.Format, tree.Root.Inline, tree.Root.InlineType, tree.Root.Description, tree.Root.DefaultValue, tree.Root.ExampleText, tree.LinkTarget, tree.LinkSelect, tree.LinkSwap} {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return validOperationSchemaTreeNodeStrings(tree.Root)
}

func validOperationSchemaTreeNodeStrings(node operationSchemaTreeNodeData) bool {
	for _, value := range []string{node.Name, node.SchemaName, node.Type, node.Format, node.Inline, node.InlineType, node.Description, node.DefaultValue, node.ExampleText, node.ReferenceHref} {
		if !utf8.ValidString(value) {
			return false
		}
	}
	for _, value := range node.EnumValues {
		if !utf8.ValidString(value) {
			return false
		}
	}
	for _, constraint := range node.Constraints {
		if !utf8.ValidString(constraint.Name) || !utf8.ValidString(constraint.Value) {
			return false
		}
	}
	for _, property := range node.Properties {
		if !utf8.ValidString(property.Name) || !validOperationSchemaTreeNodeStrings(property.Schema) {
			return false
		}
	}
	return node.Items == nil || validOperationSchemaTreeNodeStrings(*node.Items)
}

func validRequestSectionChildren(authorization OperationAuthorizationFragment, parameters OperationParametersFragment) bool {
	for _, requirement := range authorization.requirements {
		values := []string{requirement.Name, requirement.TypeLabel, requirement.Instruction, requirement.Description, requirement.ParameterLabel, requirement.ParameterName, requirement.Scheme, requirement.BearerFormat, requirement.OpenIDConnectURL}
		values = append(values, requirement.Scopes...)
		for _, value := range values {
			if !utf8.ValidString(value) {
				return false
			}
		}
	}
	for _, group := range parameters.groups {
		if !utf8.ValidString(group.ID) || !utf8.ValidString(group.Title) {
			return false
		}
		for _, parameter := range group.Parameters {
			for _, value := range []string{parameter.ID, parameter.Name, parameter.TypeLabel, parameter.Description, parameter.Example} {
				if !utf8.ValidString(value) {
					return false
				}
			}
		}
	}
	return true
}

func cloneOperationAuthorizationFragment(source OperationAuthorizationFragment) OperationAuthorizationFragment {
	clone := source
	clone.requirements = make([]operationAuthorizationData, len(source.requirements))
	for index, requirement := range source.requirements {
		clone.requirements[index] = requirement
		clone.requirements[index].Scopes = append([]string(nil), requirement.Scopes...)
	}
	return clone
}

func cloneOperationParametersFragment(source OperationParametersFragment) OperationParametersFragment {
	clone := source
	clone.groups = make([]operationParameterGroupData, len(source.groups))
	for index, group := range source.groups {
		clone.groups[index] = group
		clone.groups[index].Parameters = append([]operationParameterData(nil), group.Parameters...)
	}
	return clone
}

func cloneOperationRequestBodyFragment(source OperationRequestBodyFragment) OperationRequestBodyFragment {
	clone := source
	clone.data.Media = make([]operationRequestBodyItemData, len(source.data.Media))
	for index, media := range source.data.Media {
		clone.data.Media[index] = media
		clone.data.Media[index].Tree = cloneOperationSchemaTreeData(media.Tree)
	}
	return clone
}

func cloneOperationRequestSectionFragment(source OperationRequestSectionFragment) OperationRequestSectionFragment {
	clone := source
	clone.data.Authorization = cloneOperationAuthorizationFragment(source.data.Authorization)
	clone.data.Parameters = cloneOperationParametersFragment(source.data.Parameters)
	if source.data.Body != nil {
		body := cloneOperationRequestBodyFragment(*source.data.Body)
		clone.data.Body = &body
	}
	return clone
}

// OperationRequestSection renders the admitted request landmark.
func OperationRequestSection(fragment OperationRequestSectionFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidOperationRequestSectionFragment
		}
		var output boundedBuffer
		if err := operationRequestSection(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationRequestSectionFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationRequestSection(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationRequestSectionField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationRequestSectionFragment, strings.TrimSpace(name))
}
