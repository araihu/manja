package openapi

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestSimpleSampleBoundsRecursiveSchema(t *testing.T) {
	node := openapi3.NewObjectSchema().WithProperty("name", openapi3.NewStringSchema())
	children := openapi3.NewArraySchema().WithItems(node)
	node.WithProperty("children", children)

	got := simpleSample(node)
	want := map[string]any{
		"children": []any{},
		"name":     "string",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("simpleSample(recursive schema) = %#v, want %#v", got, want)
	}
}

func TestSimpleSampleBoundsDeepAcyclicSchema(t *testing.T) {
	root := openapi3.NewObjectSchema()
	cursor := root
	for range maxSimpleSampleDepth + 2 {
		next := openapi3.NewObjectSchema()
		cursor.WithProperty("child", next)
		cursor = next
	}
	cursor.WithProperty("value", openapi3.NewStringSchema())

	got := simpleSample(root)
	for depth := 0; depth <= maxSimpleSampleDepth; depth++ {
		object, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("sample depth %d = %#v, want object", depth, got)
		}
		if depth == maxSimpleSampleDepth {
			if len(object) != 0 {
				t.Fatalf("sample at depth bound = %#v, want empty object", object)
			}
			return
		}
		got = object["child"]
	}
}
