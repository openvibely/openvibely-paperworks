# Image reuse benchmark

Benchmark command: `go test ./internal/docx -run '^$' -bench BenchmarkBuildImageReuse -benchtime=1x -count=3`

Machine: Apple MacBook Pro with Apple M5 Pro, Darwin 25.5.0, arm64. Go: `go1.27.3 darwin/arm64`.

Each build authors 100 image blocks. The repeat case uses one byte-identical 1 MiB JPEG at every placement. The control uses 100 byte-distinct 1 MiB JPEG files. The baseline was recorded with the original `internal/docx/create.go`; the optimized results use the content reuse implementation. Both use the same benchmark fixture and command. Times below are medians of three one-build samples.

| Case | Build time, baseline | Build time, optimized | Change | Media payload, baseline → optimized | DOCX size, baseline → optimized |
| --- | ---: | ---: | ---: | ---: | ---: |
| 100 repeated images | 1.305 s | 35.7 ms | 97.3% faster | 100 MiB → 1 MiB (99.0% smaller) | 104,910,802 B → 1,075,785 B |
| 100 distinct images | 1.560 s | 1.270 s | 18.6% faster | 100 MiB → 100 MiB | 104,911,993 B → 104,911,993 B |

Raw `ns/op` samples (baseline / optimized):

- Repeated: 1,241,595,541 / 35,737,708; 1,529,562,167 / 35,715,166; 1,304,884,166 / 34,985,542.
- Distinct: 1,573,093,542 / 1,301,495,000; 1,560,102,125 / 1,259,630,834; 1,513,406,792 / 1,270,051,583.

## Per-build path cache

Command: `go test ./internal/docx -run '^$' -bench '^BenchmarkBuildImageReuse$' -benchtime=1x -count=3`.

On the same Apple M5 Pro / Go 1.27.3 environment, these are medians from three one-build samples comparing the pre-cache content-dedup implementation at HEAD with the normalized-path cache:

| Case | Pre-cache median | Path-cache median | Time change | Pre-cache → path-cache B/op |
| --- | ---: | ---: | ---: | ---: |
| 100 repeated images | 44.46 ms | 20.14 ms | 54.7% faster | 130,333,600 → 25,586,720 (104,746,880 B / 99.9 MiB lower) |
| 100 distinct images | 1.409 s | 1.404 s | 0.4% faster | 1,281,728,976 → 1,281,747,584 |

The historical 1.270 s optimized distinct-image median above is not comparable to the latest host performance: the pre-cache control itself measured 1.409 s in one required 1x run and 1.474 s in a later paired 1x run (16.1% above the historical result in the latter). In that later 1x pair, the path-cache median was 1.547 s (4.96% slower, within the 5% limit). A longer paired run (`-benchtime=2x -count=3`) gave pre-cache 1.529 s and path-cache 1.556 s (1.8% slower). These control runs show that the historical absolute gap is present before the cache change; the paired comparisons remain within the acceptance limit.

Paired 2x samples (pre-cache / path-cache, ns/op):

- Repeated: 54,347,334 / 22,335,646; 68,987,000 / 22,460,708; 60,787,375 / 23,508,917.
- Distinct: 1,528,689,230 / 1,543,498,354; 1,533,379,916 / 1,555,904,875; 1,472,243,292 / 1,576,044,104.
- Distinct B/op medians: 1,281,734,912 / 1,281,735,568.
