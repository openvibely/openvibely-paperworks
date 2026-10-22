# PDF Split benchmarks

Measured on macOS 26.5 (Apple M5 Pro, arm64, 48 GiB RAM) with Go 1.27.3. Fixtures are generated in the benchmark from fixed PDF object graphs. The legacy case reproduces the prior implementation: read the source once, call `SelectPages` for each group, then write each result. Each reported wall time is the median of five one-operation benchmark samples (`-benchtime=1x -count=5`). Directory setup and output cleanup are outside the timed section.

| Fixture | Pages | Bytes | SHA-256 |
| --- | ---: | ---: | --- |
| Text | 50 | 13,658 | `c981c9e63324db4c1d5f9f48dae3792727ad8e7d5c3a0f1771615f976de9f297` |
| Text | 100 | 27,159 | `12f4b7e9562e397cc98d1ed05e20568e34a0ad4a3f5a71746bfda3f54c355d50` |
| Text | 200 | 54,159 | `82a5c069e56224db29733005a439ea0a1d60fc4951e6b1e2bf7df4a8a3c0ec6e` |
| Image | 100 | 27,638 | `6a1c7c861076ed5304e3b223550620564177450c9fee441f2287b42eb58245e3` |
| Image | 200 | 54,338 | `ef33fb2e08f85bbbdfb9ef35ddc082a665b3526515d08e8c5271ce2f538ab6f9` |

Median wall time in milliseconds; a negative change is an improvement.

| Fixture | One output group: before | One output group: after | Change | One page per output: before | One page per output: after | Change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Text, 50 pages | 1.431 | 1.283 | -10.4% | 37.826 | 4.816 | -87.3% |
| Text, 100 pages | 2.562 | 2.587 | +0.9% | 130.830 | 10.410 | -92.0% |
| Text, 200 pages | 5.324 | 5.439 | +2.2% | 499.645 | 20.584 | -95.9% |
| Image, 100 pages | 2.562 | 2.616 | +2.1% | 135.180 | 10.316 | -92.4% |
| Image, 200 pages | 5.411 | 5.508 | +1.8% | 510.467 | 20.547 | -96.0% |

The 200-page one-page-per-output cases exceed the 50% wall-time reduction target. The largest measured single-group regression was 2.2%, below the 10% limit.

For peak RSS, the test binary was built once and each case ran in its own process under `/usr/bin/time -l`. The median of five maximum-resident-set samples was 31,621,120 bytes before and 29,278,208 bytes after for 200-page text (-7.4%); image was 31,621,120 bytes before and 28,409,856 bytes after (-10.2%). These measurements include the Go test harness and fixture, so they are process-level comparisons. Both measured multi-output cases reduced peak RSS.

Commands:

```sh
go test ./internal/pdf -run '^$' -bench '^BenchmarkSplit$' -benchtime=1x -count=5 -benchmem -v
go test -c -o /tmp/openvibely-internal-pdf.test ./internal/pdf
/usr/bin/time -l /tmp/openvibely-internal-pdf.test -test.run '^$' -test.bench '^BenchmarkSplit/text_200/one_page_per_output/split$' -test.benchtime=1x -test.count=1
```
