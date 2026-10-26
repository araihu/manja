package selfhosted

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestOverviewFooterInHostedAndStaticBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping overview browser regression in short mode")
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
	for _, long := range []bool{false, true} {
		t.Run(fmt.Sprintf("long=%t", long), func(t *testing.T) {
			readme := overviewReadme
			if long {
				readme += strings.Repeat("\n\nMore documentation with enough detail to fill the page.", 80)
			}
			configPath := overviewRendererConfig(t, readme)
			hosted, _, err := NewRenderer(context.Background(), RendererOptions{ConfigPath: configPath})
			if err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "public")
			if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath, DataDir: t.TempDir()}, Output: output, BasePath: "/reference/"}); err != nil {
				t.Fatal(err)
			}
			for mode, handler := range map[string]http.Handler{
				"hosted": hosted,
				"static": http.StripPrefix("/reference", http.FileServer(http.Dir(output))),
			} {
				t.Run(mode, func(t *testing.T) {
					server := httptest.NewServer(handler)
					t.Cleanup(server.Close)
					base := ""
					if mode == "static" {
						base = "/reference"
					}
					page, err := browser.NewPage()
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = page.Close() })
					if err := page.EmulateMedia(playwright.PageEmulateMediaOptions{ReducedMotion: playwright.ReducedMotionReduce}); err != nil {
						t.Fatal(err)
					}
					// Exercise the shipped offline fallback without depending on CDN availability.
					if err := page.Route("https://**", func(route playwright.Route) { _ = route.Abort() }); err != nil {
						t.Fatal(err)
					}
					var errorMu sync.Mutex
					var pageErrors []string
					page.On("pageerror", func(err error) {
						errorMu.Lock()
						defer errorMu.Unlock()
						pageErrors = append(pageErrors, err.Error())
					})
					for _, route := range []string{"/", "/example/", "/example/search", "/example/documents/example/"} {
						if _, err := page.Goto(server.URL + base + route); err != nil {
							t.Fatal(err)
						}
						if mode == "static" && route != "/" {
							waitStaticExportReady(t, page)
						}
						if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`); err != nil {
							t.Fatal(err)
						}
						// Measure settled layouts and colors, including immediately after a theme change.
						if _, err := page.AddStyleTag(playwright.PageAddStyleTagOptions{Content: playwright.String(`*, *::before, *::after { transition: none !important; animation: none !important; }`)}); err != nil {
							t.Fatal(err)
						}
						if script := os.Getenv("MANJA_OVERVIEW_AXE_SCRIPT"); script != "" {
							if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(script)}); err != nil {
								t.Fatal(err)
							}
						}
						for _, viewport := range [][2]int{{390, 844}, {984, 1109}, {1440, 900}} {
							if err := page.SetViewportSize(viewport[0], viewport[1]); err != nil {
								t.Fatal(err)
							}
							for _, theme := range []string{"goshtoso", "minimal"} {
								for _, dark := range []bool{false, true} {
									if _, err := page.Evaluate(`settings => {
										document.documentElement.dataset.theme = settings.theme;
										document.documentElement.classList.toggle('dark', settings.dark);
									}`, map[string]any{"theme": theme, "dark": dark}); err != nil {
										t.Fatal(err)
									}
									label := fmt.Sprintf("%s %dx%d %s dark=%t", route, viewport[0], viewport[1], theme, dark)
									assertOverviewFooterLayout(t, page, label, long && route == "/")
									if os.Getenv("MANJA_OVERVIEW_AXE_SCRIPT") != "" {
										violations, err := page.Evaluate(`async () => {
											const results = await axe.run(document.getElementById('main-content'), {runOnly: {type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21aa']}});
											return results.violations.filter(v => v.impact === 'serious' || v.impact === 'critical').map(v => ({id: v.id, nodes: v.nodes.map(n => n.target)}));
										}`)
										if err != nil {
											t.Fatal(err)
										}
										if found, ok := violations.([]any); !ok || len(found) != 0 {
											t.Fatalf("%s accessibility: %#v", label, violations)
										}
									}
									if directory := os.Getenv("MANJA_OVERVIEW_SCREENSHOT_DIR"); directory != "" && viewport[0] != 984 {
										if err := os.MkdirAll(directory, 0o755); err != nil {
											t.Fatal(err)
										}
										name := fmt.Sprintf("%s-long-%t-%s-%d-%s-dark-%t.png", mode, long, strings.ReplaceAll(route, "/", "-"), viewport[0], theme, dark)
										if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(filepath.Join(directory, name)), Animations: playwright.ScreenshotAnimationsDisabled}); err != nil {
											t.Fatal(err)
										}
									}
								}
							}
						}
					}
					// Follow a real document navigation link and exercise history. Both
					// HTMX and static navigation replace the main content fragment.
					if err := page.SetViewportSize(1440, 900); err != nil {
						t.Fatal(err)
					}
					operationHref, err := page.Locator("[data-catalog-sidebar-operation]").First().GetAttribute("href")
					if err != nil || operationHref == "" {
						t.Fatalf("operation href = %q, %v", operationHref, err)
					}
					if _, err := page.Goto(server.URL + operationHref); err != nil {
						t.Fatal(err)
					}
					if mode == "static" {
						waitStaticExportReady(t, page)
					}
					if err := page.Locator("#catalog-sidebar-spec-overview").Click(); err != nil {
						t.Fatal(err)
					}
					if _, err := page.WaitForFunction(`() => document.querySelector('[data-catalog-document-header]') && !document.querySelector('[data-catalog-detail]')`, nil); err != nil {
						t.Fatal(err)
					}
					assertOverviewFooterLayout(t, page, "after document navigation", false)
					if _, err := page.GoBack(); err != nil {
						t.Fatal(err)
					}
					if _, err := page.WaitForFunction(`() => document.querySelector('#catalog-main-content [data-public-doc-identity]')`, nil); err != nil {
						t.Fatal(err)
					}
					assertOverviewFooterLayout(t, page, "after Back", false)
					if _, err := page.GoForward(); err != nil {
						t.Fatal(err)
					}
					if _, err := page.WaitForFunction(`() => document.querySelector('[data-catalog-document-header]') && !document.querySelector('[data-catalog-detail]')`, nil); err != nil {
						t.Fatal(err)
					}
					assertOverviewFooterLayout(t, page, "after Forward", false)
					if _, err := page.Reload(); err != nil {
						t.Fatal(err)
					}
					assertOverviewFooterLayout(t, page, "after reload", false)
					errorMu.Lock()
					defer errorMu.Unlock()
					if len(pageErrors) > 0 {
						t.Fatalf("unexpected page errors: %v", pageErrors)
					}
				})
			}
		})
	}
}

func assertOverviewFooterLayout(t *testing.T, page playwright.Page, label string, wantOverflow bool) {
	t.Helper()
	result, err := page.Evaluate(`wantOverflow => {
		const main = document.getElementById('main-content');
		const content = document.getElementById('catalog-main-content');
		const footer = content.querySelector('[data-manja-capability-footer]');
		main.scrollTop = 0;
		const box = main.getBoundingClientRect();
		const style = getComputedStyle(main);
		const f = footer.getBoundingClientRect();
		const overflow = main.scrollHeight > main.clientHeight + 2;
		const previous = footer.previousElementSibling.getBoundingClientRect();
		const errors = [];
		if (wantOverflow && !overflow) errors.push('long page does not scroll');
		if (!overflow && Math.abs(f.bottom - (box.bottom - parseFloat(style.paddingBottom))) > 2) errors.push('footer is above page bottom');
		if (f.top < previous.bottom + 63) errors.push('footer overlaps content or loses its gap');
		if (getComputedStyle(footer).position !== 'static') errors.push('footer is outside normal flow');
		if (main.scrollWidth > main.clientWidth + 2) errors.push('horizontal overflow');
		if (document.querySelectorAll('[data-manja-capability-footer]').length !== 1) errors.push('duplicate footer');
		main.scrollTop = main.scrollHeight;
		if (footer.getBoundingClientRect().bottom > box.bottom + 2) errors.push('footer is unreachable by scrolling');
		return {errors, overflow, footerBottom: f.bottom, mainBottom: box.bottom, paddingBottom: style.paddingBottom};
	}`, wantOverflow)
	if err != nil {
		t.Fatal(err)
	}
	value := result.(map[string]any)
	if errors := value["errors"].([]any); len(errors) != 0 {
		t.Fatalf("%s: %#v", label, result)
	}
}
