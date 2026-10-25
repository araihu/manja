import assert from "node:assert/strict"
import test from "node:test"

import { resolvePublication } from "../src/publication.ts"

const sha = "0123456789abcdef0123456789abcdef01234567"

test("main uses SHA build version and main OCI version", () => {
  assert.deepEqual(resolvePublication("branch", "main", sha), {
    buildVersion: sha,
    ociVersion: "main",
    tags: ["main", sha],
  })
})

test("stable tag keeps v for build and removes it from OCI version", () => {
  assert.deepEqual(resolvePublication("tag", "v1.2.3", sha), {
    buildVersion: "v1.2.3",
    ociVersion: "1.2.3",
    tags: ["1.2.3", "1.2", "1", "latest"],
  })
})

for (const [refType, refName] of [
  ["branch", "feature"],
  ["tag", "v1.2.3-rc.1"],
  ["tag", "1.2.3"],
]) {
  test(`rejects ${refType} ${refName}`, () => {
    assert.throws(
      () => resolvePublication(refType, refName, sha),
      /publish ref must be main or a stable SemVer tag/,
    )
  })
}
