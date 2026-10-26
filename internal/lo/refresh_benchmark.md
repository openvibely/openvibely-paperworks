# LibreOffice refresh benchmark

Measured on macOS arm64 with LibreOffice 25.8.7.3 (`30742500f2d3eb4366ac312fa33d3dcabdb3eba5`). The baseline source is commit `ce7558f`; final after measurements were run against staged-output implementation commit `d2f3f5eeb7a7b71b228b4eea95cd65e16cf45bbe`, using the same LibreOffice executable and workbook inputs.

Each cohort contains 30 end-to-end `lo.Refresh` operations writing distinct outputs. Cold samples had no benchmark warm-up; warm samples followed three unmeasured refreshes. The operating-system cache was not flushed. The small input is `testdata/fixtures/reference-pie.xlsx` (5,274 bytes), a chart workbook with no worksheet formulas. The updated path rebuilds its supported standard chart caches from saved cell values and launches no LibreOffice process; this local path is unaffected by the staged-output change. The large input is a generated 50,000-row workbook containing 50,000 formulas (1,081,190 bytes). Its updated cold and warm cohorts were rerun against the staged-output implementation, including the final copy to the requested destination. The baseline starts LibreOffice twice per refresh; the updated large-workbook path starts it once. Launches were counted at the process runner boundary.

| Workbook | Sample | Baseline median | Baseline p95 | Baseline launches/op | Updated median | Updated p95 | Updated launches/op | Median reduction |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Small chart workbook | Cold | 2.032 s | 2.188 s | 2 | 0.713 ms | 1.453 ms | 0 | 99.96% |
| Small chart workbook | Warm | 1.805 s | 1.863 s | 2 | 0.707 ms | 1.137 ms | 0 | 99.96% |
| Large formula workbook | Cold | 1.988 s | 2.043 s | 2 | 1.777 s | 1.986 s | 1 | 10.6% |
| Large formula workbook | Warm | 1.970 s | 2.033 s | 2 | 1.812 s | 1.932 s | 1 | 8.0% |

The small-workbook median reduction exceeds the 20% target in both cohorts. With the staged-output implementation, large-workbook p95 improved by 2.79% cold and 4.98% warm, within the 5% regression limit. Formula-bearing workbooks, unsupported chart caches, tables, external links, pivots, and ambiguous packages fall back to the isolated LibreOffice path.

The gated harness is `refresh_benchmark_test.go`; run each cohort with `PAPERWORKS_REFRESH_BENCHMARK_GROUP=small-cold`, `small-warm`, `large-cold`, or `large-warm` and `go test ./internal/lo -run '^TestRefreshBenchmarkSample$' -count=1 -v`.

## Large chartless package memory benchmark

Measured on the same macOS arm64 host (Darwin 27.0.0, Go 1.27.3) in isolated child processes. Each cohort performed 30 no-formula/no-chart `Refresh` operations and wrote distinct output files. The generated input was 193,107 compressed bytes with 188,761,375 expanded bytes (180.02 MiB) across three 60 MiB members. LibreOffice was deliberately unavailable to each child, and the test checked that the final output was byte-identical to the source.

The before cohort reproduces the former chartless path (`opc.Open`, `office.ChartParts`, then source copy); after uses `Refresh` with the metadata-only chart probe and CRC streaming to discard. Peak RSS is the child process maximum resident set size.

| Measurement | Before | After | Change |
| --- | ---: | ---: | ---: |
| Median latency | 56.007 ms | 45.232 ms | 19.2% faster |
| p95 latency | 77.033 ms | 80.015 ms | 3.9% slower |
| Allocated bytes/op | 484,035,424 B | 1,178,369 B | 99.76% fewer |
| Peak RSS | 530,776,064 B (506.19 MiB) | 19,791,872 B (18.88 MiB) | 487.31 MiB lower |

Run this gated acceptance benchmark with `PAPERWORKS_REFRESH_LARGE_BENCHMARK=1 go test ./internal/lo -run '^TestRefreshBenchmarkLargeExpandedPlainXLSX$' -count=1 -v`. It compares both implementations on the same machine and checks the allocation, RSS, and median-latency thresholds.
