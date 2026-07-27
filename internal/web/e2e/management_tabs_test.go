package e2e

import (
	"context"
	"testing"

	"github.com/playwright-community/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

func TestManagementTabActionsInitializeSwappedContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewServerWithOptions(core.SpecIndex{}, web.Options{
		Management: web.ManagementOptions{
			Store: &recordingPublicationStore{},
			Specs: []web.ManagedSpec{{
				ID:             "payments-api",
				Index:          managementCandidateIndex(),
				PublishedIndex: managementPublishedIndex(),
				Project: core.Project{
					ID:   "payments",
					Name: "Acme Payments API",
					SEO:  core.ProjectSEO{Robots: "index,follow"},
				},
				Source: core.Source{
					ID:        "local-git/payments-api.git",
					ProjectID: "payments",
					Kind:      "git",
					SpecPath:  "openapi.yaml",
				},
				Revision: core.Revision{
					ID:          "rev-candidate",
					SourceID:    "local-git/payments-api.git",
					Ref:         "release/breaking-auth",
					CommitSHA:   "2da5e5002090fcc0cfb63194eeda0bc65c098f0d",
					AuthorName:  "Ada Lovelace",
					AuthorEmail: "ada@acme.test",
					Message:     "Require payment versioning and retire customers endpoint",
				},
				Candidates: []core.RevisionCandidate{
					{SourceID: "local-git/payments-api.git", Ref: "release/breaking-auth", Kind: "branch", CommitSHA: "2da5e5002090fcc0cfb63194eeda0bc65c098f0d"},
					{SourceID: "local-git/payments-api.git", Ref: "main", Kind: "branch", CommitSHA: "64d8e2a013f76b5f28c9f14881065d3f6c4f8e17"},
				},
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
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{Width: 1024, Height: 768},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/manage/spec/payments-api"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	if err := page.Locator(`[role="tab"]:has-text("Publish")`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Publish"})

	if _, err := page.ExpectResponse("**/manage/publication", func() error {
		return page.Locator(`button:has-text("Publish this revision")`).Click()
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Publish"})

	if err := page.Locator(`[role="tab"]:has-text("Route")`).Click(); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Route"})
	if err := page.Locator(`#management-main-content [role="tabpanel"][aria-label="Route"] input[name="path"]`).Fill("/payments/live"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.ExpectResponse("**/manage/publication", func() error {
		return page.Locator(`#management-main-content [role="tabpanel"][aria-label="Route"] button:has-text("Save route settings")`).Click()
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Publish"})

	if err := page.Locator(`[role="tab"]:has-text("Route")`).Click(); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Route"})
	focusManagementControl(t, page, `#management-visibility-public-payments-api`)
	assertKeyboardControlWithoutScroll(t, page, "ArrowRight", `() => {
		const control = document.querySelector('#management-visibility-private-payments-api');
		return Boolean(control && control.checked && document.activeElement === control);
	}`)

	if err := page.Locator(`[role="tab"]:has-text("Sync")`).Click(); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Sync"})
	focusManagementControl(t, page, `#management-payments-api-sync-publish`)
	assertKeyboardControlWithoutScroll(t, page, "Space", `() => {
		const control = document.querySelector('#management-payments-api-sync-publish');
		return Boolean(control && control.checked && document.activeElement === control);
	}`)
}

func TestManagementListFiltersAndSelectedIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewServerWithOptions(core.SpecIndex{}, web.Options{
		Management: web.ManagementOptions{Specs: []web.ManagedSpec{
			{
				ID:          "payments-api",
				Index:       core.SpecIndex{Title: "Payments API", Version: "v1"},
				Project:     core.Project{ID: "payments", Name: "Payments"},
				Source:      core.Source{ID: "payments-source", Kind: "git", SpecPath: "openapi/payments.yaml"},
				Revision:    core.Revision{ID: "payments-rev", Ref: "main"},
				Publication: core.Publication{Public: true, Path: "/payments/v1"},
			},
			{
				ID:          "billing-api",
				Index:       core.SpecIndex{Title: "Billing API", Version: "v2"},
				Project:     core.Project{ID: "billing", Name: "Billing"},
				Source:      core.Source{ID: "billing-source", Kind: "file", SpecPath: "billing.yaml"},
				Revision:    core.Revision{ID: "billing-rev", Ref: "main"},
				Publication: core.Publication{Public: false},
			},
		}},
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
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{Viewport: &playwright.Size{Width: 1024, Height: 768}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/manage/specs"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	if err := page.Locator(`#management-specs-table`).WaitFor(); err != nil {
		t.Fatal(err)
	}

	if err := page.Locator(`input[name="q"]`).Fill("missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.ExpectResponse("**/manage/specs?q=missing&status=", func() error {
		return page.Locator(`button:has-text("Apply filters")`).Click()
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-filtered-empty="true"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if got, err := page.Locator(`[data-management-results="true"]`).TextContent(); err != nil || got != "0 of 2 specs" {
		t.Fatalf("filtered result count = %q, err=%v", got, err)
	}

	if err := page.Locator(`a:has-text("Clear filters")`).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`#management-specs-table`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`() => { window.__managementNavigationSentinel = 'kept'; }`); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`aside[aria-label="Management sections"] a[href="/manage/spec/payments-api"]`).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-contract-identity="payments-api"]`).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() =>
		document.querySelector('main')?.dataset.selectedContract === 'payments-api' &&
		document.activeElement?.dataset.managementContractIdentity === 'payments-api' &&
		document.title === 'Payments API · Management'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("management selected identity did not settle: %v", err)
	}
	identity, err := page.Evaluate(`() => ({
		url: location.pathname,
		main: document.querySelector('main')?.dataset.selectedContract,
		content: document.querySelector('#management-main-content')?.dataset.selectedContract,
		heading: document.querySelector('[data-management-contract-identity]')?.dataset.managementContractIdentity,
		focus: document.activeElement?.dataset.managementContractIdentity,
		current: document.querySelector('aside[aria-label="Management sections"] [aria-current="page"]')?.getAttribute('href'),
		title: document.title,
		sentinel: window.__managementNavigationSentinel
	})`)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := identity.(map[string]any)
	if !ok {
		t.Fatalf("management identity should be a map, got %#v", identity)
	}
	for key, want := range map[string]string{
		"url":      "/manage/spec/payments-api",
		"main":     "payments-api",
		"content":  "payments-api",
		"heading":  "payments-api",
		"focus":    "payments-api",
		"current":  "/manage/spec/payments-api",
		"title":    "Payments API · Management",
		"sentinel": "kept",
	} {
		if metrics[key] != want {
			t.Fatalf("management identity %s = %#v, want %q; metrics=%#v", key, metrics[key], want, metrics)
		}
	}

	if _, err := page.GoBack(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-page-header="specs"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	back, err := page.Evaluate(`() => document.querySelector('main')?.dataset.selectedContract + '|' + document.querySelector('aside[aria-label="Management sections"] [aria-current="page"]')?.getAttribute('href')`)
	if err != nil {
		t.Fatal(err)
	}
	if back != "|/manage/specs" {
		t.Fatalf("management Back identity = %#v", back)
	}
}

type recordingPublicationStore struct {
	publication core.Publication
}

func (s *recordingPublicationStore) SavePublication(_ context.Context, publication core.Publication) error {
	s.publication = publication
	return nil
}

func assertKeyboardControlWithoutScroll(t *testing.T, page playwright.Page, key string, settled string) {
	t.Helper()

	before, err := page.Evaluate(`() => [window.scrollX, window.scrollY]`)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press(key); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(settled, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("control did not settle after %s: %v", key, err)
	}
	unchanged, err := page.Evaluate(`(before) => window.scrollX === before[0] && window.scrollY === before[1]`, before)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != true {
		t.Fatalf("%s should operate the focused control without scrolling; before=%#v", key, before)
	}
}

func focusManagementControl(t *testing.T, page playwright.Page, selector string) {
	t.Helper()

	focused, err := page.Evaluate(`(selector) => {
		const control = document.querySelector(selector);
		if (!control) return false;
		control.focus();
		return document.activeElement === control;
	}`, selector)
	if err != nil {
		t.Fatal(err)
	}
	if focused != true {
		metrics, metricsErr := page.Evaluate(`(selector) => {
			const control = document.querySelector(selector);
			const panel = control && control.closest('[role="tabpanel"]');
			const bounds = control && control.getBoundingClientRect();
			return {
				exists: Boolean(control),
				disabled: Boolean(control && control.disabled),
				hidden: Boolean(control && control.hidden),
				offsetParent: Boolean(control && control.offsetParent),
				panelDisplay: panel ? getComputedStyle(panel).display : '',
				panelInert: Boolean(panel && panel.inert),
				width: bounds ? bounds.width : 0,
				height: bounds ? bounds.height : 0,
			};
		}`, selector)
		t.Fatalf("management control %s should accept browser focus; metrics=%#v err=%v", selector, metrics, metricsErr)
	}
}

func assertVisibleManagementTabPanels(t *testing.T, page playwright.Page, want []string) {
	t.Helper()

	if err := page.Locator(`#management-main-content [role="tabpanel"]`).First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateAttached,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`(want) => {
		const visible = Array.from(document.querySelectorAll('#management-main-content [role="tabpanel"]'))
			.filter((panel) => getComputedStyle(panel).display !== 'none')
			.map((panel) => panel.getAttribute('aria-label'));
		return JSON.stringify(visible) === JSON.stringify(want);
	}`, want, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("visible management tabs did not settle to %#v: %v; metrics %#v", want, err, managementTabMetrics(t, page))
	}
	metrics := managementTabMetrics(t, page)
	if numericValue(metrics["mainContentCount"]) != 1 {
		t.Fatalf("management swap should keep one main fragment, got metrics %#v", metrics)
	}
	if metrics["nestedMainContent"] == true {
		t.Fatalf("management swap nested a replacement fragment, got metrics %#v", metrics)
	}
}

func managementTabMetrics(t *testing.T, page playwright.Page) map[string]any {
	t.Helper()

	result, err := page.Evaluate(`() => {
		const visible = Array.from(document.querySelectorAll('#management-main-content [role="tabpanel"]'))
			.filter((panel) => getComputedStyle(panel).display !== 'none')
			.map((panel) => panel.getAttribute('aria-label'));
		return {
			visible,
			mainContentCount: document.querySelectorAll('#management-main-content').length,
			nestedMainContent: !!document.querySelector('#management-main-content #management-main-content'),
			contractVisible: Array.from(document.querySelectorAll('#management-main-content [role="tabpanel"]'))
				.some((panel) => getComputedStyle(panel).display !== 'none' && panel.innerText.includes('Contract check')),
		};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("management tab metrics should be a map, got %#v", result)
	}
	return metrics
}

func managementPublishedIndex() core.SpecIndex {
	return core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev-live",
		Title:      "Acme Payments API",
		Version:    "2024-10-01",
		Operations: []core.Operation{{
			ID:     "listPayments",
			Method: "GET",
			Path:   "/payments",
			Parameters: []core.OperationParameter{{
				Name:     "expand",
				In:       "query",
				Required: false,
				Schema:   core.SchemaSummary{Type: "string"},
			}},
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Payments returned",
			}, {
				Status:      "404",
				Description: "Customer account was not found",
			}},
		}, {
			ID:     "listCustomers",
			Method: "GET",
			Path:   "/customers",
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Customers returned",
			}},
		}},
		Schemas: []core.Schema{{
			Name: "Customer",
			Summary: core.SchemaSummary{
				Name: "Customer",
				Type: "object",
			},
		}},
	}
}

func managementCandidateIndex() core.SpecIndex {
	return core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev-candidate",
		Title:      "Acme Payments API",
		Version:    "2025-02-15",
		Operations: []core.Operation{{
			ID:     "listPayments",
			Method: "GET",
			Path:   "/payments",
			Parameters: []core.OperationParameter{{
				Name:     "expand",
				In:       "query",
				Required: true,
				Schema:   core.SchemaSummary{Type: "string"},
			}, {
				Name:     "api-version",
				In:       "header",
				Required: true,
				Schema:   core.SchemaSummary{Type: "string"},
			}},
			Responses: []core.OperationResponse{{
				Status:      "200",
				Description: "Payments returned",
			}, {
				Status:      "202",
				Description: "Payments export queued",
			}},
		}, {
			ID:     "createPaymentIntent",
			Method: "POST",
			Path:   "/payment-intents",
			Responses: []core.OperationResponse{{
				Status:      "202",
				Description: "Payment intent queued",
			}},
		}},
		Schemas: []core.Schema{{
			Name: "Refund",
			Summary: core.SchemaSummary{
				Name: "Refund",
				Type: "object",
			},
		}},
	}
}
