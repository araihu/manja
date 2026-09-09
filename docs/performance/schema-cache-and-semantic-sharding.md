# Schema cache and semantic sharding experiment

This extends the static-build streamlining investigation at `9ec0e19`. The user
requested smaller schema files, operation data grouped by path, and a configurable
128 MiB schema cache. These changes were evaluated separately so a failed wire
layout experiment would not obscure the cache's effect.

## Partitioning result

The prototype keeps each deduplicated schema node in its own JSON file, keeps
standalone schema detail records separate, and groups operation detail records by
literal API path. Large paths retain the existing record and byte ceilings.
Ordinals, graph references, public resource identities, and the projection-v2
wire shape stay unchanged. The partition-policy identity changes, so old compiled
snapshots cannot be mistaken for new ones.

This exact interpretation is deliberately fine-grained: a node is not a complete
named schema with all its descendants. It avoids bundling unrelated nodes, but
increases the number of files. The corpus contains 57,444 nodes in 165 original
schema shards. The original node order is sorted by content hash, not semantic
ownership.

The small fixture cold and warm exports pass complete verification. However, this
layout is **not compatible with the full corpus under current manifest admission**:

| Catalog | Old manifest bytes | Prototype manifest bytes | Strict manifest admission |
| --- | ---: | ---: | --- |
| GitHub | 40,140 | 1,339,237 | Pass |
| Kubernetes | 164,104 | 5,732,955 | Fails 4 MiB limit |
| Stripe | 30,257 | 2,067,392 | Pass |
| VMware (both documents) | 550,376 | 24,472,153 | Fails 4 MiB limit |

The manifest repeats every child identity inside the snapshot identity and in its
child list. The catalog also grows, from 14,162,308 to 24,256,481 bytes for VMware.
The existing static export deliberately permits catalogs above the ordinary
4 MiB catalog bound; therefore the decisive new incompatibility here is the
manifest, not merely the strict catalog decoder result in the measurement JSON.
No admission limit or integrity check was relaxed for the experiment.

Named-schema grouping has a second format constraint: current shards require
contiguous node ordinals, while public schema-node links contain those ordinals.
Reordering nodes solely to group them would change existing node links for the
same input. A future grouping layout must preserve ordinals through an explicit
mapping or support non-contiguous node membership.

A production version of this layout needs a compact or hierarchical inventory so
clients can discover and verify individual files without loading every file's
metadata up front. Merely raising the manifest ceiling would impose a large
startup payload. This experiment does not establish that all forms of semantic
partitioning fail: it establishes that the tested one-node/one-schema-detail
layout needs accompanying inventory changes.

The prototype is retained in `semantic-sharding-experiment.patch`, with the
opt-in corpus reconstruction test. It is not active production code. To reproduce,
apply the patch with `git apply --unidiff-zero` in an isolated checkout of
`0ff0302`, set `MANJA_PARTITION_INPUT`
to a verified export directory and `MANJA_PARTITION_REPORT` to a writable JSON
path, then run `go test ./internal/selfhosted -run '^TestSemanticPartitionProfile$'
-count=1 -v`. The test reconstructs projections from the export and uses the real
partitioner and admission decoders; it does not parse the original specs again.

## Search

Search has its own directory, exact-key buckets, token/trigram postings, and
result records. Schema shards are not the search index. The experiment rebuilds
search from the modified document directories and compares every resulting search
file with the original export. All four catalogs' search payloads are identical.
This proves partition-only search-index equivalence, not that an oversized
publication can be admitted: the manifest incompatibility still prevents normal
client activation on the affected catalogs.

Search-index construction occurs during ingestion, before the export CPU profile
starts. This experiment checks its output equivalence, not its ingestion-time
performance.

The selected cache changes neither search construction nor search caching. Full
HTML and search-byte comparison and static browser coverage are recorded below.

## Selected cache implementation

`manja export --schema-cache-mib 128` is the default. The argument accepts a MiB
budget; `0` disables shared retention. Native callers can set
`ExportOptions.SchemaCacheBytes`, with nil selecting the default.

One cache is shared across the export's workers, documents, and catalogs. It stores
only immutable, fully decoded and validated schema shards. The pinned dependency
is [HashiCorp's `golang-lru/v2`](https://github.com/hashicorp/golang-lru) v2.0.7; the adapter adds synchronized byte accounting,
oversized-entry rejection, and eviction before admitting a new entry.

Costs include decoded structs, backing-array capacity, and string payloads, with
twofold allocation headroom plus separately charged keys and entry overhead.
The budget bounds charged retained cache storage, not whole-process RSS or active
workers' temporary values. Concurrent misses may decode the same shard before
insertion; only one copy is retained. There are no cache background goroutines.

Each route still reads and verifies its raw child data. Cache hits skip decoding
and structural/canonical validation of already verified immutable content; they
do not bypass the loaded file's length/hash admission. The key uses document ID
and the schema digest already verified against the manifest. Node selections
return defensive copies of mutable slices. ReleaseChildren drops route inputs,
while shared decoded entries remain eligible for reuse and eviction. Final
verification still checks the complete exported tree.

## Measurements and validation

The shared-cache implementation is committed locally as `0ff0302`. Two controlled
four-worker cold runs took 269.325 and 319.934 seconds, allocating 397.02 and
397.07 GB. The completed warm run took 29.979 seconds and allocated 24.98 GB.
Both cold runs retained 126,482,610 charged bytes with no evictions. This spread
in wall time is why individual timings should not be treated as exact speedups.
The subsequent [HTML-only export](html-only-static-export.md) supersedes this
bundle layout; the bounded-cache repeat queue was stopped after these runs. The complete Go suite, affected race checks, the
Node static/worker tests, asset checks, and strict Muamba checks pass. Frozen binaries and exact patches
are kept under `tmp/perf`; original inputs and environment are recorded in
`static-build-streamlining-inputs.json` and the preceding performance report.

The exploratory `cache128.test` run retained a redundant per-lookup SHA calculation:
302.578 seconds of cold export versus 678.681 seconds for the previous selected
implementation; 396.05 GB allocated versus 1,493.15 GB. Its cache recorded 293,641
hits, 304 misses, no evictions, and 126,472,050 charged bytes at peak. Process RSS
peaked at 591,352 KiB. These are exploratory single-run timings; short checks and
prototype work overlapped. `bounded.test` removes only that redundant hash and is
the isolated cache measurement candidate.

The exploratory cache output has exactly the previous output's path set. All
37,783 HTML files and all 1,436 search files are byte-identical. Changes are limited
to fragment sidecars, the export manifest, and regenerated runtime assets. Warm
export took 31.964 seconds and made no schema-cache accesses because details were
reused. Its timing overlapped regression work and is not a controlled repeat.

The frozen semantic prototype also passes the real Chromium static-host browser
scenario at `/` and `/group/project/` on the small multi-catalog fixture, including
cross-catalog search, selection/navigation, lazy schema/group loading, and cached
offline behavior. This small-fixture pass does not remove the full-corpus manifest
failure.
