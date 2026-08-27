package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/localdocs"
	localrender "github.com/araihu/manja/internal/localdocs/render"
	"html"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

type Browser struct {
	descriptor localdocs.DescriptorV1
	activation localdocs.Activation
	manifest   catalog.ManifestV1
	directory  catalog.CatalogArtifactV1
	children   map[string][]byte
	search     *catalog.SearchService
}

type Route struct {
	DocumentKey string
	Selected    string
	Node        *uint32
	Groups      []string
}

type Page struct {
	MainHTML    string `json:"mainHtml"`
	SidebarHTML string `json:"sidebarHtml"`
	Title       string `json:"title"`
	Canonical   string `json:"canonical"`
}

func Prepare(descriptor localdocs.DescriptorV1, manifestBytes, catalogBytes []byte, children map[string][]byte) (*Browser, error) {
	if descriptor.Static == nil {
		return nil, errors.New("local docs browser requires a static descriptor")
	}
	activation, err := localdocs.Admit(descriptor, manifestBytes)
	if err != nil {
		return nil, err
	}
	manifest, err := catalogjson.DecodeManifest(manifestBytes)
	if err != nil {
		return nil, errors.New("local docs browser manifest is invalid")
	}
	directory, err := catalogjson.DecodeCatalogWithResourceLimits(catalogBytes, false)
	if err != nil || directory.CatalogID != descriptor.CatalogID {
		return nil, errors.New("local docs browser catalog is invalid")
	}
	if err := catalogjson.ValidateCatalogManifest(directory, manifest); err != nil {
		return nil, errors.New("local docs browser catalog differs from manifest")
	}
	catalogIdentity, exists := browserChildIdentity(manifest, "catalog.json")
	if !exists || !verifiedBrowserBytes(catalogIdentity, catalogBytes) {
		return nil, errors.New("local docs browser catalog bytes differ")
	}
	want := make(map[string]catalog.ChildIdentityV1)
	for _, child := range manifest.Children {
		if strings.HasPrefix(child.Kind, "search-") || child.Kind == "detail" || child.Kind == "schema-node" {
			want[child.Path] = child
		}
	}
	owned := make(map[string][]byte, len(children))
	for childPath, data := range children {
		identity, ok := want[childPath]
		if !ok || !verifiedBrowserBytes(identity, data) {
			return nil, fmt.Errorf("local docs browser child %q differs", childPath)
		}
		owned[childPath] = append([]byte(nil), data...)
	}
	var search *catalog.SearchService
	if searchBytes, ok := owned[directory.SearchChild]; ok {
		searchDirectory, err := catalogjson.DecodeSearchDirectory(searchBytes)
		if err != nil || catalogjson.ValidateSearchManifest(searchDirectory, manifest) != nil {
			return nil, errors.New("local docs browser search directory is invalid")
		}
		runtimeSnapshot := catalog.RuntimeSnapshot{ID: manifest.SnapshotID, Directory: directory, Search: searchDirectory, Manifest: manifest}
		search, err = catalog.NewRuntimeSearchService(runtimeSnapshot, catalog.NewSearchCache(), func(_ context.Context, childPath string) ([]byte, catalog.ChildIdentityV1, error) {
			data, ok := owned[childPath]
			identity, declared := browserChildIdentity(manifest, childPath)
			if !ok || !declared {
				return nil, catalog.ChildIdentityV1{}, errors.New("local docs browser search child is missing")
			}
			return append([]byte(nil), data...), identity, nil
		})
		if err != nil {
			return nil, err
		}
	}
	return &Browser{descriptor: descriptor, activation: activation, manifest: manifest, directory: directory, children: owned, search: search}, nil
}

func (browser *Browser) Render(ctx context.Context, route Route) (Page, error) {
	if browser == nil {
		return Page{}, errors.New("local docs browser is not prepared")
	}
	document, ok := browser.document(route.DocumentKey)
	if !ok {
		return Page{}, errors.New("local docs document is missing")
	}
	documentHref := browser.descriptor.PublicationBase + "documents/" + document.Key + "/"
	sidebar := browser.renderSidebar(document, route)
	sidebar = browser.deploymentHTML(sidebar)
	if route.Selected == "" {
		main, err := browser.renderDocument(ctx, document, documentHref)
		return Page{MainHTML: browser.deploymentHTML(main), SidebarHTML: sidebar, Title: document.Key, Canonical: documentHref}, err
	}
	detail, err := browser.detail(document, domain.DetailID(route.Selected))
	if err != nil {
		return Page{}, err
	}
	canonical := documentHref + "?selected=" + url.QueryEscape(route.Selected) + "#" + url.PathEscape(route.Selected)
	if detail.Operation != nil {
		main, title, err := browser.renderOperation(ctx, document, detail, documentHref, route.Groups)
		return Page{MainHTML: browser.deploymentHTML(main), SidebarHTML: sidebar, Title: title, Canonical: canonical}, err
	}
	if detail.Schema != nil {
		main, title, err := browser.renderSchema(ctx, document, detail, documentHref, route.Node)
		if route.Node != nil {
			canonical = documentHref + "?node=" + strconv.FormatUint(uint64(*route.Node), 10) + "&selected=" + url.QueryEscape(route.Selected) + "#schema-node-panel"
		}
		return Page{MainHTML: browser.deploymentHTML(main), SidebarHTML: sidebar, Title: title, Canonical: canonical}, err
	}
	return Page{}, errors.New("local docs detail kind is invalid")
}

func (browser *Browser) deploymentHTML(value string) string {
	base := browser.descriptor.Static.DeploymentBase
	if base == "/" {
		return value
	}
	value = strings.ReplaceAll(value, `="/assets/`, `="`+base+`assets/`)
	return strings.ReplaceAll(value, `="/manja-assets/`, `="`+base+`manja-assets/`)
}

func (browser *Browser) Search(ctx context.Context, query string) ([]catalog.SearchRecordV1, error) {
	if browser == nil || browser.search == nil {
		return nil, errors.New("local docs browser search is not prepared")
	}
	result, err := browser.search.Search(ctx, catalog.SnapshotID(browser.descriptor.SnapshotID), query)
	if err != nil {
		return nil, err
	}
	records := append([]catalog.SearchRecordV1(nil), result.Results...)
	for index := range records {
		records[index].Href = browser.descriptor.PublicationBase + strings.TrimPrefix(records[index].Href, "/")
		records[index].Documents = append([]string(nil), records[index].Documents...)
	}
	return records, nil
}

func (browser *Browser) renderDocument(ctx context.Context, document catalog.DocumentDirectoryV1, documentHref string) (string, error) {
	download := browser.descriptor.PublicationBase + "snapshots/" + browser.descriptor.SnapshotID + "/openapi/" + document.Key + path.Ext(document.SourceChild)
	header, err := localrender.PrepareCatalogDocumentHeader(document, documentHref, download)
	if err != nil {
		return "", err
	}
	info, err := localrender.PrepareCatalogDocumentInfo(document)
	if err != nil {
		return "", err
	}
	metrics, err := localrender.PrepareCatalogDocumentMetrics(document)
	if err != nil {
		return "", err
	}
	security, err := localrender.PrepareCatalogDocumentSecuritySchemes(document)
	if err != nil {
		return "", err
	}
	fragments := []func() ([]byte, error){
		func() ([]byte, error) { return header.Bytes(ctx, nil) },
		func() ([]byte, error) { return info.Bytes(ctx) },
		func() ([]byte, error) { return metrics.Bytes(ctx) },
		func() ([]byte, error) { return security.Bytes(ctx) },
	}
	return renderBrowserFragments(fragments)
}

func (browser *Browser) renderOperation(ctx context.Context, document catalog.DocumentDirectoryV1, detail catalog.DetailRecordV1, documentHref string, groups []string) (string, string, error) {
	projected := detail.Operation
	directory, ok := browser.operation(document, detail.ID)
	if !ok {
		return "", "", errors.New("local docs operation directory is missing")
	}
	operation := domain.Operation{
		ID: directory.OperationID, Anchor: projected.Anchor, Title: projected.Heading, Method: projected.Method,
		Path: projected.Path, Summary: projected.Summary, Description: projected.Description, Deprecated: projected.Deprecated,
		Tags: browserTextValues(projected.Tags),
	}
	if operation.ID == "" {
		operation.ID = string(detail.ID)
	}
	header, err := localrender.PrepareOperationHeader(detail, operation, documentHref)
	if err != nil {
		return "", "", err
	}
	open := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		open[group] = struct{}{}
	}
	navigation, err := localrender.PrepareOperationNavigation(detail, operation, document, documentHref, open)
	if err != nil {
		return "", "", err
	}
	main, err := renderBrowserFragments([]func() ([]byte, error){
		func() ([]byte, error) { return header.Bytes(ctx, nil, nil) },
		func() ([]byte, error) { return navigation.Bytes(ctx) },
	})
	return main, projected.Heading, err
}

func (browser *Browser) renderSchema(ctx context.Context, document catalog.DocumentDirectoryV1, detail catalog.DetailRecordV1, documentHref string, selected *uint32) (string, string, error) {
	ordinal := uint32(detail.Schema.SchemaRef)
	if selected != nil {
		ordinal = *selected
	}
	node, err := browser.schemaNode(document, ordinal)
	if err != nil {
		return "", "", err
	}
	references := make([]projection.SchemaNode, 0, len(node.Items)+len(node.Properties))
	seen := make(map[projection.SchemaRef]struct{})
	for _, ref := range browserNodeReferences(node) {
		if ref == projection.SchemaRef(node.Ordinal) {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		reference, err := browser.schemaNode(document, uint32(ref))
		if err != nil {
			return "", "", err
		}
		seen[ref] = struct{}{}
		references = append(references, reference)
	}
	sort.Slice(references, func(i, j int) bool { return references[i].Ordinal < references[j].Ordinal })
	nodeFragment, err := localrender.PrepareSchemaNode(detail, node, references, documentHref)
	if err != nil {
		return "", "", err
	}
	schema := domain.Schema{Name: detail.Schema.Heading, Description: detail.Schema.Description, Example: domain.SchemaExample{JSON: detail.Schema.ExampleSchemaJSON}}
	header, err := localrender.PrepareSchemaDetailHeader(detail, schema, document, &nodeFragment)
	if err != nil {
		return "", "", err
	}
	var example *localrender.SchemaDetailExampleFragment
	if detail.Schema.ExampleSchemaJSON != "" {
		prepared, err := localrender.PrepareSchemaDetailExample(detail, schema, document)
		if err != nil {
			return "", "", err
		}
		example = &prepared
	}
	body, err := localrender.PrepareSchemaDetailBody(detail, schema, document, &nodeFragment, example)
	if err != nil {
		return "", "", err
	}
	fragment, err := localrender.PrepareSchemaDetail(detail, schema, document, &header, &body)
	if err != nil {
		return "", "", err
	}
	main, err := fragment.Bytes(ctx, nil, nil)
	return string(main), detail.Schema.Heading, err
}

func (browser *Browser) renderSidebar(document catalog.DocumentDirectoryV1, route Route) string {
	open := make(map[string]struct{}, len(route.Groups))
	for _, group := range route.Groups {
		open[group] = struct{}{}
	}
	defaultOpen := len(route.Groups) == 0 && len(document.Operations)+len(document.Schemas) <= 600
	var output strings.Builder
	output.WriteString(`<nav data-manja-local-sidebar="true" aria-label="API navigation">`)
	type group struct {
		id, label  string
		operations []catalog.OperationDirectoryV1
	}
	byLabel := make(map[string][]catalog.OperationDirectoryV1)
	for _, operation := range document.Operations {
		label := localrender.OperationGroupLabel(operation)
		byLabel[label] = append(byLabel[label], operation)
	}
	groups := make([]group, 0, len(byLabel))
	for label, operations := range byLabel {
		groups = append(groups, group{id: browserGroupID("operations-" + label), label: label, operations: operations})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].label < groups[j].label })
	for _, item := range groups {
		_, explicitlyOpen := open[item.id]
		opened := defaultOpen || explicitlyOpen || browserGroupContains(item.operations, route.Selected)
		output.WriteString(`<section data-manja-sidebar-group="` + item.id + `"><button type="button" data-manja-static-group="` + item.id + `" data-catalog-group-control="true" aria-controls="` + item.id + `-items" aria-expanded="` + strconv.FormatBool(opened) + `">` + html.EscapeString(item.label) + `</button>`)
		if opened {
			output.WriteString(`<div id="` + item.id + `-items" data-manja-sidebar-items="true">`)
			for _, operation := range item.operations {
				output.WriteString(browserSidebarLink(browser.descriptor.PublicationBase, document.Key, operation.DetailID, operation.Title, operation.Method, route.Selected))
			}
			output.WriteString(`</div>`)
		}
		output.WriteString(`</section>`)
	}
	if len(document.Schemas) > 0 {
		id := browserGroupID("schemas")
		_, explicitlyOpen := open[id]
		opened := defaultOpen || explicitlyOpen
		for _, schema := range document.Schemas {
			opened = opened || string(schema.DetailID) == route.Selected
		}
		output.WriteString(`<section data-manja-sidebar-group="` + id + `"><button type="button" data-manja-static-group="` + id + `" data-catalog-group-control="true" aria-controls="` + id + `-items" aria-expanded="` + strconv.FormatBool(opened) + `">Schemas</button>`)
		if opened {
			output.WriteString(`<div id="` + id + `-items" data-manja-sidebar-items="true">`)
			for _, schema := range document.Schemas {
				output.WriteString(browserSidebarLink(browser.descriptor.PublicationBase, document.Key, schema.DetailID, schema.Name, "", route.Selected))
			}
			output.WriteString(`</div>`)
		}
		output.WriteString(`</section>`)
	}
	output.WriteString(`</nav>`)
	return output.String()
}

func (browser *Browser) detail(document catalog.DocumentDirectoryV1, detailID domain.DetailID) (catalog.DetailRecordV1, error) {
	childPath := ""
	for _, operation := range document.Operations {
		if operation.DetailID == detailID {
			childPath = operation.DetailChild
		}
	}
	for _, schema := range document.Schemas {
		if schema.DetailID == detailID {
			childPath = schema.DetailChild
		}
	}
	if childPath == "" {
		return catalog.DetailRecordV1{}, errors.New("local docs detail is missing")
	}
	return browser.activation.SelectDetail(childPath, document.Key, detailID, browser.children[childPath])
}

func (browser *Browser) schemaNode(document catalog.DocumentDirectoryV1, ordinal uint32) (projection.SchemaNode, error) {
	index := sort.Search(len(document.SchemaNodeShards), func(index int) bool { return document.SchemaNodeShards[index].LastOrdinal >= ordinal })
	if index == len(document.SchemaNodeShards) || ordinal < document.SchemaNodeShards[index].FirstOrdinal {
		return projection.SchemaNode{}, errors.New("local docs schema node is missing")
	}
	reference := document.SchemaNodeShards[index]
	return browser.activation.SelectSchemaNode(reference.Path, document.Key, ordinal, browser.children[reference.Path])
}

func (browser *Browser) document(key string) (catalog.DocumentDirectoryV1, bool) {
	for _, document := range browser.directory.Documents {
		if document.Key == key {
			return document, true
		}
	}
	return catalog.DocumentDirectoryV1{}, false
}

func (browser *Browser) operation(document catalog.DocumentDirectoryV1, id domain.DetailID) (catalog.OperationDirectoryV1, bool) {
	for _, operation := range document.Operations {
		if operation.DetailID == id {
			return operation, true
		}
	}
	return catalog.OperationDirectoryV1{}, false
}

func renderBrowserFragments(fragments []func() ([]byte, error)) (string, error) {
	var output bytes.Buffer
	output.WriteString(`<div data-manja-local-main="true">`)
	for _, render := range fragments {
		data, err := render()
		if err != nil {
			return "", err
		}
		output.Write(data)
	}
	output.WriteString(`</div>`)
	return output.String(), nil
}

func browserChildIdentity(manifest catalog.ManifestV1, childPath string) (catalog.ChildIdentityV1, bool) {
	for _, child := range manifest.Children {
		if child.Path == childPath {
			return child, true
		}
	}
	return catalog.ChildIdentityV1{}, false
}

func verifiedBrowserBytes(identity catalog.ChildIdentityV1, data []byte) bool {
	if uint64(len(data)) != identity.Length {
		return false
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == identity.SHA256
}

func browserNodeReferences(node projection.SchemaNode) []projection.SchemaRef {
	result := make([]projection.SchemaRef, 0, len(node.Items)+len(node.Properties))
	for _, item := range node.Items {
		result = append(result, item.SchemaRef)
	}
	for _, property := range node.Properties {
		result = append(result, property.SchemaRef)
	}
	return result
}

func browserTextValues(records []projection.TextRecord) []string {
	result := make([]string, len(records))
	for index, record := range records {
		result[index] = record.Value
	}
	return result
}

func browserGroupID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "group-" + hex.EncodeToString(digest[:6])
}

func browserGroupContains(operations []catalog.OperationDirectoryV1, selected string) bool {
	for _, operation := range operations {
		if string(operation.DetailID) == selected {
			return true
		}
	}
	return false
}

func browserSidebarLink(publicationBase, documentKey string, id domain.DetailID, label, method, selected string) string {
	href := publicationBase + "documents/" + url.PathEscape(documentKey) + "/?selected=" + url.QueryEscape(string(id)) + "#" + url.PathEscape(string(id))
	attributes := ` data-catalog-sidebar-item="true"`
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != "" {
		attributes += ` data-catalog-sidebar-operation="true" data-catalog-method="` + html.EscapeString(method) + `"`
	}
	if string(id) == selected {
		attributes += ` data-catalog-sidebar-selected="true" aria-current="page"`
	}
	label = html.EscapeString(browserPlainText(label))
	return `<a data-manja-static-route="true" href="` + html.EscapeString(href) + `" title="` + label + `"` + attributes + `>` + label + `</a>`
}

func browserPlainText(value string) string {
	var output strings.Builder
	insideTag := false
	for _, character := range value {
		switch character {
		case '<':
			insideTag = true
		case '>':
			if insideTag {
				insideTag = false
				continue
			}
			output.WriteRune(character)
		default:
			if !insideTag {
				output.WriteRune(character)
			}
		}
	}
	return strings.TrimSpace(html.UnescapeString(output.String()))
}
