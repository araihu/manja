(function (root, factory) {
  "use strict"
  const api = factory()
  if (typeof module === "object" && module.exports) module.exports = api
  root.ManjaLocalDocsStorage = api
}(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict"

  const DATABASE_NAME = "manja-local-docs"
  const DATABASE_VERSION = 1
  const METADATA_STORE = "publications"
  const GENERATION_STORE = "generations"
  const CACHE_NAME = "manja-local-docs-assets-v1"
  const MAX_PUBLICATIONS = 3
  const MAX_GENERATIONS = 2
  const SCHEMA_VERSION = 1
  const DIGEST_PATTERN = /^[0-9a-f]{64}$/

  function cloneBytes(value, name) {
    if (!(value instanceof Uint8Array)) throw new TypeError(`${name} must be a Uint8Array`)
    return value.slice()
  }

  function validDigest(value, name) {
    if (typeof value !== "string" || !DIGEST_PATTERN.test(value)) throw new TypeError(`${name} must be a lowercase SHA-256 digest`)
    return value
  }

  function validPublicationKey(value) {
    if (typeof value !== "string" || !/^[a-z0-9][a-z0-9._-]{0,63}$/.test(value)) throw new TypeError("publication key is invalid")
    return value
  }

  function validRevision(value) {
    if (typeof value !== "string" || value.length === 0 || value.length > 256 || value !== value.trim() || /[\u0000-\u001f\u007f]/.test(value)) throw new TypeError("revision id is invalid")
    return value
  }

  function nowISO(now) {
    const value = typeof now === "function" ? now() : new Date()
    return value instanceof Date ? value.toISOString() : String(value)
  }

  function key(publicationKey, revisionId) {
    return `${validPublicationKey(publicationKey)}\u0000${validRevision(revisionId)}`
  }

  function cloneGeneration(value) {
    if (!value || typeof value !== "object") throw new TypeError("generation is required")
    return {
      publicationKey: validPublicationKey(value.publicationKey),
      revisionId: validRevision(value.revisionId),
      projectionDigest: validDigest(value.projectionDigest, "projection digest"),
      snapshotId: typeof value.snapshotId === "string" ? value.snapshotId : "",
      projectionBytes: cloneBytes(value.projectionBytes, "projection bytes"),
      manifestBytes: value.manifestBytes instanceof Uint8Array ? value.manifestBytes.slice() : undefined,
      etag: typeof value.etag === "string" ? value.etag : "",
      lastObservedAt: typeof value.lastObservedAt === "string" ? value.lastObservedAt : "",
      createdAt: typeof value.createdAt === "string" ? value.createdAt : "",
      shellURL: typeof value.shellURL === "string" ? value.shellURL : "",
      assetURLs: Array.isArray(value.assetURLs) ? value.assetURLs.filter((url) => typeof url === "string").slice() : [],
    }
  }

  function cloneMetadata(value) {
    if (!value) return undefined
    return {
      schemaVersion: value.schemaVersion,
      publicationKey: value.publicationKey,
      catalogId: value.catalogId || "",
      publicationBase: value.publicationBase || "",
      snapshotId: value.snapshotId || "",
      revisionId: value.revisionId || "",
      projectionFormat: value.projectionFormat || "",
      projectionDigest: value.projectionDigest || "",
      projectionManifestUrl: value.projectionManifestUrl || "",
      catalogUrl: value.catalogUrl || "",
      searchDataBase: value.searchDataBase || "",
      projectionDataBase: value.projectionDataBase || "",
      fallbackAssets: Array.isArray(value.fallbackAssets) ? value.fallbackAssets.filter((asset) => asset && typeof asset.url === "string").map((asset) => ({ ...asset })) : [],
      offlineShellUrl: value.offlineShellUrl || value.shellURL || "",
      shellURL: value.shellURL || value.offlineShellUrl || "",
      assetURLs: Array.isArray(value.assetURLs) ? value.assetURLs.filter((url) => typeof url === "string").slice() : [],
      activeRevision: value.activeRevision || "",
      previousRevision: value.previousRevision || "",
      candidateRevision: value.candidateRevision || "",
      lastObservedAt: value.lastObservedAt || "",
      etag: value.etag || "",
      lastUsedAt: value.lastUsedAt || "",
      disabled: value.disabled === true,
      tombstone: value.tombstone ? {
        state: value.tombstone.state || "revoked",
        reason: value.tombstone.reason || "",
        observedAt: value.tombstone.observedAt || "",
        revisionId: value.tombstone.revisionId || "",
      } : undefined,
    }
  }

  function emptyMetadata(publicationKey, now) {
    return {
      schemaVersion: SCHEMA_VERSION,
      publicationKey: validPublicationKey(publicationKey),
      catalogId: "",
      publicationBase: "",
      snapshotId: "",
      revisionId: "",
      projectionFormat: "",
      projectionDigest: "",
      projectionManifestUrl: "",
      catalogUrl: "",
      searchDataBase: "",
      projectionDataBase: "",
      fallbackAssets: [],
      offlineShellUrl: "",
      shellURL: "",
      assetURLs: [],
      activeRevision: "",
      previousRevision: "",
      candidateRevision: "",
      lastObservedAt: "",
      etag: "",
      lastUsedAt: nowISO(now),
      disabled: false,
    }
  }

  function rememberDescriptor(state, value) {
    if (!state || !value || typeof value !== "object") return
    for (const field of ["catalogId", "publicationBase", "snapshotId", "revisionId", "projectionFormat", "projectionDigest", "projectionManifestUrl", "catalogUrl", "searchDataBase", "projectionDataBase"]) {
      if (typeof value[field] === "string" && value[field] !== "") state[field] = value[field]
    }
    if (Array.isArray(value.fallbackAssets)) state.fallbackAssets = value.fallbackAssets.filter((asset) => asset && typeof asset.url === "string").map((asset) => ({ ...asset }))
    const shellURL = value.offlineShellUrl || value.shellURL
    if (typeof shellURL === "string" && shellURL !== "") state.offlineShellUrl = state.shellURL = shellURL
  }

  function cloneResponse(response) {
    return response && typeof response.clone === "function" ? response.clone() : response
  }

  function createMemoryStorage(options = {}) {
    const metadata = new Map()
    const generations = new Map()
    const shells = new Map()
    const assets = new Map()
    const hooks = {}
    let recreationAttempted = false

    const getMetadata = (publicationKey) => metadata.get(validPublicationKey(publicationKey))
    const ensureMetadata = (publicationKey) => {
      const normalized = validPublicationKey(publicationKey)
      let state = metadata.get(normalized)
      if (!state) {
        state = emptyMetadata(normalized, options.now)
        metadata.set(normalized, state)
      }
      return state
    }
    const storedGeneration = (publicationKey, revisionId) => {
      const value = generations.get(key(publicationKey, revisionId))
      return value ? cloneGeneration(value) : undefined
    }
    const purge = async (publicationKey) => {
      const normalized = validPublicationKey(publicationKey)
      if (typeof hooks.beforePurge === "function") await hooks.beforePurge(normalized)
      if (typeof hooks.purge === "function") await hooks.purge(normalized)
      for (const value of [...generations.keys()]) if (value.startsWith(`${normalized}\u0000`)) generations.delete(value)
      for (const value of [...shells.keys()]) if (value.startsWith(`${normalized}\u0000`)) shells.delete(value)
      for (const value of [...assets.keys()]) if (value.startsWith(`${normalized}\u0000`)) assets.delete(value)
    }
    const prune = (publicationKey, state) => {
      const normalized = validPublicationKey(publicationKey)
      const keep = new Set([state.activeRevision, state.previousRevision, state.candidateRevision].filter(Boolean).map((value) => key(normalized, value)))
      for (const value of [...generations.keys()]) if (value.startsWith(`${normalized}\u0000`) && !keep.has(value)) generations.delete(value)
    }
    const evict = async () => {
      const candidates = [...metadata.values()].filter((value) => !value.disabled).sort((left, right) => String(right.lastUsedAt).localeCompare(String(left.lastUsedAt)))
      for (const value of candidates.slice(MAX_PUBLICATIONS)) {
        await purge(value.publicationKey).catch(() => {})
        metadata.delete(value.publicationKey)
      }
    }
    const storage = {
      async loadMetadata(publicationKey) { return cloneMetadata(getMetadata(publicationKey)) },
      async listMetadata() { return [...metadata.values()].map(cloneMetadata) },
      async loadGeneration(publicationKey, revisionId) { return storedGeneration(publicationKey, revisionId) },
      async loadActive(publicationKey) {
        const state = getMetadata(publicationKey)
        return state && !state.disabled && state.activeRevision ? storedGeneration(publicationKey, state.activeRevision) : undefined
      },
      async loadPrevious(publicationKey) {
        const state = getMetadata(publicationKey)
        return state && !state.disabled && state.previousRevision ? storedGeneration(publicationKey, state.previousRevision) : undefined
      },
      async loadCandidate(publicationKey) {
        const state = getMetadata(publicationKey)
        return state && state.candidateRevision ? storedGeneration(publicationKey, state.candidateRevision) : undefined
      },
      async commitGeneration(descriptor, candidate) {
        const publicationKey = validPublicationKey(descriptor.publicationKey)
        const revisionId = validRevision(candidate.revisionId || descriptor.revisionId)
        const state = ensureMetadata(publicationKey)
        if (state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
        rememberDescriptor(state, descriptor)
        rememberDescriptor(state, candidate)
        const record = cloneGeneration({
          ...candidate,
          publicationKey,
          revisionId,
          projectionDigest: candidate.projectionDigest || descriptor.projectionDigest,
          snapshotId: candidate.snapshotId || descriptor.snapshotId,
          shellURL: candidate.shellURL || descriptor.offlineShellUrl || "",
          createdAt: candidate.createdAt || nowISO(options.now),
        })
        generations.set(key(publicationKey, revisionId), record)
        state.candidateRevision = revisionId
        state.lastUsedAt = nowISO(options.now)
        metadata.set(publicationKey, state)
        return cloneGeneration(record)
      },
      async activate(publicationKey, revisionId) {
        const normalized = validPublicationKey(publicationKey)
        const state = ensureMetadata(normalized)
        const candidate = storedGeneration(normalized, revisionId)
        if (!candidate) throw new Error("candidate generation is unavailable")
        if (state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
        const token = {
          metadata: cloneMetadata(state),
          active: state.activeRevision ? storedGeneration(normalized, state.activeRevision) : undefined,
          previous: state.previousRevision ? storedGeneration(normalized, state.previousRevision) : undefined,
        }
        state.previousRevision = state.activeRevision && state.activeRevision !== revisionId ? state.activeRevision : state.previousRevision
        state.activeRevision = revisionId
        state.candidateRevision = ""
        state.disabled = false
        delete state.tombstone
        state.lastUsedAt = nowISO(options.now)
        state.lastObservedAt = candidate.lastObservedAt || state.lastObservedAt
        state.etag = candidate.etag || state.etag
        metadata.set(normalized, state)
        prune(normalized, state)
        await evict()
        return token
      },
      async rollback(publicationKey, token) {
        const normalized = validPublicationKey(publicationKey)
        if (!token || !token.metadata) throw new Error("rollback token is invalid")
        const state = cloneMetadata(token.metadata)
        metadata.set(normalized, state)
        if (token.active) generations.set(key(normalized, token.active.revisionId), cloneGeneration(token.active))
        if (token.previous) generations.set(key(normalized, token.previous.revisionId), cloneGeneration(token.previous))
        prune(normalized, state)
        return cloneMetadata(state)
      },
      async discardCandidate(publicationKey, revisionId) {
        const normalized = validPublicationKey(publicationKey)
        const state = getMetadata(normalized)
        if (state && state.candidateRevision === revisionId) state.candidateRevision = ""
        if (state) metadata.set(normalized, state)
        generations.delete(key(normalized, revisionId))
      },
      async observe(publicationKey, value = {}) {
        const state = ensureMetadata(publicationKey)
        rememberDescriptor(state, value)
        state.lastObservedAt = typeof value.lastObservedAt === "string" ? value.lastObservedAt : nowISO(options.now)
        if (typeof value.etag === "string") state.etag = value.etag
        state.lastUsedAt = nowISO(options.now)
        metadata.set(state.publicationKey, state)
        return cloneMetadata(state)
      },
      async tombstone(publicationKey, reason = "revoked", stateName = "revoked") {
        const normalized = validPublicationKey(publicationKey)
        const state = ensureMetadata(normalized)
        const revisionId = state.activeRevision || state.candidateRevision || state.revisionId || ""
        state.disabled = true
        state.activeRevision = state.previousRevision = state.candidateRevision = ""
        state.tombstone = { state: stateName, reason: String(reason).slice(0, 128), observedAt: nowISO(options.now), revisionId }
        metadata.set(normalized, state)
        await purge(normalized).catch(() => {})
        return cloneMetadata(state)
      },
      async putShell(publicationKey, url, response) {
        const state = getMetadata(publicationKey)
        if (!state || state.disabled) throw new Error("publication is disabled")
        shells.set(`${validPublicationKey(publicationKey)}\u0000${String(url)}`, cloneResponse(response))
        state.shellURL = state.offlineShellUrl = String(url)
        metadata.set(state.publicationKey, state)
      },
      async getShell(publicationKey, url) { return cloneResponse(shells.get(`${validPublicationKey(publicationKey)}\u0000${String(url)}`)) },
      async putAsset(publicationKey, url, response) {
        const state = getMetadata(publicationKey)
        if (!state || state.disabled) throw new Error("publication is disabled")
        const normalized = validPublicationKey(publicationKey)
        const assetURL = String(url)
        assets.set(`${normalized}\u0000${assetURL}`, cloneResponse(response))
        if (!state.assetURLs.includes(assetURL)) state.assetURLs.push(assetURL)
        if (state.assetURLs.length > 256) state.assetURLs = state.assetURLs.slice(-256)
        metadata.set(normalized, state)
      },
      async getAsset(publicationKey, url) { return cloneResponse(assets.get(`${validPublicationKey(publicationKey)}\u0000${String(url)}`)) },
      async isDisabled(publicationKey) { const state = getMetadata(publicationKey); return Boolean(state && state.disabled) },
      async isReenableAllowed(publicationKey, revisionId) {
        const state = getMetadata(publicationKey)
        return !state || !state.disabled || !state.tombstone || state.tombstone.revisionId !== revisionId
      },
      async recreateFromPointer(publicationKey, value) {
        if (recreationAttempted) throw new Error("database recreation already attempted")
        recreationAttempted = true
        metadata.clear(); generations.clear()
        return storage.commitGeneration(value, value)
      },
      setHooks(value) { Object.assign(hooks, value || {}) },
      snapshot() { return { metadata: [...metadata.values()].map(cloneMetadata), generations: [...generations.values()].map(cloneGeneration) } },
      async evict() { await evict() },
    }
    return storage
  }

  function requestPromise(request) {
    return new Promise((resolve, reject) => {
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error || new Error("IndexedDB request failed"))
    })
  }

  function openDatabase(scope) {
    if (!scope || !scope.indexedDB || typeof scope.indexedDB.open !== "function") return Promise.reject(new Error("IndexedDB is unavailable"))
    return new Promise((resolve, reject) => {
      const request = scope.indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
      request.onupgradeneeded = () => {
        const database = request.result
        if (!database.objectStoreNames.contains(METADATA_STORE)) database.createObjectStore(METADATA_STORE, { keyPath: "publicationKey" })
        if (!database.objectStoreNames.contains(GENERATION_STORE)) database.createObjectStore(GENERATION_STORE, { keyPath: ["publicationKey", "revisionId"] })
      }
      request.onblocked = () => reject(new Error("open IndexedDB local-docs state was blocked"))
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error || new Error("open IndexedDB local-docs state failed"))
    })
  }

  function serialiseGeneration(value) {
    const record = cloneGeneration(value)
    record.projectionBytes = record.projectionBytes.buffer.slice(record.projectionBytes.byteOffset, record.projectionBytes.byteOffset + record.projectionBytes.byteLength)
    if (record.manifestBytes) record.manifestBytes = record.manifestBytes.buffer.slice(record.manifestBytes.byteOffset, record.manifestBytes.byteOffset + record.manifestBytes.byteLength)
    return record
  }

  function deserialiseGeneration(value) {
    if (!value) return undefined
    return cloneGeneration({
      ...value,
      projectionBytes: value.projectionBytes instanceof ArrayBuffer ? new Uint8Array(value.projectionBytes) : value.projectionBytes,
      manifestBytes: value.manifestBytes instanceof ArrayBuffer ? new Uint8Array(value.manifestBytes) : value.manifestBytes,
    })
  }

  function createBrowserStorage(scope, options = {}) {
    if (!scope || !scope.indexedDB || !scope.caches) throw new Error("IndexedDB and CacheStorage are required")
    let databasePromise
    let recreationAttempted = false
    const now = () => nowISO(options.now)
    const db = () => { if (!databasePromise) databasePromise = openDatabase(scope); return databasePromise }
    async function read(storeName, storeKey) {
      const database = await db()
      return requestPromise(database.transaction(storeName, "readonly").objectStore(storeName).get(storeKey))
    }
    async function write(storeNames, operation) {
      const database = await db()
      return new Promise((resolve, reject) => {
        const transaction = database.transaction(storeNames, "readwrite")
        let result
        transaction.oncomplete = () => resolve(result)
        transaction.onerror = () => reject(transaction.error || new Error("IndexedDB transaction failed"))
        transaction.onabort = () => reject(transaction.error || new Error("IndexedDB transaction aborted"))
        try { result = operation(transaction) } catch (error) { transaction.abort(); reject(error) }
      })
    }
    async function cached(url, response) {
      const cache = await scope.caches.open(CACHE_NAME)
      if (response === undefined) return cache.match(String(url))
      await cache.put(String(url), cloneResponse(response))
    }
    const storage = {
      async loadMetadata(publicationKey) { return cloneMetadata(await read(METADATA_STORE, validPublicationKey(publicationKey))) },
      async listMetadata() {
        const database = await db()
        return new Promise((resolve, reject) => {
          const request = database.transaction(METADATA_STORE, "readonly").objectStore(METADATA_STORE).getAll()
          request.onsuccess = () => resolve((request.result || []).map(cloneMetadata))
          request.onerror = () => reject(request.error || new Error("IndexedDB metadata read failed"))
        })
      },
      async loadGeneration(publicationKey, revisionId) { return deserialiseGeneration(await read(GENERATION_STORE, [validPublicationKey(publicationKey), validRevision(revisionId)])) },
      async loadActive(publicationKey) { const state = await storage.loadMetadata(publicationKey); return state && !state.disabled && state.activeRevision ? storage.loadGeneration(publicationKey, state.activeRevision) : undefined },
      async loadPrevious(publicationKey) { const state = await storage.loadMetadata(publicationKey); return state && !state.disabled && state.previousRevision ? storage.loadGeneration(publicationKey, state.previousRevision) : undefined },
      async loadCandidate(publicationKey) { const state = await storage.loadMetadata(publicationKey); return state && state.candidateRevision ? storage.loadGeneration(publicationKey, state.candidateRevision) : undefined },
      async commitGeneration(descriptor, candidate) {
        const publicationKey = validPublicationKey(descriptor.publicationKey)
        const revisionId = validRevision(candidate.revisionId || descriptor.revisionId)
        const state = (await storage.loadMetadata(publicationKey)) || emptyMetadata(publicationKey, options.now)
        if (state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
        rememberDescriptor(state, descriptor)
        rememberDescriptor(state, candidate)
        const record = cloneGeneration({ ...candidate, publicationKey, revisionId, projectionDigest: candidate.projectionDigest || descriptor.projectionDigest, snapshotId: candidate.snapshotId || descriptor.snapshotId, shellURL: candidate.shellURL || descriptor.offlineShellUrl || "", createdAt: candidate.createdAt || now() })
        state.candidateRevision = revisionId
        state.lastUsedAt = now()
        await write([METADATA_STORE, GENERATION_STORE], (transaction) => {
          transaction.objectStore(GENERATION_STORE).put(serialiseGeneration(record))
          transaction.objectStore(METADATA_STORE).put(state)
        })
        return cloneGeneration(record)
      },
      async activate(publicationKey, revisionId) {
        const normalized = validPublicationKey(publicationKey)
        const state = await storage.loadMetadata(normalized)
        const candidate = await storage.loadGeneration(normalized, revisionId)
        if (!state || !candidate) throw new Error("candidate generation is unavailable")
        if (state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
        const token = { metadata: state, active: state.activeRevision ? await storage.loadGeneration(normalized, state.activeRevision) : undefined, previous: state.previousRevision ? await storage.loadGeneration(normalized, state.previousRevision) : undefined }
        state.previousRevision = state.activeRevision && state.activeRevision !== revisionId ? state.activeRevision : state.previousRevision
        state.activeRevision = revisionId
        state.candidateRevision = ""
        state.disabled = false
        delete state.tombstone
        state.lastObservedAt = candidate.lastObservedAt || state.lastObservedAt
        state.etag = candidate.etag || state.etag
        state.lastUsedAt = now()
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        await storage.evict()
        return token
      },
      async rollback(publicationKey, token) {
        if (!token || !token.metadata) throw new Error("rollback token is invalid")
        await write([METADATA_STORE, GENERATION_STORE], (transaction) => {
          transaction.objectStore(METADATA_STORE).put(token.metadata)
          if (token.active) transaction.objectStore(GENERATION_STORE).put(serialiseGeneration(token.active))
          if (token.previous) transaction.objectStore(GENERATION_STORE).put(serialiseGeneration(token.previous))
        })
        return cloneMetadata(token.metadata)
      },
      async discardCandidate(publicationKey, revisionId) {
        const normalized = validPublicationKey(publicationKey)
        const state = await storage.loadMetadata(normalized)
        if (!state) return
        if (state.candidateRevision === revisionId) state.candidateRevision = ""
        await write([METADATA_STORE, GENERATION_STORE], (transaction) => {
          transaction.objectStore(METADATA_STORE).put(state)
          transaction.objectStore(GENERATION_STORE).delete([normalized, validRevision(revisionId)])
        })
      },
      async observe(publicationKey, value = {}) {
        const state = (await storage.loadMetadata(publicationKey)) || emptyMetadata(publicationKey, options.now)
        rememberDescriptor(state, value)
        state.lastObservedAt = typeof value.lastObservedAt === "string" ? value.lastObservedAt : now()
        if (typeof value.etag === "string") state.etag = value.etag
        state.lastUsedAt = now()
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        return cloneMetadata(state)
      },
      async tombstone(publicationKey, reason = "revoked", stateName = "revoked") {
        const normalized = validPublicationKey(publicationKey)
        const state = (await storage.loadMetadata(normalized)) || emptyMetadata(normalized, options.now)
        const revisionId = state.activeRevision || state.candidateRevision || state.revisionId || ""
        state.disabled = true
        state.activeRevision = state.previousRevision = state.candidateRevision = ""
        state.tombstone = { state: stateName, reason: String(reason).slice(0, 128), observedAt: now(), revisionId }
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        const cache = await scope.caches.open(CACHE_NAME)
        await Promise.all([state.shellURL, ...(state.assetURLs || [])].filter(Boolean).map((url) => cache.delete(url).catch(() => false)))
        const database = await db()
        await new Promise((resolve) => {
          const transaction = database.transaction(GENERATION_STORE, "readwrite")
          const request = transaction.objectStore(GENERATION_STORE).openCursor()
          request.onsuccess = () => { const cursor = request.result; if (!cursor) return; if (cursor.value.publicationKey === normalized) cursor.delete(); cursor.continue() }
          transaction.oncomplete = resolve
          transaction.onerror = resolve
        })
        return cloneMetadata(state)
      },
      async putShell(publicationKey, url, response) {
        const state = await storage.loadMetadata(publicationKey)
        if (!state || state.disabled) throw new Error("publication is disabled")
        state.shellURL = state.offlineShellUrl = String(url)
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        await cached(url, response)
      },
      async getShell(_publicationKey, url) { return cached(url) },
      async putAsset(publicationKey, url, response) {
        const state = await storage.loadMetadata(publicationKey)
        if (!state || state.disabled) throw new Error("publication is disabled")
        const assetURL = String(url)
        state.assetURLs = Array.isArray(state.assetURLs) ? state.assetURLs : []
        if (!state.assetURLs.includes(assetURL)) state.assetURLs.push(assetURL)
        if (state.assetURLs.length > 256) state.assetURLs = state.assetURLs.slice(-256)
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        await cached(assetURL, response)
      },
      async getAsset(_publicationKey, url) { return cached(url) },
      async isDisabled(publicationKey) { const state = await storage.loadMetadata(publicationKey); return Boolean(state && state.disabled) },
      async isReenableAllowed(publicationKey, revisionId) { const state = await storage.loadMetadata(publicationKey); return !state || !state.disabled || !state.tombstone || state.tombstone.revisionId !== revisionId },
      async recreateFromPointer(publicationKey, value) {
        if (recreationAttempted) throw new Error("database recreation already attempted")
        recreationAttempted = true
        databasePromise = undefined
        await requestPromise(scope.indexedDB.deleteDatabase(DATABASE_NAME))
        return storage.commitGeneration(value, value)
      },
      async evict() {
        const values = await storage.listMetadata()
        const candidates = values.filter((value) => !value.disabled).sort((left, right) => String(right.lastUsedAt).localeCompare(String(left.lastUsedAt)))
        for (const value of candidates.slice(MAX_PUBLICATIONS)) await storage.tombstone(value.publicationKey, "lru eviction", "evicted").catch(() => {})
      },
    }
    return storage
  }

  function createStorage(scope, options = {}) {
    return options.memory ? createMemoryStorage(options) : createBrowserStorage(scope, options)
  }

  return {
    CACHE_NAME,
    DATABASE_NAME,
    DATABASE_VERSION,
    GENERATION_STORE,
    MAX_GENERATIONS,
    MAX_PUBLICATIONS,
    METADATA_STORE,
    SCHEMA_VERSION,
    createBrowserStorage,
    createMemoryStorage,
    createStorage,
    cloneGeneration,
    cloneMetadata,
  }
}))
