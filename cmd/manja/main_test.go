package main

import "testing"

func TestDefaultSpecPathUsesGitHubRESTFixture(t *testing.T) {
	want := "internal/adapters/openapi/testdata/github-v3-rest.json"
	if got := defaultSpecPath(); got != want {
		t.Fatalf("defaultSpecPath() = %q, want %q", got, want)
	}
}
