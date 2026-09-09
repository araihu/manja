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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63286, sha256: "32c42c07ddd22ada69bfc65d61ee4bf52d06f88eb7a44d7cfe765a6b19655422" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 80545, sha256: "4e15aba066e008cdffa48e9073d4121a2801334e7c2749f65e55d9399beecad2" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 15825837, sha256: "51c55cb6de451b058fa6dee220d6fcce86e1f4f8d08e48b0028728b468eb6ca9" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2823187, sha256: "d3387e658be5e28b014e8bbdc53a9c5d40300e451bf3921b6e879ed3285842d1" }),
    }),
  })
}))
