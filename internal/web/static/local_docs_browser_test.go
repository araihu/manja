package static_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

type localDocsIdentity struct {
	SchemaVersion    uint32 `json:"schemaVersion"`
	CatalogID        string `json:"catalogId"`
	RevisionID       string `json:"revisionId"`
	ProjectionFormat string `json:"projectionFormat"`
}

type localDocsDescriptor struct {
	SchemaVersion         uint32 `json:"schemaVersion"`
	CatalogID             string `json:"catalogId"`
	PublicationKey        string `json:"publicationKey"`
	PublicationBase       string `json:"publicationBase"`
	SnapshotID            string `json:"snapshotId"`
	RevisionID            string `json:"revisionId"`
	ProjectionFormat      string `json:"projectionFormat"`
	ProjectionDigest      string `json:"projectionDigest"`
	ProjectionManifestURL string `json:"projectionManifestUrl"`
	CatalogURL            string `json:"catalogUrl"`
	SearchDataBase        string `json:"searchDataBase"`
	ProjectionDataBase    string `json:"projectionDataBase"`
}

func TestLocalDocsEnhancerActivatesAfterSameOriginManifestAndLeavesSSRBodyIntact(t *testing.T) {
	page := localDocsPage(t)
	descriptor, manifestJSON := localDocsFixture(t, "/docs/")
	body := `<main id="ssr"><h1>Rendered on the server</h1><p>Projection remains visible.</p></main>`
	if err := page.SetContent(body + `<script id="manja-local-docs-descriptor" type="application/json">` + descriptor + `</script>`); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`(manifest) => {
		window.__manifestRequests = [];
		window.__activation = null;
		window.ManjaLocalDocs = {activate: (descriptor, value) => {
			window.__activation = {catalogId: descriptor.catalogId, snapshotId: value.snapshotId, digest: value.identityDigest};
			return {ok: true, children: value.children};
		}};
		window.fetch = (url) => {
			window.__manifestRequests.push(url);
			return Promise.resolve(new Response(manifest, {status: 200, headers: {'Content-Length': String(new TextEncoder().encode(manifest).byteLength)}}));
		};
	}`, manifestJSON); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("local-docs.js")
	if err != nil {
		t.Fatal(err)
	}
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		debug, _ := page.Evaluate(`() => ({state: document.documentElement.dataset.manjaLocalDocsState || '', reason: document.documentElement.dataset.manjaLocalDocsReason || '', requests: window.__manifestRequests, activation: window.__activation})`)
		t.Fatalf("%v; debug=%#v", err, debug)
	}
	after, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("SSR body changed during activation: before=%q after=%q", before, after)
	}
	state, err := page.Evaluate(`() => ({requests: window.__manifestRequests, activation: window.__activation, ready: document.documentElement.dataset.manjaLocalDocsReady || '', fallback: document.documentElement.dataset.manjaLocalDocsFallback || ''})`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := state.(map[string]any)
	if !ok {
		t.Fatalf("activation state = %#v", state)
	}
	requests, ok := values["requests"].([]any)
	var requestURL string
	requestOK := false
	if ok && len(requests) > 0 {
		requestURL, requestOK = requests[0].(string)
	}
	wantSuffix := "/docs/snapshots/" + descriptorSnapshot(descriptor) + "/manifest.json"
	if !ok || !requestOK || len(requests) != 1 || !strings.HasSuffix(requestURL, wantSuffix) {
		t.Fatalf("manifest requests = %#v", values["requests"])
	}
	if values["ready"] != "true" || values["fallback"] != "" {
		t.Fatalf("activation unexpectedly fell back: %#v", values)
	}
}

func TestLocalDocsEnhancerFallsBackWithoutPartialActivationWhenWasmAssetFails(t *testing.T) {
	page := localDocsPage(t)
	descriptor, _ := localDocsFixture(t, "/docs/")
	body := `<main id="ssr"><h1>Server projection</h1><p>Keep this exact body.</p></main>`
	if err := page.SetContent(body + `<script id="manja-local-docs-descriptor" type="application/json">` + descriptor + `</script>`); err != nil {
		t.Fatal(err)
	}
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("local-docs.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'fallback'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	state, err := page.Evaluate(`() => ({body: document.body.innerHTML, ready: document.documentElement.dataset.manjaLocalDocsReady || '', fallback: document.documentElement.dataset.manjaLocalDocsState || ''})`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := state.(map[string]any)
	if !ok || values["body"] != before || values["ready"] != "" || values["fallback"] != "fallback" {
		t.Fatalf("Wasm failure must preserve SSR and avoid ready state: %#v", state)
	}
}

func TestLocalDocsEnhancerRejectsCrossOriginDescriptorBeforeFetching(t *testing.T) {
	page := localDocsPage(t)
	descriptor, manifestJSON := localDocsFixture(t, "/docs/")
	descriptor = strings.Replace(descriptor, `"/docs/snapshots/`, `"https://attacker.example/snapshots/`, 1)
	if err := page.SetContent(`<main id="ssr"><h1>Server projection</h1></main><script id="manja-local-docs-descriptor" type="application/json">` + descriptor + `</script>`); err != nil {
		t.Fatal(err)
	}
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`(manifest) => {
		window.__manifestRequests = 0;
		window.ManjaLocalDocs = {activate: () => ({ok: true})};
		window.fetch = () => { window.__manifestRequests++; return Promise.resolve(new Response(manifest)); };
	}`, manifestJSON); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("local-docs.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{Path: playwright.String(path)}); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'fallback'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	state, err := page.Evaluate(`() => ({requests: window.__manifestRequests, requestsZero: window.__manifestRequests === 0, state: document.documentElement.dataset.manjaLocalDocsState || '', body: document.body.innerHTML})`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := state.(map[string]any)
	if !ok || values["requestsZero"] != true || values["state"] != "fallback" || values["body"] != before {
		t.Fatalf("cross-origin descriptor was not rejected fail-closed: %#v", state)
	}
}

func localDocsFixture(t *testing.T, publicationBase string) (string, string) {
	t.Helper()
	identity := localDocsIdentity{SchemaVersion: 1, CatalogID: "core", RevisionID: "revision-immutable-1", ProjectionFormat: "projection-v2"}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityJSON)
	digestHex := hex.EncodeToString(digest[:])
	snapshot := "snapshot-sha256-" + digestHex
	descriptorValue := localDocsDescriptor{
		SchemaVersion: 1, CatalogID: "core", PublicationKey: "public-core", PublicationBase: publicationBase,
		SnapshotID: snapshot, RevisionID: identity.RevisionID, ProjectionFormat: identity.ProjectionFormat, ProjectionDigest: digestHex,
		ProjectionManifestURL: publicationBase + "snapshots/" + snapshot + "/manifest.json", CatalogURL: publicationBase + "snapshots/" + snapshot + "/catalog.json",
		SearchDataBase: publicationBase + "snapshots/" + snapshot + "/search-data/", ProjectionDataBase: publicationBase + "snapshots/" + snapshot + "/projection-data/",
	}
	descriptorJSON, err := json.Marshal(descriptorValue)
	if err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"schemaVersion":1,"snapshotId":%q,"identity":%s,"children":[{"path":"details/core.json","kind":"detail","length":7,"sha256":"%s"},{"path":"schema-nodes/core.json","kind":"schema-node","length":8,"sha256":"%s"},{"path":"catalog.json","kind":"catalog","length":9,"sha256":"%s"}]}`, snapshot, identityJSON, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64))
	return string(descriptorJSON), manifest
}

func descriptorSnapshot(descriptorJSON string) string {
	var descriptor localDocsDescriptor
	if err := json.Unmarshal([]byte(descriptorJSON), &descriptor); err != nil {
		return ""
	}
	return descriptor.SnapshotID
}

func localDocsPage(t *testing.T) playwright.Page {
	t.Helper()
	page := browserPage(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	if _, err := page.Goto(server.URL); err != nil {
		t.Fatal(err)
	}
	return page
}
