package store

import "testing"

func TestStrictJSONRejectsDuplicateDecodedMemberNamesRecursively(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "literal", raw: `{"name":1,"name":2}`},
		{name: "escaped equivalent", raw: `{"name":1,"\u006eame":2}`},
		{name: "surrogate pair equivalent", raw: `{"😀":1,"\ud83d\ude00":2}`},
		{name: "nested object", raw: `{"outer":{"name":1,"\u006eame":2}}`},
		{name: "object in array", raw: `{"outer":[{"name":1,"\u006eame":2}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value any
			if err := strictJSONUnmarshal([]byte(test.raw), &value); err == nil {
				t.Fatal("duplicate decoded JSON member names were accepted")
			}
		})
	}
}

func TestStrictJSONPreservesValidObjectsArraysAndEscapes(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "nested values", raw: `{"name":1,"nested":{"name":2},"array":[{"name":3}]}`},
		{name: "distinct surrogate pairs", raw: `{"😀":1,"\ud83d\ude01":2}`},
		{name: "double escaped key is literal", raw: `{"a":1,"\\u0061":2}`},
		{name: "replacement forms in values", raw: `{"escaped":"\ufffd","literal":"�"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value any
			if err := strictJSONUnmarshal([]byte(test.raw), &value); err != nil {
				t.Fatalf("valid strict JSON rejected: %v", err)
			}
		})
	}
}
