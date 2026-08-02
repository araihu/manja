package scripts

import (
	"os"
	"strings"
	"testing"
)

func TestRedoclyWrapper(t *testing.T) {
	contents, err := os.ReadFile("redocly")
	if err != nil {
		t.Fatal(err)
	}
	source := string(contents)
	for _, want := range []string{"npx", "--yes", "@redocly/cli@1.34.15", "exec"} {
		if !strings.Contains(source, want) {
			t.Errorf("wrapper missing %q", want)
		}
	}
	for _, forbidden := range []string{"@latest", "npm install", "npm ci", "npm run"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("wrapper contains %q", forbidden)
		}
	}
}
