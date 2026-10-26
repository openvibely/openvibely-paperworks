# pdf-select one-pass benchmark

- Go: `go1.27.3`
- OS/architecture: `darwin/arm64`
- CPU: `Apple M5 Pro`
- Revision: `df04cca7ee63680998b82bc138503a08f568720b + working-tree source patch sha256:7b06ed3e8027ce168ac82ec2c3b229aeb69f0ead6415a4dc1bfe35c729887432`
- Method: one warm-up followed by five runs in fresh processes, each writing a new output file. Median wall time, heap allocations, allocated bytes, and per-process peak RSS are reported. Preparation pass counts are instrumented at the pdfcpu preparation entry points.

| Fixture | Source size | SHA-256 | Prep passes old/new | Median wall ms old/new | Wall reduction | Median allocs old/new | Median allocated MiB old/new | Median peak RSS MiB old/new |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| text, 50 pages | 13658 bytes | `c981c9e63324db4c1d5f9f48dae3792727ad8e7d5c3a0f1771615f976de9f297` | 2 / 1 | 2.859 / 1.570 | 45.1% | 13800 / 8025 | 3.65 / 2.40 | 22.08 / 20.16 |
| text, 200 pages | 54159 bytes | `82a5c069e56224db29733005a439ea0a1d60fc4951e6b1e2bf7df4a8a3c0ec6e` | 2 / 1 | 8.309 / 4.784 | 42.4% | 50848 / 28542 | 11.04 / 6.14 | 24.20 / 23.81 |
| text, 500 pages | 135170 bytes | `38bd3c9788a8264c49632998748002120aa7af1892f6db4ad6f9b9ac56db2691` | 2 / 1 | 18.732 / 9.381 | 49.9% | 124849 / 69521 | 25.86 / 13.68 | 27.00 / 25.89 |
| image, 50 pages | 14287 bytes | `38ab13363a7465f81dc2ae4e182c583eee13a868af2ae5e159947334f0392efe` | 2 / 1 | 2.538 / 1.840 | 27.5% | 14247 / 8406 | 3.69 / 2.44 | 21.80 / 20.84 |
| image, 200 pages | 54338 bytes | `ef33fb2e08f85bbbdfb9ef35ddc082a665b3526515d08e8c5271ce2f538ab6f9` | 2 / 1 | 9.333 / 5.695 | 39.0% | 52334 / 29826 | 11.13 / 6.23 | 24.09 / 23.58 |
| image, 500 pages | 134449 bytes | `6da4bca181d7ca169227320cf46be2fe0dcba803090c6431ad6d81cf02ecbe0e` | 2 / 1 | 18.611 / 9.895 | 46.8% | 128444 / 72602 | 26.12 / 13.89 | 26.83 / 26.17 |
