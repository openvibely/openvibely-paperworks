# SaveNew benchmark

Benchmarks: `BenchmarkSaveNew200MiBMedia` and `BenchmarkSaveNewManyParts` in `save_bench_test.go`. Both create a multi-part DOCX-style package, edit only `word/document.xml`, and keep all media unchanged. The large case has 20 incompressible media parts totaling 200 MiB. The many-part case has 10,000 unchanged 256-byte media parts.

Measurements were taken on Darwin arm64, Apple M5 Pro, with Go 1.27.3. Baseline used a test binary compiled before the `SaveNew` change; the updated binary used the same fixtures and a three-iteration run (`-benchtime=3x -benchmem`). Each fixture was measured in a separate process.

| Fixture | Baseline ns/op | Updated ns/op | Time change | Baseline B/op | Updated B/op | Allocation change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 200 MiB unchanged media | 187,377,861 | 101,088,361 | 46.1% faster | 1,341,403,613 | 661,871,994 | 648.1 MiB fewer (50.7%) |
| 10,000 media parts | 8,701,722 | 4,491,889 | 48.4% faster | 32,615,224 | 17,685,637 | 14.2 MiB fewer (45.8%) |

The large fixture exceeds the targets of 25% lower save time and 200 MiB fewer allocated bytes. Peak RSS was measurable with macOS `/usr/bin/time -l`: 2,916,728,832 bytes before and 2,745,843,712 bytes after, a 163.0 MiB reduction. This is process peak RSS across three iterations; Go retains freed heap pages, so the RSS change is smaller than the per-operation allocation reduction.

Reproduction commands (run each benchmark in a separate process):

```sh
go test ./internal/opc -run '^$' -bench '^BenchmarkSaveNew200MiBMedia$' -benchmem -benchtime=3x
go test ./internal/opc -run '^$' -bench '^BenchmarkSaveNewManyParts$' -benchmem -benchtime=3x
/usr/bin/time -l /tmp/opc-save-benchmark-baseline.test -test.run '^$' -test.bench '^BenchmarkSaveNew200MiBMedia$' -test.benchtime=3x -test.benchmem
/usr/bin/time -l /tmp/opc-save-benchmark-current.test -test.run '^$' -test.bench '^BenchmarkSaveNew200MiBMedia$' -test.benchtime=3x -test.benchmem
/tmp/opc-save-benchmark-baseline.test -test.run '^$' -test.bench '^BenchmarkSaveNewManyParts$' -test.benchtime=3x -test.benchmem
/tmp/opc-save-benchmark-current.test -test.run '^$' -test.bench '^BenchmarkSaveNewManyParts$' -test.benchtime=3x -test.benchmem
```

The baseline and current test binaries are temporary and are not included in the repository.
