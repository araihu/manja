//go:build !manja_runtime

package selfhosted

import (
	"strings"
	"testing"
)

func TestExtractLazySchemaHTMLFragmentsDeduplicatesCanonicalTrees(t *testing.T) {
	input := []byte(`<article><section id="operation-a-response-200-schema" aria-label="Response body schema tree" class="gs-schema-tree" data-manja-schema-tree="true"><div id="operation-a-tooltip" data-schema-tree-node="Pet"><a href="#operation-a-tooltip" aria-describedby="operation-a-tooltip" data-tooltip-content-id="operation-a-tooltip">Pet</a></div></section><section id="operation-b-request-body-schema" aria-label="Request body schema tree" class="gs-schema-tree" data-manja-schema-tree="true"><div id="operation-b-tooltip" data-schema-tree-node="Pet"><a href="#operation-b-tooltip" aria-describedby="operation-b-tooltip" data-tooltip-content-id="operation-b-tooltip">Pet</a></div></section></article>`)

	operation, fragments, err := extractLazySchemaHTMLFragments(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 1 {
		t.Fatalf("standalone schema fragments = %d, want 1", len(fragments))
	}
	operationText := string(operation)
	if strings.Count(operationText, `class="min-h-12 min-w-0"`) != 2 || strings.Contains(operationText, "border-outline") || strings.Contains(operationText, "rounded-radius") {
		t.Fatalf("lazy schema containers must stay unboxed: %s", operationText)
	}
	if strings.Contains(operationText, `class="gs-schema-tree" data-manja-schema-tree="true"`) || strings.Contains(operationText, `data-schema-tree-node`) {
		t.Fatalf("operation retained schema HTML: %s", operationText)
	}
	if strings.Count(operationText, `data-manja-schema-resource="`+fragments[0].Resource+`"`) != 2 {
		t.Fatalf("operation placeholders do not share content resource: %s", operationText)
	}
	fragmentText := string(fragments[0].HTML)
	for _, want := range []string{`aria-label="Schema tree"`, `id="manja-schema-tree-id-1"`, `id="manja-schema-tree-id-2"`, `href="#manja-schema-tree-id-2"`, `aria-describedby="manja-schema-tree-id-2"`, `data-tooltip-content-id="manja-schema-tree-id-2"`, `data-schema-tree-node="Pet"`} {
		if !strings.Contains(fragmentText, want) {
			t.Errorf("standalone schema fragment lacks %q: %s", want, fragmentText)
		}
	}
	resources, err := lazySchemaHTMLResources(operation)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0] != fragments[0].Resource {
		t.Fatalf("placeholder resources = %#v", resources)
	}
}

func TestLazySchemaHTMLResourcesRejectsUntrustedResource(t *testing.T) {
	_, err := lazySchemaHTMLResources([]byte(`<div data-manja-static-schema-fragment="true" data-manja-schema-resource="../../schema"></div>`))
	if err == nil {
		t.Fatal("invalid lazy schema resource was accepted")
	}
}
