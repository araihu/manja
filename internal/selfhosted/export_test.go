package selfhosted

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	artifact "github.com/araihu/manja/application/htmlartifact"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/web"
	"github.com/araihu/manja/renderer"
)

func TestExportBasePathValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"/", "/group/project/", "/a-b_1/"} {
		if err := canonicalExportBasePath(value); err != nil {
			t.Errorf("canonicalExportBasePath(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"", "project/", "/project", "//project/", "/group//project/", "/group/../project/", "/group/./project/", `/group\project/`, "/project%2f/", "/project/?x=1", "/project/#x", "/project name/", "/project\n/"} {
		if err := canonicalExportBasePath(value); err == nil {
			t.Errorf("canonicalExportBasePath(%q) succeeded", value)
		}
	}
}

func TestExportRendererIncludesConfiguredCatalogWithLocalDocsVisibility(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "private.json"), []byte(`{"openapi":"3.0.3","info":{"title":"Private API","version":"v1"},"paths":{"/charges":{"get":{"operationId":"listCharges","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Charge"}}}}}}}},"components":{"schemas":{"Charge":{"type":"object","properties":{"id":{"type":"string"}}}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `version: 1
dataDir: data
catalogs:
  - id: private
    mount: /private
    title: Private
    localDocs:
      public: true
      anonymous: true
      publicationKey: private
    defaultDocument: private
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [private.json]
`
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	receipt, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Catalogs) != 1 || receipt.Catalogs[0].CatalogID != "private" || receipt.Catalogs[0].PublicationKey != "private" {
		t.Fatalf("receipt = %#v", receipt)
	}
	catalogBytes, err := os.ReadFile(filepath.Join(output, "private", "snapshots", receipt.Catalogs[0].SnapshotID, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	directory, err := catalogjson.DecodeCatalogWithResourceLimits(catalogBytes, false)
	if err != nil || len(directory.Documents) != 1 || len(directory.Documents[0].Operations) != 1 {
		t.Fatalf("export catalog directory = %#v, %v", directory, err)
	}
	fragmentPath, sidecarPath, err := artifact.FragmentLocation("/private", "private", artifact.FragmentIdentity{
		Format: artifact.FragmentFormatV2, Kind: artifact.FragmentOperation,
		Resource: string(directory.Documents[0].Operations[0].DetailID),
	})
	if err != nil {
		t.Fatal(err)
	}
	examplePath, exampleSidecar, err := artifact.FragmentLocation("/private", "private", artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentExample, Resource: string(directory.Documents[0].Operations[0].DetailID)})
	if err != nil {
		t.Fatal(err)
	}
	pathPath, pathSidecar, err := artifact.FragmentLocation("/private", "private", artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentPath, Resource: "/charges"})
	if err != nil {
		t.Fatal(err)
	}
	sidebarPath, sidebarSidecar, err := artifact.FragmentLocation("/private", "private", artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: "private:operations:0"})
	if err != nil {
		t.Fatal(err)
	}
	groupID := staticSidebarGroupID("operations-charges")
	groupPath, groupSidecar, err := artifact.FragmentLocation("/private", "private", artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: "private:" + sidebarOperationGroupCollection(groupID) + ":0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "private/index.html", "private/documents/private/index.html", "private/_manja/offline-shell/index.html", deploymentSearchDirectoryPath, fragmentPath, sidecarPath, examplePath, exampleSidecar, pathPath, pathSidecar, sidebarPath, sidebarSidecar, groupPath, groupSidecar, "sw.js", exportIdentityPath, exportManifestPath} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(name))); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	if _, err := VerifyExport(context.Background(), output); err != nil {
		t.Fatalf("VerifyExport: %v", err)
	}
	documentShell, err := os.ReadFile(filepath.Join(output, "private", "documents", "private", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`Spec overview`, `data-manja-sidebar-tabs="true"`, `data-manja-sidebar-tab="operations"`, `data-manja-sidebar-tab="schemas"`} {
		if !strings.Contains(string(documentShell), want) {
			t.Errorf("static document shell lacks sidebar control %q", want)
		}
	}
	if strings.Contains(string(documentShell), `Back to catalog`) || strings.Contains(string(documentShell), `Back to organization`) {
		t.Fatal("static document shell retained redundant sidebar back navigation")
	}
	sidebarChunk, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(sidebarPath)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-manja-sidebar-group=`, `data-manja-static-group=`, `data-catalog-group-control="true"`, `aria-expanded="true"`, `data-manja-sidebar-group-load="true"`, `data-manja-sidebar-collection="operation-group-`} {
		if !strings.Contains(string(sidebarChunk), want) {
			t.Errorf("operation sidebar chunk lacks %q: %s", want, sidebarChunk)
		}
	}
	groupChunk, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(groupPath)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-manja-sidebar-operation-group-chunk="0"`, `catalog-method-get`, `ml-auto`} {
		if !strings.Contains(string(groupChunk), want) {
			t.Errorf("operation group chunk lacks %q: %s", want, groupChunk)
		}
	}
	if labelAt, badgeAt := strings.Index(string(groupChunk), `listCharges</span>`), strings.Index(string(groupChunk), `catalog-method-get`); labelAt < 0 || badgeAt < labelAt {
		t.Fatalf("operation method badge is not after its label: %s", groupChunk)
	}
	body, err := os.ReadFile(filepath.Join(output, "private/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `id="manja-local-docs-descriptor"`) || !strings.Contains(string(body), `"publicationKey":"private"`) || !strings.Contains(string(body), `src="/manja-assets/local-docs.js"`) {
		t.Fatalf("visibility-disabled static shell lacks export authority: %s", body)
	}
	for _, want := range []string{`data-search-global="true"`, `data-search-deployment-directory-url="/_manja/search/directory.json"`, `data-search-deployment-directory-sha256="`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("static shell lacks deployment search binding %q", want)
		}
	}
	fragment, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(fragmentPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fragment), `data-manja-local-main="true"`) || !strings.Contains(string(fragment), `listCharges`) {
		t.Fatalf("operation fragment is incomplete: %s", fragment)
	}
	if strings.Contains(string(fragment), `class="manja-schema-tree"`) || strings.Contains(string(fragment), `data-schema-tree-node`) {
		t.Fatalf("operation fragment retained duplicated schema HTML: %s", fragment)
	}
	resources, err := lazySchemaHTMLResources(fragment)
	if err != nil || len(resources) != 1 {
		t.Fatalf("operation lazy schema resources = %#v, %v", resources, err)
	}
	lazySchemaPath, lazySchemaSidecar, err := artifact.FragmentLocation("/private", "private", artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSchema, Resource: resources[0]})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{lazySchemaPath, lazySchemaSidecar} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(name))); err != nil {
			t.Fatalf("missing lazy schema artifact %s: %v", name, err)
		}
	}
	lazySchemaInfoBefore, err := os.Stat(filepath.Join(output, filepath.FromSlash(lazySchemaPath)))
	if err != nil {
		t.Fatal(err)
	}
	sidecarBefore, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(sidecarPath)))
	if err != nil {
		t.Fatal(err)
	}
	fragmentInfoBefore, err := os.Stat(filepath.Join(output, filepath.FromSlash(fragmentPath)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/"}); err != nil {
		t.Fatalf("incremental export: %v", err)
	}
	fragmentAfter, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(fragmentPath)))
	if err != nil {
		t.Fatal(err)
	}
	sidecarAfter, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(sidecarPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(fragmentAfter) != string(fragment) || string(sidecarAfter) != string(sidecarBefore) {
		t.Fatal("incremental export changed a verified unchanged fragment")
	}
	fragmentInfoAfter, err := os.Stat(filepath.Join(output, filepath.FromSlash(fragmentPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(fragmentInfoBefore, fragmentInfoAfter) {
		t.Fatal("incremental export copied an unchanged fragment instead of linking the verified cache hit")
	}
	lazySchemaInfoAfter, err := os.Stat(filepath.Join(output, filepath.FromSlash(lazySchemaPath)))
	if err != nil {
		t.Fatalf("incremental export omitted cached lazy schema: %v", err)
	}
	if !os.SameFile(lazySchemaInfoBefore, lazySchemaInfoAfter) {
		t.Fatal("incremental export copied an unchanged lazy schema instead of linking the verified cache hit")
	}

	// A document-level dependency must invalidate even an unchanged detail child.
	specPath := filepath.Join(root, "private.json")
	source, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	source = []byte(strings.Replace(string(source), `"title":"Private API"`, `"title":"Updated API"`, 1))
	if err := os.WriteFile(specPath, source, 0600); err != nil {
		t.Fatal(err)
	}
	options := ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/"}
	if _, err := ExportRenderer(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	changedSidecar, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(sidecarPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(changedSidecar) == string(sidecarBefore) {
		t.Fatal("document title change reused the old fragment identity")
	}
	changedFragment, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(fragmentPath)))
	if err != nil {
		t.Fatal(err)
	}
	options.Output = filepath.Join(root, "fresh-changed")
	if _, err := ExportRenderer(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	freshFragment, err := os.ReadFile(filepath.Join(options.Output, filepath.FromSlash(fragmentPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(changedFragment) != string(freshFragment) {
		t.Fatal("invalidated warm fragment differs from fresh export")
	}
}

func TestExportRendererRewritesSubpathDeployment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "api.json"), []byte(`{"openapi":"3.0.3","info":{"title":"API","version":"v1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := `version: 1
dataDir: data
catalogs:
  - id: api
    mount: /api
    title: API
    defaultDocument: api
    profile: strict-v1
    source:
      kind: files
      root: .
      include: [api.json]
`
	configPath := filepath.Join(root, "renderer.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "public")
	if _, err := ExportRenderer(context.Background(), ExportOptions{RendererOptions: RendererOptions{ConfigPath: configPath}, Output: output, BasePath: "/group/project/"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(output, "api/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`src="/group/project/manja-assets/local-docs.js"`, `"deploymentBase":"/group/project/"`, `"publicationBase":"/group/project/api/"`, `href="/group/project/"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("subpath shell missing %q", want)
		}
	}
}

func TestStaticExportCapturesAndVerifiesCatalogAboveRuntimeByteLimit(t *testing.T) {
	searchBytes, err := catalogjson.EncodeSearchDirectory(catalog.SearchDirectoryV1{SchemaVersion: 1, SearchVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	directory := catalog.CatalogArtifactV1{
		SchemaVersion: 1, CatalogID: "large", Title: strings.Repeat("x", 4<<20), SearchChild: "search/directory.json",
	}
	catalogBytes, err := json.Marshal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalogBytes) <= 4<<20 {
		t.Fatalf("large catalog fixture = %d bytes, want more than 4 MiB", len(catalogBytes))
	}
	children := []catalog.ChildIdentityV1{
		exportTestChild("catalog.json", "catalog", catalogBytes),
		exportTestChild("search/directory.json", "search-directory", searchBytes),
	}
	identity := catalog.SnapshotIdentityV1{
		SchemaVersion: 1, CatalogID: "large", RevisionID: "files-sha256-large", SourceManifestSHA256: strings.Repeat("a", 64),
		Versions: catalog.CompilerVersions{ProjectionFormat: "projection-v2"}, Children: children,
	}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityBytes)
	snapshotID := catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:]))
	manifestBytes, err := catalogjson.EncodeManifest(catalog.ManifestV1{SchemaVersion: 1, SnapshotID: snapshotID, Identity: identity, Children: children})
	if err != nil {
		t.Fatal(err)
	}
	active := renderer.ActivationReceipt{CatalogID: "large", Mount: "/large", RevisionID: identity.RevisionID, SnapshotID: string(snapshotID)}
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/large/", "/large/search":
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write([]byte("<!doctype html><html><body></body></html>"))
		case "/large/llms.txt":
			_, _ = response.Write([]byte("Large"))
		case "/large/snapshots/" + string(snapshotID) + "/manifest.json":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(manifestBytes)
		case "/large/snapshots/" + string(snapshotID) + "/catalog.json":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(catalogBytes)
		case "/large/snapshots/" + string(snapshotID) + "/search-data/search/directory.json":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(searchBytes)
		default:
			http.NotFound(response, request)
		}
	})
	root := t.TempDir()
	writer := exportTreeWriter{root: root, entries: make(map[string]exportFileEntry)}
	writeMinimalExport(t, &writer, []byte("<!doctype html><html><body></body></html>"))
	receipt, _, err := captureCatalog(context.Background(), handler, &writer, active, "/", "", artifact.BuildProfile{}.Resolved(), 4)
	if err != nil {
		t.Fatalf("captureCatalog rejected catalog above runtime byte limit: %v", err)
	}
	manifest := exportManifest{SchemaVersion: 1, BasePath: "/", Catalogs: []ExportCatalogReceipt{receipt}, Files: writer.sortedEntries()}
	if err := verifyExportStructure(root, manifest, writer.entries); err != nil {
		t.Fatalf("verifyExportStructure rejected catalog above runtime byte limit: %v", err)
	}
}

func exportTestChild(pathValue, kind string, data []byte) catalog.ChildIdentityV1 {
	digest := sha256.Sum256(data)
	return catalog.ChildIdentityV1{Path: pathValue, Kind: kind, Length: uint64(len(data)), SHA256: hex.EncodeToString(digest[:])}
}

func TestExportWorkerChangesWithShellsAndRemainsDeterministic(t *testing.T) {
	exportWorker := func(shell string) []byte {
		t.Helper()
		assets := web.NewCatalogAssetsHandler()
		handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/" {
				response.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = response.Write([]byte(shell))
				return
			}
			assets.ServeHTTP(response, request)
		})
		output := filepath.Join(t.TempDir(), "public")
		if _, err := exportFromHandler(context.Background(), handler, nil, output, "/"); err != nil {
			t.Fatal(err)
		}
		worker, err := os.ReadFile(filepath.Join(output, "sw.js"))
		if err != nil {
			t.Fatal(err)
		}
		return worker
	}

	first := exportWorker(`<!doctype html><title>First</title>`)
	changed := exportWorker(`<!doctype html><title>Changed</title>`)
	repeated := exportWorker(`<!doctype html><title>First</title>`)
	if string(first) == string(changed) {
		t.Fatal("exported Service Worker did not change with cacheable HTML")
	}
	if string(first) != string(repeated) {
		t.Fatal("identical export shells produced different Service Worker bytes")
	}
}

func TestExportRejectsNonEmptyOutputWithoutMutation(t *testing.T) {
	output := t.TempDir()
	marker := filepath.Join(output, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareExportOutput(context.Background(), output); err == nil || !strings.Contains(err.Error(), "not an intact Manja export") {
		t.Fatalf("prepareExportOutput error = %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("marker changed: %q %v", data, err)
	}
}

func TestExportCaptureRejectsRedirect(t *testing.T) {
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/target", http.StatusTemporaryRedirect)
	})
	if _, err := captureHTTP(context.Background(), handler, "/source", 0, ""); err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("captureHTTP error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/source", nil))
	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("fixture status = %d", response.Code)
	}
}

func TestExportCaptureReportsNonOKBody(t *testing.T) {
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Error(response, "catalog temporarily unavailable: overview metrics limit", http.StatusServiceUnavailable)
	})
	if _, err := captureHTTP(context.Background(), handler, "/large/", 0, ""); err == nil ||
		!strings.Contains(err.Error(), "status 503") ||
		!strings.Contains(err.Error(), "catalog temporarily unavailable: overview metrics limit") {
		t.Fatalf("captureHTTP error = %v", err)
	}
}
