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
      "/manja-assets/local-docs.js": Object.freeze({ length: 45227, sha256: "3cc753316d4009442353597e17fc066d69e33bd3ecfdaf90b0bbd505c60addf9" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14559662, sha256: "5f281215d2d64580be6349dac1657303216a7c805c212a098e3aa7b4dad0b78b" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2674988, sha256: "802aef7fefaa0b910202fa298c9b65e2d4d00fc3107019d4a20b6de38b84ec83" }),
    }),
  })
}))
