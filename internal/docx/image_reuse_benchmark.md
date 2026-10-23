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
