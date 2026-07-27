package templates

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/a-h/templ"

	core "github.com/araihu/manja/domain"
)

func TestPublicDocsUsesOneGoshtosoApplicationShell(t *testing.T) {
	doc := parseRenderedDocument(t, PublicDocsWithOptions(core.SpecIndex{
		Title:   "Petstore",
		Version: "1.0.0",
	}, "", PublicDocsOptions{}))

	assertApplicationShellContract(t, doc, "Open API sections")
}

func TestManagementUsesOneGoshtosoApplicationShell(t *testing.T) {
	doc := parseRenderedDocument(t, ManagementOverview(ManagementOverviewModel{
		Specs: []ManagedSpecModel{{ID: "payments", Title: "Payments API", Version: "v1"}},
	}))

	assertApplicationShellContract(t, doc, "Open management sections")
}

func TestApplicationShellsExposeOneMobileNavigationTrigger(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		content templ.Component
	}{
		{
			name:  "public",
			label: "Open API sections",
			content: PublicDocs(core.SpecIndex{
				Title: "Petstore",
			}, ""),
		},
		{
			name:  "management",
			label: "Open management sections",
			content: ManagementSpecsPage(ManagementOverviewModel{
				Specs: []ManagedSpecModel{{ID: "payments", Title: "Payments API"}},
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := parseRenderedDocument(t, test.content)
			assertTagCount(t, doc, "button", map[string]string{"aria-label": test.label}, 1, "mobile navigation trigger")
		})
	}
}

func TestApplicationShellsExposeOnePrimaryScrollRegion(t *testing.T) {
	tests := []struct {
		name    string
		content templ.Component
	}{
		{name: "public", content: PublicDocs(core.SpecIndex{Title: "Petstore"}, "")},
		{name: "management", content: ManagementOverview(ManagementOverviewModel{})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := parseRenderedDocument(t, test.content)
			assertElementCount(t, doc, map[string]string{"data-manja-primary-scroll": "true"}, 1, "primary scroll region")
		})
	}
}

func TestManagementRoutesExposeOnePageHeading(t *testing.T) {
	model := ManagementOverviewModel{
		Specs:          []ManagedSpecModel{{ID: "payments", Title: "Payments API", Version: "v1"}},
		SelectedSpecID: "payments",
	}
	for _, test := range []struct {
		name    string
		content templ.Component
	}{
		{name: "overview", content: ManagementOverview(model)},
		{name: "specs", content: ManagementSpecsPage(model)},
		{name: "detail", content: ManagementSpecPage(model)},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := parseRenderedDocument(t, test.content)
			assertTagCount(t, doc, "h1", nil, 1, "page h1")
		})
	}
}

func parseRenderedDocument(t *testing.T, component templ.Component) string {
	t.Helper()

	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render component: %v", err)
	}
	document := regexp.MustCompile(`(?is)<script\b.*?</script>`).ReplaceAllString(output.String(), "")
	return regexp.MustCompile(`(?is)<style\b.*?</style>`).ReplaceAllString(document, "")
}

func assertApplicationShellContract(t *testing.T, doc string, mobileTriggerLabel string) {
	t.Helper()

	assertTagCount(t, doc, "a", map[string]string{"href": "#main-content"}, 1, "skip link")
	assertTagCount(t, doc, "header", map[string]string{"data-boot-anim": "header"}, 1, "application header landmark")
	assertTagCount(t, doc, "main", map[string]string{"id": "main-content", "tabindex": "-1"}, 1, "focusable main landmark")
	assertTagCount(t, doc, "button", map[string]string{"aria-label": mobileTriggerLabel}, 1, "mobile navigation trigger")
	assertElementCount(t, doc, map[string]string{"data-manja-primary-scroll": "true"}, 1, "primary scroll region")
}

func assertTagCount(t *testing.T, document string, tag string, attributes map[string]string, want int, label string) {
	t.Helper()
	pattern := regexp.MustCompile(`(?i)<` + regexp.QuoteMeta(tag) + `\b[^>]*>`)
	assertMatchingTagCount(t, pattern.FindAllString(document, -1), attributes, want, label)
}

func assertElementCount(t *testing.T, document string, attributes map[string]string, want int, label string) {
	t.Helper()
	pattern := regexp.MustCompile(`(?i)<[a-z][a-z0-9-]*\b[^>]*>`)
	assertMatchingTagCount(t, pattern.FindAllString(document, -1), attributes, want, label)
}

func assertMatchingTagCount(t *testing.T, tags []string, attributes map[string]string, want int, label string) {
	t.Helper()
	got := 0
	for _, tag := range tags {
		matches := true
		for key, value := range attributes {
			attributePattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `="` + regexp.QuoteMeta(value) + `"`)
			if !attributePattern.MatchString(tag) {
				matches = false
				break
			}
		}
		if matches {
			got++
		}
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d; tags=%q", label, got, want, tags)
	}
}
