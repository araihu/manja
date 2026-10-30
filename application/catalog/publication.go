package catalog

import (
	"github.com/araihu/manja/domain"
	"sort"
	"strings"
)

// PublicationDocumentLabel is shared by server and exported client search.
func PublicationDocumentLabel(document DocumentDirectoryV1) string {
	title := strings.TrimSpace(document.Title)
	if title == "" {
		title = document.Key
	}
	if document.Publication == nil {
		return title
	}
	return title + " · " + document.Publication.API + " · " + document.Publication.Label(document.APIVersion)
}

// PreferredDocuments returns one entry per explicitly identified API. Legacy
// documents without publication metadata remain independent entries.
func PreferredDocuments(directory CatalogArtifactV1) []DocumentDirectoryV1 {
	byAPI := map[string]DocumentDirectoryV1{}
	for _, d := range directory.Documents {
		key := "document:" + d.Key
		if d.Publication != nil {
			key = "api:" + d.Publication.API
		}
		old, ok := byAPI[key]
		if !ok {
			byAPI[key] = d
			continue
		}
		a, b := d.Publication, old.Publication
		if a.Latest && !b.Latest || a.Latest == b.Latest && (a.Channel == "next" && b.Channel != "next" || a.Channel == b.Channel && domain.ComparePublicationVersions(a.Version, b.Version) > 0) {
			byAPI[key] = d
		}
	}
	result := make([]DocumentDirectoryV1, 0, len(byAPI))
	for _, d := range byAPI {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}
