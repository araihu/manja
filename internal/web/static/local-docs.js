(function (global) {
  "use strict";

  var MAX_MANIFEST_BYTES = 4 * 1024 * 1024;
  var MAX_PROJECTION_CHILD_BYTES = 2 * 1024 * 1024;
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
        if (child.kind.indexOf("search-") !== 0 || child.length > MAX_PROJECTION_CHILD_BYTES) fail("manifest search child is invalid");
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

  function hydrateStaticRoute(descriptor, manifest, catalog, cache, children, route) {
	if (!route.selected) return Promise.resolve(children);
	var documentValue = staticCatalogDocument(catalog, route.documentKey);
	var selected = staticRouteSelection(documentValue, route.selected);
	return loadStaticChild(descriptor, manifest, cache, children, selected.directory.detailChild).then(function (detailShard) {
	  if (!selected.schema) return children;
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

  function readExportManifest(descriptor, cache) {
	var url = sameOriginPath(descriptor.static.exportManifestUrl);
	return fetchWithCache(url.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }, cache).then(function (response) {
	  if (!response.ok) fail("export manifest request failed");
	  return response.text();
	}).then(function (text) {
	  var manifest = parseJSONStrict(text);
	  if (!manifest || manifest.schemaVersion !== 1 || manifest.basePath !== descriptor.static.deploymentBase || !Array.isArray(manifest.catalogs)) fail("export manifest identity is invalid");
	  var matched = manifest.catalogs.some(function (catalog) {
		return catalog && catalog.catalogId === descriptor.catalogId && catalog.publicationKey === descriptor.publicationKey && catalog.revisionId === descriptor.revisionId && catalog.snapshotId === descriptor.snapshotId;
	  });
	  if (!matched) fail("export manifest catalog differs");
	  return Promise.resolve(cache.put(url.href, new Response(text, { headers: { "Content-Type": "application/json" } }))).then(function () { return manifest; });
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

	function installStaticRouter(descriptor, manifest, catalog, cache, abi, documentValue) {
	  var main = documentValue.querySelector("[data-catalog-main-content]");
	  var sidebar = documentValue.getElementById("catalog-sidebar-groups");
	  var children = Object.create(null);
	  if (!main) fail("static catalog main target is missing");
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
		return hydrateStaticRoute(descriptor, manifest, catalog, cache, children, route).then(function () {
		var cleanManifest = Object.assign({}, manifest); delete cleanManifest.identityDigest;
		var prepared = abi.prepare(descriptor, cleanManifest, catalog, children);
		if (!prepared || prepared.ok !== true) fail(prepared && prepared.error || "static Wasm preparation failed");
		return abi.render(route);
	  }).then(function (result) {
		if (!result || result.ok !== true) fail(result && result.error || "static render failed");
		main.innerHTML = result.mainHtml;
		if (sidebar) sidebar.innerHTML = result.sidebarHtml;
		documentValue.title = result.title;
		if (historyMode === "push") global.history.pushState({}, "", result.canonical);
		if (global.htmx && typeof global.htmx.process === "function") { global.htmx.process(main); if (sidebar) global.htmx.process(sidebar); }
		if (typeof global.manjaCatalogScrollSidebarSelection === "function") global.manjaCatalogScrollSidebarSelection();
		if (options.focus) {
		  var focusTarget = main.querySelector('[data-manja-settled-focus="true"]');
		  if (focusTarget && typeof focusTarget.focus === "function") focusTarget.focus({ preventScroll: true });
		}
		return result;
	  });
	}
	documentValue.addEventListener("click", function (event) {
	  var origin = event.target && event.target.closest && event.target.closest("a[href]");
	  if (origin) {
		var route = staticRoute(descriptor, origin.href);
		if (route) {
		  var current = staticRoute(descriptor, global.location.href);
		  if (current) {
		    if (route.groups.length === 0) route.groups = current.groups.slice();
		    if (route.closedGroups.length === 0) route.closedGroups = current.closedGroups.slice();
		  }
		  event.preventDefault(); swap(route, "push").catch(function () {});
		}
		return;
	  }
	  var group = event.target && event.target.closest && event.target.closest("[data-manja-static-group]");
	  if (!group) return;
	  var route = staticRoute(descriptor, global.location.href);
	  if (!route) return;
	  event.preventDefault();
	  var id = group.getAttribute("data-manja-static-group");
	  var index = route.groups.indexOf(id);
	  var closedIndex = route.closedGroups.indexOf(id);
	  var navigation = group.closest && group.closest("nav[data-manja-local-sidebar]");
	  var defaultOpen = navigation && navigation.getAttribute("data-manja-static-default-open") === "true";
	  if (closedIndex >= 0) {
	    route.closedGroups.splice(closedIndex, 1);
	    if (!defaultOpen && route.groups.indexOf(id) < 0) route.groups.push(id);
	  } else if (index >= 0) {
	    route.groups.splice(index, 1);
	    route.closedGroups.push(id);
	  } else if (group.getAttribute("aria-expanded") === "true") {
	    route.closedGroups.push(id);
	  } else if (!defaultOpen) {
	    route.groups.push(id);
	  }
	  swap(route, "push").catch(function () {});
	});
	global.addEventListener("popstate", function () { var route = staticRoute(descriptor, global.location.href); if (route) swap(route, "none").catch(function () {}); });
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
		return Promise.all([registerWorker(root, descriptor, staticOptions), loadABI(staticOptions), readExportManifest(descriptor, cache)]).then(function (values) {
		  var abi = values[1];
		  return readManifest(sameOriginPath(descriptor.projectionManifestUrl), descriptor, cache).then(function (manifest) {
			var activated = validateActivation(abi.activate(descriptor, manifest), descriptor);
			var catalogIdentity = manifestChild(manifest, "catalog.json");
			return readVerifiedJSON(descriptor.catalogUrl, catalogIdentity, cache).then(function (catalog) {
			  var router = installStaticRouter(descriptor, manifest, catalog, cache, abi, documentValue);
			  return (router.initial ? router.swap(router.initial, "none") : Promise.resolve()).then(function () {
				if (global.ManjaLocalDocsEnhancer) global.ManjaLocalDocsEnhancer.navigate = router.navigate;
				mark(root, "ready");
			  return { ok: true, result: activated };
			});
		  });
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
