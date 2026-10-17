package cli

import (
	"context"
	"github.com/openvibely/openvibely-paperworks/internal/office"
	"github.com/openvibely/openvibely-paperworks/internal/opc"
)

func init() {
	register(&Command{Name: "slide-comments", Summary: "List or edit native modern PowerPoint comments, text anchors, replies, resolution and reactions.", Usage: "paperworks slide-comments INPUT.pptx [--requirements COMMENT.json --output NEW.pptx]", Run: func(ctx context.Context, a Args) (any, error) {
		in, err := input(a, 1)
		if err != nil {
			return nil, err
		}
		if ext(in[0]) != ".pptx" {
			return nil, usagef("slide-comments requires PPTX")
		}
		var s office.SlideCommentSpec
		if a.Str("requirements") != "" {
			if err = a.JSON("requirements", &s); err != nil {
				return nil, err
			}
		}
		edit := s.Action != "" && s.Action != "list"
		out := a.Path("output")
		if edit {
			if err = newFile(out, in[0]); err != nil {
				return nil, err
			}
		} else if out != "" {
			return nil, usagef("list is read-only; omit --output")
		}
		p, err := opc.Open(in[0])
		if err != nil {
			return nil, err
		}
		r, err := office.SlideComments(p, s)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		file := in[0]
		if edit {
			if err = p.SaveNew(out); err != nil {
				return nil, err
			}
			file = out
		}
		return map[string]any{"file": file, "comments": r, "native_powerpoint_ui_verified": false}, nil
	}})
}
