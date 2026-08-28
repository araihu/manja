import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'

function searchModel() {
  const assignments = []
  const events = []
  const window = {
    location: { origin: 'https://docs.test', assign: href => assignments.push(href) },
    navigator: {},
    addEventListener() {},
    dispatchEvent: event => events.push(event),
    ManjaLocalDocsEnhancer: null,
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
  })
  const root = { dataset: { searchMount: '/group/project/pets', searchCatalogId: 'pets' } }
  const model = window.manjaCatalogSearch(root)
  model.$nextTick = callback => callback()
  return { model, window, assignments, events }
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
