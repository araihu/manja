package templates

import (
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func TestPublicationAPILabelsDisambiguateMatchingTitles(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		title := "Internal API"
		if duplicate {
			title = "Shared API"
		}
		documents := []catalog.DocumentDirectoryV1{
			{Key: "public-next", Title: "Shared API", APIVersion: "1.0.0", Publication: &domain.CatalogPublication{API: "public", Channel: "branch", Ref: "refs/heads/main"}},
			{Key: "public-v1", Title: "Shared API", APIVersion: "1.0.0", Publication: &domain.CatalogPublication{API: "public", Channel: "release", Version: "1.0.0", Latest: true}},
			{Key: "internal-next", Title: title, APIVersion: "1.0.0", Publication: &domain.CatalogPublication{API: "internal", Channel: "branch", Ref: "refs/heads/main"}},
		}
		data := CatalogPageData{Mount: "/project", Directory: catalog.CatalogArtifactV1{Documents: documents, DefaultDocumentKey: "public-next"}, Document: &documents[0]}
		options := catalogPublicationAPIOptions(data)
		if len(options) != 2 {
			t.Fatalf("options=%#v", options)
		}
		expected := map[string]string{"public-v1": "Shared API", "internal-next": "Internal API"}
		if duplicate {
			expected["public-v1"] = "Shared API · public"
			expected["internal-next"] = "Shared API · internal"
		}
		for _, option := range options {
			if option.Label != expected[option.Value] {
				t.Fatalf("duplicate=%t option=%#v want=%q", duplicate, option, expected[option.Value])
			}
			if option.Selected != (option.Value == "public-v1") {
				t.Fatalf("selection changed: %#v", option)
			}
		}
		for _, group := range catalogPublicationGroups(data) {
			if duplicate && !strings.HasSuffix(group.Label, " · "+group.API) {
				t.Fatalf("group label omits identity: %#v", group)
			}
		}
	}
}
