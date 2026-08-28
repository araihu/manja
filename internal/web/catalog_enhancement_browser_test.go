package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/mxschmitt/playwright-go"
)

func TestCatalogEnhancementServedOfflineShellPersistsManifestAndTombstonesInBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless browser regression in short mode")
	}
	baseHandler, snapshot := catalogHandlerFixture(t, "/kubernetes")
	base := baseHandler.(*CatalogHandler)
	snapshot = catalogEnhancementSnapshot(t, snapshot)
	runtime := catalog.NewRuntime(1)
	if _, err := runtime.ActivateMount("/kubernetes", "", 1, snapshot); err != nil {
		t.Fatal(err)
	}
	production := NewCatalogHandlerWithOrganizationAndEnhancement(runtime, base.children, base.presentation, base.organization, eligibleCatalogEnhancementPolicyForMount(snapshot, "/kubernetes"))
	catalogHandler := production.(*CatalogHandler)
	assets := NewCatalogAssetsHandler()
	var requestMu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestMu.Lock()
		requests[request.URL.Path]++
		requestMu.Unlock()
		if strings.HasPrefix(request.URL.Path, "/manja-assets/") {
			assets.ServeHTTP(response, request)
			return
		}
		production.ServeHTTP(response, request)
	}))
	t.Cleanup(server.Close)

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pw.Stop() })
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = browser.Close() })
	context, err := browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = context.Close() })
	page, err := context.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server.URL + "/kubernetes/"); err != nil {
		t.Fatal(err)
	}
	wait := playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(30_000)}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready' && document.documentElement.dataset.manjaLocalDocsWorker === 'ready'`, nil, wait); err != nil {
		debug, _ := page.Evaluate(`() => ({state: document.documentElement.dataset.manjaLocalDocsState || '', reason: document.documentElement.dataset.manjaLocalDocsReason || '', worker: document.documentElement.dataset.manjaLocalDocsWorker || '', workerReason: document.documentElement.dataset.manjaLocalDocsWorkerReason || ''})`)
		t.Fatalf("real CatalogHandler browser activation failed: %v; debug=%#v", err, debug)
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
		const metadataRequest = transaction.objectStore('publications').get('public-kubernetes')
		const generationsRequest = transaction.objectStore('generations').getAll()
		const [metadata, generations] = await Promise.all([requestValue(metadataRequest), requestValue(generationsRequest)])
		const cacheNames = await caches.keys()
		let shell = false
		for (const name of cacheNames.filter((value) => value.startsWith('manja-local-docs-assets-v1::public-kubernetes::'))) {
			const cached = await (await caches.open(name)).match(new URL('/kubernetes/_manja/offline-shell', location.origin).href)
			if (cached) shell = true
		}
		database.close()
		return {
			state: document.documentElement.dataset.manjaLocalDocsState || '',
			worker: document.documentElement.dataset.manjaLocalDocsWorker || '',
			wasm: Boolean(window.ManjaLocalDocs && typeof window.ManjaLocalDocs.activate === 'function'),
			shell,
			metadata: metadata ? { publicationKey: metadata.publicationKey, public: metadata.public === true, anonymous: metadata.anonymous === true, activeRevision: metadata.activeRevision, disabled: metadata.disabled === true } : null,
			generations: generations.filter((value) => value.publicationKey === 'public-kubernetes').map((value) => ({ revisionId: value.revisionId, projectionDigest: value.projectionDigest, manifestBytes: value.manifestBytes ? value.manifestBytes.byteLength : 0 })),
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	receiptMap, ok := receipt.(map[string]any)
	if !ok || receiptMap["state"] != "ready" || receiptMap["worker"] != "ready" || receiptMap["wasm"] != true || receiptMap["shell"] != true {
		t.Fatalf("served browser receipt = %#v", receipt)
	}
	metadata, ok := receiptMap["metadata"].(map[string]any)
	if !ok || metadata["public"] != true || metadata["anonymous"] != true || metadata["disabled"] != false || metadata["activeRevision"] != snapshot.Manifest.Identity.RevisionID {
		t.Fatalf("served browser metadata receipt = %#v", receiptMap["metadata"])
	}
	generations, ok := receiptMap["generations"].([]any)
	if !ok || len(generations) != 1 {
		t.Fatalf("served browser generation receipt = %#v", receiptMap["generations"])
	}

	if err := context.SetOffline(true); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => document.documentElement.dataset.manjaLocalDocsState === 'ready' && document.documentElement.dataset.manjaLocalDocsWorker === 'ready'`, nil, wait); err != nil {
		debug, _ := page.Evaluate(`() => ({state: document.documentElement.dataset.manjaLocalDocsState || '', reason: document.documentElement.dataset.manjaLocalDocsReason || '', worker: document.documentElement.dataset.manjaLocalDocsWorker || '', workerReason: document.documentElement.dataset.manjaLocalDocsWorkerReason || ''})`)
		t.Fatalf("offline reload did not recover: %v; debug=%#v", err, debug)
	}
	offlineManifest, err := page.Evaluate(`async () => {
		const response = await fetch(document.getElementById('manja-local-docs-descriptor').textContent ? JSON.parse(document.getElementById('manja-local-docs-descriptor').textContent).projectionManifestUrl : '')
		return {status: response.status, body: await response.text()}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	offlineManifestMap, ok := offlineManifest.(map[string]any)
	if !ok || browserInt(offlineManifestMap["status"]) != http.StatusOK || !strings.Contains(fmt.Sprint(offlineManifestMap["body"]), `"snapshotId":"`+string(snapshot.ID)+`"`) {
		t.Fatalf("offline persisted manifest receipt = %#v", offlineManifest)
	}
	if err := context.SetOffline(false); err != nil {
		t.Fatal(err)
	}

	// A normal SSR response that no longer carries the public enhancement
	// descriptor must still revoke the worker's previously cached shell.
	catalogHandler.enhancement.Publications = nil
	withdrawnOnline, err := page.Evaluate(`async () => {
		const response = await fetch('/kubernetes/', {cache: 'no-store'})
		return {status: response.status, state: response.headers.get('X-Manja-Publication-State') || '', body: await response.text()}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	withdrawnOnlineMap, ok := withdrawnOnline.(map[string]any)
	if !ok || browserInt(withdrawnOnlineMap["status"]) != http.StatusOK || withdrawnOnlineMap["state"] != "deleted" || strings.Contains(fmt.Sprint(withdrawnOnlineMap["body"]), `id="manja-local-docs-descriptor"`) {
		t.Fatalf("normal SSR withdrawal receipt = %#v", withdrawnOnline)
	}
	if err := context.SetOffline(true); err != nil {
		t.Fatal(err)
	}
	_, reloadErr := page.Reload(playwright.PageReloadOptions{Timeout: playwright.Float(5_000), WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	if reloadErr == nil {
		withdrawnOffline, err := page.Evaluate(`() => ({body: document.documentElement.outerHTML})`)
		if err != nil {
			t.Fatal(err)
		}
		withdrawnOfflineMap, ok := withdrawnOffline.(map[string]any)
		if !ok || strings.Contains(fmt.Sprint(withdrawnOfflineMap["body"]), `data-manja-catalog-shell="true"`) {
			t.Fatalf("offline reload served stale shell after normal SSR withdrawal: reload=%v receipt=%#v", reloadErr, withdrawnOffline)
		}
	}
	if err := context.SetOffline(false); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server.URL+"/kubernetes/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded}); err != nil {
		t.Fatal(err)
	}

	// The browser is still exercising the real CatalogHandler. Changing its
	// composition-authoritative policy makes the same production route return
	// the revocation response that the worker must tombstone.
	catalogHandler.enhancement.Disabled = true
	revoked, err := page.Evaluate(`async () => {
		const response = await fetch('/kubernetes/_manja/offline-shell', {cache: 'no-store'})
		return {status: response.status, state: response.headers.get('X-Manja-Publication-State') || '', body: await response.text()}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	revokedMap, ok := revoked.(map[string]any)
	if !ok || browserInt(revokedMap["status"]) != http.StatusGone || revokedMap["state"] != "revoked" || strings.Contains(fmt.Sprint(revokedMap["body"]), `data-manja-catalog-shell="true"`) {
		t.Fatalf("served revocation receipt = %#v", revoked)
	}
	tombstone, err := page.Evaluate(`async () => {
		const requestValue = (request) => new Promise((resolve, reject) => { request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error || new Error('IndexedDB request failed')) })
		const database = await new Promise((resolve, reject) => { const request = indexedDB.open('manja-local-docs'); request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error || new Error('IndexedDB open failed')) })
		const transaction = database.transaction(['publications', 'generations'], 'readonly')
		const metadata = await requestValue(transaction.objectStore('publications').get('public-kubernetes'))
		const generations = await requestValue(transaction.objectStore('generations').getAll())
		const cacheNames = await caches.keys()
		let shell = false
		for (const name of cacheNames.filter((value) => value.startsWith('manja-local-docs-assets-v1::public-kubernetes::'))) {
			if (await (await caches.open(name)).match(new URL('/kubernetes/_manja/offline-shell', location.origin).href)) shell = true
		}
		database.close()
		return {disabled: metadata && metadata.disabled === true, tombstone: metadata && metadata.tombstone && metadata.tombstone.state, generations: generations.filter((value) => value.publicationKey === 'public-kubernetes').length, shell}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	tombstoneMap, ok := tombstone.(map[string]any)
	if !ok || tombstoneMap["disabled"] != true || tombstoneMap["tombstone"] != "revoked" || browserInt(tombstoneMap["generations"]) != 0 || tombstoneMap["shell"] != false {
		t.Fatalf("browser tombstone receipt = %#v", tombstone)
	}
	if err := context.SetOffline(true); err != nil {
		t.Fatal(err)
	}
	cachedAfterRevocation, err := page.Evaluate(`async () => {
		try {
			const response = await fetch('/kubernetes/_manja/offline-shell', {cache: 'no-store'})
			return {status: response.status, body: await response.text()}
		} catch (error) {
			return {error: String(error)}
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	cachedAfterRevocationMap, ok := cachedAfterRevocation.(map[string]any)
	if !ok || browserInt(cachedAfterRevocationMap["status"]) == http.StatusOK || strings.Contains(fmt.Sprint(cachedAfterRevocationMap["body"]), `data-manja-catalog-shell="true"`) {
		t.Fatalf("revoked offline shell still served cached bytes: %#v", cachedAfterRevocation)
	}
	if err := context.SetOffline(false); err != nil {
		t.Fatal(err)
	}

	if _, err := page.AddScriptTag(playwright.PageAddScriptTagOptions{URL: playwright.String(server.URL + "/manja-assets/local-docs/storage.js")}); err != nil {
		t.Fatal(err)
	}
	recovery, err := page.Evaluate(`async () => {
		const api = window.ManjaLocalDocsStorage
		const storage = api.createBrowserStorage(window)
		const digest = (letter) => letter.repeat(64)
		const value = (index) => {
			const revisionId = 'browser-revision-' + index
			const projectionDigest = digest(String(index))
			const snapshotId = 'snapshot-sha256-' + projectionDigest
			return {schemaVersion: 1, catalogId: 'browser-recovery', publicationKey: 'browser-recovery', public: true, anonymous: true, publicationBase: '/browser-recovery/', snapshotId, revisionId, projectionFormat: 'projection-v2', projectionDigest, projectionManifestUrl: '/browser-recovery/snapshots/' + snapshotId + '/manifest.json', catalogUrl: '/browser-recovery/snapshots/' + snapshotId + '/catalog.json', searchDataBase: '/browser-recovery/snapshots/' + snapshotId + '/search-data/', projectionDataBase: '/browser-recovery/snapshots/' + snapshotId + '/projection-data/', offlineShellUrl: '/browser-recovery/_manja/offline-shell'}
		}
		for (let index = 1; index <= 3; index++) {
			const current = value(index)
			await storage.commitGeneration(current, {publicationKey: current.publicationKey, revisionId: current.revisionId, projectionDigest: current.projectionDigest, snapshotId: current.snapshotId, projectionBytes: new Uint8Array([index]), manifestBytes: new Uint8Array([123, 125])})
			await storage.activate(current.publicationKey, current.revisionId)
		}
		const database = await new Promise((resolve, reject) => { const request = indexedDB.open('manja-local-docs'); request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error || new Error('IndexedDB open failed')) })
		const requestValue = (request) => new Promise((resolve, reject) => { request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error || new Error('IndexedDB request failed')) })
		const before = await requestValue(database.transaction('generations', 'readonly').objectStore('generations').getAll())
		database.onversionchange = () => setTimeout(() => database.close(), 50)
		const pointer = value(3)
		const recreated = await storage.recreateFromPointer(pointer.publicationKey, {publicationKey: pointer.publicationKey, revisionId: 'recreated-revision', projectionDigest: digest('f'), snapshotId: 'snapshot-sha256-' + digest('f'), projectionBytes: new Uint8Array([9]), manifestBytes: new Uint8Array([123, 125])})
		const active = await storage.loadCandidate(pointer.publicationKey)
		return {before: before.filter((item) => item.publicationKey === pointer.publicationKey).length, recreatedRevision: recreated.revisionId, candidateRevision: active && active.revisionId}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	recoveryMap, ok := recovery.(map[string]any)
	if !ok || browserInt(recoveryMap["before"]) != 2 || recoveryMap["recreatedRevision"] != "recreated-revision" || recoveryMap["candidateRevision"] != "recreated-revision" {
		t.Fatalf("browser IDB generation/recovery receipt = %#v", recovery)
	}

	requestMu.Lock()
	defer requestMu.Unlock()
	for _, path := range []string{
		"/manja-assets/local-docs.js",
		"/manja-assets/local-docs/sw.js",
		"/manja-assets/local-docs/storage.js",
		"/manja-assets/local-docs/wasm_exec.js",
		"/manja-assets/local-docs/manja.wasm",
		"/kubernetes/_manja/offline-shell",
		"/kubernetes/snapshots/" + string(snapshot.ID) + "/manifest.json",
	} {
		if requests[path] == 0 {
			t.Errorf("real browser never requested production path %s; requests=%#v", path, requests)
		}
	}
}

func browserInt(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return -1
	}
}
