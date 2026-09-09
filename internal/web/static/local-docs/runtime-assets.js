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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 63201, sha256: "505b208cc8206470d833efcb7122bc8d175d7d3c1dd2fce58a0bc99001ce692b" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 78697, sha256: "878455c6e5773f03970732b80b6e4caa7261afd0273c85543dcef0326c49334e" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 15814221, sha256: "6f923aea52d43129fefff1d5cc993b9543660381b4f223ed30338544b898c118" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2816672, sha256: "936ce14086bbc068dbac56be866eada7fd42a5a840d5e479b2ac5bd741370761" }),
    }),
  })
}))
