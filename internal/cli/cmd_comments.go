package cli

import (
	"context"

	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func init() {
	register(&Command{Name: "comments", Summary: "List, add, reply to, resolve, or reopen native XLSX comment threads.", Usage: "paperworks comments INPUT.xlsx [--requirements COMMENT.json --output NEW.xlsx]", Run: func(ctx context.Context, a Args) (any, error) {
		in, e := input(a, 1)
		if e != nil {
			return nil, e
		}
		if ext(in[0]) != ".xlsx" {
			return nil, usagef("comments requires XLSX")
		}
		spec := office.CommentSpec{Action: "list"}
		if a.Str("requirements") != "" {
			if e = a.JSON("requirements", &spec); e != nil {
				return nil, e
			}
		}
		edit := spec.Action != "" && spec.Action != "list"
		out := a.Path("output")
		if edit {
			if e = newFile(out, in[0]); e != nil {
				return nil, e
			}
		} else if out != "" {
			return nil, usagef("list is read-only; omit --output")
		}
		p, e := opc.Open(in[0])
		if e != nil {
			return nil, e
		}
		threads, e := office.WorkbookComments(p, spec)
		if e != nil {
			return nil, e
		}
		file := in[0]
		if edit {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			if e = p.SaveNew(out); e != nil {
				return nil, e
			}
			file = out
		}
		if threads == nil {
			threads = []office.Thread{}
		}
		return map[string]any{"file": file, "comments": threads, "native_excel_ui_verified": false}, nil
	}})
}
