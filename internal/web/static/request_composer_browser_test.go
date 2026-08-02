package static_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestRequestComposerBundleBuildsCurlAndExposesLanguages(t *testing.T) {
	page := browserPage(t)
	if err := page.SetContent(`<main></main>`); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("request-composer.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	value, err := page.Evaluate(`() => window.ManjaRequestComposer.buildCurl({method:'post',urlTemplate:'{protocol}://{hostname}/repos/{owner}',serverVariables:{protocol:'https',hostname:'api.example.test'},parameters:[{name:'owner',in:'path',value:'araihu'},{name:'page',in:'query',value:'2'},{name:'accept',in:'header',value:'application/json'}],body:'{"name":"web"}',bodyContentType:'application/json'})`)
	if err != nil {
		t.Fatal(err)
	}
	curl, ok := value.(string)
	if !ok {
		t.Fatalf("curl type = %T", value)
	}
	for _, want := range []string{"curl --request POST", "https://api.example.test/repos/araihu?page=2", "accept: application/json", "content-type: application/json", `{"name":"web"}`} {
		if !strings.Contains(curl, want) {
			t.Fatalf("curl missing %q: %s", want, curl)
		}
	}
	available, err := page.Evaluate(`() => Boolean(window.ManjaHTTPSnippet && window.ManjaHighlight && window.ManjaRequestComposer)`)
	if err != nil {
		t.Fatal(err)
	}
	if available != true {
		t.Fatal("bundle globals unavailable")
	}
}
