import assert from 'node:assert/strict'
import { webcrypto } from 'node:crypto'
import fs from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

function searchModel(dataset = {}, additions = {}) {
  const assignments = []
  const events = []
  const window = {
    location: { origin: 'https://docs.test', assign: href => assignments.push(href) },
    navigator: {},
    addEventListener() {},
    dispatchEvent: event => events.push(event),
    ManjaLocalDocsEnhancer: null,
    ...additions,
  }
  const document = {
    readyState: 'complete',
    addEventListener() {},
    querySelectorAll() { return [] },
    getElementById() { return null },
  }
  class CustomEvent { constructor(type, options = {}) { this.type = type; this.detail = options.detail } }
  vm.runInNewContext(fs.readFileSync(new URL('./catalog-search.js', import.meta.url), 'utf8'), {
    window, document, navigator: window.navigator, URL, TextEncoder, TextDecoder, Promise, CustomEvent,
    crypto: additions.crypto, fetch: additions.fetch,
  })
  const root = { dataset: { searchMount: '/group/project/pets', searchCatalogId: 'pets', ...dataset } }
  const model = window.manjaCatalogSearch(root)
  model.$nextTick = callback => callback()
  return { model, root, window, assignments, events }
}

test('search selection delegates same-publication document routes to static router', async () => {
  const fixture = searchModel()
  const navigations = []
  const closed = []
  fixture.window.ManjaLocalDocsEnhancer = {
    navigate: href => {
      navigations.push(href)
      return Promise.resolve({ ok: true })
    },
  }
  fixture.model.closeSearch = restoreFocus => closed.push(restoreFocus)

  fixture.model.select({ href: '/group/project/pets/documents/core/?selected=detail#detail', title: 'Get pets' })
  await new Promise(resolve => setImmediate(resolve))

  assert.deepEqual(navigations, ['/group/project/pets/documents/core/?selected=detail#detail'])
  assert.deepEqual(closed, [false])
  assert.deepEqual(fixture.assignments, [])
})

test('search selection keeps normal navigation for non-document routes', () => {
  const fixture = searchModel()
  fixture.window.ManjaLocalDocsEnhancer = { navigate: () => null }

  fixture.model.select({ href: '/group/project/pets/search/?q=pets', title: 'Search pets' })

  assert.deepEqual(fixture.assignments, ['/group/project/pets/search/?q=pets'])
})

async function indexedSearchFixture(titles = ['Needle']) {
  const query = 'needle'
  const encoder = new TextEncoder()
  const digest = await webcrypto.subtle.digest('SHA-256', encoder.encode(query))
  const digestHex = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
  const exactPayload = JSON.stringify({
    schemaVersion: 1,
    searchVersion: 1,
    entries: [{ key: query, matches: titles.map((_, record) => ({ record, priority: 1 })) }],
  })
  const recordPayload = JSON.stringify({
    schemaVersion: 1,
    searchVersion: 1,
    firstRecord: 0,
    records: titles.map((title, index) => ({ detailId: `detail-${index}`, documentKey: 'openapi', kind: 'operation', title, href: `documents/openapi/?selected=detail-${index}#detail-${index}`, operationId: 'needle', method: 'GET', path: '/needle' })),
  })
  const bytes = value => encoder.encode(value)
  const hex = async value => Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256', bytes(value))), byte => byte.toString(16).padStart(2, '0')).join('')
  const exactDigest = await hex(exactPayload)
  const recordDigest = await hex(recordPayload)
  const directory = {
    schemaVersion: 1,
    searchVersion: 1,
    exactBuckets: [{ prefix: digestHex, path: `search/exact/${exactDigest}.json`, entries: 1, postings: titles.length, length: bytes(exactPayload).byteLength, sha256: exactDigest }],
    tokenRoutes: [],
    trigramRoutes: [],
    postingSegments: [],
    trigramSegments: [],
    recordSegments: [{ firstRecord: 0, records: titles.length, path: `search/records/${recordDigest}.json`, length: bytes(recordPayload).byteLength, sha256: recordDigest }],
    ranks: titles.map(t => ({ t, k: 'operation' })),
  }
  const directoryPayload = JSON.stringify(directory)
  const directoryDigest = await hex(directoryPayload)
  const payloads = new Map([
    ['search/directory.json', directoryPayload],
    [directory.exactBuckets[0].path, exactPayload],
    [directory.recordSegments[0].path, recordPayload],
  ])
  const fetch = async input => {
    const path = new URL(input, 'https://docs.test').pathname.replace('/search-data/', '')
    const payload = payloads.get(path)
    if (payload === undefined) return { ok: false, arrayBuffer: async () => new ArrayBuffer(0) }
    return { ok: true, arrayBuffer: async () => bytes(payload).buffer }
  }
  const fixture = searchModel({
    searchChildBase: '/search-data/',
    searchDirectoryPath: 'search/directory.json',
    searchDirectoryLength: bytes(directoryPayload).byteLength,
    searchDirectorySha256: directoryDigest,
    searchDocumentLabels: JSON.stringify({ openapi: 'Virtual Infrastructure JSON API' }),
  }, { crypto: webcrypto, fetch })
  const router = fixture.window.ManjaCatalogSearchRouter.create(fixture.root)
  return { router, fixture }
}

test('client search resolves deep exact shards and keeps document labels human', async () => {
  const { router } = await indexedSearchFixture()
  const records = await router.searchClient('needle')
  assert.equal(records.length, 1)
  assert.equal(records[0].section, 'Virtual Infrastructure JSON API')
  assert.equal(records[0].href, '/group/project/pets/documents/openapi/?selected=detail-0#detail-0')
})

test('global search reports a broad query instead of a generic outage', async () => {
  const fixture = searchModel({
    searchGlobal: 'true',
    searchFallbackUrl: '/search.json',
    searchMount: '/',
  }, {
    fetch: async () => ({ ok: false, status: 422 }),
  })
  const router = fixture.window.ManjaCatalogSearchRouter.create(fixture.root)
  await assert.rejects(router.search('reset'), /Search is too broad\. Add another term/)
})

test('deployment ranking accepts multiline spec descriptions as corpus text', () => {
  const fixture = searchModel()
  const rank = fixture.window.ManjaCatalogSearchRouter.deploymentNavigationMatch
  assert.equal(rank('Reconfigures the alarm properties.\nAdditional VMware details.', 'alarm properties') >= 0, true)
  assert.equal(rank('Unrelated description\nwith several words', 'alarm properties'), -1)
})

test('Ctrl K refocuses an already-open dialog restored by browser history', () => {
  const fixture = searchModel()
  let focused = 0
  let focusOptions = null
  let prevented = false
  fixture.model.$refs = {
    input: {
      dataset: {},
      addEventListener() {},
      focus: options => { focused++; focusOptions = options },
    },
  }
  fixture.model.open = true
  fixture.model.handleWindowKey({
    defaultPrevented: false,
    ctrlKey: true,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    key: 'k',
    preventDefault: () => { prevented = true },
  })
  assert.equal(prevented, true)
  assert.equal(focused, 1)
  assert.equal(focusOptions?.focusVisible, true)
  assert.equal(fixture.model.$refs.input.dataset.keyboardFocus, 'true')
})

const corpusTitles = [
  'Needle\nAdditional details',
  'Needle · Virtual infrastructure',
  'Needle\twith controls\u0085and more',
  'Needle ' + 'long description '.repeat(30),
  '',
  'Ｎｅｅｄｌｅ',
]

test('client ranking accepts unrestricted corpus text and preserves exact title priority', async () => {
  const { router } = await indexedSearchFixture(corpusTitles)
  const records = await router.searchClient('needle')
  assert.equal(records.length, corpusTitles.length)
  assert.equal(records[0].title, 'Ｎｅｅｄｌｅ')
  assert.deepEqual(Array.from(records, record => record.title).sort(), [...corpusTitles].sort())
})

test('deployment search ranks corpus titles without reporting healthy catalogs unavailable', async () => {
  const { router } = await indexedSearchFixture(corpusTitles)
  router.loadDeploymentDirectory = async () => ({ catalogs: [
    { catalogId: 'pets', title: 'Needle · Catalog', mount: '/pets', documents: [
      { title: 'Needle\nDocument', key: 'openapi', href: '/pets/documents/openapi/' },
      { title: 'Needle · Other', key: 'other', href: '/pets/documents/other/' },
    ] },
  ] })
  router.catalogRouter = () => router
  const result = await router.searchDeployment('needle')
  assert.equal(result.failures, 0)
  assert.equal(result.items.length, corpusTitles.length + 3)
  assert.deepEqual(Array.from(result.items.filter(item => item.kind === 'operation'), item => item.title).sort(), [...corpusTitles].sort())
})

test('client queries retain strict validation independently of corpus normalization', async () => {
  const { router } = await indexedSearchFixture(corpusTitles)
  for (const query of ['', 'needle\n', 'needle\t', 'n'.repeat(129), 'n'.repeat(257)]) {
    assert.throws(() => router.searchClient(query), /Invalid search query/)
  }
  assert.throws(() => router.searchClient('Ｎｅｅｄｌｅ'), /Server normalization required/)
})
