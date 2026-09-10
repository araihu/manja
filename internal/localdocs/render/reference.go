package render

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// OperationReference composes documentation and examples without duplicating examples
// inside response accordions. All input fragments retain their original state.
func OperationReference(sections *OperationDetailSectionsFragment, examples OperationExamplesFragment, header templ.Component, requestGenerator ...templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !examples.valid {
			return errInvalidOperationExamplesFragment
		}
		var content *OperationDetailSectionsFragment
		if sections != nil {
			if !sections.valid || sections.binding.parent != examples.parent {
				return errInvalidOperationDetailSectionsFragment
			}
			clone := *sections
			if sections.data.Responses != nil {
				responses := cloneOperationResponsesFragment(*sections.data.Responses)
				for i := range responses.data.Responses {
					for j := range responses.data.Responses[i].Media {
						responses.data.Responses[i].Media[j].Example.visible = false
					}
				}
				clone.data.Responses = &responses
			}
			content = &clone
		}
		var responses []operationResponseExampleData
		for i, media := range examples.responses {
			responses = append(responses, media...)
			if len(media) == 0 && sections != nil && sections.data.Responses != nil && i < len(sections.data.Responses.data.Responses) {
				responses = append(responses, operationResponseExampleData{Status: sections.data.Responses.data.Responses[i].Status})
			}
		}
		var output boundedBuffer
		var request templ.Component
		if len(requestGenerator) > 0 {
			request = requestGenerator[0]
		}
		if err := operationReference(content, examples.codeSamples, responses, header, request).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}
