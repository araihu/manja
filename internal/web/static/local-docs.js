(function (global) {
  "use strict";

  var MAX_MANIFEST_BYTES = 4 * 1024 * 1024;
  var MAX_PROJECTION_CHILD_BYTES = 2 * 1024 * 1024;
  var DEFAULT_RUNTIME_URL = "/manja-assets/local-docs/wasm_exec.js";
  var DEFAULT_WASM_URL = "/manja-assets/local-docs/manja.wasm";

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

  function validateManifest(manifest, descriptor) {
    if (!manifest || typeof manifest !== "object" || manifest.schemaVersion !== 1 || manifest.snapshotId !== descriptor.snapshotId || !manifest.identity || typeof manifest.identity !== "object" || manifest.identity.schemaVersion !== 1 || manifest.identity.catalogId !== descriptor.catalogId || manifest.identity.revisionId !== descriptor.revisionId || manifest.identity.projectionFormat !== descriptor.projectionFormat || !Array.isArray(manifest.children) || manifest.children.length > 10000) {
      fail("manifest identity is invalid");
    }
    var seen = Object.create(null);
    manifest.children.forEach(function (child) {
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
          manifest = JSON.parse(text);
        } catch (_) {
          fail("manifest JSON is invalid");
        }
        validateManifest(manifest, descriptor);
        return global.crypto.subtle.digest("SHA-256", new TextEncoder().encode(JSON.stringify(manifest.identity))).then(function (digest) {
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
      return loadABI(options).then(function (abi) {
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
