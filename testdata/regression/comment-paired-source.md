# Pre-existing paired spreadsheet comments

`comment-paired-source.xlsx` is a synthetic OOXML compatibility fixture, not an Excel UI export. It extends `comment-identity-source.xlsx` by directly authoring the following package entries independently of `WorkbookComments` and its legacy-note helpers:

- `xl/comments/comment7.xml`: one B2 legacy comment with rich-text runs and author `tc={E1AEAE6A-3B24-4A7B-9634-7AB7C3DB172B}`, matching the existing root thread ID. The fallback includes the existing root and reply texts.
- `xl/drawings/vmlDrawing7.vml`: one hidden Note shape, `_x0000_s2049`, with zero-based Row/Column 1/1 for B2, plus the note shapetype and Office shape layout.
- Worksheet relationships `rIdNotes7` and `rIdVML7`, the worksheet's `legacyDrawing` reference, and comments/VML content types.

All original threaded comments and persons are retained. Non-default part names and rich-text fallback content exercise reuse of an imported pair rather than the writer's newly created default layout.

`TestExcelPairedThreadReply` checks this pairing before its first mutation, then verifies two distinct replies before and after save/reopen. It checks updated fallback text, native-only listing counts, one legacy note/author/shape, stable part names and relationships, and byte-preserved VML and worksheet XML. This fixture does not claim native Excel UI validation.
