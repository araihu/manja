//go:build js && wasm

package main

import "testing"

func TestCanonicalizeJSONStringPreservesGoCanonicalEscapes(t *testing.T) {
	input := "{\"value\":\"<>&\u2028\u2029\"}"
	want := `{"value":"\u003c\u003e\u0026\u2028\u2029"}`
	if got := canonicalizeJSONString(input); got != want {
		t.Fatalf("canonicalizeJSONString() = %q, want %q", got, want)
	}
}
