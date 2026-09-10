package selfhosted

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestStaticResponseExampleCopy(t *testing.T) {
	output := os.Getenv("MANJA_GITHUB_MARKDOWN_PREVIEW")
	if output == "" {
		t.Skip("set MANJA_GITHUB_MARKDOWN_PREVIEW to the exported GitHub tree")
	}
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	server := httptest.NewServer(http.FileServer(http.Dir(output)))
	defer server.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	const id = "detail-sha256-c392a62c2c0fa9fb24013708a1bfcab1034233e37a95101511814334bc66ad0e"
	if _, err := page.Goto(server.URL + "/catalogs/github/documents/github-v3-rest/?selected=" + id + "#" + id); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: async text => { window.copiedExample = text; }}})`); err != nil {
		t.Fatal(err)
	}
	request := page.Locator(`[data-manja-request-samples]`)
	if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-composer]')?.dataset.manjaRequestComposerHydrated === 'true'`, nil); err != nil {
		t.Fatal(err)
	}
	// History snapshots preserve data attributes, but not attached event listeners.
	if _, err := page.Evaluate(`() => { const main = document.querySelector('#catalog-main-content'); main.innerHTML = main.innerHTML; main.querySelectorAll('[data-code-block-copy]').forEach(button => button.hidden = true); document.dispatchEvent(new CustomEvent('htmx:historyRestore', {bubbles: true})); }`); err != nil {
		t.Fatal(err)
	}
	if err := request.Locator(`[role="combobox"]`).Click(); err != nil {
		t.Fatal(err)
	}
	if err := request.Locator(`[data-combobox-search]`).Fill("Go / NewRequest"); err != nil {
		t.Fatal(err)
	}
	if err := request.Locator(`[data-combobox-option]:visible`).Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.querySelector('[data-manja-request-sample] .codeblock').textContent.includes('http.NewRequest')`, nil); err != nil {
		t.Fatalf("selecting Go must update the sample before opening responses: %v", err)
	}
	if err := request.Locator(`[data-code-block-copy]`).Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => window.copiedExample?.includes('http.NewRequest')`, nil); err != nil {
		t.Fatalf("request Copy must copy the selected language: %v", err)
	}
	if err := page.Locator(`[aria-controls="content-` + id + `-response-200"]`).Click(); err != nil {
		t.Fatal(err)
	}
	example := page.Locator(`[id="` + id + `-response-200-application-json-example"]`)
	button := example.Locator(`[data-code-block-copy]`)
	if err := button.Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => typeof window.copiedExample === 'string'`, nil); err != nil {
		t.Fatal(err)
	}
	want, err := example.Locator(`[id="` + id + `-response-200-application-json-example-code"]`).TextContent()
	if err != nil {
		t.Fatal(err)
	}
	got, err := page.Evaluate(`window.copiedExample`)
	if err != nil || got != want || !strings.Contains(want, "total_active_caches_count") {
		t.Fatalf("copied example = %v; want %q; error %v", got, want, err)
	}
	if border, err := page.Locator(".manja-response-media-block").Evaluate(`el => getComputedStyle(el).borderTopWidth`, nil); err != nil || border != "0px" {
		t.Fatalf("response separator: %v %v", border, err)
	}
	if border, err := page.Locator("[data-manja-operation-navigation]").Evaluate(`el => getComputedStyle(el).borderTopWidth`, nil); err != nil || border != "0px" {
		t.Fatalf("operation navigation separator: %v %v", border, err)
	}
	if _, err := page.WaitForFunction(`() => { const description = [...document.querySelectorAll('.gs-schema-tree-description')].find(el => el.textContent.includes('The slug version of the enterprise name.')); if (!description) return false; const body = description.closest('.gs-schema-tree-body'); const node = body.closest('.gs-schema-tree-node'); return parseFloat(getComputedStyle(body).paddingInlineStart) === 40 && parseFloat(getComputedStyle(description).paddingInlineStart) === 0 && parseFloat(getComputedStyle(node).paddingTop) === 4 && description.scrollWidth <= description.clientWidth + 1; }`, nil); err != nil {
		t.Fatalf("schema description must have a small indent without overflow: %v", err)
	}
	if err := page.SetViewportSize(1653, 994); err != nil {
		t.Fatal(err)
	}
	if compact, err := page.Locator(".manja-operation-reference").Evaluate(`root => { const header = root.querySelector('[data-public-page-header]'); const layout = root.querySelector('.manja-endpoint-detail-layout'); const request = layout.querySelector('[aria-label="Request"]'); const responses = layout.querySelector('[aria-label="Responses"]'); return getComputedStyle(header).paddingBottom === '0px' && getComputedStyle(layout).paddingTop === '0px' && getComputedStyle(request).rowGap === '16px' && responses.getBoundingClientRect().top - request.getBoundingClientRect().bottom <= 25; }`, nil); err != nil || compact != true {
		t.Fatalf("operation blocks must use compact, non-stacked spacing: %v %v", compact, err)
	}
	if _, err := page.WaitForFunction(`() => { const row = document.querySelector('#catalog-sidebar-groups a[data-catalog-sidebar-operation]'); if (!row) return false; const label = row.querySelector('span'); const style = getComputedStyle(label); return style.whiteSpace === 'normal' && label.getBoundingClientRect().height > parseFloat(style.lineHeight) * 1.5 && row.scrollWidth <= row.clientWidth + 1; }`, nil); err != nil {
		t.Fatalf("sidebar operation label must wrap without overflow: %v", err)
	}
	if err := page.Locator(`[data-manja-sidebar-tab="schemas"]`).Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => { const row = document.querySelector('[data-manja-static-sidebar-schemas] a[title="actions-artifact-and-log-retention-response"]'); if (!row) return false; const label = row.querySelector('span'); const style = getComputedStyle(label); return style.whiteSpace === 'normal' && label.getBoundingClientRect().height > parseFloat(style.lineHeight) * 1.5 && row.scrollWidth <= row.clientWidth + 1; }`, nil); err != nil {
		t.Fatalf("sidebar schema label must wrap without overflow: %v", err)
	}
	if _, err := page.WaitForFunction(`() => { const grid = document.querySelector('.manja-operation-reference-grid'); const doc = grid.firstElementChild.getBoundingClientRect(); const rail = grid.querySelector('aside').getBoundingClientRect(); return rail.left >= doc.right; }`, nil); err != nil {
		t.Fatal(err)
	}
	if aligned, err := page.Locator(".manja-operation-reference-grid").Evaluate(`grid => { const heading = grid.querySelector('h1').getBoundingClientRect(); const rail = grid.querySelector('aside').getBoundingClientRect(); return Math.abs(rail.top - heading.top) < 4; }`, nil); err != nil || aligned != true {
		t.Fatalf("examples must align with operation title: %v %v", aligned, err)
	}
	if err := page.SetViewportSize(822, 994); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => { const grid = document.querySelector('.manja-operation-reference-grid'); const doc = grid.firstElementChild.getBoundingClientRect(); const rail = grid.querySelector('aside').getBoundingClientRect(); return rail.top >= doc.bottom; }`, nil); err != nil {
		t.Fatal(err)
	}
	propertyPage, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	defer propertyPage.Close()
	const propertyID = "detail-sha256-757e24149d89e15b9549420d517c7529253f73093fa9a17b18eaf0294fc172a5"
	if _, err := propertyPage.Goto(server.URL + "/catalogs/github/documents/github-v3-rest/?selected=" + propertyID + "#" + propertyID); err != nil {
		t.Fatal(err)
	}
	if _, err := propertyPage.WaitForFunction(`() => { const name = [...document.querySelectorAll('.gs-schema-tree-name')].find(el => el.textContent === 'labels'); if (!name) return false; const node = name.closest('.gs-schema-tree-node'); const description = node.querySelector('.gs-schema-tree-description'); const constraints = node.querySelector('.gs-schema-tree-constraints'); if (!description || !constraints) return false; return Math.abs(description.getBoundingClientRect().left - constraints.getBoundingClientRect().left) < 1 && Math.abs(description.getBoundingClientRect().left - name.getBoundingClientRect().left - 12) < 1; }`, nil); err != nil {
		t.Fatalf("property description and constraints must share the same indent: %v", err)
	}
	queryPage, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	defer queryPage.Close()
	const queryID = "detail-sha256-0f00cc441a2a49198191ec2c03b91dd17be5ace1b780559c160af1d0c150293b"
	if _, err := queryPage.Goto(server.URL + "/catalogs/github/documents/github-v3-rest/?selected=" + queryID + "#" + queryID); err != nil {
		t.Fatal(err)
	}
	if _, err := queryPage.WaitForFunction(`() => { const tree = document.querySelector('.gs-schema-tree[aria-label="Query Parameters"]'); if (!tree) return false; const names = [...tree.querySelectorAll('.gs-schema-tree-name')].map(el => el.textContent); return ['per_page', 'cursor', 'status'].every(name => names.includes(name)) && tree.querySelector('.gs-schema-tree-description .margo-document code') && ![...tree.querySelectorAll('.gs-schema-tree-state')].some(el => el.textContent === 'optional'); }`, nil); err != nil {
		t.Fatalf("query parameters must use SchemaTree with Markdown descriptions: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(output, "catalogs/github/documents/github-v3-rest/_manja/fragments/operations/*.html"))
	if err != nil {
		t.Fatal(err)
	}
	var selected string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `aria-label="Response example"`) {
			match := regexp.MustCompile(`data-public-doc-identity="([^"]+)"`).FindSubmatch(data)
			if len(match) == 2 {
				selected = string(match[1])
				break
			}
		}
	}
	if selected == "" {
		t.Fatal("missing operation with multiple response examples")
	}
	page, err = browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if err := page.SetViewportSize(1653, 994); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server.URL + "/catalogs/github/documents/github-v3-rest/?selected=" + selected + "#" + selected); err != nil {
		t.Fatal(err)
	}
	selector := page.Locator(`[role="combobox"][aria-label="Response example"]`)
	if err := selector.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-manja-response-samples] [role="option"]`).Nth(1).Click(); err != nil {
		body, _ := page.Locator("body").InnerText()
		t.Fatalf("select response for %s: %v; page: %s", selected, err, body)
	}
	if _, err := page.WaitForFunction(`() => { const panels = document.querySelectorAll('[data-manja-response-samples] > div[x-bind\\:hidden]'); return panels.length > 1 && panels[0].hidden && !panels[1].hidden; }`, nil); err != nil {
		t.Fatal(err)
	}
	if overflow, err := page.Locator(".manja-operation-reference-grid").Evaluate(`el => el.scrollWidth > el.clientWidth + 1`, nil); err != nil || overflow != false {
		t.Fatalf("examples overflow their layout: %v %v", overflow, err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("/tmp/manja-samples-column.png")}); err != nil {
		t.Fatal(err)
	}
}

// Run against the complete generated GitHub corpus, without a Manja server.
func TestStaticResponsesWithoutExamplesRemainVisible(t *testing.T) {
	output := os.Getenv("MANJA_GITHUB_MARKDOWN_PREVIEW")
	if output == "" {
		t.Skip("set MANJA_GITHUB_MARKDOWN_PREVIEW to the exported GitHub tree")
	}
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	server := httptest.NewServer(http.FileServer(http.Dir(output)))
	defer server.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	const id = "detail-sha256-d724fa4eb9aa7a521d61b9881c3b1b55f45a19a01423f91ccc6ca7b1dd4d94cd"
	if _, err := page.Goto(server.URL + "/catalogs/github/documents/github-v3-rest/?selected=" + id + "#" + id); err != nil {
		t.Fatal(err)
	}
	target := page.Locator(`[data-manja-request-sample-target]`)
	if err := target.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if count, err := target.Locator("[data-combobox-option]").Count(); err != nil || count != 22 {
		t.Fatalf("request targets: %d %v", count, err)
	}
	for _, sample := range []struct{ target, content string }{{"JavaScript / fetch", "fetch("}, {"Python / Requests", "requests."}, {"Go / NewRequest", "http.NewRequest"}, {"Shell / cURL", "curl --request POST"}} {
		if err := target.Locator(`[role="combobox"]`).Click(); err != nil {
			t.Fatal(err)
		}
		if err := target.Locator(`[data-combobox-search]`).Fill(sample.target); err != nil {
			t.Fatal(err)
		}
		if count, err := target.Locator(`[data-combobox-option]:visible`).Count(); err != nil || count != 1 {
			t.Fatalf("search must filter to one target: %d %v", count, err)
		}
		if err := target.Locator(`[data-combobox-search]`).Press("ArrowDown"); err != nil {
			t.Fatal(err)
		}
		if err := target.Locator(`[data-combobox-option]:visible`).Press("Enter"); err != nil {
			t.Fatal(err)
		}
		if _, err := page.WaitForFunction(`expected => document.querySelector('[data-manja-request-sample] .codeblock').textContent.includes(expected)`, sample.content); err != nil {
			state, _ := target.Evaluate(`el => ({value: el.querySelector('input[type=hidden]')?.value, label: el.querySelector('[role=combobox]').textContent, code: el.closest('[data-manja-request-sample]').querySelector('.codeblock').textContent})`, nil)
			t.Fatalf("request target %s: %v; state: %v", sample.target, err, state)
		}
	}
	selector := page.Locator(`[role="combobox"][aria-label="Response example"]`)
	if err := selector.WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := selector.Click(); err != nil {
		t.Fatal(err)
	}
	optionLabels, err := page.Locator(`[data-manja-response-samples] [role="option"]`).AllTextContents()
	labels := strings.Join(optionLabels, " ")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"202", "400", "422"} {
		if !strings.Contains(labels, status) {
			t.Fatalf("missing response %s: %s", status, labels)
		}
	}
	if err := page.Locator(`[data-manja-response-example-empty]`).First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-manja-response-samples] [role="option"]`).Nth(1).Click(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => { const panels = document.querySelectorAll('[data-manja-response-samples] > div[x-bind\\:hidden]'); return panels[0].hidden && !panels[1].hidden; }`, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubMarkdownStaticPreview(t *testing.T) {
	output := os.Getenv("MANJA_GITHUB_MARKDOWN_PREVIEW")
	if output == "" {
		t.Skip("set MANJA_GITHUB_MARKDOWN_PREVIEW to the exported GitHub tree")
	}
	basePath := os.Getenv("MANJA_GITHUB_MARKDOWN_BASE_PATH")
	if basePath == "" {
		basePath = "/"
	}
	files, err := filepath.Glob(filepath.Join(output, "catalogs/github/documents/github-v3-rest/_manja/fragments/operations/*.html"))
	if err != nil {
		t.Fatal(err)
	}
	identity := regexp.MustCompile(`data-public-doc-identity="([^"]+)"`)
	var tableID, codeID string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		match := identity.FindSubmatch(data)
		if len(match) != 2 {
			continue
		}
		if tableID == "" && strings.Contains(string(data), "margo-table") {
			tableID = string(match[1])
		}
		if codeID == "" && strings.Contains(string(data), "data-margo-code-copy") {
			codeID = string(match[1])
		}
	}
	if tableID == "" || codeID == "" {
		t.Fatal("missing rich GitHub operation fragments")
	}
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	server := httptest.NewServer(http.StripPrefix(strings.TrimSuffix(basePath, "/"), http.FileServer(http.Dir(output))))
	defer server.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	page.OnPageError(func(err error) { t.Errorf("browser error: %v", err) })
	page.OnResponse(func(response playwright.Response) {
		if response.Status() >= 400 {
			t.Errorf("HTTP %d: %s", response.Status(), response.URL())
		}
	})
	for _, id := range []string{tableID, codeID} {
		url := server.URL + basePath + "catalogs/github/documents/github-v3-rest/?selected=" + id + "#" + id
		if _, err := page.Goto(url); err != nil {
			t.Fatal(err)
		}
		if err := page.Locator(".manja-operation-markdown .margo-document").WaitFor(); err != nil {
			t.Fatal(err)
		}
		if count, _ := page.Locator(`link[href$="/margo/document.css"]`).Count(); count != 1 {
			t.Fatalf("stylesheet count %d", count)
		}
		if count, _ := page.Locator("[data-manja-markdown-fallback]").Count(); count != 0 {
			t.Fatalf("unexpected plain-text fallback %d", count)
		}
		if id == codeID {
			button := page.Locator(".manja-operation-markdown [data-code-block-copy]").First()
			if err := button.WaitFor(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.Evaluate(`() => Object.defineProperty(navigator, 'clipboard', {configurable:true,value:{writeText:async text => {window.copiedMarkdownCode=text}}})`); err != nil {
				t.Fatal(err)
			}
			if err := button.Click(); err != nil {
				t.Fatal(err)
			}
			if _, err := page.WaitForFunction(`() => !!window.copiedMarkdownCode`, nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := page.SetViewportSize(800, 950); err != nil {
			t.Fatal(err)
		}
		if overflow, err := page.Evaluate(`() => document.documentElement.scrollWidth > innerWidth`); err != nil || overflow == true {
			t.Fatalf("horizontal overflow: %v %v", overflow, err)
		}
		t.Logf("verified GitHub fragment: %s", id)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("/tmp/manja-margo-operation.png")}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`() => document.documentElement.classList.add('dark')`); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("/tmp/manja-margo-operation-dark.png")}); err != nil {
		t.Fatal(err)
	}
	// Regression for the runner-group visibility property reported in review.
	const runnerGroupID = "detail-sha256-c685048700ef665a432d5e348b48ac32efd431d3662d6a63ca307921ea4e9a7d"
	if _, err := page.Goto(server.URL + basePath + "catalogs/github/documents/github-v3-rest/?selected=" + runnerGroupID + "#" + runnerGroupID); err != nil {
		t.Fatal(err)
	}
	description := page.Locator(".gs-schema-tree-description").Filter(playwright.LocatorFilterOptions{HasText: "Visibility of a runner group."}).First()
	if err := description.Locator("code").First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	if codes, err := description.Locator("code").AllTextContents(); err != nil || strings.Join(codes, ",") != "all,selected,private" {
		t.Fatalf("visibility inline code: %v %v", codes, err)
	}
	if text, err := description.InnerText(); err != nil || strings.Contains(text, "`") {
		t.Fatalf("unrendered visibility Markdown: %q %v", text, err)
	}
	if err := description.ScrollIntoViewIfNeeded(); err != nil {
		t.Fatal(err)
	}
	if boxed, err := description.Evaluate(`el => { const box = el.closest('[data-manja-static-schema-fragment]'); if (!box) return true; const style = getComputedStyle(box); return ['borderTopWidth','borderRightWidth','borderBottomWidth','borderLeftWidth','paddingTop','paddingRight','paddingBottom','paddingLeft'].some(key => parseFloat(style[key]) !== 0); }`, nil); err != nil || boxed != false {
		t.Fatalf("schema fragment has a box or padding: %v %v", boxed, err)
	}
	pathParameters := page.Locator(`[id$="-path-parameters"].gs-schema-tree`)
	if count, err := pathParameters.Count(); err != nil || count != 1 {
		t.Fatalf("path SchemaTree: %d %v", count, err)
	}
	if boxed, err := pathParameters.Evaluate(`el => { const section = el.closest('section'); const heading = section.querySelector('h5'); return parseFloat(getComputedStyle(section).borderTopWidth) > 0 && parseFloat(getComputedStyle(heading).borderBottomWidth) > 0 && parseFloat(getComputedStyle(el).borderTopWidth) === 0; }`, nil); err != nil || boxed != true {
		t.Fatalf("path parameters need an outer card and divided header, without an inner tree border: %v %v", boxed, err)
	}
	if count, err := pathParameters.Locator(`[data-manja-parameter-row]`).Count(); err != nil || count == 0 {
		t.Fatalf("path parameter rows: %d %v", count, err)
	}
	if count, err := pathParameters.Locator(`[data-required="true"]`).Count(); err != nil || count == 0 {
		t.Fatalf("required path parameters: %d %v", count, err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("/tmp/manja-margo-schema.png")}); err != nil {
		t.Fatal(err)
	}
	if err := pathParameters.ScrollIntoViewIfNeeded(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("/tmp/manja-path-parameters.png")}); err != nil {
		t.Fatal(err)
	}
}
