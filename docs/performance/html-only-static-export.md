# HTML-only static exports

Static exports now publish prebuilt, verified HTML instead of shipping a second
rendering representation. Detail/schema-node projection JSON, `manja.wasm`, its
Brotli copy, and `wasm_exec.js` remain available to browser-rendered deployments
and build inputs, but are absent from a new static bundle.

## Dependency audit

Operation navigation, lazy standalone schema trees, grouped sidebar chunks,
and examples already consume prebuilt HTML. Search has independent directory,
posting, exact-key, and result files. Neither path needs projection JSON.
The remaining node-navigation gap was `?selected=…&node=N`: loading only the
selected schema's root HTML did not render the requested node. Export now walks
schema roots and their rendered reference links, emitting each reachable node
panel once per document. The browser verifies the root and shared panel sidecars,
replaces the node panel, and binds its reference links to the selected schema.
This preserves the schema heading/example and node ordinals without duplicating
every reachable panel for every named schema. Cycles terminate through ordinal
deduplication.

The build captures each needed projection child once from the snapshot handler
with expected length and SHA-256 into private disk scratch space, then serves
subsequent reads from that file. Concurrent first requests for the same child
share one capture; different children can load concurrently. Scratch files are
removed when the catalog finishes and never enter the publication tree. Every
browser read still undergoes admission before using the shared decoded cache. The snapshot manifest and
catalog directory remain as provenance and verification metadata; the export
receipt is the authoritative published-file inventory.

## Integrity and compatibility

New receipts declare `rendering: "html"`, paired with `static.htmlOnly` in shell
descriptors. Existing receipts remain verifiable and usable as warm-build inputs.
The existing executable identity invalidates stale HTML and sidecars. Static
navigation reports unavailable HTML instead of attempting WASM rendering.

Verification still hashes every declared file and enforces the exact file set.
It additionally requires operation/schema roots, initial sidebar chunks and their
referenced group/chunk chains, lazy trees and node panels, validates every HTML fragment's canonical sidecar, and checks panel
ordinals and content digests. Negative tests remove a required HTML/sidecar pair
and update the declared inventory (including sidebar/group cases); verification
still rejects the incomplete
bundle. Node panels must contain exactly one correctly identified panel.

Browser acceptance covers a generic static server at `/` and `/group/project/`,
cross-catalog search, lazy operation/schema/sidebar loading, nested schema-node
navigation, reload, history, and visited content offline. The test asserts both
that excluded payloads are absent and that no interaction requests them.

## Measurements

Results are recorded from fixed binaries with GOMAXPROCS=4, GOMEMLIMIT=512MiB,
and the unchanged input corpus described in static-build-streamlining.md.
Cold means a fresh export directory with compiled source snapshots retained;
warm reuses the previous valid output. Source ingestion and standalone full
verification are measured separately from export. Cache retention is a charged
estimate; RSS measures the complete process and can exceed the cache budget.

The first HTML-only prototype routed every raw-child read through HTTP. Its full
four-worker export verified in 542.350 seconds but allocated 1,349.10 GB. The
allocation profile identified 481.14 GiB in snapshot cloning and 324.98 GiB in
`io.ReadAll` (profile totals also include ingestion). This regression justified
the private disk scratch cache in the selected implementation; the prototype is
not the selected performance result.

That prototype produced 108,609 files and 603,893,352 logical bytes, compared with
77,391 files and 664,001,490 bytes for the cache-only implementation. It removed
303 projection files and three WASM/loader files (133,479,428 bytes), adding
15,762 node panels and their sidecars (63,994,004 bytes). The remaining difference
includes the larger export inventory. More HTML files are the cost of completing
deep-node navigation without a runtime renderer.

All 1,436 search files are byte-identical. Of 37,700 existing HTML fragments,
15,233 differ only by the schema-node ordinal attribute; the remainder are
byte-identical. Source downloads are unchanged. New shell descriptors, asset
metadata, build keys and export inventory are intentional changes.

The private-cache implementation reduced that first prototype to 390.966 seconds
and 443.75 GB allocated, with identical published bytes. Its CPU profile attributes
23.34% cumulatively to detail-shard decoding, 25.00% flat to system calls, and
14.38% cumulatively to file synchronization. These percentages overlap and must
not be added. Node-panel emission accounts for 9.59% cumulatively. Detail decoding
and durable artifact I/O are the remaining bottlenecks.

The final implementation is committed as `b9619ba`; the additional sidebar-chain
verification passes negative, browser, and race checks. Its first full four-worker
cold export took 390.816 seconds and allocated 444.63 GB. The first warm export
took 40.225 seconds, allocated 28.26 GB, reused all 53,462 fragments, and recorded
zero schema-cache accesses. At the user's request, further fire-corpus repeats were stopped. The interrupted
second cold run is excluded. Remaining repeat, worker-scaling, cache-override and
binary-invalidation checks use the bounded fixture below.

## Bounded benchmark and toolchain

All measured binaries use Go 1.27.1. This installed toolchain defaults JSONv2 to
true: `go list -f '{{.GoFiles}}' encoding/json` selects its `v2_*.go` files, and
the CPU profiles contain the v2 implementation. The final bounded binary also
sets `GOTOOLCHAIN=go1.27.1 GOEXPERIMENT=jsonv2` explicitly. These are not measurements
of a JSON v1-to-v2 migration; the earlier baseline already used the v2 engine.
Existing `encoding/json` compatibility semantics are preserved.

The deterministic input has 256 operations, 130 schemas, 16 sidebar groups, shared
nested schema references, and 158,030 source bytes. SHA-256:
`7aa9621c6d9d54797087df1f601ff311cd51d7a278d25c800195ceee49453a7c`.
It exercises repeated schema access, lazy schemas/groups and node panels without
requiring the fire-test corpus. Two cold/warm pairs were run at each worker count.

| Implementation | Workers | Cold export seconds | Warm export seconds | Cold allocated GB |
| --- | ---: | ---: | ---: | ---: |
| Original baseline (one pair) | 4 | 19.025 | 0.667 | 26.533 |
| Final HTML-only | 1 | 8.598–9.964 | 0.816–1.006 | 2.955–2.960 |
| Final HTML-only | 4 | 4.735–9.917 | 0.682–0.704 | 2.938–2.940 |

The broad cold wall-time spread limits worker-speedup claims. Allocations are
stable and approximately 89% below the original baseline. Small-fixture warm
latency does not improve over the original baseline. The cache-only intermediate
version took 4.221 seconds cold and allocated 2.866 GB; HTML-only output adds node
panels and more complete dependency verification, so it is not claimed to improve
on that intermediate version's rendering time.

The bounded output has 2,273 files and 16,573,498 bytes, versus 2,015 files and
34,769,609 bytes for the original baseline. All 26 search files are byte-identical.
All six alternate outputs (worker repeats, invalidated rebuild and cache overrides)
have identical verified file inventories to the reference output. Warm runs reuse
all 1,076 fragment files; the six shells are rebuilt. Schema-cache accesses are zero
on warm builds.

With four workers, the default cache recorded 8,203 hits and four concurrent
misses, retaining 738,770 charged bytes. Disabling the cache took 25.214 seconds and
allocated 33.600 GB. A 16 MiB override verified successfully with identical output
and retained the same working set. This fixture does not exercise eviction at
16 MiB; the cache's concurrent budget/eviction tests cover that contract. Rebuilding
a copied older cache-only bundle took 5.400 seconds, reused zero old HTML inodes,
and produced exactly the fresh final output.

The existing full-corpus result remains useful scale evidence: 390.816 seconds
cold and 40.225 seconds warm, versus the original single baseline's 994.151 and
101.778 seconds. Peak full-process RSS was 575,512 KiB cold and 556,852 KiB warm.
Do not treat this single-pair comparison as a statistically established speedup.
The full corpus includes both large VMware documents. No further full-corpus
benchmark is required for this task.

## Reproduction and gates

Generate the bounded input and build a fixed profiling binary:

```bash
python3 docs/performance/generate-export-fixture.py /tmp/manja-profile
GOTOOLCHAIN=go1.27.1 GOEXPERIMENT=jsonv2 go test -c \
  -o /tmp/manja-profile.test ./internal/selfhosted
GOMAXPROCS=4 GOMEMLIMIT=512MiB \
MANJA_PERF_CONFIG=/tmp/manja-profile/renderer.yaml \
MANJA_PERF_DATA=/tmp/manja-profile/data \
MANJA_PERF_OUTPUT=/tmp/manja-profile/public \
MANJA_PERF_REPORT=/tmp/manja-profile/cold \
MANJA_PERF_WORKERS=4 \
  /tmp/manja-profile.test -test.run '^TestExportProfile$' -test.v
```

Repeat the same command and binary with a new report prefix for warm timing;
use a fresh output directory for another cold run. Set
`MANJA_PERF_SCHEMA_CACHE_MIB` to override the default 128 MiB budget. The harness
records ingestion, export, full verification, allocations, cache statistics and
output size separately, plus CPU/heap profiles. Raw results and frozen binary/
patch hashes are in `html-only-results.json`; timings include profiling overhead.

The complete Go suite passed, including browser-rendered deployment tests.
Final affected export/browser and race checks passed after the private-input and
sidebar-completeness changes. The 43 JavaScript tests, asset checks, strict Muamba
checks and diff whitespace checks passed. WASM, Brotli and runtime metadata were
regenerated with Go 1.27.1 for the shared render changes. The static export browser
test verifies generic root/subpath hosting without projection/WASM requests.

The separate semantic-sharding prototype remains experimental: its enlarged
inventory exceeds current manifest limits. This change preserves the production
partition format and ships the bounded cache and HTML-only publication changes.
