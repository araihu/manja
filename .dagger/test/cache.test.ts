import assert from "node:assert/strict"
import test from "node:test"

import { resolveCachePartition } from "../src/cache.ts"

for (const domain of ["fork", "internal"]) {
  test(`${domain} uses stable PR cache partition`, () => {
    assert.equal(resolveCachePartition(domain), "pr")
  })
}

for (const domain of ["main", "release", "assets", "local"]) {
  test(`${domain} keeps an isolated cache partition`, () => {
    assert.equal(resolveCachePartition(domain), domain)
  })
}

for (const domain of ["", "pr", "trusted", "../main"]) {
  test(`rejects caller-selected cache domain ${JSON.stringify(domain)}`, () => {
    assert.throws(() => resolveCachePartition(domain), /unsafe trust domain/)
  })
}
