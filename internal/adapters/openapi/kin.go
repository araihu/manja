package openapi

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/araihu/manja/internal/core"
)

type Parser struct{}

func (Parser) Parse(ctx context.Context, file core.SpecFile, rev core.Revision) (core.SpecIndex, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromDataWithPath(file.Bytes, &url.URL{Path: file.Path})
	if err != nil {
		return core.SpecIndex{}, err
	}
	if err := doc.Validate(ctx); err != nil {
		return core.SpecIndex{}, err
	}

	idx := core.SpecIndex{
		RevisionID: rev.ID,
		Title:      doc.Info.Title,
		Version:    doc.Info.Version,
	}

	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			operation := core.Operation{
				ID:          op.OperationID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Summary:     op.Summary,
				Description: op.Description,
				Tags:        append([]string(nil), op.Tags...),
				Deprecated:  op.Deprecated,
			}
			operation.Anchor = operationAnchor(operation)
			idx.Operations = append(idx.Operations, operation)
		}
	}
	sort.Slice(idx.Operations, func(i, j int) bool {
		if idx.Operations[i].Path == idx.Operations[j].Path {
			return idx.Operations[i].Method < idx.Operations[j].Method
		}
		return idx.Operations[i].Path < idx.Operations[j].Path
	})

	for name, schema := range doc.Components.Schemas {
		description := ""
		if schema != nil && schema.Value != nil {
			description = schema.Value.Description
		}
		idx.Schemas = append(idx.Schemas, core.Schema{Name: name, Description: description})
	}
	sort.Slice(idx.Schemas, func(i, j int) bool { return idx.Schemas[i].Name < idx.Schemas[j].Name })

	idx.Search = buildSearch(idx)
	idx.PublicRoutes = buildPublicRoutes(idx)
	return idx, nil
}

func buildSearch(idx core.SpecIndex) []core.SearchDocument {
	docs := make([]core.SearchDocument, 0, len(idx.Operations)+len(idx.Schemas)+1)
	for _, op := range idx.Operations {
		anchor := operationAnchor(op)
		title := fmt.Sprintf("%s %s", op.Method, op.Path)
		docs = append(docs, core.SearchDocument{
			ID:          anchor,
			Title:       title,
			Description: firstNonEmpty(op.Summary, op.Description),
			Href:        "#" + anchor,
			Kind:        "Operation",
			Section:     strings.Join(op.Tags, ", "),
			Keywords:    []string{op.ID, op.Method, op.Path, strings.Join(op.Tags, " ")},
		})
	}
	for _, schema := range idx.Schemas {
		docs = append(docs, core.SearchDocument{
			ID:          "schema-" + schema.Name,
			Title:       schema.Name,
			Description: schema.Description,
			Href:        "#schema-" + strings.ToLower(schema.Name),
			Kind:        "Schema",
			Section:     "Schemas",
			Keywords:    []string{"schema", schema.Name},
		})
	}
	docs = append(docs, core.SearchDocument{
		ID:          "overview",
		Title:       idx.Title,
		Description: "API overview",
		Href:        "/",
		Kind:        "Overview",
	})
	return docs
}

func buildPublicRoutes(idx core.SpecIndex) []core.PublicRoute {
	routes := []core.PublicRoute{{Path: "/", Title: idx.Title, Description: "API overview"}}
	for _, op := range idx.Operations {
		anchor := operationAnchor(op)
		routes = append(routes, core.PublicRoute{
			Path:        "#" + anchor,
			Title:       op.Method + " " + op.Path,
			Description: firstNonEmpty(op.Summary, op.Description),
		})
	}
	return routes
}

func operationAnchor(op core.Operation) string {
	if op.Anchor != "" {
		return op.Anchor
	}
	fragment := anchorFragment(op.ID)
	if fragment == "" {
		fragment = anchorFragment(op.Method + " " + op.Path)
	}
	return "operation-" + fragment
}

func anchorFragment(value string) string {
	var builder strings.Builder
	lastWasSeparator := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastWasSeparator = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastWasSeparator = false
		default:
			if builder.Len() > 0 && !lastWasSeparator {
				builder.WriteByte('-')
				lastWasSeparator = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
