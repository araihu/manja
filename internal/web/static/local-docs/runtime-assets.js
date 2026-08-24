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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 61549, sha256: "8fa63aa055306a78c4cd3092769a631f9d18e0deeee3986ceec8d4fb83054c6c" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 32705, sha256: "471a3b3519369b2a7461bf9f1043f7f7039246e81fb03579c029f483f482d0e8" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14504586, sha256: "79ab7594150dd84806f8911843752cb092c0658d73ca515406d8f13e6256eb10" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2662294, sha256: "23a98614bccd4ec7b2a6c2add3f0e92d642e4b80557eaf44c242aef3376ded97" }),
    }),
  })
}))
