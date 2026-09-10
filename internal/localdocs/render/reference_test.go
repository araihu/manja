package render

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestOperationReferenceExampleSelectors(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		examples := OperationExamplesFragment{valid: true}
		for i := 0; i < count; i++ {
			examples.codeSamples = append(examples.codeSamples, operationCodeSampleData{DisplayLabel: "curl", Language: "shell", Code: "curl /pets"})
			examples.responses = append(examples.responses, []operationResponseExampleData{{ID: string(rune('a' + i)), Status: "200", ContentType: "application/json", Example: `{}`, visible: true}})
		}
		var out bytes.Buffer
		if err := OperationReference(nil, examples, nil).Render(context.Background(), &out); err != nil {
			t.Fatal(err)
		}
		body := out.String()
		if got := strings.Count(body, "<select") + strings.Count(body, `role="combobox"`); got != map[bool]int{false: 0, true: 2}[count > 1] {
			t.Fatalf("%d examples: selectors = %d", count, got)
		}
		if got := strings.Count(body, "data-manja-example>"); got != count {
			t.Fatalf("%d examples: rendered %d", count, got)
		}
	}
}

func TestOperationReferenceKeepsResponsesWithoutExamples(t *testing.T) {
	examples := OperationExamplesFragment{valid: true, responses: [][]operationResponseExampleData{
		{{Status: "202", ContentType: "application/json"}},
		{},
	}}
	responses := OperationResponsesFragment{valid: true, data: operationResponsesData{Responses: []operationResponseSectionData{{Status: "202"}, {Status: "204"}}}}
	sections := OperationDetailSectionsFragment{valid: true, data: operationDetailSectionsData{Responses: &responses}}
	var out bytes.Buffer
	if err := OperationReference(&sections, examples, nil).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`aria-label="Response example"`, `data-select-config=`, `202 · application/json`, `>204</p>`, `No example provided`, `No response body defined.`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "data-code-block-copy") {
		t.Fatal("missing examples must not have fabricated copyable payloads")
	}
}
