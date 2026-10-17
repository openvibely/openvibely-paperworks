#!/usr/bin/env bash
# Runtime smoke test: uses only the installed executable and shell/file tools.
# No LibreOffice is needed for this subset. It is not visual/native-app acceptance.
set -euo pipefail
if [[ $# != 2 ]]; then echo 'usage: bash scripts/smoke.sh /absolute/paperworks /new/output-directory' >&2; exit 2; fi
cli="$1"
output="$2"
if [[ "$cli" != /* || "$output" != /* ]]; then echo 'use absolute paths' >&2; exit 2; fi
if [[ -e "$output" ]]; then echo 'output already exists' >&2; exit 2; fi
cd "$(dirname "$0")/.."
mkdir -p "$output"
"$cli" version > "$output/version.json"
"$cli" capabilities > "$output/capabilities.json"
"$cli" create-xlsx --spec testdata/workflows/revenue.json --output "$output/revenue.xlsx" > "$output/create-xlsx.json"
"$cli" trace "$output/revenue.xlsx" --sheet Revenue --cell D4 > "$output/trace.json"
"$cli" inspect "$output/revenue.xlsx" > "$output/inspect-xlsx.json"
"$cli" create-pdf --spec testdata/workflows/revenue-pdf.json --output "$output/report.pdf" > "$output/create-pdf.json"
"$cli" inspect "$output/report.pdf" > "$output/inspect-pdf.json"
"$cli" render "$output/report.pdf" --output "$output/pages" --dpi 72 > "$output/render.json"
"$cli" create-pdf --spec testdata/workflows/revenue-flow.json --output "$output/flow.pdf" > "$output/create-flow.json"
"$cli" pdf-text "$output/flow.pdf" --output "$output/flow.txt" > "$output/flow-text.json"
"$cli" render "$output/flow.pdf" --output "$output/flow-pages" --dpi 72 > "$output/render-flow.json"
"$cli" create-pptx --spec testdata/workflows/revenue-deck.json --output "$output/deck.pptx" > "$output/create-pptx.json"
"$cli" verify-slides "$output/deck.pptx" --requirements testdata/workflows/revenue-deck-checks.json > "$output/verify-slides.json"
"$cli" slide-edit "$output/deck.pptx" --spec testdata/workflows/revenue-deck-edit.json --output "$output/edited.pptx" > "$output/edit-pptx.json"
"$cli" verify-slides "$output/edited.pptx" --requirements testdata/workflows/revenue-deck-checks.json > "$output/verify-edited.json"
cat > "$output/expression.txt" <<'TEX'
\begin{aligned} f(x)&=\sum_{n=0}^{\infty}\frac{x^n}{n!}\\ A&=\begin{pmatrix}1&2\\3&4\end{pmatrix}\end{aligned}
TEX
"$cli" equation "$output/expression.txt" "$output/equation.png" > "$output/equation.json"
"$cli" equation "$output/expression.txt" "$output/equation-inline.png" --inline > "$output/equation-inline.json"
"$cli" equation-fonts > "$output/equation-fonts.json"
echo "Smoke outputs: $output" >&2
