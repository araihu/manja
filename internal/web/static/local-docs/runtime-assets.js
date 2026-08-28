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
      "/manja-assets/local-docs.js": Object.freeze({ length: 45188, sha256: "dcc30c0d4f15317a87244008ea09f0067d8226ac3b0f7026e0f98e00b2308c62" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14559662, sha256: "5f281215d2d64580be6349dac1657303216a7c805c212a098e3aa7b4dad0b78b" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2674988, sha256: "802aef7fefaa0b910202fa298c9b65e2d4d00fc3107019d4a20b6de38b84ec83" }),
    }),
  })
}))
