(function (global) {
  "use strict";

  var MAX_MANIFEST_BYTES = 4 * 1024 * 1024;
  var MAX_PROJECTION_CHILD_BYTES = 2 * 1024 * 1024;
  var MAX_SEARCH_DIRECTORY_BYTES = 4 * 1024 * 1024;
  var DEFAULT_RUNTIME_URL = "/manja-assets/local-docs/wasm_exec.js";
  var DEFAULT_WASM_URL = "/manja-assets/local-docs/manja.wasm";
  var DEFAULT_WORKER_URL = "/manja-assets/local-docs/sw.js";

  function fail(message) {
    throw new Error(message);
  }

  function sameOriginPath(value) {
    if (typeof value !== "string" || value.charAt(0) !== "/" || value.indexOf("\\") !== -1 || value.indexOf("%") !== -1 || value.indexOf("?") !== -1 || value.indexOf("#") !== -1) {
      return null;
    }
    var parsed;
    try {
      parsed = new URL(value, global.location.href);
    } catch (_) {
      if (global.location.protocol !== "about:") {
        return null;
      }
      parsed = new URL(value, "http://manja-local-docs.invalid");
    }
    if (global.location.protocol === "about:") {
      if (parsed.origin !== "http://manja-local-docs.invalid") {
        return null;
      }
    } else if (parsed.origin !== global.location.origin) {
      return null;
    }
    if (parsed.pathname !== value || parsed.search !== "" || parsed.hash !== "") {
      return null;
    }
    return parsed;
  }

  function validBase(value) {
    if (value === "/") {
      return true;
    }
    if (typeof value !== "string" || value.length < 3 || value.charAt(0) !== "/" || value.charAt(value.length - 1) !== "/" || value.indexOf("//") !== -1 || value.indexOf("\\") !== -1 || value.indexOf("%") !== -1 || value.indexOf("?") !== -1 || value.indexOf("#") !== -1) {
      return false;
    }
    var parsed = sameOriginPath(value);
    var pieces = value.split("/").filter(Boolean);
    return Boolean(parsed) && pieces.indexOf("manage") === -1 && pieces.indexOf("api") === -1;
  }

  function publicPath(value) {
    if (typeof value !== "string") return false;
    var pieces = value.split("/").filter(Boolean);
    return pieces.indexOf("manage") === -1 && pieces.indexOf("api") === -1;
  }

  function canonicalIdentity(value) {
    return typeof value === "string" && value.length > 0 && value === value.trim() && !/[\u0000-\u001f\u007f]/.test(value);
  }

  function sha256(value) {
    return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
  }

  function validateDescriptor(descriptor) {
    if (!descriptor || typeof descriptor !== "object" || descriptor.schemaVersion !== 1 || !canonicalIdentity(descriptor.catalogId) || !/^[a-z0-9][a-z0-9._-]{0,63}$/.test(descriptor.catalogId) || typeof descriptor.publicationKey !== "string" || !/^[a-z0-9][a-z0-9._-]{0,63}$/.test(descriptor.publicationKey) || !canonicalIdentity(descriptor.revisionId) || !validBase(descriptor.publicationBase) || descriptor.projectionFormat !== "projection-v2" || !sha256(descriptor.projectionDigest) || descriptor.snapshotId !== "snapshot-sha256-" + descriptor.projectionDigest) {
      fail("descriptor identity is invalid");
    }
    if (descriptor.public !== true || descriptor.anonymous !== true || descriptor.private === true || descriptor.disabled === true || descriptor.eligibility && (descriptor.eligibility.public !== true || descriptor.eligibility.anonymous !== true)) {
      fail("descriptor public eligibility is invalid");
    }
    var base = descriptor.publicationBase + "snapshots/" + descriptor.snapshotId + "/";
    var expected = {
      projectionManifestUrl: base + "manifest.json",
      catalogUrl: base + "catalog.json",
      searchDataBase: base + "search-data/",
      projectionDataBase: base + "projection-data/"
    };
    Object.keys(expected).forEach(function (key) {
      if (descriptor[key] !== expected[key] || !sameOriginPath(descriptor[key])) {
        fail("descriptor route is invalid");
      }
    });
    if (descriptor.offlineShellUrl !== undefined && (!sameOriginPath(descriptor.offlineShellUrl) || !publicPath(descriptor.offlineShellUrl) || descriptor.offlineShellUrl !== descriptor.publicationBase + "_manja/offline-shell")) {
      fail("descriptor offline shell route is invalid");
    }
    if (descriptor.fallbackAssets !== undefined && (!Array.isArray(descriptor.fallbackAssets) || descriptor.fallbackAssets.some(function (asset) {
      return !asset || typeof asset !== "object" || !sameOriginPath(asset.url) || !publicPath(asset.url) || asset.length !== undefined && (!Number.isSafeInteger(asset.length) || asset.length <= 0 || asset.length > 16 * 1024 * 1024) || asset.sha256 !== undefined && !sha256(asset.sha256);
    }))) {
      fail("descriptor fallback asset is invalid");
    }
	if (descriptor.static !== undefined) {
	  var staticValue = descriptor.static;
	  if (!staticValue || typeof staticValue !== "object" || !validBase(staticValue.deploymentBase) || descriptor.publicationBase.indexOf(staticValue.deploymentBase) !== 0 || staticValue.workerUrl !== staticValue.deploymentBase + "sw.js" || staticValue.workerScope !== staticValue.deploymentBase || staticValue.offlineShellUrl !== descriptor.publicationBase + "_manja/offline-shell/" || staticValue.exportManifestUrl !== staticValue.deploymentBase + "_manja/export.json") {
		fail("descriptor static routes are invalid");
	  }
	  [staticValue.workerUrl, staticValue.workerScope, staticValue.offlineShellUrl, staticValue.exportManifestUrl].forEach(function (route) {
		if (!sameOriginPath(route) || !publicPath(route)) fail("descriptor static route is invalid");
	  });
	  descriptor.offlineShellUrl = staticValue.offlineShellUrl;
	}
    return descriptor;
  }

  function canonicalRelativePath(value) {
    if (typeof value !== "string" || value.length === 0 || value.charAt(0) === "/" || value.indexOf("\\") !== -1 || value.indexOf("%") !== -1 || value.indexOf("?") !== -1 || value.indexOf("#") !== -1 || /[\u0000-\u001f\u007f]/.test(value)) {
      return false;
    }
    var segments = value.split("/");
    return segments.every(function (segment) { return segment !== "" && segment !== "." && segment !== ".."; });
  }

  function validProjectionChild(child) {
    if (!child || typeof child !== "object" || !canonicalRelativePath(child.path)) {
      return false;
    }
    var expectedKind = child.path.indexOf("details/") === 0 ? "detail" : child.path.indexOf("schema-nodes/") === 0 ? "schema-node" : "";
    return expectedKind !== "" && child.kind === expectedKind && Number.isSafeInteger(child.length) && child.length > 0 && child.length <= MAX_PROJECTION_CHILD_BYTES && sha256(child.sha256);
  }

  function identityProjectionFormat(identity) {
    return identity && (identity.projectionFormat || identity.versions && identity.versions.projectionFormat);
  }

  function validateManifest(manifest, descriptor) {
    if (!manifest || typeof manifest !== "object" || Array.isArray(manifest) || manifest.schemaVersion !== 1 || manifest.snapshotId !== descriptor.snapshotId || !manifest.identity || typeof manifest.identity !== "object" || Array.isArray(manifest.identity) || manifest.identity.schemaVersion !== 1 || manifest.identity.catalogId !== descriptor.catalogId || manifest.identity.revisionId !== descriptor.revisionId || identityProjectionFormat(manifest.identity) !== descriptor.projectionFormat || !Array.isArray(manifest.children) || manifest.children.length > 10000) {
      fail("manifest identity is invalid");
    }
    var allowedRoot = { schemaVersion: true, snapshotId: true, identity: true, children: true };
    Object.keys(manifest).forEach(function (key) { if (!allowedRoot[key]) fail("manifest field is unknown"); });
    var allowedIdentity = { schemaVersion: true, catalogId: true, catalogTitle: true, branding: true, defaultDocumentKey: true, profileId: true, revisionKind: true, revisionId: true, projectionFormat: true, commitSha: true, sourceManifestSha256: true, profileAllowlistLength: true, profileAllowlistSha256: true, versions: true, bounds: true, sources: true, children: true };
    Object.keys(manifest.identity).forEach(function (key) { if (!allowedIdentity[key]) fail("manifest identity field is unknown"); });
    var seen = Object.create(null);
    manifest.children.forEach(function (child) {
      if (!child || typeof child !== "object" || Array.isArray(child)) fail("manifest child is invalid");
      Object.keys(child).forEach(function (key) { if (key !== "path" && key !== "kind" && key !== "length" && key !== "sha256") fail("manifest child field is unknown"); });
      if (typeof child.path !== "string" || !canonicalRelativePath(child.path) || typeof child.kind !== "string" || child.kind.length === 0 || child.kind.length > 64 || !Number.isSafeInteger(child.length) || child.length <= 0 || child.length > 64 * 1024 * 1024 || !sha256(child.sha256)) fail("manifest child is invalid");
      if (seen[child.path]) fail("manifest child is duplicated");
      seen[child.path] = true;
      var projection = child.path.indexOf("details/") === 0 || child.path.indexOf("schema-nodes/") === 0;
      if (projection && !validProjectionChild(child)) fail("manifest projection child is invalid");
      if (projection) {
        var expectedKind = child.path.indexOf("details/") === 0 ? "detail" : "schema-node";
        if (child.kind !== expectedKind) fail("manifest projection child is invalid");
      } else if (child.path === "catalog.json") {
        if (child.kind !== "catalog") fail("manifest catalog child is invalid");
      } else if (child.path.indexOf("sources/") === 0) {
        if (child.kind !== "source") fail("manifest source child is invalid");
      } else if (child.path.indexOf("support/") === 0) {
        if (child.kind !== "support") fail("manifest support child is invalid");
      } else if (child.path.indexOf("search/") === 0) {
        var maximumSearchBytes = child.path === "search/directory.json" ? MAX_SEARCH_DIRECTORY_BYTES : MAX_PROJECTION_CHILD_BYTES;
        if (child.kind.indexOf("search-") !== 0 || child.length > maximumSearchBytes) fail("manifest search child is invalid");
      } else {
        fail("manifest child route is invalid");
      }
    });
    if (Array.isArray(manifest.identity.children)) {
      if (manifest.identity.children.length !== manifest.children.length) fail("manifest children differ from identity");
      manifest.children.forEach(function (child, index) {
        var identityChild = manifest.identity.children[index];
        if (!identityChild || identityChild.path !== child.path || identityChild.kind !== child.kind || identityChild.length !== child.length || identityChild.sha256 !== child.sha256 || index > 0 && manifest.children[index - 1].path >= child.path) {
          fail("manifest children differ from identity");
        }
      });
    }
    return manifest;
  }

  function parseJSONStrict(text) {
    var index = 0;
    function whitespace() { while (index < text.length && /\s/.test(text.charAt(index))) index += 1; }
    function stringValue() {
      var start = index;
      if (text.charAt(index) !== '"') fail("manifest JSON is invalid");
      index += 1;
      while (index < text.length) {
        var character = text.charAt(index++);
        if (character === "\\") { if (index >= text.length) fail("manifest JSON is invalid"); index += 1; continue; }
        if (character === '"') {
          try { return JSON.parse(text.slice(start, index)); } catch (_) { fail("manifest JSON is invalid"); }
        }
        if (character < " ") fail("manifest JSON is invalid");
      }
      fail("manifest JSON is invalid");
    }
    function value(depth) {
      if (depth > 32) fail("manifest JSON is too deep");
      whitespace();
      var character = text.charAt(index);
      if (character === "{") {
        index += 1; whitespace(); var object = Object.create(null); var keys = Object.create(null);
        if (text.charAt(index) === "}") { index += 1; return object; }
        while (index < text.length) {
          whitespace(); var key = stringValue();
          if (keys[key]) fail("manifest JSON has duplicate keys");
          keys[key] = true; whitespace();
          if (text.charAt(index) !== ":") fail("manifest JSON is invalid");
          index += 1; object[key] = value(depth + 1); whitespace();
          if (text.charAt(index) === "}") { index += 1; return object; }
          if (text.charAt(index) !== ",") fail("manifest JSON is invalid");
          index += 1;
        }
      } else if (character === "[") {
        index += 1; whitespace(); var array = [];
        if (text.charAt(index) === "]") { index += 1; return array; }
        while (index < text.length) {
          array.push(value(depth + 1)); whitespace();
          if (text.charAt(index) === "]") { index += 1; return array; }
          if (text.charAt(index) !== ",") fail("manifest JSON is invalid");
          index += 1;
        }
      } else if (character === '"') {
        return stringValue();
      } else {
        var start = index;
        while (index < text.length && !/[\s,\]}]/.test(text.charAt(index))) index += 1;
        var token = text.slice(start, index);
        if (!/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)$/.test(token)) fail("manifest JSON is invalid");
        return JSON.parse(token);
      }
      fail("manifest JSON is invalid");
    }
    var result = value(0); whitespace();
    if (index !== text.length) fail("manifest JSON has trailing data");
    return result;
  }

  function identityBytes(identity) {
    if (identity.versions === undefined && identity.projectionFormat !== undefined) {
      return new TextEncoder().encode(JSON.stringify({ schemaVersion: identity.schemaVersion, catalogId: identity.catalogId, revisionId: identity.revisionId, projectionFormat: identity.projectionFormat }));
    }
    return new TextEncoder().encode(JSON.stringify({
      schemaVersion: identity.schemaVersion, catalogId: identity.catalogId, catalogTitle: identity.catalogTitle,
      branding: identity.branding, defaultDocumentKey: identity.defaultDocumentKey, profileId: identity.profileId,
      revisionKind: identity.revisionKind, revisionId: identity.revisionId, commitSha: identity.commitSha,
      sourceManifestSha256: identity.sourceManifestSha256, profileAllowlistLength: identity.profileAllowlistLength,
      profileAllowlistSha256: identity.profileAllowlistSha256, versions: identity.versions, bounds: identity.bounds,
      sources: identity.sources, children: identity.children
    }));
  }

  function hexDigest(buffer) {
    return Array.prototype.map.call(new Uint8Array(buffer), function (byte) {
      return byte.toString(16).padStart(2, "0");
    }).join("");
  }

  function fetchWithCache(url, init, cache) {
	return global.fetch(url, init).then(function (response) {
	  return response;
	}).catch(function (error) {
	  if (!cache) throw error;
	  return cache.match(url).then(function (response) { if (!response) throw error; return response; });
	});
  }

  function readManifest(url, descriptor, cache) {
	return fetchWithCache(url.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }, cache).then(function (response) {
      if (!response.ok) {
        fail("manifest request failed");
      }
      var declaredLength = response.headers.get("Content-Length");
      if (declaredLength !== null && (!/^\d+$/.test(declaredLength) || Number(declaredLength) > MAX_MANIFEST_BYTES)) {
        fail("manifest length is invalid");
      }
      return response.arrayBuffer().then(function (bytes) {
        if (bytes.byteLength === 0 || bytes.byteLength > MAX_MANIFEST_BYTES || (declaredLength !== null && Number(declaredLength) !== bytes.byteLength)) {
          fail("manifest length is invalid");
        }
        var text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
        var manifest;
        try {
          manifest = parseJSONStrict(text);
        } catch (_) {
          fail("manifest JSON is invalid");
        }
        validateManifest(manifest, descriptor);
		return global.crypto.subtle.digest("SHA-256", identityBytes(manifest.identity)).then(function (digest) {
          var identityDigest = hexDigest(digest);
          if (identityDigest !== descriptor.projectionDigest) {
            fail("manifest identity digest differs");
          }
		  manifest.identityDigest = identityDigest;
		  return Promise.resolve(cache ? cache.put(url.href, new Response(bytes, { headers: { "Content-Type": "application/json" } })) : undefined).then(function () { return manifest; });
        });
      });
    });
  }

  function loadScript(url) {
    return new Promise(function (resolve, reject) {
      var script = document.createElement("script");
      script.src = url;
      script.async = true;
      script.onload = resolve;
      script.onerror = function () { reject(new Error("Wasm runtime asset failed")); };
      document.head.appendChild(script);
    });
  }

  function loadABI(options) {
    if (typeof options.loadABI === "function") {
      return Promise.resolve().then(options.loadABI);
    }
    if (global.ManjaLocalDocs && typeof global.ManjaLocalDocs.activate === "function") {
      return Promise.resolve(global.ManjaLocalDocs);
    }
    var runtimeURL = options.runtimeURL || DEFAULT_RUNTIME_URL;
    var wasmURL = options.wasmURL || DEFAULT_WASM_URL;
    var runtimePath = sameOriginPath(runtimeURL);
    var wasmPath = sameOriginPath(wasmURL);
    if (!runtimePath || !wasmPath) {
      return Promise.reject(new Error("Wasm asset route is invalid"));
    }
    return loadScript(runtimePath.href).then(function () {
      if (typeof global.Go !== "function") {
        fail("Wasm runtime is unavailable");
      }
      var go = new global.Go();
      return global.fetch(wasmPath.href, { credentials: "same-origin", cache: "no-store" }).then(function (response) {
        if (!response.ok) {
          fail("Wasm asset request failed");
        }
        return response.arrayBuffer();
      }).then(function (bytes) {
        return WebAssembly.instantiate(bytes, go.importObject).then(function (result) {
          go.run(result.instance);
          return new Promise(function (resolve, reject) {
            var started = Date.now();
            (function waitForABI() {
              if (global.ManjaLocalDocs && typeof global.ManjaLocalDocs.activate === "function") {
                resolve(global.ManjaLocalDocs);
                return;
              }
              if (Date.now() - started > 2000) {
                reject(new Error("Wasm ABI did not activate"));
                return;
              }
              setTimeout(waitForABI, 10);
            }());
          });
        });
      });
    });
  }

  function mark(root, state, reason) {
    if (state === "ready" && root.dataset.manjaLocalDocsWorker === "fallback") {
      state = "fallback";
      reason = root.dataset.manjaLocalDocsWorkerReason || reason || "worker unavailable";
    }
    root.dataset.manjaLocalDocsState = state;
    if (state === "ready") {
      root.dataset.manjaLocalDocsReady = "true";
      delete root.dataset.manjaLocalDocsFallback;
    } else {
      root.dataset.manjaLocalDocsFallback = "true";
      delete root.dataset.manjaLocalDocsReady;
    }
    if (reason) {
      root.dataset.manjaLocalDocsReason = reason;
    }
    root.dispatchEvent(new CustomEvent("manja:local-" + state, { detail: reason ? { reason: reason } : {} }));
  }

  function announceWorkerMessage(root, event) {
    var message = event && event.data;
    if (!message || typeof message.type !== "string") return;
    if (message.type === "manja:local-ready") {
      root.dataset.manjaLocalDocsWorker = "ready";
      root.dispatchEvent(new CustomEvent("manja:local-ready", { bubbles: true, detail: message }));
    } else if (message.type === "manja:local-fallback") {
      root.dataset.manjaLocalDocsWorker = "fallback";
      root.dataset.manjaLocalDocsWorkerReason = String(message.reason || "worker unavailable").slice(0, 256);
      mark(root, "fallback", root.dataset.manjaLocalDocsWorkerReason);
    }
  }

  function descriptorFromDocument(documentValue) {
    var script = documentValue.getElementById("manja-local-docs-descriptor");
    if (!script) {
      return null;
    }
    var descriptor;
    try {
      descriptor = JSON.parse(script.textContent || "");
    } catch (_) {
      fail("descriptor JSON is invalid");
    }
    return validateDescriptor(descriptor);
  }

  function validateActivation(result, descriptor) {
    if (!result || result.ok !== true || result.catalogId !== descriptor.catalogId || result.publicationKey !== descriptor.publicationKey || result.snapshotId !== descriptor.snapshotId || result.revisionId !== descriptor.revisionId || result.projectionDigest !== descriptor.projectionDigest) {
      fail("Wasm ABI activation identity differs");
    }
    return result;
  }

  function workerFromNavigator() {
    return global.navigator && global.navigator.serviceWorker;
  }

  function workerTarget(serviceWorker, registration) {
    return registration && (registration.active || registration.waiting || registration.installing) || serviceWorker.controller;
  }

  function registerWorker(root, descriptor, options) {
    options = options || {};
    var serviceWorker = workerFromNavigator();
    if (!serviceWorker || typeof serviceWorker.register !== "function") {
      root.dataset.manjaLocalDocsWorker = "unsupported";
      return Promise.resolve({ skipped: true, reason: "service worker unsupported" });
    }
    var workerURL = sameOriginPath(options.workerURL || DEFAULT_WORKER_URL);
    if (!workerURL) {
      return Promise.reject(new Error("Service Worker asset route is invalid"));
    }
	var scope = options.scope || "/";
	if (!validBase(scope)) {
	  return Promise.reject(new Error("Service Worker scope is invalid"));
    }
    if (!root.__manjaLocalDocsWorkerListener && typeof serviceWorker.addEventListener === "function") {
      serviceWorker.addEventListener("message", function (event) { announceWorkerMessage(root, event); });
      root.__manjaLocalDocsWorkerListener = true;
    }
	return Promise.resolve(serviceWorker.register(workerURL.href, { scope: scope })).then(function (registration) {
      return Promise.resolve(serviceWorker.ready || registration).then(function (ready) {
        var target = workerTarget(serviceWorker, ready);
        if (!target || typeof target.postMessage !== "function") {
          throw new Error("Service Worker did not become active");
        }
        target.postMessage({ type: "manja:configure", descriptor: descriptor });
        root.dataset.manjaLocalDocsWorker = "registered";
        delete root.dataset.manjaLocalDocsWorkerReason;
        return { ok: true, registration: ready };
      });
    }).catch(function (error) {
      root.dataset.manjaLocalDocsWorker = "fallback";
      root.dataset.manjaLocalDocsWorkerReason = error && error.message ? error.message : "Service Worker registration failed";
      throw error;
    });
  }

  function staticCacheName(descriptor) {
	return "manja-local-docs-assets-v1::" + encodeURIComponent(descriptor.publicationKey) + "::" + encodeURIComponent(descriptor.revisionId) + "::" + descriptor.projectionDigest;
  }

  function manifestChild(manifest, path) {
	for (var index = 0; index < manifest.children.length; index += 1) if (manifest.children[index].path === path) return manifest.children[index];
	return null;
  }

  function childURL(descriptor, child) {
	if (child.path.indexOf("search/") === 0) return descriptor.searchDataBase + child.path;
	if (child.path.indexOf("details/") === 0 || child.path.indexOf("schema-nodes/") === 0) return descriptor.projectionDataBase + child.path;
	return "";
  }

  function loadStaticChild(descriptor, manifest, cache, children, childPath) {
	if (Object.prototype.hasOwnProperty.call(children, childPath)) return Promise.resolve(children[childPath]);
	var identity = manifestChild(manifest, childPath);
	var url = identity && childURL(descriptor, identity);
	if (!url) return Promise.reject(new Error('static child "' + childPath + '" is not declared'));
	return readVerifiedJSON(url, identity, cache).then(function (value) {
	  children[childPath] = value;
	  return value;
	}).catch(function (error) {
	  throw new Error('static child "' + childPath + '" failed: ' + (error && error.message ? error.message : "request failed"));
	});
  }

  function staticCatalogDocument(catalog, key) {
	if (!catalog || !Array.isArray(catalog.documents)) fail("static catalog document inventory is invalid");
	for (var index = 0; index < catalog.documents.length; index += 1) if (catalog.documents[index].key === key) return catalog.documents[index];
	fail("static catalog document is missing");
  }

  function staticRouteSelection(documentValue, selected) {
	var groups = [documentValue.operations, documentValue.schemas];
	for (var groupIndex = 0; groupIndex < groups.length; groupIndex += 1) {
	  var group = groups[groupIndex];
	  if (!Array.isArray(group)) continue;
	  for (var index = 0; index < group.length; index += 1) if (group[index].detailId === selected) return { directory: group[index], schema: groupIndex === 1 };
	}
	fail("static route detail is missing");
  }

  function staticSchemaShard(documentValue, ordinal) {
	if (!Array.isArray(documentValue.schemaNodeShards)) fail("static schema-node inventory is invalid");
	for (var index = 0; index < documentValue.schemaNodeShards.length; index += 1) {
	  var shard = documentValue.schemaNodeShards[index];
	  if (Number.isSafeInteger(shard.firstOrdinal) && Number.isSafeInteger(shard.lastOrdinal) && shard.firstOrdinal <= ordinal && ordinal <= shard.lastOrdinal) return shard;
	}
	fail("static schema-node shard is missing");
  }

  function operationSchemaReferences(detail) {
	var references = [];
	function add(value) {
	  if (!Number.isSafeInteger(value) || value < 0) fail("static operation schema reference is invalid");
	  if (references.indexOf(value) < 0) references.push(value);
	}
	(Array.isArray(detail.parameters) ? detail.parameters : []).forEach(function (parameter) { add(parameter.schemaRef); });
	if (detail.hasRequestBody && detail.requestBody && Array.isArray(detail.requestBody.mediaTypes)) {
	  detail.requestBody.mediaTypes.forEach(function (media) { add(media.schemaRef); });
	}
	(Array.isArray(detail.responses) ? detail.responses : []).forEach(function (response) {
	  (Array.isArray(response.headers) ? response.headers : []).forEach(function (header) { add(header.schemaRef); });
	  (Array.isArray(response.mediaTypes) ? response.mediaTypes : []).forEach(function (media) { add(media.schemaRef); });
	});
	return references;
  }

  // Keep operation navigation incremental: only schema shards reachable from
  // the selected operation are admitted, with the same depth/node budget as
  // the Go renderer instead of eagerly loading a large catalog.
  function hydrateOperationSchemaGraph(descriptor, manifest, cache, children, documentValue, detail) {
	var queue = operationSchemaReferences(detail).map(function (ordinal) { return { ordinal: ordinal, depth: 0 }; });
	var seen = Object.create(null);
	var loaded = 0;
	function visit() {
	  while (queue.length > 0) {
		var item = queue.shift();
		var key = String(item.ordinal);
		if (seen[key]) continue;
		seen[key] = true;
		loaded += 1;
		if (loaded > 256) return Promise.resolve(children);
		var shard = staticSchemaShard(documentValue, item.ordinal);
		return loadStaticChild(descriptor, manifest, cache, children, shard.path).then(function (value) {
		  if (!value || !Array.isArray(value.nodes)) fail("static schema-node shard is invalid");
		  var node = null;
		  for (var index = 0; index < value.nodes.length; index += 1) {
			if (value.nodes[index] && value.nodes[index].ordinal === item.ordinal) { node = value.nodes[index]; break; }
		  }
		  if (!node) fail("static operation schema node is missing");
		  if (item.depth < 4) {
			(Array.isArray(node.properties) ? node.properties : []).concat(Array.isArray(node.items) ? node.items : []).forEach(function (reference) {
			  if (!reference || !Number.isSafeInteger(reference.schemaRef) || reference.schemaRef < 0) fail("static operation schema reference is invalid");
			  var referenceKey = String(reference.schemaRef);
			  if (!seen[referenceKey]) queue.push({ ordinal: reference.schemaRef, depth: item.depth + 1 });
			});
		  }
		  return visit();
		});
	  }
	  return Promise.resolve(children);
	}
	return visit();
  }

  function hydrateStaticRoute(descriptor, manifest, catalog, cache, children, route) {
	if (!route.selected) return Promise.resolve(children);
	var documentValue = staticCatalogDocument(catalog, route.documentKey);
	var selected = staticRouteSelection(documentValue, route.selected);
	return loadStaticChild(descriptor, manifest, cache, children, selected.directory.detailChild).then(function (detailShard) {
	  if (!selected.schema) {
		if (!detailShard || !Array.isArray(detailShard.records)) fail("static detail shard is invalid");
		var operation = null;
		for (var operationIndex = 0; operationIndex < detailShard.records.length; operationIndex += 1) {
		  if (detailShard.records[operationIndex].id === route.selected) operation = detailShard.records[operationIndex].operation;
		}
		if (!operation) fail("static operation detail is missing");
		return hydrateOperationSchemaGraph(descriptor, manifest, cache, children, documentValue, operation);
	  }
	  if (!detailShard || !Array.isArray(detailShard.records)) fail("static detail shard is invalid");
	  var detail = null;
	  for (var index = 0; index < detailShard.records.length; index += 1) if (detailShard.records[index].id === route.selected) detail = detailShard.records[index];
	  var ordinal = route.node !== undefined ? route.node : detail && detail.schema && detail.schema.schemaRef;
	  if (!Number.isSafeInteger(ordinal) || ordinal < 0) fail("static schema-node route is invalid");
	  var selectedShard = staticSchemaShard(documentValue, ordinal);
	  return loadStaticChild(descriptor, manifest, cache, children, selectedShard.path).then(function (shard) {
		if (!shard || !Array.isArray(shard.nodes)) fail("static schema-node shard is invalid");
		var node = null;
		for (var nodeIndex = 0; nodeIndex < shard.nodes.length; nodeIndex += 1) if (shard.nodes[nodeIndex].ordinal === ordinal) node = shard.nodes[nodeIndex];
		if (!node) fail("static schema node is missing");
		var references = [];
		;(Array.isArray(node.properties) ? node.properties : []).concat(Array.isArray(node.items) ? node.items : []).forEach(function (reference) {
		  if (!Number.isSafeInteger(reference.schemaRef) || reference.schemaRef < 0) fail("static schema-node reference is invalid");
		  var path = staticSchemaShard(documentValue, reference.schemaRef).path;
		  if (references.indexOf(path) < 0) references.push(path);
		});
		return references.reduce(function (pending, childPath) {
		  return pending.then(function () { return loadStaticChild(descriptor, manifest, cache, children, childPath); });
		}, Promise.resolve()).then(function () { return children; });
	  });
	});
  }

  function readVerifiedJSON(url, identity, cache) {
	var parsed = sameOriginPath(url);
	if (!parsed || !identity || !Number.isSafeInteger(identity.length) || identity.length <= 0 || !sha256(identity.sha256)) return Promise.reject(new Error("static child identity is invalid"));
	return fetchWithCache(parsed.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }, cache).then(function (response) {
	  if (!response.ok) fail("static child request failed");
	  return response.arrayBuffer();
	}).then(function (bytes) {
	  if (bytes.byteLength !== identity.length) fail("static child length differs");
	  return global.crypto.subtle.digest("SHA-256", bytes).then(function (digest) {
		if (hexDigest(digest) !== identity.sha256) fail("static child digest differs");
		var object = parseJSONStrict(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
		return Promise.resolve(cache.put(parsed.href, new Response(bytes, { headers: { "Content-Type": "application/json" } }))).then(function () { return object; });
	  });
	});
  }

  function readExportIdentity(descriptor, cache) {
	var url = sameOriginPath(descriptor.static.deploymentBase + "_manja/identity.json");
	if (!url) return Promise.reject(new Error("export identity route is invalid"));
	return fetchWithCache(url.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }, cache).then(function (response) {
	  if (!response.ok) fail("export identity request failed");
	  return response.text();
	}).then(function (text) {
	  var identity = parseJSONStrict(text);
	  var keys = identity && typeof identity === "object" && !Array.isArray(identity) ? Object.keys(identity).sort() : [];
	  if (!identity || keys.join(",") !== "basePath,catalogs,schemaVersion" || identity.schemaVersion !== 1 || identity.basePath !== descriptor.static.deploymentBase || !Array.isArray(identity.catalogs)) fail("export identity is invalid");
	  var matched = identity.catalogs.some(function (catalog) {
		return catalog && catalog.catalogId === descriptor.catalogId && catalog.publicationKey === descriptor.publicationKey && catalog.revisionId === descriptor.revisionId && catalog.snapshotId === descriptor.snapshotId;
	  });
	  if (!matched) fail("export identity catalog differs");
	  return Promise.resolve(cache.put(url.href, new Response(text, { headers: { "Content-Type": "application/json" } }))).then(function () { return identity; });
	});
  }

  function staticRoute(descriptor, href) {
	var value;
	try { value = new URL(href, global.location.href); } catch (_) { return null; }
	if (value.origin !== global.location.origin || value.pathname.indexOf(descriptor.publicationBase + "documents/") !== 0) return null;
	var relative = value.pathname.slice((descriptor.publicationBase + "documents/").length);
	var pieces = relative.split("/").filter(Boolean);
	if (pieces.length !== 1) return null;
	var node = value.searchParams.get("node");
	return {
	  documentKey: pieces[0],
	  selected: value.searchParams.get("selected") || "",
	  node: node !== null && /^\d+$/.test(node) ? Number(node) : undefined,
	  groups: value.searchParams.getAll("group").filter(Boolean),
	  closedGroups: value.searchParams.getAll("closed").filter(Boolean),
	};
  }

  function fragmentResourceKey(kind, resource) {
	var fields = ["manja.html.fragment.resource.v2", "manja-html-fragment-v2", kind, resource];
	var encoder = new TextEncoder();
	var encoded = fields.map(function (field) { return encoder.encode(field); });
	var length = encoded.reduce(function (total, value) { return total + 4 + value.byteLength; }, 0);
	var buffer = new ArrayBuffer(length);
	var view = new DataView(buffer);
	var bytes = new Uint8Array(buffer);
	var offset = 0;
	encoded.forEach(function (value) {
	  view.setUint32(offset, value.byteLength, false);
	  offset += 4;
	  bytes.set(value, offset);
	  offset += value.byteLength;
	});
	return global.crypto.subtle.digest("SHA-256", buffer).then(function (digest) {
	  return kind + "-sha256-" + hexDigest(digest);
	});
  }

  function staticRouteCanonical(descriptor, route) {
	var pathname = descriptor.publicationBase + "documents/" + encodeURIComponent(route.documentKey) + "/";
	var parameters = new URLSearchParams();
	(route.groups || []).forEach(function (group) { parameters.append("group", group); });
	(route.closedGroups || []).forEach(function (group) { parameters.append("closed", group); });
	if (route.selected) parameters.set("selected", route.selected);
	if (route.node !== undefined) parameters.set("node", String(route.node));
	var query = parameters.toString();
	var fragment = route.node !== undefined ? "schema-node-panel" : route.selected || "";
	return pathname + (query ? "?" + query : "") + (fragment ? "#" + encodeURIComponent(fragment) : "");
  }

  function readStaticHTMLFragmentKind(descriptor, cache, route, kind) {
	if (!route.selected) return Promise.reject(new Error("static fragment selection is missing"));
	var directory = kind === "schema" ? "schemas" : "operations";
	return fragmentResourceKey(kind, route.selected).then(function (resourceKey) {
	  var base = descriptor.publicationBase + "documents/" + encodeURIComponent(route.documentKey) + "/_manja/fragments/" + directory + "/" + resourceKey + ".html";
	  var htmlURL = sameOriginPath(base);
	  var sidecarURL = sameOriginPath(base + ".meta.json");
	  if (!htmlURL || !sidecarURL) fail("static fragment route is invalid");
	  return fetchWithCache(sidecarURL.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }, cache).then(function (response) {
		if (!response.ok) fail("static fragment sidecar request failed");
		return response.arrayBuffer();
	  }).then(function (bytes) {
		if (!bytes.byteLength || bytes.byteLength > 64 * 1024) fail("static fragment sidecar length is invalid");
		var sidecar = parseJSONStrict(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
		if (!sidecar || sidecar.schemaVersion !== 1 || !sidecar.fragment || sidecar.fragment.format !== "manja-html-fragment-v2" ||
			sidecar.fragment.kind !== kind || sidecar.fragment.resource !== route.selected || typeof sidecar.buildKey !== "string" ||
			sidecar.buildKey.indexOf("fragment-build-sha256:") !== 0 || !sidecar.content || !Number.isSafeInteger(sidecar.content.length) ||
			sidecar.content.length < 0 || !sha256(sidecar.content.sha256)) {
		  fail("static fragment sidecar is invalid");
		}
		return fetchWithCache(htmlURL.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "text/html" } }, cache).then(function (response) {
		  if (!response.ok) fail("static fragment request failed");
		  return response.arrayBuffer();
		}).then(function (htmlBytes) {
		  if (htmlBytes.byteLength !== sidecar.content.length) fail("static fragment length differs");
		  return global.crypto.subtle.digest("SHA-256", htmlBytes).then(function (digest) {
			var contentDigest = hexDigest(digest);
			if (contentDigest !== sidecar.content.sha256) fail("static fragment digest differs");
			if (kind === "schema" && route.selected.indexOf("tree-sha256-") === 0 && route.selected.slice("tree-sha256-".length) !== contentDigest) fail("static schema resource identity differs");
			var htmlValue = new TextDecoder("utf-8", { fatal: true }).decode(htmlBytes);
			var title = route.selected;
			if (typeof global.DOMParser === "function") {
			  var parsedHTML = new global.DOMParser().parseFromString(htmlValue, "text/html");
			  var heading = parsedHTML && parsedHTML.querySelector("h1, h2");
			  if (heading && heading.textContent && heading.textContent.trim()) title = heading.textContent.trim();
			}
			return Promise.all([
			  cache.put(htmlURL.href, new Response(htmlBytes, { headers: { "Content-Type": "text/html" } })),
			  cache.put(sidecarURL.href, new Response(bytes, { headers: { "Content-Type": "application/json" } }))
			]).then(function () {
			  return { ok: true, mainHtml: htmlValue, title: title, canonical: staticRouteCanonical(descriptor, route) };
			});
		  });
		});
	  });
	});
  }

  function readStaticHTMLFragment(descriptor, cache, route) {
	return readStaticHTMLFragmentKind(descriptor, cache, route, "operation").catch(function (operationError) {
	  return readStaticHTMLFragmentKind(descriptor, cache, route, "schema").catch(function () { throw operationError; });
	});
  }

  var lazySchemaInstance = 0;

  function scopeLazySchemaIDs(root) {
	if (!root || !root.querySelectorAll) return;
	lazySchemaInstance += 1;
	var prefix = "manja-lazy-schema-" + lazySchemaInstance + "-";
	var ids = Object.create(null);
	var identified = root.querySelectorAll("[id]");
	for (var index = 0; index < identified.length; index += 1) {
	  var oldID = identified[index].getAttribute("id");
	  if (!oldID) continue;
	  ids[oldID] = prefix + oldID;
	  identified[index].setAttribute("id", ids[oldID]);
	}
	var references = root.querySelectorAll("[href^='#'], [aria-controls], [aria-describedby], [aria-labelledby], [data-tooltip-content-id], [for]");
	for (var referenceIndex = 0; referenceIndex < references.length; referenceIndex += 1) {
	  var node = references[referenceIndex];
	  ["aria-controls", "aria-describedby", "aria-labelledby", "data-tooltip-content-id", "for"].forEach(function (name) {
		var value = node.getAttribute(name);
		if (!value) return;
		node.setAttribute(name, value.split(/\s+/).map(function (part) { return ids[part] || part; }).join(" "));
	  });
	  var href = node.getAttribute("href");
	  if (href && href.charAt(0) === "#" && ids[href.slice(1)]) node.setAttribute("href", "#" + ids[href.slice(1)]);
	}
  }

  function installLazySchemaFragments(descriptor, cache, route, root) {
	if (!root || !root.querySelectorAll) return function () {};
	var placeholders = root.querySelectorAll('[data-manja-static-schema-fragment="true"]');
	if (!placeholders.length) return function () {};
	var active = true;
	var retryBindings = [];
	var observer = typeof global.IntersectionObserver === "function" ? new global.IntersectionObserver(function (entries) {
	  entries.forEach(function (entry) {
		if (!active || !entry.isIntersecting) return;
		observer.unobserve(entry.target);
		load(entry.target);
	  });
	}, { rootMargin: "256px 0px" }) : null;
	function load(placeholder) {
	  if (!active || !placeholder || placeholder.getAttribute("data-manja-schema-state") === "loading" || placeholder.getAttribute("data-manja-schema-state") === "ready") return;
	  var resource = placeholder.getAttribute("data-manja-schema-resource") || "";
	  if (!/^tree-sha256-[0-9a-f]{64}$/.test(resource)) {
		placeholder.setAttribute("data-manja-schema-state", "error");
		return;
	  }
	  placeholder.setAttribute("data-manja-schema-state", "loading");
	  placeholder.setAttribute("aria-busy", "true");
	  var schemaRoute = { documentKey: route.documentKey, selected: resource, groups: [], closedGroups: [] };
	  readStaticHTMLFragmentKind(descriptor, cache, schemaRoute, "schema").then(function (fragment) {
		if (!active || placeholder.isConnected === false) return;
		placeholder.innerHTML = fragment.mainHtml;
		scopeLazySchemaIDs(placeholder);
		placeholder.setAttribute("data-manja-schema-state", "ready");
		placeholder.setAttribute("aria-busy", "false");
		if (global.htmx && typeof global.htmx.process === "function") global.htmx.process(placeholder);
	  }).catch(function () {
		if (!active || placeholder.isConnected === false) return;
		placeholder.setAttribute("data-manja-schema-state", "error");
		placeholder.setAttribute("aria-busy", "false");
		var retry = placeholder.querySelector && placeholder.querySelector("[data-manja-schema-retry]");
		if (retry) retry.hidden = false;
	  });
	}
	for (var index = 0; index < placeholders.length; index += 1) {
	  (function (placeholder) {
		var retry = placeholder.querySelector && placeholder.querySelector("[data-manja-schema-retry]");
		if (retry && retry.addEventListener) {
		  var retryHandler = function () { load(placeholder); };
		  retry.addEventListener("click", retryHandler);
		  retryBindings.push([retry, retryHandler]);
		}
		if (observer) observer.observe(placeholder); else load(placeholder);
	  }(placeholders[index]));
	}
	return function () {
	  active = false;
	  if (observer) observer.disconnect();
	  retryBindings.forEach(function (binding) { binding[0].removeEventListener("click", binding[1]); });
	  retryBindings = [];
	};
  }

  function readStaticSidebarChunk(descriptor, cache, documentKey, collection, chunk) {
	var resource = documentKey + ":" + collection + ":" + chunk;
	return fragmentResourceKey("sidebar", resource).then(function (resourceKey) {
	  var base = descriptor.publicationBase + "documents/" + encodeURIComponent(documentKey) + "/_manja/fragments/sidebar/operations/" + resourceKey + ".html";
	  var htmlURL = sameOriginPath(base);
	  var sidecarURL = sameOriginPath(base + ".meta.json");
	  if (!htmlURL || !sidecarURL) fail("static sidebar route is invalid");
	  return fetchWithCache(sidecarURL.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }, cache).then(function (response) {
		if (response.status === 404) return null;
		if (!response.ok) fail("static sidebar sidecar request failed");
		return response.arrayBuffer().then(function (sidecarBytes) {
		  if (!sidecarBytes.byteLength || sidecarBytes.byteLength > 64 * 1024) fail("static sidebar sidecar length is invalid");
		  var sidecar = parseJSONStrict(new TextDecoder("utf-8", { fatal: true }).decode(sidecarBytes));
		  if (!sidecar || sidecar.schemaVersion !== 1 || !sidecar.fragment || sidecar.fragment.format !== "manja-html-fragment-v2" ||
			  sidecar.fragment.kind !== "sidebar" || sidecar.fragment.resource !== resource || !sidecar.content ||
			  !Number.isSafeInteger(sidecar.content.length) || sidecar.content.length < 0 || !sha256(sidecar.content.sha256)) fail("static sidebar sidecar is invalid");
		  return fetchWithCache(htmlURL.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "text/html" } }, cache).then(function (htmlResponse) {
			if (!htmlResponse.ok) fail("static sidebar request failed");
			return htmlResponse.arrayBuffer();
		  }).then(function (htmlBytes) {
			if (htmlBytes.byteLength !== sidecar.content.length) fail("static sidebar length differs");
			return global.crypto.subtle.digest("SHA-256", htmlBytes).then(function (digest) {
			  if (hexDigest(digest) !== sidecar.content.sha256) fail("static sidebar digest differs");
			  return Promise.all([
				cache.put(htmlURL.href, new Response(htmlBytes, { headers: { "Content-Type": "text/html" } })),
				cache.put(sidecarURL.href, new Response(sidecarBytes, { headers: { "Content-Type": "application/json" } }))
			  ]).then(function () { return new TextDecoder("utf-8", { fatal: true }).decode(htmlBytes); });
			});
		  });
		});
	  });
	});
  }

  function installStaticSidebarContinuation(descriptor, cache, documentKey, sidebar) {
	var navigation = sidebar && sidebar.querySelector && sidebar.querySelector('nav[data-manja-local-sidebar="true"]');
	if (!navigation || !navigation.querySelectorAll) return function () {};
	var active = true;
	var observer = null;
	var scheduled = false;
	function visible(marker) {
	  var panel = marker && marker.closest && marker.closest("[data-manja-sidebar-tab-panel]");
	  if (panel && panel.hidden) return false;
	  var groupItems = marker && marker.closest && marker.closest("[data-manja-sidebar-items]");
	  if (groupItems && groupItems.hidden) return false;
	  if (!marker || !marker.getBoundingClientRect || !navigation.getBoundingClientRect) return true;
	  var markerBox = marker.getBoundingClientRect();
	  var rootBox = navigation.getBoundingClientRect();
	  return markerBox.bottom >= rootBox.top - 256 && markerBox.top <= rootBox.bottom + 256;
	}
	function failMarker(marker) {
	  if (!active || !marker || marker.isConnected === false) return;
	  marker.setAttribute("aria-hidden", "false");
	  marker.setAttribute("data-manja-sidebar-state", "error");
	  marker.innerHTML = '<button type="button" data-manja-sidebar-retry="true">Retry loading more items</button>';
	  var retry = marker.querySelector && marker.querySelector("[data-manja-sidebar-retry]");
	  if (retry && retry.addEventListener) retry.addEventListener("click", function () {
		marker.innerHTML = "";
		marker.setAttribute("aria-hidden", "true");
		marker.removeAttribute("data-manja-sidebar-state");
		load(marker);
	  }, { once: true });
	}
	function load(marker) {
	  if (!active || !marker || marker.getAttribute("data-manja-sidebar-state")) return;
	  var collection = marker.getAttribute("data-manja-sidebar-collection");
	  var chunkValue = marker.getAttribute("data-manja-sidebar-chunk") || "";
	  var operationGroup = /^operation-group-group-[0-9a-f]{12}$/.test(collection || "");
	  if ((collection !== "operations" && collection !== "schemas" && !operationGroup) || !/^\d+$/.test(chunkValue) || Number(chunkValue) < (operationGroup ? 0 : 1)) {
		failMarker(marker);
		return;
	  }
	  marker.setAttribute("data-manja-sidebar-state", "loading");
	  if (observer) observer.unobserve(marker);
	  readStaticSidebarChunk(descriptor, cache, documentKey, collection, Number(chunkValue)).then(function (html) {
		if (!active || marker.isConnected === false) return;
		if (!html) {
		  marker.remove();
		  return;
		}
		var parent = marker.parentNode;
		marker.insertAdjacentHTML("beforebegin", html);
		marker.remove();
		coalesceStaticSidebarGroups(parent);
		applyStaticSidebarGroupState(parent, staticRoute(descriptor, global.location.href));
		if (global.htmx && typeof global.htmx.process === "function" && parent) global.htmx.process(parent);
		schedule();
	  }).catch(function () { failMarker(marker); });
	}
	function scan() {
	  scheduled = false;
	  if (!active) return;
	  var markers = navigation.querySelectorAll('[data-manja-sidebar-next-chunk="true"]');
	  for (var index = 0; index < markers.length; index += 1) {
		var marker = markers[index];
		if (marker.getAttribute("data-manja-sidebar-state")) continue;
		if (observer) observer.observe(marker);
		if (visible(marker)) load(marker);
	  }
	}
	function schedule() {
	  if (!active || scheduled) return;
	  scheduled = true;
	  if (typeof global.requestAnimationFrame === "function") global.requestAnimationFrame(scan); else global.setTimeout(scan, 0);
	}
	if (typeof global.IntersectionObserver === "function") observer = new global.IntersectionObserver(function (entries) {
	  entries.forEach(function (entry) { if (entry.isIntersecting) load(entry.target); });
	}, { root: navigation, rootMargin: "256px 0px" });
	if (navigation.addEventListener) navigation.addEventListener("scroll", schedule, { passive: true });
	if (sidebar.addEventListener) sidebar.addEventListener("manja:sidebar-tab", schedule);
	if (sidebar.addEventListener) sidebar.addEventListener("manja:sidebar-group", schedule);
	schedule();
	return function () {
	  active = false;
	  if (observer) observer.disconnect();
	  if (navigation.removeEventListener) navigation.removeEventListener("scroll", schedule);
	  if (sidebar.removeEventListener) sidebar.removeEventListener("manja:sidebar-tab", schedule);
	  if (sidebar.removeEventListener) sidebar.removeEventListener("manja:sidebar-group", schedule);
	};
  }

  function coalesceStaticSidebarGroups(root) {
	if (!root || !root.querySelectorAll) return;
	var firstByID = Object.create(null);
	var groups = Array.prototype.slice.call(root.querySelectorAll("section[data-manja-sidebar-group]"));
	groups.forEach(function (group) {
	  var id = group.getAttribute("data-manja-sidebar-group") || "";
	  if (!id || !firstByID[id]) {
		if (id) firstByID[id] = group;
		return;
	  }
	  var target = firstByID[id].querySelector("[data-manja-sidebar-items] ul");
	  var source = group.querySelector("[data-manja-sidebar-items] ul");
	  if (!target || !source) return;
	  while (source.firstChild) target.appendChild(source.firstChild);
	  group.remove();
	});
  }

  function applyStaticSidebarGroupState(root, route) {
	if (!root || !root.querySelectorAll || !route) return;
	var opened = route.groups || [];
	var closed = route.closedGroups || [];
	var groups = root.querySelectorAll("section[data-manja-sidebar-group]");
	for (var index = 0; index < groups.length; index += 1) {
	  var id = groups[index].getAttribute("data-manja-sidebar-group") || "";
	  var control = groups[index].querySelector("[data-manja-static-group]");
	  var items = groups[index].querySelector("[data-manja-sidebar-items]");
	  var explicit = opened.length > 0 || closed.length > 0;
	  var expanded = closed.indexOf(id) < 0 && (opened.indexOf(id) >= 0 || !explicit && index === 0);
	  if (control) control.setAttribute("aria-expanded", expanded ? "true" : "false");
	  if (items) items.hidden = !expanded;
	}
  }

  function expandedStaticSidebarGroups(root) {
	if (!root || !root.querySelectorAll) return [];
	var result = [];
	var controls = root.querySelectorAll("[data-manja-static-group]");
	for (var index = 0; index < controls.length; index += 1) {
	  if (controls[index].getAttribute("aria-expanded") !== "true") continue;
	  var id = controls[index].getAttribute("data-manja-static-group") || "";
	  if (id && result.indexOf(id) < 0) result.push(id);
	}
	return result;
  }

  function installStaticSidebarTabs(sidebar) {
	if (!sidebar || !sidebar.querySelectorAll) return;
	var tabs = Array.prototype.slice.call(sidebar.querySelectorAll('[role="tab"][data-manja-sidebar-tab]'));
	if (!tabs.length) return;
	function select(tab) {
	  var selected = tab.getAttribute("data-manja-sidebar-tab");
	  tabs.forEach(function (candidate) {
		var active = candidate === tab;
		candidate.setAttribute("aria-selected", active ? "true" : "false");
		candidate.setAttribute("tabindex", active ? "0" : "-1");
	  });
	  var panels = sidebar.querySelectorAll("[data-manja-sidebar-tab-panel]");
	  for (var index = 0; index < panels.length; index += 1) panels[index].hidden = panels[index].getAttribute("data-manja-sidebar-tab-panel") !== selected;
	  if (sidebar.dispatchEvent && typeof global.CustomEvent === "function") sidebar.dispatchEvent(new global.CustomEvent("manja:sidebar-tab"));
	}
	tabs.forEach(function (tab, index) {
	  if (tab.getAttribute("data-manja-sidebar-tab-bound") === "true") return;
	  tab.setAttribute("data-manja-sidebar-tab-bound", "true");
	  tab.addEventListener("click", function () { select(tab); });
	  tab.addEventListener("keydown", function (event) {
		if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
		event.preventDefault();
		var offset = event.key === "ArrowRight" ? 1 : -1;
		var next = tabs[(index + offset + tabs.length) % tabs.length];
		select(next);
		next.focus();
	  });
	});
	select(tabs.filter(function (tab) { return tab.getAttribute("aria-selected") === "true"; })[0] || tabs[0]);
  }

  function replaceStaticSidebarContinuation(descriptor, cache, documentKey, sidebar) {
	if (!sidebar) return;
	if (typeof sidebar.manjaStaticSidebarDispose === "function") sidebar.manjaStaticSidebarDispose();
	installStaticSidebarTabs(sidebar);
	sidebar.manjaStaticSidebarDispose = installStaticSidebarContinuation(descriptor, cache, documentKey, sidebar);
  }

  function installStaticSidebar(descriptor, cache, documentValue, route) {
	var sidebar = documentValue.getElementById("catalog-sidebar-groups");
	if (!sidebar) return Promise.resolve();
	var documentHref = descriptor.publicationBase + "documents/" + encodeURIComponent(route.documentKey) + "/";
	var overviewActive = !route.selected;
	sidebar.innerHTML = '<nav data-manja-local-sidebar="true" data-manja-static-default-open="true" aria-label="API navigation" class="min-h-0 overflow-y-auto scrollbar-custom px-3 pb-4">' +
	  '<div data-manja-static-sidebar-top="true"><a id="catalog-sidebar-spec-overview" data-manja-static-sidebar-top-link="true" data-manja-static-route="true" data-catalog-sidebar-item="true"' + (overviewActive ? ' data-catalog-sidebar-selected="true" aria-current="page"' : '') + ' href="' + documentHref + '" class="flex min-h-11 items-center gap-2 rounded-radius px-2 py-2 font-semibold"><svg viewBox="0 0 20 20" fill="currentColor" class="size-5 shrink-0" aria-hidden="true"><path d="M4.5 2.75A1.75 1.75 0 0 0 2.75 4.5v11A1.75 1.75 0 0 0 4.5 17.25h11a1.75 1.75 0 0 0 1.75-1.75v-11a1.75 1.75 0 0 0-1.75-1.75h-11Zm1.25 3h8.5v1.5h-8.5v-1.5Zm0 3.5h8.5v1.5h-8.5v-1.5Zm0 3.5h5.5v1.5h-5.5v-1.5Z"></path></svg><span class="min-w-0 flex-1 truncate">Spec overview</span></a></div>' +
	  '<div role="tablist" aria-label="API resources" data-manja-sidebar-tabs="true"><button id="manja-sidebar-tab-operations" type="button" role="tab" data-manja-sidebar-tab="operations" aria-controls="manja-sidebar-panel-operations" aria-selected="true">Operations</button><button id="manja-sidebar-tab-schemas" type="button" role="tab" data-manja-sidebar-tab="schemas" aria-controls="manja-sidebar-panel-schemas" aria-selected="false" tabindex="-1">Schemas</button></div>' +
	  '<section id="manja-sidebar-panel-operations" role="tabpanel" aria-labelledby="manja-sidebar-tab-operations" data-manja-sidebar-tab-panel="operations" data-manja-static-sidebar-section="operations"><div data-manja-static-sidebar-operations="true"></div></section>' +
	  '<section id="manja-sidebar-panel-schemas" role="tabpanel" aria-labelledby="manja-sidebar-tab-schemas" data-manja-sidebar-tab-panel="schemas" data-manja-static-sidebar-section="schemas" hidden><div data-manja-static-sidebar-schemas="true"></div></section></nav>';
	var operations = sidebar.querySelector && sidebar.querySelector("[data-manja-static-sidebar-operations]");
	var schemas = sidebar.querySelector && sidebar.querySelector("[data-manja-static-sidebar-schemas]");
	return Promise.all([
	  readStaticSidebarChunk(descriptor, cache, route.documentKey, "operations", 0).then(function (html) { if (html && operations) operations.innerHTML = html; }),
	  readStaticSidebarChunk(descriptor, cache, route.documentKey, "schemas", 0).then(function (html) { if (html && schemas) schemas.innerHTML = html; })
	]).then(function () {
	  if (schemas && !schemas.innerHTML && schemas.parentNode) schemas.parentNode.hidden = true;
	  applyStaticSidebarGroupState(sidebar, route);
	  if (global.htmx && typeof global.htmx.process === "function") global.htmx.process(sidebar);
	  replaceStaticSidebarContinuation(descriptor, cache, route.documentKey, sidebar);
	});
  }

	function installStaticRouter(descriptor, cache, documentValue, loadCompatibility) {
	  var root = documentValue.documentElement;
	  var main = documentValue.querySelector("[data-catalog-main-content]");
	  var sidebar = documentValue.getElementById("catalog-sidebar-groups");
	  var children = Object.create(null);
	  var admittedChildren = Object.create(null);
	  var childAdmissionPromises = Object.create(null);
	  var browserPrepared = false;
	  var browserPreparation = null;
	  var lastRoute = null;
	  var retryInFlight = false;
	  var manifest = null;
	  var catalog = null;
	  var abi = null;
	  var cleanManifest = null;
	  var disposeLazySchemaFragments = function () {};
	  var initialMainHTML = main && main.innerHTML || "";
	  var initialTitle = documentValue.title;
	  if (!main) fail("static catalog main target is missing");
	  if (global.history && "scrollRestoration" in global.history) global.history.scrollRestoration = "manual";
	  function mainScrollContainer() {
		if (main.closest) return main.closest("[data-manja-primary-scroll]") || main;
		return main;
	  }
	  function sidebarScrollContainer() {
		if (!sidebar) return null;
		return sidebar.querySelector && sidebar.querySelector("nav[data-manja-local-sidebar]") || sidebar;
	  }
	  function scrollPosition() {
		var mainPanel = mainScrollContainer();
		var sidebarPanel = sidebarScrollContainer();
		return {
		  main: mainPanel && typeof mainPanel.scrollTop === "number" ? Math.max(0, mainPanel.scrollTop) : 0,
		  sidebar: sidebarPanel && typeof sidebarPanel.scrollTop === "number" ? Math.max(0, sidebarPanel.scrollTop) : 0,
		};
	  }
	  function restoreScroll(position) {
		if (!position) return;
		var mainPanel = mainScrollContainer();
		var sidebarPanel = sidebarScrollContainer();
		if (mainPanel && typeof position.main === "number" && isFinite(position.main)) mainPanel.scrollTop = Math.max(0, position.main);
		if (sidebarPanel && typeof position.sidebar === "number" && isFinite(position.sidebar)) sidebarPanel.scrollTop = Math.max(0, position.sidebar);
	  }
	  function historyState(position) {
		var current = global.history && global.history.state;
		var state = current && typeof current === "object" && !Array.isArray(current) ? Object.assign({}, current) : {};
		var value = position || scrollPosition();
		state.manjaLocalDocs = { main: Math.max(0, Number(value.main) || 0), sidebar: Math.max(0, Number(value.sidebar) || 0) };
		return state;
	  }
	  function currentHistoryScroll() {
		var state = global.history && global.history.state;
		var value = state && state.manjaLocalDocs;
		if (!value || typeof value !== "object") return null;
		if (!isFinite(Number(value.main)) || !isFinite(Number(value.sidebar))) return null;
		return { main: Math.max(0, Number(value.main)), sidebar: Math.max(0, Number(value.sidebar)) };
	  }
	  function saveHistoryScroll(position) {
		if (global.history && typeof global.history.replaceState === "function") {
		  global.history.replaceState(historyState(position), "", global.location.href);
		}
	  }
	  function revealSidebarSelection() {
		var navigation = sidebarScrollContainer();
		if (!navigation || !navigation.querySelector || !navigation.getBoundingClientRect) return;
		var selected = navigation.querySelector("[data-catalog-sidebar-selected]");
		if (!selected || !selected.getBoundingClientRect) return;
		var selectedBox = selected.getBoundingClientRect();
		var panelBox = navigation.getBoundingClientRect();
		if (selectedBox.top >= panelBox.top && selectedBox.bottom <= panelBox.bottom) return;
		var maxTop = Math.max(0, navigation.scrollHeight - navigation.clientHeight);
		var targetTop = navigation.scrollTop + selectedBox.top - panelBox.top - (navigation.clientHeight - selectedBox.height) / 2;
		navigation.scrollTop = Math.max(0, Math.min(maxTop, targetTop));
	  }
	  function focusRenderedDetail() {
		var focusTarget = main.querySelector('[data-manja-settled-focus="true"]');
		if (focusTarget && typeof focusTarget.focus === "function") focusTarget.focus({ preventScroll: true });
	  }
	  function settleRenderedDetailFocus() {
		focusRenderedDetail();
		// Alpine's focus trap restores the search trigger on the next render
		// frame when a result closes the dialog. Re-assert the destination focus
		// after that release without delaying the navigation promise or affecting
		// non-browser test harnesses that do not provide animation frames.
		if (typeof global.requestAnimationFrame !== "function") return;
		global.requestAnimationFrame(function () {
			global.requestAnimationFrame(focusRenderedDetail);
		});
	  }
	  function focusGroup(id) {
		if (!sidebar || !sidebar.querySelectorAll) return;
		var controls = sidebar.querySelectorAll("[data-manja-static-group]");
		for (var index = 0; index < controls.length; index++) {
		  if (controls[index].getAttribute("data-manja-static-group") !== id) continue;
		  if (typeof controls[index].focus === "function") controls[index].focus({ preventScroll: true });
		  return;
		}
	  }
	  function setNavigationState(busy, error) {
		if (root && root.setAttribute) root.setAttribute("aria-busy", busy ? "true" : "false");
		if (main && main.setAttribute) main.setAttribute("aria-busy", busy ? "true" : "false");
		if (root && root.dataset) {
		  if (busy) root.dataset.manjaLocalDocsNavigation = "loading";
		  else if (error) {
			root.dataset.manjaLocalDocsNavigation = "error";
			root.dataset.manjaLocalDocsNavigationReason = error && error.message ? String(error.message).slice(0, 256) : "navigation failed";
		  } else {
			root.dataset.manjaLocalDocsNavigation = "ready";
			delete root.dataset.manjaLocalDocsNavigationReason;
		  }
		}
		var status = documentValue.querySelector && documentValue.querySelector("[data-manja-static-navigation-status]");
		if (!status && documentValue.createElement && documentValue.body && documentValue.body.appendChild) {
		  status = documentValue.createElement("div");
		  status.setAttribute("data-manja-static-navigation-status", "true");
		  status.setAttribute("role", "status");
		  status.setAttribute("aria-live", "polite");
		  status.className = "sr-only";
		  documentValue.body.appendChild(status);
		}
		var errorPanel = documentValue.querySelector && documentValue.querySelector("[data-manja-static-navigation-error]");
		if (errorPanel) {
			errorPanel.hidden = !error;
			var retry = navigationRetryControl(errorPanel);
			if (retry) {
				retry.hidden = !error;
				retry.disabled = busy || retryInFlight;
			}
		}
		if (status) status.textContent = busy ? "Loading documentation…" : error ? "Unable to load this documentation section. Please try again." : "";
		if (!busy && error && root) root.dispatchEvent(new CustomEvent("manja:local-navigation-error", { detail: { reason: error && error.message ? error.message : "navigation failed" } }));
	  }
	  function navigationRetryControl(scope) {
		if (!scope || !scope.querySelector) return null;
		var retry = scope.querySelector("[data-manja-static-navigation-retry]");
		if (!retry) retry = scope.querySelector('button:not([aria-label])');
		if (retry && retry.setAttribute) retry.setAttribute("data-manja-static-navigation-retry", "true");
		return retry;
	  }
	  function retryNavigation() {
		if (!lastRoute || retryInFlight) return;
		retryInFlight = true;
		setNavigationState(true);
		var pending = swap(lastRoute, "none", { focus: true });
		pending.then(function () {
			retryInFlight = false;
			setNavigationState(false);
		}).catch(function (error) {
			retryInFlight = false;
			setNavigationState(false, error);
		});
		return pending;
	  }
	  function prepareBrowser() {
		if (browserPrepared) return Promise.resolve();
		if (browserPreparation) return browserPreparation;
		browserPreparation = Promise.resolve().then(loadCompatibility).then(function (compatibility) {
			manifest = compatibility.manifest;
			catalog = compatibility.catalog;
			abi = compatibility.abi;
			cleanManifest = Object.assign({}, manifest); delete cleanManifest.identityDigest;
			return abi.prepare(descriptor, cleanManifest, catalog, Object.create(null));
		}).then(function (prepared) {
			if (!prepared || prepared.ok !== true) fail(prepared && prepared.error || "static Wasm preparation failed");
			browserPrepared = true;
		}).catch(function (error) {
			browserPreparation = null;
			throw error;
		});
		return browserPreparation;
	  }
	  function admitLoadedChildren() {
		var paths = Object.keys(children).sort();
		if (typeof abi.admit !== "function") {
			var pending = paths.some(function (childPath) { return !admittedChildren[childPath]; });
			if (!pending) return Promise.resolve();
			var prepared = abi.prepare(descriptor, cleanManifest, catalog, children);
			return Promise.resolve(prepared).then(function (result) {
				if (!result || result.ok !== true) fail(result && result.error || "static Wasm preparation failed");
				paths.forEach(function (childPath) { admittedChildren[childPath] = true; });
			});
		}
		return paths.reduce(function (pending, childPath) {
			if (admittedChildren[childPath]) return pending;
			return pending.then(function () {
				if (admittedChildren[childPath]) return undefined;
				if (!childAdmissionPromises[childPath]) {
					childAdmissionPromises[childPath] = Promise.resolve().then(function () { return abi.admit(childPath, children[childPath]); }).then(function (result) {
						if (!result || result.ok !== true) fail(result && result.error || 'static child "' + childPath + '" admission failed');
						admittedChildren[childPath] = true;
						delete childAdmissionPromises[childPath];
					}, function (error) {
						delete childAdmissionPromises[childPath];
						throw error;
					});
					return childAdmissionPromises[childPath];
				}
			});
		}, Promise.resolve());
	  }
	  function navigate(href) {
		var route = staticRoute(descriptor, href);
		if (!route) return null;
		var current = staticRoute(descriptor, global.location.href);
		if (current) {
		  if (route.groups.length === 0) route.groups = current.groups.slice();
		  if (route.closedGroups.length === 0) route.closedGroups = current.closedGroups.slice();
		}
		return swap(route, "push", { focus: true });
	  }
	  function swap(route, historyMode) {
	  var options = arguments.length > 2 && arguments[2] || {};
	  lastRoute = route;
	  var beforeScroll = scrollPosition();
	  if (historyMode === "push") saveHistoryScroll(beforeScroll);
	  setNavigationState(true);
	  var preRendered = !options.sidebarOnly && route.selected ? readStaticHTMLFragment(descriptor, cache, route).then(function (fragment) {
		return fragment;
	  }).catch(function (error) {
		root.dataset.manjaStaticFragmentFallbackReason = error && error.message ? String(error.message).slice(0, 256) : "static fragment unavailable";
		throw error;
	  }) : !options.sidebarOnly && !route.selected ? Promise.resolve({ ok: true, mainHtml: initialMainHTML, title: initialTitle, canonical: staticRouteCanonical(descriptor, route) }) : Promise.resolve(null);
	  return preRendered.then(function (fragmentResult) {
		if (fragmentResult) return fragmentResult;
		return prepareBrowser().then(function () {
		return hydrateStaticRoute(descriptor, manifest, catalog, cache, children, route);
	  }).then(function () {
		return admitLoadedChildren();
	  }).then(function () {
		return options.sidebarOnly && typeof abi.renderSidebar === "function" ? abi.renderSidebar(route) : abi.render(route);
	  });
	  }).then(function (result) {
		if (!result || result.ok !== true) fail(result && result.error || "static render failed");
		if (!options.sidebarOnly) {
		  disposeLazySchemaFragments();
		  main.innerHTML = result.mainHtml;
		  disposeLazySchemaFragments = installLazySchemaFragments(descriptor, cache, route, main);
		}
		if (sidebar && typeof result.sidebarHtml === "string") {
		  sidebar.innerHTML = result.sidebarHtml;
		  replaceStaticSidebarContinuation(descriptor, cache, route.documentKey, sidebar);
		}
		if (!options.sidebarOnly) documentValue.title = result.title;
		if (options.restoreScroll) restoreScroll(options.restoreScroll);
		else if (options.preserveScroll) restoreScroll(beforeScroll);
		else if (!options.sidebarOnly) {
		  var mainPanel = mainScrollContainer();
		  if (mainPanel) mainPanel.scrollTop = 0;
		}
		if (!options.preserveScroll && !options.restoreScroll) revealSidebarSelection();
		if (historyMode === "push" && global.history && typeof global.history.pushState === "function") global.history.pushState(historyState(scrollPosition()), "", result.canonical);
		if (historyMode === "replace" && global.history && typeof global.history.replaceState === "function") global.history.replaceState(historyState(scrollPosition()), "", result.canonical);
		if (options.initial && global.history && typeof global.history.replaceState === "function") global.history.replaceState(historyState(scrollPosition()), "", result.canonical);
		if (global.htmx && typeof global.htmx.process === "function") { if (!options.sidebarOnly) global.htmx.process(main); if (sidebar) global.htmx.process(sidebar); }
	  if (!options.preserveScroll && typeof global.manjaCatalogScrollSidebarSelection === "function") global.manjaCatalogScrollSidebarSelection();
		if (options.focus) settleRenderedDetailFocus();
		if (sidebar && sidebar.querySelectorAll) {
		  var links = sidebar.querySelectorAll("[data-catalog-sidebar-item], [data-catalog-sidebar-operation]");
		  for (var linkIndex = 0; linkIndex < links.length; linkIndex++) {
			var linkRoute = staticRoute(descriptor, links[linkIndex].href || links[linkIndex].getAttribute && links[linkIndex].getAttribute("href") || "");
			var selected = Boolean(linkRoute && linkRoute.selected === route.selected && (route.selected || links[linkIndex].getAttribute("id") === "catalog-sidebar-spec-overview"));
			if (selected) {
			  links[linkIndex].setAttribute("aria-current", "page");
			  links[linkIndex].setAttribute("data-catalog-sidebar-selected", "true");
			} else {
			  links[linkIndex].removeAttribute("aria-current");
			  links[linkIndex].removeAttribute("data-catalog-sidebar-selected");
			}
		  }
		}
	  if (options.focusGroup) focusGroup(options.focusGroup);
	  setNavigationState(false);
	  return result;
	  }, function (error) {
	  setNavigationState(false, error);
	  throw error;
	  });
	  }
	  var navigationError = documentValue.querySelector && documentValue.querySelector("[data-manja-static-navigation-error]");
	  var retryControl = navigationRetryControl(navigationError);
	  if (retryControl && retryControl.addEventListener) retryControl.addEventListener("click", retryNavigation);
	  documentValue.addEventListener("click", function (event) {
	  var group = event.target && event.target.closest && event.target.closest("[data-manja-static-group]");
	  if (group) {
		var groupRoute = staticRoute(descriptor, global.location.href);
		if (!groupRoute) return;
		event.preventDefault();
		var id = group.getAttribute("data-manja-static-group");
		if (groupRoute.groups.length === 0 && groupRoute.closedGroups.length === 0) groupRoute.groups = expandedStaticSidebarGroups(sidebar);
		var index = groupRoute.groups.indexOf(id);
		var closedIndex = groupRoute.closedGroups.indexOf(id);
		if (closedIndex >= 0) {
		  groupRoute.closedGroups.splice(closedIndex, 1);
		  if (groupRoute.groups.indexOf(id) < 0) groupRoute.groups.push(id);
		} else if (index >= 0) {
		  groupRoute.groups.splice(index, 1);
		  groupRoute.closedGroups.push(id);
		} else if (group.getAttribute("aria-expanded") === "true") {
		  groupRoute.closedGroups.push(id);
		} else {
		  groupRoute.groups.push(id);
		}
		applyStaticSidebarGroupState(sidebar, groupRoute);
		if (sidebar.dispatchEvent && typeof global.CustomEvent === "function") sidebar.dispatchEvent(new global.CustomEvent("manja:sidebar-group"));
		if (global.history && typeof global.history.replaceState === "function") global.history.replaceState(historyState(scrollPosition()), "", staticRouteCanonical(descriptor, groupRoute));
		focusGroup(id);
		return;
	  }
	  var origin = event.target && event.target.closest && event.target.closest("a[href]");
	  if (origin) {
		var route = staticRoute(descriptor, origin.href);
		if (route) {
		  var current = staticRoute(descriptor, global.location.href);
		  if (current) {
		    if (route.groups.length === 0) route.groups = current.groups.slice();
		    if (route.closedGroups.length === 0) route.closedGroups = current.closedGroups.slice();
		  }
		  event.preventDefault(); swap(route, "push", { focus: true }).catch(function () {});
		}
		return;
	  }
	  });
	  global.addEventListener("popstate", function () {
	  var route = staticRoute(descriptor, global.location.href);
	  if (route) swap(route, "none", { restoreScroll: currentHistoryScroll() }).catch(function () {});
	  });
	  return { swap: swap, navigate: navigate, initial: staticRoute(descriptor, global.location.href) };
	}

  function startStatic(root, descriptor, options, documentValue) {
	var deployment = descriptor.static.deploymentBase;
	var staticOptions = Object.assign({}, options, {
	  workerURL: descriptor.static.workerUrl,
	  scope: descriptor.static.workerScope,
	  runtimeURL: deployment + "manja-assets/local-docs/wasm_exec.js",
	  wasmURL: deployment + "manja-assets/local-docs/manja.wasm",
	});
	return global.caches.open(staticCacheName(descriptor)).then(function (cache) {
		return readExportIdentity(descriptor, cache).then(function () {
		  var compatibility = null;
		  function loadCompatibility() {
			if (compatibility) return compatibility;
			compatibility = Promise.all([loadABI(staticOptions), readManifest(sameOriginPath(descriptor.projectionManifestUrl), descriptor, cache)]).then(function (values) {
			  var abi = values[0];
			  var manifest = values[1];
			  var activated = validateActivation(abi.activate(descriptor, manifest), descriptor);
			  var catalogIdentity = manifestChild(manifest, "catalog.json");
			  return readVerifiedJSON(descriptor.catalogUrl, catalogIdentity, cache).then(function (catalog) {
				return { abi: abi, manifest: manifest, catalog: catalog, activated: activated };
			  });
			}).catch(function (error) { compatibility = null; throw error; });
			return compatibility;
		  }
		  var router = installStaticRouter(descriptor, cache, documentValue, loadCompatibility);
		  var sidebarReady = router.initial ? installStaticSidebar(descriptor, cache, documentValue, router.initial) : Promise.resolve();
		  return sidebarReady.then(function () {
			return router.initial && router.initial.selected ? router.swap(router.initial, "none", { initial: true }) : undefined;
		  }).then(function () {
			if (global.ManjaLocalDocsEnhancer) global.ManjaLocalDocsEnhancer.navigate = router.navigate;
			return registerWorker(root, descriptor, staticOptions);
		  }).then(function () {
			mark(root, "ready");
			return { ok: true, result: { static: true } };
		  });
		});
	});
  }

  function start(options) {
    options = options || {};
    var documentValue = options.document || global.document;
    var root = documentValue.documentElement;
    try {
      var descriptor = descriptorFromDocument(documentValue);
	  if (!descriptor) {
		return Promise.resolve({ skipped: true });
	  }
	  if (descriptor.static) {
		return startStatic(root, descriptor, options, documentValue).catch(function (error) {
		  mark(root, "fallback", error && error.message ? error.message : "static activation failed");
		  return { ok: false, reason: error && error.message ? error.message : "static activation failed" };
		});
	  }
      var manifestURL = sameOriginPath(descriptor.projectionManifestUrl);
      return registerWorker(root, descriptor, options).then(function () {
        return loadABI(options);
      }).then(function (abi) {
        if (!abi || typeof abi.activate !== "function") {
          fail("Wasm ABI is unavailable");
        }
        return readManifest(manifestURL, descriptor).then(function (manifest) {
          return Promise.resolve(abi.activate(descriptor, manifest)).then(function (result) {
            var activated = validateActivation(result, descriptor);
            mark(root, "ready");
            return { ok: true, result: activated };
          });
        });
      }).catch(function (error) {
        mark(root, "fallback", error && error.message ? error.message : "activation failed");
        return { ok: false, reason: error && error.message ? error.message : "activation failed" };
      });
    } catch (error) {
      mark(root, "fallback", error && error.message ? error.message : "activation failed");
      return Promise.resolve({ ok: false, reason: error && error.message ? error.message : "activation failed" });
    }
  }

  var api = {
    start: start,
    autoStart: function () { return start({}); },
    registerWorker: registerWorker,
	validateDescriptor: validateDescriptor,
	validateManifest: validateManifest,
	staticRoute: staticRoute
  };
  global.ManjaLocalDocsEnhancer = api;
  if (global.document && global.document.readyState === "loading") {
    global.document.addEventListener("DOMContentLoaded", function () { api.autoStart(); }, { once: true });
  } else if (global.document) {
    api.autoStart();
  }
}(window));
