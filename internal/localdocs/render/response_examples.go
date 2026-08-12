package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationExamplesFragment = errors.New("local docs operation examples fragment is invalid")

const maximumOperationExampleRecords = 4096

// OperationExamplesFragment holds admitted response examples and operation code
// samples. It owns copied display bytes only; OpenAPI parsing, request
// composition, browser activation, and persistence stay outside this fragment.
type OperationExamplesFragment struct {
	responses   [][]operationResponseExampleData
	codeSamples []operationCodeSampleData
	valid       bool
}

type operationResponseExampleData struct {
	Status          string
	ContentType     string
	SchemaJSON      string
	Example         string
	ExampleProvided bool
	visible         bool
}

type operationCodeSampleData struct {
	Label    string
	Language string
	Code     string
}

func PrepareOperationExamples(detail catalog.DetailRecordV1, operation domain.Operation, nodes []projection.SchemaNode) (OperationExamplesFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationExamplesFragment{}, invalidOperationExamplesField("operation")
	}
	projected := detail.Operation
	if projected.ID != string(detail.ID) || projected.Anchor != string(detail.ID) || operation.Anchor != projected.Anchor {
		return OperationExamplesFragment{}, invalidOperationExamplesField("operation identity")
	}
	if len(projected.Responses) != len(operation.Responses) {
		return OperationExamplesFragment{}, invalidOperationExamplesField("inventory")
	}
	generatedFallback := len(projected.CodeSamples) == 0 && len(operation.Snippets) == 1
	if !generatedFallback && len(projected.CodeSamples) != len(operation.Snippets) {
		return OperationExamplesFragment{}, invalidOperationExamplesField("code sample inventory")
	}
	records := len(projected.CodeSamples)
	for _, response := range projected.Responses {
		records += 1 + len(response.Headers) + len(response.MediaTypes)
	}
	if records > maximumOperationExampleRecords {
		return OperationExamplesFragment{}, invalidOperationExamplesField("record limit")
	}
	mediaCount := 0
	for _, response := range projected.Responses {
		mediaCount += len(response.MediaTypes)
	}
	resolver, err := newOperationResponseMediaSchemaResolver(nodes, mediaCount)
	if err != nil {
		return OperationExamplesFragment{}, invalidOperationExamplesField("schema-node inventory")
	}

	fragment := OperationExamplesFragment{
		responses:   make([][]operationResponseExampleData, len(projected.Responses)),
		codeSamples: make([]operationCodeSampleData, 0, len(projected.CodeSamples)),
		valid:       true,
	}
	preparedBytes := 0
	responseIDs := make(map[string]struct{}, len(projected.Responses))
	for responseIndex, response := range projected.Responses {
		preparedResponse := operation.Responses[responseIndex]
		_, duplicate := responseIDs[response.ID]
		if response.Ordinal != uint32(responseIndex) || response.ID != response.Status || duplicate || domain.ValidateCanonicalIdentity("response status", response.Status, false) != nil || response.Status != preparedResponse.Status || len(response.Headers) != len(preparedResponse.Headers) || len(response.MediaTypes) != len(preparedResponse.MediaTypes) {
			return OperationExamplesFragment{}, invalidOperationExamplesField("response identity")
		}
		responseIDs[response.ID] = struct{}{}
		if !admitOperationExampleBytes(&preparedBytes, response.Status) {
			return OperationExamplesFragment{}, invalidOperationExamplesField("response bytes")
		}

		headerIDs := make(map[string]struct{}, len(response.Headers))
		for headerIndex, header := range response.Headers {
			preparedHeader := preparedResponse.Headers[headerIndex]
			_, duplicate := headerIDs[header.ID]
			if header.Ordinal != uint32(headerIndex) || header.ID != operationResponseHeaderID(header.Name) || duplicate || domain.ValidateCanonicalIdentity("response header name", header.Name, false) != nil || header.Name != preparedHeader.Name || !operationResponseHeaderExampleMatches(header.Examples, preparedHeader.Example) {
				return OperationExamplesFragment{}, invalidOperationExamplesField("response header example")
			}
			headerIDs[header.ID] = struct{}{}
			if !admitOperationExampleBytes(&preparedBytes, header.Name, preparedHeader.Example) {
				return OperationExamplesFragment{}, invalidOperationExamplesField("response header bytes")
			}
		}

		fragment.responses[responseIndex] = make([]operationResponseExampleData, 0, len(response.MediaTypes))
		mediaIDs := make(map[string]struct{}, len(response.MediaTypes))
		for mediaIndex, media := range response.MediaTypes {
			preparedMedia := preparedResponse.MediaTypes[mediaIndex]
			_, duplicate := mediaIDs[media.ID]
			if media.Ordinal != uint32(mediaIndex) || media.ID != media.ContentType || duplicate || domain.ValidateCanonicalIdentity("response media content type", media.ContentType, false) != nil || media.ContentType != preparedMedia.ContentType || !operationResponseMediaExampleMatches(media.Examples, preparedMedia) {
				return OperationExamplesFragment{}, invalidOperationExamplesField("response media example")
			}
			mediaIDs[media.ID] = struct{}{}
			if err := resolver.validate(media.SchemaRef, preparedMedia.Schema, 0); err != nil {
				return OperationExamplesFragment{}, err
			}
			if node := resolver.nodes[media.SchemaRef]; node.JSON != preparedMedia.Schema.JSON || !utf8.ValidString(node.JSON) {
				return OperationExamplesFragment{}, invalidOperationExamplesField("response media schema JSON")
			}
			if !admitOperationExampleBytes(&preparedBytes, media.ContentType, preparedMedia.Schema.JSON, preparedMedia.Example) {
				return OperationExamplesFragment{}, invalidOperationExamplesField("response media bytes")
			}
			fragment.responses[responseIndex] = append(fragment.responses[responseIndex], operationResponseExampleData{
				Status: response.Status, ContentType: media.ContentType,
				SchemaJSON: preparedMedia.Schema.JSON, Example: preparedMedia.Example,
				ExampleProvided: preparedMedia.ExampleProvided,
				visible:         strings.TrimSpace(preparedMedia.Example) != "",
			})
		}
	}
	if len(resolver.used) != len(resolver.nodes) {
		return OperationExamplesFragment{}, invalidOperationExamplesField("schema-node inventory")
	}

	codeIDs := make(map[string]struct{}, len(projected.CodeSamples))
	if generatedFallback {
		prepared := operation.Snippets[0]
		if prepared.Label != "cURL" || prepared.Language != "shell" || prepared.Code != operationExamplesCurl(operation) || !admitOperationExampleBytes(&preparedBytes, prepared.Label, prepared.Language, prepared.Code) {
			return OperationExamplesFragment{}, invalidOperationExamplesField("generated code sample")
		}
		fragment.codeSamples = append(fragment.codeSamples, operationCodeSampleData{Label: prepared.Label, Language: prepared.Language, Code: prepared.Code})
	}
	for sampleIndex, sample := range projected.CodeSamples {
		prepared := operation.Snippets[sampleIndex]
		_, duplicate := codeIDs[sample.ID]
		if sample.Ordinal != uint32(sampleIndex) || sample.ID != operationCodeSampleID(sample.Language, sample.Label) || duplicate || domain.ValidateCanonicalIdentity("code sample language", sample.Language, true) != nil || sample.Label != prepared.Label || sample.Language != prepared.Language || sample.Code != prepared.Code {
			return OperationExamplesFragment{}, invalidOperationExamplesField("code sample identity")
		}
		if !admitOperationExampleBytes(&preparedBytes, sample.Label, sample.Language, sample.Code) {
			return OperationExamplesFragment{}, invalidOperationExamplesField("code sample bytes")
		}
		codeIDs[sample.ID] = struct{}{}
		fragment.codeSamples = append(fragment.codeSamples, operationCodeSampleData{Label: prepared.Label, Language: prepared.Language, Code: prepared.Code})
	}
	return fragment, nil
}

func operationExamplesCurl(operation domain.Operation) string {
	lines := []string{fmt.Sprintf("curl --request %s \\", strings.ToUpper(operation.Method))}
	if operation.RequestBody != nil && len(operation.RequestBody.MediaTypes) > 0 {
		media := operation.RequestBody.MediaTypes[0]
		if strings.TrimSpace(media.ContentType) != "" {
			lines = append(lines, "  --header "+operationExamplesShellQuote("content-type: "+media.ContentType)+" \\")
		}
		if strings.TrimSpace(media.Example) != "" {
			lines = append(lines, "  --data "+operationExamplesShellQuote(media.Example)+" \\")
		}
	}
	lines = append(lines, "  --url "+operationExamplesShellQuote(operation.Path))
	return strings.Join(lines, "\n")
}

func operationExamplesShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func admitOperationExampleBytes(total *int, values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || len(value) > maximumHTMLFragmentBytes-*total {
			return false
		}
		*total += len(value)
	}
	return true
}

func operationCodeSampleID(language, label string) string {
	hash := sha256.New()
	hash.Write([]byte("code-sample"))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range []string{language, label} {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		hash.Write(length[:])
		hash.Write([]byte(value))
	}
	return "code-sample-" + hex.EncodeToString(hash.Sum(nil))
}

func (fragment OperationExamplesFragment) HasResponseExample(responseIndex, mediaIndex int) bool {
	return fragment.valid && responseIndex >= 0 && responseIndex < len(fragment.responses) && mediaIndex >= 0 && mediaIndex < len(fragment.responses[responseIndex]) && fragment.responses[responseIndex][mediaIndex].visible
}

func OperationResponseExample(fragment OperationExamplesFragment, responseIndex, mediaIndex int, operationID string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.HasResponseExample(responseIndex, mediaIndex) || !utf8.ValidString(operationID) {
			return errInvalidOperationExamplesFragment
		}
		var output boundedBuffer
		if err := operationResponseExample(fragment.responses[responseIndex][mediaIndex], operationID).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func OperationCodeSample(fragment OperationExamplesFragment, sampleIndex int, displayLabel string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid || sampleIndex < 0 || sampleIndex >= len(fragment.codeSamples) || !utf8.ValidString(displayLabel) {
			return errInvalidOperationExamplesFragment
		}
		var output boundedBuffer
		if err := operationCodeSample(fragment.codeSamples[sampleIndex], displayLabel).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationExamplesFragment) ResponseExampleBytes(ctx context.Context, responseIndex, mediaIndex int, operationID string) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationResponseExample(fragment, responseIndex, mediaIndex, operationID).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func (fragment OperationExamplesFragment) CodeSampleBytes(ctx context.Context, sampleIndex int, displayLabel string) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationCodeSample(fragment, sampleIndex, displayLabel).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func operationResponseExamplePayload(data operationResponseExampleData) map[string]any {
	payload := map[string]any{
		"options": operationResponseExampleOptions{SkipNonRequired: false, Quiet: true, MaxSampleDepth: 3},
	}
	if data.ExampleProvided && strings.TrimSpace(data.Example) != "" {
		payload["hasExplicitExample"] = true
	}
	var schema any
	if strings.TrimSpace(data.SchemaJSON) != "" {
		if err := json.Unmarshal([]byte(data.SchemaJSON), &schema); err != nil {
			schema = nil
		}
	}
	payload["schema"] = schema
	return payload
}

type operationResponseExampleOptions struct {
	SkipNonRequired bool `json:"skipNonRequired"`
	Quiet           bool `json:"quiet"`
	MaxSampleDepth  int  `json:"maxSampleDepth"`
}

func invalidOperationExamplesField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationExamplesFragment, name)
}
