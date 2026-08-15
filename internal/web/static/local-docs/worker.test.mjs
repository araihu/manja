import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import test from 'node:test'

import worker from './sw.js'
import storageModule from './storage.js'

const digest = (value) => createHash('sha256').update(value).digest('hex')
const bytes = (value) => new TextEncoder().encode(value)

function descriptor(overrides = {}) {
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

test('descriptor validation is public-only and same-origin', () => {
  const value = descriptor()
  assert.equal(worker.validateDescriptor(value, 'https://docs.test').publicationKey, 'public-api-v1')
  assert.throws(() => worker.validateDescriptor({ ...value, public: false }, 'https://docs.test'), /public eligibility/)
  assert.throws(() => worker.validateDescriptor({ ...value, anonymous: false }, 'https://docs.test'), /public eligibility/)
  assert.throws(() => worker.validateDescriptor({ ...value, projectionManifestUrl: 'https://attacker.test/manifest.json' }, 'https://docs.test'), /same-origin/)
  assert.throws(() => worker.validateDescriptor({ ...value, publicationBase: '/manage/' }, 'https://docs.test'), /publication base/)
})

test('allowlist intercepts only same-origin public reader GETs', () => {
  const value = descriptor()
  const origin = 'https://docs.test'
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/docs/snapshots/${value.snapshotId}/manifest.json` }, value, origin), true)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/docs/documents/api/?selected=operation-list#operation-list` }, value, origin), true)
  assert.equal(worker.isAllowedRequest({ method: 'POST', url: `${origin}/docs/documents/api/` }, value, origin), false)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/manage/` }, value, origin), false)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: `${origin}/api/private` }, value, origin), false)
  assert.equal(worker.isAllowedRequest({ method: 'GET', url: 'https://other.test/docs/documents/api/' }, value, origin), false)
})

test('withdrawal tombstones before purge and blocks cached disclosure', async () => {
  const storage = storageModule.createMemoryStorage()
  const value = descriptor()
  await storage.commitGeneration(value, { revisionId: value.revisionId, projectionBytes: bytes('projection') })
  await storage.activate(value.publicationKey, value.revisionId)
  const purgeOrder = []
  storage.setHooks({
    beforePurge: async () => purgeOrder.push(await storage.isDisabled(value.publicationKey)),
    purge: async () => { purgeOrder.push('purge'); throw new Error('quota') },
  })
  await worker.disablePublication(storage, value, 'revoked')
  assert.deepEqual(purgeOrder, [true, 'purge'])
  assert.equal(await storage.isDisabled(value.publicationKey), true)
  assert.equal(await storage.loadActive(value.publicationKey), undefined)
})

test('candidate activation rolls back storage when runtime preparation fails', async () => {
  const storage = storageModule.createMemoryStorage()
  const value = descriptor()
  await storage.commitGeneration(value, { revisionId: 'revision-0', projectionBytes: bytes('old') })
  await storage.activate(value.publicationKey, 'revision-0')
  await assert.rejects(() => worker.commitCandidate({
    storage,
    descriptor: { ...value, revisionId: 'revision-2' },
    candidate: { revisionId: 'revision-2', projectionBytes: bytes('new') },
    prepare: async () => { throw new Error('renderer rejected candidate') },
  }), /renderer rejected candidate/)
  assert.equal((await storage.loadActive(value.publicationKey)).revisionId, 'revision-0')
  assert.equal((await storage.loadPrevious(value.publicationKey)), undefined)
})

test('freshness revalidation shares one in-flight request and keeps SSR fallback', async () => {
  const storage = storageModule.createMemoryStorage()
  const identity = { schemaVersion: 1, catalogId: 'public-api', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const projectionDigest = digest(JSON.stringify(identity))
  const snapshotId = `snapshot-sha256-${projectionDigest}`
  const value = descriptor({ projectionDigest, snapshotId, projectionManifestUrl: `/docs/snapshots/${snapshotId}/manifest.json`, catalogUrl: `/docs/snapshots/${snapshotId}/catalog.json`, searchDataBase: `/docs/snapshots/${snapshotId}/search-data/`, projectionDataBase: `/docs/snapshots/${snapshotId}/projection-data/` })
  const manifest = JSON.stringify({
    schemaVersion: 1,
    snapshotId: value.snapshotId,
    identity,
    children: [],
  })
  const calls = []
  const revalidate = worker.createRevalidator(() => worker.revalidate({
    storage,
    descriptor: value,
    fetch: async (url, init) => {
      calls.push([url, init])
      await new Promise((resolve) => setTimeout(resolve, 10))
      if (String(url).includes('/_manja/offline-shell')) {
        return new Response('<!doctype html><main>offline shell</main>', {
          status: 200,
          headers: { 'Content-Type': 'text/html' },
        })
      }
      return new Response(manifest, { status: 200, headers: { ETag: '"v1"' } })
    },
    origin: 'https://docs.test',
  }))
  const [first, second] = await Promise.all([revalidate(), revalidate()])
  assert.equal(calls.length, 2)
  assert.equal(calls.filter(([url]) => String(url).includes('/manifest.json')).length, 1)
  assert.equal(first.kind, 'ready')
  assert.equal(second.kind, 'ready')
  assert.equal((await storage.loadMetadata(value.publicationKey)).lastObservedAt !== '', true)
})

test('manifest digest helper is deterministic for exact bytes', async () => {
  const value = bytes('{"schemaVersion":1}')
  assert.equal(await worker.sha256(value), digest(value))
})

test('unknown manifest children never become cached offline resources', async () => {
  const value = descriptor()
  const identity = { schemaVersion: 1, catalogId: value.catalogId, revisionId: value.revisionId, projectionFormat: value.projectionFormat }
  const projectionDigest = digest(JSON.stringify(identity))
  const current = descriptor({
    projectionDigest,
    snapshotId: `snapshot-sha256-${projectionDigest}`,
    projectionManifestUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/manifest.json`,
    catalogUrl: `/docs/snapshots/snapshot-sha256-${projectionDigest}/catalog.json`,
    searchDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/search-data/`,
    projectionDataBase: `/docs/snapshots/snapshot-sha256-${projectionDigest}/projection-data/`,
  })
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: current.snapshotId, identity, children: [] })
  const storage = storageModule.createMemoryStorage()
  const fetches = []
  const scope = { fetch: async (request) => { fetches.push(String(request.url || request)); return new Response('server') } }
  await worker.revalidate({ storage, descriptor: current, fetch: async () => new Response(manifest, { status: 200 }), origin: 'https://docs.test' })
  const request = new Request('https://docs.test' + current.projectionDataBase + 'details/not-declared.json')
  const response = await worker.cachedOrFetched(scope, storage, current, request, async () => new Response('server'))
  assert.equal(await response.text(), 'server')
  assert.equal(await storage.getAsset(current.publicationKey, request.url), undefined)
  assert.equal(fetches.length, 0)
})

test('network document responses only populate the canonical offline shell', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.observe(value.publicationKey, value)
  const request = new Request('https://docs.test/docs/documents/api/?selected=operation-list')
  const response = await worker.cachedOrFetched({ fetch: async () => new Response('selected page') }, storage, value, request)
  assert.equal(await response.text(), 'selected page')
  assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl), undefined)
  assert.ok(await storage.getAsset(value.publicationKey, request.url))
})

test('install rejects when a required fallback asset is unavailable', async () => {
  const listeners = {}
  const scope = {
    location: { origin: 'https://docs.test' },
    addEventListener: (name, handler) => { listeners[name] = handler },
    caches: { open: async () => ({ put: async () => {} }) },
    fetch: async () => new Response('missing', { status: 503 }),
    skipWaiting: async () => {},
  }
  const storage = storageModule.createMemoryStorage()
  worker.register(scope, { storage, assets: ['/manja-assets/local-docs/wasm_exec.js'] })
  let waited
  listeners.install({ waitUntil: (promise) => { waited = promise } })
  await assert.rejects(waited, /fallback asset/)
})

test('disabled publication passes through without reading or writing local bytes', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  await storage.observe(value.publicationKey, value)
  await storage.tombstone(value.publicationKey, 'private successor', 'private')
  let networkCalls = 0
  const request = { url: 'https://docs.test/docs/documents/api/', method: 'GET', mode: 'navigate', headers: new Headers({ Accept: 'text/html' }) }
  const response = await worker.cachedOrFetched({}, storage, value, request, async () => {
    networkCalls++
    return new Response('SSR private boundary', { status: 200, headers: { 'Content-Type': 'text/html' } })
  })
  assert.equal(networkCalls, 1)
  assert.equal(await response.text(), 'SSR private boundary')
  assert.equal(await storage.getAsset(value.publicationKey, request.url), undefined)
  assert.equal(await storage.getShell(value.publicationKey, value.offlineShellUrl), undefined)
})

test('withdrawal disables routing before asynchronous tombstone and notifies clients', async () => {
  const value = descriptor()
  const storage = storageModule.createMemoryStorage()
  const listeners = {}
  const clientMessages = []
  const scope = {
    location: { origin: 'https://docs.test' },
    addEventListener: (name, handler) => { listeners[name] = handler },
    clients: { matchAll: async () => [{ postMessage: (message) => clientMessages.push(message) }] },
    fetch: async () => new Response('server'),
  }
  const runtime = worker.register(scope, { storage })
  await runtime.configure(value)
  let waited
  listeners.message({
    data: { type: 'manja:disable', publicationKey: value.publicationKey, reason: 'publication revoked', state: 'revoked' },
    waitUntil: (promise) => { waited = promise },
  })
  assert.equal(runtime.disabled.has(value.publicationKey), true)
  assert.equal(runtime.descriptors.has(value.publicationKey), false)
  await waited
  assert.equal(await storage.isDisabled(value.publicationKey), true)
  assert.deepEqual(clientMessages, [{ type: 'manja:local-fallback', publicationKey: value.publicationKey, reason: 'publication revoked' }])
  const request = new Request('https://docs.test/docs/documents/api/', { method: 'GET' })
  const response = await worker.cachedOrFetched({}, storage, value, request, async () => new Response('SSR successor'), () => runtime.disabled.has(value.publicationKey))
  assert.equal(await response.text(), 'SSR successor')
  assert.equal(await storage.getAsset(value.publicationKey, request.url), undefined)
})

test('offline shell rejects CSP nonce mismatch before cache write', async () => {
  const response = new Response('<main nonce="wrong">shell</main>', {
    status: 200,
    headers: { 'Content-Type': 'text/html', 'Content-Security-Policy': "script-src 'nonce-right'" },
  })
  await assert.rejects(() => worker.validateShell(response), /CSP nonce differs/)
})
