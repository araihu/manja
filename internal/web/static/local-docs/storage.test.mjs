import assert from 'node:assert/strict'
import test from 'node:test'

import storageModule from './storage.js'

const digest = (letter) => letter.repeat(64)
const descriptor = (key, revisionId) => ({
  publicationKey: key,
  revisionId,
  projectionDigest: digest('0'),
  snapshotId: `snapshot-sha256-${digest('0')}`,
  offlineShellUrl: `/${key}/_manja/offline-shell`,
})
const generation = (key, revisionId) => ({
  publicationKey: key,
  revisionId,
  projectionDigest: digest('0'),
  snapshotId: `snapshot-sha256-${digest('0')}`,
  projectionBytes: new TextEncoder().encode(revisionId),
})

test('publication keeps active and previous generations and drops unreachable candidates', async () => {
  const storage = storageModule.createMemoryStorage({ now: (() => { let index = 0; return () => new Date(2026, 0, ++index) })() })
  const first = descriptor('public-api', 'revision-a')
  await storage.commitGeneration(first, generation('public-api', 'revision-a'))
  await storage.activate('public-api', 'revision-a')
  for (const revisionId of ['revision-b', 'revision-c']) {
    const candidate = generation('public-api', revisionId)
    await storage.commitGeneration(descriptor('public-api', revisionId), candidate)
    await storage.activate('public-api', revisionId)
  }
  assert.equal((await storage.loadActive('public-api')).revisionId, 'revision-c')
  assert.equal((await storage.loadPrevious('public-api')).revisionId, 'revision-b')
  assert.deepEqual(storage.snapshot().generations.map((item) => item.revisionId).sort(), ['revision-b', 'revision-c'])
})

test('LRU retains three public publications and evicts the fourth without touching active peers', async () => {
  const storage = storageModule.createMemoryStorage({ now: (() => { let index = 0; return () => new Date(2026, 0, ++index) })() })
  for (const key of ['one', 'two', 'three', 'four']) {
    const value = descriptor(`public-${key}`, `revision-${key}`)
    await storage.commitGeneration(value, generation(value.publicationKey, value.revisionId))
    await storage.activate(value.publicationKey, value.revisionId)
  }
  assert.equal(await storage.loadActive('public-one'), undefined)
  assert.ok(await storage.loadActive('public-two'))
  assert.ok(await storage.loadActive('public-three'))
  assert.ok(await storage.loadActive('public-four'))
})

test('tombstone survives purge failure and does not resurrect old bytes', async () => {
  const storage = storageModule.createMemoryStorage()
  const value = descriptor('public-api', 'revision-a')
  await storage.commitGeneration(value, generation('public-api', 'revision-a'))
  await storage.activate('public-api', 'revision-a')
  storage.setHooks({ purge: async () => { throw new Error('purge failed') } })
  await storage.tombstone('public-api', 'HTTP 410', 'revoked')
  assert.equal(await storage.isDisabled('public-api'), true)
  assert.equal(await storage.loadActive('public-api'), undefined)
  assert.equal((await storage.loadMetadata('public-api')).tombstone.state, 'revoked')
})

test('same publication mutations serialize and never write after tombstone', async () => {
  const storage = storageModule.createMemoryStorage()
  const value = descriptor('public-api', 'revision-a')
  await storage.commitGeneration(value, generation('public-api', 'revision-a'))
  await storage.activate(value.publicationKey, value.revisionId)
  await storage.putAsset(value.publicationKey, '/docs/asset.js', new Response('asset'))
  await storage.tombstone(value.publicationKey, 'revoked', 'revoked')
  await assert.rejects(() => storage.putAsset(value.publicationKey, '/docs/new.js', new Response('new')), /disabled/)
  await assert.rejects(() => storage.commitGeneration({ ...value, revisionId: 'revision-b' }, generation('public-api', 'revision-b')), /disabled/)
  assert.equal(await storage.getAsset(value.publicationKey, '/docs/asset.js'), undefined)
  assert.equal(await storage.loadActive(value.publicationKey), undefined)
})

test('a newer authoritative revision can re-enable a tombstoned publication only after activation', async () => {
  const storage = storageModule.createMemoryStorage()
  const first = descriptor('public-api', 'revision-a')
  await storage.commitGeneration(first, generation('public-api', 'revision-a'))
  await storage.activate(first.publicationKey, first.revisionId)
  await storage.tombstone(first.publicationKey, 'private', 'private')
  assert.equal(await storage.isReenableAllowed(first.publicationKey, 'revision-a'), false)
  assert.equal(await storage.isReenableAllowed(first.publicationKey, 'revision-b'), true)
  await storage.allowReenable(first.publicationKey, 'revision-b')
  await storage.commitGeneration({ ...first, revisionId: 'revision-b' }, generation('public-api', 'revision-b'))
  await storage.activate(first.publicationKey, 'revision-b')
  assert.equal((await storage.loadActive(first.publicationKey)).revisionId, 'revision-b')
  assert.equal((await storage.loadMetadata(first.publicationKey)).disabled, false)
})
