import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
import { webcrypto } from 'node:crypto'

function enhancer(pathname = '/group/project/pets/documents/doc/', additions = {}) {
  const location = new URL(`https://docs.test${pathname}`)
  const window = {
    URL,
    TextEncoder,
    TextDecoder,
    crypto: webcrypto,
    location,
    navigator: {},
    document: null,
    addEventListener() {},
    history: { pushState() {} },
    ...additions,
  }
  class CustomEvent { constructor(type, options = {}) { this.type = type; this.detail = options.detail } }
  vm.runInNewContext(fs.readFileSync(new URL('../local-docs.js', import.meta.url), 'utf8'), { window, URL, TextEncoder, TextDecoder, Response, Promise, CustomEvent, setTimeout, clearTimeout })
  return window.ManjaLocalDocsEnhancer
}

function descriptor(overrides = {}) {
  const digest = 'a'.repeat(64)
  const snapshot = `snapshot-sha256-${digest}`
  return {
    schemaVersion: 1,
    catalogId: 'pets',
    publicationKey: 'pets',
    public: true,
    anonymous: true,
    publicationBase: '/group/project/pets/',
    snapshotId: snapshot,
    revisionId: 'revision-1',
    projectionFormat: 'projection-v2',
    projectionDigest: digest,
    projectionManifestUrl: `/group/project/pets/snapshots/${snapshot}/manifest.json`,
    catalogUrl: `/group/project/pets/snapshots/${snapshot}/catalog.json`,
    searchDataBase: `/group/project/pets/snapshots/${snapshot}/search-data/`,
    projectionDataBase: `/group/project/pets/snapshots/${snapshot}/projection-data/`,
    static: {
      deploymentBase: '/group/project/',
      workerUrl: '/group/project/sw.js',
      workerScope: '/group/project/',
      offlineShellUrl: '/group/project/pets/_manja/offline-shell/',
      exportManifestUrl: '/group/project/_manja/export.json',
    },
    ...overrides,
  }
}

test('static descriptor binds deployment worker shell and export manifest routes', () => {
  const api = enhancer()
  const value = api.validateDescriptor(descriptor())
  assert.equal(value.offlineShellUrl, value.static.offlineShellUrl)
  assert.throws(() => api.validateDescriptor(descriptor({ static: { ...descriptor().static, workerScope: '/' } })), /static routes/)
  assert.throws(() => api.validateDescriptor(descriptor({ static: { ...descriptor().static, deploymentBase: '/other/' } })), /static routes/)
})

test('static route parses direct selection node and expanded groups inside publication only', () => {
  const api = enhancer()
  const value = api.validateDescriptor(descriptor())
  assert.deepEqual(
    { ...api.staticRoute(value, 'https://docs.test/group/project/pets/documents/doc/?selected=detail&node=7&group=one&group=two&closed=three#schema-node-panel') },
    { documentKey: 'doc', selected: 'detail', node: 7, groups: ['one', 'two'], closedGroups: ['three'] },
  )
  assert.equal(api.staticRoute(value, 'https://docs.test/group/project/other/documents/doc/'), null)
  assert.equal(api.staticRoute(value, 'https://attacker.test/group/project/pets/documents/doc/'), null)
})

async function sha256(value) {
  const bytes = new TextEncoder().encode(value)
  const digest = await webcrypto.subtle.digest('SHA-256', bytes)
  return { bytes, hex: Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('') }
}

async function staticActivationFixture(failedPath = '') {
  const identity = { schemaVersion: 1, catalogId: 'pets', revisionId: 'revision-1', projectionFormat: 'projection-v2' }
  const identityDigest = await sha256(JSON.stringify(identity))
  const value = descriptor({
    projectionDigest: identityDigest.hex,
    snapshotId: `snapshot-sha256-${identityDigest.hex}`,
  })
  value.projectionManifestUrl = `${value.publicationBase}snapshots/${value.snapshotId}/manifest.json`
  value.catalogUrl = `${value.publicationBase}snapshots/${value.snapshotId}/catalog.json`
  value.searchDataBase = `${value.publicationBase}snapshots/${value.snapshotId}/search-data/`
  value.projectionDataBase = `${value.publicationBase}snapshots/${value.snapshotId}/projection-data/`

  const catalog = JSON.stringify({ catalogId: 'pets', documents: [{ key: 'doc', operations: [{ detailId: 'wanted', detailChild: 'details/doc.json' }], schemas: [], schemaNodeShards: [] }] })
  const payloads = {
    'catalog.json': catalog,
    'details/doc.json': JSON.stringify({ schemaVersion: 1, documentKey: 'doc', records: [{ id: 'wanted', kind: 'operation', operation: {} }] }),
    'details/other.json': JSON.stringify({ schemaVersion: 1, documentKey: 'other', records: [] }),
    'search/directory.json': JSON.stringify({ schemaVersion: 1, searchVersion: 1 }),
  }
  const children = []
  for (const path of Object.keys(payloads).sort()) {
    const hashed = await sha256(payloads[path])
    children.push({ path, kind: path === 'catalog.json' ? 'catalog' : path.startsWith('details/') ? 'detail' : 'search-directory', length: hashed.bytes.byteLength, sha256: hashed.hex })
  }
  const manifest = JSON.stringify({ schemaVersion: 1, snapshotId: value.snapshotId, identity, children })
  const exported = JSON.stringify({ schemaVersion: 1, basePath: '/group/project/', catalogs: [{ catalogId: 'pets', publicationKey: 'pets', revisionId: 'revision-1', snapshotId: value.snapshotId }] })
  const requests = []
  const responses = new Map([
    [value.static.exportManifestUrl, exported],
    [value.projectionManifestUrl, manifest],
    [value.catalogUrl, catalog],
    ...children.filter(child => child.path !== 'catalog.json').map(child => [child.path.startsWith('search/') ? value.searchDataBase + child.path : value.projectionDataBase + child.path, payloads[child.path]]),
  ])
  const cache = { match: async () => undefined, put: async () => undefined }
  const worker = { postMessage() {} }
  const serviceWorker = { register: async () => ({ active: worker }), ready: Promise.resolve({ active: worker }), addEventListener() {} }
  const fetch = async input => {
    const path = new URL(input, 'https://docs.test').pathname
    requests.push(path)
    if (failedPath && path.endsWith(failedPath)) throw new Error('network down')
    const body = responses.get(path)
    if (body === undefined) return new Response('', { status: 404 })
    return new Response(body, { status: 200, headers: { 'Content-Length': String(new TextEncoder().encode(body).byteLength) } })
  }
  const api = enhancer('/group/project/pets/documents/doc/?selected=wanted#wanted', { caches: { open: async () => cache }, fetch, navigator: { serviceWorker } })
  const root = { dataset: {}, dispatchEvent() {} }
  const main = { innerHTML: '' }
  const sidebar = { innerHTML: '' }
  const script = { textContent: JSON.stringify(value) }
  const document = {
    documentElement: root,
    title: '',
    getElementById(id) { return id === 'manja-local-docs-descriptor' ? script : id === 'catalog-sidebar-groups' ? sidebar : null },
    querySelector(selector) { return selector === '[data-catalog-main-content]' ? main : null },
    addEventListener() {},
  }
  const prepared = []
  const abi = {
    activate: () => ({ ok: true, catalogId: 'pets', publicationKey: 'pets', snapshotId: value.snapshotId, revisionId: 'revision-1', projectionDigest: value.projectionDigest }),
    prepare: (_descriptor, _manifest, _catalog, loaded) => { prepared.push(Object.keys(loaded).sort()); return { ok: true } },
    render: () => ({ ok: true, mainHtml: '<p>Wanted</p>', sidebarHtml: '<nav></nav>', title: 'Wanted', canonical: value.publicationBase + 'documents/doc/?selected=wanted#wanted' }),
  }
  const result = await api.start({ document, loadABI: () => abi })
  return { result, root, requests, prepared, value }
}

test('static direct route loads only its required projection child', async () => {
  const fixture = await staticActivationFixture()
  assert.equal(fixture.result.ok, true)
  assert.deepEqual(fixture.prepared, [['details/doc.json']])
  assert.deepEqual(fixture.requests.filter(path => path.includes('/projection-data/') || path.includes('/search-data/')), [fixture.value.projectionDataBase + 'details/doc.json'])
})

test('static child failure reports its manifest path', async () => {
  const fixture = await staticActivationFixture('details/doc.json')
  assert.equal(fixture.result.ok, false)
  assert.match(fixture.root.dataset.manjaLocalDocsReason, /details\/doc\.json/)
})
