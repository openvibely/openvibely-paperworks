package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/xlsx"
	"path/filepath"
)

func init() {
	register(&Command{Name: "workbook-edit", Summary: "Apply ordered edits to an existing workbook; write a new file. Recalculate before using formula results.", Usage: "paperworks workbook-edit INPUT.xlsx --spec EDITS.json --output NEW.xlsx\n  sizing operations: autofit-columns, autofit-rows (cell range), column-width, row-height", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 1 {
			return nil, usagef("one input workbook required")
		}
		out := a.Path("output")
		if err := newFile(out, a.Pos[0]); err != nil {
			return nil, err
		}
		var s xlsx.EditSpec
		if err := a.JSON("spec", &s); err != nil {
			return nil, err
		}
		err := xlsx.EditFile(ctx, a.Pos[0], out, filepath.Dir(a.Path("spec")), s)
		return map[string]any{"output": out, "recalculated": false, "visual_review_required": true}, err
	}})
	register(&Command{Name: "import-csv", Summary: "Import CSV/TSV fields as literal workbook values.", Usage: "paperworks import-csv INPUT.csv --output NEW.xlsx [--sheet NAME] [--delimiter tab]", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 1 {
			return nil, usagef("one input file required")
		}
		out := a.Path("output")
		if err := newFile(out, a.Pos[0]); err != nil {
			return nil, err
		}
		delimiter := ','
		if s := a.Str("delimiter"); s != "" {
			if s == "tab" {
				delimiter = '\t'
			} else if len([]rune(s)) == 1 {
				delimiter = []rune(s)[0]
			} else {
				return nil, usagef("delimiter must be one character or tab")
			}
		}
		err := xlsx.ImportCSV(ctx, a.Pos[0], out, a.Str("sheet"), delimiter)
		return map[string]any{"output": out}, err
	}})
}
