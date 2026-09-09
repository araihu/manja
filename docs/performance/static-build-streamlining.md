# Static export streamlining — 2026-09-08

This report records the earlier implementation through `9ec0e19`. The subsequent
configurable schema cache and semantic-partition experiment are documented in
[schema-cache-and-semantic-sharding.md](schema-cache-and-semantic-sharding.md).
The latest published-bundle change is documented in
[html-only-static-export.md](html-only-static-export.md).

Base: `6ee0252301ff6fd060d206abf5fb574de8b211b2` (`origin/main`). Worktree:
`/tmp/manja-static-build-streamlining`, branch `perf/static-build-streamlining`.

## Change and dependency contract

`Browser.RenderMain` uses the same main rendering implementation as `Render`,
without entering sidebar rendering. `Render` still produces both regions.
Document, operation, schema, schema-node, missing-route and nil-browser behavior
are covered; root and deployment-subpath HTML, titles, canonical URLs and errors
are compared. The exporter uses the main-only entry point and retains its existing
fallback behavior and per-detail child release.

The exporter hashes the complete document directory and publication base once
per document, before dispatching jobs. Per-detail payloads contain the shared
digest, the complete child identity and the explicit `detail-dependencies-v2`
scheme marker. Schema shard references remain inside the complete document;
removing their formerly duplicated JSON representation does not narrow the
dependency scope. Compiler identity is encoded once per catalog. Binary, renderer,
UI, normalizer, fragment kind and resource still participate in the build key.
Example artifacts derive their identity from the new detail payload as before.

This intentionally changes detail/example build keys and metadata. Older keys
miss normally. HTML integrity verification, cache verification and full export
verification are unchanged. No worker-count default, sidebar chunk setting,
publication barrier, file-writer concurrency or static-hosting behavior changes.

A third, separately measured change retains only the last fully validated schema
node shard in each browser. Previously every node selection decoded and validated
the entire shard again, including repeated selections during one operation render.
Preparation still verifies the admitted identity, byte length/hash, document key,
canonical encoding and complete shard validity. Selections check ordinal bounds
and return defensive copies of every mutable slice. The cache is private to each
browser, forks start empty, and `ReleaseChildren` clears it after each detail.
The cache holds one shard rather than accumulating a deployment-wide decoded
schema inventory. Returned nodes cannot mutate later selections; failed admission
cannot populate the cache.


## Reproduction and measurement

The opt-in `TestExportProfile` harness is compiled once per revision with
`go test -c -o tmp/perf/<variant>.test ./internal/selfhosted`. Normal test runs skip
it. Set `MANJA_PERF_CONFIG`, `MANJA_PERF_DATA`, `MANJA_PERF_OUTPUT`,
`MANJA_PERF_REPORT` (file prefix), and `MANJA_PERF_WORKERS`, then run the fixed
binary with `-test.run '^TestExportProfile$' -test.v -test.timeout=4h`.
`MANJA_PERF_INGEST_ONLY=1` prepares a store without exporting.

The harness times renderer activation/ingestion separately from export and from
an additional complete verification. Export timing includes staging, fragment
work, search metadata, mandatory staging verification and publication. CPU
profiling starts after ingestion; allocation counters cover export only. Heap
profiles include cumulative ingestion allocations as well as export allocations.
Process user/system CPU and peak RSS include ingestion, profiling and the extra
verification, so they are not interchangeable with export-only wall time.
Compilation is excluded. OS file caches are not flushed. A cold output has no
fragment cache; warm output reuse uses the exact same executable and output.

Hardware: Linux/arm64 OrbStack on Apple hardware, Go 1.27.1. The VM exposes 12
CPUs, but the observed cgroup quota is 6 CPUs and 9 GiB memory. Benchmark outputs
are on disk under `/home/gui/manja-static-build-bench`; `/tmp` is tmpfs. The
controlled full-corpus runs use `GOMAXPROCS=4`, `GOMEMLIMIT=512MiB` for both
versions and vary only fragment workers between 1 and 4. The Go memory setting
is a soft runtime budget, not a process RSS cap.

The input corpus contains 69 documents: Kubernetes (65), GitHub (1), Stripe (1),
and VMware (2: `vcenter.yaml`, `vi-json.yaml`), totaling 5,876 operations and
15,233 schemas. Full and small corpora use separate stores. Input hashes,
configuration files, exact binary/patch SHA-256 values, profiles, logs, timings,
HTML comparisons and the orchestration scripts are retained in the worktree's
`tmp/perf/` directory. `baseline.patch` is the opt-in profiling harness applied
to the base; `candidate-complete.patch` captures the measured candidate including
its generated assets and benchmark tests. Subsequent regression-only additions
do not change the measured production code.

The first unbudgeted full baseline was killed with SIGKILL after 1,212.90 s;
the environment recorded OOM kills. Its incomplete timing is excluded. Concurrent
preview/build/test activity was observed. A concurrent broad `go test ./...`
run timed out in several packages, so those results do not establish a passing
full-suite gate. Do not attribute all variation in wall time to this patch.

## Isolated profiles

Three repetitions, synthetic 2,000-operation navigation inventory, verified small
projection children; the selected detail and child-release behavior are unchanged.
These measurements isolate mechanisms and are not whole-export speedups.

| Work per detail | Original | Candidate |
| --- | ---: | ---: |
| Main rendering call, wall range | 9.08–11.71 ms | 2.21–2.93 ms |
| Main rendering call, allocated bytes | ~12.28 MB | ~1.18 MB |
| Identity payload/hash, wall range | 1.08–1.33 ms | 1.07–1.12 µs |
| Identity payload/hash, allocated bytes | ~416 KB | 528 B |

The identity candidate excludes the once-per-document preparation, which must
be amortized over the document's details. It is not a claim that hashing an
entire document now costs one microsecond.

In the original detail microprofile, `renderSidebar` accounts for 31.67% of CPU
samples cumulatively and 89.26% of allocated bytes. It is absent from the
candidate detail CPU profile. In the original identity microprofile, JSON
marshaling accounts for 75.85% of CPU samples cumulatively.

After sidebar removal, `PrepareOperationNavigation` accounts for 63.30% of CPU
samples and 84.31% of allocated bytes in the synthetic detail benchmark. Its
whole-inventory admission is a measured remaining candidate, but safely reusing
that admission requires preserving the existing consistency checks. This patch
does not change navigation admission. The full-corpus profile instead justified
the single-shard decoded cache described above.

## Small fixture

Two cold and two unchanged warm runs for each worker count, one operation and
one schema, using the final runtime budget (`GOMAXPROCS=4`, `GOMEMLIMIT=512MiB`).
Mean export-only wall time (raw results use repetition labels 3 and 4):

| Workers | Cache | Original | Final |
| ---: | --- | ---: | ---: |
| 1 | Cold | 0.324 s | 0.218 s |
| 1 | Warm | 0.224 s | 0.194 s |
| 4 | Cold | 0.201 s | 0.216 s |
| 4 | Warm | 0.233 s | 0.212 s |

All 14 HTML files compare byte-for-byte equal. Warm runs retain the 8 HTML
fragment inodes; shell regeneration accounts for the remaining HTML. This
inode count is not an instrumented cache-miss counter. Small-export allocations
remain roughly 215 MB cold / 244 MB warm, dominated by fixed export work rather
than the tiny directory. No meaningful small-fixture memory reduction is claimed.
These subsecond wall timings are noisy and do not show a consistent speedup.

Both versions publish 125 files. Logical output bytes change from 25,991,825 to
26,007,661 because the WASM, Brotli companion and asset manifest are regenerated.
The Brotli output was decompressed and compared against the raw WASM before
updating the intentional asset golden hashes. Earlier default-budget small runs
for the two-change intermediate candidate remain in the raw artifacts.

## Full corpus and readiness

The first valid original cold export with 4 fragment workers took 994.151 s
(excluding 193.314 s ingestion), allocated 2,990,879,604,960 bytes in
14,088,187,585 allocations, and published 77,391 files / 663,988,005 logical bytes.
Additional verification took 6.198 s. Process totals: 1,194.330 s wall,
3,047.371 s user, 236.372 s system, peak RSS 612,716 KiB.

Its export CPU profile attributes 59.31% of samples cumulatively to
`DecodeSchemaNodeShard` and 25.81% to duplicate-JSON-key validation (nested within
that decoding work; these percentages must not be added). In the same full profile, sidebar rendering is 4.09% of CPU samples;
per-detail document JSON encoding takes 221.17 CPU seconds (7.11%), and its
payload hash line takes 57.75 CPU seconds (1.86%). Source-line attribution uses
the saved original source with pprof `-trim_path` / `-source_path`, not the
modified worktree source. These results changed the optimization priority
relative to the synthetic navigation microbenchmark.

The intermediate two-change candidate took 857.263 s for export, with
1,840,029,554,384 allocated bytes, 77,391 files and 663,991,996 logical bytes.
Process totals: 954.097 s wall, 2,621.245 s user, 185.271 s system, peak RSS
556,920 KiB; ingestion 89.660 s, additional verification 6.322 s. Against the
first valid baseline, export wall time falls 13.8% and allocated bytes 38.5%.
All 37,783 HTML files compare byte-for-byte equal. Its CPU profile still spends
70.78% cumulatively in `DecodeSchemaNodeShard`, motivating the third change.

The selected three-change final candidate took 678.681 s for export and
allocated 1,493,149,899,872 bytes. It published the same 77,391 paths and all
37,783 HTML files compare byte-for-byte equal. Logical output is 664,003,841 bytes;
all changed files are the expected fragment sidecars, export manifest and runtime
assets. Process totals: wall 783.989 s, user 2,007.623 s, system 171.442 s,
peak RSS 557,948 KiB. Ingestion took 93.646 s and additional verification 10.968 s.
The first unchanged warm export took 33.659 s / 24,851,378,760 allocated bytes,
against the original's 101.778 s / 223,281,007,968 allocated bytes.

These are first-run values for this earlier implementation. The follow-up cache
and HTML-only reports record later measurements and completed regression gates;
this original baseline was not repeated with every worker configuration. The selected final profile still attributes 66.10% of CPU
samples cumulatively (1,352.44 CPU seconds) to schema-node shard decoding.
Repeated decoding on shard switches and across details remains the main measured
cold-export limitation; the patch does not add a larger retained schema cache.

An additional experiment retained the single decoded shard between details while
still verifying newly admitted bytes. It allocated 1,477,922,194,096 bytes, only
about 1% below the selected final implementation, and took 738.891 s. Concurrent
serial test load limits timing interpretation. The extra API/state lifetime had
no demonstrated wall/CPU benefit, so that refinement was omitted. Its binary and
profiles are retained under the `streamlined` label for audit. The measured final binary
is based on `01cd0b5` plus `tmp/perf/final-complete.patch`; exact binary and patch
hashes are in `tmp/perf/final.sha256`. The earlier `candidate` label refers to the
intermediate two-change version; `final` refers to all three improvements.
