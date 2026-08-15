(function (root, factory) {
  "use strict"
  const api = factory(root)
  if (typeof module === "object" && module.exports) module.exports = api
  root.ManjaLocalDocsWorker = api
  if (typeof ServiceWorkerGlobalScope !== "undefined" && root instanceof ServiceWorkerGlobalScope) {
    let storageAPI = root.ManjaLocalDocsStorage
    if (!storageAPI && typeof root.importScripts === "function") {
      try { root.importScripts("/manja-assets/local-docs/storage.js"); storageAPI = root.ManjaLocalDocsStorage } catch (_) {}
    }
    if (storageAPI) api.register(root, { storageAPI })
  }
}(typeof globalThis !== "undefined" ? globalThis : this, function (global) {
  "use strict"

  const MAX_MANIFEST_BYTES = 4 * 1024 * 1024
  const MAX_SHELL_BYTES = 2 * 1024 * 1024
  const MAX_ASSET_BYTES = 16 * 1024 * 1024
  const MAX_SPEC_BYTES = 64 * 1024 * 1024
  const MAX_CHILD_BYTES = 2 * 1024 * 1024
  const DIGEST_PATTERN = /^[0-9a-f]{64}$/
  const ROOT_SCOPE = "/"
  const STATIC_PREFIX = "/manja-assets/local-docs/"
  const WITHDRAWAL_STATUS = new Set([401, 403, 404, 410])
  const PUBLIC_STATES = new Set(["private", "revoked", "deleted", "disabled"])

  function fail(message) { throw new Error(message) }

  function sameOriginURL(value, origin) {
    if (typeof value !== "string" || value.charAt(0) !== "/" || value.indexOf("\\") !== -1 || value.indexOf("%") !== -1 || value.indexOf("#") !== -1 || value.indexOf("?") !== -1) return null
    let parsed
    try { parsed = new URL(value, origin || (global.location && global.location.origin) || "https://manja-local-docs.invalid") } catch (_) { return null }
    if (parsed.origin !== (origin || parsed.origin) || parsed.pathname !== value || parsed.search !== "" || parsed.hash !== "") return null
    return parsed
  }

  function validIdentity(value, maximum = 256) {
    return typeof value === "string" && value.length > 0 && value.length <= maximum && value === value.trim() && !/[\u0000-\u001f\u007f]/.test(value)
  }

  function validPublicationKey(value) { return typeof value === "string" && /^[a-z0-9][a-z0-9._-]{0,63}$/.test(value) }
  function validDigest(value) { return typeof value === "string" && DIGEST_PATTERN.test(value) }

  function validBase(value, origin) {
    if (value === "/") return true
    if (typeof value !== "string" || value.length < 3 || value.length > 1024 || value.charAt(0) !== "/" || value.charAt(value.length - 1) !== "/" || value.indexOf("//") !== -1) return false
    if (value.indexOf("%") !== -1 || value.indexOf("?") !== -1 || value.indexOf("#") !== -1 || value.indexOf("\\") !== -1) return false
    const parsed = sameOriginURL(value, origin)
    if (!parsed) return false
    const pieces = value.split("/").filter(Boolean)
    return !pieces.includes("manage") && !pieces.includes("api") && pieces.every((piece) => piece !== "." && piece !== "..")
  }

  function expectedDescriptorRoutes(descriptor) {
    const base = descriptor.publicationBase + "snapshots/" + descriptor.snapshotId + "/"
    return {
      projectionManifestUrl: base + "manifest.json",
      catalogUrl: base + "catalog.json",
      searchDataBase: base + "search-data/",
      projectionDataBase: base + "projection-data/",
      offlineShellUrl: descriptor.offlineShellUrl || descriptor.publicationBase + "_manja/offline-shell",
    }
  }

  function validateFallbackAsset(asset, origin) {
    if (!asset || typeof asset !== "object" || !sameOriginURL(asset.url, origin)) fail("fallback asset route is not same-origin")
    if (asset.length !== undefined && (!Number.isSafeInteger(asset.length) || asset.length <= 0 || asset.length > MAX_ASSET_BYTES)) fail("fallback asset length is invalid")
    if (asset.sha256 !== undefined && !validDigest(asset.sha256)) fail("fallback asset digest is invalid")
    return { url: asset.url, length: asset.length, sha256: asset.sha256 || "", version: typeof asset.version === "string" ? asset.version : "", kind: typeof asset.kind === "string" ? asset.kind : "asset" }
  }

  function validateDescriptor(input, origin) {
    if (!input || typeof input !== "object" || input.schemaVersion !== 1 || !validIdentity(input.catalogId) || !validPublicationKey(input.publicationKey) || !validIdentity(input.revisionId) || input.projectionFormat !== "projection-v2" || !validDigest(input.projectionDigest) || input.snapshotId !== "snapshot-sha256-" + input.projectionDigest) fail("descriptor identity is invalid")
    if (!validBase(input.publicationBase, origin)) fail("descriptor publication base is invalid")
    if (input.public === false || input.anonymous === false || input.private === true || input.disabled === true || (input.eligibility && (input.eligibility.public === false || input.eligibility.anonymous === false))) fail("descriptor public eligibility is invalid")
    const allowed = new Set(["schemaVersion", "catalogId", "publicationKey", "publicationBase", "snapshotId", "revisionId", "projectionFormat", "projectionDigest", "projectionManifestUrl", "catalogUrl", "searchDataBase", "projectionDataBase", "offlineShellUrl", "public", "anonymous", "private", "disabled", "eligibility", "fallbackAssets", "fallbackManifest", "runtimeURL", "wasmURL", "workerURL"])
    if (Object.keys(input).some((key) => !allowed.has(key))) fail("descriptor field is unknown")
    const routes = expectedDescriptorRoutes(input)
    Object.keys(routes).forEach((key) => {
      if (input[key] !== undefined && !sameOriginURL(input[key], origin)) fail("descriptor route is not same-origin")
      if (!sameOriginURL(routes[key], origin)) fail("descriptor route is not same-origin")
      if (input[key] !== undefined && input[key] !== routes[key]) fail("descriptor route is invalid")
    })
    for (const key of ["runtimeURL", "wasmURL", "workerURL"]) {
      if (input[key] !== undefined && !sameOriginURL(input[key], origin)) fail("descriptor asset route is not same-origin")
    }
    if (input.fallbackManifest !== undefined && !Array.isArray(input.fallbackManifest)) fail("fallback manifest is invalid")
    if (input.fallbackAssets !== undefined && !Array.isArray(input.fallbackAssets)) fail("fallback assets are invalid")
    if (input.fallbackManifest !== undefined && input.fallbackAssets !== undefined) fail("fallback manifest is duplicated")
    const fallbackAssets = Array.isArray(input.fallbackManifest) ? input.fallbackManifest.map((asset) => validateFallbackAsset(asset, origin)) : Array.isArray(input.fallbackAssets) ? input.fallbackAssets.map((asset) => validateFallbackAsset(asset, origin)) : []
    if (fallbackAssets.length > 64) fail("fallback manifest exceeds limit")
    const result = { ...input, ...routes, fallbackAssets }
    return result
  }

  function descriptorFromMetadata(metadata, origin) {
    if (!metadata || metadata.disabled || typeof metadata.publicationBase !== "string" || metadata.publicationBase === "") return undefined
    try {
      return validateDescriptor({
        schemaVersion: 1,
        catalogId: metadata.catalogId,
        publicationKey: metadata.publicationKey,
        publicationBase: metadata.publicationBase,
        snapshotId: metadata.snapshotId,
        revisionId: metadata.revisionId || metadata.activeRevision,
        projectionFormat: metadata.projectionFormat,
        projectionDigest: metadata.projectionDigest,
        projectionManifestUrl: metadata.projectionManifestUrl,
        catalogUrl: metadata.catalogUrl,
        searchDataBase: metadata.searchDataBase,
        projectionDataBase: metadata.projectionDataBase,
        offlineShellUrl: metadata.offlineShellUrl || metadata.shellURL,
      }, origin)
    } catch (_) {
      return undefined
    }
  }

  function canonicalPath(pathname) {
    if (typeof pathname !== "string" || pathname.charAt(0) !== "/" || pathname.indexOf("\\") !== -1 || pathname.indexOf("%") !== -1) return false
    if (pathname === "/") return true
    const pieces = pathname.split("/")
    for (let index = 1; index < pieces.length; index++) {
      const piece = pieces[index]
      if (piece === "." || piece === ".." || (piece === "" && index !== pieces.length - 1)) return false
    }
    return true
  }

  function isManagementPath(pathname) {
    return pathname === "/manage" || pathname.indexOf("/manage/") === 0 || pathname === "/api" || pathname.indexOf("/api/") === 0
  }

  function isAllowedRequest(request, descriptor, origin) {
    if (!request || request.method !== "GET" || !descriptor) return false
    let url
    try { url = new URL(request.url, origin) } catch (_) { return false }
    if (url.origin !== origin || !canonicalPath(url.pathname) || isManagementPath(url.pathname)) return false
    const base = descriptor.publicationBase
    const rootPath = base === "/" ? "/" : base.slice(0, -1)
    if (!(url.pathname === rootPath || url.pathname === base || url.pathname.indexOf(base) === 0)) return false
    const routes = expectedDescriptorRoutes(descriptor)
    if (url.pathname === routes.projectionManifestUrl || url.pathname === routes.catalogUrl || url.pathname === routes.offlineShellUrl) return true
    if (url.pathname.indexOf(routes.searchDataBase) === 0 || url.pathname.indexOf(routes.projectionDataBase) === 0) {
      const childBase = url.pathname.indexOf(routes.searchDataBase) === 0 ? routes.searchDataBase : routes.projectionDataBase
      const child = url.pathname.slice(childBase.length)
      return child.length > 0 && canonicalPath(url.pathname)
    }
    if (url.pathname === rootPath || url.pathname === base) return true
    const documentsBase = base + "documents/"
    if (url.pathname.indexOf(documentsBase) === 0) {
      const child = url.pathname.slice(documentsBase.length)
      const pieces = child.split("/")
      return child.length > 0 && pieces[0] !== "" && pieces.every((piece, index) => piece !== "." && piece !== ".." && (piece !== "" || index === pieces.length - 1))
    }
    if (url.pathname === base + "search" || url.pathname === base + "search.json") return true
    const openapiBase = base + "openapi/"
    if (url.pathname.indexOf(openapiBase) === 0) {
      const child = url.pathname.slice(openapiBase.length)
      const pieces = child.split("/")
      return child.length > 0 && pieces[0] !== "" && pieces.every((piece, index) => piece !== "." && piece !== ".." && (piece !== "" || index === pieces.length - 1))
    }
    return false
  }

  function isHTMXRequest(request) {
    if (!request || !request.headers || typeof request.headers.get !== "function") return false
    return request.headers.get("HX-Request") === "true" || request.headers.get("Accept") && request.headers.get("Accept").indexOf("text/html") !== -1 && request.mode !== "navigate"
  }

  function isWithdrawalResponse(response, kind = "resource") {
    if (!response) return false
    if (WITHDRAWAL_STATUS.has(response.status)) {
      return response.status !== 404 || kind !== "document" || PUBLIC_STATES.has(String(response.headers.get("X-Manja-Publication-State") || "").toLowerCase())
    }
    const state = String(response.headers.get("X-Manja-Publication-State") || "").toLowerCase()
    return PUBLIC_STATES.has(state)
  }

  async function sha256(value, cryptoImplementation) {
    const implementation = cryptoImplementation || global.crypto
    if (!implementation || !implementation.subtle) fail("Web Crypto SHA-256 is unavailable")
    const result = await implementation.subtle.digest("SHA-256", value instanceof Uint8Array ? value : new Uint8Array(value))
    return Array.from(new Uint8Array(result), (byte) => byte.toString(16).padStart(2, "0")).join("")
  }

  async function readBoundedResponse(response, maximum = MAX_MANIFEST_BYTES) {
    if (!response) fail("response is unavailable")
    const declared = response.headers && response.headers.get("Content-Length")
    if (declared !== null && declared !== undefined && (!/^\d+$/.test(declared) || Number(declared) > maximum)) fail("response exceeds byte limit")
    if (!response.body || typeof response.body.getReader !== "function") {
      const value = new Uint8Array(await response.arrayBuffer())
      if (value.byteLength > maximum || declared !== null && declared !== undefined && Number(declared) !== value.byteLength) fail("response exceeds byte limit")
      return value
    }
    const reader = response.body.getReader()
    const chunks = []
    let total = 0
    while (true) {
      const next = await reader.read()
      if (next.done) break
      const chunk = next.value instanceof Uint8Array ? next.value : new Uint8Array(next.value)
      total += chunk.byteLength
      if (total > maximum) { await reader.cancel().catch(() => {}); fail("response exceeds byte limit") }
      chunks.push(chunk.slice())
    }
    if (declared !== null && declared !== undefined && Number(declared) !== total) fail("response length differs from Content-Length")
    const result = new Uint8Array(total)
    let offset = 0
    for (const chunk of chunks) { result.set(chunk, offset); offset += chunk.byteLength }
    return result
  }

  function parseJSONString(text) {
    let index = 0
    const whitespace = () => { while (index < text.length && /\s/.test(text[index])) index++ }
    const string = () => {
      const start = index
      if (text[index] !== '"') fail("manifest JSON is invalid")
      index++
      while (index < text.length) {
        const character = text[index++]
        if (character === "\\") { if (index >= text.length) fail("manifest JSON is invalid"); index++; continue }
        if (character === '"') return JSON.parse(text.slice(start, index))
        if (character < " ") fail("manifest JSON is invalid")
      }
      fail("manifest JSON is invalid")
    }
    const value = (depth) => {
      if (depth > 32) fail("manifest JSON is too deep")
      whitespace()
      if (text[index] === '{') {
        index++; whitespace(); const object = {}
        const keys = new Set()
        if (text[index] === '}') { index++; return object }
        while (index < text.length) {
          whitespace(); const key = string(); if (keys.has(key)) fail("manifest JSON has duplicate keys"); keys.add(key); whitespace()
          if (text[index++] !== ':') fail("manifest JSON is invalid")
          object[key] = value(depth + 1); whitespace()
          if (text[index] === '}') { index++; return object }
          if (text[index++] !== ',') fail("manifest JSON is invalid")
        }
      } else if (text[index] === '[') {
        index++; whitespace(); const array = []
        if (text[index] === ']') { index++; return array }
        while (index < text.length) {
          array.push(value(depth + 1)); whitespace()
          if (text[index] === ']') { index++; return array }
          if (text[index++] !== ',') fail("manifest JSON is invalid")
        }
      } else if (text[index] === '"') return string()
      else {
        const start = index
        while (index < text.length && !/[\s,\]}]/.test(text[index])) index++
        const token = text.slice(start, index)
        if (!/^(?:true|false|null|-?(?:0|[1-9]\d*))$/.test(token)) fail("manifest JSON is invalid")
        return JSON.parse(token)
      }
      fail("manifest JSON is invalid")
    }
    const result = value(0); whitespace(); if (index !== text.length) fail("manifest JSON has trailing data"); return result
  }

  function identityBytes(identity) {
    if (identity.versions === undefined && identity.projectionFormat !== undefined) {
      return new TextEncoder().encode(JSON.stringify({ schemaVersion: identity.schemaVersion, catalogId: identity.catalogId, revisionId: identity.revisionId, projectionFormat: identity.projectionFormat }))
    }
    // Keep this object in SnapshotIdentityV1 declaration order. Undefined
    // fields are omitted by JSON.stringify, which keeps the small browser
    // fixtures compatible while matching Go's canonical identity bytes for
    // production manifests.
    return new TextEncoder().encode(JSON.stringify({
      schemaVersion: identity.schemaVersion,
      catalogId: identity.catalogId,
      catalogTitle: identity.catalogTitle,
      branding: identity.branding,
      defaultDocumentKey: identity.defaultDocumentKey,
      profileId: identity.profileId,
      revisionKind: identity.revisionKind,
      revisionId: identity.revisionId,
      commitSha: identity.commitSha,
      sourceManifestSha256: identity.sourceManifestSha256,
      profileAllowlistLength: identity.profileAllowlistLength,
      profileAllowlistSha256: identity.profileAllowlistSha256,
      versions: identity.versions,
      bounds: identity.bounds,
      sources: identity.sources,
      children: identity.children,
    }))
  }

  function identityProjectionFormat(identity) {
    return identity && (identity.projectionFormat || identity.versions && identity.versions.projectionFormat)
  }

  async function parseManifest(bytes, descriptor, cryptoImplementation) {
    let text
    try { text = new TextDecoder("utf-8", { fatal: true }).decode(bytes) } catch (_) { fail("manifest UTF-8 is invalid") }
    const manifest = parseJSONString(text)
    if (!manifest || typeof manifest !== "object" || Array.isArray(manifest) || manifest.schemaVersion !== 1 || manifest.snapshotId !== descriptor.snapshotId || !manifest.identity || manifest.identity.schemaVersion !== 1 || manifest.identity.catalogId !== descriptor.catalogId || manifest.identity.revisionId !== descriptor.revisionId || identityProjectionFormat(manifest.identity) !== descriptor.projectionFormat || !Array.isArray(manifest.children) || manifest.children.length > 10000) fail("manifest identity is invalid")
    const allowedRoot = new Set(["schemaVersion", "snapshotId", "identity", "children"])
    if (Object.keys(manifest).some((key) => !allowedRoot.has(key))) fail("manifest field is unknown")
    const allowedIdentity = new Set(["schemaVersion", "catalogId", "catalogTitle", "branding", "defaultDocumentKey", "profileId", "revisionKind", "revisionId", "projectionFormat", "commitSha", "sourceManifestSha256", "profileAllowlistLength", "profileAllowlistSha256", "versions", "bounds", "sources", "children"])
    if (Object.keys(manifest.identity).some((key) => !allowedIdentity.has(key))) fail("manifest identity field is unknown")
    const identityDigest = await sha256(identityBytes(manifest.identity), cryptoImplementation)
    if (identityDigest !== descriptor.projectionDigest) fail("manifest identity digest differs")
    const seen = new Set()
    for (let index = 0; index < manifest.children.length; index++) {
      const child = manifest.children[index]
      if (!child || typeof child !== "object" || typeof child.path !== "string" || child.path.charAt(0) === "/" || child.path.indexOf("\\") !== -1 || child.path.indexOf("%") !== -1 || child.path.split("/").some((piece) => !piece || piece === "." || piece === "..")) fail("manifest child path is invalid")
      const allowedChild = new Set(["path", "kind", "length", "sha256"])
      if (Object.keys(child).some((key) => !allowedChild.has(key))) fail("manifest child field is unknown")
      if (Array.isArray(manifest.identity.children)) {
        if (manifest.identity.children.length !== manifest.children.length || index > 0 && manifest.children[index - 1].path >= child.path) fail("manifest children are not canonical")
        const identityChild = manifest.identity.children[index]
        if (!identityChild || typeof identityChild !== "object" || Object.keys(identityChild).some((key) => !allowedChild.has(key))) fail("manifest identity child field is unknown")
        if (!identityChild || identityChild.path !== child.path || identityChild.kind !== child.kind || identityChild.length !== child.length || identityChild.sha256 !== child.sha256) fail("manifest children differ from identity")
      }
      if (!Array.isArray(manifest.identity.children) && index > 0 && manifest.children[index - 1].path >= child.path) fail("manifest children are not canonical")
      const expectedKind = child.path.indexOf("details/") === 0 ? "detail" : child.path.indexOf("schema-nodes/") === 0 ? "schema-node" : ""
      if (expectedKind && child.kind !== expectedKind) fail("manifest projection child kind is invalid")
      if (expectedKind) {
        if (seen.has(child.path)) fail("manifest projection child is duplicated")
        seen.add(child.path)
        if (!Number.isSafeInteger(child.length) || child.length <= 0 || child.length > MAX_CHILD_BYTES || !validDigest(child.sha256)) fail("manifest projection child is invalid")
      }
    }
    if (Array.isArray(manifest.identity.children) && manifest.identity.children.length !== manifest.children.length) fail("manifest children differ from identity")
    return { manifest, identityDigest }
  }

  async function validateShell(response, maximum = MAX_SHELL_BYTES) {
    const bytes = await readBoundedResponse(response.clone ? response.clone() : response, maximum)
    const csp = String(response.headers && response.headers.get("Content-Security-Policy") || "")
    let body
    try { body = new TextDecoder("utf-8", { fatal: true }).decode(bytes) } catch (_) { fail("offline shell UTF-8 is invalid") }
    const nonces = body.match(/\bnonce=["']([^"']+)["']/g) || []
    for (const value of nonces) { const nonce = value.slice(value.indexOf("=") + 2, -1); if (csp.indexOf(`'nonce-${nonce}'`) === -1) fail("offline shell CSP nonce differs") }
    return bytes
  }

  async function fetchShell(storage, descriptor, fetchImplementation, origin) {
    const shellURL = sameOriginURL(descriptor.offlineShellUrl || expectedDescriptorRoutes(descriptor).offlineShellUrl, origin)
    if (!shellURL) fail("offline shell route is not same-origin")
    const response = await fetchImplementation(shellURL.href, { method: "GET", cache: "no-store", credentials: "same-origin", headers: { Accept: "text/html" } })
    if (isWithdrawalResponse(response, "resource")) {
      await disablePublication(storage, descriptor, `HTTP ${response.status}`, response.status === 404 ? "deleted" : "revoked")
      const error = new Error("offline shell was withdrawn")
      error.kind = "disabled"
      throw error
    }
    if (!response || !response.ok) fail("offline shell request failed")
    await validateShell(response)
    return { url: shellURL.href, response }
  }

  async function validateFallbackAssets(scope, descriptor, fetchImplementation, origin) {
    const assets = Array.isArray(descriptor.fallbackAssets) ? descriptor.fallbackAssets : []
    if (assets.length === 0) return []
    if (!scope || !scope.caches) fail("fallback asset cache is unavailable")
    const cache = await scope.caches.open((scope.ManjaLocalDocsStorage && scope.ManjaLocalDocsStorage.CACHE_NAME) || "manja-local-docs-assets-v1")
    for (const asset of assets) {
      const route = sameOriginURL(asset.url, origin)
      if (!route) fail("fallback asset route is invalid")
      const response = await fetchImplementation(route.href, { method: "GET", cache: "no-store", credentials: "same-origin" })
      if (!response || !response.ok) fail("fallback asset unavailable")
      const copy = response.clone ? response.clone() : response
      const body = await readBoundedResponse(response, asset.length || MAX_ASSET_BYTES)
      if (asset.length !== undefined && body.byteLength !== asset.length) fail("fallback asset length differs")
      if (asset.sha256 && await sha256(body) !== asset.sha256) fail("fallback asset digest differs")
      await cache.put(route.href, copy)
    }
    return assets.map((asset) => asset.url)
  }

  async function validateManifestChild(storage, descriptor, request, response) {
    const pathname = new URL(request.url).pathname
    const routes = expectedDescriptorRoutes(descriptor)
    let childBase = ""
    if (pathname.indexOf(routes.searchDataBase) === 0) childBase = routes.searchDataBase
    if (pathname.indexOf(routes.projectionDataBase) === 0) childBase = routes.projectionDataBase
    if (!childBase || !response || !response.ok) return true
    const active = await storage.loadActive(descriptor.publicationKey)
    if (!active || !(active.manifestBytes instanceof Uint8Array)) return false
    const relativePath = pathname.slice(childBase.length)
    const parsed = await parseManifest(active.manifestBytes, descriptor)
    const child = parsed.manifest.children.find((value) => value.path === relativePath)
    if (!child) return false
    const maximum = child.length <= MAX_CHILD_BYTES ? child.length : MAX_CHILD_BYTES
    const bytes = await readBoundedResponse(response.clone ? response.clone() : response, maximum)
    if (bytes.byteLength !== child.length || await sha256(bytes) !== child.sha256) fail("manifest child digest differs")
    return true
  }

  function createRevalidator(run) {
    let inFlight
    return function revalidate() {
      if (inFlight) return inFlight
      const current = Promise.resolve().then(run).finally(() => { if (inFlight === current) inFlight = undefined })
      inFlight = current
      return current
    }
  }

  async function commitCandidate({ storage, descriptor, candidate, prepare, activate }) {
    if (!storage || !descriptor || !candidate || !(candidate.projectionBytes instanceof Uint8Array)) fail("candidate projection is invalid")
    const key = descriptor.publicationKey
    let token
    try {
      await storage.commitGeneration({ ...descriptor, ...candidate }, candidate)
      if (typeof prepare === "function") await prepare(candidate)
      token = await storage.activate(key, candidate.revisionId || descriptor.revisionId)
      if (typeof activate === "function") await activate(candidate)
    } catch (error) {
      if (token && typeof storage.rollback === "function") await storage.rollback(key, token).catch(() => {})
      if (!token && typeof storage.discardCandidate === "function") await storage.discardCandidate(key, candidate.revisionId || descriptor.revisionId).catch(() => {})
      throw error
    }
    return { kind: "activated", revisionId: candidate.revisionId || descriptor.revisionId }
  }

  async function disablePublication(storage, descriptor, reason, state = "revoked") {
    if (!storage || !descriptor) return
    if (typeof storage.tombstone === "function") await storage.tombstone(descriptor.publicationKey, reason, state)
  }

  async function revalidate({ storage, descriptor, fetch: fetchImplementation, origin, cryptoImplementation, prepare, activate, now, scope, routingDisabled }) {
    if (typeof routingDisabled === "function" && routingDisabled()) return { kind: "disabled", reason: "publication disabled" }
    const checkedAt = now instanceof Date ? now.toISOString() : typeof now === "string" ? now : new Date().toISOString()
    const state = await storage.loadMetadata(descriptor.publicationKey)
    if (state && state.disabled && (!state.tombstone || state.tombstone.revisionId === descriptor.revisionId)) return { kind: "disabled", reason: state.tombstone && state.tombstone.reason }
    const headers = { Accept: "application/json" }
    if (state && state.etag) headers["If-None-Match"] = state.etag
    const requestURL = sameOriginURL(descriptor.projectionManifestUrl, origin)
    if (!requestURL) fail("manifest route is not same-origin")
    let response
    try { response = await fetchImplementation(requestURL.href, { method: "GET", cache: "no-store", credentials: "same-origin", headers }) } catch (error) { return { kind: "fallback", error: String(error && error.message || error).slice(0, 256) } }
    if (typeof routingDisabled === "function" && routingDisabled()) return { kind: "disabled", reason: "publication disabled" }
    if (isWithdrawalResponse(response, "resource")) { await disablePublication(storage, descriptor, `HTTP ${response.status}`, response.status === 404 ? "deleted" : "revoked"); return { kind: "disabled", status: response.status } }
    if (response.status === 304) { await storage.observe(descriptor.publicationKey, { etag: response.headers.get("ETag") || (state && state.etag) || "", lastObservedAt: checkedAt }); return { kind: "ready", unchanged: true } }
    if (!response.ok) return { kind: "fallback", error: `HTTP ${response.status}` }
    let body
    try { body = await readBoundedResponse(response, MAX_MANIFEST_BYTES); await parseManifest(body, descriptor, cryptoImplementation) } catch (error) { return { kind: "fallback", error: String(error && error.message || error).slice(0, 256) } }
    const active = await storage.loadActive(descriptor.publicationKey)
    if (typeof routingDisabled === "function" && routingDisabled()) return { kind: "disabled", reason: "publication disabled" }
    if (active && active.projectionDigest === descriptor.projectionDigest && active.revisionId === descriptor.revisionId) { await storage.observe(descriptor.publicationKey, { etag: response.headers.get("ETag") || "", lastObservedAt: checkedAt }); return { kind: "ready", unchanged: true } }
    let shell
    let fallbackAssets
    try {
      shell = await fetchShell(storage, descriptor, fetchImplementation, origin)
      fallbackAssets = await validateFallbackAssets(scope, descriptor, fetchImplementation, origin)
    } catch (error) {
      if (error && error.kind === "disabled") return { kind: "disabled", reason: error.message }
      return { kind: "fallback", error: String(error && error.message || error).slice(0, 256) }
    }
    if (typeof routingDisabled === "function" && routingDisabled()) return { kind: "disabled", reason: "publication disabled" }
    if (typeof storage.observe === "function") await storage.observe(descriptor.publicationKey, descriptor)
    if (typeof routingDisabled === "function" && routingDisabled()) return { kind: "disabled", reason: "publication disabled" }
    if (state && state.disabled && typeof storage.allowReenable === "function") await storage.allowReenable(descriptor.publicationKey, descriptor.revisionId)
    const candidate = { publicationKey: descriptor.publicationKey, revisionId: descriptor.revisionId, projectionDigest: descriptor.projectionDigest, snapshotId: descriptor.snapshotId, projectionBytes: body, manifestBytes: body, etag: response.headers.get("ETag") || "", lastObservedAt: checkedAt, shellURL: descriptor.offlineShellUrl }
    await commitCandidate({
      storage,
      descriptor,
      candidate,
      prepare,
      activate: async (value) => {
        if (shell && typeof storage.putShell === "function") await storage.putShell(descriptor.publicationKey, shell.url, shell.response.clone ? shell.response.clone() : shell.response)
        if (fallbackAssets) value.assetURLs = fallbackAssets.slice()
        if (typeof activate === "function") await activate(value)
      },
    })
    return { kind: "ready", revisionId: candidate.revisionId, changed: true }
  }

  async function cachedOrFetched(scope, storage, descriptor, request, fetchImplementation, routingDisabled) {
    const network = fetchImplementation || ((value, init) => scope.fetch(value, init))
    const routeKind = request.mode === "navigate" ? "document" : "resource"
    if (typeof routingDisabled === "function" && routingDisabled()) return network(request)
    try {
      if (typeof storage.isDisabled === "function" && await storage.isDisabled(descriptor.publicationKey)) return network(request)
    } catch (_) {
      return network(request)
    }
    try {
      const response = await network(request)
      if (typeof routingDisabled === "function" && routingDisabled()) return response
      if (isWithdrawalResponse(response, routeKind)) {
        await disablePublication(storage, descriptor, `HTTP ${response.status}`, response.status === 404 ? "deleted" : "revoked")
        return response
      }
      const pathname = new URL(request.url).pathname
      if (response.ok && pathname.indexOf(descriptor.publicationBase + "openapi/") === 0) {
        await readBoundedResponse(response.clone ? response.clone() : response, MAX_SPEC_BYTES)
      }
      const manifestChildValid = await validateManifestChild(storage, descriptor, request, response)
      if (manifestChildValid === false) return response
      if (response.ok && !isHTMXRequest(request) && new URL(request.url).pathname === descriptor.offlineShellUrl) {
        try { await validateShell(response.clone ? response.clone() : response); await storage.putShell(descriptor.publicationKey, descriptor.offlineShellUrl, response.clone()) } catch (_) {}
      } else if (response.ok && request.mode !== "navigate" && !isHTMXRequest(request)) {
        try { await storage.putAsset(descriptor.publicationKey, request.url, response.clone()) } catch (_) {}
      }
      return response
    } catch (error) {
      try {
        if (typeof routingDisabled === "function" && routingDisabled()) return network(request)
        if (typeof storage.isDisabled === "function" && await storage.isDisabled(descriptor.publicationKey)) throw error
        const cached = await (request.mode === "navigate" ? storage.getShell(descriptor.publicationKey, descriptor.offlineShellUrl) : storage.getAsset(descriptor.publicationKey, request.url))
        if (cached) return cached
      } catch (_) {}
      return network(request)
    }
  }

  function isStaticAssetPath(pathname) {
    if (typeof pathname !== "string" || pathname.indexOf(STATIC_PREFIX) !== 0) return false
    const name = pathname.slice(STATIC_PREFIX.length)
    return ["sw.js", "storage.js", "local-docs.js", "wasm_exec.js", "manja.wasm", "manja.wasm.br"].includes(name)
  }

  async function cachedStaticAsset(scope, request, cacheName, fetchImplementation) {
    try {
      const cache = await scope.caches.open(cacheName)
      const cached = await cache.match(request)
      if (cached) return cached
      const response = await fetchImplementation(request)
      if (response && response.ok) {
        await readBoundedResponse(response.clone ? response.clone() : response, MAX_ASSET_BYTES)
        await cache.put(request, response.clone())
      }
      return response
    } catch (_) {
      return fetchImplementation(request)
    }
  }

  function findDescriptor(descriptors, requestURL) {
    for (const descriptor of descriptors.values()) {
      if (isAllowedRequest({ method: "GET", url: requestURL.href }, descriptor, requestURL.origin)) return descriptor
    }
    return undefined
  }

  function register(scope, options = {}) {
    const storageAPI = options.storageAPI || scope.ManjaLocalDocsStorage || (typeof globalThis !== "undefined" && globalThis.ManjaLocalDocsStorage)
    const storage = options.storage || (storageAPI && storageAPI.createStorage(scope))
    if (!storage) throw new Error("local-docs storage is unavailable")
    const descriptors = new Map()
    const disabled = new Set()
    const revalidators = new Map()
    const configurations = new Map()
    const route = options.route
    const fetchImplementation = options.fetch || ((value, init) => scope.fetch(value, init))
    const origin = scope.location && scope.location.origin
    const post = async (message) => {
      if (!scope.clients || typeof scope.clients.matchAll !== "function") return
      const clients = await scope.clients.matchAll({ type: "window", includeUncontrolled: true })
      for (const client of clients) if (client && typeof client.postMessage === "function") client.postMessage(message)
    }
    async function configure(input, source) {
      const descriptor = validateDescriptor(input, origin)
      if (disabled.has(descriptor.publicationKey)) return { ok: false, reason: "publication disabled" }
      if (await storage.isDisabled(descriptor.publicationKey)) {
        const current = await storage.loadMetadata(descriptor.publicationKey)
        if (current && current.tombstone && current.tombstone.revisionId === descriptor.revisionId) return { ok: false, reason: "publication tombstoned" }
      }
      if (typeof storage.observe === "function") await storage.observe(descriptor.publicationKey, descriptor)
      descriptors.set(descriptor.publicationKey, descriptor)
      if (source && typeof source.postMessage === "function") source.postMessage({ type: "manja:configured", publicationKey: descriptor.publicationKey })
      return { ok: true, descriptor }
    }
    function revalidateDescriptor(descriptor) {
      if (disabled.has(descriptor.publicationKey)) return Promise.resolve({ kind: "disabled", reason: "publication disabled" })
      let operation = revalidators.get(descriptor.publicationKey)
      if (!operation) {
        operation = createRevalidator(() => revalidate({ storage, descriptor, fetch: fetchImplementation, origin, cryptoImplementation: scope.crypto || global.crypto, prepare: options.prepare, activate: options.activate, scope, routingDisabled: () => disabled.has(descriptor.publicationKey) }))
        revalidators.set(descriptor.publicationKey, operation)
      }
      return operation()
    }
    async function descriptorForURL(url) {
      const direct = findDescriptor(descriptors, url)
      if (direct) {
        if (disabled.has(direct.publicationKey)) {
          descriptors.delete(direct.publicationKey)
          return undefined
        }
        if (typeof storage.isDisabled === "function" && await storage.isDisabled(direct.publicationKey)) {
          descriptors.delete(direct.publicationKey)
          return undefined
        }
        return direct
      }
      if (typeof storage.listMetadata !== "function") return undefined
      const metadata = await storage.listMetadata()
      for (const value of metadata) {
        if (disabled.has(value.publicationKey)) continue
        const descriptor = descriptorFromMetadata(value, origin)
        if (descriptor && isAllowedRequest({ method: "GET", url: url.href }, descriptor, origin)) {
          descriptors.set(descriptor.publicationKey, descriptor)
          return descriptor
        }
      }
      return undefined
    }
    scope.addEventListener("install", (event) => {
      event.waitUntil(Promise.resolve().then(async () => {
        const assets = Array.isArray(options.assets) ? options.assets : Array.isArray(options.fallbackAssets) ? options.fallbackAssets : []
        if (assets.length > 0 && !scope.caches) fail("fallback asset cache is unavailable")
        if (assets.length > 0) {
          const cache = await scope.caches.open(storageAPI && storageAPI.CACHE_NAME || "manja-local-docs-assets-v1")
          for (const asset of assets) {
            const expected = typeof asset === "string" ? { url: asset } : asset
            const assetURL = expected && expected.url
            if (!assetURL) fail("fallback asset declaration is invalid")
            const route = sameOriginURL(assetURL, origin)
            if (!route) fail("fallback asset route is invalid")
            const response = await fetchImplementation(route.href, { credentials: "same-origin", cache: "no-store" })
            if (!response || !response.ok) fail("fallback asset unavailable")
            const copy = response.clone ? response.clone() : response
            const body = await readBoundedResponse(response, expected.length || MAX_ASSET_BYTES)
            if (expected.length !== undefined && body.byteLength !== expected.length) fail("fallback asset length differs")
            if (expected.sha256 && await sha256(body) !== expected.sha256) fail("fallback asset digest differs")
            await cache.put(route.href, copy)
          }
        }
        if (typeof scope.skipWaiting === "function") await scope.skipWaiting()
      }))
    })
    scope.addEventListener("activate", (event) => { event.waitUntil(Promise.resolve(typeof scope.clients?.claim === "function" ? scope.clients.claim() : undefined)) })
    scope.addEventListener("message", (event) => {
      const message = event.data || {}
      if (message.type === "manja:configure") {
        const pending = configure(message.descriptor, event.source)
        configurations.set(message.descriptor && message.descriptor.publicationKey, pending)
        event.waitUntil(pending.finally(() => configurations.delete(message.descriptor && message.descriptor.publicationKey)))
      }
      else if (message.type === "manja:revalidate") {
        const configured = configurations.get(message.publicationKey)
        const descriptorPromise = configured ? configured.then(() => descriptors.get(message.publicationKey)) : Promise.resolve(descriptors.get(message.publicationKey))
        event.waitUntil(descriptorPromise.then((descriptor) => {
          if (!descriptor) return undefined
          return revalidateDescriptor(descriptor).then((result) => event.source?.postMessage({ type: "manja:local-ready", ...result })).catch((error) => event.source?.postMessage({ type: "manja:local-fallback", reason: String(error.message || error).slice(0, 256) }))
        }))
      } else if (message.type === "manja:disable") {
        const descriptor = descriptors.get(message.publicationKey)
        if (descriptor) {
          disabled.add(descriptor.publicationKey)
          descriptors.delete(descriptor.publicationKey)
          event.waitUntil((async () => {
            await disablePublication(storage, descriptor, message.reason || "disabled", message.state || "disabled")
            await post({ type: "manja:local-fallback", publicationKey: descriptor.publicationKey, reason: String(message.reason || "disabled").slice(0, 256) })
          })())
        }
      } else if (message.type === "manja:claim-client" && scope.clients?.claim) event.waitUntil(scope.clients.claim())
    })
    scope.addEventListener("fetch", (event) => {
      const request = event.request
      if (!request || request.method !== "GET") return
      let url
      try { url = new URL(request.url) } catch (_) { return }
      if (url.origin !== origin) return
      if (isStaticAssetPath(url.pathname) && scope.caches) {
        event.respondWith(cachedStaticAsset(scope, request, storageAPI && storageAPI.CACHE_NAME || "manja-local-docs-assets-v1", fetchImplementation))
        return
      }
      if (isManagementPath(url.pathname)) return
      event.respondWith((async () => {
        let descriptor
        try { descriptor = await descriptorForURL(url) } catch (_) { return fetchImplementation(request) }
        if (!descriptor || !isAllowedRequest(request, descriptor, origin)) return fetchImplementation(request)
        if (isHTMXRequest(request) && typeof route === "function") {
          return Promise.resolve().then(() => route(request, descriptor)).catch(() => cachedOrFetched(scope, storage, descriptor, request, fetchImplementation, () => disabled.has(descriptor.publicationKey)))
        }
        return cachedOrFetched(scope, storage, descriptor, request, fetchImplementation, () => disabled.has(descriptor.publicationKey))
      })())
    })
    return { storage, descriptors, configure, revalidate: revalidateDescriptor, disabled }
  }

  return {
    MAX_ASSET_BYTES,
    MAX_CHILD_BYTES,
    MAX_MANIFEST_BYTES,
    MAX_SPEC_BYTES,
    MAX_SHELL_BYTES,
    ROOT_SCOPE,
    cachedOrFetched,
    commitCandidate,
    createRevalidator,
    descriptorFromMetadata,
    disablePublication,
    isStaticAssetPath,
    isAllowedRequest,
    isHTMXRequest,
    isWithdrawalResponse,
    parseManifest,
    readBoundedResponse,
    revalidate,
    register,
    sameOriginURL,
    sha256,
    validateDescriptor,
    validateManifestChild,
    validateShell,
  }
}))
