package server

import (
	"encoding/json"
	"html"
	"strings"
	"testing"
)

func TestPrefixDependencyPathsEscapesAttributeContent(t *testing.T) {
	config := map[string]any{"dependencies": []any{map[string]any{
		"primary_url":  "/assets/runtime.js",
		"fallback_url": "https://cdn.example/runtime.js",
		"integrity":    `"><img src=x onerror=alert(1)>`,
	}}}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	input := `data-goshtoso-dependencies="` + html.EscapeString(string(encoded)) + `"`
	output := prefixDependencyPaths(input, "/demo")
	if strings.Count(output, `"`) != 2 || strings.Contains(output, "<img") {
		t.Fatalf("dependency value escaped its HTML attribute: %s", output)
	}
	match := dependencyAttribute.FindStringSubmatch(output)
	if len(match) != 2 {
		t.Fatalf("missing attribute: %s", output)
	}
	var result struct {
		Dependencies []struct {
			Primary   string `json:"primary_url"`
			Fallback  string `json:"fallback_url"`
			Integrity string `json:"integrity"`
		}
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(match[1])), &result); err != nil {
		t.Fatal(err)
	}
	dependency := result.Dependencies[0]
	if dependency.Primary != "/demo/assets/runtime.js" || dependency.Fallback != "https://cdn.example/runtime.js" || dependency.Integrity != `"><img src=x onerror=alert(1)>` {
		t.Fatalf("dependency values changed unexpectedly: %+v", dependency)
	}
}
