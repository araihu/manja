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
      "/manja-assets/local-docs.js": Object.freeze({ length: 80664, sha256: "3bb63cbd17d585dc42137d99f773a85dc8cd655c5e33fbc1b3a0d9d25f7a6961" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 16697112, sha256: "2a6f3b93bbae662256b603e9d4decd19447541aabf4c9db5c6a3656783191986" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2956732, sha256: "68e553a4251ab464c9e825c6504b3b3c38e3d4e0334d317f8815d36aef12ddb0" }),
    }),
  })
}))
