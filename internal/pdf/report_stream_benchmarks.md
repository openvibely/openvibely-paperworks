# Report stream allocation measurements

Measured on Darwin/arm64, Apple M5 Pro, Go 1.27.3. The small case repeats a short report text line 12 times; the large case repeats a report line 65,536 times; the poorly compressible case uses 4 MiB of deterministic xorshift bytes.

## Before and after

The timing column is the median of three `BenchmarkReportStream` runs with a one-second target per run. Allocation results come from the same benchmark with `-benchmem`; input setup is excluded from timing, and the reported bytes and allocation counts were stable.

| Case | ns/op before → after | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: |
| small | 18,835 → 20,380 | 557,208 → 556,920 | 42 → 39 |
| large | 4,535,289 → 4,585,738 | 3,991,986 → 3,965,360 | 54 → 52 |
| large, poorly compressible | 49,802,337 → 52,222,738 | 43,254,520 → 39,052,024 | 6,625 → 6,624 |

The large case increased by 1.1% and the large poorly compressible case by 4.9%, both within the 5% limit. The large poorly compressible case allocates 4,202,496 fewer bytes per operation.

## Isolated peak RSS

Measured the 4 MiB poorly compressible case in separate processes with `GOGC=off` and `/usr/bin/time -l`. Each process ran one encoding call; the three-run median was used to reduce process-start and OS noise.

| Encoder | Median maximum resident set size |
| --- | ---: |
| Previous contiguous encoder | 64,454,656 B |
| Segmented encoder | 60,194,816 B |
| Reduction | 4,259,840 B |

The compressed DEFLATE payload was 4,195,584 bytes. Peak RSS fell by 64,256 bytes more than that payload size.

## Validation

The ReportLab fixture hashes and encoded sizes remain pinned. Regression tests also compare the segmented output byte-for-byte with the former contiguous encoding for empty, small, large, and poorly compressible inputs, and cover standalone fixed paragraphs with fixed tables plus mixed-content `wrapContents` pages.
