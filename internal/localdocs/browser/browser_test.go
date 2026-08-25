package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	if err != nil || !strings.Contains(page.MainHTML, "doc") || !strings.Contains(page.MainHTML, "Operations") {
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
