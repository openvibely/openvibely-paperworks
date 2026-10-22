# LibreOffice refresh benchmark

Measured on macOS arm64 with LibreOffice 25.8.7.3 (`30742500f2d3eb4366ac312fa33d3dcabdb3eba5`). The before source is commit `ce7558f`; after measurements use the updated source and the same LibreOffice executable and workbook inputs.

Each cohort contains 30 end-to-end `lo.Refresh` operations writing distinct outputs. Cold samples had no benchmark warm-up; warm samples followed three unmeasured refreshes. The operating-system cache was not flushed. The small input is `testdata/fixtures/reference-pie.xlsx` (5,274 bytes), a chart workbook with no worksheet formulas. The updated path rebuilds its supported standard chart caches from saved cell values and launches no LibreOffice process. The large input is a generated 50,000-row workbook containing 50,000 formulas (1,081,190 bytes). The baseline starts LibreOffice twice per refresh; the updated large-workbook path starts it once. Launches were counted at the process runner boundary.

| Workbook | Sample | Baseline median | Baseline p95 | Baseline launches/op | Updated median | Updated p95 | Updated launches/op | Median reduction |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Small chart workbook | Cold | 2.032 s | 2.188 s | 2 | 0.411 ms | 0.915 ms | 0 | 99.98% |
| Small chart workbook | Warm | 1.805 s | 1.863 s | 2 | 0.415 ms | 0.621 ms | 0 | 99.98% |
| Large formula workbook | Cold | 1.988 s | 2.043 s | 2 | 1.693 s | 1.704 s | 1 | 14.8% |
| Large formula workbook | Warm | 1.970 s | 2.033 s | 2 | 1.719 s | 1.861 s | 1 | 12.7% |

The small-workbook median reduction exceeds the 20% target in both cohorts. Large-workbook p95 improved by 16.6% cold and 8.5% warm, within the 5% regression limit. Formula-bearing workbooks, unsupported chart caches, tables, external links, pivots, and ambiguous packages fall back to the isolated LibreOffice path.

The gated harness is `refresh_benchmark_test.go`; run each cohort with `PAPERWORKS_REFRESH_BENCHMARK_GROUP=small-cold`, `small-warm`, `large-cold`, or `large-warm` and `go test ./internal/lo -run '^TestRefreshBenchmarkSample$' -count=1 -v`.
