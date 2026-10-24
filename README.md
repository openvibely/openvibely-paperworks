# Paperworks

Standalone Go CLI for creating, editing, inspecting, and rendering documents.

Builds require the Go version declared in `go.mod` (also selected by CI).

```sh
go build -o paperworks ./cmd/paperworks
./paperworks version
./paperworks capabilities
./paperworks help
```

Office rendering and spreadsheet recalculation require LibreOffice, discovered
through `DOCUMENT_SOFFICE`, PATH, standard application locations, then OpenVibely’s
cache under `os.UserCacheDir()/openvibely/skill-tools/libreoffice`. The cache fallback
uses the binary’s OS/architecture, tries strict SemVer versions newest first, and
validates candidates with an isolated `--headless --version` call. Invalid or
non-SemVer installations are skipped; an explicit `DOCUMENT_SOFFICE` is never
overridden. Equal-precedence versions and package hashes use lexical order.
The fallback searches known executable layouts, not arbitrary custom package paths.
This fallback is available after RC6. PDF operations
use embedded PDFium WebAssembly. Equation rendering runs natively in Go.

Use command help for supported options. Creation and editing require new output
paths. JSON results go to stdout; diagnostics go to stderr.

PDF form filling accepts arbitrary choice values by default. Use
`paperworks fill INPUT.pdf --values values.json --output NEW.pdf --strict-choices`
to reject values outside a list or non-editable combo's options before writing.
For export/display option pairs, supply the export value; inherited options are
supported. Editable combos allow custom text. An empty string clears a selection;
an empty array clears a multi-select list (an empty item inside an array is still
validated against the options). Permissive fills report unmatched values in the
JSON `warnings` array, with `code: "unmatched_choice"`, `field`, `value`, and
`allowed_choices`. With `--flatten`, validation also covers existing choice values
whose appearances are regenerated. Rejected fills create no output and leave the
input unchanged.

```sh
go test ./...
bash scripts/build.sh
```

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

## Editing existing documents

See [document editing commands and JSON examples](docs/document-editing.md) for
Word, Excel, PowerPoint and PDF editing, field schemas, focused inspection and
preview export. Run `paperworks schema COMMAND` for the supported typed inputs.
