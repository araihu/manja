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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 57395, sha256: "5dad04bfc55f60dc027c00c98e7538ada7b03869828a48ff3cfd324b27494b28" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 23410, sha256: "5d371c71d4db710f721c1cfccc846c5e390fd40da50147a2efb628bb6f8174ac" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14504586, sha256: "79ab7594150dd84806f8911843752cb092c0658d73ca515406d8f13e6256eb10" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2662294, sha256: "23a98614bccd4ec7b2a6c2add3f0e92d642e4b80557eaf44c242aef3376ded97" }),
    }),
  })
}))
