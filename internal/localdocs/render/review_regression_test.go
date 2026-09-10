package render

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/araihu/manja/domain"
)

func TestWhitespaceSchemaDescriptionsDoNotPanic(t *testing.T) {
	for _, component := range []templ.Component{
		schemaDetailHeader(schemaDetailHeaderData{Title: "Schema", Description: " \n\t"}, nil, nil),
		schemaNode(schemaNodeData{Name: "child", RootHeading: "root", Description: " \n\t"}),
	} {
		var output bytes.Buffer
		if err := component.Render(context.Background(), &output); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequestGeneratorQualifiesParameterFields(t *testing.T) {
	op := domain.Operation{Anchor: "sample", Parameters: []domain.OperationParameter{
		{Name: "id", In: "path", Example: "path-value"},
		{Name: "id", In: "query", Example: "query-value"},
	}}
	var output bytes.Buffer
	if err := RequestGenerator(op, "https://example.com").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"parameters.path.id", "parameters.query.id"} {
		if !strings.Contains(output.String(), `name="`+field+`"`) {
			t.Fatalf("missing input %s", field)
		}
	}
	parameters := requestGeneratorPayload(op, "https://example.com")["parameters"].([]map[string]string)
	if parameters[0]["fieldName"] != "parameters.path.id" || parameters[1]["fieldName"] != "parameters.query.id" {
		t.Fatalf("parameter fields collide: %v", parameters)
	}
}
