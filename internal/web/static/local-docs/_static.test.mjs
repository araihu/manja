import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import vm from 'node:vm'
import { webcrypto } from 'node:crypto'

function enhancer(pathname = '/group/project/pets/documents/doc/') {
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
  }
  vm.runInNewContext(fs.readFileSync(new URL('../local-docs.js', import.meta.url), 'utf8'), { window, URL, TextEncoder, TextDecoder, Response, Promise, setTimeout, clearTimeout })
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
    { ...api.staticRoute(value, 'https://docs.test/group/project/pets/documents/doc/?selected=detail&node=7&group=one&group=two#schema-node-panel') },
    { documentKey: 'doc', selected: 'detail', node: 7, groups: ['one', 'two'] },
  )
  assert.equal(api.staticRoute(value, 'https://docs.test/group/project/other/documents/doc/'), null)
  assert.equal(api.staticRoute(value, 'https://attacker.test/group/project/pets/documents/doc/'), null)
})
