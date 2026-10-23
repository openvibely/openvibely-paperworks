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
| `autofit-rows` | cell `range` such as `A1:D20`; fit saved formatted text after setting widths and wrapping |
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

`rename-sheet` updates sheet references in formulas, defined names, internal
hyperlinks, pivot sources, and form-control cell links, input ranges, and text-box
formulas (legacy VML and modern control properties). Unqualified control links,
references to other sheets or external workbooks, and formula string literals
are retained. VML drawings with no matching references are preserved byte-for-byte.

A1 copies translate relative references and retain absolute references and string
literals. Names outside the cell grid, such as `A1048577` and `XFE1`, remain names
when copying or filling formulas. Overlapping copies use a snapshot. `copy-formulas` also copies source
literals, without styles. `copy-values` uses saved results: recalculate first.
R1C1 formulas accept cells, whole rows/columns, and ranges, including relative
coordinates and sheet qualifiers. For example, `SUM(C1)` sums column A;
`SUM(C[-2]:C[-1])` sums the two columns to the left of the formula cell.
CSV imports keep leading zeros and never interpret formulas. A single leading
UTF-8 byte order mark (as written by Excel "CSV UTF-8") is ignored. Notes are separate
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
| `paragraph-format` | `paragraph`; `style`, `alignment` (`left/right/center/both`), `before/after/line` in twips; `line` sets exact line spacing |
| `paragraph-insert` | anchor `paragraph`, `position: before/after`, `text` |
| `paragraph-delete` | `paragraph`; section-break/directly annotated paragraphs and partial field deletions are rejected; complete fields may be deleted with their paragraph |
| `table-cell-text` | `table`, `row`, `column`, `text`; replaces the selected cell content |
| `table-cell-format` | `table`, `row`, `column`, `fill` and paragraph/run format fields |
| `table-row-insert/delete` | `table`, `row`; insertion adds a blank row before the anchor |
| `table-column-insert/delete` | `table`, `column`; insertion adds a blank column before the anchor |
| `table-merge/unmerge` | `table`, `row`, `column`; merge needs `end_column` and retains cell text |
| `image-insert` | `paragraph`, PNG/JPEG `path`, optional `width_in/height_in/description` |
| `image-replace` | `image`, PNG/JPEG `path`, optional dimensions |
| `image-resize/delete` | `image`; resize needs `width_in` and/or `height_in` |
| `section-format` | `section`; `width/height/top/bottom/left/right/header/footer/gutter` in twips |
| `section-insert` | `section` whose settings to copy, terminating `paragraph`, `type: nextPage/continuous/evenPage/oddPage` controlling where the following section starts; existing section starts are preserved |
| `header-footer` | `section`, `kind: header/footer`, `type: default/first/even`; `text` or table `rows` with `widths_twips` |
| `header-footer-link` | `section`, `kind: header/footer`, optional `type: default/first/even`, required boolean `linked` |
| `batch` | `edits` array of general operations; nested batches are rejected |

The zero-based `image` index counts standalone pictures in the selected part.
Shapes, charts and grouped artwork are excluded.

Run font sizes are points; colors are six hex digits. New paragraphs inherit
paragraph formatting. In `paragraph-insert` `text`, `table-cell-text` `text` and
`replace-text` `new`, `\n` (or `\r`; `\r\n` counts once) becomes a Word line
break (`w:br`) and `\t` becomes a tab (`w:tab`); replaced text keeps the source
run formatting. Image replacements create private media parts. Header/footer
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
  selected top-level objects into equal cells. Grouped children are rejected;
  their coordinates are relative to the group, not the slide.

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
objects. `delete-slide` removes the slide's presentation relationship, custom
show and section entries, and slide-jump hyperlinks (with their relationships)
on other slides that target it. After all edits, parts the edits made
unreachable from the package root (the slide, its notes, and media, charts,
embedded workbooks or comments used only by it) are removed with their `.rels`
files and content-type overrides. Parts still referenced elsewhere, plus
masters, layouts, themes, and notes/handout masters and their dependencies, are
kept. Orphans already in the input and their dependencies are left alone.
`docProps/app.xml` counts are not maintained by deck editing and are not updated.
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
arbitrary table detection is claimed. Text wrapped onto several lines inside one
cell is joined in reading order with a single space between lines (trailing
whitespace at a wrap is collapsed); the `characters` output is unchanged.

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
and an explicit truncation flag. PPTX inspection reports the presentation-ordered
slide entries under `slides` and preserves the previous count as `slide_count`.
A path such as `/slides/0` selects the same entry as the first slide in the full
report; it reads only the presentation relationships and selected slide, so
malformed unrelated package XML does not block it. XLSX sheet/range selection
returns values and formulas for at most 10,000 cells; saved results may be stale.

Preview export supports PNG/JPEG/lossless WebP plus optional PNG contact sheets. Each contact-sheet
page uses a 320×420 white cell with a 24-pixel dark filename band. Its thumbnail is
scaled proportionally into a centered 300×376 area below the band; cells are placed
in renderer order, left to right and then top to bottom. Sheet/range and resolution
options are forwarded to the existing renderer. `visually_inspected` is false until
the model actually views the images. Office previews still use
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
`before`, `after`, `indent`, `margin_left`, `line_spacing` (a positive line-height
multiplier, e.g. `1.2`; range 0.00001–1000), `level` (0..8), `bullet` and
single-character `bullet_char`. The same object is available as `format` on
new paragraph definitions. `run-format` additionally accepts `underline` and
`hyperlink` (http/https/mailto URL, or empty string to remove).

Presentation chart creation adds `bubble`, `radar`, `stock-hlc`, `stock-ohlc`,
`area3D`, `bar3D`, `pie3D`, `line3D`, `surface`, `surface3D`, `ofPie`, and
`barOfPie`. The pie variants require one series. Bubble categories must be numeric X
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

- Word table restructuring does not handle vertical merges. Template-component
  cloning has the dependency restrictions below; arbitrary floating-picture
  positioning is not exposed.
- PowerPoint row/column insertion and deletion require unmerged tables. Reusable named text styles are supported through `named_styles`; native
  PowerPoint theme-style authoring still uses imported XML resources. Table merge/unmerge, nested layouts, pattern/image
  fills and formatted notes are covered by the local extensions below.
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

## Local document-editing extensions (unreleased)

These commands require a build from this checkout. They are not in OV's pinned
`v0.0.1-rc2` binary. To try them without publishing a release:

```sh
go build -o /tmp/paperworks-local ./cmd/paperworks
/tmp/paperworks-local schema slide-edit
```

### Copy a Word template component

`word-edit` accepts `clone-component`. `start` and `end` are inclusive, zero-based
indices of direct body blocks (paragraphs, tables and content controls), excluding
the final section properties. `path` is an optional source template, relative to
the spec file; omitted means the input document. `before` selects a destination
block, or omit it to append before the final section properties.

```json
{"operation":"clone-component","path":"template.docx","start":1,"end":2,"before":0}
```

The copy imports related images, links, charts and their related parts, styles
and numbering. Conflicting style IDs and bookmark names are renamed; internal
hyperlinks follow the cloned bookmarks. The destination keeps its page setup,
headers, footers, theme and document-wide defaults. Explicit source styles
(including a conflicting default paragraph style) are imported. Unsupported
dependencies fail explicitly: section
breaks, partial bookmark ranges, fields, comments, footnotes/endnotes, tracked
revisions, externally bound controls and embedded `altChunk` documents. Cloning
is not a replacement for copying complete document stories.

### Merge or unmerge a PowerPoint table

```json
{"edits":[
  {"operation":"table-merge","slide":1,"shape_id":"3",
   "row":0,"column":0,"end_row":1,"end_column":1}
]}
```

The inclusive rectangle uses zero-based cells. Text from every selected cell is
kept in the top-left cell, in row order, preserving paragraph/run formatting.
Other cells and grid dimensions remain unchanged. Existing merged regions cannot
be overlapped. To split a merge, use `table-unmerge` with its top-left `row` and
`column` only. Combined text stays at the top-left; original text distribution
is not reconstructed. Edit merged text using that origin, not a covered cell.
Row/column insertion and deletion still require an unmerged table.

### Pattern and image shape fills

Use `drawing_style` on a created element, or `drawing-style` through `slide-edit`:

```json
{"edits":[
  {"operation":"drawing-style","slide":1,"shape_id":"2",
   "drawing_style":{"pattern":{"preset":"diagCross","foreground":"16324F","background":"FFFFFF"}}},
  {"operation":"drawing-style","slide":1,"shape_id":"4",
   "drawing_style":{"image":{"path":"texture.png","fit":"cover"}}}
]}
```

`pattern.preset` accepts DrawingML preset pattern names, such as `pct50`, `cross`,
`diagCross`, `smCheck`, `horzBrick`, `weave` and `zigZag`. Colors use six hex digits.
Image fills embed PNG/JPEG data and accept `stretch` (default), `cover` or `tile`.
Cover uses the shape's current dimensions, with optional `anchor` and `crop` in
the same format as image elements. Apply geometry/layout edits before the cover
fill when both change. Creation applies slide layout before calculating fills.
Fill paths resolve relative to the spec file. Solid, gradient, pattern and image
fills are mutually exclusive. Outline and shadow can accompany any fill.

### Formatted speaker notes

A created slide can use `notes_paragraphs` instead of `notes`. A `notes` edit can
use `paragraphs` instead of `text`. Both accept the existing paragraph/run fields:

```json
{"edits":[{"operation":"notes","slide":1,"paragraphs":[
  {"runs":[{"text":"Opening: ","bold":true},{"text":"Explain the result.","italic":true}]},
  {"text":"Mention the caveat","bullet":true,"level":1,"format":{"after":6}}
]}]}
```

This replaces the notes body's paragraphs while retaining other notes
placeholders. Runs support bold, italic, underline, font size and color;
paragraphs support bullets, levels, alignment, spacing and indentation.
An empty array clears the notes text.

### Nested slide layout

Use `layout.root` for a tree, instead of the original flat `shape_ids`/`mode`
fields. It is available in `slide-edit` and as `layout` on a created slide:

```json
{"edits":[{"operation":"layout","slide":1,"layout":{
  "x":0.5,"y":0.5,"width":12,"height":6,
  "root":{"mode":"row","gap":0.3,"children":[
    {"shape_id":"2","width":3},
    {"mode":"column","gap":0.2,"children":[
      {"shape_id":"3","weight":1},
      {"shape_id":"4","weight":2}
    ]}
  ]}
}}]}
```

Containers support `row`, `column` and `grid` (with `columns`), plus `gap` and
`padding`. A leaf names a top-level slide shape. Dimensions and spacing are in
inches. Optional positive `width`/`height` fix a node's size; otherwise it fills
available space. Row/column siblings divide remaining space by `weight` (default
1). Grid cells are equal-sized. Fixed cross-axis sizes align at the top/left.
This positions/resizes existing native shapes. The optional `fit_text` control
below measures text height; layout does not automatically rerun after later edits. Nested grouped shapes
must be edited in their own coordinate system and are rejected here.


### Content-sized slide text and grid spans

On a layout leaf, `fit_text:true` calculates height at its allocated width using
font glyph advances and line metrics. In a column, the next item starts after
that measured height and the gap. Omit `height` on fitted leaves. For example:

```json
{"edits":[{"operation":"layout","slide":1,"layout":{
 "x":0.5,"y":0.5,"width":4,"height":6,
 "root":{"mode":"column","gap":0.2,"children":[
  {"shape_id":"2","fit_text":true},{"shape_id":"3","fit_text":true}
 ]}
}}]}
```

Fonts must be resolved in the runs or paragraph defaults and available locally;
`Go` regular/bold/italic faces are embedded for measurement. Unsupported font
or glyph lookup fails instead of silently substituting a different face. The
measurement accounts for wrapping, newlines, mixed run sizes, text-box insets,
paragraph margins, point-based before/after spacing and proportional line spacing.
Nonzero character spacing and percentage-based before/after paragraph spacing
are rejected; use explicit sizing without `fit_text` and verify the render.
Rotated/vertical text, unresolved fields and fixed-point line spacing also need
explicit sizing.
This is glyph-based measurement, not a complex-script shaping engine. Review the
render in the intended Office engine, whose font substitution may differ.
Oversized text fails instead of being squeezed into its frame. Rerun layout
explicitly after changing text or widths.

Grid children accept positive `column_span` and `row_span` (default 1). They
occupy consecutive free cells in row-major order. A spanning heading followed
by two ordinary leaves can be written as:

```json
{"mode":"grid","columns":2,"children":[
 {"shape_id":"2","column_span":2},
 {"shape_id":"3"},{"shape_id":"4"}
]}
```

Rows expand to fit the packed items, then divide the frame equally; column spans
include internal gaps. Spans apply only to grid children. There is a 10,000-cell
limit; no overlapping cells are produced.

### Fit spreadsheet row heights

```json
{"edits":[{"operation":"autofit-rows","sheet":"Data","range":"A1:D20"}]}
```

Run through `workbook-edit`. Set final column widths and wrapping first; measure
only the requested range (up to 10,000 cells). It uses formatted saved values,
font metrics and rich-text runs, and changes the affected row heights. Include
all columns whose content should determine those heights. Recalculate formula
results beforehand; a formula with no saved result fails. A saved empty-string
result (`t="str"` with an empty `<v/>`) is treated as blank, so other visible
cells determine the row height or an otherwise empty row uses its baseline height.
Formatting changes can alter the displayed value, so fit after formatting too.

Complete horizontal merges are supported. Vertical merges, rotated text,
shrink-to-fit and unavailable fonts fail; results beyond Excel's 409.5-point row
height limit also fail. Rows outside the requested range remain unchanged.
Inspect the final worksheet in the target engine.

### Create a Word header/footer table

Use `rows` instead of `text` in `header-footer`:

```json
{"operation":"header-footer","section":0,"kind":"footer","type":"default",
 "rows":[["Left","Middle","Right"]],"widths_twips":[2000,2000,2000]}
```

The section index is zero-based. `type` may be `default`, `first` or `even`.
`widths_twips` supplies one positive width per column; choose widths within the
section's usable width. The rectangular table replaces this section's selected
header/footer with a private part, preserving body content. Later sections that
inherit this header/footer also display the replacement. Inspect section
references before editing; a private part does not break inheritance. If the
change must affect only one section, preserve the later sections' original
headers/footers with explicit references before replacing the selected one.
Cells retain text, tabs and line breaks as native Word content. Subsequent
formatting/table edits can target that part with existing `word-edit` operations.
Reopen and verify the section relationship, cell text and rendered page furniture.

### Control header and footer inheritance

Newer local builds expose `header-footer-link` through `word-edit` and general
edit batches. The pinned `v0.0.1-rc2` does not include it.

```json
{"operation":"header-footer-link","section":1,"kind":"footer","type":"default","linked":false}
```

Sections are zero-based. `linked:true` removes the selected section's reference,
so it inherits the nearest preceding definition of that kind/type (or displays
nothing if none exists). It leaves the detached part intact for other sections.
`linked:false` creates an empty independent definition only when no explicit
reference exists; it does not copy inherited text, tables, images or fields.
An already independent definition remains unchanged. This matches the documented
`python-docx` `is_linked_to_previous` behavior.

Changing linkage does not enable first-page or even-page variants and does not
change other kinds/types. Those layout flags remain as saved. Later sections
that inherit the edited section can also change appearance. Inspect their
references before editing; unlinking is not a way to preserve inherited content.
Use `header-footer` to replace content separately. Link operations reject `text`,
`rows` and `widths_twips`; use the main document rather than a story `part`.
Render affected sections and verify their effective header/footer content.

### Request Word field refresh on open

`paperworks word-edit input.docx --spec refresh-on-open.json --output deferred.docx`
with `{"operation":"update-fields"}` sets `w:updateFields=true`. It does not
calculate fields or alter their saved results. Verify the results in Word after
opening and updating fields; application settings or locked fields may prevent refresh.
Only the settings part changes when it exists. If missing, the command also
registers that part in content types and document relationships. For strict
preservation, authorize those exact parts and disclose that refresh is deferred.

### Slide visibility

`create-pptx` slides accept `hidden:true`. Existing decks use
`slide-edit` with `{"edits":[{"operation":"visibility","slide":1,"hidden":true}]}`.
Use `false` to show the slide again. No shape ID is needed. Hidden slides remain
in the file and previews; slideshow applications skip them.

### Text boxes

Created text/shape elements accept `text_box`. Existing shapes use
`{"operation":"text-box","slide":1,"shape_id":"2","text_box":{"insets":{"left":12,"right":12,"top":6,"bottom":6},"vertical_align":"middle","wrap":"square","auto_fit":"none"}}`
inside `slide-edit`'s `edits` array. Insets are points; omitted fields stay unchanged.
Alignment is `top`, `middle` or `bottom`; wrap is `square` or `none`; autofit is
`none`, `shrinkText` or `resizeShapeToFitText`. Autofit is a native application
setting, not measured layout: render and check the result, and rerun measured
layout after padding/wrapping changes.

### Slide backgrounds

Creation accepts `background_fill` instead of the solid `background` string.
Use a `DrawingStyle` fill (`fill`, `gradient`, `pattern` or `image`), without
outline/shadow. For existing slides use `slide-edit` with
`{"edits":[{"operation":"background","slide":1,"background_fill":{"gradient":{"angle":90,"stops":[{"position":0,"color":"FFFFFF"},{"position":1,"color":"17324D"}]}}}]}`.
Image fills use local paths relative to the spec and `stretch`, `cover` or `tile`.
Cover uses the slide dimensions. This changes the actual background, not a
full-slide shape. Render to check contrast and cropping.

### SVG pictures

Image elements accept a local `.svg` `path` or inline `svg` text instead of
`path`. `replace-image` accepts the same sources. The SVG is embedded as vector
image data; it is not converted to editable PowerPoint shapes or rasterized.
Use self-contained SVG with embedded assets and positive width/height or a
viewBox. Existing contain/cover/crop settings apply. SVG display requires an
SVG-capable presentation application; no legacy raster fallback is generated.
Render the saved deck to verify artwork, fonts and cropping.

### One-position stacking

`slide-edit` operations `forward` and `backward` move the selected `shape_id`
one drawable position within its slide/group. At either end they do nothing.
`front` and `back` still move to the ends. Example:
`{"edits":[{"operation":"forward","slide":1,"shape_id":"2"}]}`.
These change overlap order, not geometry or group membership.

### Size-preserving automatic layout

Use `auto_layout` on a created slide, or `slide-edit` with
`{"edits":[{"operation":"auto-layout","slide":1,"auto_layout":{"shape_ids":["2","3"],"direction":"horizontal","x":1,"y":1,"width":10,"height":4,"align":"center","auto_gap":true}}]}`.
It preserves object sizes and places them in the given order. Unlike `layout`,
it does not resize shapes. Directions are `horizontal` and `vertical`.
Alignment is `center`, `topLeft`, `topCenter`, `topRight`, `left`, `right`,
`bottomLeft`, `bottomCenter` or `bottomRight`. Set a numeric `gap` or
`auto_gap:true` to distribute free space between objects. A single object
uses alignment. `padding_x`/`padding_y` inset both frame edges; all units are inches.
Omit frame coordinates for the whole slide, or use `frame_shape_id` for an
existing top-level shape's frame. The frame cannot also be a selected object.
Objects need explicit geometry and must be top-level; inherited geometry and
overflow fail instead of guessing or shrinking. Layout uses unrotated frames.
Rerun after edits and inspect the render for rotated objects or overlapping content.

### Glyph fills and outlines

New text elements accept `text_paint`; `slide-edit` operation `text-paint` applies it to a whole text shape, and `run-format` accepts it for one inspected paragraph/run. Example:

```json
{"operation":"text-paint","slide":1,"shape_id":"2","text_paint":{"fill":{"gradient":{"angle":0,"stops":[{"position":0,"color":"FF0000"},{"position":1,"color":"0000FF"}]}},"outline":{"width":1,"fill":{"fill":"000000"}}}}
```

These are glyph effects, not text-box backgrounds. Fill and outline fills accept solid colors, linear gradients or patterns; outline width is in points, zero removes the outline. Omitted settings survive. Images/shadows are not text fills. Inspect the rendered result in the intended application.

### Table cell formatting and individual sizes

Use `slide-edit` operation `table-format` with an inspected table shape ID, or `table_format` on a new table element:

```json
{"operation":"table-format","slide":1,"shape_id":"2","table_format":{"column_widths":[120,180],"row_heights":[30,40],"cells":[{"row":0,"column":0,"vertical_align":"middle","margins":{"left":8,"right":8},"borders":{"bottom":{"width":1,"fill":{"fill":"000000"}}}}]}}
```

Sizes, margins and border widths are points. Size arrays cover the whole axis; omitted axes stay unchanged. The frame is synchronized to the grid. Cell indices are zero-based; target the origin of a merged cell. Borders use `left`, `right`, `top`, `bottom` and the text-outline fill definition. Cells also accept `fill` and `text_paint`. Omitted properties and cell text survive; these controls do not rebuild a table.

### Waterfall and funnel data

`extended-charts` can also add and replace literal data for `waterfall` and
`funnel`. Use `--requirements` with `action`, `type`, `slide`, `position_emu`,
`values`, and optional `categories` for creation. For `replace-data`, supply the
inspected `chart_part` and `values`; omitted categories retain existing labels.
Funnel values must be nonnegative with at least one positive value. Waterfall
values are signed increments. Binning and hierarchy paths do not apply.
Linked workbook charts still use `chart-inputs`.

### Targeted standard-chart formatting

`slide-edit` operation `chart-format` selects an existing chart with `slide` and
`shape_id`. It changes only supplied settings and preserves chart data, formula
references, embedded workbooks, and placement. `chart_format` accepts:

- `axes`: select an existing axis by its native `id` (`c:axId`). Set `visible`,
  `title` (empty removes it), `minimum`, `maximum`, `major_unit`, or `number_format`.
  Numeric bounds apply to value/date axes.
- `legend`: `visible`, `position` (`b`, `t`, `l`, `r`, `tr`), and `overlay`.
- `series`: select by zero-based document-order `index`. Set `fill`, `outline`
  (the same paint objects used for text), or `labels` with `show_value`,
  `show_category`, `show_series`, `show_percent`, `position`, `number_format`.
  Percent labels apply to pie/doughnut families. Surface/stock labels are rejected.

```json
{"edits":[{"operation":"chart-format","slide":1,"shape_id":"2","chart_format":{"legend":{"position":"b"},"series":[{"index":0,"fill":{"fill":"336699"},"labels":{"show_value":true}}]}}]}
```

Read axis IDs from the chart XML identified by inspection; do not guess them.
This operation edits the chart part itself, so references sharing that part see
the same formatting. It does not style ChartEx charts or reconstruct image-only
charts. Per-point label overrides remain intact.

Gradient fills also accept `path:"circle"`, `"rect"`, or `"shape"` instead of a
linear angle. Optional `focus` has `left`, `top`, `right`, `bottom` insets in
0..1; omission focuses the gradient at the center. Opposing insets cannot sum
to more than 1. Path gradients apply wherever a gradient paint is accepted,
including text glyphs and outlines.

### Detailed existing-chart formatting (after RC4)

`slide-edit` → `chart-format` additionally accepts:

- Axes: positive `minor_unit`, `text_style`, `major_gridline`/`minor_gridline`
  (`TextOutline`, points). Numeric intervals require value/date axes.
- Legend: `text_style`, solid/gradient/pattern `fill`, and `outline`.
- Series labels: `text_style`.
- Series `points`: `{ "index": 0, "fill": {"fill":"FF0000"}, "outline": ... }`.
  Indices refer to the series' cached values; the data remains unchanged.
- Series `trendlines`: replaces that series' trendlines; `[]` removes them.
  Types: `linear`, `exponential`, `logarithmic`, `polynomial`, `power`,
  `movingAverage`. Optional `name`, `order` (polynomial, 2..6), `period`
  (movingAverage, 2..255), `forward`, `backward`, `intercept`,
  `display_equation`, `display_r_squared`, and `outline`.
- Series `error_bars`: `type` (`standardError`, `percentage`,
  `standardDeviation`, `none`), optional `value`, `direction` (`x`/`y`),
  `end_style` (`cap`/`noCap`) and `outline`. Defaults: direction y, cap,
  value 1 for percentage/deviation. Standard error has no value.
  `none` removes bars in the selected direction.

`text_style` has `font`, `font_size` (points), six-digit hex `color`, `bold`,
`italic`. Omitted properties remain unchanged. Trendlines/error bars require
non-stacked 2D bar, line, area, scatter or bubble charts; x bars require scatter
or bubble. Moving averages cannot specify forecast/intercept/equation options.
ChartEx remains excluded. Render the result; chart layout is application-dependent.


Preview limitation: the tested LibreOffice build ignores line-chart marker colors
(including series-level colors) even when native marker fills are stored. Verify
those colors in the intended PowerPoint application; a LibreOffice preview alone
does not confirm them.

### Reusable named text styles (after RC4)

`create-pptx` and `slide-edit` accept a top-level `named_styles` map. Definitions
have `description`, `font`, `font_size` (points), `color` (six hex digits),
`bold`, `italic`, `underline`, and `align` (`l`, `r`, `ctr`, `just`). Omitted
properties leave existing formatting intact; explicit false clears a flag.

```json
{"named_styles":{"Heading":{"font_size":28,"bold":true,"color":"17324D","align":"ctr"}},"edits":[{"operation":"apply-style","slide":1,"shape_id":"2","style":"Heading"}]}
```

`apply-style` defaults to `scope:"shape"`. Use `scope:"paragraph"` plus a
zero-based `paragraph`, or `scope:"run"` plus zero-based `paragraph`/`run`.
Run-scoped styles cannot set paragraph alignment. Shape scope also styles field
text and empty-paragraph defaults, without replacing text or hyperlinks.

For creation, set `style:"Heading"` on a text/shape element, or a run in its
`paragraphs`. Named styles override conflicting authored text properties; run
styles apply after the element style. Explicit paragraph `format` and element
`text_paint` are applied afterward. Geometry is measured after style application.
Notes do not accept named styles. Definitions are spec-level recipes: exported
PPTX stores resolved formatting, not live named-style bindings. Supply the map
again in a later edit spec; changing a definition does not restyle prior output.

### Individual chart labels (after RC4)

A `chart-format` series accepts `label_overrides`, selecting zero-based cached
point indices. Each override accepts `text`, `text_style`, `fill`, `outline`,
and `position` (the same tokens as series labels). Omitted fields and other
labels remain unchanged. Custom text replaces that label's text formula/runs,
not the series values or workbook. Existing hidden-label settings are preserved.

```json
{"operation":"chart-format","slide":1,"shape_id":"2","chart_format":{"series":[{"index":0,"label_overrides":[{"index":1,"text":"Priority","text_style":{"font_size":18,"bold":true},"position":"outEnd"}]}]}}
```

### Axis placement (after RC4)

`chart-format.axes` accepts `position` (`bottom`, `left`, `right`, `top`) and
`tick_label_position` (`nextTo`, `high`, `low`, `none`). Select the inspected
axis `id`; omitted settings are retained. This changes native placement flags,
not axis-crossing values. Render to verify placement with the chart's existing
crossing configuration.

### Chart markers and creation series options (after RC4)

A `chart-format` series accepts `marker` with `symbol` (`auto`, `circle`,
`diamond`, `dot`, `none`, `plus`, `square`, `star`, `triangle`, `x`), `size`
(2..72 points), `fill` and `outline`. Line, scatter and radar families support
markers. Omitted properties and per-point marker overrides remain unchanged.

Creation preserves `chart.options.Series[i]` formatting (Excelize field names,
such as `Marker`, `Line`, `DataLabel`). `Name`, `Categories`, `Values`, and `Sizes`
are always replaced with references to the generated workbook. Options map to
series by index; more options than data series is an error. Missing entries use
default formatting. This applies to standard charts, not ChartEx creation.

### Existing chart backgrounds (after RC4)

`chart-format` accepts `chart_fill` for the outer chart area and `plot_area_fill`
for the plot interior. Each uses a solid, gradient or pattern `DrawingStyle`;
image fills, shadows and embedded outlines are rejected. Existing borders, data,
axes and labels are retained. For example:

```json
{"operation":"chart-format","slide":1,"shape_id":"2","chart_format":{"chart_fill":{"fill":"FFFFFF"},"plot_area_fill":{"fill":"F2F5F9"}}}
```
