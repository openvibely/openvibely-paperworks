package cli

import (
	"context"

	"github.com/openvibely/openvibely-paperworks/internal/office"
)

func init() {
	register(&Command{Name: "comments", ValueOpts: []string{"requirements", "output"}, Summary: "List, add, reply to, resolve, or reopen native XLSX comment threads.", Usage: "paperworks comments INPUT.xlsx [--requirements COMMENT.json --output NEW.xlsx]", Run: func(ctx context.Context, a Args) (any, error) {
		file, threads, err := listOrEdit(ctx, a, "comments", "XLSX", office.CommentSpec{Action: "list"},
			func(s office.CommentSpec) string { return s.Action }, office.WorkbookComments)
		if err != nil {
			return nil, err
		}
		return map[string]any{"file": file, "comments": threads, "native_excel_ui_verified": false}, nil
	}})
}
