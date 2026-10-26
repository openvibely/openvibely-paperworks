package cli

import (
	"context"
	"path/filepath"

	"github.com/openvibely/openvibely-paperworks/internal/xlsx"
)

func init() {
	register(&Command{Name: "create-xlsx", ValueOpts: []string{"spec", "output"}, Summary: "Create an editable workbook from a JSON specification.", Usage: "paperworks create-xlsx --spec WORKBOOK.json --output NEW.xlsx", Run: func(ctx context.Context, a Args) (any, error) {
		if len(a.Pos) != 0 {
			return nil, usagef("create-xlsx takes no positional arguments")
		}
		out := a.Path("output")
		if err := newFile(out, ""); err != nil {
			return nil, err
		}
		var spec xlsx.Spec
		if err := a.JSON("spec", &spec); err != nil {
			return nil, err
		}
		if err := xlsx.Create(ctx, spec, filepath.Dir(a.Path("spec")), out); err != nil {
			return nil, err
		}
		return map[string]any{"output": out, "sheets": len(spec.Sheets), "recalculated": false, "visual_review_required": true}, nil
	}})
}
