# Sparkline column lookup benchmark

The end-to-end benchmark `BenchmarkOfficePreviewCopySparklineColumnRanges` creates an XLSX fixture with 10,000 numeric sparkline points and 1, 32, or 128 disjoint `<col>` ranges. Fixture generation and output cleanup are outside the timed section; each timed operation runs `OfficePreviewCopy` and writes the preview workbook.

Measurements were collected on the same Apple M5 Pro (`darwin/arm64`) with Go 1.27.3. The baseline used the original `preview.go` implementation; the indexed run used the changed implementation. The fixture and benchmark code were identical for both runs. Each sample measured three preview copies, and each case was run 10 times:

```sh
go test ./internal/office -run '^$' -bench '^BenchmarkOfficePreviewCopySparklineColumnRanges$' -benchmem -benchtime=3x -count=10
```

The table reports the median of the 10 `ns/op`, `B/op`, and `allocs/op` results for each case.

| Column ranges | Baseline time | Indexed time | Time change | Baseline B/op | Indexed B/op | Baseline allocs/op | Indexed allocs/op |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 29.47 ms | 24.47 ms | 17.0% faster | 29,168,838 | 27,809,476 | 593,883 | 533,890 |
| 32 | 42.84 ms | 25.24 ms | 41.1% faster | 39,165,217 | 27,884,824 | 695,013 | 535,016 |
| 128 | 82.46 ms | 24.67 ms | 70.1% faster | 72,649,245 | 28,098,147 | 738,382 | 538,384 |

At 128 ranges, median time is 70.1% lower and both allocation bytes and allocation count are lower. At one range, time is 17.0% lower, within the 5% regression limit.

`newGeometry` reads the sheet's `<col>` definitions once, then builds effective nonoverlapping intervals in document order. An event sweep resolves overlaps so the earliest matching definition retains precedence. Index construction is `O(R log R)` for `R` definitions; each subsequent width or hidden lookup uses binary search in `O(log R)`. For `P` sparkline points, this changes the lookup work from repeated XML traversal at `O(P × R)` to one per-sheet index build plus `O(P log R)` searches. The 10,000-point benchmark shows the indexed timings staying close across 1, 32, and 128 ranges.
