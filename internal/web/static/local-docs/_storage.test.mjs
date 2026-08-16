import assert from 'node:assert/strict'
import test from 'node:test'

import storageModule from './storage.js'

const digest = (letter) => letter.repeat(64)
const descriptor = (key, revisionId) => ({
  catalogId: key,
  publicationKey: key,
  publicationBase: `/${key}/`,
  snapshotId: `snapshot-sha256-${digest('0')}`,
  revisionId,
  projectionFormat: 'projection-v2',
  projectionDigest: digest('0'),
  projectionManifestUrl: `/${key}/snapshots/snapshot-sha256-${digest('0')}/manifest.json`,
  catalogUrl: `/${key}/snapshots/snapshot-sha256-${digest('0')}/catalog.json`,
  searchDataBase: `/${key}/snapshots/snapshot-sha256-${digest('0')}/search-data/`,
  projectionDataBase: `/${key}/snapshots/snapshot-sha256-${digest('0')}/projection-data/`,
  offlineShellUrl: `/${key}/_manja/offline-shell`,
})
const generation = (key, revisionId) => ({
  publicationKey: key,
  revisionId,
  projectionDigest: digest('0'),
  snapshotId: `snapshot-sha256-${digest('0')}`,
  projectionBytes: new TextEncoder().encode(revisionId),
})

test('storage keeps active and previous generations and evicts stale publications by LRU', async () => {
  const storage = storageModule.createMemoryStorage({
    now: (() => { let tick = 0; return () => new Date(2026, 0, ++tick) })(),
  })
  for (const revisionId of ['revision-a', 'revision-b', 'revision-c']) {
    const value = descriptor('public-api', revisionId)
    await storage.commitGeneration(value, generation('public-api', revisionId))
    await storage.activate('public-api', revisionId)
  }
  assert.equal((await storage.loadActive('public-api')).revisionId, 'revision-c')
  assert.equal((await storage.loadPrevious('public-api')).revisionId, 'revision-b')
  assert.deepEqual(storage.snapshot().generations.map((item) => item.revisionId).sort(), ['revision-b', 'revision-c'])

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

test('storage tombstones before purge and never resurrects withdrawn bytes', async () => {
  const storage = storageModule.createMemoryStorage()
  const value = descriptor('public-api', 'revision-a')
  await storage.commitGeneration(value, generation('public-api', 'revision-a'))
  await storage.activate(value.publicationKey, value.revisionId)
  const order = []
  storage.setHooks({
    beforePurge: async () => order.push(await storage.isDisabled(value.publicationKey)),
    purge: async () => { order.push('purge'); throw new Error('quota') },
  })
  await storage.tombstone(value.publicationKey, 'HTTP 410', 'revoked')
  assert.deepEqual(order, [true, 'purge'])
  assert.equal(await storage.isDisabled(value.publicationKey), true)
  assert.equal(await storage.loadActive(value.publicationKey), undefined)
  assert.equal((await storage.loadMetadata(value.publicationKey)).tombstone.state, 'revoked')
})
