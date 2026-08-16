import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
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
