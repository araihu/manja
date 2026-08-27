package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationDetailSectionsFragment = errors.New("local docs operation detail-sections fragment is invalid")

// OperationDetailSectionsFragment owns the bounded request/response layout
// below an already-prepared operation header. Child fragments are copied at
// admission, so rendering never reads mutable projection or domain input.
type OperationDetailSectionsFragment struct {
	data    operationDetailSectionsData
	binding operationPreparationBinding
	valid   bool
}

type operationDetailSectionsData struct {
	Class     string
	Request   *OperationRequestSectionFragment
	Responses *OperationResponsesFragment
}

// PrepareOperationDetailSections binds the operation detail layout to one
// immutable operation parent and its prepared request/response children.
// Endpoint header, navigation, request composer, and rail remain outside this
// fragment.
func PrepareOperationDetailSections(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	request *OperationRequestSectionFragment,
	responses *OperationResponsesFragment,
) (OperationDetailSectionsFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("operation detail")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || operation.Anchor != id {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("operation identity")
	}
	if projected.HasRequestBody != (operation.RequestBody != nil) || len(projected.Security) != len(operation.Security) || len(projected.Parameters) != len(operation.Parameters) || len(projected.Responses) != len(operation.Responses) {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("operation inventory")
	}
	parent, err := bindOperationPreparationParent(detail, operation)
	if err != nil {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("preparation context")
	}
	wantRequest := len(operation.Security) > 0 || len(operation.Parameters) > 0 || operation.RequestBody != nil
	if wantRequest != (request != nil) {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("request declaration")
	}
	if request != nil && (!request.valid || request.binding.parent != parent) {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("request context")
	}
	wantResponses := len(operation.Responses) > 0
	if wantResponses != (responses != nil) {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("response declaration")
	}
	if responses != nil && (!responses.valid || responses.binding.parent != parent) {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("response context")
	}
	if request != nil && responses != nil && request.binding != responses.binding {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("mixed preparation contexts")
	}
	if !wantRequest && !wantResponses {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("empty detail sections")
	}

	data := operationDetailSectionsData{
		Class: operationDetailSectionsClass(wantRequest, wantResponses),
	}
	if request != nil {
		clone := cloneOperationRequestSectionFragment(*request)
		data.Request = &clone
	}
	if responses != nil {
		clone := cloneOperationResponsesFragment(*responses)
		data.Responses = &clone
	}
	var binding operationPreparationBinding
	if request != nil {
		binding = request.binding
	} else {
		binding = responses.binding
	}
	fragment := OperationDetailSectionsFragment{data: data, binding: binding, valid: true}
	var output boundedBuffer
	if err := operationDetailSections(fragment.data).Render(context.Background(), &output); err != nil {
		return OperationDetailSectionsFragment{}, invalidOperationDetailSectionsField("rendered bytes")
	}
	return fragment, nil
}

func operationDetailSectionsClass(hasRequest, hasResponses bool) string {
	class := "manja-endpoint-detail-layout"
	if !hasRequest || !hasResponses {
		class += " manja-endpoint-detail-layout-single"
	}
	return class
}

// OperationDetailSections renders the admitted request/response layout.
func OperationDetailSections(fragment OperationDetailSectionsFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidOperationDetailSectionsFragment
		}
		var output boundedBuffer
		if err := operationDetailSections(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationDetailSectionsFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationDetailSections(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationDetailSectionsField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationDetailSectionsFragment, strings.TrimSpace(name))
}
