# Paperworks

Standalone Go CLI for creating, editing, inspecting, and rendering documents.

```sh
go build -o paperworks ./cmd/paperworks
./paperworks version
./paperworks capabilities
./paperworks help
```

Office rendering and spreadsheet recalculation require LibreOffice, discovered
through `DOCUMENT_SOFFICE`, PATH, or standard application locations. PDF operations
use embedded PDFium WebAssembly. Equation rendering runs natively in Go.

Use command help for supported options. Creation and editing require new output
paths. JSON results go to stdout; diagnostics go to stderr.

```sh
go test ./...
bash scripts/build.sh
```

Release packages are built with Go 1.26.4:

```sh
PAPERWORKS_VERSION=0.0.1-rc1 bash scripts/release.sh
```

This creates `dist/paperworks-{os}-{arch}.zip`, `SHA256SUMS`, and build-information
files. Each ZIP contains `paperworks` at its root (`paperworks.exe` on Windows).
Publish the ZIPs and checksums, then use each asset URL and ZIP checksum in the
OpenVibely skill dependency's `packages` entry (`format: zip`). LibreOffice is a
separate dependency; Node and SheetJS are test-only and are not bundled.

Pushing a version tag such as `v0.0.1-rc1` runs `.github/workflows/release.yml`:
tests and vet, cross-platform builds, a Linux smoke test, then GitHub publication.
Tags containing a prerelease suffix are marked prereleases, not the latest stable
release. Existing release assets are never overwritten automatically.

GitHub Actions installs LibreOffice, Node, and fonts, then uses `make deps` for
the checksum-verified SheetJS 0.20.3 reader and shared dependency checks. Downloaded
tools are cached between runs. CI runs the full Go suite and vet. Test logs are retained
as workflow artifacts, including failures and skips. Pinned macOS font tests can
skip on Linux; installing dependencies does not guarantee acceptance tests pass.

For native local integration tests:

```sh
make deps       # Once: reuse installed tools/cache, download only missing tools
go test ./...   # Discovers dependencies automatically; no shell exports
make test       # Ensure dependencies, then run the full suite and vet
```

The download script is an implementation detail of `make deps`. Plain `go test`
never downloads or installs anything. Missing dependencies fail with instructions
to run `make deps`; explicit environment overrides take precedence over the cache.

Go must already be installed. On macOS, setup reuses Node 22+ and
LibreOffice when available, or downloads pinned official releases into
`~/Library/Caches/paperworks-tests`. It verifies SHA-256 checksums before
extracting Node and copying LibreOffice from its DMG. No Homebrew, Java,
Docker, administrator access, or system installation is needed. The first run
may download several hundred megabytes; subsequent runs reuse the cache.

The pinned SheetJS reader is downloaded and verified automatically. It is only
a test dependency. `DOCUMENT_SOFFICE` and `PAPERWORKS_TEST_SHEETJS` overrides
are respected (the reader must have the pinned checksum). `XDG_CACHE_HOME`
overrides the cache parent. Existing system fonts are used; font-dependent
tests can fail or skip when their required fonts are absent.

On Linux, Node and SheetJS can be downloaded the same way, but LibreOffice must
already be installed: a portable Linux Office download is not configured.
The script stops with an explanation if it is unavailable. Tests and vet run
natively, and failures return a nonzero exit status; dependencies do not waive
acceptance failures or platform-specific skips.

Document skills are maintained outside this repository. Regression examples live
under `testdata/document-examples/`.
