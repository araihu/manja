package e2e

import (
	"context"
	"testing"

	"github.com/playwright-community/playwright-go"

	"github.com/araihu/manja/internal/core"
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
}

type recordingPublicationStore struct {
	publication core.Publication
}

func (s *recordingPublicationStore) SavePublication(_ context.Context, publication core.Publication) error {
	s.publication = publication
	return nil
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
