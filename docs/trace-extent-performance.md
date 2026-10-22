# Whole-range trace extent indexing

Baseline: `1a3e7992da0f67fd3a5d4d1d1c37d1e93f10176f`, with the new benchmark
fixture added before production changes. Candidate: the per-worksheet sparse
index in this change. Measured on the same Apple M5 Pro (18 logical CPUs,
48 GiB RAM), macOS 26.5, Go 1.26.4 darwin/arm64, default GOMAXPROCS=18.

## Method and limitations

Each fixture is a preconstructed OPC package with Inputs populated from A1
through CV1000 or CV10000. Output!A1 is SUM of K distinct whole columns, for
K=1,10,100. Full-call measurements invoke
`TraceWorkbook(context.Background(), p, "Output", "A1", 200, 20)` and include
OpenWorkbook/XML parsing, but exclude fixture construction and ZIP loading.
The isolated benchmark opens the workbook before timing and creates a fresh
tracer per iteration; all K queries, including the first index build, are timed.
Both versions used `-benchtime=1x -count=5` with `b.ReportAllocs()`. Tables show independent
medians of ns/op, B/op and allocs/op, not a selected fastest sample.

The baseline and candidate benchmark suites ran sequentially on the same host.
This was a shared development machine: unrelated tests and benchmarks, and
brief compilation/verification work, ran during measurement. No CPU pinning,
thermal control or cold filesystem-cache control was applied. The one-iteration
samples expose GC and host-load variation; the raw samples are retained in
[before.txt](benchmarks/trace-extent/before.txt) and
[after.txt](benchmarks/trace-extent/after.txt). These are measured development
results, not statistical confidence intervals or an idle-host guarantee.

## Full TraceWorkbook medians

| Rows | K | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op | Speedup |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1,000 | 1 | 122,821,375 | 83,531,667 | 81,534,600 | 80,049,496 | 1,625,752 | 1,425,784 | 1.47× |
| 1,000 | 10 | 440,830,625 | 82,289,250 | 96,026,072 | 80,060,184 | 3,425,862 | 1,425,875 | 5.36× |
| 1,000 | 100 | 4,307,948,833 | 90,206,917 | 240,423,088 | 80,166,888 | 21,426,730 | 1,426,699 | 47.76× |
| 10,000 | 1 | 1,240,902,708 | 959,171,000 | 857,973,256 | 843,071,416 | 16,249,426 | 14,249,537 | 1.29× |
| 10,000 | 10 | 4,237,927,792 | 949,785,791 | 1,002,174,376 | 843,081,960 | 34,249,548 | 14,249,627 | 4.46× |
| 10,000 | 100 | 41,593,421,042 | 956,356,167 | 2,442,639,368 | 843,133,976 | 214,250,435 | 14,250,446 | 43.49× |

## Isolated savedExtent medians

| Rows | K | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op | Speedup |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1,000 | 1 | 35,771,125 | 26,153,500 | 1,603,320 | 118,520 | 200,005 | 39 | 1.37× |
| 1,000 | 10 | 336,904,000 | 27,033,708 | 16,007,272 | 118,520 | 2,000,012 | 39 | 12.46× |
| 1,000 | 100 | 3,272,097,875 | 26,815,208 | 160,030,256 | 118,520 | 20,000,029 | 39 | 122.02× |
| 10,000 | 1 | 445,240,500 | 340,351,500 | 16,003,320 | 883,032 | 2,000,005 | 98 | 1.31× |
| 10,000 | 10 | 4,641,351,208 | 325,189,209 | 160,007,640 | 883,032 | 20,000,016 | 98 | 14.27× |
| 10,000 | 100 | 35,671,110,584 | 325,178,334 | 1,600,031,536 | 883,032 | 200,000,043 | 98 | 109.70× |

## Acceptance and behavior

For K=100,N=100,000, isolated median improves **122.02×** (target ≥5×),
and full-call median improves **97.91%** (target ≥20%). At N=1,000,000,
the corresponding improvements are 109.70× and 97.70%. The full-call target
is met; the loading cost does not erase the value of removing repeated scans.

K=1 time decreases 26.9%/23.6% in isolation and 32.0%/22.7% for the full call
(1,000/10,000 rows). Allocated bytes decrease 92.6%/94.5% in isolation and
1.8%/1.7% for the full call. All are inside the ≤10% regression bounds.

`TestTraceExtentEnumeration` instruments context checks at cell visits,
subtracting an identical cached-query workload and the final publication check.
It measures exactly **100,000** worksheet cell visits for 100 column queries;
the retained test-only legacy scan measures exactly **10,000,000**. This measures
the extent algorithm, not the separate XML enumeration needed by OpenWorkbook.

The index stores min/max row per populated column and min/max column per
populated row. It is lazy, per worksheet, per tracer. Narrow intervals probe
individual map entries; wide intervals enumerate only populated axes. Thus
single-column queries are O(1) after an O(N) build, with O(populated rows +
populated columns) index storage. Map construction is local until every cell
has been inspected and a final cancellation check passes. Queries also check
cancellation, including index hits. There is no early stop at the node budget.

The populated predicate remains the presence of a namespace-correct f/v/is
child, including empty elements, ignoring style-only cells and invalid
coordinates. The predicate now iterates XML child tokens directly, avoiding
temporary ChildElements slices. This offsets the cost of building both axis
maps even for K=1. Reference expansion, node budgets, issue ordering, cycle
tracking and report completion rules are unchanged.

Regression coverage includes the legacy extent oracle; Excel boundaries;
style-only/foreign-namespace cells; sparse gaps; empty/overlapping/reversed
ranges; case-insensitive resolution; workbook isolation; cancellation during
build, at publication, and during cached queries; multi-sheet/3D references;
cycles; default and maximum budgets; all 100 unresolved entries; and unchanged
OPC parts. Existing trace tests pass on both baseline and candidate.

## CLI wall time, including loading

Five paired samples per fixture ran the built baseline and candidate CLI,
including process startup, ZIP loading, XML loading and JSON output. Version
order alternated each sample. All **60 complete JSON reports** matched their
fixture's baseline (nodes, edges, cycles, unresolved entries, truncation,
completeness and method); the six source file SHA-256 hashes stayed unchanged.
[Raw timings and hashes](benchmarks/trace-extent/cli.json).

| Rows | K | Before median seconds | After median seconds | Speedup |
| ---: | ---: | ---: | ---: | ---: |
| 1,000 | 1 | 0.100480 | 0.104621 | 0.96× |
| 1,000 | 10 | 0.348817 | 0.109988 | 3.17× |
| 1,000 | 100 | 2.545986 | 0.102744 | 24.78× |
| 10,000 | 1 | 1.039252 | 1.067774 | 0.97× |
| 10,000 | 10 | 3.916880 | 1.028379 | 3.81× |
| 10,000 | 100 | 32.209200 | 0.992616 | 32.45× |

K=1 CLI wall time regressed 4.1% and 2.7%, still within the 10% bound. Unlike
the preconstructed-package benchmark, this includes ZIP loading and process
startup. The loading-inclusive measurements remain valuable even though they
show smaller K=100 speedups than the full-call benchmark.

## Index memory

Retained heap was measured with ten live tracers/indexes after GC, subtracting
an already-open workbook's post-GC heap, dividing by ten, and taking the median
of five independent runs. This includes each tracer and its worksheet-index
cache; small runtime/pool fluctuations make these approximate retained sizes.
Allocation totals include transient map growth and are distinct from retained
heap. [Heap samples](benchmarks/trace-extent/memory.txt),
[sparse benchmark samples](benchmarks/trace-extent/sparse.txt).

| Worksheet | Index entries (columns + rows) | Retained bytes/index | Allocated bytes/build |
| --- | ---: | ---: | ---: |
| Sparse A1 and XFD1048576 | 2 + 2 | 736 | 784 |
| Dense A1:CV1000 | 100 + 1,000 | 58,749 | 118,520 |
| Dense A1:CV10000 | 100 + 10,000 | 441,015 | 883,032 |

Sparse builds had median 430.3 ns/op and 7 allocs/op. Dense allocated sizes are
the isolated K=1 medians above. No arrays are allocated up to the largest saved
row/column: the two far-apart cells retain less than 1 KiB including the tracer.

## CPU profiling

Separate CPU profiles used the opened-workbook K=100, 1,000-row benchmark:
five timed iterations before, 100 after, plus the benchmark harness's calibration
iteration. Profiles include fixture construction, so percentages are diagnostic,
not another before/after latency comparison. The baseline sampled 11.56 CPU
seconds, with savedExtent accounting for 86.68% cumulatively and Child for
33.91%. Afterward, coordinate validation dominates (traceCoord 75.12%
cumulative); map assignment is 5.63%, and tracePopulated 5.63%. Repeated
ChildElements slice construction is no longer in the top 25 candidate entries.
This supports the measured drop from roughly 20 million allocations per
100-column lookup batch to 39. The existing coordinate-validation semantics
were deliberately retained.

[Baseline CPU top](benchmarks/trace-extent/cpu-before.txt) and
[candidate CPU top](benchmarks/trace-extent/cpu-after.txt) preserve the sampled
function breakdowns; binary profiles are reproducible with the commands below.

## Validation

- Baseline existing trace tests pass; candidate `go test ./internal/office
  -run '^TestTrace' -count=1` passes, including the new enumeration/oracle tests.
- `go vet ./internal/office ./internal/cli` passes.
- `git diff --check` passes.
- `bash scripts/test-local.sh` ran dependency verification, the full test suite,
  module-graph checks, and `go vet ./...`. All buildable tested packages passed,
  including office, CLI, conversion and PDF. Overall validation is blocked by
  existing `internal/pptx/deck_edit_test.go:399,425` calls to undefined
  `relativePart`; vet reports the same error. Those calls are present in baseline
  commit `1a3e7992` and no PPTX files were changed. This needs separate repair.
  [Validation output](benchmarks/trace-extent/validation.txt).

## Reproduction

Copy `internal/office/trace_extent_bench_test.go` into a checkout of the baseline
revision before building its test/CLI binaries. Run the same command on baseline
and candidate; the benchmark creates its OPC fixture outside the timed region:

```sh
go test ./internal/office -run '^$' \
  -bench 'Benchmark(TraceExtent|SavedExtent)$' -benchtime=1x -count=5
go test -c -o /tmp/office.test ./internal/office
/tmp/office.test -test.run '^$' \
  -test.bench '^BenchmarkSavedExtent/rows=1000$/K=100$' \
  -test.benchtime=5x -test.cpuprofile=/tmp/extent.pprof
go tool pprof -top /tmp/office.test /tmp/extent.pprof
```

Use 100x for the candidate profile to obtain enough samples. For candidate
retained-memory and sparse-allocation measurements:

```sh
PAPERWORKS_EXTENT_MEMORY=1 go test ./internal/office \
  -run '^TestTraceExtentIndexMemory$' -count=5 -v
go test ./internal/office -run '^$' -bench '^BenchmarkSavedExtentSparse$' \
  -benchtime=100ms -count=5
```

Build baseline and candidate CLIs with `go build -o <binary> ./cmd/paperworks`.
Export fixtures once into an existing empty directory, then run the paired
loading-inclusive comparison (five samples by default):

```sh
mkdir -p /tmp/trace-fixtures
PAPERWORKS_EXTENT_FIXTURES=/tmp/trace-fixtures go test ./internal/office \
  -run '^TestExportExtentBenchmark$' -count=1
python3 scripts/benchmarks/trace_extent.py \
  --before /tmp/paperworks-before --after /tmp/paperworks-after \
  --fixtures /tmp/trace-fixtures --output /tmp/trace-cli.json
```
