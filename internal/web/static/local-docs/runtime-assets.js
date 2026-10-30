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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63608, sha256: "ee0baaa0229fc849df93036a4df152005420a637bd48c44bd998df1a42279b1f" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 78851, sha256: "2a32f827e19ca4860077136ae79819734dcb9e95ba2abb84f9e0911a6933de13" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 16553269, sha256: "2159641002d9d56ea9e721f74c8b6c560ae17e5b6709a37b6a8c99df7b2f71f4" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2938256, sha256: "14e9821a38b9a5b5f382ad0a728fe1ce6891edbaeabc69a7c5efc4e4233ef7b2" }),
    }),
  })
}))
