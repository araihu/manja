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

async function indexedSearchFixture(titles = ['Needle'], query = 'needle') {
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
  await assert.rejects(router.search('reset'), /Search query is too broad/)
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
  for (const query of ['\ud800', 'needle\u0085', 'İ'.repeat(85), '界'.repeat(86)]) {
    assert.throws(() => router.searchClient(query), /Invalid search query/)
  }
})

test('deployment search pins legacy URLs and rejects same-length corrupted bytes', async () => {
  const bytes = new TextEncoder().encode('{"value":"old"}')
  const changed = new TextEncoder().encode('{"value":"new"}')
  const sha = Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256', bytes)), b => b.toString(16).padStart(2, '0')).join('')
  const pinned = `/docs/_manja/search/directory-sha256-${sha}.json`
  const requests = []
  const fixture = searchModel({}, { crypto: webcrypto, fetch: async path => {
    requests.push(path)
    return { ok: true, arrayBuffer: async () => (path === pinned ? bytes : changed).buffer }
  } })
  const router = fixture.window.ManjaCatalogSearchRouter.create(fixture.root)
  const result = await router.fetchVerifiedURL('/docs/_manja/search/directory.json', bytes.length, sha)
  assert.equal(result.value, 'old')
  assert.deepEqual(requests, [pinned])

  const corrupt = searchModel({}, { crypto: webcrypto, fetch: async () => ({ ok: true, arrayBuffer: async () => changed.buffer }) })
  const corruptRouter = corrupt.window.ManjaCatalogSearchRouter.create(corrupt.root)
  await assert.rejects(corruptRouter.fetchVerifiedURL(pinned, bytes.length, sha), /Verified JSON digest differs/)
})

// Expected keys follow application/catalog's NFKC + cases.Fold normalization.
test('static search accepts Unicode queries with the compiler case fold', async () => {
  for (const [query, exact] of [
    ['FICTÍCIO', 'fictício'], ['ficti\u0301cio', 'fictício'],
    ['Ｎｅｅｄｌｅ', 'needle'], ['Straße', 'strasse'], ['ẞ', 'ss'],
    ['ΟΣ', 'οσ'], ['ος', 'οσ'], ['Ꭰꭰ', 'ꭰᎠ'], ['İ', 'i\u0307'],
    ['界', '界'], ['𐐀', '𐐨'], ['\u00a0Needle\u3000', 'needle'],
  ]) {
    const { router } = await indexedSearchFixture([query], exact)
    const results = await router.searchClient(query)
    assert.equal(results.length, 1, query)
    assert.equal(results[0].title, query)
  }
})

test('deployment search handles accented partial and unknown terms without outages', async () => {
  const { router } = await indexedSearchFixture(['API fictícia'], 'fictícia')
  router.loadDeploymentDirectory = async () => ({ catalogs: [
    { catalogId: 'fiction', title: 'Catálogo fictício', mount: '/fiction', documents: [
      { title: 'API fictícia', key: 'public', href: '/fiction/documents/public/', description: 'Contrato administrativo fictício.' },
    ] },
  ] })
  router.catalogRouter = () => router
  const partial = await router.searchDeployment('fictí')
  assert.equal(partial.failures, 0)
  assert.equal(partial.items.length, 2)
  const description = await router.searchDeployment('administrativo')
  assert.equal(description.failures, 0)
  assert.equal(description.items.length, 1)
  assert.equal(description.items[0].description, 'Contrato administrativo fictício.')
  const unknown = await router.searchDeployment('inexistência')
  assert.equal(unknown.failures, 0)
  assert.equal(unknown.items.length, 0)
})

test('Unicode prefix routing uses the compiler scalar sort order', async () => {
  const { router } = await indexedSearchFixture()
  router.loadPostingEntries = async (_references, ordinals) => {
    assert.deepEqual(Array.from(ordinals), [1])
    return new Map([['𐐨route', [0]]])
  }
  const directory = {tokenRoutes: [{key:'界',segment:0},{key:'𐐨route',segment:1}],postingSegments:[]}
  const result = await router.loadCandidates(directory, ['𐐨'], {})
  assert.deepEqual(Array.from(result), [0])
})

test('static search preserves index errors without requesting an absent server fallback', async () => {
  const {router} = await indexedSearchFixture()
  const failure = new Error('Search query is too broad')
  router.searchClient = () => Promise.reject(failure)
  router.searchFallback = () => { throw new Error('unexpected server request') }
  await assert.rejects(router.search('needle'), error => error === failure)
})

test('missing global server fallback fails without fetching the current page as JSON', async () => {
  const fixture = searchModel({searchGlobal:'true'}, {fetch:()=>{throw new Error('unexpected HTTP request')}})
  const router = fixture.window.ManjaCatalogSearchRouter.create(fixture.root)
  await assert.rejects(router.search('needle'), /fallback is not configured/)
})

test('invalid local query errors remain asynchronous for modal recovery', async () => {
 const {router} = await indexedSearchFixture()
 await assert.rejects(router.search('x'.repeat(129)),/Invalid search query/)
})

for (const [status, expected] of [
  [400, 'Enter a shorter search query with letters or numbers.'],
  [422, 'Search is too broad. Add more detail to your query.'],
  [503, 'Search is unavailable. Reload the page and try again.'],
]) {
  test(`hosted search modal presents the recovery for HTTP ${status}`, async () => {
    const { model } = searchModel({ searchGlobal: 'true', searchFallbackUrl: '/search.json', searchMount: '/' }, {
      fetch: async () => ({ ok: false, status }),
    })
    model.$refs = { results: { scrollTop: 10 } }
    model.query = 'reset'
    model.generation = 1
    model.loading = true
    model.runSearch(1)
    await new Promise(resolve => setImmediate(resolve))
    assert.equal(model.loading, false)
    assert.equal(model.error, expected)
    assert.equal(model.results.length, 0)
  })
}
