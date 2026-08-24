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
      "/manja-assets/local-docs/sw.js": Object.freeze({ length: 61666, sha256: "fd972b00a5e66aeec5de0d33354e0e57c2e2a247c371b9ad8f33e05877993690" }),
      "/manja-assets/local-docs/storage.js": Object.freeze({ length: 43160, sha256: "c3a31f9b8baa1138811aabc4b19d9781db085c450507a60cb52d888b0e10a180" }),
      "/manja-assets/local-docs.js": Object.freeze({ length: 32705, sha256: "471a3b3519369b2a7461bf9f1043f7f7039246e81fb03579c029f483f482d0e8" }),
      "/manja-assets/local-docs/wasm_exec.js": Object.freeze({ length: 16992, sha256: "0c949f4996f9a89698e4b5c586de32249c3b69b7baadb64d220073cc04acba14" }),
      "/manja-assets/local-docs/manja.wasm": Object.freeze({ length: 14506662, sha256: "57066c6b657b319d6abbd38789096e8d393e9f2583c41b782aca3d75c77cb96d" }),
      "/manja-assets/local-docs/manja.wasm.br": Object.freeze({ length: 2664272, sha256: "946672ce1eeccb9ae68d3df1144e32b84ef36f25c56e71fa93f3b2b513cce2cb" }),
    }),
  })
}))
