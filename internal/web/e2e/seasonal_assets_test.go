package e2e

import (
	"net/http"
	"sync"
	"testing"

	"github.com/mxschmitt/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

func TestSeasonalAssetsRuntimeFailurePreservesPreferenceAndFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	const runtimeURL = "https://araihu.com/assets/campaign/v1.js"
	server := httptestServer(t, web.NewPublicServer(core.SpecIndex{Title: "Petstore"}))

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
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{Width: 390, Height: 844},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddInitScript(playwright.Script{Content: playwright.String(`
		localStorage.setItem("theme", "minimal");
	`)}); err != nil {
		t.Fatal(err)
	}
	if err := page.Route(runtimeURL, func(route playwright.Route) {
		if err := route.Fulfill(playwright.RouteFulfillOptions{
			Status:      playwright.Int(http.StatusServiceUnavailable),
			Body:        "seasonal runtime unavailable",
			ContentType: playwright.String("text/plain"),
		}); err != nil {
			t.Errorf("fulfill seasonal runtime failure: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var pageErrors []string
	page.OnPageError(func(err error) {
		mu.Lock()
		pageErrors = append(pageErrors, err.Error())
		mu.Unlock()
	})
	response, err := page.Goto(server, playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateLoad})
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || response.Status() != http.StatusOK {
		t.Fatalf("public docs response = %v, want HTTP 200", response)
	}
	if _, err := page.WaitForFunction(`() =>
		document.documentElement.dataset.theme === "minimal" &&
		document.documentElement.dataset.themeSource === "preference"`, nil); err != nil {
		t.Fatalf("explicit theme preference did not remain authoritative: %v", err)
	}
	if got, err := page.Locator(`[data-asset-brand="logo"]`).GetAttribute("src"); err != nil || got != "/manja-assets/manja-mark.svg" {
		t.Fatalf("fallback logo src = %q, err = %v", got, err)
	}
	if got, err := page.Locator(`[data-asset-brand="icon"]`).GetAttribute("href"); err != nil || got != "/manja-assets/favicon.svg" {
		t.Fatalf("fallback favicon href = %q, err = %v", got, err)
	}
	if hidden, err := page.Locator(`[data-campaign-toggle]`).IsHidden(); err != nil || !hidden {
		t.Fatalf("campaign toggle hidden after runtime failure = %t, err = %v", hidden, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(pageErrors) != 0 {
		t.Fatalf("unexpected page errors: %v", pageErrors)
	}
}
