if (typeof importScripts === "function" && typeof globalThis !== "undefined" && !globalThis.ManjaLocalDocsAssetManifest) {
  try { importScripts("/manja-assets/local-docs/runtime-assets.js") } catch (_) {}
}

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
  const DEFAULT_STATIC_ASSETS = [
    STATIC_PREFIX + "sw.js",
    STATIC_PREFIX + "storage.js",
    "/manja-assets/local-docs.js",
    STATIC_PREFIX + "wasm_exec.js",
    STATIC_PREFIX + "manja.wasm",
    STATIC_PREFIX + "manja.wasm.br",
  ]

  // The companion is generated from the embedded production bytes. Keeping
  // sw.js outside its own source avoids a self-referential digest while still
  // binding every default JS/Wasm response to immutable build metadata.
  let assetManifest = global && global.ManjaLocalDocsAssetManifest
  if (!assetManifest && typeof module === "object" && module.exports && typeof require === "function") {
    try { assetManifest = require("./runtime-assets.js") } catch (_) {}
  }
  const DEFAULT_STATIC_ASSET_EXPECTATIONS = Object.freeze(DEFAULT_STATIC_ASSETS.reduce((result, path) => {
    const expected = assetManifest && assetManifest.schemaVersion === 1 && assetManifest.assets && assetManifest.assets[path]
    if (expected && Number.isSafeInteger(expected.length) && expected.length > 0 && expected.length <= MAX_ASSET_BYTES && typeof expected.sha256 === "string" && DIGEST_PATTERN.test(expected.sha256)) {
      result[path] = Object.freeze({ length: expected.length, sha256: expected.sha256 })
    }
    return result
  }, Object.create(null)))

  function fail(message) { throw new Error(message) }

  function routingIsDisabled(routingDisabled) {
    return typeof routingDisabled === "function" && routingDisabled()
  }

  function ensureRoutingEnabled(routingDisabled) {
    if (routingIsDisabled(routingDisabled)) fail("publication disabled")
  }

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

  function validCatalogKey(value) { return validIdentity(value, 64) && /^[a-z0-9][a-z0-9._-]{0,63}$/.test(value) }
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

  function isPublicShellURL(value, descriptor, origin) {
    if (!descriptor || typeof descriptor.publicationBase !== "string") return false
    const expected = descriptor.publicationBase + "_manja/offline-shell"
    const parsed = sameOriginURL(value, origin)
    return Boolean(parsed) && value === expected && !isManagementPath(parsed.pathname)
  }

  function validateFallbackAsset(asset, origin) {
    if (!asset || typeof asset !== "object" || !sameOriginURL(asset.url, origin)) fail("fallback asset route is not same-origin")
    if (isManagementPath(new URL(asset.url, origin).pathname)) fail("fallback asset route is reserved")
    if (asset.length !== undefined && (!Number.isSafeInteger(asset.length) || asset.length <= 0 || asset.length > MAX_ASSET_BYTES)) fail("fallback asset length is invalid")
    if (asset.sha256 !== undefined && !validDigest(asset.sha256)) fail("fallback asset digest is invalid")
    return { url: asset.url, length: asset.length, sha256: asset.sha256 || "", version: typeof asset.version === "string" ? asset.version : "", kind: typeof asset.kind === "string" ? asset.kind : "asset" }
  }

  function validateDescriptor(input, origin) {
    if (!input || typeof input !== "object" || input.schemaVersion !== 1 || !validCatalogKey(input.catalogId) || !validPublicationKey(input.publicationKey) || !validIdentity(input.revisionId) || input.projectionFormat !== "projection-v2" || !validDigest(input.projectionDigest) || input.snapshotId !== "snapshot-sha256-" + input.projectionDigest) fail("descriptor identity is invalid")
    if (!validBase(input.publicationBase, origin)) fail("descriptor publication base is invalid")
    if (input.public !== true || input.anonymous !== true || input.private === true || input.disabled === true || (input.eligibility && (input.eligibility.public !== true || input.eligibility.anonymous !== true))) fail("descriptor public eligibility is invalid")
    const routes = expectedDescriptorRoutes(input)
    if (!isPublicShellURL(routes.offlineShellUrl, input, origin)) fail("descriptor offline shell route is not public")
    Object.keys(routes).forEach((key) => {
      if (input[key] !== undefined && !sameOriginURL(input[key], origin)) fail("descriptor route is not same-origin")
      if (!sameOriginURL(routes[key], origin)) fail("descriptor route is not same-origin")
      if (input[key] !== undefined && input[key] !== routes[key]) fail("descriptor route is invalid")
    })
    const fallbackAssets = Array.isArray(input.fallbackAssets) ? input.fallbackAssets.map((asset) => validateFallbackAsset(asset, origin)) : []
    if (input.fallbackManifest !== undefined && !Array.isArray(input.fallbackManifest)) fail("fallback manifest is invalid")
    const result = { ...input, ...routes, fallbackAssets: Array.isArray(input.fallbackManifest) ? input.fallbackManifest.map((asset) => validateFallbackAsset(asset, origin)) : fallbackAssets }
    return result
  }

  function descriptorFromMetadata(metadata, origin) {
    if (!metadata || metadata.disabled || typeof metadata.publicationBase !== "string" || metadata.publicationBase === "") return undefined
    try {
      return validateDescriptor({
        schemaVersion: 1,
        catalogId: metadata.catalogId,
        publicationKey: metadata.publicationKey,
        public: metadata.public === true,
        anonymous: metadata.anonymous === true,
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
        fallbackAssets: metadata.fallbackAssets,
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

  function validatePublicShellResponse(response, descriptor, origin) {
    if (!response || !response.ok || response.status !== 200) fail("offline shell response is not public")
    if (response.redirected || response.type === "opaqueredirect") fail("offline shell response redirected")
    if (response.url) {
      const expected = new URL(descriptor.offlineShellUrl, origin || (global.location && global.location.origin) || "https://manja-local-docs.invalid")
      let actual
      try { actual = new URL(response.url, expected.origin) } catch (_) { actual = undefined }
      if (!actual || actual.origin !== expected.origin || actual.username !== "" || actual.password !== "" || actual.pathname !== expected.pathname || actual.search !== "" || actual.hash !== "") fail("offline shell response route differs")
    }
    for (const header of ["WWW-Authenticate", "Set-Cookie", "X-Manja-Authenticated", "X-Manja-Auth", "X-Authenticated-User", "X-Auth-Request-User"]) {
      if (response.headers && response.headers.get(header)) fail("offline shell response is authenticated")
    }
    return response
  }

  function isWithdrawalResponse(response, kind = "resource") {
    if (!response) return false
    if (WITHDRAWAL_STATUS.has(response.status)) {
      return response.status !== 404 || kind !== "document" || PUBLIC_STATES.has(String(response.headers.get("X-Manja-Publication-State") || "").toLowerCase())
    }
    const state = String(response.headers.get("X-Manja-Publication-State") || "").toLowerCase()
    return PUBLIC_STATES.has(state)
  }

  function withdrawalState(response) {
    const state = String(response && response.headers && response.headers.get("X-Manja-Publication-State") || "").toLowerCase()
    if (PUBLIC_STATES.has(state)) return state
    return response && response.status === 404 ? "deleted" : "revoked"
  }

  function isCanonicalShellWithdrawal(response, descriptor, request) {
    if (!response) return false
    if (isWithdrawalResponse(response, request && request.mode === "navigate" ? "document" : "resource")) return true
    // A disappeared canonical shell is an eligibility withdrawal even when the
    // server's 404 has no state header. Other document 404s keep SSR fallback.
    if (response.status !== 404 || !descriptor || !request) return false
    try { return new URL(request.url).pathname === descriptor.offlineShellUrl } catch (_) { return false }
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
        index++; whitespace(); const object = Object.create(null)
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
        if (!/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)$/.test(token)) fail("manifest JSON is invalid")
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
      if (seen.has(child.path)) fail("manifest child is duplicated")
      seen.add(child.path)
      if (typeof child.kind !== "string" || child.kind.length === 0 || child.kind.length > 64 || !Number.isSafeInteger(child.length) || child.length <= 0 || child.length > MAX_SPEC_BYTES || !validDigest(child.sha256)) fail("manifest child is invalid")
      if (Array.isArray(manifest.identity.children)) {
        if (manifest.identity.children.length !== manifest.children.length || index > 0 && manifest.children[index - 1].path >= child.path) fail("manifest children are not canonical")
        const identityChild = manifest.identity.children[index]
        if (!identityChild || identityChild.path !== child.path || identityChild.kind !== child.kind || identityChild.length !== child.length || identityChild.sha256 !== child.sha256) fail("manifest children differ from identity")
      }
      const expectedKind = child.path.indexOf("details/") === 0 ? "detail" : child.path.indexOf("schema-nodes/") === 0 ? "schema-node" : ""
      if (expectedKind && child.kind !== expectedKind) fail("manifest projection child kind is invalid")
      if (expectedKind) {
        if (child.length > MAX_CHILD_BYTES) fail("manifest projection child is invalid")
      } else if (child.path === "catalog.json") {
        if (child.kind !== "catalog") fail("manifest catalog child kind is invalid")
      } else if (child.path.indexOf("sources/") === 0) {
        if (child.kind !== "source") fail("manifest source child kind is invalid")
      } else if (child.path.indexOf("support/") === 0) {
        if (child.kind !== "support") fail("manifest support child kind is invalid")
      } else if (child.path.indexOf("search/") === 0) {
        if (child.kind.indexOf("search-") !== 0 || child.length > MAX_CHILD_BYTES) fail("manifest search child is invalid")
      } else {
        fail("manifest child route is invalid")
      }
    }
    return { manifest, identityDigest }
  }

  async function validateShell(response, maximum = MAX_SHELL_BYTES) {
    const bytes = await readBoundedResponse(response.clone ? response.clone() : response, maximum)
    const csp = String(response.headers && response.headers.get("Content-Security-Policy") || "")
    const nonces = String(new TextDecoder().decode(bytes)).match(/\bnonce=["']([^"']+)["']/g) || []
    for (const value of nonces) { const nonce = value.slice(value.indexOf("=") + 2, -1); if (csp.indexOf(`'nonce-${nonce}'`) === -1) fail("offline shell CSP nonce differs") }
    return bytes
  }

  function childPathForRequest(descriptor, pathname) {
    const routes = expectedDescriptorRoutes(descriptor)
    if (pathname === routes.catalogUrl) return "catalog.json"
    const openapiBase = descriptor.publicationBase + "snapshots/" + descriptor.snapshotId + "/openapi/"
    if (pathname.indexOf(openapiBase) === 0) return "sources/" + pathname.slice(openapiBase.length)
    if (pathname.indexOf(routes.searchDataBase) === 0) return "search/" + pathname.slice(routes.searchDataBase.length)
    if (pathname.indexOf(routes.projectionDataBase) === 0) return pathname.slice(routes.projectionDataBase.length)
    return ""
  }

  function childMaximum(child) {
    return child.path.indexOf("details/") === 0 || child.path.indexOf("schema-nodes/") === 0 || child.path.indexOf("search/") === 0 ? MAX_CHILD_BYTES : MAX_SPEC_BYTES
  }

  async function validateManifestChild(storage, descriptor, request, response) {
    const pathname = new URL(request.url).pathname
    const childPath = childPathForRequest(descriptor, pathname)
    if (!childPath || !response || !response.ok) return response
    const active = await storage.loadActive(descriptor.publicationKey)
    if (!active || active.projectionDigest !== descriptor.projectionDigest || active.revisionId !== descriptor.revisionId || !(active.manifestBytes instanceof Uint8Array)) return response
    const parsed = await parseManifest(active.manifestBytes, descriptor)
    const child = parsed.manifest.children.find((value) => value.path === childPath)
    if (!child) fail("manifest child is not declared")
    const maximum = childMaximum(child)
    const bytes = await readBoundedResponse(response.clone ? response.clone() : response, maximum)
    if (bytes.byteLength !== child.length || await sha256(bytes) !== child.sha256) fail("manifest child digest differs")
    return response
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

  async function commitCandidate({ storage, descriptor, candidate, prepare, activate, routingDisabled }) {
    if (!storage || !descriptor || !candidate || !(candidate.projectionBytes instanceof Uint8Array)) fail("candidate projection is invalid")
    ensureRoutingEnabled(routingDisabled)
    const key = descriptor.publicationKey
    let token
    try {
      await storage.commitGeneration({ ...descriptor, ...candidate }, candidate)
      ensureRoutingEnabled(routingDisabled)
      if (typeof prepare === "function") await prepare(candidate)
      ensureRoutingEnabled(routingDisabled)
      token = await storage.activate(key, candidate.revisionId || descriptor.revisionId)
      ensureRoutingEnabled(routingDisabled)
      if (typeof activate === "function") await activate(candidate)
    } catch (error) {
      if (token && typeof storage.rollback === "function") await storage.rollback(key, token).catch(() => {})
      if (!token && typeof storage.discardCandidate === "function") await storage.discardCandidate(key, candidate.revisionId || descriptor.revisionId).catch(() => {})
      throw error
    }
    return { kind: "activated", revisionId: candidate.revisionId || descriptor.revisionId }
  }

  async function recoverPersistedManifest(storage, descriptor, cryptoImplementation, checkedAt, routingDisabled) {
    if (!storage || typeof storage.loadActive !== "function") return undefined
    ensureRoutingEnabled(routingDisabled)
    const candidates = []
    try { candidates.push(await storage.loadActive(descriptor.publicationKey)) } catch (_) {}
    ensureRoutingEnabled(routingDisabled)
    if (typeof storage.loadPrevious === "function") {
      try { candidates.push(await storage.loadPrevious(descriptor.publicationKey)) } catch (_) {}
      ensureRoutingEnabled(routingDisabled)
    }
    for (const candidate of candidates) {
      if (!candidate || candidate.publicationKey !== descriptor.publicationKey || candidate.revisionId !== descriptor.revisionId || candidate.projectionDigest !== descriptor.projectionDigest || candidate.snapshotId !== descriptor.snapshotId || !(candidate.manifestBytes instanceof Uint8Array)) continue
      try {
        await parseManifest(candidate.manifestBytes, descriptor, cryptoImplementation)
        ensureRoutingEnabled(routingDisabled)
        if (typeof storage.observe === "function") {
          await storage.observe(descriptor.publicationKey, { etag: candidate.etag || "", lastObservedAt: checkedAt })
          ensureRoutingEnabled(routingDisabled)
        }
        return { kind: "ready", revisionId: candidate.revisionId, offline: true }
      } catch (error) {
        ensureRoutingEnabled(routingDisabled)
      }
    }
    return undefined
  }

  async function persistedManifestResponse(storage, descriptor, cryptoImplementation) {
    if (!storage || typeof storage.loadActive !== "function") return undefined
    const candidates = []
    try { candidates.push(await storage.loadActive(descriptor.publicationKey)) } catch (_) {}
    if (typeof storage.loadPrevious === "function") {
      try { candidates.push(await storage.loadPrevious(descriptor.publicationKey)) } catch (_) {}
    }
    for (const candidate of candidates) {
      if (!candidate || candidate.publicationKey !== descriptor.publicationKey || candidate.revisionId !== descriptor.revisionId || candidate.projectionDigest !== descriptor.projectionDigest || candidate.snapshotId !== descriptor.snapshotId || !(candidate.manifestBytes instanceof Uint8Array)) continue
      try {
        await parseManifest(candidate.manifestBytes, descriptor, cryptoImplementation)
        const headers = { "Content-Type": "application/json; charset=utf-8" }
        if (candidate.etag) headers.ETag = candidate.etag
        return new Response(candidate.manifestBytes.slice(), { status: 200, headers })
      } catch (_) {}
    }
    return undefined
  }

  async function disablePublication(storage, descriptor, reason, state = "revoked", disableRouting) {
    if (!storage || !descriptor) return
    if (typeof disableRouting === "function") disableRouting(descriptor.publicationKey)
    if (typeof storage.tombstone === "function") await storage.tombstone(descriptor.publicationKey, reason, state)
  }

  async function revalidate({ storage, descriptor, fetch: fetchImplementation, origin, cryptoImplementation, prepare, activate, now, routingDisabled, disableRouting }) {
    ensureRoutingEnabled(routingDisabled)
    const checkedAt = now instanceof Date ? now.toISOString() : typeof now === "string" ? now : new Date().toISOString()
    const state = await storage.loadMetadata(descriptor.publicationKey)
    ensureRoutingEnabled(routingDisabled)
    if (state && state.disabled && (!state.tombstone || state.tombstone.revisionId === descriptor.revisionId)) return { kind: "disabled", reason: state.tombstone && state.tombstone.reason }
    const headers = { Accept: "application/json" }
    if (state && state.etag) headers["If-None-Match"] = state.etag
    const requestURL = sameOriginURL(descriptor.projectionManifestUrl, origin)
    if (!requestURL) fail("manifest route is not same-origin")
    let response
    try { response = await fetchImplementation(requestURL.href, { method: "GET", cache: "no-store", credentials: "same-origin", headers }) } catch (error) {
      const recovered = await recoverPersistedManifest(storage, descriptor, cryptoImplementation, checkedAt, routingDisabled)
      ensureRoutingEnabled(routingDisabled)
      return recovered || { kind: "fallback", error: String(error && error.message || error).slice(0, 256) }
    }
    if (isWithdrawalResponse(response, "resource")) { await disablePublication(storage, descriptor, `HTTP ${response.status}`, withdrawalState(response), disableRouting); return { kind: "disabled", status: response.status } }
    ensureRoutingEnabled(routingDisabled)
    if (response.status === 304) {
      await storage.observe(descriptor.publicationKey, { etag: response.headers.get("ETag") || (state && state.etag) || "", lastObservedAt: checkedAt })
      ensureRoutingEnabled(routingDisabled)
      return { kind: "ready", unchanged: true }
    }
    if (!response.ok) {
      const recovered = await recoverPersistedManifest(storage, descriptor, cryptoImplementation, checkedAt, routingDisabled)
      ensureRoutingEnabled(routingDisabled)
      return recovered || { kind: "fallback", error: `HTTP ${response.status}` }
    }
    let body
    try { body = await readBoundedResponse(response, MAX_MANIFEST_BYTES); await parseManifest(body, descriptor, cryptoImplementation) } catch (error) {
      ensureRoutingEnabled(routingDisabled)
      return { kind: "fallback", error: String(error && error.message || error).slice(0, 256) }
    }
    ensureRoutingEnabled(routingDisabled)
    const active = await storage.loadActive(descriptor.publicationKey)
    ensureRoutingEnabled(routingDisabled)
    if (active && active.projectionDigest === descriptor.projectionDigest && active.revisionId === descriptor.revisionId) {
      await storage.observe(descriptor.publicationKey, { etag: response.headers.get("ETag") || "", lastObservedAt: checkedAt })
      ensureRoutingEnabled(routingDisabled)
      return { kind: "ready", unchanged: true }
    }
    const candidate = { publicationKey: descriptor.publicationKey, revisionId: descriptor.revisionId, projectionDigest: descriptor.projectionDigest, snapshotId: descriptor.snapshotId, projectionBytes: body, manifestBytes: body, etag: response.headers.get("ETag") || "", lastObservedAt: checkedAt, shellURL: descriptor.offlineShellUrl }
    await commitCandidate({ storage, descriptor, candidate, prepare, activate, routingDisabled })
    return { kind: "ready", revisionId: candidate.revisionId, changed: true }
  }

  async function cachedOrFetched(scope, storage, descriptor, request, fetchImplementation, routingDisabled, disableRouting) {
    const network = fetchImplementation || ((value, init) => scope.fetch(value, init))
    const routeKind = request.mode === "navigate" ? "document" : "resource"
    ensureRoutingEnabled(routingDisabled)
    try {
      const response = await network(request)
      if (isCanonicalShellWithdrawal(response, descriptor, request)) {
        await disablePublication(storage, descriptor, `HTTP ${response.status}`, withdrawalState(response), disableRouting)
        return response
      }
      ensureRoutingEnabled(routingDisabled)
      const pathname = new URL(request.url).pathname
      const canonicalShell = pathname === descriptor.offlineShellUrl
      const anonymousShellRequest = request.credentials === undefined || request.credentials === "omit"
      if (response.ok && pathname.indexOf(descriptor.publicationBase + "openapi/") === 0) {
        await readBoundedResponse(response.clone ? response.clone() : response, MAX_SPEC_BYTES)
      }
      await validateManifestChild(storage, descriptor, request, response)
      ensureRoutingEnabled(routingDisabled)
      if (response.ok && !isHTMXRequest(request) && canonicalShell && anonymousShellRequest) {
        validatePublicShellResponse(response, descriptor, new URL(request.url).origin)
        await validateShell(response.clone ? response.clone() : response)
        ensureRoutingEnabled(routingDisabled)
        await storage.putShell(descriptor.publicationKey, descriptor.offlineShellUrl, response.clone(), descriptor)
      } else if (response.ok && routeKind !== "document") {
        ensureRoutingEnabled(routingDisabled)
        try { await storage.putAsset(descriptor.publicationKey, request.url, response.clone(), descriptor) } catch (_) {}
      }
      ensureRoutingEnabled(routingDisabled)
      return response
    } catch (error) {
      if (routingIsDisabled(routingDisabled)) throw error
      if (typeof storage.isDisabled === "function" && await storage.isDisabled(descriptor.publicationKey)) throw error
      const pathname = new URL(request.url).pathname
      if (pathname === descriptor.projectionManifestUrl) {
        const persisted = await persistedManifestResponse(storage, descriptor, scope && scope.crypto || global.crypto)
        if (persisted) {
          ensureRoutingEnabled(routingDisabled)
          return persisted
        }
      }
      const cached = await (request.mode === "navigate" ? storage.getShell(descriptor.publicationKey, descriptor.offlineShellUrl, descriptor) : storage.getAsset(descriptor.publicationKey, request.url, descriptor))
      if (routingIsDisabled(routingDisabled)) throw error
      if (cached) {
        if (request.mode === "navigate") {
          validatePublicShellResponse(cached, descriptor, new URL(request.url).origin)
          await validateShell(cached.clone ? cached.clone() : cached)
        }
        else await validateManifestChild(storage, descriptor, request, cached)
        ensureRoutingEnabled(routingDisabled)
        return cached
      }
      throw error
    }
  }

  async function fetchedWhileDisabled(storage, descriptor, request, fetchImplementation, disableRouting) {
    const response = await fetchImplementation(request, { cache: "no-store" })
    if (isCanonicalShellWithdrawal(response, descriptor, request)) {
      await disablePublication(storage, descriptor, `HTTP ${response.status}`, withdrawalState(response), disableRouting)
    }
    return response
  }

  function isStaticAssetPath(pathname) {
    if (pathname === "/manja-assets/local-docs.js") return true
    if (typeof pathname !== "string" || pathname.indexOf(STATIC_PREFIX) !== 0) return false
    const name = pathname.slice(STATIC_PREFIX.length)
    return ["sw.js", "storage.js", "local-docs.js", "wasm_exec.js", "manja.wasm", "manja.wasm.br"].includes(name)
  }

  function staticAssetExpectation(request) {
    let pathname
    try {
      pathname = new URL(typeof request === "string" ? request : request && request.url || "", "https://manja-local-docs.invalid").pathname
    } catch (_) {
      return undefined
    }
    const expected = DEFAULT_STATIC_ASSET_EXPECTATIONS[pathname]
    return expected ? { ...expected } : undefined
  }

  async function cachedStaticAsset(scope, request, cacheName, fetchImplementation, expected, routingDisabled) {
    expected = expected || staticAssetExpectation(request)
    let requestPath = ""
    try { requestPath = new URL(typeof request === "string" ? request : request && request.url || "", "https://manja-local-docs.invalid").pathname } catch (_) {}
    if (isStaticAssetPath(requestPath) && (!expected || !validDigest(expected.sha256))) fail("fallback asset digest manifest unavailable")
    ensureRoutingEnabled(routingDisabled)
    const cache = await scope.caches.open(cacheName)
    ensureRoutingEnabled(routingDisabled)
    const network = fetchImplementation || ((value, init) => scope.fetch(value, init))
    let networkError
    try {
      const response = await network(request, { credentials: "same-origin", cache: "no-store" })
      if (!response || !response.ok) fail("static asset request failed")
      const maximum = expected && expected.length ? expected.length : MAX_ASSET_BYTES
      const bytes = await readBoundedResponse(response.clone ? response.clone() : response, maximum)
      if (expected && expected.length !== undefined && bytes.byteLength !== expected.length) fail("fallback asset length differs")
      if (expected && expected.sha256 && await sha256(bytes) !== expected.sha256) fail("fallback asset digest differs")
      // Validate complete network bytes before replacing the stable cache key.
      ensureRoutingEnabled(routingDisabled)
      await cache.put(request, response.clone ? response.clone() : response)
      return response
    } catch (error) {
      networkError = error
    }
    const cached = await cache.match(request)
    if (cached) {
      try {
        if (!expected) {
          await readBoundedResponse(cached.clone ? cached.clone() : cached, MAX_ASSET_BYTES)
        } else {
          const cachedBytes = await readBoundedResponse(cached.clone ? cached.clone() : cached, expected.length || MAX_ASSET_BYTES)
          if (expected.length !== undefined && cachedBytes.byteLength !== expected.length) fail("fallback asset length differs")
          if (expected.sha256 && await sha256(cachedBytes) !== expected.sha256) fail("fallback asset digest differs")
        }
        ensureRoutingEnabled(routingDisabled)
        return cached
      } catch (_) {
        await cache.delete(request).catch(() => {})
      }
    }
    throw networkError || new Error("static asset unavailable")
  }

  function findFallbackAsset(descriptors, requestURL) {
    for (const descriptor of descriptors.values()) {
      const asset = (descriptor.fallbackAssets || []).find((value) => value.url === requestURL.href)
      if (asset) return { descriptor, asset }
    }
    return undefined
  }

  function descriptorGenerationCacheName(storageAPI, descriptor) {
    if (storageAPI && typeof storageAPI.generationCacheName === "function") {
      return storageAPI.generationCacheName(descriptor.publicationKey, descriptor)
    }
    return `manja-local-docs-assets-v1::${encodeURIComponent(descriptor.publicationKey)}::${encodeURIComponent(descriptor.revisionId)}::${descriptor.projectionDigest}`
  }

  async function cacheDescriptorAssets(scope, descriptor, cacheName, fetchImplementation, routingDisabled) {
    if (!scope.caches || !Array.isArray(descriptor.fallbackAssets)) return true
    let ready = true
    for (const asset of descriptor.fallbackAssets) {
      try {
        ensureRoutingEnabled(routingDisabled)
        await cachedStaticAsset(scope, asset.url, cacheName, fetchImplementation, asset, routingDisabled)
      } catch (_) { ready = false }
    }
    return ready
  }

  async function cacheStaticAssets(scope, cacheName, fetchImplementation, assets, routingDisabled) {
    if (!scope.caches) return true
    let ready = true
    for (const asset of assets) {
      if (typeof asset !== "string") continue
      try {
        ensureRoutingEnabled(routingDisabled)
        await cachedStaticAsset(scope, asset, cacheName, fetchImplementation, staticAssetExpectation(asset), routingDisabled)
      } catch (_) { ready = false }
    }
    return ready
  }

  async function cacheOfflineShell(storage, descriptor, fetchImplementation, routingDisabled, disableRouting) {
    if (!descriptor.offlineShellUrl || !storage || typeof storage.putShell !== "function") return false
    ensureRoutingEnabled(routingDisabled)
    const origin = global.location && global.location.origin || "https://manja-local-docs.invalid"
    if (!isPublicShellURL(descriptor.offlineShellUrl, descriptor, origin)) return false
    try {
      const response = await fetchImplementation(descriptor.offlineShellUrl, { method: "GET", cache: "no-store", credentials: "omit", redirect: "error" })
      if (isWithdrawalResponse(response, "document") || response.status === 404) {
        await disablePublication(storage, descriptor, `HTTP ${response.status}`, withdrawalState(response), disableRouting)
        return false
      }
      validatePublicShellResponse(response, descriptor, origin)
      await validateShell(response.clone ? response.clone() : response)
      ensureRoutingEnabled(routingDisabled)
      await storage.putShell(descriptor.publicationKey, descriptor.offlineShellUrl, response.clone ? response.clone() : response, descriptor)
      return true
    } catch (_) { return false }
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
    const disableRouting = (publicationKey) => {
      disabled.add(publicationKey)
    }
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
        ensureRoutingEnabled(() => disabled.has(descriptor.publicationKey))
        if (current && current.tombstone && current.tombstone.revisionId === descriptor.revisionId) return { ok: false, reason: "publication tombstoned" }
      }
      ensureRoutingEnabled(() => disabled.has(descriptor.publicationKey))
      if (typeof storage.observe === "function") {
        await storage.observe(descriptor.publicationKey, descriptor)
        ensureRoutingEnabled(() => disabled.has(descriptor.publicationKey))
      }
      descriptors.set(descriptor.publicationKey, descriptor)
      if (source && typeof source.postMessage === "function") source.postMessage({ type: "manja:configured", publicationKey: descriptor.publicationKey })
      return { ok: true, descriptor }
    }
    async function notify(source, message) {
      if (source && typeof source.postMessage === "function") {
        source.postMessage(message)
        return
      }
      await post(message)
    }
    function revalidateDescriptor(descriptor) {
      let operation = revalidators.get(descriptor.publicationKey)
      if (!operation) {
        operation = createRevalidator(() => revalidate({
          storage,
          descriptor,
          fetch: fetchImplementation,
          origin,
          cryptoImplementation: scope.crypto || global.crypto,
          prepare: options.prepare,
          activate: options.activate,
          routingDisabled: () => disabled.has(descriptor.publicationKey),
          disableRouting,
        }))
        revalidators.set(descriptor.publicationKey, operation)
      }
      return operation()
    }
    async function descriptorForURL(url) {
      const direct = findDescriptor(descriptors, url)
      if (direct) return direct
      if (typeof storage.listMetadata !== "function") return undefined
      const metadata = await storage.listMetadata()
      for (const value of metadata) {
        const descriptor = descriptorFromMetadata(value, origin)
        if (descriptor && !disabled.has(descriptor.publicationKey) && isAllowedRequest({ method: "GET", url: url.href }, descriptor, origin)) {
          descriptors.set(descriptor.publicationKey, descriptor)
          return descriptor
        }
      }
      return undefined
    }
    scope.addEventListener("install", (event) => {
      event.waitUntil(Promise.resolve().then(async () => {
        if (scope.caches) await cacheStaticAssets(scope, storageAPI && storageAPI.CACHE_NAME || "manja-local-docs-assets-v1", fetchImplementation, [...DEFAULT_STATIC_ASSETS, ...(Array.isArray(options.assets) ? options.assets : [])])
        if (typeof scope.skipWaiting === "function") await scope.skipWaiting()
      }))
    })
    scope.addEventListener("activate", (event) => { event.waitUntil(Promise.resolve(typeof scope.clients?.claim === "function" ? scope.clients.claim() : undefined)) })
    scope.addEventListener("message", (event) => {
      const message = event.data || {}
      if (message.type === "manja:configure") {
        const pending = configure(message.descriptor, event.source).then(async (result) => {
          if (!result.ok) return result
          ensureRoutingEnabled(() => disabled.has(result.descriptor.publicationKey))
          const cacheName = storageAPI && storageAPI.CACHE_NAME || "manja-local-docs-assets-v1"
          const previousState = await storage.loadMetadata(result.descriptor.publicationKey)
          ensureRoutingEnabled(() => disabled.has(result.descriptor.publicationKey))
          const reenable = previousState && previousState.disabled === true
          if (reenable) {
            const refreshed = await revalidateDescriptor(result.descriptor)
            if (refreshed.kind !== "ready") {
              await notify(event.source, { type: "manja:local-fallback", reason: refreshed.error || "replacement publication unavailable" })
              return { ok: false, reason: refreshed.error || "replacement publication unavailable" }
            }
          }
          const routingDisabled = () => disabled.has(result.descriptor.publicationKey)
          const staticReady = await cacheStaticAssets(scope, cacheName, fetchImplementation, DEFAULT_STATIC_ASSETS, routingDisabled)
          const assetsReady = staticReady && await cacheDescriptorAssets(scope, result.descriptor, descriptorGenerationCacheName(storageAPI, result.descriptor), fetchImplementation, routingDisabled)
          let shellReady = await cacheOfflineShell(storage, result.descriptor, fetchImplementation, routingDisabled, disableRouting)
          if (!shellReady && typeof storage.getShell === "function") {
            try {
              ensureRoutingEnabled(() => disabled.has(result.descriptor.publicationKey))
              const persistedShell = await storage.getShell(result.descriptor.publicationKey, result.descriptor.offlineShellUrl, result.descriptor)
              if (persistedShell) {
                validatePublicShellResponse(persistedShell, result.descriptor, origin)
                await validateShell(persistedShell.clone ? persistedShell.clone() : persistedShell)
                ensureRoutingEnabled(() => disabled.has(result.descriptor.publicationKey))
                shellReady = true
              }
            } catch (_) {}
          }
          if (!assetsReady || !shellReady) {
            await notify(event.source, { type: "manja:local-fallback", reason: "offline shell or fallback asset unavailable" })
            return { ok: false, reason: "offline shell or fallback asset unavailable" }
          }
          try {
            ensureRoutingEnabled(() => disabled.has(result.descriptor.publicationKey))
            const ready = reenable ? { kind: "ready", unchanged: true } : await revalidateDescriptor(result.descriptor)
            await notify(event.source, { type: "manja:local-ready", ...ready })
          } catch (error) {
            await notify(event.source, { type: "manja:local-fallback", reason: String(error && error.message || error).slice(0, 256) })
          }
          return result
        })
        configurations.set(message.descriptor && message.descriptor.publicationKey, pending)
        event.waitUntil(pending.finally(() => configurations.delete(message.descriptor && message.descriptor.publicationKey)))
      }
      else if (message.type === "manja:revalidate") {
        const configured = configurations.get(message.publicationKey)
        const descriptorPromise = configured ? configured.then(() => descriptors.get(message.publicationKey)) : Promise.resolve(descriptors.get(message.publicationKey) || [...descriptors.values()][0])
        event.waitUntil(descriptorPromise.then((descriptor) => {
          if (!descriptor || disabled.has(descriptor.publicationKey)) return undefined
          return revalidateDescriptor(descriptor).then((result) => notify(event.source, { type: "manja:local-ready", ...result })).catch((error) => notify(event.source, { type: "manja:local-fallback", reason: String(error.message || error).slice(0, 256) }))
        }))
      } else if (message.type === "manja:disable") {
        const descriptor = descriptors.get(message.publicationKey)
        if (descriptor) { disableRouting(descriptor.publicationKey); event.waitUntil(disablePublication(storage, descriptor, message.reason || "disabled", message.state || "disabled")) }
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
      const fallbackAsset = findFallbackAsset(descriptors, url)
      if (fallbackAsset && scope.caches) {
        event.respondWith((async () => {
          if (disabled.has(fallbackAsset.descriptor.publicationKey)) return fetchedWhileDisabled(storage, fallbackAsset.descriptor, request, fetchImplementation, disableRouting)
          if (typeof storage.isDisabled === "function" && await storage.isDisabled(fallbackAsset.descriptor.publicationKey)) return fetchImplementation(request)
          return cachedStaticAsset(scope, request, descriptorGenerationCacheName(storageAPI, fallbackAsset.descriptor), fetchImplementation, fallbackAsset.asset, () => disabled.has(fallbackAsset.descriptor.publicationKey))
        })())
        return
      }
      if (isManagementPath(url.pathname)) return
      event.respondWith((async () => {
        const descriptor = await descriptorForURL(url)
        // Unconfigured routes retain the SSR fallback: event.respondWith(fetch(request)).
        if (!descriptor || !isAllowedRequest(request, descriptor, origin)) return fetchImplementation(request)
        if (disabled.has(descriptor.publicationKey)) return fetchedWhileDisabled(storage, descriptor, request, fetchImplementation, disableRouting)
        if (isHTMXRequest(request) && typeof route === "function") {
          return Promise.resolve().then(async () => {
            const routingDisabled = () => disabled.has(descriptor.publicationKey)
            ensureRoutingEnabled(routingDisabled)
            const response = await route(request, descriptor)
            ensureRoutingEnabled(routingDisabled)
            return response
          }).catch(() => cachedOrFetched(scope, storage, descriptor, request, fetchImplementation, () => disabled.has(descriptor.publicationKey), disableRouting))
        }
        return cachedOrFetched(scope, storage, descriptor, request, fetchImplementation, () => disabled.has(descriptor.publicationKey), disableRouting)
      })())
    })
    return { storage, descriptors, configure, revalidate: revalidateDescriptor, disabled }
  }

  return {
    DEFAULT_STATIC_ASSETS,
    DEFAULT_STATIC_ASSET_EXPECTATIONS,
    MAX_ASSET_BYTES,
    MAX_CHILD_BYTES,
    MAX_MANIFEST_BYTES,
    MAX_SPEC_BYTES,
    MAX_SHELL_BYTES,
    ROOT_SCOPE,
    cachedOrFetched,
    cachedStaticAsset,
    cacheOfflineShell,
    cacheStaticAssets,
    commitCandidate,
    createRevalidator,
    descriptorFromMetadata,
    disablePublication,
    isStaticAssetPath,
    isAllowedRequest,
    isHTMXRequest,
    isWithdrawalResponse,
    withdrawalState,
    parseManifest,
    recoverPersistedManifest,
    persistedManifestResponse,
    readBoundedResponse,
    revalidate,
    register,
    sameOriginURL,
    sha256,
    staticAssetExpectation,
    validateDescriptor,
    validateManifestChild,
    validatePublicShellResponse,
    validateShell,
  }
}))
