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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63523, sha256: "2ef0a75ca968780fafc7f7f8aa3923e864e4f8bf6f887a42b3d02570da58489b" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 76165, sha256: "54b0da03c55d497a422cdd33a4db427dcd73d32ce13b776776484b92892a4711" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 17275860, sha256: "f431a2cb4ab01d71e07589c7612a35c2dd4c727767d38b801622f410896f79f6" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2988410, sha256: "ce52c57c44bf720e96505a9dea595a5dec6acbcab1f74fc49c2a6831c968ca25" }),
    }),
  })
}))
