package static_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	web "github.com/araihu/manja/internal/web"
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
	Public                bool   `json:"public"`
	Anonymous             bool   `json:"anonymous"`
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
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`(manifest) => {
		window.__manifestRequests = [];
		window.__activation = null;
		window.ManjaLocalDocs = {activate: (descriptor, value) => {
			window.__activation = {catalogId: descriptor.catalogId, snapshotId: value.snapshotId, digest: value.identityDigest};
			return {ok: true, catalogId: descriptor.catalogId, publicationKey: descriptor.publicationKey, snapshotId: descriptor.snapshotId, revisionId: descriptor.revisionId, projectionDigest: descriptor.projectionDigest, children: value.children};
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

func TestLocalDocsEnhancerRegistersRootScopedWorkerWithoutBlockingSSR(t *testing.T) {
	page := localDocsPage(t)
	descriptor, manifestJSON := localDocsFixture(t, "/docs/")
	body := `<main id="ssr"><h1>Rendered on the server</h1></main>`
	if err := page.SetContent(body + `<script id="manja-local-docs-descriptor" type="application/json">` + descriptor + `</script>`); err != nil {
		t.Fatal(err)
	}
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`(manifest) => {
		window.__workerRegistration = null;
		window.__workerMessages = [];
		const worker = { postMessage: (message) => window.__workerMessages.push(message) };
		Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value: {
			register: (url, options) => { window.__workerRegistration = {url, options}; return Promise.resolve({active: worker}); },
			ready: Promise.resolve({active: worker}),
			addEventListener: () => {},
		} });
		window.ManjaLocalDocs = {activate: (value, input) => ({
			ok: true,
			catalogId: value.catalogId,
			publicationKey: value.publicationKey,
			snapshotId: value.snapshotId,
			revisionId: value.revisionId,
			projectionDigest: value.projectionDigest,
			children: input.children
		})};
		window.fetch = () => Promise.resolve(new Response(manifest, {status: 200, headers: {'Content-Length': String(new TextEncoder().encode(manifest).byteLength)}}));
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
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready'`, nil, playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatal(err)
	}
	after, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("worker registration changed SSR body: before=%q after=%q", before, after)
	}
	value, err := page.Evaluate(`() => ({registration: window.__workerRegistration, messages: window.__workerMessages, worker: document.documentElement.dataset.manjaLocalDocsWorker || ''})`)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("worker state = %#v", value)
	}
	registration, ok := state["registration"].(map[string]any)
	if !ok {
		t.Fatalf("worker registration = %#v", state["registration"])
	}
	workerURL, workerOK := registration["url"].(string)
	if !workerOK || !strings.HasSuffix(workerURL, "/manja-assets/local-docs/sw.js") {
		t.Fatalf("worker registration = %#v", state["registration"])
	}
	options, ok := registration["options"].(map[string]any)
	if !ok || options["scope"] != "/" {
		t.Fatalf("worker registration options = %#v", registration["options"])
	}
	if state["worker"] != "registered" {
		t.Fatalf("worker state = %#v, want registered", state["worker"])
	}
	if messages, ok := state["messages"].([]any); !ok || len(messages) != 1 || messages[0].(map[string]any)["type"] != "manja:configure" {
		t.Fatalf("worker messages = %#v", state["messages"])
	}
}

func TestLocalDocsEnhancerFallsBackBeforeActivationWhenWorkerRegistrationFails(t *testing.T) {
	page := localDocsPage(t)
	descriptor, manifestJSON := localDocsFixture(t, "/docs/")
	body := `<main id="ssr"><h1>Rendered on the server</h1></main>`
	if err := page.SetContent(body + `<script id="manja-local-docs-descriptor" type="application/json">` + descriptor + `</script>`); err != nil {
		t.Fatal(err)
	}
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`(manifest) => {
		window.__manifestRequests = 0;
		Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value: {
			register: () => Promise.reject(new Error('worker unavailable')),
		} });
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
	state, err := page.Evaluate(`() => ({body: document.body.innerHTML, noRequests: window.__manifestRequests === 0, worker: document.documentElement.dataset.manjaLocalDocsWorker || '', reason: document.documentElement.dataset.manjaLocalDocsWorkerReason || ''})`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := state.(map[string]any)
	if !ok || values["body"] != before || values["noRequests"] != true || values["worker"] != "fallback" || values["reason"] != "worker unavailable" {
		t.Fatalf("worker failure was not fail-closed before activation: %#v", state)
	}
}

func TestLocalDocsEnhancerRejectsActivationIdentityDriftWithoutTouchingSSR(t *testing.T) {
	page := localDocsPage(t)
	descriptor, manifestJSON := localDocsFixture(t, "/docs/")
	body := `<main id="ssr"><h1>Rendered on the server</h1><p>Keep this exact body.</p></main>`
	if err := page.SetContent(body + `<script id="manja-local-docs-descriptor" type="application/json">` + descriptor + `</script>`); err != nil {
		t.Fatal(err)
	}
	before, err := page.Locator("body").InnerHTML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Evaluate(`(manifest) => {
		window.ManjaLocalDocs = {activate: (descriptor, value) => ({
			ok: true,
			catalogId: descriptor.catalogId,
			publicationKey: descriptor.publicationKey,
			snapshotId: descriptor.snapshotId,
			revisionId: descriptor.revisionId,
			projectionDigest: 'f'.repeat(64),
			children: value.children
		})};
		window.fetch = () => Promise.resolve(new Response(manifest, {status: 200, headers: {'Content-Length': String(new TextEncoder().encode(manifest).byteLength)}}));
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
	state, err := page.Evaluate(`() => ({body: document.body.innerHTML, ready: document.documentElement.dataset.manjaLocalDocsReady || '', fallback: document.documentElement.dataset.manjaLocalDocsFallback || ''})`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := state.(map[string]any)
	if !ok || values["body"] != before || values["ready"] != "" || values["fallback"] != "true" {
		t.Fatalf("activation identity drift was not rejected fail-closed: %#v", state)
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

func TestLocalDocsEnhancerServesRealWasmAndPersistsBrowserState(t *testing.T) {
	descriptorJSON, manifestJSON := localDocsFixture(t, "/docs/")
	var descriptor localDocsDescriptor
	if err := json.Unmarshal([]byte(descriptorJSON), &descriptor); err != nil {
		t.Fatal(err)
	}
	manifestBytes := []byte(manifestJSON)
	assets := web.NewCatalogAssetsHandler()
	var requestMu sync.Mutex
	requests := make(map[string]int)
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestMu.Lock()
		requests[request.URL.Path]++
		requestMu.Unlock()

		switch request.URL.Path {
		case "/docs/":
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(response, `<main id="ssr"><h1>Rendered on the server</h1><p>Projection remains visible.</p></main><script id="manja-local-docs-descriptor" type="application/json">%s</script><script src="/manja-assets/local-docs.js"></script>`, descriptorJSON)
		case descriptor.ProjectionManifestURL:
			response.Header().Set("Content-Type", "application/json")
			response.Header().Set("Content-Length", fmt.Sprint(len(manifestBytes)))
			response.Header().Set("ETag", `"local-docs-browser-fixture"`)
			_, _ = response.Write(manifestBytes)
		case "/docs/_manja/offline-shell":
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
			response.Header().Set("Content-Security-Policy", "default-src 'self'")
			_, _ = io.WriteString(response, `<main id="offline-shell"><h1>Offline shell</h1></main>`)
		default:
			assets.ServeHTTP(response, request)
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	page := browserPage(t)
	if _, err := page.Goto(server.URL + "/docs/"); err != nil {
		t.Fatal(err)
	}
	waitOptions := playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(30_000)}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready' && document.documentElement.dataset.manjaLocalDocsWorker === 'ready'`, nil, waitOptions); err != nil {
		debug, _ := page.Evaluate(`() => ({state: document.documentElement.dataset.manjaLocalDocsState || '', reason: document.documentElement.dataset.manjaLocalDocsReason || '', worker: document.documentElement.dataset.manjaLocalDocsWorker || '', workerReason: document.documentElement.dataset.manjaLocalDocsWorkerReason || ''})`)
		requestMu.Lock()
		requestSnapshot := fmt.Sprintf("%#v", requests)
		requestMu.Unlock()
		t.Fatalf("real local-docs browser activation failed: %v; debug=%#v; requests=%s", err, debug, requestSnapshot)
	}

	receipt, err := page.Evaluate(`async () => {
		const requestValue = (request) => new Promise((resolve, reject) => {
			request.onsuccess = () => resolve(request.result)
			request.onerror = () => reject(request.error || new Error('IndexedDB request failed'))
		})
		const database = await new Promise((resolve, reject) => {
			const request = indexedDB.open('manja-local-docs')
			request.onsuccess = () => resolve(request.result)
			request.onerror = () => reject(request.error || new Error('IndexedDB open failed'))
		})
		const transaction = database.transaction(['publications', 'generations'], 'readonly')
		const metadataRequest = transaction.objectStore('publications').get('public-core')
		const generationsRequest = transaction.objectStore('generations').getAll()
		const [metadata, generations] = await Promise.all([requestValue(metadataRequest), requestValue(generationsRequest)])
		const cacheNames = await caches.keys()
		const staticCache = await caches.open('manja-local-docs-assets-v1')
		const wasm = await staticCache.match(new URL('/manja-assets/local-docs/manja.wasm', location.origin).href)
		let shell = false
		for (const name of cacheNames.filter((value) => value.startsWith('manja-local-docs-assets-v1::public-core::'))) {
			const cached = await (await caches.open(name)).match(new URL('/docs/_manja/offline-shell', location.origin).href)
			if (cached) shell = true
		}
		return {
			state: document.documentElement.dataset.manjaLocalDocsState || '',
			worker: document.documentElement.dataset.manjaLocalDocsWorker || '',
			wasmABI: Boolean(window.ManjaLocalDocs && typeof window.ManjaLocalDocs.activate === 'function'),
			wasmStatus: wasm ? wasm.status : 0,
			wasmType: wasm ? wasm.headers.get('Content-Type') || '' : '',
			cacheNames,
			shell,
			metadata: metadata ? {
				publicationKey: metadata.publicationKey,
				activeRevision: metadata.activeRevision,
				activeDigest: metadata.activeDigest,
				disabled: metadata.disabled === true,
			} : null,
			generations: generations.map((value) => ({
				publicationKey: value.publicationKey,
				revisionId: value.revisionId,
				projectionDigest: value.projectionDigest,
				projectionBytesLength: value.projectionBytes ? value.projectionBytes.byteLength : 0,
				manifestBytesLength: value.manifestBytes ? value.manifestBytes.byteLength : 0,
			})),
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := receipt.(map[string]any)
	if !ok {
		t.Fatalf("browser receipt = %#v", receipt)
	}
	wasmStatus, wasmStatusOK := browserInteger(values["wasmStatus"])
	if values["state"] != "ready" || values["worker"] != "ready" || values["wasmABI"] != true || !wasmStatusOK || wasmStatus != http.StatusOK || values["shell"] != true {
		t.Fatalf("browser receipt did not prove served runtime and shell cache: %#v", values)
	}
	metadata, ok := values["metadata"].(map[string]any)
	if !ok || metadata["publicationKey"] != descriptor.PublicationKey || metadata["activeRevision"] != descriptor.RevisionID || metadata["activeDigest"] != descriptor.ProjectionDigest || metadata["disabled"] != false {
		t.Fatalf("browser IndexedDB metadata = %#v", values["metadata"])
	}
	generations, ok := values["generations"].([]any)
	if !ok || len(generations) != 1 {
		t.Fatalf("browser IndexedDB generations = %#v", values["generations"])
	}
	generation, ok := generations[0].(map[string]any)
	projectionBytesLength, projectionBytesOK := browserInteger(generation["projectionBytesLength"])
	manifestBytesLength, manifestBytesOK := browserInteger(generation["manifestBytesLength"])
	if !ok || generation["publicationKey"] != descriptor.PublicationKey || generation["revisionId"] != descriptor.RevisionID || generation["projectionDigest"] != descriptor.ProjectionDigest || !projectionBytesOK || projectionBytesLength != len(manifestBytes) || !manifestBytesOK || manifestBytesLength != len(manifestBytes) {
		t.Fatalf("browser IndexedDB generation = %#v", generations[0])
	}

	requestMu.Lock()
	defer requestMu.Unlock()
	for _, path := range []string{
		"/manja-assets/local-docs.js",
		"/manja-assets/local-docs/sw.js",
		"/manja-assets/local-docs/storage.js",
		"/manja-assets/local-docs/wasm_exec.js",
		"/manja-assets/local-docs/manja.wasm",
		"/manja-assets/local-docs/manja.wasm.br",
		descriptor.ProjectionManifestURL,
		"/docs/_manja/offline-shell",
	} {
		if requests[path] == 0 {
			t.Errorf("real browser never requested served asset %s; requests=%#v", path, requests)
		}
	}
	ssr, err := page.Locator("#ssr").TextContent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ssr, "Rendered on the server") {
		t.Fatalf("browser activation replaced SSR body: %q", ssr)
	}
}

func TestLocalDocsBrowserStorageWithdrawalWinsOverDelayedActivation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless browser regression in short mode")
	}
	assets := web.NewCatalogAssetsHandler()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assets.ServeHTTP(response, request)
	}))
	t.Cleanup(server.Close)

	page := browserPage(t)
	if _, err := page.Goto(server.URL + "/"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"storage.js", "sw.js"} {
		if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{URL: playwright.String(server.URL + "/manja-assets/local-docs/" + path)}); err != nil {
			t.Fatalf("load local-docs %s: %v", path, err)
		}
	}
	value, err := page.Evaluate(`async () => {
		const digest = (letter) => letter.repeat(64)
		const descriptor = (revisionId, projectionDigest) => ({
			catalogId: 'browser-race',
			publicationKey: 'public-browser-race',
			publicationBase: '/browser-race/',
			snapshotId: 'snapshot-sha256-' + projectionDigest,
			revisionId,
			projectionFormat: 'projection-v2',
			projectionDigest,
			offlineShellUrl: '/browser-race/_manja/offline-shell',
		})
		const previous = descriptor('revision-previous', digest('1'))
		const next = descriptor('revision-next', digest('2'))
		const storage = ManjaLocalDocsStorage.createStorage(window)
		const generation = (value, body) => ({
			publicationKey: value.publicationKey,
			revisionId: value.revisionId,
			projectionDigest: value.projectionDigest,
			snapshotId: value.snapshotId,
			projectionBytes: new TextEncoder().encode(body),
			manifestBytes: new TextEncoder().encode('{}'),
		})
		await storage.commitGeneration(previous, generation(previous, 'previous'))
		await storage.activate(previous.publicationKey, previous.revisionId)
		await storage.putShell(previous.publicationKey, previous.offlineShellUrl, new Response('STALE SHELL'), previous)
		let activationStarted
		const activationStartedPromise = new Promise((resolve) => { activationStarted = resolve })
		let releaseActivation
		const activationRelease = new Promise((resolve) => { releaseActivation = resolve })
		const activation = ManjaLocalDocsWorker.commitCandidate({
			storage,
			descriptor: next,
			candidate: generation(next, 'next'),
			activate: async () => {
				activationStarted()
				await activationRelease
				throw new Error('delayed activation failed')
			},
			routingDisabled: () => false,
		})
		await activationStartedPromise
		await ManjaLocalDocsWorker.disablePublication(storage, next, 'HTTP 410', 'revoked')
		releaseActivation()
		let activationError = ''
		try { await activation } catch (error) { activationError = String(error && error.message || error) }
		const metadata = await storage.loadMetadata(next.publicationKey)
		const active = await storage.loadActive(next.publicationKey)
		const shell = await storage.getShell(next.publicationKey, next.offlineShellUrl, next)
		const cacheNames = await caches.keys()
		return {
			activationError,
			disabled: metadata && metadata.disabled === true,
			tombstone: metadata && metadata.tombstone && metadata.tombstone.state,
			active: Boolean(active),
			previousRevision: metadata && metadata.previousRevision,
			candidateRevision: metadata && metadata.candidateRevision,
			shell: Boolean(shell),
			generationCaches: cacheNames.filter((name) => name.indexOf('manja-local-docs-assets-v1::public-browser-race::') === 0),
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := value.(map[string]any)
	if !ok || values["activationError"] != "delayed activation failed" || values["disabled"] != true || values["tombstone"] != "revoked" || values["active"] != false || values["previousRevision"] != "" || values["candidateRevision"] != "" || values["shell"] != false {
		t.Fatalf("browser withdrawal race receipt = %#v", value)
	}
	if caches, ok := values["generationCaches"].([]any); !ok || len(caches) != 0 {
		t.Fatalf("browser withdrawal generation caches = %#v", values["generationCaches"])
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
		Public: true, Anonymous: true,
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

func browserInteger(value any) (int, bool) {
	switch value := value.(type) {
	case int:
		return value, true
	case float64:
		return int(value), value == float64(int(value))
	default:
		return 0, false
	}
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
	if _, err := page.Evaluate(`() => {
		Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value: undefined });
	}`); err != nil {
		t.Fatal(err)
	}
	return page
}
