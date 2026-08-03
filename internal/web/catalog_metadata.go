package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/araihu/manja/internal/web/templates"
)

func (handler *CatalogHandler) catalogPageMetadata(request *http.Request, data templates.CatalogPageData) templates.PageMetadata {
	presentation := handler.presentation[data.Mount]
	description := strings.TrimSpace(presentation.Description)
	if data.Selected != nil && data.Selected.Operation != nil {
		description = firstNonempty(data.Selected.Operation.Description, data.Selected.Operation.Summary, strings.TrimSpace(strings.ToUpper(data.Selected.Operation.Method)+" "+data.Selected.Operation.Path))
	} else if data.Selected != nil && data.Selected.Schema != nil {
		description = firstNonempty(data.Selected.Schema.Description, "OpenAPI schema "+data.Selected.Schema.Heading+" in "+data.Directory.Title+".")
	} else if data.Document != nil {
		description = firstNonempty(data.Document.Overview.Description, "OpenAPI operations and schemas for "+data.Document.Title+".")
	} else if data.Search != nil {
		description = "Search operations, paths, and schemas across " + data.Directory.Title + "."
	}
	if description == "" {
		description = "Browse OpenAPI operations and schemas in " + data.Directory.Title + "."
	}
	metadata := templates.PageMetadata{
		Title: catalogPageTitleForMetadata(data), Description: description,
		SocialImageURL: presentation.SocialImage, SocialImageAlt: presentation.SocialImageAlt,
		Robots: "index,follow",
	}
	if data.Search != nil {
		metadata.Robots = "noindex,follow"
	}
	metadata.CanonicalURL = catalogCanonicalURL(request, data.Mount, presentation.CanonicalBase)
	return metadata
}

func catalogPageTitleForMetadata(data templates.CatalogPageData) string {
	if data.Selected != nil && data.Selected.Operation != nil {
		return data.Selected.Operation.Heading + " · " + data.Directory.Title
	}
	if data.Selected != nil && data.Selected.Schema != nil {
		return data.Selected.Schema.Heading + " · " + data.Directory.Title
	}
	if data.Search != nil {
		return "Search · " + data.Directory.Title
	}
	if data.Document != nil {
		return data.Document.Title + " · " + data.Directory.Title
	}
	return data.Directory.Title
}

func catalogCanonicalURL(request *http.Request, mount, base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	relative := request.URL.EscapedPath()
	if mount != "/" {
		relative = strings.TrimPrefix(relative, mount)
	}
	if relative == "" {
		relative = "/"
	}
	canonical := base + relative
	query := url.Values{}
	for _, key := range []string{"selected", "node"} {
		if value := request.URL.Query().Get(key); value != "" {
			query.Set(key, value)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		canonical += "?" + encoded
	}
	return canonical
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
