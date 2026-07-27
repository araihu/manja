package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"golang.org/x/net/html"

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
			assertNodeCount(t, doc, func(node *html.Node) bool {
				return node.Type == html.ElementNode && node.Data == "button" && attribute(node, "aria-label") == test.label
			}, 1, "mobile navigation trigger")
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
			assertNodeCount(t, doc, func(node *html.Node) bool {
				return node.Type == html.ElementNode && attribute(node, "data-manja-primary-scroll") == "true"
			}, 1, "primary scroll region")
		})
	}
}

func parseRenderedDocument(t *testing.T, component templ.Component) *html.Node {
	t.Helper()

	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render component: %v", err)
	}
	doc, err := html.Parse(strings.NewReader(output.String()))
	if err != nil {
		t.Fatalf("parse rendered HTML: %v", err)
	}
	return doc
}

func assertApplicationShellContract(t *testing.T, doc *html.Node, mobileTriggerLabel string) {
	t.Helper()

	assertNodeCount(t, doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "a" && attribute(node, "href") == "#main-content"
	}, 1, "skip link")
	assertNodeCount(t, doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "header"
	}, 1, "header landmark")
	assertNodeCount(t, doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "main" && attribute(node, "id") == "main-content" && attribute(node, "tabindex") == "-1"
	}, 1, "focusable main landmark")
	assertNodeCount(t, doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "button" && attribute(node, "aria-label") == mobileTriggerLabel
	}, 1, "mobile navigation trigger")
	assertNodeCount(t, doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && attribute(node, "data-manja-primary-scroll") == "true"
	}, 1, "primary scroll region")
}

func assertNodeCount(t *testing.T, root *html.Node, match func(*html.Node) bool, want int, label string) {
	t.Helper()

	got := 0
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if match(node) {
			got++
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if got != want {
		t.Fatalf("%s count = %d, want %d", label, got, want)
	}
}

func attribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
