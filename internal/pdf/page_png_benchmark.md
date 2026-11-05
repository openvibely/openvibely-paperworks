# Page PNG streaming benchmark

Measured the original encoder at `d7ea289` and the streaming encoder on the same Apple M5 Pro (`darwin/arm64`, Darwin 27.0.0, Go 1.27.3). Each timing and peak RSS value is the median of five independent test-binary processes. RSS comes from `/usr/bin/time -l` and is shown in MiB. The input `image.RGBA` is created before the benchmark timer starts; its allocation is included in process RSS.

| Pixels and pattern | Original time / RSS | Streaming time / RSS | RSS reduction | Time change |
| --- | ---: | ---: | ---: | ---: |
| 5 MP, high entropy | 0.573 s / 121.7 MiB | 0.587 s / 52.7 MiB | 69.0 MiB | +2.39% |
| 5 MP, repeated background | 0.066 s / 53.2 MiB | 0.061 s / 38.7 MiB | 14.5 MiB | -6.66% |
| 20 MP, high entropy | 2.403 s / 420.2 MiB | 2.432 s / 152.4 MiB | 267.8 MiB | +1.21% |
| 20 MP, repeated background | 0.254 s / 154.0 MiB | 0.249 s / 96.1 MiB | 57.9 MiB | -2.16% |
| 40 MP, high entropy | 4.727 s / 886.0 MiB | 4.721 s / 285.2 MiB | 600.8 MiB | -0.14% |
| 40 MP, repeated background | 0.516 s / 288.1 MiB | 0.496 s / 172.8 MiB | 115.3 MiB | -3.96% |

The benchmark is `BenchmarkPagePNG` in `page_png_benchmark_test.go`. Build a test binary, then run a single case in a fresh process with:

```sh
go test -c -o /tmp/pagepng.test ./internal/pdf
/usr/bin/time -l /tmp/pagepng.test -test.run='^$' -test.bench='^BenchmarkPagePNG/40MP/high-entropy$' -test.benchtime=1x
```

Repeat for each size and pattern five times to reproduce the medians. For a baseline comparison, build the benchmark test against the pre-change revision with the same Go toolchain and machine.
