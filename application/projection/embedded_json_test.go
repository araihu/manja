package projection

import (
	"strings"
	"testing"
)

func TestEmbeddedJSONCanonicalization(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "sorted object", input: `{ "z": 1, "a": [true, null, "x"] }`, want: `{"a":[true,null,"x"],"z":1}`},
		{name: "HTML and separators", input: "{\"x\":\"<>&\\u2028\\u2029\"}", want: `{"x":"\u003c\u003e\u0026\u2028\u2029"}`},
		{name: "scalar", input: `true`, want: `true`},
		{name: "positive exponent", input: `1e+03`, want: `1000`},
		{name: "fraction zeros", input: `1.0`, want: `1`},
		{name: "negative zero", input: `-0`, want: `0`},
		{name: "negative exponent", input: `1e-3`, want: `0.001`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := canonicalEmbeddedJSON(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("canonical = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEmbeddedJSONRejectsMalformedAndUnboundedInput(t *testing.T) {
	invalid := []string{
		`{"a":1,"a":2}`,
		`1 2`,
		`plain text`,
		string([]byte{0xff}),
		`1e999999999999999999999999`,
	}
	for _, input := range invalid {
		if _, err := canonicalEmbeddedJSON(input); err == nil {
			t.Errorf("input %q accepted", input)
		}
	}

	depth64 := strings.Repeat("[", 64) + "0" + strings.Repeat("]", 64)
	if _, err := canonicalEmbeddedJSON(depth64); err != nil {
		t.Fatalf("depth 64 rejected: %v", err)
	}
	depth65 := "[" + depth64 + "]"
	if _, err := canonicalEmbeddedJSON(depth65); err == nil {
		t.Fatal("depth 65 accepted")
	}
}

func TestEmbeddedJSONDoesNotMutateInput(t *testing.T) {
	input := `{"z":1,"a":2}`
	want := input
	if _, err := canonicalEmbeddedJSON(input); err != nil {
		t.Fatal(err)
	}
	if input != want {
		t.Fatalf("input = %q, want unchanged %q", input, want)
	}
}
