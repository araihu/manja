package e2e

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/web"
)

func TestPublicDocsGoshtosoCDNFailureUsesEmbeddedFallback(t *testing.T) {
	testPublicDocsGoshtosoDependencyJourney(t, true)
}

func TestPublicDocsGoshtosoCDNPrimaryJourney(t *testing.T) {
	testPublicDocsGoshtosoDependencyJourney(t, false)
}

func TestPublicDocsGoshtosoDependencyJourneyRejectsFirstPartyAssetAndUnrelatedJavaScriptFailures(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		forceFallback bool
	}{
		{name: "normal"},
		{name: "forced-fallback", forceFallback: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			testPublicDocsGoshtosoDependencyJourneyRejectsFirstPartyAssetAndUnrelatedJavaScriptFailures(t, testCase.forceFallback)
		})
	}
}

func testPublicDocsGoshtosoDependencyJourneyRejectsFirstPartyAssetAndUnrelatedJavaScriptFailures(t *testing.T, forceFallback bool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)
	server := httptestServer(t, web.NewPublicServer(goshtosoFallbackIndex()))
	expectedPrimaryURLs := goshtosoRenderedPrimaryURLs(t, server)

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
	if err := page.AddInitScript(playwright.Script{Content: playwright.String(`
		window.__manjaControlRejections = [];
		window.addEventListener("unhandledrejection", event => {
			window.__manjaControlRejections.push(String(event.reason));
		});
	`)}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var routeErrors []string
	var interceptedPrimaryURLs []string
	if forceFallback {
		routeGoshtosoPrimaryFailures(t, page, expectedPrimaryURLs, http.StatusBadGateway, &mu, &interceptedPrimaryURLs, &routeErrors)
	}
	unapprovedExternalURL := "https://example.invalid/manja-unapproved-external.js"
	if err := page.Route(unapprovedExternalURL, func(route playwright.Route) {
		if err := route.Fulfill(playwright.RouteFulfillOptions{
			Status:      playwright.Int(http.StatusServiceUnavailable),
			Body:        "simulated unapproved external asset outage",
			ContentType: playwright.String("text/plain"),
		}); err != nil {
			mu.Lock()
			routeErrors = append(routeErrors, err.Error())
			mu.Unlock()
		}
	}); err != nil {
		t.Fatal(err)
	}
	failedAssetURL := server + "/assets/styles.css"
	if err := page.Route(failedAssetURL, func(route playwright.Route) {
		if err := route.Fulfill(playwright.RouteFulfillOptions{
			Status:      playwright.Int(503),
			Body:        "simulated first-party asset outage",
			ContentType: playwright.String("text/plain"),
		}); err != nil {
			mu.Lock()
			routeErrors = append(routeErrors, err.Error())
			mu.Unlock()
		}
	}); err != nil {
		t.Fatal(err)
	}

	var failedResponses []goshtosoDependencyResponseFailure
	var failedRequests []goshtosoDependencyRequestFailure
	var consoleErrors []goshtosoDependencyConsoleError
	var pageErrors []string
	pageErrorSeen := make(chan string, 1)
	page.OnResponse(func(response playwright.Response) {
		if response.Status() < 400 {
			return
		}
		mu.Lock()
		failedResponses = append(failedResponses, goshtosoDependencyResponseFailure{
			URL:        response.URL(),
			RequestURL: response.Request().URL(),
			Status:     response.Status(),
		})
		mu.Unlock()
	})
	page.OnRequestFailed(func(request playwright.Request) {
		failure := request.Failure()
		failureText := "unknown request failure"
		if failure != nil {
			failureText = failure.Error()
		}
		mu.Lock()
		failedRequests = append(failedRequests, goshtosoDependencyRequestFailure{URL: request.URL(), Error: failureText})
		mu.Unlock()
	})
	page.OnPageError(func(err error) {
		message := err.Error()
		mu.Lock()
		pageErrors = append(pageErrors, message)
		mu.Unlock()
		if strings.Contains(message, "manja-unrelated-pageerror") {
			select {
			case pageErrorSeen <- message:
			default:
			}
		}
	})
	page.On("console", func(message playwright.ConsoleMessage) {
		if message.Type() != "error" {
			return
		}
		mu.Lock()
		consoleErrors = append(consoleErrors, goshtosoDependencyConsoleErrorFrom(message))
		mu.Unlock()
	})

	if _, err := page.Goto(server+"/?selected=operation-listpets#operation-listpets", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}
	if _, err := page.Evaluate(`() => {
		console.error("manja-unrelated-console-error");
		Promise.reject(new Error("manja-unrelated-rejection"));
		setTimeout(() => { throw new Error("manja-unrelated-pageerror"); }, 0);
		const script = document.createElement("script");
		script.src = "https://example.invalid/manja-unapproved-external.js";
		script.addEventListener("error", () => { window.__manjaUnapprovedExternalSettled = true; });
		document.head.appendChild(script);
	}`, nil); err != nil {
		t.Fatalf("inject unrelated JavaScript controls: %v", err)
	}
	if _, err := page.WaitForFunction(`() => window.__manjaControlRejections.some(value => value.includes("manja-unrelated-rejection"))`, nil); err != nil {
		t.Fatalf("wait for unrelated unhandled rejection control: %v", err)
	}
	if _, err := page.WaitForFunction(`() => window.__manjaUnapprovedExternalSettled === true`, nil); err != nil {
		t.Fatalf("wait for unapproved external asset control: %v", err)
	}
	select {
	case <-pageErrorSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for unrelated pageerror control")
	}
	rawRejections, err := page.Evaluate(`() => JSON.stringify(window.__manjaControlRejections)`, nil)
	if err != nil {
		t.Fatalf("read unrelated rejection controls: %v", err)
	}
	var rejections []string
	if err := json.Unmarshal([]byte(fmt.Sprint(rawRejections)), &rejections); err != nil {
		t.Fatalf("decode unrelated rejection controls: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	wantFailedResponseCount := 2
	if forceFallback {
		wantFailedResponseCount += len(expectedPrimaryURLs)
	}
	if len(failedResponses) != wantFailedResponseCount {
		t.Fatalf("failed response count = %d, want %d: %#v", len(failedResponses), wantFailedResponseCount, failedResponses)
	}
	foundFirstPartyResponse := false
	for _, response := range failedResponses {
		if response.URL == failedAssetURL && response.RequestURL == failedAssetURL && response.Status == http.StatusServiceUnavailable {
			foundFirstPartyResponse = true
			break
		}
	}
	if !foundFirstPartyResponse {
		t.Fatalf("failed responses do not contain correlated first-party %s status 503: %#v", failedAssetURL, failedResponses)
	}
	failures := validateGoshtosoDependencyBrowserEvidence(forceFallback, expectedPrimaryURLs, goshtosoDependencyBrowserEvidence{
		FailedResponses:        failedResponses,
		FailedRequests:         failedRequests,
		ConsoleErrors:          consoleErrors,
		PageErrors:             pageErrors,
		Rejections:             rejections,
		InterceptedPrimaryURLs: interceptedPrimaryURLs,
		RouteErrors:            routeErrors,
	})
	for _, sentinel := range []string{failedAssetURL, unapprovedExternalURL, "manja-unrelated-console-error", "manja-unrelated-pageerror", "manja-unrelated-rejection"} {
		if !strings.Contains(strings.Join(failures, "\n"), sentinel) {
			t.Errorf("strict browser validation did not block %q; failures=%v", sentinel, failures)
		}
	}
	if forceFallback && !strings.Contains(strings.Join(failures, "\n"), "status=502") {
		t.Errorf("strict browser validation did not block non-503 primary responses; failures=%v", failures)
	}
}

type goshtosoDependencyResponseFailure struct {
	URL        string
	RequestURL string
	Status     int
}

type goshtosoDependencyConsoleError struct {
	Text string
	URL  string
}

type goshtosoDependencyRequestFailure struct {
	URL   string
	Error string
}

type goshtosoDependencyBrowserEvidence struct {
	FailedResponses        []goshtosoDependencyResponseFailure
	FailedRequests         []goshtosoDependencyRequestFailure
	ConsoleErrors          []goshtosoDependencyConsoleError
	PageErrors             []string
	Rejections             []string
	InterceptedPrimaryURLs []string
	RouteErrors            []string
}

func goshtosoDependencyConsoleErrorFrom(message playwright.ConsoleMessage) goshtosoDependencyConsoleError {
	location := message.Location()
	url := ""
	if location != nil {
		url = location.URL
	}
	return goshtosoDependencyConsoleError{Text: message.Text(), URL: url}
}

func goshtosoRenderedPrimaryURLs(t *testing.T, server string) []string {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(server + "/")
	if err != nil {
		t.Fatalf("GET rendered Goshtoso dependency configuration: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET rendered Goshtoso dependency configuration status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read rendered Goshtoso dependency configuration: %v", err)
	}
	match := regexp.MustCompile(`data-goshtoso-dependencies="([^"]+)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("rendered public page is missing data-goshtoso-dependencies")
	}

	type dependencyEntry struct {
		Name       string `json:"name"`
		PrimaryURL string `json:"primary_url"`
	}
	var config struct {
		Dependencies []dependencyEntry `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(string(match[1]))), &config); err != nil {
		t.Fatalf("decode rendered Goshtoso dependency configuration: %v", err)
	}

	wantOrder := []string{"alpine-collapse", "alpine-focus", "alpine-mask", "alpine", "htmx", "combobox"}
	if len(config.Dependencies) != len(wantOrder) {
		t.Fatalf("rendered dependency count = %d, want %d: %#v", len(config.Dependencies), len(wantOrder), config.Dependencies)
	}
	primaryURLs := make([]string, 0, 5)
	seen := make(map[string]struct{}, 5)
	for i, wantName := range wantOrder {
		dependency := config.Dependencies[i]
		if dependency.Name != wantName {
			t.Fatalf("rendered dependency %d name = %q, want %q", i, dependency.Name, wantName)
		}
		if i == len(wantOrder)-1 {
			if dependency.PrimaryURL != "/assets/js/combobox.js" {
				t.Fatalf("rendered combobox primary URL = %q, want first-party /assets/js/combobox.js", dependency.PrimaryURL)
			}
			continue
		}
		if !strings.HasPrefix(dependency.PrimaryURL, "https://unpkg.com/") {
			t.Fatalf("rendered %s primary URL = %q, want exact-version unpkg URL", dependency.Name, dependency.PrimaryURL)
		}
		if _, duplicate := seen[dependency.PrimaryURL]; duplicate {
			t.Fatalf("rendered primary URL is duplicated: %s", dependency.PrimaryURL)
		}
		seen[dependency.PrimaryURL] = struct{}{}
		primaryURLs = append(primaryURLs, dependency.PrimaryURL)
	}
	return primaryURLs
}

func routeGoshtosoPrimaryFailures(
	t *testing.T,
	page playwright.Page,
	primaryURLs []string,
	status int,
	mu *sync.Mutex,
	interceptedPrimaryURLs *[]string,
	routeErrors *[]string,
) {
	t.Helper()
	for _, expectedPrimaryURL := range primaryURLs {
		primaryURL := expectedPrimaryURL
		if err := page.Route(primaryURL, func(route playwright.Route) {
			mu.Lock()
			*interceptedPrimaryURLs = append(*interceptedPrimaryURLs, route.Request().URL())
			mu.Unlock()
			if err := route.Fulfill(playwright.RouteFulfillOptions{
				Status:      playwright.Int(status),
				Body:        "simulated CDN outage",
				ContentType: playwright.String("text/plain"),
			}); err != nil {
				mu.Lock()
				*routeErrors = append(*routeErrors, err.Error())
				mu.Unlock()
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func validateGoshtosoDependencyBrowserEvidence(forceFallback bool, expectedPrimaryURLs []string, evidence goshtosoDependencyBrowserEvidence) []string {
	var failures []string
	for _, routeError := range evidence.RouteErrors {
		failures = append(failures, "CDN interception error: "+routeError)
	}
	for _, pageError := range evidence.PageErrors {
		failures = append(failures, "unexpected page error: "+pageError)
	}
	for _, rejection := range evidence.Rejections {
		failures = append(failures, "unexpected unhandled rejection: "+rejection)
	}

	if !forceFallback {
		for _, request := range evidence.FailedRequests {
			failures = append(failures, fmt.Sprintf("unexpected failed request: URL=%s error=%s", request.URL, request.Error))
		}
		for _, response := range evidence.FailedResponses {
			failures = append(failures, fmt.Sprintf("unexpected failed response: URL=%s request=%s status=%d", response.URL, response.RequestURL, response.Status))
		}
		for _, consoleError := range evidence.ConsoleErrors {
			failures = append(failures, fmt.Sprintf("unexpected console error: text=%q URL=%s", consoleError.Text, consoleError.URL))
		}
		if len(evidence.InterceptedPrimaryURLs) != 0 {
			failures = append(failures, fmt.Sprintf("normal journey intercepted CDN primaries: %v", evidence.InterceptedPrimaryURLs))
		}
		return failures
	}

	if len(expectedPrimaryURLs) != 5 {
		failures = append(failures, fmt.Sprintf("expected primary URL count = %d, want 5", len(expectedPrimaryURLs)))
	}
	expectedURLCounts := goshtosoDependencyURLCounts(expectedPrimaryURLs)
	if len(expectedURLCounts) != len(expectedPrimaryURLs) {
		failures = append(failures, fmt.Sprintf("expected primary URLs are not unique: %v", expectedPrimaryURLs))
	}
	failures = append(failures, goshtosoExactURLMultisetFailures("intercepted primary URL", expectedPrimaryURLs, evidence.InterceptedPrimaryURLs)...)

	failedResponseURLs := make([]string, 0, len(evidence.FailedResponses))
	validResponseCounts := make(map[string]int, len(evidence.FailedResponses))
	for _, response := range evidence.FailedResponses {
		failedResponseURLs = append(failedResponseURLs, response.URL)
		if response.URL != response.RequestURL || response.Status != http.StatusServiceUnavailable {
			failures = append(failures, fmt.Sprintf("failed response URL=%s request=%s status=%d, want matching URL and status=503", response.URL, response.RequestURL, response.Status))
			continue
		}
		validResponseCounts[response.URL]++
	}
	failures = append(failures, goshtosoExactURLMultisetFailures("failed response URL", expectedPrimaryURLs, failedResponseURLs)...)

	consoleErrorURLs := make([]string, 0, len(evidence.ConsoleErrors))
	for _, consoleError := range evidence.ConsoleErrors {
		consoleErrorURLs = append(consoleErrorURLs, consoleError.URL)
		if expectedURLCounts[consoleError.URL] != 1 || !strings.Contains(consoleError.Text, "Failed to load resource") {
			failures = append(failures, fmt.Sprintf("unexpected console error: text=%q URL=%s", consoleError.Text, consoleError.URL))
		}
	}
	failures = append(failures, goshtosoExactURLMultisetFailures("console resource error URL", expectedPrimaryURLs, consoleErrorURLs)...)

	failedRequestCounts := make(map[string]int, len(evidence.FailedRequests))
	for _, request := range evidence.FailedRequests {
		failedRequestCounts[request.URL]++
		if failedRequestCounts[request.URL] > 1 {
			failures = append(failures, fmt.Sprintf("failed request URL %q count = %d, want at most 1", request.URL, failedRequestCounts[request.URL]))
		}
		if expectedURLCounts[request.URL] != 1 {
			failures = append(failures, fmt.Sprintf("failed request URL %q is outside the exact expected primary URL set (error=%q)", request.URL, request.Error))
			continue
		}
		if validResponseCounts[request.URL] != 1 {
			failures = append(failures, fmt.Sprintf("failed request URL %q has %d correlated response(s) with matching request URL and status=503, want 1 (error=%q)", request.URL, validResponseCounts[request.URL], request.Error))
		}
	}
	return failures
}

func goshtosoDependencyURLCounts(urls []string) map[string]int {
	counts := make(map[string]int, len(urls))
	for _, url := range urls {
		counts[url]++
	}
	return counts
}

func goshtosoExactURLMultisetFailures(channel string, expectedURLs, actualURLs []string) []string {
	expectedCounts := goshtosoDependencyURLCounts(expectedURLs)
	actualCounts := goshtosoDependencyURLCounts(actualURLs)
	var failures []string

	seenExpected := make(map[string]struct{}, len(expectedCounts))
	for _, expectedURL := range expectedURLs {
		if _, seen := seenExpected[expectedURL]; seen {
			continue
		}
		seenExpected[expectedURL] = struct{}{}
		if actualCounts[expectedURL] != expectedCounts[expectedURL] {
			failures = append(failures, fmt.Sprintf("%s %q count = %d, want %d", channel, expectedURL, actualCounts[expectedURL], expectedCounts[expectedURL]))
		}
	}

	seenActual := make(map[string]struct{}, len(actualCounts))
	for _, actualURL := range actualURLs {
		if _, seen := seenActual[actualURL]; seen {
			continue
		}
		seenActual[actualURL] = struct{}{}
		if expectedCounts[actualURL] == 0 {
			failures = append(failures, fmt.Sprintf("%s contains unexpected URL %q count=%d", channel, actualURL, actualCounts[actualURL]))
		}
	}
	return failures
}

func testPublicDocsGoshtosoDependencyJourney(t *testing.T, forceFallback bool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	chdirRepoRoot(t)
	server := httptestServer(t, web.NewPublicServer(goshtosoFallbackIndex()))
	expectedPrimaryURLs := goshtosoRenderedPrimaryURLs(t, server)

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
		Viewport: &playwright.Size{Width: 1440, Height: 900},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := page.AddInitScript(playwright.Script{Content: playwright.String(`
		window.__manjaDependencyEvents = { fallbacks: [], ready: 0, errors: [], rejections: [] };
		window.addEventListener("goshtoso:dependency-fallback", event => {
			window.__manjaDependencyEvents.fallbacks.push(event.detail.dependency);
		});
		window.addEventListener("goshtoso:dependencies-ready", () => {
			window.__manjaDependencyEvents.ready += 1;
		});
		window.addEventListener("goshtoso:dependency-error", event => {
			window.__manjaDependencyEvents.errors.push(event.detail && event.detail.dependency);
		});
		window.addEventListener("unhandledrejection", event => {
			window.__manjaDependencyEvents.rejections.push(String(event.reason));
		});
	`)}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var routeErrors []string
	var interceptedPrimaryURLs []string
	if forceFallback {
		routeGoshtosoPrimaryFailures(t, page, expectedPrimaryURLs, http.StatusServiceUnavailable, &mu, &interceptedPrimaryURLs, &routeErrors)
	}

	var failedResponses []goshtosoDependencyResponseFailure
	var failedRequests []goshtosoDependencyRequestFailure
	var pageErrors []string
	var consoleErrors []goshtosoDependencyConsoleError
	stage := "boot"
	setStage := func(next string) {
		mu.Lock()
		stage = next
		mu.Unlock()
	}
	page.OnResponse(func(response playwright.Response) {
		if response.Status() < 400 {
			return
		}
		mu.Lock()
		failedResponses = append(failedResponses, goshtosoDependencyResponseFailure{
			URL:        response.URL(),
			RequestURL: response.Request().URL(),
			Status:     response.Status(),
		})
		mu.Unlock()
	})
	page.OnRequestFailed(func(request playwright.Request) {
		failure := request.Failure()
		failureText := "unknown request failure"
		if failure != nil {
			failureText = failure.Error()
		}
		mu.Lock()
		failedRequests = append(failedRequests, goshtosoDependencyRequestFailure{URL: request.URL(), Error: failureText})
		mu.Unlock()
	})
	page.OnPageError(func(err error) {
		mu.Lock()
		pageErrors = append(pageErrors, stage+": "+err.Error())
		mu.Unlock()
	})
	page.On("console", func(message playwright.ConsoleMessage) {
		if message.Type() != "error" {
			return
		}
		mu.Lock()
		consoleErrors = append(consoleErrors, goshtosoDependencyConsoleErrorFrom(message))
		mu.Unlock()
	})

	_, err = page.Goto(server+"/?selected=operation-listpets#operation-listpets", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`async () => await window.goshtosoDependencies.ready`, nil); err != nil {
		t.Fatalf("await Goshtoso dependency readiness: %v", err)
	}

	type dependencyEvidence struct {
		Fallbacks  []string          `json:"fallbacks"`
		Ready      int               `json:"ready"`
		Errors     []string          `json:"errors"`
		Rejections []string          `json:"rejections"`
		Sources    map[string]string `json:"sources"`
	}
	raw, err := page.Evaluate(`() => JSON.stringify({
		fallbacks: window.__manjaDependencyEvents.fallbacks,
		ready: window.__manjaDependencyEvents.ready,
		errors: window.__manjaDependencyEvents.errors,
		rejections: window.__manjaDependencyEvents.rejections,
		sources: window.goshtosoDependencies.sources
	})`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var evidence dependencyEvidence
	if err := json.Unmarshal([]byte(fmt.Sprint(raw)), &evidence); err != nil {
		t.Fatalf("decode browser dependency evidence: %v (%#v)", err, raw)
	}
	wantFallbacks := []string{}
	wantSource := "primary"
	if forceFallback {
		wantFallbacks = []string{"alpine-collapse", "alpine-focus", "alpine-mask", "alpine", "htmx"}
		wantSource = "fallback"
	}
	if fmt.Sprint(evidence.Fallbacks) != fmt.Sprint(wantFallbacks) {
		t.Errorf("fallback events = %v, want %v", evidence.Fallbacks, wantFallbacks)
	}
	if evidence.Ready != 1 {
		t.Errorf("ready event count = %d, want 1", evidence.Ready)
	}
	if len(evidence.Errors) != 0 {
		t.Errorf("dependency error events = %v, want none", evidence.Errors)
	}
	if len(evidence.Rejections) != 0 {
		t.Errorf("unhandled rejections = %v, want none", evidence.Rejections)
	}
	for _, name := range []string{"alpine-collapse", "alpine-focus", "alpine-mask", "alpine", "htmx"} {
		if evidence.Sources[name] != wantSource {
			t.Errorf("%s source = %q, want %s", name, evidence.Sources[name], wantSource)
		}
	}
	if evidence.Sources["combobox"] != "primary" {
		t.Errorf("combobox source = %q, want primary", evidence.Sources["combobox"])
	}

	setStage("disclosure")
	disclosure := page.Locator(`aside a[aria-controls="tag-pets-children"]`)
	if err := disclosure.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#tag-pets-children").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}
	if expanded, err := disclosure.GetAttribute("aria-expanded"); err != nil || expanded != "false" {
		t.Fatalf("collapsed disclosure aria-expanded = %q, err = %v", expanded, err)
	}
	if err := disclosure.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#tag-pets-children").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatal(err)
	}

	setStage("search")
	if err := page.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.activeElement && document.activeElement.id === "docs-search-input"`, nil); err != nil {
		t.Fatalf("search focus after CDN fallback: %v", err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#docs-search-dialog").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatal(err)
	}

	setStage("htmx-navigation")
	openSidebarTagGroup(t, page, "tag-stores-children")
	storeLink := page.Locator(`aside a[href="/?selected=operation-createstore#operation-createstore"]`)
	if err := storeLink.Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-createstore:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	if got, want := page.URL(), server+"/?selected=operation-createstore#operation-createstore"; got != want {
		t.Fatalf("HTMX navigation URL = %q, want %q", got, want)
	}
	setStage("history-back")
	if _, err := page.GoBack(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-listpets:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
	setStage("history-forward")
	if _, err := page.GoForward(); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-createstore:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}

	overflow, err := page.Evaluate(`() => Math.max(document.documentElement.scrollWidth, document.body.scrollWidth) > window.innerWidth`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if overflow == true {
		t.Error("public docs page has horizontal overflow after dependency fallback interactions")
	}
	rawRejections, err := page.Evaluate(`() => JSON.stringify(window.__manjaDependencyEvents.rejections)`, nil)
	if err != nil {
		t.Fatalf("read final unhandled rejection evidence: %v", err)
	}
	var finalRejections []string
	if err := json.Unmarshal([]byte(fmt.Sprint(rawRejections)), &finalRejections); err != nil {
		t.Fatalf("decode final unhandled rejection evidence: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, failure := range validateGoshtosoDependencyBrowserEvidence(forceFallback, expectedPrimaryURLs, goshtosoDependencyBrowserEvidence{
		FailedResponses:        failedResponses,
		FailedRequests:         failedRequests,
		ConsoleErrors:          consoleErrors,
		PageErrors:             pageErrors,
		Rejections:             finalRejections,
		InterceptedPrimaryURLs: interceptedPrimaryURLs,
		RouteErrors:            routeErrors,
	}) {
		t.Error(failure)
	}
}

func goshtosoFallbackIndex() core.SpecIndex {
	return core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{
			{ID: "listPets", Anchor: "operation-listpets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}},
			{ID: "createStore", Anchor: "operation-createstore", Method: "POST", Path: "/stores", Summary: "Create store", Tags: []string{"Stores"}},
		},
		Search: []core.SearchDocument{
			{ID: "operation-listpets", Title: "GET /pets", Description: "List pets", Href: "#operation-listpets", Kind: "Operation", Section: "Pets"},
			{ID: "operation-createstore", Title: "POST /stores", Description: "Create store", Href: "#operation-createstore", Kind: "Operation", Section: "Stores"},
		},
	}
}
