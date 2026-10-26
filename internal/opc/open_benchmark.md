# `Open` allocation and RSS benchmark

Measured on macOS arm64 with an Apple M5 Pro. The baseline binary was built before the ownership-path change; the final binary includes it. Both binaries ran the same fixtures with the same Go toolchain and machine.

The timing/allocation results are medians of three `-benchtime=2s` runs. Peak RSS was measured in separate processes with `/usr/bin/time -l`, one benchmark operation per process. The small file RSS values include process and Go runtime overhead, so their changes are below the useful resolution of this measurement.

| Fixture | Input bytes | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op | Before peak RSS | After peak RSS |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| DOCX `call-0344.docx` | 42,477 | 1,124,040 | 1,076,216 | 1,932,919 | 1,884,873 | 536 | 535 | 10,633,216 B | 10,649,600 B |
| XLSX `reference-pie.xlsx` | 5,274 | 111,765 | 100,620 | 53,399 | 47,939 | 223 | 222 | 8,781,824 B | 8,912,896 B |
| PPTX `reference-waterfall.pptx` | 82,469 | 201,931 | 113,362 | 404,750 | 314,612 | 606 | 605 | 8,896,512 B | 8,847,360 B |
| Large-media DOCX (32 MiB stored media) | 33,556,789 | 11,907,234 | 10,628,411 | 142,425,783 | 108,867,920 | 699 | 698 | 143,376,384 B | 99,893,248 B |

The large-media case allocates 33,557,863 fewer bytes per operation, a reduction of 1,074 bytes beyond the archive size due to run-to-run allocation accounting. Peak RSS fell by 43,483,136 bytes (about 1.30 times the input archive size). Its elapsed time improved by about 10.7%. The representative DOCX, XLSX, and PPTX medians also improved; no fixture showed an elapsed-time regression in this run.

The benchmark is `BenchmarkOpen` in `open_bench_test.go`. It opens the repository's DOCX, XLSX, and PPTX fixtures plus a generated DOCX-shaped archive containing 32 MiB of deterministic incompressible stored media. Fixture creation occurs before the benchmark timer starts.
