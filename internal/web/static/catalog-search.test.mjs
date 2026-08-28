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

test('client search resolves deep exact shards and keeps document labels human', async () => {
  const query = 'needle'
  const encoder = new TextEncoder()
  const digest = await webcrypto.subtle.digest('SHA-256', encoder.encode(query))
  const digestHex = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
  const exactPayload = JSON.stringify({
    schemaVersion: 1,
    searchVersion: 1,
    entries: [{ key: query, matches: [{ record: 0, priority: 1 }] }],
  })
  const recordPayload = JSON.stringify({
    schemaVersion: 1,
    searchVersion: 1,
    firstRecord: 0,
    records: [{ detailId: 'detail', documentKey: 'openapi', kind: 'operation', title: 'Needle', href: 'documents/openapi/?selected=detail#detail', operationId: 'needle', method: 'GET', path: '/needle' }],
  })
  const bytes = value => encoder.encode(value)
  const hex = async value => Array.from(new Uint8Array(await webcrypto.subtle.digest('SHA-256', bytes(value))), byte => byte.toString(16).padStart(2, '0')).join('')
  const exactDigest = await hex(exactPayload)
  const recordDigest = await hex(recordPayload)
  const directory = {
    schemaVersion: 1,
    searchVersion: 1,
    exactBuckets: [{ prefix: digestHex, path: `search/exact/${exactDigest}.json`, entries: 1, postings: 1, length: bytes(exactPayload).byteLength, sha256: exactDigest }],
    tokenRoutes: [],
    trigramRoutes: [],
    postingSegments: [],
    trigramSegments: [],
    recordSegments: [{ firstRecord: 0, records: 1, path: `search/records/${recordDigest}.json`, length: bytes(recordPayload).byteLength, sha256: recordDigest }],
    ranks: [{ t: 'Needle', k: 'operation' }],
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
  const records = await router.searchClient(query)
  assert.equal(records.length, 1)
  assert.equal(records[0].section, 'Virtual Infrastructure JSON API')
  assert.equal(records[0].href, '/group/project/pets/documents/openapi/?selected=detail#detail')
})
