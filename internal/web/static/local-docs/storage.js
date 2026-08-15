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
  const DIGEST_PATTERN = /^[0-9a-f]{64}$/
  const SCHEMA_VERSION = 1

  function cloneBytes(value, name) {
    if (!(value instanceof Uint8Array)) throw new TypeError(`${name} must be a Uint8Array`)
    return value.slice()
  }

  function digest(value, name) {
    if (typeof value !== "string" || !DIGEST_PATTERN.test(value)) throw new TypeError(`${name} must be a lowercase SHA-256 digest`)
    return value
  }

  function publicationKey(value) {
    if (typeof value !== "string" || !/^[a-z0-9][a-z0-9._-]{0,63}$/.test(value)) throw new TypeError("publication key is invalid")
    return value
  }

  function revision(value) {
    if (typeof value !== "string" || value.length === 0 || value.length > 256 || /[\u0000-\u001f\u007f]/.test(value) || value !== value.trim()) throw new TypeError("revision id is invalid")
    return value
  }

  function cloneGeneration(value) {
    if (!value || typeof value !== "object") throw new TypeError("generation is required")
    const result = {
      publicationKey: publicationKey(value.publicationKey),
      revisionId: revision(value.revisionId),
      projectionDigest: digest(value.projectionDigest, "projection digest"),
      snapshotId: typeof value.snapshotId === "string" ? value.snapshotId : "",
      projectionBytes: cloneBytes(value.projectionBytes, "projection bytes"),
      manifestBytes: value.manifestBytes instanceof Uint8Array ? value.manifestBytes.slice() : undefined,
      etag: typeof value.etag === "string" ? value.etag : "",
      lastObservedAt: typeof value.lastObservedAt === "string" ? value.lastObservedAt : "",
      createdAt: typeof value.createdAt === "string" ? value.createdAt : "",
      shellURL: typeof value.shellURL === "string" ? value.shellURL : "",
      assetURLs: Array.isArray(value.assetURLs) ? value.assetURLs.filter((url) => typeof url === "string").slice() : [],
    }
    return result
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
      offlineShellUrl: value.offlineShellUrl || value.shellURL || "",
      shellURL: value.shellURL || value.offlineShellUrl || "",
      assetURLs: Array.isArray(value.assetURLs) ? value.assetURLs.filter((url) => typeof url === "string").slice() : [],
      activeRevision: value.activeRevision || "",
      previousRevision: value.previousRevision || "",
      candidateRevision: value.candidateRevision || "",
      reenableRevision: value.reenableRevision || "",
      lastObservedAt: value.lastObservedAt || "",
      etag: value.etag || "",
      lastUsedAt: value.lastUsedAt || "",
      disabled: value.disabled === true,
      tombstone: value.tombstone ? { state: value.tombstone.state || "revoked", reason: value.tombstone.reason || "", observedAt: value.tombstone.observedAt || "", revisionId: value.tombstone.revisionId || "" } : undefined,
    }
  }

  function nowISO(now) {
    const value = typeof now === "function" ? now() : new Date()
    return value instanceof Date ? value.toISOString() : String(value)
  }

  function generationKey(key, revisionId) {
    return `${publicationKey(key)}\u0000${revision(revisionId)}`
  }

  function emptyMetadata(key, now) {
    return {
      schemaVersion: SCHEMA_VERSION,
      publicationKey: publicationKey(key),
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
      offlineShellUrl: "",
      shellURL: "",
      assetURLs: [],
      activeRevision: "",
      previousRevision: "",
      candidateRevision: "",
      reenableRevision: "",
      lastObservedAt: "",
      etag: "",
      lastUsedAt: nowISO(now),
      disabled: false,
    }
  }

  function rememberDescriptor(state, value) {
    if (!state || !value || typeof value !== "object") return state
    const fields = ["catalogId", "publicationBase", "snapshotId", "revisionId", "projectionFormat", "projectionDigest", "projectionManifestUrl", "catalogUrl", "searchDataBase", "projectionDataBase"]
    for (const field of fields) if (typeof value[field] === "string" && value[field] !== "") state[field] = value[field]
    const shellURL = value.offlineShellUrl || value.shellURL
    if (typeof shellURL === "string" && shellURL !== "") {
      state.offlineShellUrl = shellURL
      state.shellURL = shellURL
    }
    return state
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

    function getMetadata(key) {
      return metadata.get(publicationKey(key))
    }
    function ensureMetadata(key) {
      const normalized = publicationKey(key)
      let value = metadata.get(normalized)
      if (!value) {
        value = emptyMetadata(normalized, options.now)
        metadata.set(normalized, value)
      }
      return value
    }
    function storedGeneration(key, revisionId) {
      const value = generations.get(generationKey(key, revisionId))
      if (!value) return undefined
      return cloneGeneration(value)
    }
    function pruneGenerations(key, state) {
      const keep = new Set([state.activeRevision, state.previousRevision, state.candidateRevision].filter(Boolean).map((revisionId) => generationKey(key, revisionId)))
      for (const keyValue of [...generations.keys()]) if (keyValue.startsWith(`${publicationKey(key)}\u0000`) && !keep.has(keyValue)) generations.delete(keyValue)
    }
    function activeOrPrevious(key, which) {
      const state = getMetadata(key)
      if (!state || state.disabled) return undefined
      const revisionId = which === "previous" ? state.previousRevision : state.activeRevision
      return revisionId ? storedGeneration(key, revisionId) : undefined
    }
    async function purge(key) {
      const normalized = publicationKey(key)
      if (typeof hooks.beforePurge === "function") await hooks.beforePurge(normalized)
      if (typeof hooks.purge === "function") {
        await hooks.purge(normalized)
      }
      for (const keyValue of [...generations.keys()]) if (keyValue.startsWith(`${normalized}\u0000`)) generations.delete(keyValue)
      for (const keyValue of [...shells.keys()]) if (keyValue.startsWith(`${normalized}\u0000`)) shells.delete(keyValue)
      for (const keyValue of [...assets.keys()]) if (keyValue.startsWith(`${normalized}\u0000`)) assets.delete(keyValue)
    }
    async function evict() {
      const candidates = [...metadata.values()].filter((value) => !value.disabled).sort((left, right) => String(right.lastUsedAt).localeCompare(String(left.lastUsedAt)))
      for (const value of candidates.slice(MAX_PUBLICATIONS)) {
        await purge(value.publicationKey).catch(() => {})
        metadata.delete(value.publicationKey)
      }
    }
    function assertWritable(key, revisionId) {
      const state = getMetadata(key)
      if (!state || state.disabled && state.reenableRevision !== revisionId) throw new Error("publication is disabled")
      return state
    }
    const storage = {
      async loadMetadata(key) { const value = getMetadata(key); return cloneMetadata(value) },
      async listMetadata() { return [...metadata.values()].map(cloneMetadata) },
      async loadActive(key) { const state = activeOrPrevious(key, "active"); return state },
      async loadPrevious(key) { return activeOrPrevious(key, "previous") },
      async loadCandidate(key) { const state = getMetadata(key); return state && state.candidateRevision ? storedGeneration(key, state.candidateRevision) : undefined },
      async commitGeneration(value, candidate) {
        const key = publicationKey(value.publicationKey)
        const revisionId = revision(candidate.revisionId || value.revisionId)
        const state = ensureMetadata(key)
        if (state.disabled && state.reenableRevision !== revisionId) throw new Error("publication is disabled")
        rememberDescriptor(state, value)
        rememberDescriptor(state, candidate)
        const record = cloneGeneration({
          ...candidate, publicationKey: key, revisionId,
          projectionDigest: candidate.projectionDigest || value.projectionDigest,
          snapshotId: candidate.snapshotId || value.snapshotId,
          shellURL: candidate.shellURL || value.offlineShellUrl || "",
          createdAt: candidate.createdAt || nowISO(options.now),
        })
        generations.set(generationKey(key, revisionId), record)
        state.candidateRevision = revisionId
        state.lastUsedAt = nowISO(options.now)
        metadata.set(key, state)
        return cloneGeneration(record)
      },
      async activate(key, revisionId) {
        const normalized = publicationKey(key)
        const state = ensureMetadata(normalized)
        const candidate = storedGeneration(normalized, revisionId)
        if (!candidate) throw new Error("candidate generation is unavailable")
        if (state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
        const token = { metadata: cloneMetadata(state), active: state.activeRevision ? storedGeneration(normalized, state.activeRevision) : undefined, previous: state.previousRevision ? storedGeneration(normalized, state.previousRevision) : undefined }
        state.previousRevision = state.activeRevision && state.activeRevision !== revisionId ? state.activeRevision : state.previousRevision
        state.activeRevision = revisionId
        state.candidateRevision = ""
        state.reenableRevision = ""
        state.disabled = false
        state.tombstone = undefined
        state.lastUsedAt = nowISO(options.now)
        state.lastObservedAt = candidate.lastObservedAt || state.lastObservedAt
        state.etag = candidate.etag || state.etag
        metadata.set(normalized, state)
        pruneGenerations(normalized, state)
        await evict()
        return token
      },
      async rollback(key, token) {
        const normalized = publicationKey(key)
        if (!token || !token.metadata) throw new Error("rollback token is invalid")
        const state = token.metadata
        metadata.set(normalized, { ...state, tombstone: state.tombstone && { ...state.tombstone } })
        if (token.active) generations.set(generationKey(normalized, token.active.revisionId), cloneGeneration(token.active))
        if (token.previous) generations.set(generationKey(normalized, token.previous.revisionId), cloneGeneration(token.previous))
        return cloneMetadata(state)
      },
      async discardCandidate(key, revisionId) {
        const normalized = publicationKey(key)
        const state = getMetadata(normalized)
        if (state && state.candidateRevision === revisionId) {
          state.candidateRevision = ""
          metadata.set(normalized, state)
        }
        generations.delete(generationKey(normalized, revisionId))
      },
      async observe(key, value = {}) {
        const state = ensureMetadata(key)
        rememberDescriptor(state, value)
        state.lastObservedAt = typeof value.lastObservedAt === "string" ? value.lastObservedAt : nowISO(options.now)
        if (typeof value.etag === "string") state.etag = value.etag
        state.lastUsedAt = nowISO(options.now)
        metadata.set(state.publicationKey, state)
        return cloneMetadata(state)
      },
      async allowReenable(key, revisionId) {
        const normalized = publicationKey(key)
        const state = getMetadata(normalized)
        if (!state || !state.disabled || !state.tombstone || state.tombstone.revisionId === revisionId) {
          if (state && state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
          return cloneMetadata(state)
        }
        revision(revisionId)
        state.reenableRevision = revisionId
        state.lastUsedAt = nowISO(options.now)
        metadata.set(normalized, state)
        return cloneMetadata(state)
      },
      async tombstone(key, reason = "revoked", state = "revoked") {
        const normalized = publicationKey(key)
        const value = ensureMetadata(normalized)
        const revisionId = value.activeRevision || value.candidateRevision || value.revisionId || ""
        value.disabled = true
        value.activeRevision = ""
        value.previousRevision = ""
        value.candidateRevision = ""
        value.reenableRevision = ""
        value.tombstone = { state, reason: String(reason).slice(0, 128), observedAt: nowISO(options.now), revisionId }
        metadata.set(normalized, value)
        try { await purge(normalized) } catch (_) {}
        return cloneMetadata(value)
      },
      async putShell(key, url, response) {
        assertWritable(key)
        const normalized = publicationKey(key)
        shells.set(`${normalized}\u0000${String(url)}`, cloneResponse(response))
        assertWritable(key)
        const value = ensureMetadata(normalized)
        value.shellURL = String(url)
        value.offlineShellUrl = String(url)
        metadata.set(normalized, value)
      },
      async getShell(key, url) { return cloneResponse(shells.get(`${publicationKey(key)}\u0000${String(url)}`)) },
      async putAsset(key, url, response) {
        assertWritable(key)
        const normalized = publicationKey(key)
        assets.set(`${normalized}\u0000${String(url)}`, cloneResponse(response))
        assertWritable(key)
        const value = ensureMetadata(normalized)
        if (!value.assetURLs.includes(String(url))) value.assetURLs.push(String(url))
        if (value.assetURLs.length > 256) value.assetURLs = value.assetURLs.slice(-256)
        metadata.set(normalized, value)
      },
      async getAsset(key, url) { return cloneResponse(assets.get(`${publicationKey(key)}\u0000${String(url)}`)) },
      async isDisabled(key) { const value = getMetadata(key); return Boolean(value && value.disabled) },
      async isReenableAllowed(key, revisionId) { const value = getMetadata(key); return !value || !value.disabled || !value.tombstone || value.tombstone.revisionId !== revisionId },
      async recreateFromPointer(key, value) {
        if (recreationAttempted) throw new Error("database recreation already attempted")
        recreationAttempted = true
        metadata.clear(); generations.clear()
        return storage.commitGeneration(value, value)
      },
      setHooks(value) { Object.assign(hooks, value || {}) },
      snapshot() { return { metadata: [...metadata.values()].map(cloneMetadata), generations: [...generations.values()].map(cloneGeneration) } },
      async evict() { await evict() },
    }
    return serializeMutations(storage)
  }

  function requestPromise(request) {
    return new Promise((resolve, reject) => {
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error || new Error("IndexedDB request failed"))
    })
  }

  function openDatabase(scope) {
    const indexedDB = scope && scope.indexedDB
    if (!indexedDB || typeof indexedDB.open !== "function") return Promise.reject(new Error("IndexedDB is unavailable"))
    return new Promise((resolve, reject) => {
      const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
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
    return cloneGeneration({ ...value, projectionBytes: value.projectionBytes instanceof ArrayBuffer ? new Uint8Array(value.projectionBytes) : value.projectionBytes, manifestBytes: value.manifestBytes instanceof ArrayBuffer ? new Uint8Array(value.manifestBytes) : value.manifestBytes })
  }

  function createBrowserStorage(scope, options = {}) {
    if (!scope || !scope.indexedDB || !scope.caches) throw new Error("IndexedDB and CacheStorage are required")
    let databasePromise
    let recreationAttempted = false
    const now = () => nowISO(options.now)
    const db = () => { if (!databasePromise) databasePromise = openDatabase(scope); return databasePromise }
    async function read(storeName, key) { const database = await db(); const transaction = database.transaction(storeName, "readonly"); return requestPromise(transaction.objectStore(storeName).get(key)) }
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
    async function cached(key, url, response) {
      const cache = await scope.caches.open(CACHE_NAME)
      const cacheURL = String(url)
      if (response === undefined) return cache.match(cacheURL)
      await cache.put(cacheURL, response.clone ? response.clone() : response)
    }
    async function purgeCacheFor(key) {
      const metadataValue = await storage.loadMetadata(key)
      const cache = await scope.caches.open(CACHE_NAME)
      const urls = []
      if (metadataValue && metadataValue.shellURL) urls.push(metadataValue.shellURL)
      if (metadataValue && Array.isArray(metadataValue.assetURLs)) urls.push(...metadataValue.assetURLs)
      await Promise.all(urls.map((url) => cache.delete(url).catch(() => false)))
    }
    async function pruneGenerationsFor(key, state) {
      const normalized = publicationKey(key)
      const keep = new Set([state && state.activeRevision, state && state.previousRevision, state && state.candidateRevision].filter(Boolean).map((revisionId) => `${normalized}\u0000${revisionId}`))
      const database = await db()
      await new Promise((resolve, reject) => {
        const transaction = database.transaction(GENERATION_STORE, "readwrite")
        const request = transaction.objectStore(GENERATION_STORE).openCursor()
        request.onsuccess = () => {
          const cursor = request.result
          if (!cursor) return
          const value = cursor.value
          const cursorKey = `${value.publicationKey}\u0000${value.revisionId}`
          if (value.publicationKey === normalized && !keep.has(cursorKey)) cursor.delete()
          cursor.continue()
        }
        request.onerror = () => reject(request.error || new Error("IndexedDB generation scan failed"))
        transaction.oncomplete = resolve
        transaction.onerror = () => reject(transaction.error || new Error("IndexedDB generation prune failed"))
        transaction.onabort = () => reject(transaction.error || new Error("IndexedDB generation prune aborted"))
      })
    }
    const storage = {
      async loadMetadata(key) { return cloneMetadata(await read(METADATA_STORE, publicationKey(key))) },
      async listMetadata() {
        const database = await db()
        return new Promise((resolve, reject) => {
          const transaction = database.transaction(METADATA_STORE, "readonly")
          const request = transaction.objectStore(METADATA_STORE).getAll()
          request.onsuccess = () => resolve((request.result || []).map(cloneMetadata))
          request.onerror = () => reject(request.error || new Error("IndexedDB metadata read failed"))
        })
      },
      async loadGeneration(key, revisionId) { return deserialiseGeneration(await read(GENERATION_STORE, [publicationKey(key), revision(revisionId)])) },
      async loadActive(key) { const value = await storage.loadMetadata(key); return value && !value.disabled && value.activeRevision ? storage.loadGeneration(key, value.activeRevision) : undefined },
      async loadPrevious(key) { const value = await storage.loadMetadata(key); return value && !value.disabled && value.previousRevision ? storage.loadGeneration(key, value.previousRevision) : undefined },
      async loadCandidate(key) { const value = await storage.loadMetadata(key); return value && value.candidateRevision ? storage.loadGeneration(key, value.candidateRevision) : undefined },
      async commitGeneration(value, candidate) {
        const key = publicationKey(value.publicationKey)
        const state = (await storage.loadMetadata(key)) || emptyMetadata(key, options.now)
        const revisionId = revision(candidate.revisionId || value.revisionId)
        if (state.disabled && state.reenableRevision !== revisionId) throw new Error("publication is disabled")
        rememberDescriptor(state, value)
        rememberDescriptor(state, candidate)
        const record = cloneGeneration({ ...candidate, publicationKey: key, revisionId: candidate.revisionId || value.revisionId, projectionDigest: candidate.projectionDigest || value.projectionDigest, snapshotId: candidate.snapshotId || value.snapshotId, shellURL: candidate.shellURL || value.offlineShellUrl || "", createdAt: candidate.createdAt || now() })
        state.candidateRevision = record.revisionId
        state.lastUsedAt = now()
        await write([METADATA_STORE, GENERATION_STORE], (transaction) => {
          transaction.objectStore(GENERATION_STORE).put(serialiseGeneration(record))
          transaction.objectStore(METADATA_STORE).put(state)
        })
        return record
      },
      async activate(key, revisionId) {
        const normalized = publicationKey(key)
        const state = await storage.loadMetadata(normalized)
        const candidate = await storage.loadGeneration(normalized, revisionId)
        if (!state || !candidate) throw new Error("candidate generation is unavailable")
        if (state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
        const token = { metadata: state, active: state.activeRevision ? await storage.loadGeneration(normalized, state.activeRevision) : undefined, previous: state.previousRevision ? await storage.loadGeneration(normalized, state.previousRevision) : undefined }
        state.previousRevision = state.activeRevision && state.activeRevision !== revisionId ? state.activeRevision : state.previousRevision
        state.activeRevision = revisionId
        state.candidateRevision = ""
        state.reenableRevision = ""
        state.disabled = false
        delete state.tombstone
        state.lastObservedAt = candidate.lastObservedAt || state.lastObservedAt
        state.etag = candidate.etag || state.etag
        state.lastUsedAt = now()
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        await pruneGenerationsFor(normalized, state)
        await storage.evict(normalized)
        return token
      },
      async rollback(key, token) {
        if (!token || !token.metadata) throw new Error("rollback token is invalid")
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(token.metadata) })
        await pruneGenerationsFor(key, token.metadata)
        return cloneMetadata(token.metadata)
      },
      async discardCandidate(key, revisionId) {
        const normalized = publicationKey(key)
        const state = await storage.loadMetadata(normalized)
        if (!state) return
        if (state.candidateRevision === revisionId) state.candidateRevision = ""
        await write([METADATA_STORE, GENERATION_STORE], (transaction) => {
          transaction.objectStore(METADATA_STORE).put(state)
          transaction.objectStore(GENERATION_STORE).delete([normalized, revision(revisionId)])
        })
      },
      async observe(key, value = {}) {
        const state = (await storage.loadMetadata(key)) || emptyMetadata(key, options.now)
        rememberDescriptor(state, value)
        state.lastObservedAt = value.lastObservedAt || now(); state.etag = typeof value.etag === "string" ? value.etag : state.etag; state.lastUsedAt = now()
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        return cloneMetadata(state)
      },
      async allowReenable(key, revisionId) {
        const normalized = publicationKey(key)
        const state = await storage.loadMetadata(normalized)
        if (!state || !state.disabled || !state.tombstone || state.tombstone.revisionId === revisionId) {
          if (state && state.disabled && state.tombstone && state.tombstone.revisionId === revisionId) throw new Error("publication is tombstoned")
          return state
        }
        revision(revisionId)
        state.reenableRevision = revisionId
        state.lastUsedAt = now()
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(state) })
        return cloneMetadata(state)
      },
      async tombstone(key, reason = "revoked", stateName = "revoked") {
        const normalized = publicationKey(key)
        const value = (await storage.loadMetadata(normalized)) || emptyMetadata(normalized, options.now)
        const revisionId = value.activeRevision || value.candidateRevision || value.revisionId || ""
        value.disabled = true; value.activeRevision = ""; value.previousRevision = ""; value.candidateRevision = ""; value.reenableRevision = ""; value.tombstone = { state: stateName, reason: String(reason).slice(0, 128), observedAt: now(), revisionId }
        await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(value) })
        await purgeCacheFor(normalized).catch(() => {})
        const database = await db(); await new Promise((resolve) => { const transaction = database.transaction(GENERATION_STORE, "readwrite"); const store = transaction.objectStore(GENERATION_STORE); const request = store.openCursor(); request.onsuccess = () => { const cursor = request.result; if (!cursor) return; if (cursor.value.publicationKey === normalized) cursor.delete(); cursor.continue() }; transaction.oncomplete = resolve; transaction.onerror = resolve })
        return cloneMetadata(value)
      },
      async putShell(key, url, response) {
        const value = await storage.loadMetadata(key)
        if (!value || value.disabled) throw new Error("publication is disabled")
        const shellURL = String(url)
        const copy = cloneResponse(response)
        await cached(key, shellURL, copy)
        const current = await storage.loadMetadata(key)
        if (!current || current.disabled) {
          const cache = await scope.caches.open(CACHE_NAME)
          await cache.delete(shellURL).catch(() => false)
          throw new Error("publication is disabled")
        }
        current.shellURL = shellURL; current.offlineShellUrl = shellURL
        try {
          await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(current) })
        } catch (error) {
          const cache = await scope.caches.open(CACHE_NAME)
          await cache.delete(shellURL).catch(() => false)
          throw error
        }
      },
      async getShell(key, url) { return cached(key, url) },
      async putAsset(key, url, response) {
        const value = await storage.loadMetadata(key)
        if (!value || value.disabled) throw new Error("publication is disabled")
        const assetURL = String(url)
        const copy = cloneResponse(response)
        await cached(key, assetURL, copy)
        const current = await storage.loadMetadata(key)
        if (!current || current.disabled) {
          const cache = await scope.caches.open(CACHE_NAME)
          await cache.delete(assetURL).catch(() => false)
          throw new Error("publication is disabled")
        }
        current.assetURLs = Array.isArray(current.assetURLs) ? current.assetURLs : []
        if (!current.assetURLs.includes(assetURL)) current.assetURLs.push(assetURL)
        if (current.assetURLs.length > 256) current.assetURLs = current.assetURLs.slice(-256)
        try {
          await write(METADATA_STORE, (transaction) => { transaction.objectStore(METADATA_STORE).put(current) })
        } catch (error) {
          const cache = await scope.caches.open(CACHE_NAME)
          await cache.delete(assetURL).catch(() => false)
          throw error
        }
      },
      async getAsset(key, url) { return cached(key, url) },
      async isDisabled(key) { const value = await storage.loadMetadata(key); return Boolean(value && value.disabled) },
      async isReenableAllowed(key, revisionId) { const value = await storage.loadMetadata(key); return !value || !value.disabled || !value.tombstone || value.tombstone.revisionId !== revisionId },
      async recreateFromPointer(key, value) {
        if (recreationAttempted) throw new Error("database recreation already attempted")
        recreationAttempted = true
        const indexedDB = scope.indexedDB
        databasePromise = undefined
        await requestPromise(indexedDB.deleteDatabase(DATABASE_NAME))
        return storage.commitGeneration(value, value)
      },
      async evict(protectedKey) {
        const database = await db(); const entries = await new Promise((resolve, reject) => { const transaction = database.transaction(METADATA_STORE, "readonly"); const request = transaction.objectStore(METADATA_STORE).getAll(); request.onsuccess = () => resolve(request.result || []); request.onerror = () => reject(request.error) })
        const candidates = entries.filter((entry) => !entry.disabled && entry.publicationKey !== protectedKey).sort((left, right) => String(right.lastUsedAt).localeCompare(String(left.lastUsedAt)))
        for (const entry of candidates.slice(MAX_PUBLICATIONS)) await storage.tombstone(entry.publicationKey, "lru eviction", "evicted").catch(() => {})
      },
    }
    return serializeMutations(storage)
  }

  // Service-worker events can overlap. Queue every publication mutation so a
  // withdrawal cannot race a candidate write or cache metadata update.
  function serializeMutations(storage) {
    const queues = new Map()
    const keyOf = {
      commitGeneration: (value) => value && value.publicationKey,
      activate: (key) => key,
      rollback: (key) => key,
      discardCandidate: (key) => key,
      observe: (key) => key,
      allowReenable: (key) => key,
      tombstone: (key) => key,
      putShell: (key) => key,
      putAsset: (key) => key,
    }
    for (const [name, getKey] of Object.entries(keyOf)) {
      const original = storage[name]
      if (typeof original !== "function") continue
      storage[name] = function (...args) {
        let key
        try { key = publicationKey(getKey(...args)) } catch (_) { return original.apply(this, args) }
        const previous = queues.get(key) || Promise.resolve()
        const current = previous.catch(() => {}).then(() => original.apply(this, args))
        queues.set(key, current)
        current.then(() => {
          if (queues.get(key) === current) queues.delete(key)
        }, () => {
          if (queues.get(key) === current) queues.delete(key)
        })
        return current
      }
    }
    return storage
  }

  function createStorage(scope, options = {}) {
    if (options.memory) return createMemoryStorage(options)
    return createBrowserStorage(scope, options)
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
