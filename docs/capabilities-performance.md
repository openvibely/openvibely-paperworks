# Capability command performance

The capability acceptance threshold is a median of at most 50 ms and p95 of at most 100 ms for 30 cold and 30 warm measurements on the available same-machine environment. A default report must not launch LibreOffice; an exact-version report served from a valid cache must not launch it again.

## Reproduce

From the repository root, build the pre-change command from the implementation base (`d7ea289`) and the current command, then run the benchmark with an installed LibreOffice executable:

```sh
mkdir -p /tmp/paperworks-capabilities-baseline
git archive d7ea289 | tar -x -C /tmp/paperworks-capabilities-baseline
(cd /tmp/paperworks-capabilities-baseline && go build -o /tmp/paperworks-capabilities-before ./cmd/paperworks)
go build -o /tmp/paperworks-capabilities-after ./cmd/paperworks
python3 scripts/benchmark_capabilities.py \
  --before /tmp/paperworks-capabilities-before \
  --after /tmp/paperworks-capabilities-after \
  --soffice /path/to/soffice \
  --runs 30
```

The benchmark starts a fresh CLI process for every sample and reports the median and nearest-rank p95. The first 30-process cohort is labeled cold and the next 30 warm; the operating system page cache is not flushed, so cold describes fresh CLI processes, not cold disk state. The comparison uses a temporary `DOCUMENT_SOFFICE` wrapper that delegates to the same installed `soffice` and records each launch. A separate measurement uses the current machine's normal cached LibreOffice discovery path. The script also primes an exact-version cache, verifies the reported version equals direct `soffice --version` output, then checks 30 cache-served reports for additional launches.

## Measurement

Measured on macOS 27.0.1, arm64, Go 1.27.3, with LibreOffice 26.8.1.1 (`bf6ef2662401c7d6dce671ab4deb26ee0af3a7c1`). The executable was the cached installation at:

`/Users/dubee/Library/Caches/openvibely/skill-tools/libreoffice/26.8.1/darwin/arm64/54b421b15b30d3fb549038109e00ab6527e0e88fc50d0e33731a9a0166c2db18/LibreOffice.app/Contents/MacOS/soffice`

Command used:

```sh
python3 scripts/benchmark_capabilities.py \
  --before /tmp/paperworks-capabilities-before \
  --after /tmp/paperworks-capabilities-after \
  --soffice /Users/dubee/Library/Caches/openvibely/skill-tools/libreoffice/26.8.1/darwin/arm64/54b421b15b30d3fb549038109e00ab6527e0e88fc50d0e33731a9a0166c2db18/LibreOffice.app/Contents/MacOS/soffice \
  --runs 30
```

Results (milliseconds):

| Path | Cohort | Median | p95 | Samples | LibreOffice launches |
| --- | --- | ---: | ---: | ---: | ---: |
| Before, custom `DOCUMENT_SOFFICE`, exact version included by default | Cold | 122.484 | 128.046 | 30 | 30 |
| Before, custom `DOCUMENT_SOFFICE`, exact version included by default | Warm | 121.983 | 125.929 | 30 | 30 |
| After, custom `DOCUMENT_SOFFICE`, version skipped | Cold | 8.048 | 13.181 | 30 | 0 |
| After, custom `DOCUMENT_SOFFICE`, version skipped | Warm | 7.962 | 8.344 | 30 | 0 |
| After, cached-install discovery, version skipped | Cold | 8.088 | 8.525 | 30 | 0 |
| After, cached-install discovery, version skipped | Warm | 8.093 | 8.387 | 30 | 0 |
| After, exact version served from valid cache | Warm | 8.151 | 8.582 | 30 | 0 additional (1 priming launch) |

The current fast-path cold and warm measurements meet both latency thresholds. The default custom-executable path made zero launches in all 60 measured calls. Exact-version output matched direct `soffice --version`; after the one intentional cache-priming probe, the 30 measured cache hits made zero additional launches.
