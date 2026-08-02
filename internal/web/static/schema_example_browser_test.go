package static_test

import (
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestSchemaExampleBundleHydratesGeneratedExplicitAndMalformedRoots(t *testing.T) {
	page := browserPage(t)
	if err := page.SetContent(`<main>
<div data-manja-example><div class="codeblock"><code>fallback</code></div><span data-manja-example-status></span><script type="application/json">{"schema":{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"done":{"type":"boolean"}}},"options":{"skipNonRequired":true}}</script></div>
<div data-manja-example><div class="codeblock"><code>{"name":"provided"}</code></div><span data-manja-example-status></span><script type="application/json">{"hasExplicitExample":true,"schema":{"type":"object"}}</script></div>
<div data-manja-example><div class="codeblock"><code>fallback</code></div><span data-manja-example-status></span><script type="application/json">{bad</script></div>
</main>`); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("schema-example.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	values, err := page.Locator("[data-manja-example-status]").AllTextContents()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Example generated", "Spec example", "Example unavailable"}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("statuses = %v", values)
		}
	}
	code, err := page.Locator("[data-manja-example]").First().Locator("code").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if code == "fallback" {
		t.Fatal("schema example was not generated")
	}
}

func browserPage(t *testing.T) playwright.Page {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping browser test in short mode")
	}
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pw.Stop() })
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = browser.Close() })
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	return page
}
