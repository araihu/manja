(function (global) {
  "use strict";

  var MAX_MANIFEST_BYTES = 4 * 1024 * 1024;
  var MAX_PROJECTION_CHILD_BYTES = 2 * 1024 * 1024;
  var MAX_WASM_BYTES = 32 * 1024 * 1024;
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
    return Boolean(parsed);
  }

  function canonicalIdentity(value) {
    return typeof value === "string" && value.length > 0 && value === value.trim() && !/[\u0000-\u001f\u007f]/.test(value);
  }

  function sha256(value) {
    return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
  }

  function validateDescriptor(descriptor) {
    if (!descriptor || typeof descriptor !== "object" || descriptor.schemaVersion !== 1 || !canonicalIdentity(descriptor.catalogId) || !canonicalIdentity(descriptor.publicationKey) || !canonicalIdentity(descriptor.revisionId) || !validBase(descriptor.publicationBase) || descriptor.projectionFormat !== "projection-v2" || !sha256(descriptor.projectionDigest) || descriptor.snapshotId !== "snapshot-sha256-" + descriptor.projectionDigest) {
      fail("descriptor identity is invalid");
    }
    if (descriptor.public === false || descriptor.anonymous === false || descriptor.private === true || descriptor.disabled === true) {
      fail("descriptor public eligibility is invalid");
    }
    var allowed = {
      schemaVersion: true, catalogId: true, publicationKey: true, publicationBase: true,
      snapshotId: true, revisionId: true, projectionFormat: true, projectionDigest: true,
      projectionManifestUrl: true, catalogUrl: true, searchDataBase: true, projectionDataBase: true,
      offlineShellUrl: true, public: true, anonymous: true, private: true, disabled: true,
      eligibility: true, fallbackAssets: true, fallbackManifest: true,
      runtimeURL: true, wasmURL: true, workerURL: true
    };
    Object.keys(descriptor).forEach(function (key) {
      if (!allowed[key]) fail("descriptor field is unknown");
    });
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
    ["offlineShellUrl", "runtimeURL", "wasmURL", "workerURL"].forEach(function (key) {
      if (descriptor[key] !== undefined && !sameOriginPath(descriptor[key])) {
        fail("descriptor asset route is invalid");
      }
    });
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
    if (!manifest || typeof manifest !== "object" || manifest.schemaVersion !== 1 || manifest.snapshotId !== descriptor.snapshotId || !manifest.identity || typeof manifest.identity !== "object" || manifest.identity.schemaVersion !== 1 || manifest.identity.catalogId !== descriptor.catalogId || manifest.identity.revisionId !== descriptor.revisionId || identityProjectionFormat(manifest.identity) !== descriptor.projectionFormat || !Array.isArray(manifest.children) || manifest.children.length > 10000) {
      fail("manifest identity is invalid");
    }
    var allowedRoot = { schemaVersion: true, snapshotId: true, identity: true, children: true };
    Object.keys(manifest).forEach(function (key) {
      if (!allowedRoot[key]) {
        fail("manifest field is unknown");
      }
    });
    var allowedIdentity = {
      schemaVersion: true, catalogId: true, catalogTitle: true, branding: true,
      defaultDocumentKey: true, profileId: true, revisionKind: true, revisionId: true, projectionFormat: true,
      commitSha: true, sourceManifestSha256: true, profileAllowlistLength: true,
      profileAllowlistSha256: true, versions: true, bounds: true, sources: true, children: true
    };
    Object.keys(manifest.identity).forEach(function (key) {
      if (!allowedIdentity[key]) {
        fail("manifest identity field is unknown");
      }
    });
    var seen = Object.create(null);
    manifest.children.forEach(function (child) {
      if (!child || typeof child !== "object") {
        fail("manifest child is invalid");
      }
      Object.keys(child).forEach(function (key) {
        if (key !== "path" && key !== "kind" && key !== "length" && key !== "sha256") {
          fail("manifest child field is unknown");
        }
      });
      var projection = child && typeof child.path === "string" && (child.path.indexOf("details/") === 0 || child.path.indexOf("schema-nodes/") === 0);
      if (projection && !validProjectionChild(child)) {
        fail("manifest projection child is invalid");
      }
      if (projection) {
        if (seen[child.path]) {
          fail("manifest projection child is duplicated");
        }
        seen[child.path] = true;
      }
    });
    return manifest;
  }

  function hexDigest(buffer) {
    return Array.prototype.map.call(new Uint8Array(buffer), function (byte) {
      return byte.toString(16).padStart(2, "0");
    }).join("");
  }

  function parseJSONStrict(text) {
    var index = 0;
    function whitespace() {
      while (index < text.length && /\s/.test(text.charAt(index))) {
        index += 1;
      }
    }
    function stringValue() {
      var start = index;
      if (text.charAt(index) !== '"') {
        fail("manifest JSON is invalid");
      }
      index += 1;
      while (index < text.length) {
        var character = text.charAt(index);
        index += 1;
        if (character === "\\") {
          if (index >= text.length) {
            fail("manifest JSON is invalid");
          }
          index += 1;
        } else if (character === '"') {
          try {
            return JSON.parse(text.slice(start, index));
          } catch (_) {
            fail("manifest JSON is invalid");
          }
        } else if (character < " ") {
          fail("manifest JSON is invalid");
        }
      }
      fail("manifest JSON is invalid");
    }
    function value(depth) {
      if (depth > 32) {
        fail("manifest JSON is too deep");
      }
      whitespace();
      var character = text.charAt(index);
      if (character === "{") {
        index += 1;
        whitespace();
        var object = {};
        var keys = Object.create(null);
        if (text.charAt(index) === "}") {
          index += 1;
          return object;
        }
        while (index < text.length) {
          whitespace();
          var key = stringValue();
          if (keys[key]) {
            fail("manifest JSON has duplicate keys");
          }
          keys[key] = true;
          whitespace();
          if (text.charAt(index) !== ":") {
            fail("manifest JSON is invalid");
          }
          index += 1;
          object[key] = value(depth + 1);
          whitespace();
          if (text.charAt(index) === "}") {
            index += 1;
            return object;
          }
          if (text.charAt(index) !== ",") {
            fail("manifest JSON is invalid");
          }
          index += 1;
        }
      } else if (character === "[") {
        index += 1;
        whitespace();
        var array = [];
        if (text.charAt(index) === "]") {
          index += 1;
          return array;
        }
        while (index < text.length) {
          array.push(value(depth + 1));
          whitespace();
          if (text.charAt(index) === "]") {
            index += 1;
            return array;
          }
          if (text.charAt(index) !== ",") {
            fail("manifest JSON is invalid");
          }
          index += 1;
        }
      } else if (character === '"') {
        return stringValue();
      } else {
        var start = index;
        while (index < text.length && !/[\s,\]}]/.test(text.charAt(index))) {
          index += 1;
        }
        var token = text.slice(start, index);
        if (!/^(?:true|false|null|-?(?:0|[1-9]\d*))$/.test(token)) {
          fail("manifest JSON is invalid");
        }
        return JSON.parse(token);
      }
      fail("manifest JSON is invalid");
    }
    var result = value(0);
    whitespace();
    if (index !== text.length) {
      fail("manifest JSON has trailing data");
    }
    return result;
  }

  function identityBytes(identity) {
    if (identity.versions === undefined && identity.projectionFormat !== undefined) {
      return new TextEncoder().encode(JSON.stringify({
        schemaVersion: identity.schemaVersion,
        catalogId: identity.catalogId,
        revisionId: identity.revisionId,
        projectionFormat: identity.projectionFormat
      }));
    }
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
      children: identity.children
    }));
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
        if (bytes.byteLength > MAX_WASM_BYTES) {
          fail("Wasm asset exceeds byte limit");
        }
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
    root.dataset.manjaLocalDocsState = state;
    if (state === "ready") {
      root.dataset.manjaLocalDocsReady = "true";
      delete root.dataset.manjaLocalDocsFallback;
      delete root.dataset.manjaLocalDocsReason;
    } else {
      root.dataset.manjaLocalDocsFallback = "true";
      delete root.dataset.manjaLocalDocsReady;
    }
    if (reason) {
      root.dataset.manjaLocalDocsReason = reason;
    }
    root.dispatchEvent(new CustomEvent("manja:local-" + state, { bubbles: true, detail: reason ? { reason: reason } : {} }));
  }

  function descriptorFromDocument(documentValue) {
    var script = documentValue.getElementById("manja-local-docs-descriptor");
    if (!script) {
      return null;
    }
    var descriptor;
    try {
      descriptor = parseJSONStrict(script.textContent || "");
    } catch (_) {
      fail("descriptor JSON is invalid");
    }
    return validateDescriptor(descriptor);
  }

  function workerFromNavigator() {
    return global.navigator && global.navigator["service" + "Worker"];
  }

  function announceWorkerMessage(root, event) {
    var message = event && event.data;
    if (!message || typeof message.type !== "string") {
      return;
    }
    if (message.type === "manja:local-ready") {
      root.dataset.manjaLocalDocsOfflineReady = "true";
      delete root.dataset.manjaLocalDocsWorkerReason;
      root.dispatchEvent(new CustomEvent("manja:local-ready", { bubbles: true, detail: message }));
    }
    if (message.type === "manja:local-fallback") {
      delete root.dataset.manjaLocalDocsOfflineReady;
      root.dataset.manjaLocalDocsWorker = "fallback";
      root.dataset.manjaLocalDocsWorkerReason = String(message.reason || "worker unavailable").slice(0, 256);
      root.dispatchEvent(new CustomEvent("manja:local-fallback", { bubbles: true, detail: message }));
    }
  }

  function workerTarget(serviceWorker, registration) {
    return serviceWorker.controller || registration && (registration.active || registration.waiting || registration.installing);
  }

  function tellWorker(serviceWorker, registration, descriptor, type) {
    var target = workerTarget(serviceWorker, registration);
    if (!target || typeof target.postMessage !== "function") {
      return false;
    }
    target.postMessage({ type: "manja:configure", descriptor: descriptor });
    target.postMessage({ type: type, publicationKey: descriptor.publicationKey });
    return true;
  }

  function registerWorker(root, descriptor, options) {
    options = options || {};
    var serviceWorker = workerFromNavigator();
    if (!serviceWorker || typeof serviceWorker.register !== "function") {
      return Promise.resolve({ skipped: true, reason: "service worker unsupported" });
    }
    var workerURL = sameOriginPath(options.workerURL || DEFAULT_WORKER_URL);
    if (!workerURL) {
      return Promise.reject(new Error("Service Worker asset route is invalid"));
    }
    if (!root.__manjaLocalDocsWorkerListener && typeof serviceWorker.addEventListener === "function") {
      serviceWorker.addEventListener("message", function (event) { announceWorkerMessage(root, event); });
      root.__manjaLocalDocsWorkerListener = true;
    }
    if (options.scope !== undefined && options.scope !== "/") {
      return Promise.reject(new Error("Service Worker scope must be root-scoped"));
    }
    return serviceWorker.register(workerURL.href, { scope: "/" }).then(function (registration) {
      return Promise.resolve(serviceWorker.ready || registration).then(function (ready) {
        if (!tellWorker(serviceWorker, ready, descriptor, "manja:revalidate")) {
          throw new Error("Service Worker did not become active");
        }
        root.dataset.manjaLocalDocsWorker = "registered";
        if (!root.__manjaLocalDocsWorkerLifecycle) {
          var lastVisibleRevalidation = Date.now();
          var documentValue = root.ownerDocument;
          var revalidate = function () {
            if (Date.now() - lastVisibleRevalidation < 5 * 60 * 1000) {
              return;
            }
            lastVisibleRevalidation = Date.now();
            tellWorker(serviceWorker, ready, descriptor, "manja:revalidate");
          };
          if (typeof global.addEventListener === "function") {
            global.addEventListener("online", function () {
              lastVisibleRevalidation = Date.now();
              tellWorker(serviceWorker, ready, descriptor, "manja:revalidate");
            });
            global.addEventListener("pageshow", function () {
              lastVisibleRevalidation = Date.now();
              tellWorker(serviceWorker, ready, descriptor, "manja:revalidate");
            });
          }
          if (documentValue && typeof documentValue.addEventListener === "function") {
            documentValue.addEventListener("visibilitychange", function () {
              if (documentValue.visibilityState === "visible") {
                revalidate();
              }
            });
          }
          if (typeof serviceWorker.addEventListener === "function") {
            serviceWorker.addEventListener("controllerchange", function () {
              tellWorker(serviceWorker, ready, descriptor, "manja:revalidate");
            });
          }
          root.__manjaLocalDocsWorkerLifecycle = true;
        }
        return { ok: true, registration: ready };
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
      var manifestURL = sameOriginPath(descriptor.projectionManifestUrl);
      var workerPromise = registerWorker(root, descriptor, options).catch(function (error) {
        root.dataset.manjaLocalDocsWorker = "fallback";
        root.dataset.manjaLocalDocsWorkerReason = String(error && error.message ? error.message : "worker unavailable").slice(0, 256);
        return { ok: false, reason: root.dataset.manjaLocalDocsWorkerReason };
      });
      return loadABI(options).then(function (abi) {
        if (!abi || typeof abi.activate !== "function") {
          fail("Wasm ABI is unavailable");
        }
        return readManifest(manifestURL, descriptor).then(function (manifest) {
          return Promise.resolve(abi.activate(descriptor, manifest)).then(function (result) {
            if (!result || result.ok !== true) {
              fail("Wasm ABI rejected activation");
            }
            mark(root, "ready");
            return { ok: true, result: result };
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
