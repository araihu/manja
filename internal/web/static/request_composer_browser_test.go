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
	value, err := page.Evaluate(`() => window.ManjaRequestComposer.buildCurl({method:'post',urlTemplate:'{protocol}://{hostname}/repos/{owner}',serverVariables:{protocol:'https',hostname:'api.example.test'},parameters:[{name:'owner',in:'path',value:'araihu'},{name:'page',in:'query',value:'2'},{name:'accept',in:'header',value:'application/json'}],security:[{in:'header',parameterName:'Authorization',placeholder:'Bearer YOUR_ACCESS_TOKEN'}],body:'{"name":"web"}',bodyContentType:'application/json'})`)
	if err != nil {
		t.Fatal(err)
	}
	curl, ok := value.(string)
	if !ok {
		t.Fatalf("curl type = %T", value)
	}
	for _, want := range []string{"curl --request POST", "https://api.example.test/repos/araihu?page=2", "accept: application/json", "Authorization: Bearer YOUR_ACCESS_TOKEN", "content-type: application/json", `{"name":"web"}`} {
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

func TestRequestComposerBundleValidatesJSONBodyBeforeCopying(t *testing.T) {
	page := browserPage(t)
	if err := page.SetContent(`<section id="composer"><textarea data-manja-request-body-input></textarea><p data-manja-request-body-status></p></section>`); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("request-composer.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	invalid, err := page.Evaluate(`() => {
		const root = document.getElementById('composer');
		const input = root.querySelector('[data-manja-request-body-input]');
		input.value = '{ invalid';
		const valid = window.ManjaRequestComposer.updateBodyValidation(root, input, {bodyContentType:'application/json'});
		return {valid, invalid: input.getAttribute('aria-invalid'), state: input.dataset.manjaRequestBodyValid, status: root.querySelector('[data-manja-request-body-status]').textContent};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := invalid.(map[string]any)
	if !ok || state["valid"] != false || state["invalid"] != "true" || state["state"] != "false" || !strings.Contains(state["status"].(string), "valid JSON") {
		t.Fatalf("invalid JSON state = %#v", invalid)
	}
	valid, err := page.Evaluate(`() => {
		const root = document.getElementById('composer');
		const input = root.querySelector('[data-manja-request-body-input]');
		input.value = '{"ok":true}';
		return window.ManjaRequestComposer.updateBodyValidation(root, input, {bodyContentType:'application/json'});
	}`)
	if err != nil || valid != true {
		t.Fatalf("valid JSON result = %#v, %v", valid, err)
	}
	cleared, err := page.Evaluate(`() => ({invalid: document.querySelector('[data-manja-request-body-input]').getAttribute('aria-invalid'), state: document.querySelector('[data-manja-request-body-input]').dataset.manjaRequestBodyValid, status: document.querySelector('[data-manja-request-body-status]').textContent})`)
	if err != nil {
		t.Fatal(err)
	}
	clearedState, ok := cleared.(map[string]any)
	if !ok || clearedState["invalid"] != nil || clearedState["state"] != "true" || clearedState["status"] != "" {
		t.Fatalf("valid JSON did not clear validation state = %#v", cleared)
	}
}

func TestRequestComposerBundleEnhancesOnlyHydratedAlpineReadyShells(t *testing.T) {
	page := browserPage(t)
	if err := page.SetContent(`
		<section id="ready-first-shell" data-manja-request-config-root data-manja-request-config-controls-ready="true">
			<div data-manja-request-composer>
				<div data-manja-request-sample><div class="codeblock"><code>curl</code></div></div>
				<script id="ready-first-request-composer-payload" type="application/json">{"method":"GET","urlTemplate":"https://api.example.test/widgets"}</script>
			</div>
		</section>
		<section id="ready-later-shell" data-manja-request-config-root>
			<div data-manja-request-composer>
				<div data-manja-request-sample><div class="codeblock"><code>curl</code></div></div>
				<script id="ready-later-request-composer-payload" type="application/json">{"method":"GET","urlTemplate":"https://api.example.test/widgets"}</script>
			</div>
		</section>
		<section id="invalid-shell" data-manja-request-config-root data-manja-request-config-controls-ready="true">
			<div data-manja-request-composer>
				<div data-manja-request-sample><div class="codeblock"><code>curl</code></div></div>
				<script id="invalid-request-composer-payload" type="application/json">{</script>
			</div>
		</section>
	`); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("request-composer.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	beforeReady, err := page.Evaluate(`() => ({
		readyFirstEnhanced: document.getElementById('ready-first-shell')?.dataset.manjaRequestConfigEnhanced || '',
		readyFirstHydrated: document.querySelector('#ready-first-shell [data-manja-request-composer]')?.dataset.manjaRequestComposerHydrated || '',
		readyLaterEnhanced: document.getElementById('ready-later-shell')?.dataset.manjaRequestConfigEnhanced || '',
		readyLaterHydrated: document.querySelector('#ready-later-shell [data-manja-request-composer]')?.dataset.manjaRequestComposerHydrated || '',
		invalidEnhanced: document.getElementById('invalid-shell')?.dataset.manjaRequestConfigEnhanced || '',
		invalidHydrated: document.querySelector('#invalid-shell [data-manja-request-composer]')?.dataset.manjaRequestComposerHydrated || '',
	})`)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := beforeReady.(map[string]any)
	if !ok {
		t.Fatalf("request composer enhancement state should be a map, got %#v", beforeReady)
	}
	if state["readyFirstEnhanced"] != "true" || state["readyFirstHydrated"] != "true" || state["readyLaterEnhanced"] != "" || state["readyLaterHydrated"] != "true" || state["invalidEnhanced"] != "" || state["invalidHydrated"] != "" {
		t.Fatalf("request composer should require hydration and Alpine controls readiness, got %#v", state)
	}

	afterReady, err := page.Evaluate(`() => {
		const shell = document.getElementById('ready-later-shell');
		shell.dataset.manjaRequestConfigControlsReady = 'true';
		shell.dispatchEvent(new CustomEvent('manja:request-config-controls-ready'));
		const firstLifecycle = shell.dataset.manjaRequestConfigEnhanced || '';
		delete shell.dataset.manjaRequestConfigControlsReady;
		delete shell.dataset.manjaRequestConfigEnhanced;
		shell.dataset.manjaRequestConfigControlsReady = 'true';
		shell.dispatchEvent(new CustomEvent('manja:request-config-controls-ready'));
		return {
			firstLifecycle,
			secondLifecycle: shell.dataset.manjaRequestConfigEnhanced || '',
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	state, ok = afterReady.(map[string]any)
	if !ok || state["firstLifecycle"] != "true" || state["secondLifecycle"] != "true" {
		t.Fatalf("request composer should enhance across Alpine control lifecycles after hydration, got %#v", afterReady)
	}
}
