# LibreOffice refresh benchmark

Measured on macOS arm64 with LibreOffice 25.8.7.3 (`30742500f2d3eb4366ac312fa33d3dcabdb3eba5`). Baseline source was commit `ce7558f`; the updated source uses the same LibreOffice executable and input files.

Each sample contains 30 end-to-end `lo.Refresh` operations. Every operation uses a new temporary profile and writes a distinct output. Cold samples had no benchmark warm-up; warm samples followed three unmeasured refreshes. “Cold” describes the fresh process and profile; the operating-system file cache was not flushed. The small input is `testdata/fixtures/reference-pie.xlsx` (5,274 bytes). The large input is a generated 50,000-row workbook with 50,000 formulas (1,081,190 bytes). Launch counts were observed at the process runner boundary.

| Workbook | Sample | Baseline median | Baseline p95 | Baseline launches/op | Updated median | Updated p95 | Updated launches/op | Median change |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Small | Cold | 2.032 s | 2.188 s | 2 | 1.742 s | 2.014 s | 1 | -14.3% |
| Small | Warm | 1.805 s | 1.863 s | 2 | 1.649 s | 1.731 s | 1 | -8.7% |
| Large | Cold | 1.988 s | 2.043 s | 2 | 1.915 s | 1.964 s | 1 | -3.7% |
| Large | Warm | 1.970 s | 2.033 s | 2 | 1.749 s | 1.786 s | 1 | -11.2% |

The update meets the one-launch requirement and the large-workbook p95 limit. Small-workbook median improvement was below the 20% target in both samples. The gated harness is `refresh_benchmark_test.go`; run each cohort with `PAPERWORKS_REFRESH_BENCHMARK_GROUP=small-cold`, `small-warm`, `large-cold`, or `large-warm` and `go test ./internal/lo -run '^TestRefreshBenchmarkSample$' -count=1 -v`.
