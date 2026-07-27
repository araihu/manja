import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import test from "node:test";

const fixtureRoot = new URL("../../../application/projection/testdata/", import.meta.url);
const excludedSentinels = [
  "__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__",
  "__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__",
];

async function readVector(name, expectedBytes, expectedDigest) {
  const bytes = await readFile(new URL(`${name}.json`, fixtureRoot));
  assert.equal(bytes.length, expectedBytes);
  assert.notEqual(bytes.at(-1), 0x0a, "golden must have no final newline");
  assert.equal(createHash("sha256").update(bytes).digest("hex"), expectedDigest);
  assert.equal((await readFile(new URL(`${name}.sha256`, fixtureRoot), "utf8")).trim(), expectedDigest);
  for (const sentinel of excludedSentinels) {
    assert.equal(bytes.includes(Buffer.from(sentinel)), false);
  }
  return JSON.parse(bytes.toString("utf8"));
}

test("dependency-free consumer verifies canonical projection vectors", async () => {
  const empty = await readVector(
    "v1-empty",
    872,
    "8267e1a8a597a6561409e81492b06c24b44b6cbd12875fc90985295c5765889d",
  );
  assert.equal(empty.formatVersion, 1);
  assert.equal(empty.projectId, "payments");
  assert.equal(empty.revisionId, "rev-0001");
  assert.deepEqual(empty.mainLandmark, { id: "main-content", role: "main" });
  assert.equal(empty.overview.anchor, "overview");
  assert.equal(empty.overview.href, "?selected=overview#overview");

  const operation = await readVector(
    "v1-operation",
    2780,
    "6609c4e78e6556c8a178e500aeff8da85801ce30aaa784129c85b2c4e63cdc41",
  );
  assert.equal(operation.projectId, "pets");
  assert.equal(operation.revisionId, "rev-0002");
  assert.equal(operation.operations[0].ordinal, 0);
  assert.equal(operation.operations[0].id, "operation-list/pets");
  assert.equal(operation.operations[0].anchor, "operation-list/pets");
  assert.equal(operation.operations[0].href, "?selected=operation-list%2Fpets#operation-list/pets");
  assert.equal(operation.operationDetails[0].headingId, "operation-list/pets");
  assert.equal(operation.search[0].resultId, "search-result-1e46984659c11bcb062b8421268996f857f71a88283c62ea0a79d7f47ec4c4b1");
  assert.equal(operation.publicRoutes[1].path, "/?selected=operation-list%2Fpets#operation-list/pets");
});
