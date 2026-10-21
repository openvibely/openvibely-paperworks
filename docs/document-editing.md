# Document editing commands

Editors write a **new file** and reject an existing output. Failed edit batches
publish no partial file. Inspect first, edit, then render and actually view the
saved result before delivery. These commands do not claim identical rendering or
support for every feature in every Office document.

`paperworks schema COMMAND` prints field schemas for typed JSON inputs, including
nested library options. Runtime validation additionally checks required fields
for each operation. Paths inside specifications are relative to the spec file.

## Excel

```sh
paperworks workbook-edit input.xlsx --spec edits.json --output edited.xlsx
paperworks schema workbook-edit
paperworks import-csv input.csv --sheet Data --output imported.xlsx
paperworks import-csv input.tsv --delimiter tab --output imported.xlsx
```

```json
{"edits":[
 {"operation":"formula","sheet":"Data","cell":"C2","formula":"A2*B2"},
 {"operation":"fill-formula","sheet":"Data","range":"C3:C20","formula":"A3*B3"},
 {"operation":"style","sheet":"Data","range":"C2:C20","style":{"NumFmt":4,"Font":{"Bold":true}}},
 {"operation":"note","sheet":"Data","note":{"Cell":"C2","Author":"Reviewer","Text":"Confirm this assumption."}}
]}
```

All operations except add-sheet and defined-name operations require `sheet`.

| Operation | Additional fields |
| --- | --- |
| `value` | `cell`, literal `value` (leading `=` stays literal) |
| `formula` | `cell`, `formula`, optional `formula_style: "r1c1"` |
| `fill-formula` | `range`, top-left `formula`, optional `formula_style: "r1c1"` |
| `copy`, `copy-values`, `copy-formulas`, `copy-formats` | source `range`, destination top-left `target`; optional destination sheet in `name` |
| `clear-values`, `clear-formats`, `clear-all` | `range`; values includes formulas; all means cell contents/styles, not comments/drawings |
| `style` | `range`, Excelize `style` |
| `column-width`, `autofit-columns` | column `range` such as `A:C`; width needs `size` |
| `row-height` | one-based `index`, `size` in points |
| `panes`, `view` | Excelize `panes` or `view`; clearing panes uses `{"Freeze":false,"Split":false}` |
| `add-sheet`, `rename-sheet` | `name`; add does not need `sheet` |
| `delete-sheet`, `move-sheet` | move needs existing target sheet in `target` |
| `merge`, `unmerge` | `range` |
| `insert-rows`, `delete-row` | one-based `index`; insert needs `count` |
| `insert-columns`, `delete-column` | column name in `cell`; insert needs `count` |
| `set-name`, `delete-name` | Excelize `defined_name` |
| `add-table`, `set-table`, `delete-table` | add needs `table`; set needs old `name` and complete replacement `table`; delete needs `name` |
| `validation`, `delete-validation` | add needs `validation`; delete needs `range` |
| `conditional-format`, `delete-conditional-format` | `range`; add needs `conditional` array and optional differential `style` |
| `add-chart`, `replace-chart`, `delete-chart` | anchor `cell`; add/replace need complete Excelize `chart`, optional `combo` array |
| `add-picture`, `replace-picture`, `delete-picture` | anchor `cell`; add/replace need `picture: {"path":"asset.png","options":{...}}` |
| `note`, `delete-note` | add needs Excelize `note`; delete needs `cell` |

A1 copies translate relative references and retain absolute references and string
literals. Overlapping copies use a snapshot. `copy-formulas` also copies source
literals, without styles. `copy-values` uses saved results: recalculate first.
CSV imports keep leading zeros and never interpret formulas. Notes are separate
from existing threaded `comments` commands.

The broader editor uses Excelize and may normalize workbook XML. Prefer existing
`workbook-inputs` for its narrower literal-only preservation behavior when that
is sufficient. Recalculate before relying on formula caches. Complex arrays,
external links and specialized features require checks on the actual file.

New workbook charts additionally accept radar, bubble, stock-hlc, stock-ohlc and
an `options` object for full Excelize chart settings. New conditional formats
accept `options` for color scales, data bars and icon sets. See `schema create-xlsx`.
Nested Excelize field names use their exported names, such as `Font` and `NumFmt`.

## Word

```sh
paperworks word-edit input.docx --spec edit.json --output edited.docx
paperworks schema word-edit
```

```json
{"operation":"replace-text","paragraph":1,"old":"Paver","new":"Acme","occurrence":0}
```

Indices are **zero-based**. Body paragraph indices exclude table paragraphs; a
title counts. `part` optionally targets an existing header/footer XML part.
Targeted replacement searches ordinary consecutive text runs; fields, hyperlinks,
revision markup and annotation boundaries are not flattened into plain text.

| Operation | Fields |
| --- | --- |
| `replace-text` | `paragraph`, `old`, `new`, optional `occurrence` |
| `run-format` | `paragraph`, `run`; `font/font_size/bold/italic/strike/color/style` |
| `paragraph-format` | `paragraph`; `style`, `alignment` (`left/right/center/both`), `before/after/line` in twips |
| `paragraph-insert` | anchor `paragraph`, `position: before/after`, `text` |
| `paragraph-delete` | `paragraph`; section-break/directly annotated paragraphs are rejected |
| `table-cell-text` | `table`, `row`, `column`, `text`; replaces the selected cell content |
| `table-cell-format` | `table`, `row`, `column`, `fill` and paragraph/run format fields |
| `table-row-insert/delete` | `table`, `row`; insertion adds a blank row before the anchor |
| `table-column-insert/delete` | `table`, `column`; insertion adds a blank column before the anchor |
| `table-merge/unmerge` | `table`, `row`, `column`; merge needs `end_column` and retains cell text |
| `image-insert` | `paragraph`, PNG/JPEG `path`, optional `width_in/height_in/description` |
| `image-replace` | `image`, PNG/JPEG `path`, optional dimensions |
| `image-resize/delete` | `image`; resize needs `width_in` and/or `height_in` |
| `section-format` | `section`; `width/height/top/bottom/left/right/header/footer/gutter` in twips |
| `section-insert` | `section` whose settings to copy, terminating `paragraph`, `type: nextPage/continuous/evenPage/oddPage` |
| `header-footer` | `section`, `kind: header/footer`, `type: default/first/even`, `text` |
| `batch` | `edits` array of general operations; nested batches are rejected |

The zero-based `image` index counts standalone pictures in the selected part.
Shapes, charts and grouped artwork are excluded.

Run font sizes are points; colors are six hex digits. New paragraphs inherit
paragraph formatting. Image replacements create private media parts. Header/footer
changes follow normal Word section inheritance. Column edits require unmerged
rectangular tables. Merge/unmerge handles horizontal spans; vertical restructuring
is rejected. Existing redline, comment, control-fill, style-pack and specialized
operations remain available and are not replaced by this general editor.

## PowerPoint

```sh
paperworks slide-edit input.pptx --spec objects.json --output edited.pptx
paperworks deck-edit input.pptx --spec slides.json --output edited.pptx
paperworks schema slide-edit
paperworks schema deck-edit
```

```json
{"edits":[
 {"operation":"geometry","slide":1,"shape_id":"2","x":1,"y":1,"w":8,"h":1.5},
 {"operation":"run-format","slide":1,"shape_id":"2","paragraph":0,"run":0,"font_size":28,"bold":true},
 {"operation":"notes","slide":1,"text":"Mention the revised forecast."}
]}
```

Slides are **one-based**; paragraph/run/table indices are zero-based. Shape IDs
are IDs from inspection. Geometry uses inches, rotation degrees, fonts points.
`slide-edit` retains text/run-text/replace-text/table-cell/notes and adds:

- `geometry`: optional x/y/w/h/rotation/flip_h/flip_v. Inherited geometry
  needs explicit x/y/w/h to avoid losing the inherited dimensions.
- `delete/front/back`: remove or reorder a selected object.
- `run-format`: font/font_size/bold/italic/color on a selected paragraph/run.
- `paragraph-format`: align l/r/ctr/just.
- `fill`: solid hex fill; empty clears it.
- `replace-image`: PNG/JPEG path, creating a private media relationship.
- `crop`: left/right/top/bottom source fractions in a `crop` object.
- `table-row-insert/delete`, `table-column-insert/delete`: unmerged tables only.
- `table-cell-format`: row/column and fill/font_size/bold/color.
- `layout`: a `layout` object containing shape_ids, mode row/column/grid,
  x/y/width/height/gap, and columns for grid. This positions **and resizes** the
  selected objects into equal cells.

Use an existing master/layout `part` instead of `slide` to edit master/layout
objects. Notes apply to slides only; missing notes parts are now created.

`deck-edit` accepts ordered operations:

| Operation | Fields |
| --- | --- |
| `add-slide` | `content` using create-pptx Slide schema; optional insertion `position` |
| `duplicate-slide` | source `slide`, optional insertion `position` |
| `move-slide` | source `slide`, final `position` |
| `delete-slide` | `slide`; cannot delete the last slide |
| `add-element` | `slide`, native `element` using create-pptx Element schema |
| `replace-element` | `slide`, `shape_id`, complete replacement `element` |
| `set-layout` | `slide`, existing layout `part` |
| `replace-theme` | existing theme `part`, local DrawingML theme XML `path` |

Positions are evaluated after preceding edits; omitted insertion positions append.
Imported dependencies are copied and relationships remapped, retaining editable
objects. Deleted parts may remain orphaned in the ZIP: this is not sanitization.
Replacing a theme affects all objects using it. Charts also support radar/stock
and full Excelize `options`. Replacing a chart object requires its complete spec;
it does not infer omitted settings from the old chart.

## PDF

`create-pdf` retains fixed/flow/forms authoring. For embedded Unicode fonts and
vector drawing, use its mutually exclusive `canvas` format:

```json
{"canvas":{"pages":[{"width":612,"height":792,"elements":[
 {"type":"text","x":36,"y":50,"text":"Café — Δοκιμή","font_size":18},
 {"type":"rect","x":36,"y":70,"w":200,"h":60,"fill":"EEEEFF","stroke":"000000"}
]}]}}
```

Canvas points start at **top-left**. Types: text, rect, ellipse, line, polygon,
bezier, image. Polygon uses points; cubic bezier takes four points. Default embedded
Go font covers Latin/Greek/Cyrillic. Map names to local TTF paths in canvas.fonts,
then select font per text element for other glyphs. This is not complex-script
shaping. Existing flow/form authoring remains ASCII-only.

```sh
paperworks pdf-layout input.pdf --spec selection.json
paperworks pdf-objects input.pdf
paperworks pdf-object-edit input.pdf --spec edits.json --output edited.pdf
```

Layout extraction returns characters with boxes in PDF points from **bottom-left**.
Select one-based pages, bounds [left,bottom,right,top], max_chars. Explicit ascending
columns/rows boundaries extract table cells top-to-bottom; no OCR or automatic
arbitrary table detection is claimed.

Objects are indexed top-level page objects: 1 text, 2 path, 3 image, 4 shading,
5 form. Inspect and render before selecting. For example:

```json
{"edits":[{"page":1,"remove":[3,4,5],"overlay_pdf":"replacement-art.pdf","overlay_page":1,"matrix":[1,0,0,1,0,0]}]}
```

This removes selected objects and imports vector artwork from another PDF page.
Omit remove for an overlay. Matrix is [a,b,c,d,e,f], default identity. Indices are
relative to the page before each edit. Nested artwork in a form must be treated
as a whole form; no semantic “find the robot” detector is provided. This is not
secure redaction. Always verify the saved preview.

## Focused inspection and preview export

```sh
paperworks inspect-select input.pptx --path /slides/0 --max-chars 20000
paperworks inspect-select input.xlsx --sheet Data --range A1:F20 --search Revenue
paperworks inspect-select input.docx --search Paver --max-items 50
paperworks preview-export input.pdf --output previews --format jpeg --columns 3
```

Inspection selects JSON pointers or returns matching leaf values with their paths
and an explicit truncation flag. XLSX sheet/range selection returns values and
formulas for at most 10,000 cells; saved results may be stale.

Preview export supports PNG/JPEG/lossless WebP plus optional PNG contact sheets. Sheet/range and
resolution options are forwarded to the existing renderer. `visually_inspected`
is false until the model actually views the images. Office previews still use
the existing runtime/LibreOffice requirements and are not claimed pixel-identical
to Microsoft Office or Codex.

## Additional native slide drawing controls

New elements support `type: "connector"` with `connector.kind` set to
`straight`, `elbow`, or `curved`. Optional `start` and `end` each contain
`shape_id` and a zero-based connection `site`. Targets must exist in the slide.
For example:

```json
{"type":"connector","x":2,"y":2,"w":4,"h":1,
 "connector":{"kind":"elbow","start":{"shape_id":"2","site":1},"end":{"shape_id":"3","site":0}}}
```

`type: "path"` accepts `commands`: `move` and `line` each need one `[x,y]`
point, `quadratic` needs two, `cubic` needs three, and `close` needs none.
Coordinates use a 0..100000 square scaled into the element's inch bounds.
The first command must be `move`. Output remains editable vector geometry.

`drawing_style` works on new elements and existing objects via
`slide-edit` operation `drawing-style`:

```json
{"edits":[{"operation":"drawing-style","slide":1,"shape_id":"2",
 "drawing_style":{"gradient":{"angle":90,"stops":[{"position":0,"color":"2463EB"},{"position":1,"color":"FFFFFF"}]},
 "outline":{"color":"172B4D","width":2,"dash":"dash","tail":"triangle"},
 "shadow":{"color":"000000","opacity":0.25,"blur":4,"distance":2,"direction":45}}}]}
```

Colors are six hex digits; opacity and gradient positions are 0..1; angles
are degrees; outline width and shadow blur/distance are points. A solid `fill`
and `opacity` can be used instead of a gradient. Setting a shadow replaces
existing shape effects. Existing images accept `image-mask` with `mask` set to
`rect`, `roundRect`, or `ellipse`.

`paragraph-format` accepts `paragraph_style` with `align`, point-valued
`before`, `after`, `indent`, `margin_left`, `level` (0..8), `bullet` and
single-character `bullet_char`. The same object is available as `format` on
new paragraph definitions. `run-format` additionally accepts `underline` and
`hyperlink` (http/https/mailto URL, or empty string to remove).

Presentation chart creation adds `bubble`, `radar`, `stock-hlc`, `stock-ohlc`,
`area3D`, `bar3D`, `pie3D`, and `line3D`. Bubble categories must be numeric X
coordinates; each series needs `sizes` alongside `values`. `options` exposes
Excelize chart settings while type/series bindings come from the authoring spec.
This does not claim support for every chart name in the reference APIs.

```sh
paperworks chart-series-replace input.pptx --spec series.json --output updated.pptx
```

`series.json` contains `part` (chart XML part), `name`, `categories` and `values`.
This operation supports a single-series column chart and deliberately replaces
its embedded workbook. It preserves chart placement and formatting. Use the
existing chart input editor when other workbook cells must remain intact.

## Boundaries of these additions

These are native command implementations, not a guarantee of the entire API
surface of the reference's document libraries. Unsupported operations fail
rather than being described as completed edits. In particular:

- Word table restructuring does not handle vertical merges. There is no generic
  template-component cloning API or arbitrary floating-picture position editor.
- PowerPoint table restructuring requires unmerged tables. General table merges,
  composed layout trees, named-style authoring, patterned/image shape fills and
  rich speaker-note controls are not exposed by these new commands.
- Chart type names not listed above are not newly enabled. Full reference chart
  export support is itself not established; do not infer it from an API name.
- PDF layout extraction needs explicit table boundaries. Canvas drawing provides
  embedded fonts and paths, not the whole ReportLab flowable/shaping API.
- Field schemas describe JSON types; runtime checks enforce operation-specific
  requirements. The Word schema leaves older specialized operation fields open.

Use the new CLI alongside the existing specialized commands. The app's external
skills and distributed binary must be updated separately to advertise newly
added commands to a model. Creating preview files does not itself make the model
see them: it still needs its image-reading tool.
