package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/playwright-community/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

func TestGoshtosoAffectedSurfaceVisualMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewServerWithOptions(goshtosoFallbackIndex(), web.Options{
		Management: web.ManagementOptions{
			Store: &visualMatrixPublicationStore{},
			Specs: []web.ManagedSpec{{
				ID:    "payments-api",
				Index: managementCandidateIndex(),
				Project: core.Project{
					ID:   "payments",
					Name: "Acme Payments API",
				},
				Source: core.Source{
					ID:        "local-git/payments-api.git",
					ProjectID: "payments",
					Kind:      "git",
					SpecPath:  "openapi.yaml",
				},
				Revision: core.Revision{
					ID:        "rev-candidate",
					SourceID:  "local-git/payments-api.git",
					Ref:       "main",
					CommitSHA: "64d8e2a013f76b5f28c9f14881065d3f6c4f8e17",
				},
				Candidates: []core.RevisionCandidate{{
					SourceID:  "local-git/payments-api.git",
					Ref:       "main",
					Kind:      "branch",
					CommitSHA: "64d8e2a013f76b5f28c9f14881065d3f6c4f8e17",
				}},
				Publication: core.Publication{
					ProjectID:  "payments",
					RevisionID: "rev-live",
					Public:     true,
					Path:       "/payments/v1",
				},
				SyncRecord: core.SyncRecord{
					ProjectID:  "payments",
					SourceID:   "local-git/payments-api.git",
					RevisionID: "rev-candidate",
					Result:     core.SyncResultSuccess,
					Trigger:    "e2e",
				},
			}},
		},
	}))

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
	browserContext, err := browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer browserContext.Close()
	visualDir := os.Getenv("MANJA_VISUAL_DIR")
	if visualDir != "" {
		if err := os.MkdirAll(visualDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	surfaces := []struct {
		name string
		path string
		root string
	}{
		{name: "public", path: "/?selected=operation-listpets#operation-listpets", root: "#main-content"},
		{name: "management", path: "/manage/spec/payments-api", root: "#management-main-content"},
	}
	for _, surface := range surfaces {
		for _, width := range []int{390, 1440} {
			for _, theme := range []string{"araihu", "manja", "goshtoso", "minimal"} {
				for _, dark := range []bool{false, true} {
					name := fmt.Sprintf("%s/%d/%s/dark-%t", surface.name, width, theme, dark)
					t.Run(name, func(t *testing.T) {
						page, err := browserContext.NewPage()
						if err != nil {
							t.Fatal(err)
						}
						defer page.Close()
						if err := page.SetViewportSize(width, 900); err != nil {
							t.Fatal(err)
						}
						if err := page.AddInitScript(playwright.Script{Content: playwright.String(fmt.Sprintf(`
							localStorage.setItem('theme', %q);
							localStorage.setItem('darkMode', %q);
						`, theme, fmt.Sprintf("%t", dark)))}); err != nil {
							t.Fatal(err)
						}

						var mu sync.Mutex
						var pageErrors []string
						var consoleErrors []string
						page.OnPageError(func(err error) {
							mu.Lock()
							pageErrors = append(pageErrors, err.Error())
							mu.Unlock()
						})
						page.On("console", func(message playwright.ConsoleMessage) {
							if message.Type() != "error" {
								return
							}
							mu.Lock()
							consoleErrors = append(consoleErrors, message.Text())
							mu.Unlock()
						})

						response, err := page.Goto(server+surface.path, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
						if err != nil {
							t.Fatal(err)
						}
						if response == nil || response.Status() != 200 {
							t.Fatalf("GET %s status = %v, want 200", surface.path, response)
						}
						if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
							t.Fatalf("await Goshtoso dependency readiness: %v", err)
						}
						if _, err := page.WaitForFunction(`() => !document.documentElement.classList.contains('boot')`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
							t.Fatalf("wait for visual boot state to settle: %v", err)
						}
						if err := page.Locator(surface.root).WaitFor(); err != nil {
							t.Fatal(err)
						}

						metrics, err := page.Evaluate(`({theme, dark, surface, width}) => {
							const root = document.documentElement;
							const publicSurface = surface === 'public';
							const desktopSidebar = document.querySelector(publicSurface ? 'aside[aria-label="API sections"]' : 'aside[aria-label="Management sections"]');
							const mobileSidebarTrigger = document.querySelector(publicSurface ? '[aria-label="Open API sections"]' : '[aria-label="Open management sections"]');
							const visible = (element) => Boolean(element && element.getClientRects().length > 0);
							const triggerBox = mobileSidebarTrigger?.getBoundingClientRect();
							const main = document.querySelector('#main-content');
							const mainBox = main?.getBoundingClientRect();
							return {
								themeMatches: root.dataset.theme === theme,
								darkMatches: root.classList.contains('dark') === dark,
								overflow: Math.max(root.scrollWidth, document.body.scrollWidth) > window.innerWidth,
								viewportMatches: window.innerWidth === width,
								responsiveShell: width === 390
									? !visible(desktopSidebar) && visible(mobileSidebarTrigger)
									: visible(desktopSidebar) && !visible(mobileSidebarTrigger),
								mobileTarget: width !== 390 || Boolean(triggerBox && triggerBox.width >= 44 && triggerBox.height >= 44),
								primaryScrollOwners: document.querySelectorAll('[data-manja-primary-scroll="true"]').length,
								nestedInteractive: document.querySelectorAll('a a, a button, button a, button button').length,
								mainHorizontalClipping: main?.scrollWidth > main?.clientWidth,
								wideElements: !mainBox ? [] : Array.from(main.querySelectorAll('*')).filter(element => {
									const box = element.getBoundingClientRect();
									return box.right > mainBox.right + 1 || box.width > mainBox.width + 1;
								}).slice(0, 8).map(element => ({
									tag: element.tagName,
									className: element.className?.toString().slice(0, 160),
									width: Math.round(element.getBoundingClientRect().width),
									right: Math.round(element.getBoundingClientRect().right),
								})),
							};
						}`, map[string]any{"theme": theme, "dark": dark, "surface": surface.name, "width": width})
						if err != nil {
							t.Fatal(err)
						}
						values, ok := metrics.(map[string]any)
						if !ok {
							t.Fatalf("visual matrix metrics should be a map, got %#v", metrics)
						}
						for _, key := range []string{"themeMatches", "darkMatches", "viewportMatches", "responsiveShell", "mobileTarget"} {
							if values[key] != true {
								t.Errorf("%s = %#v, want true; metrics=%#v", key, values[key], values)
							}
						}
						if values["overflow"] == true {
							t.Errorf("horizontal viewport overflow; metrics=%#v", values)
						}
						if values["mainHorizontalClipping"] == true {
							t.Errorf("main workspace clips horizontal content; metrics=%#v", values)
						}
						if fmt.Sprint(values["primaryScrollOwners"]) != "1" {
							t.Errorf("primary scroll owner count = %#v, want 1; metrics=%#v", values["primaryScrollOwners"], values)
						}
						if fmt.Sprint(values["nestedInteractive"]) != "0" {
							t.Errorf("nested interactive count = %#v, want 0; metrics=%#v", values["nestedInteractive"], values)
						}
						if visualDir != "" {
							path := filepath.Join(visualDir, fmt.Sprintf("%s-%d-%s-dark-%t.png", surface.name, width, theme, dark))
							if _, err := page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String(path)}); err != nil {
								t.Fatalf("capture visual evidence: %v", err)
							}
						}

						mu.Lock()
						defer mu.Unlock()
						if len(pageErrors) != 0 {
							t.Errorf("page errors = %v", pageErrors)
						}
						if len(consoleErrors) != 0 {
							t.Errorf("console errors = %v", consoleErrors)
						}
					})
				}
			}
		}
	}
}

type visualMatrixPublicationStore struct{}

func (*visualMatrixPublicationStore) SavePublication(context.Context, core.Publication) error {
	return nil
}
