package e2e

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

func TestManagementMutationLoadingPreventsDuplicateSubmission(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	var calls atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	handler := managementWorkflowServer(func(_ context.Context, spec web.ManagedSpec, ref string) (web.ManagedSpec, error) {
		calls.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		spec.Revision.Ref = ref
		return spec, nil
	})
	server := httptestServer(t, handler)
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})

	pw, browser, page := managementWorkflowPage(t, server)
	defer pw.Stop()
	defer browser.Close()
	openManagementSyncPanel(t, page)

	button := page.Locator(`#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]`)
	if _, err := page.Evaluate(`() => document.querySelector('#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]').click()`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => {
		const button = document.querySelector('#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]');
		return Boolean(button && button.disabled && button.textContent.includes('Syncing ref'));
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("mutation did not expose its disabled loading state: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("sync handler did not receive the in-flight mutation")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("in-flight sync count = %d, want 1", got)
	}
	if _, err := button.Evaluate(`button => button.click()`, nil); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("disabled repeat sync count = %d, want 1", got)
	}

	close(release)
	released = true
	if err := page.Locator(`[data-management-contract-identity="payments-api"]`).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.activeElement?.dataset.managementContractIdentity === 'payments-api'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("successful mutation did not focus the authoritative workspace: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("settled sync count = %d, want 1", got)
	}
}

func TestManagementTransportFailureRetainsValuesAndRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	var calls atomic.Int32
	handler := managementWorkflowServer(func(_ context.Context, spec web.ManagedSpec, ref string) (web.ManagedSpec, error) {
		calls.Add(1)
		spec.Revision.Ref = ref
		return spec, nil
	})
	server := httptestServer(t, handler)
	pw, browser, page := managementWorkflowPage(t, server)
	defer pw.Stop()
	defer browser.Close()
	openManagementSyncPanel(t, page)

	if _, err := page.Evaluate(`() => {
		const select = document.querySelector('#management-payments-api-sync-ref');
		select.value = 'release/v2';
		select.dispatchEvent(new Event('change', { bubbles: true }));
	}`, nil); err != nil {
		t.Fatal(err)
	}
	if err := page.Route("**/manage/sync", func(route playwright.Route) {
		_ = route.Abort()
	}); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]`).Click(); err != nil {
		t.Fatal(err)
	}
	recovery := page.Locator(`[data-management-transport-recovery="true"]`)
	if err := recovery.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("transport recovery did not become visible: %v", err)
	}
	retained, err := page.Locator(`#management-payments-api-sync-ref`).InputValue()
	if err != nil {
		t.Fatal(err)
	}
	if retained != "release/v2" {
		t.Fatalf("retained ref = %q, want release/v2", retained)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("aborted transport completed %d sync effects, want 0", got)
	}
	if active, err := page.Evaluate(`() => document.activeElement?.dataset.managementRetry === 'true'`, nil); err != nil || active != true {
		t.Fatalf("transport recovery focus = %#v, err=%v", active, err)
	}

	if err := page.Unroute("**/manage/sync"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.ExpectResponse("**/manage/sync", func() error {
		return page.Locator(`[data-management-retry="true"]`).Click()
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-contract-identity="payments-api"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("retried sync count = %d, want 1", got)
	}
}

func TestManagementApplicationErrorSwapsAndFocusesRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	var calls atomic.Int32
	server := httptestServer(t, managementWorkflowServer(func(_ context.Context, spec web.ManagedSpec, _ string) (web.ManagedSpec, error) {
		calls.Add(1)
		return spec, nil
	}))
	pw, browser, page := managementWorkflowPage(t, server)
	defer pw.Stop()
	defer browser.Close()
	openManagementSyncPanel(t, page)
	if _, err := page.Evaluate(`() => {
		const select = document.querySelector('#management-payments-api-sync-ref');
		select.insertAdjacentHTML('beforeend', '<option value="forged/ref">forged/ref</option>');
		select.value = 'forged/ref';
	}`, nil); err != nil {
		t.Fatal(err)
	}

	response, err := page.ExpectResponse("**/manage/sync", func() error {
		return page.Locator(`#management-main-content [role="tabpanel"][aria-label="Sync"] button[type="submit"]`).Click()
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status() != 200 {
		t.Fatalf("application error status = %d, want 200", response.Status())
	}
	if got, err := response.HeaderValue("X-Manja-Application-Status"); err != nil || got != "validation-error" {
		t.Fatalf("application status = %q, err=%v", got, err)
	}
	if err := page.Locator(`[data-management-application-error="true"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.activeElement?.dataset.applicationStatus === 'validation-error'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("application recovery did not receive settled focus: %v", err)
	}
	state, err := page.Evaluate(`() => ({
		url: location.pathname,
		selected: document.querySelector('#management-main-content')?.dataset.selectedContract,
		entered: document.querySelector('[data-management-application-error] code')?.textContent,
		focused: document.activeElement?.dataset.applicationStatus
	})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	metrics := state.(map[string]any)
	for key, want := range map[string]string{
		"url":      "/manage/spec/payments-api",
		"selected": "payments-api",
		"entered":  "forged/ref",
		"focused":  "validation-error",
	} {
		if metrics[key] != want {
			t.Fatalf("application recovery %s = %#v, want %q; state=%#v", key, metrics[key], want, metrics)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("forged ref completed %d sync effects, want 0", got)
	}
}

func TestManagementHTMXMissingContractSwapsAuthoritativeRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, managementWorkflowServer(func(_ context.Context, spec web.ManagedSpec, _ string) (web.ManagedSpec, error) {
		return spec, nil
	}))
	pw, browser, page := managementWorkflowPage(t, server)
	defer pw.Stop()
	defer browser.Close()
	if _, err := page.Evaluate(`() => {
		const link = document.createElement('a');
		link.href = '/manage/spec/unknown-api';
		link.textContent = 'Open missing contract';
		link.dataset.testMissingContract = 'true';
		link.dataset.managementNav = 'true';
		link.setAttribute('hx-get', link.getAttribute('href'));
		link.setAttribute('hx-target', '#management-main-content');
		link.setAttribute('hx-swap', 'outerHTML');
		link.setAttribute('hx-push-url', 'true');
		document.body.appendChild(link);
		window.htmx.process(link);
	}`, nil); err != nil {
		t.Fatal(err)
	}
	response, err := page.ExpectResponse("**/manage/spec/unknown-api", func() error {
		_, clickErr := page.Evaluate(`() => document.querySelector('[data-test-missing-contract="true"]').click()`, nil)
		return clickErr
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status() != 200 {
		t.Fatalf("HTMX missing-contract status = %d, want 200", response.Status())
	}
	if got, err := response.HeaderValue("X-Manja-Application-Status"); err != nil || got != "not-found" {
		t.Fatalf("application status = %q, err=%v", got, err)
	}
	if err := page.Locator(`[data-management-spec-not-found="true"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() =>
		location.pathname === '/manage/spec/unknown-api' &&
		document.querySelector('#management-main-content')?.dataset.selectedContract === 'unknown-api' &&
		document.activeElement?.id === 'management-spec-not-found-heading' &&
		!document.querySelector('aside[aria-label="Management sections"] [aria-current="page"]') &&
		document.title === 'Spec not found · Management'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("missing-contract identity did not settle: %v", err)
	}
}

func TestManagementMutationBackForwardAndReloadRemainAuthoritative(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)

	server := httptestServer(t, web.NewServerWithOptions(core.SpecIndex{}, web.Options{Management: web.ManagementOptions{
		Store: &recordingPublicationStore{},
		Specs: []web.ManagedSpec{{
			ID:          "payments-api",
			Index:       core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
			Project:     core.Project{ID: "payments", Name: "Payments"},
			Source:      core.Source{ID: "payments-source", Kind: "git"},
			Revision:    core.Revision{ID: "rev-main", Ref: "main"},
			Publication: core.Publication{ProjectID: "payments", RevisionID: "rev-main", Path: "/payments/old"},
		}},
	}}))
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
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/manage/specs"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`aside[aria-label="Management sections"] a[href="/manage/spec/payments-api"]`).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-contract-identity="payments-api"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	before, err := page.Evaluate(`() => history.length`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[role="tab"]:has-text("Route")`).Click(); err != nil {
		t.Fatal(err)
	}
	pathInput := page.Locator(`#management-main-content [role="tabpanel"][aria-label="Route"] input[name="path"]`)
	if err := pathInput.Fill("/payments/fresh"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.ExpectResponse("**/manage/publication", func() error {
		return page.Locator(`#management-main-content [role="tabpanel"][aria-label="Route"] button:has-text("Save route settings")`).Click()
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	after, err := page.Evaluate(`() => history.length`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("same-URL mutation history length = %#v, want %#v", after, before)
	}
	if _, err := page.GoBack(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-page-header="specs"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.GoForward(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator(`[data-management-contract-identity="payments-api"]`).WaitFor(); err != nil {
		t.Fatal(err)
	}
	assertManagementPublicationPath(t, page, "/payments/fresh")
	if _, err := page.Reload(); err != nil {
		t.Fatal(err)
	}
	assertManagementPublicationPath(t, page, "/payments/fresh")
}

func assertManagementPublicationPath(t *testing.T, page playwright.Page, want string) {
	t.Helper()
	if err := page.Locator(`[role="tab"]:has-text("Route")`).Click(); err != nil {
		t.Fatal(err)
	}
	input := page.Locator(`#management-main-content [role="tabpanel"][aria-label="Route"] input[name="path"]`)
	if err := input.WaitFor(); err != nil {
		t.Fatal(err)
	}
	got, err := input.InputValue()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("authoritative publication path = %q, want %q", got, want)
	}
}

func managementWorkflowServer(syncAction web.ManagementSyncAction) http.Handler {
	return web.NewServerWithOptions(core.SpecIndex{}, web.Options{Management: web.ManagementOptions{
		SyncAction: syncAction,
		Specs: []web.ManagedSpec{{
			ID:         "payments-api",
			Index:      core.SpecIndex{ProjectID: "payments", RevisionID: "rev-main", Title: "Payments API"},
			Project:    core.Project{ID: "payments", Name: "Payments"},
			Source:     core.Source{ID: "payments-source", Kind: "git"},
			Revision:   core.Revision{ID: "rev-main", Ref: "main", CommitSHA: "abc123"},
			Candidates: []core.RevisionCandidate{{Ref: "main"}, {Ref: "release/v2"}},
		}},
	}})
}

func managementWorkflowPage(t *testing.T, server string) (*playwright.Playwright, playwright.Browser, playwright.Page) {
	t.Helper()
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	browser, err := pw.Chromium.Launch()
	if err != nil {
		pw.Stop()
		t.Fatal(err)
	}
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{Viewport: &playwright.Size{Width: 1024, Height: 768}})
	if err != nil {
		browser.Close()
		pw.Stop()
		t.Fatal(err)
	}
	if _, err := page.Goto(server + "/manage/spec/payments-api"); err != nil {
		browser.Close()
		pw.Stop()
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		browser.Close()
		pw.Stop()
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	return pw, browser, page
}

func openManagementSyncPanel(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.Locator(`[role="tab"]:has-text("Sync")`).Click(); err != nil {
		t.Fatal(err)
	}
	assertVisibleManagementTabPanels(t, page, []string{"Sync"})
}
