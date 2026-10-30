package templates

import (
	"fmt"
	"sort"
	"strings"

	selectfield "github.com/araihu/goshtoso/components/select"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

type catalogPublicationOption struct {
	Key, Href, Label, APIVersion string
	Document                     catalog.DocumentDirectoryV1
}
type catalogPublicationGroup struct {
	ID, API, Label, Audience string
	Options                  []catalogPublicationOption
}

// CatalogPublicationSummary labels counts in overviews and organization navigation.
func CatalogPublicationSummary(apis, publications int) string {
	return catalogCountLabel(apis, "API") + " · " + catalogCountLabel(publications, "publication")
}

func catalogCountLabel(count int, singular string) string {
	if count != 1 {
		singular += "s"
	}
	return fmt.Sprintf("%d %s", count, singular)
}

func catalogPublicationOptionFilterText(group catalogPublicationGroup, option catalogPublicationOption) string {
	return strings.ToLower(strings.Join([]string{group.API, group.Label, group.Audience, option.Label, option.Key}, " "))
}

func catalogHeaderClass(data CatalogPageData) string {
	if data.Document != nil && data.Document.Publication != nil {
		return "manja-catalog-header-publication"
	}
	return "flex h-16 items-center justify-between gap-4 px-4 lg:px-8"
}

func catalogPublicationFilterText(group catalogPublicationGroup) string {
	parts := []string{group.ID, group.Label, group.Audience}
	for _, o := range group.Options {
		parts = append(parts, o.Label, o.Key)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func catalogHasPublications(data CatalogPageData) bool {
	for _, d := range data.Directory.Documents {
		if d.Publication != nil {
			return true
		}
	}
	return false
}
func catalogPublicationGroups(data CatalogPageData) []catalogPublicationGroup {
	groups := map[string]*catalogPublicationGroup{}
	for _, d := range data.Directory.Documents {
		id := d.Key
		key := "document:" + id
		label := catalogDocumentTitle(d)
		api := label
		audience := ""
		if p := d.Publication; p != nil {
			id = p.API
			api = p.API
			key = "api:" + id
			audience = p.Audience
		}
		g := groups[key]
		if g == nil {
			g = &catalogPublicationGroup{ID: key, API: api, Label: label, Audience: audience}
			groups[key] = g
		}
		state := "API " + d.APIVersion
		if d.Publication != nil {
			state = d.Publication.Label(d.APIVersion)
		}
		g.Options = append(g.Options, catalogPublicationOption{Key: d.Key, Href: strings.TrimSuffix(data.Mount, "/") + "/documents/" + d.Key + "/", Label: state, APIVersion: d.APIVersion, Document: d})
	}
	result := make([]catalogPublicationGroup, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.Options, func(i, j int) bool {
			a, b := g.Options[i].Document.Publication, g.Options[j].Document.Publication
			if a == nil || b == nil {
				return g.Options[i].Key < g.Options[j].Key
			}
			if a.Latest != b.Latest {
				return a.Latest
			}
			if (a.Channel == "next") != (b.Channel == "next") {
				return a.Channel == "next"
			}
			if c := domain.ComparePublicationVersions(a.Version, b.Version); c != 0 {
				return c > 0
			}
			return g.Options[i].Key < g.Options[j].Key
		})
		g.Label = catalogDocumentTitle(g.Options[0].Document)
		result = append(result, *g)
	}
	defaultAPI := ""
	for _, d := range data.Directory.Documents {
		if d.Key == data.Directory.DefaultDocumentKey && d.Publication != nil {
			defaultAPI = "api:" + d.Publication.API
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if (result[i].ID == defaultAPI) != (result[j].ID == defaultAPI) {
			return result[i].ID == defaultAPI
		}
		return result[i].ID < result[j].ID
	})
	return result
}
func catalogPublicationAPIOptions(data CatalogPageData) []selectfield.Option {
	options := []selectfield.Option{}
	for _, g := range catalogPublicationGroups(data) {
		selected := false
		for _, o := range g.Options {
			if data.Document != nil && o.Key == data.Document.Key {
				selected = true
			}
		}
		options = append(options, selectfield.Option{Value: g.Options[0].Key, Label: g.Label, Selected: selected})
	}
	return options
}
func catalogPublicationVersionOptions(data CatalogPageData) []selectfield.Option {
	options := []selectfield.Option{}
	if data.Document == nil || data.Document.Publication == nil {
		return options
	}
	for _, g := range catalogPublicationGroups(data) {
		if g.ID == "api:"+data.Document.Publication.API {
			for _, o := range g.Options {
				label := o.Label
				if o.Document.Publication != nil && o.Document.Publication.Channel == "next" {
					label = "next · API " + o.APIVersion
				}
				options = append(options, selectfield.Option{Value: o.Key, Label: label, Selected: o.Key == data.Document.Key})
			}
		}
	}
	return options
}
func CatalogPublicationPageTitle(data CatalogPageData) string {
	if data.Document == nil {
		return data.Directory.Title
	}
	label := data.Directory.Title + " · " + catalog.PublicationDocumentLabel(*data.Document)
	if data.Selected != nil && data.Selected.Operation != nil {
		return data.Selected.Operation.Heading + " · " + label
	}
	if data.Selected != nil && data.Selected.Schema != nil {
		return data.Selected.Schema.Heading + " · " + label
	}
	return label
}
func catalogPublicationPageTitle(data CatalogPageData) string {
	return CatalogPublicationPageTitle(data)
}

func catalogPublicationInitialAPI(data CatalogPageData) string {
	for _, option := range catalogPublicationAPIOptions(data) {
		if option.Selected {
			return option.Value
		}
	}
	return ""
}
