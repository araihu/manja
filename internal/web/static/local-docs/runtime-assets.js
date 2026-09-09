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
      "/manja-assets/local-docs.js": Object.freeze({ length: 78079, sha256: "4ebe9525a61d2ee658bb6001e8ec623101c6fde8704fc3705baf363d785de088" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 17287779, sha256: "8c846450706ef89a7b34b8ba1e24f9ee9149d6f11ed4e3af8127cfd73b39d590" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2993699, sha256: "b9cb12088bf1a77886e2f719757065e531ae652a736fc75026e7c97dd68907dc" }),
    }),
  })
}))
