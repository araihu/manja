package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/goshtoso/components/table"
	"github.com/araihu/manja/domain"
)

const (
	maximumCatalogDocumentTableRows  = 256
	maximumCatalogDocumentTableBytes = 1024
)

var errInvalidCatalogDocumentTableFragment = errors.New("local docs catalog document table fragment is invalid")

// CatalogDocumentTableEntry contains the immutable directory values consumed
// by one catalog document table row. It excludes source and request state.
type CatalogDocumentTableEntry struct {
	Key        string
	Label      string
	Version    string
	Operations int
	Schemas    int
	SearchText string
	Href       string
	AvatarSrc  string
}

// CatalogDocumentTableFragment owns copied catalog directory rows and the
// route/sort state used by the Goshtoso table. It performs no source parsing,
// network access, or browser activation.
type CatalogDocumentTableFragment struct {
	data  catalogDocumentTableData
	valid bool
}

type catalogDocumentTableData struct {
	Endpoint string
	SortBy   string
	SortDir  string
	Entries  []catalogDocumentTableEntry
}

type catalogDocumentTableEntry struct {
	Key        string
	Label      string
	Version    string
	Operations int
	Schemas    int
	SearchText string
	Href       string
	AvatarSrc  string
}

// PrepareCatalogDocumentTable validates and copies the bounded document
// directory used by both the complete catalog overview and its HTMX table
// update. The renderer remains independent of internal/web and adapters.
//
// Keep this strict wrapper for callers that render request-sized responses.
// Static export and other explicitly unbounded renderers should call
// PrepareCatalogDocumentTableWithResourceLimits with resourceLimits=false.
func PrepareCatalogDocumentTable(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry) (CatalogDocumentTableFragment, error) {
	return PrepareCatalogDocumentTableWithResourceLimits(mount, sortBy, sortDir, entries, true)
}

// PrepareCatalogDocumentTableWithResourceLimits validates and copies the
// document directory while applying the row limit only when resourceLimits is
// true. Row identity, href, text, and rendered-fragment limits remain enabled
// in both modes.
func PrepareCatalogDocumentTableWithResourceLimits(mount, sortBy, sortDir string, entries []CatalogDocumentTableEntry, resourceLimits bool) (CatalogDocumentTableFragment, error) {
	endpoint, ok := catalogDocumentTableEndpoint(mount)
	if !ok {
		return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("endpoint")
	}
	if !validCatalogDocumentTableSort(sortBy, sortDir) {
		return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("sort state")
	}
	if resourceLimits && len(entries) > maximumCatalogDocumentTableRows {
		return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("row inventory")
	}
	data := catalogDocumentTableData{
		Endpoint: endpoint,
		SortBy:   sortBy,
		SortDir:  sortDir,
		Entries:  make([]catalogDocumentTableEntry, 0, len(entries)),
	}
	seenKeys := make(map[string]struct{}, len(entries))
	seenHrefs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if domain.ValidateCatalogDocumentKey(entry.Key) != nil || !validCatalogDocumentTableText(entry.Label) || strings.TrimSpace(entry.Label) == "" || !validCatalogDocumentTableText(entry.Version) ||
			!validCatalogDocumentTableText(entry.SearchText) || !validCatalogDocumentTableText(entry.AvatarSrc) ||
			entry.Operations < 0 || entry.Schemas < 0 {
			return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("row identity")
		}
		if _, exists := seenKeys[entry.Key]; exists {
			return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("duplicate row key")
		}
		if _, exists := seenHrefs[entry.Href]; exists {
			return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("duplicate row href")
		}
		if entry.SearchText != CatalogDocumentTableSearchText(entry.Key, entry.Label, entry.Version) {
			return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("search identity")
		}
		expectedHref := strings.TrimSuffix(endpoint, "/") + "/documents/" + entry.Key + "/"
		if entry.Href != expectedHref || !validDocumentHref(entry.Href) {
			return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("row href")
		}
		seenKeys[entry.Key] = struct{}{}
		seenHrefs[entry.Href] = struct{}{}
		data.Entries = append(data.Entries, catalogDocumentTableEntry{
			Key: entry.Key, Label: entry.Label, Version: entry.Version, Operations: entry.Operations,
			Schemas: entry.Schemas, SearchText: entry.SearchText, Href: entry.Href, AvatarSrc: entry.AvatarSrc,
		})
	}
	fragment := CatalogDocumentTableFragment{data: data, valid: true}
	if err := renderCatalogDocumentTable(fragment.data, false); err != nil {
		return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("rendered rows")
	}
	if err := renderCatalogDocumentTable(fragment.data, true); err != nil {
		return CatalogDocumentTableFragment{}, invalidCatalogDocumentTableField("rendered table")
	}
	return fragment, nil
}

// CatalogDocumentTableSearchText returns the stable, lowercase filter value
// for a document row. Keep the internal key available for deep-link searches,
// while including the human title so filtering works on what users see.
func CatalogDocumentTableSearchText(key, label, version string) string {
	if strings.TrimSpace(label) == "" || strings.TrimSpace(label) == strings.TrimSpace(key) {
		return strings.ToLower(strings.TrimSpace(key + " " + version))
	}
	return strings.ToLower(strings.TrimSpace(key + " " + label + " " + version))
}

// CatalogDocumentTable renders the complete table used by catalog SSR.
func CatalogDocumentTable(fragment CatalogDocumentTableFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogDocumentTableFragment
		}
		output, err := renderCatalogDocumentTableWithContext(ctx, fragment.data, true)
		if err != nil {
			return err
		}
		_, err = writer.Write(output)
		return err
	})
}

// CatalogDocumentTableRows renders the HTMX rows and out-of-band header used
// by the catalog table sort endpoint.
func CatalogDocumentTableRows(fragment CatalogDocumentTableFragment) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidCatalogDocumentTableFragment
		}
		output, err := renderCatalogDocumentTableWithContext(ctx, fragment.data, false)
		if err != nil {
			return err
		}
		_, err = writer.Write(output)
		return err
	})
}

// Bytes returns the HTMX table-update representation. Full-page callers use
// CatalogDocumentTable, while this method keeps the existing endpoint shape.
func (fragment CatalogDocumentTableFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogDocumentTableRows(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

// PageBytes returns the complete table representation used inside the catalog
// overview, useful for exact full-page parity tests.
func (fragment CatalogDocumentTableFragment) PageBytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := CatalogDocumentTable(fragment).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func renderCatalogDocumentTable(data catalogDocumentTableData, full bool) error {
	_, err := renderCatalogDocumentTableWithContext(context.Background(), data, full)
	return err
}

func renderCatalogDocumentTableWithContext(ctx context.Context, data catalogDocumentTableData, full bool) ([]byte, error) {
	var output boundedBuffer
	var err error
	if full {
		err = catalogDocumentTable(data).Render(ctx, &output)
	} else {
		err = catalogDocumentTableRows(data).Render(ctx, &output)
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func catalogDocumentTableConfig(data catalogDocumentTableData) table.Config {
	rows := make([]table.Row, 0, len(data.Entries))
	for _, entry := range data.Entries {
		rows = append(rows, table.Row{
			ID: entry.Key, Link: entry.Href, LinkMode: table.LinkFull,
			Cells: map[string]table.Cell{
				"name":       {Component: table.ImageCell(entry.AvatarSrc, entry.Label, "")},
				"version":    {Text: entry.Version, Code: true},
				"operations": {Text: strconv.Itoa(entry.Operations)},
				"schemas":    {Text: strconv.Itoa(entry.Schemas)},
			},
			AlpineAttrs: map[string]string{
				"data-catalog-document-row": "true",
				"data-search-text":          entry.SearchText,
				"x-show":                    catalogDocumentTableFilterExpression(entry.SearchText),
			},
		})
	}
	return table.Config{
		ID: "catalog-documents-table", Caption: "OpenAPI documents in this catalog", Columns: []table.Column{
			{Key: "name", Label: "Name", Sortable: true, Width: "w-[46%]"},
			{Key: "version", Label: "Version", Sortable: true, Width: "w-[22%]"},
			{Key: "operations", Label: "Operations", Sortable: true, Width: "w-[16%]", Align: "right"},
			{Key: "schemas", Label: "Schemas", Sortable: true, Width: "w-[16%]", Align: "right"},
		},
		Rows: rows, SortBy: data.SortBy, SortDir: table.SortDir(data.SortDir),
		HTMX: &table.HTMXConfig{Endpoint: data.Endpoint}, RootClass: "catalog-documents-table",
	}
}

func catalogDocumentTableFilterExpression(searchText string) string {
	return "filter.trim() === '' || " + strconv.Quote(strings.ToLower(searchText)) + ".includes(filter.toLowerCase())"
}

func catalogDocumentTableEndpoint(mount string) (string, bool) {
	if !utf8.ValidString(mount) || mount == "" || len(mount) > maximumCatalogDocumentTableBytes || strings.ContainsAny(mount, `\\%?#`) {
		return "", false
	}
	trimmed := strings.TrimSuffix(mount, "/")
	if trimmed == "" {
		if mount != "/" {
			return "", false
		}
		return "/", true
	}
	if domain.ValidateCanonicalPublicPath("catalog table mount", trimmed, false) != nil || path.Clean(trimmed) != trimmed || strings.Contains(trimmed, "//") {
		return "", false
	}
	for _, character := range mount {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return "", false
		}
	}
	return trimmed + "/", true
}

func validCatalogDocumentTableSort(sortBy, sortDir string) bool {
	if sortBy == "" && sortDir == "" {
		return true
	}
	if sortDir != "asc" && sortDir != "desc" {
		return false
	}
	switch sortBy {
	case "name", "version", "operations", "schemas":
		return true
	default:
		return false
	}
}

func validCatalogDocumentTableText(value string) bool {
	if !utf8.ValidString(value) || len(value) > maximumCatalogDocumentTableBytes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func invalidCatalogDocumentTableField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidCatalogDocumentTableFragment, strings.TrimSpace(name))
}
