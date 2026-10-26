package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/pdf"
	"path/filepath"
)

func init() {
	register(&Command{Name: "create-pdf", ValueOpts: []string{"spec", "output"}, Summary: "Create a flowing report or fixed-layout PDF with text, tables, images and forms.", Usage: "paperworks create-pdf --spec DOCUMENT.json --output NEW.pdf", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 0 {
			return nil, usagef("create-pdf takes no positional arguments")
		}
		out := a.Path("output")
		if e := newFile(out, ""); e != nil {
			return nil, e
		}
		var spec pdf.CreateSpec
		if e := a.JSON("spec", &spec); e != nil {
			return nil, e
		}
		if e := pdf.Create(ctx, spec, filepath.Dir(a.Path("spec")), out); e != nil {
			return nil, e
		}
		report, err := pdf.Report(ctx, out)
		if err != nil {
			return nil, err
		}
		return map[string]any{"output": out, "pages": report.Pages, "visual_review_required": true}, nil
	}})
}
