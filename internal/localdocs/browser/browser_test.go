package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/localdocs"
)

func TestBrowserRendersDocumentUnseenOperationAndSchema(t *testing.T) {
	descriptor, manifest, catalogBytes, children, operationID, schemaID := browserFixture(t)
	browser, err := Prepare(descriptor, manifest, catalogBytes, children)
	if err != nil {
		t.Fatal(err)
	}
	page, err := browser.Render(context.Background(), Route{DocumentKey: "doc"})
	if err != nil || page.Title != "Pets" || !strings.Contains(page.MainHTML, "title=\"Pets\">Pets</h1>") || !strings.Contains(page.MainHTML, "Operations") {
		t.Fatalf("document page = %#v, %v", page, err)
	}
	page, err = browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(operationID)})
	if err != nil || page.Title != "List pets" || !strings.Contains(page.MainHTML, "GET") || !strings.Contains(page.MainHTML, "/pets") || !strings.Contains(page.SidebarHTML, `aria-current="page"`) {
		t.Fatalf("operation page = %#v, %v", page, err)
	}
	page, err = browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(schemaID)})
	if err != nil || page.Title != "Pet" || !strings.Contains(page.MainHTML, "Pet") || !strings.Contains(page.MainHTML, "object") {
		t.Fatalf("schema page = %#v, %v", page, err)
	}
	if records, err := browser.Search(context.Background(), "pets"); err != nil || len(records) != 0 {
		t.Fatalf("empty search = %#v, %v", records, err)
	}
}

func TestBrowserSidebarRetainsDocumentNavigationChrome(t *testing.T) {
	descriptor, manifest, catalogBytes, children, operationID, _ := browserFixture(t)
	browser, err := Prepare(descriptor, manifest, catalogBytes, children)
	if err != nil {
		t.Fatal(err)
	}

	overview, err := browser.Render(context.Background(), Route{DocumentKey: "doc"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-manja-static-sidebar-top="true"`,
		`data-manja-static-sidebar-top-link="true"`,
		`href="/docs/"`,
		`Back to organization`,
		`href="/docs/pets/documents/doc/"`,
		`Spec overview`,
		`data-manja-static-sidebar-section="paths"`,
		`data-manja-static-sidebar-heading="paths"`,
		`data-catalog-sidebar-selected="true" aria-current="page"`,
	} {
		if !strings.Contains(overview.SidebarHTML, want) {
			t.Errorf("overview sidebar missing %q: %s", want, overview.SidebarHTML)
		}
	}

	operation, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(operationID)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(operation.SidebarHTML, `Back to organization`) || !strings.Contains(operation.SidebarHTML, `Spec overview`) {
		t.Fatalf("operation sidebar lost document navigation chrome: %s", operation.SidebarHTML)
	}
	if strings.Contains(operation.SidebarHTML, `id="catalog-sidebar-spec-overview" data-manja-static-route="true" data-catalog-sidebar-selected="true"`) {
		t.Fatalf("operation sidebar marked spec overview active: %s", operation.SidebarHTML)
	}
}

func TestBrowserSidebarUsesSharedVisualContractAndPlainTextLabels(t *testing.T) {
	operation := browserSidebarLink("/docs/", "doc", domain.DetailID("operation"), "Adds an <code>issue</code> safely.", "get", "operation")
	for _, want := range []string{
		`data-catalog-sidebar-item="true"`,
		`data-catalog-sidebar-operation="true"`,
		`data-catalog-method="GET"`,
		`data-catalog-sidebar-selected="true"`,
		`aria-current="page"`,
		`title="Adds an issue safely."`,
		`<span class="min-w-0 flex-1 truncate">Adds an issue safely.</span></a>`,
	} {
		if !strings.Contains(operation, want) {
			t.Errorf("operation sidebar link missing %q: %s", want, operation)
		}
	}
	if strings.Contains(operation, "&lt;code&gt;") {
		t.Fatalf("operation sidebar link exposed literal markup: %s", operation)
	}

	schema := browserSidebarLink("/docs/", "doc", domain.DetailID("schema"), "Pet", "", "")
	if !strings.Contains(schema, `data-catalog-sidebar-item="true"`) || strings.Contains(schema, "data-catalog-sidebar-operation") || strings.Contains(schema, "data-catalog-method") {
		t.Fatalf("schema sidebar link contract = %s", schema)
	}
}

func TestBrowserSidebarClosedGroupOverridesSelectedAutoOpen(t *testing.T) {
	descriptor, manifest, catalogBytes, children, operationID, _ := browserFixture(t)
	browser, err := Prepare(descriptor, manifest, catalogBytes, children)
	if err != nil {
		t.Fatal(err)
	}
	document, ok := browser.document("doc")
	if !ok {
		t.Fatal("browser fixture document missing")
	}
	groupID := browserGroupID("operations-Pets")
	page, err := browser.Render(context.Background(), Route{DocumentKey: document.Key, Selected: string(operationID), ClosedGroups: []string{groupID}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page.SidebarHTML, `id="`+groupID+`-items"`) {
		t.Fatalf("explicitly closed selected group should not render children: %s", page.SidebarHTML)
	}
	if !strings.Contains(page.SidebarHTML, `data-manja-static-group="`+groupID+`"`) || !strings.Contains(page.SidebarHTML, `aria-expanded="false"`) {
		t.Fatalf("explicitly closed selected group should remain discoverable and collapsed: %s", page.SidebarHTML)
	}
	if !strings.Contains(page.Canonical, "closed="+url.QueryEscape(groupID)) {
		t.Fatalf("canonical route should preserve closed group state: %q", page.Canonical)
	}
}

func TestBrowserRendersSidebarWithoutDetailChild(t *testing.T) {
	descriptor, manifest, catalogBytes, _, operationID, _ := browserFixture(t)
	browser, err := Prepare(descriptor, manifest, catalogBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := browser.RenderSidebar(Route{DocumentKey: "doc", Selected: string(operationID), Groups: []string{"group"}})
	if err != nil {
		t.Fatalf("sidebar render without detail child: %v", err)
	}
	if page.MainHTML != "" || !strings.Contains(page.SidebarHTML, `data-manja-static-group`) || !strings.Contains(page.Canonical, "selected=") {
		t.Fatalf("sidebar-only page = %#v", page)
	}
}

func TestBrowserPreparesOnlyVerifiedChildrenNeededByRoute(t *testing.T) {
	descriptor, manifest, catalogBytes, children, operationID, schemaID := browserFixture(t)
	browser, err := Prepare(descriptor, manifest, catalogBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc"}); err != nil {
		t.Fatalf("document route needs no projection children: %v", err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(operationID)}); err == nil {
		t.Fatal("operation route rendered without its detail child")
	}

	detailOnly := map[string][]byte{"details/doc.json": children["details/doc.json"]}
	browser, err = Prepare(descriptor, manifest, catalogBytes, detailOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(operationID)}); err != nil {
		t.Fatalf("operation route with detail child: %v", err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(schemaID)}); err == nil {
		t.Fatal("schema route rendered without its schema-node child")
	}

	for path, data := range children {
		if strings.HasPrefix(path, "schema-nodes/") {
			detailOnly[path] = data
		}
	}
	browser, err = Prepare(descriptor, manifest, catalogBytes, detailOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(schemaID)}); err != nil {
		t.Fatalf("schema route with selected shard: %v", err)
	}
}

func TestBrowserAdmitsProjectionChildrenIncrementally(t *testing.T) {
	descriptor, manifest, catalogBytes, children, operationID, schemaID := browserFixture(t)
	browser, err := Prepare(descriptor, manifest, catalogBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc"}); err != nil {
		t.Fatalf("overview render before child admission: %v", err)
	}

	detail := append([]byte(nil), children["details/doc.json"]...)
	if err := browser.AdmitChild("details/doc.json", detail); err != nil {
		t.Fatalf("admit detail child: %v", err)
	}
	detail[0] = 'x'
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(operationID)}); err != nil {
		t.Fatalf("operation render after detail admission: %v", err)
	}
	if err := browser.AdmitChild("details/doc.json", children["details/doc.json"]); err != nil {
		t.Fatalf("idempotent detail admission: %v", err)
	}

	if err := browser.AdmitChildren(map[string][]byte{
		"schema-nodes/doc-000000.json": children["schema-nodes/doc-000000.json"],
		"details/unknown.json":         []byte(`{}`),
	}); err == nil {
		t.Fatal("mixed valid and unknown children were admitted")
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(schemaID)}); err == nil {
		t.Fatal("schema child was partially admitted after a rejected batch")
	}
	if err := browser.AdmitChild("schema-nodes/doc-000000.json", children["schema-nodes/doc-000000.json"]); err != nil {
		t.Fatalf("admit schema child: %v", err)
	}
	if _, err := browser.Render(context.Background(), Route{DocumentKey: "doc", Selected: string(schemaID)}); err != nil {
		t.Fatalf("schema render after shard admission: %v", err)
	}
}

func TestBrowserPreparesVerifiedCatalogAboveRuntimeByteLimit(t *testing.T) {
	descriptor, manifestBytes, catalogBytes, children, _, _ := browserFixture(t)
	directory, err := catalogjson.DecodeCatalog(catalogBytes)
	if err != nil {
		t.Fatal(err)
	}
	directory.Title = strings.Repeat("x", 4<<20)
	catalogBytes, err = json.Marshal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalogBytes) <= 4<<20 {
		t.Fatalf("large catalog fixture = %d bytes, want more than 4 MiB", len(catalogBytes))
	}
	manifest, err := catalogjson.DecodeManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	identity := browserIdentity("catalog.json", "catalog", catalogBytes)
	for index := range manifest.Children {
		if manifest.Children[index].Path == identity.Path {
			manifest.Children[index] = identity
			manifest.Identity.Children[index] = identity
		}
	}
	identityBytes, err := json.Marshal(manifest.Identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityBytes)
	manifest.SnapshotID = catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:]))
	manifestBytes, err = catalogjson.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := localdocs.PrepareStaticDescriptor("pets", catalog.RuntimeSnapshot{ID: manifest.SnapshotID, Directory: directory, Manifest: manifest}, "/docs/pets/", "/docs/")
	if !ok {
		t.Fatal("PrepareStaticDescriptor failed")
	}
	if _, err := Prepare(descriptor, manifestBytes, catalogBytes, children); err != nil {
		t.Fatalf("Prepare rejected verified static catalog above runtime byte limit: %v", err)
	}
}

func TestBrowserRejectsUnknownOrChangedChildren(t *testing.T) {
	descriptor, manifest, catalogBytes, children, _, _ := browserFixture(t)
	for name, mutate := range map[string]func(map[string][]byte){
		"changed": func(values map[string][]byte) {
			for childPath := range values {
				values[childPath] = append(values[childPath], 'x')
				break
			}
		},
		"extra": func(values map[string][]byte) { values["details/extra.json"] = []byte(`{}`) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := make(map[string][]byte, len(children)+1)
			for childPath, data := range children {
				candidate[childPath] = append([]byte(nil), data...)
			}
			mutate(candidate)
			if browser, err := Prepare(descriptor, manifest, catalogBytes, candidate); err == nil || browser != nil {
				t.Fatalf("Prepare admitted %s mutation: %#v, %v", name, browser, err)
			}
		})
	}
}

func browserFixture(t *testing.T) (localdocs.DescriptorV1, []byte, []byte, map[string][]byte, domain.DetailID, domain.DetailID) {
	t.Helper()
	operationID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	schemaID := domain.DetailID("detail-sha256-" + strings.Repeat("b", 64))
	operationHref := "documents/doc/?selected=" + string(operationID) + "#" + string(operationID)
	schemaHref := "documents/doc/?selected=" + string(schemaID) + "#" + string(schemaID)
	detailBytes, err := catalogjson.EncodeDetailShard(catalog.DetailShardV1{SchemaVersion: 1, DocumentKey: "doc", Records: []catalog.DetailRecordV1{
		{ID: operationID, Kind: "operation", Operation: &projection.OperationDetail{ID: string(operationID), Anchor: string(operationID), Href: operationHref, HeadingID: string(operationID), Heading: "List pets", HeadingLevel: 2, Method: "GET", Path: "/pets", Summary: "List pets", Description: "Returns pets", Tags: []projection.TextRecord{{Ordinal: 0, ID: "tag-pets", Value: "Pets"}}}},
		{ID: schemaID, Kind: "schema", Schema: &projection.SchemaDetail{ID: string(schemaID), Anchor: string(schemaID), Href: schemaHref, HeadingID: string(schemaID), Heading: "Pet", HeadingLevel: 2, Description: "A pet", SchemaRef: 0, ExampleSchemaJSON: `{"name":"Fido"}`}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	schemaBytes, err := catalogjson.EncodeSchemaNodeShard(catalog.SchemaNodeShardV1{SchemaVersion: 1, DocumentKey: "doc", FirstOrdinal: 0, Nodes: []projection.SchemaNode{{Ordinal: 0, ID: "node-pet", Name: "Pet", Type: "object", Description: "A pet", JSON: `{"type":"object"}`}}})
	if err != nil {
		t.Fatal(err)
	}
	searchBytes, err := catalogjson.EncodeSearchDirectory(catalog.SearchDirectoryV1{SchemaVersion: 1, SearchVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	schemaIdentity := browserIdentity("schema-nodes/doc-000000.json", "schema-node", schemaBytes)
	directory := catalog.CatalogArtifactV1{SchemaVersion: 1, CatalogID: "pets", Title: "Pets", DefaultDocumentKey: "doc", SearchChild: "search/directory.json", Documents: []catalog.DocumentDirectoryV1{{
		Key: "doc", SourcePath: "doc.json", Title: "Pets", APIVersion: "v1", SourceChild: "sources/doc.json",
		Operations:       []catalog.OperationDirectoryV1{{DetailID: operationID, OperationID: "listPets", Method: "GET", Path: "/pets", Title: "List pets", Description: "Returns pets", Href: operationHref, DetailChild: "details/doc.json", Tags: []string{"Pets"}}},
		Schemas:          []catalog.SchemaDirectoryV1{{DetailID: schemaID, Name: "Pet", Description: "A pet", Href: schemaHref, DetailChild: "details/doc.json", CanonicalSHA256: strings.Repeat("c", 64), ProjectionSHA256: strings.Repeat("d", 64)}},
		SchemaNodeShards: []catalog.ShardReferenceV1{{Path: schemaIdentity.Path, FirstOrdinal: 0, LastOrdinal: 0, Records: 1, Length: schemaIdentity.Length, SHA256: schemaIdentity.SHA256}},
	}}}
	catalogBytes, err := catalogjson.EncodeCatalog(directory)
	if err != nil {
		t.Fatal(err)
	}
	sourceBytes := []byte(`{"openapi":"3.0.3"}`)
	children := []catalog.ChildIdentityV1{
		browserIdentity("catalog.json", "catalog", catalogBytes),
		browserIdentity("details/doc.json", "detail", detailBytes),
		schemaIdentity,
		browserIdentity("search/directory.json", "search-directory", searchBytes),
		browserIdentity("sources/doc.json", "source", sourceBytes),
	}
	identity := catalog.SnapshotIdentityV1{SchemaVersion: 1, CatalogID: "pets", RevisionID: "files-sha256-abc", SourceManifestSHA256: strings.Repeat("e", 64), Versions: catalog.CompilerVersions{ProjectionFormat: "projection-v2"}, Children: append([]catalog.ChildIdentityV1(nil), children...)}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityBytes)
	snapshotID := catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:]))
	manifestValue := catalog.ManifestV1{SchemaVersion: 1, SnapshotID: snapshotID, Identity: identity, Children: children}
	manifestBytes, err := catalogjson.EncodeManifest(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := localdocs.PrepareStaticDescriptor("pets", catalog.RuntimeSnapshot{ID: snapshotID, Directory: directory, Manifest: manifestValue}, "/docs/pets/", "/docs/")
	if !ok {
		t.Fatal("PrepareStaticDescriptor failed")
	}
	return descriptor, manifestBytes, catalogBytes, map[string][]byte{
		"details/doc.json": detailBytes, schemaIdentity.Path: schemaBytes, "search/directory.json": searchBytes,
	}, operationID, schemaID
}

func browserIdentity(pathValue, kind string, data []byte) catalog.ChildIdentityV1 {
	digest := sha256.Sum256(data)
	return catalog.ChildIdentityV1{Path: pathValue, Kind: kind, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:])}
}
