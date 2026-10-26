package cli

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/openvibely/openvibely-paperworks/internal/docx"
	"github.com/openvibely/openvibely-paperworks/internal/wordedit"
)

func init() {
	register(&Command{
		Name:      "word-edit",
		ValueOpts: []string{"output", "spec"},
		Summary:   "Apply one Word edit operation (redline, comment, toc, merge, redact, ...) from a JSON spec into a new DOCX.",
		Usage:     "paperworks word-edit INPUT.docx --output NEW.docx --spec OPERATION.json\n  operations: " + strings.Join(wordedit.Operations, ", "),
		Run: func(ctx context.Context, a Args) (any, error) {
			in, err := input(a, 1)
			if err != nil {
				return nil, err
			}
			if err := existing(in[0]); err != nil {
				return nil, err
			}
			out := a.Path("output")
			if err := newFile(out, in[0]); err != nil {
				return nil, err
			}
			var spec map[string]any
			if err := a.JSON("spec", &spec); err != nil {
				return nil, err
			}
			return wordedit.Edit(in[0], out, spec, filepath.Dir(a.Path("spec")))
		},
	})
	register(&Command{
		Name:      "create-docx",
		ValueOpts: []string{"output", "spec"},
		Summary:   "Author a new DOCX from a JSON document spec (headings, runs, lists, tables, images, TOC, equations).",
		Usage:     "paperworks create-docx --spec DOC.json --output NEW.docx",
		Run: func(ctx context.Context, a Args) (any, error) {
			if len(a.Pos) != 0 {
				return nil, usagef("create-docx takes no positional arguments")
			}
			out := a.Path("output")
			if err := newFile(out, ""); err != nil {
				return nil, err
			}
			var spec docx.Spec
			if err := a.JSON("spec", &spec); err != nil {
				return nil, err
			}
			if err := docx.Create(spec, filepath.Dir(a.Path("spec")), out); err != nil {
				return nil, err
			}
			return map[string]any{"output": out, "blocks": len(spec.Blocks), "visual_review_required": true}, nil
		},
	})
}
