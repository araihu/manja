(function (root, factory) {
  "use strict"
  const manifest = factory()
  if (typeof module === "object" && module.exports) module.exports = manifest
  root.ManjaLocalDocsAssetManifest = manifest
}(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict"

  // Generated from the embedded production runtime bytes. This companion
  // stays separate so sw.js can validate its own bytes without recursion.
  return Object.freeze({
    schemaVersion: 1,
    assets: Object.freeze({
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 61654, sha256: "02ecd501f7b46981fb122cba1d091daf945f029f5dfd07f10de7f6c634c3cabb" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 36533, sha256: "92efd4bd600e829ecb08bf9299f7d0b45c728e9a52db07cb122800eb12086fa4" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14508019, sha256: "6694bc7dde8b75076eac234e3d3809266a34c02bf29569643688c6a92c3c3356" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2672172, sha256: "709885ee3e8b59a1a45e296b000e0170d6f630977e51cb6ed7fafbe6805f1587" }),
    }),
  })
}))
