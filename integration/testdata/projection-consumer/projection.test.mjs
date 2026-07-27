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
    "v2-empty",
    889,
    "2f69e78dff07b7275ab021ca6c8006332ec822703f606fec086f06538df0642c",
  );
  assert.equal(empty.formatVersion, 2);
  assert.equal(empty.projectId, "payments");
  assert.equal(empty.revisionId, "rev-0001");
  assert.deepEqual(empty.mainLandmark, { id: "main-content", role: "main" });
  assert.equal(empty.overview.anchor, "overview");
  assert.equal(empty.overview.href, "?selected=overview#overview");

  const operation = await readVector(
    "v2-operation",
    2797,
    "93281a850731cf51d286a27f1eb40468b202b28ccc39c8be5406eb0f9608e95d",
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

  const full = await readVector(
    "v2-full",
    10129,
    "639c40aeb0afcce1da80e04dbe672c31860086543bd3ed1247ec70f91563b573",
  );
  assert.equal(full.formatVersion, 2);
  assert.equal(full.schemaNodes.length, 6);
  assert.deepEqual(full.schemaNodes.map((node) => node.ordinal), [0, 1, 2, 3, 4, 5]);
  const nodeIds = full.schemaNodes.map((node) => node.id);
  assert.deepEqual(nodeIds, [...nodeIds].sort());
  const rootRefs = [
    ...full.operationDetails.flatMap((detail) => detail.parameters.map((parameter) => parameter.schemaRef)),
    ...full.operationDetails.flatMap((detail) => detail.requestBody.mediaTypes.map((media) => media.schemaRef)),
    ...full.operationDetails.flatMap((detail) => detail.responses.flatMap((response) => response.mediaTypes.map((media) => media.schemaRef))),
    ...full.schemaDetails.map((detail) => detail.schemaRef),
  ];
  assert.deepEqual(rootRefs, [5, 0, 3, 1, 4]);
  assert.equal(rootRefs.every((ref) => Number.isInteger(ref) && ref >= 0 && ref < full.schemaNodes.length), true);
  assert.equal("schema" in full.operationDetails[0].parameters[0], false);
  assert.equal("schema" in full.schemaDetails[0], false);
  assert.equal(full.schemaDetails[1].exampleSchemaJSON, '{"shape":1}');
  assert.equal(full.schemaDetails[1].examples[0].text, "__EXPLICIT_PET__");
});
