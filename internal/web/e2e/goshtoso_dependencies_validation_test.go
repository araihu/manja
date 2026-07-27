package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestValidateGoshtosoDependencyBrowserEvidenceAcceptsIndependentChannelPermutations(t *testing.T) {
	expectedURLs := goshtosoDependencyValidationURLs()
	evidence := goshtosoDependencyValidationEvidence(expectedURLs)
	evidence.InterceptedPrimaryURLs = permuteStrings(evidence.InterceptedPrimaryURLs, 4, 2, 0, 3, 1)
	evidence.FailedResponses = permuteResponses(evidence.FailedResponses, 1, 4, 2, 0, 3)
	evidence.FailedRequests = permuteRequests(evidence.FailedRequests, 3, 0, 4, 1, 2)
	evidence.ConsoleErrors = permuteConsoleErrors(evidence.ConsoleErrors, 2, 3, 1, 4, 0)

	if failures := validateGoshtosoDependencyBrowserEvidence(true, expectedURLs, evidence); len(failures) != 0 {
		t.Fatalf("valid URL-correlated multisets rejected after harmless callback permutation: %v", failures)
	}
}

func TestValidateGoshtosoDependencyBrowserEvidenceAcceptsOptionalCorrelatedRequestFailures(t *testing.T) {
	expectedURLs := goshtosoDependencyValidationURLs()
	for _, testCase := range []struct {
		name           string
		failedRequests []goshtosoDependencyRequestFailure
	}{
		{name: "none"},
		{
			name: "subset in independent order",
			failedRequests: []goshtosoDependencyRequestFailure{
				{URL: expectedURLs[3], Error: "net::ERR_ABORTED"},
				{URL: expectedURLs[0], Error: "browser-specific failure text"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := goshtosoDependencyValidationEvidence(expectedURLs)
			evidence.FailedRequests = testCase.failedRequests
			if failures := validateGoshtosoDependencyBrowserEvidence(true, expectedURLs, evidence); len(failures) != 0 {
				t.Fatalf("optional correlated request failures rejected: %v", failures)
			}
		})
	}
}

func TestValidateGoshtosoDependencyBrowserEvidenceRejectsInvalidFallbackEvidence(t *testing.T) {
	expectedURLs := goshtosoDependencyValidationURLs()
	firstPartyURL := "http://127.0.0.1:8080/assets/styles.css"
	externalURL := "https://example.invalid/unapproved.js"

	for _, testCase := range []struct {
		name        string
		mutate      func(*goshtosoDependencyBrowserEvidence)
		wantFailure string
	}{
		{
			name: "duplicate intercepted URL replaces expected URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.InterceptedPrimaryURLs[4] = expectedURLs[0]
			},
			wantFailure: expectedURLs[4],
		},
		{
			name: "missing intercepted URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.InterceptedPrimaryURLs = evidence.InterceptedPrimaryURLs[:4]
			},
			wantFailure: "intercepted primary URL",
		},
		{
			name: "extra intercepted URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.InterceptedPrimaryURLs = append(evidence.InterceptedPrimaryURLs, externalURL)
			},
			wantFailure: externalURL,
		},
		{
			name: "duplicate response URL replaces expected URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedResponses[4].URL = expectedURLs[0]
				evidence.FailedResponses[4].RequestURL = expectedURLs[0]
			},
			wantFailure: expectedURLs[4],
		},
		{
			name: "missing response",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedResponses = evidence.FailedResponses[:4]
			},
			wantFailure: "failed response",
		},
		{
			name: "extra first-party response",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedResponses = append(evidence.FailedResponses, goshtosoDependencyResponseFailure{
					URL: firstPartyURL, RequestURL: firstPartyURL, Status: http.StatusServiceUnavailable,
				})
			},
			wantFailure: firstPartyURL,
		},
		{
			name: "wrong response URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedResponses[2].URL = externalURL
				evidence.FailedResponses[2].RequestURL = externalURL
			},
			wantFailure: externalURL,
		},
		{
			name: "divergent response request URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedResponses[2].RequestURL = expectedURLs[1]
			},
			wantFailure: "request=" + expectedURLs[1],
		},
		{
			name: "wrong response status",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedResponses[2].Status = http.StatusBadGateway
			},
			wantFailure: "status=502",
		},
		{
			name: "duplicate console URL replaces expected URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.ConsoleErrors[4].URL = expectedURLs[0]
			},
			wantFailure: expectedURLs[4],
		},
		{
			name: "missing console error",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.ConsoleErrors = evidence.ConsoleErrors[:4]
			},
			wantFailure: "console resource error",
		},
		{
			name: "divergent first-party console location",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.ConsoleErrors[2].URL = firstPartyURL
			},
			wantFailure: firstPartyURL,
		},
		{
			name: "arbitrary console error at expected primary URL",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.ConsoleErrors[2].Text = "manja-unrelated-console-error"
			},
			wantFailure: "manja-unrelated-console-error",
		},
		{
			name: "duplicate request failure",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedRequests = append(evidence.FailedRequests, evidence.FailedRequests[0])
			},
			wantFailure: "failed request",
		},
		{
			name: "uncorrelated external request failure",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedRequests = []goshtosoDependencyRequestFailure{{URL: externalURL, Error: "net::ERR_ABORTED"}}
			},
			wantFailure: externalURL,
		},
		{
			name: "uncorrelated first-party request failure",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.FailedRequests = []goshtosoDependencyRequestFailure{{URL: firstPartyURL, Error: "net::ERR_ABORTED"}}
			},
			wantFailure: firstPartyURL,
		},
		{
			name: "page error",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.PageErrors = []string{"manja-pageerror"}
			},
			wantFailure: "manja-pageerror",
		},
		{
			name: "unhandled rejection",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.Rejections = []string{"manja-rejection"}
			},
			wantFailure: "manja-rejection",
		},
		{
			name: "route error",
			mutate: func(evidence *goshtosoDependencyBrowserEvidence) {
				evidence.RouteErrors = []string{"route fulfill failed"}
			},
			wantFailure: "route fulfill failed",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := goshtosoDependencyValidationEvidence(expectedURLs)
			testCase.mutate(&evidence)
			failures := validateGoshtosoDependencyBrowserEvidence(true, expectedURLs, evidence)
			if len(failures) == 0 {
				t.Fatal("invalid fallback evidence accepted")
			}
			if joined := strings.Join(failures, "\n"); !strings.Contains(joined, testCase.wantFailure) {
				t.Fatalf("failures = %q, want diagnostic containing %q", joined, testCase.wantFailure)
			}
		})
	}
}

func TestValidateGoshtosoDependencyBrowserEvidenceNormalModeAcceptsZeroFailures(t *testing.T) {
	if failures := validateGoshtosoDependencyBrowserEvidence(false, goshtosoDependencyValidationURLs(), goshtosoDependencyBrowserEvidence{}); len(failures) != 0 {
		t.Fatalf("normal mode with zero failures rejected: %v", failures)
	}
}

func goshtosoDependencyValidationURLs() []string {
	return []string{
		"https://unpkg.com/dependency-collapse@v0.0.13/index.js",
		"https://unpkg.com/dependency-focus@v0.0.13/index.js",
		"https://unpkg.com/dependency-mask@v0.0.13/index.js",
		"https://unpkg.com/dependency-alpine@v0.0.13/index.js",
		"https://unpkg.com/dependency-htmx@v0.0.13/index.js",
	}
}

func goshtosoDependencyValidationEvidence(expectedURLs []string) goshtosoDependencyBrowserEvidence {
	evidence := goshtosoDependencyBrowserEvidence{
		InterceptedPrimaryURLs: append([]string(nil), expectedURLs...),
	}
	for _, expectedURL := range expectedURLs {
		evidence.FailedResponses = append(evidence.FailedResponses, goshtosoDependencyResponseFailure{
			URL: expectedURL, RequestURL: expectedURL, Status: http.StatusServiceUnavailable,
		})
		evidence.FailedRequests = append(evidence.FailedRequests, goshtosoDependencyRequestFailure{
			URL: expectedURL, Error: "net::ERR_ABORTED",
		})
		evidence.ConsoleErrors = append(evidence.ConsoleErrors, goshtosoDependencyConsoleError{
			Text: "Failed to load resource: the server responded with a status of 503 ()",
			URL:  expectedURL,
		})
	}
	return evidence
}

func permuteStrings(values []string, indexes ...int) []string {
	result := make([]string, len(indexes))
	for i, index := range indexes {
		result[i] = values[index]
	}
	return result
}

func permuteResponses(values []goshtosoDependencyResponseFailure, indexes ...int) []goshtosoDependencyResponseFailure {
	result := make([]goshtosoDependencyResponseFailure, len(indexes))
	for i, index := range indexes {
		result[i] = values[index]
	}
	return result
}

func permuteRequests(values []goshtosoDependencyRequestFailure, indexes ...int) []goshtosoDependencyRequestFailure {
	result := make([]goshtosoDependencyRequestFailure, len(indexes))
	for i, index := range indexes {
		result[i] = values[index]
	}
	return result
}

func permuteConsoleErrors(values []goshtosoDependencyConsoleError, indexes ...int) []goshtosoDependencyConsoleError {
	result := make([]goshtosoDependencyConsoleError, len(indexes))
	for i, index := range indexes {
		result[i] = values[index]
	}
	return result
}
