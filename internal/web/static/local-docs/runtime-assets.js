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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63201, sha256: "505b208cc8206470d833efcb7122bc8d175d7d3c1dd2fce58a0bc99001ce692b" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 76165, sha256: "54b0da03c55d497a422cdd33a4db427dcd73d32ce13b776776484b92892a4711" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 15984276, sha256: "23ade72709385c9853404d700bd7a78f499f4df0f54854ae5ba7891dccb2724a" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2841497, sha256: "7a3b10433ee426be4958cb35c1b356a582b61ca381d0ab62fd3858dcc2504da0" }),
    }),
  })
}))
