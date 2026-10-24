# Paperworks

Paperworks is a command-line tool for working with Word documents, Excel
spreadsheets, PowerPoint presentations, and PDFs. Create files from JSON, edit
existing documents, inspect their contents, and render pages for review—all from
your terminal or an automated workflow.

It works with local files. Microsoft Office is not required. LibreOffice is
needed for Office-to-PDF conversion, Office rendering, spreadsheet recalculation,
and field refresh; most other operations run directly in Paperworks.

## What you can do

| Format | Features |
| --- | --- |
| **Word (.docx)** | Create documents with headings, styles, tables, images, and equations. Edit text, add comments and tracked changes, and work with headers, footers, and tables of contents. |
| **Excel (.xlsx)** | Create and edit workbooks, formulas, formatting, tables, charts, and comments. Import CSV, trace formula references, and recalculate saved results with LibreOffice. |
| **PowerPoint (.pptx)** | Create slides with editable text, tables, charts, images, and speaker notes. Edit slide contents, reorder or duplicate slides, and manage comments. |
| **PDF (.pdf)** | Create PDFs, extract text and layout, inspect fonts, fill forms, merge or split files, select and rotate pages, add watermarks, and edit selected page objects. |
| **Review and export** | Inspect document structure as JSON, render page previews, export Office documents to PDF, and compare before-and-after documents visually. |

Paperworks is a file-processing tool, not a desktop editor. Commands support
specific operations and document features; they do not guarantee identical
rendering in every Office application. Render and review important edits before
sharing the result.

## Install

Download the ZIP for your operating system and processor from
[Releases](https://github.com/openvibely/openvibely-paperworks/releases/latest).

Builds are available for **macOS, Linux, and Windows**, each on **amd64 (x64)** and
**arm64**. Extract the executable and put it on your PATH. The examples below use
the name `paperworks`; rename the downloaded binary to `paperworks`
(`paperworks.exe` on Windows), or use its original filename.

macOS downloads are signed and notarized. Windows executables are signed and
timestamped. Each release includes `SHA256SUMS` for verifying the downloads.

```sh
paperworks version
paperworks capabilities
```

`capabilities` reports the available engines and whether LibreOffice was found.

### LibreOffice

Install LibreOffice if you need Office rendering, PDF export, recalculation, or
field refresh. Paperworks looks on PATH, in standard installation locations,
and in OpenVibely's dependency cache. To select a particular installation, set
`DOCUMENT_SOFFICE` to its executable path:

```sh
export DOCUMENT_SOFFICE="/Applications/LibreOffice.app/Contents/MacOS/soffice"
```

PDF operations use an embedded engine and do not require LibreOffice or Poppler.
Fonts available on the machine can affect document appearance.

## Try it

### Inspect a document

```sh
paperworks inspect report.docx
paperworks inspect budget.xlsx
paperworks pdf-fonts report.pdf
```

### Create a Word document

Save this as `report.json`:

```json
{
  "blocks": [
    {"type": "paragraph", "text": "A document created with Paperworks."}
  ]
}
```

Then create the document and export it to PDF:

```sh
paperworks create-docx --spec report.json --output report.docx
paperworks convert report.docx --output exported
```

The conversion step requires LibreOffice.

### Edit and review

Inspect the document to find the paragraph you want to change. For a document
whose first body paragraph contains `Draft`, save this as `edit.json`:

```json
{
  "operation": "replace-text",
  "paragraph": 0,
  "old": "Draft",
  "new": "Final",
  "occurrence": 0
}
```

```sh
paperworks word-edit draft.docx --spec edit.json --output final.docx
paperworks render final.docx --output preview
paperworks compare draft.docx --against final.docx --output comparison
```

Open the generated previews to check the result. Word paragraph indices are
zero-based; see the editing reference for how paragraphs are counted.

### Combine PDFs

```sh
paperworks pdf-merge cover.pdf report.pdf --output combined.pdf
```

## Command help and JSON inputs

```sh
paperworks help
paperworks help word-edit
paperworks schema word-edit
```

Command help describes arguments and flags. `schema` describes the JSON inputs
for supported creation and editing commands. Paths inside JSON specifications
are relative to the specification file.

Results are JSON on stdout, with diagnostics on stderr. Creation and editing
commands write new files and reject existing output files. Rendering can reuse
an output directory and overwrite matching preview images.

See [document editing commands and examples](docs/document-editing.md) for Word,
Excel, PowerPoint, and PDF operations, input fields, and limitations.

## Build from source

Use the Go version declared in [go.mod](go.mod):

```sh
go build -o paperworks ./cmd/paperworks
```

For development, `make deps` prepares test dependencies and `make test` runs the
test suite and vet. Linux test setup requires an existing LibreOffice installation.
Release maintainers can use the [release workflow](.openvibely/skills/paperworks_release_workflow/SKILL.md).
