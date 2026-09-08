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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63523, sha256: "2ef0a75ca968780fafc7f7f8aa3923e864e4f8bf6f887a42b3d02570da58489b" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 78697, sha256: "878455c6e5773f03970732b80b6e4caa7261afd0273c85543dcef0326c49334e" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 17123608, sha256: "4976d724949b4f717df941ed1b1369959f3f36bef9e3bece781763aad7237058" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2967192, sha256: "baaa90dd966407bdf4f432442ff716f0a9227aae4a8a9e5ea6dd5eb8f842fd1f" }),
    }),
  })
}))
