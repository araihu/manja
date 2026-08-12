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

var errInvalidOperationRequestBodyFragment = errors.New("local docs operation request-body fragment is invalid")

// OperationRequestBodyFragment holds one admitted request-body section. Its
// render data is copied from already verified projection/domain child
// fragments, so rendering never needs a parser or mutable domain input.
type OperationRequestBodyFragment struct {
	data  operationRequestBodyData
	valid bool
}

type operationRequestBodyData struct {
	Description string
	Required    bool
	Media       []operationRequestBodyItemData
}

type operationRequestBodyItemData struct {
	ContentType string
	Summary     operationRequestBodyMediaData
	Tree        operationSchemaTreeData
}

func PrepareOperationRequestBody(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	media OperationRequestBodyMediaFragment,
	trees OperationSchemaTreesFragment,
) (OperationRequestBodyFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) ||
		!detail.Operation.HasRequestBody || operation.RequestBody == nil || !media.valid || !trees.valid {
		return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("operation request body")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || operation.Anchor != projected.Anchor {
		return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("operation identity")
	}
	parentBinding, err := bindOperationPreparationParent(detail, operation)
	if err != nil || media.binding != trees.binding || media.binding.parent != parentBinding {
		return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("preparation context")
	}
	projectedBody := projected.RequestBody
	preparedBody := operation.RequestBody
	if !utf8.ValidString(projectedBody.Description) || projectedBody.Description != preparedBody.Description || projectedBody.Required != preparedBody.Required ||
		len(projectedBody.MediaTypes) != len(preparedBody.MediaTypes) || len(media.media) != len(projectedBody.MediaTypes) || len(trees.request) != len(projectedBody.MediaTypes) {
		return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("request body")
	}

	fragment := OperationRequestBodyFragment{data: operationRequestBodyData{
		Description: projectedBody.Description,
		Required:    projectedBody.Required,
		Media:       make([]operationRequestBodyItemData, 0, len(projectedBody.MediaTypes)),
	}, valid: true}
	mediaIDs := make(map[string]struct{}, len(projectedBody.MediaTypes))
	for index, projectedMedia := range projectedBody.MediaTypes {
		preparedMedia := preparedBody.MediaTypes[index]
		preparedSummary := media.media[index]
		preparedTree := trees.request[index]
		_, duplicate := mediaIDs[projectedMedia.ID]
		if projectedMedia.Ordinal != uint32(index) || projectedMedia.ID != projectedMedia.ContentType || duplicate ||
			domain.ValidateCanonicalIdentity("media content type", projectedMedia.ContentType, false) != nil ||
			projectedMedia.ContentType != preparedMedia.ContentType || !operationRequestBodyMediaExampleMatches(projectedMedia.Examples, preparedMedia) {
			return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("media identity")
		}
		mediaIDs[projectedMedia.ID] = struct{}{}
		if preparedSummary.ContentType != projectedMedia.ContentType || preparedSummary.SchemaLabel != parameterSchemaInline(preparedMedia.Schema) {
			return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("media summary")
		}
		wantTreeID := operationSchemaTreeMediaID(operation.Anchor+"-request-body", projectedMedia.ContentType)
		if preparedTree.ID != wantTreeID || preparedTree.Caption != "Request body schema for "+projectedMedia.ContentType ||
			preparedTree.LinkTarget != "#catalog-main-content" || preparedTree.LinkSelect != "#catalog-main-content" || preparedTree.LinkSwap != "outerHTML show:#main-content:top" {
			return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("schema tree")
		}
		fragment.data.Media = append(fragment.data.Media, operationRequestBodyItemData{
			ContentType: projectedMedia.ContentType,
			Summary:     preparedSummary,
			Tree:        cloneOperationSchemaTreeData(preparedTree),
		})
	}

	var output boundedBuffer
	if err := operationRequestBody(fragment.data).Render(context.Background(), &output); err != nil {
		return OperationRequestBodyFragment{}, invalidOperationRequestBodyField("rendered bytes")
	}
	return fragment, nil
}

func cloneOperationSchemaTreeData(source operationSchemaTreeData) operationSchemaTreeData {
	clone := source
	clone.Root = cloneOperationSchemaTreeNodeData(source.Root)
	return clone
}

func cloneOperationSchemaTreeNodeData(source operationSchemaTreeNodeData) operationSchemaTreeNodeData {
	clone := source
	clone.EnumValues = append([]string(nil), source.EnumValues...)
	clone.Constraints = append([]schemaConstraintData(nil), source.Constraints...)
	clone.Properties = make([]operationSchemaTreePropertyData, len(source.Properties))
	for index, property := range source.Properties {
		clone.Properties[index] = operationSchemaTreePropertyData{Name: property.Name, Required: property.Required, Schema: cloneOperationSchemaTreeNodeData(property.Schema)}
	}
	if source.Items != nil {
		items := cloneOperationSchemaTreeNodeData(*source.Items)
		clone.Items = &items
	}
	return clone
}

func OperationRequestBody(fragment OperationRequestBodyFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidOperationRequestBodyFragment
		}
		var output boundedBuffer
		if err := operationRequestBody(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationRequestBodyFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationRequestBody(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationRequestBodyField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationRequestBodyFragment, strings.TrimSpace(name))
}
