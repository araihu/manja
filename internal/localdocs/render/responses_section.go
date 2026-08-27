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

var errInvalidOperationResponsesFragment = errors.New("local docs operation-responses fragment is invalid")

// OperationResponsesFragment holds one admitted response section assembled
// from copied OC-04I/J/K/L child data. Rendering needs no parser or mutable
// projection/domain input.
type OperationResponsesFragment struct {
	data    operationResponsesData
	binding operationPreparationBinding
	valid   bool
}

type operationResponsesData struct {
	OperationID string
	Responses   []operationResponseSectionData
	SchemaLinks map[string]string
}

type operationResponseSectionData struct {
	Status  string
	Title   string
	Details operationResponseDetailData
	Media   []operationResponseSectionMediaData
}

type operationResponseSectionMediaData struct {
	Summary operationResponseMediaData
	Tree    operationSchemaTreeData
	Example operationResponseExampleData
}

func PrepareOperationResponses(
	detail catalog.DetailRecordV1,
	operation domain.Operation,
	media OperationResponseMediaFragment,
	details OperationResponseDetailsFragment,
	examples OperationExamplesFragment,
	trees OperationSchemaTreesFragment,
) (OperationResponsesFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) || len(operation.Responses) == 0 ||
		!media.valid || !details.valid || !examples.valid || !trees.valid {
		return OperationResponsesFragment{}, invalidOperationResponsesField("operation responses")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || operation.Anchor != projected.Anchor || len(projected.Responses) != len(operation.Responses) ||
		len(media.media) != len(projected.Responses) || len(details.responses) != len(projected.Responses) || len(examples.responses) != len(projected.Responses) || len(trees.responses) != len(projected.Responses) {
		return OperationResponsesFragment{}, invalidOperationResponsesField("operation identity")
	}
	parent, err := bindOperationPreparationParent(detail, operation)
	if err != nil || media.binding != details.binding || media.binding != trees.binding || media.binding.parent != parent || examples.parent != parent {
		return OperationResponsesFragment{}, invalidOperationResponsesField("preparation context")
	}

	fragment := OperationResponsesFragment{data: operationResponsesData{
		OperationID: operation.Anchor,
		Responses:   make([]operationResponseSectionData, 0, len(projected.Responses)),
		SchemaLinks: cloneResponseDetailSchemaLinks(details.schemaLinks),
	}, binding: media.binding, valid: true}
	responseIDs := make(map[string]struct{}, len(projected.Responses))
	for responseIndex, response := range projected.Responses {
		prepared := operation.Responses[responseIndex]
		_, duplicate := responseIDs[response.ID]
		if response.Ordinal != uint32(responseIndex) || response.ID != response.Status || duplicate ||
			domain.ValidateCanonicalIdentity("response status", response.Status, false) != nil || response.Status != prepared.Status ||
			len(response.MediaTypes) != len(prepared.MediaTypes) || len(media.media[responseIndex]) != len(response.MediaTypes) ||
			len(examples.responses[responseIndex]) != len(response.MediaTypes) || len(trees.responses[responseIndex]) != len(response.MediaTypes) {
			return OperationResponsesFragment{}, invalidOperationResponsesField("response identity")
		}
		responseIDs[response.ID] = struct{}{}
		preparedDetails := details.responses[responseIndex]
		if preparedDetails.Description != prepared.Description || len(preparedDetails.Headers) != len(prepared.Headers) {
			return OperationResponsesFragment{}, invalidOperationResponsesField("response details")
		}
		section := operationResponseSectionData{
			Status: response.Status, Title: operationResponseTitle(response.Status),
			Details: cloneOperationResponseDetailData(preparedDetails),
			Media:   make([]operationResponseSectionMediaData, 0, len(response.MediaTypes)),
		}
		mediaIDs := make(map[string]struct{}, len(response.MediaTypes))
		for mediaIndex, projectedMedia := range response.MediaTypes {
			preparedMedia := prepared.MediaTypes[mediaIndex]
			preparedSummary := media.media[responseIndex][mediaIndex]
			preparedTree := trees.responses[responseIndex][mediaIndex]
			preparedExample := examples.responses[responseIndex][mediaIndex]
			_, duplicate := mediaIDs[projectedMedia.ID]
			if projectedMedia.Ordinal != uint32(mediaIndex) || projectedMedia.ID != projectedMedia.ContentType || duplicate ||
				domain.ValidateCanonicalIdentity("response media content type", projectedMedia.ContentType, false) != nil ||
				projectedMedia.ContentType != preparedMedia.ContentType || preparedSummary.ContentType != projectedMedia.ContentType {
				return OperationResponsesFragment{}, invalidOperationResponsesField("response media identity")
			}
			mediaIDs[projectedMedia.ID] = struct{}{}
			wantTreeID := operationSchemaTreeMediaID(operation.Anchor+"-response-"+anchorFragment(response.Status), projectedMedia.ContentType)
			if preparedTree.ID != wantTreeID || preparedTree.Caption != "Response body" ||
				preparedTree.LinkTarget != "#catalog-main-content" || preparedTree.LinkSelect != "#catalog-main-content" || preparedTree.LinkSwap != "outerHTML show:#main-content:top" {
				return OperationResponsesFragment{}, invalidOperationResponsesField("response schema tree")
			}
			wantExampleID := operation.Anchor + "-response-" + anchorFragment(response.Status) + "-" + anchorFragment(projectedMedia.ContentType) + "-example"
			if preparedExample.ID != wantExampleID || preparedExample.Status != response.Status || preparedExample.ContentType != projectedMedia.ContentType ||
				preparedExample.visible != (strings.TrimSpace(preparedMedia.Example) != "") {
				return OperationResponsesFragment{}, invalidOperationResponsesField("response example")
			}
			section.Media = append(section.Media, operationResponseSectionMediaData{
				Summary: preparedSummary,
				Tree:    cloneOperationSchemaTreeData(preparedTree),
				Example: cloneOperationResponseExampleData(preparedExample),
			})
		}
		fragment.data.Responses = append(fragment.data.Responses, section)
	}

	var output boundedBuffer
	if err := operationResponses(fragment.data).Render(context.Background(), &output); err != nil {
		return OperationResponsesFragment{}, invalidOperationResponsesField("rendered bytes")
	}
	return fragment, nil
}

func cloneOperationResponsesFragment(source OperationResponsesFragment) OperationResponsesFragment {
	clone := source
	clone.data.SchemaLinks = cloneResponseDetailSchemaLinks(source.data.SchemaLinks)
	clone.data.Responses = make([]operationResponseSectionData, len(source.data.Responses))
	for responseIndex, response := range source.data.Responses {
		clone.data.Responses[responseIndex] = operationResponseSectionData{
			Status:  response.Status,
			Title:   response.Title,
			Details: cloneOperationResponseDetailData(response.Details),
			Media:   make([]operationResponseSectionMediaData, len(response.Media)),
		}
		for mediaIndex, media := range response.Media {
			clone.data.Responses[responseIndex].Media[mediaIndex] = operationResponseSectionMediaData{
				Summary: media.Summary,
				Tree:    cloneOperationSchemaTreeData(media.Tree),
				Example: cloneOperationResponseExampleData(media.Example),
			}
		}
	}
	return clone
}

func operationResponseDetailsWithLegacySpacing(data operationResponseDetailData, scope string, schemaLinks map[string]string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if err := operationResponseDetailDescription(data.Description).Render(ctx, writer); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, " "); err != nil {
			return err
		}
		return operationResponseDetailHeaders(data.Headers, scope, schemaLinks).Render(ctx, writer)
	})
}

func cloneOperationResponseDetailData(source operationResponseDetailData) operationResponseDetailData {
	clone := operationResponseDetailData{Description: source.Description, Headers: make([]operationResponseHeaderData, len(source.Headers))}
	for index, header := range source.Headers {
		clone.Headers[index] = header
		clone.Headers[index].Schema = cloneResponseDetailSchema(header.Schema)
	}
	return clone
}

func cloneOperationResponseExampleData(source operationResponseExampleData) operationResponseExampleData {
	clone := source
	clone.Schema = append([]byte(nil), source.Schema...)
	return clone
}

func operationResponseTitle(status string) string {
	trimmed := strings.TrimSpace(status)
	if code := responseStatusCode(trimmed); code != 0 {
		if title := operationResponseStatusText(code); title != "" {
			return title
		}
	}
	if strings.EqualFold(trimmed, "default") {
		return "Default response"
	}
	return "Response"
}

// operationResponseStatusText mirrors net/http.StatusText without importing
// the server package into the Wasm-compatible renderer boundary.
func operationResponseStatusText(code int) string {
	switch code {
	case 100:
		return "Continue"
	case 101:
		return "Switching Protocols"
	case 102:
		return "Processing"
	case 103:
		return "Early Hints"
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 202:
		return "Accepted"
	case 203:
		return "Non-Authoritative Information"
	case 204:
		return "No Content"
	case 205:
		return "Reset Content"
	case 206:
		return "Partial Content"
	case 207:
		return "Multi-Status"
	case 208:
		return "Already Reported"
	case 226:
		return "IM Used"
	case 300:
		return "Multiple Choices"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 303:
		return "See Other"
	case 304:
		return "Not Modified"
	case 305:
		return "Use Proxy"
	case 307:
		return "Temporary Redirect"
	case 308:
		return "Permanent Redirect"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 402:
		return "Payment Required"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 406:
		return "Not Acceptable"
	case 407:
		return "Proxy Authentication Required"
	case 408:
		return "Request Timeout"
	case 409:
		return "Conflict"
	case 410:
		return "Gone"
	case 411:
		return "Length Required"
	case 412:
		return "Precondition Failed"
	case 413:
		return "Request Entity Too Large"
	case 414:
		return "Request URI Too Long"
	case 415:
		return "Unsupported Media Type"
	case 416:
		return "Requested Range Not Satisfiable"
	case 417:
		return "Expectation Failed"
	case 418:
		return "I'm a teapot"
	case 421:
		return "Misdirected Request"
	case 422:
		return "Unprocessable Entity"
	case 423:
		return "Locked"
	case 424:
		return "Failed Dependency"
	case 425:
		return "Too Early"
	case 426:
		return "Upgrade Required"
	case 428:
		return "Precondition Required"
	case 429:
		return "Too Many Requests"
	case 431:
		return "Request Header Fields Too Large"
	case 451:
		return "Unavailable For Legal Reasons"
	case 500:
		return "Internal Server Error"
	case 501:
		return "Not Implemented"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	case 504:
		return "Gateway Timeout"
	case 505:
		return "HTTP Version Not Supported"
	case 506:
		return "Variant Also Negotiates"
	case 507:
		return "Insufficient Storage"
	case 508:
		return "Loop Detected"
	case 510:
		return "Not Extended"
	case 511:
		return "Network Authentication Required"
	default:
		return ""
	}
}

func responseStatusCode(status string) int {
	if len(status) != 3 {
		return 0
	}
	code := 0
	for _, value := range status {
		if value < '0' || value > '9' {
			return 0
		}
		code = code*10 + int(value-'0')
	}
	return code
}

func OperationResponses(fragment OperationResponsesFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidOperationResponsesFragment
		}
		var output boundedBuffer
		if err := operationResponses(fragment.data).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationResponsesFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationResponses(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationResponsesField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationResponsesFragment, strings.TrimSpace(name))
}
