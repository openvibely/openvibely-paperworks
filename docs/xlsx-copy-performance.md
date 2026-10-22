# XLSX copy performance

`BenchmarkEditFileCopyLargeWorkbook` exercises `EditFile` on two sheets with
50,000 rows and four cells per row. Each copy moves `A1:B2` to a distinct target
on Sheet1. The benchmark includes opening and saving the workbook; fixture
creation and removal of the previous output are outside the timer.

Measured on darwin/arm64, Apple M5 Pro, Go 1.26.4:

| Operation | Edits | Before (seconds) | After (seconds) |
| --- | ---: | ---: | ---: |
| value | 1 | 0.462 | 0.559 |
| value | 10 | 0.462 | 0.542 |
| copy | 1 | 1.304 | 0.470 |
| copy | 10 | 8.878 | 0.543 |
| copy-values | 1 | 1.311 | 0.511 |
| copy-values | 10 | 8.856 | 0.515 |
| copy-formulas | 1 | — | 0.514 |
| copy-formulas | 10 | — | 0.524 |
| copy-formats | 1 | 0.474 | 0.509 |
| copy-formats | 10 | 0.461 | 0.509 |

The baseline used `-benchtime=1x`; the final measurement used Go's default
benchmark duration (2–3 iterations per case). An earlier optimized run also
measured ten copies and ten value copies at 0.517 and 0.518 seconds against a
0.508-second value baseline. Times vary with system load; the single-copy
results show no measurable added cost over the value baseline in these runs.
Both ten-copy cases meet the 2× baseline limit and the under-one-second target.

Reproduce with:

```sh
go test -run x -bench BenchmarkEditFileCopyLargeWorkbook ./internal/xlsx
```

Array discovery now scans the requested worksheet's live cells instead of
serializing a ZIP and parsing every worksheet. Date/error corrections update
live destination cells immediately, so subsequent copies, structural edits and
sheet renames see their actual types. There is no per-copy workbook reopen or
array cache to invalidate. Array discovery remains linear in the cells of the
requested sheet, rather than in the serialized workbook size.

Excelize v2.11.0 exposes `File.Sheet` but leaves its worksheet type unexported.
`worksheet_memory.go` isolates checked reflection over exported row/cell fields;
it uses no unsafe access and requires exclusive ownership of the file, as
`EditFile` already provides. Excelize upgrades must retain the adapter tests.
The regression test traps ZIP serialization and covers raw date/error copies,
partial array overlaps after a copy, clearing an array, nonstandard worksheet
part paths, and newly created worksheet relationships. Existing tests cover
merged anchors and final saved XML, including types after a rename.
