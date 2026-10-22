# Portable form appearance fixtures

These PDFs freeze the outputs of the existing Go flattening contract cases.
Before capture, every checked page was rendered on macOS using the existing
PDFium LCD helper at 144 DPI and matched its original RGB SHA-256 in
`testdata/reference-mirror/form-oracles.json` exactly. No production code was
changed during capture. These are Go-generated appearance-equivalent fixtures,
not original reference-library PDF outputs.

The contract tests render these fixed PDFs and newly generated outputs using
the same engine and host fonts, and require exact pixel equality. This retains
the verified visual baseline without requiring Linux font substitutions to
match macOS glyphs for unembedded standard fonts. The original source PDF,
RGB hashes, and preview PNGs remain unchanged in `testdata/reference-mirror`.

Each filename identifies the existing oracle case. All PDFs have two pages;
as in the original oracle, the transform cases compare only the first page.
Do not regenerate these fixtures as part of a test run or to accept a new
rendering difference without reviewing it against the original contract.
