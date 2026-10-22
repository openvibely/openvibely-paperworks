# Emoji rendering performance

Measured on darwin/arm64, Apple M5 Pro, using the installed Apple Color Emoji TTC. The one-emoji expression is `\unicode{x1F600}`; the four-emoji expression uses U+1F600, U+1F601, U+1F602, and U+1F603. Plain input is `x^2+\frac{a}{b}`. MB below means decimal megabytes.

| PNG benchmark | Before ms/op | After ms/op | Before MB/op | After MB/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Plain | 5.646 | 5.518 | 4.651 | 4.497 | 96,105 | 95,972 |
| One emoji | 135.262 | 9.870 | 236.005 | 5.255 | 9,736,970 | 21,767 |
| Four emojis | 475.766 | 40.553 | 364.139 | 17.472 | 38,935,281 | 74,412 |

Before measurements used `go test -run '^$' -bench BenchmarkEmojiPNG -benchmem ./internal/equation/`; final after measurements used `go test -bench BenchmarkEmojiPNG -benchmem ./internal/equation/`. The plain baseline on this run was slower than the review's 3.6 ms, but remained unchanged by the optimization. Timings vary with machine load.

CLI measurements used `/usr/bin/time -l` with separately built before/after executables and fresh output paths. Repeated runs after initial executable startup:

| CLI input | Before elapsed | After elapsed | Before max RSS | After max RSS |
| --- | ---: | ---: | ---: | ---: |
| One emoji | 0.15 s | 0.03 s | 271.6 MB | 41.5 MB |
| Four emojis | 0.47 s | 0.06 s | 406.8 MB | 49.0 MB |

Initial executable runs measured 0.64/0.53 s before/after for one emoji and 0.51/0.08 s for four, with max RSS 272.2/43.6 MB and 406.9/48.9 MB respectively. Both initial and repeated RSS measurements satisfy the targets.

The reader opens the font lazily and closes it when equation rendering returns. SFNT uses a bounded section reader; sbix decoding reads the selected table metadata and glyph data through checked ReaderAt ranges. The face and largest strike are retained for the duration of a render. No cross-render cache is needed.

Sampling extracts alpha once, directly from NRGBA/RGBA pixel storage when possible. The fallback retains 16-bit alpha precision. Quantized sampling no longer computes and discards a bilinear sample.

The before/after CLI PNG files compare byte-for-byte for both emoji inputs. `TestPinnedEmojiEndToEnd` passes without skipping: 90×125 pixels and SHA-256 `44184e70c7605d8c461ecb2c8c82462dc01dae9b19869c1a8fe79dcd28911d93`. Tests also cover malformed ranges, truncated readers, duplicate glyph cycles, subimage strides, fallback alpha precision, missing fonts, substituted font profile rejection, and cancellation.

Validation completed: `gofmt`, `go vet ./...`, `go test ./...`, `go test -bench BenchmarkEmojiPNG -benchmem ./internal/equation/`, CLI timing, and byte comparisons. The full test suite passed with a ten-minute command allowance (conversion tests took 305 seconds); the initial two-minute command allowance was insufficient.
