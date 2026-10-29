# Office preview performance

`internal/office/preview_performance_bench_test.go` generates XLSX and PPTX archives with 0, 1, 10, or 20 chart parts and 128 unrelated package parts. The chart fixture has eight series with 16 cached categories and values per series. Benchmarks report allocations and cover both `NeedsOfficePreview` and `OfficePreviewCopy`.

Run the benchmark suite with:

```sh
go test ./internal/office -run '^$' -bench 'Benchmark(NeedsOfficePreview|OfficePreviewCopy)$' -benchmem -benchtime=100ms -count=3
```

## Current measurements

Repeated run on darwin/arm64, Apple M5 Pro, Go benchmark output summarized as median of three runs. Time is ms/op; bytes are decimal MB/op.

| API | Package | 0 charts | 1 chart | 10 charts | 20 charts |
| --- | --- | ---: | ---: | ---: | ---: |
| `NeedsOfficePreview` | XLSX | 0.231 / 0.265 MB | 0.535 / 0.534 MB | 3.088 / 2.994 MB | 5.801 / 5.657 MB |
| `NeedsOfficePreview` | PPTX | 0.226 / 0.269 MB | 0.536 / 0.534 MB | 3.454 / 2.994 MB | 5.873 / 5.655 MB |
| `OfficePreviewCopy` | XLSX | 0.224 / 0.267 MB | 1.015 / 1.225 MB | 7.761 / 4.892 MB | 10.660 / 8.108 MB |
| `OfficePreviewCopy` | PPTX | 0.379 / 0.269 MB | 1.265 / 1.180 MB | 6.207 / 4.679 MB | 11.733 / 8.094 MB |

Each cell lists `time / allocated bytes`. Chart-free `OfficePreviewCopy` timing varied between repetitions; paired five-run comparison below is more useful for regression assessment.

## Before and after

The original implementation at `834937d` and the updated implementation were benchmarked on the same machine, with five 150 ms runs per case. The table shows median time and allocated bytes per operation. The chart-free comparison used five 200 ms runs.

| Package and path | Before | After | Median time change | Allocation change |
| --- | ---: | ---: | ---: | ---: |
| XLSX, 20-chart fallback copy | 20.351 ms / 18.134 MB | 8.901 ms / 8.107 MB | -56.3% | -55.3% |
| PPTX, 20-chart fallback copy | 23.485 ms / 22.524 MB | 9.894 ms / 8.039 MB | -57.9% | -64.3% |
| XLSX, no-fallback copy | 0.273 ms / 0.266 MB | 0.220 ms / 0.266 MB | -19.6% | +0.2% |
| PPTX, no-fallback copy | 0.314 ms / 0.268 MB | 0.251 ms / 0.269 MB | -20.1% | +0.3% |
| XLSX, 20-chart `NeedsOfficePreview` | 5.959 ms / 5.639 MB | 5.756 ms / 5.653 MB | -3.4% | +0.2% |
| PPTX, 20-chart `NeedsOfficePreview` | 6.027 ms / 5.637 MB | 6.010 ms / 5.657 MB | -0.3% | +0.4% |

The 20-chart fallback-copy path exceeds the 25% time and allocation reduction target for both formats. Standalone `NeedsOfficePreview` must still validate every chart part, so its 20-chart benchmark is near the original cost; allocation changes are below 0.5%. No-fallback copy allocations stay within 0.4% of baseline.

The generated archive benchmark isolates package parsing and chart work but does not model LibreOffice rendering or large embedded media. Results are local measurements and can vary with machine load.
