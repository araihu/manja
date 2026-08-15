package templates

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"testing"

	"github.com/a-h/templ"
	"github.com/araihu/goshtoso/components/table"
	localrender "github.com/araihu/manja/internal/localdocs/render"
)

func TestPreparedCatalogDocumentTableMatchesLegacyFullAndHTMXBytes(t *testing.T) {
	data := CatalogPageData{
		Mount:           "/kubernetes",
		DocumentSortBy:  "operations",
		DocumentSortDir: "desc",
		Documents: []CatalogDocumentOption{
			{Key: "core-v1", Label: "core-v1", Version: "v1", Operations: 2, Schemas: 1, SearchText: "core-v1 v1", Href: "/kubernetes/documents/core-v1/", AvatarSrc: "/logos/core.svg"},
			{Key: "apps-v1", Label: "apps-v1", Version: "v1", Operations: 1, Schemas: 3, SearchText: "apps-v1 v1", Href: "/kubernetes/documents/apps-v1/", AvatarSrc: "/logos/apps.svg"},
		},
	}
	legacyConfig := catalogDocumentTableParityConfig(data)
	legacyFull := renderTemplateBytes(t, table.Table(legacyConfig))
	legacyRows := renderTemplateBytes(t, catalogDocumentTableParityRows(legacyConfig))
	entries := []localrender.CatalogDocumentTableEntry{
		{Key: "core-v1", Label: "core-v1", Version: "v1", Operations: 2, Schemas: 1, SearchText: "core-v1 v1", Href: "/kubernetes/documents/core-v1/", AvatarSrc: "/logos/core.svg"},
		{Key: "apps-v1", Label: "apps-v1", Version: "v1", Operations: 1, Schemas: 3, SearchText: "apps-v1 v1", Href: "/kubernetes/documents/apps-v1/", AvatarSrc: "/logos/apps.svg"},
	}
	fragment, err := localrender.PrepareCatalogDocumentTable(data.Mount, data.DocumentSortBy, data.DocumentSortDir, entries)
	if err != nil {
		t.Fatal(err)
	}
	preparedFull, err := fragment.PageBytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	preparedRows, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preparedFull, legacyFull) {
		t.Fatalf("prepared catalog table changed full SSR bytes:\nlegacy=%s\nprepared=%s", legacyFull, preparedFull)
	}
	if !bytes.Equal(preparedRows, legacyRows) {
		t.Fatalf("prepared catalog table changed HTMX bytes:\nlegacy=%s\nprepared=%s", legacyRows, preparedRows)
	}
}

func catalogDocumentTableParityConfig(data CatalogPageData) table.Config {
	rows := make([]table.Row, 0, len(data.Documents))
	for _, option := range data.Documents {
		rows = append(rows, table.Row{
			ID: option.Key, Link: option.Href, LinkMode: table.LinkFull,
			Cells: map[string]table.Cell{
				"name":       {Component: table.ImageCell(option.AvatarSrc, option.Label, "")},
				"version":    {Text: option.Version, Code: true},
				"operations": {Text: strconv.Itoa(option.Operations)},
				"schemas":    {Text: strconv.Itoa(option.Schemas)},
			},
			AlpineAttrs: map[string]string{
				"data-catalog-document-row": "true",
				"data-search-text":          option.SearchText,
				"x-show":                    "filter.trim() === '' || " + strconv.Quote(option.SearchText) + ".includes(filter.toLowerCase())",
			},
		})
	}
	return table.Config{
		ID: "catalog-documents-table", Caption: "OpenAPI documents in this catalog",
		Columns: []table.Column{
			{Key: "name", Label: "Name", Sortable: true, Width: "w-[46%]"},
			{Key: "version", Label: "Version", Sortable: true, Width: "w-[22%]"},
			{Key: "operations", Label: "Operations", Sortable: true, Width: "w-[16%]", Align: "right"},
			{Key: "schemas", Label: "Schemas", Sortable: true, Width: "w-[16%]", Align: "right"},
		},
		Rows: rows, SortBy: data.DocumentSortBy, SortDir: table.SortDir(data.DocumentSortDir),
		HTMX: &table.HTMXConfig{Endpoint: "/kubernetes/"}, RootClass: "catalog-documents-table",
	}
}

func catalogDocumentTableParityRows(config table.Config) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if err := table.TableRows(config).Render(ctx, writer); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, `<template><thead id="`+config.TheadID()+`" hx-swap-oob="innerHTML" data-table-sort-by="`+config.SortBy+`" data-table-sort-dir="`+string(config.SortDir)+`">`); err != nil {
			return err
		}
		if err := table.TableHeadContent(config).Render(ctx, writer); err != nil {
			return err
		}
		_, err := io.WriteString(writer, `</thead></template>`)
		return err
	})
}

func renderTemplateBytes(t *testing.T, component templ.Component) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
