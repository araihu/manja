package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCatalogOverviewReadmesRenderSanitizedMarkdown(t *testing.T) {
	t.Parallel()
	for _, organization := range []bool{false, true} {
		name := "catalog"
		if organization {
			name = "organization"
		}
		t.Run(name, func(t *testing.T) {
			for _, test := range []struct {
				name  string
				input string
				want  []string
			}{
				{"links", "Browse [documentation](https://example.com/docs/).", []string{`href="https://example.com/docs/"`, ">documentation</a>"}},
				{"blocks", "## Getting started\n\n- **Install** the client\n- Browse the API", []string{"<h2", "Getting started</h2>", "<ul>", "<li>", "<strong>Install</strong>"}},
				{"plain", "Plain text & more.", []string{"<p>Plain text &amp; more.</p>"}},
				{"unsafe", `<script>alert(1)</script><img src=x onerror=alert(2)> [unsafe](javascript:alert(3)) [data](data:text/html,unsafe) [safe](https://example.com/)`, []string{`href="https://example.com/"`}},
			} {
				t.Run(test.name, func(t *testing.T) {
					data := catalogTemplateFixture()
					data.OrganizationRoot = organization
					data.Organization.Title = "Example APIs"
					data.Organization.Readme = test.input
					data.CatalogReadme = test.input
					var output bytes.Buffer
					if err := CatalogMainFragment(data).Render(context.Background(), &output); err != nil {
						t.Fatal(err)
					}
					body := output.String()
					for _, want := range append(test.want, `class="manja-markdown"`) {
						if !strings.Contains(body, want) {
							t.Errorf("README missing %q", want)
						}
					}
					for _, unsafe := range []string{"<script", "<img", "onerror=", "javascript:", "data:text/html"} {
						if strings.Contains(body, unsafe) {
							t.Errorf("README contains unsafe markup %q", unsafe)
						}
					}
				})
			}
		})
	}
}
