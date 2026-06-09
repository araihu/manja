package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/internal/core"
)

func TestManagementOverviewShowsProjectSyncAndPublicationState(t *testing.T) {
	srv := NewServerWithOptions(core.SpecIndex{
		ProjectID:  "payments",
		RevisionID: "rev1",
		Title:      "Payments API",
		Version:    "2026-06-09",
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
				ID:        "rev1",
				SourceID:  "source1",
				Ref:       "main",
				CommitSHA: "abc123",
				Version:   "2026-06-09",
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
		`action="/manage/publication"`,
		`border-success bg-surface text-success`,
		`inline-flex w-fit overflow-clip rounded-radius border border-outline bg-surface-alt divide-x`,
		`has-checked:bg-primary has-checked:text-on-primary`,
		`id="management-public-path"`,
		`name="visibility"`,
		`value="public"`,
		`value="private"`,
		`name="path"`,
		`value="/payments/v1"`,
		`focus-visible:outline-primary`,
		`rounded-2xl font-medium tracking-wide`,
		"Save publication",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{
		`inline-grid w-fit grid-cols-2`,
	} {
		if strings.Contains(body, reject) {
			t.Fatalf("body should use Goshtoso component vocabulary instead of %q:\n%s", reject, body)
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
		`value="billing-events"`,
		`id="management-public-path-github-rest"`,
		`id="management-public-path-billing-events"`,
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
	req = httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(form.Encode()))
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
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/manage" {
		t.Fatalf("Location = %q, want /manage", got)
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
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(form.Encode()))
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
	req := httptest.NewRequest(http.MethodPost, "/manage/publication", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
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

func (s *fakeManagementPublicationStore) SavePublication(_ context.Context, pub core.Publication) error {
	if s.err != nil {
		return s.err
	}
	s.saved = pub
	return nil
}
