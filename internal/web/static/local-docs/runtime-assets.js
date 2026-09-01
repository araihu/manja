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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 64267, sha256: "b262c3df31ecbd2daa31272539004caebe7f7617929665ce9bf37cb580512167" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 67329, sha256: "965756c2b9af600517483308a0e518d99652a9866c67c84074bdf4084b005d57" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 15809685, sha256: "2975ab44f785d18eb323930d6e76d14372f4408da8a752092f26a7954bebf572" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2817217, sha256: "3761da693ec728260f9c9e20a69b9130cdc47d02a92721c3cefa2833bc61a355" }),
    }),
  })
}))
