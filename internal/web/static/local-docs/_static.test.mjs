import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
import { webcrypto } from 'node:crypto'

function enhancer(pathname = '/group/project/pets/documents/doc/', additions = {}) {
  const location = additions.location || new URL(`https://docs.test${pathname}`)
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
  const location = new URL('https://docs.test/group/project/pets/documents/doc/?selected=wanted#wanted')
  const history = {
    pushes: [],
    pushStates: [],
    replacements: [],
    replaceStates: [],
    state: null,
    scrollRestoration: 'auto',
    pushState(state, _title, href) {
      this.pushes.push(href)
      this.pushStates.push(state)
      this.state = state
      location.href = new URL(href, location.href).href
    },
    replaceState(state, _title, href) {
      this.replacements.push(href)
      this.replaceStates.push(state)
      this.state = state
      location.href = new URL(href, location.href).href
    },
  }
  const focusCalls = []
  const groupFocusCalls = []
  const focusTarget = { focus(options) { focusCalls.push(options) } }
  const mainScroll = { scrollTop: 0 }
  const nav = {
    scrollTop: 0,
    clientHeight: 100,
    scrollHeight: 1000,
    getBoundingClientRect() { return { top: 0, bottom: 100 } },
    querySelector() { return null },
    getAttribute(name) { return name === 'data-manja-static-default-open' ? 'false' : null },
  }
  const groupControl = {
    getAttribute(name) {
      if (name === 'data-manja-static-group') return 'group'
      if (name === 'aria-expanded') return 'false'
      return null
    },
    closest(selector) { return selector === 'nav[data-manja-local-sidebar]' ? nav : null },
    focus(options) { groupFocusCalls.push(options) },
  }
  const listeners = {}
  const windowListeners = {}
  const api = enhancer('/group/project/pets/documents/doc/?selected=wanted#wanted', {
    caches: { open: async () => cache },
    fetch,
    navigator: { serviceWorker },
    history,
    location,
    addEventListener(type, handler) { windowListeners[type] = handler },
  })
  const root = { dataset: {}, dispatchEvent() {} }
  let mainHTML = ''
  let mainWrites = 0
  const main = {
    scrollTop: 0,
    closest(selector) { return selector === '[data-manja-primary-scroll]' ? mainScroll : null },
    querySelector(selector) { return selector === '[data-manja-settled-focus="true"]' ? focusTarget : null },
  }
  Object.defineProperty(main, 'innerHTML', { get() { return mainHTML }, set(value) { mainWrites += 1; mainHTML = value } })
  const sidebar = {
    innerHTML: '',
    querySelector(selector) { return selector === 'nav[data-manja-local-sidebar]' ? nav : null },
    querySelectorAll(selector) { return selector === '[data-manja-static-group]' ? [groupControl] : [] },
  }
  const script = { textContent: JSON.stringify(value) }
  const document = {
    documentElement: root,
    title: '',
    getElementById(id) { return id === 'manja-local-docs-descriptor' ? script : id === 'catalog-sidebar-groups' ? sidebar : null },
    querySelector(selector) { return selector === '[data-catalog-main-content]' ? main : null },
    addEventListener(type, handler) { listeners[type] = handler },
  }
  const prepared = []
  const abi = {
    activate: () => ({ ok: true, catalogId: 'pets', publicationKey: 'pets', snapshotId: value.snapshotId, revisionId: 'revision-1', projectionDigest: value.projectionDigest }),
    prepare: (_descriptor, _manifest, _catalog, loaded) => { prepared.push(Object.keys(loaded).sort()); return { ok: true } },
    render: () => ({ ok: true, mainHtml: '<p>Wanted</p>', sidebarHtml: '<nav></nav>', title: 'Wanted', canonical: value.publicationBase + 'documents/doc/?selected=wanted#wanted' }),
  }
  const result = await api.start({ document, loadABI: () => abi })
  return { result, root, requests, prepared, value, api, history, focusCalls, groupFocusCalls, mainScroll, nav, groupControl, listeners, windowListeners, location, mainWrites: () => mainWrites }
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

test('static router exposes same-document navigation for search results', async () => {
  const fixture = await staticActivationFixture()
  assert.equal(typeof fixture.api.navigate, 'function')
  const pending = fixture.api.navigate('https://docs.test/group/project/pets/documents/doc/?selected=wanted#wanted')
  assert.ok(pending && typeof pending.then === 'function')
  const result = await pending
  assert.equal(result.ok, true)
  assert.deepEqual(fixture.history.pushes, [fixture.value.publicationBase + 'documents/doc/?selected=wanted#wanted'])
  assert.equal(fixture.focusCalls.length, 1)
})

test('static detail navigation resets main scroll and saves the previous entry', async () => {
  const fixture = await staticActivationFixture()
  assert.equal(fixture.history.scrollRestoration, 'manual')
  assert.equal(fixture.mainScroll.scrollTop, 0)
  assert.equal(fixture.nav.scrollTop, 0)

  fixture.mainScroll.scrollTop = 420
  fixture.nav.scrollTop = 77
  await fixture.api.navigate('https://docs.test/group/project/pets/documents/doc/?selected=wanted#wanted')

  assert.equal(fixture.mainScroll.scrollTop, 0)
  assert.equal(fixture.nav.scrollTop, 77)
  assert.equal(fixture.history.replaceStates.at(-1).manjaLocalDocs.main, 420)
  assert.equal(fixture.history.replaceStates.at(-1).manjaLocalDocs.sidebar, 77)
  assert.equal(fixture.history.pushStates.at(-1).manjaLocalDocs.main, 0)
  assert.equal(fixture.history.pushStates.at(-1).manjaLocalDocs.sidebar, 77)
  assert.equal(fixture.focusCalls.length, 1)
})

test('static group toggles replace history, preserve both scroll containers, and restore control focus', async () => {
  const fixture = await staticActivationFixture()
  const writesBefore = fixture.mainWrites()
  fixture.mainScroll.scrollTop = 321
  fixture.nav.scrollTop = 88

  let prevented = false
  fixture.listeners.click({
    target: {
      closest(selector) {
        if (selector === 'a[href]') return null
        if (selector === '[data-manja-static-group]') return fixture.groupControl
        return null
      },
    },
    preventDefault() { prevented = true },
  })
  await new Promise(resolve => setTimeout(resolve, 0))

  assert.equal(prevented, true)
  assert.equal(fixture.history.pushes.length, 0)
  assert.equal(fixture.history.replacements.length, 2)
  assert.equal(fixture.history.replaceStates.at(-1).manjaLocalDocs.main, 321)
  assert.equal(fixture.history.replaceStates.at(-1).manjaLocalDocs.sidebar, 88)
  assert.equal(fixture.mainScroll.scrollTop, 321)
  assert.equal(fixture.nav.scrollTop, 88)
  assert.equal(fixture.mainWrites(), writesBefore)
  assert.equal(fixture.groupFocusCalls.length, 1)
})

test('static popstate restores saved nested scroll without stealing focus', async () => {
  const fixture = await staticActivationFixture()
  fixture.mainScroll.scrollTop = 410
  fixture.nav.scrollTop = 29
  await fixture.api.navigate('https://docs.test/group/project/pets/documents/doc/?selected=wanted#wanted')
  const savedInitialState = fixture.history.replaceStates.at(-1)
  const detailFocusCount = fixture.focusCalls.length

  fixture.location.href = 'https://docs.test/group/project/pets/documents/doc/?selected=wanted#wanted'
  fixture.history.state = savedInitialState
  fixture.mainScroll.scrollTop = 900
  fixture.nav.scrollTop = 900
  fixture.windowListeners.popstate()
  await new Promise(resolve => setTimeout(resolve, 0))

  assert.equal(fixture.mainScroll.scrollTop, 410)
  assert.equal(fixture.nav.scrollTop, 29)
  assert.equal(fixture.focusCalls.length, detailFocusCount)
})
