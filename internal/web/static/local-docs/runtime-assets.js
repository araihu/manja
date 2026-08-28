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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63879, sha256: "d4520170724daba8bf055d5ae36a0fc62b57f3b8c920e8e448565a60c4b1e821" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 52072, sha256: "6f9423015edc06223efc7572b83707d5be58c3712829222ab6a15f6e59a6fecd" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 15772810, sha256: "9d1f230aa372f81f6762d249de5fff6e69f64de485493abd6b961605de0b3e42" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2813712, sha256: "4c5f8868182590f6f21a756580d6cfacbc3b41a5e52ba2e08fad8e5056b45271" }),
    }),
  })
}))
