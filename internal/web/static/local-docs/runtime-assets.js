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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 57264, sha256: "4d0e0d4ee15c380e0340c375c1da482ccf35cdd5cf93b650accebb3a2d7dc3d9" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 38283, sha256: "0015b9ed3a81ebf3ecb832c07d797dbbf53149c27d5cc0465805ed4dd595e260" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 23410, sha256: "5d371c71d4db710f721c1cfccc846c5e390fd40da50147a2efb628bb6f8174ac" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 2120526, sha256: "ac0f768328de603c27820941f0fb9248e29c55a9baa0d7824e3822b92d352aab" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 475479, sha256: "3ed06ecf74038cc79fd46e56ccec375b6ca803c86829b8537c0be9b67df63324" }),
    }),
  })
}))
