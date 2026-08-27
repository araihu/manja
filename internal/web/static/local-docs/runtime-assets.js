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
      "/manja-assets/local-docs.js": Object.freeze({ length: 37563, sha256: "71c8ee3638045e2eab0caf89fdba9dd139705e0d4af9ffb1afa7848a27bce0b1" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14539807, sha256: "128fa2978caf0a36c5913227686b9eb42d6d1dc2e90aa22143f247188f2d556e" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2671922, sha256: "219300b3caaa52a1e4431e6bb89a2d99bdebca9c265089f3007a21ab569c62f9" }),
    }),
  })
}))
