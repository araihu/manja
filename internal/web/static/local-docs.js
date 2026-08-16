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
    if (descriptor.public === false || descriptor.anonymous === false || descriptor.private === true || descriptor.disabled === true || descriptor.eligibility && (descriptor.eligibility.public === false || descriptor.eligibility.anonymous === false)) {
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
    if (descriptor.offlineShellUrl !== undefined && (!sameOriginPath(descriptor.offlineShellUrl) || descriptor.offlineShellUrl !== descriptor.publicationBase + "_manja/offline-shell")) {
      fail("descriptor offline shell route is invalid");
    }
    if (descriptor.fallbackAssets !== undefined && (!Array.isArray(descriptor.fallbackAssets) || descriptor.fallbackAssets.some(function (asset) {
      return !asset || typeof asset !== "object" || !sameOriginPath(asset.url) || asset.length !== undefined && (!Number.isSafeInteger(asset.length) || asset.length <= 0 || asset.length > 16 * 1024 * 1024) || asset.sha256 !== undefined && !sha256(asset.sha256);
    }))) {
      fail("descriptor fallback asset is invalid");
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

  function readManifest(url, descriptor) {
    return global.fetch(url.href, { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } }).then(function (response) {
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
          return manifest;
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
    if (options.scope !== undefined && options.scope !== "/") {
      return Promise.reject(new Error("Service Worker scope must be root-scoped"));
    }
    if (!root.__manjaLocalDocsWorkerListener && typeof serviceWorker.addEventListener === "function") {
      serviceWorker.addEventListener("message", function (event) { announceWorkerMessage(root, event); });
      root.__manjaLocalDocsWorkerListener = true;
    }
    return Promise.resolve(serviceWorker.register(workerURL.href, { scope: "/" })).then(function (registration) {
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

  function start(options) {
    options = options || {};
    var documentValue = options.document || global.document;
    var root = documentValue.documentElement;
    try {
      var descriptor = descriptorFromDocument(documentValue);
      if (!descriptor) {
        return Promise.resolve({ skipped: true });
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
    validateManifest: validateManifest
  };
  global.ManjaLocalDocsEnhancer = api;
  if (global.document && global.document.readyState === "loading") {
    global.document.addEventListener("DOMContentLoaded", function () { api.autoStart(); }, { once: true });
  } else if (global.document) {
    api.autoStart();
  }
}(window));
