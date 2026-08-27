package web

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/araihu/manja/internal/web/templates"
)

func (handler *CatalogHandler) catalogPageMetadata(request *http.Request, data templates.CatalogPageData) templates.PageMetadata {
	presentation := handler.presentation[data.Mount]
	if data.OrganizationRoot {
		presentation = handler.organizationPresentation(request)
	}
	description := strings.TrimSpace(presentation.Description)
	if data.OrganizationRoot {
		description = firstNonempty(description, "Browse OpenAPI catalogs and standalone specs published by "+data.Organization.Title+".")
	} else if data.Selected != nil && data.Selected.Operation != nil {
		description = firstNonempty(data.Selected.Operation.Description, data.Selected.Operation.Summary, strings.TrimSpace(strings.ToUpper(data.Selected.Operation.Method)+" "+data.Selected.Operation.Path))
	} else if data.Selected != nil && data.Selected.Schema != nil {
		description = firstNonempty(data.Selected.Schema.Description, "OpenAPI schema "+data.Selected.Schema.Heading+" in "+data.Directory.Title+".")
	} else if data.Document != nil {
		description = firstNonempty(data.Document.Overview.Description, "OpenAPI operations and schemas for "+data.Document.Key+".")
	} else if data.Search != nil {
		description = "Search operations, paths, and schemas across " + data.Directory.Title + "."
	}
	if description == "" {
		description = "Browse OpenAPI operations and schemas in " + data.Directory.Title + "."
	}
	metadata := templates.PageMetadata{
		Title: catalogPageTitleForMetadata(data), Description: description,
		SocialImageURL: presentation.SocialImage, SocialImageMIMEType: presentation.SocialImageMIMEType, SocialImageAlt: presentation.SocialImageAlt,
		Robots: "index,follow",
	}
	if data.Search != nil {
		metadata.Robots = "noindex,follow"
	}
	metadata.CanonicalURL = catalogCanonicalURL(request, data.Mount, presentation.CanonicalBase)
	return metadata
}

func (handler *CatalogHandler) organizationPresentation(request *http.Request) CatalogPresentation {
	presentation := CatalogPresentation{}
	if handler != nil {
		presentation = handler.organization.SEO
		mounts := make([]string, 0, len(handler.presentation))
		for mount := range handler.presentation {
			if mount != "/" {
				mounts = append(mounts, mount)
			}
		}
		sort.Strings(mounts)
		for _, mount := range mounts {
			if origin, ok := absoluteOrigin(handler.presentation[mount].CanonicalBase); ok {
				presentation = organizationPresentationDefaults(presentation, origin)
				return presentation
			}
		}
	}
	if request != nil && request.URL != nil {
		if origin, ok := absoluteOrigin(request.URL.String()); ok {
			return organizationPresentationDefaults(presentation, origin)
		}
	}
	return presentation
}

func organizationPresentationDefaults(presentation CatalogPresentation, origin string) CatalogPresentation {
	if presentation.CanonicalBase == "" {
		presentation.CanonicalBase = origin
	}
	if presentation.SocialImage == "" {
		presentation.SocialImage = origin + "/manja-assets/manja-social.png"
		presentation.SocialImageMIMEType = "image/png"
		presentation.SocialImageAlt = "Manja OpenAPI workbench"
	}
	return presentation
}

func absoluteOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

func catalogPageTitleForMetadata(data templates.CatalogPageData) string {
	if data.OrganizationRoot {
		return catalogManjaDocumentTitle("Dashboard")
	}
	if data.Selected != nil && data.Selected.Operation != nil {
		return catalogManjaDocumentTitle(data.Selected.Operation.Heading)
	}
	if data.Selected != nil && data.Selected.Schema != nil {
		return catalogManjaDocumentTitle(data.Selected.Schema.Heading)
	}
	if data.Search != nil {
		return catalogManjaDocumentTitle("Search")
	}
	if data.Document != nil {
		return catalogManjaDocumentTitle(data.Document.Key)
	}
	return catalogManjaDocumentTitle(data.Directory.Title)
}

func catalogManjaDocumentTitle(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "OpenAPI documentation"
	}
	if strings.HasSuffix(label, " · Manja") {
		return label
	}
	return label + " · Manja"
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
