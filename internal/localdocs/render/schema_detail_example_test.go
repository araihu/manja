package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func TestPreparedSchemaDetailExampleRendersCodeBlock(t *testing.T) {
	detail, schema, document := schemaDetailExampleFixture(t)
	fragment, err := PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Root JSON Schema",
		"data-code-block",
		"&lt;pod&gt;",
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("schema detail example missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte("<pod>")) {
		t.Fatalf("schema detail example leaked unescaped JSON: %s", body)
	}
}

func TestPrepareSchemaDetailExampleFailsClosedOnInconsistentInputs(t *testing.T) {
	baseDetail, baseSchema, baseDocument := schemaDetailExampleFixture(t)
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Schema)
	}{
		{name: "operation detail", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Schema) {
			detail.Kind, detail.Schema = "operation", nil
		}},
		{name: "schema identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Schema) {
			detail.Schema.ID = "changed"
		}},
		{name: "schema href", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Schema) {
			detail.Schema.Href = "documents/other-v1/?selected=" + string(detail.ID) + "#" + string(detail.ID)
		}},
		{name: "prepared schema", mutate: func(_ *catalog.DetailRecordV1, schema *domain.Schema) {
			schema.Name = "changed"
		}},
		{name: "example mismatch", mutate: func(_ *catalog.DetailRecordV1, schema *domain.Schema) {
			schema.Example.JSON = `{"changed":true}`
		}},
		{name: "empty example", mutate: func(detail *catalog.DetailRecordV1, schema *domain.Schema) {
			detail.Schema.ExampleSchemaJSON = ""
			schema.Example.JSON = ""
		}},
		{name: "invalid example utf8", mutate: func(detail *catalog.DetailRecordV1, schema *domain.Schema) {
			invalid := string([]byte{0xff})
			detail.Schema.ExampleSchemaJSON = invalid
			schema.Example.JSON = invalid
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := cloneDetail(baseDetail)
			schema := baseSchema
			test.mutate(&detail, &schema)
			fragment, err := PrepareSchemaDetailExample(detail, schema, baseDocument)
			if err == nil || !reflect.DeepEqual(fragment, SchemaDetailExampleFragment{}) {
				t.Fatalf("inconsistent input prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPreparedSchemaDetailExampleCopiesRenderInputs(t *testing.T) {
	detail, schema, document := schemaDetailExampleFixture(t)
	fragment, err := PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	detail.Schema.ExampleSchemaJSON = `{"changed":true}`
	schema.Example.JSON = `{"changed":true}`
	got, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("prepared schema detail example changed after input mutation\nwant=%s\n got=%s", want, got)
	}
	want[0] = 'x'
	again, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again, want) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPreparedSchemaDetailExampleRejectsOversizedOutputWithoutPartialBytes(t *testing.T) {
	detail, schema, document := schemaDetailExampleFixture(t)
	huge := strings.Repeat("x", maximumHTMLFragmentBytes)
	detail.Schema.ExampleSchemaJSON = huge
	schema.Example.JSON = huge
	fragment, err := PrepareSchemaDetailExample(detail, schema, document)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("oversized fragment = bytes=%d err=%v", len(body), err)
	}
}

func TestZeroSchemaDetailExampleFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (SchemaDetailExampleFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero fragment = bytes=%d err=%v", len(body), err)
	}
}

func schemaDetailExampleFixture(t *testing.T) (catalog.DetailRecordV1, domain.Schema, catalog.DocumentDirectoryV1) {
	t.Helper()
	detail, schema, document, _ := schemaDetailHeaderFixture(t)
	example := `{"type":"object","description":"<pod>"}`
	detail.Schema.ExampleSchemaJSON = example
	schema.Example.JSON = example
	return detail, schema, document
}
