import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import test from 'node:test'

import worker from './sw.js'
import storageModule from './storage.js'

const digest = (value) => createHash('sha256').update(value).digest('hex')
const bytes = (value) => new TextEncoder().encode(value)
const descriptor = (overrides = {}) => {
  const projectionDigest = 'a'.repeat(64)
  const snapshotId = `snapshot-sha256-${projectionDigest}`
  return {
    schemaVersion: 1,
    catalogId: 'public-api',
    publicationKey: 'public-api-v1',
    public: true,
    anonymous: true,
    publicationBase: '/docs/',
    snapshotId,
    revisionId: 'revision-1',
    projectionFormat: 'projection-v2',
    projectionDigest,
    projectionManifestUrl: `/docs/snapshots/${snapshotId}/manifest.json`,
    catalogUrl: `/docs/snapshots/${snapshotId}/catalog.json`,
    searchDataBase: `/docs/snapshots/${snapshotId}/search-data/`,
    projectionDataBase: `/docs/snapshots/${snapshotId}/projection-data/`,
    offlineShellUrl: '/docs/_manja/offline-shell',
    ...overrides,
  }
}

test('worker validates public eligibility and intercepts only same-origin reader GETs', () => {
  const value = descriptor()
  const origin = 'https://docs.test'
  assert.equal(worker.validateDescriptor(value, origin).publicationKey, 'public-api-v1')
  assert.throws(() => worker.validateDescriptor({ ...value, public: false }, origin), /public eligibility/)
  assert.throws(() => worker.validateDescriptor({ ...value, anonymous: false }, origin), /public eligibility/)
  assert.throws(() => worker.validateDescriptor({ ...value, public: undefined }, origin), /public eligibility/)
  assert.throws(() => worker.validateDescriptor({ ...value, anonymous: undefined }, origin), /public eligibility/)
  assert.throws(() => worker.validateDescriptor({ ...value, projectionManifestUrl: 'https://attacker.test/manifest.json' }, origin), /same-origin/)
  assert.throws(() => worker.validateDescriptor({ ...value, fallbackAssets: [{ url: '/manage/session.js' }] }, origin), /reserved/)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/docs/snapshots/${value.snapshotId}/manifest.json` }, value, origin), true)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/docs/documents/api/?selected=operation-list#operation-list` }, value, origin), true)
  assert.equal(worker.isAllowedRequest({ method: 'POST', url: `${origin}/docs/documents/api/` }, value, origin), false)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/manage/` }, value, origin), false)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/api/private` }, value, origin), false)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: 'https://other.test/docs/documents/api/' }, value, origin), false)
  assert.equal(worker.isStaticAssetPath('/manja-assets/local-docs.js'), true)
  assert.equal(worker.isStaticAssetPath('/manja-assets/local-docs/sw.js'), true)
})

test('worker runtime assets use an exact same-origin allowlist', () => {
  assert.deepEqual(worker.DEFAULT_STATIC_ASSETS, [
    '/manja-assets/local-docs/sw.js',
    '/manja-assets/local-docs/storage.js',
    '/manja-assets/local-docs.js',
    '/manja-assets/local-docs/wasm_exec.js',
    '/manja-assets/local-docs/manja.wasm',
    '/manja-assets/local-docs/manja.wasm.br',
  ])
  for (const path of worker.DEFAULT_STATIC_ASSETS) assert.equal(worker.isStaticAssetPath(path), true)
  assert.equal(worker.isStaticAssetPath('/manja-assets/local-docs/_worker.test.mjs'), false)
})

test('default runtime asset manifest binds every embedded JS and Wasm byte', () => {
  const files = {
    '/manja-assets/local-docs/sw.js': new URL('./sw.js', import.meta.url),
    '/manja-assets/local-docs/storage.js': new URL('./storage.js', import.meta.url),
    '/manja-assets/local-docs.js': new URL('../local-docs.js', import.meta.url),
    '/manja-assets/local-docs/wasm_exec.js': new URL('./wasm_exec.js', import.meta.url),
    '/manja-assets/local-docs/manja.wasm': new URL('./manja.wasm', import.meta.url),
    '/manja-assets/local-docs/manja.wasm.br': new URL('./manja.wasm.br', import.meta.url),
  }
  for (const path of worker.DEFAULT_STATIC_ASSETS) {
    const expected = worker.DEFAULT_STATIC_ASSET_EXPECTATIONS[path]
    const body = fs.readFileSync(files[path])
    assert.equal(expected.length, body.byteLength, path)
    assert.equal(expected.sha256, digest(body), path)
  }
})

test('static runtime assets refresh validated bytes and retain the newest offline fallback', async () => {
  const entries = new Map()
  const cache = {
    async match(request) {
      const response = entries.get(String(request))
      return response ? response.clone() : undefined
    },
    async put(request, response) {
      entries.set(String(request), response.clone())
    },
    async delete(request) {
      return entries.delete(String(request))
    },
  }
  const scope = { caches: { open: async () => cache } }
  for (const path of ['/manja-assets/local-docs.js', '/manja-assets/local-docs/manja.wasm']) {
    let current = 'runtime-v1'
    let online = true
    const fetchImplementation = async () => {
      if (!online) throw new Error('offline')
      return new Response(bytes(current), { status: 200 })
    }
    const expected = () => ({ length: bytes(current).byteLength, sha256: digest(bytes(current)) })

    const first = await worker.cachedStaticAsset(scope, path, 'manja-local-docs-assets-v1', fetchImplementation, expected())
    assert.equal(await first.text(), 'runtime-v1', path)

    current = 'runtime-v2'
    const second = await worker.cachedStaticAsset(scope, path, 'manja-local-docs-assets-v1', fetchImplementation, expected())
    assert.equal(await second.text(), 'runtime-v2', path)

    online = false
    const offline = await worker.cachedStaticAsset(scope, path, 'manja-local-docs-assets-v1', fetchImplementation, expected())
    assert.equal(await offline.text(), 'runtime-v2', path)
  }
})

test('default runtime asset caching rejects an invalid wasm response before offline fallback', async () => {
  const entries = new Map()
  const cache = {
    async match(request) {
      const response = entries.get(String(request))
      return response ? response.clone() : undefined
    },
    async put(request, response) {
      entries.set(String(request), response.clone())
    },
    async delete(request) {
      return entries.delete(String(request))
    },
  }
  const scope = { caches: { open: async () => cache } }
  const files = {
    '/manja-assets/local-docs/sw.js': new URL('./sw.js', import.meta.url),
    '/manja-assets/local-docs/storage.js': new URL('./storage.js', import.meta.url),
    '/manja-assets/local-docs.js': new URL('../local-docs.js', import.meta.url),
    '/manja-assets/local-docs/wasm_exec.js': new URL('./wasm_exec.js', import.meta.url),
    '/manja-assets/local-docs/manja.wasm': new URL('./manja.wasm', import.meta.url),
    '/manja-assets/local-docs/manja.wasm.br': new URL('./manja.wasm.br', import.meta.url),
  }
  let online = true
  const fetchImplementation = async (request) => {
    if (!online) throw new Error('offline')
    const pathname = new URL(String(request), 'https://docs.test').pathname
    const body = pathname.endsWith('/manja.wasm') ? bytes('not-a-wasm-binary') : fs.readFileSync(files[pathname])
    return new Response(body, { status: 200 })
  }

  const ready = await worker.cacheStaticAssets(scope, 'manja-local-docs-assets-v1', fetchImplementation, worker.DEFAULT_STATIC_ASSETS)
  assert.equal(ready, false)
  assert.equal(entries.has('/manja-assets/local-docs/manja.wasm'), false)

  online = false
  await assert.rejects(() => worker.cachedStaticAsset(scope, '/manja-assets/local-docs/manja.wasm', 'manja-local-docs-assets-v1', fetchImplementation), /offline/)
})

test('default runtime asset caching rejects same-length invalid production bytes before cache replacement', async () => {
  const entries = new Map()
  const cache = {
    async match(request) {
      const response = entries.get(String(request))
      return response ? response.clone() : undefined
    },
    async put(request, response) {
      entries.set(String(request), response.clone())
    },
    async delete(request) {
      return entries.delete(String(request))
    },
  }
  const scope = { caches: { open: async () => cache } }
  const path = '/manja-assets/local-docs/sw.js'
  const original = fs.readFileSync(new URL('./sw.js', import.meta.url))
  const invalid = new Uint8Array(original)
  invalid[invalid.length - 1] ^= 1
  assert.equal(invalid.byteLength, original.byteLength)
  let online = true
  const fetchImplementation = async () => {
    if (!online) throw new Error('offline')
    return new Response(invalid, { status: 200 })
  }

  await assert.rejects(
    () => worker.cachedStaticAsset(scope, path, 'manja-local-docs-assets-v1', fetchImplementation),
    /fallback asset digest differs/,
  )
  assert.equal(entries.has(path), false)

  online = false
  await assert.rejects(() => worker.cachedStaticAsset(scope, path, 'manja-local-docs-assets-v1', fetchImplementation), /offline/)
})

test('worker rejects duplicate manifest keys and binds child bytes to declared digest', async () => {
  const identity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const projectionDigest = digest(JSON.stringify(identity))
  const value = descriptor({
    projectionDigest,
    snapshotId: `snapshot-sha256-${projectionDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/projection-data/`,
  })
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: value.snapshotId, identity, children: [] })
  const parsed = await worker.parseManifest(bytes(manifest), value)
  assert.equal(parsed.identityDigest, projectionDigest)
  await assert.rejects(() => worker.parseManifest(bytes('{"schemaVersion":1,"schemaVersion":1}'), value), /duplicate keys/)

  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(value, { ...descriptor(value), projectionBytes: bytes('projection'), manifestBytes: bytes(manifest) })
  await storage.activate(value.publicationKey, value.revisionId)
  const request = new Request(`https://docs.test${value.projectionDataBase}details/core.json`)
  const body = bytes('child')
  const childManifest = JSON.stringify({ ...JSON.parse(manifest), children: [{ path: 'details/core.json', kind: 'detail', length: body.byteLength, sha256: digest(body) }] })
  await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: body, manifestBytes: bytes(childManifest) })
  await storage.activate(value.publicationKey, value.revisionId)
  const response = new Response(bytes('xxxxx'), { status: 200 })
  await assert.rejects(() => worker.validateManifestChild(storage, value, request, response), /manifest child digest differs|manifest children differ/)
})

test('revalidation is single-flight and network failure serves validated offline shell', async () => {
  const storage = storageModule.createMemoryStorage()
  const identity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const projectionDigest = digest(JSON.stringify(identity))
  const value = descriptor({
    projectionDigest,
    snapshotId: `snapshot-sha256-${projectionDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/projection-data/`,
  })
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: value.snapshotId, identity, children: [] })
  let calls = 0
  const revalidate = worker.createRevalidator(() => worker.revalidate({
    storage,
    descriptor: value,
    fetch: async () => { calls += 1; await new Promise((resolve) => setTimeout(resolve, 5)); return new Response(manifest, { status: 200, headers: { ETag: '"v1"' } }) },
    origin: 'https://docs.test',
  }))
  const [first, second] = await Promise.all([revalidate(), revalidate()])
  assert.equal(calls, 1)
  assert.equal(first.kind, 'ready')
  assert.equal(second.kind, 'ready')

  await storage.putShell(value.publicationKey, value.offlineShellUrl, new Response('<main>offline</main>', { status: 200, headers: { 'Content-Security-Policy': "default-src 'self'" } }))
  const request = { method: 'GET', mode: 'navigate', url: 'https://docs.test/docs/documents/api/', headers: new Headers() }
  const response = await worker.cachedOrFetched({}, storage, value, request, async () => { throw new Error('offline') })
  assert.equal(await response.text(), '<main>offline</main>')
})

test('offline manifest fetch recovers the persisted manifest bytes before falling back', async () => {
  const value = descriptor()
  const identity = { schemaVersion: 1, catalogId: value.catalogId, revisionId: value.revisionId, projectionFormat: value.projectionFormat }
  const projectionDigest = digest(JSON.stringify(identity))
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: `snapshot-sha256-${projectionDigest}`, identity, children: [] })
  const persisted = { ...value, projectionDigest, snapshotId: `snapshot-sha256-${projectionDigest}`, projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/manifest.json`, catalogUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/catalog.json`, searchDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/search-data/`, projectionDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/projection-data/` }
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(persisted, { ...generationFor(persisted), projectionBytes: bytes('projection'), manifestBytes: bytes(manifest) })
  await storage.activate(persisted.publicationKey, persisted.revisionId)
  const request = { method: 'GET', mode: 'cors', credentials: 'same-origin', url: `https://docs.test${persisted.projectionManifestUrl}`, headers: new Headers() }
  const response = await worker.cachedOrFetched({ crypto }, storage, persisted, request, async () => { throw new Error('offline') })
  assert.equal(response.status, 200)
  assert.equal(await response.text(), manifest)
})

test('withdrawn offline shell tombstones before returning and blocks cached bytes', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
  await storage.activate(value.publicationKey, value.revisionId)
  await storage.putShell(value.publicationKey, value.offlineShellUrl, new Response('<main>canonical</main>'), value)
  const request = { method: 'GET', mode: 'navigate', credentials: 'omit', url: `https://docs.test${value.offlineShellUrl}`, headers: new Headers() }
  const withdrawn = await worker.cachedOrFetched({}, storage, value, request, async () => new Response('gone', { status: 410 }))
  assert.equal(withdrawn.status, 410)
  assert.equal((await storage.loadMetadata(value.publicationKey)).disabled, true)
  assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl, value), undefined)
  await assert.rejects(() => worker.cachedOrFetched({}, storage, value, request, async () => { throw new Error('offline') }), /offline/)
})

test('offline shell 404 without a state header tombstones before fallback', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
  await storage.activate(value.publicationKey, value.revisionId)
  await storage.putShell(value.publicationKey, value.offlineShellUrl, new Response('<main>stale</main>'), value)
  const request = { method: 'GET', mode: 'navigate', credentials: 'omit', url: `https://docs.test${value.offlineShellUrl}`, headers: new Headers() }

  const absent = await worker.cachedOrFetched({}, storage, value, request, async () => new Response('not found', { status: 404 }))
  assert.equal(absent.status, 404)
  const state = await storage.loadMetadata(value.publicationKey)
  assert.equal(state.disabled, true)
  assert.equal(state.tombstone.state, 'deleted')
  assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl, value), undefined)
  assert.equal(storage.snapshot().generations.length, 0)
  await assert.rejects(() => worker.cachedOrFetched({}, storage, value, request, async () => { throw new Error('offline') }), /offline/)
})

test('private and non-anonymous shell responses tombstone before cached fallback', async () => {
  for (const [name, response, expectedState] of [
    ['private', new Response('private', { status: 404, headers: { 'X-Manja-Publication-State': 'private' } }), 'private'],
    ['non-anonymous', new Response('restricted', { status: 200, headers: { 'X-Manja-Publication-State': 'private' } }), 'private'],
  ]) {
    const value = descriptor()
    const storage = storageModule.createMemoryStorage()
    await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
    await storage.activate(value.publicationKey, value.revisionId)
    await storage.putShell(value.publicationKey, value.offlineShellUrl, new Response('<main>stale</main>'), value)
    const cached = await worker.cacheOfflineShell(storage, value, async () => response)
    assert.equal(cached, false, name)
    const state = await storage.loadMetadata(value.publicationKey)
    assert.equal(state.disabled, true, name)
    assert.equal(state.tombstone.state, expectedState, name)
    assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl, value), undefined, name)
    assert.equal(storage.snapshot().generations.length, 0, name)
  }
})

test('normal SSR withdrawal tombstones before offline reload fallback', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
  await storage.activate(value.publicationKey, value.revisionId)
  await storage.putShell(value.publicationKey, value.offlineShellUrl, new Response('STALE OFFLINE SHELL'), value)
  const request = { method: 'GET', mode: 'navigate', credentials: 'omit', url: 'https://docs.test/docs/', headers: new Headers({ Accept: 'text/html' }) }

  const online = await worker.cachedOrFetched({}, storage, value, request, async () => new Response('<main>SSR withdrawal</main>', {
    status: 200,
    headers: { 'X-Manja-Publication-State': 'private' },
  }))
  assert.equal(await online.text(), '<main>SSR withdrawal</main>')
  const metadata = await storage.loadMetadata(value.publicationKey)
  assert.equal(metadata.disabled, true)
  assert.equal(metadata.tombstone.state, 'private')
  assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl, value), undefined)

  await assert.rejects(() => worker.cachedOrFetched({}, storage, value, request, async () => { throw new Error('offline') }), /offline/)
})

test('withdrawal disables in-memory routing before delayed tombstone so concurrent navigation cannot serve stale shell', async () => {
  const value = descriptor()
  const baseStorage = storageModule.createMemoryStorage()
  await baseStorage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
  await baseStorage.activate(value.publicationKey, value.revisionId)
  await baseStorage.putShell(value.publicationKey, value.offlineShellUrl, new Response('STALE SHELL'), value)

  let tombstoneStarted
  const tombstoneStartedPromise = new Promise((resolve) => { tombstoneStarted = resolve })
  let releaseTombstone
  const tombstoneRelease = new Promise((resolve) => { releaseTombstone = resolve })
  const storage = {
    ...baseStorage,
    async tombstone(...args) {
      tombstoneStarted()
      await tombstoneRelease
      return baseStorage.tombstone(...args)
    },
  }
  const listeners = {}
  const scope = {
    location: { origin: 'https://docs.test' },
    addEventListener: (type, listener) => { listeners[type] = listener },
    clients: { matchAll: async () => [] },
  }
  let networkCalls = 0
  const runtime = worker.register(scope, {
    storage,
    fetch: async (request) => {
      networkCalls++
      if (networkCalls === 1 && String(request.url || request).endsWith('/documents/api/')) {
        return new Response('<main>withdrawn</main>', { status: 200, headers: { 'X-Manja-Publication-State': 'private' } })
      }
      throw new Error('offline')
    },
  })
  await runtime.configure(value)

  const withdrawnRequest = { method: 'GET', mode: 'navigate', credentials: 'omit', url: 'https://docs.test/docs/documents/api/', headers: new Headers() }
  const withdrawnEvent = { request: withdrawnRequest, respondWith: (promise) => { withdrawnEvent.pending = promise } }
  listeners.fetch(withdrawnEvent)
  await tombstoneStartedPromise

  const concurrentRequest = { method: 'GET', mode: 'navigate', credentials: 'omit', url: 'https://docs.test/docs/documents/api/', headers: new Headers() }
  const concurrentEvent = { request: concurrentRequest, respondWith: (promise) => { concurrentEvent.pending = promise } }
  listeners.fetch(concurrentEvent)
  await assert.rejects(() => concurrentEvent.pending, /offline/)

  releaseTombstone()
  const withdrawn = await withdrawnEvent.pending
  assert.equal(withdrawn.status, 200)
  assert.equal(runtime.disabled.has(value.publicationKey), true)
})

test('withdrawal wins over a delayed activation rollback and leaves no enabled metadata or shell', async () => {
  const previous = descriptor({ revisionId: 'revision-previous' })
  const next = descriptor({ revisionId: 'revision-next' })
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(previous, { ...generationFor(previous), projectionBytes: bytes('previous'), manifestBytes: bytes('{}') })
  await storage.activate(previous.publicationKey, previous.revisionId)
  await storage.putShell(previous.publicationKey, previous.offlineShellUrl, new Response('STALE SHELL'), previous)

  let activationStarted
  const activationStartedPromise = new Promise((resolve) => { activationStarted = resolve })
  let releaseActivation
  const activationRelease = new Promise((resolve) => { releaseActivation = resolve })
  const activation = worker.commitCandidate({
    storage,
    descriptor: next,
    candidate: { ...generationFor(next), projectionBytes: bytes('next'), manifestBytes: bytes('{}') },
    activate: async () => {
      activationStarted()
      await activationRelease
      throw new Error('delayed activation failed')
    },
    routingDisabled: () => false,
  })
  await activationStartedPromise

  await worker.disablePublication(storage, next, 'HTTP 410', 'revoked')
  releaseActivation()
  await assert.rejects(() => activation, /delayed activation failed/)

  const state = await storage.loadMetadata(next.publicationKey)
  assert.equal(state.disabled, true)
  assert.equal(state.tombstone.state, 'revoked')
  assert.equal(state.activeRevision, '')
  assert.equal(state.previousRevision, '')
  assert.equal(state.candidateRevision, '')
  assert.equal(await storage.loadActive(next.publicationKey), undefined)
  assert.equal(await storage.getShell(next.publicationKey, next.offlineShellUrl, next), undefined)
  assert.equal(storage.snapshot().generations.length, 0)
})

test('offline revalidation recovers the persisted manifest before reporting fallback', async () => {
  const storage = storageModule.createMemoryStorage()
  const identity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const projectionDigest = digest(JSON.stringify(identity))
  const value = descriptor({
    projectionDigest,
    snapshotId: `snapshot-sha256-${projectionDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/projection-data/`,
  })
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: value.snapshotId, identity, children: [] })
  await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes(manifest) })
  await storage.activate(value.publicationKey, value.revisionId)

  const result = await worker.revalidate({ storage, descriptor: value, fetch: async () => { throw new Error('offline') }, origin: 'https://docs.test' })

  assert.deepEqual(result, { kind: 'ready', revisionId: value.revisionId, offline: true })
})

test('offline shell accepts only the public allowlist and omits credentials', async () => {
  const value = descriptor()
  assert.throws(() => worker.validateDescriptor({ ...value, offlineShellUrl: '/manage/offline-shell' }, 'https://docs.test'), /offline shell route/)
  const storage = storageModule.createMemoryStorage()
  await storage.observe(value.publicationKey, value)
  let requestInit
  const cached = await worker.cacheOfflineShell(storage, value, async (_url, init) => {
    requestInit = init
    return new Response('<main>private</main>', { status: 200, headers: { 'X-Manja-Authenticated': 'true' } })
  })

  assert.equal(cached, false)
  assert.equal(requestInit.credentials, 'omit')
  assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl, value), undefined)
})

test('offline shell withdrawal tombstones an active descriptor before fallback', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.observe(value.publicationKey, value)
  const cached = await worker.cacheOfflineShell(storage, value, async () => new Response('gone', { status: 410 }))
  assert.equal(cached, false)
  const state = await storage.loadMetadata(value.publicationKey)
  assert.equal(state.disabled, true)
  assert.equal(state.tombstone.state, 'revoked')
})

test('offline shell accepts the absolute URL returned by a real HTTP fetch', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.observe(value.publicationKey, value)
  const response = new Response('<main>public shell</main>', { status: 200, headers: { 'Content-Security-Policy': "default-src 'self'" } })
  Object.defineProperty(response, 'url', { value: 'https://docs.test/docs/_manja/offline-shell' })
  assert.doesNotThrow(() => worker.validatePublicShellResponse(response, value, 'https://docs.test'))
  const cachedResponse = new Response('<main>public shell</main>', { status: 200, headers: { 'Content-Security-Policy': "default-src 'self'" } })
  Object.defineProperty(cachedResponse, 'url', { value: 'https://manja-local-docs.invalid/docs/_manja/offline-shell' })

  const cached = await worker.cacheOfflineShell(storage, value, async () => cachedResponse)
  assert.equal(cached, true)
  assert.equal(await (await storage.getShell(value.publicationKey, value.offlineShellUrl, value)).text(), '<main>public shell</main>')
})

test('deep-link navigation never replaces canonical offline shell', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
  await storage.activate(value.publicationKey, value.revisionId)
  await storage.putShell(value.publicationKey, value.offlineShellUrl, new Response('<main>canonical</main>'), value)
  const request = { method: 'GET', mode: 'navigate', url: 'https://docs.test/docs/documents/api/', headers: new Headers() }

  const online = await worker.cachedOrFetched({}, storage, value, request, async () => new Response('<main>deep-link</main>', { status: 200 }))
  assert.equal(await online.text(), '<main>deep-link</main>')
  const offline = await worker.cachedOrFetched({}, storage, value, request, async () => { throw new Error('offline') })
  assert.equal(await offline.text(), '<main>canonical</main>')

  const credentialStorage = storageModule.createMemoryStorage()
  await credentialStorage.commitGeneration(value, { ...generationFor(value), projectionBytes: bytes('projection'), manifestBytes: bytes('{}') })
  await credentialStorage.activate(value.publicationKey, value.revisionId)
  const credentialedShell = { method: 'GET', mode: 'navigate', credentials: 'include', url: `https://docs.test${value.offlineShellUrl}`, headers: new Headers() }
  await worker.cachedOrFetched({}, credentialStorage, value, credentialedShell, async () => new Response('<main>credentialed</main>', { status: 200 }))
  assert.equal(await credentialStorage.getShell(value.publicationKey, value.offlineShellUrl, value), undefined)
})

test('worker configuration activates public revision and caches shell without intercepting management', async () => {
  const identity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const projectionDigest = digest(JSON.stringify(identity))
  const value = descriptor({
    projectionDigest,
    snapshotId: `snapshot-sha256-${projectionDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/projection-data/`,
  })
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: value.snapshotId, identity, children: [] })
  const listeners = {}
  const messages = []
  const storage = storageModule.createMemoryStorage()
  const scope = {
    location: { origin: 'https://docs.test' },
    addEventListener: (type, listener) => { listeners[type] = listener },
    clients: { matchAll: async () => [] },
  }
  const runtime = worker.register(scope, {
    storage,
    fetch: async (url) => String(url).endsWith('/offline-shell')
      ? new Response('<main>shell</main>', { status: 200, headers: { 'Content-Security-Policy': "default-src 'self'" } })
      : new Response(manifest, { status: 200, headers: { ETag: '"v1"' } }),
  })
  const event = {
    data: { type: 'manja:configure', descriptor: value },
    source: { postMessage: (message) => messages.push(message) },
    waitUntil: (promise) => { event.pending = promise },
  }
  listeners.message(event)
  await event.pending
  assert.equal((await storage.loadActive(value.publicationKey)).revisionId, value.revisionId)
  assert.equal(await (await storage.getShell(value.publicationKey, value.offlineShellUrl)).text(), '<main>shell</main>')
  assert.ok(messages.some((message) => message.type === 'manja:local-ready'))
  assert.equal(runtime.descriptors.get(value.publicationKey).publicationKey, value.publicationKey)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: 'https://docs.test/manage/' }, value, 'https://docs.test'), false)
})

test('worker re-enables a tombstoned publication only for a new revision', async () => {
  const firstIdentity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const firstDigest = digest(JSON.stringify(firstIdentity))
  const first = descriptor({
    projectionDigest: firstDigest,
    snapshotId: `snapshot-sha256-${firstDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${firstDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${firstDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${firstDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${firstDigest}/projection-data/`,
  })
  const secondIdentity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-2', projectionFormat: 'projection-v2' }
  const secondDigest = digest(JSON.stringify(secondIdentity))
  const second = descriptor({
    revisionId: 'revision-2',
    projectionDigest: secondDigest,
    snapshotId: `snapshot-sha256-${secondDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${secondDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${secondDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${secondDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${secondDigest}/projection-data/`,
  })
  const manifest = (value, identity) => JSON.stringify({ schemaVersion: 1, snapshotId: value.snapshotId, identity, children: [] })
  const storage = storageModule.createMemoryStorage()
  await storage.commitGeneration(first, { ...generationFor(first), projectionBytes: bytes('first'), manifestBytes: bytes(manifest(first, firstIdentity)) })
  await storage.activate(first.publicationKey, first.revisionId)
  await storage.tombstone(first.publicationKey, 'HTTP 410', 'revoked')
  const listeners = {}
  const messages = []
  const scope = {
    location: { origin: 'https://docs.test' },
    addEventListener: (type, listener) => { listeners[type] = listener },
    clients: { matchAll: async () => [] },
  }
  const runtime = worker.register(scope, {
    storage,
    fetch: async (url) => String(url).endsWith('/offline-shell')
      ? new Response('<main>replacement</main>', { status: 200, headers: { 'Content-Security-Policy': "default-src 'self'" } })
      : new Response(manifest(second, secondIdentity), { status: 200, headers: { ETag: '"v2"' } }),
  })
  const event = {
    data: { type: 'manja:configure', descriptor: second },
    source: { postMessage: (message) => messages.push(message) },
    waitUntil: (promise) => { event.pending = promise },
  }
  listeners.message(event)
  await event.pending
  assert.equal((await storage.loadActive(second.publicationKey)).revisionId, 'revision-2')
  assert.ok(messages.some((message) => message.type === 'manja:local-ready'))
  assert.equal(runtime.descriptors.get(second.publicationKey).revisionId, 'revision-2')
})

function generationFor(value) {
  return { publicationKey: value.publicationKey, revisionId: value.revisionId, projectionDigest: value.projectionDigest, snapshotId: value.snapshotId }
}
