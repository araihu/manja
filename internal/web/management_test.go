package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web/templates"
)

func TestManagementSpecsUsesPageHeaderToolbarAndTable(t *testing.T) {
	body := renderManagementRequest(t, managementStructureServer(), "/manage/specs", http.StatusOK)

	for _, want := range []string{
		`data-management-page-header="specs"`,
		`role="toolbar"`,
		`aria-label="Managed spec tools"`,
		`id="management-specs-table"`,
		`data-management-results="true"`,
		`aria-live="polite"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("management specs list missing %q:\n%s", want, body)
		}
	}
}

func TestManagementSpecsEmptyAndFilteredEmptyDiffer(t *testing.T) {
	var emptyOutput strings.Builder
	if err := templates.ManagementSpecsPage(templates.ManagementOverviewModel{}).Render(context.Background(), &emptyOutput); err != nil {
		t.Fatal(err)
	}
	emptyBody := emptyOutput.String()
	for _, want := range []string{`data-management-empty="true"`, `No managed specs`} {
		if !strings.Contains(emptyBody, want) {
			t.Fatalf("zero-data empty state missing %q:\n%s", want, emptyBody)
		}
	}

	filteredBody := renderManagementRequest(t, managementStructureServer(), "/manage/specs?q=missing", http.StatusOK)
	for _, want := range []string{`data-management-filtered-empty="true"`, `No specs match these filters`, `href="/manage/specs"`} {
		if !strings.Contains(filteredBody, want) {
			t.Fatalf("filtered empty state missing %q:\n%s", want, filteredBody)
		}
	}
	if strings.Contains(filteredBody, `data-management-empty="true"`) {
		t.Fatalf("filtered empty state must remain distinct from zero-data state:\n%s", filteredBody)
	}
}

func TestManagementTableRowsKeepNativeLinkAndActionsSeparate(t *testing.T) {
	body := renderManagementRequest(t, managementStructureServer(), "/manage/specs", http.StatusOK)

	nativeLink := regexp.MustCompile(`(?s)<a[^>]*href="/manage/spec/payments-api"[^>]*>.*?Payments API.*?</a>`)
	if !nativeLink.MatchString(body) {
		t.Fatalf("management table should expose a native detail link:\n%s", body)
	}
	if !strings.Contains(body, `>View published docs</a>`) {
		t.Fatalf("management table should keep the published-docs action separate:\n%s", body)
	}
	if regexp.MustCompile(`<tr[^>]*(hx-get|x-on:click|onclick)=`).MatchString(body) {
		t.Fatalf("management rows with actions must not be row-click handlers:\n%s", body)
	}
}

func TestManagementSpecUsesPageHeaderAndDominantWorkspace(t *testing.T) {
	body := renderManagementRequest(t, managementStructureServer(), "/manage/spec/payments-api", http.StatusOK)

	for _, want := range []string{
		`data-management-page-header="detail"`,
		`data-management-contract-identity="payments-api"`,
		`data-management-detail-workspace="true"`,
		`Payments API`,
		`role="tablist"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("management detail workspace missing %q:\n%s", want, body)
		}
	}
	if got := strings.Count(body, "<h1"); got != 1 {
		t.Fatalf("management detail h1 count = %d, want 1:\n%s", got, body)
	}
}

func TestManagementUnknownSpecKeepsApplicationShell(t *testing.T) {
	body := renderManagementRequest(t, managementStructureServer(), "/manage/spec/unknown-api", http.StatusNotFound)

	for _, want := range []string{
		`id="main-content"`,
		`id="management-main-content"`,
		`data-selected-contract="unknown-api"`,
		`data-management-spec-not-found="true"`,
		`Spec not found`,
		`href="/manage/specs"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("unknown management spec should retain shell marker %q:\n%s", want, body)
		}
	}
}

func TestManagementSelectedIdentityIsServerAuthored(t *testing.T) {
	body := renderManagementRequest(t, managementStructureServer(), "/manage/spec/payments-api", http.StatusOK)

	for _, want := range []string{
		`data-selected-contract="payments-api"`,
		`data-document-title="Payments API · Management"`,
		`data-management-contract-identity="payments-api"`,
		`aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("management selected identity missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{`activeItemClasses`, `topIconActive`, `tabClassNames`} {
		if strings.Contains(body, reject) {
			t.Fatalf("management selection must not copy Goshtoso private classes %q:\n%s", reject, body)
		}
	}
}

func managementStructureServer() http.Handler {
	return NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{
		Specs: []ManagedSpec{
			{
				ID:          "payments-api",
				Index:       core.SpecIndex{Title: "Payments API", Version: "v1"},
				Project:     core.Project{ID: "payments", Name: "Payments"},
				Source:      core.Source{ID: "payments-source", Kind: "git", SpecPath: "openapi/payments.yaml"},
				Revision:    core.Revision{ID: "payments-rev", Ref: "main", CommitSHA: "abc123"},
				Publication: core.Publication{Public: true, Path: "/payments/v1"},
				SyncRecord:  core.SyncRecord{Result: core.SyncResultSuccess},
			},
			{
				ID:          "billing-api",
				Index:       core.SpecIndex{Title: "Billing API", Version: "v2"},
				Project:     core.Project{ID: "billing", Name: "Billing"},
				Source:      core.Source{ID: "billing-source", Kind: "file", SpecPath: "billing.yaml"},
				Revision:    core.Revision{ID: "billing-rev", Ref: "main"},
				Publication: core.Publication{Public: false},
			},
		},
	}})
}

func renderManagementRequest(t *testing.T, handler http.Handler, path string, wantStatus int) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s status = %d, want %d: %s", path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec.Body.String()
}

func TestManagementOverviewShowsProjectSyncAndPublicationState(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev1",
		Title:      "Payments API",
		Version:    "2026-06-09",
		Operations: []core.Operation{
			{ID: "create-payment", Method: "POST", Path: "/payments"},
			{ID: "get-payment", Method: "GET", Path: "/payments/{payment_id}"},
		},
		Schemas:      []core.Schema{{Name: "Payment"}},
		PublicRoutes: []core.PublicRoute{{Path: "/", Title: "Payments API"}, {Path: "/?selected=operation-create-payment#operation-create-payment", Title: "Create payment"}},
	}, Options{
		Management: ManagementOptions{
			Project: core.Project{
				ID:   "payments",
				Name: "Payments",
				Slug: "payments",
				SEO: core.ProjectSEO{
					CanonicalBase: "https://docs.example.test/payments",
					Robots:        "index,follow",
				},
				Theme: core.ThemeSettings{Theme: "manja", DarkMode: "auto"},
			},
			Source: core.Source{
				ID:        "source1",
				ProjectID: "payments",
				Kind:      "file",
				SpecPath:  "openapi/payments.yaml",
			},
			Revision: core.Revision{
				ID:         "rev1",
				ContractID: "payments",
				SourceID:   "source1",
				Ref:        "main",
				CommitSHA:  "abc123",
				Version:    "2026-06-09",
			},
			Publication: core.Publication{
				ProjectID:  "payments",
				RevisionID: "rev1",
				Public:     true,
				Path:       "/payments/v1",
			},
			SyncRecord: core.SyncRecord{
				ProjectID:  "payments",
				SourceID:   "source1",
				RevisionID: "rev1",
				Trigger:    "startup",
				Result:     core.SyncResultSuccess,
				SpecPath:   "openapi/payments.yaml",
				FinishedAt: time.Date(2026, 6, 9, 12, 30, 0, 0, time.UTC),
			},
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Management",
		"Payments",
		"Payments API",
		"2026-06-09",
		"source1",
		"file",
		"openapi/payments.yaml",
		"main",
		"abc123",
		"Public",
		"/payments/v1",
		"https://docs.example.test/payments",
		"index,follow",
		"startup",
		"success",
		"2026-06-09 12:30 UTC",
		`id="management-specs-table"`,
		`data-management-nav="top"`,
		`hx-target="#management-main-content"`,
		`action="/manage/publication"`,
		`id="management-public-path-payments-source1-rev1"`,
		`name="visibility"`,
		`value="public"`,
		`value="private"`,
		`name="path"`,
		`value="/payments/v1"`,
		"Production docs",
		"Live route",
		"Published revision",
		"Publish candidate",
		"Ready revision",
		"Spec diff",
		"Contract check",
		"No contract breaks",
		`name="revision_id"`,
		`value="rev1"`,
		"Publish this revision",
		"Route settings",
		"Configured path",
		"Canonical base",
		"Robots",
		"Save route settings",
		"Release safety",
		"Release history",
		"Readers are pinned to this docs revision until another candidate is promoted.",
		"Endpoints",
		"Schemas",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for label, value := range map[string]string{"Endpoints": "2", "Schemas": "1"} {
		pattern := regexp.MustCompile(`(?s)<dt[^>]*>` + regexp.QuoteMeta(label) + `</dt>\s*<dd[^>]*>` + regexp.QuoteMeta(value) + `</dd>`)
		if !pattern.MatchString(body) {
			t.Fatalf("management metric %q should have value %q in its description list:\n%s", label, value, body)
		}
	}
	for _, reject := range []string{`href="/">View docs`} {
		if strings.Contains(body, reject) {
			t.Fatalf("body should use Goshtoso component vocabulary instead of %q:\n%s", reject, body)
		}
	}
}

func TestManagementOverviewShowsCandidateContractDiff(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Specs: []ManagedSpec{{
				ID: "payments-api",
				Index: core.SpecIndex{
					ProjectID:  "payments",
					RevisionID: "rev-candidate",
					Title:      "Payments API",
					Version:    "v2",
					Operations: []core.Operation{
						{
							Method: "GET",
							Path:   "/payments",
							Parameters: []core.OperationParameter{
								{Name: "expand", In: "query", Required: true},
								{Name: "version", In: "query", Required: true},
							},
							Responses: []core.OperationResponse{{Status: "200"}, {Status: "202"}},
						},
						{Method: "POST", Path: "/payments"},
					},
					Schemas: []core.Schema{{Name: "Payment"}, {Name: "Refund"}},
				},
				PublishedIndex: core.SpecIndex{
					ProjectID:  "payments",
					RevisionID: "rev-live",
					Title:      "Payments API",
					Version:    "v1",
					Operations: []core.Operation{
						{Method: "GET", Path: "/customers", Responses: []core.OperationResponse{{Status: "200"}}},
						{
							Method:     "GET",
							Path:       "/payments",
							Parameters: []core.OperationParameter{{Name: "expand", In: "query"}},
							Responses:  []core.OperationResponse{{Status: "200"}, {Status: "404"}},
						},
					},
					Schemas: []core.Schema{{Name: "Customer"}, {Name: "Payment"}},
				},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source:  core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision: core.Revision{
					ID:          "rev-candidate",
					SourceID:    "repo-payments",
					Ref:         "release/v2",
					CommitSHA:   "def456",
					AuthorName:  "Ada Lovelace",
					AuthorEmail: "ada@acme.test",
					Message:     "Require payment versioning",
				},
				Publication: core.Publication{ProjectID: "payments", RevisionID: "rev-live", Public: true, Path: "/payments/v1"},
				SyncRecord:  core.SyncRecord{ProjectID: "payments", SourceID: "repo-payments", RevisionID: "rev-candidate", Trigger: "manual", Result: core.SyncResultSuccess},
			}},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Spec diff",
		"Contract check",
		"Candidate has contract-breaking changes against the production revision.",
		"5 breaking",
		"5 changes",
		"3 changes",
		"Removed endpoint",
		"GET /customers",
		"Parameter became required",
		"GET /payments expand (query)",
		"Required parameter added",
		"GET /payments version (query)",
		"Response status removed",
		"GET /payments 404",
		"Removed schema",
		"Customer",
		"Added endpoint",
		"POST /payments",
		"Response status added",
		"GET /payments 202",
		"Added schema",
		"Refund",
		"Ada Lovelace",
		"ada@acme.test",
		"Require payment versioning",
		"Candidate: Ada Lovelace",
		`name="revision_id"`,
		`value="rev-candidate"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestManagementOverviewShowsContractBaselineMessageBeforeFirstPublish(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev1",
		Title:      "Payments API",
	}, Options{
		Management: ManagementOptions{
			Project:  core.Project{ID: "payments", Name: "Payments"},
			Revision: core.Revision{ID: "rev1"},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Spec diff",
		"Contract check",
		"No baseline",
		"Publish once to create a production baseline for contract checks.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestManagementOverviewRejectsNonGET(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
	}
}

func TestManagementHomeListsMultipleSpecsAndUpdatesChosenPublication(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store: store,
			Specs: []ManagedSpec{
				{
					ID: "github-rest",
					Index: core.SpecIndex{
						ProjectID:  "github",
						RevisionID: "rev-github",
						Title:      "GitHub REST API",
						Version:    "2022-11-28",
					},
					Project: core.Project{ID: "github", Name: "GitHub", Slug: "github"},
					Source: core.Source{
						ID:        "repo-github",
						ProjectID: "github",
						Kind:      "git",
						SpecPath:  "openapi/github.yaml",
					},
					Revision: core.Revision{
						ID:        "rev-github",
						SourceID:  "repo-github",
						Ref:       "main",
						CommitSHA: "abc123",
					},
					Publication: core.Publication{
						ProjectID:  "github",
						RevisionID: "rev-github",
						Public:     true,
						Path:       "/github/v3",
					},
					SyncRecord: core.SyncRecord{
						ProjectID:  "github",
						SourceID:   "repo-github",
						RevisionID: "rev-github",
						Trigger:    "startup",
						Result:     core.SyncResultSuccess,
						SpecPath:   "openapi/github.yaml",
						FinishedAt: time.Date(2026, 6, 9, 12, 30, 0, 0, time.UTC),
					},
				},
				{
					ID: "billing-events",
					Index: core.SpecIndex{
						ProjectID:  "billing",
						RevisionID: "rev-billing",
						Title:      "Billing Events API",
						Version:    "v2",
					},
					Project: core.Project{ID: "billing", Name: "Billing", Slug: "billing"},
					Source: core.Source{
						ID:        "spec-billing",
						ProjectID: "billing",
						Kind:      "file",
						SpecPath:  "specs/billing.yaml",
					},
					Revision: core.Revision{
						ID:       "rev-billing",
						SourceID: "spec-billing",
						Ref:      "release/v2",
					},
					Publication: core.Publication{
						ProjectID:  "billing",
						RevisionID: "rev-billing",
						Public:     false,
						Path:       "/billing/events",
					},
					SyncRecord: core.SyncRecord{
						ProjectID:    "billing",
						SourceID:     "spec-billing",
						RevisionID:   "rev-billing",
						Trigger:      "manual",
						Result:       core.SyncResultFailure,
						SpecPath:     "specs/billing.yaml",
						ErrorSummary: "missing schema",
					},
				},
			},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Managed specs",
		"2 specs",
		"GitHub REST API",
		"Billing Events API",
		"repo-github",
		"spec-billing",
		"git",
		"file",
		"openapi/github.yaml",
		"specs/billing.yaml",
		"abc123",
		"release/v2",
		"Public",
		"Private",
		"missing schema",
		`name="spec_id"`,
		`value="github-rest"`,
		`href="/manage/spec/billing-events"`,
		`id="management-public-path-github-rest"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}

	form := url.Values{
		"spec_id":    {"billing-events"},
		"visibility": {"public"},
		"path":       {"/billing/v2"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if store.saved.ProjectID != "billing" || store.saved.RevisionID != "rev-billing" || !store.saved.Public || store.saved.Path != "/billing/v2" {
		t.Fatalf("saved publication = %#v", store.saved)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)
	body = rec.Body.String()
	if !strings.Contains(body, `/billing/v2`) {
		t.Fatalf("updated overview missing billing publication path:\n%s", body)
	}
	if !strings.Contains(body, `/github/v3`) {
		t.Fatalf("updated overview should retain github publication path:\n%s", body)
	}
}

func TestManagementHTMXRoutesRenderFragments(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Specs: []ManagedSpec{{
				ID:      "payments-api",
				Index:   core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API", Version: "v1"},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source:  core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision: core.Revision{
					ID:        "rev-main",
					SourceID:  "repo-payments",
					Ref:       "main",
					CommitSHA: "abc123",
				},
				Publication: core.Publication{ProjectID: "payments", RevisionID: "rev-main", Public: true, Path: "/payments/v1"},
			}},
		},
	})

	for _, path := range []string{"/manage", "/manage/specs", "/manage/spec/payments-api"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("HX-Request", "true")
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`id="management-main-content"`,
			"Payments API",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s fragment missing %q:\n%s", path, want, body)
			}
		}
		for _, reject := range []string{
			"<!doctype html>",
			`id="management-sidebar-content"`,
			`data-boot-anim="header"`,
		} {
			if strings.Contains(body, reject) {
				t.Fatalf("%s fragment should not include shell marker %q:\n%s", path, reject, body)
			}
		}
	}
}

func TestManagementPublicationPostSavesPublicationAndUpdatesOverview(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev1",
		Title:      "Payments API",
	}, Options{
		Management: ManagementOptions{
			Store: store,
			Project: core.Project{
				ID:   "payments",
				Name: "Payments",
			},
			Source: core.Source{
				ID:        "source1",
				ProjectID: "payments",
				Kind:      "file",
				SpecPath:  "openapi/payments.yaml",
			},
			Revision: core.Revision{
				ID:       "rev1",
				SourceID: "source1",
				Ref:      "main",
			},
		},
	})
	form := url.Values{
		"visibility": {"public"},
		"path":       {"/payments/v1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/manage/spec/payments-source1-rev1" {
		t.Fatalf("Location = %q, want /manage/spec/payments-source1-rev1", got)
	}
	if store.saved.ProjectID != "payments" || store.saved.RevisionID != "rev1" || !store.saved.Public || store.saved.Path != "/payments/v1" {
		t.Fatalf("saved publication = %#v", store.saved)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("overview status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Public", "/payments/v1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("overview missing %q:\n%s", want, body)
		}
	}
}

func TestManagementPublicationPostPromotesExplicitCandidateRevision(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store:   store,
			Project: core.Project{ID: "payments"},
			Revision: core.Revision{
				ID: "rev-candidate",
			},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "rev-live", Public: true, Path: "/payments/v1"},
		},
	})
	form := url.Values{
		"visibility":  {"public"},
		"path":        {"/payments/v1"},
		"revision_id": {"rev-candidate"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if store.saved.RevisionID != "rev-candidate" {
		t.Fatalf("saved revision = %q, want rev-candidate", store.saved.RevisionID)
	}
}

func TestManagementSyncPostSyncsSelectedGitRefAndCanPublish(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	var syncedRef string
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store: store,
			SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
				syncedRef = ref
				spec.Index = core.SpecIndex{
					ProjectID:  "payments",
					RevisionID: "rev-release",
					Title:      "Payments Release API",
					Version:    "v2",
				}
				spec.Revision = core.Revision{
					ID:        "rev-release",
					SourceID:  "repo-payments",
					Ref:       ref,
					CommitSHA: "def456",
				}
				spec.SyncRecord = core.SyncRecord{
					ProjectID:  "payments",
					SourceID:   "repo-payments",
					RevisionID: "rev-release",
					Trigger:    "manual",
					Result:     core.SyncResultSuccess,
				}
				return spec, nil
			},
			Specs: []ManagedSpec{{
				ID:      "payments-api",
				Index:   core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API", Version: "v1"},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source: core.Source{
					ID:        "repo-payments",
					ProjectID: "payments",
					Kind:      "git",
					SpecPath:  "docs/openapi.yaml",
				},
				Revision: core.Revision{ID: "rev-main", SourceID: "repo-payments", Ref: "main", CommitSHA: "abc123"},
				Candidates: []core.RevisionCandidate{
					{SourceID: "repo-payments", Ref: "main", Kind: "branch", CommitSHA: "abc123"},
					{SourceID: "repo-payments", Ref: "release/v2", Kind: "branch", CommitSHA: "def456"},
					{SourceID: "repo-payments", Ref: "v1.0.0", Kind: "tag", CommitSHA: "abc123"},
				},
			}},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Available refs",
		`action="/manage/sync"`,
		`hx-post="/manage/sync"`,
		`id="management-payments-api-sync-ref-trigger"`,
		`name="ref"`,
		`role="switch"`,
		`name="publish"`,
		`value="public"`,
		"Sync selected ref",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if !selectConfigContainsValues(body, "main", "release/v2", "v1.0.0") {
		t.Fatalf("available refs missing from encoded select configuration:\n%s", body)
	}

	form := url.Values{
		"spec_id": {"payments-api"},
		"ref":     {"release/v2"},
		"publish": {"public"},
		"path":    {"/payments/v2"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("sync status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if syncedRef != "release/v2" {
		t.Fatalf("synced ref = %q", syncedRef)
	}
	if store.saved.ProjectID != "payments" || store.saved.RevisionID != "rev-release" || !store.saved.Public || store.saved.Path != "/payments/v2" {
		t.Fatalf("saved publication = %#v", store.saved)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)
	body = rec.Body.String()
	for _, want := range []string{"Payments Release API", "release/v2", "def456", "/payments/v2", ">Public<"} {
		if !strings.Contains(body, want) {
			t.Fatalf("updated overview missing %q:\n%s", want, body)
		}
	}
}

func TestManagementPublicationPostCanReturnHTMXFragment(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev1",
		Title:      "Payments API",
	}, Options{
		Management: ManagementOptions{
			Store:    store,
			Project:  core.Project{ID: "payments", Name: "Payments"},
			Source:   core.Source{ID: "source1", ProjectID: "payments", Kind: "file", SpecPath: "openapi/payments.yaml"},
			Revision: core.Revision{ID: "rev1", SourceID: "source1", Ref: "main"},
		},
	})
	form := url.Values{
		"visibility": {"public"},
		"path":       {"/payments/v1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("HX-Push-Url"); got != "/manage/spec/payments-source1-rev1" {
		t.Fatalf("HX-Push-Url = %q, want /manage/spec/payments-source1-rev1", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="management-main-content"`,
		"Payments API",
		"/payments/v1",
		"View docs",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fragment missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{"<!doctype html>", `id="management-sidebar-content"`} {
		if strings.Contains(body, reject) {
			t.Fatalf("fragment should not include shell marker %q:\n%s", reject, body)
		}
	}
}

func TestManagementSyncPostCanReturnHTMXFragment(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store: store,
			SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
				spec.Index = core.SpecIndex{ProjectID: "payments", RevisionID: "rev-release", Title: "Payments Release API", Version: "v2"}
				spec.Revision = core.Revision{ID: "rev-release", SourceID: "repo-payments", Ref: ref, CommitSHA: "def456"}
				spec.SyncRecord = core.SyncRecord{ProjectID: "payments", SourceID: "repo-payments", RevisionID: "rev-release", Trigger: "manual", Result: core.SyncResultSuccess}
				return spec, nil
			},
			Specs: []ManagedSpec{{
				ID:         "payments-api",
				Index:      core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API", Version: "v1"},
				Project:    core.Project{ID: "payments", Name: "Payments"},
				Source:     core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision:   core.Revision{ID: "rev-main", SourceID: "repo-payments", Ref: "main", CommitSHA: "abc123"},
				Candidates: []core.RevisionCandidate{{SourceID: "repo-payments", Ref: "main", Kind: "branch", CommitSHA: "abc123"}, {SourceID: "repo-payments", Ref: "release/v2", Kind: "branch", CommitSHA: "def456"}},
			}},
		},
	})

	form := url.Values{
		"spec_id": {"payments-api"},
		"ref":     {"release/v2"},
		"publish": {"public"},
		"path":    {"/payments/v2"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("HX-Push-Url"); got != "/manage/spec/payments-api" {
		t.Fatalf("HX-Push-Url = %q, want /manage/spec/payments-api", got)
	}
	if store.saved.ProjectID != "payments" || store.saved.RevisionID != "rev-release" || !store.saved.Public || store.saved.Path != "/payments/v2" {
		t.Fatalf("saved publication = %#v", store.saved)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="management-main-content"`,
		"Payments Release API",
		"release/v2",
		"def456",
		"/payments/v2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fragment missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("fragment should not include full shell:\n%s", body)
	}
}

func TestManagementSyncPostKeepsSyncedStateWhenPublicationSaveFails(t *testing.T) {
	store := &fakeManagementPublicationStore{err: errors.New("unsafe path")}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store: store,
			SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
				spec.Index = core.SpecIndex{
					ProjectID:  "payments",
					RevisionID: "rev-release",
					Title:      "Payments Release API",
				}
				spec.Revision = core.Revision{
					ID:        "rev-release",
					SourceID:  "repo-payments",
					Ref:       ref,
					CommitSHA: "def456",
				}
				return spec, nil
			},
			Specs: []ManagedSpec{{
				ID:      "payments-api",
				Index:   core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source:  core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision: core.Revision{
					ID:        "rev-main",
					SourceID:  "repo-payments",
					Ref:       "main",
					CommitSHA: "abc123",
				},
				Candidates: []core.RevisionCandidate{
					{SourceID: "repo-payments", Ref: "main", Kind: "branch", CommitSHA: "abc123"},
					{SourceID: "repo-payments", Ref: "release/v2", Kind: "branch", CommitSHA: "def456"},
				},
			}},
		},
	})

	form := url.Values{
		"spec_id": {"payments-api"},
		"ref":     {"release/v2"},
		"publish": {"public"},
		"path":    {"unsafe"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Manja-Application-Status"); got != "persistence-error" {
		t.Fatalf("application status = %q, want persistence-error", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"Payments Release API", "release/v2", "def456"} {
		if !strings.Contains(body, want) {
			t.Fatalf("updated overview missing %q after publish failure:\n%s", want, body)
		}
	}
}

func TestManagementSyncPostPassesDeadlineToSyncAction(t *testing.T) {
	var hasDeadline bool
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			SyncAction: func(ctx context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
				_, hasDeadline = ctx.Deadline()
				return spec, nil
			},
			Specs: []ManagedSpec{{
				ID:      "payments-api",
				Index:   core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source:  core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision: core.Revision{
					ID:        "rev-main",
					SourceID:  "repo-payments",
					Ref:       "main",
					CommitSHA: "abc123",
				},
				Candidates: []core.RevisionCandidate{
					{SourceID: "repo-payments", Ref: "main", Kind: "branch", CommitSHA: "abc123"},
				},
			}},
		},
	})

	form := url.Values{
		"spec_id": {"payments-api"},
		"ref":     {"main"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("sync status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if !hasDeadline {
		t.Fatal("sync action context should have a deadline")
	}
}

func TestManagementOverviewRendersOneSyncFormForSelectedSpec(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Specs: []ManagedSpec{{
				ID:      "payments-api",
				Index:   core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source:  core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision: core.Revision{
					ID:        "rev-main",
					SourceID:  "repo-payments",
					Ref:       "main",
					CommitSHA: "abc123",
				},
				Candidates: []core.RevisionCandidate{
					{SourceID: "repo-payments", Ref: "main", Kind: "branch", CommitSHA: "abc123"},
					{SourceID: "repo-payments", Ref: "release/v2", Kind: "branch", CommitSHA: "def456"},
				},
			}},
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manage", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if count := strings.Count(rec.Body.String(), `action="/manage/sync"`); count != 1 {
		t.Fatalf("sync form count = %d, body:\n%s", count, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<legend`,
		`>Visibility</legend>`,
		`type="submit"`,
		`Publish this revision`,
		`Save route settings`,
		`Sync selected ref`,
		`hx-disabled-elt="find button[type='submit']"`,
		`data-goshtoso-loading`,
		`Publishing revision…`,
		`Saving route…`,
		`Syncing ref…`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("management Goshtoso control semantics missing %q:\n%s", want, body)
		}
	}
	if count := strings.Count(body, `name="request_id"`); count != 3 {
		t.Fatalf("management mutation request ID count = %d, want 3", count)
	}
	inputTag := func(id string) string {
		t.Helper()
		tag := regexp.MustCompile(`<input\b[^>]*\bid="` + regexp.QuoteMeta(id) + `"[^>]*>`).FindString(body)
		if tag == "" {
			t.Fatalf("management control input %q missing:\n%s", id, body)
		}
		return tag
	}
	assertAttrs := func(id string, want ...string) string {
		t.Helper()
		tag := inputTag(id)
		for _, attr := range want {
			if !strings.Contains(tag, attr) {
				t.Fatalf("management control %q missing semantic attribute %q in %s", id, attr, tag)
			}
		}
		return tag
	}
	publicRadio := assertAttrs("management-visibility-public-payments-api", `type="radio"`, `name="visibility"`, `value="public"`)
	assertAttrs("management-visibility-private-payments-api", `type="radio"`, `name="visibility"`, `value="private"`, ` checked`)
	if strings.Contains(publicRadio, ` checked`) {
		t.Fatalf("public visibility radio should not be checked: %s", publicRadio)
	}
	if !strings.Contains(body, `for="management-visibility-public-payments-api"`) ||
		!strings.Contains(body, `for="management-visibility-private-payments-api"`) {
		t.Fatalf("visibility radios should have associated labels:\n%s", body)
	}
	toggleTag := assertAttrs("management-payments-api-sync-publish", `type="checkbox"`, `role="switch"`, `name="publish"`, `value="public"`)
	if strings.Contains(toggleTag, ` checked`) {
		t.Fatalf("publish-on-sync switch should start unchecked: %s", toggleTag)
	}
	if !strings.Contains(body, `for="management-payments-api-sync-publish"`) {
		t.Fatalf("publish-on-sync switch should have an associated label:\n%s", body)
	}
}

func TestManagementSyncPostRejectsRefWhenCandidatesUnavailable(t *testing.T) {
	var called bool
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
				called = true
				return spec, nil
			},
			Specs: []ManagedSpec{{
				ID:      "payments-api",
				Index:   core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
				Project: core.Project{ID: "payments", Name: "Payments"},
				Source:  core.Source{ID: "repo-payments", ProjectID: "payments", Kind: "git", SpecPath: "docs/openapi.yaml"},
				Revision: core.Revision{
					ID:        "rev-main",
					SourceID:  "repo-payments",
					Ref:       "main",
					CommitSHA: "abc123",
				},
			}},
		},
	})

	form := url.Values{
		"spec_id": {"payments-api"},
		"ref":     {"release/v2"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Manja-Application-Status"); got != "validation-error" {
		t.Fatalf("application status = %q, want validation-error", got)
	}
	if called {
		t.Fatal("sync action should not be called for unavailable ref")
	}
	if !strings.Contains(rec.Body.String(), "ref is not available for this source") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestManagementSyncRejectsUnavailableCandidateWithoutEffect(t *testing.T) {
	var syncCalls int
	srv := NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{
		SyncAction: func(_ context.Context, spec ManagedSpec, _ string) (ManagedSpec, error) {
			syncCalls++
			return spec, nil
		},
		Specs: []ManagedSpec{{
			ID:         "payments-api",
			Index:      core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
			Project:    core.Project{ID: "payments"},
			Source:     core.Source{ID: "payments-source"},
			Revision:   core.Revision{ID: "rev-main", Ref: "main"},
			Candidates: []core.RevisionCandidate{{Ref: "main"}},
		}},
	}})
	form := url.Values{"spec_id": {"payments-api"}, "ref": {"forged/ref"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("X-Manja-Application-Status"); got != "validation-error" {
		t.Fatalf("application status = %q", got)
	}
	for _, want := range []string{`id="management-main-content"`, `data-selected-contract="payments-api"`, `data-management-application-error="true"`, `forged/ref`, `ref is not available for this source`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("recovery fragment missing %q:\n%s", want, rec.Body.String())
		}
	}
	if syncCalls != 0 {
		t.Fatalf("sync effect count = %d, want 0", syncCalls)
	}
}

func TestManagementPublicationFailureRetainsSelectedContractAndValues(t *testing.T) {
	store := &fakeManagementPublicationStore{err: errors.New("persistence unavailable")}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{
		Store: store,
		Specs: []ManagedSpec{{
			ID:          "payments-api",
			Index:       core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
			Project:     core.Project{ID: "payments"},
			Source:      core.Source{ID: "payments-source"},
			Revision:    core.Revision{ID: "rev-main"},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "rev-main", Path: "/old"},
		}},
	}})
	form := url.Values{"spec_id": {"payments-api"}, "visibility": {"public"}, "path": {"/entered/path"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("X-Manja-Application-Status"); got != "persistence-error" {
		t.Fatalf("application status = %q", got)
	}
	for _, want := range []string{`data-selected-contract="payments-api"`, `data-management-application-error="true"`, `persistence unavailable`, `value="/entered/path"`, `Retry publication`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("publication recovery missing %q:\n%s", want, rec.Body.String())
		}
	}
	if store.saved != (core.Publication{}) {
		t.Fatalf("committed publication = %#v, want zero", store.saved)
	}
}

func TestManagementUnknownRouteReturnsInShellRecovery(t *testing.T) {
	body := renderManagementRequest(t, managementStructureServer(), "/manage/not-a-route", http.StatusNotFound)
	for _, want := range []string{`<!doctype html>`, `id="main-content"`, `id="management-main-content"`, `data-management-route-not-found="true"`, `Management page not found`, `href="/manage"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("unknown management route missing %q:\n%s", want, body)
		}
	}
}

func TestManagementRepeatedSubmissionDoesNotDuplicateEffect(t *testing.T) {
	var syncCalls int
	srv := NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{
		SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
			syncCalls++
			spec.Revision.Ref = ref
			return spec, nil
		},
		Specs: []ManagedSpec{{
			ID:         "payments-api",
			Index:      core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
			Project:    core.Project{ID: "payments"},
			Source:     core.Source{ID: "payments-source"},
			Revision:   core.Revision{ID: "rev-main", Ref: "main"},
			Candidates: []core.RevisionCandidate{{Ref: "main"}, {Ref: "release/v2"}},
		}},
	}})
	form := url.Values{"spec_id": {"payments-api"}, "ref": {"release/v2"}, "request_id": {"sync-repeat-1"}}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(managementMutationForm(form).Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("submission %d status = %d, want %d", i+1, rec.Code, http.StatusSeeOther)
		}
	}
	if syncCalls != 1 {
		t.Fatalf("sync effect count = %d, want 1", syncCalls)
	}
}

func TestManagementSameTokenDifferentPayloadIsRejectedWithoutEffect(t *testing.T) {
	var syncCalls int
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{
		Store: store,
		SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
			syncCalls++
			spec.Revision.ID = "rev-" + strings.ReplaceAll(ref, "/", "-")
			spec.Revision.Ref = ref
			return spec, nil
		},
		Specs: []ManagedSpec{{
			ID:         "payments-api",
			Index:      core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
			Project:    core.Project{ID: "payments"},
			Source:     core.Source{ID: "payments-source"},
			Revision:   core.Revision{ID: "rev-main", Ref: "main"},
			Candidates: []core.RevisionCandidate{{Ref: "main"}, {Ref: "release/v2"}},
		}},
	}})

	first := url.Values{"spec_id": {"payments-api"}, "ref": {"release/v2"}, "request_id": {"stale-tab-token"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(first.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("first status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	second := url.Values{
		"spec_id":    {"payments-api"},
		"ref":        {"main"},
		"publish":    {"public"},
		"path":       {"/payments/stale"},
		"request_id": {"stale-tab-token"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/manage/sync", strings.NewReader(second.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("conflict status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Manja-Application-Status"); got != "validation-error" {
		t.Fatalf("application status = %q, want validation-error", got)
	}
	if !strings.Contains(rec.Body.String(), "request token does not match the submitted sync values") {
		t.Fatalf("payload conflict recovery missing: %s", rec.Body.String())
	}
	if syncCalls != 1 {
		t.Fatalf("sync effects = %d, want 1", syncCalls)
	}
	if store.saved != (core.Publication{}) {
		t.Fatalf("conflicting stale tab published %#v, want zero", store.saved)
	}
}

func TestManagementInvalidMutationTokensAreRejectedWithoutEffect(t *testing.T) {
	invalidTokens := []struct {
		name  string
		token string
	}{
		{name: "missing", token: ""},
		{name: "oversized", token: strings.Repeat("a", 257)},
		{name: "malformed", token: "token with spaces!"},
	}
	for _, action := range []string{"sync", "publication"} {
		for _, invalid := range invalidTokens {
			t.Run(action+"/"+invalid.name, func(t *testing.T) {
				var syncCalls int
				store := &fakeManagementPublicationStore{}
				srv := NewServerWithOptions(core.SpecIndex{}, Options{Management: ManagementOptions{
					Store: store,
					SyncAction: func(_ context.Context, spec ManagedSpec, ref string) (ManagedSpec, error) {
						syncCalls++
						spec.Revision.Ref = ref
						return spec, nil
					},
					Specs: []ManagedSpec{{
						ID:         "payments-api",
						Index:      core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
						Project:    core.Project{ID: "payments"},
						Source:     core.Source{ID: "payments-source"},
						Revision:   core.Revision{ID: "rev-main", Ref: "main"},
						Candidates: []core.RevisionCandidate{{Ref: "main"}},
					}},
				}})
				form := url.Values{"spec_id": {"payments-api"}, "request_id": {invalid.token}}
				path := "/manage/publication"
				if action == "sync" {
					path = "/manage/sync"
					form.Set("ref", "main")
				} else {
					form.Set("revision_id", "rev-main")
					form.Set("visibility", "public")
					form.Set("path", "/payments")
				}
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(managementMutationForm(form).Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("HX-Request", "true")
				srv.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
				}
				if got := rec.Header().Get("X-Manja-Application-Status"); got != "validation-error" {
					t.Fatalf("application status = %q, want validation-error", got)
				}
				if !strings.Contains(rec.Body.String(), "valid request token is required") {
					t.Fatalf("token recovery missing: %s", rec.Body.String())
				}
				if syncCalls != 0 {
					t.Fatalf("sync effects = %d, want 0", syncCalls)
				}
				if store.saved != (core.Publication{}) {
					t.Fatalf("publication effect = %#v, want zero", store.saved)
				}
			})
		}
	}
}

func TestManagementHTMXNotFoundReturnsSwappableRecovery(t *testing.T) {
	for _, path := range []string{"/manage/spec/unknown-api", "/manage/not-a-route"} {
		t.Run(path, func(t *testing.T) {
			srv := managementStructureServer()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("HX-Request", "true")
			req.Header.Set("HX-Target", "management-main-content")
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := rec.Header().Get("X-Manja-Application-Status"); got != "not-found" {
				t.Fatalf("application status = %q, want not-found", got)
			}
			if !strings.Contains(rec.Body.String(), `id="management-main-content"`) {
				t.Fatalf("recovery fragment missing: %s", rec.Body.String())
			}
		})
	}
}

func TestManagementSameURLMutationUsesReplaceURLAndDisablesHistorySnapshots(t *testing.T) {
	srv := managementStructureServer()
	form := url.Values{"spec_id": {"payments-api"}, "visibility": {"private"}, "path": {"/payments/v1"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://example.test/manage/spec/payments-api")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("HX-Replace-Url"); got != "/manage/spec/payments-api" {
		t.Fatalf("HX-Replace-Url = %q", got)
	}
	if got := rec.Header().Get("HX-Push-Url"); got != "" {
		t.Fatalf("HX-Push-Url = %q, want empty", got)
	}

	body := renderManagementRequest(t, srv, "/manage/spec/payments-api", http.StatusOK)
	if !strings.Contains(body, `hx-history="false"`) {
		t.Fatalf("management shell must disable HTMX history snapshots: %s", body)
	}
}

func TestManagementPublicationPostCanMakeRevisionPrivate(t *testing.T) {
	store := &fakeManagementPublicationStore{}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store:       store,
			Project:     core.Project{ID: "payments"},
			Revision:    core.Revision{ID: "rev1"},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "rev1", Public: true, Path: "/payments/v1"},
		},
	})
	form := url.Values{
		"visibility": {"private"},
		"path":       {"/payments/v1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if store.saved.Public {
		t.Fatalf("publication should be private: %#v", store.saved)
	}
	if store.saved.Path != "/payments/v1" {
		t.Fatalf("private publication should retain path for later publishing, got %#v", store.saved)
	}
}

func TestManagementPublicationPostRejectsInvalidRequests(t *testing.T) {
	store := &fakeManagementPublicationStore{err: errors.New("unsafe store path")}
	srv := NewServerWithOptions(core.SpecIndex{}, Options{
		Management: ManagementOptions{
			Store:    store,
			Project:  core.Project{ID: "payments"},
			Revision: core.Revision{ID: "rev1"},
		},
	})
	form := url.Values{
		"visibility": {"public"},
		"path":       {"payments/v1"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(managementMutationForm(form).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Manja-Application-Status"); got != "persistence-error" {
		t.Fatalf("application status = %q, want persistence-error", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/manage/publication", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

type fakeManagementPublicationStore struct {
	saved core.Publication
	err   error
}

func managementMutationForm(form url.Values) url.Values {
	withToken := make(url.Values, len(form)+1)
	for key, values := range form {
		withToken[key] = append([]string(nil), values...)
	}
	if _, present := withToken["request_id"]; !present {
		withToken.Set("request_id", "test-request-token")
	}
	return withToken
}

func (s *fakeManagementPublicationStore) SavePublication(_ context.Context, pub core.Publication) error {
	if s.err != nil {
		return s.err
	}
	s.saved = pub
	return nil
}
